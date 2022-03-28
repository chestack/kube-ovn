package util

import v1 "k8s.io/api/core/v1"

var (
	//TODO global param
	// NodeWhiteLabels: whiteLabel nodes will allocated ip from default logic switch
	// when this is nil, should allocate all nodes which is same with community,
	// labels is 'or' relations, node which have anyone label will allocated ip.
	NodeWhiteLabels = map[string]string{}
	NodeBlackLabels = map[string]string{}
)

func InWhiteList(node *v1.Node) bool {
	if len(NodeWhiteLabels) == 0 {
		return true
	}
	if len(node.Labels) == 0 {
		return false
	}
	for k, v := range NodeWhiteLabels {
		if nodev, ok := node.Labels[k]; ok {
			if nodev == v {
				return true
			}
		}
	}
	return false
}

func InBlackList(node *v1.Node) bool {
	if len(NodeBlackLabels) == 0 {
		return false
	}
	if len(node.Labels) == 0 {
		return false
	}
	for k, v := range NodeBlackLabels {
		if nodev, ok := node.Labels[k]; ok {
			if nodev == v {
				return true
			}
		}
	}
	return false
}