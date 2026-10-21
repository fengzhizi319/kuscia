# 如何通过 Docker 命令对已部署的节点进行 Memory 扩容

## 背景

在使用脚本部署 Kuscia 时，可以使用 `-m` 或者 `--memory-limit` 参数给节点容器设置适当的内存限制。

例如:

- `-m 4GiB` 或 `--memory-limit=4GiB` 表示限制最大内存 4GiB。
- `-m -1` 或 `--memory-limit=-1` 表示没有限制。
- 不设置时默认 master 为 2GiB，lite 节点 4GiB，autonomy 节点 6GiB。

如果您的节点已经部署好，但内存限制不符合需求，您可以通过 Docker 命令对已部署的节点进行 Memory 扩容。

> 适用场景：使用 Docker 部署的 Master / Lite / Autonomy 节点。Kubernetes 部署请直接调整 Pod 的 `resources.limits.memory`。

## 前置条件

- 能够访问节点所在宿主机，并具有执行 `docker` 命令的权限；
- 已知目标节点的容器名，例如中心化模式下的 `${USER}-kuscia-master`、`${USER}-kuscia-lite-alice`；
- 宿主机剩余内存足够支撑新的限制值，避免扩容后触发宿主机 OOM。

## 步骤

1. 查看节点容器的当前内存限制：

    ```bash
    docker inspect ${container_name} --format '{{.HostConfig.Memory}}'
    ```

    返回值单位为字节。`0` 表示未设置限制；例如返回 `4294967296` 表示当前限制为 4GiB。

2. 确认容器当前的内存使用量，评估扩容幅度：

    ```bash
    docker stats --no-stream ${container_name}
    ```

3. 根据需要调整内存限制。例如，如果您需要增加内存限制到 20GiB，您可以运行以下命令：

    ```bash
    docker update ${container_name} --memory=20GiB --memory-swap=20GiB
    ```

    `--memory-swap` 建议与 `--memory` 保持一致，表示容器只能使用 20GiB 内存、不额外使用 swap。

4. 验证调整结果：

    ```bash
    docker inspect ${container_name} --format '{{.HostConfig.Memory}} {{.HostConfig.MemorySwap}}'
    ```

    输出的两个数值应分别等于 `20GiB` 与 `20GiB` 对应的字节数。

<span style="color: red;">注：</span> 使用 `docker update` 扩展内存时，如果 Kuscia 以 `runtime: runc` 方式部署时，需要重启
Kuscia 后，内存调整的限制才能对任务调度实际生效。

参考重启命令：

```bash
docker restart ${container_name}
```

重启后可通过下面的命令确认节点恢复：

```bash
docker ps --filter name=${container_name}
docker exec -it ${container_name} kubectl get nodes
```

`runk` / `runp` 模式下任务不依赖 Docker 为每个 Pod 单独创建容器，内存限制通常在任务运行时由 Kuscia 读取并应用；若扩容后新任务仍未生效，同样建议重启节点容器。

## 回滚

如需恢复原限制，重复上述 `docker update` 命令并传入原值即可。例如恢复为 4GiB：

```bash
docker update ${container_name} --memory=4GiB --memory-swap=4GiB
```

## 相关文档

- [部署要求](../../deployment/deploy_check.md)
- [Kuscia 配置文件](../../deployment/kuscia_config_cn.md)
- [Docker 多机部署中心化集群](../../deployment/Docker_deployment_kuscia/deploy_master_lite_cn.md)
- [Kuscia 部署指引](../../deployment/kuscia_deployment_instructions.md)
