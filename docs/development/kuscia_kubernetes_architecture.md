# Kuscia 中 Kubernetes 配置与运行模式详解

## 1. 目录

- [2. 核心问题解答](#2-核心问题解答)
- [3. Kuscia 的三种运行模式](#3-kuscia-的三种运行模式)
- [4. 嵌入式 K3s 架构](#4-嵌入式-k3s-架构)
- [5. Kubernetes 配置的适用性分析](#5-kubernetes-配置的适用性分析)
- [6. 命名空间隔离机制](#6-命名空间隔离机制)
- [7. 网络与服务发现补充说明](#7-网络与服务发现补充说明)
- [8. 容器运行时支持](#8-容器运行时支持)
- [9. 实际应用场景对比](#9-实际应用场景对比)
- [10. 常见问题 FAQ](#10-常见问题-faq)
- [11. 嵌入式 K3s 中运行的功能](#11-嵌入式-k3s-中运行的功能)
- [12. Kuscia 如何管理 K8s](#12-kuscia-如何管理-k8s)
- [13. Kuscia 镜像体系](#13-kuscia-镜像体系)
- [14. 总结](#14-总结)

---

## 2. 核心问题解答

### 2.1 Kubernetes 与 K3s 的区别

**K3s 是什么？**

K3s 是 Rancher Labs 开发的轻量级 Kubernetes 发行版，专为资源受限环境设计。

**主要区别对比**：

| 特性 | Kubernetes (标准版) | K3s | Kuscia 中的 K3s |
| ------ | ------------------- | ----- | ---------------- |
| **二进制大小** | > 2GB | ~100MB | ~100MB |
| **内存占用** | > 2GB | 512MB-1GB | ~300MB (空闲) |
| **CPU 要求** | 多核 (推荐 2C+) | 1核即可 | 1核即可 |
| **依赖组件** | 需要独立安装 etcd, CNI, CoreDNS 等 | 集成 SQLite 默认存储 | 内置 etcd，精简组件 |
| **部署复杂度** | 复杂，需要多组件协调 | 简单，单二进制文件 | 作为 Kuscia 子进程 |
| **启动时间** | 几分钟 | 30-60秒 | 10-30秒 |
| **适用场景** | 大规模生产集群 | 边缘计算、IoT、开发测试 | 隐私计算、联邦学习 |
| **认证授权** | 完整的 RBAC、OIDC 等 | 支持标准 RBAC | 支持 RBAC，与 Kuscia 集成 |
| **网络插件** | 支持多种 CNI 插件 | 内置 Flannel，默认支持多种 | 禁用 Flannel，自定义网络方案 |
| **存储插件** | 支持多种 CSI 插件 | 支持 CSI 和本地存储 | 主要使用 etcd 存储 CRD |
| **组件完整性** | 完整的 K8s 组件 | 精简版，移除非必要组件 | 进一步精简，仅保留核心功能 |

**K3s 的精简策略**：

K3s 通过以下方式实现轻量化：

1. **移除非必要组件**：
   - 移除了 alpha 特性
   - 移除了非默认的 storage drivers
   - 移除了非默认的云提供商插件

2. **集成关键组件**：
   - 集成了 SQLite 作为默认存储（可选）
   - 集成了 CoreDNS
   - 集成了 Traefik ingress controller

3. **优化依赖**：
   - 用 SQLite 替代 etcd 作为默认存储
   - 将多个组件打包到单个二进制文件中

**Kuscia 中的特殊定制**：

Kuscia 使用的 K3s 进行了额外定制：

```go
// 来自 cmd/kuscia/modules/k3s.go
args := []string{
    "server",
    "--disable-agent",              // 禁用 Kubelet（不需要节点代理）
    "--disable-scheduler",          // 禁用默认调度器（Kuscia 有自己的调度器）
    "--flannel-backend=none",       // 禁用网络插件（使用自定义网络）
    "--disable=traefik",            // 禁用 Ingress（使用 Kuscia Gateway）
    "--disable=coredns",            // 禁用 DNS（使用自定义方案）
    "--disable=servicelb",          // 禁用负载均衡
    "--disable=local-storage",      // 禁用本地存储
    "--disable=metrics-server",     // 禁用监控（使用自定义监控）
}
```

**性能对比**：

| 指标 | 标准 K8s | K3s | Kuscia K3s |
| ------ | ---------- | ----- | ------------ |
| 内存占用 | >2GB | 512MB-1GB | ~300MB |
| CPU 占用 | 多核 | 单核 | 单核 |
| 启动时间 | >5分钟 | 1分钟内 | ~30秒 |
| 磁盘占用 | >10GB | ~500MB | ~500MB |

**为什么 Kuscia 选择定制 K3s 而不是标准 K8s？**

1. **资源效率**：Kuscia 通常部署在资源受限环境，需要更轻量的解决方案
2. **集成度**：K3s 可以更容易地嵌入到 Kuscia 中作为子进程运行
3. **定制化**：可以根据隐私计算场景的需要移除不必要的功能
4. **部署简化**：单二进制文件部署，降低运维复杂度
5. **兼容性**：保持与 Kubernetes API 的兼容性，可以使用相同的客户端库

**功能保留情况**：

尽管进行了大量精简，Kuscia 中的 K3s 仍保留了核心功能：

✅ **API Server** - 提供完整的 Kubernetes API

✅ **etcd 存储** - 存储 CRD 和系统对象

✅ **CRD 支持** - 可以定义和使用自定义资源 

✅ **RBAC** - 完整的权限控制系统 

✅ **Informer 机制** - 支持控制器模式

✅ **Namespace 隔离** - 提供逻辑隔离

这些核心功能足以支撑 Kuscia 的隐私计算和联邦学习工作负载，同时避免了标准 K8s 的复杂性和资源开销。

---

### 2.2 问题1：如果 Kuscia 不运行在容器中，Kubernetes 配置还起作用吗？

**简短回答：是的，完全起作用！**

**详细解释**：

Kuscia 的设计非常巧妙，它采用了 **"嵌入式 Kubernetes"** 架构。这意味着：

1. ✅ **Kuscia 不依赖外部 Kubernetes 集群**
   - 它自己启动一个嵌入式的 K3s（轻量级 Kubernetes）
   - K3s 作为 Kuscia 进程的一个子进程运行
   - 无论 Kuscia 运行在容器内还是宿主机上，K3s 都会启动

2. ✅ **所有 Kubernetes API 和配置都正常工作**
   - CRD（Custom Resource Definition）完全可用
   - Namespace 隔离机制正常运作
   - RBAC 权限控制有效
   - Informer/Controller 机制完整

3. ✅ **Kubernetes 概念被“复用”而非“依赖”**
   - Kuscia 不是“运行在 Kubernetes 上的应用”
   - Kuscia 是“自带 Kubernetes 的系统”
   - K8s API 是 Kuscia 的内部接口层

---

### 2.3 设计思想与流程通俗解释

#### 2.3.1 Kuscia 的整体设计思想

Kuscia 的设计思想可以用一句话概括：**“轻量化、自包含、易部署”**。让我们通过一个生活中的类比来理解：

**类比：Kuscia 如同一个“智能厨房”**

```
传统厨房（复杂的部署环境）：
┌─────────────────────────────────────────────────────┐
│  燃气灶（K8s API Server）                          │
│  冰箱（etcd 存储）                                 │
│  水龙头（认证模块）                                │
│  橱柜（资源管理）                                  │
│  需要分别购买、安装、配置、维护                   │
└─────────────────────────────────────────────────────┘

智能厨房（Kuscia）：
┌─────────────────────────────────────────────────────┐
│  一体化厨房（K3s）                                  │
│  ├─ 燃气灶（API Server）                            │
│  ├─ 冰箱（etcd）                                    │
│  ├─ 水龙头（认证）                                  │
│  └─ 橱柜（资源管理）                                │
│  一键启动，即插即用                                │
└─────────────────────────────────────────────────────┘
```

**Kuscia 的设计哲学**：

1. **一体化集成**：将所有必要组件打包在一起，避免复杂的外部依赖
2. **自包含运行**：不需要外部环境，自己管理自己的运行时
3. **标准化接口**：使用业界通用的 Kubernetes 接口，降低学习成本
4. **轻量化部署**：最小化资源占用，适应边缘计算等资源受限场景

#### 2.3.2 Kuscia 的工作流程

##### 2.3.2.1 流程图解：从启动到运行

```
┌─────────────────────────────────────────────────────────────┐
│                    Kuscia 启动流程                           │
├─────────────────────────────────────────────────────────────┤
│  1. 主进程启动                                              │
│     ↓                                                      │
│  2. 启动嵌入式 K3s 子进程                                   │
│     ↓                                                      │
│  3. K3s 初始化 API Server、etcd、Controller Manager        │
│     ↓                                                      │
│  4. 注册 CRD（DomainData、DomainDataGrant 等）              │
│     ↓                                                      │
│  5. 创建 Namespace（alice、bob、charlie 等）                │
│     ↓                                                      │
│  6. 启动业务控制器（DomainData Controller 等）              │
│     ↓                                                      │
│  7. 监听资源变化，开始正常工作                            │
└─────────────────────────────────────────────────────────────┘
```

##### 2.3.2.2 详细流程说明

**第一步：主进程启动**

当您运行 `./kuscia start` 命令时，Kuscia 主进程首先启动，此时它只是一个普通的 Go 程序，还没有任何 Kubernetes 功能。

```bash
# 启动命令
./kuscia start --config autonomy_alice.yaml
```

**第二步：启动嵌入式 K3s**

Kuscia 主进程调用 `exec.Command` 启动 K3s 子进程，这相当于在程序内部启动了一个完整的 Kubernetes 系统：

```go
// cmd/kuscia/modules/k3s.go
func (s *k3sModule) Run(ctx context.Context) error {
    // 构建 K3s 启动参数
    args := []string{
        "server",
        "-d=" + s.dataDir,                    // 数据目录
        "-o=" + s.kubeconfigFile,             // 生成 kubeconfig
        "--disable-agent",                   // 禁用 agent
        "--disable-scheduler",               // 禁用默认调度器
        "--flannel-backend=none",            // 禁用网络插件
        // ... 更多参数
    }
    
    // 启动 K3s 子进程
    cmd := exec.Command(filepath.Join(s.rootDir, "bin/k3s"), args...)
    cmd.Start()
    
    return nil
}
```

**第三步：K3s 初始化**

K3s 子进程启动后，会初始化以下组件：

- **API Server**：提供 RESTful API 接口
- **etcd**：分布式键值存储，保存所有资源对象
- **Controller Manager**：运行内置控制器（如 Namespace Controller）

**第四步：注册 CRD**

Kuscia 会自动注册所有自定义资源定义（CRD），比如 DomainData、DomainDataGrant 等：

```go
// Kuscia 自动执行 kubectl apply -f crd_yaml_files
func initKusciaEnvAfterReady(ctx context.Context) error {
    crdFiles := []string{
        "crds/v1alpha1/kuscia.secretflow_domaindatas.yaml",
        "crds/v1alpha1/kuscia.secretflow_domaindatagrants.yaml",
        // ... 更多 CRD 文件
    }
    
    for _, crdFile := range crdFiles {
        cmd := exec.Command("kubectl", "apply", "-f", crdFile)
        cmd.Run()
    }
    return nil
}
```

**详细解释：Kuscia 会自动注册所有自定义资源定义（CRD）**

1. **CRD 定义来源**：Kuscia 预定义了一系列 CRD 文件，存储在 `crds/v1alpha1/` 目录下，包括但不限于：
   - `kuscia.secretflow_domaindatas.yaml` - 定义 DomainData 资源
   - `kuscia.secretflow_domaindatagrants.yaml` - 定义 DomainDataGrant 资源
   - `kuscia.secretflow_domains.yaml` - 定义 Domain 资源
   - `kuscia.secretflow_kusciajobs.yaml` - 定义 KusciaJob 资源
   - `kuscia.secretflow_kusciatasks.yaml` - 定义 KusciaTask 资源
   - 以及其他业务相关的 CRD 定义

2. **自动注册时机**：在 K3s 启动并就绪后，Kuscia 会自动执行 CRD 注册流程，确保所有自定义资源类型在系统启动时就已经可用。

3. **注册方式**：通过调用内置的 kubectl 命令，将 CRD 定义应用到 K3s 集群中，使 API Server 能够识别和处理这些自定义资源。

4. **注册效果**：一旦 CRD 注册成功，用户就可以通过标准的 Kubernetes API 来创建、更新、删除和查询这些自定义资源，就像使用原生的 Pod、Service 等资源一样。

**可用的标准API接口**：注册后，以下标准Kubernetes API接口可自动使用：

| HTTP方法 | API路径 | 说明 | 示例 |
| --------- | -------- | ------ | ------ |
| GET | `/apis/kuscia.secretflow/v1alpha1/namespaces/{namespace}/{resources}` | 列出指定命名空间下的资源 | `GET /apis/kuscia.secretflow/v1alpha1/namespaces/alice/domaindatas` |
| GET | `/apis/kuscia.secretflow/v1alpha1/namespaces/{namespace}/{resources}/{name}` | 获取指定资源 | `GET /apis/kuscia.secretflow/v1alpha1/namespaces/alice/domaindatas/user-table` |
| POST | `/apis/kuscia.secretflow/v1alpha1/namespaces/{namespace}/{resources}` | 创建资源 | `POST /apis/kuscia.secretflow/v1alpha1/namespaces/alice/domaindatas` |
| PUT | `/apis/kuscia.secretflow/v1alpha1/namespaces/{namespace}/{resources}/{name}` | 更新资源 | `PUT /apis/kuscia.secretflow/v1alpha1/namespaces/alice/domaindatas/user-table` |
| PATCH | `/apis/kuscia.secretflow/v1alpha1/namespaces/{namespace}/{resources}/{name}` | 部分更新资源 | `PATCH /apis/kuscia.secretflow/v1alpha1/namespaces/alice/domaindatas/user-table` |
| DELETE | `/apis/kuscia.secretflow/v1alpha1/namespaces/{namespace}/{resources}/{name}` | 删除资源 | `DELETE /apis/kuscia.secretflow/v1alpha1/namespaces/alice/domaindatas/user-table` |
| DELETE | `/apis/kuscia.secretflow/v1alpha1/namespaces/{namespace}/{resources}` | 批量删除资源 | `DELETE /apis/kuscia.secretflow/v1alpha1/namespaces/alice/domaindatas` |

**自动生成的客户端接口**：Kubernetes 会自动生成 CRD 的客户端接口实现，包括：

- **Clientset**：提供强类型的 Go 客户端，用于与 API Server 通信
- **Lister**：提供本地缓存和索引功能，避免频繁的 API 调用
- **Informer**：提供事件驱动的通知机制，支持监听资源变化
- **SharedInformer**：高效的共享缓存机制，减少重复的 API 请求

**代码生成过程**：这些接口是通过 `k8s.io/code-generator` 工具自动生成的，基于 CRD 类型定义中的 `+genclient`、`+k8s:deepcopy-gen` 等注释标记。在 Kuscia 中，可以通过运行 `./hack/update-codegen.sh` 脚本来生成这些客户端代码。生成的代码位于 `pkg/crd/clientset`、`pkg/crd/listers` 和 `pkg/crd/informers` 目录下。

**编程示例**：开发者可以直接使用这些自动生成的接口来操作自定义资源，例如：

```go
// 创建 DomainData 资源
clientSet, _ := versioned.NewForConfig(config)
domainData := &v1alpha1.DomainData{
    ObjectMeta: metav1.ObjectMeta{Name: "my-data", Namespace: "alice"},
    Spec: v1alpha1.DomainDataSpec{...},
}
result, err := clientSet.KusciaV1alpha1().DomainDatas("alice").Create(ctx, domainData, metav1.CreateOptions{})

// 使用 Informer 监听资源变化
informerFactory := informers.NewSharedInformerFactory(clientSet, time.Hour)
domainDataInformer := informerFactory.Kuscia().V1alpha1().DomainDatas()
informer := domainDataInformer.Informer()
informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc: func(obj interface{}) {
        dd := obj.(*v1alpha1.DomainData)
        fmt.Printf("DomainData %s created\n", dd.Name)
    },
})
```

这种方式避免了手动编写底层的 HTTP 请求代码，提供了类型安全的接口和丰富的功能。

**常用资源类型**：

- `domaindatas` - 域数据资源
- `domaindatagrants` - 域数据授权资源
- `domains` - 域资源
- `kusciajobs` - Kuscia作业资源
- `kusciatasks` - Kuscia任务资源
- `kusciadeployments` - Kuscia部署资源
- `domainroutes` - 域路由资源
- `gateways` - 网关资源
- `appimages` - 应用镜像资源

**使用示例**：

```bash
# 列出所有域数据
kubectl get domaindatas -A

# 在特定命名空间中创建域数据
kubectl create -f domaindata.yaml -n alice

# 获取特定域数据
kubectl get domaindata user-table -n alice

# 删除特定域数据
kubectl delete domaindata user-table -n alice

# 使用REST API直接调用
curl -X GET http://localhost:8080/apis/kuscia.secretflow/v1alpha1/namespaces/alice/domaindatas
```

**第五步：创建 Namespace**

为每个参与方创建独立的命名空间：

```yaml
# 为 Alice 创建 Namespace
apiVersion: v1
kind: Namespace
metadata:
  name: alice
```

**第六步：启动业务控制器**

启动各种业务控制器，这些控制器会监听资源变化并执行相应操作：

```go
// DomainData Controller
func (c *Controller) syncDomainDataGrantHandler(ctx context.Context, key string) error {
    // 监听 DomainDataGrant 的创建/更新
    dg, err := c.domainDataGrantLister.Get(key)
    
    // 执行业务逻辑（跨域同步、权限检查等）
    err = c.ensureDomainData(dg)
    
    // 更新状态
    updateStatus(dg, phase, message)
    
    return nil
}
```

#### 2.3.3 为什么选择这种设计？

##### 2.3.3.1 优势对比

| 传统方式 | Kuscia 方式 |
| --------- | ------------ |
| 需要预先安装 Kubernetes 集群 | 一键启动，无需外部依赖 |
| 需要手动注册 CRD | 自动注册，开箱即用 |
| 需要管理多个组件 | 一体化管理，简化运维 |
| 资源占用大（几 GB） | 轻量化（几百 MB） |
| 部署复杂（需要专业知识） | 简单部署（普通用户即可） |

##### 2.3.3.2 解决的核心问题

1. **部署门槛高**：传统 K8s 部署需要专业的运维知识，Kuscia 将其简化为一键启动
2. **资源消耗大**：传统 K8s 需要大量资源，Kuscia 优化为轻量化部署
3. **运维复杂**：多个组件需要分别管理，Kuscia 统一管理
4. **标准化接口**：虽然简化了部署，但仍然使用标准的 K8s API，保持了生态兼容性

#### 2.3.4 实际运行场景

##### 2.3.4.1 场景 1：单机开发

```bash
# 在笔记本电脑上运行
./kuscia start --config autonomy_dev.yaml --rootless

# 立即可用标准 K8s API
kubectl get domaindatas -A
kubectl apply -f my-data.yaml
```

##### 2.3.4.2 场景 2：边缘计算

```bash
# 在边缘设备上运行（资源受限）
./kuscia start --config autonomy_edge.yaml

# 轻量化运行，满足边缘计算需求
```

##### 2.3.4.3 场景 3：生产环境

```bash
# 在生产环境中运行
./kuscia start --config autonomy_prod.yaml

# 使用外部数据库，支持高可用
```

这种设计使得 Kuscia 既保持了 Kubernetes 的强大功能，又大大降低了使用门槛，真正做到了“即插即用”。

---

### 2.4 问题2：为什么 Kuscia 要用 Kubernetes？直接用 Go 代码不行吗？

**答：理论上可以，但实际开发工作量会巨大，且难以保证质量和可维护性。**

Kubernetes 提供了**成熟的抽象和生态**，让开发工作量大幅降低。下面通过具体例子详细说明。

---

#### 2.4.1 Kubernetes 提供的核心抽象能力

##### 2.4.1.1 **声明式 API（Declarative API）**

**Kubernetes 方式**：

```yaml
# 用户只需要声明“要什么”，不需要关心“怎么做”
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: user-table
  namespace: alice
spec:
  name: "用户行为数据"
  type: "table"
  dataSource: "localfs-001"
```

**纯 Go 实现需要**：

```go
// ❌ 需要自己设计 API 协议
type CreateDomainDataRequest struct {
    Name       string `json:"name"`
    Type       string `json:"type"`
    DataSource string `json:"dataSource"`
}

type UpdateDomainDataRequest struct {
    // ... 又要定义更新协议
}

type DeleteDomainDataRequest struct {
    // ... 又要定义删除协议
}

// ❌ 需要自己实现 HTTP/gRPC Server
func (s *Server) CreateDomainData(w http.ResponseWriter, r *http.Request) {
    var req CreateDomainDataRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    // 验证参数
    if req.Name == "" {
        w.WriteHeader(400)
        return
    }
    
    // 检查权限
    if !s.checkPermission(r.Context(), req.Namespace) {
        w.WriteHeader(403)
        return
    }
    
    // 写入数据库
    err := s.db.Create(&req)
    if err != nil {
        w.WriteHeader(500)
        return
    }
    
    // 返回结果
    json.NewEncoder(w).Encode(req)
}

// ❌ 需要自己实现 Update、Delete、Get、List... 至少 5 个方法
// ❌ 每个方法都要重复验证、鉴权、错误处理逻辑
```

**工作量对比**：

| 功能 | Kubernetes CRD | 纯 Go 实现 |
| ------ | --------------- | ----------- |
| API 定义 | YAML 声明 | 手动定义 Request/Response 结构 |
| HTTP Server | API Server 内置 | 需要自己实现路由、Handler |
| 参数验证 | OpenAPI Schema 自动生成 | 每个字段手动验证 |
| 权限控制 | RBAC 内置 | 自己实现鉴权中间件 |
| 错误处理 | 标准错误码 | 自己定义错误码体系 |
| 文档生成 | kubectl explain 自动支持 | 手动编写 API 文档 |
| **代码行数** | **~100 行（类型定义）** | **~2000+ 行** |

---

##### 2.4.1.2 **Watch 机制（事件驱动）**

**Kubernetes 方式**：

```go
// ✅ 使用 Informer，几十行代码搞定
informer := factory.Kuscia().V1alpha1().DomainDatas()
informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc: func(obj interface{}) {
        dd := obj.(*v1alpha1.DomainData)
        handleNewDomainData(dd)
    },
    UpdateFunc: func(oldObj, newObj interface{}) {
        oldDD := oldObj.(*v1alpha1.DomainData)
        newDD := newObj.(*v1alpha1.DomainData)
        handleUpdateDomainData(oldDD, newDD)
    },
    DeleteFunc: func(obj interface{}) {
        dd := obj.(*v1alpha1.DomainData)
        handleDeleteDomainData(dd)
    },
})

factory.Start(stopCh)
factory.WaitForCacheSync(stopCh)
```

**纯 Go 实现需要**：

```go
// ❌ 需要自己实现长轮询 Watch 机制
func (s *Server) WatchDomainDatas(ctx context.Context, sinceRevision int64) (<-chan Event, error) {
    eventCh := make(chan Event, 100)
    
    go func() {
        defer close(eventCh)
        
        for {
            select {
            case <-ctx.Done():
                return
            default:
                // 查询数据库，找出变化的记录
                changes, err := s.db.GetChangesSince(sinceRevision)
                if err != nil {
                    log.Printf("Error: %v", err)
                    continue
                }
                
                for _, change := range changes {
                    // 发送事件
                    select {
                    case eventCh <- change.ToEvent():
                    case <-ctx.Done():
                        return
                    }
                }
                
                // 更新 revision
                sinceRevision = changes.LastRevision()
                
                // 避免频繁轮询
                time.Sleep(100 * time.Millisecond)
            }
        }
    }()
    
    return eventCh, nil
}

// ❌ 需要自己处理：
// - 连接断开重连
// - 事件丢失补偿
// - 并发控制（多个 Watcher 同时监听）
// - 内存管理（事件队列积压）
// - 心跳保活
```

**工作量对比**：

| 功能 | Kubernetes Informer | 纯 Go 实现 |
| ------ | --------------------- | ----------- |
| 事件监听 | 内置 Watch API | 自己实现长轮询 |
| 本地缓存 | Indexer 自动维护 | 自己实现缓存结构 |
| 断线重连 | Reflector 自动处理 | 自己实现重试逻辑 |
| 事件过滤 | Label Selector 支持 | 自己实现过滤算法 |
| 并发控制 | 线程安全 | 自己加锁 |
| **代码行数** | **~30 行** | **~500+ 行** |

---

##### 2.4.1.3 **存储与序列化**

**Kubernetes 方式**：

```go
// ✅ etcd 存储 + Protocol Buffer 序列化由 K8s 自动处理
domainData := &v1alpha1.DomainData{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "user-table",
        Namespace: "alice",
    },
    Spec: v1alpha1.DomainDataSpec{
        Name: "用户数据",
        Type: "table",
    },
}

// 一行代码完成创建（自动序列化、写入 etcd）
result, _ := client.KusciaV1alpha1().DomainDatas("alice").Create(
    ctx, domainData, metav1.CreateOptions{},
)
```

**纯 Go 实现需要**：

```go
// ❌ 需要自己选择存储引擎（MySQL? PostgreSQL? MongoDB?）
type DomainDataStore struct {
    db *sql.DB
}

func (s *DomainDataStore) Create(data *DomainData) error {
    // 手动序列化 JSON
    jsonData, err := json.Marshal(data)
    if err != nil {
        return err
    }
    
    // 手动编写 SQL
    query := `INSERT INTO domain_datas (namespace, name, data, created_at) 
              VALUES (?, ?, ?, NOW())`
    
    _, err = s.db.Exec(query, data.Namespace, data.Name, jsonData)
    if err != nil {
        // 处理主键冲突、数据类型错误等
        if isDuplicateKey(err) {
            return ErrAlreadyExists
        }
        return err
    }
    
    return nil
}

func (s *DomainDataStore) Get(namespace, name string) (*DomainData, error) {
    query := `SELECT data FROM domain_datas WHERE namespace = ? AND name = ?`
    
    var jsonData []byte
    err := s.db.QueryRow(query, namespace, name).Scan(&jsonData)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, ErrNotFound
        }
        return nil, err
    }
    
    // 手动反序列化
    var data DomainData
    err = json.Unmarshal(jsonData, &data)
    if err != nil {
        return nil, err
    }
    
    return &data, nil
}

// ❌ 还要实现 Update、Delete、List、分页、排序...
// ❌ 还要处理事务、锁、并发控制
// ❌ 还要做数据迁移、版本升级
```

**工作量对比**：

| 功能 | Kubernetes etcd | 纯 Go 实现 |
| ------ | ---------------- | ----------- |
| 数据存储 | etcd 内置 | 自己选数据库、建表 |
| 序列化 | Protocol Buffer 自动生成 | 手动 JSON 序列化 |
| 事务支持 | etcd 原子操作 | 自己实现事务 |
| 并发控制 | MVCC 内置 | 自己加锁 |
| 数据备份 | etcd snapshot | 自己实现备份工具 |
| 数据迁移 | CRD Versioning | 手动写迁移脚本 |
| **代码行数** | **0 行（直接使用）** | **~1000+ 行** |

---

##### 2.4.1.4 **缓存与性能优化**

**Kubernetes 方式**：

```go
// ✅ Lister 提供带索引的本地缓存，查询速度 <1μs
lister := v1alpha1.NewDomainDataLister(informer.GetIndexer())

// 按 namespace 查询（从内存读取，超快）
dataList, _ := lister.DomainDatas("alice").List(labels.Everything())

// 按 label 过滤（索引加速）
selector, _ := labels.Parse("type=table")
tableList, _ := lister.DomainDatas("alice").List(selector)
```

**纯 Go 实现需要**：

```go
// ❌ 需要自己实现缓存系统
type DomainDataCache struct {
    mu    sync.RWMutex
    store map[string]*DomainData  // key: namespace/name
    index map[string]map[string]*DomainData  // index: label -> objects
}

func (c *DomainDataCache) Add(data *DomainData) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    key := data.Namespace + "/" + data.Name
    c.store[key] = data
    
    // 维护索引
    for k, v := range data.Labels {
        if c.index[k] == nil {
            c.index[k] = make(map[string]*DomainData)
        }
        c.index[k][v] = data
    }
}

func (c *DomainDataCache) ListByLabel(labelKey, labelValue string) []*DomainData {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    var result []*DomainData
    if idx, ok := c.index[labelKey]; ok {
        for v, data := range idx {
            if v == labelValue {
                result = append(result, data)
            }
        }
    }
    return result
}

// ❌ 还要处理：
// - 缓存失效（对象更新/删除时同步）
// - 内存限制（缓存太大怎么办）
// - 缓存预热（启动时加载全量数据）
// - 并发安全（读写锁优化）
```

**工作量对比**：

| 功能 | Kubernetes Lister | 纯 Go 实现 |
| ------ | ------------------- | ----------- |
| 本地缓存 | Indexer 自动维护 | 自己实现 Map 结构 |
| 索引查询 | Label Index 内置 | 自己维护索引 Map |
| 缓存同步 | Informer 自动更新 | 自己监听变化 |
| 线程安全 | 内置读写锁 | 自己加锁 |
| 内存管理 | GC 自动回收 | 自己监控内存 |
| **代码行数** | **0 行（直接使用）** | **~300+ 行** |

---

##### 2.4.1.5 **权限控制（RBAC）**

**Kubernetes 方式**：

```yaml
# ✅ 声明式配置，几行 YAML 搞定
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: alice
  name: domaindata-reader
rules:
  - apiGroups: ["kuscia.secretflow"]
    resources: ["domaindatas"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  namespace: alice
  name: alice-binding
subjects:
  - kind: ServiceAccount
    name: alice-sa
roleRef:
  kind: Role
  name: domaindata-reader
```

**纯 Go 实现需要**：

```go
// ❌ 需要自己设计权限模型
type Permission struct {
    Resource string
    Verb     string
    Scope    string  // namespace or cluster
}

type Role struct {
    Name        string
    Permissions []Permission
}

type RBACMiddleware struct {
    roles map[string]Role
    bindings map[string]string  // user -> role
}

func (m *RBACMiddleware) CheckPermission(ctx context.Context, user, resource, verb string) error {
    // 查找用户的角色
    roleName, ok := m.bindings[user]
    if !ok {
        return ErrNoRole
    }
    
    // 查找角色的权限
    role, ok := m.roles[roleName]
    if !ok {
        return ErrRoleNotFound
    }
    
    // 检查是否有权限
    for _, perm := range role.Permissions {
        if perm.Resource == resource && perm.Verb == verb {
            return nil
        }
    }
    
    return ErrPermissionDenied
}

// ❌ 还要实现：
// - 权限缓存（避免每次查询数据库）
// - 权限继承（角色嵌套）
// - 动态更新（运行时修改权限）
// - 审计日志（记录谁做了什么操作）
```

**工作量对比**：

| 功能 | Kubernetes RBAC | 纯 Go 实现 |
| ------ | ---------------- | ----------- |
| 权限定义 | YAML 声明 | 自己设计数据结构 |
| 权限检查 | API Server 自动拦截 | 每个 Handler 手动调用 |
| 权限缓存 | 内置缓存 | 自己实现 |
| 审计日志 | Audit Policy 配置 | 自己记录日志 |
| 动态更新 | 热更新支持 | 重启或复杂同步逻辑 |
| **代码行数** | **~20 行 YAML** | **~500+ 行** |

---

##### 2.4.1.6 **可扩展性（Webhook/Admission Control）**

**Kubernetes 方式**：

```go
// ✅ 实现 Admission Webhook，几行代码扩展验证逻辑
func (w *Webhook) ValidateDomainData(ar v1.AdmissionReview) v1.AdmissionResponse {
    dd := &v1alpha1.DomainData{}
    json.Unmarshal(ar.Request.Object.Raw, dd)
    
    // 自定义验证逻辑
    if dd.Spec.Type != "table" && dd.Spec.Type != "model" {
        return v1.AdmissionResponse{
            Allowed: false,
            Result: &metav1.Status{
                Message: "Invalid domain data type",
            },
        }
    }
    
    return v1.AdmissionResponse{Allowed: true}
}
```

**纯 Go 实现需要**：

```go
// ❌ 需要在每个创建/更新方法中插入验证逻辑
func (s *Server) CreateDomainData(w http.ResponseWriter, r *http.Request) {
    var req CreateDomainDataRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    // 硬编码验证逻辑（耦合严重）
    if req.Type != "table" && req.Type != "model" {
        w.WriteHeader(400)
        w.Write([]byte("Invalid type"))
        return
    }
    
    // ... 后续逻辑
}

// ❌ 如果要新增验证规则，需要修改所有相关代码
// ❌ 无法动态插拔验证逻辑
```

---

#### 2.4.2 Kubernetes 生态带来的红利

##### 2.4.2.1 **工具生态**

**kubectl 命令行工具**：

```bash
# ✅ 无需开发任何 UI，立即拥有完整的 CLI
kubectl get domaindatas -A
kubectl describe domaindata user-table -n alice
kubectl edit domaindata user-table -n alice
kubectl delete domaindata user-table -n alice
kubectl explain domaindata.spec  # 查看字段说明
```

**纯 Go 实现需要**：

- ❌ 自己开发 CLI 工具（cobra/cli）
- ❌ 自己实现表格输出
- ❌ 自己实现交互式编辑
- ❌ 自己实现帮助文档
- **工作量**：~2000 行

---

##### 2.4.2.2 **监控生态**

**Prometheus 集成**：

```go
// ✅ Kubernetes 指标自动暴露
// /metrics 端点自动包含：
// - apiserver_request_total
// - etcd_requests_duration_seconds
// - workqueue_depth
```

**纯 Go 实现需要**：

- ❌ 自己埋点
- ❌ 自己暴露指标
- ❌ 自己定义指标规范
- **工作量**：~500 行

---

##### 2.4.2.3 **测试生态**

**Fake Client 单元测试**：

```go
// ✅ Kubernetes 提供 Fake Client，无需真实集群
func TestDomainDataController(t *testing.T) {
    client := fake.NewSimpleClientset()
    
    // 创建测试对象
    dd := &v1alpha1.DomainData{...}
    client.KusciaV1alpha1().DomainDatas("alice").Create(ctx, dd)
    
    // 执行测试
    controller := NewController(client)
    err := controller.Sync("alice/test-data")
    
    assert.NoError(t, err)
}
```

**纯 Go 实现需要**：

- ❌ 自己 Mock 数据库
- ❌ 自己 Mock HTTP Server
- ❌ 自己构造测试数据
- **工作量**：~300 行/测试用例

---

##### 2.4.2.4 **社区支持与人才储备**

**Kubernetes 技能通用**：

- ✅ 工程师学习 Kubernetes 后可以快速上手 Kuscia
- ✅ 遇到问题可以搜索到大量 K8s 相关资料
- ✅ 招聘容易（K8s 工程师很多）

**纯 Go 实现**：

- ❌ 需要自己编写完整文档
- ❌ 遇到问题无人可问
- ❌ 只能招聘熟悉该系统的人

---

#### 2.4.3 实际代码量对比

以实现 **DomainData 资源的 CRUD + Watch + 缓存 + 权限控制** 为例：

| 模块 | Kubernetes CRD | 纯 Go 实现 |
| ------ | --------------- | ----------- |
| **类型定义** | 100 行 | 100 行 |
| **API Server** | 0 行（内置） | 500 行 |
| **存储层** | 0 行（etcd） | 1000 行 |
| **Watch 机制** | 30 行（Informer） | 500 行 |
| **缓存系统** | 0 行（Lister） | 300 行 |
| **权限控制** | 20 行（YAML） | 500 行 |
| **错误处理** | 0 行（标准） | 200 行 |
| **日志审计** | 0 行（内置） | 300 行 |
| **单元测试** | 100 行（Fake） | 500 行 |
| **CLI 工具** | 0 行（kubectl） | 2000 行 |
| **文档** | 0 行（自动生成） | 500 行 |
| **总计** | **~250 行** | **~6400 行** |

**结论**：使用 Kubernetes CRD 可以减少 **96%** 的代码量！

---

#### 2.4.4 长期维护成本对比

| 维度 | Kubernetes CRD | 纯 Go 实现 |
| ------ | --------------- | ----------- |
| **新功能开发** | 快速（复用现有框架） | 慢（需要改造基础设施） |
| **Bug 修复** | 少（K8s 已验证） | 多（自己踩坑） |
| **性能优化** | 自动享受 K8s 优化 | 自己调优 |
| **安全补丁** | K8s 团队定期发布 | 自己发现并修复 |
| **人员流动** | 新人易上手 | 老人离职后无人懂 |
| **技术债务** | 低（跟随上游） | 高（累积自定义代码） |

---

#### 2.4.5 小结

**Kubernetes 提供的核心价值**：

1. ✅ **标准化抽象**：API、存储、缓存、权限等都有成熟模式
2. ✅ **开箱即用**：无需重复造轮子
3. ✅ **生态丰富**：工具、监控、测试、文档一应俱全
4. ✅ **质量保障**：经过全球数百万集群验证
5. ✅ **人才通用**：K8s 技能可迁移

**Kuscia 的选择**：

- ✅ 使用 Kubernetes CRD 管理业务对象
- ✅ 复用 K8s API、etcd、Informer、RBAC
- ✅ 专注业务逻辑（隐私计算调度、跨域通信等）
- ✅ 避免重复开发基础设施

**这就是为什么 Kuscia 选择 Kubernetes，而不是纯 Go 实现！**

---

## 3. Kuscia 的三种运行模式

**控制平面的含义**

控制平面是 Kubernetes 集群的大脑，负责管理整个集群的状态和协调各种操作。在 Kuscia 中，控制平面包含了一系列核心组件。

**主要作用**

控制平面主要有以下几个关键作用：

- ✅ 集群管理：维护集群的整体状态，包括节点健康状况、资源分配等
- ✅ 工作负载调度：决定在哪些节点上运行应用容器，并确保期望状态与实际状态一致
- ✅ API 服务：提供 REST API 接口供用户和其他组件与集群交互
- ✅ 状态协调：持续监控集群状态，并根据需要进行调整以达到期望状态
- ✅ 控制器管理：运行各种控制器来处理特定类型的资源（如 Pod 控制器、服务控制器等）

**在 Kuscia 中的应用**
Kuscia 有两种运行模式：

- **Autonomy Mode**：Kuscia 自有控制平面，可以独立运行
- **Lite Mode**：不自带控制平面，作为工作节点连接到 Master 节点
这种设计允许 Kuscia 在不同的部署场景下灵活使用，既可以在独立环境中运行完整功能，也可以作为更大集群的一部分协同工作。
Lite 模式是作为工作节点接入到 Master 节点，这意味着它将控制平面的职责委托给 Master 节点，自己专注于本地的资源管理任务。

### 3.1 模式 1：Autonomy Mode（自治模式）

```
┌─────────────────────────────────────────────┐
│         宿主机 / VM / 容器                    │
│                                              │
│  ┌──────────────────────────────────────┐   │
│  │      Kuscia 主进程                    │   │
│  │                                      │   │
│  │  ┌────────────────────────────────┐  │   │
│  │  │  嵌入式 K3s (子进程)            │  │   │
│  │  │  - API Server                  │  │   │
│  │  │  - etcd (存储后端)              │  │   │
│  │  │  - Controller Manager          │  │   │
│  │  └────────────────────────────────┘  │   │
│  │                                      │   │
│  │  ┌────────────────────────────────┐  │   │
│  │  │  Kuscia Controllers             │  │   │
│  │  │  - DomainData Controller       │  │   │
│  │  │  - DomainDataGrant Controller  │  │   │
│  │  │  - KusciaJob Controller        │  │   │
│  │  └────────────────────────────────┘  │   │
│  │                                      │   │
│  │  ┌────────────────────────────────┐  │   │
│  │  │  HTTP/gRPC API Server           │  │   │
│  │  └────────────────────────────────┘  │   │
│  └──────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
```

**特点**：

- ✅ **无需外部 K8s 集群**
- ✅ **单二进制文件启动**
- ✅ **资源占用极低**（最低 1C2G）
- ✅ **适合边缘计算、单机部署**
- ✅ **可以在任何 Linux 环境运行**（容器内外均可）

**启动命令示例**：

```bash
# 直接在宿主机上运行（不需要 Docker）
./kuscia start \
  --config autonomy_alice.yaml \
  --rootless  # 非 root 用户

# 也可以在 Docker 中运行
docker run -d kuscia:latest start --config autonomy_alice.yaml
```

**配置文件示例** (`autonomy_alice.yaml`)：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: AutonomyConfig
metadata:
  name: alice
spec:
  domainID: alice
  master:
    # K3s 数据存储路径（本地文件系统）
    datastoreEndpoint: ""  # 空表示使用嵌入式 etcd
    
    # kubeconfig 文件路径
    kubeconfigFile: /var/lib/kuscia/etc/kubeconfig
  
  # 日志配置
  logLevel: info
  rootDir: /var/lib/kuscia
```

### 3.2 模式 2：Master Mode（连接外部 K8s）

```
┌──────────────────────────────────────────────┐
│     外部 Kubernetes 集群                      │
│  ┌────────────────────────────────────┐     │
│  │  API Server                        │     │
│  │  Scheduler                         │     │
│  │  etcd                              │     │
│  └────────────────────────────────────┘     │
└──────────────────┬──────────────────────────┘
                   │ kubeconfig
┌──────────────────▼──────────────────────────┐
│         Kuscia Master 节点                   │
│  ┌────────────────────────────────────┐    │
│  │  Kuscia Controllers                 │    │
│  │  (监听外部 K8s 的资源变化)           │    │
│  └────────────────────────────────────┘    │
└─────────────────────────────────────────────┘
```

**特点**：

- ⚠️ **需要外部 K8s 集群**
- ✅ **可以利用现有 K8s 基础设施**
- ✅ **适合大规模集群部署**
- ⚠️ **配置复杂度更高**

**使用场景**：

- 企业已有 K8s 集群
- 需要跨多个节点调度任务
- 需要 K8s 的高级功能（如 HPA、PDB 等）

### 3.3 模式 3：Lite Mode（接入节点）

Lite Mode 与 Autonomy Mode 不同，它**不自带控制平面**，而是作为一个工作节点接入到 Master 节点：

```text
┌──────────────────────────────────────────────┐
│           Master 节点                         │
│  ┌────────────────────────────────────┐     │
│  │  K3s + Kuscia Controllers           │     │
│  │  （负责调度与资源管理）              │     │
│  └──────────────────┬─────────────────┘     │
└─────────────────────┼───────────────────────┘
                      │ kubeconfig
┌─────────────────────▼───────────────────────┐
│           Lite 节点                          │
│  ┌────────────────────────────────────┐    │
│  │  Kuscia Agent / DataMesh / Envoy    │    │
│  │  （负责任务执行、数据访问、网络）     │    │
│  └────────────────────────────────────┘    │
└─────────────────────────────────────────────┘
```

**与 Autonomy 的区别**：

| 特性 | Autonomy | Lite |
| ------ | ---------- | ------ |
| 控制平面 | 自带 K3s | 无，依赖 Master |
| 部署位置 | 可独立部署 | 必须注册到 Master |
| 适用场景 | 单机构 P2P | 大型机构中心化组网 |
| 资源要求 | 稍高（需运行 K3s） | 较低 |

**Lite 节点启动要点**：

- 配置文件需要指向 Master 的 API Server 地址与 token。
- Lite 节点本身不启动 K3s，而是使用 Master 提供的 kubeconfig。
- 适合“一个 Master + 多个 Lite”的中心化组网。

### 3.4 选型建议

从实现上看，`cmd/kuscia/modules/runtime.go` 中的 `NewModuleRuntimeConfigs()` 会根据运行模式准备不同的 `ApiserverEndpoint`、`KubeconfigFile` 与客户端集合，因此三种模式的核心差异本质上是：**控制平面由谁提供，以及 Kuscia 的业务模块最终连到哪一个 API Server。**

**可以按下面的原则选型：**

1. **优先选择 Autonomy Mode**：当你需要单机自包含、可离线部署、希望尽量减少外部依赖时，它通常是默认首选。
2. **选择 Master Mode**：当你已经有外部 K8s 集群、希望复用既有调度与运维体系、或者需要更大的集群规模时，更适合采用该模式。
3. **理解 Lite 节点定位**：Lite 模式下 Kuscia 不再提供本地控制平面，而是更多承担“接入节点”的职责，直接使用指向 Master 的客户端配置。适合“一个 Master + 多个 Lite”的中心化组网。

---

## 4. 嵌入式 K3s 架构

### 4.1 K3s 是什么？

**K3s** 是 Rancher 开发的轻量级 Kubernetes 发行版，特点：

| 特性 | 说明 |
| ------ | ------ |
| **轻量化** | 二进制文件 < 100MB |
| **单文件** | 包含所有 K8s 组件 |
| **低资源** | 最低 512MB 内存即可运行 |
| **完全兼容** | 100% 通过 K8s 一致性测试 |
| **嵌入式 etcd** | 可选内置 etcd，无需外部数据库 |

### 4.2 Kuscia 如何集成 K3s？

#### 4.2.1 K3s 作为子进程启动

在 `cmd/kuscia/modules/k3s.go` 中：

```go
func (s *k3sModule) Run(ctx context.Context) error {
    // 构建 K3s 启动参数
    args := []string{
        "server",
        "-v=5",
        "-d=" + s.dataDir,                    // K3s 数据目录
        "-o=" + s.kubeconfigFile,             // 生成 kubeconfig
        "--disable-agent",                     // 禁用 agent（只需要 API Server）
        "--bind-address=" + s.bindAddress,
        "--https-listen-port=" + s.listenPort, // 默认 6443
        "--node-ip=" + s.hostIP,
        "--disable-cloud-controller",
        "--disable-network-policy",
        "--disable-scheduler",                 // 禁用默认调度器
        "--flannel-backend=none",              // 禁用网络插件
        "--disable=traefik",
        "--disable=coredns",
        "--disable=servicelb",
        "--disable=local-storage",
        "--disable=metrics-server",
    }
    
    // 非 root 用户启用 rootless 模式
    if !pkgcom.IsRootUser() {
        args = append(args, "--rootless")
    }
    
    // 如果指定了外部 datastore，使用外部存储
    if s.datastoreEndpoint != "" {
        args = append(args, "--datastore-endpoint="+s.datastoreEndpoint)
    }
    
    // 启动 K3s 进程
    sp := supervisor.NewSupervisor("k3s", nil, -1)
    err = sp.Run(ctx, func(ctx context.Context) supervisor.Cmd {
        cmd := exec.Command(filepath.Join(s.rootDir, "bin/k3s"), args...)
        cmd.Stderr = n
        cmd.Stdout = n
        return &ModuleCMD{cmd: cmd}
    })
    
    return err
}
```

**关键点**：

- ✅ K3s 是 Kuscia 进程的**子进程**
- ✅ 通过 `exec.Command` 启动，生命周期绑定
- ✅ 使用 `supervisor` 管理，自动重启
- ✅ 可以配置为 `rootless` 模式（非特权运行）

#### 4.2.2 数据存储方式

**方式 A：嵌入式 etcd（默认）**

```bash
# K3s 数据存储在本地目录
/var/lib/kuscia/data/k3s/server/db/
├── etcd/
│   ├── member/
│   │   ├── snap/          # Raft 快照
│   │   └── wal/           # Write-Ahead Log
│   └── etcd.db
```

**优点**：

- 无需外部依赖
- 部署简单
- 适合单机

**方式 B：外部 datastore**

```yaml
master:
  datastoreEndpoint: "mysql://user:pass@tcp(host:3306)/kube_db"
  # 或
  datastoreEndpoint: "postgres://user:pass@host:5432/kube_db"
  # 或
  datastoreEndpoint: "etcd://host:2379"
```

**优点**：

- 高可用
- 数据持久化
- 适合生产环境

**存储方式对比**：

| 特性 | 嵌入式 etcd | 外部 MySQL/PostgreSQL | 外部 etcd |
| ------ | ------------ | ---------------------- | ---------- |
| 部署复杂度 | 低 | 中 | 高 |
| 高可用 | 单节点 | 依赖数据库集群 | 原生 Raft 集群 |
| 备份恢复 | etcd snapshot | 数据库备份工具 | etcd snapshot |
| 适用场景 | 开发测试、单机 | 生产环境（中小规模） | 生产环境（大规模） |
| 运维工具 | etcdctl | 标准 SQL 工具 | etcdctl |

#### 4.2.3 客户端连接

Kuscia 内部组件通过 **kubeconfig** 连接到嵌入式 K3s：

```go
// pkg/utils/kubeconfig/client.go
func CreateClientSetsFromKubeconfig(kubeconfigFile string, apiserverEndpoint string) (*KubeClients, error) {
    var config *rest.Config
    var err error
    
    // 优先使用 kubeconfig 文件
    if kubeconfigFile != "" {
        config, err = clientcmd.BuildConfigFromFlags("", kubeconfigFile)
    } else if apiserverEndpoint != "" {
        // 或者直接使用 API Server 地址
        config = &rest.Config{
            Host: apiserverEndpoint,
            TLSClientConfig: rest.TLSClientConfig{
                Insecure: true, // 内部通信可以跳过证书验证
            },
        }
    } else {
        // 尝试使用 in-cluster config（如果在 K8s Pod 内运行）
        config, err = rest.InClusterConfig()
    }
    
    if err != nil {
        return nil, err
    }
    
    // 创建标准 K8s 客户端
    k8sClient, _ := kubernetes.NewForConfig(config)
    
    // 创建 CRD 客户端（用于访问 DomainData 等自定义资源）
    crdClient, _ := versioned.NewForConfig(config)
    
    return &KubeClients{
        KubeClient: k8sClient,
        CrdClient:  crdClient,
        RestConfig: config,
    }, nil
}
```

**连接流程**：

```
Kuscia Controllers
       ↓
   KubeClients (使用 kubeconfig)
       ↓
   REST API (HTTPS)
       ↓
   嵌入式 K3s API Server (localhost:6443)
       ↓
   etcd (本地存储)
```

#### 4.2.4 Rootless 模式

Kuscia 支持在非 root 用户下运行，此时会自动为 K3s 添加 `--rootless` 参数：

```go
if !pkgcom.IsRootUser() {
    args = append(args, "--rootless")
}
```

**Rootless 的限制**：

- ⚠️ 不能绑定特权端口（< 1024）。
- ⚠️ 部分需要 root 权限的系统调用不可用。
- ⚠️ 容器运行时的隔离能力可能受限（如无法加载某些内核模块）。

**Rootless 的优势**：

- ✅ 降低部署权限要求。
- ✅ 减少宿主机攻击面。
- ✅ 适合开发测试与权限受限环境。

### 4.3 K3s 就绪检测与初始化收敛

Kuscia 并不是“拉起 K3s 进程后立刻继续”，而是显式等待 K3s 就绪，再执行一组 Kuscia 自己的初始化动作：

```go
go func() {
 s.readyError = s.startCheckReady(ctx)
 if s.readyError == nil {
  s.readyError = s.initKusciaEnvAfterReady(ctx)
 }
 close(s.readyCh)
}()
```

**实际收敛过程可以分成两段：**

1. **K3s Ready 检测**
   - 每秒轮询一次 `https://127.0.0.1:6443/readyz`
   - 总超时时间为 30 秒
   - `readyz()` 不只检查 HTTP 返回值，还会检查 K3s 进程、证书文件是否已经生成

2. **Kuscia 环境初始化**
   - `applyCRD(conf)`：扫描 `crds/v1alpha1/` 并并发执行 `kubectl apply`
   - `applyKusciaResources(conf)`：写入 Kuscia 依赖的基础资源
   - `genKusciaKubeConfig(conf)`：生成 Kuscia 专用 kubeconfig 与 cluster role
   - `CreateClientSetsFromKubeconfig(...)`：初始化 K8s 客户端与 Kuscia CRD 客户端
   - `createDefaultDomain(...)`：确保当前 `Domain` 对象存在
   - `createCrossNamespace(...)`：确保跨域命名空间存在

这段逻辑的意义是把“**k3s 进程已经启动**”和“**Kuscia 控制面已经可用**”区分开来，避免控制器或业务模块过早启动。

---

## 5. Kubernetes 配置的适用性分析

Kuscia 通过嵌入式 K3s 复用了大量 Kubernetes 能力，但由于隐私计算场景的特殊性（如跨域通信、轻量部署），部分 K8s 原生能力（如 CNI、默认调度器）被替换或禁用。本节总结哪些 Kubernetes 配置在 Kuscia 中完全可用，哪些部分依赖容器或外部 K8s。

**总体适用性一览**：

| K8s 能力 | Kuscia 中是否可用 | 是否依赖容器 | 说明 |
| ---------- | ------------------ | -------------- | ------ |
| CRD | ✅ 完全可用 | ❌ 不依赖 | Kuscia 所有业务对象都是 CRD |
| Namespace | ✅ 完全可用 | ❌ 不依赖 | 逻辑隔离，按域划分 |
| RBAC | ✅ 完全可用 | ❌ 不依赖 | API Server 内置 |
| Informer/Controller | ✅ 完全可用 | ❌ 不依赖 | 基于 Watch API |
| etcd 存储 | ✅ 完全可用 | ❌ 不依赖 | 可嵌入或外接 |
| Service | ⚠️ 部分可用 | ⚠️ 依赖网络配置 | 作为 Envoy 配置输入 |
| Pod（RunK/RunC） | ⚠️ 任务执行时可用 | ✅ 依赖容器运行时 | 取决于运行时 |
| Ingress/LoadBalancer | ❌ 禁用 | - | 由 Kuscia Gateway 替代 |
| CNI/Flannel | ❌ 禁用 | - | 由 NetworkMesh 替代 |
| 默认 Scheduler | ❌ 禁用 | - | 由 Kuscia Scheduler 替代 |

### ✅ 完全起作用的配置

#### 1. **CRD（Custom Resource Definition）**

**是否依赖容器？** ❌ 不依赖

**工作原理**：

```yaml
# 注册 DomainData CRD
kubectl apply -f crds/kuscia.secretflow_domaindatas.yaml
# ↑ 这个命令无论在哪执行，都会注册到嵌入式 K3s 的 API Server
```

**代码层面**：

```go
// 控制器监听 CRD 变化
informer := factory.Kuscia().V1alpha1().DomainDatas()
informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc: func(obj interface{}) {
        // 当创建 DomainData 时触发
        dd := obj.(*v1alpha1.DomainData)
        handleNewDomainData(dd)
    },
})
```

**结论**：✅ CRD 完全工作，与是否容器化无关

---

#### 2. **Namespace 隔离**

**是否依赖容器？** ❌ 不依赖

**工作原理**：

Namespace 是 Kubernetes 的**逻辑隔离**机制，不是容器级别的隔离。

```yaml
# Alice 域的数据
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: user-table
  namespace: alice  # ← 逻辑隔离标识

# Bob 域的数据
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: order-table
  namespace: bob
```

**查询时的隔离**：

```bash
# 只能看到 alice 命名空间的数据
kubectl get domaindatas -n alice

# 只能看到 bob 命名空间的数据
kubectl get domaindatas -n bob
```

**代码实现**：

```go
// Lister 按 namespace 过滤
lister.DomainDatas("alice").List(labels.Everything())
// ↑ 只返回 namespace=alice 的对象
```

**结论**：✅ Namespace 是完全的逻辑隔离，不需要容器支持

---

#### 3. **RBAC（基于角色的访问控制）**

**是否依赖容器？** ❌ 不依赖

**示例**：

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: alice
  name: domaindata-reader
rules:
  - apiGroups: ["kuscia.secretflow"]
    resources: ["domaindatas"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  namespace: alice
  name: alice-binding
subjects:
  - kind: ServiceAccount
    name: alice-sa
roleRef:
  kind: Role
  name: domaindata-reader
```

**结论**：✅ RBAC 是 API Server 的功能，与容器无关

---

#### 4. **Informer/Controller 机制**

**是否依赖容器？** ❌ 不依赖

**工作原理**：

```
┌─────────────────────┐
│  API Server         │ ← 提供 Watch API
│  (K3s 内置)          │
└──────────┬──────────┘
           │ HTTPS Watch
           ↓
┌─────────────────────┐
│  Reflector          │ ← 监听变化
│  (Kuscia 内置)       │
└──────────┬──────────┘
           │ 事件流
           ↓
┌─────────────────────┐
│  DeltaFIFO Queue    │ ← 事件队列
└──────────┬──────────┘
           │
           ↓
┌─────────────────────┐
│  Indexer (本地缓存)  │ ← 内存中的对象存储
└──────────┬──────────┘
           │
           ↓
┌─────────────────────┐
│  Handler Callback   │ ← 业务逻辑
│  (Controller)        │
└─────────────────────┘
```

**代码示例**：

```go
// 这段代码在任何环境下都能工作
factory := informers.NewSharedInformerFactory(crdClient, time.Minute*10)
informer := factory.Kuscia().V1alpha1().DomainDatas()

informer.Informer().AddEventHandler(
    cache.ResourceEventHandlerFuncs{
        AddFunc:    onAdd,
        UpdateFunc: onUpdate,
        DeleteFunc: onDelete,
    },
)

stopCh := make(chan struct{})
factory.Start(stopCh)
factory.WaitForCacheSync(stopCh)
```

**结论**：✅ Informer 是基于 HTTP Watch API，与容器无关

---

#### 5. **etcd 存储**

**是否依赖容器？** ❌ 不依赖

**存储位置**：

```bash
# 嵌入式 etcd 数据存储在本地文件系统
/var/lib/kuscia/data/k3s/server/db/etcd/

# 数据结构（键值对）
/registry/domaindatas/alice/user-table → {DomainData Object}
/registry/domaindatas/bob/order-table → {DomainData Object}
/registry/domaindatagrants/alice/grant-001 → {DomainDataGrant Object}
```

**Raft 共识算法**：

即使只有一个节点，etcd 也使用 Raft 算法保证数据一致性：

```
Client Write Request
       ↓
┌──────────────┐
│   Leader     │ ← 接收写请求
│  (K3s Node)  │
└──────┬───────┘
       │
       ↓ (Append Entry)
┌──────────────┐
│ WAL Log      │ ← 写入预写日志
│ (磁盘持久化)  │
└──────┬───────┘
       │
       ↓ (Apply)
┌──────────────┐
│ State Machine│ ← 应用到状态机
│   (etcd.db)  │
└──────────────┘
```

**结论**：✅ etcd 是独立的分布式数据库，与容器无关

---

### ⚠️ 部分依赖容器的功能

#### 1. **Pod 调度和容器运行时**

**场景**：提交联邦学习任务（KusciaJob）

**依赖关系**：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: federated-learning
  namespace: alice
spec:
  tasks:
    - name: trainer
      image: secretflow/secretflow:latest  # ← 需要容器运行时
      command: ["python", "train.py"]
```

**如果使用 RunK 运行时**（Kubernetes-native）：

```go
// pkg/controllers/kusciajob/handler/runK.go
func (h *RunKHandler) Handle(task *v1alpha1.KusciaTask) error {
    // 创建 Kubernetes Pod
    pod := &corev1.Pod{
        Spec: corev1.PodSpec{
            Containers: []corev1.Container{
                {
                    Name:    task.Name,
                    Image:   task.Spec.Image,
                    Command: task.Spec.Command,
                },
            },
        },
    }
    
    // 调用 K8s API 创建 Pod
    _, err := h.kubeClient.CoreV1().Pods(task.Namespace).Create(
        ctx, pod, metav1.CreateOptions{},
    )
    
    return err
}
```

**这种情况下**：

- ⚠️ **需要有容器运行时**（Docker、containerd 等）
- ⚠️ **如果在 Autonomy Mode，需要在宿主机安装 containerd**
- ⚠️ **如果在外置 K8s Mode，K8s 节点需要有容器运行时**

**如果使用 RunP 运行时**（Process-based）：

```go
// pkg/controllers/kusciajob/handler/runP.go
func (h *RunPHandler) Handle(task *v1alpha1.KusciaTask) error {
    // 直接启动进程，不需要容器
    cmd := exec.Command(task.Spec.Command[0], task.Spec.Command[1:]...)
    cmd.Dir = task.Spec.WorkingDir
    cmd.Env = task.Spec.Env
    
    // 启动子进程
    err := cmd.Start()
    
    return err
}
```

**这种情况下**：

- ✅ **不需要容器运行时**
- ✅ **直接在宿主机启动进程**
- ✅ **更轻量，适合简单任务**

---

#### 2. **Service/Ingress（服务暴露）**

**场景**：将 Kuscia API 暴露给外部访问

**使用 K8s Service**：

```yaml
apiVersion: v1
kind: Service
metadata:
  name: kuscia-api
  namespace: kuscia-system
spec:
  type: NodePort
  ports:
    - port: 8080
      nodePort: 30080
  selector:
    app: kuscia
```

**依赖关系**：

- ⚠️ **Service 需要 CNI 网络插件**
- ⚠️ **在 Autonomy Mode，Kuscia 禁用了 flannel（`--flannel-backend=none`）**
- ⚠️ **需要手动配置网络或使用 hostNetwork**

**替代方案**：

在 Autonomy Mode，通常直接使用宿主机的端口：

```yaml
# autonomy_config.yaml
spec:
  apiServer:
    bindAddress: "0.0.0.0"
    port: 8080  # 直接绑定宿主机端口
```

---

## 6. 命名空间隔离机制

### 核心概念：Namespace 是逻辑隔离，不是容器隔离

很多初学者会混淆 **Kubernetes Namespace** 和 **Linux Namespace**：

| 特性 | K8s Namespace | Linux Namespace |
| ------ | --------------- | ----------------- |
| **层级** | API 级别 | 内核级别 |
| **作用** | 资源分组和权限隔离 | 进程隔离（PID、网络、挂载等） |
| **实现** | etcd 中的标签字段 | 内核系统调用 |
| **依赖容器** | ❌ 否 | ✅ 是 |
| **示例** | `namespace: alice` | `unshare --pid` |

### Kuscia 如何使用 Namespace 实现多租户

#### 场景：三个参与方的数据隔离

```
┌─────────────────────────────────────────────┐
│         单一 K3s 实例                        │
│                                              │
│  etcd 存储：                                  │
│  ┌────────────────────────────────────┐    │
│  │ /registry/domaindatas/             │    │
│  │   ├── alice/                       │    │
│  │   │   ├── user-table               │    │
│  │   │   └── behavior-data            │    │
│  │   ├── bob/                         │    │
│  │   │   ├── order-table              │    │
│  │   │   └── transaction-data         │    │
│  │   └── charlie/                     │    │
│  │       ├── model-v1                 │    │
│  │       └── report-q1                │    │
│  └────────────────────────────────────┘    │
└─────────────────────────────────────────────┘
```

**创建数据时的隔离**：

```bash
# Alice 注册自己的数据
curl -X POST http://localhost:8080/api/v1/domaindatas \
  -H "X-Namespace: alice" \
  -d '{
    "name": "user-table",
    "type": "table",
    "dataSource": "localfs-001"
  }'

# Bob 注册自己的数据
curl -X POST http://localhost:8080/api/v1/domaindatas \
  -H "X-Namespace: bob" \
  -d '{
    "name": "order-table",
    "type": "table",
    "dataSource": "localfs-002"
  }'
```

**查询时的隔离**：

```bash
# Alice 只能看到自己的数据
curl http://localhost:8080/api/v1/domaindatas?namespace=alice
# 返回: ["user-table"]

# Bob 只能看到自己的数据
curl http://localhost:8080/api/v1/domaindatas?namespace=bob
# 返回: ["order-table"]
```

**底层实现**：

```go
// pkg/controllers/domaindata/controller.go
func (c *Controller) syncDomainDataGrantHandler(ctx context.Context, key string) error {
    // key 格式: namespace/name
    namespace, name, _ := cache.SplitMetaNamespaceKey(key)
    
    // 从缓存中获取特定 namespace 的对象
    dg, err := c.domainDataGrantLister.DomainDataGrants(namespace).Get(name)
    
    // 执行业务逻辑...
    // 确保数据只在授权的 namespace 中可见
    err = c.ensureDomainDataInNamespace(dg, dg.Spec.GrantDomain)
    
    return nil
}
```

### 跨域数据授权如何实现

**场景**：Alice 授权 Bob 访问她的 `user-table` 数据

**步骤 1：Alice 创建授权**

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataGrant
metadata:
  name: grant-alice-to-bob
  namespace: alice  # ← 授权方 namespace
spec:
  author: alice
  domainDataID: user-table
  grantDomain: bob  # ← 被授权方
  signature: "RSA-SHA256 signature..."
```

**步骤 2：Controller 自动同步**

```go
func (c *Controller) syncDomainDataGrantHandler(ctx context.Context, key string) error {
    dg, _ := c.domainDataGrantLister.Get(key)
    
    // 在被授权方的 namespace 中创建镜像
    targetNS := dg.Spec.GrantDomain
    
    // 1. 创建 DomainDataGrant 镜像
    mirrorGrant := &v1alpha1.DomainDataGrant{
        ObjectMeta: metav1.ObjectMeta{
            Name:      dg.Name,
            Namespace: targetNS,  // ← Bob 的 namespace
            Labels: map[string]string{
                "kuscia.secretflow/grant-source": dg.Namespace,
            },
        },
        Spec: dg.Spec,
    }
    c.crdClient.KusciaV1alpha1().DomainDataGrants(targetNS).Create(ctx, mirrorGrant)
    
    // 2. 创建 DomainData 镜像（让 Bob 能看到 Alice 的数据）
    sourceData, _ := c.crdClient.KusciaV1alpha1().DomainDatas(dg.Namespace).Get(
        ctx, dg.Spec.DomainDataID, metav1.GetOptions{},
    )
    
    mirrorData := &v1alpha1.DomainData{
        ObjectMeta: metav1.ObjectMeta{
            Name:      sourceData.Name,
            Namespace: targetNS,  // ← Bob 的 namespace
            Labels: map[string]string{
                "kuscia.secretflow/data-source": dg.Namespace,
            },
        },
        Spec: sourceData.Spec,
    }
    c.crdClient.KusciaV1alpha1().DomainDatas(targetNS).Create(ctx, mirrorData)
    
    return nil
}
```

**结果**：

```bash
# Bob 可以看到 Alice 授权的数据
kubectl get domaindatas -n bob
# NAME              TYPE    AUTHOR
# user-table        table   alice  ← 来自 Alice 的授权

# 但 Bob 不能看到 Alice 的其他数据
kubectl get domaindatas -n alice
# Error: Forbidden
```

**关键点**：

- ✅ 这一切都是**逻辑隔离**
- ✅ 数据存储在同一个 etcd 中
- ✅ 通过 `namespace` 字段区分归属
- ✅ Controller 负责维护隔离规则
- ❌ **完全不依赖容器或 Linux Namespace**

---

## 7. 网络与服务发现补充说明

Kuscia 的嵌入式 K3s 被显式禁用了 Flannel、kube-proxy、CoreDNS 等原生网络组件，因此不能直接使用标准 Kubernetes 的 Pod 网络或服务发现机制。Kuscia 在这之上构建了自己的 **NetworkMesh**，用于满足隐私计算场景下的跨域、安全、服务发现需求。

### 7.1 为什么禁用 K8s 默认网络组件

K3s 默认启动参数中已经关闭相关组件：

```go
args := []string{
    "server",
    "--flannel-backend=none",   // 禁用 Flannel CNI
    "--disable=coredns",        // 禁用 CoreDNS
    "--disable=servicelb",      // 禁用 Service LoadBalancer
    // ...
}
```

原因：

- **Flannel**：Kuscia 任务 Pod 并不直接依赖 K8s CNI，而是通过 Envoy 与 DomainRoute 实现跨域/域内通信。
- **CoreDNS**：Kuscia 使用自定义 CoreDNS 或本地 hosts 机制完成域内服务名解析。
- **ServiceLB**：Kuscia Gateway 承担流量入口与负载均衡职责，无需 K8s 的 LoadBalancer。

### 7.2 Pod 在 Kuscia 中的角色

#### 7.2.1 什么是 Pod

在 Kubernetes / K3s 中，**Pod** 是最小的可部署单元，一个 Pod 可以包含一个或多个紧密耦合的容器，它们共享：

- **网络命名空间**：相同的 IP 地址和端口空间。
- **存储卷**：可以通过 Volume 共享文件。
- **生命周期**：Pod 创建、调度、运行、终止。

标准 K8s 中，Pod 由 kubelet 创建并通过 CNI 分配网络。而在 Kuscia 的嵌入式 K3s 中，kubelet 被禁用，**Pod 的生命周期由 Kuscia Agent 模块直接管理**。

#### 7.2.2 Kuscia 中 Pod 的特殊性

| 特性 | 标准 K8s | Kuscia 嵌入式 K3s |
| ------ | --------- | ------------------ |
| 创建者 | kubelet | Kuscia Agent |
| 网络分配 | CNI（Flannel/Calico 等） | 不依赖 CNI，由 Envoy + DomainRoute 接管 |
| 调度器 | kube-scheduler | Kuscia 自定义调度逻辑 |
| 运行位置 | 任意 Node | 与 Domain 同节点或指定节点 |
| 典型用途 | 通用微服务 | 隐私计算任务进程（如 SecretFlow 节点） |

> 在 Kuscia 中，一个 Pod 通常对应一个隐私计算任务实例，例如 SecretFlow 的某个参与方节点。

#### 7.2.3 从 KusciaJob 到 Pod 的映射关系

Kuscia 通过两层 CRD 将用户作业最终转化为 Pod：

```text
KusciaJob（跨域作业，位于 cross-domain 命名空间）
    │
    ▼
KusciaTask（每个参与方的任务，位于各自 Domain 命名空间）
    │
    ▼
Pod（实际运行容器/进程，位于各自 Domain 命名空间）
    │
    ▼
Container（SecretFlow / 其他引擎进程）
```

- 一个 `KusciaJob` 可以包含多个 `parties`，每个 party 对应一个 `KusciaTask`。
- 每个 `KusciaTask` 由 Kuscia 调度器解析后，生成一个或多个 Pod 规格。
- Agent 根据 `KusciaTask` 创建 Pod，并注入网络、存储、环境变量等配置。

#### 7.2.4 Pod 的命名空间归属

Pod 与创建它的 `KusciaTask` 位于**同一个 Domain 命名空间**：

```text
Namespace alice
└── Pod: psi-alice-bob-alice-driver

Namespace bob
└── Pod: psi-alice-bob-bob-driver
```

这种设计保证：

- **资源隔离**：alice 的 Pod 不会出现在 bob 的命名空间中。
- **权限隔离**：通过 RBAC，alice 只能操作自己命名空间下的 Pod。
- **日志与监控**：按命名空间聚合，便于多租户场景下的审计。

#### 7.2.5 典型的任务 Pod 示例

Kuscia Agent 根据 `KusciaTask` 生成的 Pod 大致如下：

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: psi-alice-bob-alice-driver
  namespace: alice
  labels:
    kuscia.secretflow/task-id: psi-task
    kuscia.secretflow/job-id: psi-alice-bob
    kuscia.secretflow/domain: alice
spec:
  containers:
    - name: sf-node
      image: secretflow/secretflow-lite-anolis8:1.11.0b1
      command:
        - python
        - -m
        - secretflow
      env:
        - name: PARTY
          value: "alice"
        - name: TASK_ID
          value: "psi-task"
      ports:
        - containerPort: 10001
          name: grpc
  restartPolicy: Never
```

> 实际生成的 Pod 还会挂载数据卷、注入证书、配置 Envoy sidecar 等，这里做了简化。

#### 7.2.6 Pod 网络与通信

Kuscia 中的任务 Pod 虽然没有 CNI 分配的 IP，但仍然需要与以下对象通信：

1. **同域内其他 Pod**：通过自定义 CoreDNS 解析 `<service>.<namespace>.svc`。
2. **跨域 Pod**：通过 Envoy + DomainRoute 建立加密通道。
3. **本节点 K3s API Server**：用于读取 ConfigMap / Secret 等配置。
4. **外部存储或数据源**：通过 DomainData 挂载或网络访问。

因此，Pod 在 Kuscia 中不是独立的网络个体，而是 NetworkMesh 中的一个端点。

### 7.3 NetworkMesh 组成

| 组件 | 作用 | 说明 |
| ------ | ------ | ------ |
| **Envoy** | 边缘/服务代理 | 处理节点内与跨节点流量，支持 mTLS |
| **DomainRoute / ClusterDomainRoute** | 路由与认证策略 | 定义源域到目的域的通信路径与授权方式 |
| **CoreDNS（自定义）** | 域内服务发现 | 解析任务 Pod 的 Service 域名 |
| **Transport** | 消息队列传输 | 可选的 gRPC/HTTP 消息传输通道 |

### 7.4 域内服务发现

在 Kuscia 中，每个任务 Pod 可以对应一个 K8s Service，用于同域内 Pod 之间的通信：

```yaml
apiVersion: v1
kind: Service
metadata:
  name: sf-node-alice
  namespace: alice
spec:
  selector:
    app: sf-node
  ports:
    - port: 10001
      targetPort: 10001
```

Pod 之间通过 `<service>.<namespace>.svc` 形式的域名访问，该域名由自定义 CoreDNS 解析。

### 7.5 跨域通信路径

```text
Pod A (alice)
    │
    ▼
Envoy (alice 节点)
    │
    ▼
DomainRoute / ClusterDomainRoute
    │
    ▼
Envoy (bob 节点 / Master)
    │
    ▼
Pod B / K3s ApiServer (bob)
```

跨域通信必须经过 DomainRoute 授权，支持 Token、mTLS、None 等认证方式，并可通过 Transit（THIRD-DOMAIN、REVERSE-TUNNEL）解决复杂网络场景。

### 7.6 对 Kubernetes Service 的理解差异

| 场景 | 标准 Kubernetes | Kuscia 嵌入式 K3s |
| ------ | ---------------- | ------------------ |
| Pod IP 网络 | 通过 CNI 分配 | 不依赖 CNI |
| Service 负载均衡 | kube-proxy + iptables/IPVS | Envoy 代理 |
| 跨节点通信 | 依赖 CNI 路由 | 依赖 DomainRoute + Envoy |
| DNS 解析 | CoreDNS 默认启用 | 自定义 CoreDNS |
| 外部暴露 | LoadBalancer / Ingress | Kuscia Gateway |

> 因此，在 Kuscia 中创建 `Service` 资源并不意味着标准 K8s 网络会生效，而是作为 Envoy 配置生成与服务发现的输入。

### 7.7 关键结论

- Kuscia 不依赖 K8s CNI，因此 Autonomy 模式下无需容器网络插件。
- 服务发现与流量转发由 Envoy + DomainRoute + 自定义 CoreDNS 共同完成。
- 跨域安全通信是 NetworkMesh 的核心能力，所有流量默认加密（mTLS/HTTPS）。

---

### 7.8 DataMesh 服务地址与端口配置

DataMesh 是 Kuscia 中负责数据资产管理、数据授权与数据访问的核心模块，对外同时暴露 HTTP 与 gRPC 两套服务。本节说明其默认监听地址、端口、配置文件位置以及可配置项。

#### 7.8.1 默认监听地址与端口

DataMesh 启动时会同时启动两个服务端点：

| 协议 | 默认端口 | 默认监听地址 | 用途说明 |
| ------ | --------- | ------------- | --------- |
| **HTTP** | `8070` | 空字符串（监听所有网卡） | 提供 DomainData、DomainDataSource、DomainDataGrant 等 RESTful API，以及 `/healthZ` 健康检查接口 |
| **gRPC / Arrow Flight** | `8071` | 空字符串（监听所有网卡） | 提供基于 Arrow Flight 的数据上传、下载与元数据查询接口 |

> 默认端口定义在 `pkg/datamesh/config/dmconfig.go` 的 `NewDefaultDataMeshConfig` 中：
>
> ```go
> func NewDefaultDataMeshConfig() *DataMeshConfig {
>     return &DataMeshConfig{
>         HTTPPort:       8070,
>         GRPCPort:       8071,
>         ConnectTimeOut: 5,
>         ReadTimeout:    20,
>         WriteTimeout:   20,
>         IdleTimeout:    300,
>         DisableTLS:     false,
>     }
> }
> ```

#### 7.8.2 配置文件位置

DataMesh 的可运行配置项位于 Kuscia 启动配置文件 **`kuscia.yaml`** 的顶层 **`dataMesh`** 字段下，对应 Go 结构体为 `cmd/kuscia/confloader/kuscia_config.go` 中 `KusciaConfig.DataMesh`：

```go
type KusciaConfig struct {
    // ...
    DataMesh *dmconfig.DataMeshConfig `yaml:"dataMesh,omitempty"`
    // ...
}
```

实际配置示例可参考仓库中的 `etc/conf/kuscia.yaml`：

```yaml
# DataMesh Config
dataMesh:
  dataProxyList:
    - endpoint: "dataproxy-grpc:8023" # data proxy endpoint
      dataSourceTypes:                # the type of datasource that data proxy supported
        - "odps"                      # odps also call as Aliyun MaxCompute
        - "hive"
        - "kingbase"
        - "dameng"                    # dameng database
```

#### 7.8.3 当前可配置项

`dataMesh` 节点下目前支持以下字段：

| 字段 | 类型 | 默认值 | 说明 |
| ------ | ------ | ------- | ------ |
| `disableTLS` | `bool` | `false` | 是否禁用 TLS。默认启用 TLS，HTTP/gRPC 均使用 Kuscia 自签名证书 |
| `dataProxyList` | `[]DataProxyConfig` | `[]` | 外部数据源代理列表，用于将特定类型的数据源访问转发到 DataProxy |

`dataProxyList` 中每一项 `DataProxyConfig` 的字段说明：

```yaml
dataProxyList:
  - endpoint: "dataproxy-grpc:8023"   # DataProxy 的 gRPC 端点，必填
    clientTLSConfig:                  # 访问 DataProxy 时使用的 TLS 配置（可选）
      certFile: ""
      keyFile: ""
      caFile: ""
      # ... 其他 TLS 字段
    dataSourceTypes:                  # 该代理负责的数据源类型列表
      - "odps"
      - "hive"
    mode: "proxy"                     # IO 模式：proxy 或 direct，默认 proxy
```

> **注意**：HTTP 端口 `8070` 与 gRPC 端口 `8071` 当前在 `DataMeshConfig` 中**没有 YAML tag**，因此无法通过 `kuscia.yaml` 直接修改。如果需要变更默认端口，需要修改 `pkg/datamesh/config/dmconfig.go` 中的默认值并重新编译 Kuscia。

#### 7.8.4 服务发现与访问方式

1. **节点本地访问**

   在 Kuscia 节点（或容器）内部，DataMesh HTTP 服务通常通过 `https://127.0.0.1:8070` 访问。部署脚本 `scripts/deploy/start_standalone.sh` 与 `scripts/deploy/kuscia.sh` 中的健康检查即采用该地址：

   ```bash
   do_http_probe "$domain_ctr" "https://127.0.0.1:8070/healthZ" 30 true
   ```

2. **Pod 内访问**

   Kuscia 自定义 CoreDNS 将 `datamesh` 解析为节点宿主 IP（`pkg/coredns/setup.go` 中的 `localService` 列表），因此任务 Pod 内部可以通过以下域名访问本节点的 DataMesh：

   ```text
   datamesh
   datamesh.<domainID>.svc
   ```

   例如 domain 为 `alice` 时：

   ```text
   datamesh.alice.svc:8070
   datamesh.alice.svc:8071
   ```

3. **Kubernetes Service 模板**

   仓库中提供了 DataMesh 的 Service 模板 `scripts/templates/datamesh_svc.yaml`：

   ```yaml
   apiVersion: v1
   kind: Service
   metadata:
     name: datamesh
     namespace: {{.DOMAIN_ID}}
   spec:
     externalName: {{.DATAMESH_ENDPOINT}}
     sessionAffinity: None
     type: ExternalName
   ```

   该模板用于在对应 Domain 的 namespace 下创建一个 `ExternalName` 类型的 Service，将 `datamesh.<domainID>.svc` 指向用户指定的 `DATAMESH_ENDPOINT`。若需要自定义解析地址，可基于该模板渲染后通过 `kubectl apply` 应用到 K3s 中。

4. **外部宿主机访问**

   DataMesh 默认不通过 Kuscia Gateway 暴露到宿主机，也不会在 `scripts/deploy/kuscia.sh` 等部署脚本中单独映射 `8070/8071` 端口。若需从容器外部访问，可：

   - 在启动容器时通过 `-p <host-port>:8070 -p <host-port>:8071` 手动映射端口；
   - 或者在容器内部通过 `docker exec` 调用本地地址。

#### 7.8.5 配置检查清单

- 确认 `kuscia.yaml` 中 `dataMesh.disableTLS` 是否需要显式设置（生产环境建议保持默认启用 TLS）。
- 若使用 ODPS、Hive 等外部数据源，确认 `dataMesh.dataProxyList` 中已配置对应 `dataSourceTypes` 与 `endpoint`。
- 若任务 Pod 中无法解析 `datamesh` 域名，检查自定义 CoreDNS 是否正常运行，以及 `pkg/coredns/setup.go` 中 `localService` 是否包含 `datamesh`。
- 若需要修改默认端口，需修改源码 `pkg/datamesh/config/dmconfig.go` 后重新编译。

---

## 8. 容器运行时支持

Kuscia 的 Agent 模块负责任务 Pod 的生命周期管理，目前支持三种运行时模式。选择哪种运行时取决于部署环境、安全要求和资源约束。

### 8.1 运行时概览

| 运行时 | 实现方式 | 隔离性 | 是否需要容器引擎 | 典型部署 |
| -------- | ---------- | -------- | ------------------ | ---------- |
| **RunC** | 原生 Linux 容器（namespace + cgroup） | 强 | 需要 containerd | Docker/VM 部署 |
| **RunP** | 直接启动进程，使用 PRoot 做轻量隔离 | 弱 | 不需要 | K8s 内嵌、开发测试 |
| **RunK** | 对接外部 Kubernetes 集群创建 Pod | 强 | 需要外部 K8s | 大规模生产集群 |

### 8.2 RunC：原生容器运行时

**原理**：使用 Linux Namespace 与 Cgroup 提供完整的容器隔离，由 Agent 直接调用 containerd 创建容器。

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaDeployment
metadata:
  name: sf-deployment
spec:
  runtime: RunC  # ← 原生容器
  template:
    spec:
      containers:
        - name: sf-node
          image: secretflow/secretflow:latest
          command: ["python", "run.py"]
```

**需要的环境**：

```bash
# 需要 containerd 与 runc
sudo apt-get install containerd runc

# 或者 Docker（内置 containerd）
sudo apt-get install docker.io
```

**适用场景**：

- ✅ 需要强隔离（文件系统、网络、PID）
- ✅ 生产环境、多租户共享节点
- ✅ 任务需要特定系统库或运行环境

**限制**：

- ⚠️ 通常需要特权或较高的宿主机权限
- ⚠️ 启动速度比 RunP 慢

### 8.3 RunP：进程运行时

**原理**：直接启动进程，不使用容器。可通过 PRoot 提供轻量级的文件系统隔离。

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaDeployment
metadata:
  name: sf-deployment
spec:
  runtime: RunP  # ← 直接启动进程
  template:
    spec:
      command: ["/usr/bin/python3", "/opt/app/run.py"]
      env:
        - name: PYTHONPATH
          value: /opt/app
```

**需要的环境**：

```bash
# 只需要在宿主机安装依赖
sudo apt-get install python3
pip3 install secretflow
```

**适用场景**：

- ✅ 性能敏感（无容器开销）
- ✅ 简单任务（不需要复杂隔离）
- ✅ 资源受限（无法运行容器）
- ✅ 开发调试（直接看进程输出）

**限制**：

- ⚠️ 隔离性弱，任务间可能相互影响
- ⚠️ 安全风险扩散较高

### 8.4 RunK：对接外部 Kubernetes

**原理**：将任务 Pod 的创建请求转发到外部 Kubernetes 集群，由外部集群负责调度与执行。

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaDeployment
metadata:
  name: sf-deployment
spec:
  runtime: RunK  # ← 使用外部 K8s Pod
  template:
    spec:
      containers:
        - name: sf-node
          image: secretflow/secretflow:latest
          command: ["python", "run.py"]
```

**需要的环境**：

```bash
# 需要可访问的外部 Kubernetes 集群
# 并在配置中指定 kubeconfig
```

**适用场景**：

- ✅ 大规模、高并发任务
- ✅ 需要利用现有 K8s 调度与运维体系
- ✅ 需要自动扩缩容、资源配额等高级能力

**限制**：

- ⚠️ 需要外部 K8s 集群
- ⚠️ 配置复杂度更高

### 8.5 运行时对比

| 特性 | RunC | RunP | RunK |
| ------ | ------ | ------ | ------ |
| **隔离性** | 强（namespace/cgroup） | 弱（进程级） | 强（外部 K8s 容器） |
| **启动速度** | 中（秒级） | 快（毫秒级） | 慢（依赖外部调度） |
| **资源开销** | 中 | 低 | 中 |
| **依赖安装** | containerd/Docker | 宿主依赖 | 外部 K8s 集群 |
| **版本管理** | 容易（镜像标签） | 困难 | 容易（镜像标签） |
| **安全性** | 高 | 低 | 高 |
| **部署权限** | 通常需要特权 | 无特殊要求 | 需要 K8s 创建 Pod 权限 |
| **适用场景** | 生产环境、强隔离 | 开发测试、资源受限 | 大规模集群 |

### 8.6 如何选择运行时

- **开发/POC**：优先使用 RunP，启动快、配置简单。
- **生产单机/VM**：使用 RunC，隔离性好、安全风险低。
- **生产大规模 K8s**：使用 RunK，复用现有 K8s 调度能力。

运行时可以在配置文件中设置默认值，也可以在任务级别覆盖：

```yaml
spec:
  defaultRuntime: RunC  # 全局默认
```

---

## 9. 实际应用场景对比

### 场景 1：单机开发环境

**需求**：

- 开发者笔记本
- 快速原型验证
- 不需要多节点

**推荐部署**：

```bash
# Autonomy Mode + RunP 运行时
./kuscia start \
  --config autonomy_dev.yaml \
  --rootless

# 配置文件中指定 RunP
cat autonomy_dev.yaml
spec:
  defaultRuntime: RunP
  domainID: dev-local
```

**Kubernetes 配置有效性**：

- ✅ CRD 完全工作
- ✅ Namespace 隔离有效
- ✅ Informer/Controller 正常运行
- ⚠️ Pod 相关功能不可用（因为没有容器运行时）
- ✅ DomainData、DomainDataGrant 等业务功能正常

---

### 场景 2：边缘计算节点

**需求**：

- 资源受限（2C4G）
- 离线运行
- 数据本地处理

**推荐部署**：

```bash
# Autonomy Mode + 嵌入式 etcd + RunP
./kuscia start \
  --config autonomy_edge.yaml \
  --datastore-endpoint=""  # 使用本地 etcd
```

**特点**：

- ✅ 无需网络连接（不依赖外部 K8s）
- ✅ 资源占用低（< 1GB 内存）
- ✅ 数据本地存储
- ⚠️ 无高可用（单节点）

---

### 场景 3：企业私有云

**需求**：

- 多节点集群
- 高可用
- 统一资源调度

**推荐部署**：

```bash
# Master Mode + 外部 K8s + RunK
# 在企业 K8s 集群上部署 Kuscia Controller

kubectl apply -f kuscia-master-deployment.yaml
kubectl apply -f kuscia-configmap.yaml
```

**特点**：

- ✅ 利用现有 K8s 基础设施
- ✅ 高可用（多副本）
- ✅ 统一监控和日志
- ⚠️ 配置复杂度高

---

### 场景 4：混合云联邦学习

**需求**：

- 多方参与
- 跨云部署
- 数据安全隔离

**推荐部署**：

```
参与方 A（阿里云）          参与方 B（腾讯云）
┌─────────────────┐      ┌─────────────────┐
│ Kuscia Master   │      │ Kuscia Master   │
│ + K3s Embedded  │      │ + K3s Embedded  │
│ Namespace: orgA │      │ Namespace: orgB │
└────────┬────────┘      └────────┬────────┘
         │                        │
         │   gRPC (TLS 加密)      │
         └────────────────────────┘
              跨域通信通道
```

**Kubernetes 配置作用**：

- ✅ 每方独立的 Namespace 隔离
- ✅ DomainDataGrant 跨域授权
- ✅ RBAC 控制访问权限
- ✅ 无需关心底层是否是容器

---

## 10. 常见问题 FAQ

### Q1: 如果我不懂 Kubernetes，能用 Kuscia 吗？

**答**：完全可以！

**原因**：

1. **Autonomy Mode 隐藏了 K8s 复杂性**
   - 你只需要运行一个二进制文件
   - K3s 自动启动，无需手动配置
   
2. **可以使用简化的配置**

   ```yaml
   # 最简配置
   domainID: my-domain
   rootDir: /var/lib/kuscia
   logLevel: info
   ```

3. **HTTP API 更友好**

   ```bash
   # 不需要懂 kubectl
   curl -X POST http://localhost:8080/api/v1/domaindatas \
     -d '{"name": "my-data", "type": "table"}'
   ```

4. **Kubernetes 概念是内部的**
   - 对外暴露的是 Kuscia 的业务概念
   - DomainData、DomainDataSource 等是领域模型
   - 不是纯粹的 K8s 资源

---

### Q2: 嵌入式 K3s 会影响性能吗？

**答**：影响很小，几乎可忽略。

**性能数据**：

| 指标 | 数值 |
| ------ | ------ |
| **内存占用** | ~300MB（空闲时） |
| **CPU 占用** | < 1%（无负载时） |
| **API 延迟** | < 5ms（本地查询） |
| **etcd 写入** | ~10ms（单次写入） |

**优化建议**：

```yaml
# 如果资源紧张，可以限制 K3s 资源
spec:
  k3s:
    memoryLimit: 512Mi
    cpuLimit: 500m
```

---

### Q3: 能否在没有 root 权限的机器上运行？

**答**：可以！使用 `--rootless` 模式。

**启动命令**：

```bash
# 普通用户即可运行
./kuscia start --config autonomy.yaml --rootless
```

**原理**：

```go
if !pkgcom.IsRootUser() {
    args = append(args, "--rootless")
}
```

**限制**：

- ⚠️ 不能使用 privileged 端口（< 1024）
- ⚠️ 某些系统调用受限
- ✅ 但核心功能完全正常

---

### Q4: 如果我想迁移到真正的 Kubernetes 集群，难吗？

**答**：非常容易！数据完全兼容。

**迁移步骤**：

1. **备份数据**

   ```bash
   # 导出 etcd 数据
   etcdctl snapshot save backup.db \
     --endpoints localhost:6443
   ```

2. **在新集群恢复**

   ```bash
   # 恢复到外部 K8s
   etcdctl snapshot restore backup.db \
     --data-dir /var/lib/k8s-etcd
   ```

3. **修改配置**

   ```yaml
   # 从 Autonomy Mode 切换到 Master Mode
   mode: Master
   apiserverEndpoint: https://k8s-api.example.com:6443
   kubeconfigFile: /etc/kuscia/kubeconfig
   ```

4. **重启 Kuscia**

   ```bash
   ./kuscia start --config master.yaml
   ```

**因为数据都在 etcd 中，格式完全一致！**

---

### Q5: Namespace 隔离安全吗？会不会被绕过？

**答**：在 API 层面是安全的，但不是物理隔离。

**安全保障**：

1. **API Server 强制检查**

   ```go
   // Kubernetes API Server 源码
   func (r *REST) Get(ctx context.Context, name string) (runtime.Object, error) {
       namespace, _ := apirequest.NamespaceFrom(ctx)
       // ← 从上下文提取 namespace，无法伪造
       
       obj, err := r.store.Get(ctx, namespace+"/"+name)
       return obj, err
   }
   ```

2. **RBAC 权限控制**

   ```yaml
   # Alice 只能访问 alice namespace
   apiVersion: rbac.authorization.k8s.io/v1
   kind: RoleBinding
   metadata:
     namespace: alice
   subjects:
     - kind: User
       name: alice
   ```

3. **审计日志**

   ```bash
   # 记录所有 API 调用
   tail -f /var/lib/kuscia/logs/k3s-audit.log
   ```

**但不是物理隔离**：

- ⚠️ 数据在同一个 etcd 中
- ⚠️ 如果有 etcd 访问权限，可以看到所有数据
- ⚠️ Root 用户可以读取 etcd 文件

**增强安全**：

```yaml
# 启用 etcd 加密
spec:
  encryption:
    enabled: true
    type: aescbc
```

---

### Q6: 为什么 Kuscia 要用 Kubernetes？直接用 Go 代码不行吗？

**答**：Kubernetes 提供了成熟的抽象和生态。

**优势对比**：

| 维度 | 纯 Go 实现 | 基于 K8s |
| ------ | ----------- | ---------- |
| **开发工作量** | 巨大（需重写 API、存储、缓存） | 小（复用 K8s 生态） |
| **可靠性** | 需自行测试和验证 | K8s 经过全球验证 |
| **扩展性** | 需自行设计插件系统 | CRD + Webhook 成熟 |
| **社区支持** | 无 | 大量教程和工具 |
| **人才储备** | 难招 | K8s 工程师很多 |
| **长期维护** | 负担重 | 跟随上游更新 |

**实际例子**：

要实现"监听数据变化并同步"：

**纯 Go 实现**（假设）：

```go
// 需要自己实现...
- HTTP Server 接收请求
- 数据库读写
- 缓存机制
- Watch 长轮询
- 事件通知
- 重试机制
- 幂等控制
// ... 至少几千行代码
```

**基于 K8s**：

```go
// 使用 Informer，几十行搞定
informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc:    onAdd,
    UpdateFunc: onUpdate,
})
factory.Start(stopCh)
```

---

### Q7: Kuscia 的数据存储在哪里？可以替换吗？

**答**：默认存储在嵌入式 etcd 中，也可以配置为外部 MySQL、PostgreSQL 或 etcd。

**默认方式（嵌入式 etcd）**：

```bash
/var/lib/kuscia/data/k3s/server/db/etcd/
```

**外部 datastore 配置示例**：

```yaml
master:
  datastoreEndpoint: "mysql://user:pass@tcp(host:3306)/kuscia_db"
  # 或
  # datastoreEndpoint: "postgres://user:pass@host:5432/kuscia_db"
  # 或
  # datastoreEndpoint: "etcd://host:2379"
```

**选型建议**：

- **开发测试**：使用嵌入式 etcd，零配置。
- **生产环境**：使用外部 MySQL/PostgreSQL/etcd，便于备份、高可用和运维。

---

### Q8: Kuscia 的 K3s 会占用哪些端口？

**答**：K3s API Server 默认监听 `6443`（可通过配置修改）。此外 Kuscia 自身还会暴露：

| 端口 | 用途 |
| ------ | ------ |
| 6443 | K3s API Server（内部） |
| 8082 | KusciaAPI HTTP |
| 8083 | KusciaAPI gRPC |
| 8090 | 控制器健康检查 |
| 8070 | DataMesh HTTP |
| 8071 | DataMesh gRPC/Arrow Flight |

如果端口冲突，可以在配置中调整 `apiserverEndpoint` 与 KusciaAPI 绑定地址。

---

## 11. 嵌入式 K3s 中运行的功能

### 哪些功能运行在嵌入的 K8s 中？

Kuscia 的嵌入式 K3s 不是完整的 Kubernetes，而是**精简定制版**，只保留了必要的组件。

#### 1. **启用的 K8s 核心组件**

| 组件 | 状态 | 说明 |
| ------ | ------ | ------ |
| **API Server** | ✅ 启用 | 提供 RESTful API，所有操作的入口 |
| **etcd** | ✅ 启用 | 存储所有 CRD 对象和集群状态 |
| **Controller Manager** | ⚠️ 部分启用 | 运行内置控制器（如 Namespace、ServiceAccount） |
| **Scheduler** | ❌ 禁用 | Kuscia 有自己的任务调度器 |
| **Kubelet** | ❌ 禁用 | Autonomy Mode 不需要节点代理 |
| **kube-proxy** | ❌ 禁用 | 不使用 K8s Service 网络 |
| **CoreDNS** | ❌ 禁用 | 使用自定义 DNS 方案 |
| **Traefik Ingress** | ❌ 禁用 | 使用 Kuscia Gateway |
| **Metrics Server** | ❌ 禁用 | 使用自定义监控方案 |
| **Flannel CNI** | ❌ 禁用 | 不使用 Pod 网络 |
| **ServiceLB** | ❌ 禁用 | 不使用 LoadBalancer |

**启动参数证明**（来自 `cmd/kuscia/modules/k3s.go`）：

```go
args := []string{
    "server",
    "--disable-agent",              // ← 禁用 Kubelet
    "--disable-scheduler",          // ← 禁用调度器
    "--flannel-backend=none",       // ← 禁用网络插件
    "--disable=traefik",            // ← 禁用 Ingress
    "--disable=coredns",            // ← 禁用 DNS
    "--disable=servicelb",          // ← 禁用负载均衡
    "--disable=local-storage",      // ← 禁用本地存储
    "--disable=metrics-server",     // ← 禁用监控
}
```

---

#### 2. **运行在 K3s 中的 Kuscia 功能**

以下功能**完全依赖**嵌入式 K3s 提供的 API 和存储：

##### A. **CRD 资源管理**（核心功能）

```go
// pkg/controllers/domaindata/controller.go
// 这些控制器监听 K3s API Server 的资源变化

type Controller struct {
    domainDataLister        v1alpha1.DomainDataLister
    domainDataGrantLister   v1alpha1.DomainDataGrantLister
    crdClient               versioned.Interface
}

func (c *Controller) syncDomainDataGrantHandler(ctx context.Context, key string) error {
    // 从 K3s etcd 中读取 DomainDataGrant
    dg, err := c.domainDataGrantLister.Get(key)
    
    // 执行业务逻辑（跨域同步、签名验证等）
    err = c.ensureDomainData(dg)
    
    // 更新状态回写到 K3s etcd
    updateStatus(dg, phase, message)
    
    return nil
}
```

**涉及的 CRD 列表**（来自 `cmd/kuscia/modules/controllers.go`）：

```go
[]controllers.ControllerConstruction{
    {
        NewController: taskresourcegroup.NewController,
        CRDNames:      []string{"taskresourcegroups", "taskresources"},
    },
    {
        NewController: domain.NewController,
        CRDNames:      []string{"domains"},
    },
    {
        NewController: kusciatask.NewController,
        CRDNames:      []string{"kusciatasks", "appimages"},
    },
    {
        NewController: domainroute.NewController,
        CRDNames:      []string{"domains", "domainroutes", "gateways"},
    },
    {
        NewController: clusterdomainroute.NewController,
        CRDNames:      []string{"domains", "clusterdomainroutes", "domainroutes", "gateways"},
    },
    {
        NewController: kusciajob.NewController,
        CRDNames:      []string{"kusciajobs"},
    },
    {
        NewController: kusciadeployment.NewController,
        CRDNames:      []string{"kusciadeployments"},
    },
    {
        NewController: domaindata.NewController,
        CRDNames:      []string{"domains", "domaindatagrants"},
    },
    {
        NewController: portflake.NewController,  // 端口分配
    },
    {
        NewController: garbagecollection.NewKusciaJobGCController,  // GC
    },
    {
        NewController: garbagecollection.NewKusciaDomainDataGCController,  // GC
    },
}
```

**总结**：所有业务领域的 CRD 都运行在 K3s 中！

---

##### B. **Namespace 隔离**

```bash
# 每个参与方有独立的 Namespace
kubectl get namespaces

NAME              STATUS   AGE
alice             Active   10d
bob               Active   10d
charlie           Active   10d
kuscia-system     Active   10d
```

**作用**：

- ✅ 数据隔离（不同域的数据在不同 namespace）
- ✅ 权限隔离（RBAC 按 namespace 授权）
- ✅ 资源隔离（可以限制每个 namespace 的资源配额）

---

##### C. **ConfigMap/Secret 配置管理**

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

---

##### D. **ServiceAccount & RBAC**

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

---

#### 3. **不运行在 K3s 中的功能**

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

---

#### 4. **功能分层架构图**

```
┌─────────────────────────────────────────────────────┐
│                  Kuscia 用户层                        │
│  HTTP/gRPC API | CLI | Web Console                   │
└──────────────────┬──────────────────────────────────┘
                   │
┌──────────────────▼──────────────────────────────────┐
│              Kuscia 业务逻辑层                        │
│  ┌─────────────────────────────────────────────┐   │
│  │  Controllers (运行在 K3s 之上)               │   │
│  │  - DomainData Controller                    │   │
│  │  - DomainDataGrant Controller               │   │
│  │  - KusciaJob Controller                     │   │
│  │  - KusciaTask Controller                    │   │
│  │  - DomainRoute Controller                   │   │
│  │  - Garbage Collection Controller            │   │
│  └─────────────────────────────────────────────┘   │
└──────────────────┬──────────────────────────────────┘
                   │ 调用 K8s API
┌──────────────────▼──────────────────────────────────┐
│              嵌入式 K3s 层                           │
│  ┌─────────────────────────────────────────────┐   │
│  │  API Server ✅                               │   │
│  │  - CRUD 操作                                 │   │
│  │  - Watch 事件                                │   │
│  │  - 认证授权                                  │   │
│  └──────────────────┬──────────────────────────┘   │
│                     │                               │
│  ┌──────────────────▼──────────────────────────┐   │
│  │  etcd ✅                                     │   │
│  │  - DomainData 对象                           │   │
│  │  - DomainDataGrant 对象                      │   │
│  │  - KusciaJob 对象                            │   │
│  │  - Namespace/SA/RBAC 对象                    │   │
│  └─────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
                   │
┌──────────────────▼──────────────────────────────────┐
│              Kuscia 基础设施层（不依赖 K8s）          │
│  ┌─────────────────────────────────────────────┐   │
│  │  Task Runtime                                │   │
│  │  - RunK (containerd)                        │   │
│  │  - RunP (process)                           │   │
│  └─────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────┐   │
│  │  Transport (gRPC/HTTP)                       │   │
│  └─────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────┐   │
│  │  Gateway (Envoy)                             │   │
│  └─────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

---

## 12. Kuscia 如何管理 K8s

### Kuscia 与 K3s 的管理关系

Kuscia 不是被动地使用 K8s，而是**主动管理**整个嵌入式 K3s 的生命周期。

---

#### 1. **K3s 生命周期管理**

##### A. **启动管理**

```go
// cmd/kuscia/modules/k3s.go
func (s *k3sModule) Run(ctx context.Context) error {
    // 1. 检查数据存储端点
    err := datastore.CheckDatastoreEndpoint(s.datastoreEndpoint)
    if err != nil {
        return err
    }
    
    // 2. 构建启动参数（根据配置动态调整）
    args := s.buildK3sArgs()
    
    // 3. 创建 Supervisor 管理进程
    sp := supervisor.NewSupervisor("k3s", nil, -1)
    
    // 4. 启动 K3s 子进程
    err = sp.Run(ctx, func(ctx context.Context) supervisor.Cmd {
        cmd := exec.Command(filepath.Join(s.rootDir, "bin/k3s"), args...)
        cmd.Stderr = n
        cmd.Stdout = n
        
        // 设置环境变量
        envs := os.Environ()
        envs = append(envs, "CATTLE_NEW_SIGNED_CERT_EXPIRATION_DAYS=3650")
        cmd.Env = envs
        
        return &ModuleCMD{cmd: cmd, score: &k3sOOMScore}
    })
    
    return err
}
```

**关键点**：

- ✅ Kuscia 决定 K3s 何时启动
- ✅ Kuscia 决定 K3s 的参数配置
- ✅ Kuscia 通过 Supervisor 监控 K3s 进程
- ✅ 如果 K3s 崩溃，Supervisor 会自动重启它

---

##### B. **就绪检查**

```go
// cmd/kuscia/modules/k3s.go
func (s *k3sModule) startCheckReady(ctx context.Context) error {
    // 等待 kubeconfig 文件生成
    for i := 0; i < 60; i++ {
        if _, err := os.Stat(s.kubeconfigFile); err == nil {
            break
        }
        time.Sleep(1 * time.Second)
    }
    
    // 创建客户端并测试连接
    clients, err := kubeconfig.CreateClientSetsFromKubeconfig(
        s.conf.KubeconfigFile, 
        s.conf.ApiserverEndpoint,
    )
    if err != nil {
        return err
    }
    
    // 尝试获取版本信息
    _, err = clients.KubeClient.Discovery().ServerVersion()
    if err != nil {
        return err
    }
    
    // 初始化 Kuscia 环境
    s.initKusciaEnvAfterReady(ctx)
    
    return nil
}
```

**检查流程**：

```
Kuscia 主进程启动
       ↓
启动 K3s 子进程
       ↓
等待 kubeconfig 文件生成
       ↓
创建 K8s 客户端
       ↓
测试 API Server 连通性
       ↓
初始化 Kuscia 环境（创建 Namespace、CRD 等）
       ↓
标记 K3s 为 Ready 状态
```

---

##### C. **停止管理**

```go
// 当 Kuscia 收到退出信号时
func (s *k3sModule) Stop(ctx context.Context) error {
    // 1. 通知 Supervisor 停止 K3s
    sp.Stop()
    
    // 2. 等待 K3s 进程优雅退出
    select {
    case <-sp.Done():
        nlog.Info("K3s stopped gracefully")
    case <-time.After(30 * time.Second):
        // 3. 超时后强制杀死进程
        sp.Kill()
        nlog.Warn("K3s force killed")
    }
    
    return nil
}
```

---

##### D. **Supervisor 崩溃恢复机制**

Kuscia 使用自定义的 `supervisor` 组件管理 K3s 子进程，确保 K3s 异常退出时能够自动拉起：

```go
sp := supervisor.NewSupervisor("k3s", nil, -1)
err = sp.Run(ctx, func(ctx context.Context) supervisor.Cmd {
    cmd := exec.Command(filepath.Join(s.rootDir, "bin/k3s"), args...)
    return &ModuleCMD{cmd: cmd, score: &k3sOOMScore}
})
```

**Supervisor 职责**：

- **进程守护**：监控 K3s 进程状态，崩溃时自动重启。
- **退避策略**：支持指数退避，避免频繁重启耗尽系统资源。
- **优雅停止**：收到停止信号后，先尝试优雅退出，超时后强制终止。
- **OOM 保护**：通过调整 OOM Score，降低 K3s 被系统 OOM Killer 优先杀死的概率。

**效果**：

- K3s 崩溃后通常能在数秒内自动恢复。
- Kuscia 其他模块无需关心 K3s 是否重启，只需通过 Informer 重新同步缓存。

---

#### 2. **CRD 注册管理**

Kuscia 在启动时自动注册所有需要的 CRD：

```go
// cmd/kuscia/modules/k3s.go
func (s *k3sModule) initKusciaEnvAfterReady(ctx context.Context) error {
    clients, _ := kubeconfig.CreateClientSetsFromKubeconfig(...)
    
    // 应用所有 CRD YAML 文件
    crdFiles := []string{
        "crds/v1alpha1/kuscia.secretflow_domaindatas.yaml",
        "crds/v1alpha1/kuscia.secretflow_domaindatagrants.yaml",
        "crds/v1alpha1/kuscia.secretflow_domains.yaml",
        "crds/v1alpha1/kuscia.secretflow_kusciajobs.yaml",
        "crds/v1alpha1/kuscia.secretflow_kusciatasks.yaml",
        // ... 更多 CRD
    }
    
    for _, crdFile := range crdFiles {
        // 执行 kubectl apply -f <crd_file>
        cmd := exec.Command(
            filepath.Join(s.rootDir, "bin/kubectl"),
            "--kubeconfig", s.kubeconfigFile,
            "apply", "-f", crdFile,
        )
        cmd.Run()
    }
    
    return nil
}
```

**自动化程度**：

- ✅ 无需手动注册 CRD
- ✅ 启动时自动检测并注册
- ✅ 支持 CRD 版本升级

---

#### 3. **Namespace 和用户管理**

Kuscia 自动创建和管理 Namespace：

```go
func (s *k3sModule) initKusciaEnvAfterReady(ctx context.Context) error {
    clients, _ := kubeconfig.CreateClientSetsFromKubeconfig(...)
    
    // 1. 创建 Domain 对应的 Namespace
    domainNS := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{
            Name: s.conf.DomainID,  // 例如 "alice"
        },
    }
    clients.KubeClient.CoreV1().Namespaces().Create(ctx, domainNS)
    
    // 2. 创建 ServiceAccount
    sa := &corev1.ServiceAccount{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "default",
            Namespace: s.conf.DomainID,
        },
    }
    clients.KubeClient.CoreV1().ServiceAccounts(s.conf.DomainID).Create(ctx, sa)
    
    // 3. 创建默认角色和绑定
    // ... RBAC 配置
    
    return nil
}
```

---

#### 4. **控制器管理器**

Kuscia 启动一个统一的控制器管理器：

```go
// cmd/kuscia/modules/controllers.go
func NewControllersModule(i *ModuleRuntimeConfigs) (Module, error) {
    opt := &controllers.Options{
        ControllerName: "kuscia-controller-manager",
        HealthCheckPort: 8090,
        Workers:         4,  // 4 个工作协程
        RunMode:         i.RunMode,
        Namespace:       i.DomainID,
    }
    
    // 创建控制器服务器
    return controllers.NewServer(
        opt, 
        i.Clients,
        []controllers.ControllerConstruction{
            // 注册所有控制器
            {NewController: domaindata.NewController, ...},
            {NewController: kusciajob.NewController, ...},
            {NewController: kusciatask.NewController, ...},
            // ... 更多控制器
        },
    ), nil
}
```

**控制器管理器职责**：

- ✅ 启动所有业务控制器
- ✅ 提供健康检查接口（`:8090/healthz`）
- ✅ 管理工作协程池
- ✅ 统一错误处理和日志

---

#### 5. **配置管理**

Kuscia 通过配置文件控制 K3s 行为：

```yaml
# autonomy_config.yaml
spec:
  master:
    # K3s 数据存储
    datastoreEndpoint: ""  # 空表示使用嵌入式 etcd
    
    # 外部 MySQL（生产环境推荐）
    # datastoreEndpoint: "mysql://user:pass@host:3306/kuscia_db"
    
    # Kubeconfig 路径
    kubeconfigFile: /var/lib/kuscia/etc/kubeconfig
    
    # API Server 地址
    apiserverEndpoint: https://localhost:6443
    
  # 垃圾回收配置
  garbageCollection:
    kusciaDomainDataGC:
      enable: true
      durationHours: 720  # 30 天后清理
    kusciaJobGC:
      durationHours: 168  # 7 天后清理
```

---

#### 6. **监控和诊断**

Kuscia 监控 K3s 的健康状态：

```go
// 健康检查端点
GET http://localhost:8090/healthz

// 返回示例
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

```bash
/var/lib/kuscia/logs/
├── kuscia.log          # Kuscia 主进程日志
├── k3s.log             # K3s 进程日志
├── k3s-audit.log       # K8s API 审计日志
└── controller.log      # 控制器日志
```

---

#### 7. **管理关系总结图**

```
┌─────────────────────────────────────────────┐
│         Kuscia 主进程（管理者）               │
│                                              │
│  ┌──────────────────────────────────────┐   │
│  │  生命周期管理                         │   │
│  │  - 启动 K3s                          │   │
│  │  - 停止 K3s                          │   │
│  │  - 重启（崩溃恢复）                   │   │
│  └──────────────────────────────────────┘   │
│  ┌──────────────────────────────────────┐   │
│  │  配置管理                             │   │
│  │  - 生成启动参数                       │   │
│  │  - 选择存储后端                       │   │
│  │  - 设置网络端口                       │   │
│  └──────────────────────────────────────┘   │
│  ┌──────────────────────────────────────┐   │
│  │  资源管理                             │   │
│  │  - 注册 CRD                          │   │
│  │  - 创建 Namespace                    │   │
│  │  - 配置 RBAC                         │   │
│  └──────────────────────────────────────┘   │
│  ┌──────────────────────────────────────┐   │
│  │  控制器管理                           │   │
│  │  - 启动业务控制器                    │   │
│  │  - 健康检查                          │   │
│  │  - 错误恢复                          │   │
│  └──────────────────────────────────────┘   │
│  ┌──────────────────────────────────────┐   │
│  │  监控诊断                             │   │
│  │  - 日志收集                          │   │
│  │  - 指标暴露                          │   │
│  │  - 审计记录                          │   │
│  └──────────────────────────────────────┘   │
└──────────────────┬──────────────────────────┘
                   │ 管理
                   ↓
┌─────────────────────────────────────────────┐
│         K3s 子进程（被管理者）                │
│                                              │
│  - API Server (提供 API)                     │
│  - etcd (存储数据)                           │
│  - Controller Manager (内置控制器)           │
└─────────────────────────────────────────────┘
```

**关键理念**：

- ✅ Kuscia 是 **Owner**，K3s 是 **Managed Component**
- ✅ K3s 对上层透明，用户感知不到 K3s 的存在
- ✅ Kuscia 负责所有运维操作（升级、备份、恢复）

---

## 13. Kuscia 镜像体系

### 有镜像吗？

**答：有！Kuscia 提供完整的容器镜像体系。**

---

#### 1. **Kuscia 官方镜像**

##### A. **主镜像：`secretflow/kuscia`**

**镜像地址**：

```bash
# 阿里云镜像仓库（国内）
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:latest
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0

# Docker Hub（国际）
docker.io/secretflow/kuscia:latest
docker.io/secretflow/kuscia:1.2.0b0
```

**构建方式**（来自 `scripts/make/image.mk`）：

```makefile
TAG = ${KUSCIA_VERSION_TAG}-${DATETIME}
IMG := secretflow/kuscia:${TAG}

.PHONY: image
image: build
 DOCKER_BUILDKIT=1
 @$(call start_docker_buildx)
 docker buildx build -t ${IMG} \
   --build-arg KUSCIA_ENVOY_IMAGE=${ENVOY_IMAGE} \
   --build-arg DEPS_IMAGE=${DEPS_IMAGE} \
   -f ./build/dockerfile/kuscia-anolis.Dockerfile \
   . --platform linux/${ARCH} --load
```

**支持的架构**：

- ✅ `linux/amd64` (x86_64)
- ✅ `linux/arm64` (ARM64, Apple Silicon)

**多架构构建**：

```bash
# 创建多架构构建器
docker buildx create --name kuscia --platform linux/arm64,linux/amd64

# 构建并推送多架构镜像
docker buildx build -t secretflow/kuscia:latest \
  --platform linux/amd64,linux/arm64 \
  -f ./build/dockerfile/kuscia-anolis.Dockerfile \
  . --push
```

---

##### B. **依赖镜像**

**kuscia-deps**（基础依赖）：

```bash
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-deps:0.7.0b0
```

包含：

- Python 运行时
- 系统库（openssl、curl 等）
- 常用工具（kubectl、helm 等）

**kuscia-envoy**（网关）：

```bash
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-envoy:0.6.2b0
```

包含：

- Envoy Proxy
- 自定义过滤器

**proot**（进程隔离）：

```bash
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/proot
```

包含：

- PRoot 工具（无 root 权限的 chroot）
- 用于 RunP 运行时

---

##### C. **监控镜像**

**kuscia-monitor**：

```bash
docker.io/secretflow/kuscia-monitor:latest
```

包含：

- Prometheus Exporter
- 自定义监控指标

构建命令：

```makefile
.PHONY: build-monitor
build-monitor:
 docker build -t secretflow/kuscia-monitor \
   -f ./build/dockerfile/kuscia-monitor.Dockerfile .
```

---

#### 2. **引擎镜像**

Kuscia 支持多种计算引擎，每种引擎有自己的镜像：

##### A. **SecretFlow 引擎**

```bash
# Lite 版本（轻量级）
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite-anolis8:1.11.0b1

# Full 版本（完整版）
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-anolis8:1.11.0b1
```

**用途**：联邦学习、隐私计算

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

---

##### B. **SCQL 引擎**

```bash
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/scql:latest
```

**用途**：安全查询语言（Secure Query Language）

---

##### C. **Serving 引擎**

```bash
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/serving:latest
```

**用途**：模型推理服务

---

##### D. **自定义引擎**

用户可以注册自己的引擎镜像：

```bash
# 注册自定义 AppImage
./scripts/deploy/register_app_image.sh \
  --image my-registry.com/my-engine:v1.0 \
  --name my-engine \
  --version 1.0
```

---

#### 3. **镜像使用场景**

##### 场景 A：Docker 部署

```bash
## 1. 拉取 Kuscia 镜像
docker pull secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0

## 2. 拉取引擎镜像
docker pull secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite-anolis8:1.11.0b1

## 3. 启动 Kuscia
docker run -d \
  --name kuscia \
  -v /var/lib/kuscia:/var/lib/kuscia \
  -p 8080:8080 \
  secretflow/kuscia:1.2.0b0 \
  start --config /etc/kuscia/autonomy.yaml
```

---

##### 场景 B：K8s 部署

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kuscia
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kuscia
  template:
    metadata:
      labels:
        app: kuscia
    spec:
      containers:
        - name: kuscia
          image: secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0
          command: ["./kuscia", "start", "--config", "/etc/kuscia/config.yaml"]
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: data
              mountPath: /var/lib/kuscia
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: kuscia-data-pvc
```

---

##### 场景 C：离线部署（无网络）

```bash
## 1. 在有网络的机器上保存镜像
docker save secretflow/kuscia:1.2.0b0 -o kuscia.tar
docker save secretflow-lite-anolis8:1.11.0b1 -o secretflow.tar

## 2. 传输到离线机器
scp kuscia.tar offline-host:/tmp/
scp secretflow.tar offline-host:/tmp/

## 3. 在离线机器上加载镜像
docker load -i /tmp/kuscia.tar
docker load -i /tmp/secretflow.tar

## 4. 导入到 Kuscia
./kuscia.sh image import kuscia.tar
./kuscia.sh image import secretflow.tar
```

---

#### 4. **镜像策略配置**

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

---

#### 5. **镜像管理命令**

Kuscia 提供专门的镜像管理命令：

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

---

#### 6. **镜像架构**

**Kuscia 镜像内容**：

```dockerfile
# build/dockerfile/kuscia-anolis.Dockerfile (简化版)
FROM anolis:8 AS builder

# 编译 Go 代码
COPY . /src
RUN cd /src && make build

FROM anolis:8

# 安装依赖
RUN yum install -y python3 openssl curl

# 复制编译产物
COPY --from=builder /src/build/kuscia /usr/bin/kuscia
COPY --from=builder /src/bin/k3s /usr/bin/k3s
COPY --from=builder /src/bin/kubectl /usr/bin/kubectl

# 复制 CRD 定义
COPY crds/ /etc/kuscia/crds/

# 复制配置文件
COPY etc/conf/ /etc/kuscia/conf/

# 设置入口点
ENTRYPOINT ["/usr/bin/kuscia"]
CMD ["start"]
```

**镜像大小优化**：

- 使用多阶段构建
- 清理不必要的文件
- 压缩二进制文件

最终镜像大小：~1.2GB

---

#### 7. **镜像版本策略**

**版本命名规范**：

```
格式: {MAJOR}.{MINOR}.{PATCH}{PRERELEASE}

示例:
- 1.2.0        # 稳定版
- 1.2.0b0      # Beta 版
- 1.2.0a1      # Alpha 版
- latest       # 最新版（指向最新稳定版）
```

**标签策略**：

| 标签 | 含义 | 更新频率 |
| ------ | ------ | ---------- |
| `latest` | 最新稳定版 | 每次发布更新 |
| `1.2.0` | 特定版本 | 固定不变 |
| `1.2.0b0` | Beta 版 | 测试期间更新 |
| `nightly` | 每日构建 | 每天更新 |

---

#### 8. **私有镜像仓库**

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
## 1. 打标签
docker tag secretflow/kuscia:1.2.0b0 \
  harbor.example.com/library/kuscia:1.2.0b0

## 2. 登录
docker login harbor.example.com -u admin -p Harbor12345

## 3. 推送
docker push harbor.example.com/library/kuscia:1.2.0b0
```

---

#### 9. **镜像安全**

**镜像扫描**：

```bash
# 使用 Trivy 扫描漏洞
trivy image secretflow/kuscia:1.2.0b0

# 输出示例：
# Total: 15 (UNKNOWN: 0, LOW: 10, MEDIUM: 3, HIGH: 2, CRITICAL: 0)
```

**签名验证**（未来计划）：

```bash
# 使用 Cosign 验证签名
cosign verify secretflow/kuscia:1.2.0b0
```

---

#### 10. **镜像体系总结图**

```
┌─────────────────────────────────────────────────────┐
│              Kuscia 镜像体系                          │
└─────────────────────────────────────────────────────┘

核心镜像:
  ├── secretflow/kuscia:1.2.0b0          ← Kuscia 主程序
  │   ├── kuscia 二进制
  │   ├── k3s 二进制
  │   ├── kubectl 二进制
  │   ├── CRD 定义
  │   └── 配置文件
  │
  ├── secretflow/kuscia-deps:0.7.0b0     ← 基础依赖
  │   ├── Python 运行时
  │   ├── 系统库
  │   └── 工具集
  │
  ├── secretflow/kuscia-envoy:0.6.2b0    ← 网关
  │   └── Envoy Proxy
  │
  └── secretflow/kuscia-monitor:latest   ← 监控
      └── Prometheus Exporter

引擎镜像:
  ├── secretflow/secretflow-lite-anolis8:1.11.0b1
  ├── secretflow/secretflow-anolis8:1.11.0b1
  ├── secretflow/scql:latest
  └── secretflow/serving:latest

镜像仓库:
  ├── 阿里云 (国内): secretflow-registry.cn-hangzhou.cr.aliyuncs.com
  ├── Docker Hub (国际): docker.io
  └── 私有仓库: harbor.example.com

管理工具:
  ├── ./kuscia image list      # 列出镜像
  ├── ./kuscia image import    # 导入镜像
  ├── ./kuscia image export    # 导出镜像
  └── ./kuscia image remove    # 删除镜像
```

---

### 13.10 镜像体系总结

1. ✅ **Kuscia 有完整的镜像体系**
   - 主镜像：`secretflow/kuscia`
   - 依赖镜像：deps、envoy、proot
   - 引擎镜像：SecretFlow、SCQL、Serving

2. ✅ **支持多架构和多仓库**
   - linux/amd64、linux/arm64
   - 阿里云、Docker Hub、私有仓库

3. ✅ **提供完善的镜像管理工具**
   - 导入、导出、删除
   - 离线部署支持
   - 拉取策略配置

4. ✅ **镜像与嵌入式 K3s 的关系**
   - 镜像是**分发载体**
   - K3s 是**运行时组件**
   - 两者独立但配合工作

---

## 14. 总结

### 14.1 核心要点

1. ✅ **Kuscia 的 Kubernetes 配置完全不依赖容器环境**
   - 嵌入式 K3s 作为子进程运行
   - 可以在宿主机、VM、容器中运行
   - K8s API、CRD、Namespace、Informer 全部正常工作

2. ✅ **Namespace 是逻辑隔离，不是容器隔离**
   - 基于 etcd 中的字段标记
   - API Server 强制检查权限
   - 与 Linux Namespace 无关

3. ✅ **Kuscia 的网络不依赖 K8s CNI**
   - 禁用 Flannel、kube-proxy、CoreDNS
   - 通过 NetworkMesh（Envoy + DomainRoute + 自定义 CoreDNS）实现通信
   - 跨域流量默认加密

4. ✅ **Kuscia 支持三种运行模式**
   - **Autonomy**：自带 K3s，适合 P2P、单机、边缘场景
   - **Master**：连接外部 K8s，适合大规模、高可用场景
   - **Lite**：作为工作节点接入 Master，适合中心化组网

5. ✅ **只有任务运行时才可能需要容器**
   - RunK 运行时：需要 containerd/Docker
   - RunP 运行时：不需要容器
   - RunC 运行时：原生容器，资源隔离强

6. ✅ **Autonomy Mode 适合大多数入门场景**
   - 单机部署
   - 边缘计算
   - 开发测试
   - 小规模生产

7. ✅ **Master Mode 适合大规模集群**
   - 利用现有 K8s 设施
   - 多节点调度
   - 高可用需求

### 14.2 架构图回顾

```text
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
│  │  │  CRD ✅                         │  │   │
│  │  │  - DomainData                  │  │   │
│  │  │  - DomainDataGrant             │  │   │
│  │  └────────────────────────────────┘  │   │
│  │                                      │   │
│  │  ┌────────────────────────────────┐  │   │
│  │  │  Namespace 隔离 ✅              │  │   │
│  │  │  RBAC ✅                        │  │   │
│  │  │  Informer/Controller ✅         │  │   │
│  │  └────────────────────────────────┘  │   │
│  │                                      │   │
│  │  ┌────────────────────────────────┐  │   │
│  │  │  NetworkMesh ✅                 │  │   │
│  │  │  Envoy + DomainRoute            │  │   │
│  │  │  自定义 CoreDNS                 │  │   │
│  │  └────────────────────────────────┘  │   │
│  └──────────────────────────────────────┘   │
└─────────────────────────────────────────────┘

✅ 所有 Kubernetes 配置和用法都起作用
❌ 不依赖外部 Kubernetes 集群
❌ 不依赖容器运行时（除非使用 RunK/RunC）
```

## 15. Kuscia 如何向内置 K3s 发送指令

Kuscia 对嵌入式 K3s 的管理不只是“拉起进程”，还包括持续地向 K3s 发送各类指令来完成资源创建、状态查询、事件监听和故障恢复。本节详细说明 Kuscia 与 K3s 之间的指令交互方式、典型场景和失败处理。

### 15.1 交互层次概览

Kuscia 与 K3s 的交互可以分为三个层次：

```text
┌─────────────────────────────────────────────────────────────┐
│                    Kuscia 业务层                             │
│  Controllers / KusciaAPI / Agent / DataMesh / Gateway        │
└──────────────────────┬──────────────────────────────────────┘
                       │ 调用 K8s Go Client (clientset/lister)
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                    K8s Client 层                             │
│  kubernetes.ClientSet  +  Kuscia CRD ClientSet               │
└──────────────────────┬──────────────────────────────────────┘
                       │ HTTPS / REST / Watch
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                    嵌入式 K3s                                │
│  API Server  ←→  etcd                                        │
└─────────────────────────────────────────────────────────────┘
```

在上层业务代码看来，K3s 就是一个标准的 Kubernetes API Server；Kuscia 通过 **Go Client**、**kubectl 二进制** 和 **REST/Watch 调用** 三种方式向它发送指令。

### 15.2 方式一：通过 kubectl 二进制执行命令

Kuscia 镜像中内置了 `kubectl`，在初始化阶段会调用它来完成一些一次性或批量操作，最典型的就是 **CRD 注册**。

**代码位置**：`cmd/kuscia/modules/k3s.go`

```go
func applyCRD(conf *ModuleRuntimeConfigs) error {
    dirPath := filepath.Join(conf.RootDir, "crds/v1alpha1")
    dirs, err := os.ReadDir(dirPath)
    if err != nil {
        return err
    }
    sw := sync.WaitGroup{}
    for _, dir := range dirs {
        if dir.IsDir() {
            continue
        }
        file := filepath.Join(dirPath, dir.Name())
        sw.Add(1)
        go func(f string) {
            applyFile(conf, f)
            sw.Done()
        }(file)
    }
    sw.Wait()
    return nil
}

func applyFile(conf *ModuleRuntimeConfigs, file string) {
    cmd := exec.Command(
        filepath.Join(conf.RootDir, "bin/kubectl"),
        "--kubeconfig", conf.KubeconfigFile,
        "apply", "-f", file,
    )
    cmd.Stderr = os.Stderr
    err := cmd.Run()
    if err != nil {
        nlog.Fatalf("apply %s err:%s", file, err.Error())
    }
    nlog.Infof("apply %s", file)
}
```

**为什么使用 kubectl 而不是 Go Client？**

- CRD YAML 文件本身就是为 `kubectl apply` 设计的，直接复用无需解析。
- `kubectl apply` 会自动处理创建或更新。
- 对运维人员更直观，便于调试时手动复现。

**其他 kubectl 使用场景**：

| 场景 | 示例命令 |
| ------ | ---------- |
| 查看资源 | `kubectl get domaindatas -n alice` |
| 查看日志 | `kubectl logs <pod> -n alice` |
| 进入容器 | `kubectl exec -it <pod> -n alice -- /bin/bash` |
| 诊断节点 | `kubectl get nodes` / `kubectl describe node <node>` |

> 注意：这些命令在 Kuscia 内部由程序自动调用，普通用户通常通过 KusciaAPI 或 HTTP API 与系统交互。

### 15.3 方式二：通过 K8s Go Client 编程式交互

对于需要频繁、类型安全地操作 K8s 资源的场景，Kuscia 使用 Go Client（clientset）直接调用 K3s API Server。

#### 15.3.1 什么是 Clientset

**Clientset** 是 Kubernetes 官方 Go 客户端对 REST API 的封装，把每一种 K8s 资源（如 Pod、Service、Namespace、Deployment 等）都映射成一组类型安全的方法（Create / Get / Update / Delete / List / Watch）。业务代码只需操作 Go 结构体，无需手写 HTTP 请求或解析 JSON。

在 Kuscia 中，一个 `KubeClients` 结构体把与 K3s 交互所需的全部客户端集中管理：

```text
┌─────────────────────────────────────────────────────────────────────┐
│                           KubeClients                               │
├─────────────────┬───────────────────────────────────────────────────┤
│  KubeClient     │  kubernetes.Interface                             │
│                 │  操作 Pod/Namespace/Service/ConfigMap/Secret/     │
│                 │  ServiceAccount/Role/RoleBinding 等标准 K8s 资源   │
├─────────────────┼───────────────────────────────────────────────────┤
│  KusciaClient   │  kusciaclientset.Interface                        │
│                 │  操作 Domain / DomainData / DomainDataGrant /     │
│                 │  KusciaJob / KusciaTask / AppImage 等 Kuscia CRD   │
├─────────────────┼───────────────────────────────────────────────────┤
│ ExtensionsClient│  apiextensionsclientset.Interface                 │
│                 │  操作 CustomResourceDefinition（CRD 定义本身）     │
├─────────────────┼───────────────────────────────────────────────────┤
│  Kubeconfig     │  *restclient.Config                               │
│                 │  公共的 REST 配置：Server 地址、证书、Token、       │
│                 │  QPS、Burst、Timeout，所有 clientset 共享         │
└─────────────────┴───────────────────────────────────────────────────┘
```

**三个 clientset 的关系**：

| 字段 | 类型 | 作用范围 | 典型使用场景 |
| ------ | ------ | ---------- | -------------- |
| `KubeClient` | `kubernetes.Interface` | 标准 K8s 资源 | 创建 Namespace、ServiceAccount、RBAC、Secret、查询节点等 |
| `KusciaClient` | `kusciaclientset.Interface` | Kuscia 自定义资源 | 创建/监听 DomainData、KusciaJob、KusciaTask、DomainRoute 等 |
| `ExtensionsClient` | `apiextensionsclientset.Interface` | CRD 元数据 | 查询或维护 CRD 定义（较少直接使用，通常由 kubectl apply 完成） |

使用接口（`Interface`）而非具体实现的好处是：

- **可测试性**：单元测试时可以注入 fake clientset，无需真实 K3s。
- **可替换性**：未来如果切换为其他 K8s 发行版，只要接口不变，业务代码无需改动。
- **限流可控**：通过共享的 `*restclient.Config` 统一设置 QPS/Burst，避免对 API Server 造成流量冲击。

#### 15.3.2 创建客户端

**代码位置**：`pkg/utils/kubeconfig/kube_config.go`

```go
func CreateClientSetsFromKubeconfig(kubeconfigPath, masterURL string) (*KubeClients, error) {
    return CreateClientSetsFromKubeconfigWithOptions(kubeconfigPath, masterURL, 500, 1000, 0)
}

func CreateClientSetsFromKubeconfigWithOptions(
    kubeconfigPath, masterURL string,
    QPS float32, Burst int, Timeout time.Duration,
) (*KubeClients, error) {
    kubeConfig, err := BuildClientConfigFromKubeconfig(kubeconfigPath, masterURL)
    if err != nil {
        return nil, fmt.Errorf("error building config, detail-> %v", err)
    }
    if QPS != 0 {
        kubeConfig.QPS = QPS
    }
    if Burst != 0 {
        kubeConfig.Burst = Burst
    }
    if Timeout != 0 {
        kubeConfig.Timeout = Timeout
    }
    return CreateClientSets(kubeConfig)
}

func CreateClientSets(config *restclient.Config) (*KubeClients, error) {
    kubeClient, err := kubernetes.NewForConfig(config)
    if err != nil {
        return nil, fmt.Errorf("error building kubernetes client set, detail-> %v", err)
    }
    kusciaClient, err := kusciaclientset.NewForConfig(config)
    if err != nil {
        return nil, fmt.Errorf("error building domain client set, detail-> %v", err)
    }
    extensionClient, err := apiextensionsclientset.NewForConfig(config)
    if err != nil {
        return nil, fmt.Errorf("error building apiextensions client set, detail-> %v", err)
    }
    return &KubeClients{
        KubeClient:       kubeClient,
        KusciaClient:     kusciaClient,
        ExtensionsClient: extensionClient,
        Kubeconfig:       config,
    }, nil
}
```

**典型编程式操作示例**：

```go
// 创建 Namespace
ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "alice"}}
if _, err := clients.KubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
    if !k8serrors.IsAlreadyExists(err) {
        return err
    }
}

// 创建 ServiceAccount
sa := &corev1.ServiceAccount{
    ObjectMeta: metav1.ObjectMeta{Name: "alice-sa", Namespace: "alice"},
}
if _, err := clients.KubeClient.CoreV1().ServiceAccounts("alice").Create(ctx, sa, metav1.CreateOptions{}); err != nil {
    return err
}

// 创建 Domain CR（Kuscia CRD）
domain := &kusciaapisv1alpha1.Domain{
    ObjectMeta: metav1.ObjectMeta{Name: "alice"},
    Spec: kusciaapisv1alpha1.DomainSpec{Cert: certBase64},
}
if _, err := clients.KusciaClient.KusciaV1alpha1().Domains().Create(ctx, domain, metav1.CreateOptions{}); err != nil {
    return err
}

// 查询 KusciaJob
job, err := clients.KusciaClient.KusciaV1alpha1().KusciaJobs("cross-domain").Get(ctx, "my-job", metav1.GetOptions{})
if err != nil {
    return err
}
```

**Go Client 的优势**：

- ✅ 类型安全，编译期即可发现错误。
- ✅ 无需解析 shell 输出。
- ✅ 支持上下文控制、超时、重试（QPS/Burst/Timeout 可配置）。
- ✅ 可以方便地操作自定义资源（CRD）。

### 15.4 方式三：通过 Informer/Lister 监听资源变化

Kuscia Controller 的核心工作模式是 **事件驱动**：通过 Informer 监听 K3s 中资源的变化，然后在 Handler 中执行业务逻辑。

#### 15.4.1 Informer 可以监听哪些资源

Informer 本质上是对 K3s API Server 的 **List & Watch** 封装，因此 K3s 中几乎所有资源都可以被监听。在 Kuscia 中，主要分为三类：

**1. 标准 Kubernetes 资源**

通过 `k8s.io/client-go/informers` 创建：

| 资源类型 | Informer 获取方式 | 典型监听目的 |
| ---------- | ------------------- | -------------- |
| Node | `factory.Core().V1().Nodes()` | 感知节点就绪/资源变化 |
| Namespace | `factory.Core().V1().Namespaces()` | 感知域命名空间创建/删除 |
| Pod | `factory.Core().V1().Pods()` | 跟踪任务 Pod 生命周期 |
| Service | `factory.Core().V1().Services()` | 服务发现、端点同步 |
| ConfigMap | `factory.Core().V1().ConfigMaps()` | 配置变更热加载 |
| Secret | `factory.Core().V1().Secrets()` | 证书、Token 变更感知 |
| ServiceAccount | `factory.Core().V1().ServiceAccounts()` | 权限配置同步 |
| Role / RoleBinding | `factory.Rbac().V1().Roles()` / `RoleBindings()` | RBAC 变更同步 |

**2. Kuscia 自定义资源（CRD）**

通过 `github.com/secretflow/kuscia/pkg/crd/informers/externalversions` 创建：

| 资源类型 | Informer 获取方式 | 典型监听目的 |
| ---------- | ------------------- | -------------- |
| Domain | `factory.Kuscia().V1alpha1().Domains()` | 节点/域注册与证书变更 |
| DomainData | `factory.Kuscia().V1alpha1().DomainDatas()` | 数据发布、授权、更新 |
| DomainDataGrant | `factory.Kuscia().V1alpha1().DomainDataGrants()` | 跨域数据授权变化 |
| KusciaJob | `factory.Kuscia().V1alpha1().KusciaJobs()` | 作业调度与状态推进 |
| KusciaTask | `factory.Kuscia().V1alpha1().KusciaTasks()` | 任务执行与状态同步 |
| DomainRoute | `factory.Kuscia().V1alpha1().DomainRoutes()` | 跨域路由配置变更 |
| AppImage | `factory.Kuscia().V1alpha1().AppImages()` | 算法镜像注册与更新 |

**3. CRD 定义本身**

通过 `apiextensionsinformers.NewSharedInformerFactory` 监听 `CustomResourceDefinition`，用于判断某类 CRD 是否已就绪，或者在动态扩展 CRD 时做响应。

> 在 Kuscia 中，**最常用的是 Kuscia CRD Informer**，因为业务状态（Job、Task、DomainData 等）都保存在 CRD 中；标准 K8s 资源 Informer 主要用于 Pod/Namespace 等基础设施资源的协同。

#### 15.4.2 如何编写 Handler

Informer 通过 `AddEventHandler` 注册回调函数。一个典型的 Handler 需要实现三种事件：

```go
informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc: func(obj interface{}) {
        dd := obj.(*v1alpha1.DomainData)
        // 资源被创建
        handleDomainDataCreate(dd)
    },
    UpdateFunc: func(oldObj, newObj interface{}) {
        oldDD := oldObj.(*v1alpha1.DomainData)
        newDD := newObj.(*v1alpha1.DomainData)
        // 通常只在 Generation 或关键字段变化时处理，避免无限循环
        if oldDD.Generation != newDD.Generation {
            handleDomainDataUpdate(newDD)
        }
    },
    DeleteFunc: func(obj interface{}) {
        // 需要注意 DeletedFinalStateUnknown，它是 Watch 断开期间缓存的 tombstone
        dd, ok := obj.(*v1alpha1.DomainData)
        if !ok {
            tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
            if ok {
                dd, ok = tombstone.Obj.(*v1alpha1.DomainData)
            }
        }
        if dd != nil {
            handleDomainDataDelete(dd)
        }
    },
})
```

**Handler 编写的最佳实践**：

1. **不要在 Handler 中做重逻辑**  
   Handler 运行在 Informer 的反射器（Reflector）协程中，如果阻塞会拖慢事件消费。正确做法是把 key（如 `namespace/name`）放入 `workqueue`，由多个 worker 协程异步处理。

2. **利用 Generation 过滤无效 Update**  
   `metadata.generation` 只在 spec 变化时递增。如果 Handler 只关心 spec 变化，可以忽略 status 更新导致的重复回调。

3. **处理 Delete 事件时注意 tombstone**  
   当 Watch 连接断开后，缓存中的对象可能被包装成 `cache.DeletedFinalStateUnknown`，需要类型断言处理。

4. **幂等性**  
   同一个事件可能因为网络抖动、Informer 重连或控制器重启被多次处理，业务逻辑应当能够安全地重复执行。

#### 15.4.3 带 WorkQueue 的完整 Handler 示例

```go
// 1. 创建 Informer 和 Lister
factory := kusciainformers.NewSharedInformerFactory(kusciaClient, time.Minute*10)
domainDataInformer := factory.Kuscia().V1alpha1().DomainDatas()
domainDataLister := domainDataInformer.Lister()

// 2. 创建工作队列
workqueue := workqueue.NewNamedRateLimitingQueue(
    workqueue.DefaultControllerRateLimiter(),
    "DomainData",
)

// 3. 注册事件 Handler，只做最简单的入队
var enqueueDomainData = func(obj interface{}) {
    key, err := cache.MetaNamespaceKeyFunc(obj)
    if err != nil {
        return
    }
    workqueue.Add(key)
}

domainDataInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc:    enqueueDomainData,
    UpdateFunc: func(old, new interface{}) { enqueueDomainData(new) },
    DeleteFunc: enqueueDomainData,
})

// 4. 启动 worker 处理队列
for i := 0; i < 3; i++ {
    go func() {
        for processNextItem(workqueue, domainDataLister, kusciaClient) {
        }
    }()
}

stopCh := make(chan struct{})
factory.Start(stopCh)
factory.WaitForCacheSync(stopCh)
<-stopCh

// 5. 业务处理函数
func processNextItem(queue workqueue.RateLimitingInterface, lister v1alpha1listers.DomainDataLister, client kusciaclientset.Interface) bool {
    key, quit := queue.Get()
    if quit {
        return false
    }
    defer queue.Done(key)

    namespace, name, err := cache.SplitMetaNamespaceKey(key.(string))
    if err != nil {
        queue.Forget(key)
        return true
    }

    dd, err := lister.DomainDatas(namespace).Get(name)
    if err != nil {
        if k8serrors.IsNotFound(err) {
            // 对象已被删除，执行清理逻辑
            handleDomainDataDeleted(namespace, name)
            queue.Forget(key)
            return true
        }
        queue.AddRateLimited(key)
        return true
    }

    // 执行业务逻辑
    if err := reconcileDomainData(dd, client); err != nil {
        queue.AddRateLimited(key)
        return true
    }

    queue.Forget(key)
    return true
}
```

**Informer 的工作机制**：

1. **List & Watch**：启动时先 List 全量资源，然后建立长连接 Watch 后续变化。
2. **DeltaFIFO**：将事件放入队列。
3. **Indexer**：维护本地缓存和索引，支持快速查询。
4. **Handler**：回调业务逻辑。

**为什么使用 Informer 而不是轮询？**

- ✅ 实时性高（秒级甚至毫秒级事件通知）。
- ✅ 减少 API Server 压力（长连接而非频繁 List）。
- ✅ 本地缓存提供微秒级查询速度。

### 15.5 启动阶段：Kuscia 初始化内置 K3s 的完整流程

Kuscia 启动时通过以下步骤完成 K3s 生命周期管理和初始指令下发：

```text
1. Kuscia 主进程启动
        │
        ▼
2. buildK3sArgs() 生成启动参数
   禁用 scheduler / agent / flannel / coredns / traefik / servicelb /
   metrics-server / local-storage，并可选 rootless 与外部 datastore
        │
        ▼
3. Supervisor 拉起 K3s 子进程
        │
        ▼
4. startCheckReady() 轮询 /readyz 与证书文件
        │
        ▼
5. initKusciaEnvAfterReady()
   ├─ 5.1 applyCRD()          通过 kubectl apply 注册全部 CRD
   ├─ 5.2 applyKusciaResources() 通过 kubectl apply 初始化集群级资源
   ├─ 5.3 genKusciaKubeConfig()  生成 kuscia 客户端 kubeconfig 与 RBAC
   ├─ 5.4 CreateClientSetsFromKubeconfig() 创建 Go Client
   ├─ 5.5 createDefaultDomain() 创建 Domain CR
   └─ 5.6 createCrossNamespace() 创建跨域命名空间
        │
        ▼
6. 各业务模块（Controllers / API / Gateway）启动，通过 Informer/Client 交互
```

**关键代码**：

```go
func (s *k3sModule) initKusciaEnvAfterReady(ctx context.Context) error {
    if err := applyCRD(s.conf); err != nil {
        return err
    }
    if err := applyKusciaResources(s.conf); err != nil {
        return err
    }
    if err := genKusciaKubeConfig(s.conf); err != nil {
        return err
    }

    clients, err := kubeconfig.CreateClientSetsFromKubeconfig(
        s.conf.KubeconfigFile,
        s.conf.ApiserverEndpoint,
    )
    if err != nil {
        return err
    }
    s.conf.Clients = clients

    if err := createDefaultDomain(ctx, s.conf); err != nil {
        return err
    }
    if err := createCrossNamespace(ctx, s.conf); err != nil {
        return err
    }
    return nil
}
```

### 15.6 常见指令场景对照表

| 业务场景 | 使用方式 | 示例 |
| ---------- | ---------- | ------ |
| 注册 CRD | kubectl | `kubectl --kubeconfig kubeconfig apply -f crds/v1alpha1/...yaml` |
| 初始化集群资源 | kubectl | `kubectl --kubeconfig kubeconfig apply -f conf/domain-cluster-res.yaml` |
| 创建 Namespace | Go Client | `clients.KubeClient.CoreV1().Namespaces().Create(...)` |
| 创建 Domain CR | Go Client (CRD) | `clients.KusciaClient.KusciaV1alpha1().Domains().Create(...)` |
| 创建/查询 KusciaJob | Go Client (CRD) | `clients.KusciaClient.KusciaV1alpha1().KusciaJobs(...)` |
| 监听资源变化 | Informer | `informer.Informer().AddEventHandler(...)` |
| 查询资源（缓存） | Lister | `lister.DomainDatas("alice").Get(name)` |
| 查看 Pod 日志 | kubectl | `kubectl logs <pod> -n <ns>` |
| 进入容器调试 | kubectl | `kubectl exec -it <pod> -n <ns> -- /bin/sh` |

### 15.7 指令失败处理与重试

Kuscia 向 K3s 发送指令时可能遇到多种失败场景，Controller 通常会做如下处理：

**1. 网络/API Server 暂时不可用**

```go
if err != nil && k8serrors.IsTimeout(err) {
    // 重新入队，稍后重试
    c.workqueue.AddRateLimited(key)
    return err
}
```

**2. 资源已存在（AlreadyExists）**

```go
if err != nil && k8serrors.IsAlreadyExists(err) {
    // 通常忽略或执行更新
    return nil
}
```

**3. 冲突（Conflict）**

```go
if err != nil && k8serrors.IsConflict(err) {
    // 对象版本冲突，重新从 API Server 获取最新版本后重试
    c.workqueue.AddRateLimited(key)
    return err
}
```

**4. 配额不足或权限不足**

- 记录事件和状态 Reason/Message。
- 不上报为瞬时错误，通常需要管理员介入。

**5. K3s 进程崩溃**

- Supervisor 检测到进程退出后自动重启 K3s。
- 其他模块通过 Informer 的 Watch 连接断开重连机制自动恢复。

### 15.8 与外部 K8s 模式的区别

| 维度 | 嵌入式 K3s（Autonomy） | 外部 K8s（Master） |
| ------ | ------------------------ | -------------------- |
| 进程管理 | Kuscia 通过 Supervisor 管理 K3s | Kuscia 不管理外部 K8s |
| 客户端地址 | `https://127.0.0.1:6443` | 配置文件中指定的 API Server |
| 认证方式 | 本地证书/kubeconfig | 通常使用 ServiceAccount Token |
| 指令对象 | K3s 子进程 | 外部 K8s 集群 |
| 失败恢复 | Supervisor 自动重启 | 依赖外部集群自身高可用 |

### 15.9 关键结论

- Kuscia 通过 **kubectl 二进制**、**K8s Go Client** 和 **Informer/Lister** 三层方式与内置 K3s 交互。
- 初始化阶段多用 kubectl 完成批量 CRD 与集群资源注册；运行期间主要使用 Go Client 和 Informer 做类型安全、事件驱动的操作。
- 所有指令最终都通过 K3s API Server 落到 etcd，Kuscia 自身不直接操作 etcd。
- K3s 进程由 Kuscia Supervisor 守护，崩溃后可自动恢复，上层业务通过 Informer 重连感知恢复。

## 16. Domain、K3s 命名空间与资源归属

在 Kuscia 中，**Domain** 是最核心的逻辑概念之一，它既是参与隐私计算任务的参与方标识，也是 K3s 中资源隔离的基本单位。理解 Domain、K3s 命名空间（Namespace）以及资源创建时如何指定归属，是正确使用 Kuscia API 和排查多域问题的关键。

### 16.1 核心概念对应关系

```text
┌─────────────────────────────────────────────────────────────────────┐
│                         Kuscia 概念层                                │
│  Domain（域）: 一个参与方，例如 alice、bob、carol                    │
└──────────────────────────────┬──────────────────────────────────────┘
                               │ 一一映射
                               ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      K3s / Kubernetes 层                             │
│  Namespace（命名空间）: alice、bob、carol                            │
└──────────────────────────────┬──────────────────────────────────────┘
                               │ 资源创建时通过 metadata.namespace 指定
                               ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         实际资源                                     │
│  DomainData / KusciaJob / KusciaTask / Pod / Service ...            │
└─────────────────────────────────────────────────────────────────────┘
```

**关键对应关系**：

| Kuscia 概念 | K3s 概念 | 作用 |
| ------------- | ---------- | ------ |
| Domain | Namespace | 逻辑参与方与资源隔离边界 |
| Domain ID | Namespace Name | 通常相同，例如 `alice` |
| Domain Cert | Secret / 证书配置 | 用于跨域身份认证 |
| KusciaJob | CRD in `cross-domain` Namespace | 跨域作业协调 |
| KusciaTask | CRD in Domain Namespace | 单域任务执行 |

### 16.2 Domain 是如何映射到 Namespace 的

Kuscia 启动时会在 K3s 中完成以下初始化：

1. **创建 Domain 对应的 Namespace**  
   例如在 Autonomy 模式下启动域 `alice` 时，Kuscia 会调用：

   ```go
   clients.KubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
       ObjectMeta: metav1.ObjectMeta{Name: "alice"},
   }, metav1.CreateOptions{})
   ```

2. **创建 Domain CR**  
   在 K3s 中创建一个 `Domain` 自定义资源，记录该域的证书信息：

   ```go
   clients.KusciaClient.KusciaV1alpha1().Domains().Create(ctx, &kusciaapisv1alpha1.Domain{
       ObjectMeta: metav1.ObjectMeta{Name: "alice"},
       Spec: kusciaapisv1alpha1.DomainSpec{
           Cert: domainCertBase64,
       },
   }, metav1.CreateOptions{})
   ```

3. **创建跨域 Namespace**  
   用于存放需要多个域共同可见的协调资源（如 `KusciaJob`）：

   ```go
   clients.KubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
       ObjectMeta: metav1.ObjectMeta{Name: "cross-domain"},
   }, metav1.CreateOptions{})
   ```

> Kuscia 中的 `Domain` CR 与 `Namespace` 是**同名一一对应**的。看到 `Domain alice`，就可以认为 K3s 中存在 `Namespace alice`。

### 16.3 创建资源时如何指定 Domain

在 Kuscia 中，资源的 Domain 归属由其所在的 **K3s Namespace** 决定。创建任何 CR 时，只需要设置 `metadata.namespace` 字段即可。

#### 16.3.1 单域资源：放在 Domain 自己的 Namespace

以 `DomainData` 为例，表示某个域拥有的数据：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: user-table
  namespace: alice          # ← 指定该数据属于 alice 域
spec:
  type: table
  relative_uri: alice/user-table.csv
  columns:
    - name: id
      type: str
    - name: age
      type: int
  author: alice
```

通过 kubectl 创建：

```bash
kubectl --kubeconfig /var/lib/kuscia/etc/kubeconfig apply -f alice-user-table.yaml
```

通过 Go Client 创建：

```go
dd := &kusciaapisv1alpha1.DomainData{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "user-table",
        Namespace: "alice",   // ← 指定 Domain
    },
    Spec: kusciaapisv1alpha1.DomainDataSpec{
        Type:        "table",
        RelativeURI: "alice/user-table.csv",
        Author:      "alice",
    },
}
_, err := clients.KusciaClient.KusciaV1alpha1().DomainDatas("alice").Create(ctx, dd, metav1.CreateOptions{})
```

#### 16.3.2 跨域资源：放在 `cross-domain` Namespace

当多个域需要协同完成一个作业时，由发起方创建一个 `KusciaJob`，放在 `cross-domain` 命名空间：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-alice-bob
  namespace: cross-domain    # ← 跨域协调命名空间
spec:
  initiator: alice           # ← 发起方
  schedule_mode: Strict
  tasks:
    - id: psi-task
      alias: psi
      app_image: secretflow/psi
      parties:
        - name: alice        # ← 参与方 1
          role: host
          domain_id: alice
          inputs:
            - domaindata_id: user-table
        - name: bob          # ← 参与方 2
          role: guest
          domain_id: bob
          inputs:
            - domaindata_id: order-table
```

在这个例子中：

- `KusciaJob` 本身在 `cross-domain` 命名空间。
- `spec.initiator: alice` 说明由 alice 发起。
- `spec.tasks[].parties[].domain_id` 说明每个子任务由哪些域参与。
- alice 的输入 `user-table` 来自 `alice` 命名空间下的 `DomainData`。
- bob 的输入 `order-table` 来自 `bob` 命名空间下的 `DomainData`。

### 16.4 资源可见性与隔离规则

Kuscia 通过 K3s 的 Namespace 机制实现域间资源隔离：

```text
Namespace alice                      Namespace bob
├─ DomainData: user-table            ├─ DomainData: order-table
├─ KusciaTask: task-xxx              ├─ KusciaTask: task-yyy
├─ Pod: job-xxx-driver               ├─ Pod: job-yyy-driver
└─ Secret: alice-cert                └─ Secret: bob-cert

Namespace cross-domain
├─ KusciaJob: psi-alice-bob          ← 协调资源，所有相关域可见
└─ DomainDataGrant: grant-xxx        ← 跨域授权记录
```

**隔离规则**：

| 资源类型 | 所在 Namespace | 谁能看到 |
| ---------- | ---------------- | ---------- |
| DomainData | 各 Domain Namespace | 仅该 Domain（及被授权方） |
| KusciaTask | 各 Domain Namespace | 仅该 Domain |
| Pod / Service / Secret | 各 Domain Namespace | 仅该 Domain |
| KusciaJob | `cross-domain` | 所有参与方可读 |
| DomainDataGrant | `cross-domain` | 所有相关方可读 |

> 这里的“谁能看到”包含两层含义：K3s RBAC 的Namespace 级权限，以及 Kuscia 业务层对 Domain 字段的校验。

### 16.5 常见误区与排查

**误区 1：认为 Domain 是 K3s 的 Node**

- 实际上 K3s 的 Node 通常只有 Kuscia 自身节点；Domain 是逻辑概念，通过 Namespace 隔离。
- 查询节点用 `kubectl get nodes`，查询域用 `kubectl get domains`。

**误区 2：创建资源时不指定 namespace，默认归属当前域**

- K3s 没有“当前域”的概念，不指定 `metadata.namespace` 的资源会进入 `default` 命名空间，可能被拒绝或无法被正确识别。
- **最佳实践**：始终显式指定 `metadata.namespace`。

**误区 3：把跨域作业的资源也放在自己域的 Namespace**

- `KusciaJob` 是协调资源，必须放在 `cross-domain`；每个参与方的实际执行单元 `KusciaTask` 会分别落在各自 Domain 的 Namespace。

**排查命令**：

```bash
# 查看所有 Domain 对应的 Namespace
kubectl get namespaces

# 查看某个 Domain 下的所有资源
kubectl get all -n alice

# 查看 cross-domain 下的协调资源
kubectl get kusciajobs -n cross-domain
kubectl get domaindatagrants -n cross-domain

# 查看 Domain CR 本身
kubectl get domains
```

### 16.6 关键结论

- **Domain = K3s Namespace**：Kuscia 用 K3s 命名空间实现域级隔离，Domain ID 通常就是 Namespace 名称。
- **资源归属由 `metadata.namespace` 决定**：创建 DomainData、KusciaTask 等单域资源时，放在对应 Domain 的 Namespace。
- **跨域协调资源放在 `cross-domain`**：`KusciaJob` 及其授权 `DomainDataGrant` 放在该命名空间，供所有参与方可见。
- **参与方通过 `domain_id` 显式指定**：跨域作业中，`parties[].domain_id` 明确说明每个子任务由哪个域执行。

## 17. Kuscia 数据存储与 DataMesh

Kuscia 不仅要管理任务的调度和执行，还需要管理隐私计算过程中涉及的各类数据（样本表、模型、规则、报告等）。为了做到数据**可用不可见**，Kuscia 将**数据元信息**与**实际数据**分离：元信息通过 K3s CRD 管理，实际数据通过 **DataMesh** 统一访问。

### 17.1 Kuscia 数据存储概述

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

### 17.2 核心数据对象

Kuscia 定义了三种与数据相关的 CRD：

#### 17.2.1 DomainDataSource（数据源）

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

#### 17.2.2 DomainData（数据对象）

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

#### 17.2.3 DomainDataGrant（跨域授权）

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

#### 17.2.4 DomainDataSource 与 DataMesh 的关系

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

### 17.3 DataMesh 架构

DataMesh 是 Kuscia 中负责数据管理与访问的独立模块，通常以 gRPC/HTTP 服务形式运行在每个 Kuscia 节点（Autonomy / Lite）上。

#### 17.3.1 DataMesh 如何启动

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

#### 17.3.2 创建 DomainData / DomainDataSource 时，DataMesh 会自动介入吗？

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

#### 17.3.3 DataMesh 控制平面与数据平面

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

#### 17.3.4 内部管理与外部访问的接口边界

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

### 17.4 DataMesh 与数据存储的关系

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

### 17.5 数据读写流程

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

### 17.6 支持的存储后端

DataMesh 目前支持多种内置存储后端：

| 后端类型 | 类型标识 | 说明 |
| ---------- | ---------- | ------ |
| 本地文件系统 | `localfs` | 默认数据源，路径在 `var/storage/data` 下 |
| OSS / S3 兼容 | `oss` | 支持阿里云 OSS 等 S3 兼容对象存储 |
| MySQL | `mysql` | 通过 SQL 读取表数据 |
| PostgreSQL | `postgresql` | 通过 SQL 读取表数据 |
| 外部 DataProxy | `external` | 转发到外部数据代理，支持 ODPS、Hive 等 |

每种后端对应一个 `DataMeshDataIOInterface`，实现 `GetFlightInfo`、`Read`、`Write` 三个核心方法。

### 17.7 数据隔离

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

### 17.8 DataMesh 在任务执行中的作用

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

### 17.9 关键结论

- Kuscia 将数据存储分为**元数据层**（K3s CRD）和**数据平面**（实际存储）。
- `DomainData` 是数据的“元信息名片”，`DomainDataSource` 是“存储地址簿”。
- `DomainData.relative_uri` 是相对于 `DomainDataSource` 的相对路径，DataMesh 负责拼接出实际访问位置，无需在 `DomainData` 中写死完整路径。
- **DataMesh 作为 Kuscia 模块启动**，启动时会注册默认 `localfs` 数据源，并按需监听/查询 K3s 中的 CRD；它不会主动扫描或预热所有数据。
- 创建 `DomainData` / `DomainDataSource` **不需要手动调用 DataMesh 接口注册**，两者的关联完全由 `DomainData.spec.data_source` 字段声明；DataMesh 在收到 Flight 请求时自动解析。
- **内部管理与外部访问有明确边界**：Kuscia 内部通过 kubectl/KusciaAPI/Go Client 管理 CRD 元数据；外部引擎（如 SecretFlow）必须通过 DataMesh Arrow Flight (`GetFlightInfo` / `DoGet` / `DoPut`) 读写实际数据。
- **DataMesh 是统一数据访问层**，根据 `DomainDataSource.type` 选择对应的 IO Channel，通过 Apache Arrow Flight 为引擎提供与后端无关的读写能力。
- 跨域数据使用需要 `DomainDataGrant` 授权，控制器会验证签名、有效期、使用次数，并将数据拷贝到目标域。
- 数据隔离主要依靠 K3s Namespace + Author 字段 + 授权拷贝机制实现。

## 18. 附录 A：如何将宿主机命令转换为 Docker 命令

### 18.1 宿主机与 Docker 两种运行方式的差异

#### 宿主机直接运行

```bash
./kuscia start --config autonomy_alice.yaml --rootless
```

**特点：**

- 直接在操作系统层面运行 Kuscia 应用
- 使用 `--rootless` 参数表示以非 root 用户身份运行
- 直接访问宿主机的文件系统和网络资源
- 性能更好，没有容器化开销
- 对系统环境要求更高

#### Docker 容器运行

```bash
docker run -d kuscia:latest start --config autonomy_alice.yaml
```

**特点：**

- 在隔离的容器环境中运行 Kuscia
- 使用 `-d` 参数以后台模式运行
- 与宿主机环境隔离，更安全
- 需要正确映射配置文件和端口
- 便于部署和迁移

### 18.2 添加必要的参数和挂载

```bash
docker run -d \
  --name kuscia-autonomy \
  -v $(pwd)/autonomy_alice.yaml:/etc/kuscia/config/autonomy_alice.yaml \
  -v /tmp/kuscia:/tmp/kuscia \
  --network host \
  kuscia:latest start --config /etc/kuscia/config/autonomy_alice.yaml
```

### 18.3 关键转换要点

**配置文件映射：**

- 宿主机上 `--config autonomy_alice.yaml` 是相对路径
- Docker 中需要将配置文件挂载到容器内相应位置

**端口映射：**

- 如果配置文件中指定了端口，可能需要添加 `-p` 参数
- 或者使用 `--network host` 模式直接使用宿主机网络

**用户权限：**

- 宿主机使用 `--rootless` 以非 root 用户运行
- Docker 中可以通过 `--user` 参数指定运行用户

### 18.4 完整的转换示例

如果要完全对应您的宿主机命令，应该这样写：

```bash
# 假设配置文件在当前目录
docker run -d \
  --name kuscia-autonomy \
  -v $(pwd)/autonomy_alice.yaml:/etc/kuscia/config/autonomy_alice.yaml \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --network host \
  kuscia:latest start --config /etc/kuscia/config/autonomy_alice.yaml
```

**注意事项：**

- 确保配置文件路径在容器中可用
- 根据配置文件内容确定是否需要额外的卷挂载或端口映射
- 检查配置文件中是否有依赖于宿主机特定路径的设置
