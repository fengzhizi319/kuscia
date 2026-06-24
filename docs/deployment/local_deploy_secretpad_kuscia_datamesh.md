# SecretPad + Kuscia + DataMesh 本地联合部署指南

> 适用场景：在本地（单台 Linux / WSL2）同时部署 SecretPad、Kuscia 与 DataMesh，使三者能够互相通信、完成隐私计算任务。  
> 示例路径：SecretPad 源码位于 `/home/charles/code/secretpad`（对应 Windows 访问路径 `\\wsl.localhost\Ubuntu\home\charles\code\secretpad`）。

## 目录

1. [架构与通信关系](#1-架构与通信关系)
2. [前置条件](#2-前置条件)
3. [方案一：All-in-one 容器化一键部署](#3-方案一all-in-one-容器化一键部署)
4. [方案二：Kuscia 容器 + SecretPad 源码联调](#4-方案二kuscia-容器--secretpad-源码联调)
5. [验证 DataMesh 与 SecretPad 的交互](#5-验证-datamesh-与-secretpad-的交互)
6. [常见问题](#6-常见问题)
7. [附录](#7-附录)

---

## 1. 架构与通信关系

```text
┌─────────────────┐      HTTP / gRPC       ┌─────────────────────┐
│   浏览器         │ ─────────────────────> │ SecretPad 前端/后端  │
│ localhost:8000  │                        │   localhost:8080    │
└─────────────────┘                        └──────────┬──────────┘
                                                      │ KusciaAPI (gRPC/HTTP)
                                                      ▼
                                            ┌─────────────────────┐
                                            │ Kuscia master/lite  │
                                            │  (Docker 容器)       │
                                            └──────────┬──────────┘
                                                      │ 内部启动
                                                      ▼
                                            ┌─────────────────────┐
                                            │      DataMesh       │
                                            │  :8070 / :8071      │
                                            └─────────────────────┘
```

**关键点：**

- **SecretPad 不直接连接 DataMesh**，它通过 KusciaAPI 操作 DomainData、DomainDataSource、DomainDataGrant。
- **DataMesh 随 Kuscia Lite / Autonomy 节点自动启动**，不需要单独部署。
- SecretPad 后端到 Kuscia 的通信依赖 **KusciaAPI gRPC 端口（默认 8083）** 和 **Gateway/Envoy 内部端口（默认 80，映射到宿主机需指定）**。
- 本地方便起见，推荐使用 **方案一** 快速跑通；如果是二次开发，使用 **方案二** 在 IDEA/VS Code 中调试 SecretPad 源码。

---

## 2. 前置条件

| 项目 | 版本/要求 | 说明 |
|------|----------|------|
| Docker | ≥ 20.10.24 | 运行 Kuscia/SecretPad 容器 |
| Git | 任意 | 克隆前端代码 |
| OpenJDK | 17 | SecretPad 后端（方案二需要） |
| Maven | 3.8.8 | 后端编译（方案二需要） |
| Node.js | ≥ 16.14.0（推荐 v20.14.0） | 前端运行（方案二需要） |
| pnpm | 8.8.0 | 前端包管理（方案二需要） |
| 推荐资源 | 8 核 / 16G 内存 / 200G 磁盘 | 体验集群同时跑多个容器 |

> **版本兼容提示**：当前 SecretPad 仓库默认使用 Kuscia `0.13.0b0`、SecretPad `0.12.0b0`。如果你要用当前 Kuscia 源码仓库（`1.2.0b0`）构建镜像，请确认 SecretPad 版本与 Kuscia 版本匹配，参考 [SecretPad README 组件版本表](https://github.com/secretflow/secretpad?tab=readme-ov-file#组件版本)。

---

## 3. 方案一：All-in-one 容器化一键部署

这是最简单的方式：`secretpad` 仓库的 `scripts/install.sh` 会一次性拉起 Kuscia master、Lite 节点（alice/bob/tee）以及 SecretPad 容器。

### 3.1 进入 SecretPad 仓库

```bash
cd /home/charles/code/secretpad
```

### 3.2 启动 master 模式（中心化组网）

```bash
# 使用 notls 协议，本地开发最方便
bash scripts/install.sh master -P notls

# 如果需要使用默认 mtls，直接执行：
# bash scripts/install.sh master
```

默认会创建以下容器（假设当前用户为 `charles`）：

| 容器名 | 说明 |
|--------|------|
| `charles-kuscia-master` | Kuscia master 控制平面 |
| `charles-kuscia-lite-alice` | Lite 节点 alice |
| `charles-kuscia-lite-bob` | Lite 节点 bob |
| `charles-kuscia-lite-tee` | Lite 节点 tee（ALL-IN-ONE 模式） |
| `charles-kuscia-master-secretpad` | SecretPad Web 服务 |

默认端口映射：

| 服务 | 宿主机端口 | 容器端口 |
|------|-----------|---------|
| SecretPad Web | 8080 | 8080 |
| Kuscia master gateway | 18080 | 1080 |
| Kuscia master KusciaAPI HTTP | 18082 | 8082 |
| Kuscia master KusciaAPI gRPC | 18083 | 8083 |
| Kuscia master Envoy 内部端口 | 13081 | 80 |
| Kuscia lite alice gateway | 28080 | 1080 |
| Kuscia lite alice KusciaAPI HTTP | 28082 | 8082 |
| Kuscia lite alice KusciaAPI gRPC | 28083 | 8083 |
| Kuscia lite bob gateway | 38080 | 1080 |
| Kuscia lite bob KusciaAPI HTTP | 38082 | 8082 |
| Kuscia lite bob KusciaAPI gRPC | 38083 | 8083 |

### 3.3 查看安装输出

安装成功后会打印类似信息：

```text
Web server started successfully
Please visit the website http://localhost:8080 ...
The login name:'admin' ,The login password:'xxxxx' .
The data would be stored in the path: /home/charles/kuscia/master/data .
```

### 3.4 访问 SecretPad

浏览器打开：

```text
http://localhost:8080
```

使用安装日志中的用户名/密码登录。

### 3.5 验证 Kuscia 与 DataMesh

```bash
# 查看 Kuscia 节点
docker exec -it ${USER}-kuscia-master kubectl get nodes

# 查看 Pod
docker exec -it ${USER}-kuscia-master kubectl get po -A

# 在 alice/lite 节点内检查 DataMesh 健康
docker exec -it ${USER}-kuscia-lite-alice curl -k https://127.0.0.1:8070/healthZ
```

### 3.6 验证 alice / bob 示例数据

`install.sh master` 会自动调用 `add_alice_bob_data`，在 alice 和 bob 节点创建默认 DomainData 并互相授权：

```bash
# 查看 alice 的 DomainData
docker exec -it ${USER}-kuscia-lite-alice curl -k \
  https://127.0.0.1:8070/api/v1/datamesh/domaindata/query \
  -X POST -H 'content-type: application/json' \
  --cacert /home/kuscia/var/certs/ca.crt \
  --cert /home/kuscia/var/certs/ca.crt \
  --key /home/kuscia/var/certs/ca.key \
  -d '{"domaindata_id": "alice-table"}'
```

登录 SecretPad 后，在“数据管理”中应能看到 alice / bob 的示例数据表。

---

## 4. 方案二：Kuscia 容器 + SecretPad 源码联调

如果你需要修改 SecretPad 代码并本地调试，可以先通过 `install.sh master` 拉起 Kuscia 容器，然后停止容器内的 SecretPad，改从源码启动后端和前端。

### 4.1 启动 Kuscia 容器

```bash
cd /home/charles/code/secretpad
bash scripts/install.sh master -P notls
```

### 4.2 停止容器内的 SecretPad

```bash
docker stop ${USER}-kuscia-master-secretpad
```

> 这会释放宿主机的 8080 端口，供本地 SecretPad 后端使用。

### 4.3 让宿主机解析 Kuscia 容器名

SecretPad 后端的 `config/application-dev.yaml` 默认使用容器名（如 `root-kuscia-master`）连接 Kuscia。当后端在宿主机直接运行时，需要把这些名字解析到本地。

**方法 A：修改 `/etc/hosts`**（推荐，最简单）

```bash
sudo tee -a /etc/hosts <<'EOF'
127.0.0.1 root-kuscia-master
127.0.0.1 root-kuscia-lite-alice
127.0.0.1 root-kuscia-lite-bob
127.0.0.1 root-kuscia-lite-tee
EOF
```

> 如果你的容器名不是 `root-kuscia-*`（例如 `charles-kuscia-master`），请使用 `docker ps` 查看实际名称，并相应地替换上面的名字。

**方法 B：修改 `config/application-dev.yaml` 为 `127.0.0.1` 和对应端口**

```yaml
kuscia:
  nodes:
    - domainId: ${NODE_ID:kuscia-system}
      mode: master
      host: 127.0.0.1
      port: 18083
      protocol: ${KUSCIA_PROTOCOL:notls}
      ...
    - domainId: alice
      mode: lite
      host: 127.0.0.1
      port: 28083
      ...
    - domainId: bob
      mode: lite
      host: 127.0.0.1
      port: 38083
      ...
```

### 4.4 配置 Gateway 端口

`config/application-dev.yaml` 中默认 `gateway: 127.0.0.1:18301`。对于 `install.sh master` 默认部署，master 的 Envoy 内部端口映射到宿主机的 `13081`，因此需要覆盖：

```bash
export KUSCIA_GW_ADDRESS=127.0.0.1:13081
```

如果使用 `notls`，同时设置：

```bash
export KUSCIA_PROTOCOL=notls
```

### 4.5 准备本地运行时（JDK/Maven/Node/pnpm）

如果你已经全局安装了 Java 17、Maven 3.8.8、Node 20 和 pnpm 8.8.0，可跳过本步。

否则，可以下载到项目内 `.tools` 目录：

```bash
cd /home/charles/code/secretpad
mkdir -p .tools

# Node.js 20.14.0
curl -L -o .tools/node.tar.xz 'https://nodejs.org/dist/v20.14.0/node-v20.14.0-linux-x64.tar.xz'
tar -xf .tools/node.tar.xz -C .tools
mv .tools/node-v20.14.0-linux-x64 .tools/node
rm .tools/node.tar.xz

export PATH=/home/charles/code/secretpad/.tools/node/bin:$PATH
corepack enable
corepack prepare pnpm@8.8.0 --activate

# JDK 17
curl -L -o .tools/jdk17.tar.gz 'https://github.com/adoptium/temurin17-binaries/releases/download/jdk-17.0.11%2B9/OpenJDK17U-jdk_x64_linux_hotspot_17.0.11_9.tar.gz'
tar -xzf .tools/jdk17.tar.gz -C .tools
mv .tools/jdk-17.0.11+9 .tools/jdk-17
rm .tools/jdk17.tar.gz

# Maven 3.8.8
curl -L -o .tools/maven.tar.gz 'https://archive.apache.org/dist/maven/maven-3/3.8.8/binaries/apache-maven-3.8.8-bin.tar.gz'
tar -xzf .tools/maven.tar.gz -C .tools
mv .tools/apache-maven-3.8.8 .tools/maven
rm .tools/maven.tar.gz

export JAVA_HOME=/home/charles/code/secretpad/.tools/jdk-17
export PATH=$JAVA_HOME/bin:/home/charles/code/secretpad/.tools/maven/bin:/home/charles/code/secretpad/.tools/node/bin:$PATH
```

### 4.6 拉取前端源码

```bash
cd /home/charles/code/secretpad
git clone --depth=1 https://github.com/fengzhizi319/secretpad-frontend.git frontend-src
```

### 4.7 生成证书与数据库目录

```bash
cd /home/charles/code/secretpad
bash scripts/test/setup.sh
```

这会生成：

```text
config/
├── certs/
│   ├── client.crt / client.pem / token / ca.crt
│   ├── alice/
│   └── bob/
└── server.jks
db/
```

### 4.8 编译后端

```bash
mvn clean install -Dmaven.test.skip=true
mvn compile
```

产物：`secretpad-web/target/secretpad-web-*.jar`。

### 4.9 启动后端

```bash
cd /home/charles/code/secretpad

export KUSCIA_PROTOCOL=notls
export KUSCIA_GW_ADDRESS=127.0.0.1:13081

java -Dspring.profiles.active=dev \
     -Dsun.net.http.allowRestrictedHeaders=true \
     -jar secretpad-web/target/secretpad-web-*.jar
```

启动成功后日志会打印：

```text
SecretPad start success, http://...:443 innerHttpPort:9001 Profile:dev
userName:admin password:xxxxx
```

- 后端 HTTP 端口：`8080`
- 默认用户名：`admin`
- 默认密码：日志中随机生成

### 4.10 启动前端

```bash
cd /home/charles/code/secretpad/frontend-src
pnpm install
pnpm run setup
```

配置代理：

```bash
cat > apps/platform/.env <<'EOF'
PROXY_URL=http://127.0.0.1:8080
EOF
```

启动 dev 服务器：

```bash
pnpm --filter secretpad dev
```

默认监听：`http://localhost:8000`。

### 4.11 浏览器访问并登录

1. 打开 `http://localhost:8000`
2. 用户名：`admin`
3. 密码：后端启动日志中打印的随机密码

### 4.12 使用当前 Kuscia 源码仓库的镜像（可选）

如果你希望用 `/home/charles/code/kuscia` 源码构建的镜像替换默认 Kuscia 镜像：

```bash
cd /home/charles/code/kuscia
make image

# 查看构建出的镜像 tag
docker images | grep kuscia

# 在 secretpad 目录执行 install.sh 前设置环境变量
cd /home/charles/code/secretpad
export KUSCIA_IMAGE=my-kuscia:local
bash scripts/install.sh master -P notls
```

> 注意版本兼容性，建议 SecretPad 与 Kuscia 使用官方推荐的版本组合。

---

## 5. 验证 DataMesh 与 SecretPad 的交互

### 5.1 在 SecretPad UI 中查看数据

登录 SecretPad 后：

1. 进入“节点管理”，确认 alice / bob 节点在线。
2. 进入“数据管理”，应能看到 `alice-table`、`bob-table` 等示例数据。
3. 点击数据表详情，可以看到对应的 `DomainData` 信息。

### 5.2 在 Kuscia 容器内查看 DataMesh 数据

```bash
# 查询 alice 的 DomainData
docker exec -it ${USER}-kuscia-lite-alice curl -k \
  https://127.0.0.1:8070/api/v1/datamesh/domaindata/query \
  -X POST -H 'content-type: application/json' \
  --cacert /home/kuscia/var/certs/ca.crt \
  --cert /home/kuscia/var/certs/ca.crt \
  --key /home/kuscia/var/certs/ca.key \
  -d '{"domaindata_id": "alice-table"}'

# 查询 bob 给 alice 的数据授权
docker exec -it ${USER}-kuscia-lite-alice curl -k \
  https://127.0.0.1:8070/api/v1/datamesh/domaindatagrant/query \
  -X POST -H 'content-type: application/json' \
  --cacert /home/kuscia/var/certs/ca.crt \
  --cert /home/kuscia/var/certs/ca.crt \
  --key /home/kuscia/var/certs/ca.key \
  -d '{"domaindatagrant_id": "bob-table-grant"}'
```

### 5.3 创建自定义 DomainData

在 SecretPad UI 中：

1. 选择一个节点（如 alice）。
2. 上传 CSV 文件或指定本地文件路径（需位于 `/home/kuscia/var/storage/data` 下）。
3. 填写字段类型，保存。

SecretPad 会通过 KusciaAPI 自动在对应节点的 DataMesh 中创建 DomainData。

也可以在容器内手动创建：

```bash
docker exec -it ${USER}-kuscia-lite-alice curl -k \
  https://127.0.0.1:8070/api/v1/datamesh/domaindata/create \
  -X POST -H 'content-type: application/json' \
  --cacert /home/kuscia/var/certs/ca.crt \
  --cert /home/kuscia/var/certs/ca.crt \
  --key /home/kuscia/var/certs/ca.key \
  -d '{
    "domain_id": "alice",
    "domaindata_id": "my-table",
    "datasource_id": "default-data-source",
    "name": "my-table",
    "type": "table",
    "relative_uri": "my.csv",
    "columns": [{"name": "id", "type": "str"}, {"name": "x", "type": "float"}]
  }'
```

### 5.4 运行一个隐私计算任务

在 SecretPad UI 中：

1. 创建项目，添加 alice、bob 为合作节点。
2. 在“数据管理”中选择双方的数据表。
3. 进入“训练流”，拖拽组件（如 PSI / 逻辑回归），配置输入输出。
4. 提交任务。

任务运行时，SecretPad → KusciaAPI → Kuscia 调度 → SecretFlow 引擎 → DataMesh 读取/写入数据。可以在 master 容器查看任务状态：

```bash
docker exec -it ${USER}-kuscia-master kubectl get kj -n cross-domain
```

---

## 6. 常见问题

### 6.1 SecretPad 后端启动报错 `Connection refused: localhost/127.0.0.1:18083`

原因：Kuscia 容器的端口没有正确映射到宿主机，或 `/etc/hosts` 解析不正确。

解决：

```bash
docker ps --format "table {{.Names}}\t{{.Ports}}"
```

确认 `charles-kuscia-master` 暴露了 `18083->8083`。如果端口不同，修改 `config/application-dev.yaml` 或对应环境变量。

### 6.2 前端能打开，但 `/api/*` 请求 404/502

原因：前端 dev 服务器没有正确代理到后端。

解决：

```bash
cat frontend-src/apps/platform/.env
# 应为：PROXY_URL=http://127.0.0.1:8080

curl http://localhost:8080/actuator/health
# 应返回 {"status":"UP"}
```

### 6.3 登录提示“用户名或密码错误”

- 确认后端已经启动。
- 确认 `.env` 代理地址正确。
- 密码以后端启动日志中打印的为准，不是固定的。
- 可删除数据库重新初始化：`rm -f db/secretpad.sqlite db/secretpadQuartz.mv.db`。

### 6.4 端口冲突

- `8080`：SecretPad 后端 / 容器内 SecretPad，可修改 `PAD_PORT` 或 `server.http-port`。
- `8000`：前端 dev 服务器，可用 `PORT=8001 pnpm --filter secretpad dev`。
- `18080/18082/18083/13081`：Kuscia master，可通过 `install.sh master -p ... -k ... -g ... -q ...` 修改。

### 6.5 Kuscia 与 SecretPad 版本不匹配

如果你用自己构建的 Kuscia 镜像，请确认 SecretPad 版本支持该 Kuscia 版本。推荐先使用 `scripts/install.sh` 默认拉取的镜像跑通，再逐步替换。

---

## 7. 附录

### 7.1 关键端口速查（install.sh master 默认）

| 服务 | 宿主机端口 | 容器内端口 | 说明 |
|------|-----------|-----------|------|
| SecretPad Web | 8080 | 8080 | 浏览器访问 |
| Kuscia master gateway | 18080 | 1080 | 节点间认证鉴权 |
| Kuscia master KusciaAPI HTTP | 18082 | 8082 | HTTP API |
| Kuscia master KusciaAPI gRPC | 18083 | 8083 | SecretPad 后端连接 |
| Kuscia master Envoy | 13081 | 80 | SecretPad gateway 配置 |
| Kuscia lite alice gateway | 28080 | 1080 |  |
| Kuscia lite alice KusciaAPI gRPC | 28083 | 8083 |  |
| Kuscia lite bob gateway | 38080 | 1080 |  |
| Kuscia lite bob KusciaAPI gRPC | 38083 | 8083 |  |
| DataMesh HTTP | 不暴露宿主机 | 8070 | 仅在容器内访问 |
| DataMesh gRPC | 不暴露宿主机 | 8071 | 引擎内部读取数据 |

### 7.2 关键容器名速查

```bash
# 默认前缀为 ${USER}-kuscia，例如 charles-kuscia-master
export KUSCIA_CTR_PREFIX="${USER}-kuscia"
```

| 节点 | 容器名 |
|------|--------|
| master | `${USER}-kuscia-master` |
| lite alice | `${USER}-kuscia-lite-alice` |
| lite bob | `${USER}-kuscia-lite-bob` |
| lite tee | `${USER}-kuscia-lite-tee` |
| SecretPad | `${USER}-kuscia-master-secretpad` |

### 7.3 推荐启动顺序

1. 启动 Kuscia：`bash scripts/install.sh master -P notls`
2. （源码联调时）停止容器 SecretPad：`docker stop ${USER}-kuscia-master-secretpad`
3. 配置 `/etc/hosts` 或 `application-dev.yaml`
4. 生成证书：`bash scripts/test/setup.sh`
5. 编译后端：`mvn clean install -Dmaven.test.skip=true`
6. 启动后端：`java -Dspring.profiles.active=dev -jar secretpad-web/target/secretpad-web-*.jar`
7. 安装前端依赖并启动：`pnpm --filter secretpad dev`
8. 浏览器访问 `http://localhost:8000` 登录

### 7.4 参考文档

- SecretPad 项目：`/home/charles/code/secretpad/README.zh-CN.md`
- SecretPad 本地运行完整指南：`/home/charles/code/secretpad/docs/development/local_run_guide.md`
- [Kuscia 快速开始](./quickstart_cn.md)
- [DataMesh API 概览](../reference/apis/datamesh/summary_cn.md)
