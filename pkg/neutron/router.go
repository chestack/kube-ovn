package neutron

import (
	"github.com/gophercloud/gophercloud/openstack/identity/v3/projects"
	tags "github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/attributestags"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/external"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/routers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/kubeovn/kube-ovn/pkg/util"
	"k8s.io/klog"
)

func (c Client) CreateRouter(name string, externalNet string) (string, error) {

	tenantID, err := c.getAdminTenantID()
	if err != nil || tenantID == ""{
		klog.Errorf("failed to get tenantID of admin %v", err)
		return "", err
	}
	klog.Infof("Admin tenantID is %s", tenantID)

	opts := routers.CreateOpts{
		Name: name,
		Description: "Router used for containers",
		TenantID: tenantID,
	}

	if externalNet != "" {
		netId, err := c.getExternalNetworkID(externalNet)
		if err != nil {
			klog.Errorf("failed to get external net id %v", err)
			return "", err
		}
		iTrue := true
		gateway := routers.GatewayInfo {
			NetworkID: netId,
			EnableSNAT: &iTrue,
		}
		opts.GatewayInfo = &gateway
	}

	r, err := routers.Create(c.networkCliV2, opts).Extract()
	if err != nil {
		klog.Errorf("failed to create router %v", err)
		return "", err
	}

	err = c.AddRouterTags(r.ID, util.NeutronRouterTag)
	if err != nil {
		klog.Errorf("failed to add tag to router: %v", err)
		return "", err
	}

	return r.ID, err
}

func (c Client) GetRouter(id string) (string, error){
	r, err := routers.Get(c.networkCliV2, id).Extract()
	return r.ID, err
}

func (c Client) getExternalNetworkID(networkName string) (string, error) {
	iTrue := true
	networkListOpts := networks.ListOpts{}
	listOpts := external.ListOptsExt{
		ListOptsBuilder: networkListOpts,
		External: &iTrue,
	}

	type NetworkWithExternalExt struct {
		networks.Network
		external.NetworkExternalExt
	}

	var allNetworks []NetworkWithExternalExt

	allPages, err := networks.List(c.networkCliV2, listOpts).AllPages()
	if err != nil {
		return "", err
	}

	err = networks.ExtractNetworksInto(allPages, &allNetworks)
	if err != nil {
		return "", err
	}

	id := ""
	for _, network := range allNetworks {
		if network.Name == networkName {
			id = network.ID
			break
		}
	}
	klog.Infof("neutron external network name is %s, id is %s", networkName, id)
	return  id, nil
}

func (c Client) getAdminTenantID() (string, error) {

	allPages, err := projects.List(c.identityCliV3, projects.ListOpts{}).AllPages()
	if err != nil {
		return "", err
	}

	allProjects, err := projects.ExtractProjects(allPages)
	if err != nil {
		return "", err
	}
	for _, project := range allProjects {
		if project.Name == "admin" {
			return project.ID, nil
		}
	}
	return "", nil
}

func (c Client) AddRouterTags(id string, tag string) error {
	r := tags.Add(c.networkCliV2, "routers", id, tag)
	return r.ExtractErr()
}

func (c Client) DeleteRouterTags(id string, tag string) error {
	r := tags.Delete(c.networkCliV2, "routers", id, tag)
	return r.ExtractErr()
}

func (c Client) DeleteRouter(id string) error {
	r := routers.Delete(c.networkCliV2, id)
	return r.ExtractErr()
}

func (c Client) SetExternalGateway(id string) error {
	r := routers.Delete(c.networkCliV2, id)
	return r.ExtractErr()
}