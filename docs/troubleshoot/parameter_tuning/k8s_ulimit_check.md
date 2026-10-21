# Kuscia K8s 部署模式下 SecretFlow 应用线程限制问题

## 问题描述

在 Kuscia K8s 部署模式下，SecretFlow 应用运行过程中出现创建线程失败问题：

```text
pthread_create failed: Resource temporarily unavailable
```

Pod 可能反复重启，或在任务日志中看到 Ray / Python 线程创建失败的堆栈。

## 原因分析

K8s 集群 Kubelet 组件的 `podPidsLimit` 参数被限制为某个特定值，而 SecretFlow 默认启动线程数为：物理机 CPU * 32 + Ray 的线程数 ，如果 SecretFlow 运行启动的线程数超过了 Kubelet 限制的线程数时则会出现以上报错。

在线程实现中，每个线程对应一个 PID；因此 `pids.max` 达到上限时，`pthread_create` 会返回 `EAGAIN`。此外，以下限制也可能叠加导致问题：

- 容器内 `ulimit -n`（nofile）过小；
- 节点 `fs.file-max` / `fs.inotify.*` 不足；
- SecretFlow 与 Ray 的线程数随 CPU 核数增长，大规格节点更容易触发。

## 排查

```bash
# 1. 查看 Kubelet 的实际启动参数
ps -ef | grep kubelet | tr ' ' '\n' | grep -E 'config|pids'

# 2. 查看 Kubelet 配置中的 podPidsLimit
sudo grep -n 'podPidsLimit' /var/lib/kubelet/config.yaml

# 3. 查看 Pod 内的 pids 上限与文件句柄
kubectl exec -n <namespace> <pod-name> -- sh -c 'cat /sys/fs/cgroup/pids.max; ulimit -n'

# 4. 查看节点事件与 Pod 重启原因
kubectl describe po -n <namespace> <pod-name>
```

如果 `pids.max` 明显小于 SecretFlow 需要的线程数（例如只有 1024），即可确认是本问题。

## 解决方案

1、找到 kubelet.service 的配置文件 /var/lib/kubelet/config.yaml（此处以官网推荐路径为例），修改 `podPidsLimit` 参数，例如 `podPidsLimit: -1`。详情参考 [K8s 官方文档](https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/#kubelet-config-k8s-io-v1beta1-KubeletConfiguration)。

```yaml
# /var/lib/kubelet/config.yaml
podPidsLimit: -1
```

> 生产环境不建议直接使用 `-1`（无限制）。可以根据节点规格设置为一个足够大的有限值，例如 `65536`，避免单个 Pod 耗尽节点 PID。

2、修改后重启 Kubelet 服务

```bash
sudo systemctl daemon-reload
sudo systemctl restart kubelet.service
```

3、如果同时存在文件句柄限制，调整 Kubelet 的 `LimitNOFILE`：

```bash
sudo mkdir -p /etc/systemd/system/kubelet.service.d
sudo tee /etc/systemd/system/kubelet.service.d/20-kuscia-limits.conf >/dev/null <<'EOF'
[Service]
LimitNOFILE=1048576
EOF

sudo systemctl daemon-reload
sudo systemctl restart kubelet.service
```

4、节点级内核参数也需要满足要求，见[内核参数](./kernel_params.md)。

## 验证

```bash
# 新 Pod 的 pids 上限已放开
kubectl exec -n <namespace> <pod-name> -- cat /sys/fs/cgroup/pids.max

# Kubelet 配置已生效
sudo grep -n 'podPidsLimit' /var/lib/kubelet/config.yaml

# 重新提交 SecretFlow 任务，确认不再出现 pthread_create 失败
kubectl logs -n <namespace> <pod-name> --tail=200 | grep -i pthread
```

## 注意事项

- 修改 Kubelet 配置会影响节点上的所有 Pod，请在维护窗口执行。
- 如果使用托管 K8s（ACK、TKE 等），`podPidsLimit` 可能由控制台或节点池配置管理，需在云平台侧调整。
- 大规格节点（高 CPU 核数）应同步放大 PID 上限和文件句柄上限。

## 相关文档

- [内核参数](./kernel_params.md)
- [部署要求](../../deployment/deploy_check.md)
- [任务失败排查](../runtask/run_job_failed.md)
- [Create fsnotify failed](../deployment/create_fsnotify_failed.md)
