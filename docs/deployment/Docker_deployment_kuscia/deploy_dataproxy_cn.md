# DataProxy 部署

## 前言

本教程帮助您在 Kuscia 节点上部署 DataProxy。DataProxy 为任务容器提供统一的数据访问代理，使任务可以通过它读取 ODPS、Hive 等外部数据源，而不必在任务镜像中内置各数据源的客户端。工作原理与配置字段详见[Kuscia 数据代理](../../数据代理.md)。

使用 `--data-proxy` 启动参数时，Kuscia 会自动完成以下动作：

1. 在 Lite / Autonomy 节点导入 DataProxy 镜像；
2. 在 Master 节点注册 DataProxy 的 AppImage；
3. 创建名为 `dataproxy-<domain-id>` 的 Serving，由 Kuscia 调度到对应节点。

## 前提条件

- 已完成 Kuscia 节点的部署，且节点状态正常。
- 节点可以访问 `secretflow-registry.cn-hangzhou.cr.aliyuncs.com`，或已离线导入 DataProxy 镜像。
- 如果使用外部数据源，已配置好对应的 DomainDataSource，见[数据源与 DataProxy 的关联](../../数据代理.md#6-数据源与-dataproxy-的关联)。

## 部署

您可以参考[这里](./deploy_p2p_cn.md)了解如何使用 Docker 部署 Kuscia，本文不做过多赘述。

部署 Kuscia 时，在启动命令后面加上 `--data-proxy` 即可。示例如下：

- 点对点模式

使用 `--data-proxy` 参数在 autonomy 节点上导入 DataProxy 的镜像和注册 DataProxy 的 AppImage

```bash
./kuscia.sh start -c autonomy_alice.yaml -p 11080 -k 11081 --data-proxy
```

- 中心化模式

使用 `--data-proxy` 参数在 master 节点上注册 DataProxy 的 AppImage

```bash
./kuscia.sh start -c kuscia_master.yaml -p 18080 -k 18081 --data-proxy
```

使用 `--data-proxy` 参数在 lite 节点上导入 DataProxy 的镜像

```bash
./kuscia.sh start -c lite_alice.yaml -p 28080 -k 28081 --data-proxy
```

> **说明**：中心化模式下，DataProxy 的 Serving 由 Master 统一调度；Lite 节点只需保证镜像已导入。请先在 Master 上注册 AppImage，再启动 Lite。

## 验证

- 点对点模式

在成功启动 Kuscia 后，执行如下命令看到 pod 为 Running 代表 DataProxy 部署成功。

```bash
docker exec -it ${USER}-kuscia-autonomy-alice kubectl get po -A

NAMESPACE   NAME                              READY   STATUS    RESTARTS   AGE
alice       dataproxy-alice-699dc7455-sxvpj   1/1     Running   0          26s
```

- 中心化模式

各节点成功启动 Kuscia ，其中 master 节点上成功注册 DataProxy 的 AppImage ，并在 lite 节点上成功导入 DataProxy 的镜像后，在 master 节点执行如下命令看到 pod 为 Running 代表 DataProxy 部署成功。

```bash
docker exec -it ${USER}-kuscia-master kubectl get po -A

NAMESPACE   NAME                              READY   STATUS    RESTARTS   AGE
alice       dataproxy-alice-699dc7455-sxvpj   1/1     Running   0          26s
```

进一步检查：

```bash
# 查看 DataProxy Pod 详情与事件（<pod-name> 替换为上一命令中的 Pod 名称）
docker exec -it ${USER}-kuscia-autonomy-alice kubectl describe po -n alice <pod-name>

# 查看 DataProxy 日志
docker exec -it ${USER}-kuscia-autonomy-alice kubectl logs -n alice <pod-name> --tail=100

# 确认与 DataMesh 的连通性
docker exec -it ${USER}-kuscia-autonomy-alice kubectl get svc -n alice
```

如果 Pod 长时间处于 `Pending` 或 `ImagePullBackOff`，请检查镜像是否已导入、节点资源是否充足，以及 Serving 调度是否成功。

## 手动部署

如果不希望使用 `--data-proxy`，也可以通过 KusciaAPI 手动注册 AppImage 并创建 Serving，或直接在 K8s 中部署。完整步骤、请求示例和删除方式见[Kuscia 数据代理](../../数据代理.md#5-部署方式)。

## 常见问题

| 现象 | 可能原因 | 处理方式 |
| --- | --- | --- |
| 没有 DataProxy Pod | 未加 `--data-proxy`，或 AppImage 未注册 | 检查启动参数与 `kubectl get appimage`（在容器内执行） |
| `ImagePullBackOff` | 镜像未导入或网络不可达 | 在节点内执行 `kuscia image list`，离线导入镜像 |
| Serving 创建失败 | Master 未注册 AppImage，或 Lite 未导入镜像 | 按中心化模式的顺序重新执行 |
| 任务读取外部数据失败 | DomainDataSource 配置错误或网络不通 | 检查数据源配置与 DataProxy 日志，参考[数据代理](../../数据代理.md) |

## 相关文档

- [Kuscia 数据代理](../../数据代理.md)
- [Kuscia 部署指引](../kuscia_deployment_instructions.md)
- [部署要求](../deploy_check.md)
- [DomainDataSource 参考](../../reference/concepts/domaindatasource_cn.md)
