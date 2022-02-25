package controller

import (
	"fmt"
	"reflect"
	"time"

	"github.com/kubeovn/kube-ovn/pkg/neutron"
	neutronv1 "github.com/kubeovn/kube-ovn/pkg/neutron/apis/neutron/v1"
	lister "github.com/kubeovn/kube-ovn/pkg/neutron/client/listers/neutron/v1"
	"github.com/neverlee/keymutex"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	clientset "github.com/kubeovn/kube-ovn/pkg/neutron/client/clientset/versioned"
	informer "github.com/kubeovn/kube-ovn/pkg/neutron/client/informers/externalversions"
)

type NeutronController struct {
	isLeader func() bool

	ntrnCli     *neutron.Client
	kubeNtrnCli clientset.Interface
	kubeCli     k8s.Interface

	fipsLister lister.FipLister
	fipsSyncd  cache.InformerSynced

	portsLister lister.PortLister
	portsSyncd  cache.InformerSynced

	fipKeyMutex  *keymutex.KeyMutex
	portKeyMutex *keymutex.KeyMutex

	syncFipQueue workqueue.RateLimitingInterface

	addPortQueue    workqueue.RateLimitingInterface
	deletePortQueue workqueue.RateLimitingInterface
	updatePortQueue workqueue.RateLimitingInterface

	informerFactory informer.SharedInformerFactory
}

func MustNewNeutronController(config *Configuration) *NeutronController {
	kubeNtrnCli := neutron.NewClientset(config.KubeRestConfig)

	informerFactory := informer.NewSharedInformerFactoryWithOptions(kubeNtrnCli, 0,
		informer.WithTweakListOptions(func(listOption *metav1.ListOptions) {
			listOption.AllowWatchBookmarks = true
		}))

	portsInformer := informerFactory.Kubeovn().V1().Ports()
	fipsInformer := informerFactory.Kubeovn().V1().Fips()

	c := &NeutronController{
		ntrnCli:     neutron.NewClient(),
		kubeNtrnCli: kubeNtrnCli,
		kubeCli:     config.KubeClient,

		fipsLister: fipsInformer.Lister(),
		fipsSyncd:  fipsInformer.Informer().HasSynced,

		portsLister: portsInformer.Lister(),
		portsSyncd:  portsInformer.Informer().HasSynced,

		fipKeyMutex:  keymutex.New(97),
		portKeyMutex: keymutex.New(97),

		syncFipQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "SyncFip"),

		addPortQueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "AddPort"),
		deletePortQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "DeletePort"),
		updatePortQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "UpdatePort"),

		informerFactory: informerFactory,
	}

	portsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueAddPort,
		DeleteFunc: c.enqueueDelPort,
		UpdateFunc: c.enqueueUpdatePort,
	})

	return c
}

func (c *NeutronController) enqueueAddPort(obj interface{}) {
	if !c.isLeader() {
		return
	}

	port := obj.(*neutronv1.Port)
	if port.Status.IsConditionTrue(neutronv1.ConditionCreated) {
		return
	}

	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}

	klog.Infof("enqueue %s to %s", key, "addPortQueue")
	c.addPortQueue.Add(obj)
}

func (c *NeutronController) enqueueDelPort(obj interface{}) {
	if !c.isLeader() {
		return
	}
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}

	klog.Infof("enqueue %s to %s", key, "delPortQueue")
	c.deletePortQueue.Add(obj)
}

func (c *NeutronController) enqueueUpdatePort(old, new interface{}) {
	var recnsl bool
	oldPort, ok := old.(*neutronv1.Port)
	newPort, ok1 := new.(*neutronv1.Port)
	if ok && ok1 {
		if oldPort.ResourceVersion != newPort.ResourceVersion {
			recnsl = true
		}
		if !newPort.DeletionTimestamp.IsZero() {
			recnsl = false
		}
	}
	if reflect.DeepEqual(oldPort.Spec, newPort.Spec) {
		recnsl = false
	}

	if recnsl {
		klog.Infof("enqueue %s to %s", newPort.Name, "updatePortQueue")
		c.updatePortQueue.Add(new)
	}
}

func (c *NeutronController) run(stopCh <-chan struct{}) {

	c.informerFactory.Start(stopCh)

	klog.Info("wait for neutron informers to sync")

	if ok := cache.WaitForCacheSync(stopCh, c.fipsSyncd, c.portsSyncd); !ok {
		klog.Fatal("neutron informer failed to sync")
	}

	for i := 0; i < 5; i++ {
		go wait.Until(c.runSyncFipWorker(), time.Second, stopCh)

		go wait.Until(c.runAddPortWorker(), time.Second, stopCh)
		go wait.Until(c.runDeletePortWorker(), time.Second, stopCh)
		go wait.Until(c.runUpdatePortWorker(), time.Second, stopCh)
	}

	go func() {
		<-stopCh
		c.shutdown()
	}()

}

func (c *NeutronController) shutdown() {
	klog.Info("shutting down Neutron controller")

	utilruntime.HandleCrash()

	c.syncFipQueue.ShutDown()

	c.addPortQueue.ShutDown()
	c.deletePortQueue.ShutDown()
	c.updatePortQueue.ShutDown()
}

// runWorker 抽象了从队列中取出元素，再由handle函数处理的逻辑
func runWorker(action string, q workqueue.RateLimitingInterface, handle func(interface{}) error) {
	for func() bool {
		obj, shutdown := q.Get()
		if shutdown {
			return false
		}
		now := time.Now()

		err := func(obj interface{}) error {
			defer q.Done(obj)

			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				q.Forget(obj)
				utilruntime.HandleError(fmt.Errorf("expected string or metav1.Object in workqueue but got %#v, err: %v", obj, err))
				return nil
			}

			klog.Infof("worker handles %s neutron port %s", action, key)

			if err := handle(obj); err != nil {
				q.AddRateLimited(obj)
				return fmt.Errorf("%s '%s' error: %s, requeuing", action, key, err.Error())
			}

			last := time.Since(now)
			klog.Infof("takes %d ms to %s neutron port %s", last.Milliseconds(), action, key)
			q.Forget(obj)
			return nil
		}(obj)

		if err != nil {
			utilruntime.HandleError(err)
		}
		return true
	}() {
		// do nothing inside for-loop
	}
}

func syncWorker(action string, q workqueue.RateLimitingInterface, handle func(interface{}) error) {
	for func() bool {
		obj, shutdown := q.Get()
		if shutdown {
			return false
		}
		now := time.Now()

		err := func(obj interface{}) error {
			defer q.Done(obj)

			klog.Infof("worker handles %s neutron fip", action)

			if err := handle(obj); err != nil {
				q.AddRateLimited(obj)
				return fmt.Errorf("%s error: %s, requeuing", action, err.Error())
			}

			last := time.Since(now)
			klog.Infof("takes %d ms to %s neutron fip", last.Milliseconds(), action)
			q.Forget(obj)
			return nil
		}(obj)

		if err != nil {
			utilruntime.HandleError(err)
		}
		return true
	}() {
		// do nothing inside for-loop
	}
}
