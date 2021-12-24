# kube-ovn-neutron 的一些实现细节

## 总体流程

为了能够在 kube-ovn 的 controller 中调用 Neutron 的 API，为 Pod 创建相应的网络资源，并在 Pod 创建后为其配置网卡，我们在 kube-ovn 中集成了 Neutron 客户端，并修改了相应的 Controller 的工作流程。这部分修改主要集中在下面两个 commit 中：

1. <https://github.com/easystack/kube-ovn/commit/1acd56f426155828272783a67633e03d546618e3>

    集成了 Neutron 客户端，以及相应的网络资源创建的功能，并且在 kube-ovn 的创建 Pod 流程中加入了判断分支。一旦发现是走 Neutron 的流程，就会同步的去创建 Port 等资源。

    在 CNI Daemon 那边并没有多少修改，只是如果发现是 Neutron 路径，则会少做一些事情。具体看代码

2. <https://github.com/easystack/kube-ovn/commit/ba0ec17d080393d75711837d392058338dea8741>

    这个 commit 将 Port 的生命周期维护从上面的 commit 中剥离出来。用 CR 的形式声明一个 Port，由 Controller 去 Neutron 那边调谐出一个 Port 来给 Pod 使用。这样做的原因是为了分离 Pod 和 Port 的生命周期管理。如果要固定 Pod 的 IP 的话，就必须让 Port CR 一直存在，否则IP可能会被其他的 VM 或者 Pod 抢走。

## 代码

- pkg/neutron： 包定义了 CRD Port，以及根据其定义生成的客户端和 informer 代码（执行 hack/update-codegen-neutron.sh）。另外，pkg/neutron/port.go 等文件封装了 gopherCloud 的代码，方便调用 Neutron 的接口来维护网络资源。

- pkg/daemon： 其下面文件中的修改，大都是为了使其流程在走 Neutron 的分支的时候，不出问题。具体做法代码中都有注释。

- pkg/controller/controller.go： 在 Controller 中加入了 NeutronController 字段，供后面的代码调用。另外，如果环境变量中 NO_OVN 为 true，则也会跳过一些初始化过程，以免代码调用 ovn 的接口而发生错误。

- pkg/controller/pod.go： 主要的流程修改就集中在这个文件。在创建 Pod 时，根据注解，来决定走原生 ovn 路径还是 Neutron 路径。

- pkg/controller/ 下其他新增的 neutron， port 相关的 .go文件：几乎就是 Port 资源的 Controller，根据 CR 的 CUD 操作，去调用 Neutron的接口来调谐资源。
