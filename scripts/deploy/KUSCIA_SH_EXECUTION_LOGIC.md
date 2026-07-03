# Kuscia 部署脚本执行逻辑说明

## 概述

`kuscia.sh` 是 Kuscia 的核心部署脚本，用于快速启动和配置不同网络模式的 Kuscia 集群。

## 支持的部署模式

### 1. **center（中心化模式）**
- **架构**：1个Master节点 + 多个Lite节点
- **特点**：Master负责调度和管理，Lite执行计算任务
- **适用场景**：生产环境，需要集中管理

### 2. **p2p（点对点模式）**
- **架构**：多个Autonomy节点互相连接
- **特点**：每个节点都有完整功能，对等通信
- **适用场景**：小规模协作，无需中心节点

### 3. **cxc（Center x Center模式）**
- **架构**：多个Master节点互联，每个Master下有Lite节点
- **特点**：多层级管理，支持跨域路由转发
- **适用场景**：大规模多组织协作

### 4. **cxp（Center x P2P模式）**
- **架构**：Master节点与Autonomy节点混合
- **特点**：结合中心化和点对点的优势
- **适用场景**：混合组网场景

### 5. **start（单机/多机模式）**
- **架构**：根据配置文件启动单个节点
- **特点**：灵活配置，可独立运行或加入现有集群
- **适用场景**：自定义部署、测试环境

## 核心执行流程

### 阶段一：初始化准备

```bash
# 1. 设置默认镜像地址
KUSCIA_IMAGE -> Kuscia主镜像
SECRETFLOW_IMAGE -> SecretFlow计算引擎
DATAPROXY_IMAGE -> DataProxy数据代理
KUSCIA_MONITOR_IMAGE -> 监控服务

# 2. 解析SecretFlow镜像信息
init_sf_image_info() 
  - 提取仓库地址、镜像名称、标签

# 3. 解析命令行参数
while getopts ... 
  - 解析各种选项（-a, -c, -d, -l, -m, -p等）
```

### 阶段二：容器启动

#### 2.1 初始化工作目录

```bash
init() 
  - 创建DOMAIN_WORK_DIR（工作目录）
  - 创建子目录：data（数据）、logs（日志）、images（镜像）、k3s（数据库）
  - 检查目录权限
```

#### 2.2 生成配置文件

```bash
init_kuscia_conf_file()
  - Lite节点：从Master获取Deploy Token
  - Autonomy/Master节点：直接生成配置
  - 调用 `kuscia init --mode <mode>` 生成kuscia.yaml
  - 追加DataProxy配置（如果启用）
  - 追加协议特定配置（BFIA或Kuscia）
```

#### 2.3 启动Docker容器

```bash
start_container()
  1. 构建docker run命令
     - 设置特权模式（runc需要）
     - 挂载配置文件：kuscia.yaml
     - 挂载数据卷：data、logs、images、k3s
     - 暴露端口：80(内部)、1080(网关)、8082(KusciaAPI HTTP)、8083(KusciaAPI gRPC)、9091(Metrics)
     - 设置内存限制
  
  2. 执行docker run启动容器
     命令格式：docker run -dit --name=<ctr> --hostname=<host> \
              --network=kuscia-exchange \
              -v <config>:/home/kuscia/etc/conf/kuscia.yaml \
              -v <data>:/home/kuscia/var/storage/data \
              -v <logs>:/home/kuscia/var/stdout \
              -v <images>:/home/kuscia/var/images \
              -p 13081:80 -p <host_port>:1080 \
              -p 13082:8082 -p 13083:8083 -p 13084:9091 \
              <image> bin/kuscia start -c etc/conf/kuscia.yaml
  
  3. 导入内置镜像（runp模式）
  
  4. 健康检查
     - probe_gateway_crd(): 检查Gateway CRD（非Lite节点）
     - probe_datamesh(): 检查DataMesh服务（非Master节点）
```

### 阶段三：后处理

#### 3.1 导入SecretFlow镜像

```bash
# 对于非Master节点
register_app_image.sh -c <container> -i <sf_image> --import
  - 将SecretFlow镜像导入到容器的镜像存储中

# 对于非Lite节点
scripts/deploy/register_app_image.sh -i <sf_image> -m
  - 注册SecretFlow AppImage CRD
```

#### 3.2 启动DataProxy（可选）

```bash
start_data_proxy()
  1. 导入DataProxy镜像
  2. 注册DataProxy AppImage
  3. 通过KusciaAPI创建Serving部署
     POST /api/v1/serving/create
     {
       "serving_id": "dataproxy-<domain>",
       "initiator": "<domain>",
       "parties": [{
         "domain_id": "<domain>",
         "app_image": "dataproxy-image",
         "service_name_prefix": "dataproxy"
       }]
     }
```

### 阶段四：组网配置（集群模式）

#### 4.1 Center模式

```bash
start_center_cluster()
  1. 启动Master节点（domain=kuscia-system）
  2. 启动Alice Lite节点（连接到Master）
  3. 启动Bob Lite节点（连接到Master）
  4. 创建双向ClusterDomainRoute
     - alice -> bob: http://alice-lite-bob:1080
     - bob -> alice: http://bob-lite-alice:1080
  5. 初始化示例数据
```

#### 4.2 P2P模式

```bash
start_p2p_cluster()
  1. 启动Alice Autonomy节点
  2. 启动Bob Autonomy节点
  3. 建立双向互联
     build_interconn(bob, alice, alice_domain, bob_domain, protocol)
       - 复制证书：bob的domain.crt -> alice的alice_domain.domain.crt
       - 添加域：alice添加bob为对等域
       - 加入主机：bob通过https://alice:1080加入alice
  4. 反向建立互联（alice -> bob）
  5. 初始化示例数据
```

#### 4.3 CxC模式

```bash
start_cxc_cluster()
  1. 启动两个Master节点（master-cxc-alice, master-cxc-bob）
  2. 启动两个Lite节点（分别连接到各自的Master）
  3. Master之间建立P2P互联
  4. 复制证书跨Master
  5. 添加跨域配置
  6. 配置路由转发（transit模式）
     - 直接模式：Lite直接通信
     - 中转模式：通过Master转发
```

#### 4.4 CxP模式

```bash
start_cxp_cluster()
  1. 启动Master节点（master-cxp-alice）
  2. 启动Alice Lite节点（连接到Master）
  3. 启动Bob Autonomy节点（也连接到Alice的Master）
  4. Bob与Master建立互联
  5. Alice Lite与Bob Autonomy建立互联
  6. 配置路由转发
```

## 关键组件端口

| 组件 | 端口 | 协议 | 说明 |
|------|------|------|------|
| K3s API Server | 6443 | HTTPS | Kubernetes API |
| Gateway | 1080 | HTTPS | 域间通信网关 |
| DataMesh HTTP | 8070 | HTTPS | 元数据管理REST API |
| DataMesh gRPC | 8071 | gRPC | 数据传输服务 |
| KusciaAPI HTTP | 8082 | HTTP/HTTPS | Kuscia管理API |
| KusciaAPI gRPC | 8083 | gRPC | Kuscia gRPC API |
| Metrics | 9091 | HTTP | Prometheus指标采集 |
| Internal | 80 | HTTP | 内部服务端口 |

## 健康检查机制

### 1. K3s检查
```bash
probe_k3s()
  - 端点：https://127.0.0.1:6443
  - 重试：60次
  - 成功条件：HTTP状态码 200/404/401
```

### 2. Gateway检查
```bash
probe_gateway_crd()
  - 命令：kubectl get gateways -n <domain>
  - 重试：60次
  - 成功条件：找到匹配的Gateway资源
```

### 3. DataMesh检查
```bash
probe_datamesh()
  - 端点：https://127.0.0.1:8070/healthZ
  - 认证：mTLS双向认证
  - 重试：30次
  - 成功条件：HTTP状态码 200/404/401
```

## 配置文件结构

生成的 `kuscia.yaml` 包含以下关键配置：

```yaml
# 通用配置
mode: lite/master/autonomy          # 节点类型
domainID: alice                     # 域ID
domainKeyData: <base64私钥>         # 域私钥
logLevel: INFO                      # 日志级别

# Lite专用配置
liteDeployToken: <token>            # 部署令牌
masterEndpoint: https://<ip>:1080   # Master地址

# 运行时配置
runtime: runc/runk/runp             # 容器运行时

# 资源容量
capacity:
  cpu: 4
  memory: 8Gi
  pods: 500

# DataMesh配置
dataMesh:
  dataProxyList:
    - endpoint: "dataproxy-grpc:8023"
      dataSourceTypes: ["odps", "hive", ...]
```

## 环境变量

可通过环境变量覆盖默认配置：

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| KUSCIA_IMAGE | secretflow/kuscia:latest | Kuscia镜像 |
| SECRETFLOW_IMAGE | secretflow-lite-anolis8:1.11.0b1 | SecretFlow镜像 |
| DATAPROXY_IMAGE | dataproxy:0.1.0b1 | DataProxy镜像 |
| DOMAIN_HOST_PORT | - | 网关外部端口 |
| KUSCIAAPI_HTTP_PORT | 13082 | KusciaAPI HTTP端口 |
| KUSCIAAPI_GRPC_PORT | 13083 | KusciaAPI gRPC端口 |
| METRICS_PORT | 13084 | Metrics端口 |
| MEMORY_LIMIT | 自动 | 内存限制 |
| RUNTIME | runc | 运行时类型 |

## 常见问题

### Q1: 容器启动失败？
- 检查端口是否被占用
- 查看容器日志：`docker logs <container>`
- 检查K3s日志：`/home/kuscia/var/logs/k3s.log`
- 检查Envoy日志：`/home/kuscia/var/logs/envoy`

### Q2: 健康检查超时？
- 增加重试次数或等待时间
- 检查网络连接
- 验证证书是否正确

### Q3: 如何清理重新部署？
```bash
# 停止并删除所有Kuscia容器
docker rm -f $(docker ps -aq --filter name=kuscia)

# 删除工作目录
rm -rf ~/kuscia-*

# 删除Docker网络
docker network rm kuscia-exchange
```

## 最佳实践

1. **生产环境**：使用center模式，启用enableWorkloadApprove
2. **开发测试**：使用p2p模式或单机start模式
3. **多组织协作**：使用cxc或cxp模式
4. **资源规划**：Master 2GB、Lite 4GB、Autonomy 6GB
5. **数据安全**：使用安全的密钥管理方案，不要明文存储私钥
