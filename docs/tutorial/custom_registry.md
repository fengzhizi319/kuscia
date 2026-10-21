# 如何在 Kuscia 中使用自定义镜像仓库

Kuscia支持自动拉取远程的应用镜像（比如：SecretFlow 等），这样可以不用手动导入镜像到容器中。可以在 [Kuscia 配置文件](../deployment/kuscia_config_cn.md)中配置私有（or 公开）镜像仓库地址。

## 适用场景

- 使用企业私有镜像仓库，统一管理 SecretFlow / 算法镜像；
- 使用镜像代理或镜像加速地址；
- 离线环境中通过内网仓库分发镜像；
- 需要在切换仓库时避免逐个修改 AppImage。

## 前提条件

- 节点可以访问目标仓库的 endpoint；
- 已准备好具有拉取权限的账号（公开仓库可省略）；
- AppImage 中的镜像名与仓库配置匹配（见下文表格）；
- 修改配置后需要重启对应 Kuscia 节点。

## 如何配置使用自定义镜像仓库

配置文件中的 `image` 字段用来配置自定义仓库。相关含义参考 [Kuscia 配置文件说明](../deployment/kuscia_config_cn.md)

### 私有镜像仓库

如果有一个私有镜像仓库（示例：`private.registry.com`），对应的配置如下：

```yaml
image:
  # 对应 registries 中的 name
  defaultRegistry: private
  registries:
    - name: private
      endpoint: private.registry.com/test
      username: testname
      password: testpass
```

### 公开镜像仓库

如果使用公开的镜像仓库（示例：`secretflow-registry.cn-hangzhou.cr.aliyuncs.com`），对应的配置如下：

```yaml
image:
  defaultRegistry: aliyun
  registries:
    - name: aliyun
      endpoint: secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow
```

### 字段说明

| 字段 | 说明 |
| --- | --- |
| `defaultRegistry` | 默认使用的仓库名，必须与某个 `registries[].name` 一致 |
| `name` | 仓库标识，可自定义 |
| `endpoint` | 仓库地址，可包含 namespace，例如 `private.registry.com/test` |
| `username` / `password` | 拉取镜像的凭据，公开仓库可省略 |

> 当前每个节点仅支持配置一个镜像仓库。仓库地址与 AppImage 的拼接规则见下一节。

## 关于镜像仓库和AppImage的搭配使用

配置文件中有`image`字段，`AppImage` 中也存在image相关的配置，他们的搭配关系示例如下：

| 配置文件 | AppImage配置 | 实际镜像地址 | 备注 |
| - | - | - | - |
| 无配置 | secretflow/app:v1 | docker.io/secretflow/app:v1 | |
| 无配置 | private.registry.com/secretflow/app:v1 | private.registry.com/secretflow/app:v1 | |
| private.registry.com | secretflow/app:v1 | private.registry.com/app:v1 | |
| private.registry.com/secretflow | app:v1 | private.registry.com/secretflow/app:v1 | 推荐配置 |
| private.registry.com/secretflow | secretflow/app:v1 | private.registry.com/secretflow/app:v1 | |
| private.registry.com/secretflow | test/app:v1 | private.registry.com/secretflow/app:v1 | |
| private.registry.com/secretflow | private.registry.com/secretflow/app:v1 | private.registry.com/secretflow/app:v1 | |
| private.registry.com/secretflow | public.aliyun.com/secretflow/app:v1 | public.aliyun.com/secretflow/app:v1 | 强烈不推荐配置，未来可能会禁止这种配置 |

注：Kuscia推荐在 `AppImage` 中只配置镜像名（不带镜像仓库地址），否则切换仓库的时候，需要批量修改`AppImage`，所以不建议如此配置。

## 应用配置

1. 编辑节点配置文件：

   ```bash
   docker exec -it ${USER}-kuscia-lite-alice vi /home/kuscia/etc/conf/kuscia.yaml
   ```

2. 确认配置格式正确：

   ```bash
   docker exec -it ${USER}-kuscia-lite-alice \
     grep -A8 '^image:' /home/kuscia/etc/conf/kuscia.yaml
   ```

3. 重启节点：

   ```bash
   docker restart ${USER}-kuscia-lite-alice
   ```

4. 提交一个使用该 AppImage 的任务，观察镜像拉取与任务启动。

## 验证

```bash
# 查看节点是否拉取到镜像（runc / runp）
docker exec -it ${USER}-kuscia-lite-alice crictl images | grep <image-name>

# 查看任务 Pod 状态
docker exec -it ${USER}-kuscia-lite-alice kubectl get po -A

# 查看拉取失败的具体原因
docker exec -it ${USER}-kuscia-lite-alice kubectl describe po -n <namespace> <pod-name>
```

任务成功启动且镜像列表中出现目标镜像，说明仓库配置生效。

## 镜像拉取失败

当发现镜像拉取失败时，请确认 配置文件中仓库地址，以及账密相关配置是否正确， 以及参考上文，确保 AppImage 的镜像地址配置正确.

```text
2024-06-06 13:33:00.534 ERROR framework/pod_workers.go:978 Error syncing pod "ant-test-0_ant(7fd5285b-2a5c-4a75-930a-2908e98c8799)", skipping: failed to "StartContainer" for "test" with ErrImagePull: "failed to pull image \"registry.xxxx.com/secretflow/nginx:v1\" with credentials, detail-> rpc error: code = Unknown desc = failed to pull and unpack image \"registry.xxxx.com/secretflow/nginx:v1\": failed to resolve reference \"registry.xxxx.com/secretflow/nginx:v1\": unexpected status from HEAD request to https://registry.xxxx.com/v2/secretflow/nginx/manifests/v1: 401 Unauthorized"
```

常见原因：

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| `401 Unauthorized` | 账号/密码错误或无权拉取 | 核对凭据，确认账号有仓库读权限 |
| `manifest unknown` | 镜像名或 tag 不存在 | 核对 AppImage 与仓库中的路径、tag |
| `x509: certificate signed by unknown authority` | 仓库使用自签证书 | 将 CA 安装到节点信任链，或改用受信任证书 |
| `connection refused` / timeout | 节点无法访问仓库 | 检查 DNS、网络与防火墙 |
| 拉取到错误仓库 | `defaultRegistry` 与 AppImage 前缀冲突 | 按上文表格调整 AppImage，只保留镜像名 |

## 安全建议

- 不要把仓库密码提交到代码仓库；配置文件权限应限制为受信任用户可读；
- 私有仓库建议使用只读机器人账号，并定期轮换凭据；
- 生产环境建议固定镜像 tag 或 digest，避免使用 `latest`；
- 切换仓库后，先在单个 Lite 节点验证，再推广到全部节点。

## 相关文档

- [Kuscia 配置文件](../deployment/kuscia_config_cn.md)
- [AppImage 参考](../reference/apis/appimage_cn.md)
- [Kuscia 部署指引](../deployment/kuscia_deployment_instructions.md)
- [镜像拉取失败排查](../troubleshoot/deployment/deploy_failed.md)
