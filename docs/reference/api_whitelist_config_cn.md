# 设置 K3S API 白名单

## 说明

K3S API 白名单会过滤由 Envoy 转发的 K3S API 请求，不在白名单中的请求会返回 404，在白名单请求可以正常返回。

如果配置列表为空，则默认放行所有请求。

白名单只作用于 **Master 转发到其 API Server 的请求**，不改变 KusciaAPI 自身的鉴权，也不替代 Token / mTLS。其目标是收敛 Lite / 任务侧可以访问的 K8s 资源范围，降低误用与越权风险。

## 配置方法

配置文件位于 `/home/kuscia/etc/kuscia.yaml`，配置项位于 master.apiWhitelist 节点下，由多个正则表达式组成，多个正则表达式之间是`或`的关系。

下面两个个配置的效果是一样的：

```yaml
# config 1
master:
  apiWhitelist:
    - /(api(s)?(/[0-9A-Za-z_.-]+)?/v1(alpha1)?/namespaces/[0-9A-Za-z_.-]+/(pods|gateways|domainroutes|endpoints|services|events|configmaps|leases|taskresources|secrets|domaindatas|domaindatagrants|domaindatasources)(/[0-9A-Za-z_.-]+(/status$)?)?)
    - /api/v1/namespaces/[0-9A-Za-z_.-]+
    - /api/v1/nodes(/.*)?

# config 2
master:
  apiWhitelist:
    - (/(api(s)?(/[0-9A-Za-z_.-]+)?/v1(alpha1)?/namespaces/[0-9A-Za-z_.-]+/(pods|gateways|domainroutes|endpoints|services|events|configmaps|leases|taskresources|secrets|domaindatas|domaindatagrants|domaindatasources)(/[0-9A-Za-z_.-]+(/status$)?)?))|(/api/v1/namespaces/[0-9A-Za-z_.-]+)|(/api/v1/nodes(/.*)?)
```

## 正则语义

- 列表内多个表达式按“或”拼接：任一匹配即放行；
- 使用 Envoy `safe_regex`（RE2 语法），不支持回溯引用等 PCRE 特性；
- 匹配的是**完整请求路径**（含 `/api`、`/apis`、namespace、资源名、子资源）；
- 空列表或不配置时，不启用过滤，所有路径放行。

## 常用配置示例

只允许读取本节点的 Pod、Service、ConfigMap 与 Kuscia CRD：

```yaml
master:
  apiWhitelist:
    - ^/api/v1/namespaces/[^/]+/(pods|services|configmaps|events)(/[^/]+)?$
    - ^/apis/kuscia.secretflow/v1alpha1/namespaces/[^/]+/(domains|domainroutes|domaindatas|domaindatagrants|domaindatasources|taskresources)(/[^/]+)?$
    - ^/api/v1/nodes(/.*)?$
```

允许查看 Pod 日志：

```yaml
master:
  apiWhitelist:
    - ^/api/v1/namespaces/[^/]+/pods/[^/]+/log$
```

> 业务需要 `kubectl exec` 时，还需要放行 `pods/exec`、`pods/attach` 等子资源路径；这属于高权限能力，生产环境应谨慎评估。

## 应用配置

1. 编辑节点容器内的配置：

   ```bash
   docker exec -it ${USER}-kuscia-master vi /home/kuscia/etc/conf/kuscia.yaml
   ```

   如果配置已挂载到宿主机，直接编辑宿主机文件。

2. 确认 YAML 合法：

   ```bash
   docker exec -it ${USER}-kuscia-master \
     grep -A20 'apiWhitelist' /home/kuscia/etc/conf/kuscia.yaml
   ```

3. 重启 Master 使配置生效：

   ```bash
   docker restart ${USER}-kuscia-master
   ```

   K8s 部署则更新 ConfigMap 后重启对应 Pod：

   ```bash
   kubectl -n kuscia-master rollout restart deployment/kuscia-master
   ```

## 验证

在 Lite 节点容器内通过 Envoy 访问 Master API，验证白名单行为：

```bash
# 白名单内的资源应正常返回
docker exec -it ${USER}-kuscia-lite-alice kubectl get pods -n alice

# 不在白名单内的资源应返回 404（以 roles 为例）
docker exec -it ${USER}-kuscia-lite-alice kubectl get roles -n alice
```

如果白名单为空，以上命令都应正常返回；启用白名单后，只有匹配的资源可访问。

同时确认 Kuscia 自身功能不受影响：

```bash
docker exec -it ${USER}-kuscia-lite-alice kubectl get domaindatas -n alice
docker exec -it ${USER}-kuscia-lite-alice kubectl get domainroutes
```

## 注意事项

- 不要使用 `.*` 或过宽的正则，否则白名单形同虚设；
- 新增 CRD 或业务资源时，同步更新白名单，否则 Lite 侧会出现 404；
- 修改后先在一个 Lite 节点验证，再推广到全部节点；
- 白名单不负责认证，仍需依赖 mTLS / Token 与 RBAC 控制身份；
- 配置错误可能导致 Lite 无法同步任务状态，请保留回滚配置。

## 相关文档

- [Kuscia K8s 架构](../development/kuscia_kubernetes_architecture.md)
- [DomainRoute 概念](./concepts/domainroute_cn.md)
- [Kuscia 部署指引](../deployment/kuscia_deployment_instructions.md)
- [网络连通性排查](../troubleshoot/network/network_troubleshoot.md)
