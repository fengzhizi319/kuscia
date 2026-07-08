# golangci-lint 修复汇总

## 背景

执行 `make check` 时，golangci-lint 报出 77 个问题，共分为两类：

- `goconst`：43 处字符串字面量被重复 3 次及以上，应提取为常量。
- `govet`（`inline` checker）：34 处仍使用 `golang.org/x/net/context`，Go  Vet 建议直接使用标准库 `context`。

本批次修改只针对这两类问题，未调整业务逻辑。

## 验证结果

修复后执行：

```bash
cd /home/charles/code/sfwork/kuscia
golangci-lint run --config=.golangci.yml
```

结果：

```text
0 issues
```

受影响包的单测也已跑通：

```bash
go test ./pkg/gateway/utils/... ./pkg/gateway/controller/... \
  ./pkg/kusciaapi/... ./pkg/controllers/kusciadeployment/... \
  ./pkg/scheduler/kusciascheduling/... ./pkg/interconn/bfia/handler/... \
  ./pkg/utils/tls/... ./pkg/datamesh/metaserver/service/...
```

全部 `ok`。

---

## 1. govet inline：替换 `golang.org/x/net/context` 为标准库 `context`

这些文件仅把第三方 `context` 包换成标准库 `context`，其他逻辑未改动。

| 文件 |
| --- |
| `cmd/kuscia/utils/kuscia_image.go` |
| `pkg/agent/kuberuntime/fake_kuberuntime_manager.go` |
| `pkg/agent/prober/prober.go` |
| `pkg/diagnose/app/netstat/bandwidth.go` |
| `pkg/diagnose/app/netstat/bandwidth_test.go` |
| `pkg/diagnose/app/netstat/buffer.go` |
| `pkg/diagnose/app/netstat/connection.go` |
| `pkg/diagnose/app/netstat/latency.go` |
| `pkg/diagnose/app/netstat/latency_test.go` |
| `pkg/diagnose/app/netstat/proxytimeout.go` |
| `pkg/diagnose/app/netstat/proxytimeout_test.go` |
| `pkg/diagnose/app/netstat/request_size.go` |
| `pkg/diagnose/app/netstat/request_size_test.go` |
| `pkg/diagnose/app/netstat/util_test.go` |
| `pkg/diagnose/mods/network.go` |
| `pkg/transport/server/grpc/server.go` |
| `pkg/controllers/kusciadeployment/reconcile.go`（同时有 goconst 修复） |

---

## 2. goconst：提取重复字符串为常量

按模块/包汇总如下。

### Agent 模块

| 文件 | 新增/使用的常量 | 说明 |
| --- | --- | --- |
| `pkg/agent/local/store/image_reader.go` | `sha256Algorithm = "sha256"` | 镜像 digest 算法 |
| `pkg/agent/local/store/image_reader_test.go` | 复用 `sha256Algorithm` | 测试同步 |
| `pkg/agent/provider/pod/env.go` | `specNodeNameFieldPath = "spec.nodeName"`、`specServiceAccountNameFieldPath = "spec.serviceAccountName"` | 环境变量 fieldPath |
| `pkg/agent/provider/pod/env_test.go` | 复用上述常量 | 测试同步 |
| `pkg/agent/provider/pod/k8s_provider.go` | `resolvConfigVolumeName = "resolv-config"` | resolv 配置卷名 |
| `pkg/agent/provider/pod/k8s_provider_test.go` | 复用 `resolvConfigVolumeName` | 测试同步 |
| `pkg/agent/status/status_manager.go` | `containerStatusUnknownReason = "ContainerStatusUnknown"`、`containerStatusUnknownMessage = "..."` | 容器终止原因/消息 |
| `pkg/agent/status/status_manager_test.go` | 复用上述常量 | 测试同步 |
| `pkg/agent/utils/format/pod.go` | `nilString = "<nil>"` | 格式化 `<nil>` |
| `pkg/agent/utils/format/pod_test.go` | 复用 `nilString` | 测试同步 |

### Common / 通用

| 文件 | 新增/使用的常量 | 说明 |
| --- | --- | --- |
| `pkg/common/convert.go` | `dataTypeInt32 = "int32"`、`dataTypeString = "string"`、`dataTypeStr = "str"` | 数据类型转换 |
| `pkg/common/convert_test.go` | 复用上述常量 | 测试同步 |

### Controllers

| 文件 | 新增/使用的常量 | 说明 |
| --- | --- | --- |
| `pkg/controllers/clusterdomainroute/monitor.go` | `domainRouteMetricType = "DomainRoute"` | 监控指标类型 |
| `pkg/controllers/clusterdomainroute/controller_test.go` | 复用 `domainRouteMetricType` | 测试同步 |
| `pkg/controllers/kusciatask/handler/pending_handler.go` | `trueStr = "true"` | annotation 值 |
| `pkg/controllers/kusciatask/handler/pending_handler_test.go` | 复用 `trueStr` | 测试同步 |
| `pkg/controllers/kusciadeployment/reconcile.go` | `rollingUpdateMaxRatio = "25%"`、`hostnameTopologyKey = "kubernetes.io/hostname"`、`trueStr = "true"` | 滚动更新、拓扑键 |

### DataMesh

| 文件 | 新增/使用的常量 | 说明 |
| --- | --- | --- |
| `pkg/datamesh/bean/http_server_bean.go` | `queryRelativePath = "query"` | HTTP 路由路径 |
| `pkg/datamesh/metaserver/service/domaindata.go` | `dataTypeString = "string"`、`dataTypeStr = "str"` | 数据列类型 |
| `pkg/datamesh/metaserver/service/domaindata_test.go` | 复用 `dataTypeString` | 测试同步 |
| `pkg/datamesh/metaserver/service/domaindatagrant_test.go` | 复用 `dataTypeString` | 测试同步 |

### Gateway

| 文件 | 新增/使用的常量 | 说明 |
| --- | --- | --- |
| `pkg/gateway/controller/domain_route.go` | 复用 `xds.ProtocolGRPC`、`xds.ProtocolHTTP` | 协议比较 |
| `pkg/gateway/controller/sort_domain_ports.go` | 复用 `xds.ProtocolGRPC`、`xds.ProtocolHTTP` | 端口排序权重 |
| `pkg/gateway/controller/interconn/factory.go` | `kusciaTokenHeader = "Kuscia-Token"` | 网关请求头 |
| `pkg/gateway/controller/interconn/bfia_handler.go` | 复用 `kusciaTokenHeader` | 请求头 |
| `pkg/gateway/controller/interconn/kuscia_handler.go` | 复用 `kusciaTokenHeader` | 请求头 |
| `pkg/gateway/metrics/metrics.go` | `statLabel = "stat"` | Prometheus label |
| `pkg/gateway/utils/handshake.go` | `HandshakePathSuffix = "/handshake"`（导出） | 握手路径后缀 |
| `pkg/gateway/utils/handshake_test.go` | 复用 `HandshakePathSuffix` | 测试同步 |
| `pkg/gateway/utils/http.go` | `protocolHTTP = "http"`、`protocolHTTPS = "https"` | URL 协议解析 |
| `pkg/gateway/utils/http_test.go` | 复用 `protocolHTTP`、`protocolHTTPS` | 测试同步 |

### Interconn / BFIA

| 文件 | 新增/使用的常量 | 说明 |
| --- | --- | --- |
| `pkg/interconn/bfia/handler/create_job.go` | `statusField = "status"` | 响应字段 key |
| `pkg/interconn/bfia/handler/poll_task_status.go` | 复用 `statusField` | 响应字段 |
| `pkg/interconn/bfia/handler/query_job_status_all.go` | 复用 `statusField` | 响应字段 |

### KusciaAPI

| 文件 | 新增/使用的常量 | 说明 |
| --- | --- | --- |
| `pkg/kusciaapi/bean/http_server_bean.go` | `pathCreate`、`pathDelete`、`pathQuery`、`pathUpdate`、`pathBatchQuery`、`pathList`、`pathStatusBatchQuery` | REST 路由路径 |
| `pkg/kusciaapi/service/domain_route_service.go` | `protocolHTTP`、`protocolHTTPS`、`protocolGRPC`、`protocolGRPCS` | DomainRoute 协议 |
| `pkg/kusciaapi/service/serving_service.go` | `rollingUpdateMaxRatio = "25%"`、`hostnameTopologyKey = "kubernetes.io/hostname"` | 部署策略/拓扑 |
| `pkg/kusciaapi/service/serving_service_test.go` | 复用上述常量 | 测试同步 |

### Scheduler

| 文件 | 新增/使用的常量 | 说明 |
| --- | --- | --- |
| `pkg/scheduler/kusciascheduling/kusciascheduling.go` | `contentTypeJSON = "application/json"` | kubeconfig ContentType |
| `pkg/scheduler/kusciascheduling/kusciascheduling_test.go` | 复用 `contentTypeJSON` | 测试同步 |

### TLS

| 文件 | 新增/使用的常量 | 说明 |
| --- | --- | --- |
| `pkg/utils/tls/ca.go` | 复用 `CERTIFICATE`、`RsaPKCS1PrivateKey` | PEM block type |
| `pkg/utils/tls/cert.go` | 复用 `CERTIFICATE`、`RsaPKCS1PrivateKey` | PEM block type |

### Thirdparty / FATE

| 文件 | 新增/使用的常量 | 说明 |
| --- | --- | --- |
| `thirdparty/fate/pkg/adapter/adapter.go` | `jobIDField = "job_id"` | FATE 请求参数 key |

---

## 3. 统计

| 类型 | 修复文件数 | 主要策略 |
| --- | --- | --- |
| `govet inline` | 17 | `golang.org/x/net/context` → `context` |
| `goconst` | 42 | 提取包内常量；跨包复用已有常量（如 `xds.ProtocolHTTP`、`CERTIFICATE`） |
| **合计** | **59** | 纯重构，无业务逻辑变更 |

---

## 4. 注意事项

1. 常量命名尽量遵循原文件风格；仅在 `pkg/gateway/utils/handshake.go` 导出了 `HandshakePathSuffix`，因为测试包外（如 `pkg/gateway/clusters/master_test.go`）仍保留了字面量 `/handshake`，导出后方便后续统一。
2. `pkg/kusciaapi/service/domain_route_service.go` 中的协议常量是小写 `protocolHTTP` 等，避免与 CRD 的 `ProtocolHTTP` 类型冲突。
3. 所有修改已通过 `golangci-lint run` 和受影响包的 `go test`。
