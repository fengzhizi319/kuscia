# 部署要求

## 前言

Kuscia 对操作系统、Docker / Kubernetes 版本、处理器架构、内核参数和网络都有要求。本文先给出可直接执行的**部署前自检**，再逐项说明最低要求与生产建议。

- 仅在本机体验 Kuscia：按[快速体验](../getting_started/quickstart_cn.md)准备一台满足最低配置的机器即可。
- 生产或跨机构部署：请逐项完成本文检查，并继续阅读[网络要求](./networkrequirements.md)、[端口说明](./kuscia_ports_cn.md)和[Kuscia 部署指引](./kuscia_deployment_instructions.md)。

## 部署前自检

在每台待部署的宿主机上执行以下命令，确认输出符合预期。

```bash
# 1. 处理器架构：应为 x86_64 或 arm64
uname -m

# 2. 操作系统与内核：Linux 内核应不低于 4.6
uname -srv
cat /etc/os-release | head -5

# 3. 资源：CPU、内存、磁盘
nproc
free -h
df -h /var/lib/docker /home

# 4. Docker（Docker 部署方式）
docker version --format 'client={{.Client.Version}} server={{.Server.Version}}'
docker info --format '{{.Driver}} {{.CgroupDriver}} {{.CgroupVersion}}'

# 5. Kubernetes（K8s 部署方式）
kubectl version --client=true
kubectl get nodes -o wide
kubectl get storageclass

# 6. 内核与文件句柄
sysctl fs.inotify.max_user_instances fs.inotify.max_user_watches
sysctl net.ipv4.ip_forward
ulimit -n

# 7. 时间同步：跨域证书和握手依赖准确的系统时间
timedatectl status
```

自检结果的判定标准：

| 检查项 | 通过标准 | 不通过时的处理 |
| --- | --- | --- |
| 架构 | `x86_64` 或 `arm64` | 更换机器；其他架构不受支持 |
| 内核 | `>= 4.6`，建议 `5.10+` | 升级内核，参考[内核参数](../troubleshoot/parameter_tuning/kernel_params.md) |
| Docker | 服务端 `>= 20.10.24` | 升级 Docker，见下文 |
| K8s | `>= 1.20.0`，节点均为 `Ready` | 升级或修复集群，见下文 |
| inotify | `max_user_instances >= 8192` | `sudo sysctl fs.inotify.max_user_instances=8192` |
| 文件句柄 | `ulimit -n >= 65536` | 调整 `limits.conf` 或 Kubelet `podPidsLimit` |
| 端口 | `1080` 等端口未被占用 | 修改端口映射参数，见[端口说明](./kuscia_ports_cn.md) |
| 时间 | `System clock synchronized: yes` | 配置 chrony / NTP |

## 操作系统要求

支持的操作系统包括：

- **Linux**：CentOS 7 / CentOS 8、Ubuntu 16.04 及以上版本，以及兼容上述发行版的内核
- **MacOS**：仅建议用于本地体验
- **Windows**：通过 [WSL2 上的 Ubuntu](https://docs.microsoft.com/en-us/windows/wsl/install-win10) 运行，Docker 相关注意事项见 [WSL 启动 Docker 失败](../troubleshoot/deployment/wsl_start_docker.md)

Linux 内核版本要求为 **Kernel 4.6 或更高版本**。高并发任务对内核参数更敏感，Kuscia 会在 Node 上标记 `Kernel-Params` Condition；调整方法见[内核参数](../troubleshoot/parameter_tuning/kernel_params.md)。

## 处理器要求

支持的处理器包括：

- **x86_64**（含 `amd64`）
- **arm64**（含 `aarch64`）

使用 `uname -m` 确认架构，并确保所拉取的 Kuscia 镜像与宿主机架构一致。

## Docker 版本要求

推荐使用 **Docker 20.10.24 或更高版本**。Docker 的安装请参考[官方文档](https://docs.docker.com/engine/install/)，Docker 部署包下载参考 [Docker 软件包](https://download.docker.com/linux/centos/7/x86_64/stable/Packages/)。

除版本外，还需确认：

```bash
# 当前用户可以直接访问 Docker（否则命令前需要 sudo）
docker info >/dev/null && echo "docker ok"

# cgroup v2 环境下确认 driver 为 systemd，避免与主机 systemd 冲突
docker info --format 'cgroupDriver={{.CgroupDriver}} cgroupVersion={{.CgroupVersion}}'
```

如果需要以非 root 用户运行，只有当运行时为 RunP 时才支持 `--rootless`，详见[非 root 用户部署节点](./Docker_deployment_kuscia/docker_deploy_kuscia_with_rootless.md)。

## K8s 版本要求

推荐使用 **K8s 1.20.0 或更高版本**。K8s 的安装请参考[官方文档](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/install-kubeadm/)。

K8s 部署前额外确认：

```bash
kubectl get nodes -o wide          # 所有节点 Ready
kubectl get storageclass           # 至少有默认 StorageClass，供 PVC 使用
kubectl auth can-i create pod -n default
```

Kuscia 的任务 Pod 需要运行提权容器，命名空间通常需要启用 `privileged` Pod Security Admission。请使用专用节点承载 Kuscia 任务，并参考 [K8s 部署中心化集群](./K8s_deployment_kuscia/K8s_master_lite_cn.md)中的安全说明。

## MySQL 版本要求

Kuscia 默认使用内置存储即可完成部署与体验。如果配置了 MySQL 作为数据存储，推荐使用 **MySQL 8.0.23 或更高版本**，防止由于小版本（如 8.0.20）Bug 导致的连接异常。

```sql
SELECT VERSION();
SHOW VARIABLES LIKE 'max_connections';
```

连接相关问题可参考 [MySQL 连接数过多](../troubleshoot/parameter_tuning/mysql_too_many_connections.md)和 [Kuscia MySQL 问题](../troubleshoot/deployment/kuscia_mysql_kine.md)。

## 资源要求

部署 Kuscia 的最低资源为：

- **1 核 CPU**
- **2 GB 内存**
- **20 GB 硬盘**

为了确保良好的性能，建议的系统资源为：

- **8 核 CPU**
- **16 GB 内存**
- **200 GB 硬盘**

不同角色的资源规划：

| 场景 | CPU | 内存 | 磁盘 | 说明 |
| --- | --- | --- | --- | --- |
| 体验 / 单节点 | 1 核 | 2 GB | 20 GB | 仅用于功能验证，不适合承载真实任务 |
| 生产 Master | 4 核起 | 8 GB 起 | 100 GB 起 | 保存控制面数据、证书与任务元数据 |
| 生产 Lite / Autonomy | 8 核起 | 16 GB 起 | 200 GB 起 | 同时运行 Kuscia 组件与任务容器 |
| RunP 模式 | 按任务峰值预留 | 按任务峰值预留 | 按数据规模预留 | 任务以节点内进程运行，跳过容器运行时，**不支持 job 资源限制**，见[RunP 部署](./deploy_with_runp_cn.md) |

任务容器自身的资源由 AppImage 的 `resource_requirements` 控制，见 [AppImage 参考](../reference/apis/appimage_cn.md#resourcerequirements)。磁盘规划时需同时考虑镜像层、`/var/lib/docker`（或 K3s 数据目录）、DomainData 挂载目录和日志目录。

## 网络要求

节点之间至少需要保证**跨域网关端口 1080** 互通；如果使用中心化模式，Lite 节点还需要能访问 Master 的 KusciaAPI。端口与暴露对象的完整说明见[Kuscia 端口介绍](./kuscia_ports_cn.md)。

如果节点入口网络存在网关、NAT 或代理，需要满足[网络要求](./networkrequirements.md)中的限制；否则可能出现任务卡在 `Pending`、DomainRoute 握手失败或 DataMesh 读取超时。

常见防火墙配置示例：

```bash
# firewalld
sudo firewall-cmd --permanent --add-port=1080/tcp
sudo firewall-cmd --reload

# ufw
sudo ufw allow 1080/tcp
```

> **注意**：Docker 发布端口时会写入 `DOCKER-USER` 链，可能绕过宿主机的 firewalld / ufw 规则。生产环境请通过云安全组、`DOCKER-USER` 链或仅绑定内网地址（如 `127.0.0.1:8082:8082`）限制访问，不要仅依赖主机防火墙。

跨域部署还应确认：

- 双方系统时间同步，证书未过期；
- 代理或网关允许 HTTP/HTTPS 长连接与双向 TLS；
- 节点之间 MTU 一致，避免大包被丢弃；
- DNS 可解析对端地址，或直接使用 IP 配置 DomainRoute。

## 权限要求

- Docker 部署：当前用户需要能访问 Docker daemon；Lite / Autonomy 默认以 `--privileged` 运行容器，需要相应权限。
- K8s 部署：需要创建 Namespace、CRD、PVC、Deployment 等资源的权限，通常需要集群管理员协助完成首次安装。
- 内核参数调整：必须在宿主机上以 `root` 执行，容器内无法修改。

## 部署检查清单

| 序号 | 检查项 | 命令 | 期望结果 |
| --- | --- | --- | --- |
| 1 | 架构 | `uname -m` | `x86_64` / `arm64` |
| 2 | 内核 | `uname -r` | `>= 4.6` |
| 3 | 资源 | `nproc && free -h && df -h` | 满足上表 |
| 4 | Docker / K8s | `docker info` / `kubectl get nodes` | 正常且节点 Ready |
| 5 | 端口 | `ss -lntp \| grep -E '1080\|8082\|8083'` | 无冲突 |
| 6 | inotify | `sysctl fs.inotify.max_user_instances` | `>= 8192` |
| 7 | 文件句柄 | `ulimit -n` | `>= 65536` |
| 8 | 时间同步 | `timedatectl status` | 已同步 |
| 9 | 镜像仓库 | `docker pull <kuscia-image>` | 可拉取或已离线导入 |

全部通过后，按[Kuscia 部署指引](./kuscia_deployment_instructions.md)选择部署方式。

## 相关文档

- [快速体验 Kuscia](../getting_started/quickstart_cn.md)
- [Kuscia 部署指引](./kuscia_deployment_instructions.md)
- [Kuscia 端口介绍](./kuscia_ports_cn.md)
- [网络要求](./networkrequirements.md)
- [部署失败排查](../troubleshoot/deployment/deploy_failed.md)
- [Create fsnotify failed](../troubleshoot/deployment/create_fsnotify_failed.md)
- [内核参数](../troubleshoot/parameter_tuning/kernel_params.md)
- [K8s 线程限制问题](../troubleshoot/parameter_tuning/k8s_ulimit_check.md)
