# 在 EOS 上安装 multus 并配置 kube-ovn

multus 默认调用 flannel 作为 CNI，在配置了 NetworkAttachDefinition 的 Pod 上调用 kube-ovn 为其配置网络

## 1, 安装 multus：

```shell
$ kubectl apply -f https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/master/images/multus-daemonset.yml
```

## 2, 创建 NetworkAttachDefinition：

```yaml
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  name: kube-ovn
  namespace: openstack
spec:
  config: '{ "name":"kube-ovn", "cniVersion":"0.3.1", "plugins":[ { "type":"kube-ovn",
    "server_socket":"/run/openvswitch/kube-ovn-daemon.sock" }, { "type":"portmap",
    "capabilities":{ "portMappings":true } } ] }'

```

## 3, 在 Pod Spec 的 v1.multus-cni.io/default-network 注解中指明其使用 kube-ovn 作为 CNI 配置网络

例如：

```yaml
spec:
  template:
    metadata:
      annotations:
        ecns.io/cni: kube-ovn
        openstack.org/kuryr-project-id: f61179aec36e4eb3bd4e3983d8c3b70b
        openstack.org/kuryr-security-groups: 1636c3d7-7633-4c2e-942a-88ef165b1cfa
        openstack.org/kuryr-subnet-id: d50548c2-b35a-4cdc-88f5-6ee1f9d63c43
        openstack.org/network_id: 19fc735d-4232-422b-a71b-951744add99a
        openstack.org/network_name: share_net
        openstack.org/subnet_id: 70f184ec-368f-463b-a702-3d49093369bd
        openstack.org/subnet_name: share_net__subnet
        v1.multus-cni.io/default-network: openstack/kube-ovn

```