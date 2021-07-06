package daemon

import (
	"time"

	"github.com/kubeovn/kube-ovn/pkg/ovs"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

// Run starts controller in no-ovn environment
func (c *Controller) RunWithoutOVN(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.addOrUpdateProviderNetworkQueue.ShutDown()
	defer c.deleteProviderNetworkQueue.ShutDown()
	defer c.subnetQueue.ShutDown()
	defer c.podQueue.ShutDown()

	go wait.Until(ovs.CleanLostInterface, time.Minute, stopCh)

	if ok := cache.WaitForCacheSync(stopCh, c.podsSynced); !ok {
		klog.Fatalf("failed to wait for caches to sync")
		return
	}

	klog.Info("Started workers")
	go wait.Until(c.runPodWorker, time.Second, stopCh)
	<-stopCh
	klog.Info("Shutting down workers")
}
