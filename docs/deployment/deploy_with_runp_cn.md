# 使用 RunP 模式部署节点

## 前言

本教程帮助你使用进程运行时（RunP）来部署节点（Lite 和 Autonomy），当前本方案仅支持 SecretFlow 引擎，对更多算法引擎的支持敬请期待。

关于 RunP 以及更多运行时的介绍请参考 [容器运行模式](../reference/architecture_cn.md#agent)。

**⚠️ 资源限制说明**：Kuscia RunP 模式暂不支持 job 资源限制。

RunP 的特点：

- 任务不经过容器运行时，直接以**节点内进程**启动，因此节点镜像中必须包含任务所需的运行环境；
- 适合无法授予容器特权、或希望减少容器层开销的环境；
- 任务之间共享节点镜像环境，缺少容器级隔离，建议一个节点只运行可信任务；
- 在 K8s 上使用 RunP 时，任务运行在 Kuscia Pod 内，因此 `kuscia-secretflow` 镜像必须包含 SecretFlow 及其依赖。

## 前提条件

- 已阅读[部署要求](./deploy_check.md)，节点资源满足任务峰值需求。
- 使用包含 SecretFlow 运行环境的 `kuscia-secretflow` 镜像，或自行构建的等价镜像，构建方法见[构建 kuscia-secretflow 镜像](../development/build_kuscia_cn.md#构建-kuscia-secretflow-image)。
- 仅提交 SecretFlow 引擎任务；其他引擎暂不支持。
- 如需非 root 运行，参考[非 root 用户部署](./Docker_deployment_kuscia/docker_deploy_kuscia_with_rootless.md)（仅 RunP 生效）。

## 部署流程

物理机和 K8s 两种部署环境的配置不同，接下来我们将分别介绍在这两种环境中 RunP 模式的部署流程。

### 在物理机上部署

完整的详细流程请参考 [多机部署中心化集群](./Docker_deployment_kuscia/deploy_master_lite_cn.md) 和 [多机部署点对点集群](./Docker_deployment_kuscia/deploy_p2p_cn.md)。

其中，使用 RunP 部署的不同点是：

需要在生成配置文件时，使用 --runtime 参数指定运行时为 `runp`。

```bash
# Generate the configuration file and specify the runtime as runp
docker run -it --rm ${KUSCIA_IMAGE} kuscia init --mode autonomy --domain "alice" --runtime "runp" > autonomy_alice.yaml 2>&1 || cat autonomy_alice.yaml
```

确认配置中的运行时：

```bash
grep -n '^runtime:' autonomy_alice.yaml
# 期望输出：runtime: runp
```

然后按普通 Docker 部署流程启动：

```bash
./kuscia.sh start -c autonomy_alice.yaml -p 11080 -k 11081
```

如果希望以当前非 root 用户运行，追加 `--rootless`：

```bash
./kuscia.sh start -c autonomy_alice.yaml -p 11080 -k 11081 --rootless
```

> **镜像说明**：RunP 任务依赖 Kuscia 镜像内的运行环境。请确认所使用的镜像包含 SecretFlow，或已通过 `kuscia image` 导入内置 RunP 镜像。可使用 `kuscia-secretflow` 镜像作为 `KUSCIA_IMAGE`。

### 在 K8s 集群上部署

完整的详细流程请参考 [K8s 部署中心化集群](./K8s_deployment_kuscia/K8s_master_lite_cn.md) 和 [K8s 部署点对点集群](./K8s_deployment_kuscia/K8s_p2p_cn.md)。

其中，使用 RunP 部署的不同点是：

1. 修改 [configmap.yaml](https://github.com/secretflow/kuscia/blob/main/hack/k8s/autonomy/configmap.yaml) （以 autonomy 为例）文件中 Kuscia 的 `runtime` 配置为 `runp`，
   以及填充模板变量，例如 `{{.DOMAIN_ID}}`、`{{.DOMAIN_KEY_DATA}}` 等。 `runk` 相关的配置不需要关注，包括不需要配置 `kubeconfigFile` 以及不需要创建 `rbac.yaml`。

   ```yaml
   runtime: runp

   # runk configuration does not need to be concerned
   runk:
      namespace:
      dnsServers:
      kubeconfigFile:
   ```

2. 创建 [deployment](https://github.com/secretflow/kuscia/blob/main/hack/k8s/autonomy/deployment.yaml) 时修改镜像为 `kuscia-secretflow` 镜像，下面是一个示例镜像地址，您可以根据实际需求参考[这里](../development/build_kuscia_cn.md#构建-kuscia-secretflow-image)自行编译。此外，`automountServiceAccountToken` 字段可以设置为 `false`。

   ```yaml
      spec:
        template:
          spec:
            containers:
              - image: secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-secretflow:1.2.0
            # Can be set to false to reduce dependency on K8s cluster permissions (optional)
            automountServiceAccountToken: false
   ```

3. 按 K8s 部署文档创建 Namespace、ConfigMap、Deployment 与 Service，并等待 Pod 就绪：

   ```bash
   kubectl get po -n autonomy-alice
   kubectl logs -n autonomy-alice deployment/autonomy-alice --tail=100
   ```

> **说明**：RunK 模式不支持使用本地数据训练；如需使用本地数据，请切换到 RunP 模式。RunP 任务在 Kuscia Pod 内运行，数据挂载与 Pod 的 Volume 配置保持一致。

## 验证

1. 检查运行时配置已生效：

   ```bash
   # Docker
   docker exec -it ${USER}-kuscia-autonomy-alice cat /home/kuscia/etc/conf/kuscia.yaml | grep '^runtime:'

   # K8s
   kubectl get cm -n autonomy-alice -o yaml | grep 'runtime:'
   ```

2. 提交一个 SecretFlow 任务，确认任务进入 `Succeeded`：

   - 通过 KusciaAPI 提交：[使用 KusciaAPI 运行 SecretFlow 作业](../tutorial/run_sf_job_with_api_cn.md)
   - 通过 SecretPad 提交：[快速体验](../getting_started/quickstart_cn.md)

3. 任务运行期间查看进程：

   ```bash
   # Docker：任务进程位于 Kuscia 容器内
   docker exec -it ${USER}-kuscia-autonomy-alice ps -ef | grep -E 'secretflow|python'

   # K8s：任务进程位于 Kuscia Pod 内
   kubectl exec -n autonomy-alice deployment/autonomy-alice -- ps -ef | grep -E 'secretflow|python'
   ```

4. 检查任务日志与资源占用。由于 RunP 不支持 job 资源限制，请关注节点整体 CPU / 内存，避免多个任务互相挤占。

## 常见问题

| 现象 | 可能原因 | 处理方式 |
| --- | --- | --- |
| 任务启动即失败，提示找不到依赖 | 镜像不含 SecretFlow 运行环境 | 使用 `kuscia-secretflow` 镜像或补齐依赖 |
| 任务 OOM / CPU 争抢 | RunP 不支持 job 资源限制 | 控制并发、拆分节点或改用 RunC / RunK |
| 非 SecretFlow 任务无法运行 | RunP 仅支持 SecretFlow | 切换到 RunC / RunK，或等待后续支持 |
| K8s 下 Pod 启动失败 | ConfigMap 未改为 `runp`，或仍配置了 `runk` 的 kubeconfig | 按上文修改 ConfigMap，移除 runk 相关配置 |
| 文件权限错误 | 非 root 运行且数据目录属主不匹配 | 调整目录属主，参考[非 root 部署](./Docker_deployment_kuscia/docker_deploy_kuscia_with_rootless.md) |

## 相关文档

- [容器运行模式](../reference/architecture_cn.md#agent)
- [Kuscia 部署指引](./kuscia_deployment_instructions.md)
- [非 root 用户部署](./Docker_deployment_kuscia/docker_deploy_kuscia_with_rootless.md)
- [构建 kuscia-secretflow 镜像](../development/build_kuscia_cn.md#构建-kuscia-secretflow-image)
- [部署要求](./deploy_check.md)
