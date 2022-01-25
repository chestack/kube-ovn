package neutron

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"net/http"
	"os"
	"sync"
	"time"

	neutronv1 "github.com/kubeovn/kube-ovn/pkg/neutron/apis/neutron/v1"
	clientset "github.com/kubeovn/kube-ovn/pkg/neutron/client/clientset/versioned"
	"github.com/kubeovn/kube-ovn/pkg/neutron/client/clientset/versioned/scheme"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
)

const (
	KURYR_PROJECT_ID      = "openstack.org/kuryr-project-id"
	KURYR_SECURITY_GROUPS = "openstack.org/kuryr-security-groups"
	KURYR_SUBNET_ID       = "openstack.org/kuryr-subnet-id"
	FIX_IP                = "openstack.org/kuryr-fixed-ip"
	NETWORK_ID            = "openstack.org/network_id"
	NETWORK_NAME          = "openstack.org/network_name"
	SUBNET_ID             = "openstack.org/subnet_id"
	SUBNET_NAME           = "openstack.org/subnet_name"
	PORT_ID               = "openstack.org/port_id"
	PORT_NAME             = "openstack.org/port_name"

	ANNO_ECNS_DEF_NETWORK   = "v1.multus-cni.io/default-network"
	SEC_CON_KUBE_OVN        = "secure-container/kube-ovn-neutron"
	SEC_CON_KUBE_OVN_ORIGIN = "secure-container/kube-ovn"
)

func NewClientset(kubeconfig *rest.Config) clientset.Interface {
	old := kubeconfig.ContentType
	defer func() {
		kubeconfig.ContentType = old
	}()
	kubeconfig.ContentType = "application/json"
	utilruntime.Must(neutronv1.AddToScheme(scheme.Scheme))
	kubeNtrnCli, err := clientset.NewForConfig(kubeconfig)
	utilruntime.Must(err)
	return kubeNtrnCli
}

type Client struct {
	networkCliV2 *gophercloud.ServiceClient
	identityCliV3 *gophercloud.ServiceClient

	podsDeleteLock *sync.Mutex
	portIDs        map[string]string
}

func NewClient() *Client {
	provider := newProviderClientOrDie(false)
	domainTokenProvider := newProviderClientOrDie(true)
	return &Client{
		networkCliV2:   newNetworkV2ClientOrDie(provider),
		identityCliV3:  newIdentityV3ClientOrDie(domainTokenProvider),
		podsDeleteLock: &sync.Mutex{},
		portIDs:        make(map[string]string),
	}
}

func newProviderClientOrDie(domainScope bool) *gophercloud.ProviderClient {
	opt, err := openstack.AuthOptionsFromEnv()
	if err != nil {
		klog.Fatalf("openstack auth options from environment error: %v", err)
	}
	// with OS_PROJECT_NAME in env, AuthOptionsFromEnv return project scope token
	// which can not list projects, we need a domain scope token here
	if domainScope {
		opt.TenantName = ""
		opt.Scope = &gophercloud.AuthScope{
			DomainName:  os.Getenv("OS_DOMAIN_NAME"),
		}
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

func newIdentityV3ClientOrDie(p *gophercloud.ProviderClient) *gophercloud.ServiceClient {
	cli, err := openstack.NewIdentityV3(p, gophercloud.EndpointOpts{})
	if err != nil {
		klog.Fatalf("new NewIdentityV3 error : %v", err)
	}
	return cli
}