# 安全加固方案

## 说明

在面对不可知任务场景或未知代码执行的场景中，建议使用进程安全沙箱 [NsJail](https://github.com/google/nsjail) 保障运行时安全以及使用安全容器 [Kata](https://github.com/kata-containers/kata-containers) 保障容器安全。

SecretFlow 支持通过 NsJail 启动，这使得 SecretFlow 任务可以在安全沙箱中运行，增强任务运行时的安全性。

## 威胁模型

Kuscia 默认的 `runc` 运行时会在节点内启动任务容器。任务镜像由参与方提供时，恶意代码可能尝试：

- 逃逸容器、访问宿主机文件或 Docker socket；
- 读取同节点其他任务的数据；
- 发起横向网络扫描或对外连接。

对应地，可以从两个层面加固：

| 层面 | 方案 | 目标 |
| --- | --- | --- |
| 进程 | NsJail | 限制任务进程的系统调用、文件系统与权限 |
| 容器 | Kata Containers | 使用轻量虚拟机隔离容器，降低内核逃逸风险 |

两者可以叠加使用；至少应在不可信任务场景启用其中一种。

## 前提条件

- 已完成 Kuscia 部署，节点可以正常运行 SecretFlow 任务；
- Docker / containerd 已安装并可切换运行时；
- 使用 NsJail 时需要以特权方式启动 Kuscia 任务容器（`ALLOW_PRIVILEGED=true`）；
- 使用 Kata 时需要在宿主机安装 Kata Containers，并配置对应的 runtime class。

> **安全提示**：开启容器特权会带来容器逃逸风险，生产环境应优先使用 Kata 等安全容器，并限制节点上的任务来源。

## 在安全沙箱中运行 SecretFlow 任务

基于 Kuscia 框架， 您可以快速体验在安全沙箱中运行 SecretFlow 任务，这里以部署中心化组网模式为例，关于更多的部署细节请参考[快速入门](../getting_started/quickstart_cn.md)。

1. 配置 Kuscia 镜像，建议固定到具体版本，避免使用浮动的 `latest`：

    ```bash
    export KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0
    ```

2. 获取 Kuscia 安装脚本，安装脚本会下载到当前目录：

    ```bash
    docker pull ${KUSCIA_IMAGE} && docker run --rm -v $(pwd):/tmp/kuscia ${KUSCIA_IMAGE} cp -f /home/kuscia/scripts/deploy/kuscia.sh /tmp/kuscia
    ```

3. 开启特权配置，并以中心化组网模式启动集群（开启容器特权时，会带来容器逃逸的风险，建议使用安全容器 kata)：

    ```bash
    # 开启特权配置，使得 Kuscia 可以通过特权方式启动 Secretflow 容器。
    export ALLOW_PRIVILEGED=true

    # 启动集群，会拉起 3 个 docker 容器，包括一个控制平面 master 和两个 Lite 节点 alice 和 bob。
    ./kuscia.sh center
    ```

4. 执行作业：

    ```bash
    # 登入 master 容器。
    docker exec -it ${USER}-kuscia-master bash

    # 创建并启动作业（两方 PSI 任务），作业会在安全沙箱中执行。
    scripts/user/create_example_job.sh NSJAIL_PSI

    # 查看作业状态。
    kubectl get kj -n cross-domain
    ```

    `NSJAIL_PSI` 会使用 `secretflow-nsjail-image` AppImage，其启动命令为 `.nsjail/run.sh`。

## 验证沙箱生效

```bash
# 查看任务 Pod
kubectl get po -n cross-domain

# 查看任务容器内的进程树，应能看到 nsjail 作为父进程
kubectl exec -n cross-domain <pod-name> -- ps -ef

# 查看 NsJail 配置（路径以镜像内实际为准）
kubectl exec -n cross-domain <pod-name> -- sh -c 'find / -path "*.nsjail/run.sh" 2>/dev/null'
```

如果进程树中任务进程由 `nsjail` 拉起，且无法访问未挂载的宿主机路径，说明沙箱已生效。

## 强化建议

- **最小挂载**：任务只挂载必要的 DomainData 与配置，避免挂载宿主机目录、Docker socket、`/proc` 等敏感路径；
- **最小网络**：通过 NetworkPolicy 或主机防火墙限制任务出网范围，只放行 DataMesh / 对端网关；
- **只读根文件系统**：在自定义 AppImage 中设置只读根文件系统，并显式挂载可写目录；
- **资源限制**：为容器设置 CPU / 内存上限，RunP 模式不支持 job 资源限制，需在节点层控制；
- **镜像可信**：使用私有仓库与镜像签名，禁止任务拉取未审核镜像；
- **最小权限**：任务容器默认以非 root 运行，移除不必要的能力（`cap_drop: ALL`）；
- **审计**：记录任务提交、镜像来源与数据访问，便于事后追溯。

## 注意事项

- NsJail 与 Kata 都需要宿主机额外配置，部署前请先在测试环境验证兼容性；
- 启用 Kata 后，任务镜像与节点架构必须匹配，且启动时间会变长；
- `ALLOW_PRIVILEGED=true` 只应授予可信节点，并配合节点隔离使用；
- 安全加固不能替代数据授权与最小权限原则，DomainDataGrant 仍需按需配置。

## 相关文档

- [Kuscia 架构](../reference/architecture_cn.md)
- [AppImage 参考](../reference/apis/appimage_cn.md)
- [使用 KusciaAPI 运行 SecretFlow 作业](./run_sf_job_with_api_cn.md)
- [Kuscia 部署要求](../deployment/deploy_check.md)
- [NsJail 项目](https://github.com/google/nsjail)
- [Kata Containers 项目](https://github.com/kata-containers/kata-containers)
