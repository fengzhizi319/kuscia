# FATE 部署失败

## 问题描述

在 Kuscia 上部署 FATE 时，Pod 无法启动，常见状态包括 `ErrImagePull`、`ImagePullBackOff`、`CrashLoopBackOff` 或长时间 `Pending`。

例如：

```shell
kubectl get pods -A

NAMESPACE   NAME                               READY   STATUS         RESTARTS   AGE
bob         fate-deploy-bob-6b85647f8b-5nvtz   0/1     ErrImagePull   0          43s
```

## 排查步骤

### 1. 查看 Pod 事件

```bash
kubectl describe po -n bob fate-deploy-bob-6b85647f8b-5nvtz
kubectl get events -n bob --sort-by=.lastTimestamp | tail -20
```

重点看 `Events` 中的 `Failed to pull image`、`no space left on device`、`exec format error` 等信息。

### 2. 检查容器内的镜像

FATE 部署时会把相关的镜像 import 到节点容器中。

进入节点，使用 `crictl images` 命令查看镜像。中心化组网的节点容器中完备的镜像如下：

```shell
docker exec -it ${USER}-kuscia-lite-bob bash

crictl images | grep fate

docker.io/secretflow/fate-adapter                                                    0.0.1               8b19bcdc69d4c       260MB
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/fate-adapter              0.0.1               8b19bcdc69d4c       260MB
docker.io/secretflow/fate-deploy-basic                                               0.0.1               38ba174f12520       3.23GB
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/fate-deploy-basic         0.0.1               38ba174f12520       3.23GB
```

P2P 组网的节点容器中完备的镜像如下：

```shell
docker exec -it ${USER}-kuscia-autonomy-bob bash

crictl images | grep fate

docker.io/secretflow/fate-adapter                                                    0.0.1               8b19bcdc69d4c       260MB
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/fate-adapter              0.0.1               8b19bcdc69d4c       260MB
docker.io/secretflow/fate-deploy-basic                                               0.0.1               38ba174f12520       3.23GB
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/fate-deploy-basic         0.0.1               38ba174f12520       3.23GB
```

### 3. 核对镜像名与标签

Pod 事件中的镜像名必须与 `crictl images` 中的**完整名称（含 registry 前缀和 tag）**一致。常见不一致：

- 缺少 `docker.io/` 或 `secretflow-registry...` 前缀；
- tag 不是 `0.0.1`；
- 架构不匹配（例如在 arm64 节点上导入了 amd64 镜像）；
- 镜像只导入了一部分，`crictl images` 中显示不完整或大小为 0。

### 4. 检查磁盘空间

```bash
df -h /var/lib/docker /home
docker exec -it ${USER}-kuscia-lite-bob df -h /home/kuscia
```

FATE 基础镜像约 3 GB，导入过程还需要额外空间；磁盘不足会导致导入中断。

## 解决方案

如果缺少 `secretflow/fate-adapter:0.0.1` 和 `secretflow/fate-deploy-basic:0.0.1` 相关的任意镜像（注意镜像地址的前缀，一共有四个），请使用 `crictl rmi` 命令移除镜像。之后删除已部署好的 Kuscia 集群，按照[部署文档](../../tutorial/run_fate_cn.md)中的流程重新部署。

```shell
crictl rmi secretflow/fate-adapter:0.0.1
crictl rmi secretflow/fate-deploy-basic:0.0.1
```

> `crictl rmi` 需要删除所有前缀下的同名镜像，确保重新导入时不会命中损坏的旧层。

重新部署前建议：

1. 释放磁盘空间，保证至少 10 GB 可用；
2. 确认 Kuscia 节点可以访问镜像仓库，或已准备离线镜像包；
3. 按 [FATE 部署文档](../../tutorial/run_fate_cn.md)重新执行镜像导入与部署；
4. 导入完成后再次执行 `crictl images | grep fate`，确认四个镜像均存在且大小正常。

## 验证

```bash
# Pod 正常启动
kubectl get po -n bob

# FATE 容器日志无镜像/依赖错误
kubectl logs -n bob deployment/fate-deploy-bob --tail=200

# 提交 FATE 任务验证
kubectl get kj -n cross-domain
```

Pod 进入 `Running` 且 FATE 任务可以正常提交，即表示部署成功。

## 常见原因汇总

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| `ErrImagePull` / `ImagePullBackOff` | 镜像未导入、名称/前缀不匹配 | 核对 `crictl images`，重新导入完整镜像 |
| `no space left on device` | 磁盘空间不足 | 清理镜像与日志，扩容磁盘 |
| `exec format error` | 镜像架构与节点不一致 | 导入对应架构镜像 |
| `CrashLoopBackOff` | 镜像完整但依赖缺失/配置错误 | 查看容器日志，按 FATE 部署文档检查配置 |
| Pod `Pending` | 节点资源不足或调度失败 | `kubectl describe po` 查看事件，扩容或调整资源 |

## 相关文档

- [在 Kuscia 上运行 FATE 任务](../../tutorial/run_fate_cn.md)
- [任务失败排查](../runtask/run_job_failed.md)
- [部署失败排查](./deploy_failed.md)
- [部署要求](../../deployment/deploy_check.md)
