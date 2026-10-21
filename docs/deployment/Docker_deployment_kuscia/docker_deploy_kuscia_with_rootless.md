# 非 root 用户部署节点

## 前言

本教程将帮助您在权限受限制的环境中，以非 root 用户来部署 Kuscia 集群。

Kuscia 容器默认以 root 运行。启用 `--rootless` 后，容器会以**当前宿主机用户的 UID** 启动，避免容器内进程持有 root 权限。该能力依赖 RunP 运行时：Lite / Autonomy 节点使用 RunP 时任务以节点内进程运行，不需要容器内的特权操作。

## 前提条件

- 宿主机当前用户**不是 root**；如果以 root 执行，`--rootless` 会被忽略。
- Lite / Autonomy 节点的运行时必须为 `runp`；`runc` 运行时下 `--rootless` 不生效。
- 当前用户有权限访问 Docker daemon（或已配置 rootless Docker）。
- 数据、日志、K3s 数据目录对当前用户可写。
- 已按[部署要求](../deploy_check.md)完成环境检查，并阅读 [RunP 部署](../deploy_with_runp_cn.md)的限制。

## 部署

您可以参考[这里](./deploy_p2p_cn.md)了解如何使用 Docker 部署 Kuscia，本文不做过多赘述。

启动 Kuscia 时，非 root 用户只需在启动命令后面加上 `--rootless` 即可。示例如下：

```bash
./kuscia.sh start -c autonomy_alice.yaml -p 11080 -k 11081 --rootless
```

如果是首次部署，还需要确保配置中的运行时为 `runp`：

```bash
docker run -it --rm ${KUSCIA_IMAGE} kuscia init \
  --mode autonomy --domain alice --runtime runp > autonomy_alice.yaml
```

中心化模式下，Master 与 Lite 都可以使用 `--rootless`：

```bash
# Master
./kuscia.sh start -c kuscia_master.yaml -p 18080 -k 18081 --rootless

# Lite（运行时必须为 runp）
./kuscia.sh start -c lite_alice.yaml -p 28080 -k 28081 --rootless
```

启用后，Kuscia 的工作目录会带 `-rootless` 后缀（例如 `root-kuscia-autonomy-alice-rootless`），避免与 root 部署的数据目录混用。

## 验证

```bash
# 容器内进程用户应为宿主机 UID，而不是 root
docker exec -it ${USER}-kuscia-autonomy-alice whoami
docker exec -it ${USER}-kuscia-autonomy-alice id

# 容器状态
docker ps --filter name=${USER}-kuscia-autonomy-alice
```

`whoami` 返回当前宿主机用户名（或 UID），即表示 rootless 生效。

## 常见问题

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 容器内仍是 root | 以 root 用户执行，或运行时为 `runc` | 切换到普通用户，并将运行时改为 `runp` |
| 启动时报目录无权限 | 数据/日志目录属于 root | `sudo chown -R $(id -u):$(id -g) <data-dir>` |
| 端口绑定失败（<1024） | 使用 rootless Docker 且未放开特权端口 | `sudo sysctl net.ipv4.ip_unprivileged_port_start=0`，或改用高端口 |
| 任务启动失败 | RunP 依赖节点镜像内的运行环境 | 检查 Kuscia 镜像是否内置 SecretFlow/任务依赖，见 [RunP 部署](../deploy_with_runp_cn.md) |
| 升级后权限异常 | 升级继承了原容器的用户配置 | 使用 `docker inspect` 核对 `Config.User`，必要时重新以 `--rootless` 部署 |

:::{tip}

1. 目前仅支持 RunP 模式以非 root 用户来部署 Kuscia。
2. 如果主机用户是 root，Kuscia 还是以 root 用户启动，`--rootless` 不生效。如果主机用户不是 root，Kuscia 以主机用户启动。
3. 更多部署要求请参考[这里](../deploy_check.md)。

:::

## 相关文档

- [Kuscia 部署指引](../kuscia_deployment_instructions.md)
- [RunP 部署](../deploy_with_runp_cn.md)
- [部署要求](../deploy_check.md)
- [部署失败排查](../../troubleshoot/deployment/deploy_failed.md)
