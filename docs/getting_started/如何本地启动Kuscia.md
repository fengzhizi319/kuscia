# 1. 如何本地启动 Kuscia

本指南介绍如何在本地快速启动和体验 Kuscia，支持 **Docker 容器化部署** 和 **本地二进制部署** 两种方式。

## 1.1 目录

- [1. 如何本地启动 Kuscia](#1-如何本地启动-kuscia)
  - [1.1 目录](#11-目录)
  - [1.2 概述](#12-概述)
  - [1.3 前置要求](#13-前置要求)
    - [1.3.1 硬件要求](#131-硬件要求)
    - [1.3.2 软件要求](#132-软件要求)
    - [1.3.3 安装 Docker（Docker 模式）](#133-安装-dockerdocker-模式)
    - [1.3.4 安装 Go（本地模式）](#134-安装-go本地模式)
  - [1.4 部署模式简介](#14-部署模式简介)
  - [1.5 Docker 模式快速启动](#15-docker-模式快速启动)
    - [1.5.1 方式一：使用一键脚本](#151-方式一使用一键脚本)
    - [1.5.2 方式二：手动分步启动](#152-方式二手动分步启动)
    - [1.5.3 验证 Docker 集群](#153-验证-docker-集群)
  - [1.6 本地模式启动（不使用 Docker）](#16-本地模式启动不使用-docker)
    - [1.6.1 方式一：使用一键脚本](#161-方式一使用一键脚本)
    - [1.6.2 方式二：手动分步启动](#162-方式二手动分步启动)
    - [1.6.3 验证本地集群](#163-验证本地集群)
  - [1.7 运行示例任务](#17-运行示例任务)
    - [1.7.1 Docker 模式运行 PSI 示例](#171-docker-模式运行-psi-示例)
    - [1.7.2 本地模式运行 PSI 示例](#172-本地模式运行-psi-示例)
  - [1.8 常用运维命令](#18-常用运维命令)
  - [1.9 停止和清理](#19-停止和清理)
    - [1.9.1 Docker 模式](#191-docker-模式)
    - [1.9.2 本地模式](#192-本地模式)
  - [1.10 故障排查](#110-故障排查)
  - [1.11 附录](#111-附录)
    - [1.11.1 Docker 模式与本地模式对比](#1111-docker-模式与本地模式对比)
    - [1.11.2 配置文件说明](#1112-配置文件说明)

## 1.2 概述

Kuscia 提供两种本地运行方式：

| 方式 | 适用场景 | 优点 | 缺点 |
| ------ | --------- | ------ | ------ |
| **Docker 模式** | 快速体验、功能验证、生产部署 | 一键启动、环境隔离、易于清理 | 需要 Docker、有容器开销 |
| **本地模式** | 二次开发、源码调试、深度定制 | 无容器开销、可直接调试源码 | 环境依赖多、配置复杂 |

**建议**：首次体验请选择 **Docker 模式**；开发者请选择 **本地模式**。

## 1.3 前置要求

### 1.3.1 硬件要求

**最低配置（POC 体验）**：

- CPU：4 核
- 内存：8 GB
- 磁盘：100 GB

**推荐配置（完整体验）**：

- CPU：8 核
- 内存：16 GB
- 磁盘：200 GB

### 1.3.2 软件要求

**操作系统**：

- Ubuntu 18.04+ / CentOS 7+
- macOS（Docker 模式）
- Windows（WSL2 + Ubuntu，Docker 模式）

**Docker 模式必需**：

- Docker 20.10+
- curl
- jq

**本地模式必需**：

- Go 1.24+（与 `go.mod` 保持一致）
- gcc / g++
- make
- curl
- jq
- git
- openssl

### 1.3.3 安装 Docker（Docker 模式）

以 Ubuntu 为例：

```bash
# 1. 更新包索引
sudo apt-get update

# 2. 安装依赖
sudo apt-get install -y \
    apt-transport-https \
    ca-certificates \
    curl \
    gnupg \
    lsb-release \
    jq

# 3. 添加 Docker GPG key
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker.gpg

# 4. 设置稳定版仓库
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker.gpg] \
  https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

# 5. 安装 Docker Engine
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io

# 6. 启动 Docker
sudo systemctl start docker
sudo systemctl enable docker

# 7. 验证安装
docker --version

# 8. （可选）将当前用户加入 docker 组，重新登录后生效
sudo usermod -aG docker $USER
```

### 1.3.4 安装 Go（本地模式）

以 Ubuntu 为例：

```bash
# 1. 下载 Go（请根据仓库 go.mod 选择对应版本）
GO_VERSION=$(grep "^go " /path/to/kuscia/go.mod | awk '{print $2}')
wget "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"

# 2. 解压安装
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"

# 3. 配置环境变量
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 4. 验证安装
go version
```

## 1.4 部署模式简介

Kuscia 支持多种组网模式：

| 模式 | 说明 | 容器数量 | 适用场景 |
| ------ | ------ | --------- | ---------- |
| **中心化组网（center）** | 1 个 Master + 2 个 Lite 节点 | 3 个 | 统一管控、资源成本低 |
| **点对点组网（p2p）** | 2 个 Autonomy 节点 | 2 个 | 安全性高、独立部署 |
| **中心化 × 中心化（cxc）** | 2 个 Master + 2 个 Lite 节点 | 4 个 | 跨机构协作 |
| **中心化 × 点对点（cxp）** | 1 个 Master + 1 个 Lite + 1 个 Autonomy | 3 个 | 混合组网 |

**推荐**：初次体验建议使用 **点对点组网（p2p）** 或 **中心化组网（center）**。

## 1.5 Docker 模式快速启动

### 1.5.1 方式一：使用一键脚本

仓库已提供 `scripts/deploy/run_docker_quickstart.sh`，支持 p2p / center / cxc / cxp 四种模式：

```bash
cd ../../

# 点对点组网（推荐初学者）
bash scripts/deploy/run_docker_quickstart.sh p2p

# 中心化组网
bash scripts/deploy/run_docker_quickstart.sh center

# 其他模式
bash scripts/deploy/run_docker_quickstart.sh cxc
bash scripts/deploy/run_docker_quickstart.sh cxp
```

脚本默认使用阿里云镜像：

```text
secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:latest
```

如需使用其他镜像，可设置环境变量：

```bash
export KUSCIA_IMAGE=secretflow/kuscia:1.2.0b0
bash scripts/deploy/run_docker_quickstart.sh p2p
```

### 1.5.2 方式二：手动分步启动

```bash
# 1. 配置镜像版本
export KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:latest

# 2. 拉取镜像
docker pull ${KUSCIA_IMAGE}

# 3. 从镜像中提取部署脚本
docker run --rm ${KUSCIA_IMAGE} cat /home/kuscia/scripts/deploy/kuscia.sh > kuscia.sh
chmod u+x kuscia.sh

# 4. 启动集群（选择一种模式）
./kuscia.sh p2p      # 点对点组网
./kuscia.sh center   # 中心化组网
./kuscia.sh cxc      # 中心化 × 中心化
./kuscia.sh cxp      # 中心化 × 点对点
```

### 1.5.3 验证 Docker 集群

```bash
# 查看运行中的容器
docker ps

# 点对点模式预期输出
# CONTAINER ID   IMAGE          COMMAND                  STATUS         PORTS     NAMES
# abc123def456   secretflow/..  "/home/kuscia/bin/..."   Up 2 minutes             ${USER}-kuscia-autonomy-alice
# def456ghi789   secretflow/..  "/home/kuscia/bin/..."   Up 2 minutes             ${USER}-kuscia-autonomy-bob

# 进入节点查看 Kubernetes 资源
docker exec -it ${USER}-kuscia-autonomy-alice bash
kubectl get namespaces
kubectl get nodes
```

## 1.6 本地模式启动（不使用 Docker）

本地模式直接在宿主机上编译并运行 Kuscia 二进制，适合源码开发和调试。

### 1.6.1 方式一：使用一键脚本

仓库已提供 `scripts/run_local_kuscia.sh`，默认以 **master** 模式启动：

```bash
cd ../../
# 启动本地 master 节点
bash scripts/run_local_kuscia.sh

# 或启动本地 autonomy 节点
bash scripts/run_local_kuscia.sh autonomy

# 指定域 ID
DOMAIN_ID=alice bash scripts/run_local_kuscia.sh autonomy

# 指定工作目录
KUSCIA_HOME=/tmp/kuscia-local bash scripts/run_local_kuscia.sh
```

脚本会完成：

1. 自动编译 Kuscia 二进制（如未编译）
2. 创建本地工作目录
3. 生成节点私钥和配置文件
4. 启动 Kuscia 进程

### 1.6.2 方式二：手动分步启动

#### 步骤 1：编译 Kuscia

```bash
cd ../../
make build

# 验证编译结果
ls -lh build/apps/kuscia/kuscia
./build/apps/kuscia/kuscia version
```

#### 步骤 2：初始化本地工作目录

```bash
export KUSCIA_HOME=/tmp/kuscia-local
mkdir -p ${KUSCIA_HOME}/{bin,etc/conf,var/logs,var/storage,var/certs,crds}

# 复制二进制文件
cp build/apps/kuscia/kuscia ${KUSCIA_HOME}/bin/

# 复制 CRDs
cp crds/v1alpha1/*.yaml ${KUSCIA_HOME}/crds/
```

#### 步骤 3：生成节点私钥

```bash
DOMAIN_KEY=$(openssl genrsa 2048 2>/dev/null | base64 | tr -d "\n" && echo)
echo "DOMAIN_KEY generated."
```

#### 步骤 4：生成配置文件

以 **master** 模式为例，创建 `${KUSCIA_HOME}/etc/conf/kuscia.yaml`：

```yaml
mode: master
domainID: master
domainKeyData: <上一步生成的 DOMAIN_KEY>
logLevel: INFO
datastoreEndpoint: ""
```

以 **autonomy** 模式为例：

```yaml
mode: autonomy
domainID: alice
domainKeyData: <上一步生成的 DOMAIN_KEY>
logLevel: INFO
runtime: runp
```

> **注意**：本地 Autonomy 节点默认使用 `runp` 运行时，实际任务容器仍由 Docker 启动；如需使用 `runc` 直接运行容器，请确保已安装 containerd/runc 并正确配置。

#### 步骤 5：启动 Kuscia

```bash
export KUSCIA_HOME=/tmp/kuscia-local
${KUSCIA_HOME}/bin/kuscia start -c ${KUSCIA_HOME}/etc/conf/kuscia.yaml
```

> **提示**：本地 Master 模式需要监听 53 / 80 / 8083 等端口，部分端口需要 root 权限，请确保当前用户具备相应权限或端口未被占用。

### 1.6.3 验证本地集群

```bash
# 查看 Kuscia 进程
ps aux | grep kuscia

# 查看端口监听
ss -tlnp | grep -E ':(53|80|8082|8083|8070|8060)\b'

# 查看 KusciaAPI 健康状态（本地模式默认可能使用 TLS，根据实际情况调整）
curl -k http://127.0.0.1:8082/healthz 2>/dev/null || true
curl -k https://127.0.0.1:8082/healthz 2>/dev/null || true

# 查看日志
tail -f ${KUSCIA_HOME}/var/logs/kuscia.log
```

## 1.7 运行示例任务

### 1.7.1 Docker 模式运行 PSI 示例

```bash
# 点对点模式：进入 alice 节点创建示例任务
docker exec -it ${USER}-kuscia-autonomy-alice scripts/user/create_example_job.sh

# 中心化模式：进入 master 容器创建示例任务
docker exec -it ${USER}-kuscia-master scripts/user/create_example_job.sh
```

查看任务状态：

```bash
# 点对点模式
docker exec -it ${USER}-kuscia-autonomy-alice kubectl get kj -n cross-domain

# 中心化模式
docker exec -it ${USER}-kuscia-master kubectl get kj -n cross-domain
```

查看结果：

```bash
docker exec -it ${USER}-kuscia-autonomy-alice cat var/storage/data/psi-output.csv | head -5
```

### 1.7.2 本地模式运行 PSI 示例

本地模式启动后，可通过 KusciaAPI 或 kubectl 提交任务。以下示例使用 kubectl 提交一个最小 KusciaJob：

```bash
export KUSCIA_HOME=/tmp/kuscia-local

# 创建示例 DomainData（如脚本存在）
${KUSCIA_HOME}/bin/kuscia scripts/deploy/create_domaindata_alice_table.sh alice

# 提交 PSI 任务（需提前注册 AppImage 并准备双方数据）
kubectl apply -f examples/psi-job.yaml

# 查看任务状态
kubectl get kusciajobs -n cross-domain
```

> **说明**：本地模式若要运行完整的跨节点 PSI，需要启动多个 Kuscia 实例并完成节点路由、数据授权等配置，详细流程请参考多机部署文档。

## 1.8 常用运维命令

### 查看容器日志（Docker 模式）

```bash
# alice 节点
docker logs -f ${USER}-kuscia-autonomy-alice

# master 节点
docker logs -f ${USER}-kuscia-master

# 最近 100 行
docker logs --tail=100 ${USER}-kuscia-autonomy-alice
```

### 进入容器调试（Docker 模式）

```bash
docker exec -it ${USER}-kuscia-autonomy-alice bash

# 容器内可执行：
# kubectl get namespaces
# cat /home/kuscia/etc/conf/kuscia.yaml
# tail -f /home/kuscia/var/logs/kuscia.log
```

### 查看 Kubernetes 资源

```bash
# Docker 模式需先进入容器
docker exec -it ${USER}-kuscia-autonomy-alice bash

# 通用命令
kubectl get namespaces
kubectl get pods -n alice
kubectl get kusciajobs -n cross-domain
kubectl get domaindata -n alice
kubectl get appimages
```

## 1.9 停止和清理

### 1.9.1 Docker 模式

```bash
# 停止集群（保留数据）
docker run --rm ${KUSCIA_IMAGE} cat /home/kuscia/scripts/deploy/stop.sh > stop.sh
chmod u+x stop.sh
./stop.sh all

# 完全卸载（删除所有数据，请谨慎操作）
docker run --rm ${KUSCIA_IMAGE} cat /home/kuscia/scripts/deploy/uninstall.sh > uninstall.sh
chmod u+x uninstall.sh
./uninstall.sh
```

### 1.9.2 本地模式

```bash
# 方式一：通过脚本停止
bash scripts/run_local_kuscia.sh --stop

# 方式二：手动停止
pkill -f "kuscia start -c"
# 或
kill $(cat ${KUSCIA_HOME}/var/kuscia.pid)

# 清理数据（请谨慎操作）
rm -rf ${KUSCIA_HOME}
```

## 1.10 故障排查

### 问题 1：容器启动失败（Docker 模式）

```bash
# 检查 Docker 是否运行
sudo systemctl status docker

# 查看容器日志
docker logs ${USER}-kuscia-autonomy-alice

# 常见原因
# - Docker 未启动
# - 端口冲突：检查 8080 / 8082 / 1080 等端口
# - 磁盘空间不足：df -h
```

### 问题 2：Kuscia 启动失败（本地模式）

```bash
# 查看日志
tail -n 100 ${KUSCIA_HOME}/var/logs/kuscia.log

# 检查配置文件语法
cat ${KUSCIA_HOME}/etc/conf/kuscia.yaml

# 检查端口占用
ss -tlnp | grep -E ':(53|80|8082|8083)\b'

# 检查权限
ls -la ${KUSCIA_HOME}
```

### 问题 3：任务一直处于 Pending 状态

```bash
# 查看任务事件
kubectl describe kusciajob <job-name> -n cross-domain

# 查看 Pod 状态
kubectl get pods -n alice
kubectl describe pod <pod-name> -n alice

# 常见原因
# - 资源不足
# - 镜像拉取失败
# - 节点间网络不通
```

### 问题 4：kubectl 无法连接（本地模式）

本地模式不依赖外部 kubeconfig，Kuscia 内置了 K3s。若 `kubectl` 无法连接，请检查：

```bash
# Kuscia 进程是否存活
ps aux | grep kuscia

# 日志中是否有 K3s 启动失败信息
grep -i "k3s\|apiserver" ${KUSCIA_HOME}/var/logs/kuscia.log
```

## 1.11 附录

### 1.11.1 Docker 模式与本地模式对比

| 维度 | Docker 模式 | 本地模式 |
| ------ | ------------ | --------- |
| 部署复杂度 | 低 | 高 |
| 调试便利性 | 中 | 高 |
| 性能 | 中 | 高 |
| 隔离性 | 高 | 低 |
| 资源占用 | 中 | 低 |
| 适用场景 | 快速体验、生产部署 | 开发调试、源码定制 |
| 升级难度 | 低 | 高 |

### 1.11.2 配置文件说明

Kuscia 配置文件为 YAML 格式，关键字段说明如下：

| 字段 | 说明 | 可选值 |
| ------ | ------ | -------- |
| `mode` | 部署模式 | `master` / `lite` / `autonomy` |
| `domainID` | 节点域 ID | 自定义字符串 |
| `domainKeyData` | Base64 编码的 RSA 私钥 | 通过 `openssl genrsa` 生成 |
| `logLevel` | 日志级别 | `INFO` / `DEBUG` / `WARN` |
| `runtime` | 容器运行时 | `runc` / `runk` / `runp` |
| `datastoreEndpoint` | 数据库连接串 | 空则使用 SQLite |

模板文件参考：

- Master：`scripts/templates/kuscia-master.yaml`
- Lite：`scripts/templates/kuscia-lite.yaml`
- Autonomy：`scripts/templates/kuscia-autonomy.yaml`

---

**祝你使用愉快！** 🎉
