# Kuscia 与 DataMesh 本地部署指南

> 适用版本：Kuscia 1.2.0b0（与文档编写时 `.VERSION` 一致）  
> 阅读对象：希望在本地（单台 Linux / WSL2）快速部署 Kuscia 服务并了解 DataMesh 使用的开发者、测试人员。

## 目录

1. [概述](#1-概述)
2. [环境要求](#2-环境要求)
3. [快速体验：一键启动体验集群](#3-快速体验一键启动体验集群)
4. [DataMesh 是什么](#4-datamesh-是什么)
5. [自定义本地多节点部署（暴露端口版）](#5-自定义本地多节点部署暴露端口版)
6. [通过源码/二进制启动（可选）](#6-通过源码二进制启动可选)
7. [验证与调试](#7-验证与调试)
8. [停止与卸载](#8-停止与卸载)
9. [附录](#9-附录)

---

## 1. 概述

[Kuscia](https://www.secretflow.org.cn/docs/kuscia/) 是基于 K3s 的轻量级隐私计算任务编排框架。DataMesh 是 Kuscia 内部负责数据资产（DomainData、DomainDataSource、DomainDataGrant）管理的核心模块，提供 HTTP/gRPC 两种访问方式。

**关键结论：**

- DataMesh 不独立部署，它随 Kuscia 的 **Lite / Autonomy** 节点一起启动。
- 在本地体验时，通常先通过官方部署脚本拉起 Kuscia 集群，DataMesh 会自动运行。
- 如果后续要对接 SecretPad，只需要让 SecretPad 后端能访问 Kuscia 暴露的 API 端口即可，DataMesh 对 SecretPad 是透明的。

---

## 2. 环境要求

| 项目 | 要求 |
| ------ | ------ |
| 操作系统 | macOS、CentOS 7/8、Ubuntu 16.04+、Windows（WSL2 Ubuntu） |
| CPU 架构 | x86_64 / amd64 / arm64 |
| Docker | ≥ 20.10.24 |
| 最低资源 | 1 核 / 2G 内存 / 20G 硬盘 |
| 推荐资源 | 8 核 / 16G 内存 / 200G 硬盘（体验集群需要同时跑多个容器） |

> **注意**：如果当前用户不在 `docker` 组，所有 `docker` 命令需要加 `sudo`。本文示例默认使用普通用户执行，必要时请自行在命令前加 `sudo`。

---

## 3. 快速体验：一键启动体验集群

### 3.1 获取部署脚本

```bash
# 设置要使用的 Kuscia 镜像版本
export KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0

# 拉取镜像并导出部署脚本到当前目录
docker pull ${KUSCIA_IMAGE} && \
docker run --rm ${KUSCIA_IMAGE} cat /home/kuscia/scripts/deploy/kuscia.sh > kuscia.sh && \
chmod u+x kuscia.sh
```

### 3.2 中心化组网模式（推荐）

启动 1 个 master + 2 个 Lite 节点（alice / bob）：

```bash
./kuscia.sh center
```

容器命名规则（假设当前用户为 `charles`）：

- `charles-kuscia-master`
- `charles-kuscia-lite-alice`
- `charles-kuscia-lite-bob`

### 3.3 点对点组网模式

启动 2 个 Autonomy 节点：

```bash
./kuscia.sh p2p
```

容器命名规则：

- `charles-kuscia-autonomy-alice`
- `charles-kuscia-autonomy-bob`

### 3.4 运行示例任务

**中心化模式：**

```bash
# 创建并运行一个 PSI 示例任务
docker exec -it ${USER}-kuscia-master scripts/user/create_example_job.sh

# 查看任务状态
docker exec -it ${USER}-kuscia-master kubectl get kj -n cross-domain
```

**点对点模式：**

```bash
docker exec -it ${USER}-kuscia-autonomy-alice scripts/user/create_example_job.sh
docker exec -it ${USER}-kuscia-autonomy-alice kubectl get kj -n cross-domain
```

任务成功后，可在对应节点查看结果：

```bash
# 中心化：alice lite 节点
docker exec -it ${USER}-kuscia-lite-alice cat /home/kuscia/var/storage/data/psi-output.csv

# 点对点：alice autonomy 节点
docker exec -it ${USER}-kuscia-autonomy-alice cat /home/kuscia/var/storage/data/psi-output.csv
```

> 体验模式**默认不向宿主机暴露端口**，仅供容器内验证。如果要对接外部服务（如 SecretPad），请使用第 5 章的自定义部署方式。

---

## 4. DataMesh 是什么

### 4.1 定位

DataMesh 是 Kuscia 的数据访问层，负责：

- 管理 **DomainDataSource**（数据源，如本地文件、OSS、MySQL 等）
- 管理 **DomainData**（数据表/数据集）
- 管理 **DomainDataGrant**（跨域数据授权）
- 通过 Apache Arrow Flight 提供数据读写能力

### 4.2 默认端口

| 协议 | 容器内端口 | 说明 |
| ------ | ----------- | ------ |
| HTTP | 8070 | DataMesh API、健康检查 |
| gRPC / Arrow Flight | 8071 | 数据读写、DomainData gRPC 服务 |

### 4.3 启动方式

DataMesh 是 `kuscia` 二进制内部的模块，注册在 `cmd/kuscia/start/start.go` 中：

```go
mm.Regist("datamesh", modules.NewDataMesh, autonomy, lite)
```

因此只要 Lite / Autonomy 节点启动成功，DataMesh 就会自动启动。

### 4.4 默认本地文件数据源

DataMesh 启动后，Operator Bean 会自动在每个 Lite / Autonomy 节点创建默认本地数据源：

- 数据源 ID：`default-data-source`
- 类型：`localfs`
- 路径：`/home/kuscia/var/storage/data`

把 CSV 文件放到该目录下，即可通过 DataMesh 读取。

### 4.5 健康检查

进入任意 Lite / Autonomy 节点容器：

```bash
docker exec -it ${USER}-kuscia-lite-alice bash
curl -k https://127.0.0.1:8070/healthZ
```

### 4.6 通过 DataMesh API 创建 DomainData（容器内示例）

```bash
docker exec -it ${USER}-kuscia-lite-alice curl -k \
  https://127.0.0.1:8070/api/v1/datamesh/domaindata/create \
  -X POST -H 'content-type: application/json' \
  --cacert /home/kuscia/var/certs/ca.crt \
  --cert /home/kuscia/var/certs/ca.crt \
  --key /home/kuscia/var/certs/ca.key \
  -d '{
    "domain_id": "alice",
    "domaindata_id": "alice-001",
    "datasource_id": "default-data-source",
    "name": "alice001",
    "type": "table",
    "relative_uri": "alice.csv",
    "columns": [
      {"name": "id1", "type": "str"},
      {"name": "x1", "type": "float"}
    ]
  }'
```

### 4.7 通过 KusciaAPI 创建 DomainDataSource（推荐，可自动加密）

KusciaAPI 的 HTTP 端口默认映射到宿主机后，可以从宿主机调用：

```bash
curl -k -X POST 'https://localhost:18082/api/v1/domaindatasource/create' \
  --header "Token: $(docker exec -i ${USER}-kuscia-master cat /home/kuscia/var/certs/token)" \
  --header 'Content-Type: application/json' \
  --cert /home/charles/code/kuscia/${USER}-kuscia-master/certs/kusciaapi-server.crt \
  --key /home/charles/code/kuscia/${USER}-kuscia-master/certs/kusciaapi-server.key \
  --cacert /home/charles/code/kuscia/${USER}-kuscia-master/certs/ca.crt \
  -d '{
    "domain_id": "alice",
    "datasource_id": "demo-local-datasource",
    "type": "localfs",
    "name": "DemoDataSource",
    "info": {"localfs": {"path": "/home/kuscia/var/storage/data"}},
    "access_directly": true
  }'
```

> 上面假设使用第 5 章的方式把 master 的 KusciaAPI HTTP 端口映射到了宿主机的 `18082`。

---

## 5. 自定义本地多节点部署（暴露端口版）

如果你需要让 Kuscia 与宿主机上的 SecretPad、IDEA 调试器或其他工具通信，需要显式暴露端口。

### 5.1 生成配置文件

```bash
export KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0

# master 配置
docker run -it --rm ${KUSCIA_IMAGE} kuscia init \
  --mode master \
  --domain "mycompany-secretflow-master" > kuscia_master.yaml 2>&1 || cat kuscia_master.yaml

# autonomy 配置（可选）
docker run -it --rm ${KUSCIA_IMAGE} kuscia init \
  --mode autonomy \
  --domain "alice" > autonomy_alice.yaml 2>&1 || cat autonomy_alice.yaml
```

### 5.2 在 master 注册 Lite 节点并获取 Token

```bash
export ALICE_TOKEN=$(docker exec -i ${USER}-kuscia-master sh scripts/deploy/add_domain_lite.sh alice)
echo "Alice 的部署 Token: ${ALICE_TOKEN}"
```

如果 Token 遗忘了，可以重新获取：

```bash
export ALICE_TOKEN=$(docker exec -it ${USER}-kuscia-master kubectl get domain alice -o=jsonpath='{.status.deployTokenStatuses[?(@.state=="unused")].token}')
```

### 5.3 生成 Lite 节点配置

```bash
docker run -it --rm ${KUSCIA_IMAGE} kuscia init \
  --mode lite \
  --domain "alice" \
  --master-endpoint "https://127.0.0.1:18080" \
  --lite-deploy-token "${ALICE_TOKEN}" > lite_alice.yaml 2>&1 || cat lite_alice.yaml
```

### 5.4 启动节点并暴露端口

```bash
# master：-p 外部 HTTPS 端口，-k KusciaAPI HTTP 端口，-g KusciaAPI gRPC 端口
./kuscia.sh start -c kuscia_master.yaml -p 18080 -k 18082 -g 18083

# lite alice：-q 为内部 domain 端口（Envoy / DataMesh 所在网络），-x 为 Metrics 端口
./kuscia.sh start -c lite_alice.yaml -p 28080 -k 28082 -g 28083 -q 28081 -x 28084
```

常用端口说明：

| 容器内端口 | 用途 | 脚本参数 |
| ----------- | ------ | --------- |
| 1080 | 节点间认证鉴权（Gateway） | `-p` |
| 80 | 节点内部应用访问（Envoy） | `-q` |
| 8082 | KusciaAPI HTTP | `-k` |
| 8083 | KusciaAPI gRPC | `-g` |
| 9091 | Metrics | `-x` |

### 5.5 建立节点间授权

如果有多个 Lite 节点要相互通信，需要在 master 上创建集群域路由：

```bash
docker exec -it ${USER}-kuscia-master sh scripts/deploy/create_cluster_domain_route.sh alice bob https://${USER}-kuscia-lite-bob:1080
docker exec -it ${USER}-kuscia-master sh scripts/deploy/create_cluster_domain_route.sh bob alice https://${USER}-kuscia-lite-alice:1080
```

查看授权状态：

```bash
docker exec -it ${USER}-kuscia-master kubectl get cdr
```

当 `type=Ready` 的 `status` 为 `True` 时，表示授权成功。

---

## 6. 通过源码/二进制启动（可选）

如果你想使用当前 Kuscia 源码编译的二进制，可以执行：

```bash
# 构建 kuscia 二进制
bash hack/build.sh -t kuscia
# 产物：build/apps/kuscia/kuscia

# 构建 transport 二进制
bash hack/build.sh -t transport
# 产物：build/apps/transport/transport

# 构建 Docker 镜像
make image
```

构建出镜像后，把镜像 tag 导出并设置 `KUSCIA_IMAGE`：

```bash
export KUSCIA_IMAGE=my-kuscia:local
```

然后按第 3 / 5 章的方式启动即可。

---

## 7. 验证与调试

### 7.1 查看容器状态

```bash
docker ps -f name=${USER}-kuscia
```

### 7.2 查看 Pod 与任务

```bash
# master 内查看所有 Pod
docker exec -it ${USER}-kuscia-master kubectl get po -A

# 查看 KusciaJob
docker exec -it ${USER}-kuscia-master kubectl get kj -n cross-domain
```

### 7.3 查看日志

```bash
# Kuscia 主日志
docker logs -f ${USER}-kuscia-master

# 节点内部日志
docker exec -it ${USER}-kuscia-lite-alice tail -f /home/kuscia/var/logs/k3s.log
docker exec -it ${USER}-kuscia-lite-alice tail -f /home/kuscia/var/logs/envoy
docker exec -it ${USER}-kuscia-lite-alice tail -f /home/kuscia/var/logs/datamesh.log
```

### 7.4 检查 Gateway 是否可达

```bash
curl -kvvv https://127.0.0.1:18080
```

正常应返回 HTTP 401（unauthorized）。

### 7.5 获取 KusciaAPI 证书

```bash
docker cp ${USER}-kuscia-master:/home/kuscia/var/certs/kusciaapi-server.key .
docker cp ${USER}-kuscia-master:/home/kuscia/var/certs/kusciaapi-server.crt .
docker cp ${USER}-kuscia-master:/home/kuscia/var/certs/ca.crt .
docker cp ${USER}-kuscia-master:/home/kuscia/var/certs/token .
```

---

## 8. 停止与卸载

### 8.1 获取停止/卸载脚本

```bash
docker pull ${KUSCIA_IMAGE} && \
docker run --rm ${KUSCIA_IMAGE} cat /home/kuscia/scripts/deploy/stop.sh > stop.sh && \
chmod u+x stop.sh

docker pull ${KUSCIA_IMAGE} && \
docker run --rm ${KUSCIA_IMAGE} cat /home/kuscia/scripts/deploy/uninstall.sh > uninstall.sh && \
chmod u+x uninstall.sh
```

### 8.2 停止集群

```bash
./stop.sh center
./stop.sh p2p
./stop.sh all
```

### 8.3 卸载集群

```bash
./uninstall.sh center
./uninstall.sh p2p
./uninstall.sh all
```

卸载会删除容器、volume、network（如果没有其他容器使用）。

---

## 9. 附录

### 9.1 关键文件路径

| 用途 | 路径（宿主机） |
| ------ | --------------- |
| master 配置文件 | `${PWD}/${USER}-kuscia-master/kuscia.yaml` |
| lite 配置文件 | `${PWD}/${USER}-kuscia-lite-<domain>/kuscia.yaml` |
| autonomy 配置文件 | `${PWD}/${USER}-kuscia-autonomy-<domain>/kuscia.yaml` |
| 节点数据目录 | `${PWD}/${USER}-kuscia-lite-<domain>/data` |
| 节点日志目录 | `${PWD}/${USER}-kuscia-lite-<domain>/logs` |

| 用途 | 路径（容器内） |
| ------ | --------------- |
| 主配置 | `/home/kuscia/etc/conf/kuscia.yaml` |
| 数据目录 | `/home/kuscia/var/storage/data` |
| 日志目录 | `/home/kuscia/var/logs` |
| 证书目录 | `/home/kuscia/var/certs` |

### 9.2 常用环境变量

| 变量 | 说明 |
| ------ | ------ |
| `KUSCIA_IMAGE` | Kuscia 镜像 |
| `SECRETFLOW_IMAGE` | SecretFlow 引擎镜像 |
| `DOMAIN_HOST_PORT` | 节点对外 HTTPS 端口（`-p`） |
| `KUSCIAAPI_HTTP_PORT` | KusciaAPI HTTP 端口（`-k`） |
| `KUSCIAAPI_GRPC_PORT` | KusciaAPI gRPC 端口（`-g`） |
| `DOMAIN_HOST_INTERNAL_PORT` | 节点内部端口（`-q`） |
| `METRICS_PORT` | Metrics 端口（`-x`） |
| `RUNTIME` | 运行时：`runc`（默认）/ `runp` / `runk` |
| `MEMORY_LIMIT` | 内存限制，如 `4GiB`、`-1` |

### 9.3 参考文档

- [Kuscia 快速开始](./quickstart_cn.md)
- [多机部署中心化集群](./Docker_deployment_kuscia/deploy_master_lite_cn.md)
- [多机部署 P2P 集群](./Docker_deployment_kuscia/deploy_p2p_cn.md)
- [DataMesh API 概览](../reference/apis/datamesh/summary_cn.md)
- [Kuscia 端口介绍](./kuscia_ports_cn.md)
