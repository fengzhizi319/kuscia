# LiteKusciaConfig 配置字段详细说明

## 概述

`LiteKusciaConfig` 是 Kuscia Lite 节点的完整配置结构，定义了 Lite 节点运行所需的所有参数。Lite 节点是工作节点，负责执行隐私计算任务，需要连接到 Master 节点进行注册和接收任务。

## 字段详细说明

### 1. CommonConfig（嵌入结构）

通用配置，所有节点类型共享的基础配置项。

#### 1.1 Mode (string)
- **YAML键**: `mode`
- **示例值**: `"lite"`
- **说明**: 部署模式，固定为 "lite"
- **作用**: 标识当前节点为 Lite 类型，决定启动哪些模块
- **取值**: `master` / `lite` / `autonomy`

#### 1.2 DomainID (string)
- **YAML键**: `domainID`
- **示例值**: `"alice"`
- **说明**: 域的唯一标识符
- **作用**: 
  - 区分不同的参与方（如 alice、bob、carol）
  - 用作 Kubernetes 命名空间名称
  - 在跨域通信中标识身份
- **要求**: 不能为空，不能使用 "master"（保留字）

#### 1.3 DomainKeyData (string)
- **YAML键**: `domainKeyData`
- **示例值**: `"LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQ..."`
- **说明**: Base64 编码的 RSA 私钥
- **作用**:
  - 用于节点间的安全通信（TLS/MTLS）
  - 生成证书签名请求（CSR）
  - 身份认证和数字签名
- **安全提示**: 生产环境应使用安全的密钥管理方案，不要明文存储

#### 1.4 LogLevel (string)
- **YAML键**: `logLevel`
- **示例值**: `"INFO"`
- **说明**: 日志输出级别
- **作用**: 控制日志的详细程度
- **取值**:
  - `DEBUG`: 最详细，包含调试信息
  - `INFO`: 常规信息（推荐）
  - `WARN`: 仅警告和错误
  - `ERROR`: 仅错误信息

#### 1.5 Protocol (common.Protocol)
- **YAML键**: `protocol`
- **示例值**: `"MTLS"`
- **说明**: 通信协议类型
- **作用**: 定义节点间通信的加密方式
- **取值**:
  - `NOTLS`: 无加密（不推荐）
  - `TLS`: 单向 TLS 加密
  - `MTLS`: 双向 TLS 加密（推荐，更安全）

---

### 2. LiteDeployToken (string) ⭐ Lite 特有

- **YAML键**: `liteDeployToken`
- **示例值**: `"LS0tLS1CRUdJTi..."`
- **说明**: Lite 节点部署令牌（一次性使用）
- **作用**:
  - Lite 节点向 Master 注册时的身份认证凭证
  - Master 验证令牌的合法性后颁发证书
  - 注册完成后令牌被标记为"已使用"
- **获取方式**: 通过 Master 节点的 `add_domain_lite.sh` 脚本生成
- **安全性**: 令牌是一次性的，使用后失效，防止重放攻击

---

### 3. MasterEndpoint (string) ⭐ Lite 特有

- **YAML键**: `masterEndpoint`
- **示例值**: `"https://172.18.0.2:1080"`
- **说明**: Master 节点的访问端点
- **作用**:
  - Lite 节点连接到此地址进行注册
  - 接收来自 Master 的任务调度
  - 上报任务执行状态
- **格式**: `https://<master-ip>:<port>`
- **默认端口**: 1080（Gateway 端口）

---

### 4. Runtime (string)

- **YAML键**: `runtime`
- **示例值**: `"runc"`
- **说明**: 容器运行时类型
- **作用**: 决定如何执行隐私计算任务
- **取值**:
  - `runc`: 标准 OCI 容器运行时（默认，适用于本地 Docker）
  - `runk`: 对接外部 Kubernetes 集群，将任务 Pod 调度到外部 K8s
  - `runp`: 进程模式，以进程方式运行任务，无需容器化（性能更好）

---

### 5. Runk (RunkConfig) ⭐ 仅在 runtime=runk 时生效

对接外部 Kubernetes 集群的配置。

#### 5.1 Namespace (string)
- **YAML键**: `namespace`
- **示例值**: `"secretflow-tasks"`
- **说明**: 外部 K8s 集群的命名空间
- **作用**: 任务 Pod 将被调度到此命名空间中

#### 5.2 DNSServers ([]string)
- **YAML键**: `dnsServers`
- **示例值**: `["8.8.8.8", "8.8.4.4"]`
- **说明**: DNS 服务器列表
- **作用**: 外部 K8s 集群的 DNS 解析配置

#### 5.3 KubeconfigFile (string)
- **YAML键**: `kubeconfigFile`
- **示例值**: `"/path/to/kubeconfig"`
- **说明**: Kubeconfig 文件路径
- **作用**: 访问外部 K8s 集群的认证凭证
- **注意**: 如果不指定，将使用 Pod 内的默认 ServiceAccount

#### 5.4 EnableLogging (bool)
- **YAML键**: `enableLogging`
- **示例值**: `true`
- **说明**: 是否启用容器日志收集
- **作用**: 控制是否记录任务容器的标准输出和错误输出

#### 5.5 LogDirectory (string)
- **YAML键**: `logDirectory`
- **示例值**: `"/var/log/kuscia"`
- **说明**: 日志存储目录
- **作用**: 容器日志保存的路径

#### 5.6 LogMaxFiles (int)
- **YAML键**: `logMaxFiles`
- **示例值**: `5`
- **说明**: 每个容器保留的最大日志文件数
- **作用**: 控制日志文件数量，超过后旧文件被删除
- **要求**: 必须 > 1

#### 5.7 LogMaxSize (string)
- **YAML键**: `logMaxSize`
- **示例值**: `"100Mi"`
- **说明**: 单个日志文件的最大大小
- **作用**: 达到此大小后触发日志轮转
- **格式**: 支持 Kubernetes 数量格式（Ki/Mi/Gi）

---

### 6. Capacity (config.CapacityCfg)

资源容量配置，定义节点可用于调度的资源上限。

#### 6.1 CPU (resource.Quantity)
- **YAML键**: `cpu`
- **示例值**: `4`
- **说明**: 可用 CPU 核心数
- **作用**: 调度器根据此限制分配任务，避免超配

#### 6.2 Memory (resource.Quantity)
- **YAML键**: `memory`
- **示例值**: `"8Gi"`
- **说明**: 可用内存容量
- **作用**: 限制可调度任务的总内存使用

#### 6.3 Pods (int)
- **YAML键**: `pods`
- **示例值**: `500`
- **说明**: 最大 Pod 数量
- **作用**: 限制同时运行的任务数量

#### 6.4 Storage (resource.Quantity)
- **YAML键**: `storage`
- **示例值**: `"100Gi"`
- **说明**: 可用存储容量
- **作用**: 限制数据存储的使用量

#### 6.5 Bandwidth (resource.Quantity)
- **YAML键**: `bandwidth`
- **示例值**: `1000`
- **说明**: 带宽资源（Mbps）
- **作用**: 自定义资源，限制网络传输速率

---

### 7. ReservedResources (config.ReservedResourcesCfg)

预留资源配置，为系统组件预留的资源，不参与任务调度。

#### 7.1 CPU (string)
- **YAML键**: `reservedResources.cpu`
- **示例值**: `"500m"`
- **说明**: 预留 CPU 资源
- **作用**: 确保系统组件（Agent、DataMesh等）有足够的 CPU

#### 7.2 Memory (string)
- **YAML键**: `reservedResources.memory`
- **示例值**: `"1Gi"`
- **说明**: 预留内存资源
- **作用**: 确保系统组件有足够的内存

#### 7.3 Bandwidth (string)
- **YAML键**: `reservedResources.bandwidth`
- **示例值**: `"100"`
- **说明**: 预留带宽资源（Mbps）
- **作用**: 确保系统通信不受任务影响

---

### 8. Image (ImageConfig)

镜像管理配置，控制容器镜像的拉取行为。

#### 8.1 PullPolicy (string)
- **YAML键**: `image.pullPolicy`
- **示例值**: `"IfNotPresent"`
- **说明**: 镜像拉取策略
- **取值**:
  - `Always`: 总是从仓库拉取最新镜像
  - `IfNotPresent`: 本地不存在时才拉取（默认，推荐）
  - `Never`: 从不拉取，仅使用本地镜像

#### 8.2 DefaultRegistry (string)
- **YAML键**: `image.defaultRegistry`
- **示例值**: `"secretflow-registry.cn-hangzhou.cr.aliyuncs.com"`
- **说明**: 默认镜像仓库地址
- **作用**: 当镜像名称未指定仓库时使用此地址

#### 8.3 Registries ([]ImageRegistry)
- **YAML键**: `image.registries`
- **说明**: 私有镜像仓库列表
- **作用**: 配置需要认证的私有仓库

##### 8.3.1 Name (string)
- 仓库名称标识符

##### 8.3.2 Endpoint (string)
- 仓库访问地址

##### 8.3.3 UserName (string)
- 认证用户名

##### 8.3.4 Password (string)
- 认证密码

#### 8.4 HTTPProxy (string)
- **YAML键**: `image.httpProxy`
- **示例值**: `"http://127.0.0.1:8080"`
- **说明**: HTTP 代理地址
- **作用**: 加速镜像拉取，特别适用于网络受限环境

---

### 9. AdvancedConfig（嵌入结构）

高级配置，包含各个子系统的详细配置。

#### 9.1 KusciaAPI (*kaconfig.KusciaAPIConfig)
- **YAML键**: `kusciaAPI`
- **说明**: KusciaAPI 服务配置
- **作用**: 管理 API 的端口、证书、SANs 等

#### 9.2 ConfManager (*cmconfig.ConfManagerConfig)
- **YAML键**: `confManager`
- **说明**: 配置管理器配置
- **作用**: 密钥后端存储配置（mem/rfile）

#### 9.3 DataMesh (*dmconfig.DataMeshConfig)
- **YAML键**: `dataMesh`
- **说明**: DataMesh 数据网格配置
- **作用**: 配置数据代理服务、端口等

##### 9.3.1 DataProxyList ([]DataProxyConfig)
- 数据代理列表，配置不同数据源的访问方式

#### 9.4 DomainRoute (DomainRouteConfig)
- **YAML键**: `domainRoute`
- **说明**: 域路由配置
- **作用**: 配置跨域通信的 TLS 设置

#### 9.5 Agent (config.AgentConfig)
- **YAML键**: `agent`
- **说明**: Agent 配置
- **作用**: 容器运行时、插件、资源限制等

##### 9.5.1 AllowPrivileged (bool)
- 是否允许特权容器

##### 9.5.2 Plugins ([]PluginConfig)
- Agent 插件列表（cert-issuance、config-render、env-import 等）

#### 9.6 Debug (bool)
- **YAML键**: `debug`
- **示例值**: `false`
- **说明**: 是否启用调试模式
- **作用**: 启用 pprof 性能分析

#### 9.7 DebugPort (int)
- **YAML键**: `debugPort`
- **示例值**: `28080`
- **说明**: 调试端口
- **作用**: pprof 服务监听端口

#### 9.8 EnableWorkloadApprove (bool)
- **YAML键**: `enableWorkloadApprove`
- **示例值**: `false`
- **说明**: 是否启用工作负载审批
- **作用**: 启用后，任务执行前需要手动审批（生产环境推荐启用）

#### 9.9 Logrotate (LogrotateConfig)
- **YAML键**: `logrotate`
- **说明**: 日志轮转配置

##### 9.9.1 MaxFiles (int)
- 最大日志文件数

##### 9.9.2 MaxFileSizeMB (int)
- 单文件最大大小（MB）

##### 9.9.3 MaxAgeDays (int)
- 最大保留天数

#### 9.10 KusciaAPISans ([]string)
- **YAML键**: `kusciaAPISans`
- **说明**: KusciaAPI 证书的 SANs 列表
- **作用**: 允许额外的域名/IP 访问 API

---

### 10. GarbageCollection (GarbageCollectionConfig)

垃圾回收配置，控制过期资源的自动清理。

#### 10.1 KusciaDomainDataGC (KusciaDomainDataGCConfig)
- **YAML键**: `garbageCollection.kusciaDomainDataGC`
- **说明**: 域数据垃圾回收配置

##### 10.1.1 Enable (*bool)
- 是否启用域数据 GC

##### 10.1.2 DurationHours (int)
- 保留时长（小时），默认 720（30天）

#### 10.2 KusciaJobGC (KusciaJobGCConfig)
- **YAML键**: `garbageCollection.kusciaJobGC`
- **说明**: 任务垃圾回收配置

##### 10.2.1 DurationHours (int)
- 保留时长（小时），默认 720（30天）

---

## 配置示例

```yaml
# Lite 节点完整配置示例
mode: lite
domainID: alice
domainKeyData: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQ...
logLevel: INFO
protocol: MTLS

# Lite 特有配置
liteDeployToken: LS0tLS1CRUdJTi...
masterEndpoint: https://172.18.0.2:1080

# 运行时配置
runtime: runc

# 资源容量
capacity:
  cpu: 4
  memory: 8Gi
  pods: 500
  storage: 100Gi
  bandwidth: 1000

# 预留资源
reservedResources:
  cpu: 500m
  memory: 1Gi
  bandwidth: 100

# 镜像配置
image:
  pullPolicy: IfNotPresent
  defaultRegistry: ""
  httpProxy: ""
  registries:
    - name: my-registry
      endpoint: https://registry.example.com
      username: user
      password: pass

# 高级配置
kusciaAPI:
  port: 8082
  grpcPort: 8083
  
dataMesh:
  dataProxyList:
    - endpoint: "dataproxy-grpc:8023"
      dataSourceTypes:
        - "odps"
        - "hive"

agent:
  allowPrivileged: false
  plugins:
    - name: cert-issuance
    - name: config-render

debug: false
debugPort: 28080
enableWorkloadApprove: false

logrotate:
  maxFiles: 5
  maxFileSizeMB: 100
  maxAgeDays: 30

# 垃圾回收
garbageCollection:
  kusciaDomainDataGC:
    enable: false
    durationHours: 720
  kusciaJobGC:
    durationHours: 720
```

---

## 配置优先级说明

当日志轮转等配置存在多个来源时，优先级如下：

1. **CRI 级别配置**（最高优先级）
   - `agent.provider.cri.containerLogMaxFiles`
   - `agent.provider.cri.containerLogMaxSize`

2. **Advanced 配置**
   - `advancedConfig.logrotate.maxFiles`
   - `advancedConfig.logrotate.maxFileSizeMB`

3. **全局配置**
   - `logrotate.maxFiles`
   - `logrotate.maxFileSizeMB`

4. **默认值**（最低优先级）
   - `config.DefaultLogRotateMaxFiles`
   - `config.DefaultLogRotateMaxSizeStr`

---

## 常见问题

### Q1: LiteDeployToken 从哪里获取？
通过 Master 节点执行以下命令生成：
```bash
docker exec -it <master_container> scripts/deploy/add_domain_lite.sh <domain_id>
```

### Q2: 如何切换到 runk 运行时？
修改配置文件：
```yaml
runtime: runk
runk:
  namespace: secretflow-tasks
  kubeconfigFile: /path/to/kubeconfig
  dnsServers:
    - 8.8.8.8
```

### Q3: 如何配置私有镜像仓库？
```yaml
image:
  registries:
    - name: my-private-registry
      endpoint: https://registry.example.com
      username: myuser
      password: mypassword
```

### Q4: 如何启用工作负载审批？
```yaml
enableWorkloadApprove: true
```
启用后，任务执行前需要通过 KusciaAPI 手动审批。

---

## 最佳实践

1. **生产环境**：
   - 启用 MTLS 协议
   - 启用工作负载审批
   - 配置合理的资源容量和预留
   - 启用垃圾回收

2. **开发测试**：
   - 使用 INFO 日志级别
   - 可以不启用审批流程
   - 资源容量可以设置较小

3. **安全建议**：
   - 不要明文存储私钥和令牌
   - 使用 SecretBackend 管理敏感信息
   - 定期轮换 Deploy Token

4. **性能优化**：
   - 根据实际硬件配置设置 capacity
   - 合理预留系统资源
   - 配置合适的日志轮转策略
