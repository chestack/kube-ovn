package controller

import (
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

func (c *Controller) RunWithoutOVN(stopCh <-chan struct{}) {
	defer c.shutdown()
	klog.Info("Starting OVN controller without OVN")

	// wait for becoming a leader
	c.leaderElection()

	// Wait for the caches to be synced before starting workers
	c.informerFactory.Start(stopCh)
	c.cmInformerFactory.Start(stopCh)
	//c.kubeovnInformerFactory.Start(stopCh)

	klog.Info("Waiting for informer caches to sync")
	cacheSyncs := []cache.InformerSynced{
		//c.ipSynced,
		c.podsSynced,
	}

	if ok := cache.WaitForCacheSync(stopCh, cacheSyncs...); !ok {
		klog.Fatalf("failed to wait for caches to sync")
	}

	// remove resources in ovndb that not exist any more in kubernetes resources
	//	if err := c.gc(); err != nil {
	//		klog.Fatalf("gc failed %v", err)
	//	}

	//	c.registerSubnetMetrics()
	//	if err := c.initSyncCrdIPs(); err != nil {
	//		klog.Errorf("failed to sync crd ips %v", err)
	//	}
	//	if err := c.initSyncCrdSubnets(); err != nil {
	//		klog.Errorf("failed to sync crd subnets %v", err)
	//	}
	//	if err := c.initSyncCrdVlans(); err != nil {
	//		klog.Errorf("failed to sync crd vlans: %v", err)
	//	}

	// start workers to do all the network operations
	c.startWorkersWithoutOVN(stopCh)
	c.neutronController.run(stopCh)
	<-stopCh
	klog.Info("Shutting down workers")

}

func (c *Controller) startWorkersWithoutOVN(stopCh <-chan struct{}) {
	klog.Info("Starting workers without OVN")

	//	for {
	//		ready := true
	//		time.Sleep(3 * time.Second)
	//		nodes, err := c.nodesLister.List(labels.Everything())
	//		if err != nil {
	//			klog.Fatalf("failed to list nodes, %v", err)
	//		}
	//		for _, node := range nodes {
	//			if node.Annotations[util.AllocatedAnnotation] != "true" {
	//				klog.Infof("wait node %s annotation ready", node.Name)
	//				ready = false
	//				break
	//			}
	//		}
	//		if ready {
	//			break
	//		}
	//	}

	for i := 0; i < c.config.WorkerNum; i++ {
		go wait.Until(c.runAddPodWorker, time.Second, stopCh)
		go wait.Until(c.runDeletePodWorker, time.Second, stopCh)
		go wait.Until(c.runUpdatePodWorker, time.Second, stopCh)
	}
}
