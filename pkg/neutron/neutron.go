package neutron

import (
	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
)

const (
	ECS_HOSTNAME_SUFFIX = ".domain.tld"
	// device_owner will be parsed by neutron dashboard
	// dashboard will use device_owner which judged the port is belong to 'secure container' or not
	// NOTE: neutron dashboard only use 'compute:kuryr' as 'secure container' created
	KUBE_OVN_DEV_OWNER = "compute:kuryr"
)

// 在 EOS 上的 Pod中，如果注解包含了:
// v1.multus-cni.io/default-network: secure-container/kube-ovn
// 就会调用 Neutron 的客户端去配置 Pod 的网络
func HandledByNeutron(as map[string]string) bool {
	if as == nil {
		return false
	}
	v, ok := as[ANNO_ECNS_DEF_NETWORK]
	if !ok {
		return false
	}
	return v == SEC_CON_KUBE_OVN
}

// 在 EOS 上的 Pod中，如果注解包含了:
// v1.multus-cni.io/default-network: secure-container/kube-ovn-origin
// 就会调用社区原生 kube-ovn 去配置 Pod 的网络
func HandledByKubeOvnOrigin(as map[string]string) bool {
	if as == nil {
		return false
	}
	v, ok := as[ANNO_ECNS_DEF_NETWORK]
	if !ok {
		return false
	}
	return v == SEC_CON_KUBE_OVN_ORIGIN
}

func ValidateNeutronConfig(as map[string]string) error {
	//TODO: TO IMPLEMENT
	return nil
}

func IsNeutronRouter(vpc *kubeovnv1.Vpc, neutronRouter bool) bool {
	if neutronRouter {
		// vpc.Status.Default for default vpc
		// vpc.Spec.NeutronRouter for manually created vpc to every network AZ
		if vpc.Status.Default || vpc.Spec.NeutronRouter != "" {
			return true
		}
	}
	return false
}
