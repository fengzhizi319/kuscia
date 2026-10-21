# Kuscia 部署指引

Kuscia 支持 Docker 和 Kubernetes 两种部署环境，每种环境又可以选择点对点或中心化组网。本文帮助你选择部署方式，并给出部署后的最小验证步骤。

## 部署方式选型

| 维度 | 点对点（P2P） | 中心化（Master / Lite） |
| --- | --- | --- |
| 控制面 | 每个 Autonomy 节点各自独立 | Master 统一管理，Lite 受控 |
| 适用场景 | 机构对等协作、快速组网 | 有一个中心管理方、节点数量较多 |
| 节点角色 | Autonomy | Master + Lite |
| 部署复杂度 | 低，节点间两两授权 | 中，需要先部署 Master 再签发 Token 加入 Lite |
| 典型文档 | [Docker 点对点](./Docker_deployment_kuscia/deploy_p2p_cn.md) / [K8s 点对点](./K8s_deployment_kuscia/K8s_p2p_cn.md) | [Docker 中心化](./Docker_deployment_kuscia/deploy_master_lite_cn.md) / [K8s 中心化](./K8s_deployment_kuscia/K8s_master_lite_cn.md) |

运行时的选择：

| 运行时 | 隔离方式 | 适用场景 | 说明 |
| --- | --- | --- | --- |
| `runc` | 容器 | 默认，生产推荐 | 需要容器运行时能力 |
| `runk` | K8s Pod | 已具备 K8s 集群 | 与集群调度集成 |
| `runp` | 节点内进程 | 受限环境、无容器运行时 | 当前仅支持 SecretFlow 引擎，**不支持 job 资源限制**，见[RunP 部署](./deploy_with_runp_cn.md) |

## 前置准备

Kuscia 对部署环境有一定要求，请先完成[部署要求](./deploy_check.md)中的自检，并确认端口规划符合[端口说明](./kuscia_ports_cn.md)。

## Docker 多机部署 Kuscia

Docker 多机部署 Kuscia 分为点对点模式和中心化模式，请根据需要选择相应的部署方式。

- 点对点模式：部署参考[多机部署点对点集群](./Docker_deployment_kuscia/deploy_p2p_cn.md)
- 中心化模式：部署参考[多机部署中心化集群](./Docker_deployment_kuscia/deploy_master_lite_cn.md)

部署完成后如需启用 DataProxy、以非 root 用户运行或使用 RunP，可继续参考：

- [部署 DataProxy](./Docker_deployment_kuscia/deploy_dataproxy_cn.md)
- [非 root 用户部署](./Docker_deployment_kuscia/docker_deploy_kuscia_with_rootless.md)
- [使用 RunP 模式部署节点](./deploy_with_runp_cn.md)
- [升级 Kuscia 版本](./Docker_deployment_kuscia/deploy_upgrade_kuscia_cn.md)

## K8s 集群部署 Kuscia

在 K8s 集群部署中，我们将介绍点对点集群、中心化集群以及如何使用 RunP 运行时模式来训练任务。

- 点对点模式：部署参考[部署点对点集群](./K8s_deployment_kuscia/K8s_p2p_cn.md)
- 中心化模式：部署参考[部署中心化集群](./K8s_deployment_kuscia/K8s_master_lite_cn.md)
- 使用 RunP 运行时模式来训练任务：部署参考[使用 RunP 模式部署节点](./deploy_with_runp_cn.md)

K8s 部署的命名空间按节点角色划分，例如中心化模式下常见为 `kuscia-master`、`lite-alice`、`lite-bob`；点对点模式下常见为 `autonomy-alice`、`autonomy-bob`。具体以部署文档中的清单为准。

## 验证部署

Kuscia 部署完成后，可以通过以下几个方面来验证部署是否成功：

1. 容器或 Pod 正常运行；
2. Kuscia 服务健康检查通过；
3. 节点与路由状态正常；
4. 能够提交并完成一个最小任务。

### 1. 检查容器 / Pod

Docker 部署：

```bash
docker ps --format 'table {{.Names}}\t{{.Status}}' | grep kuscia

# 进入节点容器查看内部 Pod（以 Autonomy 为例）
docker exec -it ${USER}-kuscia-autonomy-alice kubectl get po -A
```

中心化模式下分别检查 Master 与 Lite：

```bash
docker exec -it ${USER}-kuscia-master kubectl get po -n kuscia-master
docker exec -it ${USER}-kuscia-lite-alice kubectl get po -n lite-alice
```

K8s 部署：

```bash
kubectl get po -n kuscia-master
kubectl get po -n lite-alice
kubectl get po -n autonomy-alice
```

所有 Kuscia 组件应为 `Running`，且 `READY` 与期望副本数一致。

### 2. 健康检查

KusciaAPI 使用 mTLS，推荐在节点容器内使用预置证书调用：

```bash
docker exec -it ${USER}-kuscia-autonomy-alice bash -c '
export CTR_CERTS_ROOT=/home/kuscia/var/certs
curl -k -s -X POST "https://localhost:8082/healthZ" \
  --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
  --header "Content-Type: application/json" \
  --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
  --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
  --cacert ${CTR_CERTS_ROOT}/ca.crt \
  -d "{}"'
```

期望返回 `"code": 0` 且 `"ready": true`。接口字段与更多调用方式见 [Health API](../reference/apis/health_cn.md)。

### 3. 检查节点与路由

```bash
# 容器内或 K8s 控制面执行
kubectl get domains
kubectl get domainroutes
kubectl get domaindatas

# 中心化模式查看证书签发状态
kubectl get cdr
```

`DomainRoute` 的 `Ready` 应为 `True`。点对点模式下还需要确认双方已完成授权，排查方法见[网络连通性排查](../troubleshoot/network/network_troubleshoot.md)。

### 4. 提交最小任务

使用 SecretFlow 作业验证端到端链路：

- 通过 KusciaAPI 提交：[使用 KusciaAPI 运行 SecretFlow 作业](../tutorial/run_sf_job_with_api_cn.md)
- 通过 SecretPad 提交：参考[快速体验](../getting_started/quickstart_cn.md)中的示例任务
- 差分隐私任务：[在 Kuscia 上运行差分隐私任务](../tutorial/run_dp_on_kuscia_cn.md)

任务 `Phase` 最终应为 `Succeeded`。

## 升级与回滚

Docker 部署使用 `kuscia.sh upgrade` 升级镜像，升级前请备份数据目录与配置，详见[升级 Kuscia 版本](./Docker_deployment_kuscia/deploy_upgrade_kuscia_cn.md)。K8s 部署通过更新 Deployment 镜像版本完成滚动升级，回滚使用 `kubectl rollout undo`。

## 故障排查

部署过程中遇到问题可以先参考[部署失败排查](../troubleshoot/deployment/deploy_failed.md)，并按以下顺序收集信息：

```bash
# 容器日志
docker logs --tail 200 ${USER}-kuscia-autonomy-alice

# Kuscia 组件日志
docker exec -it ${USER}-kuscia-autonomy-alice tail -n 200 /home/kuscia/var/logs/kuscia.log

# K8s 事件
kubectl describe po -n lite-alice <pod-name>
kubectl logs -n lite-alice <pod-name> --tail=200
```

常见问题：

- [Create fsnotify failed](../troubleshoot/deployment/create_fsnotify_failed.md)
- [内核参数不满足要求](../troubleshoot/parameter_tuning/kernel_params.md)
- [K8s 线程限制](../troubleshoot/parameter_tuning/k8s_ulimit_check.md)
- [网络授权检查](../troubleshoot/network/network_authorization_check.md)

## 更多帮助

如需进一步帮助，请在 Kuscia [Issue](https://github.com/secretflow/kuscia/issues) 上提交问题。
