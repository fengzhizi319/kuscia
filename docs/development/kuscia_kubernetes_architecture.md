# Kuscia 中 Kubernetes 配置与运行模式详解

## 目录

- [核心问题解答](#核心问题解答)
- [Kuscia 的两种运行模式](#kuscia-的两种运行模式)
- [嵌入式 K3s 架构](#嵌入式-k3s-架构)
- [Kubernetes 配置的适用性分析](#kubernetes-配置的适用性分析)
- [命名空间隔离机制](#命名空间隔离机制)
- [容器运行时支持](#容器运行时支持)
- [实际应用场景对比](#实际应用场景对比)
- [常见问题 FAQ](#常见问题-faq)
- [嵌入式 K3s 中运行的功能](#嵌入式-k3s-中运行的功能)
- [Kuscia 如何管理 K8s](#kuscia-如何管理-k8s)
- [Kuscia 镜像体系](#kuscia-镜像体系)

---

## 核心问题解答

### 问题1：为什么不直接运行在容器中，Kubernetes 配置还起作用吗？

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

### 设计思想与流程通俗解释

#### 1. Kuscia 的整体设计思想

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

#### 2. Kuscia 的工作流程

##### 流程图解：从启动到运行

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

##### 详细流程说明

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

#### 3. 为什么选择这种设计？

##### 优势对比

| 传统方式 | Kuscia 方式 |
|---------|------------|
| 需要预先安装 Kubernetes 集群 | 一键启动，无需外部依赖 |
| 需要手动注册 CRD | 自动注册，开箱即用 |
| 需要管理多个组件 | 一体化管理，简化运维 |
| 资源占用大（几 GB） | 轻量化（几百 MB） |
| 部署复杂（需要专业知识） | 简单部署（普通用户即可） |

##### 解决的核心问题

1. **部署门槛高**：传统 K8s 部署需要专业的运维知识，Kuscia 将其简化为一键启动
2. **资源消耗大**：传统 K8s 需要大量资源，Kuscia 优化为轻量化部署
3. **运维复杂**：多个组件需要分别管理，Kuscia 统一管理
4. **标准化接口**：虽然简化了部署，但仍然使用标准的 K8s API，保持了生态兼容性

#### 4. 实际运行场景

##### 场景 1：单机开发

```bash
# 在笔记本电脑上运行
./kuscia start --config autonomy_dev.yaml --rootless

# 立即可用标准 K8s API
kubectl get domaindatas -A
kubectl apply -f my-data.yaml
```

##### 场景 2：边缘计算

```bash
# 在边缘设备上运行（资源受限）
./kuscia start --config autonomy_edge.yaml

# 轻量化运行，满足边缘计算需求
```

##### 场景 3：生产环境

```bash
# 在生产环境中运行
./kuscia start --config autonomy_prod.yaml

# 使用外部数据库，支持高可用
```

这种设计使得 Kuscia 既保持了 Kubernetes 的强大功能，又大大降低了使用门槛，真正做到了“即插即用”。

---

### 问题2: 为什么 Kuscia 要用 Kubernetes？直接用 Go 代码不行吗？

**答：理论上可以，但实际开发工作量会巨大，且难以保证质量和可维护性。**

Kubernetes 提供了**成熟的抽象和生态**，让开发工作量大幅降低。下面通过具体例子详细说明。

---

#### 一、Kubernetes 提供的核心抽象能力

##### 1. **声明式 API（Declarative API）**

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
|------|---------------|-----------|
| API 定义 | YAML 声明 | 手动定义 Request/Response 结构 |
| HTTP Server | API Server 内置 | 需要自己实现路由、Handler |
| 参数验证 | OpenAPI Schema 自动生成 | 每个字段手动验证 |
| 权限控制 | RBAC 内置 | 自己实现鉴权中间件 |
| 错误处理 | 标准错误码 | 自己定义错误码体系 |
| 文档生成 | kubectl explain 自动支持 | 手动编写 API 文档 |
| **代码行数** | **~100 行（类型定义）** | **~2000+ 行** |

---

##### 2. **Watch 机制（事件驱动）**

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
|------|---------------------|-----------|
| 事件监听 | 内置 Watch API | 自己实现长轮询 |
| 本地缓存 | Indexer 自动维护 | 自己实现缓存结构 |
| 断线重连 | Reflector 自动处理 | 自己实现重试逻辑 |
| 事件过滤 | Label Selector 支持 | 自己实现过滤算法 |
| 并发控制 | 线程安全 | 自己加锁 |
| **代码行数** | **~30 行** | **~500+ 行** |

---

##### 3. **存储与序列化**

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
|------|----------------|-----------|
| 数据存储 | etcd 内置 | 自己选数据库、建表 |
| 序列化 | Protocol Buffer 自动生成 | 手动 JSON 序列化 |
| 事务支持 | etcd 原子操作 | 自己实现事务 |
| 并发控制 | MVCC 内置 | 自己加锁 |
| 数据备份 | etcd snapshot | 自己实现备份工具 |
| 数据迁移 | CRD Versioning | 手动写迁移脚本 |
| **代码行数** | **0 行（直接使用）** | **~1000+ 行** |

---

##### 4. **缓存与性能优化**

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
|------|-------------------|-----------|
| 本地缓存 | Indexer 自动维护 | 自己实现 Map 结构 |
| 索引查询 | Label Index 内置 | 自己维护索引 Map |
| 缓存同步 | Informer 自动更新 | 自己监听变化 |
| 线程安全 | 内置读写锁 | 自己加锁 |
| 内存管理 | GC 自动回收 | 自己监控内存 |
| **代码行数** | **0 行（直接使用）** | **~300+ 行** |

---

##### 5. **权限控制（RBAC）**

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
|------|----------------|-----------|
| 权限定义 | YAML 声明 | 自己设计数据结构 |
| 权限检查 | API Server 自动拦截 | 每个 Handler 手动调用 |
| 权限缓存 | 内置缓存 | 自己实现 |
| 审计日志 | Audit Policy 配置 | 自己记录日志 |
| 动态更新 | 热更新支持 | 重启或复杂同步逻辑 |
| **代码行数** | **~20 行 YAML** | **~500+ 行** |

---

##### 6. **可扩展性（Webhook/Admission Control）**

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

#### 二、Kubernetes 生态带来的红利

##### 1. **工具生态**

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

##### 2. **监控生态**

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

##### 3. **测试生态**

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

##### 4. **社区支持与人才储备**

**Kubernetes 技能通用**：

- ✅ 工程师学习 Kubernetes 后可以快速上手 Kuscia
- ✅ 遇到问题可以搜索到大量 K8s 相关资料
- ✅ 招聘容易（K8s 工程师很多）

**纯 Go 实现**：
- ❌ 需要自己编写完整文档
- ❌ 遇到问题无人可问
- ❌ 只能招聘熟悉该系统的人

---

#### 三、实际代码量对比

以实现 **DomainData 资源的 CRUD + Watch + 缓存 + 权限控制** 为例：

| 模块 | Kubernetes CRD | 纯 Go 实现 |
|------|---------------|-----------|
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

#### 四、长期维护成本对比

| 维度 | Kubernetes CRD | 纯 Go 实现 |
|------|---------------|-----------|
| **新功能开发** | 快速（复用现有框架） | 慢（需要改造基础设施） |
| **Bug 修复** | 少（K8s 已验证） | 多（自己踩坑） |
| **性能优化** | 自动享受 K8s 优化 | 自己调优 |
| **安全补丁** | K8s 团队定期发布 | 自己发现并修复 |
| **人员流动** | 新人易上手 | 老人离职后无人懂 |
| **技术债务** | 低（跟随上游） | 高（累积自定义代码） |

---

#### 五、总结

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

## Kuscia 的两种运行模式

### 模式 1：Autonomy Mode（自治模式 / Lite Mode）

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

### 模式 2：Master/Lite Mode（连接外部 K8s）

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

---

## 嵌入式 K3s 架构

### K3s 是什么？

**K3s** 是 Rancher 开发的轻量级 Kubernetes 发行版，特点：

| 特性 | 说明 |
|------|------|
| **轻量化** | 二进制文件 < 100MB |
| **单文件** | 包含所有 K8s 组件 |
| **低资源** | 最低 512MB 内存即可运行 |
| **完全兼容** | 100% 通过 K8s 一致性测试 |
| **嵌入式 etcd** | 可选内置 etcd，无需外部数据库 |

### Kuscia 如何集成 K3s？

#### 1. K3s 作为子进程启动

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

#### 2. 数据存储方式

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

#### 3. 客户端连接

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

---

## Kubernetes 配置的适用性分析

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

## 命名空间隔离机制

### 核心概念：Namespace 是逻辑隔离，不是容器隔离

很多初学者会混淆 **Kubernetes Namespace** 和 **Linux Namespace**：

| 特性 | K8s Namespace | Linux Namespace |
|------|---------------|-----------------|
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

## 容器运行时支持

### Kuscia 支持的运行时

#### 1. **RunK（Kubernetes-native Runtime）**

**原理**：使用 Kubernetes Pod 来运行任务

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaDeployment
metadata:
  name: sf-deployment
spec:
  runtime: RunK  # ← 使用 K8s Pod
  template:
    spec:
      containers:
        - name: sf-node
          image: secretflow/secretflow:latest
          command: ["python", "run.py"]
```

**需要的环境**：

```bash
# Autonomy Mode 需要安装 containerd
sudo apt-get install containerd

# 或者 Docker
sudo apt-get install docker.io
```

**适用场景**：
- ✅ 需要完整的容器隔离
- ✅ 任务需要特定的系统库
- ✅ 多版本共存（不同任务用不同镜像）

---

#### 2. **RunP（Process-based Runtime）**

**原理**：直接启动进程，不使用容器

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

---

### 对比表格

| 特性 | RunK (容器) | RunP (进程) |
|------|-------------|-------------|
| **隔离性** | 强（文件系统、网络、PID） | 弱（仅进程级） |
| **启动速度** | 慢（秒级，需拉取镜像） | 快（毫秒级） |
| **资源开销** | 高（每个容器独立 FS） | 低（共享宿主环境） |
| **依赖安装** | 需要 containerd/Docker | 只需宿主依赖 |
| **版本管理** | 容易（不同镜像标签） | 困难（需手动管理） |
| **安全性** | 高（沙箱隔离） | 低（直接访问宿主） |
| **调试难度** | 中（需进入容器） | 低（直接查看） |
| **适用场景** | 生产环境、多租户 | 开发测试、简单任务 |

---

## 实际应用场景对比

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

## 常见问题 FAQ

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
|------|------|
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
|------|-----------|----------|
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

## 总结

### 核心要点

1. ✅ **Kuscia 的 Kubernetes 配置完全不依赖容器环境**
   - 嵌入式 K3s 作为子进程运行
   - 可以在宿主机、VM、容器中运行
   - K8s API、CRD、Namespace、Informer 全部正常工作

2. ✅ **Namespace 是逻辑隔离，不是容器隔离**
   - 基于 etcd 中的字段标记
   - API Server 强制检查权限
   - 与 Linux Namespace 无关

3. ✅ **只有任务运行时才可能需要容器**
   - RunK 运行时：需要 containerd/Docker
   - RunP 运行时：不需要容器
   - 可以根据场景选择

4. ✅ **Autonomy Mode 适合大多数场景**
   - 单机部署
   - 边缘计算
   - 开发测试
   - 小规模生产

5. ✅ **Master Mode 适合大规模集群**
   - 利用现有 K8s 设施
   - 多节点调度
   - 高可用需求

### 架构图回顾

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
│  └──────────────────────────────────────┘   │
└─────────────────────────────────────────────┘

✅ 所有 Kubernetes 配置和用法都起作用
❌ 不依赖外部 Kubernetes 集群
❌ 不依赖容器运行时（除非使用 RunK）
```

希望这份文档能帮助您理解 Kuscia 中 Kubernetes 的实际作用！如有更多问题，欢迎继续提问。🎉

---

## 嵌入式 K3s 中运行的功能

### 哪些功能运行在嵌入的 K8s 中？

Kuscia 的嵌入式 K3s 不是完整的 Kubernetes，而是**精简定制版**，只保留了必要的组件。

#### 1. **启用的 K8s 核心组件**

| 组件 | 状态 | 说明 |
|------|------|------|
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
|------|---------|------|
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

## Kuscia 如何管理 K8s

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

## Kuscia 镜像体系

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
# 1. 拉取 Kuscia 镜像
docker pull secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0

# 2. 拉取引擎镜像
docker pull secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite-anolis8:1.11.0b1

# 3. 启动 Kuscia
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
# 1. 在有网络的机器上保存镜像
docker save secretflow/kuscia:1.2.0b0 -o kuscia.tar
docker save secretflow-lite-anolis8:1.11.0b1 -o secretflow.tar

# 2. 传输到离线机器
scp kuscia.tar offline-host:/tmp/
scp secretflow.tar offline-host:/tmp/

# 3. 在离线机器上加载镜像
docker load -i /tmp/kuscia.tar
docker load -i /tmp/secretflow.tar

# 4. 导入到 Kuscia
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
|------|------|----------|
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
# 1. 打标签
docker tag secretflow/kuscia:1.2.0b0 \
  harbor.example.com/library/kuscia:1.2.0b0

# 2. 登录
docker login harbor.example.com -u admin -p Harbor12345

# 3. 推送
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

### 总结

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
