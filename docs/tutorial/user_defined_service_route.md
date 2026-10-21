# 如何给 Kuscia 自定义 Service 路由

## 说明

通过复用 Kuscia 提供的网络打平功能，可以在 Alice 和 Bob 中自定义 service，进行请求转发。

Kuscia 会把各 Domain 的 Service 名称打通到其他授权节点，因此可以使用 `<service>.<namespace>.svc` 的形式跨域访问。典型场景是两个机构的管理平台需要互相调用 API，但又不希望直接暴露宿主机地址。

## 前提条件

- 两个节点已完成组网与授权，`DomainRoute` 状态为 `Ready`；
- 目标服务在各自机构内可访问，且端口已放行；
- 自定义 Service 的 namespace 必须与 DomainID 一致；
- 目标服务自身已配置认证、TLS 或访问控制（Kuscia 只保证网络可达，不替代业务鉴权）。

## 示例

下面是 Alice 和 Bob 的管理平台之间需要进行通信示例：

### 在 Alice 节点创建 Service

在 Alice 节点容器内，手动创建一个 ExternalName 类型的 Service， 其中 ExternalName 设置为 Alice 平台的地址，例如:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: alice-pad
  namespace: alice
spec:
  # Alice-side platform runs on address 10.88.0.2
  externalName: 10.88.0.2
  ports:
    - name: cluster
      # Alice-side platform runs on port 10010
      port: 10010
      targetPort: 10010
  type: ExternalName
```

内容 copy 到 alice-pad.yaml，执行：

```bash
docker cp alice-pad.yaml ${USER}-kuscia-autonomy-alice:/tmp/alice-pad.yaml
docker exec -it ${USER}-kuscia-autonomy-alice kubectl create -f /tmp/alice-pad.yaml
```

### 在 Bob 节点创建 Service

在 Bob 节点容器内，手动创建一个 ExternalName 类型的 Service, 其中 ExternalName 设置为 Bob 平台的地址，例如:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: bob-pad
  namespace: bob
spec:
  # bob-side platform runs on address 10.88.0.3
  externalName: 10.88.0.3
  ports:
    - name: cluster
      # bob-side platform runs on port 10010
      port: 10010
      targetPort: 10010
  type: ExternalName
```

内容 copy 到 bob-pad.yaml，执行：

```bash
docker cp bob-pad.yaml ${USER}-kuscia-autonomy-bob:/tmp/bob-pad.yaml
docker exec -it ${USER}-kuscia-autonomy-bob kubectl create -f /tmp/bob-pad.yaml
```

中心化模式下，把容器名替换为对应的 Lite 节点容器（如 `${USER}-kuscia-lite-alice`）。

## 访问方法

下面是 Alice 访问 Bob 侧平台的方法，反之类似：

- 若在 Alice Docker 容器内，直接访问 Bob 平台的方式：`curl -v http://bob-pad.bob.svc`
- 若在 Alice Docker 容器外，那么需要把 Alice 节点的 80 端口暴露到宿主机上，然后通过 `curl -v http://127.0.0.1:{暴露在宿主机上的端口} -H "host:bob-pad.bob.svc"`

示例：

```bash
# 在 Alice 节点容器内
docker exec -it ${USER}-kuscia-autonomy-alice \
  curl -v http://bob-pad.bob.svc:10010/health

# 在宿主机上（假设节点 80 端口映射到 18080）
curl -v http://127.0.0.1:18080/health -H "host: bob-pad.bob.svc"
```

> Tips：通过上述方式，将 Service 暴露出来后，虽然 Kuscia 做了安全性的防护（只有授权后的节点才能访问到该 Service），但是毕竟是内部服务暴露出来给其他机构，请注意服务自身的安全性加强，比如越权漏洞等。

## 验证

```bash
# Service 已创建
docker exec -it ${USER}-kuscia-autonomy-alice kubectl get svc -n alice alice-pad

# 路由仍为 Ready
docker exec -it ${USER}-kuscia-autonomy-alice kubectl get domainroutes

# DNS 解析（在节点容器内）
docker exec -it ${USER}-kuscia-autonomy-alice getent hosts bob-pad.bob.svc

# 端到端连通性
docker exec -it ${USER}-kuscia-autonomy-alice curl -v http://bob-pad.bob.svc:10010/
```

如果 DNS 能解析但连接失败，优先检查目标平台是否监听、端口是否放行，以及目标节点的 DomainRoute 状态。

## 清理

```bash
docker exec -it ${USER}-kuscia-autonomy-alice kubectl delete -f /tmp/alice-pad.yaml
docker exec -it ${USER}-kuscia-autonomy-bob kubectl delete -f /tmp/bob-pad.yaml
```

## 常见问题

| 现象 | 可能原因 | 处理 |
| --- | --- | --- |
| `NXDOMAIN` / 无法解析 | Service 未创建、namespace 不匹配或路由未 Ready | 检查 `kubectl get svc`、`kubectl get domainroutes` |
| `Connection refused` | 目标平台未监听或端口映射错误 | 在目标机构内直接 `curl` 验证服务 |
| `404` / 错误路由 | HTTP Host 头与 Service DNS 不一致 | 使用 `-H "host: <service>.<namespace>.svc"` |
| 跨域访问超时 | 1080 端口不通或代理/防火墙拦截 | 检查节点间网络与[端口配置](../deployment/kuscia_ports_cn.md) |
| 只有部分节点可访问 | 未完成双向授权 | 按组网文档完成授权并确认 `cdr` 状态 |

## 安全建议

- 只暴露必要的服务与端口，不要将管理后台、数据库、Docker API 暴露给合作方；
- 在应用层启用 mTLS、Token 或 IP 白名单；
- 对访问方做最小权限授权，定期审查 `DomainRoute` 与 Service；
- 跨域流量建议走专线或 VPN，并记录访问日志。

## 相关文档

- [Kuscia 网络与组网模式](../组网模式.md)
- [DomainRoute 概念](../reference/concepts/domainroute_cn.md)
- [Kuscia 端口介绍](../deployment/kuscia_ports_cn.md)
- [网络连通性排查](../troubleshoot/network/network_troubleshoot.md)
