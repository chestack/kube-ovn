package controller

import (
	"context"
	"errors"
	"fmt"

	neutronv1 "github.com/kubeovn/kube-ovn/pkg/neutron/apis/neutron/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

func (c *NeutronController) runAddPortWorker() func() {
	return func() {
		runWorker("add", c.addPortQueue, c.handleAddPort)
	}
}

func (c *NeutronController) runDeletePortWorker() func() {
	return func() {
		runWorker("delete", c.deletePortQueue, c.handleDeletePort)
	}
}

func (c *NeutronController) runUpdatePortWorker() func() {
	return func() {
		runWorker("update", c.updatePortQueue, c.handleUpdatePort)
	}
}

func (c *NeutronController) handleAddPort(obj interface{}) error {
	key := obj.(string)
	c.portKeyMutex.Lock(key)
	defer c.portKeyMutex.Unlock(key)

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	port, err := c.portsLister.Ports(ns).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	//TODO: projectID, sgs 改
	p, err := c.ntrnCli.CreatePort(key, "", port.Spec.NetworkID, port.Spec.SubnetID, port.Spec.IP, "")
	if err != nil {
		klog.Errorf("creating port error %v", err)
		return err
	}

	if port.Spec.IP == "" {
		port.Spec.IP = p.IP
	}
	if port.Spec.MAC == "" {
		port.Spec.MAC = p.MAC
	}
	if len(port.Spec.SecurityGroupID) == 0 {
		port.Spec.SecurityGroupID = p.Sgs
	}

	err = c.updatePort(port)
	if err != nil {
		// TODO: 要回滚创建
		klog.Errorf("updating port error: %v", err)
	}

	port.Status.ID = p.ID
	port.Status.CIDR = p.CIDR
	port.Status.Gateway = p.Gateway
	port.Status.MTU = p.MTU

	err = c.patchPortStatus(port)
	if err != nil {
		klog.Errorf("updating port status error: %v", err)
	}

	return nil
}

func (c *NeutronController) handleDeletePort(obj interface{}) error {
	port := obj.(*neutronv1.Port)
	err := c.ntrnCli.DeletePort(port.Status.ID)
	if err != nil {
		return err
	}
	return errors.New("not implemented yet")
}

func (c *NeutronController) handleUpdatePort(obj interface{}) error {
	key := obj.(string)
	_ = key
	return errors.New("not implemented yet")
}

func (c *NeutronController) updatePort(port *neutronv1.Port) error {
	_, err := c.kubeNtrnCli.KubeovnV1().Ports(port.Namespace).Update(context.Background(), port, metav1.UpdateOptions{})
	return err
}

func (c *NeutronController) patchPortStatus(port *neutronv1.Port) error {
	b, err := port.Status.Bytes()
	if err != nil {
		return err
	}
	_, err = c.kubeNtrnCli.KubeovnV1().Ports(port.Namespace).Patch(context.Background(),
		port.Name, types.MergePatchType, b, metav1.PatchOptions{}, "status")
	return err

}
