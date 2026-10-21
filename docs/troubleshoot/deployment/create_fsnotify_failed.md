# Kuscia 启动失败：Create fsnotify failed

## 问题描述

在 Kuscia 部署时 Lite 或者 Autonomy 节点启动失败，执行 `docker logs xxxx` 或者查看 Kuscia.log 日志打印如下报错：

```bash
WARN[2024-07-18T18:50:54.907210832+08:00] failed to load plugin io.containerd.grpc.v1.cri  error="failed to create CRI service: failed to create cni conf monitor for default: failed to create fsnotify watcher: too many open files"
```

该报错表示容器内的 containerd 无法创建 inotify 监听器，通常是宿主机的 inotify 实例数或文件句柄数耗尽。

## 原因分析

Kuscia 在 Lite / Autonomy 节点内会启动 containerd、K3s 等组件，这些组件会监听大量文件和目录。以下内核参数过小时会触发该问题：

- `fs.inotify.max_user_instances`：一个用户可以同时使用的 inotify 实例数；
- `fs.inotify.max_user_watches`：一个用户可以同时监视的最大文件数；
- `fs.inotify.max_queued_events`：内核队列中等待处理的事件最大数量；
- `fs.file-max` / `ulimit -n`：系统与进程可打开的文件句柄数。

## 解决方案

### 1. 查看当前配置

```bash
# 注意：以下命令读取的是具体数值
cat /proc/sys/fs/inotify/max_user_instances
cat /proc/sys/fs/inotify/max_user_watches
cat /proc/sys/fs/inotify/max_queued_events

sysctl fs.inotify.max_user_instances fs.inotify.max_user_watches fs.inotify.max_queued_events
```

常见默认值如下（不同发行版可能不同）：

```text
fs.inotify.max_queued_events = 16384
fs.inotify.max_user_instances = 128
fs.inotify.max_user_watches = 8192
```

### 2. 临时调大参数

出现上述报错时，推荐通过执行如下命令扩大一个用户能同时使用的 inotify 最大实例数来解决：

```bash
sudo sysctl fs.inotify.max_user_instances=8192
sudo sysctl fs.inotify.max_user_watches=524288
```

临时修改立即生效，但重启后会丢失。

### 3. 持久化配置

```bash
sudo tee /etc/sysctl.d/99-kuscia.conf >/dev/null <<'EOF'
fs.inotify.max_user_instances = 8192
fs.inotify.max_user_watches = 524288
fs.inotify.max_queued_events = 32768
EOF

sudo sysctl --system
```

### 4. 检查文件句柄限制

```bash
ulimit -n
cat /proc/sys/fs/file-max
```

如果 `ulimit -n` 偏小（例如 1024），请在 `/etc/security/limits.conf` 或 systemd 服务中调整，并重新登录或重启 Docker：

```bash
sudo tee -a /etc/security/limits.d/99-kuscia.conf >/dev/null <<'EOF'
* soft nofile 65536
* hard nofile 65536
EOF
```

### 5. 重启节点

```bash
docker restart ${USER}-kuscia-autonomy-alice
docker logs -f ${USER}-kuscia-autonomy-alice
```

## 验证

```bash
# 参数已生效
sysctl fs.inotify.max_user_instances fs.inotify.max_user_watches

# 容器内 containerd 正常启动
docker exec -it ${USER}-kuscia-autonomy-alice crictl info >/dev/null && echo "containerd ok"

# Kuscia 组件正常
docker exec -it ${USER}-kuscia-autonomy-alice kubectl get po -A
```

节点不再反复重启，且 `kubectl get po -A` 能正常返回，即表示问题已解决。

## 预防措施

- 在部署前把 inotify 与文件句柄参数写入 `/etc/sysctl.d/` 和 `/etc/security/limits.d/`；
- 单机运行多个 Kuscia 节点时，按节点数量放大 `max_user_instances`；
- 将上述检查加入[部署前自检](../../deployment/deploy_check.md#部署前自检)。

## 相关文档

- [部署要求](../../deployment/deploy_check.md)
- [内核参数](../parameter_tuning/kernel_params.md)
- [K8s 线程限制问题](../parameter_tuning/k8s_ulimit_check.md)
- [部署失败排查](./deploy_failed.md)
