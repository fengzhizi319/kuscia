# 内核参数

Kuscia 运行小任务时对内核参数要求不高，但是在运行一些比较大的任务或者任务并发量较高时，如果内核参数配置的不太合理，会非常容易导致任务失败（常见的表现是网络通讯失败）。

Kuscia 内置了对内核参数的检查，如果内核参数不满足 Kuscia 推荐的高并发配置，会在节点实例的 `Node` 上进行 Condition 标记。日常运维时，可以通过该标记来排查任务失败是否可能是内核参数不满足要求导致的。

## 检查 Node Condition

示例说明：

```bash
# Execute inside the Kuscia Master/Autonomy container
kubectl get node <node-instance-id> -o yaml
```

如果返回如下的示例，代表内核参数不符合 Kuscia 的推荐配置，可以参考脚本 [script/deploy/set_kernel_params.sh](https://github.com/secretflow/kuscia/blob/main/scripts/deploy/set_kernel_params.sh) 来进行内核参数的调整：

> 注： 因为内核参数在容器内无法调整，需要在宿主机上进行调整（且需要 `root` 权限才可以调整）

```text
  - lastHeartbeatTime: "2024-07-03T06:35:16Z"
    lastTransitionTime: "2024-07-03T06:35:16Z"
    message: tcp_max_syn_backlog=1024[ERR];somaxconn=4096[OK];tcp_retries2=15[ERR];tcp_slow_start_after_idle=1[ERR];tcp_tw_reuse=2[OK];file-max=913998[OK]
    reason: Kernel parameters not satisfy kuscia recommended requirements
    status: "False"
    type: Kernel-Params
```

如果 `status: "True"` 代表宿主机内核参数符合 Kuscia 的推荐内核参数配置，不再需要调整。

## 推荐参数

Kuscia 检查的关键参数与推荐值：

| 参数 | 推荐值 | 作用 |
| --- | --- | --- |
| `net.ipv4.tcp_max_syn_backlog` | `2048` | SYN 队列长度，影响高并发建连 |
| `net.core.somaxconn` | `2048` | accept 队列长度 |
| `net.ipv4.tcp_retries2` | `5` | TCP 重传次数，默认 15 会导致故障感知过慢 |
| `net.ipv4.tcp_slow_start_after_idle` | `0` | 空闲连接恢复后不重新慢启动 |
| `net.ipv4.tcp_tw_reuse` | `1` | 复用 TIME_WAIT 连接 |
| `fs.file-max` | `102400` 或更高 | 系统级最大文件句柄数 |

## 调整参数

在**宿主机**上以 root 执行：

```bash
sudo tee /etc/sysctl.d/99-kuscia.conf >/dev/null <<'EOF'
net.ipv4.tcp_max_syn_backlog = 2048
net.core.somaxconn = 2048
net.ipv4.tcp_retries2 = 5
net.ipv4.tcp_slow_start_after_idle = 0
net.ipv4.tcp_tw_reuse = 1
fs.file-max = 102400
EOF

sudo sysctl --system
```

也可以使用仓库中的脚本：

```bash
sudo bash scripts/deploy/set_kernel_params.sh
```

## 验证

```bash
# 宿主机上确认参数已生效
sysctl net.ipv4.tcp_max_syn_backlog net.core.somaxconn net.ipv4.tcp_retries2 \
       net.ipv4.tcp_slow_start_after_idle net.ipv4.tcp_tw_reuse fs.file-max

# 在节点容器内确认 Node Condition 变为 True
kubectl get node <node-instance-id> -o jsonpath='{.status.conditions[?(@.type=="Kernel-Params")]}'
```

Condition 的 `status` 为 `"True"` 后，即可重新提交之前失败的任务验证。

## 注意事项

- 修改 `tcp_retries2` 会缩短故障连接的重试时间，请结合业务对网络抖动的容忍度评估。
- 修改后建议重启 Docker / Kubelet，确保新连接使用新的参数。
- 生产环境请把参数写入配置管理或初始化脚本，避免节点重启后丢失。
- 相关文件句柄问题还可参考 [Create fsnotify failed](../deployment/create_fsnotify_failed.md)。

## 相关文档

- [部署要求](../../deployment/deploy_check.md)
- [K8s 线程限制问题](./k8s_ulimit_check.md)
- [Create fsnotify failed](../deployment/create_fsnotify_failed.md)
- [任务失败排查](../runtask/run_job_failed.md)
