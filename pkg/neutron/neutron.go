package neutron

const (
	ECS_HOSTNAME_SUFFIX = ".domain.tld"
	KUBE_OVN_DEV_OWNER  = "compute:kube-ovn"
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
