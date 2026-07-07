# Kuscia 架构及各模块设计文档

> 版本：基于 Kuscia 1.2.0b0 源码与文档整理  
> 目标读者：Kuscia 开发者、运维人员、希望深入理解 Kuscia 内部架构的工程师

---

## 目录

1. [概述](#1-概述)
2. [总体架构分层](#2-总体架构分层)
3. [部署模式与组网](#3-部署模式与组网)
    - 3.4 [运行模式核心差异](#34-运行模式核心差异)
    - 3.5 [实现代码层面的差异](#35-实现代码层面的差异)
    - 3.6 [模式选择建议](#36-模式选择建议)
4. [核心模块详解](#4-核心模块详解)
5. [运行时与资源隔离](#5-运行时与资源隔离)
6. [任务调度与数据流](#6-任务调度与数据流)
    - 6.4 [按 DAG 创建 KusciaTask CR](#64-按-dag-创建-kusciatask-cr)
        - 6.4.6 [P2P 模式下多方 PSI 的完整调度与数据流](#646-示例-5p2p-模式下多方-psi-的完整调度与数据流)
        - 6.4.7 [相关单元测试介绍](#647-相关单元测试介绍)
        - 6.4.8 [关键代码路径](#648-关键代码路径)
7. [网络与安全](#7-网络与安全)
8. [数据层：DataMesh](#8-数据层datamesh)
9. [API 层](#9-api-层)
10. [运维、监控与诊断](#10-运维监控与诊断)
11. [嵌入式 K3s 架构](#11-嵌入式-k3s-架构)
12. [CRD 代码生成机制](#12-crd-代码生成机制)
13. [镜像体系](#13-镜像体系)
14. [附录](#14-附录)

---

## 1. 概述

Kuscia（Kubernetes-based Secure Collaborative InfrA）是一款基于 K3s 的轻量级隐私计算任务编排框架，旨在屏蔽异构基础设施和协议差异，为隐私计算引擎提供统一的运行底座。

Kuscia 解决的核心问题：

- **轻量化部署**：最低 1C2G 即可完成 POC 级 PSI 任务。
- **跨域网络安全通信**：单公网端口复用、支持 MTLS/HTTPS、HTTP 转发适配机构网关。
- **跨域任务编排**：支持 DAG 任务流、多方 Co-Scheduling、多引擎协同。
- **异构数据源**：通过 DataMesh 统一对接 localfs、OSS、MySQL 等多种数据源。
- **互联互通**：支持 Kuscia-InterOp 协议，可与第三方厂商节点互通。
- **统一 API**：通过 KusciaAPI 以 gRPC/HTTP 方式对外提供任务编排能力。

---

## 2. 总体架构分层

从功能视角，Kuscia 体系可分为三层：

```text
┌─────────────────────────────────────────────┐
│                 产品层                        │
│   SecretPad / 业务系统 / 第三方平台            │
├─────────────────────────────────────────────┤
│                 框架层                        │
│   Kuscia：任务编排、网络互通、数据访问、统一 API │
├─────────────────────────────────────────────┤
│                 引擎层                        │
│   SecretFlow / TrustedFlow / Easy PSI / SCQL  │
└─────────────────────────────────────────────┘
```

Kuscia 自身又分为 **控制平面** 与 **节点** 两大部分：

| 部分 | 职责 | 核心组件 |
|------|------|----------|
| **控制平面** | 资源管理、任务调度、路由与认证 | K3s、Kuscia Controllers、Envoy、DomainRoute Controller、InterConn 适配器 |
| **节点** | 运行隐私计算应用的 Pod | Agent、NetworkMesh、DataMesh |

---

## 3. 部署模式与组网

### 3.1 组网模式

| 组网模式 | 控制平面 | 节点类型 | 适用场景 |
|----------|----------|----------|----------|
| **中心化组网** | 多个 Lite 节点共享一个 Master | Lite 节点 | 大型机构内部，统一运维，资源成本低 |
| **P2P 组网** | 每个节点自带独立控制平面 | Autonomy 节点 | 小型机构、安全性要求高 |

Kuscia 还支持 **中心化与 P2P 混合组网**，以及 **与第三方厂商节点互联互通**。

### 3.2 节点角色

| 角色 | 说明 | 运行模式 |
| ------ | ------ | ---------- |
| **Master** | 控制平面，负责任务调度、资源管理、路由控制 | `RunModeMaster` |
| **Lite** | 无控制平面的工作节点，依赖 Master 调度 | `RunModeLite` |
| **Autonomy** | 自带控制平面的 P2P 节点 | `RunModeAutonomy` |

### 3.3 基础设施部署方式

| 部署方式 | 说明 | 适用场景 |
| ---------- | ------ | ---------- |
| **Docker 模式** | 以 Docker 容器方式部署控制平面和节点 | 物理机/虚拟机 |
| **K8s 模式** | 将控制平面和节点以 K8s 应用方式部署 | 公有 K8s 集群 |
| **K8s 控制器模式** | 将 Kuscia Controllers、Storage、Envoy 部署在 K8s 控制平面 | 专有 K8s 集群 |

### 3.4 运行模式核心差异

三种运行模式并不是简单地在配置文件中切换一个字段，而是直接决定了 Kuscia 会启动哪些内部模块、是否嵌入 K3s、KubeClient 如何初始化、以及节点在联邦中扮演的角色。下面从模块组合、职责划分和配置文件三个维度进行系统对比。

#### 3.4.1 模块启动矩阵

Kuscia 的模块注册集中在 `cmd/kuscia/start/start.go` 中。`ModuleManager.Regist` 的第三个参数开始是可变长的模式白名单，只有当前运行模式出现在白名单中的模块才会被启动。下表汇总了主要模块与模式的对应关系。

| 模块 | Master | Lite | Autonomy | 关键职责 |
| ------ | -------- | ------ | ---------- | ---------- |
| **k3s** | ✅ | ❌ | ✅ | 嵌入 K3s 作为控制平面，提供 Kubernetes API |
| **agent** | ❌ | ✅ | ✅ | Kubelet 替代，管理本地容器生命周期 |
| **containerd** | ❌ | ✅* | ✅* | 本地容器运行时，仅在 `EnableContainerd=true` 时启动 |
| **controllers** | ✅ | ❌ | ✅ | 运行 Kuscia 自定义控制器（Domain、Task、Job 等 CRD） |
| **coredns** | ✅ | ✅ | ✅ | 为 K3s 集群与 Pod 提供 DNS 服务 |
| **scheduler** | ✅ | ❌ | ✅ | 运行增强版 kube-scheduler，支持 Kuscia 调度策略 |
| **datamesh** | ❌ | ✅ | ✅ | 提供数据访问代理与 Arrow Flight 服务 |
| **transport** | ❌ | ✅ | ✅ | 提供节点间安全通信通道 |
| **domainroute** | ✅ | ✅ | ✅ | 维护跨域路由与证书交换 |
| **interconn** | ✅ | ❌ | ✅ | 互联互通协议适配器 |
| **reporter** | ✅ | ❌ | ✅ | 负责节点任务状态上报/采集 |
| **kusciaapi** | ✅ | ✅ | ✅ | 对外暴露 Kuscia API，非 Master 模式下 Initiator 默认为 DomainID |
| **envoy** | ✅ | ✅ | ✅ | 七层网关，代理跨域与域内流量 |
| **confmanager** | ✅ | ✅ | ✅ | 配置中心，Lite 模式下复用 Master 签发的域证书 |
| **metricexporter/nodeexporter/ssexporter/diagnose** | ✅ | ✅ | ✅ | 监控、节点指标、系统状态、诊断等可观测模块 |

> *注：containerd 是否启动取决于运行时配置；若使用外部容器运行时则不会注册。*

从上表可以清晰看出：

- **Master** 只承担控制平面职责：运行 K3s、controllers、scheduler、reporter 等。
- **Lite** 只承担工作节点职责：运行 agent、datamesh、transport 等，通过 `masterEndpoint` 连接到远程 Master 的 K3s API。
- **Autonomy** 是 Master 与 Lite 的合体：既嵌入 K3s 控制平面，又运行 agent 等数据平面模块，因此具备独立对外提供联邦协作的能力（P2P 节点）。

#### 3.4.2 控制平面与数据平面职责划分

| 维度 | Master | Lite | Autonomy |
| ------ | -------- | ------ | ---------- |
| **是否嵌入 K3s** | 是 | 否 | 是 |
| **K8s API 来源** | 本地 K3s | 远程 Master | 本地 K3s |
| **是否运行 Agent** | 否 | 是 | 是 |
| **是否运行 Scheduler** | 是 | 否 | 是 |
| **是否运行 Controllers** | 是 | 否 | 是 |
| **域证书来源** | 自签 CA | 由 Master 注册时签发 | 自签 CA |
| **典型部署位置** | 中心集群 | 参与方边缘节点 | 独立参与方全栈节点 |

#### 3.4.3 配置文件结构差异

Kuscia 为三种模式定义了独立的配置结构体，位于 `cmd/kuscia/confloader/kuscia_config.go`。

**Master 配置（`MasterKusciaConfig`）**

```go
type MasterKusciaConfig struct {
CommonConfig `yaml:",inline"`
DatastoreEndpoint string
ClusterToken      string
}
```

关键字段：`DatastoreEndpoint` 指定 etcd/Datastore 地址；`ClusterToken` 用于 K3s 集群加入认证。

**Lite 配置（`LiteKusciaConfig`）**

```go
type LiteKusciaConfig struct {
CommonConfig `yaml:",inline"`
LiteDeployToken   string
MasterEndpoint    string
Runtime           string
Runk              RunkConfig
Capacity          config.CapacityCfg
ReservedResources config.ReservedResourcesCfg
Image             ImageConfig
}
```

关键字段：`MasterEndpoint` 指向远程 Master 的 API 入口；`Runtime`/`Runk` 指定本地容器运行时；`Capacity` 与 `ReservedResources` 描述节点资源。

**Autonomy 配置（`AutonomyKusciaConfig`）**

```go
type AutonomyKusciaConfig struct {
CommonConfig `yaml:",inline"`
Runtime           string
Runk              RunkConfig
Capacity          config.CapacityCfg
ReservedResources config.ReservedResourcesCfg
Image             ImageConfig
DatastoreEndpoint string
}
```

Autonomy 同时具备控制平面与工作节点属性，因此既有 `DatastoreEndpoint` 这类控制平面字段，也包含 `Runtime`、`Capacity` 等节点字段。

对应的默认初始化逻辑也体现了差异：

- `defaultMaster`：生成 K3s API Server 监听地址、kubeconfig 路径等。
- `defaultLite`：仅初始化 agent 相关配置，不生成 K3s 配置。
- `defaultAutonomy`：合并 `defaultMaster` 与 Lite 的 agent 配置。

### 3.5 实现代码层面的差异

#### 3.5.1 模块注册差异（`cmd/kuscia/start/start.go`）

代码中通过 `mm.Regist` 显式声明每个模块支持的模式。`cmd/kuscia/start/start.go` 中的实际注册如下（已省略依赖设置与就绪钩子）：

```go
master, lite, autonomy := common.RunModeMaster, common.RunModeLite, common.RunModeAutonomy

mm.Regist("coredns", modules.NewCoreDNS, autonomy, lite, master)
mm.Regist("k3s", modules.NewK3s, autonomy, master)
mm.Regist("agent", modules.NewAgent, autonomy, lite)
mm.Regist("envoy", modules.NewEnvoy, autonomy, lite, master)
if conf.EnableContainerd {
mm.Regist("containerd", modules.NewContainerd, autonomy, lite)
}
mm.Regist("config", modules.NewConfManager, autonomy, lite, master)
mm.Regist("controllers", modules.NewControllersModule, autonomy, master)
mm.Regist("datamesh", modules.NewDataMesh, autonomy, lite)
mm.Regist("domainroute", modules.NewDomainRoute, autonomy, master, lite)
mm.Regist("interconn", modules.NewInterConn, autonomy, master)
mm.Regist("kusciaapi", modules.NewKusciaAPI, autonomy, lite, master)
mm.Regist("metricexporter", modules.NewMetricExporter, autonomy, lite, master)
mm.Regist("nodeexporter", modules.NewNodeExporter, autonomy, lite, master)
mm.Regist("ssexporter", modules.NewSsExporter, autonomy, lite, master)
mm.Regist("scheduler", modules.NewScheduler, autonomy, master)
mm.Regist("transport", modules.NewTransport, autonomy, lite)
mm.Regist("reporter", modules.NewReporter, autonomy, master)
mm.Regist("diagnose", modules.NewDiagnose, autonomy, lite, master)
```

`ModuleManager.Regist` 的第三个参数开始是可变长的模式白名单；如果当前运行模式不在白名单中，该模块不会被实例化。例如 Lite 节点不会启动 `k3s`、`controllers`、`scheduler`、`reporter`、`interconn`，因此进程数更少、资源占用更低。第 4.1 节还会进一步说明模块间的依赖关系（如 `envoy -> k3s`、`agent -> envoy/k3s/kusciaapi` 等）。

#### 3.5.2 运行时配置初始化差异（`cmd/kuscia/modules/runtime.go`）

`BuildModuleRuntimeConfigs` 在读取 `kuscia.yaml` 后，会按模式填充 `ModuleRuntimeConfigs`。关键差异如下。

**Master / Autonomy**：

```go
if dependencies.RunMode == common.RunModeMaster || dependencies.RunMode == common.RunModeAutonomy {
dependencies.ApiserverEndpoint = dependencies.Master.APIServer.Endpoint
dependencies.KubeconfigFile = dependencies.Master.APIServer.KubeConfig
dependencies.KusciaKubeConfig = filepath.Join(dependencies.RootDir, "etc/kuscia.kubeconfig")
dependencies.InterConnSchedulerPort = defaultInterConnSchedulerPort
}
```

K3s 模块随后会使用 `KubeconfigFile` 启动本地 API Server；controllers、scheduler 等模块再通过该 kubeconfig 连接本地 K3s。`KusciaKubeConfig` 用于生成面向 Kuscia 内部组件的 kubeconfig，`InterConnSchedulerPort` 则在 Master/Autonomy 上启用互联互通调度端口。

**Autonomy / Lite（工作节点属性）**：

```go
if dependencies.RunMode == common.RunModeAutonomy || dependencies.RunMode == common.RunModeLite {
dependencies.ContainerdSock = common.ContainerdSocket()
dependencies.TransportConfigFile = filepath.Join(dependencies.RootDir, "etc/conf/transport/transport.yaml")
dependencies.TransportPort, err = GetTransportPort(...)
}
```

这两类模式需要本地容器运行时与跨节点通信通道，因此会初始化 containerd socket 与 transport 配置。

**Lite 独有逻辑**：

```go
if dependencies.RunMode == common.RunModeLite {
dependencies.ApiserverEndpoint = defaultEndpointForLite
clients, err := kubeconfig.CreateClientSetsFromKubeconfig(
dependencies.KubeconfigFile,
dependencies.ApiserverEndpoint,
)
dependencies.Clients = clients
}
```

Lite 模式没有本地 K3s，因此直接使用 Master 提供的 kubeconfig 创建到远程 API Server 的 Kubernetes 客户端，并把 `ApiserverEndpoint` 固定为面向 Lite 的默认值。

#### 3.5.3 K3s 启动差异（`cmd/kuscia/modules/k3s.go`）

`k3s` 模块仅存在于 Master/Autonomy。`Run` 方法会启动精简版 K3s server：

```go
args := []string{
"server",
"-d=" + s.dataDir,
"-o=" + s.kubeconfigFile,
"--disable-agent",
"--disable-scheduler",
"--flannel-backend=none",
"--disable=coredns",
"--disable=traefik",
"--disable=servicelb",
// ...
}
```

注意 Kuscia 关掉了 K3s 自带的 agent、scheduler、flannel、coredns、traefik、servicelb 等组件，只保留 API Server 与数据存储，并复用自己的 scheduler、agent（仅 Autonomy）和网络代理。

#### 3.5.4 各模块内部的模式判断

即使某些模块在三种模式下都会启动，其内部行为也会根据 `RunMode` 做分支。

**KusciaAPI（`cmd/kuscia/modules/kusciaapi.go`）**

```go
if d.RunMode != common.RunModeMaster {
kusciaAPIConfig.Initiator = d.DomainID
}
```

在非 Master 模式下，KusciaAPI 的默认发起方被设置为当前节点 DomainID，因为 Lite/Autonomy 节点通常以自身身份提交任务。

**ConfManager（`cmd/kuscia/modules/confmanager.go`）**

```go
switch d.RunMode {
case common.RunModeLite:
conf.DomainCertValue = &d.DomainCertByMasterValue
case common.RunModeAutonomy:
conf.DomainCertValue = &atomic.Value{}
conf.DomainCertValue.Store(d.DomainCert)
}
```

Lite 节点的域证书由 Master 在注册时签发并保存到 `DomainCertByMasterValue`；Autonomy 则使用本地自签 CA 生成的域证书。

**DomainRoute（`cmd/kuscia/modules/domainroute.go`）**

```go
func getPubkeyForToken(pubkey string, runmode common.RunModeType) (*rsa.PublicKey, error) {
if pubkey == "" && runmode == common.RunModeLite {
err := fmt.Errorf("query peer master pubkey failed, pubkey is empty")
return nil, err
}
if runmode == common.RunModeLite {
masterDer, decodeErr := base64.StdEncoding.DecodeString(pubkey)
// ...
return pubKey, nil
}
return nil, nil
}
```

Lite 节点必须获取对端 Master 的公钥用于 UID-RSA Token 校验；Autonomy/Master 则不需要此流程。

### 3.6 模式选择建议

| 场景 | 推荐模式 | 理由 |
| ------ | ---------- | ------ |
| 多方协作，由一方统一托管控制平面 | **Master + Lite** | Master 运行中心控制平面，各参与方部署 Lite 节点接入，运维边界清晰 |
| 单个机构独立部署，既要管理又要跑任务 | **Autonomy** | 一台节点即可完成控制与计算，减少部署组件数 |
| 已有外部 Kubernetes 集群，只想添加 Kuscia 能力 | **K8s 模式** | 不依赖 K3s，复用现有 K8s 控制平面 |
| 对资源极度敏感的边缘节点 | **Lite** | 不跑 K3s、scheduler、controllers，内存与 CPU 占用最低 |

---

## 4. 核心模块详解

### 4.1 模块启动总览

`cmd/kuscia/start/start.go` 中注册了所有模块及其运行角色、依赖关系：

```go
mm.Regist("coredns",      modules.NewCoreDNS,       autonomy, lite, master)
mm.Regist("k3s",          modules.NewK3s,           autonomy, master)
mm.Regist("agent",        modules.NewAgent,         autonomy, lite)
mm.Regist("envoy",        modules.NewEnvoy,         autonomy, lite, master)
mm.Regist("containerd",   modules.NewContainerd,    autonomy, lite)   // 可选
mm.Regist("config",       modules.NewConfManager,   autonomy, lite, master)
mm.Regist("controllers",  modules.NewControllersModule, autonomy, master)
mm.Regist("datamesh",     modules.NewDataMesh,      autonomy, lite)
mm.Regist("domainroute",  modules.NewDomainRoute,   autonomy, lite, master)
mm.Regist("interconn",    modules.NewInterConn,     autonomy, master)
mm.Regist("kusciaapi",    modules.NewKusciaAPI,     autonomy, lite, master)
mm.Regist("metricexporter", modules.NewMetricExporter, autonomy, lite, master)
mm.Regist("nodeexporter", modules.NewNodeExporter,  autonomy, lite, master)
mm.Regist("ssexporter",   modules.NewSsExporter,    autonomy, lite, master)
mm.Regist("scheduler",    modules.NewScheduler,     autonomy, master)
mm.Regist("transport",    modules.NewTransport,     autonomy, lite)
mm.Regist("reporter",     modules.NewReporter,      autonomy, master)
mm.Regist("diagnose",     modules.NewDiagnose,      autonomy, lite, master)
```

核心依赖关系：

```text
k3s -> coredns
envoy -> k3s
controllers -> k3s
scheduler -> k3s
domainroute -> k3s
config -> k3s, envoy, domainroute, controllers
kusciaapi -> k3s, config, domainroute
datamesh -> k3s, config, envoy, domainroute
agent -> k3s, envoy, kusciaapi, [containerd]
transport -> envoy
metricexporter -> agent, envoy, ssexporter, nodeexporter
reporter -> k3s, kusciaapi
```

### 4.2 各模块职责

| 模块 | 代码路径 | 核心职责 |
| ------ | ---------- | ---------- |
| **K3s** | `cmd/kuscia/modules/k3s.go` | 轻量级 Kubernetes 控制平面，处理 K8s 内置资源 |
| **CoreDNS** | `cmd/kuscia/modules/coredns.go` | 域内服务发现，解析应用 Service 域名 |
| **Agent** | `pkg/agent/` | 节点注册、容器生命周期管理，支持 RunC/RunP/RunK |
| **Envoy** | `pkg/gateway/` | 边缘/服务代理，负责跨域流量转发、认证鉴权 |
| **Containerd** | `cmd/kuscia/modules/containerd.go` | RunC 模式下的容器运行时（可选） |
| **Controllers** | `pkg/controllers/` | Kuscia 自定义资源控制器集合 |
| **Scheduler** | `pkg/scheduler/` | Kuscia 调度器插件，实现 PodGroup Co-Scheduling |
| **DomainRoute** | `pkg/controllers/domainroute/` | 节点间路由规则与授权策略管理 |
| **InterConn** | `pkg/interconn/` | 互联互通协议适配（Kuscia-InterOp / BFIA 等） |
| **KusciaAPI** | `pkg/kusciaapi/` | 统一对外 API 接入层（gRPC/HTTP） |
| **DataMesh** | `pkg/datamesh/` | 数据源/数据集元信息管理、数据读写服务 |
| **ConfManager** | `pkg/confmanager/` | 配置与证书管理服务 |
| **Transport** | `pkg/transport/` | 消息队列模式传输层组件 |
| **Reporter** | `pkg/reporter/` | 状态/事件上报 |
| **MetricExporter / NodeExporter / SsExporter** | `pkg/metricexporter/` 等 | 监控指标采集与暴露 |
| **Diagnose** | `pkg/diagnose/` | 网络、路由、日志等诊断工具 |

### 4.3 关键控制器

| 控制器 | 代码路径 | 职责 |
| -------- | ---------- | ------ |
| **KusciaJob Controller** | `pkg/controllers/kusciajob/` | 解析 Job DAG，按依赖关系创建 KusciaTask，管理 Job 状态机 |
| **KusciaTask Controller** | `pkg/controllers/kusciatask/` | 解析 Task，创建 Pod/Service/ConfigMap/TaskResourceGroup，实现多方 Co-Scheduling |
| **TaskResourceGroup Controller** | `pkg/controllers/taskresourcegroup/` | 跨域资源预留协调 |
| **Kuscia Scheduler** | `pkg/scheduler/kusciascheduling/` | PodGroup 的 All-or-Nothing Co-Scheduling |
| **Domain Controller** | `pkg/controllers/domain/` | 管理 Domain 资源，创建 Namespace、ResourceQuota、部署 Token |
| **DomainRoute Controller** | `pkg/controllers/domainroute/`、`pkg/controllers/clusterdomainroute/` | 管理 DomainRoute / ClusterDomainRoute，控制路由与认证 |
| **Data Controller** | `pkg/controllers/domaindata/` | 数据授权管理相关 |
| **InterConn Controllers** | `pkg/interconn/kuscia/`、`pkg/interconn/bfia/` | 互联互通协议适配 |

---

## 5. 运行时与资源隔离

### 5.1 Agent 架构概述

#### 5.1.1 Kubernetes 基础概念

在深入 Agent 之前,先了解 Kubernetes (K8s) 的两个核心概念:**节点(Node)** 和 **Pod**。

##### A. 什么是节点 (Node)?

**节点**是 Kubernetes 集群中的**工作机器**,可以是一台:

- 物理服务器
- 虚拟机 (VM)
- 云服务器实例 (如 AWS EC2、阿里云 ECS)

**节点的作用**:

```
┌──────────────────────────────────────────┐
│         Node (物理机/虚拟机)              │
├──────────────────────────────────────────┤
│                                          │
│  硬件资源:                                │
│  - CPU: 16 cores                         │
│  - Memory: 64 GB                         │
│  - Disk: 500 GB SSD                      │
│  - Network: 1 Gbps                       │
│                                          │
│  运行的组件:                              │
│  - Agent (Kuscia 的节点代理)             │
│  - containerd (容器运行时)               │
│  - kubelet (K8s 节点代理,可选)           │
│                                          │
└──────────────────────────────────────────┘
```

**节点的关键特性**:

1. **资源提供者**:为 Pod 提供 CPU、内存、存储等计算资源
2. **注册机制**:向 K8s API Server 注册自己,宣告可用资源
3. **状态上报**:定期上报健康状态(Ready/NotReady)和资源使用情况
4. **Pod 承载**:一个节点可以同时运行多个 Pod

**查看节点信息**:

```bash
# 查看所有节点
kubectl get nodes

# 输出示例:
# NAME     STATUS   ROLES    AGE   VERSION
# node-1   Ready    <none>   10d   v1.27.3+k3s1

# 查看节点详细信息
kubectl describe node node-1

# 输出包含:
# - Capacity: 总资源(CPU: 16, Memory: 64Gi, Pods: 110)
# - Allocatable: 可分配资源(扣除系统保留)
# - Conditions: 节点状态(Ready, MemoryPressure, DiskPressure)
# - Addresses: IP 地址信息
```

##### B. 什么是 Pod?

**Pod** 是 Kubernetes 中**最小的调度单元**,可以理解为一个"逻辑主机",包含:

- 一个或多个容器(Container)
- 共享的网络命名空间(同一 IP)
- 共享的存储卷(Volumes)
- 统一的资源配置(CPU/Memory limits)

**Pod 的特点**:

```
┌─────────────────────────────────────────────┐
│              Pod (最小调度单元)               │
├─────────────────────────────────────────────┤
│                                             │
│  ┌──────────────┐  ┌──────────────┐        │
│  │ Container 1  │  │ Container 2  │        │
│  │ (主应用)      │  │ (Sidecar)    │        │
│  └──────────────┘  └──────────────┘        │
│                                             │
│  共享资源:                                   │
│  - IP: 10.42.0.5                           │
│  - Volume: /data (挂载到两个容器)           │
│  - Network: localhost 通信                  │
│                                             │
└─────────────────────────────────────────────┘
```

**关键概念**:

1. **原子调度**:Pod 是调度的最小单位,不能调度单个容器
2. **共享网络**:Pod 内所有容器共享同一个 IP 和端口空间
3. **共享存储**:通过 Volumes 实现容器间文件共享
4. **同生共死**:Pod 内的容器一起启动、一起停止
5. **短暂性**:Pod 是 ephemeral(短暂的),重启后会创建新 Pod

**Pod YAML 示例**:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: secretflow-task-pod
  namespace: alice
spec:
  # 资源配置
  containers:
  - name: ray-head
    image: secretflow/ray:latest
    resources:
      requests:
        cpu: "2"
        memory: "4Gi"
      limits:
        cpu: "4"
        memory: "8Gi"
    ports:
    - containerPort: 6379  # Ray 端口
    
  - name: app-worker
    image: secretflow/app:latest
    resources:
      requests:
        cpu: "2"
        memory: "4Gi"
      limits:
        cpu: "4"
        memory: "8Gi"
        
  # 共享存储
  volumes:
  - name: data-volume
    hostPath:
      path: /home/kuscia/data
```

**查看 Pod 信息**:

```bash
# 查看所有 Pod
kubectl get pods -A

# 输出示例:
# NAMESPACE   NAME                    READY   STATUS    RESTARTS   AGE
# alice       psi-task-pod-abc123     2/2     Running   0          5m
# bob         psi-task-pod-def456     2/2     Running   0          5m

# 查看 Pod 详细信息
kubectl describe pod psi-task-pod-abc123 -n alice

# 查看 Pod 日志
kubectl logs psi-task-pod-abc123 -n alice -c ray-head

# 进入 Pod 内部
kubectl exec -it psi-task-pod-abc123 -n alice -- bash
```

##### C. Node 与 Pod 的关系

```
物理世界                          Kubernetes 抽象
────────────                     ─────────────────

┌──────────────┐
│  Physical    │                 ┌──────────────┐
│  Server      │  ──────►       │    Node      │
│  (物理机)     │   抽象化        │  (K8s 节点)   │
└──────────────┘                 └──────┬───────┘
                                       │
                                       │ 承载
                                       ▼
                                 ┌──────────────┐
                                 │    Pod 1     │
                                 │  (2个容器)    │
                                 └──────────────┘
                                 ┌──────────────┐
                                 │    Pod 2     │
                                 │  (1个容器)    │
                                 └──────────────┘
                                 ┌──────────────┐
                                 │    Pod 3     │
                                 │  (3个容器)    │
                                 └──────────────┘
```

**关系总结**:

- **1个 Node** 可以运行 **N个 Pod**
- **1个 Pod** 包含 **1个或多个 Container**
- Pod 是调度的最小单位,Node 是资源的提供者
- Scheduler 决定 Pod 运行在哪个 Node 上
- Agent 负责在 Node 上管理 Pod 的生命周期

##### D. 什么是 Namespace (命名空间)?

**Namespace** 是 Kubernetes 中的**逻辑隔离层**,用于在同一个物理集群中划分多个虚拟集群。

**核心作用**:

1. **资源隔离**:不同 Namespace 中的资源互不干扰
2. **权限控制**:可以针对不同 Namespace 设置不同的访问权限(RBAC)
3. **配额管理**:可以为每个 Namespace 设置资源配额(ResourceQuota)
4. **组织管理**:按团队、项目、环境等维度组织资源

**类比理解**:

```
Kubernetes Cluster (集群)
    = 一栋大楼
    
Namespace (命名空间)
    = 大楼的不同楼层
    - 1楼: alice 公司
    - 2楼: bob 公司
    - 3楼: charlie 公司
    
Pod (容器组)
    = 楼层里的办公室
    - alice 楼层可以有多个办公室
    - 每个办公室可以有多个人(容器)
    
Container (容器)
    = 办公室里的工作人员
```

**查看 Namespace**:

```bash
# 查看所有 Namespace
kubectl get namespaces

# 输出示例:
# NAME              STATUS   AGE
# alice             Active   10d
# bob               Active   10d
# charlie           Active   10d
# kuscia-system     Active   10d
# default           Active   10d
# kube-system       Active   10d

# 查看某个 Namespace 的详细信息
kubectl describe namespace alice

# 输出包含:
# - Labels: 标签信息
# - Annotations: 注释信息
# - Resource Quotas: 资源配额
# - Limit Ranges: 限制范围
```

**创建 Namespace**:

```yaml
# namespace-alice.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: alice
  labels:
    team: data-science
    environment: production
```

```bash
kubectl apply -f namespace-alice.yaml
```

**在 Namespace 中创建 Pod**:

```yaml
# pod-in-namespace.yaml
apiVersion: v1
kind: Pod
metadata:
  name: psi-task-pod
  namespace: alice  # 指定命名空间
spec:
  containers:
  - name: ray-head
    image: secretflow/ray:latest
```

```bash
# 在指定 Namespace 中创建 Pod
kubectl apply -f pod-in-namespace.yaml -n alice

# 或者在 YAML 中已经指定了 namespace
kubectl apply -f pod-in-namespace.yaml
```

**查看特定 Namespace 的资源**:

```bash
# 查看 alice Namespace 中的所有 Pod
kubectl get pods -n alice

# 输出示例:
# NAME                    READY   STATUS    RESTARTS   AGE
# psi-task-pod-001        1/1     Running   0          5m
# fl-task-pod-002         1/1     Running   0          3m
# data-process-pod-003    1/1     Running   0          1m

# 查看 alice Namespace 中的所有资源
kubectl get all -n alice

# 输出包括:
# - Pods (Pod 列表)
# - Services (服务)
# - Deployments (部署)
# - ReplicaSets (副本集)
# - StatefulSets (有状态应用)
# - DaemonSets (守护进程)
# - Jobs (任务)
# - CronJobs (定时任务)
```

**⚠️ 注意**: `kubectl get all` **不会显示所有资源类型**,它只显示常用的工作负载资源。要查看完整的资源列表,需要使用以下命令:

```bash
# 查看所有资源类型(包括 ConfigMaps, Secrets 等)
kubectl api-resources --namespaced=true | grep -E "NAME|configmap|secret|serviceaccount"

# 或者分别查询不同类型的资源
kubectl get configmaps -n alice
kubectl get secrets -n alice
kubectl get serviceaccounts -n alice
kubectl get kusciajobs -n alice      # Kuscia 自定义资源
kubectl get kusciatasks -n alice     # Kuscia 自定义资源
kubectl get domaindata -n alice      # Kuscia 自定义资源
```

---

#### 5.1.2 Namespace 中的完整资源类型详解

Kubernetes Namespace 中可以包含**多种类型的资源**,下面按功能分类详细介绍:

##### A. 计算资源 (Workloads)

这些是**运行应用程序**的核心资源:

| 资源类型 | 简称 | 说明 | 示例 |
| --------- | ----- | ------ | ------ |
| **Pod** | po | 最小调度单元,包含一个或多个容器 | `psi-task-pod-001` |
| **Deployment** | deploy | 无状态应用的声明式管理 | `nginx-deployment` |
| **ReplicaSet** | rs | 维护 Pod 副本数量 | `nginx-rs-abc123` |
| **StatefulSet** | sts | 有状态应用的管理(如数据库) | `mysql-sts` |
| **DaemonSet** | ds | 在每个节点上运行一个 Pod | `log-collector-ds` |
| **Job** | job | 一次性任务,完成后退出 | `backup-job` |
| **CronJob** | cj | 定时执行的任务 | `daily-cleanup-cj` |
| **ReplicationController** | rc | 老版本的副本管理(已废弃) | `legacy-rc` |

**在 Kuscia 中的应用**:

```bash
# 查看隐私计算任务的 Pod
kubectl get pods -n alice
# NAME                              READY   STATUS    AGE
# psi-task-001-alice-pod-0          1/1     Running   5m
# fl-task-002-alice-pod-0           1/1     Running   3m

# 查看 Deployment (如果使用 K8s 模式)
kubectl get deployments -n alice
# NAME              READY   UP-TO-DATE   AVAILABLE   AGE
# gateway-deploy    1/1     1            1           10d

# 查看 Job
kubectl get jobs -n alice
# NAME             COMPLETIONS   DURATION   AGE
# data-import      1/1           30s        1h
```

**Pod 示例**:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: psi-task-pod-001
  namespace: alice
spec:
  containers:
  - name: ray-head
    image: secretflow/ray:latest
    command: ["python", "/app/psi_compute.py"]
```

**Deployment 示例**:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: alice
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:latest
        ports:
        - containerPort: 80
```

---

##### B. 网络资源 (Networking)

这些资源用于**服务发现和负载均衡**:

| 资源类型 | 简称 | 说明 | 示例 |
| --------- | ----- | ------ | ------ |
| **Service** | svc | 稳定的网络端点,提供负载均衡 | `psi-service` |
| **Endpoint** | ep | Service 背后的实际 Pod IP | `psi-service-ep` |
| **EndpointSlice** | eps | Endpoint 的扩展版本 | `psi-service-eps` |
| **Ingress** | ing | HTTP/HTTPS 路由规则 | `api-ingress` |
| **NetworkPolicy** | netpol | 网络访问控制策略 | `deny-all-policy` |

**在 Kuscia 中的应用**:

```bash
# 查看 Service
kubectl get services -n alice
# NAME            TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)     AGE
# psi-service     ClusterIP   10.43.123.45    <none>        54509/TCP   5m
# envoy-service   ClusterIP   10.43.67.89     <none>        8080/TCP    10d

# 查看 Endpoint
kubectl get endpoints -n alice
# NAME            ENDPOINTS           AGE
# psi-service     10.42.0.15:54509    5m

# 查看 NetworkPolicy
kubectl get networkpolicies -n alice
# NAME              POD-SELECTOR   AGE
# default-deny      <none>         10d
```

**Service 示例**:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: psi-service
  namespace: alice
spec:
  selector:
    app: psi
    task-id: psi-task-001
  ports:
  - name: psi-port
    port: 54509
    targetPort: 54509
    protocol: TCP
  type: ClusterIP  # 集群内部访问
```

**NetworkPolicy 示例**:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-all
  namespace: alice
spec:
  podSelector: {}  # 选择所有 Pod
  policyTypes:
  - Ingress
  - Egress
  # 默认拒绝所有进出流量
```

---

##### C. 存储资源 (Storage)

这些资源用于**数据持久化和配置管理**:

| 资源类型 | 简称 | 说明 | 示例 |
| --------- | ----- | ------ | ------ |
| **PersistentVolumeClaim** | pvc | 存储卷申请 | `psi-data-pvc` |
| **PersistentVolume** | pv | 存储卷(集群级) | `nfs-pv-001` |
| **ConfigMap** | cm | 非敏感配置数据 | `task-config-cm` |
| **Secret** | secret | 敏感数据(密码、密钥) | `tls-secret` |
| **StorageClass** | sc | 存储类定义(集群级) | `fast-ssd` |

**在 Kuscia 中的应用**:

```bash
# 查看 ConfigMap
kubectl get configmaps -n alice
# NAME               DATA   AGE
# task-config        3      5m
# envoy-config       5      10d

# 查看 Secret
kubectl get secrets -n alice
# NAME                  TYPE                                  DATA   AGE
# tls-cert              kubernetes.io/tls                     2      10d
# registry-credentials  kubernetes.io/dockerconfigjson        1      10d

# 查看 PVC
kubectl get pvc -n alice
# NAME            STATUS   VOLUME    CAPACITY   ACCESS MODES   AGE
# psi-data-pvc    Bound    pv-001    10Gi       RWO            5m
```

**ConfigMap 示例**:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: task-config
  namespace: alice
data:
  task_id: "psi-task-001"
  party_id: "alice"
  config.yaml: |
    max_iterations: 100
    learning_rate: 0.01
```

**Secret 示例**:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: tls-cert
  namespace: alice
type: kubernetes.io/tls
data:
  tls.crt: LS0tLS1CRUdJTi...  # Base64 编码的证书
  tls.key: LS0tLS1CRUdJTi...  # Base64 编码的私钥
```

**PVC 示例**:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: psi-data-pvc
  namespace: alice
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: fast-ssd
```

---

##### D. 身份与权限资源 (RBAC)

这些资源用于**认证和授权**:

| 资源类型 | 简称 | 说明 | 示例 |
| --------- | ----- | ------ | ------ |
| **ServiceAccount** | sa | 服务账号,Pod 的身份标识 | `alice-sa` |
| **Role** | role | 命名空间内的角色定义 | `pod-reader` |
| **RoleBinding** | rb | 将角色绑定到用户/账号 | `alice-pod-reader-binding` |
| **LimitRange** | limitrange | 资源限制范围 | `default-limits` |
| **ResourceQuota** | quota | 资源配额限制 | `alice-quota` |

**在 Kuscia 中的应用**:

```bash
# 查看 ServiceAccount
kubectl get serviceaccounts -n alice
# NAME      SECRETS   AGE
# default   1         10d
# alice-sa  1         10d

# 查看 Role
kubectl get roles -n alice
# NAME          AGE
# pod-reader    10d
# admin         10d

# 查看 RoleBinding
kubectl get rolebindings -n alice
# NAME                    ROLE           AGE
# alice-pod-reader        Role/pod-reader   10d

# 查看 ResourceQuota
kubectl get resourcequotas -n alice
# NAME          AGE   REQUEST                                      LIMIT
# alice-quota   10d   cpu: 8/16, memory: 16Gi/32Gi, pods: 10/20   cpu: 16/16, memory: 32Gi/32Gi
```

**ServiceAccount 示例**:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: alice-sa
  namespace: alice
```

**Role 示例**:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: pod-reader
  namespace: alice
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "watch", "list"]
```

**RoleBinding 示例**:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: alice-pod-reader-binding
  namespace: alice
subjects:
- kind: ServiceAccount
  name: alice-sa
  namespace: alice
roleRef:
  kind: Role
  name: pod-reader
  apiGroup: rbac.authorization.k8s.io
```

**ResourceQuota 示例**:

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: alice-quota
  namespace: alice
spec:
  hard:
    cpu: "16"              # CPU 总量限制
    memory: 32Gi           # 内存总量限制
    pods: "20"             # Pod 数量限制
    persistentvolumeclaims: "10"  # PVC 数量限制
```

---

##### E. Kuscia 自定义资源 (CRDs)

这些是 **Kuscia 特有的资源类型**,用于隐私计算任务编排:

| 资源类型 | 简称 | 说明 | 示例 |
| --------- | ----- | ------ | ------ |
| **KusciaJob** | kj | 隐私计算作业(DAG) | `psi-job-001` |
| **KusciaTask** | kt | 隐私计算任务(单个节点) | `psi-task-001` |
| **TaskResourceGroup** | trg | 任务资源组 | `psi-task-001-trg` |
| **TaskResource** | tr | 任务资源(单参与方) | `psi-task-001-alice-tr` |
| **Domain** | domain | 参与方域信息 | `alice`, `bob` |
| **DomainData** | dd | 数据资产注册 | `alice-training-data` |
| **DomainDataSource** | dds | 数据源配置 | `mysql-source` |
| **DomainRoute** | dr | 跨域路由配置 | `alice-to-bob-route` |
| **AppImage** | aimg | 应用镜像配置 | `psi-image` |
| **ClusterDomainRoute** | cdr | 集群级路由 | `global-route` |
| **Gateway** | gw | 网关配置 | `alice-gateway` |
| **InteropConfig** | ic | 互联互通配置 | `interop-config` |

**查看 Kuscia CRDs**:

```bash
# 查看所有 Kuscia CRDs
kubectl get crds | grep kuscia
# kusciajobs.kuscia.secretflow
# kusciatasks.kuscia.secretflow
# taskresourcegroups.kuscia.secretflow
# taskresources.kuscia.secretflow
# domains.kuscia.secretflow
# domaindatas.kuscia.secretflow
# ...

# 查看特定 Namespace 中的 Kuscia 资源
kubectl get kusciajobs -n alice
# NAME           STATUS      AGE
# psi-job-001    Running     5m
# fl-job-002     Pending     3m

kubectl get kusciatasks -n alice
# NAME             STATUS      AGE
# psi-task-001     Running     5m

kubectl get taskresourcegroups -n alice
# NAME                     STATUS      AGE
# psi-task-001-trg         Reserved    5m

kubectl get taskresources -n alice
# NAME                     STATUS      AGE
# psi-task-001-alice-tr    Reserved    5m

kubectl get domaindata -n alice
# NAME                  TYPE       OWNER   AGE
# alice-training-data   localfs    alice   10d
# alice-test-data       mysql      alice   10d

kubectl get appimages
# NAME        AGE
# psi-image   10d
# fl-image    10d
```

**KusciaJob 示例**:

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-job-001
  namespace: alice
spec:
  initiator: alice
  maxParallelism: 2
  tasks:
  - taskID: psi-task-001
    taskAlias: psi-compute
    parties:
    - domainID: alice
      role: Client
      appImage: psi-image
    - domainID: bob
      role: Server
      appImage: psi-image
```

**DomainData 示例**:

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: alice-training-data
  namespace: alice
spec:
  name: "训练数据集"
  type: localfs
  attributes:
    format: csv
    delimiter: ","
  localPath: /home/kuscia/var/storage/data/training.csv
```

---

##### F. 其他资源 (Others)

| 资源类型 | 简称 | 说明 | 示例 |
| --------- | ----- | ------ | ------ |
| **Event** | ev | 事件记录 | `pod-created-event` |
| **HorizontalPodAutoscaler** | hpa | 水平自动扩缩容 | `nginx-hpa` |
| **PodDisruptionBudget** | pdb | Pod 中断预算 | `psi-pdb` |
| **PriorityClass** | pc | 优先级类(集群级) | `high-priority` |

**查看事件**:

```bash
# 查看 Namespace 中的事件
kubectl get events -n alice --sort-by='.lastTimestamp'
# LAST SEEN   TYPE      REASON      OBJECT                        MESSAGE
# 5m          Normal    Scheduled   pod/psi-task-pod-001          Successfully assigned alice/psi-task-pod-001 to node-1
# 5m          Normal    Pulled      pod/psi-task-pod-001          Container image "secretflow/ray:latest" already present
# 5m          Normal    Created     pod/psi-task-pod-001          Created container ray-head
# 5m          Normal    Started     pod/psi-task-pod-001          Started container ray-head
```

---

##### G. 资源层次关系图

```
Namespace: alice
│
├─ 计算资源 (Workloads)
│  ├─ Pod: psi-task-pod-001
│  │  └─ Container: ray-head
│  ├─ Deployment: nginx-deploy
│  │  └─ ReplicaSet: nginx-rs-abc123
│  │     └─ Pod: nginx-pod-xyz789
│  └─ Job: backup-job
│     └─ Pod: backup-pod-001
│
├─ 网络资源 (Networking)
│  ├─ Service: psi-service
│  │  └─ Endpoint: 10.42.0.15:54509
│  └─ NetworkPolicy: deny-all
│
├─ 存储资源 (Storage)
│  ├─ ConfigMap: task-config
│  ├─ Secret: tls-cert
│  └─ PVC: psi-data-pvc
│     └─ PV: pv-001 (集群级)
│
├─ RBAC 资源
│  ├─ ServiceAccount: alice-sa
│  ├─ Role: pod-reader
│  ├─ RoleBinding: alice-pod-reader-binding
│  └─ ResourceQuota: alice-quota
│
└─ Kuscia CRDs
   ├─ KusciaJob: psi-job-001
   ├─ KusciaTask: psi-task-001
   ├─ TaskResourceGroup: psi-task-001-trg
   ├─ TaskResource: psi-task-001-alice-tr
   ├─ DomainData: alice-training-data
   └─ AppImage: psi-image
```

---

##### H. 常用查询命令速查表

```bash
# ========== 查看所有资源的通用方法 ==========

# 方法 1: 列出所有资源类型
kubectl api-resources --namespaced=true

# 方法 2: 查看特定 Namespace 的所有资源
kubectl get all -n alice

# 方法 3: 分别查询各类资源
kubectl get pods,services,deployments,configmaps,secrets -n alice

# 方法 4: 查看 Kuscia 自定义资源
kubectl get kusciajobs,kusciatasks,domaindata,appimages -n alice

# ========== 按资源类型查询 ==========

# 计算资源
kubectl get pods -n alice
kubectl get deployments -n alice
kubectl get jobs -n alice

# 网络资源
kubectl get services -n alice
kubectl get endpoints -n alice
kubectl get networkpolicies -n alice

# 存储资源
kubectl get configmaps -n alice
kubectl get secrets -n alice
kubectl get pvc -n alice

# RBAC 资源
kubectl get serviceaccounts -n alice
kubectl get roles -n alice
kubectl get rolebindings -n alice
kubectl get resourcequotas -n alice

# Kuscia CRDs
kubectl get kusciajobs -n alice
kubectl get kusciatasks -n alice
kubectl get taskresourcegroups -n alice
kubectl get taskresources -n alice
kubectl get domaindata -n alice
kubectl get domaindatasources -n alice
kubectl get domainroutes -n alice
kubectl get appimages -n alice  # 集群级,不需要 -n

# 事件
kubectl get events -n alice --sort-by='.lastTimestamp'

# ========== 高级查询 ==========

# 按标签筛选
kubectl get pods -n alice -l app=psi
kubectl get pods -n alice -l task-type=computing

# 查看详细信息
kubectl get pods -n alice -o wide
kubectl get services -n alice -o wide

# 以 YAML 格式输出
kubectl get pod psi-task-pod-001 -n alice -o yaml

# 以 JSON 格式输出
kubectl get pod psi-task-pod-001 -n alice -o json

# 只输出特定字段
kubectl get pod psi-task-pod-001 -n alice -o jsonpath='{.status.phase}'
kubectl get pod psi-task-pod-001 -n alice -o jsonpath='{.spec.containers[0].image}'

# 统计资源数量
kubectl get pods -n alice --no-headers | wc -l

# 监控资源变化
kubectl get pods -n alice -w

# 导出所有资源配置
kubectl get all -n alice -o yaml > namespace-alice-backup.yaml
```

---

##### I. 资源生命周期管理

**创建资源**:

```bash
# 从 YAML 文件创建
kubectl apply -f pod.yaml -n alice
kubectl apply -f service.yaml -n alice

# 命令行直接创建
kubectl run nginx --image=nginx -n alice
kubectl create configmap my-config --from-literal=key=value -n alice
```

**更新资源**:

```bash
# 编辑资源
kubectl edit pod psi-task-pod-001 -n alice

# 应用新的配置
kubectl apply -f updated-pod.yaml -n alice

# 打补丁
kubectl patch pod psi-task-pod-001 -n alice -p '{"spec":{"restartPolicy":"Never"}}'
```

**删除资源**:

```bash
# 删除单个资源
kubectl delete pod psi-task-pod-001 -n alice
kubectl delete service psi-service -n alice

# 从 YAML 文件删除
kubectl delete -f pod.yaml -n alice

# 删除所有资源(危险!)
kubectl delete all --all -n alice

# 删除整个 Namespace(会级联删除所有资源)
kubectl delete namespace alice
```

**备份和恢复**:

```bash
# 备份 Namespace 所有资源
kubectl get all -n alice -o yaml > backup.yaml
kubectl get configmaps,secrets -n alice -o yaml >> backup.yaml

# 恢复资源
kubectl apply -f backup.yaml -n alice
```

---

##### J. 在 Kuscia 中的实际应用

在 Kuscia 隐私计算场景中,一个典型的 Namespace 包含以下资源:

```bash
# 查看 alice Namespace 的完整资源清单
kubectl get all,pvc,cm,secret,sa,role,rb,kj,kt,trg,tr,dd,aimg -n alice
```

**典型输出**:

```
NAMESPACE: alice

【计算资源】
PODS:
  psi-task-001-alice-pod-0    Running   2/2     5m
  fl-task-002-alice-pod-0     Running   1/1     3m

【网络资源】
SERVICES:
  psi-service                 ClusterIP   10.43.123.45   54509/TCP
  fl-service                  ClusterIP   10.43.67.89    8080/TCP

【存储资源】
CONFIGMAPS:
  task-config-psi             3 data entries
  task-config-fl              2 data entries
  
SECRETS:
  tls-cert                    kubernetes.io/tls
  
PVCs:
  psi-data-pvc                Bound   10Gi

【RBAC 资源】
SERVICEACCOUNTS:
  alice-sa
  
ROLES:
  pod-reader
  
ROLEBINDINGS:
  alice-pod-reader-binding
  
RESOURCEQUOTAS:
  alice-quota                 cpu: 8/16, memory: 16Gi/32Gi

【Kuscia CRDs】
KUSCIAJOBS:
  psi-job-001                 Running
  fl-job-002                  Pending
  
KUSCIATASKS:
  psi-task-001                Running
  fl-task-002                 Pending
  
TASKRESOURCEGROUPS:
  psi-task-001-trg            Reserved
  
TASKRESOURCES:
  psi-task-001-alice-tr       Reserved
  
DOMAINDATA:
  alice-training-data         localfs
  alice-test-data             mysql
  
APPIMAGES (集群级):
  psi-image
  fl-image
```

这种资源组织方式使得:

1. ✅ **隔离性**:每个参与方的资源完全隔离
2. ✅ **可管理性**:通过 Namespace 快速定位资源
3. ✅ **安全性**:RBAC 控制访问权限
4. ✅ **可追溯性**:所有任务都有完整的资源记录
5. ✅ **灵活性**:支持多种资源类型协同工作

```

**Namespace 与 Pod 的关系**:
```

Kubernetes Cluster (集群)
    │
    ├─ Namespace: alice (命名空间1)
    │   ├─ Pod: psi-task-pod-abc123
    │   │   ├─ Container: ray-head
    │   │   └─ Container: app-worker
    │   ├─ Pod: fl-task-pod-def456
    │   │   └─ Container: trainer
    │   └─ Pod: data-preprocess-pod-ghi789
    │       └─ Container: preprocessor
    │
    ├─ Namespace: bob (命名空间2)
    │   ├─ Pod: psi-task-pod-jkl012
    │   │   ├─ Container: ray-head
    │   │   └─ Container: app-worker
    │   └─ Pod: mpc-task-pod-mno345
    │       └─ Container: mpc-compute
    │
    └─ Namespace: kuscia-system (系统命名空间)
        ├─ Pod: gateway-pod-xyz789
        └─ Pod: core-dns-pod-uvw456

```

**关键关系**:
- **1个 Cluster** 包含 **N个 Namespace**
- **1个 Namespace** 包含 **M个 Pod**
- **1个 Pod** 包含 **K个 Container**
- Pod 必须属于某个 Namespace(默认是 `default`)
- 不同 Namespace 中的 Pod 可以同名(因为命名空间隔离)

**常见误解澄清**:

| 误解 | 正确理解 |
|------|----------|
| "一个 Namespace 就是一个 Pod" | ❌ 一个 Namespace 可以包含多个 Pod |
| "Pod 必须在不同的 Namespace" | ❌ 同一 Namespace 可以有多个 Pod |
| "Namespace 是物理隔离" | ❌ Namespace 是逻辑隔离,共享底层硬件 |
| "删除 Namespace 会删除所有 Pod" | ✅ 是的,删除 Namespace 会级联删除其中所有资源 |
| "Namespace 之间完全隔离" | ⚠️ 网络层面可以通过 Service 跨 Namespace 通信 |

**实际操作示例**:
```bash
# 1. 创建两个 Namespace
kubectl create namespace alice
kubectl create namespace bob

# 2. 在 alice 中创建 3 个 Pod
kubectl run pod1 --image=nginx -n alice
kubectl run pod2 --image=redis -n alice
kubectl run pod3 --image=mysql -n alice

# 3. 在 bob 中创建 2 个 Pod
kubectl run pod4 --image=nginx -n bob
kubectl run pod5 --image=redis -n bob

# 4. 查看各 Namespace 的 Pod
kubectl get pods -n alice
# NAME   READY   STATUS    RESTARTS   AGE
# pod1   1/1     Running   0          1m
# pod2   1/1     Running   0          1m
# pod3   1/1     Running   0          1m

kubectl get pods -n bob
# NAME   READY   STATUS    RESTARTS   AGE
# pod4   1/1     Running   0          1m
# pod5   1/1     Running   0          1m

# 5. 查看所有 Namespace
kubectl get namespaces
# NAME              STATUS   AGE
# alice             Active   5m
# bob               Active   5m
# default           Active   10d
# kube-system       Active   10d

# 6. 删除 alice Namespace (会删除其中所有 Pod)
kubectl delete namespace alice
# namespace "alice" deleted
# 现在 pod1, pod2, pod3 都被删除了

# 验证
kubectl get pods -n alice
# Error from server (NotFound): namespaces "alice" not found
```

**在 Kuscia 中的应用**:

在 Kuscia 隐私计算场景中,Namespace 用于隔离不同参与方(Domain)的资源:

```
参与方 alice (Domain)
    ↓
对应 Namespace: alice
    ↓
包含多个 Pod:
    ├─ Pod 1: PSI 任务 (ray-head + workers)
    ├─ Pod 2: 联邦学习任务 (trainer + evaluator)
    └─ Pod 3: 数据预处理任务 (preprocessor)

参与方 bob (Domain)
    ↓
对应 Namespace: bob
    ↓
包含多个 Pod:
    ├─ Pod 1: PSI 任务 (与 alice 协同)
    └─ Pod 2: MPC 任务 (多方安全计算)
```

##### 隐私计算任务与 Pod 的关系

**是的,隐私计算任务会运行在 Pod 上!** 这是 Kuscia 的核心设计理念。

**执行流程**:

```
用户提交 KusciaJob (隐私计算任务)
    ↓
Kuscia 控制器解析任务配置
    ↓
为每个参与方创建对应的 Pod
    ↓
Pod 中运行 SecretFlow 计算引擎
    ↓
实际执行隐私计算算法(PSI/FL/MPC等)
```

**具体示例 - PSI 任务**:

```yaml
# 用户提交的隐私求交任务
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-job-001
  namespace: alice
spec:
  initiator: alice
  tasks:
  - taskID: psi-task-001
    parties:
    - domainID: alice
      appImage: secretflow/psi:latest  # SecretFlow PSI 镜像
    - domainID: bob
      appImage: secretflow/psi:latest
```

**系统自动创建的 Pod**:

```bash
# Alice 方的 Pod
kubectl get pods -n alice | grep psi
# NAME                              READY   STATUS    AGE
# psi-task-001-alice-pod-0          1/1     Running   5m

# Bob 方的 Pod  
kubectl get pods -n bob | grep psi
# NAME                              READY   STATUS    AGE
# psi-task-001-bob-pod-0            1/1     Running   5m
```

**Pod 内部运行的内容**:

```
┌──────────────────────────────────────────┐
│  Pod: psi-task-001-alice-pod-0           │
├──────────────────────────────────────────┤
│                                          │
│  Container: ray-head                     │
│  - Image: secretflow/ray:latest         │
│  - Process: python psi_compute.py       │
│  - Resources: 2 CPU, 4GB Memory         │
│                                          │
│  Container: ray-worker (可选)            │
│  - Image: secretflow/ray:latest         │
│  - Process: worker process              │
│                                          │
└──────────────────────────────────────────┘
```

**支持的隐私计算任务类型**:

| 任务类型 | 说明 | 运行镜像 | Pod 示例 |
| --------- | ------ | --------- | ---------- |
| **PSI** | 隐私集合求交 | secretflow/psi:latest | psi-compute-pod |
| **联邦学习** | 横向/纵向联邦训练 | secretflow/fl:latest | fl-train-pod |
| **MPC** | 多方安全计算 | secretflow/mpc:latest | mpc-compute-pod |
| **TEE** | 可信执行环境 | secretflow/tee:latest | tee-task-pod |
| **数据处理** | 数据预处理/特征工程 | secretflow/data:latest | data-process-pod |

**查看正在运行的隐私计算任务**:

```bash
# 方法1: 通过 KusciaJob 查看
kubectl get kusciajobs -n alice
# NAME           STATUS      AGE
# psi-job-001    Running     10m
# fl-job-002     Pending     5m

# 方法2: 通过 KusciaTask 查看
kubectl get kusciatasks -n alice
# NAME             STATUS      AGE
# psi-task-001     Running     10m
# fl-task-002      Pending     5m

# 方法3: 直接查看 Pod
kubectl get pods -n alice
# NAME                              READY   STATUS    AGE
# psi-task-001-alice-pod-0          1/1     Running   10m
# fl-task-002-alice-pod-0           1/1     Running   5m

# 方法4: 查看任务容器的日志
kubectl logs psi-task-001-alice-pod-0 -n alice -f
# [2024-01-15 10:30:00] Starting PSI computation...
# [2024-01-15 10:30:05] Loading dataset: 1000000 records
# [2024-01-15 10:30:10] Computing intersection...
# [2024-01-15 10:30:30] PSI completed! Result: 850000 matches
```

**进入任务 Pod 调试**:

```bash
# 进入正在运行的隐私计算任务 Pod
kubectl exec -it psi-task-001-alice-pod-0 -n alice -- bash

# 在 Pod 内部
root@psi-task-001-alice-pod-0:/app# ps aux
# USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
# root         1  2.3  1.5 123456 78901 ?        Ss   10:30   0:15 python psi_compute.py
# root        15  0.5  0.3 45678 23456 ?        S    10:30   0:03 ray worker

# 查看计算结果
root@psi-task-001-alice-pod-0:/app# ls -lh /data/output/
# -rw-r--r-- 1 root root 50M Jan 15 10:30 psi_result.csv

# 查看资源使用
root@psi-task-001-alice-pod-0:/app# top
```

**任务完成后的清理**:

```bash
# 任务完成后,Pod 会根据重启策略处理
# restartPolicy: Never - Pod 保持 Completed 状态
kubectl get pods -n alice
# NAME                              READY   STATUS      AGE
# psi-task-001-alice-pod-0          0/1     Completed   1h

# 删除已完成的 Pod
kubectl delete pod psi-task-001-alice-pod-0 -n alice

# 或者删除整个 Job(会自动删除关联的 Pod)
kubectl delete kusciajob psi-job-001 -n alice
```

**容器嵌套架构** (Docker 部署模式):

在 Docker 部署模式下,存在**容器嵌套容器**的架构:

```
宿主机 (Host)
    │
    ├─ Docker 启动节点容器 (alice-node)
    │   │
    │   ├─ K3s Server (嵌入式)
    │   ├─ Kuscia Agent
    │   │
    │   └─ crictl 启动任务容器 (psi-task-container)
    │       │
    │       └─ 运行隐私计算程序 (python psi_compute.py)
```

**关键理解**:

- **宿主机** → Docker 启动节点容器
- **节点容器内** → K3s + Agent 运行
- **任务容器** → Agent 通过 crictl 在节点容器内创建
- **隐私计算程序** → 在任务容器中执行

这种设计实现了:

1. **资源隔离**:每个任务有独立的容器和资源限制
2. **安全隔离**:任务之间互不影响,数据隔离
3. **灵活调度**:可以动态分配任务到不同节点
4. **易于管理**:通过 Kubernetes API 统一管理

**为什么 Kuscia 需要 Namespace?**

1. **数据隔离**: alice 的数据不会泄露给 bob

   ```bash
   # alice 的 DomainData 只在 alice Namespace
   kubectl get domaindata -n alice
   
   # bob 无法直接访问 alice 的数据
   kubectl get domaindata -n bob  # 看不到 alice 的数据
   ```

2. **权限隔离**: alice 的管理员只能管理 alice 的资源

   ```yaml
   # RoleBinding 限制权限到特定 Namespace
   apiVersion: rbac.authorization.k8s.io/v1
   kind: RoleBinding
   metadata:
     name: alice-admin
     namespace: alice  # 只在 alice Namespace 有效
   subjects:
   - kind: User
     name: alice-admin-user
   roleRef:
     kind: Role
     name: admin
   ```

3. **资源配额**: 可以限制每个参与方的资源使用

   ```yaml
   # ResourceQuota 限制 alice Namespace 的资源
   apiVersion: v1
   kind: ResourceQuota
   metadata:
     name: alice-quota
     namespace: alice
   spec:
     hard:
       cpu: "16"           # 最多 16 核 CPU
       memory: 32Gi        # 最多 32GB 内存
       pods: "20"          # 最多 20 个 Pod
       persistentvolumeclaims: "10"  # 最多 10 个 PVC
   ```

4. **清晰管理**: 通过 Namespace 快速区分不同参与方的资源

   ```bash
   # 查看 alice 的所有任务
   kubectl get kusciajobs -n alice
   
   # 查看 bob 的所有任务
   kubectl get kusciajobs -n bob
   
   # 一目了然,不会混淆
   ```

**Namespace 的最佳实践**:

1. **按参与方划分**: 每个 Domain 一个 Namespace
2. **命名规范**: 使用有意义的名称(如 `alice`, `bob`, `charlie`)
3. **标签管理**: 为 Namespace 添加标签便于筛选

   ```yaml
   metadata:
     name: alice
     labels:
       domain-type: participant
       organization: company-a
       environment: production
   ```

4. **资源配额**: 为每个 Namespace 设置合理的资源限制
5. **权限最小化**: 遵循最小权限原则,只授予必要的访问权限

---

#### 5.1.3 K3s 简介

**K3s** 是 Rancher 开发的**轻量级 Kubernetes 发行版**,专为边缘计算、IoT 和嵌入式场景设计。

##### A. 为什么 Kuscia 选择 K3s?

| 对比项 | 标准 K8s | K3s | Kuscia 的需求 |
| -------- | --------- | ----- | ------------- |
| **安装包大小** | ~500MB | ~70MB | ✅ 轻量部署 |
| **内存占用** | ~2GB | ~512MB | ✅ 低资源消耗 |
| **启动时间** | ~2分钟 | ~10秒 | ✅ 快速启动 |
| **依赖组件** | etcd+apiserver+... | 二进制单文件 | ✅ 简化运维 |
| **外部依赖** | 需要 containerd 等 | 内置 containerd | ✅ 开箱即用 |
| **适用场景** | 数据中心 | 边缘/嵌入式 | ✅ 符合定位 |

**K3s 的核心优势**:

1. **单二进制文件**:所有组件打包成一个可执行文件
2. **嵌入式 etcd**:使用 SQLite 或 embedded etcd,无需独立部署
3. **自动配置**:默认启用必要组件,禁用不必要的功能
4. **低资源需求**:最低 512MB RAM 即可运行
5. **完全兼容**:100% 通过 K8s 一致性认证

##### B. K3s 架构

**标准 K8s 架构**:

```
┌─────────────────────────────────────────┐
│         Control Plane (控制平面)         │
├─────────────────────────────────────────┤
│  - kube-apiserver                       │
│  - etcd (独立部署)                       │
│  - kube-scheduler                       │
│  - kube-controller-manager              │
│  - cloud-controller-manager             │
└─────────────────────────────────────────┘
              │
              │ API 调用
              ▼
┌─────────────────────────────────────────┐
│         Worker Nodes (工作节点)          │
├─────────────────────────────────────────┤
│  Node 1:                                │
│  - kubelet                              │
│  - kube-proxy                           │
│  - containerd                           │
│  - CoreDNS                              │
│  - Flannel (CNI)                        │
└─────────────────────────────────────────┘
```

**K3s 精简架构**:

```
┌─────────────────────────────────────────┐
│      K3s Server (单进程多组件)           │
├─────────────────────────────────────────┤
│  - k3s server (主进程)                   │
│    ├─ embedded etcd                      │
│    ├─ kube-apiserver                     │
│    ├─ kube-scheduler (可禁用)            │
│    ├─ kube-controller-manager            │
│    ├─ containerd (内置)                  │
│    └─ ...                                │
└─────────────────────────────────────────┘
              │
              │ 本地调用(无网络开销)
              ▼
┌─────────────────────────────────────────┐
│         Kuscia Agent                     │
├─────────────────────────────────────────┤
│  - PodsController                        │
│  - NodeProvider                          │
│  - PodProvider                           │
└─────────────────────────────────────────┘
```

**关键区别**:

- K3s 将所有控制平面组件**打包成一个进程**
- 通过 **Unix Socket** 通信,而非网络调用
- **禁用了不必要的组件**(如 Cloud Controller、Metrics Server)
- 使用 **SQLite** 作为默认存储(也可用 embedded etcd)

##### C. K3s 在 Kuscia 中的运行模式

Kuscia 采用**嵌入式 K3s** 模式,K3s 作为 Kuscia 的子进程运行:

```
┌─────────────────────────────────────────────┐
│         Kuscia 主进程                        │
├─────────────────────────────────────────────┤
│                                             │
│  ┌──────────────────────────────────┐      │
│  │  K3s Server (子进程)              │      │
│  │  PID: 12345                       │      │
│  │                                  │      │
│  │  - API Server :6443              │      │
│  │  - etcd (embedded)               │      │
│  │  - Controller Manager            │      │
│  └──────────────────────────────────┘      │
│           │                                 │
│           │ Watch CRD Changes               │
│           ▼                                 │
│  ┌──────────────────────────────────┐      │
│  │  CRD Controllers                  │      │
│  │  - DomainData Controller         │      │
│  │  - KusciaJob Controller          │      │
│  │  - KusciaTask Controller         │      │
│  └──────────────────────────────────┘      │
│                                             │
└─────────────────────────────────────────────┘
           │
           │ Manage Pods
           ▼
┌─────────────────────────────────────────────┐
│         Agent (节点代理)                     │
├─────────────────────────────────────────────┤
│  - Register Node to K3s API                │
│  - Create/Delete Pods                      │
│  - Sync Pod Status                         │
└─────────────────────────────────────────────┘
```

**生命周期管理**:

```bash
# 启动 Kuscia (自动启动 K3s)
./kuscia start --mode Autonomy

# K3s 作为子进程启动
ps aux | grep k3s
# root     12345  2.3  1.2  /usr/local/bin/k3s server --disable-agent ...

# 停止 Kuscia (K3s 也会停止)
kill <kuscia-pid>

# Supervisor 监控 K3s,崩溃时自动重启
```

##### D. K3s 精简配置

Kuscia 禁用了大量不必要的 K8s 组件:

```go
// cmd/kuscia/modules/k3s.go
args := []string{
    "server",
    "--disable-agent",              // ✅ 禁用 Kubelet (由 Agent 替代)
    "--disable-scheduler",          // ✅ 禁用默认调度器 (使用 Kuscia Scheduler)
    "--flannel-backend=none",       // ✅ 禁用网络插件 (不使用 Pod 网络)
    "--disable=traefik",            // ✅ 禁用 Ingress Controller
    "--disable=coredns",            // ✅ 禁用 DNS (使用自定义 CoreDNS)
    "--disable=servicelb",          // ✅ 禁用 LoadBalancer
    "--disable=local-storage",      // ✅ 禁用本地存储类
    "--disable=metrics-server",     // ✅ 禁用指标服务器
}

// 非 root 用户启用 rootless 模式
if !pkgcom.IsRootUser() {
    args = append(args, "--rootless")
}
```

**保留的核心组件**:

- ✅ **API Server**:提供 RESTful API,接收 CRD 操作请求
- ✅ **etcd**:存储所有 CRD 对象(DomainData、KusciaJob 等)
- ✅ **Controller Manager**:运行内置控制器(Namespace、ServiceAccount 等)

**禁用的组件及原因**:

- ❌ **Kubelet**:Kuscia 有自己的 Agent,不需要 Kubelet
- ❌ **Scheduler**:Kuscia 有自定义调度器,支持资源预留
- ❌ **CNI (Flannel)**:隐私计算任务不需要 Pod 间网络
- ❌ **CoreDNS**:使用自定义 DNS 方案,支持跨域服务发现
- ❌ **Metrics Server**:使用自定义 MetricExporter

##### E. K3s vs 标准 K8s 对比

| 特性 | K3s | 标准 K8s |
| ------ | ----- | ---------- |
| **二进制大小** | ~70MB | ~500MB |
| **内存占用** | 512MB - 1GB | 2GB - 4GB |
| **启动时间** | 10-20秒 | 1-2分钟 |
| **存储后端** | SQLite/embedded etcd | etcd (独立集群) |
| **安装复杂度** | 单命令安装 | 复杂的多步骤安装 |
| **升级方式** | 替换二进制文件 | 滚动升级各组件 |
| **高可用** | 支持(embedded etcd) | 需要独立 etcd 集群 |
| **适用规模** | < 100 节点 | 数千节点 |
| **社区支持** | Rancher 维护 | CNCF 官方 |

**在 Kuscia 中的选择理由**:

1. **轻量级**:隐私计算节点通常资源有限
2. **易部署**:一键启动,无需复杂配置
3. **低维护**:单进程架构,故障排查简单
4. **足够用**:Kuscia 只需要 API Server + etcd,不需要完整 K8s 功能

---

#### 5.1.4 如何动态创建 Pod

在 Kubernetes (包括 K3s) 中,**动态创建 Pod** 是指通过 API 或命令行工具,在运行时按需创建新的 Pod 实例。本节详细介绍 Pod 创建的多种方式及其参数配置。

##### A. Pod 创建的核心流程

```mermaid
graph LR
    A[用户请求] --> B[K8s API Server]
    B --> C[认证授权]
    C --> D[验证 Pod Spec]
    D --> E[保存到 etcd]
    E --> F[Scheduler 调度]
    F --> G[选择 Node]
    G --> H[Kubelet/Agent 监听]
    H --> I[拉取镜像]
    I --> J[创建容器]
    J --> K[启动容器]
    K --> L[更新状态]
```

**关键步骤说明**:

1. **提交请求**:用户通过 kubectl/API/gRPC 提交 Pod 定义
2. **API Server 处理**:验证请求合法性(认证、授权、参数校验)
3. **持久化存储**:将 Pod 对象保存到 etcd
4. **调度决策**:Scheduler 选择合适的 Node(在 Kuscia 中由自定义调度器完成)
5. **Agent 执行**:PodsController 监听到新 Pod,调用运行时创建容器
6. **状态同步**:Agent 持续更新 Pod 状态到 API Server

##### B. 方式一:使用 kubectl 命令创建 Pod

**最简单的 Pod 创建**:

```bash
# 基本语法
kubectl run <pod-name> --image=<image-name> -n <namespace>

# 示例:创建一个 nginx Pod
kubectl run my-nginx --image=nginx:latest -n alice

# 输出
# pod/my-nginx created

# 查看 Pod 状态
kubectl get pods -n alice
# NAME       READY   STATUS              RESTARTS   AGE
# my-nginx   0/1     ContainerCreating   0          5s

# 等待几秒后再次查看
kubectl get pods -n alice
# NAME       READY   STATUS    RESTARTS   AGE
# my-nginx   1/1     Running   0          30s
```

**带资源限制的 Pod 创建**:

```bash
# 指定 CPU 和内存限制
kubectl run compute-task \
  --image=secretflow/ray:latest \
  --requests='cpu=2,memory=4Gi' \
  --limits='cpu=4,memory=8Gi' \
  -n alice

# 等价于以下 YAML:
# spec:
#   containers:
#   - name: compute-task
#     image: secretflow/ray:latest
#     resources:
#       requests:
#         cpu: "2"
#         memory: 4Gi
#       limits:
#         cpu: "4"
#         memory: 8Gi
```

**带环境变量的 Pod 创建**:

```bash
# 设置环境变量
kubectl run psi-worker \
  --image=secretflow/psi:latest \
  --env="TASK_TYPE=PSI" \
  --env="PARTY_ID=alice" \
  --env="WORKER_COUNT=4" \
  -n alice

# 查看环境变量
kubectl exec -it psi-worker -n alice -- env | grep TASK
# TASK_TYPE=PSI
# PARTY_ID=alice
# WORKER_COUNT=4
```

**带端口映射的 Pod 创建**:

```bash
# 暴露端口
kubectl run ray-head \
  --image=secretflow/ray:latest \
  --port=6379 \
  --port=8265 \
  -n alice

# 查看 Pod 详情
kubectl describe pod ray-head -n alice | grep Port
# Port:  6379/TCP
# Port:  8265/TCP
```

**带数据卷挂载的 Pod 创建**:

```bash
# 创建 ConfigMap
kubectl create configmap task-config \
  --from-literal=config.yaml="max_iterations: 100" \
  -n alice

# 挂载 ConfigMap 到 Pod
kubectl run trainer \
  --image=secretflow/fl:latest \
  --mount=configmap:task-config,path=/etc/config \
  -n alice

# 进入 Pod 查看挂载的文件
kubectl exec -it trainer -n alice -- cat /etc/config/config.yaml
# max_iterations: 100
```

##### C. 方式二:使用 YAML 文件创建 Pod

**完整的 Pod YAML 示例**:

```yaml
# pod-psi-task.yaml
apiVersion: v1
kind: Pod
metadata:
  name: psi-compute-pod
  namespace: alice
  labels:
    app: psi
    task-type: private-set-intersection
    party: alice
  annotations:
    description: "PSI 计算任务 Pod"
    task-id: "task-001"
spec:
  # 重启策略
  restartPolicy: Never
  
  # 服务账号
  serviceAccountName: alice-sa
  
  # 容器定义
  containers:
  - name: ray-head
    image: secretflow/ray:latest
    command: ["python", "/app/psi_task.py"]
    args:
    - "--party_id=alice"
    - "--peer_party=bob"
    - "--data_size=10000"
    
    # 资源请求和限制
    resources:
      requests:
        cpu: "2"
        memory: 4Gi
      limits:
        cpu: "4"
        memory: 8Gi
    
    # 环境变量
    env:
    - name: TASK_TYPE
      value: "PSI"
    - name: PARTY_ID
      value: "alice"
    - name: RAY_PORT
      value: "6379"
    
    # 端口暴露
    ports:
    - containerPort: 6379
      name: ray-port
      protocol: TCP
    - containerPort: 8265
      name: ray-dashboard
      protocol: TCP
    
    # 健康检查
    livenessProbe:
      httpGet:
        path: /healthz
        port: 8265
      initialDelaySeconds: 10
      periodSeconds: 30
      timeoutSeconds: 5
      failureThreshold: 3
    
    readinessProbe:
      tcpSocket:
        port: 6379
      initialDelaySeconds: 5
      periodSeconds: 10
      timeoutSeconds: 3
      failureThreshold: 5
    
    # 数据卷挂载
    volumeMounts:
    - name: data-volume
      mountPath: /data
      readOnly: true
    - name: config-volume
      mountPath: /etc/config
    - name: log-volume
      mountPath: /var/log/psi
    
    # 工作目录
    workingDir: /app
    
    # 安全上下文
    securityContext:
      runAsUser: 1000
      runAsGroup: 1000
      allowPrivilegeEscalation: false
  
  # 初始化容器
  initContainers:
  - name: init-data
    image: busybox:latest
    command: ['sh', '-c', 'echo "Initializing data..." && sleep 5']
    volumeMounts:
    - name: data-volume
      mountPath: /data
  
  # 数据卷定义
  volumes:
  - name: data-volume
    persistentVolumeClaim:
      claimName: psi-data-pvc
  - name: config-volume
    configMap:
      name: psi-task-config
  - name: log-volume
    emptyDir:
      sizeLimit: 1Gi
  
  # 节点选择器
  nodeSelector:
    kubernetes.io/hostname: node-01
    disktype: ssd
  
  # 容忍度
  tolerations:
  - key: "dedicated"
    operator: "Equal"
    value: "psi-workload"
    effect: "NoSchedule"
  
  # 亲和性
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: kubernetes.io/arch
            operator: In
            values:
            - amd64
    podAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
            - key: app
              operator: In
              values:
              - ray-cluster
          topologyKey: kubernetes.io/hostname
  
  # DNS 配置
  dnsPolicy: ClusterFirst
  dnsConfig:
    nameservers:
    - 8.8.8.8
    searches:
    - ns1.svc.cluster-domain.example
    - my.dns.search.suffix
    options:
    - name: ndots
      value: "2"
    - name: timeout
      value: "1"
  
  # 镜像拉取密钥
  imagePullSecrets:
  - name: registry-secret
```

**创建 Pod**:

```bash
# 应用 YAML 文件
kubectl apply -f pod-psi-task.yaml -n alice

# 输出
# pod/psi-compute-pod created

# 查看 Pod 状态
kubectl get pods -n alice -o wide
# NAME                READY   STATUS    RESTARTS   AGE   IP           NODE
# psi-compute-pod     1/1     Running   0          2m    10.42.0.15   node-01

# 查看 Pod 详细信息
kubectl describe pod psi-compute-pod -n alice

# 查看 Pod 日志
kubectl logs psi-compute-pod -n alice

# 进入 Pod 执行命令
kubectl exec -it psi-compute-pod -n alice -- bash
```

##### D. 方式三:通过 KusciaJob/KusciaTask 自动创建 Pod

在 Kuscia 中,**最常用的方式是提交 KusciaJob**,系统会自动创建相关的 Pod。

**⚠️ 重要概念区分:appImage vs Docker 镜像**

在上面的 YAML 示例中,你可能注意到了 `appImage: secretflow/psi:latest` 这个字段。这**不是** Docker 镜像,而是一个 **Kuscia 自定义资源(CRD)**!

---

#### 5.1.5 AppImage CRD vs Docker 镜像详解

这是 Kuscia 中最容易混淆的概念之一。让我们彻底搞清楚它们的区别:

| 维度 | **AppImage (CRD)** | **Docker/OCI 镜像** |
| ------ | ------------------- | ------------------- |
| **本质** | Kubernetes 自定义资源对象 | 容器文件系统打包 |
| **存储位置** | etcd (K3s 数据库) | 镜像仓库(Harbor/Docker Hub) |
| **作用** | **描述**如何运行应用的模板 | **包含**可执行代码和依赖 |
| **查看命令** | `kubectl get appimages` | `docker images` / `crictl images` |
| **格式** | YAML 配置文件 | 分层文件系统(tar.gz) |
| **管理方式** | kubectl apply/delete | docker pull/push |
| **是否可执行** | ❌ 不可执行,只是配置 | ✅ 可以直接运行 |
| **类比** | 📋 菜谱(说明怎么做菜) | 🍱 预制菜包(直接加热吃) |

---

##### A. AppImage CRD 详解

**AppImage** 是 Kuscia 定义的**自定义资源(CRD)**,用于**描述应用的部署配置**。

**完整 AppImage YAML 示例**:

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: AppImage
metadata:
  name: psi-image          # AppImage 的名称(不是镜像地址!)
spec:
  # ========== 1. 关联的 Docker 镜像 ==========
  image:
    name: secretflow/psi   # Docker 镜像名称
    tag: latest            # Docker 镜像标签
    id: sha256:f1c20d8...  # 镜像 ID(可选,用于验证)
    sign: ""               # 签名(可选,用于安全验证)
  
  # ========== 2. 配置模板 ==========
  configTemplates:
    task-config.conf: |
      {
        "task_id": "{{.TASK_ID}}",
        "task_input_config": "{{.TASK_INPUT_CONFIG}}",
        "task_cluster_def": "{{.TASK_CLUSTER_DEFINE}}",
        "allocated_ports": "{{.ALLOCATED_PORTS}}"
      }
  
  # ========== 3. 部署模板(可以有多个) ==========
  deployTemplates:
    - name: psi            # 部署模板名称
      replicas: 1          # 副本数
      role: Client         # 角色(可选)
      spec:
        restartPolicy: Never
        containers:
          - name: secretflow
            command:       # 启动命令
              - sh
            args:          # 参数
              - -c
              - /root/main --kuscia ./kuscia/task-config.conf
            workingDir: /work
            ports:
              - name: psi
                port: 54509
                protocol: HTTP
                scope: Cluster
            configVolumeMounts:  # 挂载配置模板
              - mountPath: /work/kuscia/task-config.conf
                subPath: task-config.conf
```

**AppImage 的核心组成**:

```
AppImage CRD
├─ image (关联的 Docker 镜像信息)
│  ├─ name: 镜像名称
│  ├─ tag: 镜像标签
│  ├─ id: 镜像 ID(可选)
│  └─ sign: 签名(可选)
│
├─ configTemplates (配置模板)
│  ├─ template-1.yaml: |
│  │   key: {{.VARIABLE}}
│  └─ template-2.conf: |
│     setting: value
│
└─ deployTemplates (部署模板,可以有多个)
   ├─ template-1 (Client 角色)
   │  ├─ name: client-template
   │  ├─ replicas: 1
   │  ├─ role: Client
   │  └─ spec: PodSpec (容器配置)
   │     ├─ containers
   │     ├─ volumes
   │     └─ ...
   │
   └─ template-2 (Server 角色)
      ├─ name: server-template
      ├─ replicas: 1
      ├─ role: Server
      └─ spec: PodSpec
```

**AppImage 的作用**:

1. **标准化应用配置**:统一定义如何运行某个应用
2. **配置模板化**:支持变量替换(`{{.TASK_ID}}` 等)
3. **多角色支持**:一个 AppImage 可以定义多个部署模板(Client/Server)
4. **解耦应用与任务**:任务只需引用 AppImage 名称,无需重复配置

---

##### B. Docker 镜像详解

**Docker 镜像**是**容器化的应用程序包**,包含:

- 可执行文件(二进制/脚本)
- 依赖库(Python packages、系统库)
- 配置文件
- 环境变量
- 文件系统层次结构

**Docker 镜像的特点**:

```bash
# 1. 查看本地镜像
docker images
# REPOSITORY           TAG       IMAGE ID       CREATED        SIZE
# secretflow/psi       latest    f1c20d8cb5c4   2 weeks ago    2.5GB

# 2. 拉取镜像
docker pull secretflow/psi:latest

# 3. 运行镜像
docker run -it secretflow/psi:latest bash

# 4. 查看镜像内容
docker run --rm secretflow/psi:latest ls -la /root/
# total 12345
# -rwxr-xr-x 1 root root 67890 Jan  1 12:00 main
# drwxr-xr-x 2 root root  4096 Jan  1 12:00 kuscia
```

**Docker 镜像的分层结构**:

```
secretflow/psi:latest
├─ Layer 1: Base OS (Ubuntu 20.04)
├─ Layer 2: Python 3.9
├─ Layer 3: pip install secretflow
├─ Layer 4: Application code (/root/main)
└─ Layer 5: Config files
```

**Docker 镜像的详细目录结构** (以 `secretflow/psi:latest` 为例):

###### **Layer 1: Base OS (Ubuntu 20.04)** - 基础系统层

```bash
/
├── bin/          # 基础命令 (ls, cat, cp, bash...)
├── boot/         # 启动文件(通常为空)
├── dev/          # 设备文件
├── etc/          # 系统配置
│   ├── apt/      # APT 包管理器配置
│   ├── dpkg/     # Debian 包数据库
│   └── passwd    # 用户信息
├── home/         # 用户主目录
├── lib/          # 共享库文件
├── lib64/        # 64位共享库
├── opt/          # 可选应用包
├── proc/         # 进程信息(挂载点)
├── root/         # root 用户主目录
├── run/          # 运行时数据
├── sbin/         # 系统管理命令
├── sys/          # 系统信息(挂载点)
├── tmp/          # 临时文件
├── usr/          # 用户程序
│   ├── bin/      # 用户命令
│   ├── lib/      # 用户库
│   └── share/    # 共享数据
└── var/          # 可变数据
    ├── cache/    # 缓存
    └── log/      # 日志
```

**大小**: ~70-100 MB  
**作用**: 提供 Linux 基础运行环境

---

###### **Layer 2: Python 3.9 运行时** - 语言环境层

```bash
/usr/local/
├── bin/
│   ├── python3        # Python 3.9 解释器
│   ├── python3.9
│   ├── pip3           # Python 包管理器
│   └── pip3.9
├── lib/
│   └── python3.9/     # Python 标准库
│       ├── collections/    # 集合类型
│       ├── datetime/       # 日期时间
│       ├── json/           # JSON 处理
│       ├── os/             # 操作系统接口
│       └── ... (约 200+ 标准模块)
└── include/
    └── python3.9/     # C 头文件(用于编译扩展)
```

**大小**: ~100-150 MB  
**作用**: 提供 Python 运行环境和标准库

---

###### **Layer 3: SecretFlow 框架及依赖** - 核心框架层 (最大层)

这是**最关键的一层**,包含 SecretFlow 隐私计算框架及其所有依赖:

```bash
/usr/local/lib/python3.9/site-packages/
│
├── secretflow/                    # SecretFlow 核心包 (~50 MB)
│   ├── __init__.py
│   ├── version.py                 # 版本信息: 1.11.0b1
│   │
│   ├── psi/                       # PSI 隐私集合求交模块
│   │   ├── __init__.py
│   │   ├── protocol.py            # PSI 协议实现 (ECDH/RSA)
│   │   ├── curve25519.py          # 椭圆曲线加密
│   │   ├── ecdh.py                # ECDH 密钥交换
│   │   └── _psi.cpython-39-x86_64-linux-gnu.so  # C++ 扩展
│   │
│   ├── fl/                        # 联邦学习模块
│   │   ├── horizontal/            # 横向联邦学习
│   │   └── vertical/              # 纵向联邦学习
│   │
│   ├── mpc/                       # 多方安全计算
│   │   ├── share.py               # 秘密分享
│   │   └── protocol.py            # MPC 协议
│   │
│   ├── device/                    # 设备抽象层
│   │   └── spu/                   # SPU (Secure Processing Unit)
│   │       ├── __init__.py
│   │       ├── libspu.so          # SPU 原生库 (~20 MB)
│   │       └── spu_pb2.py         # Protocol Buffers 定义
│   │
│   └── utils/                     # 工具函数
│       ├── network.py             # gRPC 通信
│       └── crypto.py              # 加密工具
│
├── yacl/                          # 密码学基础库 (~30 MB)
│   ├── __init__.py
│   ├── libyacl.so                 # YACL 原生库
│   └── yacl_pb2.py
│
├── ray/                           # 分布式计算框架 (~100 MB)
│   ├── __init__.py
│   ├── worker.py
│   └── ... (Ray 完整框架)
│
├── numpy/                         # 数值计算库 (~50 MB)
├── pandas/                        # 数据处理库 (~80 MB)
├── grpc/                          # gRPC 通信库 (~20 MB)
├── tensorflow/ 或 torch/          # 深度学习框架 (可选,~500 MB)
│
└── ... (总计约 200+ Python 包)
```

**大小**: ~500 MB - 1.5 GB (取决于是否包含深度学习框架)  
**作用**: 提供隐私计算的核心能力

**关键组件说明**:

- **`secretflow.psi`**: PSI 协议实现,调用底层 C++ 扩展进行高性能计算
- **`libspu.so`**: SPU 原生库,提供安全多方计算能力
- **`libyacl.so`**: YACL 密码学库,提供椭圆曲线、哈希等原语
- **`ray`**: 分布式计算框架,支持多节点并行计算

---

###### **Layer 4: 应用代码** - 业务逻辑层

这是**Kuscia 实际调用的入口**,包含隐私计算任务的具体实现:

```bash
/root/
├── main                           # ⭐ 主入口脚本 (可执行文件)
├── psi_compute.py                 # PSI 计算逻辑 (Python 脚本)
├── config/                        # 配置目录
│   ├── task_config.json           # 任务配置
│   └── party_config.yaml          # 参与方配置
├── data/                          # 输入数据目录
│   ├── alice.csv                  # Alice 的数据
│   └── bob.csv                    # Bob 的数据
├── output/                        # 输出结果目录
│   └── result.csv                 # PSI 交集结果
└── logs/                          # 日志目录
    └── psi.log                    # 计算日志
```

**`/root/main` 示例** (Shell 脚本入口):

```bash
#!/bin/bash
# /root/main - Kuscia 调用的第一层入口

set -euo pipefail

# 1. 解析参数
TASK_ID="${TASK_ID:-default-task}"
PARTY_ID="${PARTY_ID:-alice}"
PEER_ENDPOINT="${PEER_ENDPOINT:-http://bob:8080}"

# 2. 调用 Python 计算脚本
python3 /root/psi_compute.py \
  --task_id "$TASK_ID" \
  --party_id "$PARTY_ID" \
  --peer_endpoint "$PEER_ENDPOINT" \
  --input_data "/root/data/${PARTY_ID}.csv" \
  --output_result "/root/output/result.csv"

# 3. 检查结果
if [ -f "/root/output/result.csv" ]; then
  echo "✅ PSI computation completed!"
else
  echo "❌ PSI computation failed!"
  exit 1
fi
```

**`/root/psi_compute.py` 示例** (Python 应用代码):

```python
#!/usr/bin/env python3
"""PSI 隐私集合求交计算"""

import argparse
import pandas as pd
import secretflow.psi as psi  # ← 导入 SecretFlow 框架

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--task_id', required=True)
    parser.add_argument('--party_id', required=True)
    parser.add_argument('--peer_endpoint', required=True)
    parser.add_argument('--input_data', required=True)
    parser.add_argument('--output_result', required=True)
    
    args = parser.parse_args()
    
    # 1. 加载数据
    df = pd.read_csv(args.input_data)
    local_ids = df['id'].astype(str).tolist()
    
    # 2. 初始化 PSI 协议
    protocol = psi.EcdhPsiProtocol(curve="Curve25519")
    
    # 3. 执行隐私集合求交
    intersection = protocol.compute(
        local_set=local_ids,
        peer_url=args.peer_endpoint,
        party_id=args.party_id
    )
    
    # 4. 保存结果
    result_df = df[df['id'].astype(str).isin(intersection)]
    result_df.to_csv(args.output_result, index=False)
    
    print(f"PSI completed. Intersection size: {len(intersection)}")

if __name__ == '__main__':
    main()
```

**大小**: ~1-10 MB  
**作用**: 实现具体的隐私计算业务逻辑

---

###### **Layer 5: 配置文件** - 系统配置层

```bash
/etc/secretflow/
├── config.yaml              # 全局配置
├── logging.conf             # 日志配置
└── network.conf             # 网络配置

/home/kuscia/var/certs/      # TLS 证书目录
├── ca.crt                   # CA 根证书
├── ca.key                   # CA 私钥
└── domain.crt               # 域证书
```

**`config.yaml` 示例**:

```yaml
# SecretFlow PSI 配置
psi:
  protocol: "ECDH"
  curve: "Curve25519"
  hash_method: "SHA256"
  
network:
  timeout: 300
  retry_count: 3
  tls_enabled: true
  
logging:
  level: "INFO"
  format: "%(asctime)s [%(levelname)s] %(message)s"
  file: "/root/logs/psi.log"
```

**大小**: ~1 KB  
**作用**: 提供运行时配置和证书

---

###### **完整的镜像层次结构图**

```
secretflow/psi:latest (总大小: ~800 MB - 1.5 GB)
│
├─ Layer 1: ubuntu:20.04 (70 MB)
│  └─ [/] 基础文件系统
│
├─ Layer 2: Python 3.9 (100 MB)
│  └─ [/usr/local/bin/python3, /usr/local/lib/python3.9/]
│
├─ Layer 3: SecretFlow 依赖 (600 MB - 1.2 GB) ← 最大层
│  ├─ [/usr/local/lib/python3.9/site-packages/secretflow/]
│  ├─ [/usr/local/lib/python3.9/site-packages/yacl/]
│  ├─ [/usr/local/lib/python3.9/site-packages/ray/]
│  └─ [...200+ Python 包]
│
├─ Layer 4: 应用代码 (5 MB) ← Kuscia 调用层
│  ├─ [/root/main]              ← ⭐ 入口脚本
│  ├─ [/root/psi_compute.py]    ← ⭐ 业务逻辑
│  ├─ [/root/config/]           ← 任务配置
│  ├─ [/root/data/]             ← 输入数据
│  └─ [/root/output/]           ← 输出结果
│
└─ Layer 5: 配置文件 (1 KB)
   ├─ [/etc/secretflow/config.yaml]
   └─ [/home/kuscia/var/certs/]
```

---

###### **Kuscia 调用 Docker 镜像的完整流程**

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Kuscia 调度层                                              │
│    - TaskResource Controller 创建 Pod Spec                   │
│    - 指定容器启动命令: /root/main --task_id xxx              │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Container Runtime (containerd)                           │
│    - 拉取镜像: crictl pull secretflow/psi:latest            │
│    - 解压镜像层到联合文件系统                                 │
│    - 启动容器进程                                             │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Layer 4: /root/main (Shell 脚本)                         │
│    #!/bin/bash                                               │
│    # 解析环境变量和参数                                       │
│    # 调用 Python 脚本                                         │
│    python3 /root/psi_compute.py --task_id "$TASK_ID" ...    │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Layer 4: /root/psi_compute.py (Python 应用代码)          │
│    import secretflow.psi as psi  # ← 导入 SecretFlow         │
│    protocol = psi.EcdhPsiProtocol()                          │
│    intersection = protocol.compute(...)                      │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Layer 3: site-packages/secretflow/psi/ (框架层)          │
│    - protocol.py: EcdhPsiProtocol 类定义                     │
│    - curve25519.py: 椭圆曲线加密实现                          │
│    - _psi.so: C++ 扩展模块 (性能优化)                         │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ 6. Layer 3: site-packages/yacl/ (密码学层)                  │
│    - libyacl.so: YACL 原生库                                 │
│    - 提供 ECDH、SHA256 等密码学原语                           │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ 7. CPU 硬件层                                                │
│    - AES-NI: AES 硬件加速                                    │
│    - AVX2: 向量运算加速                                      │
└─────────────────────────────────────────────────────────────┘
```

---

###### **如何查看镜像内容**

**方法 1: 使用 docker inspect**

```bash
# 查看镜像元数据
docker inspect secretflow/psi:latest

# 查看分层信息
docker inspect --format='{{json .RootFS.Layers}}' secretflow/psi:latest
```

**方法 2: 进入运行中的容器**

```bash
# 启动交互式容器
docker run -it --rm secretflow/psi:latest bash

# 在容器内查看文件结构
ls -la /root/
# total 24
# drwx------ 1 root root 4096 Jul  2 10:00 .
# -rwxr-xr-x 1 root root  512 Jul  2 10:00 main
# -rw-r--r-- 1 root root 2048 Jul  2 10:00 psi_compute.py
# drwxr-xr-x 2 root root 4096 Jul  2 10:00 config
# drwxr-xr-x 2 root root 4096 Jul  2 10:00 data

# 查看 Python 包
pip list | grep secretflow
# secretflow    1.11.0b1

# 查看包位置
python3 -c "import secretflow; print(secretflow.__file__)"
# /usr/local/lib/python3.9/site-packages/secretflow/__init__.py
```

**方法 3: 导出并解压镜像**

```bash
# 保存镜像为 tar 文件
docker save secretflow/psi:latest -o psi-image.tar

# 查看 tar 内容
tar -tf psi-image.tar | head -50
# manifest.json
# layer.tar
# VERSION
# ...

# 解压某一层查看具体内容
mkdir -p /tmp/psi-layer
tar -xf layer.tar -C /tmp/psi-layer/
ls -la /tmp/psi-layer/
```

**方法 4: 使用 dive 工具 (推荐)** ⭐

```bash
# 安装 dive
sudo apt-get install dive

# 交互式分析镜像
dive secretflow/psi:latest

# 显示每层的文件和大小,支持逐层浏览
```

---

###### **总结对比表**

| 层级 | 内容 | 大小 | 可变性 | 作用 |
| ------ | ------ | ------ | -------- | ------ |
| Layer 1 | Ubuntu 基础系统 | 70 MB | 固定 | 提供 Linux 环境 |
| Layer 2 | Python 运行时 | 100 MB | 固定 | 提供 Python 环境 |
| Layer 3 | SecretFlow 框架 | 600 MB - 1.2 GB | **最大层** | 核心计算能力 |
| Layer 4 | 应用代码 | 5 MB | **经常变化** | **Kuscia 调用入口** |
| Layer 5 | 配置文件 | 1 KB | 经常变化 | 运行时配置 |

**关键理解**:

- ✅ Layer 1-3 可以缓存复用(变化少,构建慢)
- ⚠️ Layer 4-5 应该独立构建(变化频繁,构建快)
- 🎯 `/root/main` 是 **Kuscia 调用的第一层入口**
- 🎯 `/root/psi_compute.py` 是**应用业务逻辑**
- 🎯 `site-packages/secretflow/` 是**框架核心能力**

**优化建议**:

- 使用多阶段构建减小最终镜像大小
- 将频繁变化的应用代码放在上层
- 利用 Docker 层缓存加速构建

---

##### C. 两者的关系

**AppImage 引用 Docker 镜像**:

```yaml
# AppImage CRD (存储在 etcd)
apiVersion: kuscia.secretflow/v1alpha1
kind: AppImage
metadata:
  name: psi-image        # ← 这是 AppImage 的名称
spec:
  image:
    name: secretflow/psi # ← 这是 Docker 镜像的名称
    tag: latest          # ← 这是 Docker 镜像的标签
```

**工作流程**:

```
1. 用户创建 AppImage CRD
   kubectl apply -f psi-appimage.yaml
   ↓
   AppImage 存入 etcd

2. 用户提交 KusciaJob,引用 AppImage
   spec:
     tasks:
     - parties:
       - domainID: alice
         appImage: psi-image  # ← 引用 AppImage 名称
   ↓
   KusciaJob 存入 etcd

3. TaskResource Controller 读取 AppImage
   ↓
   从 AppImage.spec.image 获取 Docker 镜像信息
   name: secretflow/psi
   tag: latest

4. Agent 拉取 Docker 镜像
   crictl pull secretflow/psi:latest
   ↓
   Docker 镜像下载到本地

5. Agent 基于 Docker 镜像创建容器
   ↓
   容器运行
```

---

##### D. 实际操作对比

**管理 AppImage (CRD)**:

```bash
# 1. 查看所有 AppImages
kubectl get appimages
# NAME        AGE
# psi-image   10d
# fl-image    10d

# 2. 查看 AppImage 详情
kubectl get appimage psi-image -o yaml
# 输出完整的 YAML 配置

# 3. 创建新的 AppImage
kubectl apply -f my-appimage.yaml

# 4. 删除 AppImage
kubectl delete appimage psi-image
```

**管理 Docker 镜像**:

```bash
# 1. 查看本地 Docker 镜像
crictl images
# IMAGE                    TAG       IMAGE ID            SIZE
# secretflow/psi           latest    f1c20d8cb5c4        2.5GB

# 2. 拉取 Docker 镜像
crictl pull secretflow/psi:latest

# 3. 删除 Docker 镜像
crictl rmi secretflow/psi:latest

# 4. 推送镜像到仓库(需要先登录)
docker login harbor.example.com
docker push harbor.example.com/secretflow/psi:latest
```

---

##### E. 为什么需要 AppImage?

**问题 1: 为什么不直接用 Docker 镜像?**

如果只用 Docker 镜像,每次提交任务都要写完整的 Pod Spec:

```yaml
# ❌ 没有 AppImage - 每次都重复配置
tasks:
- parties:
  - domainID: alice
    template:
      spec:
        containers:
        - name: secretflow
          image: secretflow/psi:latest
          command: [sh, -c, /root/main --kuscia ...]
          workingDir: /work
          ports:
          - name: psi
            port: 54509
          volumeMounts:
          - name: config
            mountPath: /work/kuscia/task-config.conf
        volumes:
        - name: config
          configMap:
            name: task-config
```

**有了 AppImage 后**:

```yaml
# ✅ 使用 AppImage - 简洁清晰
tasks:
- parties:
  - domainID: alice
    appImage: psi-image  # ← 一行搞定!
```

**AppImage 的优势**:

1. **配置复用**:一次定义,多次使用

   ```yaml
   # 定义一次 AppImage
   kubectl apply -f psi-appimage.yaml
   
   # 多个 Job 都可以引用
   Job1: appImage: psi-image
   Job2: appImage: psi-image
   Job3: appImage: psi-image
   ```

2. **集中管理**:修改一处,全局生效

   ```bash
   # 升级镜像版本
   kubectl edit appimage psi-image
   # 修改 spec.image.tag: v1.0 → v2.0
   
   # 所有新任务自动使用新版本
   ```

3. **配置模板化**:自动注入运行时变量

   ```yaml
   # AppImage 中定义模板
   configTemplates:
     task-config.conf: |
       {
         "task_id": "{{.TASK_ID}}",  # ← 自动替换
         "party_id": "{{.PARTY_ID}}" # ← 自动替换
       }
   
   # 任务运行时,Kuscia 自动替换变量
   ```

4. **多角色支持**:一个 AppImage 适配不同角色

   ```yaml
   deployTemplates:
   - name: client-template
     role: Client
     spec:
       containers:
       - args: ["--role=client"]
   
   - name: server-template
     role: Server
     spec:
       containers:
       - args: ["--role=server"]
   ```

5. **安全验证**:镜像 ID 和签名校验

   ```yaml
   image:
     name: secretflow/psi
     tag: latest
     id: sha256:f1c20d8...  # 验证镜像完整性
     sign: "signed-by-ca"    # 验证镜像来源
   ```

---

##### F. 完整示例:从 AppImage 到任务运行

**Step 1: 创建 AppImage**

```yaml
# psi-appimage.yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: AppImage
metadata:
  name: psi-image
spec:
  image:
    name: secretflow/psi
    tag: latest
  configTemplates:
    task-config.conf: |
      {
        "task_id": "{{.TASK_ID}}",
        "party_id": "{{.PARTY_ID}}"
      }
  deployTemplates:
    - name: psi
      replicas: 1
      spec:
        containers:
          - name: secretflow
            command: [sh, -c, /root/main --kuscia ./kuscia/task-config.conf]
            workingDir: /work
```

```bash
kubectl apply -f psi-appimage.yaml
# appimage.kuscia.secretflow/psi-image created
```

**Step 2: 提交任务引用 AppImage**

```yaml
# job.yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-job-001
spec:
  initiator: alice
  tasks:
  - taskID: psi-task-001
    parties:
    - domainID: alice
      appImage: psi-image  # ← 引用 AppImage 名称
    - domainID: bob
      appImage: psi-image  # ← 引用同一个 AppImage
```

```bash
kubectl apply -f job.yaml
# kusciajob.kuscia.secretflow/psi-job-001 created
```

**Step 3: 系统自动处理**

```
1. TaskResource Controller 读取 AppImage "psi-image"
   ↓
2. 提取 Docker 镜像信息: secretflow/psi:latest
   ↓
3. 提取部署配置: command, workingDir, configTemplates
   ↓
4. 渲染配置模板,替换变量
   ↓
5. 构建 Pod Spec
   ↓
6. Agent 拉取 Docker 镜像: crictl pull secretflow/psi:latest
   ↓
7. Agent 创建并启动容器
   ↓
8. 容器内执行: /root/main --kuscia ./kuscia/task-config.conf
```

**Step 4: 查看结果**

```bash
# 查看 AppImage
kubectl get appimage psi-image

# 查看正在运行的 Pod
kubectl get pods -n alice

# 查看 Pod 使用的镜像
kubectl get pod psi-task-001-alice-pod-0 -n alice -o jsonpath='{.spec.containers[0].image}'
# secretflow/psi:latest

# 查看渲染后的配置
kubectl exec -it psi-task-001-alice-pod-0 -n alice -- cat /work/kuscia/task-config.conf
# {
#   "task_id": "psi-task-001",
#   "party_id": "alice"
# }
```

---

##### G. 总结对比表

| 特性 | AppImage (CRD) | Docker 镜像 |
| ------ | --------------- | ------------ |
| **是什么** | Kubernetes 配置对象 | 容器文件系统包 |
| **存储在哪** | etcd (K3s 数据库) | 本地磁盘/镜像仓库 |
| **如何查看** | `kubectl get appimages` | `docker images` |
| **如何创建** | `kubectl apply -f xxx.yaml` | `docker build` / `docker pull` |
| **能否执行** | ❌ 不能,只是配置 | ✅ 可以直接运行 |
| **大小** | 几 KB (文本) | 几百 MB ~ 几 GB |
| **作用** | 描述**如何运行**应用 | **包含**应用代码和依赖 |
| **生命周期** | 手动创建/删除 | 自动拉取/GC |
| **版本管理** | Git/YAML | 镜像 Tag |
| **类比** | 📋 菜谱 | 🍱 预制菜 |

**关键理解**:

- **AppImage** = 说明书(告诉系统如何运行应用)
- **Docker 镜像** = 工具箱(包含实际的可执行代码)
- **AppImage 引用 Docker 镜像** + **部署配置** = 完整的应用运行方案

所以,当你看到 `appImage: psi-image` 时:

1. 这是一个 **AppImage CRD 的名称**(不是 Docker 镜像地址)
2. AppImage 内部会指定要使用的 **Docker 镜像** (`secretflow/psi:latest`)
3. AppImage 还包含了**如何运行**这个镜像的配置(command、args、volumes 等)

这种设计让任务配置变得非常简洁,同时保持了灵活性和可复用性! 🎯

---

**KusciaJob 示例**:

```yaml
# kusciajob-psi.yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-job-001
  namespace: alice
spec:
  initiator: alice
  maxParallelism: 2
  tasks:
  - taskID: psi-task-001
    taskAlias: psi-compute
    priority: 10
    parties:
    - domainID: alice
      role: Client
      appImage: secretflow/psi:latest
    - domainID: bob
      role: Server
      appImage: secretflow/psi:latest
    template:
      spec:
        containers:
        - name: worker
          resources:
            requests:
              cpu: "2"
              memory: 4Gi
            limits:
              cpu: "4"
              memory: 8Gi
```

**提交流程**:

```bash
# 1. 提交 KusciaJob
kubectl apply -f kusciajob-psi.yaml -n alice

# 2. 系统自动创建 KusciaTask
kubectl get kusciatasks -n alice
# NAME             STATUS      AGE
# psi-task-001     Pending     5s

# 3. 系统自动创建 TaskResourceGroup
kubectl get taskresourcegroups -n alice
# NAME                     STATUS      AGE
# psi-task-001-trg         Creating    6s

# 4. 系统自动创建 TaskResource
kubectl get taskresources -n alice
# NAME                     STATUS      AGE
# psi-task-001-alice-tr    Reserving   7s

# 5. 系统自动创建 Pod
kubectl get pods -n alice
# NAME                              READY   STATUS              AGE
# psi-task-001-alice-pod-0          0/1     ContainerCreating   8s
# psi-task-001-bob-pod-0            0/1     ContainerCreating   8s

# 6. Pod 启动运行
kubectl get pods -n alice
# NAME                              READY   STATUS    AGE
# psi-task-001-alice-pod-0          1/1     Running   30s
# psi-task-001-bob-pod-0            1/1     Running   30s
```

**Pod 创建的内部流程**:

```
KusciaJob 提交
    ↓
KusciaJob Controller
    ├─ 验证 Job 配置
    ├─ 权限检查
    └─ 创建 KusciaTask
    ↓
KusciaTask Controller
    ├─ 解析 Task 配置
    ├─ 资源预留(TRG)
    └─ 创建 TaskResource
    ↓
TaskResource Controller
    ├─ 构建 Pod Spec
    ├─ 注入环境变量
    ├─ 配置资源限制
    └─ 调用 K8s API 创建 Pod
    ↓
K3s API Server
    ├─ 验证 Pod Spec
    ├─ 保存到 etcd
    └─ 触发调度
    ↓
Kuscia Scheduler
    ├─ 过滤可用 Node
    ├─ 评分排序
    └─ 绑定 Pod 到 Node
    ↓
Agent (PodsController)
    ├─ Watch 到新 Pod
    ├─ 拉取镜像
    ├─ 创建容器
    ├─ 启动容器
    └─ 更新状态
```

##### E. Pod 核心参数详解

**1. 容器配置 (containers)**:

```yaml
spec:
  containers:
  - name: main-container          # 容器名称
    image: secretflow/ray:latest  # 镜像地址
    command: ["python"]           # 启动命令(覆盖 Docker ENTRYPOINT)
    args: ["app.py"]              # 参数(覆盖 Docker CMD)
    workingDir: /app              # 工作目录
    
    # 环境变量
    env:
    - name: CONFIG_PATH
      value: /etc/config.yaml
    - name: SECRET_KEY
      valueFrom:
        secretKeyRef:
          name: my-secret
          key: secret-key
    
    # 资源管理
    resources:
      requests:                   # 请求的资源(调度依据)
        cpu: "2"
        memory: 4Gi
        ephemeral-storage: 10Gi
      limits:                     # 最大资源(硬性限制)
        cpu: "4"
        memory: 8Gi
        ephemeral-storage: 20Gi
```

**2. 健康检查 (probes)**:

```yaml
spec:
  containers:
  - name: worker
    # 存活探针 - 检测容器是否正常运行
    livenessProbe:
      httpGet:
        path: /healthz
        port: 8080
        httpHeaders:
        - name: X-Custom-Header
          value: health-check
      initialDelaySeconds: 15    # 容器启动后等待时间
      periodSeconds: 20          # 探测间隔
      timeoutSeconds: 3          # 超时时间
      successThreshold: 1        # 成功阈值
      failureThreshold: 3        # 失败阈值(超过则重启)
    
    # 就绪探针 - 检测容器是否准备好接收流量
    readinessProbe:
      tcpSocket:
        port: 6379
      initialDelaySeconds: 5
      periodSeconds: 10
      timeoutSeconds: 3
      failureThreshold: 5
    
    # 启动探针 - 用于慢启动容器
    startupProbe:
      httpGet:
        path: /startup
        port: 8080
      initialDelaySeconds: 0
      periodSeconds: 10
      failureThreshold: 30       # 允许最多 300 秒启动时间
```

**3. 数据卷 (volumes)**:

```yaml
spec:
  # 数据卷定义
  volumes:
  # ConfigMap 卷
  - name: config-vol
    configMap:
      name: my-config
      items:
      - key: config.yaml
        path: config.yaml
  
  # Secret 卷
  - name: secret-vol
    secret:
      secretName: my-secret
      defaultMode: 0400          # 文件权限
  
  # PersistentVolumeClaim 卷
  - name: data-vol
    persistentVolumeClaim:
      claimName: my-pvc
  
  # EmptyDir 卷(临时存储)
  - name: tmp-vol
    emptyDir:
      medium: Memory             # 使用 tmpfs
      sizeLimit: 512Mi
  
  # HostPath 卷(宿主机路径)
  - name: host-vol
    hostPath:
      path: /var/log/kuscia
      type: DirectoryOrCreate
  
  containers:
  - name: worker
    volumeMounts:
    - name: config-vol
      mountPath: /etc/config
      readOnly: true
    - name: secret-vol
      mountPath: /etc/secrets
      readOnly: true
    - name: data-vol
      mountPath: /data
    - name: tmp-vol
      mountPath: /tmp
```

**4. 调度和亲和性**:

```yaml
spec:
  # 节点选择器
  nodeSelector:
    disktype: ssd
    gpu: "true"
  
  # 容忍度(允许调度到有污点的节点)
  tolerations:
  - key: "gpu-only"
    operator: "Equal"
    value: "true"
    effect: "NoSchedule"
  
  # 节点亲和性
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: kubernetes.io/arch
            operator: In
            values:
            - amd64
    
    # Pod 亲和性(尽量和特定 Pod 在同一节点)
    podAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app
            operator: In
            values:
            - redis
        topologyKey: kubernetes.io/hostname
    
    # Pod 反亲和性(避免和特定 Pod 在同一节点)
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
            - key: app
              operator: In
              values:
              - worker
          topologyKey: kubernetes.io/hostname
```

**5. 网络和 DNS**:

```yaml
spec:
  # DNS 策略
  dnsPolicy: ClusterFirst        # ClusterFirst / Default / None
  dnsConfig:
    nameservers:
    - 8.8.8.8
    searches:
    - mydomain.local
    options:
    - name: ndots
      value: "2"
  
  # 主机网络(直接使用宿主机网络栈)
  hostNetwork: false
  
  # 主机端口映射
  containers:
  - name: web
    ports:
    - containerPort: 80
      hostPort: 8080             # 映射到宿主机端口
      protocol: TCP
```

**6. 安全配置**:

```yaml
spec:
  # Pod 级别的安全上下文
  securityContext:
    runAsUser: 1000
    runAsGroup: 3000
    fsGroup: 2000
    supplementalGroups:
    - 4000
    - 5000
  
  containers:
  - name: worker
    # 容器级别的安全上下文
    securityContext:
      runAsUser: 1000
      runAsNonRoot: true
      readOnlyRootFilesystem: true
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - ALL
        add:
        - NET_BIND_SERVICE
```

##### F. 常用 kubectl 命令速查

**创建和管理 Pod**:

```bash
# 创建 Pod
kubectl apply -f pod.yaml -n <namespace>
kubectl create -f pod.yaml -n <namespace>

# 查看 Pod
kubectl get pods -n <namespace>
kubectl get pods -o wide -n <namespace>       # 显示更多信息
kubectl get pods -l app=psi -n <namespace>    # 按标签筛选

# 查看 Pod 详情
kubectl describe pod <pod-name> -n <namespace>

# 查看 Pod 日志
kubectl logs <pod-name> -n <namespace>
kubectl logs <pod-name> -f -n <namespace>     # 实时跟踪
kubectl logs <pod-name> --tail=100 -n <namespace>  # 最近100行

# 进入 Pod
kubectl exec -it <pod-name> -n <namespace> -- bash
kubectl exec -it <pod-name> -n <namespace> -- sh

# 复制文件
kubectl cp local.txt <namespace>/<pod-name>:/remote/path.txt
kubectl cp <namespace>/<pod-name>:/remote/path.txt local.txt

# 删除 Pod
kubectl delete pod <pod-name> -n <namespace>
kubectl delete -f pod.yaml -n <namespace>

# 编辑 Pod(不推荐,应修改 YAML 后重新 apply)
kubectl edit pod <pod-name> -n <namespace>
```

**调试和诊断**:

```bash
# 查看 Pod 事件
kubectl get events -n <namespace> --field-selector involvedObject.name=<pod-name>

# 查看 Pod 状态
kubectl get pod <pod-name> -o jsonpath='{.status.phase}' -n <namespace>

# 查看容器状态
kubectl get pod <pod-name> -o jsonpath='{.status.containerStatuses}' -n <namespace>

# 查看 Pod IP
kubectl get pod <pod-name> -o jsonpath='{.status.podIP}' -n <namespace>

# 查看 Pod 所在节点
kubectl get pod <pod-name> -o jsonpath='{.spec.nodeName}' -n <namespace>

# 查看资源使用情况(需要 metrics-server)
kubectl top pod <pod-name> -n <namespace>
```

##### G. Pod 创建最佳实践

**1. 始终设置资源限制**:

```yaml
# ✅ 好的做法
resources:
  requests:
    cpu: "2"
    memory: 4Gi
  limits:
    cpu: "4"
    memory: 8Gi

# ❌ 不好的做法 - 没有资源限制,可能导致资源争抢
```

**2. 配置健康检查**:

```yaml
# ✅ 好的做法 - 配置 liveness 和 readiness 探针
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  periodSeconds: 20

readinessProbe:
  tcpSocket:
    port: 6379
  periodSeconds: 10

# ❌ 不好的做法 - 没有健康检查,容器崩溃无法自动恢复
```

**3. 使用标签和注释**:

```yaml
# ✅ 好的做法
metadata:
  labels:
    app: psi
    task-type: compute
    version: v1.0
  annotations:
    description: "PSI 计算任务"
    owner: alice-team

# ❌ 不好的做法 - 缺少标签,难以管理和筛选
```

**4. 设置重启策略**:

```yaml
# 对于一次性任务
restartPolicy: Never

# 对于长期运行的服务
restartPolicy: Always

# 仅在失败时重启
restartPolicy: OnFailure
```

**5. 使用 ConfigMap 和 Secret 管理配置**:

```yaml
# ✅ 好的做法 - 配置外部化
env:
- name: CONFIG_PATH
  valueFrom:
    configMapKeyRef:
      name: task-config
      key: config-path
- name: API_KEY
  valueFrom:
    secretKeyRef:
      name: api-secret
      key: key

# ❌ 不好的做法 - 硬编码配置
env:
- name: API_KEY
  value: "my-secret-key-12345"
```

##### H. 常见问题排查

**问题 1: Pod 一直处于 Pending 状态**

```bash
# 查看 Pod 事件
kubectl describe pod <pod-name> -n <namespace>

# 常见原因:
# - 资源不足: No nodes are available
# - 节点选择器不匹配: node(s) didn't match node selector
# - 污点未容忍: node(s) had taints that the pod didn't tolerate

# 解决方案:
# - 检查节点资源: kubectl top nodes
# - 检查节点标签: kubectl get nodes --show-labels
# - 检查节点污点: kubectl describe node <node-name> | grep Taints
```

**问题 2: Pod 处于 ImagePullBackOff 状态**

```bash
# 查看详细信息
kubectl describe pod <pod-name> -n <namespace>

# 常见原因:
# - 镜像名称错误
# - 镜像不存在
# - 缺少拉取密钥

# 解决方案:
kubectl create secret docker-registry registry-secret \
  --docker-server=<registry-url> \
  --docker-username=<username> \
  --docker-password=<password> \
  -n <namespace>
```

**问题 3: Pod 处于 CrashLoopBackOff 状态**

```bash
# 查看日志
kubectl logs <pod-name> -n <namespace>
kubectl logs <pod-name> --previous -n <namespace>  # 查看上一次崩溃的日志

# 常见原因:
# - 应用程序错误
# - 配置错误
# - 资源不足(被 OOM Kill)

# 解决方案:
# - 检查日志定位错误
# - 增加内存限制
# - 修正配置
```

**问题 4: Pod 无法访问网络**

```bash
# 检查 DNS 配置
kubectl exec -it <pod-name> -n <namespace> -- nslookup kubernetes.default

# 检查网络连接
kubectl exec -it <pod-name> -n <namespace> -- ping 8.8.8.8

# 检查 Service
kubectl get svc -n <namespace>
kubectl describe svc <svc-name> -n <namespace>
```

---

#### 5.1.6 Agent 核心职责

Agent 是 Kuscia 中负责节点资源管理和容器生命周期管理的核心组件,运行在每个工作节点上。它的主要职责包括:

- **节点注册与管理**:向 Kubernetes API Server 注册节点,定期上报节点状态和资源使用情况
- **Pod 生命周期管理**:监听 Pod 事件,执行容器的创建、启动、停止和删除操作
- **镜像管理**:拉取、缓存和管理容器镜像,支持镜像垃圾回收
- **资源监控**:监控 CPU、内存、存储等资源使用情况,实施资源限制
- **状态同步**:将容器和 Pod 的状态同步回 API Server
- **日志管理**:收集和管理容器日志,支持日志轮转和清理

Agent 采用模块化设计,主要包含以下核心模块:

| 模块 | 代码路径 | 功能描述 |
| ------ | ---------- | ---------- |
| **Framework** | `pkg/agent/framework/` | PodsController 核心控制器,协调 Pod 同步流程 |
| **Provider** | `pkg/agent/provider/` | 提供不同运行时的实现(NodeProvider 和 PodProvider) |
| **KRI** | `pkg/agent/kri/` | Kubelet Runtime Interface,定义运行时接口规范 |
| **Container Runtime** | `pkg/agent/container/` | 容器运行时抽象层 |
| **KubeRuntime** | `pkg/agent/kuberuntime/` | RunC 运行时的具体实现,基于 CRI 接口 |
| **Local Runtime** | `pkg/agent/local/runtime/` | RunP 进程运行时的实现 |
| **Images** | `pkg/agent/images/` | 镜像管理器,处理镜像拉取和 GC |
| **Status Manager** | `pkg/agent/status/` | Pod 状态管理器,负责状态同步 |
| **Resource** | `pkg/agent/resource/` | 资源管理和容量计算 |
| **Middleware** | `pkg/agent/middleware/` | 中间件插件系统(带宽限制、证书颁发等) |

#### 5.1.7 Control Plane 与 Node 的分工

在 Kubernetes 架构中,**Control Plane(控制平面)** 和 **Node(工作节点)** 有明确的职责分工。Kuscia 在此基础上进行了定制化改造。

##### A. 标准 Kubernetes 的职责划分

**Control Plane (大脑)** - 负责决策和管理:

```
┌─────────────────────────────────────────────┐
│         Control Plane (控制平面)             │
├─────────────────────────────────────────────┤
│                                             │
│  1. API Server                              │
│     - 接收所有 API 请求                      │
│     - 认证、授权、准入控制                    │
│     - 提供 RESTful API                      │
│                                             │
│  2. etcd                                    │
│     - 分布式键值存储                         │
│     - 保存集群所有状态                       │
│     - CRD 对象存储                           │
│                                             │
│  3. Scheduler                               │
│     - 决定 Pod 运行在哪个 Node              │
│     - 考虑资源、亲和性、污点等               │
│                                             │
│  4. Controller Manager                      │
│     - 运行各种控制器                         │
│     - 维持期望状态与实际状态一致             │
│     - Namespace、ReplicaSet 等控制器        │
│                                             │
└─────────────────────────────────────────────┘
```

**Node (手脚)** - 负责执行:

```
┌─────────────────────────────────────────────┐
│              Node (工作节点)                  │
├─────────────────────────────────────────────┤
│                                             │
│  1. Kubelet                                 │
│     - 监听 API Server 的 Pod 分配           │
│     - 创建/启动/停止容器                     │
│     - 上报节点和 Pod 状态                    │
│                                             │
│  2. Container Runtime                       │
│     - 实际运行容器(containerd/CRI-O)        │
│     - 管理镜像                               │
│     - 提供 CRI 接口                          │
│                                             │
│  3. kube-proxy                              │
│     - 维护网络规则                           │
│     - 实现 Service 负载均衡                  │
│                                             │
└─────────────────────────────────────────────┘
```

**协作流程**:

```
用户提交任务
    ↓
API Server (接收请求)
    ↓
etcd (持久化存储)
    ↓
Scheduler (决定运行在哪个 Node)
    ↓
Controller Manager (创建相关资源)
    ↓
Kubelet (在 Node 上执行)
    ↓
Container Runtime (启动容器)
    ↓
状态回报到 API Server
```

##### B. Kuscia 的定制化分工

Kuscia 对标准 K8s 架构进行了**精简和定制**,以适应隐私计算场景:

**Kuscia Control Plane (嵌入式 K3s + 自定义控制器)**:

```
┌──────────────────────────────────────────────────┐
│         Kuscia Control Plane                      │
├──────────────────────────────────────────────────┤
│                                                  │
│  【嵌入式 K3s】                                   │
│  ├─ API Server (保留)                            │
│  │   - 接收 CRD 操作请求                         │
│  │   - 提供 HTTP/gRPC API                       │
│  │                                                │
│  ├─ etcd (保留,embedded)                         │
│  │   - 存储 DomainData/KusciaJob 等 CRD         │
│  │   - 使用 SQLite 或 embedded etcd             │
│  │                                                │
│  ├─ Controller Manager (保留部分)                │
│  │   - Namespace Controller ✅                  │
│  │   - ServiceAccount Controller ✅             │
│  │   - Secret Controller ✅                     │
│  │                                                │
│  └─ [已禁用]                                     │
│      - Scheduler ❌ (由 Kuscia Scheduler 替代)   │
│      - Kubelet ❌ (由 Agent 替代)                │
│      - Cloud Controller ❌ (不需要)              │
│                                                  │
│  【Kuscia 自定义控制器】                          │
│  ├─ DomainData Controller                        │
│  │   - 管理数据资产注册                          │
│  │   - 处理数据授权                              │
│  │                                                │
│  ├─ KusciaJob Controller                         │
│  │   - 解析 DAG 依赖                             │
│  │   - 按顺序创建 KusciaTask                     │
│  │   - 管理 Job 生命周期                         │
│  │                                                │
│  ├─ KusciaTask Controller                        │
│  │   - 为每个参与方创建 TaskResource             │
│  │   - 协调跨域资源预留                          │
│  │   - 触发调度                                  │
│  │                                                │
│  ├─ TaskResourceGroup Controller                 │
│  │   - 资源预留协调                              │
│  │   - 检查 MinReservedPods 阈值                │
│  │                                                │
│  ├─ DomainRoute Controller                       │
│  │   - 配置域间通信路由                          │
│  │   - 生成 Envoy 配置                           │
│  │                                                │
│  └─ InterConn Controller                         │
│      - 跨集群同步 CRD                            │
│      - P2P 模式下的状态同步                      │
│                                                  │
│  【Kuscia Scheduler】                             │
│  - 自定义调度器                                  │
│  - 支持资源预留(Resource Reservation)            │
│  - 支持跨域协同调度                              │
│  - 实现 Permit/Reserve/Bind 插件                 │
│                                                  │
└──────────────────────────────────────────────────┘
```

**Kuscia Node (Agent 替代 Kubelet)**:

```
┌──────────────────────────────────────────────────┐
│              Kuscia Node (Agent)                  │
├──────────────────────────────────────────────────┤
│                                                  │
│  【Agent 核心组件】                               │
│  ├─ NodeProvider                                 │
│  │   - 向 API Server 注册节点                    │
│  │   - 定期上报节点状态                          │
│  │   - 计算可用资源(Capacity Manager)           │
│  │                                                │
│  ├─ PodsController                               │
│  │   - Watch Pod 事件(Add/Update/Delete)        │
│  │   - 调用 PodProvider 执行操作                 │
│  │   - 同步 Pod 状态到 API Server               │
│  │                                                │
│  ├─ PodProvider (三种运行时)                     │
│  │   ├─ CRIProvider (RunC)                      │
│  │   │   - 通过 CRI 调用 containerd             │
│  │   │   - 完整的容器隔离                        │
│  │   │                                            │
│  │   ├─ ProcessProvider (RunP)                  │
│  │   │   - 直接 fork/exec 进程                   │
│  │   │   - cgroup 资源限制                       │
│  │   │                                            │
│  │   └─ K8sProvider (RunK)                      │
│  │       - 转发 Pod 到外部 K8s 集群              │
│  │       - 同步状态回主集群                      │
│  │                                                │
│  ├─ Image Manager                                │
│  │   - 拉取镜像                                  │
│  │   - 缓存管理                                  │
│  │   - 垃圾回收(GC)                              │
│  │                                                │
│  ├─ Status Manager                               │
│  │   - 收集容器状态                              │
│  │   - 批量更新到 API Server                     │
│  │   - 重试机制                                  │
│  │                                                │
│  └─ Middleware Plugins                           │
│      - 带宽限制(Bandwidth Filter)                │
│      - 证书颁发(Cert Issuance)                   │
│      - 配置渲染(Config Render)                   │
│      - 镜像安全(Image Security)                  │
│                                                  │
└──────────────────────────────────────────────────┘
```

##### C. 关键差异对比

| 维度 | 标准 K8s | Kuscia |
| ------ | --------- | -------- |
| **Control Plane** | 独立部署的多组件集群 | 嵌入式 K3s(单进程) |
| **Scheduler** | 通用调度器(kube-scheduler) | 自定义调度器(支持资源预留) |
| **Node Agent** | Kubelet | Agent(PodsController) |
| **CRD 控制器** | 需要手动部署 | 自动注册并启动 |
| **存储后端** | 独立 etcd 集群 | SQLite/embedded etcd |
| **网络插件** | CNI(Flannel/Calico) | 无(不使用 Pod 网络) |
| **DNS** | CoreDNS | 自定义 DNS 方案 |
| **服务发现** | Service + kube-proxy | DomainRoute + Envoy |
| **任务类型** | 通用容器化应用 | 隐私计算任务(PSI/FL/MPC) |
| **跨域支持** | 无 | InterConn Controller |

##### D. 完整执行流程示例

以创建一个 **PSI 任务**为例,展示 Control Plane 和 Node 的协作:

```yaml
# 用户提交的 KusciaJob
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-job-001
  namespace: cross-domain
spec:
  initiator: alice
  tasks:
    - alias: psi-compute
      appImage: secretflow/psi:latest
      parties:
        - domain_id: alice
          role: server
        - domain_id: bob
          role: client
```

**执行流程**:

```
┌─────────────────────────────────────────────────────────┐
│ Step 1: 用户通过 API 提交 Job                           │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 2: Control Plane - API Server 接收请求             │
│ - 认证: 验证用户身份                                     │
│ - 授权: 检查是否有权限创建 Job                           │
│ - 校验: 验证 YAML 格式                                   │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 3: Control Plane - etcd 持久化                     │
│ - 将 KusciaJob CR 写入 etcd                             │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 4: Control Plane - KusciaJob Controller            │
│ - Watch 到新 Job 事件                                    │
│ - 校验 DAG 依赖(无环)                                    │
│ - 生成 TaskID                                            │
│ - 创建 KusciaTask CR                                     │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 5: Control Plane - KusciaTask Controller           │
│ - 为 alice 创建 TaskResource(alice 命名空间)            │
│ - 为 bob 创建 TaskResource(bob 命名空间)                │
│ - 创建 TaskResourceGroup                                │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 6: Control Plane - InterConn Controller            │
│ - 将 alice 的 TaskResource 同步到 bob 集群(P2P模式)     │
│ - 将 bob 的 TaskResource 同步到 alice 集群              │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 7: Control Plane - TaskResourceGroup Controller    │
│ - 检查资源预留情况                                       │
│ - 满足 MinReservedPods 后标记为 Reserved                │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 8: Control Plane - Kuscia Scheduler                │
│ - PreFilter: 检查节点标签                                │
│ - Filter: 过滤不满足资源的节点                            │
│ - Score: 打分排序                                        │
│ - Reserve: 预留资源                                      │
│ - Bind: 绑定 Pod 到 Node                                 │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 9: Control Plane - etcd 更新                       │
│ - 更新 Pod.spec.nodeName = node-1                       │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 10: Node - Agent PodsController                    │
│ - Watch 到 Pod 绑定事件                                  │
│ - 调用 PodProvider.SyncPod()                            │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 11: Node - PodProvider (以 RunC 为例)              │
│ - 拉取镜像: secretflow/psi:latest                       │
│ - 创建 Sandbox(Pause 容器)                              │
│ - 创建业务容器(Ray Head + Workers)                      │
│ - 启动容器                                               │
└─────────────────────────────────────────────────────────┘
```

##### E. 分工总结

**Control Plane 的职责(决策层)**:

1. ✅ **接收请求**:API Server 提供统一入口
2. ✅ **持久化**:etcd 存储所有状态
3. ✅ **业务逻辑**:CRD Controllers 实现隐私计算特有逻辑
4. ✅ **调度决策**:Scheduler 决定 Pod 运行位置
5. ✅ **跨域协调**:InterConn Controller 同步跨集群状态

**Node 的职责(执行层)**:

1. ✅ **资源提供**:提供 CPU/Memory/Disk
2. ✅ **容器管理**:创建/启动/停止容器
3. ✅ **状态上报**:向 API Server 汇报实际状态
4. ✅ **镜像管理**:拉取和缓存镜像
5. ✅ **日志收集**:捕获容器输出

**关键原则**:

- **Control Plane 不做重活**:不直接运行容器,不承担计算负载
- **Node 不做决策**:只执行指令,不决定调度策略
- **状态驱动**:通过 etcd 中的状态变化触发各组件行动
- **异步解耦**:Controllers 通过 Watch 机制异步响应,避免阻塞

这种分工使得:

- Control Plane 可以水平扩展(多副本 API Server)
- Node 可以独立扩缩容(增加计算能力)
- 故障隔离(Control Plane 故障不影响已运行的 Pod)
- 易于维护(各司其职,边界清晰)

---

#### 5.1.8 Namespace、Agent、Pod、Container 的关系

在 Kuscia (基于 Kubernetes) 中,**Namespace**、**Agent**、**Pod**、**Container** 是四个核心概念,它们之间有明确的层次关系和协作机制。

##### A. 四者的基本定义

**1. Namespace (命名空间)** - 🏢 **逻辑隔离层**

- **是什么**: Kubernetes 中的虚拟集群,用于资源隔离和组织
- **作用**: 将物理集群划分为多个逻辑单元
- **类比**: 🏢 大楼中的不同楼层/部门
- **示例**: `alice`、`bob`、`kuscia-system`

**2. Agent (节点代理)** - 🤖 **节点管理者**

- **是什么**: 运行在每个工作节点上的守护进程,替代标准 K8s 的 Kubelet
- **作用**: 管理节点上的 Pod 生命周期、容器运行时、资源上报
- **类比**: 🤖 楼层管理员,负责管理该楼层的所有办公室
- **组件**: NodeProvider、PodsController、PodProvider、Status Manager

**3. Pod (最小调度单元)** - 📦 **工作负载载体**

- **是什么**: Kubernetes 中最小的可部署和调度单元
- **作用**: 封装一个或多个紧密耦合的 Container
- **类比**: 📦 办公室中的一个工作组
- **特点**: 共享网络栈、存储卷、生命周期

**4. Container (容器)** - 🐳 **应用运行环境**

- **是什么**: 轻量级的虚拟化环境,包含应用代码和依赖
- **作用**: 实际执行隐私计算任务
- **类比**: 🐳 工作组中的具体工作人员
- **运行时**: RunC (容器化)、RunP (进程)、RunK (外部 K8s)

---

##### B. 层次关系图

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                        │
│                     (物理/虚拟集群)                           │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       │ 包含多个
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                   Namespace: alice                           │
│              (逻辑隔离: Alice 参与方)                         │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌────────────────────────────────────────────────┐         │
│  │  Node 1 (运行 Agent)                            │         │
│  │  ┌──────────────────────────────────────────┐  │         │
│  │  │  Pod: psi-task-001-alice-pod-0           │  │         │
│  │  │  ┌────────────────────────────────────┐  │  │         │
│  │  │  │ Container: secretflow              │  │  │         │
│  │  │  │ - Image: secretflow/psi:latest     │  │  │         │
│  │  │  │ - Command: /root/main              │  │  │         │
│  │  │  │ - Resources: CPU 2, Memory 4Gi     │  │  │         │
│  │  │  └────────────────────────────────────┘  │  │         │
│  │  └──────────────────────────────────────────┘  │         │
│  │                                                  │         │
│  │  ┌──────────────────────────────────────────┐  │         │
│  │  │  Pod: fl-task-002-alice-pod-0            │  │         │
│  │  │  ┌────────────────────────────────────┐  │  │         │
│  │  │  │ Container: worker                  │  │  │         │
│  │  │  │ - Image: secretflow/fl:latest      │  │  │         │
│  │  │  └────────────────────────────────────┘  │  │         │
│  │  └──────────────────────────────────────────┘  │         │
│  └────────────────────────────────────────────────┘         │
│                                                              │
│  ┌────────────────────────────────────────────────┐         │
│  │  Node 2 (运行 Agent)                            │         │
│  │  ┌──────────────────────────────────────────┐  │         │
│  │  │  Pod: ...                                │  │         │
│  │  └──────────────────────────────────────────┘  │         │
│  └────────────────────────────────────────────────┘         │
│                                                              │
└─────────────────────────────────────────────────────────────┘
                       │
                       │ 同一集群中还有其他 Namespace
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                   Namespace: bob                             │
│              (逻辑隔离: Bob 参与方)                           │
└─────────────────────────────────────────────────────────────┘
```

---

##### C. 协作流程详解

**场景**: Alice 提交一个 PSI 隐私集合求交任务

```yaml
# kusciajob.yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-job-001
spec:
  initiator: alice
  tasks:
  - taskID: psi-task-001
    parties:
    - domainID: alice
      appImage: psi-image
    - domainID: bob
      appImage: psi-image
```

**完整执行流程**:

```
┌─────────────────────────────────────────────────────────────┐
│ Step 1: 用户提交 KusciaJob                                  │
└──────────────────┬──────────────────────────────────────────┘
                   │ kubectl apply -f kusciajob.yaml -n alice
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 2: API Server 接收请求                                 │
│ - 验证权限                                                   │
│ - 保存到 etcd                                                │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 3: KusciaJob Controller (Control Plane)                │
│ - 解析 Job 配置                                              │
│ - 创建 KusciaTask                                            │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 4: KusciaTask Controller (Control Plane)               │
│ - 为每个参与方创建 TaskResource                              │
│ - 触发资源预留                                               │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 5: TaskResource Controller (Control Plane)             │
│ - 构建 Pod Spec                                              │
│   ├─ Namespace: alice                                        │
│   ├─ Containers:                                             │
│   │   └─ Image: secretflow/psi:latest                       │
│   │       Command: /root/main                                │
│   ├─ Resources: CPU 2, Memory 4Gi                            │
│   └─ Volumes: config, data                                   │
│ - 调用 K8s API 创建 Pod                                      │
└──────────────────┬──────────────────────────────────────────┘
                   │ POST /api/v1/namespaces/alice/pods
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 6: API Server + Scheduler (Control Plane)              │
│ - 验证 Pod Spec                                              │
│ - 保存到 etcd                                                │
│ - Scheduler 选择目标 Node (Node 1)                           │
└──────────────────┬──────────────────────────────────────────┘
                   │ Pod 绑定到 Node 1
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 7: Agent on Node 1 (Node 层)                           │
│                                                              │
│  PodsController:                                             │
│  - Watch 到新的 Pod 事件                                     │
│  - 提取 Pod Spec                                             │
│                                                              │
│  PodProvider (RunC):                                         │
│  - 拉取镜像: crictl pull secretflow/psi:latest              │
│  - 创建容器沙箱 (pause 容器)                                  │
│  - 启动业务容器:                                              │
│    docker run secretflow/psi:latest                          │
│      /root/main --task_id psi-task-001                      │
│                                                              │
│  Status Manager:                                             │
│  - 监控容器状态                                              │
│  - 定期上报到 API Server                                     │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 8: Container Runtime (containerd)                      │
│ - 解压镜像层                                                 │
│ - 创建容器文件系统                                           │
│ - 设置 cgroup 限制 (CPU 2, Memory 4Gi)                      │
│ - 启动容器进程                                               │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 9: Container 内部执行                                   │
│                                                              │
│  /root/main (Shell 脚本):                                    │
│  #!/bin/bash                                                 │
│  python3 /root/psi_compute.py \                              │
│    --task_id "psi-task-001" \                               │
│    --party_id "alice"                                       │
│                                                              │
│  /root/psi_compute.py (Python):                              │
│  import secretflow.psi as psi                                │
│  protocol = psi.EcdhPsiProtocol()                            │
│  intersection = protocol.compute(...)                        │
│                                                              │
│  输出结果: /root/output/result.csv                           │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────┐
│ Step 10: 状态回报                                            │
│                                                              │
│  Agent (Node 1):                                             │
│  - 检测到容器退出 (Exit Code 0)                              │
│  - 更新 Pod 状态: Running → Succeeded                        │
│  - 上报到 API Server                                         │
│                                                              │
│  API Server:                                                 │
│  - 更新 etcd 中的 Pod 状态                                   │
│  - 触发 Controller 更新 Task 状态                            │
│                                                              │
│  KusciaTask Controller:                                      │
│  - 更新 KusciaTask 状态: Running → Succeeded                 │
│  - 检查所有参与方是否完成                                     │
│                                                              │
│  KusciaJob Controller:                                       │
│  - 更新 KusciaJob 状态: Running → Succeeded                  │
└─────────────────────────────────────────────────────────────┘
```

---

##### D. 四者的关键关系

**1. Namespace ↔ Agent**

- **关系**: Namespace 是逻辑隔离,Agent 是物理执行
- **说明**: 
  - 一个 Namespace 中的 Pod 可能分布在多个 Node (由不同 Agent 管理)
  - 一个 Agent 管理的 Node 上可能运行多个 Namespace 的 Pod
- **示例**:

  ```bash
  # alice Namespace 的 Pod 分布在两个 Node 上
  kubectl get pods -n alice -o wide
  # NAME                         READY   STATUS    NODE
  # psi-task-001-alice-pod-0     1/1     Running   node-1
  # fl-task-002-alice-pod-0      1/1     Running   node-2
  
  # node-1 上的 Agent 管理多个 Namespace 的 Pod
  kubectl get pods --all-namespaces -o wide | grep node-1
  # NAMESPACE   NAME                        NODE
  # alice       psi-task-001-alice-pod-0    node-1
  # bob         psi-task-001-bob-pod-0      node-1
  # kube-system coredns-xxx                 node-1
  ```

**2. Agent ↔ Pod**

- **关系**: Agent 管理 Pod 的完整生命周期
- **职责**:
  - **创建**: 根据 Pod Spec 启动容器
  - **监控**: 定期检查容器健康状态
  - **更新**: 处理 Pod 配置变更
  - **删除**: 清理容器和资源
- **工作流程**:

  ```
  Pod Spec (etcd)
      ↓ Watch
  PodsController (Agent)
      ↓ 调用
  PodProvider (RunC/RunP/RunK)
      ↓ 执行
  Container Runtime (containerd)
      ↓ 反馈
  Status Manager (Agent)
      ↓ 上报
  API Server → etcd
  ```

**3. Pod ↔ Container**

- **关系**: Pod 是 Container 的封装和调度单元
- **特点**:
  - **1:N 关系**: 一个 Pod 可以包含多个 Container
  - **共享资源**: 网络、存储、IPC、PID Namespace
  - **共同生命周期**: 同时启动、同时停止
- **示例**:

  ```yaml
  apiVersion: v1
  kind: Pod
  metadata:
    name: multi-container-pod
    namespace: alice
  spec:
    containers:
    - name: main          # 主容器: 执行 PSI 计算
      image: secretflow/psi:latest
      command: ["/root/main"]
    - name: sidecar       # 边车容器: 日志收集
      image: fluentd:latest
      volumeMounts:
      - name: logs
        mountPath: /var/log
    volumes:
    - name: logs
      emptyDir: {}
  ```

**4. Namespace ↔ Pod**

- **关系**: Namespace 是 Pod 的逻辑分组
- **作用**:
  - **隔离**: 不同 Namespace 的 Pod 默认无法通信
  - **配额**: 可以为每个 Namespace 设置资源配额
  - **权限**: RBAC 基于 Namespace 进行授权
- **示例**:

  ```bash
  # 查看 alice Namespace 中的所有 Pod
  kubectl get pods -n alice
  
  # 为 alice Namespace 设置资源配额
  kubectl create quota alice-quota \
    --hard=cpu=100,memory=200Gi,pods=50 \
    -n alice
  ```

---

##### E. 数据流与状态流

**数据流 (Data Flow)**:

```
用户输入数据
    ↓
DomainData (注册到 Namespace)
    ↓
挂载到 Pod (Volume)
    ↓
Container 读取数据
    ↓
执行隐私计算
    ↓
输出结果 (写入 Volume)
    ↓
持久化到存储
```

**状态流 (Status Flow)**:

```
Container 状态变化
    ↓
PodProvider 检测
    ↓
Status Manager 收集
    ↓
Agent 批量上报
    ↓
API Server 接收
    ↓
etcd 持久化
    ↓
Controller Watch 到变化
    ↓
更新上层对象状态 (Task → Job)
```

---

##### F. 实际案例分析

**案例 1: 单域 PSI 任务**

```bash
# 1. 查看 Namespace
kubectl get namespaces
# NAME              STATUS   AGE
# alice             Active   30d
# bob               Active   30d

# 2. 查看 alice Namespace 中的 Pod
kubectl get pods -n alice
# NAME                              READY   STATUS      AGE
# psi-task-001-alice-pod-0          0/1     Completed   5m

# 3. 查看 Pod 详情
kubectl describe pod psi-task-001-alice-pod-0 -n alice
# Name:         psi-task-001-alice-pod-0
# Namespace:    alice
# Node:         node-1/192.168.1.101
# Status:       Succeeded
# IP:           10.42.0.15
# Controlled By: TaskResource/psi-task-001-alice-tr

# Containers:
#   secretflow:
#     Image:      secretflow/psi:latest
#     Port:       54509/TCP
#     State:      Terminated (Exit Code: 0)
#     Ready:      True
#     Restart Count: 0

# 4. 查看 Pod 所在的 Node 和 Agent
kubectl get pod psi-task-001-alice-pod-0 -n alice -o jsonpath='{.spec.nodeName}'
# node-1

# 5. 查看该 Node 上的 Agent 状态
kubectl get nodes node-1
# NAME     STATUS   ROLES   AGE   VERSION
# node-1   Ready    agent   30d   v1.27.3+kuscia
```

**案例 2: 跨域协同任务**

```bash
# Alice 和 Bob 的 Pod 分别运行在不同的 Namespace 和 Node 上

# Alice 侧
kubectl get pods -n alice -o wide
# NAME                         READY   STATUS    NODE     IP
# psi-task-001-alice-pod-0     1/1     Running   node-1   10.42.0.15

# Bob 侧
kubectl get pods -n bob -o wide
# NAME                         READY   STATUS    NODE     IP
# psi-task-001-bob-pod-0       1/1     Running   node-2   10.42.1.20

# 两个 Pod 通过 Envoy 进行跨域通信
# alice-pod (10.42.0.15) ←→ Envoy ←→ bob-pod (10.42.1.20)
```

---

##### G. 常见问题解答

**Q1: 一个 Pod 可以跨越多个 Namespace 吗?**

- ❌ **不可以**。Pod 必须属于且仅属于一个 Namespace。
- ✅ 如果需要跨 Namespace 通信,使用 Service 或 NetworkPolicy。

**Q2: 一个 Agent 可以管理多个 Namespace 的 Pod 吗?**

- ✅ **可以**。Agent 不关心 Namespace,它只负责执行分配给该 Node 的所有 Pod。
- 示例:

  ```bash
  # node-1 上的 Agent 管理多个 Namespace 的 Pod
  kubectl get pods --all-namespaces -o wide | grep node-1
  # alice    psi-task-001-alice-pod-0    node-1
  # bob      psi-task-001-bob-pod-0      node-1
  # system   monitoring-pod              node-1
  ```

**Q3: 一个 Pod 可以有多个 Container 吗?**

- ✅ **可以**。这是常见模式 (Sidecar、Init Container)。
- 示例:

  ```yaml
  spec:
    initContainers:
    - name: init-data    # 初始化容器: 准备数据
      image: busybox
      command: ['sh', '-c', 'cp /data/* /work/']
    containers:
    - name: main         # 主容器: 执行计算
      image: secretflow/psi:latest
    - name: logger       # 边车容器: 日志收集
      image: fluentd:latest
  ```

**Q4: Namespace 删除后,Pod 会怎样?**

- ⚠️ **所有 Pod 会被级联删除**。
- 警告:

  ```bash
  kubectl delete namespace alice
  # 这会删除 alice 中的所有 Pod、Service、ConfigMap 等资源!
  ```

**Q5: Agent 宕机后,Pod 会怎样?**

- ⚠️ **Pod 状态变为 Unknown**,但不会立即删除。
- 恢复机制:

  ```
  1. Node Controller 检测到 Node NotReady (5分钟)
  2. 标记该 Node 上的 Pod 为 Terminating
  3. Scheduler 在其他 Node 上重新调度 Pod (如果有 ReplicaSet/Deployment)
  4. 原 Node 恢复后,Agent 重新同步状态
  ```

---

##### H. 总结对比表

| 维度 | Namespace | Agent | Pod | Container |
| ------ | ----------- | ------- | ----- | ----------- |
| **层级** | 逻辑隔离层 | 节点管理层 | 调度单元层 | 执行单元层 |
| **数量关系** | 1个集群有多个 | 1个Node有1个 | 1个Namespace有多个 | 1个Pod有1-N个 |
| **生命周期** | 手动创建/删除 | 随Node启动/停止 | 自动创建/销毁 | 随Pod启动/停止 |
| **隔离级别** | 逻辑隔离 | 无隔离 | 弱隔离 | 强隔离(RunC) |
| **资源管理** | ResourceQuota | Capacity Manager | requests/limits | cgroup限制 |
| **可见性** | `kubectl get ns` | `kubectl get nodes` | `kubectl get pods` | `crictl ps` |
| **类比** | 🏢 大楼楼层 | 🤖 楼层管理员 | 📦 办公室 | 🐳 工作人员 |

**核心理解**:

- **Namespace** = 逻辑边界(为什么隔离)
- **Agent** = 物理执行者(在哪里运行)
- **Pod** = 调度单位(运行什么)
- **Container** = 实际载体(如何运行)

四者协同工作,构成了 Kuscia 的完整运行时架构! 🎯

### 5.2 三种运行时的详细对比

**⚠️ 重要说明:不一定非要使用 Docker 镜像!**

Kuscia 支持**三种运行时模式**,对“是否需要镜像”有不同的要求:

| 特性 | RunC | RunP | RunK |
| ------ | ------ | ------ | ------ |
| **资源隔离** | 完整隔离(Namespace + Cgroup) | 弱隔离(仅 Cgroup) | 完整隔离(依赖后端 K8s) |
| **部署权限** | 需要特权或 root | 无特殊要求 | 需要 K8s 资源创建权限 |
| **安全风险扩散** | 低(容器隔离) | 高(进程共享) | 低(容器隔离) |
| **资源利用率** | 较低(每个 Pod 独立容器) | 较低(共享宿主资源) | 高(可动态扩缩) |
| **启动速度** | 中等(秒级) | 快(毫秒级) | 中等(依赖后端) |
| **运维复杂度** | 中等 | 低 | 高(需维护两套 K8s) |
| **适用规模** | 中小规模 | 小规模/开发测试 | 大规模/生产环境 |
| **网络模式** | 独立网络命名空间 | 共享宿主网络 | 后端 K8s 网络 |
| **存储隔离** | 独立卷挂载 | 共享文件系统 | 后端 K8s 存储 |
| **日志管理** | 完善(标准输出+文件) | 基础(文件重定向) | 完善(依赖后端) |
| **镜像管理** | 完整(拉取/缓存/GC) | 无(直接使用本地) | 依赖后端 K8s |

| 运行时 | 是否需要镜像 | 执行方式 | 适用场景 |
| -------- | ------------ | --------- | ---------- |
| **RunC** | ✅ 需要 | 容器化运行 | 生产环境,强隔离 |
| **RunK** | ✅ 需要 | K8s Pod 运行 | K8s 集群部署 |
| **RunP** | ❌ **不需要** | **直接运行本地代码/进程** | 开发测试,快速部署 |

#### 5.2.1 RunC (Container Runtime) - 需要镜像

```yaml
# 配置示例
agent:
  provider:
    runtime: runc  # 容器运行时
```

**执行流程**:

```
1. Agent 从 Pod Spec 读取镜像地址
   image: secretflow/psi:latest
   
2. 调用 containerd 拉取镜像
   crictl pull secretflow/psi:latest
   
3. 基于镜像创建容器
   - 创建 Sandbox (Pause 容器)
   - 创建业务容器 (Ray Head + Workers)
   
4. 启动容器,运行镜像中的程序
   docker run secretflow/psi:latest python psi_compute.py
```

**特点**:

- ✅ 完整的容器隔离(CPU、内存、网络、文件系统)
- ✅ 标准化的镜像管理
- ✅ 可重复、可移植
- ❌ 需要先构建并推送镜像
- ❌ 需要 containerd 或 Docker
- ❌ 需要特权权限或 root 访问

---

#### 5.2.2 RunP (Process Runtime) - ⭐ 可以直接运行本地代码!**

```yaml
# 配置示例
agent:
  provider:
    runtime: runp  # 进程运行时
```

**执行流程**:

```
1. Agent 读取 AppImage 配置
   appImage:
     name: secretflow-app
     config:
       command: "python /app/psi_compute.py"
       workDir: /home/kuscia/var/apps/psi
       
2. ⭐ 直接在宿主机上 fork/exec 启动进程
   # 不需要拉取镜像!
   cd /home/kuscia/var/apps/psi
   python psi_compute.py
   
3. 通过 cgroup 限制资源
   echo 2000 > /sys/fs/cgroup/cpu/task/cpu.cfs_quota_us
   
4. 捕获进程输出到日志文件
   nohup python psi_compute.py > /var/log/task.log 2>&1
```

**关键特点**:

- ✅ **不需要容器镜像** - 直接运行本地代码/脚本/二进制文件
- ✅ **不需要特权权限** - 普通用户即可运行
- ✅ **启动速度快** - 无容器创建开销
- ✅ **资源开销小** - 无容器运行时 overhead
- ✅ **适合快速迭代** - 修改代码后立即运行
- ⚠️ 隔离性较弱 - 依赖 cgroup 和 namespace
- ⚠️ 仅支持 SecretFlow 引擎(当前版本)

**本地代码目录结构**:

```
/home/kuscia/var/apps/
├── psi-task-001/
│   ├── psi_compute.py      # 你的 Python 代码
│   ├── requirements.txt     # 依赖包
│   ├── data/               # 数据文件
│   │   ├── alice_data.csv
│   │   └── bob_data.csv
│   └── output/             # 输出结果
│       └── result.csv
│
├── fl-task-002/
│   ├── train.py
│   ├── model/
│   └── dataset/
│
└── custom-task-003/
    ├── main.sh             # Shell 脚本
    └── config.yaml
```

**如何准备本地代码**:

**方法 1: 手动放置代码文件**

```bash
# 1. 在节点上创建应用目录
mkdir -p /home/kuscia/var/apps/psi-task-001

# 2. 复制你的代码
cp ~/my_projects/psi_compute.py /home/kuscia/var/apps/psi-task-001/
cp ~/my_projects/requirements.txt /home/kuscia/var/apps/psi-task-001/

# 3. 安装依赖(如果需要)
cd /home/kuscia/var/apps/psi-task-001
pip install -r requirements.txt

# 4. 准备数据文件
cp ~/data/alice_data.csv /home/kuscia/var/apps/psi-task-001/data/
```

**方法 2: 通过 Docker cp 复制到节点容器**

```bash
# 如果是 Docker 部署模式
docker cp ~/my_projects/psi_compute.py alice-node:/home/kuscia/var/apps/psi-task-001/
docker cp ~/my_projects/requirements.txt alice-node:/home/kuscia/var/apps/psi-task-001/

# 进入节点容器执行
docker exec -it alice-node bash
cd /home/kuscia/var/apps/psi-task-001
pip install -r requirements.txt
```

**方法 3: 通过挂载卷共享代码**

```yaml
# docker-compose.yaml 或 K8s Deployment
volumes:
  - type: bind
    source: /host/path/to/my/code
    target: /home/kuscia/var/apps
```

**提交任务时使用本地代码**:

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-local-code-job
  namespace: alice
spec:
  initiator: alice
  tasks:
  - taskID: psi-task-local
    parties:
    - domainID: alice
      # ⭐ 不需要指定镜像,而是指定本地代码路径
      appImage:
        name: local-psi-app
        config:
          command: "python /home/kuscia/var/apps/psi-task-local/psi_compute.py"
          workDir: /home/kuscia/var/apps/psi-task-local
          env:
            DATA_PATH: /home/kuscia/var/apps/psi-task-local/data
            OUTPUT_PATH: /home/kuscia/var/apps/psi-task-local/output
```

**查看本地代码任务的执行**:

```bash
# 1. 查看进程
ps aux | grep psi_compute
# root  12345  5.2  2.1  python /home/kuscia/var/apps/psi-task-local/psi_compute.py

# 2. 查看日志
tail -f /var/log/kuscia/tasks/psi-task-local.log
# [2024-01-15 10:30:00] Starting PSI computation with local code...
# [2024-01-15 10:30:05] Loading data from /home/kuscia/var/apps/psi-task-local/data/alice_data.csv
# [2024-01-15 10:30:30] PSI completed! Result saved to output/result.csv

# 3. 查看资源使用
cat /sys/fs/cgroup/cpu/kuscia/psi-task-local/cpuacct.usage
# 1234567890  # CPU 使用时间(纳秒)

cat /sys/fs/cgroup/memory/kuscia/psi-task-local/memory.usage_in_bytes
# 4294967296  # 内存使用(字节)
```

**调试本地代码任务**:

```bash
# 1. 直接进入代码目录
cd /home/kuscia/var/apps/psi-task-local

# 2. 手动运行代码进行测试
python psi_compute.py --debug

# 3. 修改代码后立即重新运行(无需重新构建镜像!)
vim psi_compute.py  # 修改代码
python psi_compute.py  # 立即测试

# 4. 查看进程环境变量
 cat /proc/12345/environ | tr '\0' '\n'
```

---

**总结:如何选择运行时?**

| 场景 | 推荐运行时 | 原因 |
| ------ | ----------- | ------ |
| **生产环境,多租户** | RunC | 强隔离,安全性高 |
| **K8s 集群部署** | RunK | 利用 K8s 原生能力 |
| **开发测试,快速迭代** | **RunP** | **无需镜像,修改即运行** |
| **资源受限环境** | **RunP** | **开销小,无需特权** |
| **运行自定义算法** | **RunP** | **直接运行本地代码** |
| **CI/CD 流水线** | RunC | 标准化,可重现 |

所以,**答案是**: 

- ✅ **RunC/RunK**: 必须使用 Docker/OCI 镜像
- ⭐ **RunP**: **可以直接运行本地代码,不需要镜像!**
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 12: Node - 容器内执行                              │
│ - Ray Cluster 启动                                       │
│ - SecretFlow 应用代码执行 PSI 计算                       │
│ - Alice 和 Bob 通过加密协议交换数据                      │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 13: Node - Status Manager                          │
│ - 收集容器状态(Running)                                  │
│ - 批量上报到 API Server                                  │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 14: Control Plane - etcd 更新                      │
│ - 更新 Pod.status.phase = Running                       │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 15: Control Plane - Controllers 监听到状态变化      │
│ - KusciaTask Controller 更新 Task 状态                  │
│ - KusciaJob Controller 更新 Job 状态                    │
└─────────────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────────────┐
│ Step 16: 计算完成                                       │
│ - 容器退出,Pod 状态变为 Succeeded                        │
│ - 状态逐级上报,Job 最终标记为 Succeeded                  │
└─────────────────────────────────────────────────────────┘

#### 5.2.3 RunK (K8s Runtime)

**B. RunK (Kubernetes Runtime) - 需要镜像**

```yaml
# 配置示例
agent:
  provider:
    runtime: runk  # K8s 运行时
```

**执行流程**:

```
1. TaskResource Controller 创建 Pod CR
   spec:
     containers:
     - image: secretflow/psi:latest
       
2. K8s Scheduler 调度 Pod 到节点

3. 节点的 Kubelet 拉取镜像并创建容器
   (与 RunC 类似,但由 K8s 原生管理)
```

**特点**:

- ✅ 利用 K8s 原生能力
- ✅ 适合大规模集群
- ✅ 完善的调度和监控
- ❌ 需要外部 K8s 集群
- ❌ 需要镜像仓库

---
**代码路径**: `pkg/agent/provider/pod/k8s_provider.go`

**特点**:

- 将任务 Pod 提交至外部 Kubernetes 集群执行
- Agent 本身不运行容器,仅作为调度代理
- 利用外部 K8s 集群的资源调度和管理能力
- 支持大规模并发任务
- 需要配置后端 K8s 集群的连接信息

**工作原理**:

1. K8sProvider 接收 Pod 创建请求
2. 将 Pod 转换为后端 K8s 集群的格式
3. 通过 BackendPlugin 将 Pod 提交到后端集群
4. 监听后端 Pod 状态变化并同步回主集群
5. 支持多种后端插件(Raw K8s、自定义后端等)

**关键组件**:

- `k8s_provider.go`: K8s Provider 核心实现
- `kubebackend/`: 后端 K8s 集群操作抽象
- `k8s_provider_leader.go`: 领导者选举(多实例场景)
- `env.go`: 环境变量注入和配置

**Backend 插件系统**:

```go
type BackendPlugin interface {
    Init(config *yaml.Node) error
    CreatePod(ctx context.Context, pod *v1.Pod) error
    UpdatePod(ctx context.Context, pod *v1.Pod) error
    DeletePod(ctx context.Context, namespace, name string) error
    GetPod(ctx context.Context, namespace, name string) (*v1.Pod, error)
}
```

**适用场景**:

- 高并发、大规模任务场景
- 已有成熟的 K8s 集群基础设施
- 需要动态扩缩容的场景
- 多租户资源共享场景

**配置示例**:

```yaml
agent:
  provider:
    runtime: runk
    k8s:
      namespace: kuscia-backend
      kubeconfigFile: /path/to/kubeconfig
      endpoint: https://k8s-api-server:6443
      qps: 250
      burst: 500
      backend:
        name: raw  # 使用原生 K8s 后端
      dns:
        policy: None
        servers:
          - 10.96.0.10
```

### 5.4 资源管理机制

#### 5.4.1 容量管理 (Capacity Manager)

**代码路径**: `pkg/agent/provider/node/`

容量管理器负责计算和上报节点可用资源:

```go
type CapacityManager struct {
    cpuCapacity      resource.Quantity
    memoryCapacity   resource.Quantity
    podsCapacity     resource.Quantity
    reservedCPU      resource.Quantity
    reservedMemory   resource.Quantity
    reservedBandwidth resource.Quantity
}
```

**资源计算公式**:

```
可用 CPU = 总 CPU - 保留 CPU - 系统开销
可用内存 = 总内存 - 保留内存 - 系统开销
可用 Pods = min(配置值, 系统限制)
```

**默认保留资源**:

- CPU: 0.5 Core
- 内存: 500 MiB
- 带宽: 10 Mbps

#### 5.4.2 Cgroup 管理

**代码路径**: `pkg/utils/cgroup/`

对于 RunC 和 RunP 模式,Agent 会初始化 cgroup 来限制资源使用:

```go
// RunP 模式:创建 kuscia-apps cgroup
m, err := newCgroupManager(cm, cgroup.KusciaAppsGroup)
err = m.AddCgroup()

// RunC 模式:更新 k8s.io cgroup
m, err := newCgroupManager(cm, cgroup.K8sIOGroup)
err = m.UpdateCgroup()
```

**Cgroup 配置项**:

- `cpu_quota`: CPU 配额(微秒)
- `cpu_period`: CPU 周期(微秒,默认 100ms)
- `memory_limit`: 内存限制(字节)

#### 5.4.3 资源预留策略

资源配置优先级:

1. **系统保留**:操作系统和内核使用
2. **Kuscia 保留**:Agent 和系统组件使用
3. **任务分配**:根据 Pod requests/limits 分配
4. **超卖控制**:可选的资源超卖比例

### 5.5 Pod 生命周期管理流程

#### 5.5.1 PodsController 核心流程

**代码路径**: `pkg/agent/framework/pods_controller.go`

PodsController 是 Agent 的核心控制器,负责协调 Pod 的整个生命周期:

```text
┌─────────────────────────────────────────────────────┐
│                 PodsController                       │
├─────────────────────────────────────────────────────┤
│  1. Watch Pod Events (Add/Update/Delete)            │
│         ↓                                            │
│  2. Enqueue to WorkQueue                            │
│         ↓                                            │
│  3. PodWorkers 并行处理                              │
│         ↓                                            │
│  4. SyncPod / KillPod / DeletePod                   │
│         ↓                                            │
│  5. Call PodProvider (RunC/RunP/RunK)               │
│         ↓                                            │
│  6. Update Pod Status                               │
│         ↓                                            │
│  7. Sync Status to API Server                       │
└─────────────────────────────────────────────────────┘
```

**关键方法**:

- `HandlePodAdditions()`: 处理新增 Pod
- `HandlePodUpdates()`: 处理 Pod 更新
- `HandlePodRemoves()`: 处理 Pod 删除
- `syncPod()`: 同步 Pod 到期望状态
- `syncTerminatingPod()`: 终止运行中的 Pod
- `HandlePodCleanups()`: 清理孤儿 Pod

#### 5.5.2 状态同步机制

**代码路径**: `pkg/agent/status/`

StatusManager 负责将 Pod 状态同步回 API Server:

```go
type Manager interface {
    SetPodStatus(pod *v1.Pod, status PodStatus)
    GetPodStatus(uid types.UID) (*v1.PodStatus, bool)
    RemovePodStatus(uid types.UID)
    Start() 
}
```

**同步策略**:

- 批量更新:合并多次状态变更
- 重试机制:失败后指数退避重试
- 最终一致性:确保状态最终与 API Server 一致

### 5.6 镜像管理

**代码路径**: `pkg/agent/images/`

#### 5.6.1 镜像拉取流程

```text
1. 检查本地镜像缓存
   ↓ (未命中)
2. 从 Registry 拉取镜像
   ↓
3. 验证镜像完整性
   ↓
4. 解压并存储到本地
   ↓
5. 更新镜像元数据
```

**镜像拉取优化**:

- **并发控制**:限制同时拉取的镜像数量
- **QPS 限制**:避免对 Registry 造成过大压力
- **重试机制**:网络失败时自动重试
- **镜像缓存**:复用已拉取的镜像层

#### 5.6.2 镜像垃圾回收

ImageGCManager 定期清理未使用的镜像:

```go
// 触发条件
type ImageGCPolicy struct {
    MinAge             time.Duration  // 最小存活时间
    HighThresholdPercent int          // 高水位线(%)
    LowThresholdPercent  int          // 低水位线(%)
}
```

**GC 策略**:

- 磁盘使用率超过高水位线时触发
- 优先删除最久未使用的镜像
- 保留正在使用的镜像和最近拉取的镜像
- 清理到磁盘使用率低于低水位线

### 5.7 中间件插件系统

**代码路径**: `pkg/agent/middleware/`

Agent 提供了可扩展的中间件插件系统,用于在 Pod 生命周期各个阶段注入自定义逻辑:

#### 5.7.1 Hook 插件

| 插件名称 | 代码路径 | 功能描述 |
| --------- | ---------- | ---------- |
| **bandwidthfilter** | `middleware/plugins/hook/bandwidthfilter/` | 带宽限制和流量控制 |
| **certissuance** | `middleware/plugins/hook/certissuance/` | TLS 证书自动颁发 |
| **configrender** | `middleware/plugins/hook/configrender/` | 配置文件渲染和注入 |
| **envimport** | `middleware/plugins/hook/envimport/` | 环境变量导入 |
| **imagesecurity** | `middleware/plugins/hook/imagesecurity/` | 镜像安全扫描和验证 |

#### 5.7.2 插件执行时机

```
Pod Creation:
  PreCreate → Create → PostCreate

Pod Start:
  PreStart → Start → PostStart

Pod Stop:
  PreStop → Stop → PostStop

Pod Delete:
  PreDelete → Delete → PostDelete
```

### 5.8 健康检查与探针

**代码路径**: `pkg/agent/prober/`

Agent 支持三种类型的探针:

| 探针类型 | 用途 | 失败动作 |
| --------- | ------ | ---------- |
| **Liveness** | 检测容器是否存活 | 重启容器 |
| **Readiness** | 检测容器是否就绪 | 从 Service 移除 |
| **Startup** | 检测容器是否启动完成 | 等待或重启 |

**探针支持的方式**:

- HTTP GET:发送 HTTP 请求
- TCP Socket:尝试建立 TCP 连接
- Exec:在容器内执行命令

### 5.9 日志管理

**代码路径**: `pkg/web/logs/` 和 `pkg/agent/kuberuntime/`

#### 5.9.1 容器日志

- **标准输出日志**:捕获容器的 stdout/stderr
- **日志轮转**:达到大小限制后自动轮转
- **日志保留**:保留最近 N 个日志文件
- **日志路径**:`var/stdout/<pod-uid>/<container-name>/`

**配置参数**:

```yaml
agent:
  provider:
    cri:
      containerLogMaxSize: "50Mi"   # 单个日志文件最大大小
      containerLogMaxFiles: 5       # 保留的日志文件数量
```

#### 5.9.2 Agent 自身日志

- **日志级别**:DEBUG/INFO/WARN/ERROR/FATAL
- **日志输出**:同时输出到文件和标准输出
- **日志轮转**:支持按大小和时间轮转
- **日志压缩**:自动压缩旧日志文件

### 5.10 运行时选择建议

#### 5.10.1 选择决策树

```
是否需要强隔离?
├─ 是 → 是否有特权权限?
│       ├─ 是 → 选择 RunC
│       └─ 否 → 是否有外部 K8s 集群?
│               ├─ 是 → 选择 RunK
│               └─ 否 → 无法满足需求
└─ 否 → 是否为开发/测试环境?
        ├─ 是 → 选择 RunP
        └─ 否 → 是否有外部 K8s 集群?
                ├─ 是 → 选择 RunK
                └─ 否 → 选择 RunC(如果可能)或 RunP
```

#### 5.10.2 典型部署场景

| 场景 | 推荐运行时 | 理由 |
| ------ | ----------- | ------ |
| **单机隐私计算** | RunC | 强隔离、安全性高 |
| **K8s 内嵌部署** | RunP | 部署简单、资源占用少 |
| **多云联邦计算** | RunK | 跨集群调度、资源弹性 |
| **边缘计算节点** | RunP/RunC | 根据资源情况选择 |
| **高性能计算** | RunK | 利用 HPC 集群资源 |
| **开发调试** | RunP | 快速迭代、易于调试 |

### 5.11 故障排查与诊断

#### 5.11.1 常见问题

**问题 1: Pod 一直处于 Pending 状态**

- 检查节点资源是否充足
- 检查节点标签和亲和性配置
- 查看 Agent 日志中的调度错误

**问题 2: 容器启动失败**

- RunC: 检查 containerd 服务状态
- RunP: 检查进程启动日志和权限
- RunK: 检查后端 K8s 集群连接

**问题 3: 镜像拉取失败**

- 检查 Registry 连接性
- 验证镜像名称和标签
- 检查认证凭据配置

**问题 4: 资源超限**

- 检查 cgroup 配置
- 验证资源 requests/limits 设置
- 监控实际资源使用情况

#### 5.11.2 诊断工具

Kuscia 提供了诊断工具来检查 Agent 运行状态:

```bash
# 检查 Agent 状态
kuscia diagnose agent --node <node-name>

# 检查 Pod 状态
kuscia diagnose pod --namespace <ns> --name <pod-name>

# 检查运行时环境
kuscia diagnose runtime --type runc|runp|runk
```

---

## 6. 任务调度与数据流

### 6.1 典型任务调度流程

```text
SecretPad / 业务系统
        │
        │ 调用 KusciaAPI CreateJob
        ▼
┌──────────────────┐
│ KusciaAPI Server │  pkg/kusciaapi/handler/httphandler/
└────────┬─────────┘
         │ 创建 KusciaJob CR
         ▼
┌────────────────────┐
│ KusciaJob Controller│ pkg/controllers/kusciajob/
└────────┬───────────┘
         │ 按 DAG 创建 KusciaTask CR
         ▼
┌────────────────────┐
│ KusciaTask Controller│ pkg/controllers/kusciatask/
└────────┬───────────┘
         │ 创建 TaskResourceGroup / Pod / Service / ConfigMap
         ▼
┌─────────────────────────┐
│ TaskResourceGroup Controller│ pkg/controllers/taskresourcegroup/
└────────┬────────────────┘
         │ 资源预留协调
         ▼
┌────────────────────┐
│ Kuscia Scheduler   │ pkg/scheduler/kusciascheduling/
└────────┬───────────┘
         │ 绑定 Pod 到 Node
         ▼
┌────────────────────┐
│ Agent / Pods Controller│ pkg/agent/framework/pods_controller.go
└────────┬───────────┘
         │ 拉取镜像、启动容器
         │
         │
         │
         ▼
┌────────────────────┐
│ 引擎（Ray + SecretFlow）│
└────────┬───────────┘
         │ 通过 DataMesh 读取/写入数据
         ▼
┌────────────────────┐
│      DataMesh      │
└────────────────────┘
```

### 6.2 状态机

**KusciaJob 状态机：**

```text
Initialized → PendingApproval → Running → Succeeded / Failed
                  ↓
              Rejected
```

**KusciaTask 状态机：**

```text
Pending → Running → Succeeded / Failed
```

**对应的函数接口：**

- `kusciaJobDefault()` - 设置KusciaJob默认字段
- `failKusciaJob()` - 处理KusciaJob失败状态
- `syncHandler()` - 同步处理KusciaJob状态
- `handlerFactory.KusciaJobPhaseHandlerFor(phase).HandlePhase()` - 根据阶段处理KusciaJob
- `NewController()` - 创建KusciaJob控制器实例
- `Run()` - 启动控制器运行
- `enqueueKusciaJob()` - 将KusciaJob加入工作队列
- `handleTaskObject()` - 处理任务对象事件

### 6.3 P2P 模式下的资源同步

在 P2P 组网中，调度方与参与方各自拥有独立的 K3s 控制平面：

1. Task Controller 在各参与方 Namespace 下创建 TaskResource 和 PodGroup。
2. InterConn Controller 将本方的 TaskResource / PodGroup 同步到参与方集群。
3. Kuscia Scheduler 为 PodGroup 预留资源，满足 `MinReservedPods` 阈值后更新 TaskResource 为 Reserved。
4. Task Controller 监听到满足 `MinReservedMembers` 阈值后，将 TaskResource 更新为 Schedulable。
5. Kuscia Scheduler 绑定 Pod 到已分配节点。

**对应的函数接口：**

- `handleAddedTaskResourceGroup()` - 处理新增的TaskResourceGroup
- `handleUpdatedTaskResourceGroup()` - 处理更新的TaskResourceGroup
- `handleAddedOrDeletedTaskResource()` - 处理新增或删除的TaskResource
- `handleUpdatedTaskResource()` - 处理更新的TaskResource
- `matchLabels()` - 匹配标签筛选资源
- `resourceFilter()` - 资源过滤函数
- `PreFilter()` - 预过滤插件函数
- `PostFilter()` - 后过滤插件函数
- `Permit()` - 许可插件函数
- `Reserve()` - 预留插件函数
- `PreBind()` - 预绑定插件函数
- `PostBind()` - 后绑定插件函数

### 6.4 按 DAG 创建 KusciaTask CR

KusciaJob 是用户视角的“工作流”，`spec.tasks` 与其 `dependencies` 共同构成一张**有向无环图（DAG）**；而 KusciaTask 则是真正被 KusciaTask Controller 调度的“算子实例”。
KusciaJob Controller 的核心职责之一，就是在 Job 进入 `Running` 阶段后，持续解析这张 DAG，按依赖顺序、并发度和调度策略，将满足条件的 `KusciaTaskTemplate` 实例化为 `KusciaTask` CR。

#### 6.4.1 数据模型与关键字段

在理解算法前，先明确三类对象的关系：

| 对象 | 作用 | 关键字段 |
| ------ | ------ | ---------- |
| **KusciaJob** | 工作流定义 | `spec.tasks[]`、`spec.scheduleMode`、`spec.maxParallelism`、`status.taskStatus` |
| **KusciaTaskTemplate** | Job 内的子任务模板 | `alias`、`taskID`、`dependencies`、`tolerable`、`priority`、`appImage`、`parties`、`taskInputConfig` |
| **KusciaTask** | 实际被调度的任务 CR | `metadata.name = taskID`、OwnerReference 指向 KusciaJob、`spec.parties[]` |

其中：

- `alias`：任务的**展示名称**，在 Job 内必须唯一，用于日志、状态展示。
- `taskID`：任务的**调度标识**，KusciaTask CR 的 `metadata.name` 直接使用该值；若用户未填写，由 Controller 在首次进入 Running 阶段时生成（`jobName-<uuid后缀>`）。
- `dependencies`：元素为前置任务的 `alias`。**只有当前置任务全部 `Succeeded` 后，本任务才“就绪”**。
- `tolerable`：是否可容忍失败。`false`（默认）表示关键任务；`true` 表示失败不影响 Job 最终成败。
- `priority`：就绪任务之间的优先级，值越大越先被创建。
- `scheduleMode`：`Strict` 或 `BestEffort`，决定关键任务失败时是否立即停止调度。
- `maxParallelism`：同一时刻最多有多少个任务处于 `Pending/Running` 状态，默认 `1`。

#### 6.4.2 整体控制流程

当 KusciaJob 进入 `Running` 阶段后，`RunningHandler.handleRunning()` 会按如下循环执行调度：

```text
1. 列出该 Job 已创建的所有 KusciaTask（通过 label selector: kuscia.secretflow/controller=kusciajob, kuscia.secretflow/job-uid=<uid>）
2. 根据 KusciaTask 当前 Phase，重建 Job 的 taskStatus 视图
3. 计算当前 Job 应处于的 Phase（Running/Succeeded/Failed）
4. 计算“就绪任务” readyTasks：依赖已全部 Succeeded 且尚未创建的任务
5. 按 priority 排序，并结合 maxParallelism，得到本轮将要创建的 willStartTasks
6. 将 willStartTasks 转换为 KusciaTask CR 并调用 K8s API 创建
7. 更新 Job 的 status.taskStatus 与 status.phase
```

KusciaTask 的状态变化会触发 `handleTaskObject()`，进而将对应 Job 重新加入队列，形成事件驱动的闭环。

#### 6.4.3 核心算法详解

##### 1. Job 提交时的 DAG 校验

在 `Initialized` 阶段，`kusciaJobValidate()` 会对 DAG 做两项基础校验：

- **依赖存在性检查**（`kusciaJobDependenciesExits`）：每个 `dependencies` 中的 `alias` 必须在 `spec.tasks[]` 中存在，否则 Job 进入 `Failed`。
- **环检测**（`kusciaJobHasTaskCycle`）：采用**拓扑剥离法**。循环执行：
    1. 移除所有入度为 0 的任务；
    2. 在剩余任务中删除已被移除任务的依赖；
    3. 若某轮无法移除任何任务，且仍有任务剩余，则说明存在环。

```text
示例：
  tasks: [a->b, b->c, c->a]  # 环
  第 1 轮：无入度为 0 的任务 → 发现环，校验失败

  tasks: [a, a->b, a->c, c->d]
  第 1 轮：移除 a
  第 2 轮：移除 b, c
  第 3 轮：移除 d
  全部移除 → 无环
```

##### 2. TaskID 生成

若用户未显式填写 `taskID`，Controller 会在首次进入 Running 阶段时由 `setJobTaskID()` 统一生成：

```go
taskID = jobName + "-" + uuid.LastSegment()
```

生成后会立即回写 `KusciaJob.spec.tasks[].taskID`，确保后续所有参与方看到的任务标识一致。

##### 3. 就绪任务计算

`readyTasksOf(job, currentTasks)` 的算法如下：

```text
输入：KusciaJob、当前各 alias 的 Phase（currentTasks）
输出：可以立即创建的任务模板列表

1. 深拷贝 Job，遍历每个 task
2. 从 task.dependencies 中过滤掉已经 Succeeded 的依赖
3. 保留过滤后 dependencies 为空且 taskID 非空的任务
4. 再过滤掉 currentTasks 中已存在的任务（已创建）
5. 按 priority 降序排序
6. 返回原始 task 模板的副本
```

关键点：只有依赖**全部 Succeeded** 才就绪；**Failed 的前置任务不会使后置任务就绪**。这意味着失败会自然阻断下游任务的创建，除非失败任务是 `tolerable`（但仍需其状态为 `Succeeded` 才能解除阻塞，因此 tolerable 任务失败后其下游不会继续执行）。

##### 4. 最大并发度裁剪

`willStartTasksOf(job, readyTasks, status)` 负责根据 `maxParallelism` 决定本轮实际创建多少个任务：

```text
runningCount = status 中 Phase 为 Pending/Running/空 的任务数量
可创建数     = maxParallelism - runningCount
willStart    = readyTasks 的前 min(可创建数, len(readyTasks)) 个
```

例如 `maxParallelism=2`：

- 初始时 runningCount=0，可创建 2 个，但只有 1 个就绪（根任务 a），则只创建 a。
- a Succeeded 后，b、c 同时就绪，runningCount=0，可同时创建 b、c。
- b 进入 Running、c 进入 Running 后，d 即使就绪也必须等待 b 或 c 完成。

##### 5. Job 状态计算

`jobStatusPhaseFrom(job, currentSubTasksStatus)` 综合所有任务状态，得出 Job 当前 Phase：

| 条件 | Job Phase |
| ------ | ----------- |
| 所有任务 Finished，且所有关键任务 Succeeded | `Succeeded` |
| `Strict` 模式下，任一关键任务 Failed | `Failed` |
| `BestEffort` 模式下，无就绪/运行中任务，且任一关键任务 Failed | `Failed` |
| 互联互通（InterConn）任务任一任务 Failed | `Failed` |
| 其他情况 | `Running` |

其中 `Finished = Succeeded || Failed`，关键任务 = `tolerable != true`。

##### 6. KusciaTask CR 构建

`buildWillStartKusciaTask()` 将每个就绪任务模板转换为 KusciaTask CR：

```go
KusciaTask{
    ObjectMeta: {
        Name: task.TaskID,
        OwnerReferences: [ControllerRef(KusciaJob)],
        Annotations: {
            "kuscia.secretflow/job-id":              job.Name,
            "kuscia.secretflow/task-alias":          task.Alias,
            "kuscia.secretflow/self-cluster-as-participant": "true/false",
        },
        Labels: {
            "kuscia.secretflow/controller": "kusciajob",
            "kuscia.secretflow/job-uid":    string(job.UID),
        },
    },
    Spec: {
        Initiator:       job.Spec.Initiator,
        TaskInputConfig: task.TaskInputConfig,
        Parties:         buildPartiesFromTaskInputConfig(task),
        ScheduleConfig:  task.ScheduleConfig,
    },
}
```

对于互联互通（BFIA/Kuscia-InterOp）Job，还会额外携带协议相关 annotation/label（如 `LabelInterConnProtocolType`、`LabelTaskUnschedulable` 等）。

`buildPartiesFromTaskInputConfig()` 根据 `task.appImage` 和参与方 `role` 去匹配 `AppImage` 中对应的 `DeployTemplate`，再结合 `parties[].resources` 计算每个容器最终的 `limits/requests`，最终填充到 `spec.parties[].template`。

#### 6.4.4 调度模式对 DAG 执行的影响

KusciaJob 支持两种调度模式，差异体现在**关键任务失败后的行为**：

- **Strict（严格模式）**：任一关键任务失败后，Job 立即置为 `Failed`，不再创建任何后续 KusciaTask。
- **BestEffort（尽力模式）**：关键任务失败后，仅阻塞其下游任务；不依赖失败任务的其他分支继续调度，待所有可达任务都执行完毕后，Job 最终置为 `Failed`。

`Tolerable` 任务失败不会直接导致 Job 失败，但其下游任务仍会被阻塞（因为依赖未满足）。

#### 6.4.5 示例

##### 示例 1：线性 DAG（PSI → 数据分割）

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-then-split
  namespace: cross-domain
spec:
  initiator: alice
  scheduleMode: Strict
  maxParallelism: 1
  tasks:
    - alias: psi
      taskID: psi
      appImage: secretflow-image
      taskInputConfig: '{...psi config...}'
      parties:
        - domainID: alice
        - domainID: bob
    - alias: split
      taskID: split
      dependencies: ['psi']
      appImage: secretflow-image
      taskInputConfig: '{...split config...}'
      parties:
        - domainID: alice
        - domainID: bob
```

执行过程：

| 轮次 | 就绪任务 | 当前 Running | 创建动作 |
|------|----------|--------------|----------|
| 1    | psi      | 0            | 创建 KusciaTask `psi` |
| 2    | split    | 0（psi 已 Succeeded） | 创建 KusciaTask `split` |
| 3    | 无       | 0            | Job 转为 Succeeded/Failed |

##### 示例 2：树形 DAG（并行分支）

```yaml
spec:
  scheduleMode: BestEffort
  maxParallelism: 2
  tasks:
    - alias: a
      taskID: a
    - alias: b
      taskID: b
      dependencies: ['a']
    - alias: c
      taskID: c
      dependencies: ['a']
    - alias: d
      taskID: d
      dependencies: ['c']
```

执行过程：

| 阶段 | 任务状态 | 就绪任务 | 创建动作 |
| ------ | ---------- | ---------- | ---------- |
| 初始 | 全部未创建 | a | 创建 a |
| a Succeeded | a=Succeeded | b, c | 同时创建 b、c（maxParallelism=2） |
| b Succeeded, c Running | a,b=Succeeded; c=Running | d（依赖 c 未满足） | 无 |
| c Succeeded | a,b,c=Succeeded | d | 创建 d |

##### 示例 3：Strict vs BestEffort 在失败场景的差异

沿用示例 2 的 DAG，假设 `b` 是关键任务且执行失败：

- **Strict 模式**：
  - b Failed → 立即将 Job 置为 `Failed`。
  - 即使 c 正在运行，d 也不会再被创建（`currentJobPhase == Failed` 时停止创建新 Task）。

- **BestEffort 模式**：
  - b Failed 仅阻塞 b 的下游（本例中 b 无下游）。
  - c 继续运行；c Succeeded 后 d 仍可被创建。
  - 待 a、c、d 均 Finished 后，Job 因 b 失败而最终变为 `Failed`。

##### 示例 4：创建出的 KusciaTask CR

对于示例 1 中的 `psi` 任务，Controller 会生成如下 KusciaTask：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaTask
metadata:
  name: psi
  namespace: cross-domain
  ownerReferences:
    - apiVersion: kuscia.secretflow/v1alpha1
      kind: KusciaJob
      name: psi-then-split
      uid: <job-uid>
      controller: true
  labels:
    kuscia.secretflow/controller: kusciajob
    kuscia.secretflow/job-uid: <job-uid>
  annotations:
    kuscia.secretflow/job-id: psi-then-split
    kuscia.secretflow/task-alias: psi
    kuscia.secretflow/self-cluster-as-participant: "true"
spec:
  initiator: alice
  taskInputConfig: '{...psi config...}'
  parties:
    - domainID: alice
      appImageRef: secretflow-image
    - domainID: bob
      appImageRef: secretflow-image
```

#### 6.4.6 示例 5：P2P 模式下多方 PSI 的完整调度与数据流

前面四个示例侧重于 **KusciaJob 内部的 DAG 调度规则**。本示例换一个视角，展示一次真实的 **端到端隐私集合求交（PSI）任务** 在 P2P 组网（Alice、Bob 均为 Autonomy 节点）中，从提交到执行完毕的完整数据流与资源协调过程。

##### 场景设定

- Alice 和 Bob 各自部署了一个 **Autonomy** 节点，均包含独立的 K3s 控制平面。
- Alice 是本次任务的 **Initiator**，她拥有两张表：`alice.id` 和 `alice.label`。
- Bob 是参与方，拥有表 `bob.id`。
- 任务目标：先由 Alice 本地做特征拼接，再与 Bob 做 PSI 求交，最后由 Alice 输出结果。

对应的 KusciaJob 如下：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-demo
  namespace: cross-domain
spec:
  initiator: alice
  scheduleMode: Strict
  maxParallelism: 2
  tasks:
    - alias: preprocess
      taskID: preprocess
      appImage: secretflow-image
      taskInputConfig: '{...本地特征拼接配置...}'
      parties:
        - domainID: alice
    - alias: psi
      taskID: psi
      dependencies: ['preprocess']
      appImage: secretflow-image
      taskInputConfig: '{...两方 PSI 配置...}'
      parties:
        - domainID: alice
        - domainID: bob
    - alias: postprocess
      taskID: postprocess
      dependencies: ['psi']
      appImage: secretflow-image
      taskInputConfig: '{...结果落盘配置...}'
      parties:
        - domainID: alice
```

##### 完整执行流程

```text
Alice (Autonomy)                          Bob (Autonomy)
     │                                         │
     │ 1. 创建 KusciaJob psi-demo              │
     ▼                                         │
┌─────────────┐                                │
│KusciaJob    │                                │
│Controller   │                                │
└──────┬──────┘                                │
       │ 2. DAG 校验通过，按依赖创建 KusciaTask │
       ▼                                         │
┌─────────────┐                                │
│KusciaTask   │ 3. 针对 psi 任务创建            │
│Controller   │    TaskResourceGroup trg-psi   │
└──────┬──────┘                                 │
       │ 4. InterConn Controller 同步           │
       │    TaskResource/PodGroup 到 Bob        │
       └──────────────►┌─────────────────┐      │
                       │ TaskResource    │      │
                       │ PodGroup        │      │
                       └────────┬────────┘      │
                                │ 5. 双方 Scheduler
                                │    预留并绑定 Pod
                                ▼                ▼
                       ┌─────────────────┐  ┌─────────────────┐
                       │ Alice Agent     │  │ Bob Agent       │
                       │ 启动 PSI 容器   │  │ 启动 PSI 容器   │
                       └────────┬────────┘  └────────┬────────┘
                                │ 6. 通过 DataMesh   │
                                │    读取/写入数据   │
                                ▼                  ▼
                       ┌─────────────────────────────────────┐
                       │ 引擎完成 PSI，任务状态变为 Succeeded │
                       └─────────────────────────────────────┘
                                │ 7. KusciaTask 成功触发
                                │    postprocess 就绪
                                ▼
                       ┌─────────────────┐
                       │ Alice 本地执行  │
                       │ postprocess     │
                       └─────────────────┘
                                │
                                ▼
                       ┌─────────────────┐
                       │ Job 转为        │
                       │ Succeeded       │
                       └─────────────────┘
```

按阶段拆解：

| 阶段 | 发生位置 | 关键动作 | 产生的 CR / 事件 |
| ------ | ---------- | ---------- | ------------------ |
| 1 | Alice KusciaAPI | 校验 DAG、写入 `KusciaJob` | `KusciaJob/psi-demo` |
| 2 | Alice KusciaJob Controller | `preprocess` 无依赖，立即创建 `KusciaTask` | `KusciaTask/preprocess` |
| 3 | Alice KusciaTask Controller | 为 `preprocess` 创建 Pod；为 `psi` 创建 `TaskResourceGroup` | `TaskResourceGroup/trg-psi` |
| 4 | Alice/Bob InterConn Controller | 将本方 `TaskResource`/`PodGroup` 同步到对端 | 对端 Namespace 下出现同名资源 |
| 5 | Alice/Bob Scheduler | 双方 `TaskResource` 均 Reserved 后，Pod 进入 Schedulable；Scheduler 绑定到节点 | Pod 状态 `Scheduled` → `Running` |
| 6 | Alice/Bob Agent | 拉取镜像、启动容器、挂载 ConfigMap/Secret | Pod 状态 `Running` |
| 7 | Alice/Bob 引擎 | 通过本地 DataMesh 读取 `alice.id`、`bob.id`，写入交集结果 | DataMesh 产生访问日志 |
| 8 | Alice KusciaJob Controller | `psi` Succeeded 后，`postprocess` 就绪并被创建；全部 Succeeded 后 Job 成功 | `KusciaJob` Phase = `Succeeded` |

##### 数据流要点

- **本地任务（preprocess）**：只在 Alice 侧创建 Pod，不触发跨域资源协调。
- **多方任务（psi）**：必须等待 Alice、Bob 双方都完成资源预留后才会真正调度 Pod，体现 All-or-Nothing 调度语义。
- **结果任务（postprocess）**：依赖 `psi` 成功，且只在 Alice 侧运行，因此失败场景下如果 `psi` 失败则不会执行。
- **DataMesh 访问**：引擎通过 `dm` 域名（如 `datamesh.alice.svc`）访问本地 DataMesh，DataMesh 根据 `domaindata` 定义决定实际数据源是 localfs、OSS 还是数据库。

#### 6.4.7 相关单元测试介绍

Kuscia 针对任务调度与数据流的核心路径编写了大量单元测试，主要使用 **Kubernetes fake clientset** 与 **Kuscia fake clientset** 构造虚拟集群状态，无需真实 K3s 即可验证 Controller 与 Scheduler 的行为。

##### 主要 UT 文件与测试重点

| UT 文件 | 测试对象 | 核心覆盖点 |
| --------- | ---------- | ------------ |
| `pkg/controllers/kusciajob/handler/scheduler_test.go` | `JobScheduler` | DAG 合法性校验、`kusciaJobHasTaskCycle` 环检测、就绪任务计算、并发度裁剪、Job 状态推导 |
| `pkg/controllers/kusciajob/handler/running_test.go` | `RunningHandler` | Running 阶段状态机、按 DAG 创建 KusciaTask、`Strict`/`BestEffort` 失败行为、任务重入队列 |
| `pkg/controllers/kusciajob/handler/initialized_test.go` | `InitializedHandler` | 首次处理 Job 时的默认字段填充、TaskID 生成准备 |
| `pkg/controllers/kusciatask/handler/pending_handler_test.go` | `PendingHandler` | KusciaTask 创建 TaskResourceGroup/Pod/Service/ConfigMap 的转换逻辑 |
| `pkg/controllers/taskresourcegroup/handler/reserving_handler_test.go` | `ReservingHandler` | 多方资源预留协调、`MinReservedMembers` 阈值判定、失败回滚 |
| `pkg/scheduler/kusciascheduling/kusciascheduling_test.go` | `KusciaScheduling` 插件 | `PreFilter`/`Reserve`/`Permit`/`PreBind`/`PostBind` 全链路调度插件行为 |
| `pkg/scheduler/kusciascheduling/core/core_test.go` | TaskResource 管理器 | TaskResource 生命周期、预留超时、状态同步 |

##### 典型测试模式

以 `pkg/controllers/kusciajob/handler/scheduler_test.go` 中的 `Test_kusciaJobValidate` 为例：

```go
func Test_kusciaJobValidate(t *testing.T) {
    // ...
    tests := []struct {
        name    string
        args    args
        wantErr assert.ErrorAssertionFunc
    }{
        {
            name: "BestEffort mode task{a,b,c,d} should return want{false}",
            args: args{
                kusciaJob: makeKusciaJob(KusciaJobForShapeIndependent,
                    kusciaapisv1alpha1.KusciaJobScheduleModeBestEffort, 2, nil),
            },
            wantErr: assert.NoError,
        },
        {
            name: "BestEffort mode task{a,b,c,d} cycled should return true",
            args: args{
                kusciaJob: makeKusciaJob(KusciaJobForShapeCycled,
                    kusciaapisv1alpha1.KusciaJobScheduleModeBestEffort, 2, nil),
            },
            wantErr: assert.Error,
        },
    }
    // ...
}
```

测试用例通过 `makeKusciaJob` 构造不同 DAG 形状（独立、树形、环形）的 Job，然后调用 `kusciaJobValidate` 验证是否返回预期错误。环形 DAG 必须被检测出来并返回错误。

再以 `pkg/controllers/kusciajob/handler/running_test.go` 为例，它使用 `kusciafake.NewSimpleClientset()` 预先注入若干已存在的 `KusciaTask`（模拟上一轮已创建的任务），然后调用 `RunningHandler.HandlePhase`，断言：

- 是否需要更新 Job 状态（`wantNeedUpdate`）。
- Job 最终 Phase（`wantJobPhase`）。
- 哪些 KusciaTask 应该被创建或处于 Succeeded（`wantFinalTasks`）。

##### 运行单元测试

在 Kuscia 仓库根目录执行：

```bash
# 运行第 6 章涉及的全部单元测试
go test ./pkg/controllers/kusciajob/... \
         ./pkg/controllers/kusciatask/... \
         ./pkg/controllers/taskresourcegroup/... \
         ./pkg/scheduler/kusciascheduling/...

# 仅运行 DAG 调度相关测试
go test ./pkg/controllers/kusciajob/handler -run Test_kusciaJobValidate -v

# 仅运行 Running 阶段状态机测试
go test ./pkg/controllers/kusciajob/handler -run TestRunningHandler_HandlePhase -v

# 仅运行资源预留协调测试
go test ./pkg/controllers/taskresourcegroup/handler -run TestReservingHandlerHandle -v

# 仅运行 Kuscia Scheduler 插件测试
go test ./pkg/scheduler/kusciascheduling -run TestPermit -v
```

> 提示：首次运行前请确保已执行 `go mod download` 下载依赖；部分测试依赖 `pkg/crd/clientset/versioned/fake` 生成的 fake clientset，若该文件缺失请先执行 CRD 代码生成（详见第 12 章）。

#### 6.4.8 关键代码路径

| 函数/文件 | 作用 |
| ----------- | ------ |
| `pkg/controllers/kusciajob/handler/scheduler.go` | DAG 调度核心：校验、就绪任务计算、状态机、Task CR 构建 |
| `pkg/controllers/kusciajob/handler/running.go` | `RunningHandler.handleRunning()`：Running 阶段的入口，串联调度与创建 |
| `pkg/controllers/kusciajob/handler/initialized.go` | 首次处理：DAG 校验、TaskID 生成准备、状态初始化 |
| `pkg/controllers/kusciajob/handler/factory.go` | 状态机工厂，按 Job Phase 分发 Handler |
| `pkg/controllers/kusciajob/controller.go` | Controller 主循环、Informer 监听、事件入队 |
| `pkg/crd/apis/kuscia/v1alpha1/kusciajob_types.go` | KusciaJob / KusciaTaskTemplate 类型定义 |
| `pkg/crd/apis/kuscia/v1alpha1/kusciatask_types.go` | KusciaTask 类型定义 |

**对应函数接口：**

- `NewController()` - 创建 KusciaJob 控制器实例
- `Run()` - 启动控制器运行
- `syncHandler()` - 同步处理 KusciaJob 状态
- `enqueueKusciaJob()` - 将 KusciaJob 加入工作队列
- `handleTaskObject()` - 监听 KusciaTask 变化，反查所属 Job 并入队
- `kusciaJobDefault()` - 设置 KusciaJob 默认字段（初始 phase、maxParallelism 等）
- `failKusciaJob()` - 处理 KusciaJob 失败状态
- `handlerFactory.KusciaJobPhaseHandlerFor(phase).HandlePhase()` - 根据阶段处理 KusciaJob
- `kusciaJobValidate()` - DAG 合法性校验
- `kusciaJobHasTaskCycle()` - DAG 环检测
- `readyTasksOf()` - 计算就绪任务
- `willStartTasksOf()` - 根据并发度裁剪待创建任务
- `jobStatusPhaseFrom()` - 由任务状态推导 Job 状态
- `buildWillStartKusciaTask()` - 将任务模板构建为 KusciaTask CR

### 6.5 创建 TaskResourceGroup / Pod / Service / ConfigMap CR

KusciaTask Controller 负责将 KusciaTask 转换为实际的 K8s 资源，具体流程如下：

1. **TaskResourceGroup 创建**：
    - 为多方协同任务创建 TaskResourceGroup CR，用于跨域资源协调
    - 定义任务所需的资源规格和数量
    - 设置多方协调的参数和约束条件

2. **Pod 创建**：
    - 根据 Task 规约创建对应的 Pod CR
    - 配置容器镜像、启动命令、环境变量等
    - 设置资源限制（CPU、内存）和请求
    - 配置卷挂载和网络策略

3. **Service 创建**：
    - 为需要网络通信的任务创建 Service CR
    - 配置服务发现和负载均衡
    - 设置端口映射和选择器

4. **ConfigMap 创建**：
    - 为任务创建配置文件和参数传递的 ConfigMap CR
    - 包含运行时配置、算法参数等
    - 通过卷挂载或环境变量注入到 Pod

5. **资源关联**：建立 Task 与创建的各类资源之间的关联关系，便于状态跟踪和清理

**对应的函数接口：**

- `NewController()` - 创建KusciaTask控制器实例
- `Run()` - 启动控制器运行
- `syncHandler()` - 同步处理KusciaTask状态
- `enqueueKusciaTask()` - 将KusciaTask加入工作队列
- `handleTaskResourceGroupObject()` - 处理TaskResourceGroup对象
- `handlePodObject()` - 处理Pod对象
- `handleServiceObject()` - 处理Service对象
- `handleDeletedKusciaTask()` - 处理删除的KusciaTask
- `updateTaskStatus()` - 更新任务状态
- `failKusciaTask()` - 处理KusciaTask失败状态
- `handlerFactory.GetKusciaTaskPhaseHandler(phase).Handle()` - 根据阶段处理KusciaTask

### 6.6 资源预留协调

TaskResourceGroup Controller 负责协调多方任务的资源预留，确保所有参与方的资源同时可用：

1. **资源评估**：
    - 评估各方节点的可用资源（CPU、内存、GPU等）
    - 检查资源配额和限制
    - 验证节点标签和污点容忍度

2. **预留请求**：
    - 向各参与方发送资源预留请求
    - 暂时锁定资源以防其他任务占用
    - 记录预留状态和时间戳

3. **协调机制**：
    - 等待所有参与方确认资源预留
    - 如果任一参与方无法预留资源，则取消所有预留
    - 实现超时机制防止无限等待

4. **状态同步**：
    - 同步各方预留状态到 TaskResourceGroup CR
    - 更新协调进度和异常信息
    - 提供调试和监控信息

5. **回滚机制**：
    - 当资源预留失败时，自动释放已预留的资源
    - 更新任务状态为失败并记录原因
    - 支持重试机制

**对应的函数接口：**

- `NewController()` - 创建TaskResourceGroup控制器实例
- `Run()` - 启动控制器运行
- `syncHandler()` - 同步处理TaskResourceGroup状态
- `handleAddedTaskResourceGroup()` - 处理新增的TaskResourceGroup
- `handleUpdatedTaskResourceGroup()` - 处理更新的TaskResourceGroup
- `handleAddedOrDeletedTaskResource()` - 处理新增或删除的TaskResource
- `handleUpdatedTaskResource()` - 处理更新的TaskResource
- `handleAddedPod()` - 处理新增的Pod
- `resourceFilter()` - 资源过滤函数
- `matchLabels()` - 匹配标签筛选资源
- `updateTaskResourceGroupStatus()` - 更新TaskResourceGroup状态
- `needHandleExpiredTrg()` - 检查TaskResourceGroup是否过期
- `needHandleReserveFailedTrg()` - 检查预留失败的TaskResourceGroup

### 6.7 绑定 Pod 到 Node

Kuscia Scheduler 是一个定制的调度器，负责将 Pod 绑定到合适的节点，特别针对多方协同任务进行了优化：

1. **扩展调度策略**：
    - 实现 All-or-Nothing 调度策略，确保多方协同任务要么全部调度成功，要么全部失败
    - 考虑跨节点的资源协调需求
    - 支持 Pod 亲和性和反亲和性规则

2. **资源检查**：
    - 验证目标节点是否有足够的资源（CPU、内存、存储）
    - 检查节点标签是否匹配 Pod 的节点选择器要求
    - 验证节点是否容忍 Pod 的污点设置

3. **多方协调**：
    - 在 P2P 模式下协调多个集群的调度决策
    - 确保协同任务在约定时间内调度到对应节点
    - 处理跨集群的资源冲突

4. **绑定执行**：
    - 创建 Binding 对象将 Pod 绑定到特定节点
    - 更新 Pod 状态为调度中
    - 通知 Kubelet 准备启动 Pod

5. **失败处理**：
    - 如果绑定失败，释放已预留的资源
    - 尝试重新调度或更新任务状态
    - 记录调度失败的原因

**对应的函数接口：**

- `New()` - 创建KusciaScheduling调度插件实例
- `Name()` - 返回调度插件名称
- `PreFilter()` - 预过滤插件函数
- `PostFilter()` - 后过滤插件函数
- `Reserve()` - 预留插件函数
- `Unreserve()` - 取消预留插件函数
- `Permit()` - 许可插件函数
- `PreBind()` - 预绑定插件函数
- `PostBind()` - 后绑定插件函数
- `EventsToRegister()` - 注册事件函数
- `trMgr.PreFilter()` - TaskResource管理器预过滤函数
- `trMgr.Permit()` - TaskResource管理器许可函数
- `trMgr.PreBind()` - TaskResource管理器预绑定函数
- `trMgr.PostBind()` - TaskResource管理器后绑定函数

### 6.8 Agent 基本功能

- 节点注册：向控制平面注册节点信息
- 容器生命周期管理：负责 Pod 的创建、启动、停止和删除
- 镜像管理：处理镜像拉取和管理
- 资源监控：监控节点和容器的资源使用情况
- 状态同步：向控制平面同步节点和 Pod 状态

#### 6.8.1 Pod 生命周期管理

- 创建阶段：接收来自控制平面的 Pod 创建请求，验证 Pod 规约
- 镜像拉取阶段：根据 Pod 中定义的镜像，执行镜像拉取操作
- 容器启动阶段：启动 Pod 中定义的所有容器
- 运行阶段：监控 Pod 状态，处理健康检查
- 终止阶段：处理 Pod 的优雅终止和清理工作

#### 6.8.2 镜像拉取机制

- 镜像预拉取：根据节点上的 Pod 需求预先拉取镜像
- 缓存管理：维护本地镜像缓存，避免重复拉取
- 并发控制：控制并发拉取的数量，避免对镜像仓库造成过大压力
- 认证处理：支持私有镜像仓库的认证凭据管理
- 失败重试：对拉取失败的镜像实施重试机制

#### 6.8.3 容器启动流程

- 资源准备：为容器准备必要的资源（如存储卷、网络配置等）
- 环境设置：设置容器的运行环境变量、安全上下文等
- 容器创建：调用底层运行时（RunC、RunP 或其他）创建容器
- 启动执行：启动容器内的进程
- 健康检查：启动后持续监控容器健康状态
- 状态报告：向控制平面报告容器启动状态

#### 6.8.4 运行时模式

Kuscia Agent 支持多种运行时模式：

- RunC：原生容器模式，提供更好的资源隔离
- RunP：进程运行时模式，在同一容器内以进程方式运行任务
- RunK：对接外部 K8s 集群模式

#### 6.8.5 Agent 在整体架构中的作用

- 控制平面与数据平面的桥梁：接收来自控制平面的调度指令，并在节点上执行
- 资源隔离：确保不同任务之间的资源隔离，防止相互干扰
- 安全边界：作为安全边界，控制节点上的任务执行
- 状态反馈：实时向控制平面反馈节点和任务状态
- 故障恢复：处理节点和任务级别的故障恢复

**对应的函数接口：**

- `NewPodsController()` - 创建Pods控制器实例
- `Run()` - 启动Pods控制器运行
- `HandlePodAdditions()` - 处理Pod添加事件
- `HandlePodUpdates()` - 处理Pod更新事件
- `HandlePodRemoves()` - 处理Pod移除事件
- `HandlePodReconcile()` - 处理Pod协调事件
- `HandlePodSyncs()` - 处理Pod同步事件
- `syncPod()` - 同步Pod状态
- `syncTerminatingPod()` - 同步终止中的Pod
- `syncTerminatedPod()` - 同步已终止的Pod
- `dispatchWork()` - 分发Pod工作任务
- `deletePod()` - 删除Pod
- `constructPodImage()` - 构造Pod镜像
- `getPodStatus()` - 获取Pod状态
- `RegisterProvider()` - 注册Pod提供者

### 6.9 通过 DataMesh 读取/写入数据

DataMesh 作为统一的数据访问层，为隐私计算引擎提供标准化的数据读写接口：

1. **数据抽象**：
    - 将不同来源的数据（本地文件、数据库、对象存储等）抽象为统一的访问接口
    - 提供元数据管理和数据目录服务
    - 支持多种数据格式（CSV、Parquet、JSON等）

2. **数据访问协议**：
    - HTTP 协议：通过 8070 端口提供 RESTful API
    - gRPC/Arrow Flight 协议：通过 8071 端口提供高性能的数据访问
    - 支持批量读取和流式读取

3. **安全访问**：
    - 实现细粒度的访问控制和权限验证
    - 支持数据脱敏和采样
    - 提供审计日志记录数据访问行为

4. **跨域数据交换**：
    - 在符合隐私保护要求的前提下，支持安全的数据交换
    - 实现数据水印和溯源机制
    - 提供数据完整性校验

5. **性能优化**：
    - 数据缓存机制减少重复读取开销
    - 支持数据预处理和索引加速
    - 提供连接池管理优化资源利用

**对应的函数接口：**

- `NewHTTPServerBean()` - 创建DataMesh HTTP服务器实例
- `NewGrpcServerBean()` - 创建DataMesh GRPC服务器实例
- `Start()` - 启动服务器
- `RegisterGroup()` - 注册路由组
- `protoDecorator()` - 协议装饰器
- `NewDomainDataHandler()` - 创建DomainData处理器
- `NewDomainDataSourceHandler()` - 创建DomainDataSource处理器
- `NewDomainDataGrantHandler()` - 创建DomainDataGrant处理器
- `NewDataMeshFlightHandler()` - 创建DataMesh Flight处理器
- `RegisterDomainDataServiceServer()` - 注册DomainData服务
- `RegisterDomainDataSourceServiceServer()` - 注册DomainDataSource服务
- `RegisterDomainDataGrantServiceServer()` - 注册DomainDataGrant服务
- `flight.RegisterFlightServiceServer()` - 注册Flight服务

---

## 7. 网络与安全

### 7.1 NetworkMesh 组成

NetworkMesh 是算法容器之间进行网络通信的基础设施，包含：

- **CoreDNS**：域内 Service 域名解析。
- **DomainRoute**：节点间路由规则与认证策略。
- **Envoy**：节点侧/控制平面侧流量代理。
- **Transport**：消息队列模式传输组件。

### 7.2 跨域通信路径

```text
Pod A
  │
  ▼
Envoy (节点 A)
  │
  ▼
DomainRoute / ClusterDomainRoute
  │
  ▼
Envoy (节点 B / Master)
  │
  ▼
Pod B / K3s ApiServer
```

### 7.3 身份认证与鉴权

**DomainRoute 授权：**

所有跨域通信必须通过 `DomainRoute`（P2P）或 `ClusterDomainRoute`（中心化）定义源、目标、认证方式。

**认证方式：**

| 方式 | 说明 |
| ------ | ------ |
| **Token** | 基于 RSA 协商 Token（`RSA-GEN`、`UID-RSA-GEN`），支持滚动更新 |
| **mTLS** | 双向 TLS，源节点配置客户端证书/私钥，目标节点校验 |
| **None** | 不认证，仅用于测试，不推荐生产使用 |

**Domain 证书：**

每个 Domain 拥有 RSA 密钥对，公钥写入 Domain/Config，用于 Token 协商与证书签发。

### 7.4 Kuscia Gateway（Envoy）

Kuscia Gateway 是基于 Envoy 实现的网络流量代理与入口组件，负责节点内外、跨域之间的安全流量转发。它既承担**外部公网入口**职责，也承担**域内 Service 代理**职责，是 NetworkMesh 的数据平面核心。

#### 7.4.1 Envoy 在 Kuscia 中的定位

| 角色 | 说明 |
| ------ | ------ |
| **边缘代理** | 监听外部端口（默认 `1080`），接收来自其他 Kuscia 节点或外部系统的请求 |
| **内部代理** | 监听内部端口（默认 `80`），为域内 Pod 提供 Service 级别的代理与发现 |
| **跨域安全通道** | 通过 mTLS/Token 认证、Body 加密等机制保证跨域通信安全 |
| **协议适配** | 支持 Kuscia 协议与 BFIA 互联互通协议，并处理 gRPC/HTTP 转换 |
| **统一入口** | 多任务共享同一公网端口，避免每个任务单独暴露端口 |

#### 7.4.2 核心端口与配置

Envoy 默认监听端口定义在 `pkg/gateway/config/gateway_config.go`：

```go
func DefaultStaticGatewayConfig() *GatewayConfig {
    g := &GatewayConfig{
        DomainID:      "default",
        ConfBasedir:   "./conf",
        WhiteListFile: "",
        ExternalPort:   1080,   // 外部入口端口
        HandshakePort:  1054,   // Token 握手服务端口
        XDSPort:        10001,  // xDS 管理接口（Envoy 从此端口拉取动态配置）
        EnvoyAdminPort: 10000,  // Envoy admin 接口（/ready、/stats、/config_dump）
        IdleTimeout:    60,
        ResyncPeriod:   600,
        MasterConfig:   &kusciaconfig.MasterConfig{},
    }
    return g
}
```

| 端口 | 监听地址 | 说明 |
| ------ | --------- | ------ |
| `1080` | `0.0.0.0` | 外部 Listener，接收跨域/外部流量 |
| `80` | `0.0.0.0` | 内部 Listener，接收域内 Pod 流量 |
| `443` | `0.0.0.0` | 内部 TLS Listener（当配置 InnerServerTLS 时动态生成） |
| `10000` | `127.0.0.1` | Envoy admin 接口，健康检查使用 `http://127.0.0.1:10000/ready` |
| `10001` | `127.0.0.1` | Kuscia xDS 服务端口，Envoy 通过 gRPC 拉取 Listener/Route/Cluster |
| `1054` | `127.0.0.1` | DomainRoute Token 握手 HTTP 服务 |

#### 7.4.3 启动流程

Envoy 模块在 `cmd/kuscia/modules/envoy.go` 中启动：

```go
func (s *envoyModule) Run(ctx context.Context) error {
    // 1. 准备日志目录
    os.MkdirAll(filepath.Join(s.rootDir, common.LogPrefix, "envoy/"), 0750)

    // 2. 读取 command-line.yaml 中的额外启动参数
    deltaArgs, err := s.readCommandArgs()

    // 3. 组装 Envoy 启动参数
    args := []string{
        "-c", filepath.Join(s.rootDir, common.ConfPrefix, "envoy/envoy.yaml"),
        "--service-cluster", s.cluster,      // kuscia-gateway-<domainID>
        "--service-node",    s.id,           // kuscia-gateway-<domainID>-<hostname>
        "--log-path",        filepath.Join(s.rootDir, common.LogPrefix, "envoy/envoy.log"),
    }
    args = append(args, deltaArgs.Args...)

    // 4. 通过 supervisor 启动 Envoy 子进程并守护
    sp := supervisor.NewSupervisor("envoy", nil, -1)
    return sp.Run(ctx, func(ctx context.Context) supervisor.Cmd {
        cmd := exec.Command(filepath.Join(s.rootDir, "bin/envoy"), args...)
        return &ModuleCMD{cmd: cmd, score: &envoyOOMScore}
    })
}
```

启动 readiness 探测：

```go
rdz: readyz.NewHTTPReadyZ("http://127.0.0.1:10000/ready", 200, func(body []byte) error {
    res := string(body[:len(body)-1])
    if res != "LIVE" {
        return errors.New("response is not live")
    }
    return nil
})
```

#### 7.4.4 xDS 动态配置

Kuscia 没有让 Envoy 直接读取复杂的静态 YAML，而是实现了自己的 **xDS 控制平面**：

- **代码**：`pkg/gateway/xds/xds.go`
- **协议**：Envoy Discovery Service v3
- **通信方式**：Envoy 作为客户端，通过 gRPC 连接到 `127.0.0.1:10001` 的 xDS Server

xDS Server 在 Envoy 启动后由 Gateway 命令初始化：

```go
func StartXds(gwConfig *config.GatewayConfig) error {
    xds.IdleTimeout = gwConfig.IdleTimeout
    xds.NewXdsServer(gwConfig.XDSPort, gwConfig.GetEnvoyNodeID())

    externalCert, _ := config.LoadTLSCertByTLSConfig(gwConfig.ExternalTLS)
    internalCert, _ := config.LoadTLSCertByTLSConfig(gwConfig.InnerServerTLS)

    xdsConfig := &xds.InitConfig{
        Basedir:      gwConfig.ConfBasedir,
        XDSPort:      gwConfig.XDSPort,
        ExternalPort: gwConfig.ExternalPort,
        ExternalCert: externalCert,
        InternalCert: internalCert,
        Logdir:       filepath.Join(gwConfig.RootDir, "var/logs/envoy/"),
    }
    xds.InitSnapshot(gwConfig.DomainID, utils.GetHostname(), xdsConfig)
    return nil
}
```

初始化时会生成第一份 Snapshot，包含：

| 资源类型 | 来源 | 说明 |
| ---------- | ------ | ------ |
| **Listeners** | `etc/conf/domainroute/listeners/*.json` + `*.json.tmpl` | 外部/内部 Listener 模板 |
| **Routes** | `etc/conf/domainroute/routes/*.json` + `*.json.tmpl` | 外部/内部 Route 配置 |
| **Clusters** | `etc/conf/domainroute/clusters/*.json` | 静态 Cluster，如 `xds-cluster`、`handshake-cluster`、`internal-cluster` |

后续动态资源（DomainRoute 对应的 Cluster/VirtualHost、Service 对应的 Cluster）由 Gateway Controller 通过 xDS API 实时增删改。

#### 7.4.5 Listener：外部监听与内部监听

Envoy 启动后至少有两个核心 Listener：

**1. external-listener（端口 1080）**

配置文件模板：`etc/conf/domainroute/listeners/external_listeners.json.tmpl`

```json
{
    "name": "external-listener",
    "address": {
        "socket_address": {
            "address": "0.0.0.0",
            "port_value": {{.ExternalPort}}
        }
    },
    "filter_chains": [{
        "filters": [{
            "name": "envoy.filters.network.http_connection_manager",
            "typed_config": {
                "server_name": "kuscia-gateway",
                "stat_prefix": "external_http",
                "rds": { "route_config_name": "external-route" },
                "http_filters": [
                    { "name": "envoy.filters.http.grpc_http1_bridge" },
                    { "name": "envoy.filters.http.kuscia_gress" },
                    { "name": "envoy.filters.http.kuscia_token_auth" },
                    { "name": "envoy.filters.http.router" }
                ]
            }
        }]
    }]
}
```

**2. internal-listener（端口 80）**

配置文件模板：`etc/conf/domainroute/listeners/internal_listeners.json.tmpl`

```json
{
    "name": "internal-listener",
    "address": {
        "socket_address": {
            "address": "0.0.0.0",
            "port_value": 80
        }
    },
    "filter_chains": [{
        "filters": [{
            "name": "envoy.filters.network.http_connection_manager",
            "typed_config": {
                "stat_prefix": "internal_http",
                "rds": { "route_config_name": "internal-route" },
                "http_filters": [
                    { "name": "envoy.filters.http.grpc_http1_reverse_bridge" },
                    { "name": "envoy.filters.http.kuscia_gress" },
                    { "name": "envoy.filters.http.router" }
                ],
                "http_protocol_options": { "accept_http_10": true },
                "http2_protocol_options": { "allow_connect": true }
            }
        }]
    }]
}
```

如果配置了 `InnerServerTLS`，xDS 会额外克隆生成一个 `internal-listener-tls`，监听 `443` 端口。

#### 7.4.6 路由与 VirtualHost

**外部路由（external-route）**

默认 VirtualHost：

- `kuscia-handshake.{{.Namespace}}.svc` → `handshake-cluster`（Token 握手，禁用 token auth）
- `*`（默认）→ gRPC 流量走 `internal-cluster-grpc`，其他走 `internal-cluster`

外部请求进入 Envoy 后，先经过 `kuscia_token_auth` 插件校验来源身份，再根据 `Kuscia-Host` 头路由到内部服务。

**内部路由（internal-route）**

默认 VirtualHost：

- `kuscia-handshake.{{.Namespace}}.svc` → `handshake-cluster`
- `/zipkin` → `central-gateway`（链路追踪）
- 未知 gRPC 服务返回 404 + `grpc-status: 14`
- 未知 HTTP 服务返回 404 + `Kuscia-Error-Message-Internal`

DomainRoute Controller 会为每个跨域路由动态添加 VirtualHost，例如：

```text
名称: alice-to-bob
域名: *.bob.svc
路由: / → cluster alice-to-bob-<port>
```

#### 7.4.7 Cluster 与 Endpoints 同步

**静态 Cluster**

`etc/conf/envoy/envoy.yaml` 中预定义：

```yaml
static_resources:
  clusters:
    - name: xds-cluster
      load_assignment:
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 10001 }
    - name: handshake-cluster
      load_assignment:
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 1054 }
    - name: internal-cluster
      load_assignment:
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 80 }
    - name: internal-cluster-grpc
      load_assignment:
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 80 }
```

**动态 Cluster**

- **EndpointsController**（`pkg/gateway/controller/endpoints.go`）监听 K8s `Service`（类型为 `ExternalName`）和 `Endpoints`，将变化同步为 Envoy Cluster。这样 Pod 创建的 K8s Service 可以被 Envoy 识别并代理。
- **GatewayController**（`pkg/gateway/controller/gateway.go`）维护同域 Gateway 列表，动态更新 `kuscia-gateway` Cluster 的 Endpoint，用于反向隧道（Reverse Tunnel）场景下流量回注。
- **DomainRouteController** 为每个跨域 DomainRoute 创建对应的 upstream Cluster，例如 `alice-to-bob-http`、`alice-to-bob-grpc`。

#### 7.4.8 自定义 Envoy 插件

Kuscia 使用了一个定制版 Envoy（`kuscia-envoy`），包含多个自定义 HTTP Filter：

| 插件名 | 作用 | 所在 Listener |
| -------- | ------ | -------------- |
| **kuscia_gress** | 改写 Host 头、解析 `Kuscia-host`/`x-ptp-target-node-id`/`x-source-node-id`、注入来源信息 | external/internal |
| **kuscia_token_auth** | 基于 DomainRoute Token 校验请求来源身份 | external |
| **kuscia_header_decorator** | 为外部请求添加自定义 Header | external |
| **kuscia_crypt** | 请求/响应 Body AES 加密解密 | external/internal |
| **kuscia_poller** | 反向隧道场景下轮询上游数据 | external |
| **kuscia_receiver** | 反向隧道场景下接收并转发轮询数据 | external/internal |

#### 7.4.9 跨域通信流程

以 Alice 向 Bob 发起请求为例：

```text
Pod (alice)
   │ 请求 *.bob.svc:80
   ▼
internal-listener:80 (alice Envoy)
   │ 匹配 VirtualHost alice-to-bob
   ▼
Cluster alice-to-bob-<port>
   │ mTLS/Token 认证
   ▼
external-listener:1080 (bob Envoy)
   │ kuscia_token_auth 校验
   ▼
internal-listener:80 (bob Envoy)
   │ 匹配 *.bob.svc
   ▼
目标 Pod / Service (bob)
```

关键 Header：

| Header | 说明 |
| -------- | ------ |
| `Kuscia-Source` | 来源 Domain ID |
| `Kuscia-Host` | 原始目标 Host，用于内部路由重写 |
| `Kuscia-Token` | DomainRoute 协商的 Token（Kuscia 协议） |
| `x-interconn-protocol` | 互联互通协议类型：`kuscia` 或 `bfia` |

#### 7.4.10 Gateway 模块与其他模块的关系

```text
                ┌─────────────┐
                │   Kuscia    │
                │  主进程      │
                └──────┬──────┘
                       │ 启动
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
   bin/envoy      xDS Server      Gateway Controllers
   (数据平面)      (127.0.0.1:10001)  (控制逻辑)
        │              │              │
        │              │ 拉取配置      │ 更新配置
        └──────────────┴──────────────┘
                       │
                       ▼
              Listener/Route/Cluster
                       │
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
   DomainRoute    Service/Endpoints  Master/Lite 节点
   (跨域路由)       (域内服务发现)      (多副本 Gateway)
```

#### 7.4.11 调试与诊断

常用调试入口：

```bash
# 查看 Envoy 是否就绪
curl http://127.0.0.1:10000/ready

# 查看当前配置
curl http://127.0.0.1:10000/config_dump

# 查看统计指标
curl http://127.0.0.1:10000/stats/prometheus

# 查看 Gateway CR
kubectl get gateway -n <domainID>
kubectl describe gateway <hostname> -n <domainID>

# 查看 DomainRoute 生成的 Envoy 规则
kubectl get domainroute -n <domainID>
```

日志路径：

```text
/var/logs/envoy/
├── envoy.log              # Envoy 主日志
├── envoy_admin.log        # Admin 访问日志
├── external.log           # 外部流量访问日志
├── internal.log           # 内部流量访问日志
├── kubernetes.log         # K8s API 代理日志
├── prometheus.log         # 监控代理日志
└── zipkin.log             # 链路追踪日志
```

---

### 7.5 网络安全能力

- **MTLS/HTTPS**：跨域流量默认加密。
- **端口复用**：多任务共享一个公网端口。
- **HTTP 转发**：适配只支持七层转发的机构网关。
- **路由转发（Transit）**：
  - `THIRD-DOMAIN`：经第三方节点一跳/多跳转发。
  - `REVERSE-TUNNEL`：反向隧道，解决一方无法监听端口的问题。
- **Body 加密**：转发场景下可启用 AES 加密。
- **IP 白名单**：`sourceWhiteIPList` 限制源 IP。

---

## 8. Kuscia 数据存储与 DataMesh

Kuscia 不仅要管理任务的调度和执行，还需要管理隐私计算过程中涉及的各类数据（样本表、模型、规则、报告等）。为了做到数据**可用不可见**，Kuscia 将**数据元信息**与**实际数据**分离：元信息通过 K3s CRD 管理，实际数据通过 **DataMesh** 统一访问。

### 8.1 Kuscia 数据存储概述

Kuscia 中的数据存储可以分为两个层次：

```text
┌─────────────────────────────────────────────────────────────────────┐
│                        元数据层（Metadata）                          │
│  存储在 K3s etcd 中，通过 CRD 管理                                   │
│  DomainData / DomainDataSource / DomainDataGrant                    │
└──────────────────────────────┬──────────────────────────────────────┘
                               │ DataMesh 解析与授权
                               ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        数据平面（Data Plane）                        │
│  实际数据存储位置                                                    │
│  本地文件系统 / OSS / MySQL / PostgreSQL / 外部 DataProxy            │
└─────────────────────────────────────────────────────────────────────┘
```

**核心设计原则**：

- **元数据与实际数据分离**：`DomainData` 只描述数据在哪里、是什么格式、有哪些列，不保存实际内容。
- **按域隔离**：每个 Domain 的元数据存放在各自的 K3s Namespace 中，实际数据也按域划分。
- **统一访问入口**：无论实际数据存在哪里，引擎都通过 DataMesh 的 Arrow Flight 接口读写。

### 8.2 核心数据对象

Kuscia 定义了三种与数据相关的 CRD：

#### 8.2.1 DomainDataSource（数据源）

`DomainDataSource` 定义一个域可以使用哪些存储后端，例如本地文件系统、OSS、MySQL 等。它相当于一份**连接配置**，告诉 DataMesh：数据存在哪里、用什么协议访问、访问凭据是什么。

**本地文件系统数据源示例**：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataSource
metadata:
  name: default
  namespace: alice
spec:
  type: localfs
  info:
    localfs:
      path: /var/lib/kuscia/var/storage/data
```

**OSS 数据源示例**：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataSource
metadata:
  name: oss-datasource
  namespace: alice
spec:
  type: oss
  info:
    oss:
      endpoint: oss-cn-hangzhou.aliyuncs.com
      bucket: kuscia-data
      prefix: alice/
      access_key_id: "..."
      access_key_secret: "..."
```

**MySQL 数据源示例**：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataSource
metadata:
  name: mysql-datasource
  namespace: alice
spec:
  type: mysql
  info:
    mysql:
      host: mysql.default.svc
      port: 3306
      database: alice_db
      user: alice
      password: "..."
```

DataMesh 启动时会自动为当前域注册一个默认的 `localfs` 数据源，名称为 `default`。

#### 8.2.2 DomainData（数据对象）

`DomainData` 是对一份具体数据的元信息描述，可以理解为“数据目录中的一条记录”。

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: user-table
  namespace: alice
spec:
  name: user-table
  type: table
  relative_uri: user-table.csv
  data_source: default
  author: alice
  file_format: csv
  columns:
    - name: id
      type: str
    - name: age
      type: int
```

关键字段含义：

| 字段 | 含义 |
| ------ | ------ |
| `name` | 数据对象名称 |
| `type` | 数据类型：`table` / `model` / `rule` / `report` |
| `relative_uri` | 相对于数据源的存储路径或对象键 |
| `data_source` | 引用的 `DomainDataSource` 名称 |
| `author` | 数据所有者（Domain ID） |
| `columns` | 表结构的列信息 |

**`relative_uri` 需要指定 DataMesh 路径吗？**  
**不需要**。`relative_uri` 是相对于 `DomainDataSource` 的**相对路径**，DataMesh 会自动将其与数据源中的基路径拼接，得到实际访问位置。

| 数据源类型 | `DomainDataSource` 基路径 | `relative_uri` | DataMesh 解析后的实际位置 |
| ------------ | --------------------------- | ---------------- | --------------------------- |
| localfs | `/var/lib/kuscia/var/storage/data` | `user-table.csv` | `/var/lib/kuscia/var/storage/data/user-table.csv` |
| oss | `bucket=kuscia-data`, `prefix=alice/` | `user-table.csv` | `oss://kuscia-data/alice/user-table.csv` |
| mysql | `database=alice_db` | `user_table` | `SELECT * FROM alice_db.user_table` |

> 上表中的 `relative_uri` 都不再包含域标识。如果为了文件系统层面按域隔离，可以在 `relative_uri` 前加上域目录（如 `alice/user-table.csv`），或者通过 OSS `prefix`、MySQL `database` 等方式在数据源层隔离。

> `DomainData` 本身不存储数据内容，它只告诉 DataMesh 去哪里、以什么方式读取数据。

#### 8.2.3 DomainDataGrant（跨域授权）

当 alice 想让 bob 使用自己的数据时，需要创建 `DomainDataGrant`：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataGrant
metadata:
  name: grant-user-table-to-bob
  namespace: alice
spec:
  author: alice
  domain_data_id: user-table
  grant_domain: bob
  signature: "..."
  limit:
    expiration_time: "2025-12-31T23:59:59Z"
    use_count: 10
```

Kuscia 控制器会：

1. 验证授权签名（使用 alice 的 Domain 证书）。
2. 检查有效期 `expiration_time` 和使用次数 `use_count`。
3. 将 `DomainDataGrant` 和对应的 `DomainData` 拷贝到 `bob` 命名空间。
4. bob 侧的任务就可以引用这份数据。

#### 8.2.4 DomainDataSource 与 DataMesh 的关系

`DomainDataSource` 和 `DataMesh` 是**配置与执行**的关系，可以类比为：

- `DomainDataSource` = 数据库连接串 / 文件系统挂载点配置
- `DataMesh` = 数据库连接池 / 文件访问服务

**具体关系如下**：

```text
DomainData "user-table"
    │
    │ spec.data_source = "default"
    ▼
DomainDataSource "default" (type=localfs, path=/var/lib/kuscia/var/storage/data)
    │
    │ DataMesh 读取数据源类型和连接信息
    ▼
DataMesh IO Channel 选择器
    │
    ├─ type=localfs → BuiltinLocalFileIOChannel
    ├─ type=oss     → BuiltinOssIOChannel
    ├─ type=mysql   → BuiltinMySQLIOChannel
    └─ type=external→ ExternalIOChannel
    │
    ▼
实际 IO 操作（Open / GetObject / Query）
```

**DataMesh 使用 DomainDataSource 的过程**：

1. **启动注册**：DataMesh 的 Operator Bean 启动时，会在当前域命名空间下创建一个默认的 `DomainDataSource`，名为 `default`，类型为 `localfs`。

2. **类型路由**：当引擎请求访问某个 `DomainData` 时，DataMesh 先读取 `DomainData.data_source` 字段，再查询同名的 `DomainDataSource`，根据 `spec.type` 选择对应的 IO Channel。

3. **凭据解密**：对于 OSS、MySQL 等需要认证的后端，`DomainDataSource` 中的密码、Secret 等敏感信息是加密的。DataMesh 在访问前会调用解密逻辑：

   ```go
   encryptedInfo := kusciaDomainDataSource.Spec.Data["encryptedInfo"]
   info, err := s.decryptInfo(encryptedInfo)
   ```

4. **路径拼接**：DataMesh 将 `DomainDataSource` 中的基路径与 `DomainData.relative_uri` 拼接，得到最终访问位置。例如：

   ```go
   filePath := path.Join(ds.Info.Localfs.Path, data.RelativeUri)
   ```

5. **多数据源支持**：一个域可以同时存在多个 `DomainDataSource`，例如一个默认 `localfs` 用于临时数据，一个 `oss` 用于大规模训练数据。不同的 `DomainData` 通过 `data_source` 字段选择不同的数据源。

**为什么要把数据源配置独立出来？**

- **解耦**：`DomainData` 只描述“是什么数据”，不关心“存在哪里”；`DomainDataSource` 只描述“怎么访问存储”。两者可以独立更新。
- **复用**：多个 `DomainData` 可以引用同一个 `DomainDataSource`，避免重复配置。
- **安全**：访问凭据集中在 `DomainDataSource` 中加密存储，而不是散落在每个 `DomainData` 里。
- **可扩展**：新增存储后端时，只需要在 DataMesh 中新增 IO Channel，用户通过新的 `DomainDataSource` 类型即可使用。

### 8.3 DataMesh 架构

DataMesh 是 Kuscia 中负责数据管理与访问的独立模块，通常以 gRPC/HTTP 服务形式运行在每个 Kuscia 节点（Autonomy / Lite）上。

#### 8.3.1 DataMesh 如何启动

DataMesh 作为 Kuscia 的一个内部模块启动，入口在 `cmd/kuscia/modules/datamesh.go`：

```go
func (m *dataMeshModule) Run(ctx context.Context) error {
    return commands.Run(ctx, m.conf, m.kusciaClient)
}
```

`pkg/datamesh/commands/root.go` 负责启动三个核心服务 Bean：

```go
func Run(ctx context.Context, conf *config.DataMeshConfig, kusciaClient ...) error {
    httpServer := bean.NewHTTPServerBean(conf, cmConfigService)
    grpcServer := bean.NewGrpcServerBean(conf, cmConfigService)
    opServer   := bean.NewOperatorBean(conf, cmConfigService)
    // ... 启动并等待
}
```

| Bean | 文件 | 作用 |
| ------ | ------ | ------ |
| HTTP Server | `pkg/datamesh/bean/http_server_bean.go` | 启动 HTTP 元数据服务 |
| gRPC Server | `pkg/datamesh/bean/grpc_server_bean.go` | 启动 gRPC 元数据服务 + Arrow Flight 服务 |
| Operator | `pkg/datamesh/bean/operator_bean.go` | 启动时注册默认 `localfs` 数据源 |

启动参数通常来自 Kuscia 主配置，关键配置项包括：

```go
type DataMeshConfig struct {
    KubeNamespace string   // 当前域对应的 K3s Namespace，如 "alice"
    RootDir       string   // Kuscia 根目录，如 /var/lib/kuscia
    KusciaClient  kusciaclientset.Interface
    // HTTP/gRPC 监听地址、端口、TLS 配置等
}
```

DataMesh 启动时会：

1. 创建 `DomainDataSourceService`、`DomainDataService`、`DomainDataGrantService` 等元数据服务。
2. 创建 `FlightIO` 数据平面服务，并注册内置 IO Channel（localfs / oss / mysql / postgresql）。
3. 启动 HTTP 和 gRPC 服务器。
4. **Operator Bean 在当前域 Namespace 下创建默认 `localfs` 数据源**（如果尚不存在）。

> DataMesh 启动后**不会主动扫描或预热所有 DomainData**，它采用**按需访问**模式：只有当引擎或客户端请求读取/写入某个数据对象时，才会去 K3s 中查询对应的 CRD。

#### 8.3.2 创建 DomainData / DomainDataSource 时，DataMesh 会自动介入吗？

**简短回答**：

- 创建 `DomainData` 和 `DomainDataSource` 时，**不需要手动调用 DataMesh 接口进行注册或关联**。
- 两者都是标准的 Kuscia CRD，直接通过 kubectl、KusciaAPI 或 Go Client 创建到 K3s 中即可。
- DataMesh 在收到数据访问请求时，会**自动根据 `DomainData.data_source` 字段**去查找同 Namespace 下的 `DomainDataSource`。

**详细说明**：

创建 DomainDataSource（以 localfs 为例）：

```bash
kubectl --kubeconfig /var/lib/kuscia/etc/kubeconfig apply -f - <<EOF
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataSource
metadata:
  name: default
  namespace: alice
spec:
  type: localfs
  info:
    localfs:
      path: /var/lib/kuscia/var/storage/data
EOF
```

创建 DomainData：

```bash
kubectl --kubeconfig /var/lib/kuscia/etc/kubeconfig apply -f - <<EOF
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: user-table
  namespace: alice
spec:
  name: user-table
  type: table
  relative_uri: user-table.csv
  data_source: default    # ← 这里关联 DomainDataSource 名称
  author: alice
EOF
```

**关联关系完全由 `DomainData.spec.data_source` 字段决定**：

```text
DomainData "user-table" (namespace: alice)
    │
    │ spec.data_source = "default"
    ▼
DomainDataSource "default" (namespace: alice)
    │
    │ DataMesh 收到读请求时自动查找并解析
    ▼
实际数据位置
```

DataMesh 在收到 Flight 读请求时的内部逻辑大致如下：

```go
func (dp *FlightIO) GetFlightInfo(ctx context.Context, msg proto.Message) (*flight.FlightInfo, error) {
    // 1. 解析请求，获取 domaindata_id
    reqCtx, err := utils.NewDataMeshRequestContext(dp.dd, dp.ds, msg)

    // 2. 从 K3s 查询 DomainData
    domainData, err := reqCtx.GetDomainData(ctx)

    // 3. 根据 DomainData.data_source 查询 DomainDataSource
    datasourceReq := &datamesh.QueryDomainDataSourceRequest{
        DatasourceId: domainData.DatasourceId,
    }
    datasourceResp := dp.domainDataSourceService.QueryDomainDataSource(ctx, datasourceReq)

    // 4. 根据数据源类型选择 IO Channel 并返回 FlightInfo
    ioChannel := dp.ioMap[reqCtx.DataSourceType]
    return ioChannel.GetFlightInfo(ctx, reqCtx)
}
```

**需要注意的边界情况**：

| 场景 | DataMesh 行为 |
| ------ | --------------- |
| `DomainData` 引用了不存在的 `DomainDataSource` | Flight 请求会失败，返回数据源不存在错误 |
| `DomainDataSource` 存在但凭据解密失败 | 请求失败，通常是证书或加密配置问题 |
| `DomainData` 被删除 | 已缓存的 Flight ticket 在过期前仍可能可用，新请求会返回 NotFound |
| 更新了 `DomainDataSource` 的密码/路径 | 新请求立即生效；已发放的 ticket 仍使用旧上下文 |

> 因此，**DataMesh 与 CRD 的关联是声明式、按需的**：你只需正确填写 `data_source` 字段，DataMesh 会在访问时自动完成解析。

#### 8.3.3 DataMesh 控制平面与数据平面

```text
┌─────────────────────────────────────────────────────────────────────┐
│                           DataMesh                                  │
├─────────────────────────────┬───────────────────────────────────────┤
│      控制平面（Control Plane）│         数据平面（Data Plane）        │
├─────────────────────────────┼───────────────────────────────────────┤
│  HTTP Server                │  Arrow Flight Service                 │
│  gRPC Server                │    GetFlightInfo                      │
│    DomainData CRUD          │    DoGet（读）                         │
│    DomainDataSource CRUD    │    DoPut（写）                         │
│    DomainDataGrant CRUD     │                                       │
└─────────────────────────────┴───────────────────────────────────────┘
```

**主要组件**：

| 组件 | 作用 |
| ------ | ------ |
| HTTP Server | 提供 RESTful 风格的元数据管理接口 |
| gRPC Server | 提供 gRPC 风格的元数据管理接口 |
| Arrow Flight Service | 提供数据读写能力，引擎通过 Flight 协议拉取/推送数据 |
| Operator Bean | 启动时注册默认数据源 |

#### 8.3.4 内部管理与外部访问的接口边界

Kuscia 中的数据操作分为两类：**内部元数据管理** 和 **外部数据访问**。它们使用不同的接口，承担不同的职责。

**1. 内部管理：操作 CRD**

在 Kuscia 内部（如 Kuscia 控制台、运维脚本、Controller、Agent），可以直接通过标准 K8s 接口管理 `DomainData`、`DomainDataSource`、`DomainDataGrant`：

| 操作 | 工具/接口 | 示例 |
| ------ | ----------- | ------ |
| 创建 DomainData | kubectl / KusciaAPI / Go Client | `kubectl apply -f domaindata.yaml` |
| 更新 DomainDataSource | kubectl / KusciaAPI / Go Client | `kubectl patch domaindatasource ...` |
| 授权跨域数据 | kubectl / KusciaAPI / Go Client | `kubectl apply -f domaindatagrant.yaml` |
| 查询元数据 | kubectl / KusciaAPI / Go Client | `kubectl get domaindata -n alice` |

这些操作本质上是对 K3s CRD 的增删改查，**不直接读写实际数据内容**。

**2. 外部访问：通过 DataMesh 读写数据**

对于运行在 Kuscia 内部容器中的引擎（如 SecretFlow、其他隐私计算算法），它们**不能直接访问底层文件系统、OSS 或数据库**，而是必须通过 DataMesh 提供的 Arrow Flight 接口：

| 操作 | DataMesh 接口 | 说明 |
| ------ | --------------- | ------ |
| 获取数据访问票据 | `GetFlightInfo` | 传入 `CommandDomainDataQuery` 或 `CommandDomainDataUpdate` |
| 读取数据 | `DoGet` | 根据票据拉取数据流 |
| 写入数据 | `DoPut` | 根据票据推送数据流 |
| 元数据查询（可选） | gRPC/HTTP `DomainDataService` | 查询 `DomainData` 的列、类型、作者等信息 |

```text
                    ┌─────────────────────────────────────────┐
                    │          外部引擎（如 SecretFlow）        │
                    │  不能直接读取 /var/lib/kuscia/...        │
                    │  不能直连 OSS / MySQL                    │
                    └─────────────────┬───────────────────────┘
                                      │
                                      ▼ Arrow Flight
                    ┌─────────────────────────────────────────┐
                    │              DataMesh                   │
                    │  GetFlightInfo / DoGet / DoPut          │
                    └─────────────────┬───────────────────────┘
                                      │
              ┌───────────────────────┼───────────────────────┐
              ▼                       ▼                       ▼
        localfs / OSS / MySQL    DomainData CRD       DomainDataSource CRD
```

**为什么外部引擎不能绕过 DataMesh？**

1. **安全隔离**：引擎运行在容器/进程沙箱中，不暴露底层存储凭据和路径。
2. **统一访问**：无论数据存在本地、OSS 还是数据库，引擎都使用相同的 Arrow Flight 协议。
3. **权限管控**：DataMesh 可以在数据访问层校验 `DomainDataGrant`、Namespace 权限、使用次数等。
4. **审计与计量**：所有数据读写都经过 DataMesh，便于记录访问日志和流量统计。

**3. 边界总结**

| 场景 | 使用接口 | 谁能使用 | 是否接触实际数据 |
| ------ | ---------- | ---------- | ------------------ |
| 管理 DomainData / DomainDataSource / DomainDataGrant | kubectl / KusciaAPI / Go Client | Kuscia 内部组件、管理员 | 否，只操作元数据 |
| 读取/写入实际数据 | DataMesh Arrow Flight (`GetFlightInfo` / `DoGet` / `DoPut`) | 任务引擎（SecretFlow 等） | 是 |
| 查询数据元信息 | DataMesh gRPC/HTTP 元数据接口 | 引擎或外部系统 | 否，只返回元数据 |

> 因此，**Kuscia 内部用 kubectl/Go Client 管理数据目录，外部引擎用 DataMesh Flight 接口读写数据**，两者是明确的职责边界。

### 8.4 DataMesh 与数据存储的关系

DataMesh 与数据存储的关系可以概括为：**DataMesh 不直接保存业务数据，而是作为元数据到实际存储的“路由层”和“访问层”**。

```text
引擎请求读取 "user-table"
        │
        ▼
┌───────────────┐
│  DataMesh     │
│  查询 DomainData "user-table" 的元信息
│  查询 DomainDataSource "default" 的存储配置
└───────┬───────┘
        │
        ▼
根据 type=localfs 选择 BuiltinLocalFileIOChannel
        │
        ▼
实际路径 = /var/lib/kuscia/var/storage/data/alice/user-table.csv
        │
        ▼
通过 Arrow Flight DoGet 返回数据流
```

**关键理解**：

- `DomainData` + `DomainDataSource` 共同决定数据存在哪里、怎么读取。
- DataMesh 根据 `DomainDataSource.Type` 选择对应的 IO Channel（如 `localfs`、`oss`、`mysql`）。
- 引擎不需要知道后端是本地文件还是 OSS，只需要通过 DataMesh Flight 接口访问。

### 8.5 数据读写流程

以读取本地文件为例，完整流程如下：

1. **客户端构造 Flight 查询请求**

   ```protobuf
   CommandDomainDataQuery {
     domaindata_id = "user-table"
   }
   ```

2. **DataMesh 解析请求上下文**

   ```go
   data, datasource, err := reqCtx.GetDomainDataAndSource(ctx)
   // data.RelativeUri = "alice/user-table.csv"
   // datasource.Type = "localfs"
   // datasource.Info.Localfs.Path = "/var/lib/kuscia/var/storage/data"
   ```

3. **生成 FlightInfo 票据**

   ```go
   filePath := path.Join(ds.Info.Localfs.Path, data.RelativeUri)
   ticketUUID := uuid.New().String()
   // 返回 FlightInfo，其中包含 ticketUUID
   ```

4. **客户端凭票据读取数据**

   ```go
   // DoGet(ticketUUID)
   // DataMesh 根据 ticket 找到缓存的上下文
   // 调用 BuiltinLocalFileIO.Read() 打开文件并流式返回
   ```

> 写入流程类似，使用 `CommandDomainDataUpdate` 和 `DoPut`。

### 8.6 支持的存储后端

DataMesh 目前支持多种内置存储后端：

| 后端类型 | 类型标识 | 说明 |
| ---------- | ---------- | ------ |
| 本地文件系统 | `localfs` | 默认数据源，路径在 `var/storage/data` 下 |
| OSS / S3 兼容 | `oss` | 支持阿里云 OSS 等 S3 兼容对象存储 |
| MySQL | `mysql` | 通过 SQL 读取表数据 |
| PostgreSQL | `postgresql` | 通过 SQL 读取表数据 |
| 外部 DataProxy | `external` | 转发到外部数据代理，支持 ODPS、Hive 等 |

每种后端对应一个 `DataMeshDataIOInterface`，实现 `GetFlightInfo`、`Read`、`Write` 三个核心方法。

### 8.7 数据隔离

Kuscia 通过以下机制保证不同 Domain 的数据隔离：

1. **Namespace 隔离**
    - `DomainData`、`DomainDataSource`、`DomainDataGrant` 都是 `Namespaced` CRD。
    - alice 命名空间下的数据对象，bob 无法直接读取。

2. **Author 字段标识所有者**

   ```go
   kusciaDomainData.Spec.Author = s.conf.KubeNamespace // alice
   ```

3. **授权拷贝机制**
    - 只有经过 `DomainDataGrant` 授权的数据，才会被控制器拷贝到目标域的命名空间。
    - 拷贝时会打上 `LabelDomainDataVendor = grant` 标签，表明这是授权数据。

4. **本地存储路径按域划分**
    - 虽然默认数据源指向 `var/storage/data`，但 `relative_uri` 通常包含域标识，如 `alice/user-table.csv`。

### 8.8 DataMesh 在任务执行中的作用

当 Kuscia 调度一个 `KusciaTask` 时，会：

1. 解析任务输入中引用的 `DomainData`。
2. 将对应的 `DomainData` 名称通过环境变量或配置注入到任务 Pod。
3. 任务容器启动后，调用 DataMesh 的 Arrow Flight 接口读取输入数据。
4. 任务输出也通过 DataMesh 写回，生成新的 `DomainData`。

```text
KusciaTask spec
  inputs:
    - domaindata_id: user-table   ← 引用元数据
        │
        ▼
Pod 启动，容器内引擎通过 Flight 访问 DataMesh
        │
        ▼
DataMesh 解析 DomainData → DomainDataSource → 实际文件/数据库
        │
        ▼
数据流返回给引擎
```

### 8.9 关键结论

- Kuscia 将数据存储分为**元数据层**（K3s CRD）和**数据平面**（实际存储）。
- `DomainData` 是数据的“元信息名片”，`DomainDataSource` 是“存储地址簿”。
- `DomainData.relative_uri` 是相对于 `DomainDataSource` 的相对路径，DataMesh 负责拼接出实际访问位置，无需在 `DomainData` 中写死完整路径。
- **DataMesh 作为 Kuscia 模块启动**，启动时会注册默认 `localfs` 数据源，并按需监听/查询 K3s 中的 CRD；它不会主动扫描或预热所有数据。
- 创建 `DomainData` / `DomainDataSource` **不需要手动调用 DataMesh 接口注册**，两者的关联完全由 `DomainData.spec.data_source` 字段声明；DataMesh 在收到 Flight 请求时自动解析。
- **内部管理与外部访问有明确边界**：Kuscia 内部通过 kubectl/KusciaAPI/Go Client 管理 CRD 元数据；外部引擎（如 SecretFlow）必须通过 DataMesh Arrow Flight (`GetFlightInfo` / `DoGet` / `DoPut`) 读写实际数据。
- **DataMesh 是统一数据访问层**，根据 `DomainDataSource.type` 选择对应的 IO Channel，通过 Apache Arrow Flight 为引擎提供与后端无关的读写能力。
- 跨域数据使用需要 `DomainDataGrant` 授权，控制器会验证签名、有效期、使用次数，并将数据拷贝到目标域。
- 数据隔离主要依靠 K3s Namespace + Author 字段 + 授权拷贝机制实现。

---

## 9. API 层

### 9.1 KusciaAPI 概述

KusciaAPI 是 Kuscia 对外暴露的统一接口层,提供 gRPC 和 HTTP 两种访问方式,是外部系统(如 SecretPad、业务应用)与 Kuscia 交互的唯一入口。

**核心职责**:

- **资源管理**:提供 Job、Task、Domain、DomainData 等资源的 CRUD 操作
- **任务控制**:支持任务的创建、查询、停止、重启、删除等生命周期管理
- **数据管理**:管理域数据的注册、查询、授权和访问
- **配置服务**:提供配置下发和证书管理服务
- **健康检查**:提供系统健康状态监控
- **日志查询**:提供任务和容器的日志查询能力

**架构设计**:

```
┌─────────────────────────────────────────────────────┐
│              External Systems                        │
│         (SecretPad / Business Apps)                  │
└──────────────┬──────────────────────────────────────┘
               │ HTTP/gRPC
               ▼
┌─────────────────────────────────────────────────────┐
│           KusciaAPI Server                           │
│  - HTTP Server (Port: 8082)                         │
│  - gRPC Server (Port: 8083)                         │
├─────────────────────────────────────────────────────┤
│  Handler Layer (协议转换 & 参数校验)                 │
│  - HTTP Handler: pkg/kusciaapi/handler/httphandler/ │
│  - gRPC Handler: pkg/kusciaapi/handler/grpchandler/ │
├─────────────────────────────────────────────────────┤
│  Service Layer (业务逻辑)                            │
│  - JobService                                       │
│  - DomainDataService                                │
│  - DomainService                                    │
│  - ConfigService                                    │
│  - LogService                                       │
│  - HealthService                                    │
├─────────────────────────────────────────────────────┤
│  Kubernetes Client                                  │
│  - CRD Operations (Create/Update/Delete/Watch)      │
└──────────────┬──────────────────────────────────────┘
               │ Watch & Update
               ▼
┌─────────────────────────────────────────────────────┐
│          K3s API Server (etcd)                       │
│     All CRDs stored in etcd                          │
└─────────────────────────────────────────────────────┘
```

**端口说明**:

| 端口 | 协议 | 说明 | 使用场景 |
|------|------|------|----------|
| 8082 | HTTP | KusciaAPI HTTP 访问 | Web 控制台、curl 测试、简单集成 |
| 8083 | gRPC | KusciaAPI gRPC 访问 | 高性能调用、流式传输(Watch)、SDK 集成 |

---

### 9.2 Job 管理 API

Job API 用于管理隐私计算任务的生命周期,支持任务的创建、查询、控制和监控。

**代码路径**:

- Proto 定义: `proto/api/v1alpha1/kusciaapi/job.proto`
- Service 实现: `pkg/kusciaapi/service/job_service.go`
- Handler 实现: `pkg/kusciaapi/handler/grpchandler/job_handler.go`

#### 9.2.1 CreateJob - 创建任务

**接口定义**:

```protobuf
rpc CreateJob(CreateJobRequest) returns (CreateJobResponse);
```

**请求参数** (`CreateJobRequest`):

```protobuf
message CreateJobRequest {
  RequestHeader header = 1;        // 请求头(包含认证信息)
  string job_id = 2;                // 任务唯一标识
  string initiator = 3;             // 发起方域名 ID
  int32 max_parallelism = 4;        // 最大并发数(默认 1)
  repeated Task tasks = 5;          // 任务列表(DAG 定义)
  map<string, string> custom_fields = 6;  // 自定义字段(转为 Label)
}

message Task {
  string app_image = 1;                     // 应用镜像名称
  repeated Party parties = 2;               // 参与方列表
  string alias = 3;                         // 任务别名(展示用)
  string task_id = 4;                       // 任务 ID(调度用)
  repeated string dependencies = 5;         // 依赖任务(alias 列表)
  string task_input_config = 6;             // 任务输入配置(JSON)
  int32 priority = 7;                       // 优先级(值越大越优先)
  ScheduleConfig schedule_config = 8;       // 调度配置
  bool tolerable = 9;                       // 是否可容忍失败
}

message Party {
  string domain_id = 1;                     // 参与方域名 ID
  string role = 2;                          // 角色(server/client)
  JobResource resources = 3;                // 资源配置
  repeated BandwidthLimit bandwidth_limits = 4;  // 带宽限制
}

message JobResource {
  string cpu = 1;       // CPU 限制(如 "2" 表示 2 核)
  string memory = 2;    // 内存限制(如 "4Gi")
  string bandwidth = 3; // 带宽限制(如 "10",单位 Mbps)
}
```

**响应参数** (`CreateJobResponse`):

```protobuf
message CreateJobResponse {
  Status status = 1;                    // 响应状态
  CreateJobResponseData data = 2;       // 响应数据
}

message CreateJobResponseData {
  string job_id = 1;  // 创建的任务 ID
}
```

**功能说明**:

1. **参数校验**:验证任务 ID、参与方、资源配置等的合法性
2. **权限认证**:检查发起方是否有权限创建跨域任务
3. **DAG 校验**:检查任务依赖是否形成环
4. **资源转换**:将 API 请求转换为 KusciaJob CR
5. **提交到 K8s**:调用 Kubernetes API 创建 KusciaJob CR
6. **触发调度**:KusciaJob Controller 监听到新 Job 后开始调度

**HTTP 示例**:

```bash
curl -X POST http://localhost:8082/api/v1/kusciaapi/job/create \
  -H "Content-Type: application/json" \
  -d '{
    "job_id": "psi-job-001",
    "initiator": "alice",
    "max_parallelism": 2,
    "tasks": [
      {
        "task_id": "data-preprocess",
        "alias": "数据预处理",
        "app_image": "secretflow/data-preprocess:latest",
        "parties": [
          {
            "domain_id": "alice",
            "role": "server",
            "resources": {
              "cpu": "2",
              "memory": "4Gi"
            }
          },
          {
            "domain_id": "bob",
            "role": "client",
            "resources": {
              "cpu": "2",
              "memory": "4Gi"
            }
          }
        ],
        "task_input_config": "{\"input_columns\": [\"age\", \"income\"]}"
      },
      {
        "task_id": "psi-compute",
        "alias": "PSI 计算",
        "dependencies": ["data-preprocess"],
        "app_image": "secretflow/psi:latest",
        "parties": [
          {
            "domain_id": "alice",
            "role": "server",
            "resources": {
              "cpu": "4",
              "memory": "8Gi",
              "bandwidth": "50"
            }
          },
          {
            "domain_id": "bob",
            "role": "client",
            "resources": {
              "cpu": "4",
              "memory": "8Gi",
              "bandwidth": "50"
            }
          }
        ],
        "task_input_config": "{\"protocol\": \"ECDH\"}"
      }
    ],
    "custom_fields": {
      "business_type": "marketing",
      "project_id": "proj-123"
    }
  }'
```

**gRPC 示例** (Go):

```go
import (
    "context"
    pb "github.com/secretflow/kuscia/proto/api/v1alpha1/kusciaapi"
)

func createJob() {
    conn, _ := grpc.Dial("localhost:8083", grpc.WithInsecure())
    defer conn.Close()
    
    client := pb.NewJobServiceClient(conn)
    
    req := &pb.CreateJobRequest{
        JobId: "psi-job-001",
        Initiator: "alice",
        MaxParallelism: 2,
        Tasks: []*pb.Task{
            {
                TaskId: "psi-compute",
                Alias: "PSI 计算",
                AppImage: "secretflow/psi:latest",
                Parties: []*pb.Party{
                    {
                        DomainId: "alice",
                        Role: "server",
                        Resources: &pb.JobResource{
                            Cpu: "4",
                            Memory: "8Gi",
                        },
                    },
                },
            },
        },
    }
    
    resp, err := client.CreateJob(context.Background(), req)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Job created: %s\n", resp.Data.JobId)
}
```

**返回示例**:

```json
{
  "status": {
    "code": 0,
    "message": "success"
  },
  "data": {
    "job_id": "psi-job-001"
  }
}
```

---

#### 9.2.2 QueryJob - 查询任务

**接口定义**:

```protobuf
rpc QueryJob(QueryJobRequest) returns (QueryJobResponse);
```

**功能**:查询指定任务的详细信息,包括任务状态、各参与方状态、执行进度等。

**请求参数**:

```protobuf
message QueryJobRequest {
  RequestHeader header = 1;
  string job_id = 2;  // 任务 ID
}
```

**响应数据**:

```protobuf
message QueryJobResponseData {
  string job_id = 1;
  string initiator = 2;
  int32 max_parallelism = 3;
  repeated TaskConfig tasks = 4;        // 任务配置列表
  JobStatusDetail status = 5;           // 任务状态详情
  map<string, string> custom_fields = 6;
}

message JobStatusDetail {
  string state = 1;          // 任务状态: PendingApproval/Pending/Running/Succeeded/Failed
  string message = 2;        // 状态描述信息
  int64 start_time = 3;      // 开始时间(Unix 时间戳)
  int64 end_time = 4;        // 结束时间
  repeated TaskStatus task_status = 5;  // 各子任务状态
}
```

**使用场景**:

- 前端展示任务执行进度
- 监控系统采集任务状态
- 任务完成后获取结果信息

---

#### 9.2.3 BatchQueryJobStatus - 批量查询任务状态

**接口定义**:

```protobuf
rpc BatchQueryJobStatus(BatchQueryJobStatusRequest) returns (BatchQueryJobStatusResponse);
```

**功能**:一次性查询多个任务的状态,减少网络往返次数。

**请求参数**:

```protobuf
message BatchQueryJobStatusRequest {
  RequestHeader header = 1;
  repeated string job_ids = 2;  // 任务 ID 列表
}
```

**响应数据**:

```protobuf
message BatchQueryJobStatusResponse {
  Status status = 1;
  map<string, JobStatusDetail> job_status_map = 2;  // job_id -> 状态映射
}
```

**优势**:相比多次调用 `QueryJob`,批量查询可显著提升性能,适合仪表盘等需要展示大量任务状态的场景。

---

#### 9.2.4 StopJob - 停止任务

**接口定义**:

```protobuf
rpc StopJob(StopJobRequest) returns (StopJobResponse);
```

**功能**:优雅地停止正在运行的任务,等待当前步骤完成后再终止。

**请求参数**:

```protobuf
message StopJobRequest {
  RequestHeader header = 1;
  string job_id = 2;
  string reason = 3;  // 停止原因(记录到审计日志)
}
```

**执行流程**:

1. 更新 KusciaJob 状态为 `Stopping`
2. KusciaJob Controller 检测到状态变化
3. 向所有运行中的 Pod 发送 SIGTERM 信号
4. 等待容器优雅退出(默认 30 秒)
5. 如果超时,发送 SIGKILL 强制终止
6. 更新任务状态为 `Failed`

**使用场景**:

- 用户主动取消长时间运行的任务
- 发现任务配置错误需要中止
- 资源紧张时释放资源

---

#### 9.2.5 DeleteJob - 删除任务

**接口定义**:

```protobuf
rpc DeleteJob(DeleteJobRequest) returns (DeleteJobResponse);
```

**功能**:从系统中彻底删除任务及其相关资源(Pod、ConfigMap、Secret 等)。

**注意事项**:

- 只能删除已完成(Succeeded/Failed)的任务
- 运行中的任务需要先 Stop 再 Delete
- 删除操作不可逆,会清理所有关联资源

---

#### 9.2.6 SuspendJob - 暂停任务

**接口定义**:

```protobuf
rpc SuspendJob(SuspendJobRequest) returns (SuspendJobResponse);
```

**功能**:暂停任务执行,保留当前状态,后续可通过 RestartJob 恢复。

**与 StopJob 的区别**:

- `SuspendJob`: 临时暂停,可恢复,保留中间结果
- `StopJob`: 永久停止,不可恢复,清理资源

**使用场景**:

- 维护窗口期间暂停非关键任务
- 等待外部依赖(如数据准备完成)
- 成本控制(暂停低优先级任务)

---

#### 9.2.7 RestartJob - 重启任务

**接口定义**:

```protobuf
rpc RestartJob(RestartJobRequest) returns (RestartJobResponse);
```

**功能**:重新运行已暂停或失败的任务。

**执行策略**:

- 从失败的步骤重新开始
- 复用已成功步骤的结果(如果支持断点续传)
- 重新分配资源并调度

---

#### 9.2.8 CancelJob - 取消待审批任务

**接口定义**:

```protobuf
rpc CancelJob(CancelJobRequest) returns (CancelJobResponse);
```

**功能**:取消处于 `PendingApproval` 状态的任务(在审批流程中)。

**使用场景**:

- 发起方撤回任务申请
- 审批拒绝前的主动取消

---

#### 9.2.9 ApproveJob - 审批任务

**接口定义**:

```protobuf
rpc ApproveJob(ApproveJobRequest) returns (ApproveJobResponse);
```

**功能**:参与方审批跨域任务请求。

**请求参数**:

```protobuf
message ApproveJobRequest {
  RequestHeader header = 1;
  string job_id = 2;
  bool approved = 3;        // true=通过, false=拒绝
  string reason = 4;        // 审批意见
}
```

**工作流程**:

1. 发起方创建 Job 后,状态为 `PendingApproval`
2. 各参与方收到审批通知
3. 参与方调用 `ApproveJob` 进行审批
4. 所有参与方都通过后,状态变为 `Pending`,开始调度
5. 任一方拒绝,状态变为 `Rejected`

---

#### 9.2.10 WatchJob - 监听任务事件

**接口定义**:

```protobuf
rpc WatchJob(WatchJobRequest) returns (stream WatchJobEventResponse);
```

**功能**:实时监听任务状态变化,采用服务端流式传输。

**请求参数**:

```protobuf
message WatchJobRequest {
  RequestHeader header = 1;
  string job_id = 2;
}
```

**响应流**:

```protobuf
message WatchJobEventResponse {
  string event_type = 1;   // ADDED/MODIFIED/DELETED
  JobStatusDetail status = 2;
}
```

**使用场景**:

- 前端实时显示任务进度
- 异步通知系统(任务完成时推送消息)
- 监控系统实时采集状态

**gRPC 示例** (Go):

```go
func watchJob(jobID string) {
    conn, _ := grpc.Dial("localhost:8083", grpc.WithInsecure())
    defer conn.Close()
    
    client := pb.NewJobServiceClient(conn)
    
    req := &pb.WatchJobRequest{
        JobId: jobID,
    }
    
    stream, err := client.WatchJob(context.Background(), req)
    if err != nil {
        log.Fatal(err)
    }
    
    for {
        event, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            log.Fatal(err)
        }
        
        fmt.Printf("Event: %s, Status: %s\n", event.EventType, event.Status.State)
        
        if event.Status.State == "Succeeded" || event.Status.State == "Failed" {
            break
        }
    }
}
```

---

### 9.3 DomainData 管理 API

DomainData API 用于管理隐私计算中的数据资产,支持数据的注册、查询、授权和访问控制。

**代码路径**:

- Proto 定义: `proto/api/v1alpha1/kusciaapi/domaindata.proto`
- Service 实现: `pkg/kusciaapi/service/domaindata_service.go`

#### 9.3.1 CreateDomainData - 注册数据

**接口定义**:

```protobuf
rpc CreateDomainData(CreateDomainDataRequest) returns (CreateDomainDataResponse);
```

**请求参数**:

```protobuf
message CreateDomainDataRequest {
  RequestHeader header = 1;
  string domaindata_id = 2;     // 数据唯一标识(可选,自动生成)
  string name = 3;               // 数据名称(可读)
  string type = 4;               // 数据类型: table/model/rule/report/unknown
  string relative_uri = 5;       // 相对路径(相对于 DataSource)
  string domain_id = 6;          // 所属域 ID
  string datasource_id = 7;      // 数据源 ID(可选,使用默认数据源)
  map<string,string> attributes = 8;  // 扩展属性
  Partition partition = 9;       // 分区信息(暂未支持)
  repeated DataColumn columns = 10;     // 表结构(仅 table 类型)
  string vendor = 11;            // 数据来源: manual/secretflow/other
  FileFormat file_format = 12;   // 文件格式: csv/parquet/orc
}

message DataColumn {
  string name = 1;      // 列名
  string type = 2;      // 数据类型: int/string/float/datetime
  bool is_primary = 3;  // 是否主键
}

message FileFormat {
  string format = 1;    // csv, parquet, orc
  string delimiter = 2; // 分隔符(仅 CSV)
  bool has_header = 3;  // 是否有表头
}
```

**功能说明**:

1. **参数校验**:验证数据 ID、类型、路径等的合法性
2. **数据源检查**:确认指定的 DataSource 存在
3. **权限认证**:检查是否有权限在该域注册数据
4. **元数据存储**:创建 DomainData CR,存储元数据到 etcd
5. **路径拼接**:DataSource URI + Relative URI = 完整数据路径

**HTTP 示例**:

```bash
curl -X POST http://localhost:8082/api/v1/kusciaapi/domaindata/create \
  -H "Content-Type: application/json" \
  -d '{
    "domaindata_id": "customer-table-001",
    "name": "客户信息表",
    "type": "table",
    "relative_uri": "data/customer.csv",
    "domain_id": "alice",
    "datasource_id": "local-fs-01",
    "attributes": {
      "description": "包含客户基本信息",
      "owner": "marketing-team",
      "row_count": "100000"
    },
    "columns": [
      {
        "name": "customer_id",
        "type": "string",
        "is_primary": true
      },
      {
        "name": "age",
        "type": "int",
        "is_primary": false
      },
      {
        "name": "income",
        "type": "float",
        "is_primary": false
      }
    ],
    "vendor": "manual",
    "file_format": {
      "format": "csv",
      "delimiter": ",",
      "has_header": true
    }
  }'
```

**返回示例**:

```json
{
  "status": {
    "code": 0,
    "message": "success"
  },
  "data": {
    "domaindata_id": "customer-table-001"
  }
}
```

**实际存储路径示例**:

```
假设 DataSource 配置:
  datasource_id: local-fs-01
  uri: /home/kuscia/data

则完整数据路径为:
  /home/kuscia/data/data/customer.csv
```

---

#### 9.3.2 UpdateDomainData - 更新数据元信息

**接口定义**:

```protobuf
rpc UpdateDomainData(UpdateDomainDataRequest) returns (UpdateDomainDataResponse);
```

**功能**:更新已注册数据的元信息(不修改实际数据文件)。

**可更新字段**:

- `name`: 数据名称
- `type`: 数据类型
- `relative_uri`: 数据路径
- `attributes`: 扩展属性
- `columns`: 表结构
- `vendor`: 数据来源

**注意事项**:

- 不能修改 `domaindata_id` 和 `domain_id`
- 修改 `relative_uri` 时需确保新路径存在

---

#### 9.3.3 QueryDomainData - 查询数据

**接口定义**:

```protobuf
rpc QueryDomainData(QueryDomainDataRequest) returns (QueryDomainDataResponse);
```

**请求参数**:

```protobuf
message QueryDomainDataRequest {
  RequestHeader header = 1;
  QueryDomainDataRequestData data = 2;
}

message QueryDomainDataRequestData {
  string domain_id = 1;        // 域 ID
  string domaindata_id = 2;    // 数据 ID
}
```

**响应数据**:

```protobuf
message DomainData {
  string domaindata_id = 1;
  string name = 2;
  string type = 3;
  string relative_uri = 4;
  string domain_id = 5;
  string datasource_id = 6;
  map<string,string> attributes = 7;
  Partition partition = 8;
  repeated DataColumn columns = 9;
  string vendor = 10;
  FileFormat file_format = 11;
  string author = 12;          // 创建者
  int64 create_time = 13;      // 创建时间
}
```

**使用场景**:

- 任务创建前查询可用数据
- 数据详情页展示元信息
- 验证数据是否存在

---

#### 9.3.4 ListDomainData - 列举数据

**接口定义**:

```protobuf
rpc ListDomainData(ListDomainDataRequest) returns (ListDomainDataResponse);
```

**请求参数**:

```protobuf
message ListDomainDataRequestData {
  string domain_id = 1;             // 域 ID(必填)
  string domaindata_type = 2;       // 数据类型过滤(可选)
  string domaindata_vendor = 3;     // 数据来源过滤(可选)
}
```

**响应数据**:

```protobuf
message DomainDataList {
  repeated DomainData domaindata_list = 1;
}
```

**HTTP 示例**:

```bash
# 查询 alice 域的所有 table 类型数据
curl "http://localhost:8082/api/v1/kusciaapi/domaindata/list?domain_id=alice&domaindata_type=table"
```

**使用场景**:

- 数据目录浏览
- 按类型筛选数据
- 数据资产管理页面

---

#### 9.3.5 BatchQueryDomainData - 批量查询数据

**接口定义**:

```protobuf
rpc BatchQueryDomainData(BatchQueryDomainDataRequest) returns (BatchQueryDomainDataResponse);
```

**功能**:一次性查询多个数据的元信息。

**请求参数**:

```protobuf
message BatchQueryDomainDataRequest {
  RequestHeader header = 1;
  repeated QueryDomainDataRequestData data = 2;
}
```

**优势**:减少网络往返,适合任务创建时需要引用多个数据的场景。

---

#### 9.3.6 DeleteDomainData - 删除数据注册

**接口定义**:

```protobuf
rpc DeleteDomainData(DeleteDomainDataRequest) returns (DeleteDomainDataResponse);
```

**功能**:从元数据管理中删除数据注册信息(不删除实际数据文件)。

**注意事项**:

- 仅删除元数据(CR),不删除物理文件
- 如果有 DomainDataGrant 引用该数据,删除会失败
- 删除前需确保没有任务正在使用该数据

---

#### 9.3.7 DeleteDomainDataAndRaw - 删除数据及文件

**接口定义**:

```protobuf
rpc DeleteDomainDataAndRaw(DeleteDomainDataRequest) returns (DeleteDomainDataResponse);
```

**功能**:同时删除元数据和实际数据文件。

**警告**:

- ⚠️ 此操作不可逆!
- 会永久删除物理文件
- 需要有足够的权限

---

### 9.4 DomainDataGrant 管理 API

DomainDataGrant API 用于管理数据授权,允许其他域访问本域的数据。

**代码路径**:

- Proto 定义: `proto/api/v1alpha1/kusciaapi/domaindatagrant.proto`
- Service 实现: `pkg/kusciaapi/service/domaindata_grant.go`

#### 9.4.1 CreateDomainDataGrant - 创建授权

**接口定义**:

```protobuf
rpc CreateDomainDataGrant(CreateDomainDataGrantRequest) returns (CreateDomainDataGrantResponse);
```

**请求参数**:

```protobuf
message CreateDomainDataGrantRequest {
  RequestHeader header = 1;
  string grant_id = 2;              // 授权 ID
  string domain_id = 3;             // 数据所有者域
  string domaindata_id = 4;         // 被授权的数据
  string authorized_domain = 5;     // 被授权方域
  string permission = 6;            // 权限级别: read/write
  int64 expire_time = 7;            // 过期时间(Unix 时间戳)
}
```

**功能说明**:

1. 创建 DomainDataGrant CR
2. InterConn Controller 同步授权信息到被授权方集群
3. 被授权方可通过 DomainRoute 访问该数据

**HTTP 示例**:

```bash
curl -X POST http://localhost:8082/api/v1/kusciaapi/domaindatagrant/create \
  -H "Content-Type: application/json" \
  -d '{
    "grant_id": "grant-alice-to-bob-001",
    "domain_id": "alice",
    "domaindata_id": "customer-table-001",
    "authorized_domain": "bob",
    "permission": "read",
    "expire_time": 1735689600  // 2025-01-01 00:00:00
  }'
```

---

#### 9.4.2 QueryDomainDataGrant - 查询授权

**功能**:查询特定授权的详细信息。

---

#### 9.4.3 ListDomainDataGrant - 列举授权

**功能**:列出某域的所有授权记录或被授权记录。

---

#### 9.4.4 RevokeDomainDataGrant - 撤销授权

**功能**:撤销已授予的访问权限。

**执行流程**:

1. 删除 DomainDataGrant CR
2. InterConn Controller 同步撤销信息
3. 被授权方无法再通过 DomainRoute 访问该数据

---

### 9.5 Domain 管理 API

Domain API 用于管理参与方域的注册和配置。

**代码路径**:

- Proto 定义: `proto/api/v1alpha1/kusciaapi/domain.proto`
- Service 实现: `pkg/kusciaapi/service/domain_service.go`

#### 9.5.1 RegisterDomain - 注册域

**接口定义**:

```protobuf
rpc RegisterDomain(RegisterDomainRequest) returns (RegisterDomainResponse);
```

**请求参数**:

```protobuf
message RegisterDomainRequest {
  RequestHeader header = 1;
  string domain_id = 2;              // 域唯一标识
  string domain_name = 3;            // 域名称
  string network_type = 4;           // 网络类型: centralized/p2p
  string interconn_protocol = 5;     // 互联协议: grpc/http
  map<string, string> annotations = 6;  // 注释信息
}
```

**功能**:

1. 创建 Domain CR(Cluster 级别)
2. 为该域创建独立的 Namespace
3. 生成域密钥对(如果未提供)
4. 初始化域配置(ConfigMap/Secret)

---

#### 9.5.2 QueryDomain - 查询域信息

**功能**:查询域的详细信息,包括网络配置、证书信息等。

---

#### 9.5.3 UpdateDomain - 更新域配置

**功能**:更新域的网络地址、证书等配置。

---

### 9.6 DomainRoute 管理 API

DomainRoute API 用于配置域间通信路由。

**代码路径**:

- Proto 定义: `proto/api/v1alpha1/kusciaapi/domain_route.proto`
- Service 实现: `pkg/kusciaapi/service/domain_route_service.go`

#### 9.6.1 CreateDomainRoute - 创建路由

**接口定义**:

```protobuf
rpc CreateDomainRoute(CreateDomainRouteRequest) returns (CreateDomainRouteResponse);
```

**请求参数**:

```protobuf
message CreateDomainRouteRequest {
  RequestHeader header = 1;
  string source_domain = 2;      // 源域
  string target_domain = 3;      // 目标域
  string endpoint = 4;           // 目标域端点地址
  string protocol = 5;           // 通信协议: grpc/http
  TLSConfig tls = 6;             // TLS 配置
}
```

**功能**:

1. 创建 DomainRoute CR
2. Gateway Controller 配置 Envoy 路由规则
3. 建立域间通信通道

---

### 9.7 Config 配置管理 API

Config API 提供配置下发服务,用于动态更新 Kuscia 组件配置。

**代码路径**:

- Proto 定义: `proto/api/v1alpha1/kusciaapi/config.proto`
- Service 实现: `pkg/kusciaapi/service/config_service.go`

#### 9.7.1 GetConfig - 获取配置

**接口定义**:

```protobuf
rpc GetConfig(GetConfigRequest) returns (GetConfigResponse);
```

**功能**:从 ConfigMap 或 Secret 中读取配置项。

---

#### 9.7.2 SetConfig - 设置配置

**接口定义**:

```protobuf
rpc SetConfig(SetConfigRequest) returns (SetConfigResponse);
```

**功能**:更新 ConfigMap 中的配置,触发组件热重载。

**使用场景**:

- 动态调整日志级别
- 更新调度策略参数
- 修改网络超时配置

---

### 9.8 Certificate 证书管理 API

Certificate API 提供 TLS 证书的生成和管理服务。

**代码路径**:

- Proto 定义: `proto/api/v1alpha1/kusciaapi/certificate.proto`
- ConfManager 实现: `pkg/confmanager/service/`

#### 9.8.1 GenerateCert - 生成证书

**接口定义**:

```protobuf
rpc GenerateCert(GenerateCertRequest) returns (GenerateCertResponse);
```

**功能**:

1. 使用 CA 密钥签发域证书
2. 将证书存储到 Secret
3. 返回证书内容供下载使用

---

#### 9.8.2 RenewCert - 续期证书

**功能**:在证书即将过期时自动或手动续期。

---

### 9.9 Log 日志查询 API

Log API 提供任务和容器的日志查询能力。

**代码路径**:

- Proto 定义: `proto/api/v1alpha1/kusciaapi/log.proto`
- Service 实现: `pkg/kusciaapi/service/log_service.go`

#### 9.9.1 QueryTaskLog - 查询任务日志

**接口定义**:

```protobuf
rpc QueryTaskLog(QueryTaskLogRequest) returns (QueryTaskLogResponse);
```

**请求参数**:

```protobuf
message QueryTaskLogRequest {
  RequestHeader header = 1;
  string job_id = 2;           // 任务 ID
  string task_id = 3;          // 子任务 ID
  string domain_id = 4;        // 域 ID
  int64 start_time = 5;        // 开始时间
  int64 end_time = 6;          // 结束时间
  int32 lines = 7;             // 返回行数(默认 100)
  bool follow = 8;             // 是否实时跟踪
}
```

**功能**:

1. 定位 Pod 和容器
2. 从文件系统或 containerd 读取日志
3. 支持分页和时间范围过滤
4. 支持实时流式输出(follow 模式)

**HTTP 示例**:

```bash
# 查询最近 100 行日志
curl "http://localhost:8082/api/v1/kusciaapi/log/query?job_id=psi-job-001&task_id=psi-compute&domain_id=alice&lines=100"

# 实时跟踪日志
curl "http://localhost:8082/api/v1/kusciaapi/log/query?job_id=psi-job-001&task_id=psi-compute&domain_id=alice&follow=true"
```

---

### 9.10 Health 健康检查 API

Health API 提供系统健康状态监控。

**代码路径**:

- Proto 定义: `proto/api/v1alpha1/kusciaapi/health.proto`
- Service 实现: `pkg/kusciaapi/service/health_service.go`

#### 9.10.1 CheckHealth - 健康检查

**接口定义**:

```protobuf
rpc CheckHealth(CheckHealthRequest) returns (CheckHealthResponse);
```

**响应数据**:

```protobuf
message CheckHealthResponse {
  Status status = 1;
  HealthData data = 2;
}

message HealthData {
  string node_status = 1;      // 节点状态: Ready/NotReady
  map<string, ComponentStatus> components = 2;  // 组件状态
  int64 uptime = 3;            // 运行时长(秒)
  string version = 4;          // Kuscia 版本
}

message ComponentStatus {
  string name = 1;
  bool healthy = 2;
  string message = 3;
}
```

**检查项**:

- K3s API Server 连通性
- etcd 健康状态
- Agent 运行状态
- Gateway 运行状态
- 磁盘空间使用情况
- 内存使用情况

**HTTP 示例**:

```bash
curl http://localhost:8082/api/v1/kusciaapi/health/check
```

**返回示例**:

```json
{
  "status": {
    "code": 0,
    "message": "success"
  },
  "data": {
    "node_status": "Ready",
    "components": {
      "k3s": {
        "name": "k3s",
        "healthy": true,
        "message": "running"
      },
      "agent": {
        "name": "agent",
        "healthy": true,
        "message": "running"
      },
      "gateway": {
        "name": "gateway",
        "healthy": true,
        "message": "running"
      }
    },
    "uptime": 86400,
    "version": "v1.2.0"
  }
}
```

---

### 9.11 AppImage 应用镜像 API

AppImage API 用于管理任务使用的镜像模板。

**代码路径**:

- Proto 定义: `proto/api/v1alpha1/kusciaapi/appimage.proto`
- Service 实现: `pkg/kusciaapi/service/appimage_service.go`

#### 9.11.1 RegisterAppImage - 注册镜像

**接口定义**:

```protobuf
rpc RegisterAppImage(RegisterAppImageRequest) returns (RegisterAppImageResponse);
```

**请求参数**:

```protobuf
message RegisterAppImageRequest {
  RequestHeader header = 1;
  string image_id = 2;         // 镜像 ID
  string image_name = 3;       // 镜像名称
  string image_tag = 4;        // 镜像标签
  string vendor = 5;           // 供应商
  repeated string supported_tasks = 6;  // 支持的任务类型
  map<string, string> labels = 7;       // 标签
}
```

**功能**:

1. 创建 AppImage CR
2. 镜像预拉取(可选)
3. 缓存镜像元信息

---

### 9.12 Serving 在线服务 API

Serving API 用于部署和管理在线推理服务。

**代码路径**:

- Proto 定义: `proto/api/v1alpha1/kusciaapi/serving.proto`
- Service 实现: `pkg/kusciaapi/service/serving_service.go`

#### 9.12.1 DeployServing - 部署服务

**接口定义**:

```protobuf
rpc DeployServing(DeployServingRequest) returns (DeployServingResponse);
```

**功能**:

1. 创建 Serving CR
2. 部署模型服务 Pod
3. 配置 Service 和 Ingress
4. 返回服务访问地址

---

### 9.13 API 认证与授权

#### 9.13.1 认证机制

KusciaAPI 支持多种认证方式:

**1. Token 认证**:

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8082/api/v1/...
```

**2. mTLS 认证**:

- 客户端提供证书和私钥
- 服务端验证客户端证书
- 双向身份验证

**3. API Key 认证**:

```bash
curl -H "X-API-Key: <api-key>" http://localhost:8082/api/v1/...
```

#### 9.13.2 授权机制

基于 RBAC(Role-Based Access Control):

```yaml
# Role 定义
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: alice
  name: domain-data-admin
rules:
- apiGroups: ["kuscia.secretflow"]
  resources: ["domaindatas"]
  verbs: ["get", "list", "create", "update", "delete"]
---
# RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: alice-admin-binding
  namespace: alice
subjects:
- kind: User
  name: alice-user
roleRef:
  kind: Role
  name: domain-data-admin
```

---

### 9.14 API 错误码规范

KusciaAPI 采用统一的错误码体系:

**错误码分类**:

| 错误码范围 | 类别 | 示例 |
| ----------- | ------ | ------ |
| 0 | 成功 | SUCCESS |
| 1000-1999 | 请求参数错误 | KusciaAPIErrRequestValidate |
| 2000-2999 | 认证授权错误 | KusciaAPIErrAuthFailed |
| 3000-3999 | 资源操作错误 | KusciaAPIErrCreateJobFailed |
| 4000-4999 | 系统内部错误 | KusciaAPIErrInternal |

**错误响应格式**:

```json
{
  "status": {
    "code": 3001,
    "message": "Failed to create job: job_id already exists"
  }
}
```

---

### 9.15 API 版本管理

KusciaAPI 遵循语义化版本:

**URL 路径**:

```
/api/v1/kusciaapi/job/create
/api/v1/kusciaapi/domaindata/list
```

**向后兼容原则**:

- 新增字段不影响旧版本客户端
- 废弃字段标记 `@deprecated`
- 大版本升级提供迁移指南

---

### 9.16 API 限流与配额

**限流策略**:

- 基于 IP 的速率限制
- 基于用户的配额管理
- 突发流量控制

**配置示例**:

```yaml
rate_limiting:
  requests_per_second: 100
  burst_size: 200
  per_user_quota:
    default: 50
    premium: 200
```

---

### 9.17 API 监控与指标

**暴露的指标**:

- API 请求量(QPS)
- 请求延迟(P50/P95/P99)
- 错误率
- 活跃连接数

**Prometheus 指标**:

```
kusciaapi_http_requests_total{kusciaapi_http_request_duration_seconds_bucket{method="POST", path="/job/create"}
kusciaapi_http_request_duration_seconds_bucket{method="POST", path="/job/create", le="0.1"}
```

---

### 9.18 最佳实践

#### 9.18.1 任务创建最佳实践

**1. 合理设置并发度**:

```json
{
  "max_parallelism": 3  // 根据资源情况设置
}
```

**2. 配置资源限制**:

```json
{
  "parties": [{
    "resources": {
      "cpu": "4",
      "memory": "8Gi",
      "bandwidth": "50"
    }
  }]
}
```

**3. 使用自定义字段追踪**:

```json
{
  "custom_fields": {
    "project_id": "proj-123",
    "business_type": "marketing",
    "owner": "team-a"
  }
}
```

#### 9.18.2 数据管理最佳实践

**1. 规范化命名**:

```
good: customer-table-2024-q1
bad: data1, test
```

**2. 完善元数据**:

```json
{
  "attributes": {
    "description": "客户基本信息表",
    "owner": "marketing-team",
    "row_count": "100000",
    "update_frequency": "daily"
  }
}
```

**3. 谨慎授权**:

- 最小权限原则(read vs write)
- 设置合理的过期时间
- 定期审查授权记录

#### 9.18.3 错误处理最佳实践

**1. 重试机制**:

```go
func createJobWithRetry(client pb.JobServiceClient, req *pb.CreateJobRequest) (*pb.CreateJobResponse, error) {
    var resp *pb.CreateJobResponse
    var err error
    
    for i := 0; i < 3; i++ {
        resp, err = client.CreateJob(context.Background(), req)
        if err == nil {
            return resp, nil
        }
        
        // 判断是否可重试
        if !isRetryableError(err) {
            return nil, err
        }
        
        time.Sleep(time.Second * time.Duration(i+1))  // 指数退避
    }
    
    return nil, err
}
```

**2. 超时设置**:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

resp, err := client.CreateJob(ctx, req)
```

**3. 日志记录**:

```go
if err != nil {
    log.Errorf("CreateJob failed: job_id=%s, error=%v", req.JobId, err)
    return nil, err
}
log.Infof("CreateJob success: job_id=%s", resp.Data.JobId)
```

---

### 9.19 ConfManager 配置与证书服务

ConfManager 是 Kuscia 的配置和证书管理中心,提供独立的服务接口。

**端口**:

- gRPC: 8090
- HTTP: 8091

**主要功能**:

#### 9.19.1 配置管理

**接口**:

- `GetConfig`: 获取配置项
- `SetConfig`: 更新配置项
- `WatchConfig`: 监听配置变化

**使用场景**:

- 动态调整日志级别
- 更新调度参数
- 修改网络配置

#### 9.19.2 证书管理

**接口**:

- `GenerateCert`: 生成 TLS 证书
- `RenewCert`: 续期证书
- `RevokeCert`: 吊销证书
- `QueryCert`: 查询证书状态

**证书类型**:

- CA 证书: 根证书,用于签发域证书
- 域证书: 每个参与方的 TLS 证书
- 客户端证书: API 访问认证

**证书生命周期**:

```
生成 → 分发 → 使用 → 监控 → 续期/吊销
```

**自动续期**:

- 监控证书有效期
- 到期前 30 天自动续期
- 续期后滚动更新配置

---

## 10. 运维、监控与诊断

### 10.1 监控体系架构

Kuscia 提供了完整的监控体系,涵盖业务指标、系统指标和网络指标三个层面:

```
┌─────────────────────────────────────────────────────┐
│              Prometheus Server                       │
│         (定时 scrape 各 Exporter)                    │
└──────────────┬──────────────────────────────────────┘
               │ HTTP /metrics
               ▼
┌─────────────────────────────────────────────────────┐
│          Metric Exporters                            │
├─────────────────────────────────────────────────────┤
│  1. Kuscia MetricExporter (Port: 9092)              │
│     - Job/Task 业务指标                              │
│     - Domain/Data 资源指标                           │
│     - Controller 性能指标                            │
├─────────────────────────────────────────────────────┤
│  2. NodeExporter (Port: 9100)                       │
│     - CPU/内存/磁盘使用率                            │
│     - 网络流量统计                                   │
│     - 文件系统状态                                   │
├─────────────────────────────────────────────────────┤
│  3. SsExporter (Port: 9101)                         │
│     - Envoy Socket 统计                             │
│     - 连接池状态                                     │
│     - 请求延迟分布                                   │
├─────────────────────────────────────────────────────┤
│  4. Pod Metrics Aggregator                          │
│     - 聚合所有任务 Pod 的指标                        │
│     - 统一暴露给 Prometheus                          │
└─────────────────────────────────────────────────────┘
```

**监控数据流向**:

```
Controller/Business Logic
       ↓
   Update Metrics (Prometheus Client SDK)
       ↓
   /metrics Endpoint (HTTP)
       ↓
   Prometheus Scrape (每 15 秒)
       ↓
   TSDB Storage (时间序列数据库)
       ↓
   Grafana Dashboard (可视化)
       ↓
   AlertManager (告警)
```

---

### 10.2 MetricExporter - 业务指标采集

**代码路径**: `pkg/metricexporter/`

#### 10.2.1 核心功能

MetricExporter 负责采集和暴露 Kuscia 的业务指标,包括:

1. **Job/Task 指标**:任务数量、成功率、执行时长等
2. **Domain 指标**:域数量、跨域通信状态
3. **Data 指标**:数据注册量、授权数量
4. **Controller 指标**:队列长度、同步延迟、重试次数

#### 10.2.2 工作原理

**指标聚合机制**:

```go
// pkg/metricexporter/metricexporter.go
func MetricExporter(ctx context.Context, metricURLs map[string]string, port int) {
    // 1. 收集静态配置的指标 URL
    // 2. 动态发现 Pod 中的指标端点
    podMetrics, _ := ListPodMetricUrls(podManager)
    
    // 3. 合并所有指标源
    metricURLs = combine(metricURLs, podMetrics)
    
    // 4. 启动 HTTP 服务器暴露 /metrics
    metricServer.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
        metricHandler(metricURLs, w)  // 并发抓取所有指标并聚合
    })
}
```

**Pod 指标自动发现**:

```go
func ListPodMetricUrls(podManager pod.Manager) (map[string]string, error) {
    metricUrls := map[string]string{}
    pods := podManager.GetPods()
    
    for _, pod := range pods {
        // 从 Annotation 中读取指标配置
        metricPath := pod.Annotations[common.MetricPathAnnotationKey]  // "/metrics"
        metricPort := pod.Annotations[common.MetricPortAnnotationKey]  // "9090"
        
        if metricPath != "" && metricPort != "" {
            // 构建完整的指标 URL
            url := fmt.Sprintf("http://%s:%s/%s", pod.Status.PodIP, metricPort, metricPath)
            metricUrls[pod.Name] = url
        }
    }
    return metricUrls
}
```

**并发指标抓取**:

```go
func metricHandler(metricURLs map[string]string, w http.ResponseWriter) {
    metricsChan := make(chan []byte, len(metricURLs))
    var wg sync.WaitGroup
    
    // 并发抓取所有 Pod 的指标
    for _, fullURL := range metricURLs {
        wg.Add(1)
        go func(fullURL string) {
            defer wg.Done()
            metrics, err := getMetrics(fullURL)
            if err == nil {
                metricsChan <- metrics
            } else {
                metricsChan <- nil  // 失败时发送空指标
            }
        }(fullURL)
    }
    
    // 等待所有请求完成
    go func() {
        wg.Wait()
        close(metricsChan)
    }()
    
    // 聚合所有指标并返回
    w.Header().Set("Content-Type", "text/plain")
    w.WriteHeader(http.StatusOK)
    for metrics := range metricsChan {
        if metrics != nil {
            w.Write(metrics)
        }
    }
}
```

#### 10.2.3 暴露的指标示例

**访问方式**:

```bash
curl http://localhost:9092/metrics
```

**典型输出**:

```prometheus
# HELP kuscia_job_requeue_count Counts number of KusciaJob requeue
# TYPE kuscia_job_requeue_count counter
kuscia_job_requeue_count{job_name="psi-job-001"} 3

# HELP kuscia_job_worker_queue_size Size of KusciaJob worker queue
# TYPE kuscia_job_worker_queue_size gauge
kuscia_job_worker_queue_size 5

# HELP kuscia_job_sync_durations_seconds Sync latency distributions of kuscia jobs.
# TYPE kuscia_job_sync_durations_seconds summary
kuscia_job_sync_durations_seconds{phase="Running",result="success",quantile="0.5"} 0.023
kuscia_job_sync_durations_seconds{phase="Running",result="success",quantile="0.9"} 0.045
kuscia_job_sync_durations_seconds{phase="Running",result="success",quantile="0.99"} 0.089

# HELP kuscia_job_result_stats Counts number of succeeded or failed kuscia jobs
# TYPE kuscia_job_result_stats counter
kuscia_job_result_stats{result="succeeded"} 127
kuscia_job_result_stats{result="failed"} 8

# HELP kuscia_task_duration_seconds Task execution duration
# TYPE kuscia_task_duration_seconds histogram
kuscia_task_duration_seconds_bucket{task_type="psi",le="10"} 50
kuscia_task_duration_seconds_bucket{task_type="psi",le="30"} 80
kuscia_task_duration_seconds_bucket{task_type="psi",le="60"} 95
kuscia_task_duration_seconds_bucket{task_type="psi",le="+Inf"} 100
```

---

### 10.3 Controller 层指标详解

#### 10.3.1 KusciaJob Controller 指标

**代码路径**: `pkg/controllers/kusciajob/metrics/metrics.go`

**指标列表**:

| 指标名称 | 类型 | 标签 | 说明 |
| --------- | ------ | ------ | ------ |
| `kuscia_job_requeue_count` | Counter | job_name | Job 重新入队次数(反映处理失败) |
| `kuscia_job_worker_queue_size` | Gauge | - | 工作队列当前长度 |
| `kuscia_job_sync_durations_seconds` | Summary | phase, result | 同步处理延迟分布(P50/P90/P99) |
| `kuscia_job_result_stats` | Counter | result | Job 成功/失败统计 |

**使用场景**:

- **requeue_count 激增**:控制器处理逻辑有问题或依赖资源未就绪
- **queue_size 持续增长**:处理能力不足,需要优化或扩容
- **sync_durations P99 过高**:存在慢查询或锁竞争
- **result_stats failed 增加**:任务失败率上升,需要排查原因

#### 10.3.2 KusciaTask Controller 指标

**代码路径**: `pkg/controllers/kusciatask/metrics/metrics.go`

**指标列表**:

| 指标名称 | 类型 | 标签 | 说明 |
| --------- | ------ | ------ | ------ |
| `kuscia_task_requeue_count` | Counter | task_name | Task 重新入队次数 |
| `kuscia_task_worker_queue_size` | Gauge | - | Task 工作队列长度 |
| `kuscia_task_sync_durations_seconds` | Summary | phase, result | Task 同步延迟 |
| `kuscia_task_result_stats` | Counter | result, task_type | Task 成功/失败统计(按类型) |
| `kuscia_task_duration_seconds` | Histogram | task_type, domain | Task 执行时长分布 |

#### 10.3.3 Domain Controller 指标

**代码路径**: `pkg/controllers/domain/metrics/metrics.go`

**指标列表**:

| 指标名称 | 类型 | 标签 | 说明 |
| --------- | ------ | ------ | ------ |
| `kuscia_domain_count` | Gauge | - | 注册的域总数 |
| `kuscia_domain_sync_errors` | Counter | domain_id | 域同步错误次数 |
| `kuscia_domainroute_active_count` | Gauge | source, target | 活跃的域间路由数 |

#### 10.3.4 TaskResourceGroup Controller 指标

**代码路径**: `pkg/controllers/taskresourcegroup/metrics/metrics.go`

**指标列表**:

| 指标名称 | 类型 | 标签 | 说明 |
| --------- | ------ | ------ | ------ |
| `kuscia_resource_reservation_duration_seconds` | Histogram | task_id | 资源预留时长 |
| `kuscia_resource_shortage_count` | Counter | resource_type | 资源不足事件计数 |

---

### 10.4 NodeExporter - 系统级指标

**端口**: 9100

NodeExporter 是 Prometheus 官方的节点导出器,Kuscia 直接复用其能力。

#### 10.4.1 采集的系统指标

**CPU 指标**:

```prometheus
# CPU 使用率
node_cpu_seconds_total{mode="user"}
node_cpu_seconds_total{mode="system"}
node_cpu_seconds_total{mode="idle"}

# CPU 负载
node_load1   # 1分钟平均负载
node_load5   # 5分钟平均负载
node_load15  # 15分钟平均负载
```

**内存指标**:

```prometheus
# 内存使用情况
node_memory_MemTotal_bytes
node_memory_MemFree_bytes
node_memory_MemAvailable_bytes
node_memory_Buffers_bytes
node_memory_Cached_bytes

# Swap 使用情况
node_memory_SwapTotal_bytes
node_memory_SwapFree_bytes
```

**磁盘指标**:

```prometheus
# 文件系统使用
node_filesystem_size_bytes{mountpoint="/"}
node_filesystem_avail_bytes{mountpoint="/"}
node_filesystem_free_bytes{mountpoint="/"}

# 磁盘 I/O
node_disk_read_bytes_total{device="sda"}
node_disk_written_bytes_total{device="sda"}
node_disk_io_time_seconds_total{device="sda"}
```

**网络指标**:

```prometheus
# 网络流量
node_network_receive_bytes_total{device="eth0"}
node_network_transmit_bytes_total{device="eth0"}
node_network_receive_packets_total{device="eth0"}
node_network_transmit_packets_total{device="eth0"}

# 网络连接
node_sockstat_TCP_alloc
node_sockstat_TCP_inuse
node_sockstat_TCP_tw
```

#### 10.4.2 关键告警规则

```yaml
# Prometheus Alerting Rules
groups:
- name: node_alerts
  rules:
  - alert: HighCPUUsage
    expr: 100 - (avg by(instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100) > 80
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "CPU usage is above 80%"
      description: "{{ $labels.instance }} CPU usage is {{ $value }}%"
  
  - alert: HighMemoryUsage
    expr: (1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)) * 100 > 85
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Memory usage is above 85%"
  
  - alert: DiskSpaceLow
    expr: (node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"}) * 100 < 15
    for: 10m
    labels:
      severity: critical
    annotations:
      summary: "Disk space is below 15%"
  
  - alert: HighNetworkLatency
    expr: rate(node_network_receive_errs_total[5m]) > 10
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High network error rate"
```

---

### 10.5 SsExporter - Envoy 指标

**代码路径**: `pkg/ssexporter/`
**端口**: 9101

SsExporter 专门用于采集 Envoy Gateway 的性能指标。

#### 10.5.1 采集的指标

**连接池指标**:

```prometheus
# 活跃连接数
envoy_http_downstream_cx_active

# 连接建立速率
envoy_http_downstream_cx_total

# 连接关闭速率
envoy_http_downstream_cx_destroy
```

**请求指标**:

```prometheus
# 请求总数
envoy_http_downstream_rq_total

# 请求延迟分布
envoy_http_downstream_rq_time_bucket{le="0.005"}
envoy_http_downstream_rq_time_bucket{le="0.01"}
envoy_http_downstream_rq_time_bucket{le="0.025"}
envoy_http_downstream_rq_time_bucket{le="0.05"}
envoy_http_downstream_rq_time_bucket{le="0.1"}
envoy_http_downstream_rq_time_bucket{le="+Inf"}

# 响应码分布
envoy_http_downstream_rq_xx{response_code_class="2"}  # 2xx
envoy_http_downstream_rq_xx{response_code_class="4"}  # 4xx
envoy_http_downstream_rq_xx{response_code_class="5"}  # 5xx
```

**带宽指标**:

```prometheus
# 接收字节数
envoy_http_downstream_cx_rx_bytes_total

# 发送字节数
envoy_http_downstream_cx_tx_bytes_total
```

#### 10.5.2 Envoy Admin API 集成

SsExporter 通过 Envoy 的 Admin API 获取统计数据:

```go
// pkg/ssexporter/ssexporter.go
func collectEnvoyStats(envoyAdminURL string) ([]byte, error) {
    // 访问 Envoy Admin API
    resp, err := http.Get(fmt.Sprintf("%s/stats?format=prometheus", envoyAdminURL))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    return ioutil.ReadAll(resp.Body)
}
```

**Envoy Admin 端口**: 默认 9901

---

### 10.6 Grafana Dashboard

#### 10.6.1 推荐 Dashboard

Kuscia 提供了以下预置 Dashboard:

**1. Kuscia Overview** (`kuscia-overview.json`)

- Job/Task 执行概览
- 成功率趋势
- 资源使用情况

**2. Kuscia Job Detail** (`kuscia-job-detail.json`)

- 单个 Job 的详细指标
- 各 Task 执行时长
- 参与方资源消耗

**3. Kuscia Network** (`kuscia-network.json`)

- 域间通信延迟
- 带宽使用情况
- Envoy 连接池状态

**4. Node Resources** (`node-resources.json`)

- CPU/内存/磁盘使用率
- 网络流量
- 文件系统状态

#### 10.6.2 导入 Dashboard

```bash
# 通过 Grafana UI 导入
1. 登录 Grafana (默认 http://localhost:3000)
2. 点击 "Create" → "Import"
3. 上传 JSON 文件或粘贴内容
4. 选择 Prometheus 数据源
5. 点击 "Import"

# 或通过 API 导入
curl -X POST http://localhost:3000/api/dashboards/import \
  -H "Content-Type: application/json" \
  -d @kuscia-overview.json
```

#### 10.6.3 关键面板示例

**Job 成功率趋势**:

```
Panel Title: Job Success Rate (24h)
Query: 
  sum(rate(kuscia_job_result_stats{result="succeeded"}[24h])) 
  / 
  sum(rate(kuscia_job_result_stats[24h]))
Visualization: Time series
Thresholds: Warning < 95%, Critical < 90%
```

**Task 执行时长分布**:

```
Panel Title: Task Duration Distribution
Query:
  histogram_quantile(0.95, 
    sum(rate(kuscia_task_duration_seconds_bucket[5m])) by (le, task_type))
Visualization: Heatmap
Group by: task_type
```

**域间通信延迟**:

```
Panel Title: Cross-Domain Latency P95
Query:
  histogram_quantile(0.95,
    sum(rate(envoy_http_downstream_rq_time_bucket[5m])) by (le, source_domain, target_domain))
Visualization: Time series
Legend: {{source_domain}} → {{target_domain}}
```

---

### 10.7 诊断模块

**代码路径**: `pkg/diagnose/`

诊断模块提供了一系列工具来检测和定位问题,支持命令行和 API 两种调用方式。

#### 10.7.1 诊断架构

```
┌─────────────────────────────────────────────┐
│         Diagnose CLI / API                   │
└──────────────┬──────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────┐
│        Diagnose Server (每个节点运行)         │
│  - 接收诊断请求                              │
│  - 执行诊断任务                              │
│  - 返回诊断结果                              │
└──────────────┬──────────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────────┐
│        Diagnose Tasks                        │
├─────────────────────────────────────────────┤
│  1. Network Diagnosis                        │
│     - Latency (RTT)                         │
│     - Bandwidth                             │
│     - Connection                            │
│     - Request Size                          │
├─────────────────────────────────────────────┤
│  2. Log Analysis                             │
│     - Envoy Log Parsing                     │
│     - Error Pattern Detection               │
├─────────────────────────────────────────────┤
│  3. Resource Check                           │
│     - CRD Status                            │
│     - DomainRoute Status                    │
│     - Pod Status                            │
└─────────────────────────────────────────────┘
```

#### 10.7.2 网络诊断

**代码路径**: `pkg/diagnose/app/netstat/`

##### A. 延迟诊断 (Latency/RTT)

**原理**:客户端向服务端发送 100 次请求,计算平均往返时间(RTT)。

**实现**:

```go
// pkg/diagnose/app/netstat/latency.go
type LatencyTask struct {
    Client    *client.Client
    threshold int  // 阈值(默认 50ms)
    output    *TaskOutput
}

func (t *LatencyTask) Run(ctx context.Context) {
    req := &diagnose.MockRequest{
        Duration: 0,  // 立即返回
    }
    
    var duration time.Duration
    var success int
    
    // 发送 100 次请求
    for i := 0; i < 100; i++ {
        start := time.Now()
        if _, err = t.Client.Mock(ctx, req); err != nil {
            nlog.Errorf("Mock error: %v", err)
        } else {
            duration += time.Since(start)
            success++
        }
    }
    
    // 计算平均延迟
    latency := float64(duration.Milliseconds()) / float64(success)
    
    // 判断是否达标
    if latency <= float64(t.threshold) {
        t.output.Result = common.Pass
    } else {
        t.output.Result = common.Warning
        t.output.Information = fmt.Sprintf("not satisfy threshold %vms", t.threshold)
    }
}
```

**使用方法**:

```bash
# 诊断 alice 到 bob 的网络延迟
kuscia diagnose network alice bob --type latency --threshold 50

# 输出示例
Diagnosis Result:
  Task: RTT
  Threshold: 50ms
  Detected Value: 35ms
  Result: PASS ✓
```

**API 调用**:

```bash
curl -X POST http://localhost:8082/api/v1/kusciaapi/diagnose/network \
  -H "Content-Type: application/json" \
  -d '{
    "source_domain": "alice",
    "target_domain": "bob",
    "type": "latency",
    "threshold": 50
  }'
```

##### B. 带宽诊断 (Bandwidth)

**原理**:服务端持续发送 100KB 的分块数据 10 秒,客户端记录接收到的总数据量,计算带宽。

**实现**:

```go
// pkg/diagnose/app/netstat/bandwidth.go
type BandWidthTask struct {
    client    *client.Client
    threshold int  // 阈值(默认 10 Mbps)
    output    *TaskOutput
}

func (t *BandWidthTask) Run(ctx context.Context) {
    // 请求服务端发送 100KB chunked 数据,持续 10 秒
    req := &diagnose.MockRequest{
        ChunkedSize:   100 << 10,  // 100KB
        Duration:      10000,      // 10s
        EnableChunked: true,
    }
    
    url := fmt.Sprintf("http://%v/diagnose/mock", t.client.HostName)
    resp, err := t.client.MockChunk(req, url)
    if err != nil {
        t.output.Result = common.Fail
        return
    }
    defer resp.Body.Close()
    
    // 记录接收到的数据量
    size := t.RecordData(resp)
    
    // 转换为 Mbps
    result := ToMbps(size)
    
    // 判断是否达标
    if result > float64(t.threshold) {
        t.output.Result = common.Pass
    } else {
        t.output.Result = common.Warning
        t.output.Information = fmt.Sprintf("not satisfy threshold %vMbps", t.threshold)
    }
}
```

**使用方法**:

```bash
# 诊断 alice 到 bob 的带宽
kuscia diagnose network alice bob --type bandwidth --threshold 10

# 输出示例
Diagnosis Result:
  Task: BANDWIDTH
  Threshold: 10Mbits/sec
  Detected Value: 85.5Mbits/sec
  Result: PASS ✓
```

##### C. 连接诊断 (Connection)

**功能**:检测 TCP 连接建立的成功率和耗时。

**检测项**:

- TCP 三次握手耗时
- 连接超时率
- 连接重置次数

##### D. 请求大小诊断 (Request Size)

**功能**:测试不同大小的请求传输性能。

**测试场景**:

- 小请求:1KB
- 中请求:100KB
- 大请求:1MB
- 超大请求:10MB

#### 10.7.3 日志分析诊断

**代码路径**: `pkg/diagnose/app/netstat/task_analysis.go`

**功能**:分析 Envoy 日志,检测异常模式。

**检测项**:

1. **超时模式**:
   - 检测频繁的 upstream timeout
   - 识别慢响应接口

2. **错误码分布**:
   - 5xx 错误率突增
   - 4xx 客户端错误模式

3. **重试风暴**:
   - 检测频繁的重试行为
   - 识别不健康的后端

**使用方法**:

```bash
# 分析指定任务的日志
kuscia diagnose log psi-job-001 --task psi-compute

# 输出示例
Log Analysis Report:
  Total Requests: 1523
  Success (2xx): 1480 (97.2%)
  Client Error (4xx): 30 (1.9%)
  Server Error (5xx): 13 (0.9%)
  
  Timeout Events: 5
  Retry Storms: 0
  
  Top Slow Endpoints:
    1. /secretflow.psi.compute - avg 2.3s
    2. /secretflow.data.load - avg 1.8s
```

#### 10.7.4 资源状态诊断

##### A. DomainRoute 状态检查

**功能**:检查域间路由配置是否正确。

**检查项**:

- DomainRoute CR 是否存在
- ClusterDomainRoute CR 是否同步
- Envoy Route 配置是否生效
- TLS 证书是否有效

**使用方法**:

```bash
# 检查 alice 到 bob 的路由
kuscia diagnose cdr alice bob

# 输出示例
ClusterDomainRoute Diagnosis:
  DomainRoute: ✓ Found
  ClusterDomainRoute: ✓ Synced
  Envoy Config: ✓ Applied
  TLS Certificate: ✓ Valid (expires in 180 days)
  Connectivity: ✓ PASS (RTT: 35ms)
```

##### B. CRD 资源检查

**功能**:检查关键 CRD 资源的状态。

**检查项**:

- DomainData 是否注册
- DomainDataGrant 是否授权
- KusciaJob/KusciaTask 状态
- Pod 运行状态

**使用方法**:

```bash
# 检查指定域的资源
kuscia diagnose resources alice

# 输出示例
Resource Check Report:
  Domains: 3 registered
  DomainDatas: 15 registered
  DomainDataGrants: 8 active
  Active Jobs: 2 running
  Failed Jobs (24h): 1
  Pods: 5 running, 0 pending, 0 failed
```

#### 10.7.5 综合诊断命令

**一键诊断**:

```bash
# 全面诊断系统健康状态
kuscia diagnose all

# 输出示例
Comprehensive Diagnosis Report:
========================================

1. Node Health:
   CPU Usage: 45% ✓
   Memory Usage: 62% ✓
   Disk Usage: 58% ✓
   
2. Network Connectivity:
   alice ↔ bob: RTT 35ms ✓, BW 85Mbps ✓
   alice ↔ charlie: RTT 42ms ✓, BW 78Mbps ✓
   
3. DomainRoutes:
   alice → bob: ✓ Active
   alice → charlie: ✓ Active
   
4. Recent Jobs:
   Success Rate (24h): 98.5% ✓
   Average Duration: 45s
   
5. Errors:
   No critical errors detected
   
Overall Status: HEALTHY ✓
```

---

### 10.8 常用诊断命令速查

| 命令 | 功能 | 示例 |
| ------ | ------ | ------ |
| `kuscia diagnose network` | 网络诊断 | `kuscia diagnose network alice bob --type latency` |
| `kuscia diagnose cdr` | 路由检查 | `kuscia diagnose cdr alice bob` |
| `kuscia diagnose log` | 日志分析 | `kuscia diagnose log job-001` |
| `kuscia diagnose resources` | 资源检查 | `kuscia diagnose resources alice` |
| `kuscia diagnose all` | 综合诊断 | `kuscia diagnose all` |

---

### 10.9 告警配置

#### 10.9.1 Prometheus AlertManager 配置

```yaml
# alertmanager.yml
global:
  resolve_timeout: 5m

route:
  group_by: ['alertname', 'domain']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  receiver: 'default-receiver'

receivers:
- name: 'default-receiver'
  email_configs:
  - to: 'ops@example.com'
    from: 'alertmanager@example.com'
    smarthost: 'smtp.example.com:587'
  webhook_configs:
  - url: 'http://notification-service:8080/alerts'
```

#### 10.9.2 关键告警规则

```yaml
# kuscia-alerts.yml
groups:
- name: kuscia_business_alerts
  rules:
  # Job 失败率过高
  - alert: HighJobFailureRate
    expr: |
      sum(rate(kuscia_job_result_stats{result="failed"}[1h]))
      /
      sum(rate(kuscia_job_result_stats[1h]))
      > 0.1
    for: 15m
    labels:
      severity: critical
    annotations:
      summary: "Job failure rate is above 10%"
      description: "Current failure rate: {{ $value | humanizePercentage }}"
  
  # Task 执行时间过长
  - alert: TaskExecutionTooSlow
    expr: |
      histogram_quantile(0.95,
        sum(rate(kuscia_task_duration_seconds_bucket[5m])) by (le, task_type))
      > 300
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Task execution time P95 is above 5 minutes"
  
  # 域间通信延迟过高
  - alert: HighCrossDomainLatency
    expr: |
      histogram_quantile(0.95,
        sum(rate(envoy_http_downstream_rq_time_bucket[5m])) by (le))
      > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Cross-domain latency P95 is above 100ms"
  
  # Controller 队列积压
  - alert: ControllerQueueBacklog
    expr: kuscia_job_worker_queue_size > 100
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Job controller queue size is above 100"
  
  # 资源预留失败
  - alert: ResourceReservationFailure
    expr: rate(kuscia_resource_shortage_count[15m]) > 0
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Resource reservation failures detected"

- name: kuscia_infrastructure_alerts
  rules:
  # etcd 健康检查
  - alert: EtcdUnhealthy
    expr: etcd_server_has_leader == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "etcd has no leader"
  
  # API Server 不可用
  - alert: APIServerDown
    expr: up{job="k3s-apiserver"} == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "K3s API Server is down"
  
  # Agent 离线
  - alert: AgentOffline
    expr: up{job="kuscia-agent"} == 0
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "Kuscia Agent is offline"
```

---

### 10.10 性能调优建议

#### 10.10.1 Controller 性能优化

**1. 调整 Informer 缓存刷新频率**:

```yaml
controller:
  resync_period: "30s"  # 默认 10h,可降低以提高响应速度
```

**2. 增加 Worker 并发数**:

```yaml
controller:
  workers: 10  # 默认 5,可根据负载增加
```

**3. 优化 Requeue 策略**:

```go
// 指数退避重試
backoff := wait.Backoff{
    Duration: 1 * time.Second,
    Factor: 2.0,
    Jitter: 0.1,
    Steps: 6,
}
```

#### 10.10.2 监控采集优化

**1. 调整 Scrape 间隔**:

```yaml
# prometheus.yml
scrape_configs:
- job_name: 'kuscia-metrics'
  scrape_interval: 15s  # 默认 1m,可降低以提高实时性
  scrape_timeout: 10s
```

**2. 启用指标过滤**:

```yaml
metric_relabel_configs:
- source_labels: [__name__]
  regex: 'kuscia_.*_bucket'
  action: drop  # 丢弃直方图 bucket 以减少存储
```

**3. 限制保留时间**:

```yaml
# prometheus.yml
storage:
  tsdb:
    retention.time: 15d  # 默认无限,建议设置上限
    retention.size: 50GB
```

#### 10.10.3 日志管理优化

**1. 日志轮转配置**:

```yaml
logrotate:
  max_file_size_mb: 512
  max_files: 5
  compress: true
```

**2. 日志级别动态调整**:

```bash
# 运行时调整日志级别
curl -X POST http://localhost:8082/api/v1/kusciaapi/config/set \
  -d '{"key": "log-level", "value": "debug"}'
```

**3. 结构化日志**:

```json
{
  "timestamp": "2024-01-01T12:00:00Z",
  "level": "info",
  "component": "kusciajob-controller",
  "job_id": "psi-job-001",
  "phase": "Running",
  "duration_ms": 23,
  "message": "Sync job successfully"
}
```

---

### 10.11 故障排查手册

#### 10.11.1 常见问题及解决方案

**问题 1: Job 一直处于 PendingApproval 状态**

**排查步骤**:

```bash
# 1. 检查 Job 状态
kubectl get kusciajob psi-job-001 -o yaml

# 2. 检查审批状态
kubectl describe kusciajob psi-job-001 | grep -A 10 "Approval"

# 3. 检查参与方是否收到审批请求
kubectl get domaindatagrant -A | grep psi-job-001

# 4. 手动审批(如果需要)
kuscia api approve-job --job-id psi-job-001 --approve true
```

**可能原因**:

- 参与方未配置审批回调
- 网络不通导致审批请求未送达
- 权限不足无法审批

---

**问题 2: Task 执行失败,错误信息不明确**

**排查步骤**:

```bash
# 1. 查看 Task 状态
kubectl get kusciatask <task-id> -o yaml

# 2. 查看 Task 事件
kubectl describe kusciatask <task-id>

# 3. 查看 Pod 状态
kubectl get pods -l kuscia.secretflow/task-id=<task-id>

# 4. 查看 Pod 日志
kubectl logs <pod-name> -c <container-name>

# 5. 查看上一个容器的日志(如果容器重启)
kubectl logs <pod-name> -c <container-name> --previous

# 6. 进入容器调试
kubectl exec -it <pod-name> -c <container-name> -- /bin/bash
```

**常见错误**:

- `ImagePullBackOff`: 镜像拉取失败,检查镜像名称和网络
- `CrashLoopBackOff`: 容器启动后立即退出,检查应用日志
- `OOMKilled`: 内存超限,增加 memory limit
- `Error`: 应用内部错误,查看详细日志

---

**问题 3: 域间通信超时**

**排查步骤**:

```bash
# 1. 诊断网络延迟
kuscia diagnose network alice bob --type latency

# 2. 诊断带宽
kuscia diagnose network alice bob --type bandwidth

# 3. 检查 DomainRoute
kubectl get domainroute alice-to-bob -o yaml

# 4. 检查 Envoy 配置
kubectl exec -it <gateway-pod> -- curl localhost:9901/config_dump

# 5. 检查 Envoy 日志
kubectl logs <gateway-pod> | grep "upstream timeout"

# 6. 检查 TLS 证书
openssl s_client -connect <target-endpoint>:443 -showcerts
```

**可能原因**:

- 网络延迟过高(>100ms)
- 带宽不足
- Envoy 路由配置错误
- TLS 证书过期
- 防火墙阻止连接

---

**问题 4: 资源不足导致任务调度失败**

**排查步骤**:

```bash
# 1. 检查节点资源
kubectl top nodes

# 2. 检查命名空间资源配额
kubectl describe resourcequota -n alice

# 3. 检查 TaskResourceGroup 状态
kubectl get taskresourcegroup <trg-name> -o yaml

# 4. 查看资源预留事件
kubectl get events -n alice | grep "Insufficient"

# 5. 检查容量配置
cat /home/kuscia/etc/conf/agent.yaml | grep -A 10 "capacity"
```

**解决方案**:

```yaml
# 调整容量配置
capacity:
  cpu: "16"        # 增加 CPU
  memory: "32Gi"   # 增加内存
  
# 或调整保留资源
reserved_resources:
  cpu: "0.25"      # 减少保留 CPU
  memory: "256Mi"  # 减少保留内存
```

---

**问题 5: Prometheus 无法抓取指标**

**排查步骤**:

```bash
# 1. 检查 MetricExporter 是否运行
ps aux | grep metricexporter

# 2. 测试指标端点
curl http://localhost:9092/metrics

# 3. 检查 Prometheus Target 状态
# 访问 http://prometheus-server:9090/targets

# 4. 查看 Prometheus 日志
kubectl logs <prometheus-pod> | grep "scrape"

# 5. 检查网络连接
telnet localhost 9092
```

**可能原因**:

- MetricExporter 未启动
- 端口被防火墙阻止
- Prometheus 配置错误
- 网络隔离

---

#### 10.11.2 诊断工具集

**内置诊断命令**:

```bash
# 查看所有诊断命令
kuscia diagnose --help

# 诊断特定模块
kuscia diagnose network --help
kuscia diagnose log --help
kuscia diagnose resources --help
```

**自定义诊断脚本**:

```bash
#!/bin/bash
# diagnose-kuscia.sh

echo "=== Kuscia System Diagnosis ==="
echo ""

echo "1. Checking K3s status..."
kubectl cluster-info

echo ""
echo "2. Checking node resources..."
kubectl top nodes

echo ""
echo "3. Checking running jobs..."
kubectl get kusciajobs -A

echo ""
echo "4. Checking failed tasks (last 24h)..."
kubectl get kusciatasks -A --field-selector status.phase=Failed

echo ""
echo "5. Checking domain routes..."
kubectl get domainroutes -A

echo ""
echo "6. Checking pod status..."
kubectl get pods -A | grep -v "Running\|Completed"

echo ""
echo "=== Diagnosis Complete ==="
```

---

### 10.12 监控最佳实践

#### 10.12.1 指标命名规范

**推荐格式**:

```
<子系统>_<对象>_<动作>_<单位>

示例:
kuscia_job_sync_durations_seconds
envoy_http_downstream_rq_time_bucket
node_cpu_seconds_total
```

#### 10.12.2 标签设计原则

**好标签**:

- 基数有限(如 `domain_id`, `task_type`)
- 含义明确(如 `result=succeeded/failed`)
- 维度稳定(不会频繁变化)

**避免的标签**:

- 高基数标签(如 `job_id`, `pod_name`)→ 改用 Counter + Delete
- 时间戳(如 `timestamp`)→ Prometheus 自带时间
- 连续值(如 `latency_ms`)→ 改用 Histogram bucket

#### 10.12.3 告警分级策略

| 级别 | 响应时间 | 通知方式 | 示例 |
| ------ | --------- | --------- | ------ |
| **Critical** | 5 分钟内 | 电话+短信+邮件 | etcd 宕机、API Server 不可用 |
| **Warning** | 30 分钟内 | 邮件+IM | Job 失败率>10%、CPU>80% |
| **Info** | 工作时间 | 邮件 | 证书即将过期、版本更新 |

#### 10.12.4 Dashboard 设计原则

**分层设计**:

1. **Overview**:全局概览(5-10 个关键指标)
2. **Detail**:模块详情(20-30 个指标)
3. **Troubleshoot**:故障排查(原始指标+日志)

**可视化选择**:

- 趋势:Time Series
- 分布:Histogram/Heatmap
- 占比:Pie Chart
- 状态:Stat/Gauge
- 关系:Graph/Node Graph

---

### 10.13 监控数据安全

#### 10.13.1 指标脱敏

**避免暴露敏感信息**:

```prometheus
# ❌ 错误:包含敏感数据
request_total{user_email="alice@example.com"}

# ✅ 正确:使用匿名标识
request_total{user_id="usr_abc123"}
```

#### 10.13.2 访问控制

**Prometheus 认证**:

```yaml
# prometheus.yml
web:
  basic_auth_users:
    admin: $2y$10$...
  read_only_users:
    viewer: $2y$10$...
```

**Grafana RBAC**:

```
角色:
- Admin: 完整权限
- Editor: 创建/编辑 Dashboard
- Viewer: 只读查看
```

#### 10.13.3 数据加密

**HTTPS 传输**:

```yaml
# grafana.ini
[server]
protocol = https
cert_file = /etc/grafana/certs/cert.pem
cert_key = /etc/grafana/certs/key.pem
```

---

## 11. 嵌入式 K3s 架构

### 11.1 为什么使用嵌入式 K3s？

Kuscia 采用 **"自带 Kubernetes"** 的设计理念，通过嵌入式 K3s 实现：

- ✅ **无需外部 K8s 集群**：单机即可运行完整的隐私计算节点
- ✅ **统一的资源抽象**：所有业务对象都通过 CRD 管理
- ✅ **成熟的生态复用**：利用 K8s 的 API、存储、权限控制等能力
- ✅ **轻量级部署**：最低 1C2G 即可运行

### 11.2 K3s 在 Kuscia 中的定位

```
┌─────────────────────────────────────────────┐
│         任意 Linux 环境（容器内外均可）       │
│                                              │
│  ┌──────────────────────────────────────┐   │
│  │      Kuscia 主进程                    │   │
│  │                                      │   │
│  │  ┌────────────────────────────────┐  │   │
│  │  │  嵌入式 K3s (子进程)            │  │   │
│  │  │  - API Server ✅               │  │   │
│  │  │  - etcd ✅                     │  │   │
│  │  │  - Controller Manager ✅       │  │   │
│  │  └────────────────────────────────┘  │   │
│  │                                      │   │
│  │  ┌────────────────────────────────┐  │   │
│  │  │  CRD 业务控制器                  │  │   │
│  │  │  - DomainData Controller       │  │   │
│  │  │  - DomainDataGrant Controller  │  │   │
│  │  │  - KusciaJob Controller        │  │   │
│  │  │  - KusciaTask Controller       │  │   │
│  │  └────────────────────────────────┘  │   │
│  └──────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
```

**关键点**：

- K3s 是 Kuscia 的**子进程**，通过 `exec.Command` 启动
- 生命周期绑定：Kuscia 退出时 K3s 也会停止
- Supervisor 管理：K3s 崩溃时自动重启
- 完全独立：不依赖外部 K8s 集群

### 11.3 K3s 精简配置

Kuscia 禁用了大量不必要的 K8s 组件，只保留核心功能：

```go
// cmd/kuscia/modules/k3s.go
args := []string{
"server",
"--disable-agent",              // 禁用 Kubelet
"--disable-scheduler",          // 禁用默认调度器
"--flannel-backend=none",       // 禁用网络插件
"--disable=traefik",            // 禁用 Ingress
"--disable=coredns",            // 禁用 DNS（使用自定义 CoreDNS）
"--disable=servicelb",          // 禁用负载均衡
"--disable=local-storage",      // 禁用本地存储
"--disable=metrics-server",     // 禁用监控
}

// 非 root 用户启用 rootless 模式
if !pkgcom.IsRootUser() {
args = append(args, "--rootless")
}
```

**启用的组件**：

- ✅ API Server：提供 RESTful API
- ✅ etcd：存储所有 CRD 对象
- ✅ Controller Manager：运行内置控制器（Namespace、ServiceAccount 等）

**禁用的组件**：

- ❌ Scheduler：Kuscia 有自己的调度器
- ❌ Kubelet：Autonomy Mode 不需要节点代理
- ❌ CNI：不使用 Pod 网络
- ❌ CoreDNS：使用自定义 DNS 方案
- ❌ Metrics Server：使用自定义监控

### 11.4 运行在 K3s 中的功能

#### A. CRD 资源管理（核心功能）

所有业务领域的 CRD 都存储在 K3s 的 etcd 中：

| CRD | 作用域 | 控制器 |
| ----- | -------- | -------- |
| DomainData | Namespaced | DomainData Controller |
| DomainDataGrant | Namespaced | DomainData Controller |
| KusciaJob | Namespaced (cross-domain) | KusciaJob Controller |
| KusciaTask | Namespaced (cross-domain) | KusciaTask Controller |
| Domain | Cluster | Domain Controller |
| DomainRoute | Namespaced | DomainRoute Controller |
| ClusterDomainRoute | Cluster | ClusterDomainRoute Controller |
| AppImage | Cluster | KusciaTask Controller |
| TaskResourceGroup | Namespaced | TaskResourceGroup Controller |
| KusciaDeployment | Namespaced | KusciaDeployment Controller |

#### B. Namespace 隔离

每个参与方有独立的 Namespace：

```bash
kubectl get namespaces

NAME              STATUS   AGE
alice             Active   10d
bob               Active   10d
charlie           Active   10d
kuscia-system     Active   10d
```

**作用**：

- 数据隔离：不同域的数据在不同 namespace
- 权限隔离：RBAC 按 namespace 授权
- 资源隔离：可以限制每个 namespace 的资源配额

#### C. ConfigMap/Secret 配置管理

```yaml
# 存储敏感配置（如数据库密码、证书）
apiVersion: v1
kind: Secret
metadata:
  name: domain-key
  namespace: alice
type: Opaque
data:
  private-key: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQ==...
---
# 存储非敏感配置
apiVersion: v1
kind: ConfigMap
metadata:
  name: kuscia-config
  namespace: kuscia-system
data:
  log-level: "info"
  run-mode: "Autonomy"
```

#### D. ServiceAccount & RBAC

```yaml
# 为每个域创建 ServiceAccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: alice-sa
  namespace: alice
---
# 绑定权限
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: alice-binding
  namespace: alice
subjects:
  - kind: ServiceAccount
    name: alice-sa
roleRef:
  kind: Role
  name: domain-data-admin
```

### 11.5 不运行在 K3s 中的功能

以下功能由 **Kuscia 自己实现**，不依赖 K8s：

| 功能 | 实现方式 | 说明 |
| ------ | --------- | ------ |
| **任务调度** | Kuscia Scheduler | 自己的调度算法（考虑数据位置、资源等） |
| **容器运行时** | RunK/RunP | RunK 调用 containerd，RunP 直接启动进程 |
| **网络通信** | Kuscia Transport | 自己的 gRPC/HTTP 通信框架 |
| **服务暴露** | Kuscia Gateway | 基于 Envoy 的网关 |
| **监控告警** | Prometheus + 自定义 Exporter | 不使用 K8s Metrics Server |
| **日志收集** | 本地文件 + Lumberjack | 不使用 EFK/ELK |
| **镜像管理** | Kuscia Image Command | 自己的镜像导入导出工具 |

### 11.6 Kuscia 如何管理 K3s

#### A. 生命周期管理

**启动流程**：

```go
// cmd/kuscia/modules/k3s.go
func (s *k3sModule) Run(ctx context.Context) error {
// 1. 检查数据存储端点
err := datastore.CheckDatastoreEndpoint(s.datastoreEndpoint)

// 2. 构建启动参数（根据配置动态调整）
args := s.buildK3sArgs()

// 3. 创建 Supervisor 管理进程
sp := supervisor.NewSupervisor("k3s", nil, -1)

// 4. 启动 K3s 子进程
err = sp.Run(ctx, func(ctx context.Context) supervisor.Cmd {
cmd := exec.Command(filepath.Join(s.rootDir, "bin/k3s"), args...)
return &ModuleCMD{cmd: cmd}
})

return err
}
```

**就绪检查**：

```go
func (s *k3sModule) startCheckReady(ctx context.Context) error {
// 等待 kubeconfig 文件生成
for i := 0; i < 60; i++ {
if _, err := os.Stat(s.kubeconfigFile); err == nil {
break
}
time.Sleep(1 * time.Second)
}

// 创建客户端并测试连接
clients, _ := kubeconfig.CreateClientSetsFromKubeconfig(...)
_, err = clients.KubeClient.Discovery().ServerVersion()

// 初始化 Kuscia 环境
s.initKusciaEnvAfterReady(ctx)

return nil
}
```

#### B. CRD 自动注册

Kuscia 启动时自动注册所有需要的 CRD：

```go
func (s *k3sModule) initKusciaEnvAfterReady(ctx context.Context) error {
crdFiles := []string{
"crds/v1alpha1/kuscia.secretflow_domaindatas.yaml",
"crds/v1alpha1/kuscia.secretflow_domaindatagrants.yaml",
"crds/v1alpha1/kuscia.secretflow_domains.yaml",
"crds/v1alpha1/kuscia.secretflow_kusciajobs.yaml",
// ... 更多 CRD
}

for _, crdFile := range crdFiles {
// 执行 kubectl apply -f <crd_file>
cmd := exec.Command(kubectlPath, "--kubeconfig", s.kubeconfigFile,
"apply", "-f", crdFile)
cmd.Run()
}

return nil
}
```

#### C. Namespace 自动创建

```go
func (s *k3sModule) initKusciaEnvAfterReady(ctx context.Context) error {
// 创建 Domain 对应的 Namespace
domainNS := &corev1.Namespace{
ObjectMeta: metav1.ObjectMeta{
Name: s.conf.DomainID,  // 例如 "alice"
},
}
clients.KubeClient.CoreV1().Namespaces().Create(ctx, domainNS)

// 创建 ServiceAccount
sa := &corev1.ServiceAccount{...}
clients.KubeClient.CoreV1().ServiceAccounts(...).Create(ctx, sa)

return nil
}
```

#### D. 控制器管理器

```go
// cmd/kuscia/modules/controllers.go
func NewControllersModule(i *ModuleRuntimeConfigs) (Module, error) {
opt := &controllers.Options{
ControllerName: "kuscia-controller-manager",
HealthCheckPort: 8090,
Workers:         4,  // 4 个工作协程
}

// 创建控制器服务器
return controllers.NewServer(
opt, i.Clients,
[]controllers.ControllerConstruction{
{NewController: domaindata.NewController, ...},
{NewController: kusciajob.NewController, ...},
{NewController: kusciatask.NewController, ...},
// ... 更多控制器
},
), nil
}
```

### 11.7 数据存储

**嵌入式 etcd**（默认）：

```text
/var/lib/kuscia/data/k3s/server/db/
├── etcd/
│   ├── member/
│   │   ├── snap/          # Raft 快照
│   │   └── wal/           # Write-Ahead Log
│   └── etcd.db
```

**外部 datastore**（生产环境推荐）：

```yaml
master:
  datastoreEndpoint: "mysql://user:pass@host:3306/kuscia_db"
  # 或
  datastoreEndpoint: "postgres://user:pass@host:5432/kuscia_db"
  # 或
  datastoreEndpoint: "etcd://host:2379"
```

### 11.8 监控和诊断

**健康检查端点**：

```http
GET http://localhost:8090/healthz

# 返回示例
{
  "status": "ok",
  "controllers": {
    "domaindata": "healthy",
    "kusciajob": "healthy",
    "kusciatask": "healthy"
  },
  "k3s": {
    "apiserver": "healthy",
    "etcd": "healthy"
  }
}
```

**日志分离**：

```text
/var/lib/kuscia/logs/
├── kuscia.log          # Kuscia 主进程日志
├── k3s.log             # K3s 进程日志
├── k3s-audit.log       # K8s API 审计日志
└── controller.log      # 控制器日志
```

---

## 12. CRD 代码生成机制

### 12.1 为什么需要代码生成？

Kuscia 的核心业务对象都是基于 CRD 实现的。每新增一个 CRD，需要生成：

- **Clientset**：用于操作 Kubernetes API 的客户端代码
- **Listers**：提供带缓存的列表查询功能
- **Informers**：监听资源变化的事件驱动机制
- **DeepCopy**：对象的深拷贝方法实现

这些代码高度模式化，手动编写容易出错且难以维护。

### 12.2 代码生成工具链

Kuscia 使用 Kubernetes 官方的 `code-generator` 工具包：

| 工具 | 作用 | 生成内容 | 输出目录 |
| ------ | ------ | --------- | --------- |
| **deepcopy-gen** | 生成 DeepCopy 方法 | `zz_generated.deepcopy.go` | 与 types 同目录 |
| **client-gen** | 生成 REST 客户端 | `domaindata.go`, `domaindatas_client.go` | `clientset/versioned/typed/...` |
| **lister-gen** | 生成 Listers | `domaindata.go` (带缓存的查询) | `listers/kuscia/v1alpha1/` |
| **informer-gen** | 生成 Informers | `domaindata.go` (事件监听) | `informers/externalversions/...` |

### 12.3 代码生成流程

#### 步骤 1：定义 CRD 类型

```go
// pkg/crd/apis/kuscia/v1alpha1/domaindata_types.go

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=domaindatas

type DomainData struct {
metav1.TypeMeta   `json:",inline"`
metav1.ObjectMeta `json:"metadata"`
Spec              DomainDataSpec `json:"spec"`
Status            DataStatus     `json:"status,omitempty"`
}

type DomainDataSpec struct {
RelativeURI string            `json:"relativeURI"`
Author      string            `json:"author"`
Name        string            `json:"name"`
Type        string            `json:"type"`
DataSource  string            `json:"dataSource"`
Attributes  map[string]string `json:"attributes,omitempty"`
Partition   *Partition        `json:"partitions,omitempty"`
Columns     []DataColumn      `json:"columns,omitempty"`
Vendor      string            `json:"vendor,omitempty"`
FileFormat  string            `json:"fileFormat,omitempty"`
}
```

**关键注释标记**：

| 标记 | 作用 |
| ------ | ------ |
| `+genclient` | 告诉 codegen 需要生成客户端代码 |
| `+k8s:deepcopy-gen:interfaces=...` | 实现 runtime.Object 接口 |
| `+kubebuilder:resource:path=...` | 定义 CRD 的 URL 路径 |

#### 步骤 2：运行代码生成

```bash
# 在项目根目录执行
./hack/update-codegen.sh
```

**脚本内容**：

```bash
#!/bin/bash
KUSCIA_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
TMP_DIR=${KUSCIA_ROOT}/tmp-crd-code

mkdir "${TMP_DIR}"

# 调用 generate-groups.sh 生成所有代码
"${KUSCIA_ROOT}"/hack/generate-groups.sh all \
  github.com/secretflow/kuscia/pkg/crd \
  github.com/secretflow/kuscia/pkg/crd/apis \
  "kuscia:v1alpha1" \
  --output-base "${TMP_DIR}" \
  --go-header-file "${KUSCIA_ROOT}/hack/boilerplate.go.txt"

# 将生成的代码复制到正确位置
cp -r "${TMP_DIR}"/github.com/secretflow/kuscia/pkg/crd/* \
       "${KUSCIA_ROOT}"/pkg/crd
rm -r "${TMP_DIR}"
```

#### 步骤 3：验证生成结果

生成成功后，会看到以下新文件：

```text
pkg/crd/apis/kuscia/v1alpha1/zz_generated.deepcopy.go
pkg/crd/clientset/versioned/clientset.go
pkg/crd/clientset/versioned/typed/kuscia/v1alpha1/domaindata.go
pkg/crd/clientset/versioned/typed/kuscia/v1alpha1/domaindatas_client.go
pkg/crd/listers/kuscia/v1alpha1/domaindata.go
pkg/crd/informers/externalversions/kuscia/v1alpha1/domaindata.go
pkg/crd/informers/externalversions/kuscia/interface.go
pkg/crd/informers/externalversions/factory.go
```

**检查要点**：

- ✅ 文件头部包含 `Code generated by xxx-gen. DO NOT EDIT.`
- ✅ 包含正确的许可证声明
- ✅ 没有编译错误

### 12.4 生成的代码使用示例

#### 使用 Clientset

```go
// 创建客户端
config, _ := rest.InClusterConfig()
clientset, _ := versioned.NewForConfig(config)

// 创建 DomainData
domainData := &v1alpha1.DomainData{
ObjectMeta: metav1.ObjectMeta{
Name:      "my-data",
Namespace: "alice",
},
Spec: v1alpha1.DomainDataSpec{
Name:       "我的数据",
Type:       "table",
DataSource: "datasource-001",
},
}

result, err := clientset.KusciaV1alpha1().DomainDatas("alice").Create(
context.TODO(),
domainData,
metav1.CreateOptions{},
)
```

#### 使用 Lister

```go
// 创建 Lister（通常由 Informer Factory 管理）
lister := v1alpha1.NewDomainDataLister(informer.GetIndexer())

// 查询某个 namespace 的 DomainData
namespaceLister := lister.DomainDatas("alice")

// 获取单个对象（从本地缓存，非常快！）
data, err := namespaceLister.Get("my-data")

// 列出所有对象
allData, err := namespaceLister.List(labels.Everything())
```

#### 使用 Informer

```go
// 创建 SharedInformerFactory
factory := informers.NewSharedInformerFactory(clientset, time.Minute*10)

// 获取 DomainData Informer
domainDataInformer := factory.Kuscia().V1alpha1().DomainDatas()

// 注册事件处理器
domainDataInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
AddFunc: func(obj interface{}) {
dd := obj.(*v1alpha1.DomainData)
fmt.Printf("DomainData created: %s\n", dd.Name)
},
UpdateFunc: func(oldObj, newObj interface{}) {
oldDD := oldObj.(*v1alpha1.DomainData)
newDD := newObj.(*v1alpha1.DomainData)
fmt.Printf("DomainData updated: %s\n", newDD.Name)
},
DeleteFunc: func(obj interface{}) {
dd := obj.(*v1alpha1.DomainData)
fmt.Printf("DomainData deleted: %s\n", dd.Name)
},
})

// 启动 Informer
stopCh := make(chan struct{})
factory.Start(stopCh)
factory.WaitForCacheSync(stopCh)
```

### 12.5 性能优化

**Lister vs Client 对比**：

| 操作 | 直接访问 etcd (Client) | 使用 Lister |
| ------ | ---------------------- | ------------- |
| Get | 10-50ms | <1μs |
| List | 50-200ms | <10μs |
| 并发能力 | 受 etcd 限制 | 仅受内存限制 |

**最佳实践**：

- ✅ 读多写少的场景优先使用 Lister
- ✅ 控制器中统一使用 Informer + Lister
- ✅ 关键写操作直接使用 Client

---

## 13. 镜像体系

### 13.1 Kuscia 官方镜像

#### A. 主镜像：`secretflow/kuscia`

**镜像地址**：

```bash
# 阿里云镜像仓库（国内）
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:latest
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0

# Docker Hub（国际）
docker.io/secretflow/kuscia:latest
docker.io/secretflow/kuscia:1.2.0b0
```

**支持的架构**：

- ✅ `linux/amd64` (x86_64)
- ✅ `linux/arm64` (ARM64, Apple Silicon)

**镜像内容**：

- kuscia 二进制文件
- k3s 二进制文件
- kubectl 二进制文件
- CRD 定义文件
- 配置文件模板

**镜像大小**：~1.2GB

#### B. 依赖镜像

| 镜像 | 用途 | 大小 |
| ------ | ------ | ------ |
| `kuscia-deps:0.7.0b0` | 基础依赖（Python、系统库、工具） | ~500MB |
| `kuscia-envoy:0.6.2b0` | Envoy Proxy 网关 | ~200MB |
| `proot` | 进程隔离工具（RunP 运行时） | ~50MB |
| `kuscia-monitor:latest` | Prometheus Exporter 监控 | ~100MB |

### 13.2 引擎镜像

Kuscia 支持多种计算引擎：

| 引擎 | 镜像 | 用途 |
| ------ | ------ | ------ |
| **SecretFlow Lite** | `secretflow-lite-anolis8:1.11.0b1` | 联邦学习（轻量级） |
| **SecretFlow Full** | `secretflow-anolis8:1.11.0b1` | 联邦学习（完整版） |
| **SCQL** | `scql:latest` | 安全查询语言 |
| **Serving** | `serving:latest` | 模型推理服务 |

**使用示例**：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: federated-learning
spec:
  tasks:
    - name: trainer
      image: secretflow-lite-anolis8:1.11.0b1
      command: ["python", "train.py"]
```

### 13.3 镜像使用场景

#### 场景 A：Docker 部署

```bash
# 1. 拉取镜像
docker pull secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0

# 2. 启动 Kuscia
docker run -d \
  --name kuscia \
  -v /var/lib/kuscia:/var/lib/kuscia \
  -p 8080:8080 \
  secretflow/kuscia:1.2.0b0 \
  start --config /etc/kuscia/autonomy.yaml
```

#### 场景 B：K8s 部署

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kuscia
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: kuscia
          image: secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0
          command: ["./kuscia", "start", "--config", "/etc/kuscia/config.yaml"]
          ports:
            - containerPort: 8080
```

#### 场景 C：离线部署（无网络）

```bash
# 1. 在有网络的机器上保存镜像
docker save secretflow/kuscia:1.2.0b0 -o kuscia.tar

# 2. 传输到离线机器
scp kuscia.tar offline-host:/tmp/

# 3. 在离线机器上加载镜像
docker load -i /tmp/kuscia.tar

# 4. 导入到 Kuscia
./kuscia.sh image import kuscia.tar
```

### 13.4 镜像策略配置

```yaml
# kuscia_config.yaml
spec:
  image:
    # 镜像拉取策略
    pullPolicy: local  # local | remote

    # 默认镜像仓库
    defaultRegistry: aliyun

    # 镜像仓库配置
    registries:
      - name: aliyun
        url: secretflow-registry.cn-hangzhou.cr.aliyuncs.com
        username: ""
        password: ""
      - name: dockerhub
        url: docker.io
        username: myuser
        password: mypass

    # HTTP 代理（可选）
    httpProxy: http://proxy.example.com:8080
```

**pullPolicy 说明**：

| 策略 | 行为 | 适用场景 |
|------|------|----------|
| **local** | 只使用本地镜像，不拉取 | 离线环境、高安全要求 |
| **remote** | 本地不存在时自动拉取 | 在线环境、开发测试 |

### 13.5 镜像管理命令

```bash
# 查看已导入的镜像
./kuscia image list

# 输出示例：
# NAME                                    TAG       SIZE
# secretflow/kuscia                       1.2.0b0   1.2GB
# secretflow/secretflow-lite-anolis8      1.11.0b1  2.5GB

# 导入镜像
./kuscia image import kuscia.tar

# 删除镜像
./kuscia image remove secretflow/kuscia:1.2.0b0

# 导出镜像
./kuscia image export secretflow/kuscia:1.2.0b0 -o kuscia-backup.tar
```

### 13.6 私有镜像仓库

**配置 Harbor 私有仓库**：

```yaml
spec:
  image:
    registries:
      - name: harbor
        url: harbor.example.com
        username: admin
        password: Harbor12345
        insecure: false  # 是否允许 HTTP
```

**推送镜像到私有仓库**：

```bash
# 1. 打标签
docker tag secretflow/kuscia:1.2.0b0 \
  harbor.example.com/library/kuscia:1.2.0b0

# 2. 登录
docker login harbor.example.com -u admin -p Harbor12345

# 3. 推送
docker push harbor.example.com/library/kuscia:1.2.0b0
```

---

## 14. 附录

### 11.1 关键 CRD 列表

| CRD | 作用 | 作用域 |
| ----- | ------ | -------- |
| **KusciaJob** | 作业流程/DAG 定义 | Namespaced (`cross-domain`) |
| **KusciaTask** | 单个多方任务定义 | Namespaced (`cross-domain`) |
| **Domain** | 隐私计算节点 | Cluster |
| **DomainRoute** | 节点间路由规则与认证策略 | Namespaced |
| **ClusterDomainRoute** | 中心化网络中 Lite 节点间路由规则 | Cluster |
| **DomainData** | 数据对象元信息 | Namespaced |
| **DomainDataSource** | 数据源定义 | Namespaced |
| **DomainDataGrant** | 数据授权记录 | Namespaced |
| **AppImage** | 应用镜像部署模板 | Cluster |
| **Gateway** | 网关实例状态 | Namespaced |
| **TaskResourceGroup / TaskResource** | 跨域资源预留组 | - |
| **KusciaDeployment** | 在线服务部署 | - |
| **InteropConfig** | 互联互通配置 | - |

### 11.2 关键代码路径

| 类别 | 路径 |
| ------ | ------ |
| 启动入口 | `cmd/kuscia/main.go` |
| 启动命令 | `cmd/kuscia/start/start.go` |
| 模块封装 | `cmd/kuscia/modules/*.go` |
| CRD YAML | `crds/v1alpha1/kuscia.secretflow_*.yaml` |
| CRD Go 类型 | `pkg/crd/apis/kuscia/v1alpha1/*.go` |
| Job Controller | `pkg/controllers/kusciajob/` |
| Task Controller | `pkg/controllers/kusciatask/` |
| TaskResourceGroup Controller | `pkg/controllers/taskresourcegroup/` |
| Scheduler | `pkg/scheduler/kusciascheduling/` |
| Agent | `pkg/agent/framework/pods_controller.go` |
| Gateway | `pkg/gateway/controller/domain_route.go`、`pkg/gateway/xds/` |
| API 层 | `pkg/kusciaapi/` |
| DataMesh | `pkg/datamesh/dataserver/`、`pkg/datamesh/metaserver/` |
| 互联互通 | `pkg/interconn/` |
| 诊断 | `pkg/diagnose/` |

### 11.3 参考文档

- `docs/reference/architecture_cn.md`
- `docs/reference/overview.md`
- `docs/reference/kuscia_scheduling_architecture_cn.md`
- `docs/reference/concepts/kusciajob_cn.md`
- `docs/reference/concepts/kusciatask_cn.md`
- `docs/reference/concepts/domainroute_cn.md`
- `docs/reference/concepts/domaindata_cn.md`
