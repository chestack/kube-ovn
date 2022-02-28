package neutron

import (
	"errors"

	fip "github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/floatingips"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/kubeovn/kube-ovn/pkg/util"
	"k8s.io/klog"
)

// ListFloatingIPs create network from proton api
func (c Client) ListFip() ([]fip.FloatingIP, error) {
	var (
		actual []fip.FloatingIP
		err    error
	)
	err = fip.List(c.networkCliV2, fip.ListOpts{}).EachPage(func(page pagination.Page) (bool, error) {
		actual, err = fip.ExtractFloatingIPs(page)
		if err != nil {
			klog.Errorf("Failed to extract floating IPs: %v", err)
			return false, err
		}

		return true, nil
	})
	return actual, err
}

// CreateFloatingIPs create network from proton api
func (c Client) CreateFip(floatingNetworkID, floatingIP, projectID string) (*fip.FloatingIP, error) {
	opts := fip.CreateOpts{
		Description:       util.NeutronRouterTag,
		FloatingNetworkID: floatingNetworkID,
		FloatingIP:        floatingIP,
		ProjectID:         projectID,
		TenantID:          projectID,
	}
	return fip.Create(c.networkCliV2, opts).Extract()
}

// CreateFloatingIPs create network from proton api
func (c Client) UpdateFip(id string, portID string, fixedIP string) (*fip.FloatingIP, error) {
	opts := fip.UpdateOpts{
		PortID:  &portID,
		FixedIP: fixedIP,
	}
	return fip.Update(c.networkCliV2, id, opts).Extract()
}

// DeleteFloatingIPs delete network from proton api
func (c Client) DeleteFip(id string) error {
	return fip.Delete(c.networkCliV2, id).ExtractErr()
}

// DeleteFipFromIP delete network from proton api
func (c Client) DeleteFipFromIP(floatingIP string) error {
	var (
		actual []fip.FloatingIP
		err    error
	)
	opt := fip.ListOpts{
		FloatingIP: floatingIP,
	}
	err = fip.List(c.networkCliV2, opt).EachPage(func(page pagination.Page) (bool, error) {
		actual, err = fip.ExtractFloatingIPs(page)
		if err != nil {
			klog.Errorf("Failed to extract floating IPs: %v", err)
			return false, err
		}

		return true, nil
	})
	for _, f := range actual {
		if f.FloatingIP == floatingIP {
			return fip.Delete(c.networkCliV2, f.ID).ExtractErr()
		}
	}

	klog.Errorf("Delete fip failed, fip: %s, err: not found.", floatingIP)
	return errors.New("delete fip failed, err: not found")
}
