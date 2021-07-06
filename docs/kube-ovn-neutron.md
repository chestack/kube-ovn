# kube-ovn 与 Neutron 集成方案

## 背景
在使用 OVN 作为 Neutron SDN 实现的 OpenStack 平台环境中， kube-ovn 可以与 Neutron 共享 OVN，实现 VM 网络与容器网络的打通。但使用 kube-ovn 的社区功能打通与 VM 的网络有以下一些问题：

1，kube-ovn 自己维护了一套网络资源的定义，例如 vpc，subnet等，与现有的 Neutron 重叠。用户需要另外维护这类资源，无法重用 Neutron 中的资源。

2，容器与 VM 无法共用一个子网，通信必须跨二层。增加了配置难度和安全隐患。

3，网络产品线的 LB 运行在容器中，容器的网卡需要与 其代理的 VM 在同一个二层上。

4，当前 kube-ovn 对多租户场景支持较弱

所以网络组提出一种方案，在 kube-ovn controller 中调用 Neutron 的接口，为 Pod 在 Neutron 的 Subnet 上创建 Port。在 CNI 那边将 veth 一端的网卡 add-port 到 ovs 网桥上，另一端根据 Port 的 IP， 网关等信息在容器的网络命名空间中配置网卡信息。达到使用 kube-ovn 的框架，将 Neutron 的网卡挂载进 Pod 的目的。

## 功能要求

兼容目前安全容器的网络方案与 kube-ovn 原生功能:

- 用户在产品页面上指定 Pod 网卡所在网络的 Neutron 网络的参数，包括：

    - 网络 ID (Network ID)
    - 子网 ID (Subnet ID)
    - 安全组 ID (SecurityGroup ID)

- kube-ovn-neutron 将配置了 Neutron 网络参数的 Pod 挂载到 Neutron 上

- 同时完全保留原生 kube-ovn 的所有功能

- 支持浮动 IP

- 支持固定 IP

## 方案1：Neutron Port 的生命周期与 Pod 的生命周期一致 (当前的实现方案)

### 1，集群多 CNI 配置方案：

- 集群使用 multus，仍然配置 flannel 作为默认的 CNI，对于安全容器和其他要接入 Neutron 网络的 Pod，在注解中定义 v1.multus-cni.io/default-network 指定使用 kube-ovn 作为 CNI 配置容器网络。

### 2，区分配置 Neutron 网络的 Pod

- 目前 kube-ovn controller 会监听集群中所有 Pod 的创建，并在默认的 ovn 子网中新建 port。需要修改 kube-ovn，只对显式指定由 kube-ovn 配置网络的 Pod 新建 Port，例如只关注有特定注解的 Pod。

- 对于要加入 Neutron 网络的 Pod，在其注解中，定义要加入的 Neutron 网络与子网：
    ```
    openstack.org/network_id: 101fe550-b535-4d5d-aa93-464abdd29ea6
    openstack.org/subnet_id: d50548c2-b35a-4cdc-88f5-6ee1f9d63c43
    ```

### 3，kube-ovn controller

- 在 Controller 结构体中，新增 Neutron 客户端字段，供相关流程调用

- 创建 Pod 的流程

    - 根据 Pod 的注解决定是否使用 Neutron 配置容器网络

    - 根据 Pod 注解中的网络，子网信息，新建 Port

    - 将 Port 的 ID， MAC/IP地址，所属子网的网关，CIDR等信息，用注解写回 Pod，供 CNI 配置容器网卡时使用

- 删除 Pod 的流程

    - 根据 Pod 注解中的 Port 的 ID，调用 Neutron 客户端删除 Port

### 4，kube-ovn CNI daemon

- 创建 Pod 的流程

    - 从 Pod 的注解中获取 Port 的 UUID，作为ifaceID 用作 ovs-vsctl add-port 的参数，将 veth 网卡加入 br-int 网桥

    - CNI 使用 Pod 注解中的MAC, IP, 网关等信息，配置 veth 在容器命名空间中的另一端网卡。

- 删除 Pod 的流程

### 5，Pod 固定 IP 以及问题

- 和 kube-ovn 一样，任然在 Pod 的注解中配置 Pod 的 固定IP。在 Neutron 客户端新建 Port 的时候，作为 fixed IP 的参数传给 Neutron

- 问题1：IP 在 Neutron Subnet 中无法预留。如果 IP 被其他 Port 占用，创建 Port 将失败，导致 Pod 创建失败。

- 问题2：Port 的生命周期与 Pod 绑定，频繁创建与销毁开销大

### 5，FIP 

- 见方案二

## 方案2：用 CR 管理 Neutron Port 的生命周期。Pod 与 Port 为绑定关系

- 定义 Neutron Port 的 CRD，包含 Port 的网络，子网，IP，网关 等信息。CR 保存在用户 Pod 的命名空间中。

- Port Controller 负责调谐 CR 在 Neutron 中的资源，并更新状态。

- 创建 Pod 时，用户定义 Pod 网络的方式不变。kube-ovn-neutron 根据 Neutron 的参数，选择已有的 Port CR 与 Pod 进行绑定。如果无满足条件的 Port，则新建 CR。

- 当 Port 与 Pod 绑定时，更新其 Status 中相关字段，表示 Port 已被占用。

- 删除 Pod 时，Pod 与 Port CR 解绑。 Port 状态更新为空闲。

- 维持一定数量的空闲 Port，作为 Port 池，提高 Pod 创建速度。

## 优化要点

- 代码实现

    - 新增的 Neutron 相关模块尽量高内聚低耦合，降低对 kube-ovn 社区代码的侵入

- Neutron Port 池

    在 Neutron 中预先创建若干 Port，加快 Pod 启动速度。

    - 在方案2中，可以通过动态配置 Port CR 的数量来实现

- Neutron API 调用限流

    限制对 Neutron 接口的调用并发数量，如果在方案二中，可以使用 k8s 自带的 workingQueue 进行限流

