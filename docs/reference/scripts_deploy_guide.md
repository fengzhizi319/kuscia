# scripts/deploy 目录脚本功能详解

本文档详细介绍了 Kuscia 项目中 `scripts/deploy` 目录下所有 shell 脚本的功能、使用方法和应用场景。

## 📋 目录

- [部署启动类脚本](#部署启动类脚本)
- [资源配置类脚本](#资源配置类脚本)
- [数据初始化类脚本](#数据初始化类脚本)
- [证书与安全类脚本](#证书与安全类脚本)
- [监控与运维类脚本](#监控与运维类脚本)
- [测试与调试类脚本](#测试与调试类脚本)

---

## 部署启动类脚本

### 1. kuscia.sh - 主部署脚本 ⭐

**文件**: `kuscia.sh` (1183行)

**功能**: Kuscia Docker 部署的主入口脚本，支持多种网络拓扑模式的自动化部署。

**支持的部署模式**:

- **p2p**: 点对点组网（2个 Autonomy 节点：alice + bob）
- **center/centralized**: 中心化组网（1个 Master + 2个 Lite 节点）
- **cxc**: 中心化 x 中心化（2个 Master + 2个 Lite 节点）
- **cxp**: 中心化 x 点对点（1个 Master + 1个 Lite + 1个 Autonomy）

**主要功能**:

```bash
# 1. 架构检查（x86_64 / ARM64）
arch_check()

# 2. K3s 数据目录初始化
init_k3s_data()

# 3. 配置文件生成与包装
wrap_kuscia_config_file()

# 4. 容器生命周期管理
need_start_docker_container()
start_autonomy()
start_master()
start_lite()

# 5. SecretFlow 镜像处理
check_sf_image()
create_secretflow_app_image()

# 6. 跨域网络连接
build_interconn()
create_cluster_domain_route()

# 7. 示例数据创建
create_domaindata_alice_table()
create_domaindata_bob_table()
create_domaindatagrant_alice2bob()
create_domaindatagrant_bob2alice()
```

**使用方法**:

```bash
# 下载脚本
docker run --rm ${KUSCIA_IMAGE} cat /home/kuscia/scripts/deploy/kuscia.sh > kuscia.sh
chmod u+x kuscia.sh

# 启动不同模式
./kuscia.sh p2p        # 点对点
./kuscia.sh center     # 中心化
./kuscia.sh cxc        # 中心化x中心化
./kuscia.sh cxp        # 中心化x点对点
```

**环境变量**:

```bash
KUSCIA_IMAGE          # Kuscia 镜像地址
SECRETFLOW_IMAGE      # SecretFlow 引擎镜像
DATAPROXY_IMAGE       # DataProxy 镜像
KUSCIA_MONITOR_IMAGE  # 监控镜像
ALLOW_PRIVILEGED      # 是否允许特权容器
```

---

### 2. start_standalone.sh - 独立部署脚本

**文件**: `start_standalone.sh` (802行)

**功能**: 在 `.local-kuscia` 环境中启动独立的 Kuscia 集群，主要用于本地开发和测试。

**特点**:

- 基于 Docker Compose / Swarm
- 支持 MySQL 作为外部数据存储
- 可配置副本数量（多实例部署）
- 支持反向隧道测试

**使用方法**:

```bash
cd .local-kuscia
./scripts/deploy/start_standalone.sh center  # 或 p2p
```

---

### 3. deploy.sh - 单节点部署脚本

**文件**: `deploy.sh` (652行)

**功能**: 部署单个 Kuscia 节点（Autonomy / Master / Lite 三种模式）。

**支持的模式**:

```bash
# 部署 Autonomy 节点
./deploy.sh autonomy -n alice -p 11080

# 部署 Master 节点
./deploy.sh master -n kuscia-system -p 18080

# 部署 Lite 节点
./deploy.sh lite -n alice -p 28080 \
  -m https://master:1080 \
  -t <deploy-token>
```

**参数说明**:

- `-c`: 配置文件路径
- `-d`: 数据存储目录
- `-l`: 日志存储目录
- `-n`: Domain ID
- `-p`: 外部端口
- `-q`: 内部端口
- `-k`: KusciaAPI HTTP 端口
- `-g`: KusciaAPI gRPC 端口
- `-m`: Master 端点（Lite 模式必需）
- `-t`: 部署 Token（Lite 模式必需）

---

### 4. stop.sh - 停止集群脚本

**文件**: `stop.sh` (119行)

**功能**: 停止运行中的 Kuscia 容器（保留数据和卷）。

**使用方法**:

```bash
./stop.sh p2p    # 停止 P2P 模式容器
./stop.sh center # 停止中心化模式容器
./stop.sh all    # 停止所有容器（默认）
```

**工作流程**:

1. 查找匹配的容器（按名称前缀）
2. 提示用户确认
3. 执行 `docker stop`

---

### 5. uninstall.sh - 卸载集群脚本

**文件**: `uninstall.sh` (250行)

**功能**: 完全卸载 Kuscia 集群，包括容器、卷和网络。

**使用方法**:

```bash
./uninstall.sh p2p    # 卸载 P2P 模式
./uninstall.sh center # 卸载中心化模式
./uninstall.sh all    # 卸载所有（默认）
```

**清理内容**:

- ✅ 停止并删除容器
- ✅ 删除 Docker 卷
- ✅ 删除 Docker 网络（如果没有其他容器使用）

**警告**: ⚠️ 此操作不可逆，会删除所有数据！

---

### 6. run_docker_quickstart.sh - Docker 快速启动脚本

**文件**: `run_docker_quickstart.sh` (106行)

**功能**: 一键式 Docker 快速启动脚本，自动拉取镜像、提取部署脚本并启动集群。

**使用方法**:

```bash
# 默认 P2P 模式
./run_docker_quickstart.sh

# 指定模式
./run_docker_quickstart.sh center

# 指定镜像版本
KUSCIA_IMAGE=secretflow/kuscia:1.2.0b0 ./run_docker_quickstart.sh p2p
```

**自动化流程**:

1. 检查 Docker 是否安装
2. 拉取 Kuscia 镜像
3. 从镜像中提取 `kuscia.sh` 脚本
4. 启动指定模式的集群
5. 输出验证命令

---

## 资源配置类脚本

### 7. add_domain.sh - 添加域资源

**文件**: `add_domain.sh` (64行)

**功能**: 创建 Domain CRD 资源，用于注册新的参与方。

**使用方法**:

```bash
# P2P 模式
./add_domain.sh bob p2p kuscia

# 中心化模式
./add_domain.sh alice partner kuscia kuscia-system
```

**参数**:

- `$1`: Domain ID（如 alice, bob）
- `$2`: 角色（p2p 表示对等伙伴）
- `$3`: 互联协议（kuscia / bfia，默认 kuscia）
- `$4`: Master Domain ID

**生成的资源**:

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: Domain
metadata:
  name: bob
spec:
  cert: <base64-encoded-cert>
  role: partner
  master: kuscia-system
  authCenter:
    authenticationType: Token
    tokenGenMethod: RSA-GEN
  interConnProtocols: ['kuscia']
```

---

### 8. add_domain_lite.sh - 添加 Lite 域并获取 Token

**文件**: `add_domain_lite.sh` (63行)

**功能**: 为 Lite 节点创建 Domain 资源，并等待 CSR Token 生成。

**使用方法**:

```bash
TOKEN=$(./add_domain_lite.sh alice kuscia-system)
echo "Deploy Token: $TOKEN"
```

**工作流程**:

1. 创建空的 Domain 资源（无证书）
2. 轮询等待 Controller 生成 Deploy Token
3. 返回 Token 供 Lite 节点使用

**输出**: CSR Token 字符串

---

### 9. create_cluster_domain_route.sh - 创建跨域路由

**文件**: `create_cluster_domain_route.sh` (86行)

**功能**: 创建 ClusterDomainRoute CRD，配置域间通信路由。

**使用方法**:

```bash
# 直接路由
./create_cluster_domain_route.sh alice bob http://bob-lite:1080

# 中转路由
./create_cluster_domain_route.sh alice bob http://bob-lite:1080 transit-domain
```

**参数**:

- `$1`: 源域 ID
- `$2`: 目标域 ID
- `$3`: 目标端点（http(s)://ip:port/path）
- `$4`: 中转域 ID（可选）

**自动解析**:

- 协议（HTTP/HTTPS）
- 主机名和端口
- URL 路径

---

### 10. join_to_host.sh - 加入宿主域

**文件**: `join_to_host.sh` (160行)

**功能**: 配置当前域连接到宿主域，建立跨域通信通道。

**使用方法**:

```bash
# MTLS 认证
./join_to_host.sh alice bob https://bob-host:1080 -p kuscia

# Token 认证
./join_to_host.sh alice bob https://bob-host:1080 \
  -a Token \
  -t <token> \
  -p kuscia

# 中转连接
./join_to_host.sh alice bob https://bob-host:1080 \
  -x transit-domain
```

**参数**:

- `$1`: 自身域 ID
- `$2`: 宿主域 ID
- `$3`: 宿主端点
- `-p`: 互联协议（默认 kuscia）
- `-a`: 认证类型（MTLS / Token / None）
- `-t`: Token（Token 认证时必需）
- `-x`: 中转域 ID
- `-k`: 允许不安全 SSL 连接
- `-i`: 是否需要 InteropConfig

**生成的资源**:

- ClusterDomainRoute CRD
- 可能包含中转配置

---

### 11. register_app_image.sh - 注册应用镜像

**文件**: `register_app_image.sh` (194行)

**功能**: 将 Docker 镜像导入 Kuscia 并注册为 AppImage CRD。

**使用方法**:

```bash
# 方式 1: 导入镜像并注册
./register_app_image.sh \
  -c root-kuscia-autonomy-alice \
  -i secretflow/psi:latest \
  --import

# 方式 2: 使用自定义模板
./register_app_image.sh \
  -c root-kuscia-autonomy-alice \
  -i secretflow/fl:latest \
  -f ./my-app-image.yaml

# 方式 3: 仅注册（镜像已存在）
./register_app_image.sh \
  -c root-kuscia-autonomy-alice \
  -i secretflow/mpc:latest
```

**参数**:

- `-c`: Kuscia 容器名称
- `-i`: 应用镜像地址
- `-f`: AppImage 模板文件（可选）
- `-n`: SecretFlow 名称（默认 secretflow-image）
- `--import`: 是否导入镜像到容器

**工作流程**:

1. 检查镜像是否已存在于容器中
2. 如果不存在，从宿主机导入或拉取
3. 使用 `kuscia image load` 导入到 containerd
4. 根据模板创建 AppImage CRD

---

### 12. create_secretflow_app_image.sh - 创建 SecretFlow AppImage

**文件**: `create_secretflow_app_image.sh` (46行)

**功能**: 根据镜像名称自动检测类型并创建对应的 AppImage。

**使用方法**:

```bash
export SF_IMAGE_ID=<image-id-from-crictl>
./create_secretflow_app_image.sh secretflow/psi:latest
```

**自动检测**:

- 从镜像名称提取类型（psi / secretflow）
- 选择对应的模板文件
- 替换镜像名称、标签和 ID

---

### 13. create_sf_app_image.sh - 创建 SecretFlow AppImage（简化版）

**文件**: `create_sf_app_image.sh` (43行)

**功能**: 与上一个脚本类似，但参数更明确。

**使用方法**:

```bash
./create_sf_app_image.sh secretflow/psi latest psi <image-id>
```

**参数**:

- `$1`: 镜像名称
- `$2`: 镜像标签
- `$3`: 应用类型（psi / secretflow / dataproxy / kuscia）
- `$4`: 镜像 ID

---

### 14. create_secretpad_svc.sh - 创建 SecretPad 服务

**文件**: `create_secretpad_svc.sh` (36行)

**功能**: 为 SecretPad（可视化界面）创建 Kubernetes Service。

**使用方法**:

```bash
./create_secretpad_svc.sh secretpad-container alice
```

**参数**:

- `$1`: SecretPad 容器名称
- `$2`: Domain ID

---

### 15. set_kernel_params.sh - 设置内核参数

**文件**: `set_kernel_params.sh` (35行)

**功能**: 优化 Linux 内核网络参数以提升 Kuscia 性能。

**使用方法**:

```bash
sudo ./set_kernel_params.sh
```

**调整的参数**:

```bash
# TCP SYN  backlog 队列大小
tcp_max_syn_backlog = 2048

# 监听队列大小
somaxconn = 2048

# TCP 重试次数（5次 ≈ 25-51秒）
tcp_retries2 = 5

# 禁用慢启动空闲连接
tcp_slow_start_after_idle = 0

# 复用 TIME_WAIT socket
tcp_tw_reuse = 1

# 最大文件打开数
file-max = 102400
```

**适用场景**: 高并发隐私计算任务

---

## 数据初始化类脚本

### 16. create_domaindata_alice_table.sh - 创建 Alice 示例数据

**文件**: `create_domaindata_alice_table.sh` (48行)

**功能**: 为 Alice 域创建示例 DomainData 资源（银行营销数据集）。

**使用方法**:

```bash
./create_domaindata_alice_table.sh alice
```

**数据来源**: 

- 模板文件: `scripts/templates/domaindata_alice_table.yaml`
- 实际数据: `/home/kuscia/var/storage/data/alice.csv`

**字段示例**:

- id1, age, education, default, balance
- housing, loan, day, duration, campaign
- 等 20+ 个特征列

---

### 17. create_domaindata_bob_table.sh - 创建 Bob 示例数据

**文件**: `create_domaindata_bob_table.sh` (49行)

**功能**: 为 Bob 域创建示例 DomainData 资源。

**使用方法**:

```bash
./create_domaindata_bob_table.sh bob
```

**字段示例**:

- id2, contact_cellular, contact_telephone
- month_apr, month_aug, ..., month_sep
- poutcome_failure, poutcome_success
- y (标签列)

---

### 18. init_example_data.sh - 初始化示例数据和授权

**文件**: `init_example_data.sh` (38行)

**功能**: 创建示例 DomainData 并配置跨域数据授权。

**使用方法**:

```bash
# 在 Alice 节点执行
./init_example_data.sh alice

# 在 Bob 节点执行
./init_example_data.sh bob
```

**执行的操作**:

1. 通过 KusciaAPI 创建 DomainData
2. 通过 DataMesh API 创建 DomainDataGrant
3. 配置跨域数据访问权限

**API 调用示例**:

```bash
# 创建 DomainData
curl -X POST 'https://127.0.0.1:8082/api/v1/domaindata/create' \
  --header "Token: $(cat /home/kuscia/var/certs/token)" \
  --header 'Content-Type: application/json' \
  -d '{...}'

# 创建授权
curl https://127.0.0.1:8070/api/v1/datamesh/domaindatagrant/create \
  -X POST \
  -H 'content-type: application/json' \
  -d '{"author":"alice","domaindata_id":"alice-table","grant_domain":"bob"}'
```

---

## 证书与安全类脚本

### 19. generate_cert.sh - 生成域证书

**文件**: `generate_cert.sh` (40行)

**功能**: 根据域私钥生成自签名证书。

**使用方法**:

```bash
./generate_cert.sh alice <base64-encoded-private-key>
```

**参数**:

- `$1`: Domain ID
- `$2`: Base64 编码的私钥

**生成的文件**:

- `{domain}.key`: 私钥文件
- `{domain}.csr`: 证书签名请求
- `{domain}.crt`: 自签名证书（有效期 100 年）

**输出**: 证书内容（stdout）

---

### 20. generate_rsa_key.sh - 生成 RSA 私钥

**文件**: `generate_rsa_key.sh` (20行)

**功能**: 生成 2048 位 RSA 私钥并 Base64 编码。

**使用方法**:

```bash
./generate_rsa_key.sh
```

**输出**:

```
Generate domain private key configuration:

LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcEFJQkFBS0NBUUVB...
```

**用途**: 用于 Domain 资源的 `authCenter.tokenGenMethod: RSA-GEN`

---

### 21. init_kusciaapi_client_certs.sh - 初始化 KusciaAPI 客户端证书

**文件**: `init_kusciaapi_client_certs.sh` (39行)

**功能**: 为 KusciaAPI 客户端生成 TLS 证书（PKCS#8 格式，Java 兼容）。

**使用方法**:

```bash
./init_kusciaapi_client_certs.sh
```

**生成的文件** (在 `var/certs/` 目录):

- `kusciaapi-client.key`: PKCS#8 私钥
- `kusciaapi-client.csr`: CSR
- `kusciaapi-client.crt`: 由 CA 签名的证书

**特点**:

- 使用 `openssl genpkey` 生成 PKCS#8 格式密钥
- 由根 CA (`ca.crt/ca.key`) 签名
- 有效期 1000 天
- CN = "KusciaAPIClient"

---

### 22. create_token.sh - 创建 Kubernetes Token

**文件**: `create_token.sh` (35行)

**功能**: 为指定 Namespace 生成长期有效的 Kubernetes ServiceAccount Token。

**使用方法**:

```bash
TOKEN=$(./create_token.sh alice)
echo $TOKEN
```

**参数**:

- `$1`: Domain ID (Namespace)

**Token 特性**:

- 有效期: 87600 小时（10 年）
- 用途: KusciaAPI 身份认证
- 存储位置: `/home/kuscia/var/certs/token`

**测试用法**:

```bash
curl https://127.0.0.1:6443/api/v1/namespaces/alice/pods \
  -H "Authorization: Bearer ${TOKEN}"
```

---

### 23. gen_kusciaapi_token.sh - 生成 KusciaAPI Token

**文件**: `gen_kusciaapi_token.sh` (34行)

**功能**: 使用域私钥签名生成 KusciaAPI 认证 Token。

**使用方法**:

```bash
./gen_kusciaapi_token.sh alice <base64-domain-key>
```

**参数**:

- `$1`: Domain ID
- `$2`: Base64 编码的域私钥

**生成过程**:

1. 解码私钥
2. 使用 SHA256 签名 Domain ID
3. Base64 编码签名结果
4. 截取前 32 字符作为 Token

**输出**: 32 字符 Token

---

## 监控与运维类脚本

### 24. start_monitor.sh - 启动监控系统

**文件**: `start_monitor.sh` (155行)

**功能**: 启动 Prometheus + Grafana 监控套件。

**使用方法**:

```bash
# P2P 模式
./start_monitor.sh p2p

# 中心化模式
./start_monitor.sh center
```

**启动的服务**:

- **Prometheus**: 指标收集（端口 9090）
- **Grafana**: 可视化仪表板（端口 3000）

**监控目标**:

```yaml
# 中心化模式
- master:9091
- alice-lite:9091
- bob-lite:9091

# P2P 模式
- alice-autonomy:9091
- bob-autonomy:9091
```

**Grafana 默认账户**:

- 用户名: `admin`
- 密码: `admin`

---

### 25. init_kuscia_monitor.sh - 初始化监控容器

**文件**: `init_kuscia_monitor.sh` (39行)

**功能**: 在监控容器内初始化 Prometheus 和 Grafana。

**工作流程**:

1. 后台启动 Prometheus
2. 后台启动 Grafana
3. 等待 Grafana 就绪
4. 获取 Datasource UID
5. 更新仪表板配置
6. 重启 Grafana 应用配置
7. 保持容器运行

**配置文件**:

- Prometheus: `/home/config/prometheus.yml`
- Grafana: `/etc/grafana/grafana.ini`

---

## 测试与调试类脚本

### 26. create_reverse_tunnel_test_cluster.sh - 创建反向隧道测试集群

**文件**: `create_reverse_tunnel_test_cluster.sh` (203行)

**功能**: 创建用于测试反向隧道功能的复杂集群环境。

**特点**:

- 使用 Docker Swarm
- 多副本部署（Alice: 3 副本，Bob: 1 副本）
- 外部 MySQL 存储
- 自动配置反向隧道

**使用方法**:

```bash
export KUSCIA_IMAGE=secretflow/kuscia:latest
./create_reverse_tunnel_test_cluster.sh
```

**部署架构**:

```
MySQL (13307) ←→ Alice Replica Set (3 instances)
                    ↓ 反向隧道
MySQL (13308) ←→ Bob (1 instance)
```

**主要步骤**:

1. 创建 Overlay 网络
2. 生成 kuscia.yaml（使用 MySQL 后端）
3. 部署 Docker Stack
4. 交换域证书
5. 配置反向隧道 CDR
6. 创建示例数据和授权

**清理**:

```bash
docker stack rm kuscia-autonomy
```

---

## 📊 脚本分类总览

| 类别 | 脚本数量 | 主要脚本 |
| ------ | --------- | --------- |
| **部署启动** | 6 | kuscia.sh, deploy.sh, stop.sh, uninstall.sh |
| **资源配置** | 9 | add_domain.sh, join_to_host.sh, register_app_image.sh |
| **数据初始化** | 3 | create_domaindata_*.sh, init_example_data.sh |
| **证书安全** | 5 | generate_cert.sh, init_kusciaapi_client_certs.sh |
| **监控运维** | 2 | start_monitor.sh, init_kuscia_monitor.sh |
| **测试调试** | 1 | create_reverse_tunnel_test_cluster.sh |

---

## 🔗 脚本依赖关系图

```
用户入口
  ├─ run_docker_quickstart.sh
  │   └─> kuscia.sh
  │       ├─> add_domain.sh
  │       ├─> join_to_host.sh
  │       ├─> create_cluster_domain_route.sh
  │       ├─> create_sf_app_image.sh
  │       ├─> create_domaindata_alice_table.sh
  │       └─> create_domaindata_bob_table.sh
  │
  ├─ deploy.sh (单节点)
  │   ├─> add_domain_lite.sh
  │   └─> create_sf_app_image.sh
  │
  └─ start_monitor.sh
      └─> init_kuscia_monitor.sh (容器内)

辅助工具
  ├─ register_app_image.sh (镜像注册)
  ├─ generate_cert.sh (证书生成)
  ├─ generate_rsa_key.sh (密钥生成)
  ├─ init_kusciaapi_client_certs.sh (客户端证书)
  ├─ create_token.sh (K8s Token)
  ├─ gen_kusciaapi_token.sh (API Token)
  └─ set_kernel_params.sh (性能优化)

测试工具
  └─ create_reverse_tunnel_test_cluster.sh
```

---

## 💡 最佳实践

### 1. 快速开始

```bash
# 推荐新手使用
./run_docker_quickstart.sh p2p

# 验证部署
docker ps
docker exec -it ${USER}-kuscia-autonomy-alice kubectl get pods -n alice
```

### 2. 生产部署

```bash
# 1. 优化内核参数
sudo ./set_kernel_params.sh

# 2. 部署中心化集群
./kuscia.sh center

# 3. 启动监控
./start_monitor.sh center

# 4. 注册自定义镜像
./register_app_image.sh -c root-kuscia-master -i my-app:latest --import
```

### 3. 开发调试

```bash
# 1. 使用本地模式
cd .local-kuscia
./scripts/deploy/start_standalone.sh p2p

# 2. 查看日志
docker logs -f ${USER}-kuscia-autonomy-alice

# 3. 进入容器调试
docker exec -it ${USER}-kuscia-autonomy-alice bash
```

### 4. 清理环境

```bash
# 停止但不删除数据
./stop.sh all

# 完全清理（危险！）
./uninstall.sh all
```

---

## ⚠️ 注意事项

1. **权限要求**: 部分脚本需要 `sudo` 权限（如 `set_kernel_params.sh`）
2. **网络要求**: 确保 Docker 网络正常，端口未被占用
3. **磁盘空间**: 至少预留 100GB 可用空间
4. **内存要求**: P2P 模式至少 8GB，中心化模式至少 16GB
5. **数据备份**: 执行 `uninstall.sh` 前务必备份重要数据
6. **版本兼容**: 确保脚本与 Kuscia 镜像版本匹配

---

## 📖 相关文档

- [Kuscia 快速入门](../getting_started/quickstart_cn.md)
- [如何本地启动 Kuscia](../getting_started/如何本地启动Kuscia.md)
- [Docker 部署指南](../deployment/Docker_deployment_kuscia/)
- [Kuscia 配置详解](../deployment/kuscia_config_cn.md)
- [监控与诊断](../deployment/kuscia_engine_monitor.md)

---

**最后更新**: 2025-07-02  
**维护者**: Kuscia Team
