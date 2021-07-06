package neutron

import (
	"fmt"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/portsbinding"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"k8s.io/klog"
)

type NeutronPort struct {
	Name     string
	ID       string
	SubnetID string
	MAC      string
	IP       string
	CIDR     string
	Gateway  string
	MTU      int
	Sgs      []string
}

func (c Client) CreatePort(name, project, network, subnet string, ip string, sgs string) (NeutronPort, error) {

	type FixedIPOpt struct {
		SubnetID        string `json:"subnet_id,omitempty"`
		IPAddress       string `json:"ip_address,omitempty"`
		IPAddressSubstr string `json:"ip_address_subdir,omitempty"`
	}
	type FixedIPOpts []FixedIPOpt

	opts := ports.CreateOpts{
		Name:      name,
		NetworkID: network,
		FixedIPs: FixedIPOpts{
			{
				SubnetID:  subnet,
				IPAddress: ip,
			},
		},
		ProjectID: project,
	}

	ss := strings.Split(sgs, ",")
	if len(ss) > 0 {
		opts.SecurityGroups = &ss
	}

	sbRes := c.getSubnetAsync(subnet)
	netRes := c.getNetworkAsync(network)

	p, err := ports.Create(c.networkCliV2, opts).Extract()
	if err != nil {
		return NeutronPort{}, err
	}

	sb, err := sbRes()
	if err != nil {
		defer c.DeletePort(p.ID)
		return NeutronPort{}, err
	}

	_, mtu, err := netRes()
	if err != nil {
		defer c.DeletePort(p.ID)
		return NeutronPort{}, err
	}

	np := NeutronPort{
		Name:     p.Name,
		ID:       p.ID,
		SubnetID: subnet,
		MAC:      p.MACAddress,
		IP:       p.FixedIPs[0].IPAddress,
		CIDR:     sb.CIDR,
		Gateway:  sb.GatewayIP,
		MTU:      mtu,
		Sgs:      p.SecurityGroups,
	}
	return np, nil
}

func (c Client) getPort(id string) (*ports.Port, error) {
	return ports.Get(c.networkCliV2, id).Extract()
}

func (c Client) DeletePort(id string) error {
	r := ports.Delete(c.networkCliV2, id)
	return r.ExtractErr()
}

func (c Client) RememberPortID(key, id string) {
	c.podsDeleteLock.Lock()
	defer c.podsDeleteLock.Unlock()
	c.portIDs[key] = id
}

func (c Client) ForgetPortID(key string) {
	c.podsDeleteLock.Lock()
	defer c.podsDeleteLock.Unlock()
	delete(c.portIDs, key)
}

func (c Client) PodToPortID(key string) (string, bool) {
	c.podsDeleteLock.Lock()
	defer c.podsDeleteLock.Unlock()
	id, ok := c.portIDs[key]
	return id, ok
}

// BindPort 将一个 Neutron Port 绑定到一个主机上。
// 主要用于 CNI 在配置 Pod 网卡的时候，要将对应 Port 绑定到
// hostID 所对应的主机上， ovs 才通
func (c Client) BindPort(id, hostID, deviceID string) error {
	if !strings.HasSuffix(hostID, ECS_HOSTNAME_SUFFIX) {
		hostID = hostID + ECS_HOSTNAME_SUFFIX
	}
	updateOpts := portsbinding.UpdateOptsExt{
		UpdateOptsBuilder: ports.UpdateOpts{
			DeviceOwner: func(s string) *string { return &s }(KUBE_OVN_DEV_OWNER),
			DeviceID:    &deviceID,
		},
		HostID:   &hostID,
		VNICType: "normal",
	}

	_, err := ports.Update(c.networkCliV2, id, updateOpts).Extract()

	return err
}

// WaitPortActive 返回一个函数，调用该函数会阻塞指定的 Neutron Port 状态变成 ACTIVE,
// 或者超时返回错误
func (c Client) WaitPortActive(id string, timeout float64) error {
	start := time.Now()
	ch := make(chan error, 1)
	status := "EMPTY"

	go func() {
		tk := time.NewTicker(time.Duration(2) * time.Second)
		for range tk.C {
			elapse := time.Since(start)
			if elapse.Seconds() > timeout {
				ch <- fmt.Errorf("waiting port %s become ACTIVE timeout out: %.2f seconds", id, timeout)
				break
			}

			p, err := c.getPort(id)
			if err != nil {
				klog.Errorf("getting port %s status failed: %v", id, err)
				continue
			} else {
				if status != p.Status {
					klog.Infof("port %s status changed %s -> %s", id, status, p.Status)
					status = p.Status
				}
				if p.Status == "ACTIVE" {
					ch <- nil
					klog.Infof("port %s takes %.2f seconds to become ACTIVE", id, elapse.Seconds())
					break
				}
			}
		}
		tk.Stop()
	}()
	return <-ch
}
