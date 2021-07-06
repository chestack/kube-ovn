package neutron

const (
	ECS_HOSTNAME_SUFFIX = ".domain.tld"
	KUBE_OVN_DEV_OWNER  = "compute:kube-ovn"
)

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

func ValidateNeutronConfig(as map[string]string) error {
	//TODO: TO IMPLEMENT
	return nil
}
