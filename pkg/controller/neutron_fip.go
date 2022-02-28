package controller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
	"github.com/kubeovn/kube-ovn/pkg/neutron"
	neutronv1 "github.com/kubeovn/kube-ovn/pkg/neutron/apis/neutron/v1"
	"github.com/kubeovn/kube-ovn/pkg/util"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
)

// NeutronRoutersByID makes an array of neutron routers sortable by their id in ascending order.
type NeutronRoutersByID []neutronv1.NeutronRouter

func (r NeutronRoutersByID) Len() int {
	return len(r)
}

func (r NeutronRoutersByID) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

func (r NeutronRoutersByID) Less(i, j int) bool {
	return r[i].NeutronRouterID < r[j].NeutronRouterID
}

func (c *NeutronController) runSyncFipWorker() func() {
	return func() {
		syncWorker("sync", c.syncFipQueue, c.handleSyncFip)
	}
}

func (c *Controller) syncFip() func() {
	return func() {
		// klog.Info("sync fip")
		// todo: syncFip 每3s执行一次，但是vpc并不是一个频繁更新的资源，可以考虑在vpc handler中主动调用
		vpcExternalNetworkMap := make(map[string][]*kubeovnv1.Vpc)

		vpcList, err := c.vpcsLister.List(labels.Everything())
		if err != nil {
			klog.Errorf("list vpc failed, err: %+v\n", err)
			return
		}

		for _, vpc := range vpcList {
			if _, ok := vpcExternalNetworkMap[vpc.Spec.ExternalNetwork]; ok {
				vpcExternalNetworkMap[vpc.Spec.ExternalNetwork] = append(vpcExternalNetworkMap[vpc.Spec.ExternalNetwork], vpc)
				continue
			}
			vpcExternalNetworkMap[vpc.Spec.ExternalNetwork] = []*kubeovnv1.Vpc{vpc}
		}

		for externalNetworkName, vpcs := range vpcExternalNetworkMap {
			externalNetwork, err := neutron.NewClient().GetNetworkFromName(externalNetworkName)
			if err != nil {
				klog.Errorf("get external network failed, err: %+v\n", err)
				return
			}
			// klog.Infof("get external network success, external network: %+v\n", externalNetwork)

			neutronRouters := genNeutronRouters(vpcs)
			// klog.Infof("gen neutron routers: %+v\n", neutronRouters)

			oldFip, err := c.neutronController.kubeNtrnCli.KubeovnV1().Fips().Get(context.TODO(), externalNetwork.ID, metav1.GetOptions{})
			if err == nil {
				// 如果fip cr已存在，则判断neutronRouters是否发生变更
				// 如果fip cr 所关联的 neutron routers 信息未发生变更，则不需要更新
				sort.Sort(NeutronRoutersByID(oldFip.Status.NeutronRouters))
				sort.Sort(NeutronRoutersByID(neutronRouters))
				if reflect.DeepEqual(oldFip.Status.NeutronRouters, neutronRouters) {
					// klog.Info("neutronRouters of fip deep equal, no sync required")
					continue
				}

				fipPatch := &neutronv1.FipPatch{
					Op:             "replace",
					Name:           oldFip.Name,
					Path:           "/status/neutronRouters",
					NeutronRouters: neutronRouters,
				}
				klog.Infof("add FipPatch to syncFipQueue, FipPatch: %+v\n", fipPatch)
				c.neutronController.syncFipQueue.Add(fipPatch)

				continue
			}

			allocationPools, err := getAllocationPools(externalNetwork.Subnets)
			if err != nil {
				klog.Errorf("get allocation pools from external network, err: %+v\n", err)
				return
			}
			// klog.Infof("get allocation pools success, allocation pools: %+v\n", allocationPools)

			newFip := genFipStruct(externalNetwork.ID, externalNetwork.Name, neutronRouters, allocationPools)
			fip, err := c.neutronController.kubeNtrnCli.KubeovnV1().Fips().Create(context.TODO(), newFip, metav1.CreateOptions{})
			if err != nil {
				klog.Warningf("create floating ip cr failure to api server, fip: %+v, err: %+v\n", newFip, err)
				continue
			}
			klog.Infof("create floating ip cr success to api server, fip: %+v\n", fip)
		}

		fips, err := neutron.NewClient().ListFip()
		if err != nil {
			klog.Errorf("list fip from neutron failed, err: %+v\n", err)
		}

		fipMap := getFloatingIPMap(fips)
		for fipName, forbiddenIPs := range fipMap {
			oldFip, err := c.neutronController.kubeNtrnCli.KubeovnV1().Fips().Get(context.TODO(), fipName, metav1.GetOptions{})
			if err == nil {
				sort.Sort(sort.StringSlice(oldFip.Status.ForbiddenIPs))
				sort.Sort(sort.StringSlice(forbiddenIPs))
				if reflect.DeepEqual(oldFip.Status.ForbiddenIPs, forbiddenIPs) {
					// klog.Info("forbiddenIPs of fip cr deep equal, no sync required")
					continue
				}
			}
			fipPatch := &neutronv1.FipPatch{
				Op:           "replace",
				Name:         fipName,
				Path:         "/status/forbiddenIPs",
				ForbiddenIPs: forbiddenIPs,
			}
			klog.Infof("add FipPatch to syncFipQueue, FipPatch: %+v\n", fipPatch)
			c.neutronController.syncFipQueue.Add(fipPatch)
		}
	}
}

func allocatedIPIsExist(allocatedIP neutronv1.AllocatedIP, allocatedIPs []neutronv1.AllocatedIP) bool {
	for _, ip := range allocatedIPs {
		if reflect.DeepEqual(allocatedIP, ip) {
			return true
		}
	}
	return false
}

// gcFip 回收 proton floatingip 资源
// 当用户用过 yaml 方式删除了pod eip&snat annotation，则会导致 fip cr 中的 floatingip 资源泄露，故需要定时检查fip cr并回收floatingip 资源。
func (c *Controller) gcFip() func() {
	return func() {
		klog.Info("gc fip")
		fipList, err := c.neutronController.fipsLister.List(labels.Everything())
		if err != nil {
			klog.Error("list fip failed")
			return
		}
		for _, fip := range fipList {
			for _, allocatedIP := range fip.Status.AllocatedIPs {
				if allocatedIP.Type == util.EipAnnotation {
					resources := strings.Split(allocatedIP.Resources[0], "/")
					if len(resources) != 2 {
						klog.Errorf("resource type error, allocatedIP: %+v\n", allocatedIP)
						return
					}
					_, err = c.podsLister.Pods(resources[0]).Get(resources[1])
					if k8serrors.IsNotFound(err) {
						time.Sleep(3 * time.Second)
						_, err = c.config.KubeClient.CoreV1().Pods(resources[0]).Get(context.TODO(), resources[1], metav1.GetOptions{})
						newfip, _ := c.neutronController.kubeNtrnCli.KubeovnV1().Fips().Get(context.TODO(), fip.Name, metav1.GetOptions{})
						if allocatedIPIsExist(allocatedIP, newfip.Status.AllocatedIPs) && k8serrors.IsNotFound(err) {
							// 删除 proton floatingip 资源
							err := neutron.NewClient().DeleteFipFromIP(allocatedIP.IP)
							if err != nil {
								klog.Errorf("delete floatingip from proton api failed, err: %+v\n", err)
								return
							}
							klog.Infof("delete floatingip from proton api success, floatingip: %s", allocatedIP.IP)
							fipPatch := &neutronv1.FipPatch{
								Op:          "del",
								Name:        fip.Name,
								Path:        "/status/allocatedIPs",
								AllocatedIP: allocatedIP,
							}
							klog.Infof("add FipPatch to syncFipQueue, FipPatch: %+v\n", fipPatch)
							c.neutronController.syncFipQueue.Add(fipPatch)
						}
					}
				}
			}
		}
		klog.Info("finish gc fip")
	}
}

func addAllocatedIP(fip *neutronv1.Fip, allocatedIP neutronv1.AllocatedIP) {
	for _, v := range fip.Status.AllocatedIPs {
		if reflect.DeepEqual(allocatedIP, v) {
			// klog.Infof("allocatedIP is already in fip and does not need to be added, allocatedIP: %+v\n", allocatedIP)
			return
		}
	}
	fip.Status.AllocatedIPs = append(fip.Status.AllocatedIPs, allocatedIP)
}

func delAllocatedIP(fip *neutronv1.Fip, allocatedIP neutronv1.AllocatedIP) {
	for k, v := range fip.Status.AllocatedIPs {
		if reflect.DeepEqual(allocatedIP, v) {
			fip.Status.AllocatedIPs = append(fip.Status.AllocatedIPs[:k], fip.Status.AllocatedIPs[k+1:]...)
			return
		}
	}
}

func addAllocatedIPResource(fip *neutronv1.Fip, allocatedIP neutronv1.AllocatedIP) {
	for k, v := range fip.Status.AllocatedIPs {
		if v.IP == allocatedIP.IP {
			for _, vv := range fip.Status.AllocatedIPs[k].Resources {
				if vv == allocatedIP.Resources[0] {
					// klog.Infof("allocatedIP's resource is already in fip and does not need to be added, allocatedIP: %+v\n", allocatedIP)
					return
				}
			}
			fip.Status.AllocatedIPs[k].Resources = append(fip.Status.AllocatedIPs[k].Resources, allocatedIP.Resources[0])
			return
		}
	}
}

func delAllocatedIPResource(fip *neutronv1.Fip, allocatedIP neutronv1.AllocatedIP) {
	for k, v := range fip.Status.AllocatedIPs {
		if v.IP == allocatedIP.IP {
			if reflect.DeepEqual(allocatedIP, v) {
				fip.Status.AllocatedIPs = append(fip.Status.AllocatedIPs[:k], fip.Status.AllocatedIPs[k+1:]...)
				return
			}
			for kk, vv := range fip.Status.AllocatedIPs[k].Resources {
				if vv == allocatedIP.Resources[0] {
					fip.Status.AllocatedIPs[k].Resources = append(fip.Status.AllocatedIPs[k].Resources[:kk], fip.Status.AllocatedIPs[k].Resources[kk+1:]...)
					return
				}
			}
		}
	}
}

func replaceNeutronRouters(fip *neutronv1.Fip, neutronRouters []neutronv1.NeutronRouter) {
	fip.Status.NeutronRouters = neutronRouters
}

func replaceForbiddenIPs(fip *neutronv1.Fip, forbiddenIPs []string) {
	fip.Status.ForbiddenIPs = forbiddenIPs
}

func (c *NeutronController) handleSyncFip(obj interface{}) error {
	klog.Info("handle sync fip")
	fipPacth, ok := obj.(*neutronv1.FipPatch)
	if !ok {
		klog.Error("obj type error")
		return errors.New("obj type error")
	}
	klog.Infof("handle sync fip, fipPacth: %+v\n", fipPacth)

	c.fipKeyMutex.Lock(fipPacth.Name)
	defer c.fipKeyMutex.Unlock(fipPacth.Name)

	// ***这里必须从apiserver获取fip，不能从client-go本地缓存获取，因为apiserver才是最新的数据，client-go本地缓存可能还没有同步完成，导致数据不一致***
	oldFip, err := c.kubeNtrnCli.KubeovnV1().Fips().Get(context.TODO(), fipPacth.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.Warningf("fip not found， fipName: %s\n", fipPacth.Name)
			return nil
		}
		return err
	}
	klog.Infof("handle sync fip, oldFip: %+v\n", oldFip)

	newFip := oldFip.DeepCopy()
	newFip.Status = *oldFip.Status.DeepCopy()

	switch fipPacth.Op {
	case "add":
		switch fipPacth.Path {
		case "/status/allocatedIPs":
			addAllocatedIP(newFip, fipPacth.AllocatedIP)
		case "/status/allocatedIPs/allocatedIP":
			addAllocatedIPResource(newFip, fipPacth.AllocatedIP)
		}
	case "del":
		switch fipPacth.Path {
		case "/status/allocatedIPs":
			delAllocatedIP(newFip, fipPacth.AllocatedIP)
		case "/status/allocatedIPs/allocatedIP":
			delAllocatedIPResource(newFip, fipPacth.AllocatedIP)
		}
	case "replace":
		switch fipPacth.Path {
		case "/status/neutronRouters":
			replaceNeutronRouters(newFip, fipPacth.NeutronRouters)
		case "/status/forbiddenIPs":
			replaceForbiddenIPs(newFip, fipPacth.ForbiddenIPs)
		}
	}

	klog.Infof("handle sync fip, newFip: %+v\n", newFip)

	if reflect.DeepEqual(oldFip.Status, newFip.Status) {
		klog.Info("fip status deep equal, no sync required")
		return nil
	}

	body, err := newFip.Status.Bytes()
	if err != nil {
		klog.Errorf("new floating ip convert format to byte failed, fip: %+v, err: %+v\n", newFip, err)
		return err
	}

	_, err = c.kubeNtrnCli.KubeovnV1().Fips().Patch(context.Background(), fipPacth.Name, types.MergePatchType, body, metav1.PatchOptions{}, "status")
	if err != nil {
		klog.Errorf("patch floating ip failed, name: %s, err: %+v\n", newFip.Name, err)
		return err
	}
	klog.Info("patch floating ip success")
	return nil
}

func (c *Controller) initFip() error {
	klog.Info("init fip")

	vpcExternalNetworkMap := make(map[string][]*kubeovnv1.Vpc)

	vpcList, err := c.vpcsLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("list vpc failed, err: %+v\n", err)
		return err
	}

	for _, vpc := range vpcList {
		klog.Infof("init fip, vpc: %+v\n", vpc)
		if _, ok := vpcExternalNetworkMap[vpc.Spec.ExternalNetwork]; ok {
			vpcExternalNetworkMap[vpc.Spec.ExternalNetwork] = append(vpcExternalNetworkMap[vpc.Spec.ExternalNetwork], vpc)
			continue
		}
		vpcExternalNetworkMap[vpc.Spec.ExternalNetwork] = []*kubeovnv1.Vpc{vpc}
	}

	for externalNetworkName, vpcs := range vpcExternalNetworkMap {
		externalNetwork, err := neutron.NewClient().GetNetworkFromName(externalNetworkName)
		if err != nil {
			klog.Errorf("get external network failed, err: %+v\n", err)
			return err
		}
		klog.Infof("get external network success, external network: %+v\n", externalNetwork)

		neutronRouters := genNeutronRouters(vpcs)
		klog.Infof("gen neutron routers success, neutronRouters: %+v\n", neutronRouters)

		allocationPools, err := getAllocationPools(externalNetwork.Subnets)
		if err != nil {
			klog.Errorf("get allocation pools from external network, err: %+v\n", err)
			return err
		}
		klog.Infof("get allocation pools success, allocation pools: %+v\n", allocationPools)

		newFip := genFipStruct(externalNetwork.ID, externalNetwork.Name, neutronRouters, allocationPools)

		_, err = c.neutronController.kubeNtrnCli.KubeovnV1().Fips().Get(context.TODO(), externalNetwork.ID, metav1.GetOptions{})
		if err == nil {
			klog.Infof("the fip cr in this external network has created, name: %s\n", externalNetwork.Name)
			continue
		}

		fip, err := c.neutronController.kubeNtrnCli.KubeovnV1().Fips().Create(context.TODO(), newFip, metav1.CreateOptions{})
		if err != nil {
			klog.Warningf("create floating ip cr failure to api server, fip: %+v, err: %+v\n", newFip, err)
			continue
		}
		klog.Infof("create fip cr success to api server, fip: %+v\n", fip)
	}
	return nil
}

// getFloatingIPMap 该函数将 proton fip 资源转换为 map 格式，key 为外部网络ID，value为 fip 列表
func getFloatingIPMap(fips []floatingips.FloatingIP) map[string][]string {
	var (
		fipMap = make(map[string][]string)
	)
	for _, fip := range fips {
		if _, ok := fipMap[fip.FloatingNetworkID]; ok {
			fipMap[fip.FloatingNetworkID] = append(fipMap[fip.FloatingNetworkID], fip.FloatingIP)
			continue
		}
		fipMap[fip.FloatingNetworkID] = []string{fip.FloatingIP}
	}
	return fipMap
}

// gen neutron routers
func genNeutronRouters(vpcs []*kubeovnv1.Vpc) []neutronv1.NeutronRouter {
	var (
		newtronRouters []neutronv1.NeutronRouter
	)

	for _, vpc := range vpcs {
		newtronRouter := neutronv1.NeutronRouter{
			NeutronRouterID:   vpc.Spec.NeutronRouter,
			NeutronRouterName: vpc.Name,
			AvailabilityZone:  vpc.Spec.AvailabilityZone,
			ExternalGatewayIP: vpc.Spec.ExternalGatewayIP,
			Subnets:           vpc.Status.Subnets,
		}
		// 如果vpc的subnets为nil，需要设置为空字符串数组，否则创建fip资源会失败
		if vpc.Status.Subnets == nil {
			newtronRouter.Subnets = []string{}
		}
		newtronRouters = append(newtronRouters, newtronRouter)
	}

	return newtronRouters
}

// getAllocationPools 该函数将外部网络下所有子网的 AllocationPool 合并返回
func getAllocationPools(subnets []string) ([]neutronv1.AllocationPool, error) {
	var (
		result []neutronv1.AllocationPool
	)

	if subnets == nil {
		klog.Warningf("subnets of external network is nil")
		return nil, errors.New("subnets of external network is nil")
	}

	// get external gateway network from proton api
	for _, sn := range subnets {
		// get subnet from proton api
		subnet, err := neutron.NewClient().GetSubnet(sn)
		if err != nil {
			klog.Errorf("get subnet from proton api failed, err: %+v\n", err)
			return nil, err
		}

		for _, allocationPool := range subnet.AllocationPools {
			pool := neutronv1.AllocationPool{
				Start: allocationPool.Start,
				End:   allocationPool.End,
			}
			result = append(result, pool)
		}
	}
	return result, nil
}

// genFipStruct 该函数用于生成 fip cr 数据结构
func genFipStruct(externalNetworkID, externalNetworkName string, neutronRouters []neutronv1.NeutronRouter, allocationPools []neutronv1.AllocationPool) *neutronv1.Fip {
	fip := &neutronv1.Fip{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Fip",
			APIVersion: "neutron.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: externalNetworkID,
		},
		Spec: neutronv1.FipSpec{
			ExternalNetworkID:   externalNetworkID,
			ExternalNetworkName: externalNetworkName,
			AllocationPools:     allocationPools,
		},
		Status: neutronv1.FipStatus{
			NeutronRouters: neutronRouters,
			AllocatedIPs:   []neutronv1.AllocatedIP{},
			ForbiddenIPs:   []string{},
		},
	}
	return fip
}

// handleFip 处理 fip，在 pod 创建/删除时调用
// 如果 Pod 的注解上有 "ovn.kubernetes.io/eip" 或 "ovn.kubernetes.io/snat"，则根据注解中的ip地址创建 neutron fip 资源，并更新 fip cr status
func (c *Controller) handleFip(op string, pod *corev1.Pod) error {
	klog.Infof("handle fip, op: %s\n", op)

	var (
		eip           string
		snat          string
		projectID     string
		logicalRouter string
	)

	if _, ok := pod.Annotations[util.EipAnnotation]; ok {
		eip = pod.Annotations[util.EipAnnotation]
	}
	if _, ok := pod.Annotations[util.SnatAnnotation]; ok {
		snat = pod.Annotations[util.SnatAnnotation]
	}
	if eip == "" && snat == "" {
		klog.Infof("eip and snat annotation not found, no handle")
		return nil
	}
	if eip == snat {
		klog.Error("not support eip is the same as snat")
		return errors.New("not support eip is the same as snat")
	}

	if _, ok := pod.Annotations[util.ProjectIDAnnotation]; ok {
		projectID = pod.Annotations[util.ProjectIDAnnotation]
	}
	if projectID == "" {
		klog.Error("project_id annotation not found")
		return errors.New("project_id annotation not found")
	}

	if _, ok := pod.Annotations[util.LogicalRouterAnnotation]; ok {
		logicalRouter = pod.Annotations[util.LogicalRouterAnnotation]
	}
	if logicalRouter == "" {
		klog.Error("logical_router annotation not found")
		return errors.New("logical_router annotation not found")
	}

	klog.Infof("get pod annotations, eip: %s, snat: %s, projectID: %s, logicalRouter: %s\n", eip, snat, projectID, logicalRouter)

	vpc, err := c.vpcsLister.Get(logicalRouter)
	if err != nil {
		klog.Errorf("get vpc failed, name: %s, err: %+v\n", logicalRouter, err)
		return err
	}

	externalNetwork, err := neutron.NewClient().GetNetworkFromName(vpc.Spec.ExternalNetwork)
	if err != nil {
		klog.Errorf("get external network failed, name: %s, err: %+v\n", vpc.Spec.ExternalNetwork, err)
		return err
	}

	oldFip, err := c.neutronController.fipsLister.Get(externalNetwork.ID)
	if err != nil {
		klog.Errorf("get fip cr failed, name: %s\n", externalNetwork.ID)
		return err
	}
	klog.Infof("handle fip, oldFip: %+v\n", oldFip)

	resource := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)

	switch op {
	case "add":
		// 判断eip是否已经被其他pod占用
		if isOtherPodEipAllocated(eip, resource, oldFip) {
			return errors.New("eip has been allocated")
		}

		// 判断eip是否可用
		if isAvailable(eip, oldFip) {
			// 创建 proton floatingip 资源
			floatingip, err := neutron.NewClient().CreateFip(externalNetwork.ID, eip, projectID)
			if err != nil {
				klog.Errorf("create floatingip from proton api failed, err: %+v\n", err)
				return err
			}
			klog.Infof("create floatingip from proton api success, floatingip: %+v\n", floatingip)

			// 给 proton floatingip 资源打上 'container' tag
			err = neutron.NewClient().AddTag("floatingips", floatingip.ID, util.NeutronFipTag)
			if err != nil {
				klog.Errorf("add floating ip tag failed, err: %+v\n", err)
				return err
			}

			// 这里需要修改 fip cr status，生成对应的 fip cr patch 内容
			fipPatch := genEipPatch(op, eip, resource, oldFip)
			klog.Infof("add FipPatch to syncFipQueue, FipPatch: %+v\n", fipPatch)
			c.neutronController.syncFipQueue.Add(fipPatch)
		}

		// 判断是否配置了 pod snat 注释
		if snat != "" {
			// 判断snat是否仍处于空闲状态
			// 如果snat是第一次被申请，则需要创建proton floatingip资源
			// 如果snat已经被其他pod使用，则共享proton floatingip资源，不需要创建
			if isAvailable(snat, oldFip) {
				// 创建 proton floatingip 资源
				floatingip, err := neutron.NewClient().CreateFip(externalNetwork.ID, snat, projectID)
				if err != nil {
					klog.Errorf("create fip from proton api failed, err: %+v\n", err)
					return err
				}
				klog.Infof("create floatingip from proton api success, floatingip: %+v\n", floatingip)

				// 给 proton floatingip 资源打上 'container' tag
				err = neutron.NewClient().AddTag("floatingips", floatingip.ID, util.NeutronFipTag)
				if err != nil {
					klog.Errorf("add floating ip tag failed, err: %+v\n", err)
					return err
				}
			}

			// 这里需要修改 fip cr status，生成对应的 fip cr patch 内容
			fipPatch := genSnatPatch(op, snat, resource, oldFip)
			klog.Infof("add FipPatch to syncFipQueue, FipPatch: %+v\n", fipPatch)
			c.neutronController.syncFipQueue.Add(fipPatch)
		}
	case "del":
		// 判断eip是否正在被当前pod使用
		if isPodEipAllocated(eip, resource, oldFip) {
			// 删除 proton floatingip 资源
			err := neutron.NewClient().DeleteFipFromIP(eip)
			if err != nil {
				klog.Errorf("delete floatingip from proton api failed, err: %+v\n", err)
				return err
			}
			klog.Infof("delete floatingip from proton api success, floatingip: %s", eip)

			// 这里需要修改 fip cr status，生成对应的 fip cr patch 内容
			fipPatch := genEipPatch(op, eip, resource, oldFip)
			klog.Infof("add FipPatch to syncFipQueue, FipPatch: %+v\n", fipPatch)
			c.neutronController.syncFipQueue.Add(fipPatch)
		}

		// 判断snat是否正在被当前pod使用
		if isSnatAllocated(snat, resource, oldFip) {
			// 判断snat是否正在被当前pod唯一使用
			// 若snat被当前pod唯一使用，则pod删除后需要同步删除对应proton floatingip资源
			if isSnatUniqueAllocated(snat, resource, oldFip) {
				// 删除 proton floatingip 资源
				err := neutron.NewClient().DeleteFipFromIP(snat)
				if err != nil {
					klog.Errorf("delete floatingip from proton api failed, err: %+v\n", err)
					return err
				}
				klog.Infof("delete floatingip from proton api success, floatingip: %s", snat)
			}

			// 这里需要修改 fip cr status，生成对应的 fip cr patch 内容
			fipPatch := genSnatPatch(op, snat, resource, oldFip)
			klog.Infof("add FipPatch to syncFipQueue, FipPatch: %+v\n", fipPatch)
			c.neutronController.syncFipQueue.Add(fipPatch)
		}
	default:
		klog.Fatalf("%s is an unknown operator", op)
	}
	klog.Info("finish handle fip")
	return nil
}

// isAvailable 判断 floatingIP 资源是否可用
func isAvailable(floatingIP string, fip *neutronv1.Fip) bool {
	var (
		inPool        bool = false
		isAvailable   bool = true
		floatingIPLen int  = len(floatingIP)
		startIPLen    int  = 0
		endIPLen      int  = 0
	)

	// floatingIP 如果为空，则表示不可用，返回 false
	if floatingIP == "" {
		return false
	}

	// 校验 floatingIP 是否在 allocation pool 中
	for _, allocationPool := range fip.Spec.AllocationPools {
		if strings.Compare(floatingIP, allocationPool.Start) == 0 || strings.Compare(floatingIP, allocationPool.End) == 0 {
			inPool = true
			break
		}
		startIPLen = len(allocationPool.Start)
		endIPLen = len(allocationPool.End)
		isGreaterThanStart := (floatingIPLen > startIPLen) || ((floatingIPLen == startIPLen) && (strings.Compare(floatingIP, allocationPool.Start) == 1))
		isLessThanEnd := (floatingIPLen < endIPLen) || ((floatingIPLen == endIPLen) && (strings.Compare(floatingIP, allocationPool.End) == -1))
		if isGreaterThanStart && isLessThanEnd {
			inPool = true
			break
		}
	}

	// 校验 floatingIP 是否已经被其他容器占用
	for _, allocatedIP := range fip.Status.AllocatedIPs {
		if floatingIP == allocatedIP.IP {
			isAvailable = false
			break
		}
	}

	// 校验 floatingIP 是否已经被虚机占用
	for _, forbiddenIP := range fip.Status.ForbiddenIPs {
		if floatingIP == forbiddenIP {
			isAvailable = false
			break
		}
	}

	// 如果 floatingIP 既在 allocation pool 中，也并未被占用，则表示可用，返回 true
	if inPool && isAvailable {
		return true
	}
	return false
}

// isPodEipAllocated 判断 floatingIP 资源是否被 pod 占用
func isPodEipAllocated(floatingIP string, podName string, fip *neutronv1.Fip) bool {
	if floatingIP == "" {
		return false
	}

	eipAllocatedIP := neutronv1.AllocatedIP{
		IP:        floatingIP,
		Type:      util.EipAnnotation,
		Resources: []string{podName},
	}

	for _, allocatedIP := range fip.Status.AllocatedIPs {
		if reflect.DeepEqual(allocatedIP, eipAllocatedIP) {
			return true
		}
	}
	return false
}

// isPodEipAllocated 判断 floatingIP 资源是否已经被其他 pod 占用
func isOtherPodEipAllocated(floatingIP string, podName string, fip *neutronv1.Fip) bool {
	if floatingIP == "" {
		return false
	}

	for _, allocatedIP := range fip.Status.AllocatedIPs {
		if allocatedIP.IP == floatingIP && !reflect.DeepEqual(allocatedIP.Resources, []string{podName}) {
			return true
		}
	}
	return false
}

// isSnatAllocated 判断 floatingIP 资源是否被 pod 占用
func isSnatAllocated(floatingIP string, podName string, fip *neutronv1.Fip) bool {
	if floatingIP == "" {
		return false
	}

	for _, allocatedIP := range fip.Status.AllocatedIPs {
		if floatingIP == allocatedIP.IP {
			for _, pod := range allocatedIP.Resources {
				if pod == podName {
					return true
				}
			}
		}
	}
	return false
}

// isSnatUniqueAllocated 判断 floatingIP 资源是否被 pod 唯一占用
func isSnatUniqueAllocated(floatingIP string, podName string, fip *neutronv1.Fip) bool {
	if floatingIP == "" {
		return false
	}

	snatAllocatedIP := neutronv1.AllocatedIP{
		IP:        floatingIP,
		Type:      util.SnatAnnotation,
		Resources: []string{podName},
	}

	for _, allocatedIP := range fip.Status.AllocatedIPs {
		if reflect.DeepEqual(allocatedIP, snatAllocatedIP) {
			return true
		}
	}

	return false
}

// genEipPatch 生成 eip 的 fip cr patch
func genEipPatch(op string, eip string, podName string, fip *neutronv1.Fip) *neutronv1.FipPatch {
	var (
		fipPatch     *neutronv1.FipPatch   = new(neutronv1.FipPatch)
		allocationIP neutronv1.AllocatedIP = neutronv1.AllocatedIP{
			IP:        eip,
			Type:      util.EipAnnotation,
			Resources: []string{podName},
		}
	)
	fipPatch.Op = op
	fipPatch.Name = fip.Name
	fipPatch.Path = "/status/allocatedIPs"
	fipPatch.AllocatedIP = allocationIP
	return fipPatch
}

// genSnatPatch 生成 snat 的 fip cr patch
func genSnatPatch(op string, snat string, podName string, fip *neutronv1.Fip) *neutronv1.FipPatch {
	var (
		ExistSnat    bool                  = false
		fipPatch     *neutronv1.FipPatch   = new(neutronv1.FipPatch)
		allocationIP neutronv1.AllocatedIP = neutronv1.AllocatedIP{
			IP:        snat,
			Type:      util.SnatAnnotation,
			Resources: []string{podName},
		}
	)
	fipPatch.Op = op
	fipPatch.Name = fip.Name
	for _, value := range fip.Status.AllocatedIPs {
		if value.IP != snat {
			continue
		}
		ExistSnat = true
		if op == "add" {
			fipPatch.Path = "/status/allocatedIPs/allocatedIP"
			fipPatch.AllocatedIP = allocationIP
			break
		}
		if op == "del" && len(value.Resources) == 1 {
			fipPatch.Path = "/status/allocatedIPs"
			fipPatch.AllocatedIP = allocationIP
			break
		} else if op == "del" && len(value.Resources) > 1 {
			fipPatch.Path = "/status/allocatedIPs/allocatedIP"
			fipPatch.AllocatedIP = allocationIP
			break
		}
	}

	if op == "add" && !ExistSnat {
		fipPatch.Path = "/status/allocatedIPs"
		fipPatch.AllocatedIP = allocationIP
	}
	return fipPatch
}
