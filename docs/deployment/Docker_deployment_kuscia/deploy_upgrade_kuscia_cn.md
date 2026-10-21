# 升级 Kuscia 版本

## 前言

升级 Kuscia 会**重建容器**：`kuscia.sh upgrade` 会先删除旧容器，再使用新镜像和原挂载参数重新创建容器。配置、证书和 K3s 数据是否保留，取决于部署时是否把对应目录挂载到了宿主机。因此升级前务必完成备份，并按版本发布说明确认兼容性。

## 升级前检查

1. **确认目标版本**：阅读对应版本的 Release Notes，确认 CRD、配置项和运行时是否有不兼容变更。
2. **确认镜像可获取**：提前拉取新镜像，避免升级过程中因网络问题导致容器无法重建。
3. **确认数据已持久化**：至少确认以下目录已挂载到宿主机：
   - `/home/kuscia/etc/conf/kuscia.yaml`（配置）
   - `/home/kuscia/var/certs`（证书）
   - `/home/kuscia/var/k3s/server/db`（K3s 数据）
   - `/home/kuscia/var/storage/data`（DomainData）
4. **通知协作方**：升级期间该节点的任务调度和跨域通信会短暂中断。

## 备份

```bash
CTR=<container-name>          # 例如 root-kuscia-autonomy-alice
BACKUP_DIR=~/kuscia-backup/$(date +%Y%m%d-%H%M%S)
mkdir -p "${BACKUP_DIR}"

# 容器元数据，便于回滚时核对端口、挂载和网络
docker inspect "${CTR}" > "${BACKUP_DIR}/container-inspect.json"

# 配置与证书
docker cp "${CTR}":/home/kuscia/etc/conf/kuscia.yaml "${BACKUP_DIR}/kuscia.yaml"
docker cp "${CTR}":/home/kuscia/var/certs "${BACKUP_DIR}/certs"

# K3s 数据与 DomainData（路径以 docker inspect 中的挂载源为准）
docker cp "${CTR}":/home/kuscia/var/k3s/server/db "${BACKUP_DIR}/k3s-db"
```

> 如果部署时已将上述目录挂载到宿主机，直接备份宿主机目录即可；容器内的 `docker cp` 仅作为兜底。证书丢失会导致跨域身份失效，恢复需重新签发，见[私钥丢失](../../troubleshoot/deployment/private_key_loss.md)。

## Docker 部署升级

您可以参考 [Kuscia 部署](./deploy_p2p_cn.md)中了解如何获取 Kuscia 脚本，本文不做过多赘述。

1. 指定 Kuscia 使用的镜像版本，这里使用 1.2.0b0 版本。

   ```bash
   export KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0
   ```

2. 建议先显式拉取镜像，确认镜像可用后再执行升级：

   ```bash
   docker pull "${KUSCIA_IMAGE}"
   ```

3. 升级 kuscia 版本只需要加上 upgrade 参数和容器名即可。示例如下：

   ```bash
   ./kuscia.sh upgrade root-kuscia-autonomy-alice
   ```

   中心化模式下需要对 Master 和每个 Lite 分别执行升级：

   ```bash
   ./kuscia.sh upgrade root-kuscia-master
   ./kuscia.sh upgrade root-kuscia-lite-alice
   ./kuscia.sh upgrade root-kuscia-lite-bob
   ```

   建议先升级 Master，再逐个升级 Lite / Autonomy，确认前一个节点恢复后再继续下一个。

4. 观察升级过程：

   ```bash
   docker logs -f root-kuscia-autonomy-alice
   ```

   脚本会在重建容器后探测 Gateway / DataMesh；看到类似 `upgrade successfully` 的输出即表示容器已恢复。

## 升级后验证

```bash
# 容器状态与镜像版本
docker ps --filter name=root-kuscia-autonomy-alice
docker inspect -f '{{.Config.Image}}' root-kuscia-autonomy-alice

# 组件版本
docker exec -it root-kuscia-autonomy-alice bin/kuscia version

# Pod 与路由状态
docker exec -it root-kuscia-autonomy-alice kubectl get po -A
docker exec -it root-kuscia-autonomy-alice kubectl get domainroutes
```

再执行一次[部署验证](../kuscia_deployment_instructions.md#验证部署)中的健康检查和最小任务，确认调度、DataMesh 与跨域通信均正常。

## 回滚

`kuscia.sh upgrade` 会保留原配置、证书和挂载参数，因此回滚只需把镜像改回旧版本后再次执行 upgrade：

```bash
export KUSCIA_IMAGE=<old-kuscia-image>
docker pull "${KUSCIA_IMAGE}"
./kuscia.sh upgrade root-kuscia-autonomy-alice
```

如果新版本已经修改了 CRD 或配置格式，仅回退镜像可能无法启动，此时需要：

1. 停止容器；
2. 用备份恢复 `kuscia.yaml`、`var/k3s/server/db` 与 `var/certs`；
3. 使用旧镜像重新执行 `./kuscia.sh upgrade`；
4. 按[部署验证](../kuscia_deployment_instructions.md#验证部署)确认恢复。

## K8s 部署升级

K8s 部署通过更新镜像版本完成，Master 与各节点仍需逐个升级：

```bash
# 更新镜像并等待滚动完成
kubectl -n kuscia-master set image deployment/kuscia-master kuscia=<new-image>
kubectl -n kuscia-master rollout status deployment/kuscia-master --timeout=10m

# 查看历史版本并回滚
kubectl -n kuscia-master rollout history deployment/kuscia-master
kubectl -n kuscia-master rollout undo deployment/kuscia-master
```

升级前请备份 PVC 中的配置、证书与数据库；如果使用外部 MySQL / PostgreSQL，请按数据库自身的备份流程操作。

:::{tip}
注意：[体验模式](../../getting_started/quickstart_cn.md)因未挂载 Volume 到宿主机磁盘，无法使用以上方法升级 Kuscia。
:::

## 常见问题

| 现象 | 可能原因 | 处理方式 |
| --- | --- | --- |
| 升级后 Pod 一直 `Pending` | 镜像未拉取、资源不足或挂载丢失 | `docker logs` / `kubectl describe po` 查看事件，确认镜像与挂载 |
| 升级后跨域任务失败 | 证书或 K3s 数据未备份，重建后身份丢失 | 恢复 `var/certs` 与 `var/k3s/server/db`，必要时重新授权 |
| 容器反复重启 | 新旧配置不兼容 | 回退镜像并恢复备份配置，阅读 Release Notes 后重试 |
| `upgrade` 提示容器不存在 | 容器名错误或容器已被删除 | `docker ps -a` 确认实际容器名 |

## 相关文档

- [Kuscia 部署指引](../kuscia_deployment_instructions.md)
- [部署要求](../deploy_check.md)
- [私钥丢失](../../troubleshoot/deployment/private_key_loss.md)
- [部署失败排查](../../troubleshoot/deployment/deploy_failed.md)
