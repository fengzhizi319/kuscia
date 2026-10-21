# 使用 WSL 注意事项

## 前言

Windows 上推荐通过 **WSL2 + Docker Desktop** 运行 Kuscia。本文列出安装、磁盘配置和常见问题的处理方式。完整的环境要求见[部署要求](../../deployment/deploy_check.md)。

## 前置条件

- Windows 10 2004 及以上版本，或 Windows 11；
- 已启用虚拟化（BIOS 中开启 VT-x / AMD-V）；
- 已安装 WSL2 和一个 Ubuntu 发行版；
- 建议至少 8 核 CPU、16 GB 内存、100 GB 可用磁盘。

检查 WSL 状态：

```powershell
wsl --status
wsl --list --verbose
```

`VERSION` 应为 `2`。如果发行版仍是 WSL1，可执行：

```powershell
wsl --set-version Ubuntu 2
```

## WSL 启动 Docker

**WSL 中想要使用 Docker，建议安装 Docker Desktop**，具体安装步骤如下：

### 1. 修改默认安装路径

Docker Desktop 默认安装在 "C:\Program Files\Docker"，该路径是一个只读路径，如果不修改，将无法进行写操作（比如创建或修改文件/文件夹），在部署时可能报错：

```shell
Error response from daemon: mkdir /var/lib/docker/temp/docker-export-709988460:read-only file system
```

需要在非只读目录（如 D 盘）下创建 Docker Desktop 默认安装路（如 D:\Docker），然后以管理员身份运行 cmd，通过以下命令创建链接：

```shell
mklink /J "C:\Program Files\Docker" "D:\Docker"
```

这样创建链接之后，按正常步骤安装 docker 就会安装在 D 盘里了。

### 2. 安装

参考 Docker 官方安装文档 [Install Docker Desktop on Windows](https://docs.docker.com/desktop/install/windows-install/)：

1. 从 Docker 官网下载 [Docker Desktop for Windows](https://www.docker.com/products/docker-desktop/);
2. 双击下载的 "Docker Desktop Installer.exe";
3. 使用官方建议的 WSL2 选项安装（勾选："Use the WSL 2 based engine"）;
4. 按照安装向导上的说明授权安装程序，并继续安装;
5. 安装成功后，单击"关闭"完成安装过程。

### 3. 启用 WSL 集成

打开 Docker Desktop，进入 **Settings → Resources → WSL Integration**，勾选需要使用的 Ubuntu 发行版，点击 **Apply & Restart**。

在 WSL 终端中验证：

```bash
docker version
docker info
docker run --rm hello-world
```

### 4. 建议修改镜像保存路径

Docker 镜像默认保存在 C 盘， 如果要部署和运行 FATE 作业，可能导致 C 盘空间不足，因此建议改为其他空间足够的磁盘。具体做法是，打开 Docker Desktop，找到 Settings->Resources->Browse，修改镜像保存路径。

![docker_settings.png](../../imgs/docker_settings.png)

## 资源与性能建议

### 限制 WSL2 资源

在 Windows 用户目录下创建或修改 `%UserProfile%\.wslconfig`：

```ini
[wsl2]
memory=16GB
processors=8
swap=4GB
```

修改后执行：

```powershell
wsl --shutdown
```

然后重新启动 Docker Desktop。

### 不要把 Kuscia 数据放在 /mnt/c

`/mnt/c` 的跨文件系统 I/O 明显慢于 WSL2 原生文件系统，Kuscia 的镜像、K3s 数据和任务日志应放在 WSL2 内部目录，例如 `~/kuscia`：

```bash
mkdir -p ~/kuscia
cd ~/kuscia
```

### 时间同步

WSL2 休眠/恢复后可能出现时钟漂移，导致证书校验失败。可在 WSL 中执行：

```bash
sudo hwclock -s
```

## 常见问题

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| `read-only file system` | Docker 安装在只读路径 | 按上文用 `mklink /J` 迁移安装目录 |
| `Cannot connect to the Docker daemon` | Docker Desktop 未启动或 WSL 集成未开启 | 启动 Docker Desktop，检查 WSL Integration |
| 拉取镜像很慢 | 未配置镜像加速 | 在 Docker Desktop 的 Docker Engine 配置中增加 registry mirror |
| C 盘空间不足 | 镜像默认存储在 C 盘 | 修改镜像保存路径，清理无用镜像 |
| 容器时间不正确 | WSL 时钟漂移 | 执行 `sudo hwclock -s`，或重启 WSL |
| 端口无法访问 | Windows 防火墙或端口未映射 | 检查 `docker port`，放行对应端口，或改用 `127.0.0.1` |

## 验证 Kuscia 环境

```bash
# WSL 内执行
uname -m
docker version --format '{{.Server.Version}}'
nproc
free -h
df -h /var/lib/docker
```

全部满足要求后，按[Kuscia 部署指引](../../deployment/kuscia_deployment_instructions.md)继续部署。

## 相关文档

- [部署要求](../../deployment/deploy_check.md)
- [Kuscia 部署指引](../../deployment/kuscia_deployment_instructions.md)
- [部署失败排查](./deploy_failed.md)
- [Create fsnotify failed](./create_fsnotify_failed.md)
