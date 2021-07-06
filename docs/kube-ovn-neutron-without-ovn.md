# kube-ovn 与 Neutron 集成方案 (支持无 ovn 运行)

## 背景
kube-ovn-neutron 新增的代码中虽然没有与 OVN 互动的流程，但必须要求 EOS 中已经安装了 OVN 云产品才能正常启动和工作。

但当 kube-ovn-neutron 作为基础组件安装时，EOS 还中没有安装 OVN。要求其能够正常启动和满足安全容器的网络配置需求。

## 代码修改点

### Controller

## CNI Daemon
