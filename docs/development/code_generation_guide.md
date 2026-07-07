# Kubernetes CRD 代码生成完全指南

## 目录

- [Kubernetes 简介](#kubernetes-简介)
- [Kubernetes CRD 详解](#kubernetes-crd-详解)
- [概述](#概述)
- [为什么需要代码生成](#为什么需要代码生成)
- [代码生成器工具链](#代码生成器工具链)
- [Kuscia 代码生成实践](#kuscia-代码生成实践)
- [详细生成步骤](#详细生成步骤)
- [生成的代码结构](#生成的代码结构)
- [核心概念详解](#核心概念详解)
- [实战演练](#实战演练)
- [常见问题与调试](#常见问题与调试)
- [最佳实践](#最佳实践)

---

## Kubernetes 简介

### 什么是 Kubernetes？

**Kubernetes**（简称 K8s，因为首尾字母之间有 8 个字母）是一个开源的容器编排平台，用于自动化部署、扩展和管理容器化应用程序。它最初由 Google 设计并开发，现在由 Cloud Native Computing Foundation (CNCF) 维护。

#### 核心概念

可以将 Kubernetes 理解为一个**容器管理系统**，就像操作系统的进程管理器一样，只不过它管理的是容器而不是进程。

**类比理解**：

```
传统操作系统          Kubernetes
─────────────        ─────────────
进程 (Process)   →    容器 (Container/Pod)
进程调度器       →    Kubernetes Scheduler
文件系统         →    存储卷 (Volume)
网络栈           →    CNI 网络插件
服务发现         →    Service/Ingress
配置文件         →    ConfigMap/Secret
```

#### Kubernetes 的核心组件

**控制平面（Control Plane）** - 集群的大脑：

| 组件 | 作用 | 类比 |
| ------ | ------ | ------ |
| **API Server** | 提供 RESTful API，所有操作都通过它 | 前台接待员 |
| **etcd** | 分布式键值存储，保存集群状态 | 数据库 |
| **Scheduler** | 决定 Pod 运行在哪个节点上 | 调度员 |
| **Controller Manager** | 运行各种控制器，维护期望状态 | 监控员 |

**工作节点（Worker Node）** - 执行任务的工人：

| 组件 | 作用 |
| ------ | ------ |
| **Kubelet** | 管理节点上的 Pod 和容器 |
| **Kube-proxy** | 处理网络通信和负载均衡 |
| **Container Runtime** | 实际运行容器的软件（如 Docker、containerd） |

#### Kubernetes 的基本工作流程

```yaml
# 1. 用户声明期望状态（Desired State）
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  replicas: 3  # 期望运行 3 个副本
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
        image: nginx:1.21
        ports:
        - containerPort: 80

# 2. Kubernetes 自动将实际状态调整到期望状态
# - 创建 3 个 Pod
# - 分配到合适的节点
# - 启动容器
# - 持续监控健康状态
# - 如果某个 Pod 挂了，自动创建新的替代
```

#### Kubernetes 的关键特性

- **声明式配置**：你告诉它"要什么"，而不是"怎么做"
- **自我修复**：容器挂了自动重启，节点挂了自动迁移
- **水平扩展**：根据负载自动增减实例数量
- **服务发现和负载均衡**：自动分配流量
- **滚动更新和回滚**：无停机时间地更新应用
- **存储编排**：自动挂载本地或云存储
- **密钥和配置管理**：安全地管理敏感信息

### Kubernetes 资源模型

Kubernetes 中的所有对象都被称为**资源（Resource）**，它们都是对集群状态的声明。

#### 内置资源类型

Kubernetes 提供了丰富的内置资源：

| 资源类型 | 用途 | 示例 |
| --------- | ------ | ------ |
| **Pod** | 最小的计算单元，包含一个或多个容器 | 运行你的应用 |
| **Deployment** | 管理无状态应用的部署和更新 | Web 服务、API |
| **StatefulSet** | 管理有状态应用 | 数据库、消息队列 |
| **Service** | 定义一组 Pod 的访问方式 | 负载均衡、服务发现 |
| **ConfigMap** | 存储非敏感配置数据 | 环境变量、配置文件 |
| **Secret** | 存储敏感数据 | 密码、Token、证书 |
| **Namespace** | 逻辑隔离不同项目或团队 | 多租户隔离 |
| **Ingress** | 外部访问集群的规则 | HTTP/HTTPS 路由 |

#### 资源的通用结构

所有 Kubernetes 资源都遵循相同的结构：

```go
type Resource struct {
    metav1.TypeMeta   `json:",inline"`  // API 版本和资源类型
    metav1.ObjectMeta `json:"metadata"` // 元数据（名称、命名空间、标签等）
    Spec              SomeSpec         `json:"spec"`             // 期望状态
    Status            SomeStatus       `json:"status,omitempty"` // 实际状态（由系统填写）
}
```

**字段说明**：

- **TypeMeta**：包含 `apiVersion` 和 `kind`
- **ObjectMeta**：包含 `name`、`namespace`、`labels`、`annotations` 等
- **Spec**：用户声明的期望状态
- **Status**：系统报告的实际状态（只读）

---

## Kubernetes CRD 详解

### 什么是 CRD？

**CRD（Custom Resource Definition）** 是 Kubernetes 提供的扩展机制，允许用户定义自己的资源类型，就像内置的 Pod、Service 一样使用。

#### 为什么需要 CRD？

Kubernetes 的内置资源虽然丰富，但无法覆盖所有场景。CRD 让你可以：

- ✅ **扩展 Kubernetes API**：添加符合业务需求的自定义资源
- ✅ **统一管理平台**：在同一个控制平面管理所有资源
- ✅ **复用 Kubernetes 生态**：享受 RBAC、审计、准入控制等能力
- ✅ **实现 Operator 模式**：用代码控制复杂应用的运维逻辑

**实际应用场景**：

```
场景                    内置资源不足              CRD 解决方案
──────────────────      ──────────────────       ──────────────
部署数据库              StatefulSet 太通用        Database CRD（指定主从、备份策略）
机器学习任务            Pod 无法表达训练逻辑       TrainingJob CRD（数据集、模型、评估指标）
跨域数据共享            没有数据授权概念           DomainDataGrant CRD（授权方、被授权方、有效期）
微服务治理              Service 只有基础功能        VirtualService CRD（路由规则、熔断、限流）
CI/CD 流水线            需要额外工具               Pipeline CRD（构建步骤、测试、部署）
```

### CRD vs 内置资源对比

| 特性 | 内置资源 | CRD |
| ------ | --------- | ----- |
| 定义位置 | Kubernetes 源码中硬编码 | YAML 文件或代码中声明 |
| API 路径 | `/api/v1/*` | `/apis/<group>/<version>/*` |
| 注册方式 | 编译时内置 | 运行时动态注册 |
| 扩展性 | 需要修改 K8s 源码 | 无需修改 K8s，即插即用 |
| 生态支持 | 完全支持 | 大部分支持（kubectl、RBAC、Audit 等） |
| 开发成本 | K8s 核心团队维护 | 任何开发者都可创建 |

### CRD 的核心概念

#### 1. Group-Version-Kind (GVK)

每个 CRD 都由三个维度唯一标识：

```
API Group（组）:     kuscia.secretflow
  └─ Version（版本）: v1alpha1
      └─ Kind（类型）: DomainData
```

**实际示例**：

```yaml
apiVersion: kuscia.secretflow/v1alpha1  # group/version
kind: DomainData                         # kind
metadata:
  name: my-data
  namespace: alice
```

**版本演进策略**：

```
v1alpha1 → v1beta1 → v1
  ↓          ↓        ↓
实验阶段   稳定候选  正式发布
(可能破环) (保持兼容) (严格兼容)
```

#### 2. Spec vs Status

这是 Kubernetes 的核心设计模式：**声明式 API**。

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
spec:
  # ← 用户声明的期望状态
  name: "用户行为数据"
  type: "table"
  dataSource: "datasource-001"
  
status:
  # ← 系统报告的实际状态（用户不能直接修改）
  phase: "Available"
  lastUpdateTime: "2024-01-01T12:00:00Z"
  message: "DomainData is ready"
```

**控制器的工作**：不断比较 `Spec` 和 `Status`，如果不一致就执行操作使它们对齐。

#### 3. Controller（控制器）

控制器是实现 CRD 业务逻辑的核心组件。

**工作原理 - Reconcile Loop（调谐循环）**：

```
┌─────────────┐
│  观察变化    │ ← Informer 监听到资源创建/更新/删除
└──────┬──────┘
       ↓
┌─────────────┐
│  读取当前状态 │ ← 从 etcd 读取对象
└──────┬──────┘
       ↓
┌─────────────┐
│  执行业务逻辑 │ ← 验证、授权、跨域同步等
└──────┬──────┘
       ↓
┌─────────────┐
│  更新状态    │ ← 写入 status 字段
└──────┬──────┘
       ↓
┌─────────────┐
│  等待下一次变化│ ← 回到开始
└─────────────┘
```

**Kuscia 中的实际例子**：

在 `pkg/controllers/domaindata/controller.go` 中：

```go
func (c *Controller) syncDomainDataGrantHandler(ctx context.Context, key string) error {
    // 1. 从缓存中获取 DomainDataGrant
    dg, err := c.domainDataGrantLister.Get(key)
    
    // 2. 执行业务逻辑（跨域同步、签名验证等）
    err = c.ensureDomainData(dg)
    
    // 3. 更新状态
    updateStatus(dg, phase, message)
    
    return nil
}
```

### CRD 的完整生命周期

#### 步骤 1：定义 CRD

有两种方式定义 CRD：

**方式 A：YAML 定义**（推荐用于部署）

```yaml
# crds/v1alpha1/kuscia.secretflow_domaindatas.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: domaindatas.kuscia.secretflow
spec:
  group: kuscia.secretflow
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                relativeURI:
                  type: string
                author:
                  type: string
                name:
                  type: string
                type:
                  type: string
                dataSource:
                  type: string
  scope: Namespaced  # 命名空间级别（还有 Cluster 级别）
  names:
    plural: domaindatas      # URL 路径：/apis/.../domaindatas
    singular: domaindata     # kubectl get domaindata
    kind: DomainData         # YAML 中的 kind
    shortNames: [kdd]        # kubectl get kdd
```

**方式 B：Go 代码 + Kubebuilder**（推荐用于开发）

```go
// pkg/crd/apis/kuscia/v1alpha1/domaindata_types.go

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=domaindatas

type DomainData struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata"`
    Spec              DomainDataSpec `json:"spec"`
    Status            DataStatus     `json:"status,omitempty"`
}

type DomainDataSpec struct {
    RelativeURI string `json:"relativeURI"`
    Author      string `json:"author"`
    Name        string `json:"name"`
    Type        string `json:"type"`
    DataSource  string `json:"dataSource"`
}
```

然后运行：

```bash
# 生成 CRD YAML
make manifests

# 生成客户端代码
./hack/update-codegen.sh
```

#### 步骤 2：注册 CRD

```bash
# 应用 CRD 到集群
kubectl apply -f crds/v1alpha1/kuscia.secretflow_domaindatas.yaml

# 验证注册成功
kubectl get crd domaindatas.kuscia.secretflow

# 查看 API 是否可用
kubectl api-resources | grep domaindata
```

输出：

```
NAME          SHORTNAMES   APIVERSION                  NAMESPACED   KIND
domaindatas   kdd          kuscia.secretflow/v1alpha1   true         DomainData
```

#### 步骤 3：使用 CRD

现在可以像使用内置资源一样使用 CRD：

```bash
# 创建资源
kubectl apply -f my-domaindata.yaml

# 查询资源
kubectl get domaindatas -A
kubectl get kdd -n alice  # 使用短名称

# 查看详情
kubectl describe domaindata my-data -n alice

# 解释字段
kubectl explain domaindata.spec

# 删除资源
kubectl delete domaindata my-data -n alice
```

#### 步骤 4：编写控制器

控制器负责实现 CRD 的业务逻辑。在 Kuscia 中，控制器位于 `pkg/controllers/` 目录。

**控制器的职责**：

- 监听资源变化（通过 Informer）
- 验证数据合法性
- 执行业务逻辑（如跨域同步、权限检查）
- 更新资源状态
- 与其他系统集成

### CRD 的高级特性

#### 1. Subresources（子资源）

```yaml
spec:
  subresources:
    status: {}  # 启用 status 子资源
    scale: {}   # 启用 scale 子资源（用于 HPA）
```

**作用**：允许单独更新 status 字段，避免与 spec 冲突。

```go
// 只能更新 status
client.Status().Update(ctx, domainData)
```

#### 2. Validation（验证规则）

```yaml
schema:
  openAPIV3Schema:
    properties:
      spec:
        required: ["name", "type", "dataSource"]  # 必填字段
        properties:
          type:
            enum: ["table", "model", "rule", "report"]  # 枚举值
          name:
            maxLength: 63
            pattern: '^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'  # 正则验证
```

#### 3. Conversion（版本转换）

当同时存在多个版本时，Kubernetes 可以自动转换：

```yaml
spec:
  conversion:
    strategy: Webhook
    webhook:
      clientConfig:
        service:
          name: conversion-webhook
          namespace: system
      conversionReviewVersions: ["v1"]
```

#### 4. Finalizers（终结器）

用于在删除资源前执行清理操作：

```go
// 添加终结器
if !containsString(obj.Finalizers, "kuscia.secretflow/finalizer") {
    obj.Finalizers = append(obj.Finalizers, "kuscia.secretflow/finalizer")
    client.Update(ctx, obj)
}

// 处理删除
if obj.DeletionTimestamp != nil {
    if containsString(obj.Finalizers, "kuscia.secretflow/finalizer") {
        // 执行清理逻辑
        cleanup()
        
        // 移除终结器
        obj.Finalizers = removeString(obj.Finalizers, "kuscia.secretflow/finalizer")
        client.Update(ctx, obj)
    }
}
```

### Kuscia 中的 CRD 实践

Kuscia 项目定义了多个 CRD 来管理隐私计算资源：

| CRD | 用途 | 示例场景 |
| ----- | ------ | ---------- |
| **DomainData** | 数据资产注册 | 注册表格、模型、规则文件 |
| **DomainDataSource** | 数据源配置 | 配置本地文件系统、OSS、ODPS |
| **DomainDataGrant** | 数据授权 | 授权其他域访问数据 |
| **Domain** | 参与方定义 | 定义联盟成员 |
| **KusciaJob** | 联邦学习任务 | 提交多方建模任务 |
| **AppImage** | 应用镜像 | 注册算法组件 |

**完整的 DomainData 工作流**：

```
1. 管理员创建 DomainDataSource（数据源）
   ↓
2. 数据提供方注册 DomainData（数据资产）
   ↓
3. 数据提供方创建 DomainDataGrant（授权给其他域）
   ↓
4. Controller 自动同步到被授权域的 namespace
   ↓
5. 被授权方可以通过 API 访问数据
   ↓
6. 联邦学习任务引用 DomainData 作为输入
```

### CRD 与代码生成的关系

这就是为什么我们需要代码生成！

当你定义了一个 CRD 后，需要：

1. ✅ **Clientset**：让 Go 代码可以操作这个 CRD
2. ✅ **Lister**：高效地从缓存查询 CRD 对象
3. ✅ **Informer**：监听 CRD 的变化事件
4. ✅ **DeepCopy**：实现 Kubernetes 运行时接口

这些代码都是模式化的，所以 Kubernetes 提供了 code-generator 工具自动生成它们。

```yaml
CRD 定义 (YAML/Go)
       ↓
  运行 codegen
       ↓
┌──────────────────────┐
│ 生成的代码            │
├──────────────────────┤
│ • Clientset          │ ← 用于 CRUD 操作
│ • Lister             │ ← 用于缓存查询
│ • Informer           │ ← 用于事件监听
│ • DeepCopy           │ ← 用于对象拷贝
└──────────────────────┘
       ↓
  在控制器中使用
       ↓
  实现业务逻辑
```

---

## 概述

Kubernetes CRD（Custom Resource Definition）代码生成是 Kubernetes 生态系统中的一项核心技术，它通过自动化工具从类型定义生成标准的客户端代码、Lister、Informer 等组件，大大简化了自定义资源的开发工作。

### 什么是代码生成？

代码生成是指根据开发者编写的类型定义文件（`*_types.go`），使用自动化工具生成一系列标准化的 Go 代码文件，包括：

- **Clientset**：用于操作 Kubernetes API 的客户端代码
- **Listers**：提供带缓存的列表查询功能
- **Informers**：监听资源变化的事件驱动机制
- **DeepCopy**：对象的深拷贝方法实现

### Kuscia 中的应用

在 Kuscia 项目中，DomainData、DomainDataSource、DomainDataGrant 等核心资源都是基于 CRD 实现的，它们的客户端代码全部通过自动生成获得。

**示例**：`pkg/crd/apis/kuscia/v1alpha1/domaindata_types.go`（手动编写） → `pkg/crd/clientset/versioned/typed/kuscia/v1alpha1/domaindata.go`（自动生成）

---

## 为什么需要代码生成

### 1. 减少重复劳动

如果没有代码生成，每新增一个 CRD 资源，开发者需要手动编写：

- RESTful API 客户端（GET/POST/PUT/DELETE/PATCH）
- 序列化和反序列化逻辑
- 缓存和索引机制
- Watch 事件监听
- DeepCopy 方法

这些代码高度模式化且容易出错。

### 2. 保证代码质量

生成的代码遵循 Kubernetes 官方标准，确保：

- 一致的代码风格
- 正确的错误处理
- 完善的并发控制
- 高效的缓存策略

### 3. 易于维护

当类型定义发生变化时，只需重新运行生成脚本，所有相关代码自动更新，无需手动修改多处。

### 4. 类型安全

基于 Go 语言的强类型系统，生成的代码提供编译期类型检查，避免运行时错误。

---

## 代码生成器工具链

Kubernetes 提供了完整的代码生成工具集，位于 `k8s.io/code-generator` 仓库。

### 核心工具介绍

| 工具 | 作用 | 生成内容 | 输出目录 |
| ------ | ------ | --------- | --------- |
| **deepcopy-gen** | 生成 DeepCopy 方法 | `zz_generated.deepcopy.go` | 与 types 同目录 |
| **client-gen** | 生成 REST 客户端 | `domaindata.go`, `domaindatas_client.go` | `clientset/versioned/typed/...` |
| **lister-gen** | 生成 Listers | `domaindata.go` (带缓存的查询) | `listers/kuscia/v1alpha1/` |
| **informer-gen** | 生成 Informers | `domaindata.go` (事件监听) | `informers/externalversions/...` |
| **defaulter-gen** | 生成默认值设置 | `zz_generated.defaults.go` | 与 types 同目录 |

### 工具安装方式

在 `hack/generate-groups.sh` 第 53 行可以看到安装命令：

```bash
GO111MODULE=on go install k8s.io/code-generator/cmd/{defaulter-gen,client-gen,lister-gen,informer-gen,deepcopy-gen}
```

这些工具会被安装到 `$GOBIN` 或 `$GOPATH/bin` 目录。

---

## Kuscia 代码生成实践

### 项目结构

```
kuscia/
├── hack/
│   ├── update-codegen.sh          # 主生成脚本
│   ├── generate-groups.sh         # 分组生成脚本
│   └── boilerplate.go.txt         # 许可证头模板
├── pkg/crd/apis/kuscia/v1alpha1/
│   ├── domaindata_types.go        # ← 手动编写（输入）
│   ├── domaindatagrant_types.go   # ← 手动编写（输入）
│   └── zz_generated.deepcopy.go   # ← 自动生成（输出）
└── pkg/crd/
    ├── clientset/                  # ← 自动生成
    │   └── versioned/
    │       ├── typed/kuscia/v1alpha1/
    │       │   ├── domaindata.go
    │       │   └── ...
    ├── listers/                    # ← 自动生成
    │   └── kuscia/v1alpha1/
    │       └── domaindata.go
    └── informers/                  # ← 自动生成
        └── externalversions/kuscia/v1alpha1/
            └── domaindata.go
```

### 关键配置文件

#### 1. `hack/update-codegen.sh`（主入口）

```bash
#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

# 获取项目根目录
KUSCIA_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)

# 临时输出目录（避免权限问题）
TMP_DIR=${KUSCIA_ROOT}/tmp-crd-code
mkdir "${TMP_DIR}"

# 调用 generate-groups.sh 生成所有代码
"${KUSCIA_ROOT}"/hack/generate-groups.sh all \
  github.com/secretflow/kuscia/pkg/crd \           # OUTPUT_PKG: 输出包路径
  github.com/secretflow/kuscia/pkg/crd/apis \      # APIS_PKG: API 定义路径
  "kuscia:v1alpha1" \                              # GROUPS_VERSIONS: 组名和版本
  --output-base "${TMP_DIR}" \                     # 输出基础目录
  --go-header-file "${KUSCIA_ROOT}/hack/boilerplate.go.txt"  # 许可证头

# 将生成的代码复制到正确位置
cp -r "${TMP_DIR}"/github.com/secretflow/kuscia/pkg/crd/* \
       "${KUSCIA_ROOT}"/pkg/crd

# 清理临时目录
rm -r "${TMP_DIR}"
```

**关键参数说明**：

- `all`：生成所有组件（deepcopy, client, lister, informer）
- `OUTPUT_PKG`：生成代码的目标包路径
- `APIS_PKG`：CRD 类型定义的源包路径
- `GROUPS_VERSIONS`：格式为 `group:version`，指定要处理的 API 组和版本

#### 2. `hack/generate-groups.sh`（核心逻辑）

这个脚本由 Kubernetes 官方提供，主要功能是依次调用各个生成器：

```bash
# 安装代码生成工具
cd "$(dirname "${0}")"
GO111MODULE=on go install k8s.io/code-generator/cmd/{defaulter-gen,client-gen,lister-gen,informer-gen,deepcopy-gen}

# 生成 deepcopy
if [ "${GENS}" = "all" ] || grep -qw "deepcopy" <<<"${GENS}"; then
  "${gobin}/deepcopy-gen" \
      --input-dirs "$(codegen::join , "${FQ_APIS[@]}")" \
      -O zz_generated.deepcopy \
      "$@"
fi

# 生成 clientset
if [ "${GENS}" = "all" ] || grep -qw "client" <<<"${GENS}"; then
  "${gobin}/client-gen" \
      --clientset-name "${CLIENTSET_NAME_VERSIONED:-versioned}" \
      --input-base "" \
      --input "$(codegen::join , "${FQ_APIS[@]}")" \
      --output-package "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME:-clientset}" \
      "$@"
fi

# 生成 listers
if [ "${GENS}" = "all" ] || grep -qw "lister" <<<"${GENS}"; then
  "${gobin}/lister-gen" \
      --input-dirs "$(codegen::join , "${FQ_APIS[@]}")" \
      --output-package "${OUTPUT_PKG}/listers" \
      "$@"
fi

# 生成 informers
if [ "${GENS}" = "all" ] || grep -qw "informer" <<<"${GENS}"; then
  "${gobin}/informer-gen" \
      --input-dirs "$(codegen::join , "${FQ_APIS[@]}")" \
      --versioned-clientset-package "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME:-clientset}/${CLIENTSET_NAME_VERSIONED:-versioned}" \
      --listers-package "${OUTPUT_PKG}/listers" \
      --output-package "${OUTPUT_PKG}/informers" \
      "$@"
fi
```

#### 3. `hack/boilerplate.go.txt`（许可证头）

每个生成的文件开头都会包含这个许可证声明：

```go
// Copyright 2023 Ant Group Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
```

---

## 详细生成步骤

### 步骤 1：定义 CRD 类型

首先，在 `pkg/crd/apis/kuscia/v1alpha1/domaindata_types.go` 中定义类型：

```go
package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient                                    // ← 标记1: 需要生成客户端代码
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object  // ← 标记2: 需要生成 DeepCopy
// +kubebuilder:object:root=true                 // ← 标记3: Kubebuilder 对象声明
// +kubebuilder:subresource:status               // ← 标记4: 启用 status 子资源
// +kubebuilder:resource:path=domaindatas        // ← 标记5: CRD 的 URL 路径
// +kubebuilder:resource:singular=domaindata     // ← 标记6: 单数形式
// +kubebuilder:resource:shortName=kdd           // ← 标记7: kubectl 短名称

// DomainData include feature table,model,rule,report .etc
type DomainData struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata"`
    Spec              DomainDataSpec `json:"spec"`
    Status            DataStatus     `json:"status,omitempty"`
}

// DomainDataSpec defines the spec of data object.
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

**重要注释标记说明**：

| 标记 | 作用 | 影响 |
| ------ | ------ | ------ |
| `+genclient` | 告诉 codegen 为此类型生成客户端代码 | 生成 `domaindata.go` |
| `+k8s:deepcopy-gen:interfaces=...` | 实现 runtime.Object 接口 | 生成 `DeepCopyObject()` 方法 |
| `+kubebuilder:resource:path=...` | 定义 CRD 的 API 路径 | 影响 kubectl 访问路径 |
| `+kubebuilder:subresource:status` | 启用 status 子资源 | 允许单独更新 status 字段 |

### 步骤 2：运行代码生成

在项目根目录执行：

```bash
./hack/update-codegen.sh
```

**执行流程**：

1. **创建临时目录**：`tmp-crd-code/`
2. **安装工具**：使用 `go install` 安装 5 个生成器
3. **构建参数**：
   - 输入：`github.com/secretflow/kuscia/pkg/crd/apis/kuscia:v1alpha1`
   - 输出：`github.com/secretflow/kuscia/pkg/crd`
4. **依次生成**：
   - deepcopy-gen → `zz_generated.deepcopy.go`
   - client-gen → `clientset/versioned/...`
   - lister-gen → `listers/...`
   - informer-gen → `informers/...`
5. **复制文件**：将临时目录中的代码复制到 `pkg/crd/`
6. **清理临时文件**：删除 `tmp-crd-code/`

### 步骤 3：验证生成结果

生成成功后，你会看到以下新文件：

```
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
- ✅ 方法签名符合预期

### 步骤 4：提交到版本控制

将生成的代码一起提交到 Git：

```bash
git add pkg/crd/apis/kuscia/v1alpha1/zz_generated.deepcopy.go
git add pkg/crd/clientset/
git add pkg/crd/listers/
git add pkg/crd/informers/
git commit -m "regen crd code for DomainData"
```

**注意**：生成的代码**必须**提交到 Git，因为它们是被其他代码导入和使用的。

---

## 生成的代码结构

### 1. DeepCopy 代码

**文件位置**：`pkg/crd/apis/kuscia/v1alpha1/zz_generated.deepcopy.go`

**作用**：实现对象的深拷贝，这是 Kubernetes 对象的基本要求。

**生成代码示例**：

```go
// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DomainData) DeepCopyInto(out *DomainData) {
    *out = *in
    out.TypeMeta = in.TypeMeta
    in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
    in.Spec.DeepCopyInto(&out.Spec)
    out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DomainData.
func (in *DomainData) DeepCopy() *DomainData {
    if in == nil { return nil }
    out := new(DomainData)
    in.DeepCopyInto(out)
    return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DomainData) DeepCopyObject() runtime.Object {
    if c := in.DeepCopy(); c != nil {
        return c
    }
    return nil
}
```

**为什么需要 DeepCopy？**

Kubernetes 在内部传递对象时，为了避免数据竞争和意外修改，总是使用副本而不是原始对象。

### 2. Clientset 代码

**文件位置**：`pkg/crd/clientset/versioned/typed/kuscia/v1alpha1/domaindata.go`

**作用**：提供对 DomainData 资源的 CRUD 操作接口。

**生成的接口**：

```go
type DomainDataInterface interface {
    Create(ctx context.Context, domainData *v1alpha1.DomainData, opts v1.CreateOptions) (*v1alpha1.DomainData, error)
    Update(ctx context.Context, domainData *v1alpha1.DomainData, opts v1.UpdateOptions) (*v1alpha1.DomainData, error)
    UpdateStatus(ctx context.Context, domainData *v1alpha1.DomainData, opts v1.UpdateOptions) (*v1alpha1.DomainData, error)
    Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
    Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.DomainData, error)
    List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.DomainDataList, error)
    Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
    Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions) (*v1alpha1.DomainData, error)
}
```

**使用示例**：

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

### 3. Lister 代码

**文件位置**：`pkg/crd/listers/kuscia/v1alpha1/domaindata.go`

**作用**：提供带本地缓存的只读查询，减少对 etcd 的直接访问。

**生成的接口**：

```go
type DomainDataLister interface {
    List(selector labels.Selector) ([]*v1alpha1.DomainData, error)
    DomainDatas(namespace string) DomainDataNamespaceLister
}

type DomainDataNamespaceLister interface {
    List(selector labels.Selector) ([]*v1alpha1.DomainData, error)
    Get(name string) (*v1alpha1.DomainData, error)
}
```

**优势**：

- ✅ **高性能**：从本地内存读取，无需网络请求
- ✅ **低延迟**：微秒级响应
- ✅ **减轻 etcd 压力**：避免频繁查询

**使用示例**：

```go
// 创建 Lister（通常由 Informer Factory 管理）
lister := v1alpha1.NewDomainDataLister(informer.GetIndexer())

// 查询某个 namespace 的 DomainData
namespaceLister := lister.DomainDatas("alice")

// 获取单个对象（从本地缓存）
data, err := namespaceLister.Get("my-data")

// 列出所有对象
allData, err := namespaceLister.List(labels.Everything())
```

### 4. Informer 代码

**文件位置**：`pkg/crd/informers/externalversions/kuscia/v1alpha1/domaindata.go`

**作用**：监听 Kubernetes API Server 的资源变化事件。

**工作机制**：

```
┌─────────────┐     ┌──────────┐     ┌────────┐     ┌───────┐
│ API Server  │────▶│ Reflector │────▶│ DeltaFIFO│────▶│ Indexer│
└─────────────┘     └──────────┘     └────────┘     └───┬───┘
                                                          │
                        ┌─────────────────────────────────┘
                        │
                   ┌────▼────┐
                   │ Handler │
                   │ - Add   │
                   │ - Update│
                   │ - Delete│
                   └─────────┘
```

**注册事件处理器**：

```go
informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc: func(obj interface{}) {
        domainData := obj.(*v1alpha1.DomainData)
        fmt.Printf("DomainData created: %s\n", domainData.Name)
    },
    UpdateFunc: func(oldObj, newObj interface{}) {
        oldDD := oldObj.(*v1alpha1.DomainData)
        newDD := newObj.(*v1alpha1.DomainData)
        fmt.Printf("DomainData updated: %s\n", newDD.Name)
    },
    DeleteFunc: func(obj interface{}) {
        domainData := obj.(*v1alpha1.DomainData)
        fmt.Printf("DomainData deleted: %s\n", domainData.Name)
    },
})
```

**启动 Informer**：

```go
// 创建 SharedInformerFactory
factory := informers.NewSharedInformerFactory(clientset, time.Minute*10)

// 获取 DomainData Informer
domainDataInformer := factory.Kuscia().V1alpha1().DomainDatas()

// 启动所有 Informer
factory.Start(stopCh)

// 等待缓存同步
factory.WaitForCacheSync(stopCh)
```

---

## 核心概念详解

### 1. API Groups 和 Versions

Kubernetes 使用 API Groups 来组织资源：

```
API Group: kuscia.secretflow
└── Version: v1alpha1
    ├── DomainData
    ├── DomainDataSource
    ├── DomainDataGrant
    └── ...
```

**命名规则**：

- **Group**：通常是 `project.domain` 格式
- **Version**：`v1alpha1` → `v1beta1` → `v1`（成熟度递增）
- **Kind**：资源类型名称（如 DomainData）

### 2. Generators 详解

#### deepcopy-gen

**原理**：通过反射分析结构体字段，递归生成拷贝代码。

**关键特性**：

- 处理指针、切片、map 等复杂类型
- 支持嵌套结构体
- 实现 `runtime.Object` 接口

**注意事项**：

- 如果类型包含非基本类型字段，需要为该字段类型也实现 DeepCopy
- 可以使用 `+k8s:deepcopy-gen=false` 跳过某些字段

#### client-gen

**生成的层次结构**：

```
Clientset (顶级客户端)
└── KusciaV1alpha1() (API Group)
    └── DomainDatas(namespace) (资源接口)
        ├── Create()
        ├── Update()
        ├── Get()
        ├── List()
        └── ...
```

**REST 映射**：

| 方法 | HTTP 动词 | 路径 |
| ------ | ---------- | ------ |
| Create | POST | `/apis/kuscia.secretflow/v1alpha1/namespaces/{ns}/domaindatas` |
| Update | PUT | `/apis/kuscia.secretflow/v1alpha1/namespaces/{ns}/domaindatas/{name}` |
| Get | GET | `/apis/kuscia.secretflow/v1alpha1/namespaces/{ns}/domaindatas/{name}` |
| List | GET | `/apis/kuscia.secretflow/v1alpha1/namespaces/{ns}/domaindatas` |
| Delete | DELETE | `/apis/kuscia.secretflow/v1alpha1/namespaces/{ns}/domaindatas/{name}` |

#### lister-gen

**缓存机制**：

```go
type cache struct {
    indexer cache.Indexer  // 线程安全的本地存储
}

func (l *domainDataLister) Get(name string) (*v1alpha1.DomainData, error) {
    obj, exists, err := l.indexer.GetByKey(l.namespace + "/" + name)
    if err != nil {
        return nil, err
    }
    if !exists {
        return nil, errors.NewNotFound(...)
    }
    return obj.(*v1alpha1.DomainData), nil
}
```

**性能对比**：

| 操作 | 直接访问 etcd | 使用 Lister |
| ------ | -------------- | ------------- |
| Get | 10-50ms | <1μs |
| List | 50-200ms | <10μs |
| 并发能力 | 受 etcd 限制 | 仅受内存限制 |

#### informer-gen

**工作流程**：

1. **Reflector**：调用 API Server 的 List/Watch 接口
2. **DeltaFIFO**：按顺序存储事件（Add/Update/Delete）
3. **Indexer**：建立索引（按 namespace、label 等）
4. **Handler**：触发用户定义的回调函数

**Resync 机制**：

```go
// 每隔一段时间重新 List 所有对象，确保没有遗漏
factory := informers.NewSharedInformerFactory(clientset, time.Minute*10)
//                                                                              ↑
//                                                                 Resync 周期
```

### 3. SharedInformerFactory

**设计目的**：多个组件共享同一个 Informer，避免重复创建 Watch 连接。

**实现原理**：

```go
type sharedInformerFactory struct {
    client           versioned.Interface
    namespace        string
    informers        map[reflect.Type]cache.SharedIndexInformer
    startedInformers map[reflect.Type]bool
}

// 获取或创建 Informer
func (f *sharedInformerFactory) DomainDatas() v1alpha1.DomainDataInformer {
    return &domainDataInformer{
        informer: f.getInformer(&v1alpha1.DomainData{}),
    }
}

func (f *sharedInformerFactory) getInformer(objType runtime.Object) cache.SharedIndexInformer {
    // 如果已存在，直接返回
    if informer, exists := f.informers[reflect.TypeOf(objType)]; exists {
        return informer
    }
    
    // 否则创建新的
    informer := NewFilteredInformer(...)
    f.informers[reflect.TypeOf(objType)] = informer
    return informer
}
```

---

## 实战演练

### 实验 1：添加一个新的 CRD 资源

假设我们要添加 `DomainModel` 资源。

#### 步骤 1：创建类型定义

创建文件 `pkg/crd/apis/kuscia/v1alpha1/domainmodel_types.go`：

```go
package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=domainmodels

// DomainModel represents a machine learning model.
type DomainModel struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata"`
    Spec              DomainModelSpec `json:"spec"`
    Status            DataStatus      `json:"status,omitempty"`
}

// DomainModelSpec defines the spec of model.
type DomainModelSpec struct {
    ModelPath   string `json:"modelPath"`
    Format      string `json:"format"`
    Size        int64  `json:"size"`
    Checksum    string `json:"checksum"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DomainModelList contains a list of domain models.
type DomainModelList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []DomainModel `json:"items"`
}
```

#### 步骤 2：运行代码生成

```bash
cd /path/to/kuscia
./hack/update-codegen.sh
```

#### 步骤 3：验证生成结果

```bash
# 检查是否生成了相关文件
ls pkg/crd/clientset/versioned/typed/kuscia/v1alpha1/domainmodel.go
ls pkg/crd/listers/kuscia/v1alpha1/domainmodel.go
ls pkg/crd/informers/externalversions/kuscia/v1alpha1/domainmodel.go
ls pkg/crd/apis/kuscia/v1alpha1/zz_generated.deepcopy.go | grep DomainModel
```

#### 步骤 4：编写测试代码

创建 `test_domainmodel.go`：

```go
package main

import (
    "context"
    "fmt"
    "github.com/secretflow/kuscia/pkg/crd/clientset/versioned"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/tools/clientcmd"
)

func main() {
    // 加载 kubeconfig
    config, _ := clientcmd.BuildConfigFromFlags("", "~/.kube/config")
    
    // 创建客户端
    clientset, _ := versioned.NewForConfig(config)
    
    // 创建 DomainModel
    model := &v1alpha1.DomainModel{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "lr-model",
            Namespace: "alice",
        },
        Spec: v1alpha1.DomainModelSpec{
            ModelPath: "/models/lr_model.pkl",
            Format:    "pickle",
            Size:      1024000,
            Checksum:  "sha256:abc123...",
        },
    }
    
    result, err := clientset.KusciaV1alpha1().DomainModels("alice").Create(
        context.TODO(),
        model,
        metav1.CreateOptions{},
    )
    
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Created model: %s\n", result.Name)
}
```

### 实验 2：使用 Informer 监听变化

```go
package main

import (
    "time"
    "github.com/secretflow/kuscia/pkg/crd/clientset/versioned"
    "github.com/secretflow/kuscia/pkg/crd/informers/externalversions"
    "k8s.io/client-go/tools/clientcmd"
)

func main() {
    config, _ := clientcmd.BuildConfigFromFlags("", "~/.kube/config")
    clientset, _ := versioned.NewForConfig(config)
    
    // 创建 SharedInformerFactory
    factory := externalversions.NewSharedInformerFactory(clientset, time.Minute*10)
    
    // 获取 DomainData Informer
    domainDataInformer := factory.Kuscia().V1alpha1().DomainDatas()
    
    // 注册事件处理器
    domainDataInformer.Informer().AddEventHandler(
        cache.ResourceEventHandlerFuncs{
            AddFunc: func(obj interface{}) {
                dd := obj.(*v1alpha1.DomainData)
                fmt.Printf("[ADD] New DomainData: %s/%s\n", dd.Namespace, dd.Name)
            },
            UpdateFunc: func(oldObj, newObj interface{}) {
                oldDD := oldObj.(*v1alpha1.DomainData)
                newDD := newObj.(*v1alpha1.DomainData)
                if oldDD.Spec.Type != newDD.Spec.Type {
                    fmt.Printf("[UPDATE] Type changed: %s, %s -> %s\n", 
                        newDD.Name, oldDD.Spec.Type, newDD.Spec.Type)
                }
            },
            DeleteFunc: func(obj interface{}) {
                dd := obj.(*v1alpha1.DomainData)
                fmt.Printf("[DELETE] Removed DomainData: %s/%s\n", dd.Namespace, dd.Name)
            },
        },
    )
    
    // 启动 Informer
    stopCh := make(chan struct{})
    factory.Start(stopCh)
    factory.WaitForCacheSync(stopCh)
    
    // 保持运行
    select {}
}
```

### 实验 3：使用 Lister 高效查询

```go
package main

import (
    "fmt"
    "github.com/secretflow/kuscia/pkg/crd/clientset/versioned"
    "github.com/secretflow/kuscia/pkg/crd/informers/externalversions"
    "github.com/secretflow/kuscia/pkg/crd/listers/kuscia/v1alpha1"
    "k8s.io/apimachinery/pkg/labels"
    "k8s.io/client-go/tools/clientcmd"
    "time"
)

func main() {
    config, _ := clientcmd.BuildConfigFromFlags("", "~/.kube/config")
    clientset, _ := versioned.NewForConfig(config)
    
    factory := externalversions.NewSharedInformerFactory(clientset, time.Minute*10)
    domainDataInformer := factory.Kuscia().V1alpha1().DomainDatas()
    
    // 创建 Lister
    var lister v1alpha1.DomainDataLister = v1alpha1.NewDomainDataLister(
        domainDataInformer.Informer().GetIndexer(),
    )
    
    // 启动并等待同步
    stopCh := make(chan struct{})
    factory.Start(stopCh)
    factory.WaitForCacheSync(stopCh)
    
    // 使用 Lister 查询（从本地缓存，非常快！）
    namespaceLister := lister.DomainDatas("alice")
    
    // 查询单个
    data, err := namespaceLister.Get("my-table")
    if err == nil {
        fmt.Printf("Found: %s, type=%s\n", data.Name, data.Spec.Type)
    }
    
    // 按标签过滤
    selector, _ := labels.Parse("author=alice")
    allData, _ := namespaceLister.List(selector)
    fmt.Printf("Total: %d tables\n", len(allData))
}
```

---

## 常见问题与调试

### 问题 1：生成代码后编译失败

**症状**：

```
cannot use &DomainData{} (value of type *DomainData) as runtime.Object value in argument to Scheme.AddKnownTypes: missing method DeepCopyObject
```

**原因**：忘记运行 deepcopy-gen 或生成的代码未正确导入。

**解决方案**：

```bash
# 重新生成所有代码
./hack/update-codegen.sh

# 检查是否生成了 deepcopy 文件
ls pkg/crd/apis/kuscia/v1alpha1/zz_generated.deepcopy.go

# 清理并重新编译
go clean -cache
go build ./...
```

### 问题 2：Informer 无法同步

**症状**：

```
E0101 12:00:00.000000  123456 reflector.go:xxx] Failed to watch *v1alpha1.DomainData: 
the server could not find the requested resource
```

**原因**：CRD 未在集群中注册。

**解决方案**：

```bash
# 检查 CRD 是否存在
kubectl get crd domaindatas.kuscia.secretflow

# 如果不存在，应用 CRD YAML
kubectl apply -f crds/v1alpha1/kuscia.secretflow_domaindatas.yaml

# 验证 API 是否可访问
kubectl api-resources | grep domaindata
```

### 问题 3：Lister 返回 stale 数据

**症状**：查询结果不是最新的。

**原因**：Informer 缓存尚未同步或 Resync 周期过长。

**解决方案**：

```go
// 方案 1: 等待缓存同步
factory.WaitForCacheSync(stopCh)

// 方案 2: 缩短 Resync 周期
factory := externalversions.NewSharedInformerFactory(
    clientset, 
    time.Minute*1,  // 从 10 分钟改为 1 分钟
)

// 方案 3: 对于关键操作，直接使用 Client 查询 etcd
clientset.KusciaV1alpha1().DomainDatas("alice").Get(
    context.TODO(), 
    "my-data", 
    metav1.GetOptions{},
)
```

### 问题 4：权限不足

**症状**：

```
Error from server (Forbidden): domaindatas.kuscia.secretflow is forbidden
```

**原因**：ServiceAccount 缺少 RBAC 权限。

**解决方案**：

创建 `rbac.yaml`：

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: domaindata-manager
rules:
  - apiGroups: ["kuscia.secretflow"]
    resources: ["domaindatas", "domaindatagrants"]
    verbs: ["get", "list", "watch", "create", "update", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: domaindata-manager-binding
subjects:
  - kind: ServiceAccount
    name: kuscia-controller
    namespace: kuscia-system
roleRef:
  kind: ClusterRole
  name: domaindata-manager
```

```bash
kubectl apply -f rbac.yaml
```

### 问题 5：代码生成脚本失败

**症状**：

```
hack/generate-groups.sh: line 53: go: command not found
```

**原因**：Go 环境未正确配置。

**解决方案**：

```bash
# 检查 Go 是否安装
go version

# 设置 GOPATH 和 GOBIN
export GOPATH=$HOME/go
export GOBIN=$GOPATH/bin
export PATH=$PATH:$GOBIN

# 重新运行
./hack/update-codegen.sh
```

### 调试技巧

#### 1. 启用详细日志

```bash
# 在运行生成脚本前设置
export KLOG_V=5
./hack/update-codegen.sh
```

#### 2. 检查生成的代码

```bash
# 查看生成的文件数量
find pkg/crd -name "*.go" -newer hack/update-codegen.sh | wc -l

# 检查是否有编译错误
go build ./pkg/crd/...
```

#### 3. 验证 Informer 工作状态

```go
// 添加日志输出
informer.Informer().SetWatchErrorHandler(func(r *cache.Reflector, err error) {
    klog.Errorf("Watch error: %v", err)
})

// 检查缓存同步状态
if !factory.WaitForCacheSync(stopCh) {
    klog.Error("Failed to sync cache")
}
```

---

## 最佳实践

### 1. 类型设计规范

✅ **推荐做法**：

```go
type MyResource struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata"`
    Spec              MyResourceSpec `json:"spec"`
    Status            MyStatus       `json:"status,omitempty"`
}
```

❌ **避免**：

```go
// 不要嵌入非标准类型
type MyResource struct {
    SomeCustomType  // ← 可能导致 DeepCopy 生成失败
    Spec MyResourceSpec
}
```

### 2. 注释规范

```go
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Author",type=string,JSONPath=`.spec.author`
// +kubebuilder:resource:path=myresources,singular=myresource,shortName=mr

// MyResource 的详细描述...
type MyResource struct {
    // 字段注释，会显示在 kubectl explain 中
    Spec MyResourceSpec `json:"spec"`
}
```

### 3. 版本管理策略

- **v1alpha1**：初始版本，API 可能变化
- **v1beta1**：稳定候选，保持向后兼容
- **v1**：生产就绪，严格保证兼容性

**升级流程**：

```
v1alpha1 (新功能) 
    ↓ (稳定后)
v1beta1 (广泛测试) 
    ↓ (无重大变更)
v1 (正式发布)
```

### 4. 性能优化

#### 使用 Lister 代替 Client

```go
// ❌ 慢：每次都访问 etcd
data, err := client.Get(ctx, name, metav1.GetOptions{})

// ✅ 快：从本地缓存读取
data, err := lister.Get(name)
```

#### 合理设置 Resync 周期

```go
// 高频场景：1-5 分钟
factory := NewSharedInformerFactory(client, time.Minute*1)

// 低频场景：10-30 分钟
factory := NewSharedInformerFactory(client, time.Minute*10)
```

#### 选择性启动 Informer

```go
// 只启动需要的 Informer，节省资源
domainDataInformer := factory.Kuscia().V1alpha1().DomainDatas()
factory.Start(domainDataInformer.Informer().HasSync)

// 而不是启动所有 Informer
// factory.Start(stopCh)
```

### 5. 错误处理

```go
// 检查资源是否存在
data, err := lister.Get(name)
if err != nil {
    if errors.IsNotFound(err) {
        // 资源不存在，创建它
        return client.Create(ctx, newData, metav1.CreateOptions{})
    }
    return err
}

// 处理冲突（乐观锁）
for i := 0; i < 3; i++ {
    data, _ := client.Get(ctx, name, metav1.GetOptions{})
    data.Spec.Field = newValue
    _, err := client.Update(ctx, data, metav1.UpdateOptions{})
    if err == nil {
        break
    }
    if !errors.IsConflict(err) {
        return err
    }
    // 重试
}
```

### 6. 测试建议

#### 单元测试

```go
func TestDomainDataCreation(t *testing.T) {
    // 使用 fake clientset
    clientset := fake.NewSimpleClientset()
    
    // 创建测试对象
    data := &v1alpha1.DomainData{
        ObjectMeta: metav1.ObjectMeta{Name: "test"},
        Spec: v1alpha1.DomainDataSpec{Type: "table"},
    }
    
    _, err := clientset.KusciaV1alpha1().DomainDatas("default").Create(
        context.TODO(), data, metav1.CreateOptions{},
    )
    
    assert.NoError(t, err)
}
```

#### 集成测试

```bash
# 在真实的 K8s 集群中测试
kubectl create ns test-ns
kubectl apply -f test-domaindata.yaml
kubectl get domaindatas -n test-ns
```

### 7. 代码审查清单

在提交生成的代码前，检查：

- [ ] 所有生成的文件都已添加到 Git
- [ ] 没有手动修改生成的代码
- [ ] 编译通过且无警告
- [ ] 单元测试通过
- [ ] 类型定义中有适当的注释标记
- [ ] 许可证头正确
- [ ] 版本号一致

---

## 附录

### A. 术语表

| 术语 | 解释 |
| ------ | ------ |
| **CRD** | Custom Resource Definition，Kubernetes 自定义资源 |
| **Codegen** | Code Generator，代码生成器 |
| **Clientset** | Kubernetes API 客户端集合 |
| **Informer** | 监听资源变化的事件驱动机制 |
| **Lister** | 带本地缓存的只读查询接口 |
| **DeepCopy** | 对象的深拷贝方法 |
| **SharedInformerFactory** | 共享 Informer 的工厂模式 |
| **Resync** | 定期重新 List 所有对象的机制 |
| **RBAC** | Role-Based Access Control，基于角色的访问控制 |

### B. 常用命令速查

```bash
# 生成代码
./hack/update-codegen.sh

# 检查生成的文件
find pkg/crd -name "zz_*.go" -o -name "clientset" -type d

# 编译验证
go build ./pkg/crd/...

# 运行测试
go test ./pkg/crd/... -v

# 查看 CRD
kubectl get crd | grep kuscia

# 查询资源
kubectl get domaindatas -A
kubectl describe domaindata my-data -n alice
```

### C. 参考资源

- **Kubernetes Code Generator**: https://github.com/kubernetes/code-generator
- **Custom Resource Definitions**: https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/
- **Kube Builder Book**: https://book.kubebuilder.io/
- **Kuscia 项目**: https://github.com/secretflow/kuscia

### D. 文件清单

**手动编写的文件**（需要提交到 Git）：

```
pkg/crd/apis/kuscia/v1alpha1/domaindata_types.go
pkg/crd/apis/kuscia/v1alpha1/domaindatagrant_types.go
pkg/crd/apis/kuscia/v1alpha1/...（其他 *_types.go）
```

**自动生成的文件**（也需要提交到 Git）：

```
pkg/crd/apis/kuscia/v1alpha1/zz_generated.deepcopy.go
pkg/crd/clientset/versioned/*
pkg/crd/listers/kuscia/v1alpha1/*
pkg/crd/informers/externalversions/*
```

**生成脚本**（手动编写）：

```
hack/update-codegen.sh
hack/generate-groups.sh
hack/boilerplate.go.txt
```

---

## 总结

通过本指南，您应该已经掌握了：

1. ✅ **为什么需要代码生成**：减少重复劳动，保证代码质量
2. ✅ **代码生成器的工作原理**：deepcopy-gen, client-gen, lister-gen, informer-gen
3. ✅ **如何在 Kuscia 中实践**：从类型定义到生成完整流程
4. ✅ **生成的代码如何使用**：Clientset、Lister、Informer 的实际应用
5. ✅ **常见问题如何解决**：5 个典型问题和调试技巧
6. ✅ **最佳实践**：类型设计、性能优化、测试策略

**下一步建议**：

- 尝试自己添加一个新的 CRD 资源
- 阅读生成的代码，理解其实现细节
- 研究 Kubernetes 官方的 Controller 示例
- 探索更高级的特性（如 Webhook、Conversion）

祝您学习愉快！🎉
