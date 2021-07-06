package neutron

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"

	"net/http"
	"sync"
	"time"

	"k8s.io/klog"
)

const (
	KURYR_PROJECT_ID      = "openstack.org/kuryr-project-id"
	KURYR_SECURITY_GROUPS = "openstack.org/kuryr-security-groups"
	KURYR_SUBNET_ID       = "openstack.org/kuryr-subnet-id"
	NETWORK_ID            = "openstack.org/network_id"
	NETWORK_NAME          = "openstack.org/network_name"
	SUBNET_ID             = "openstack.org/subnet_id"
	SUBNET_NAME           = "openstack.org/subnet_name"
	PORT_ID               = "openstack.org/port_id"
	FIX_IP                = "openstack.org/fix_ip"

	ANNO_ECNS_DEF_NETWORK = "v1.multus-cni.io/default-network"
	SEC_CON_KUBE_OVN      = "secure-container/kube-ovn"
)

type Client struct {
	networkCliV2 *gophercloud.ServiceClient

	podsDeleteLock *sync.Mutex
	portIDs        map[string]string
}

func NewClient() *Client {
	provider := newProviderClientOrDie()
	return &Client{
		networkCliV2:   newNetworkV2ClientOrDie(provider),
		podsDeleteLock: &sync.Mutex{},
		portIDs:        make(map[string]string),
	}
}

func newProviderClientOrDie() *gophercloud.ProviderClient {
	opt, err := openstack.AuthOptionsFromEnv()
	if err != nil {
		klog.Fatalf("openstack auth options from environment error: %v", err)
	}
	p, err := openstack.AuthenticatedClient(opt)
	if err != nil {
		klog.Fatalf("openstack authenticate client error: %v", err)
	}
	p.HTTPClient = http.Client{
		Transport: http.DefaultTransport,
		Timeout:   time.Second * 60,
	}
	p.ReauthFunc = func() error {
		newprov, err := openstack.AuthenticatedClient(opt)
		if err != nil {
			return err
		}
		p.CopyTokenFrom(newprov)
		return nil
	}
	return p
}

func newNetworkV2ClientOrDie(p *gophercloud.ProviderClient) *gophercloud.ServiceClient {
	cli, err := openstack.NewNetworkV2(p, gophercloud.EndpointOpts{})
	if err != nil {
		klog.Fatalf("new NetworkV2Client error : %v", err)
	}
	return cli
}
