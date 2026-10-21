# 任务运行网络错误排查

> 大多数任务失败是因为在运行任务前没做节点间授权，或者授权有问题，请提前参考[授权错误排查](./network_authorization_check.md)确保授权没有问题。

## 适用范围

当 KusciaTask 失败，且任务日志中出现以下现象时，可按本文排查网络问题：

- 连接超时、`connection reset`、`connection refused`；
- HTTP 状态码为 401、403、404、502、503；
- 双方节点日志中同一请求的结果不一致。

## 问题排查的准入：找到问题请求的 traceid

Envoy 通过 HTTP 请求头中的 `x-b3-traceid` 字段标识一个请求。拿到 traceid 后，就可以在两端网关日志中定位同一条请求：

- 如果引擎日志有打印 traceid，从任务日志中获取 traceid 即可；
- 如果引擎日志没有 traceid，则根据请求的 taskid、URL 或请求时间点，在 Envoy 日志中找到匹配的请求，再取出 traceid。

如下示例，Envoy 的 `internal.log` 和 `external.log` 日志中 URL 后面的第一个字段即 traceid：

```text
1.1.1.1 - [06/Dec/2023:16:33:33 +0000] bob hypq-fuexafpf-node-3-0-fed.alice.svc "POST /org.interconnection.link.ReceiverService/Push HTTP/1.1" 0ce06cf5c3249d98 0ce06cf5c3249d98 200 32 0 - -
```

在节点容器内，可以按 traceid 过滤日志：

```bash
# 进入节点容器（中心化模式为 lite 容器，点对点模式为 autonomy 容器）
docker exec -it ${USER}-kuscia-lite-alice bash

# 查看网关日志位置
ls /home/kuscia/var/logs/

# 按 traceid 过滤
grep "0ce06cf5c3249d98" /home/kuscia/var/logs/internal.log
grep "0ce06cf5c3249d98" /home/kuscia/var/logs/external.log
```

## 认识 Gateway 的日志格式

### 日志格式

Envoy 的日志格式参考 [Kuscia 日志说明](../../deployment/logdescription.md#envoy-日志格式)。

一条访问日志中，主要字段的含义如下：

| 字段 | 含义 |
| --- | --- |
| `1.1.1.1` | 请求来源 IP |
| `[06/Dec/2023:16:33:33 +0000]` | 请求时间 |
| `bob` | 处理该请求的本节点 Domain |
| `hypq-fuexafpf-node-3-0-fed.alice.svc` | 目标 Service 域名，`alice.svc` 表示目标 Domain 为 alice |
| `"POST /org.interconnection.link.ReceiverService/Push HTTP/1.1"` | 请求方法、路径与协议 |
| `0ce06cf5c3249d98` | `x-b3-traceid` |
| `0ce06cf5c3249d98` | `x-b3-spanid` |
| `200` | HTTP 响应码 |
| `32` | 响应字节数 |

### 怎么区分是入口流量还是出口流量

通常来说，`external.log` 中的请求是入口流量，`internal.log` 中的请求是出口流量。但是存在一种特殊场景，即转发场景：Alice --> Bob --> Carol。

在 Bob 上，如果收到一个发送给 Carol 的请求（例如请求的域名是 `xxx.carol.svc`），Alice 的请求会从 ExternalPort 进入，然后转到 InternalPort，最后转发到 Carol 的网关。因此 Bob 的 `external.log` 和 `internal.log` 中会有对应该请求 traceid 的两条日志，也可以根据请求域名中 ServiceName 的 Namespace 判断是不是自己的节点。

示例如下：

```text
1.1.1.1 - [08/Dec/2023:07:21:10 +00001] alice mavis10-0-psi.carol.svc "POST /org.interconnection.link.ReceiverService/Push HTTP/1.1" 8c57cbc928bb598e 8c57cbc928bb598e 200 - 1398243 10 0 10 0 - -
```

## 正常请求的日志 Demo

查看 traceid=`b257a3410662f1f3` 的日志：

- 发送端的 `internal.log`：

    ```text
    1.1.1.1 - [04/Jan/2024:05:52:40 +0000] bob squu-xaskaali-node-3-0-fed.alice.svc "POST /org.interconnection.link.ReceiverService/Push HTTP/1.1" b257a3410662f1f3 b257a3410662f1f3 200 - 149 0 0 0 0 - -
    ```

- 接收端的 `external.log`：

    ```text
    2.2.2.2 - [04/Jan/2024:05:52:40 +0000] bob squu-xaskaali-node-3-0-fed.alice.svc "POST /org.interconnection.link.ReceiverService/Push HTTP/1.1" b257a3410662f1f3 b257a3410662f1f3 200 - 149 0 - -
    ```

两端使用相同的 traceid，且响应码为 `200`，说明请求已成功跨域送达。

## 常见错误与排查方向

| 现象 | 可能原因 | 排查动作 |
| --- | --- | --- |
| `401 Unauthorized` | 节点未授权、Token 错误或协议不匹配 | 检查 `kubectl get cdr`，参考[授权错误排查](./network_authorization_check.md) |
| `403 Forbidden` | 授权存在但访问路径/方法不被允许 | 核对 DomainRoute 与目标服务路径 |
| `404 Not Found` | Service 名、Namespace 或路径错误 | 核对请求域名与 `kubectl get svc -A` |
| `502/503` | 对端服务未启动、端口未监听 | 在目标节点检查 Pod、Service 与端口 |
| 连接超时 | 防火墙、安全组、端口映射或出口 IP 白名单 | 从源节点 `curl` 目标地址，检查链路 |
| 双方日志状态码不一致 | 中间网关或转发链路异常 | 按 traceid 逐跳比对 `external.log` / `internal.log` |

## 进一步排查

- 使用 `curl -kvvv` 从源节点直接访问目标节点，观察 TLS、状态码与响应头；
- 使用 `kubectl get cdr -o yaml` 查看授权详情；
- 复杂网络拓扑下使用 Tcpdump / Wireshark 抓包，并参考[网络要求](../../deployment/networkrequirements.md)。

## 相关文档

- [授权错误排查](./network_authorization_check.md)
- [Kuscia 日志说明](../../deployment/logdescription.md)
- [Kuscia 网络要求](../../deployment/networkrequirements.md)
- [Kuscia 端口介绍](../../deployment/kuscia_ports_cn.md)
