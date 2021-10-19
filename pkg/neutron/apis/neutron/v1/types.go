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
