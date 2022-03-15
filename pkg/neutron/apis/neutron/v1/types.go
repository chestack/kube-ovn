package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// genclient:nonNamespaced

type Port struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PortSpec   `json:"spec"`
	Status PortStatus `json:"status,omitempty"`
}

type PortSpec struct {
	Name            string   `json:"name,omitempty"`
	ProjectID       string   `json:"projectId,omitempty"`
	NetworkID       string   `json:"networkId,omitempty"`
	SubnetID        string   `json:"subnetId,omitempty"`
	SecurityGroupID []string `json:"securityGroupId,omitempty"`
	FixIP           string   `json:"fixIp,omitempty"`
	FixMAC          string   `json:"fixMac,omitempty"`
	DeleteByPod     bool     `json:"deleteByPod,omitempty"`
}

type PortStatus struct {
	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []PortCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// Port 在 Neutron 中的 ID, 由 Neutron 分配
	ID string `json:"id"`

	IP string `json:"ip"`

	MAC string `json:"mac"`

	SecurityGroupID []string `json:"securityGroupId"`

	// Port 所在的 Subnet 的网段
	CIDR string `json:"cidr"`

	// Port 所在的 Subnet 的网关
	Gateway string `json:"gateway"`

	// Port 所在的 Network 的 MTU
	MTU int `json:"mtu"`

	// Port 所绑定的 Pod 的节点全名
	HostId string `json:"hostId"`

	// Port 所绑定的 Pod 全名
	BindPod string `json:"bindPod"`
}

// ConditionType encodes information on the condition
type ConditionType string

// Constants for condition
const (
	// Ready => controller considers this resource Ready
	Ready = "Ready"
	// Validated => Spec passed validating
	Validated = "Validated"
	// Error => last recorded error
	Error = "Error"

	ReasonInit = "Init"
)

const (
	ConditionCreated   ConditionType = "Created"
	ConditionActivated ConditionType = "Activated"
)

type PortCondition struct {
	// Type of condition.
	Type ConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
	// Last time the condition was probed
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PortList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Port `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

type Fip struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FipSpec   `json:"spec"`
	Status FipStatus `json:"status,omitempty"`
}

// AllocationPool represents a sub-range of cidr available for dynamic
// allocation to ports, e.g. {Start: "10.0.0.2", End: "10.0.0.254"}
type AllocationPool struct {
	CIDR  string `json:"cidr"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type FipSpec struct {
	// Description for the floating IP instance.
	Description string `json:"description"`

	// ExternalNetworkID is the UUID of the external network id.
	ExternalNetworkID string `json:"externalNetworkID"`

	// ExternalNetworkName is the external network name.
	ExternalNetworkName string `json:"externalNetworkName"`

	// Sub-ranges of CIDR available for dynamic allocation to ports.
	// See AllocationPool.
	AllocationPools []AllocationPool `json:"allocationPools"`
}

// NeutronRouter
type NeutronRouter struct {
	// NeutronRouterID is the UUID of the neutron router id.
	NeutronRouterID string `json:"neutronRouterID"`

	// NeutronRouterName is the neutron router name.
	NeutronRouterName string `json:"neutronRouterName"`

	// AvailabilityZone
	AvailabilityZone string `json:"availabilityZone"`

	// ExternalGatewayIP is the address of the external network getaway.
	ExternalGatewayIP string `json:"externalGatewayIP"`

	// Subnets is the UUID of the router subnet id.
	Subnets []string `json:"subnets"`
}

// AllocatedIP
type AllocatedIP struct {
	// IP is floating ip
	IP string `json:"ip"`

	// Type is type of floating ip, egg: eip/snat
	Type string `json:"type"`

	// Resources is bound resource list of floating ip, egg: pod_name
	Resources []string `json:"resources"`
}

type FipStatus struct {
	// Conditions represents the latest state of the object
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []FipCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// RouterID is the ID of the router used for this floating IP.
	NeutronRouters []NeutronRouter `json:"neutronRouters"`

	// AllocatedIPs is the address of the floating IP has been allocated by ecnf.
	AllocatedIPs []AllocatedIP `json:"allocatedIPs"`

	// ForbiddenIPs is the address of the floating IP has been allocated by proton.
	ForbiddenIPs []string `json:"forbiddenIPs"`
}

type FipPatch struct {
	// Op is sync operator type
	Op string `json:"op"`

	// Name is sync operator resource name
	Name string `json:"name"`

	// Path is sync operator resource path
	Path string `json:"path"`

	// NeutronRouters is sync operator resource value
	NeutronRouters []NeutronRouter `json:"neutronRouters"`

	// AllocatedIP is sync operator resource value
	AllocatedIP AllocatedIP `json:"allocatedIP"`

	// ForbiddenIPs is sync operator resource value
	ForbiddenIPs []string `json:"forbiddenIPs"`
}

// Condition describes the state of an object at a certain point.
// +k8s:deepcopy-gen=true
type FipCondition struct {
	// Type of condition.
	Type ConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
	// Last time the condition was probed
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Fip `json:"items"`
}
