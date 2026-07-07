# DomainData 详细说明文档

## 目录

- [概述](#概述)
- [数据结构定义](#数据结构定义)
- [YAML 格式示例](#yaml-格式示例)
- [存储机制](#存储机制)
- [API 接口](#api-接口)
- [数据授权 (DomainDataGrant)](#数据授权-domaindatagrant)
- [使用场景](#使用场景)

---

## 概述

DomainData 是 Kuscia 中的数据资产管理的核心概念，用于统一管理和描述域内的各类数据资源。它可以表示特征表、模型、规则、报告等数据类型。

### 主要特点

- **唯一标识**：每个 DomainData 在同一个域内具有唯一的 ID
- **类型化**：支持 table（表格）、model（模型）、rule（规则）、report（报告）等类型
- **元数据管理**：包含列信息、分区、属性等丰富的元数据
- **数据源关联**：与 DomainDataSource 关联，定位实际数据存储位置
- **访问控制**：通过 DomainDataGrant 实现细粒度的数据授权
- **持久化存储**：基于 Kubernetes CRD 和 etcd 实现高可用存储

---

## 存储机制

### 1. 存储架构

DomainData 和 DomainDataGrant 列表存储在 Kuscia 的底层存储系统中，采用以下架构：

```
┌─────────────────────────────────────────────────┐
│           Kuscia Application Layer              │
│  (DomainData API / DomainDataGrant API)         │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│      Kubernetes API Server (kube-apiserver)     │
│  - RESTful API Interface                        │
│  - Authentication & Authorization               │
│  - Validation & Admission Control               │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│         Custom Resource Definition (CRD)        │
│  - domaindatas.kuscia.secretflow                │
│  - domaindatagrants.kuscia.secretflow           │
└────────────────────┬────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────┐
│            K3s Embedded etcd                    │
│  - Distributed Key-Value Store                  │
│  - Persistent Storage                           │
│  - High Availability                            │
└─────────────────────────────────────────────────┘
```

### 2. 存储位置

#### 物理存储

- **存储引擎**：etcd v3（K3s 嵌入式版本）
- **存储路径**：`/var/lib/rancher/k3s/server/db/` （默认路径）
- **数据格式**：Protocol Buffer 序列化的 Kubernetes 对象
- **命名空间隔离**：每个域（Domain）对应一个 Kubernetes Namespace

#### Kubernetes 资源路径

```
/apis/kuscia.secretflow/v1alpha1/namespaces/{namespace}/domaindatas/{name}
/apis/kuscia.secretflow/v1alpha1/namespaces/{namespace}/domaindatagrants/{name}
```

### 3. 数据保存流程

#### DomainData 保存流程

**步骤 1: API 请求接收**

```go
// pkg/datamesh/metaserver/v1handler/httphandler/domaindata/create.go
POST /api/v1/kusciaapi/domaindata/create
```

**步骤 2: 转换为 Kubernetes CRD 对象**

```go
import (
    "github.com/secretflow/kuscia/pkg/crd/apis/kuscia/v1alpha1"
    kusciaclientset "github.com/secretflow/kuscia/pkg/crd/clientset/versioned"
)

// 创建 DomainData 对象
domainData := &v1alpha1.DomainData{
    ObjectMeta: metav1.ObjectMeta{
        Name:      request.DomaindataId,
        Namespace: request.DomainId,  // namespace = domain ID
    },
    Spec: v1alpha1.DomainDataSpec{
        RelativeURI: request.RelativeUri,
        Author:      request.Author,
        Name:        request.Name,
        Type:        request.Type,
        DataSource:  request.DatasourceId,
        Attributes:  request.Attributes,
        Columns:     convertColumns(request.Columns),
        Vendor:      request.Vendor,
        FileFormat:  request.FileFormat,
    },
}
```

**步骤 3: 通过 Kubernetes Client 写入 etcd**

```go
// pkg/crd/clientset/versioned/typed/kuscia/v1alpha1/domaindata.go
func (c *domainDatas) Create(ctx context.Context, domainData *v1alpha1.DomainData, opts v1.CreateOptions) (*v1alpha1.DomainData, error) {
    result = &v1alpha1.DomainData{}
    err = c.client.Post().
        Namespace(c.ns).           // 指定 namespace
        Resource("domaindatas").   // 指定 CRD 资源类型
        VersionedParams(&opts, scheme.ParameterCodec).
        Body(domainData).          // 序列化对象
        Do(ctx).
        Into(result)
    return result, err
}
```

**步骤 4: kube-apiserver 处理**

- 验证请求权限（RBAC）
- 执行 CRD Schema 验证
- 触发准入控制器（Admission Controllers）
- 将对象序列化并写入 etcd

**步骤 5: etcd 持久化**

- 使用 Raft 共识算法确保数据一致性
- 数据写入 WAL（Write-Ahead Log）
- 快照机制定期保存状态

#### DomainDataGrant 保存流程

DomainDataGrant 的保存流程与 `DomainData` 类似，本质上也是“保存后由控制器做一次调和（reconcile）”，但它多了一层跨域同步：当授权方域内创建或更新 `DomainDataGrant` 后，控制器会把这份授权关系镜像到被授权方对应的域中，并确保该授权关系关联的数据对象 `DomainData` 也同步过去。

**整体原理：**

1. **先在本域落库**：用户提交的 `DomainDataGrant` 先写入当前 namespace。
2. **Informer 监听事件**：`DomainDataGrant` 变更后触发 `syncDomainDataGrantHandler`，进入工作队列异步处理。
3. **按授权关系计算目标域**：
   - 一般情况下，同步目标是 `Spec.GrantDomain`。
   - 如果被授权方是 `Partner`，且其 `MasterDomain` 不为空，则实际同步目标切到 `MasterDomain`。
4. **镜像授权关系**：在目标域创建一份同名的 `DomainDataGrant`，并通过标签/注解标识它是从哪一侧同步来的。
5. **镜像数据对象**：如果目标域还没有对应的 `DomainData`，则从授权方 namespace 读取原始 `DomainData`，复制到目标域，并把来源信息、vendor 等字段补齐。
6. **幂等更新**：如果目标域已经存在同名对象，则比较 `Spec`，不一致时更新，保证最终一致性。

**为什么要这样做：**

- `DomainDataGrant` 不只是“权限记录”，它还决定了数据在跨域场景下的可见性。
- 仅保存授权关系还不够，目标域必须同时拥有可访问的 `DomainData` 副本，才能让后续消费链路正常工作。
- 通过 controller 的幂等调和，创建、更新、重复事件都能收敛到同一结果，避免因为多次写入导致状态漂移。

**对应实现（`pkg/controllers/domaindata/controller.go`）：**

```go
func (c *Controller) syncDomainDataGrantHandler(ctx context.Context, key string) (err error) {
    namespace, name, err := cache.SplitMetaNamespaceKey(key)
    if err != nil {
        return nil
    }

    // 1. 从 informer cache 中读取本地对象
    dg, err := c.domaindatagrantLister.DomainDataGrants(namespace).Get(name)
    if err != nil {
        if k8serrors.IsNotFound(err) {
            return nil
        }
        return err
    }

    // 2. 基础校验：author / grantDomain 不能为空，且不能互相相等
    if err = c.doValidate(dg); err != nil {
        return nil
    }

    // 3. 标签检查，确保对象具备后续同步所需的元数据
    update, err := c.checkLabels(dg)
    if err != nil || update {
        return err
    }

    // 4. 只有授权方 namespace 内的对象会驱动跨域同步
    if dg.Spec.Author == namespace {
        domain, err := c.kusciaClient.KusciaV1alpha1().Domains().Get(c.ctx, dg.Spec.GrantDomain, metav1.GetOptions{})
        if err != nil {
            return err
        }

        // 5. 根据被授权方角色决定最终目标域
        destDomain := dg.Spec.GrantDomain
        if domain.Spec.Role == v1alpha1.Partner && domain.Spec.MasterDomain != "" {
            destDomain = domain.Spec.MasterDomain
        }

        // 6. 目标域不存在同名 DomainDataGrant 时，先同步 DomainData，再创建授权副本
        dgGrant, err := c.domaindatagrantLister.DomainDataGrants(destDomain).Get(name)
        if err != nil {
            if k8serrors.IsNotFound(err) {
                if err = c.ensureDomainData(dg); err != nil {
                    return err
                }

                dgcopy := resources.ExtractDomainDataGrantSpec(dg)
                dgcopy.Namespace = destDomain
                dgcopy.Labels[common.LabelOwnerReferences] = dg.Name
                dgcopy.Labels[common.LabelDomainDataID] = dg.Spec.DomainDataID
                if domain.Spec.Role == v1alpha1.Partner {
                    dgcopy.Annotations[common.InitiatorAnnotationKey] = dg.Spec.Author
                    dgcopy.Annotations[common.InterConnKusciaPartyAnnotationKey] = dg.Spec.GrantDomain
                }

                _, err = c.kusciaClient.KusciaV1alpha1().DomainDataGrants(destDomain).Create(c.ctx, dgcopy, metav1.CreateOptions{})
                if err != nil {
                    return err
                }
            } else {
                return err
            }
        } else {
            // 7. 若目标域已存在副本，则做 Spec 对齐，保证最终一致性
            if _, err = c.ensureDomainDataGrantEqual(dg, dgGrant); err != nil {
                return err
            }
        }
    }

    return c.verify(dg)
}
```

**实现要点补充：**

- `doValidate` 负责最基本的合法性校验，避免无效对象进入同步逻辑。
- `checkLabels` 会修正/补齐控制器后续识别同步关系所需的标签信息。
- `ensureDomainData` 是跨域同步的核心：
  - 先判断目标域是否已经有对应 `DomainData`；
  - 没有则从授权方复制一份；
  - 复制时会设置 `LabelOwnerReferences`、`LabelDomainDataVendor=grant`，并在 `Partner` 场景下写入 `InitiatorAnnotationKey` 和 `InterConnKusciaPartyAnnotationKey`；
  - 最终把 `Spec.Vendor` 也标记为 `grant`，让下游链路知道这是授权同步来的数据。
- `ensureDomainDataGrantEqual` 用于修正目标域授权副本的 `Spec`，避免源对象和镜像对象长期不一致。
- 这套流程是**幂等**的：重复触发同一事件，只会把目标状态继续收敛到一致，不会无限创建对象。

因此，`DomainDataGrant` 的“保存”不是一次简单的 API 写入，而是“写入 + 事件驱动调和 + 跨域镜像 + 幂等对齐”的完整闭环。

#### 授权签名验证原理与流程

`DomainDataGrant` 在创建或更新时还会携带一段授权签名，用于证明这条授权关系确实由授权方域签发，而不是由其他域伪造。整个机制可以理解为“**签名生成在数据写入侧，验签发生在控制器调和侧**”：

**1. 签名是对哪些字段做的？**

在 `pkg/datamesh/metaserver/service/domaindatagrant.go` 中，服务端在把请求转换成 `DomainDataGrantSpec` 后，会先把 `Signature` 字段清空，再对整个 `Spec` 做 JSON 序列化，随后计算 `SHA256` 摘要并用授权方自己的私钥做 `RSA PKCS#1 v1.5` 签名。最终生成的签名会以 `base64` 字符串写回 `Spec.Signature`。

这样做的关键点是：

- 签名前先清空 `Signature`，避免签名内容里又包含签名本身，形成循环依赖；
- 签名覆盖的是 `DomainDataGrantSpec` 的其余字段，因此任意字段被篡改后，验签都会失败；
- 采用 `base64` 编码是为了便于在 CRD、API 和跨进程传输中保存文本值。

**2. 验签时如何定位公钥？**

控制器在 `syncDomainDataGrantHandler` 结束前会调用 `verify(dg)`。如果 `Spec.Signature` 为空，说明当前对象没有签名，控制器不会强制失败，而是继续做状态更新；如果签名不为空，则进入验签流程：

1. 先通过 `dg.Spec.Author` 找到授权方对应的 `Domain` 对象；
2. 从 `Domain.Spec.Cert` 读取证书内容；
3. 从证书中解析出 RSA 公钥；
4. 将当前 `DomainDataGrantSpec` 拷贝一份并清空 `Signature`；
5. 对该 spec 做同样的 `SHA256` 摘要；
6. 将 `Signature` 做 `base64` 解码后，使用公钥执行 `rsa.VerifyPKCS1v15`；
7. 如果任一步失败，则把该授权标记为 `GrantUnavailable`，并写入 `Verify error`。

**3. 验签失败时会发生什么？**

验签失败不会直接删除对象，而是通过 `updateStatus` 把 `DomainDataGrant.Status.Phase` 更新为不可用状态，这样上层系统可以感知到：

- 证书缺失或证书解析失败：说明无法确认签发方身份；
- 签名解码失败：说明签名数据已经损坏；
- RSA 验证失败：说明签名内容与当前 `Spec` 不匹配，可能被篡改或不是同一份授权内容。

**4. 为什么要放在控制器里验签？**

因为控制器掌握着完整的集群上下文，既能读取授权方 `Domain` 的证书，也能把验证结果落到 `Status` 中。相比在创建请求入口直接校验，控制器验签有几个好处：

- 可以和跨域同步、标签补齐、状态回写串成一条完整的调和链路；
- 可以处理“签名为空但对象需要继续调和”的兼容场景；
- 可以在授权方证书更新后重新触发验签，确保最终状态始终可信。

**5. 这条链路和保存流程的关系是什么？**

保存流程负责把授权关系和关联数据同步到目标域，验签流程负责保证这条授权关系本身的来源可信。两者合起来，构成了 `DomainDataGrant` 的完整生命周期：

- **服务端生成签名**：保证写入时内容可追溯；
- **控制器验证签名**：保证调和时内容未被篡改；
- **跨域镜像和状态回写**：保证最终状态在各域间一致且可观测。

### 4. Informer 机制

Kuscia 使用 Kubernetes Informer 机制监听数据变化：

```go
// pkg/controllers/domaindata/controller.go
func NewController(ctx context.Context, config controllers.ControllerConfig) controllers.IController {
    // 创建 SharedInformerFactory
    kusciaInformerFactory := kusciainformers.NewSharedInformerFactory(
        kusciaClient, 10*time.Minute)
    
    // 获取 DomainData 和 DomainDataGrant Informer
    domaindataInformer := kusciaInformerFactory.Kuscia().V1alpha1().DomainDatas()
    domaindataGrantInformer := kusciaInformerFactory.Kuscia().V1alpha1().DomainDataGrants()
    
    // 注册事件处理器
    domaindataGrantInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
        AddFunc: func(obj interface{}) {
            // 新增事件入队处理
            queue.EnqueueObjectWithKey(obj, controller.domainDataGrantWorkqueue)
        },
        UpdateFunc: func(oldObj, newObj interface{}) {
            // 更新事件入队处理
            queue.EnqueueObjectWithKey(newObj, controller.domainDataGrantWorkqueue)
        },
        DeleteFunc: func(obj interface{}) {
            // 删除事件特殊处理
            dd, ok := obj.(*v1alpha1.DomainDataGrant)
            if ok {
                controller.domainDataGrantDeleteWorkqueue.Add(dd.Spec.Author + "/" + dd.Name)
            }
        },
    })
    
    return controller
}
```

**Informer 工作流程：**

```
1. List & Watch → kube-apiserver
2. Delta FIFO Queue → 接收事件
3. Local Store (Indexer) → 缓存对象
4. ResourceEventHandler → 触发回调
5. Workqueue → 异步处理
6. Controller Sync → 业务逻辑
```

### 5. 数据查询机制

#### 通过 Lister 查询（从本地缓存）

```go
// 快速查询，不访问 etcd
dd, err := c.domaindataLister.DomainDatas(namespace).Get(name)
```

#### 通过 Client 查询（直接访问 etcd）

```go
// 强一致性查询，直接读取 etcd
dd, err := c.kusciaClient.KusciaV1alpha1().DomainDatas(namespace).Get(
    ctx, name, metav1.GetOptions{})
```

#### 列表查询

```go
// 支持 Label Selector 和 Field Selector
ddList, err := c.kusciaClient.KusciaV1alpha1().DomainDatas(namespace).List(
    ctx, metav1.ListOptions{
        LabelSelector: "vendor=secretflow",
        FieldSelector: "metadata.name=train-data-001",
    })
```

### 6. 跨域数据同步

当 Domain A 授权数据给 Domain B 时：

```yaml
# 原始 DomainData（Domain A）
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: train-data-001
  namespace: alice  # Domain A 的 namespace
spec:
  author: alice
  name: 训练数据
  type: table
  dataSource: alice-datasource
  relativeURI: data/train.csv
```

```yaml
# 授权记录（Domain A）
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataGrant
metadata:
  name: ddg-alice-bob-001
  namespace: alice
spec:
  author: alice
  domainDataID: train-data-001
  grantDomain: bob
```

**同步后在 Domain B 创建的资源：**

```yaml
# 同步的 DomainDataGrant（Domain B）
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataGrant
metadata:
  name: ddg-alice-bob-001
  namespace: bob  # Domain B 的 namespace
  labels:
    kuscia.secretflow/owner-references: ddg-alice-bob-001  # 关联原始授权
spec:
  author: alice
  domainDataID: train-data-001
  grantDomain: bob
```

```yaml
# 同步的 DomainData（Domain B）
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: train-data-001
  namespace: bob  # Domain B 的 namespace
  labels:
    kuscia.secretflow/owner-references: train-data-001
    kuscia.secretflow/domaindata-vendor: grant  # 标记为授权数据
spec:
  author: alice
  name: 训练数据
  type: table
  dataSource: alice-datasource  # 仍指向原始数据源
  relativeURI: data/train.csv
  vendor: grant  # 标记为授权获得
```

### 7. 数据存储特性

| 特性 | 说明 |
| ------ | ------ |
| **持久化** | etcd WAL + 快照机制，确保数据不丢失 |
| **高可用** | Raft 共识算法，支持多副本 |
| **一致性** | 强一致性保证，线性化读写 |
| **隔离性** | Kubernetes Namespace 实现多租户隔离 |
| **版本控制** | Kubernetes ResourceVersion 乐观锁 |
| **审计日志** | kube-apiserver 审计日志记录所有操作 |
| **加密存储** | 支持 etcd 静态加密（可选配置） |

### 8. 查看存储数据

#### 通过 kubectl 查看

```bash
# 查看所有 DomainData
kubectl get domaindatas -n alice

# 查看详细信息
kubectl get domaindatas train-data-001 -n alice -o yaml

# 查看 DomainDataGrant
kubectl get domaindatagrants -n alice

# 查看 etcd 中的原始数据（需要 etcdctl）
etcdctl get /registry/kuscia.secretflow/domaindatas/alice/train-data-001
```

#### 通过 API 查询

```bash
# 查询 DomainData 列表
curl http://localhost:8080/api/v1/kusciaapi/domaindata/list \
  -H "Content-Type: application/json" \
  -d '{"data": {"domain_id": "alice"}}'

# 查询单个 DomainData
curl http://localhost:8080/api/v1/kusciaapi/domaindata/query \
  -H "Content-Type: application/json" \
  -d '{"data": {"domain_id": "alice", "domaindata_id": "train-data-001"}}'
```

### 9. 性能优化

- **本地缓存**：Informer 维护本地索引，减少 etcd 访问
- **分页查询**：支持 limit/continue 分页机制
- **字段选择**：只查询需要的字段，减少网络传输
- **Watch 机制**：增量更新，避免全量轮询
- **批量操作**：支持 BatchQuery 减少 API 调用次数

### 10. K3s Embedded etcd 详解

#### 什么是 K3s？

K3s 是一个轻量级的 Kubernetes 发行版，专为边缘计算、IoT 设备和资源受限环境设计。Kuscia 选择 K3s 作为其底层容器编排平台，主要原因包括：

- **轻量化**：二进制文件小于 100MB，内存占用低于 512MB
- **简化部署**：单二进制文件包含所有组件
- **生产就绪**：完全兼容 Kubernetes API
- **低资源需求**：最低 1C2G 即可运行

#### K3s 存储后端选项

K3s 支持多种存储后端：

| 存储后端 | 适用场景 | 特点 |
| --------- | --------- | ------ |
| **Embedded etcd** | 单机或小型集群（默认） | 内置 etcd，简化部署 |
| SQLite | 开发测试 | 最轻量，无外部依赖 |
| External etcd | 大型生产集群 | 独立 etcd 集群，高可用 |
| MySQL/PostgreSQL | 企业环境 | 利用现有数据库设施 |

Kuscia 默认使用 **Embedded etcd** 作为存储后端。

#### Embedded etcd 架构

```
┌──────────────────────────────────────────────┐
│           K3s Server Process                 │
│  ┌────────────────────────────────────────┐  │
│  │     kube-apiserver                     │  │
│  └──────────────┬─────────────────────────┘  │
│                 │                             │
│  ┌──────────────▼─────────────────────────┐  │
│  │     Embedded etcd (v3)                 │  │
│  │  ┌──────────────────────────────────┐  │  │
│  │  │  Raft Consensus Engine           │  │  │
│  │  │  - Leader Election               │  │  │
│  │  │  - Log Replication               │  │  │
│  │  │  - Snapshot Management           │  │  │
│  │  └──────────────────────────────────┘  │  │
│  └────────────────────────────────────────┘  │
└──────────────────────────────────────────────┘
           │
           ▼
┌──────────────────────────────────────────────┐
│         Persistent Storage Layer             │
│  ┌────────────────────────────────────────┐  │
│  │  Write-Ahead Log (WAL)                 │  │
│  │  - snap/db                           │  │  │
│  │  - member/snap/db                    │  │  │
│  └────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────┐  │
│  │  BoltDB (B+ Tree)                      │  │
│  │  - Key-Value Index                     │  │
│  └────────────────────────────────────────┘  │
└──────────────────────────────────────────────┘
```

#### etcd v3 核心特性

**1. Raft 共识算法**

- **Leader-based**：所有写操作通过 Leader 节点
- **多数派确认**：需要 (N/2 + 1) 个节点确认
- **日志复制**：确保所有节点数据一致
- **故障恢复**：自动选举新 Leader

**2. MVCC（多版本并发控制）**

```go
// etcd 中每个 key 维护多个版本
type KeyValue struct {
    Key            string    // key 名称
    CreateRevision int64     // 创建时的 revision
    ModRevision    int64     // 最后修改的 revision
    Version        int64     // 修改次数
    Value          []byte    // 值
    Lease          int64     // 租约 ID
}
```

**3. Revision 机制**

- 每次事务递增全局 revision
- 支持历史版本查询
- 实现乐观锁（通过 ResourceVersion）

**4. Watch 机制**

- 实时监听 key 变化
- 支持前缀匹配
- 事件压缩和合并

#### Kuscia 中 etcd 的配置

**启动参数**（来自 `cmd/kuscia/modules/k3s.go`）：

```bash
k3s server \
  -d=/var/lib/rancher/k3s/server \      # 数据目录
  --datastore-endpoint=etcd \            # 使用嵌入式 etcd
  --token=<cluster-token> \              # 集群令牌
  --kube-apiserver-arg=event-ttl=10m \   # 事件保留时间
  --disable-agent \                      # 禁用 agent
  --bind-address=0.0.0.0 \               # 绑定地址
  --https-listen-port=6443               # API Server 端口
```

**数据存储结构**：

```
/var/lib/rancher/k3s/server/
├── db/
│   ├── snap/                    # 快照目录
│   │   └── db                   # etcd 数据库文件 (BoltDB)
│   └── wal/                     # WAL 日志目录
│       ├── 0000000000000000-0000000000000000.wal
│       └── ...
├── tls/                         # TLS 证书
│   ├── server-ca.crt
│   ├── server-ca.key
│   ├── client-admin.crt
│   └── client-admin.key
└── token                        # 集群令牌
```

#### etcd 数据持久化机制

**1. Write-Ahead Log (WAL)**

- 所有写操作先写入 WAL
- 确保崩溃后可恢复
- WAL 文件定期归档

**2. Snapshot 快照**

```go
// etcd 定期创建快照
Snapshot Configuration:
  - snapshot-count: 10000        // 每 10000 次操作创建快照
  - snapshot-catchup-entries: 5000
  - max-snapshots: 5             // 保留最近 5 个快照
```

**3. Compaction 压缩**

- 定期清理旧版本数据
- 保留最近的 revision
- 可通过 Kubernetes API 配置

```bash
# 手动触发 etcd 压缩
etcdctl compact <revision>

# 查看当前 revision
etcdctl endpoint status --write-out="json"
```

#### 数据安全与备份

**1. 加密配置**（可选）

```yaml
# /etc/rancher/k3s/encryption-config.yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
      - domaindatas.kuscia.secretflow
      - domaindatagrants.kuscia.secretflow
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: <base64-encoded-secret>
      - identity: {}
```

**2. 备份策略**

```bash
# 备份 etcd 数据库
kubectl k3s etcd-snapshot save \
  --name=backup-$(date +%Y%m%d) \
  --snapshot-dir=/var/lib/rancher/k3s/server/db/snapshots

# 从备份恢复
kubectl k3s etcd-snapshot restore \
  --name=backup-20240101
```

**3. 监控指标**

```bash
# 查看 etcd 健康状态
etcdctl endpoint health

# 查看集群成员
etcdctl member list

# 查看数据库大小
etcdctl endpoint status --write-out=table
```

#### etcd 性能调优

**关键指标监控**：

- `etcd_server_has_leader`：是否有 Leader
- `etcd_server_leader_changes_seen_total`：Leader 变更次数
- `etcd_disk_backend_commit_duration_seconds`：提交延迟
- `etcd_network_peer_round_trip_time_seconds`：节点间 RTT

**调优建议**：

```yaml
# K3s etcd 调优参数
--etcd-expose-metrics=true                  # 暴露监控指标
--etcd-heartbeat-interval=100               # 心跳间隔 (ms)
--etcd-election-timeout=1000                # 选举超时 (ms)
--etcd-quota-backend-bytes=8589934592       # 后端配额 (8GB)
--etcd-max-request-bytes=1572864            # 最大请求 (1.5MB)
--etcd-auto-compaction-mode=periodic        # 自动压缩模式
--etcd-auto-compaction-retention=1h         # 保留 1 小时历史
```

#### 故障排查

**常见问题**：

1. **etcd 空间不足**

```bash
# 错误: "etcdserver: mvcc: database space exceeded"
# 解决：触发压缩和碎片整理
etcdctl compact $(etcdctl endpoint status --print-value-only | jq -r '.[].revision')
etcdctl defrag
```

2. **Leader 选举失败**

```bash
# 检查网络连接
etcdctl endpoint status --cluster -w table

# 检查成员健康
etcdctl endpoint health --cluster
```

3. **数据损坏恢复**

```bash
# 从快照恢复
k3s server \
  --cluster-reset \
  --cluster-reset-restore-path=/path/to/snapshot
```

#### Kuscia 中的实际应用

在 Kuscia 隐私计算场景中，etcd 存储的关键数据：

| 数据类型 | 存储路径示例 | 大小估算 | 更新频率 |
| --------- | ------------ | --------- | --------- |
| DomainData | `/registry/kuscia.secretflow/domaindatas/alice/train-001` | ~5KB | 低频 |
| DomainDataGrant | `/registry/kuscia.secretflow/domaindatagrants/alice/grant-001` | ~3KB | 中频 |
| Domain | `/registry/kuscia.secretflow/domains/alice` | ~2KB | 极低频 |
| KusciaJob | `/registry/kuscia.secretflow/kusciajobs/alice/job-001` | ~10KB | 高频 |
| KusciaTask | `/registry/kuscia.secretflow/kusciatasks/alice/task-001` | ~8KB | 高频 |

**容量规划**：

- 单个 DomainData 对象：~5KB
- 单个 DomainDataGrant 对象：~3KB
- 假设 1000 个数据资产 + 5000 个授权记录
- 总存储需求：(1000 × 5KB) + (5000 × 3KB) ≈ 20MB
- 考虑历史版本和 overhead：~100MB
- etcd 默认配额：8GB（充足）

---

## 数据结构定义

### 1. CRD 结构 (Kubernetes Custom Resource)

#### DomainData Spec 字段说明

| 字段名 | 类型 | 必填 | 说明 |
| -------- | ------ | ------ | ------ |
| `relativeURI` | string | 是 | 相对于数据源的 URI 路径。完整路径 = DataSourceURI + RelativeURI |
| `author` | string | 是 | 数据创建者/作者 |
| `name` | string | 是 | 人类可读的名称，可以重复 |
| `type` | string | 是 | 数据类型：table, model, rule, report |
| `dataSource` | string | 是 | 数据源 ID，引用 DomainDataSource |
| `attributes` | map[string]string | 否 | 扩展属性，用户自定义的键值对 |
| `partitions` | Partition | 否 | 分区信息（目前不支持） |
| `columns` | []DataColumn | 否 | 列定义，当 type 为 table 时必须提供 |
| `vendor` | string | 否 | 数据提供方：manual（手动）, secretflow, 或其他厂商 |
| `fileFormat` | string | 否 | 文件格式，仅适用于 localfs 或 oss 数据源，默认为 csv |

#### DataColumn 字段说明

| 字段名 | 类型 | 必填 | 说明 |
| -------- | ------ | ------ | ------ |
| `name` | string | 是 | 列名 |
| `type` | string | 是 | 列数据类型（如：string, int, float, double 等） |
| `comment` | string | 否 | 列注释说明 |
| `notNullable` | bool | 否 | 是否非空约束，默认 false |

#### Partition 字段说明

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `type` | string | 是 | 分区类型：path, odps |
| `fields` | []DataColumn | 否 | 分区字段列表 |

**分区类型详解**：

##### 1. Path 分区（文件系统分区）

基于文件目录结构的分区方式，适用于本地文件系统、OSS 等存储。

**数据结构**：

```go
type Partition struct {
    Type   string       `json:"type"`   // "path"
    Fields []DataColumn `json:"fields"` // 分区字段定义
}
```

**实际示例**：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: user-behavior-data
  namespace: alice
spec:
  relativeURI: "data/users"  # 基础路径
  author: "alice"
  name: "用户行为数据"
  type: "table"
  dataSource: "datasource-localfs-001"
  vendor: "secretflow"
  
  # Path 分区配置
  partitions:
    type: "path"  # 基于路径的分区
    fields:
      - name: "dt"         # 日期分区字段
        type: "string"
        comment: "日期分区，格式：YYYY-MM-DD"
      - name: "region"     # 地区分区字段
        type: "string"
        comment: "地区分区：cn/shanghai, cn/beijing, etc."
  
  columns:
    - name: "user_id"
      type: "string"
      comment: "用户ID"
    - name: "action_type"
      type: "string"
      comment: "行为类型：click/view/purchase"
    - name: "timestamp"
      type: "int64"
      comment: "时间戳"
```

**对应的文件系统结构**：

```
/data/users/
├── dt=2024-01-01/
│   ├── region=cn_shanghai/
│   │   ├── part-00000.csv
│   │   └── part-00001.csv
│   └── region=cn_beijing/
│       ├── part-00000.csv
│       └── part-00001.csv
├── dt=2024-01-02/
│   ├── region=cn_shanghai/
│   │   └── part-00000.csv
│   └── region=cn_beijing/
│       └── part-00000.csv
└── dt=2024-01-03/
    └── region=cn_shanghai/
        └── part-00000.csv
```

**使用场景**：

- **时间序列数据**：按日期/小时分区
- **多租户数据**：按租户 ID 分区
- **地域数据**：按地区/国家分区
- **业务分类**：按业务线/产品线分区

**查询优化**：

```python
# 隐私计算任务中可以指定分区过滤，减少数据传输
{
    "domaindata_id": "user-behavior-data",
    "partition_filter": {
        "dt": "2024-01-01",      # 只读取特定日期
        "region": "cn_shanghai"  # 只读取特定地区
    }
}
# 实际读取路径：/data/users/dt=2024-01-01/region=cn_shanghai/
```

##### 2. ODPS 分区（MaxCompute 分区）

阿里云 ODPS（Open Data Processing Service，现称 MaxCompute）的分区表结构。

**ODPS 分区特点**：

- 托管在阿里云 MaxCompute 平台
- 支持海量数据（PB 级别）
- 分区作为表的一级结构
- 支持动态分区和静态分区

**配置示例**：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: odps-order-table
  namespace: alice
spec:
  relativeURI: "tables/orders"  # ODPS 表名
  author: "alice"
  name: "ODPS订单表"
  type: "table"
  dataSource: "datasource-odps-001"  # ODPS 数据源
  vendor: "manual"
  
  # ODPS 分区配置
  partitions:
    type: "odps"  # ODPS 分区类型
    fields:
      - name: "ds"          # ODPS 常用日期分区
        type: "string"
        comment: "日期分区，格式：YYYYMMDD"
      - name: "city_code"   # 城市代码分区
        type: "string"
        comment: "城市代码：330100(杭州), 110100(北京)"
  
  columns:
    - name: "order_id"
      type: "string"
      comment: "订单ID"
      notNullable: true
    - name: "user_id"
      type: "string"
      comment: "用户ID"
    - name: "amount"
      type: "double"
      comment: "订单金额"
    - name: "status"
      type: "string"
      comment: "订单状态：pending/paid/shipped"
```

**ODPS SQL 对应**：

```sql
-- 创建分区表
CREATE TABLE orders (
    order_id STRING COMMENT '订单ID',
    user_id STRING COMMENT '用户ID',
    amount DOUBLE COMMENT '订单金额',
    status STRING COMMENT '订单状态'
)
PARTITIONED BY (
    ds STRING COMMENT '日期分区',
    city_code STRING COMMENT '城市分区'
);

-- 添加分区
ALTER TABLE orders ADD PARTITION (ds='20240101', city_code='330100');
ALTER TABLE orders ADD PARTITION (ds='20240101', city_code='110100');

-- 查询特定分区
SELECT * FROM orders WHERE ds='20240101' AND city_code='330100';
```

**Kuscia 中的使用**：

```python
# 在隐私计算任务中引用 ODPS 分区数据
from secretflow.data.vertical import read_csv

# Kuscia 会自动处理 ODPS 分区信息
data = read_csv(
    domaindata_id='odps-order-table',
    partition_spec='ds=20240101,city_code=330100'  # 指定分区
)
```

#### 分区 vs 非分区对比

| 特性 | 非分区表 | 分区表 |
| ------ | --------- | -------- |
| **数据存储** | 单一文件/表 | 多个分区目录/子表 |
| **查询性能** | 全表扫描 | 分区裁剪，快速定位 |
| **数据管理** | 整体管理 | 可按分区删除/更新 |
| **适用场景** | 小数据量 | 大数据量、时间序列 |
| **存储成本** | 固定 | 可按分区设置生命周期 |
| **并发访问** | 可能冲突 | 分区隔离，高并发 |

#### 分区最佳实践

**1. 选择合适的分区字段**

```yaml
# ✅ 好的分区设计
partitions:
  type: "path"
  fields:
    - name: "dt"           # 高频查询字段
      type: "string"
    - name: "business_line" # 业务隔离字段
      type: "string"

# ❌ 避免的分区设计
partitions:
  type: "path"
  fields:
    - name: "user_id"      # 基数过大，产生过多分区
      type: "string"
    - name: "timestamp"    # 连续值，不适合分区
      type: "int64"
```

**2. 控制分区数量**

- 单个表的分区数建议 < 10,000
- 避免分区过小（< 1MB）
- 定期合并小分区

**3. 分区命名规范**

```yaml
# 推荐格式
partitions:
  fields:
    - name: "dt"            # 日期：YYYY-MM-DD 或 YYYYMMDD
      type: "string"
    - name: "hour"          # 小时：HH (00-23)
      type: "string"
    - name: "region"        # 地区：使用下划线分隔
      type: "string"
    - name: "version"       # 版本：v1, v2
      type: "string"
```

**4. 分区生命周期管理**

```yaml
# 通过 attributes 标注分区策略
attributes:
  partition.retention.days: "90"        # 保留 90 天
  partition.auto.cleanup: "true"        # 自动清理
  partition.hot.days: "7"               # 热数据 7 天
  partition.storage.tier: "SSD"         # 存储层级
```

---

### 2. Proto API 结构

#### CreateDomainDataRequest

```protobuf
message CreateDomainDataRequest {
  RequestHeader header = 1;
  string domaindata_id = 2;        // 可选，为空时由服务端生成
  string name = 3;                  // 人类可读名称
  string type = 4;                  // 数据类型
  string relative_uri = 5;          // 相对 URI
  string datasource_id = 6;         // 可选，默认使用默认数据源
  map<string, string> attributes = 7;  // 扩展属性
  Partition partition = 8;          // 分区信息
  repeated DataColumn columns = 9;  // 列定义
  string vendor = 10;               // 数据提供方
  FileFormat file_format = 11;      // 文件格式
}
```

#### DomainData 响应结构

```protobuf
message DomainData {
  string domaindata_id = 1;         // 唯一标识
  string name = 2;                  // 名称
  string type = 3;                  // 类型
  string relative_uri = 4;          // 相对 URI
  string datasource_id = 5;         // 数据源 ID
  map<string, string> attributes = 6;  // 扩展属性
  Partition partition = 7;          // 分区信息
  repeated DataColumn columns = 8;  // 列定义
  string vendor = 9;                // 数据提供方
  FileFormat file_format = 10;      // 文件格式
  string author = 11;               // 作者
}
```

---

## YAML 格式示例

### 1. 表格类型 DomainData

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: train-table-sample
  namespace: alice
  labels:
    app: secretflow
spec:
  # 相对于数据源的路径
  relativeURI: "train/table.csv"
  # 数据作者
  author: "alice@secretflow.com"
  # 显示名称
  name: "训练数据集"
  # 数据类型：table/model/rule/report
  type: "table"
  # 数据源 ID
  dataSource: "datasource-localfs-001"
  # 文件格式：csv/parquet/json 等
  fileFormat: "csv"
  # 数据提供方
  vendor: "secretflow"
  # 列定义（表格类型必须）
  columns:
    - name: "id"
      type: "string"
      comment: "用户ID"
      notNullable: true
    - name: "age"
      type: "int"
      comment: "年龄"
      notNullable: false
    - name: "income"
      type: "double"
      comment: "收入"
      notNullable: false
    - name: "city"
      type: "string"
      comment: "城市"
      notNullable: false
  # 扩展属性
  attributes:
    description: "用户训练数据集"
    version: "1.0"
    tags: "train,user,sample"
status:
  phase: Available
  conditions:
    - status: "True"
      reason: "Registered"
      message: "DomainData registered successfully"
      lastUpdateTime: "2024-01-01T00:00:00Z"
```

### 2. 模型类型 DomainData

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: lr-model-v1
  namespace: alice
spec:
  relativeURI: "models/lr_model_v1.pkl"
  author: "alice@secretflow.com"
  name: "逻辑回归模型V1"
  type: "model"
  dataSource: "datasource-oss-001"
  vendor: "secretflow"
  attributes:
    algorithm: "logistic_regression"
    accuracy: "0.95"
    training_samples: "10000"
    model_version: "1.0"
status:
  phase: Available
  conditions:
    - status: "True"
      reason: "Registered"
      message: "Model registered successfully"
      lastUpdateTime: "2024-01-01T00:00:00Z"
```

### 3. 带分区的 DomainData

#### 示例 1: Path 分区（本地文件系统）

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: user-behavior-partitioned
  namespace: alice
spec:
  # 基础路径，分区会附加在此路径后
  relativeURI: "data/users/behavior"
  author: "alice@secretflow.com"
  name: "用户行为数据(分区表)"
  type: "table"
  dataSource: "datasource-localfs-001"
  vendor: "secretflow"
  fileFormat: "parquet"  # 使用列式存储格式
  
  # Path 分区定义
  partitions:
    type: "path"  # 基于文件路径的分区
    fields:
      - name: "dt"         # 一级分区：日期
        type: "string"
        comment: "日期分区，格式：YYYY-MM-DD"
      - name: "region"     # 二级分区：地区
        type: "string"
        comment: "地区分区：cn_shanghai, cn_beijing, etc."
  
  # 表的列定义（不包含分区字段）
  columns:
    - name: "user_id"
      type: "string"
      comment: "用户ID"
      notNullable: true
    - name: "action_type"
      type: "string"
      comment: "行为类型：click/view/purchase/share"
      notNullable: true
    - name: "item_id"
      type: "string"
      comment: "物品ID"
    - name: "duration"
      type: "int"
      comment: "停留时长（秒）"
    - name: "timestamp"
      type: "int64"
      comment: "行为时间戳"
  
  # 扩展属性
  attributes:
    description: "用户行为日志数据，按日期和地区分区"
    partition.count: "365"           # 预计分区数量
    partition.strategy: "date+region" # 分区策略
    data.retention.days: "90"        # 数据保留 90 天
    storage.format: "parquet"        # 存储格式
    compression.type: "snappy"       # 压缩算法

status:
  phase: Available
  conditions:
    - status: "True"
      reason: "Registered"
      message: "Partitioned table registered successfully"
      lastUpdateTime: "2024-01-01T00:00:00Z"
```

**对应的文件系统结构**：

```
/data/users/behavior/
├── dt=2024-01-01/
│   ├── region=cn_shanghai/
│   │   ├── part-00000.parquet
│   │   ├── part-00001.parquet
│   │   └── _SUCCESS
│   ├── region=cn_beijing/
│   │   ├── part-00000.parquet
│   │   └── _SUCCESS
│   └── region=cn_guangzhou/
│       ├── part-00000.parquet
│       └── _SUCCESS
├── dt=2024-01-02/
│   ├── region=cn_shanghai/
│   │   └── part-00000.parquet
│   └── region=cn_beijing/
│       └── part-00000.parquet
└── dt=2024-01-03/
    └── region=cn_shanghai/
        └── part-00000.parquet
```

**隐私计算任务中使用分区数据**：

```python
import secretflow as sf

# 创建 SPU 设备
spu = sf.SPU('spu_config.json')

# 读取特定分区的数据（分区裁剪优化）
data = sf.data.read_csv(
    domaindata_id='user-behavior-partitioned',
    # 指定分区过滤条件
    partitions={
        'dt': ['2024-01-01', '2024-01-02'],  # 只读取这两天
        'region': ['cn_shanghai']              # 只读取上海地区
    }
)

# 实际读取的路径：
# /data/users/behavior/dt=2024-01-01/region=cn_shanghai/
# /data/users/behavior/dt=2024-01-02/region=cn_shanghai/

# 执行 PSI 联合统计
result = sf.stats.count(data, by=['action_type'])
print(result)
```

#### 示例 2: ODPS 分区（MaxCompute）

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: odps-transaction-partitioned
  namespace: alice
spec:
  # ODPS 表名
  relativeURI: "tables.transactions"
  author: "alice@secretflow.com"
  name: "ODPS交易记录表(分区)"
  type: "table"
  dataSource: "datasource-odps-001"  # ODPS 数据源配置
  vendor: "manual"
  
  # ODPS 分区定义
  partitions:
    type: "odps"  # ODPS 分区类型
    fields:
      - name: "ds"          # ODPS 标准日期分区
        type: "string"
        comment: "日期分区，格式：YYYYMMDD"
      - name: "city_code"   # 城市代码分区
        type: "string"
        comment: "城市代码：330100(杭州), 110100(北京), 310100(上海)"
  
  # 表列定义
  columns:
    - name: "transaction_id"
      type: "string"
      comment: "交易ID"
      notNullable: true
    - name: "user_id"
      type: "string"
      comment: "用户ID"
      notNullable: true
    - name: "merchant_id"
      type: "string"
      comment: "商户ID"
    - name: "amount"
      type: "double"
      comment: "交易金额（元）"
    - name: "currency"
      type: "string"
      comment: "货币类型：CNY/USD/EUR"
    - name: "status"
      type: "string"
      comment: "交易状态：success/failed/refunded"
    - name: "payment_method"
      type: "string"
      comment: "支付方式：alipay/wechat/card"
    - name: "create_time"
      type: "datetime"
      comment: "创建时间"
  
  # 扩展属性
  attributes:
    description: "ODPS 交易记录表，支持海量数据分析"
    odps.project: "alice_project"         # ODPS 项目空间
    odps.lifecycle: "365"                  # 数据生命周期 365 天
    odps.storage.tier: "standard"          # 存储类型
    partition.prune.enable: "true"         # 启用分区裁剪
    sample.partition.ds: "20240101"        # 示例分区值

status:
  phase: Available
```

**ODPS SQL 对应操作**：

```sql
-- 在 ODPS 中创建分区表
CREATE TABLE IF NOT EXISTS alice_project.transactions (
    transaction_id STRING COMMENT '交易ID',
    user_id STRING COMMENT '用户ID',
    merchant_id STRING COMMENT '商户ID',
    amount DOUBLE COMMENT '交易金额',
    currency STRING COMMENT '货币类型',
    status STRING COMMENT '交易状态',
    payment_method STRING COMMENT '支付方式',
    create_time DATETIME COMMENT '创建时间'
)
PARTITIONED BY (
    ds STRING COMMENT '日期分区',
    city_code STRING COMMENT '城市代码'
)
LIFECYCLE 365;

-- 添加分区
ALTER TABLE alice_project.transactions ADD PARTITION (ds='20240101', city_code='330100');
ALTER TABLE alice_project.transactions ADD PARTITION (ds='20240101', city_code='110100');
ALTER TABLE alice_project.transactions ADD PARTITION (ds='20240102', city_code='330100');

-- 导入数据到分区
INSERT OVERWRITE TABLE alice_project.transactions PARTITION (ds='20240101', city_code='330100')
SELECT 
    transaction_id, user_id, merchant_id, amount, 
    currency, status, payment_method, create_time
FROM source_table
WHERE ds='20240101' AND city_code='330100';

-- 查询特定分区（分区裁剪）
SELECT COUNT(*) as cnt, status
FROM alice_project.transactions
WHERE ds='20240101' AND city_code='330100'
GROUP BY status;

-- 删除过期分区
ALTER TABLE alice_project.transactions DROP PARTITION (ds='20230101');
```

**在 Kuscia 隐私计算中使用**：

```python
import secretflow as sf
from secretflow.data.vertical import read_odps_table

# 配置 ODPS 连接
odps_config = {
    'access_id': 'your_access_id',
    'access_key': 'your_access_key',
    'endpoint': 'http://service.cn-hangzhou.maxcompute.aliyun.com/api',
    'project': 'alice_project'
}

# 读取 ODPS 分区表（Kuscia 自动处理分区信息）
txn_data = read_odps_table(
    domaindata_id='odps-transaction-partitioned',
    partition_spec='ds=20240101,city_code=330100',  # 指定分区
    odps_config=odps_config
)

# 与另一方数据进行联合统计
bob_data = sf.data.read_csv(domaindata_id='bob-merchant-data')

# 执行 PSI + 统计分析
result = sf.stats.describe(
    sf.data.concat([txn_data, bob_data], axis=1),
    columns=['amount', 'user_id']
)

print(f"平均交易金额: {result['amount']['mean']}")
print(f"交易笔数: {result['user_id']['count']}")
```

#### 示例 3: 多级分区（复杂场景）

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: iot-sensor-data-multi-partition
  namespace: alice
spec:
  relativeURI: "iot/sensors"
  author: "alice@secretflow.com"
  name: "IoT传感器数据(多级分区)"
  type: "table"
  dataSource: "datasource-oss-001"
  vendor: "secretflow"
  fileFormat: "csv"
  
  # 三级分区：日期 -> 小时 -> 设备类型
  partitions:
    type: "path"
    fields:
      - name: "dt"           # 一级分区：日期
        type: "string"
        comment: "日期：YYYY-MM-DD"
      - name: "hour"         # 二级分区：小时
        type: "string"
        comment: "小时：00-23"
      - name: "device_type"  # 三级分区：设备类型
        type: "string"
        comment: "设备类型：temperature/humidity/pressure"
  
  columns:
    - name: "device_id"
      type: "string"
      comment: "设备ID"
      notNullable: true
    - name: "sensor_value"
      type: "double"
      comment: "传感器读数"
    - name: "quality"
      type: "string"
      comment: "数据质量：good/fair/poor"
    - name: "battery_level"
      type: "int"
      comment: "电池电量百分比"
  
  attributes:
    description: "IoT 传感器数据，三级分区优化查询"
    partition.levels: "3"
    partition.hot.hours: "24"            # 最近 24 小时为热数据
    iot.device.count: "10000"            # 设备数量
    data.frequency: "1min"               # 数据采集频率

status:
  phase: Available
```

**文件系统结构**：

```
iot/sensors/
├── dt=2024-01-01/
│   ├── hour=00/
│   │   ├── device_type=temperature/
│   │   │   └── sensors_0000.csv
│   │   ├── device_type=humidity/
│   │   │   └── sensors_0000.csv
│   │   └── device_type=pressure/
│   │       └── sensors_0000.csv
│   ├── hour=01/
│   │   └── ...
│   └── hour=23/
│       └── ...
└── dt=2024-01-02/
    └── ...
```

---

## API 接口

DomainData 提供了完整的 RESTful API 和 gRPC 接口，支持数据的注册、查询、更新和删除。

### 1. 注册 DomainData (Create)

#### HTTP API

```
POST /api/v1/kusciaapi/domaindata/create
```

#### gRPC API

```protobuf
rpc CreateDomainData(CreateDomainDataRequest) returns (CreateDomainDataResponse);
```

#### 请求示例 (curl)

```bash
curl -X POST http://localhost:8080/api/v1/kusciaapi/domaindata/create \
  -H "Content-Type: application/json" \
  -d '{
    "header": {
      "domain_id": "alice"
    },
    "domaindata_id": "train-data-001",
    "name": "训练数据集",
    "type": "table",
    "relative_uri": "train/table.csv",
    "datasource_id": "datasource-localfs-001",
    "columns": [
      {
        "name": "id",
        "type": "string",
        "comment": "用户ID",
        "notNullable": true
      },
      {
        "name": "age",
        "type": "int",
        "comment": "年龄"
      }
    ],
    "vendor": "secretflow",
    "file_format": "CSV",
    "attributes": {
      "description": "用户训练数据",
      "version": "1.0"
    }
  }'
```

#### 响应示例

```json
{
  "status": {
    "code": 0,
    "message": "success"
  },
  "data": {
    "domaindata_id": "train-data-001"
  }
}
```

### 2. 查询 DomainData (Query)

#### 单个查询

**HTTP API**

```
POST /api/v1/kusciaapi/domaindata/query
```

**请求示例**

```bash
curl -X POST http://localhost:8080/api/v1/kusciaapi/domaindata/query \
  -H "Content-Type: application/json" \
  -d '{
    "header": {
      "domain_id": "alice"
    },
    "data": {
      "domain_id": "alice",
      "domaindata_id": "train-data-001"
    }
  }'
```

**响应示例**

```json
{
  "status": {
    "code": 0,
    "message": "success"
  },
  "data": {
    "domaindata_id": "train-data-001",
    "name": "训练数据集",
    "type": "table",
    "relative_uri": "train/table.csv",
    "datasource_id": "datasource-localfs-001",
    "author": "alice@secretflow.com",
    "vendor": "secretflow",
    "file_format": "CSV",
    "columns": [
      {
        "name": "id",
        "type": "string",
        "comment": "用户ID",
        "notNullable": true
      },
      {
        "name": "age",
        "type": "int",
        "comment": "年龄"
      }
    ],
    "attributes": {
      "description": "用户训练数据",
      "version": "1.0"
    }
  }
}
```

#### 批量查询

**HTTP API**

```
POST /api/v1/kusciaapi/domaindata/batchquery
```

**请求示例**

```bash
curl -X POST http://localhost:8080/api/v1/kusciaapi/domaindata/batchquery \
  -H "Content-Type: application/json" \
  -d '{
    "header": {
      "domain_id": "alice"
    },
    "data": [
      {
        "domain_id": "alice",
        "domaindata_id": "train-data-001"
      },
      {
        "domain_id": "alice",
        "domaindata_id": "train-data-002"
      }
    ]
  }'
```

### 3. 列出 DomainData (List)

#### HTTP API

```
POST /api/v1/kusciaapi/domaindata/list
```

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/v1/kusciaapi/domaindata/list \
  -H "Content-Type: application/json" \
  -d '{
    "header": {
      "domain_id": "alice"
    },
    "data": {
      "domain_id": "alice",
      "domaindata_type": "table",
      "page_no": 1,
      "page_size": 10
    }
  }'
```

#### 响应示例

```json
{
  "status": {
    "code": 0,
    "message": "success"
  },
  "data": {
    "items": [
      {
        "domaindata_id": "train-data-001",
        "name": "训练数据集",
        "type": "table",
        "relative_uri": "train/table.csv",
        "datasource_id": "datasource-localfs-001",
        "author": "alice@secretflow.com"
      }
    ],
    "total_count": 1
  }
}
```

### 4. 更新 DomainData (Update)

#### HTTP API

```
POST /api/v1/kusciaapi/domaindata/update
```

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/v1/kusciaapi/domaindata/update \
  -H "Content-Type: application/json" \
  -d '{
    "header": {
      "domain_id": "alice"
    },
    "domaindata_id": "train-data-001",
    "domain_id": "alice",
    "name": "训练数据集V2",
    "attributes": {
      "description": "更新后的训练数据",
      "version": "2.0"
    }
  }'
```

**注意**：未设置的字段不会被更新，保持原值。

### 5. 删除 DomainData (Delete)

#### HTTP API

```
POST /api/v1/kusciaapi/domaindata/delete
```

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/v1/kusciaapi/domaindata/delete \
  -H "Content-Type: application/json" \
  -d '{
    "header": {
      "domain_id": "alice"
    },
    "domain_id": "alice",
    "domaindata_id": "train-data-001"
  }'
```

#### 响应示例

```json
{
  "status": {
    "code": 0,
    "message": "success"
  }
}
```

### 6. DataMesh gRPC 接口

除了 KusciaAPI，还提供了 DataMesh 服务的 gRPC 接口：

```protobuf
// DataMesh DomainData 服务
service DomainDataService {
  rpc CreateDomainData(CreateDomainDataRequest) returns (CreateDomainDataResponse);
  rpc UpdateDomainData(UpdateDomainDataRequest) returns (UpdateDomainDataResponse);
  rpc DeleteDomainData(DeleteDomainDataRequest) returns (DeleteDomainDataResponse);
  rpc QueryDomainData(QueryDomainDataRequest) returns (QueryDomainDataResponse);
}
```

**使用示例 (Go)**

```go
import (
    "context"
    "github.com/secretflow/kuscia/proto/api/v1alpha1/datamesh"
)

// 创建客户端连接
conn, _ := grpc.Dial("localhost:8082", grpc.WithInsecure())
client := datamesh.NewDomainDataServiceClient(conn)

// 创建 DomainData
req := &datamesh.CreateDomainDataRequest{
    Header: &v1alpha1.RequestHeader{
        DomainId: "alice",
    },
    DomaindataId: "train-data-001",
    Name:         "训练数据集",
    Type:         "table",
    RelativeUri:  "train/table.csv",
    DatasourceId: "datasource-localfs-001",
    Columns: []*v1alpha1.DataColumn{
        {
            Name: "id",
            Type: "string",
        },
    },
}

resp, err := client.CreateDomainData(context.Background(), req)
if err != nil {
    log.Fatalf("CreateDomainData failed: %v", err)
}
log.Printf("Created domaindata: %s", resp.GetData().GetDomaindataId())
```

---

## 数据授权 (DomainDataGrant)

DomainDataGrant 用于实现跨域的数据授权，允许一个域（授权方）将数据访问权限授予另一个域（被授权方）。

### 1. 数据结构

#### DomainDataGrant Spec 字段说明

| 字段名 | 类型 | 必填 | 说明 |
| -------- | ------ | ------ | ------ |
| `author` | string | 是 | 授权创建者 |
| `domainDataID` | string | 是 | 被授权的 DomainData ID |
| `grantDomain` | string | 是 | 被授权的域 ID |
| `signature` | string | 否 | 签名验证 |
| `limit` | GrantLimit | 否 | 授权限制条件 |
| `description` | map[string]string | 否 | 描述信息 |

#### GrantLimit 字段说明

| 字段名 | 类型 | 必填 | 说明 |
| -------- | ------ | ------ | ------ |
| `expirationTime` | Time | 否 | 过期时间 |
| `useCount` | int | 否 | 使用次数限制，0 表示无限制 |
| `grantMode` | []GrantType | 否 | 授权模式：normal, metadata, file |
| `flowID` | string | 否 | 关联的流程 ID |
| `components` | []string | 否 | 允许的组件列表 |
| `initiator` | string | 否 | 发起方 |
| `inputConfig` | string | 否 | 输入配置 |

#### GrantType 枚举

- `normal`: 正常授权，可访问完整数据
- `metadata`: 仅元数据授权，只能查看数据结构
- `file`: 文件级授权，可访问原始文件

### 2. YAML 示例

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataGrant
metadata:
  name: ddg-alice-bob-train-001
  namespace: alice
  labels:
    kuscia.secretflow/domaindatagrant-vendor: "secretflow"
    kuscia.secretflow/domaindatagrant-domain: "bob"
spec:
  # 授权方
  author: "alice@secretflow.com"
  # 被授权的数据 ID
  domainDataID: "train-data-001"
  # 被授权的域
  grantDomain: "bob"
  # 可选签名
  signature: "signed_signature_value"
  # 授权限制
  limit:
    # 过期时间
    expirationTime: "2024-12-31T23:59:59Z"
    # 使用次数限制（0 为无限制）
    useCount: 10
    # 授权模式
    grantMode:
      - "normal"
    # 允许的组件
    components:
      - "psi"
      - "preprocessing"
    # 发起方
    initiator: "bob"
  # 描述信息
  description:
    purpose: "联合建模训练数据"
    project: "cross-domain-ml-project"
status:
  phase: Ready
  message: "Grant is ready"
  use_records:
    - use_time: "2024-01-15T10:30:00Z"
      grant_domain: "bob"
      component: "psi"
      output: "psi-result-001"
```

### 3. 授权 API 接口

#### 创建授权 (Create)

**HTTP API**

```
POST /api/v1/kusciaapi/domaindatagrant/create
```

**请求示例**

```bash
curl -X POST http://localhost:8080/api/v1/kusciaapi/domaindatagrant/create \
  -H "Content-Type: application/json" \
  -d '{
    "header": {
      "domain_id": "alice"
    },
    "domain_data_grant": {
      "author": "alice@secretflow.com",
      "domain_data_id": "train-data-001",
      "grant_domain": "bob",
      "limit": {
        "expiration_time": "2024-12-31T23:59:59Z",
        "use_count": 10,
        "grant_mode": ["normal"],
        "components": ["psi", "preprocessing"]
      }
    }
  }'
```

#### 查询授权 (Query)

**HTTP API**

```
POST /api/v1/kusciaapi/domaindatagrant/query
```

**请求示例**

```bash
curl -X POST http://localhost:8080/api/v1/kusciaapi/domaindatagrant/query \
  -H "Content-Type: application/json" \
  -d '{
    "header": {
      "domain_id": "alice"
    },
    "data": {
      "domain_id": "alice",
      "domain_data_grant_id": "ddg-alice-bob-train-001"
    }
  }'
```

#### 列出授权 (List)

**HTTP API**

```
POST /api/v1/kusciaapi/domaindatagrant/list
```

**请求示例**

```bash
curl -X POST http://localhost:8080/api/v1/kusciaapi/domaindatagrant/list \
  -H "Content-Type: application/json" \
  -d '{
    "header": {
      "domain_id": "alice"
    },
    "data": {
      "domain_id": "alice",
      "page_no": 1,
      "page_size": 10
    }
  }'
```

#### 批量查询授权 (BatchQuery)

**HTTP API**

```
POST /api/v1/kusciaapi/domaindatagrant/batchquery
```

#### 更新授权 (Update)

**HTTP API**

```
POST /api/v1/kusciaapi/domaindatagrant/update
```

#### 删除授权 (Delete)

**HTTP API**

```
POST /api/v1/kusciaapi/domaindatagrant/delete
```

**请求示例**

```bash
curl -X POST http://localhost:8080/api/v1/kusciaapi/domaindatagrant/delete \
  -H "Content-Type: application/json" \
  -d '{
    "header": {
      "domain_id": "alice"
    },
    "data": {
      "domain_id": "alice",
      "domain_data_grant_id": "ddg-alice-bob-train-001"
    }
  }'
```

### 4. 授权工作流程

```
┌─────────────┐                    ┌─────────────┐
│  Domain A   │                    │  Domain B   │
│  (授权方)    │                    │  (被授权方)  │
└──────┬──────┘                    └──────┬──────┘
       │                                  │
       │  1. 创建 DomainData              │
       │     (train-data-001)             │
       ├─────────────────────────────────>│
       │                                  │
       │  2. 创建 DomainDataGrant         │
       │     (授权给 Domain B)            │
       ├─────────────────────────────────>│
       │                                  │
       │  3. 查询已授权的数据              │
       │                                  ├─────────────────────────────────>│
       │                                  │  4. 使用授权访问数据              │
       │                                  │     (在任务中引用)                │
       │                                  │                                  │
       │  5. 记录使用日志                 │
       │     (UseRecord)                  │
       │<─────────────────────────────────┤
       │                                  │
```

---

## 使用场景

### 场景 1: 联邦学习数据准备

在联邦学习场景中，参与方需要注册本地数据并授权给其他参与方使用。

```yaml
# Alice 方注册训练数据
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: alice-train-data
  namespace: alice
spec:
  relativeURI: "data/train.csv"
  author: "alice"
  name: "Alice训练数据"
  type: "table"
  dataSource: "alice-datasource"
  vendor: "secretflow"
  columns:
    - name: "sample_id"
      type: "string"
    - name: "feature_1"
      type: "float"
    - name: "feature_2"
      type: "float"
    - name: "label"
      type: "int"

# Alice 授权给 Bob
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataGrant
metadata:
  name: alice-grant-to-bob
  namespace: alice
spec:
  author: "alice"
  domainDataID: "alice-train-data"
  grantDomain: "bob"
  limit:
    grantMode: ["metadata"]  # 只共享元数据
    components: ["psi"]      # 仅用于 PSI
```

### 场景 2: 模型共享

```yaml
# 注册训练好的模型
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: trained-model-v1
  namespace: alice
spec:
  relativeURI: "models/lr_model.pkl"
  author: "alice"
  name: "逻辑回归模型V1"
  type: "model"
  dataSource: "alice-model-storage"
  vendor: "secretflow"
  attributes:
    algorithm: "logistic_regression"
    metrics.auc: "0.92"
    metrics.accuracy: "0.89"
```

### 场景 3: 多参与方数据协作

```yaml
# 三个参与方的数据注册和授权
# Alice 的数据
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: alice-data
  namespace: alice
spec:
  name: "Alice用户数据"
  type: "table"
  dataSource: "alice-ds"
  relativeURI: "users.csv"
  columns:
    - name: "user_id"
      type: "string"
    - name: "age"
      type: "int"

# Bob 的数据
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: bob-data
  namespace: bob
spec:
  name: "Bob交易数据"
  type: "table"
  dataSource: "bob-ds"
  relativeURI: "transactions.csv"
  columns:
    - name: "user_id"
      type: "string"
    - name: "amount"
      type: "double"

# Charlie 的数据
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: charlie-data
  namespace: charlie
spec:
  name: "Charlie行为数据"
  type: "table"
  dataSource: "charlie-ds"
  relativeURI: "behaviors.csv"
  columns:
    - name: "user_id"
      type: "string"
    - name: "click_count"
      type: "int"

# 相互授权用于联合建模
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataGrant
metadata:
  name: alice-grant-coordinator
  namespace: alice
spec:
  author: "alice"
  domainDataID: "alice-data"
  grantDomain: "coordinator"
  limit:
    grantMode: ["metadata"]
    components: ["psi", "statistics"]
```

### 场景 4: 数据血缘追踪

通过 `attributes` 和 `vendor` 字段记录数据来源和处理历史：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: processed-data
  namespace: alice
spec:
  name: "预处理后的数据"
  type: "table"
  dataSource: "alice-ds"
  relativeURI: "processed/data.csv"
  vendor: "secretflow"
  columns:
    - name: "user_id"
      type: "string"
    - name: "features"
      type: "string"
  attributes:
    source.domaindata: "raw-data-001"
    preprocessing.steps: "cleaning,normalization,encoding"
    preprocessing.timestamp: "2024-01-15T10:00:00Z"
    job.id: "job-12345"
```

---

## 最佳实践

### 1. 命名规范

- `domaindata_id`: 使用有意义的唯一标识，如 `{业务域}-{数据类型}-{版本}`
- `name`: 使用清晰的中文或英文描述
- `relativeURI`: 采用分层目录结构，如 `{业务}/{子业务}/{文件名}`

### 2. 元数据管理

- 充分利用 `attributes` 字段记录额外信息
- 为表格类型数据完整定义 `columns`
- 添加版本号、创建时间、业务描述等属性

### 3. 数据安全

- 根据实际需求设置最小权限的 `grantMode`
- 设置合理的 `expirationTime` 和 `useCount`
- 限制 `components` 范围，避免过度授权

### 4. 数据质量

- 为列定义添加 `comment` 说明
- 正确设置 `notNullable` 约束
- 标注数据格式、单位等信息

### 5. 生命周期管理

- 定期清理过期的 DomainData
- 监控 DomainDataGrant 的使用情况
- 及时撤销不再需要的授权

---

## 常见问题

### Q1: DomainData 和 DomainDataSource 的关系？

**A**: DomainDataSource 描述数据存储的位置和访问方式（如本地文件系统、OSS、ODPS），DomainData 描述具体的数据资产及其元数据。一个 DataSource 可以包含多个 DomainData。

### Q2: 如何确保 DomainData 的唯一性？

**A**: `domaindata_id` 在同一域（namespace）内必须唯一。系统会自动校验，重复创建会返回错误。

### Q3: 授权后如何使用数据？

**A**: 被授权方可以在任务中通过 `domaindata_id` 引用已授权的数据，系统会根据 DomainDataGrant 验证访问权限。

### Q4: 如何查看数据的使用历史？

**A**: 通过查询 DomainDataGrant 的 `status.use_records` 字段，可以看到每次使用的时间、组件和输出。

### Q5: 支持哪些文件格式？

**A**: 目前支持 CSV、Parquet、JSON 等常见格式，通过 `fileFormat` 字段指定。对于数据库类型的数据源，格式由数据源本身决定。

---

## 参考资料

- [Kuscia 官方文档](https://www.secretflow.org.cn/docs/kuscia)
- [Kubernetes CRD 开发指南](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/)
- [SecretFlow 隐私计算平台](https://www.secretflow.org.cn/)

---

**文档版本**: v1.0  
**最后更新**: 2024-06-30
