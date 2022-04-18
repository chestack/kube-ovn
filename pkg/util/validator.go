package util

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
)

const (
	V6Multicast = "ff00::/8"
	V4Multicast = "224.0.0.0/4"
	V4Loopback  = "127.0.0.1/8"
	V6Loopback  = "::1/128"
)

const (
	Group     = "cluster.ecas.io"
	Version   = "v1"
	Namespace = "openstack"
	Plural    = "ecsnodes"
	Timeout   = 60 * time.Second
)

const (
	RoleOpenstackNetwork = "openstack-network"
	RoleSecureContainer  = "secure-container"
)

// Endpoint
type Endpoint struct {
	Dev  string `json:"dev,omitempty" protobuf:"bytes,1,opt,name=dev"`
	IP   string `json:"ip,omitempty" protobuf:"bytes,2,opt,name=ip"`
	Name string `json:"name,omitempty" protobuf:"bytes,3,opt,name=name"`
}

// Data
type Data struct {
	Endpoints []Endpoint `json:"endpoints,omitempty" protobuf:"bytes,1,opt,name=endpoints"`
	RoleList  []string   `json:"role_list,omitempty" protobuf:"bytes,2,opt,name=role_list"`
}

// ECSNode
type ECSNode struct {
	Data Data `json:"data,omitempty" protobuf:"bytes,1,opt,name=data"`
}

func cidrConflict(cidr string) error {
	for _, cidrBlock := range strings.Split(cidr, ",") {
		if CIDRConflict(cidrBlock, V6Multicast) {
			return fmt.Errorf("%s conflict with v6 multicast cidr %s", cidr, V6Multicast)
		}
		if CIDRConflict(cidrBlock, V4Multicast) {
			return fmt.Errorf("%s conflict with v4 multicast cidr %s", cidr, V4Multicast)
		}
		if CIDRConflict(cidrBlock, V6Loopback) {
			return fmt.Errorf("%s conflict with v6 loopback cidr %s", cidr, V6Loopback)
		}
		if CIDRConflict(cidrBlock, V4Loopback) {
			return fmt.Errorf("%s conflict with v4 loopback cidr %s", cidr, V4Loopback)
		}
	}

	return nil
}

func ValidateSubnet(subnet kubeovnv1.Subnet) error {
	if subnet.Spec.Gateway != "" && !CIDRContainIP(subnet.Spec.CIDRBlock, subnet.Spec.Gateway) {
		return fmt.Errorf(" gateway %s is not in cidr %s", subnet.Spec.Gateway, subnet.Spec.CIDRBlock)
	}
	if err := cidrConflict(subnet.Spec.CIDRBlock); err != nil {
		return err
	}
	excludeIps := subnet.Spec.ExcludeIps
	for _, ipr := range excludeIps {
		ips := strings.Split(ipr, "..")
		if len(ips) > 2 {
			return fmt.Errorf("%s in excludeIps is not a valid ip range", ipr)
		}

		if len(ips) == 1 {
			if net.ParseIP(ips[0]) == nil {
				return fmt.Errorf("ip %s in exclude_ips is not a valid address", ips[0])
			}
		}

		if len(ips) == 2 {
			for _, ip := range ips {
				if net.ParseIP(ip) == nil {
					return fmt.Errorf("ip %s in exclude_ips is not a valid address", ip)
				}
			}
			if Ip2BigInt(ips[0]).Cmp(Ip2BigInt(ips[1])) == 1 {
				return fmt.Errorf("%s in excludeIps is not a valid ip range", ipr)
			}
		}
	}

	allow := subnet.Spec.AllowSubnets
	for _, cidr := range allow {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("%s in allowSubnets is not a valid address", cidr)
		}
	}

	gwType := subnet.Spec.GatewayType
	if gwType != "" && gwType != kubeovnv1.GWDistributedType && gwType != kubeovnv1.GWCentralizedType {
		return fmt.Errorf("%s is not a valid gateway type", gwType)
	}

	if subnet.Spec.Vpc == DefaultVpc {
		k8sApiServer := os.Getenv("KUBERNETES_SERVICE_HOST")
		if k8sApiServer != "" && CIDRContainIP(subnet.Spec.CIDRBlock, k8sApiServer) {
			return fmt.Errorf("subnet %s cidr %s conflicts with k8s apiserver svc ip %s", subnet.Name, subnet.Spec.CIDRBlock, k8sApiServer)
		}
	}

	if egw := subnet.Spec.ExternalEgressGateway; egw != "" {
		if subnet.Spec.NatOutgoing {
			return fmt.Errorf("conflict configuration: natOutgoing and externalEgressGateway")
		}
		ips := strings.Split(egw, ",")
		if len(ips) > 2 {
			return fmt.Errorf("invalid external egress gateway configuration")
		}
		for _, ip := range ips {
			if net.ParseIP(ip) == nil {
				return fmt.Errorf("IP %s in externalEgressGateway is not a valid address", ip)
			}
		}
		egwProtocol, cidrProtocol := CheckProtocol(egw), CheckProtocol(subnet.Spec.CIDRBlock)
		if egwProtocol != cidrProtocol && cidrProtocol != kubeovnv1.ProtocolDual {
			return fmt.Errorf("invalid external egress gateway configuration: address family is conflict with CIDR")
		}
	}

	return nil
}

func ValidatePodNetwork(annotations map[string]string) error {
	errors := []error{}

	if ipAddress := annotations[IpAddressAnnotation]; ipAddress != "" {
		// The format of IP Annotation in dualstack is 10.244.0.0/16,fd00:10:244:0:2::/80
		for _, ip := range strings.Split(ipAddress, ",") {
			if strings.Contains(ip, "/") {
				if _, _, err := net.ParseCIDR(ip); err != nil {
					errors = append(errors, fmt.Errorf("%s is not a valid %s", ip, IpAddressAnnotation))
					continue
				}
			} else {
				if net.ParseIP(ip) == nil {
					errors = append(errors, fmt.Errorf("%s is not a valid %s", ip, IpAddressAnnotation))
					continue
				}
			}

			if cidrStr := annotations[CidrAnnotation]; cidrStr != "" {
				if err := CheckCidrs(cidrStr); err != nil {
					errors = append(errors, fmt.Errorf("invalid cidr %s", cidrStr))
					continue
				}

				if !CIDRContainIP(cidrStr, ip) {
					errors = append(errors, fmt.Errorf("%s not in cidr %s", ip, cidrStr))
					continue
				}
			}
		}
	}

	mac := annotations[MacAddressAnnotation]
	if mac != "" {
		if _, err := net.ParseMAC(mac); err != nil {
			errors = append(errors, fmt.Errorf("%s is not a valid %s", mac, MacAddressAnnotation))
		}
	}

	ipPool := annotations[IpPoolAnnotation]
	if ipPool != "" {
		for _, ip := range strings.Split(ipPool, ",") {
			if net.ParseIP(strings.TrimSpace(ip)) == nil {
				errors = append(errors, fmt.Errorf("%s in %s is not a valid address", ip, IpPoolAnnotation))
			}
		}
	}

	ingress := annotations[IngressRateAnnotation]
	if ingress != "" {
		if _, err := strconv.Atoi(ingress); err != nil {
			errors = append(errors, fmt.Errorf("%s is not a valid %s", ingress, IngressRateAnnotation))
		}
	}

	egress := annotations[EgressRateAnnotation]
	if egress != "" {
		if _, err := strconv.Atoi(egress); err != nil {
			errors = append(errors, fmt.Errorf("%s is not a valid %s", egress, EgressRateAnnotation))
		}
	}

	return utilerrors.NewAggregate(errors)
}

func ValidatePodCidr(cidr, ip string) error {
	for _, cidrBlock := range strings.Split(cidr, ",") {
		for _, ipAddr := range strings.Split(ip, ",") {
			if CheckProtocol(cidrBlock) != CheckProtocol(ipAddr) {
				continue
			}

			ipStr := IPToString(ipAddr)
			if SubnetBroadcast(cidrBlock) == ipStr {
				return fmt.Errorf("%s is the broadcast ip in cidr %s", ipStr, cidrBlock)
			}
			if SubnetNumber(cidrBlock) == ipStr {
				return fmt.Errorf("%s is the network number ip in cidr %s", ipStr, cidrBlock)
			}
		}
	}
	return nil
}

func ValidateCidrConflict(subnet kubeovnv1.Subnet, subnetList []kubeovnv1.Subnet) error {
	for _, sub := range subnetList {
		if sub.Spec.Vpc != subnet.Spec.Vpc || sub.Spec.Vlan != subnet.Spec.Vlan || sub.Name == subnet.Name {
			continue
		}

		if CIDRConflict(sub.Spec.CIDRBlock, subnet.Spec.CIDRBlock) {
			err := fmt.Errorf("subnet %s cidr %s is conflict with subnet %s cidr %s", subnet.Name, subnet.Spec.CIDRBlock, sub.Name, sub.Spec.CIDRBlock)
			return err
		}

		if subnet.Spec.ExternalEgressGateway != "" && sub.Spec.ExternalEgressGateway != "" &&
			subnet.Spec.PolicyRoutingTableID == sub.Spec.PolicyRoutingTableID {
			err := fmt.Errorf("subnet %s policy routing table ID %d is conflict with subnet %s policy routing table ID %d", subnet.Name, subnet.Spec.PolicyRoutingTableID, sub.Name, sub.Spec.PolicyRoutingTableID)
			return err
		}
	}
	err := ValidateNodeCidrConflict(subnet)
	if err != nil {
		return err
	}
	return nil
}

func ValidateNodeCidrConflict(subnet kubeovnv1.Subnet) error {
	nodeCidr, err := getNodeCidr()
	if err != nil {
		// TODO: Ignore errors when getting node cidr fails
		klog.Errorf("Ignore errors when getting node information fails, err: %+v\n", err)
		return nil
	}
	klog.Infof("Validate node cidr conflict, node cidr: %s", nodeCidr)

	if CIDRConflict(nodeCidr, subnet.Spec.CIDRBlock) {
		return fmt.Errorf("cidr %s of subnet %s is forbiddened", subnet.Spec.CIDRBlock, subnet.Name)
	}
	return nil
}

func listEcsNode() ([]byte, error) {
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		return nil, err
	}

	ClientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	options := metav1.ListOptions{}
	result, err := ClientSet.AppsV1().RESTClient().Get().
		AbsPath("apis", Group, Version).
		Namespace(Namespace).
		Resource(Plural).
		VersionedParams(&options, scheme.ParameterCodec).
		Timeout(Timeout).
		DoRaw(context.TODO())
	if err != nil {
		return nil, err
	}

	return result, nil
}

func getNodeCidr() (string, error) {
	result, err := listEcsNode()
	if err != nil {
		return "", err
	}

	objects := new(metav1.List)
	err = json.Unmarshal(result, objects)
	if err != nil {
		return "", err
	}

	tIPCidrMap := make(map[interface{}]struct{})
	for _, item := range objects.Items {
		ecsNode := new(ECSNode)
		err = json.Unmarshal(item.Raw, ecsNode)
		if err != nil {
			return "", err
		}

		if !needValidateNodeCidrConflict(ecsNode.Data.RoleList) {
			continue
		}

		for _, endpoint := range ecsNode.Data.Endpoints {
			if endpoint.IP == "" {
				continue
			}
			_, tIpNet, err := net.ParseCIDR(endpoint.IP)
			if err != nil {
				continue
			}
			if _, ok := tIPCidrMap[tIpNet]; !ok {
				tIPCidrMap[tIpNet] = struct{}{}
			}
		}
	}

	var forbiddenIPCidr bytes.Buffer
	for tIPCidr := range tIPCidrMap {
		if forbiddenIPCidr.Len() == 0 {
			forbiddenIPCidr.WriteString(fmt.Sprintf("%s", tIPCidr))
		} else {
			forbiddenIPCidr.WriteString(fmt.Sprintf(",%s", tIPCidr))
		}
	}

	return forbiddenIPCidr.String(), nil
}

func needValidateNodeCidrConflict(roleList []string) bool {
	for _, role := range roleList {
		if role == RoleOpenstackNetwork || role == RoleSecureContainer {
			return true
		}
	}
	return false
}
