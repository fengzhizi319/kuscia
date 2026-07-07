# Kuscia 数据代理（DataProxy）

## 1. 概述

**DataProxy（数据代理）** 是 Kuscia 生态中用于扩展数据源访问能力的组件。Kuscia 原生的 DataMesh 内置支持 `localfs`、`oss`、`mysql`、`postgresql` 等常见数据源的读写，但对于某些 Java 生态更为成熟的数据源（如 ODPS、Hive、Kingbase、Dameng 等），使用 Golang 实现完整的 SDK 支持并不现实。因此，Kuscia 引入了 **DataProxy** 作为外部数据源代理，将特定类型的数据访问请求转发给 DataProxy，由 DataProxy 完成实际的数据 IO。

DataProxy 采用 **Java** 实现，基于 [Apache Arrow Flight](https://arrow.apache.org/docs/format/Flight.html) 协议与 Kuscia DataMesh 交互，基本可以满足大部分复杂数据源的扩展需求。

### 1.1 为什么需要 DataProxy

- **扩展性**：Kuscia 内置 IO 使用 Golang 实现，某些数据库/数据仓库的 Golang SDK 不完善或缺失。
- **生态丰富**：Java 在大数据领域（ODPS、Hive、JDBC 等）生态完善，DataProxy 复用这些能力。
- **安全隔离**：DataProxy 以独立 Pod/容器的形式运行，数据源凭据存储在 DataProxy 侧，降低直接暴露给计算引擎的风险。
- **即插即用**：通过 Kuscia `AppImage` + `Serving` 机制部署，可按需启用。

### 1.2 典型使用场景

| 场景 | 说明 |
| ------ | ------ |
| 访问 ODPS（MaxCompute） | ODPS 必须使用 DataProxy 访问。 |
| 访问 Hive | 通过 DataProxy 的 Hive/JDBC 能力读取数据。 |
| 访问国产数据库 | Kingbase（人大金仓）、Dameng（达梦）等通过 DataProxy 扩展。 |
| 复用企业现有 Java 数据连接器 | 企业已有的 Java 数据访问 SDK 可封装为 DataProxy 插件。 |

---

## 2. 架构与工作原理

### 2.1 DataProxy 在 Kuscia 中的位置

```text
┌─────────────────────────────────────────────────────────────┐
│                    隐私计算应用（SecretFlow）                  │
│                      通过 Arrow Flight 访问数据               │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                      Kuscia DataMesh                          │
│  1. 解析 DomainData / DomainDataSource                        │
│  2. 根据数据源类型决定：直连 or 转发给 DataProxy              │
└───────────────────────┬─────────────────────────────────────┘
                        │
        ┌───────────────┴───────────────┐
        │                               │
        ▼                               ▼
┌──────────────┐              ┌─────────────────────┐
│  内置 IO      │              │    DataProxy        │
│ localfs/oss/ │              │  (Java + Arrow Flight)│
│ mysql/pgsql  │              │  odps/hive/kingbase/ │
│              │              │  dameng/...          │
└──────────────┘              └─────────────────────┘
                                        │
                                        ▼
                                ┌───────────────┐
                                │   实际数据源    │
                                │  ODPS/Hive/... │
                                └───────────────┘
```

### 2.2 工作流程

1. **配置阶段**：管理员在 `kuscia.yaml` 的 `dataMesh.dataProxyList` 中声明 DataProxy 端点及其负责的数据源类型。
2. **元数据阶段**：应用调用 DataMesh `GetFlightInfo` 接口请求数据。
3. **路由阶段**：DataMesh 根据 `DomainDataSource.type` 判断：
   - 若类型在 `dataProxyList.dataSourceTypes` 中，则将请求转发给对应 DataProxy。
   - 否则使用 Kuscia 内置 IO 处理。
4. **数据阶段**：应用通过 Arrow Flight `DoGet` / `DoPut` 与 DataProxy 直接传输数据。

### 2.3 关键设计点

- **Arrow Flight 协议**：DataProxy 与 DataMesh、应用之间均通过 Arrow Flight 传输数据，保证高效、统一。
- **DomainDataSource 的 `accessDirectly` 字段**：
  - `true`：应用直连数据源（不经过 DataProxy）。
  - `false`（默认）：应用可通过 DataProxy 访问数据源。
  - 当前 ODPS 类型 **必须** 经过 DataProxy。
- **一个 DataProxy 可代理多种数据源类型**，通过 `dataSourceTypes` 列表配置。
- **当前每个数据源类型仅支持配置一个 DataProxy**；若重复配置，后配置项会覆盖前者。

---

## 3. 支持的数据源类型

| 数据源类型 | 说明 | 是否必须经过 DataProxy |
| ------------ | ------ | ------------------------ |
| `odps` | 阿里云 MaxCompute | 是 |
| `hive` | Apache Hive | 按需 |
| `kingbase` | 人大金仓数据库 | 按需 |
| `dameng` | 达梦数据库 | 按需 |
| 自定义类型 | 通过 DataProxy 插件扩展 | 按需 |

> 注：`localfs`、`oss`、`mysql`、`postgresql` 等类型由 Kuscia 内置 IO 原生支持，无需 DataProxy。

---

## 4. 配置说明

### 4.1 kuscia.yaml 中的 dataMesh 配置

在 `kuscia.yaml` 中新增 `dataMesh` 节点：

```yaml
dataMesh:
  dataProxyList:
    - endpoint: "dataproxy-grpc:8023"
      dataSourceTypes:
        - "odps"
        - "hive"
        - "kingbase"
        - "dameng"
      # clientTLSConfig:          # 可选：访问 DataProxy 时的 TLS 配置
      #   certFile: ""
      #   keyFile: ""
      #   caFile: ""
```

### 4.2 配置字段详解

| 字段 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `endpoint` | `string` | 是 | DataProxy 的 gRPC 服务地址。在 Kuscia 域内通常使用 Service 域名，如 `dataproxy-grpc:8023`。 |
| `dataSourceTypes` | `[]string` | 是 | 该 DataProxy 负责代理的数据源类型列表。 |
| `clientTLSConfig` | `object` | 否 | 访问 DataProxy 时使用的 mTLS 证书配置。若 DataProxy 与 DataMesh 之间需要双向认证，则配置此项。 |
| `mode` | `string` | 否 | IO 模式：`proxy`（应用 → DataMesh → DataProxy → 数据源）或 `direct`（应用直连）。默认根据场景自动选择。 |

- **什么是 Service 域名？**
- Service 域名 是 Kuscia 域内服务之间相互访问时使用的内部地址，它本质上是一种基于 DNS 的服务发现机制。
  - 当你创建一个服务时，Kuscia（借助其内置的 CoreDNS 组件）会自动为其分配一个唯一的、格式化的域名，通常的格式是 <service-name>.<namespace>.svc。
  - 例如，一个名为 datamesh 的服务，部署在 kuscia-system 命名空间下，它的 Service 域名就是 datamesh.kuscia-system.svc。
- **为什么使用 Service 域名？**
  - 在 Kuscia 域内采用 Service 域名进行通信，主要有以下几个原因：
  - 1.服务发现与解耦 (Service Discovery & Decoupling)：
    - 服务之间不需要硬编码对方的 IP 地址。IP 地址是动态变化的，尤其是在容器化的环境中，服务实例（Pod）可能会被销毁、重建或扩缩容，导致 IP 变更。
      - 通过一个固定不变的域名，服务可以轻松找到并连接到它需要通信的目标服务，实现了服务间的松耦合。
  - 2.负载均衡 (Load Balancing)：
    - 一个 Service 域名背后可以对应一个或多个服务实例（Pods）。
    - 当一个服务通过域名访问另一个服务时，Kuscia 会自动将请求流量分发到后端的健康实例上，实现了服务级别的负载均衡，提高了系统的可用性和扩展性。
  - 3.抽象与稳定性 (Abstraction & Stability)：
    - Service 域名为服务提供了一个稳定的网络端点（Endpoint）。无论后端的服务实例如何变化，这个域名始终是固定的。
    - 这极大地简化了开发和运维，开发者只需要关心服务本身的域名，而无需关心底层基础设施的复杂细节。
  - 4.命名空间隔离 (Namespace Isolation)：
    - 通过将服务划分到不同的命名空间（Namespace），可以实现网络层面的逻辑隔离。不同命名空间下的服务可以重名而不会冲突，因为它们的完整域名（FQDN）是不同的。
  总结来说，在 Kuscia 域内使用 Service 域名，是借鉴了 Kubernetes 成熟的服务治理模式。它提供了一种稳定、可靠、自动化的方式来管理域内各个组件之间的网络通信，使得整个系统更加健壮、灵活和易于扩展。

### 4.3 配置注意事项

- `dataMesh` 配置需与 `protocol`、`datastoreEndpoint` 等字段保持同级缩进。
- 修改 `kuscia.yaml` 后需**重启 Kuscia 容器**生效。
- 在 K8s（RunK）模式下，需修改 ConfigMap 后重启 Pod。

---

## 5. 部署方式

DataProxy 的部署分为两部分：

1. **注册 AppImage**：将 DataProxy 镜像注册为 Kuscia 的 `AppImage` 资源。
2. **部署 Serving**：通过 KusciaAPI Serving 接口创建 DataProxy 实例。

Kuscia 提供了两种部署方式：自动部署（推荐）和手动部署。

### 5.1 自动部署（使用 kuscia.sh 的 --data-proxy）

部署 Kuscia 时，在启动命令后追加 `--data-proxy` 参数即可。

#### 5.1.1 P2P 模式

```bash
# 在 autonomy 节点上自动导入 DataProxy 镜像、注册 AppImage 并部署 Serving
./kuscia.sh start -c autonomy_alice.yaml -p 11080 -k 11081 --data-proxy
```

#### 5.1.2 中心化模式

```bash
# 在 master 节点上注册 DataProxy AppImage
./kuscia.sh start -c kuscia_master.yaml -p 18080 -k 18081 --data-proxy

# 在 lite 节点上导入 DataProxy 镜像（Serving 由 master 统一调度部署）
./kuscia.sh start -c lite_alice.yaml -p 28080 -k 28081 --data-proxy
```

#### 5.1.3 验证自动部署

```bash
docker exec -it ${USER}-kuscia-autonomy-alice kubectl get po -A

# 预期输出
NAMESPACE   NAME                              READY   STATUS    RESTARTS   AGE
alice       dataproxy-alice-699dc7455-sxvpj   1/1     Running   0          26s
```

### 5.2 手动部署（使用 KusciaAPI Serving）

适用于需要自定义配置、自定义镜像或 K8s 部署的场景。

#### 5.2.1 准备工作

1. 修改 `kuscia.yaml` 或 K8s ConfigMap，添加 `dataMesh.dataProxyList` 配置。
2. 重启 Kuscia 节点使配置生效。
3. 登录到 Kuscia 容器内部。

#### 5.2.2 注册 DataProxy AppImage

```bash
scripts/deploy/register_app_image.sh \
  -i "secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/dataproxy:0.1.0b1" \
  -m
```

#### 5.2.3 调用 KusciaAPI 部署 DataProxy

以下以 MTLS 协议为例。若使用 NOTLS/TLS，请参考[协议说明](./troubleshoot/concept/protocol_describe.md)调整 curl 参数。

```bash
export CTR_CERTS_ROOT=/home/kuscia/var/certs

curl -X POST 'https://localhost:8082/api/v1/serving/create' \
  --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
  --header 'Content-Type: application/json' \
  --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
  --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
  --cacert ${CTR_CERTS_ROOT}/ca.crt \
  -d '{
     "serving_id": "dataproxy-alice",
     "initiator": "alice",
     "parties": [{
         "app_image": "dataproxy-image",
         "domain_id": "alice",
         "service_name_prefix": "dataproxy"
       }
     ]
  }'
```

#### 5.2.4 查询与删除 DataProxy

查询：

```bash
curl -X POST 'https://localhost:8082/api/v1/serving/query' \
  --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
  --header 'Content-Type: application/json' \
  --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
  --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
  --cacert ${CTR_CERTS_ROOT}/ca.crt \
  -d '{
     "serving_id": "dataproxy-alice",
     "domain_id": "alice"
  }' | jq
```

删除：

```bash
curl -X POST 'https://localhost:8082/api/v1/serving/delete' \
  --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
  --header 'Content-Type: application/json' \
  --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
  --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
  --cacert ${CTR_CERTS_ROOT}/ca.crt \
  -d '{
     "domain_id": "alice",
     "serving_id": "dataproxy-alice"
  }'
```

#### 5.2.5 K8s 模式部署

1. 编辑 ConfigMap：

   ```bash
   kubectl edit cm kuscia-autonomy-alice-cm -n autonomy-alice
   ```

2. 添加 `dataMesh` 配置（与 `protocol` 字段保持同级缩进）。

3. 重启 Kuscia Pod 使配置生效。

4. 登录 Pod 后执行 AppImage 注册与 Serving 创建（命令与 Docker 模式相同）。

---

## 6. 数据源与 DataProxy 的关联

### 6.1 DomainDataSource 示例

以下是一个通过 DataProxy 访问 ODPS 的 `DomainDataSource` 示例：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataSource
metadata:
  labels:
    kuscia.secretflow/domaindatasource-type: odps
  name: odps-data-source
  namespace: alice
spec:
  accessDirectly: false   # 必须通过 DataProxy 访问
  data:
    encryptedInfo: <使用 alice 域公钥加密的 ODPS 连接信息>
  name: odps-data-source
  type: odps
  uri: alice_project/tables/user_data
```

### 6.2 accessDirectly 字段说明

- `accessDirectly: true`：应用直连数据源，不经过 DataProxy。
- `accessDirectly: false`（默认）：DataMesh 根据 `type` 决定将请求转发给 DataProxy 或内置 IO。

> 当前 **ODPS 类型必须设置 `accessDirectly: false`**，因为 Kuscia 没有内置 ODPS IO。

---

## 7. 自定义 DataProxy 插件

若需支持 `dataProxyList` 中未列出的数据源类型，可基于 [secretflow/dataproxy](https://github.com/secretflow/dataproxy) 仓库开发自定义插件。

### 7.1 开发步骤

1. 在 `dataproxy-plugins` 模块下新增数据源模块。
2. 实现 `DataProxyFlightProducer` 接口：
   - `getFlightInfo`：提供数据访问元信息。
   - `getStream`：提供数据读取能力。
   - `acceptPut`：提供数据写入能力。
3. 使用 Java SPI 机制注册实现。
4. 编写单元测试与集成测试。
5. 构建镜像并注册到 Kuscia。

### 7.2 Kuscia 侧扩展

若新的数据源类型需要 Kuscia 侧识别，可参考 `pkg/datamesh/dataserver/io/external` 下的 `dataproxy_client.go` 与 `external_io.go`，必要时扩展 `DomainDataSource.type` 枚举与路由逻辑。

---

## 8. 常见问题与排查

### 8.1 DataProxy Pod 无法启动

- 检查 AppImage 是否已注册：

  ```bash
  kubectl get appimage dataproxy-image -n <domain>
  ```

- 检查镜像是否已导入：

  ```bash
  kuscia image ls
  ```

- 查看 Pod 事件与日志：

  ```bash
  kubectl describe pod dataproxy-<domain>-xxx -n <domain>
  kubectl logs dataproxy-<domain>-xxx -n <domain>
  ```

### 8.2 应用无法访问 DataProxy

- 确认 `kuscia.yaml` 中 `dataMesh.dataProxyList` 已配置对应 `dataSourceTypes`。
- 确认 DataProxy Serving 已成功创建且 Pod 为 `Running`。
- 在任务 Pod 内测试域名解析：

  ```bash
  ping dataproxy-grpc
  ```

- 检查 DataMesh 日志：

  ```bash
  tail -f /home/kuscia/var/logs/datamesh/datamesh.log
  ```

### 8.3 ODPS 数据读取失败

- 确认 `DomainDataSource.type` 为 `odps` 且 `accessDirectly` 为 `false`。
- 确认 `encryptedInfo` 已使用当前域的公钥加密。
- 检查 DataProxy 日志中的 ODPS SDK 报错。

### 8.4 多数据源类型配置冲突

当前版本每个数据源类型**只能映射到一个 DataProxy endpoint**。如果需要将不同类型路由到不同 DataProxy，需确保同一 `dataSourceTypes` 值在全局只出现一次。

---

## 9. 最佳实践

1. **生产环境**：
   - 启用 MTLS 协议，配置 `clientTLSConfig` 保证 DataMesh 与 DataProxy 之间的通信安全。
   - 将 DataProxy 部署在资源充足的节点上，避免与计算任务争抢资源。
   - 对数据源凭据使用 Kuscia SecretBackend 管理。

2. **开发测试**：
   - 使用 `./kuscia.sh start ... --data-proxy` 快速体验。
   - 使用 NOTLS 协议简化部署，但**不要用于生产**。

3. **性能优化**：
   - 对于高频访问的数据源，可独立部署多个 DataProxy 实例（需确保数据源类型不冲突）。
   - 监控 DataProxy 的 JVM 内存与 GC 情况，必要时调整 `JAVA_OPTS`。

---

## 10. 相关文档

- [DataProxy 部署指南](./deployment/Docker_deployment_kuscia/deploy_dataproxy_cn.md)
- [如何使用 Kuscia API 部署 DataProxy](./tutorial/run_dp_on_kuscia_cn.md)
- [DomainDataSource 概念](./reference/concepts/domaindatasource_cn.md)
- [DomainDataSource API](./reference/apis/domaindatasource_cn.md)
- [Kuscia 配置文件说明](./deployment/kuscia_config_cn.md)
- [DataMesh IO 扩展开发](./development/add_datamesh_io.md)
- [DataProxy 源码仓库](https://github.com/secretflow/dataproxy)
