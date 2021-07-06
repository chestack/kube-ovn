package controller

import (
	"fmt"
	"time"

	"github.com/kubeovn/kube-ovn/pkg/neutron"
	neutronv1 "github.com/kubeovn/kube-ovn/pkg/neutron/apis/neutron/v1"
	lister "github.com/kubeovn/kube-ovn/pkg/neutron/client/listers/neutron/v1"
	"github.com/neverlee/keymutex"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
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

	portsLister lister.PortLister
	portsSyncd  cache.InformerSynced

	portKeyMutex *keymutex.KeyMutex

	addPortQueue    workqueue.RateLimitingInterface
	deletePortQueue workqueue.RateLimitingInterface
	updatePortQueue workqueue.RateLimitingInterface

	informerFactory informer.SharedInformerFactory
}

func MustNewNeutronController(config *Configuration) *NeutronController {
	utilruntime.Must(neutronv1.AddToScheme(scheme.Scheme))
	kubeNtrnCli, err := clientset.NewForConfig(config.KubeRestConfig)
	utilruntime.Must(err)

	informerFactory := informer.NewSharedInformerFactoryWithOptions(kubeNtrnCli, 0,
		informer.WithTweakListOptions(func(listOption *metav1.ListOptions) {
			listOption.AllowWatchBookmarks = true
		}))

	portsInformer := informerFactory.Kubeovn().V1().Ports()

	c := &NeutronController{
		ntrnCli:     neutron.NewClient(),
		kubeNtrnCli: kubeNtrnCli,
		kubeCli:     config.KubeClient,

		portsLister: portsInformer.Lister(),
		portsSyncd:  portsInformer.Informer().HasSynced,

		portKeyMutex: keymutex.New(97),

		addPortQueue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "AddPort"),
		deletePortQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "DeletePort"),
		updatePortQueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "UpdatePort"),

		informerFactory: informerFactory,
	}

	portsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueue(c.addPortQueue, "addPortQueue", true),
		DeleteFunc: c.enqueue(c.deletePortQueue, "deletePortQueue", false),
		UpdateFunc: c.enqueue2(c.updatePortQueue, "updatePortQueue", true),
	})

	return c
}

// enqueue 返回将 k8s 资源放进相关队列的闭包
// keyOnly: 决定是放 命名空间/资源名称 的key，还是资源实例。当删除一个资源时，用 key 查不到，只能放实例
func (c *NeutronController) enqueue(q workqueue.Interface, qName string, keyOnly bool) func(interface{}) {
	return func(obj interface{}) {
		if !c.isLeader() {
			return
		}
		key, err := cache.MetaNamespaceKeyFunc(obj)
		if err != nil {
			utilruntime.HandleError(err)
			return
		}

		klog.Infof("enqueue %s to %s", key, qName)
		if keyOnly {
			q.Add(key)
		} else {
			q.Add(obj)
		}
	}
}

func (c *NeutronController) enqueue2(q workqueue.Interface, qName string, keyOnly bool) func(old, new interface{}) {
	return func(old, new interface{}) {
		var enqueue bool
		oldPort, ok := old.(*neutronv1.Port)
		newPort, ok1 := new.(*neutronv1.Port)
		if ok && ok1 {
			if oldPort.ResourceVersion != newPort.ResourceVersion {
				enqueue = true
			}
			if !newPort.DeletionTimestamp.IsZero() {
				enqueue = false
			}
		}

		// 如果未来加入了新的资源类型，先尝试转换类型再逻辑判断，最后决定添加与否
		if enqueue {
			c.enqueue(q, qName, keyOnly)(new)
		}
	}
}

func (c *NeutronController) run(stopCh <-chan struct{}) {

	c.informerFactory.Start(stopCh)

	klog.Info("wait for neutron informers to sync")

	if ok := cache.WaitForCacheSync(stopCh, c.portsSyncd); !ok {
		klog.Fatal("neutron informer failed to sync")
	}

	for i := 0; i < 5; i++ {
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
				utilruntime.HandleError(fmt.Errorf("expected string or metav1.Object in workqueue but got %#v", obj))
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
