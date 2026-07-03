#!/bin/bash
#
# Copyright 2023 Ant Group Co., Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# ==================== 脚本功能说明 ====================
# 该脚本用于创建跨域路由（ClusterDomainRoute）资源，实现不同域之间的通信连接
# ClusterDomainRoute是Kuscia中用于配置域间路由规则的核心CRD，定义了：
# - 源域（Source Domain）如何访问目标域（Destination Domain）
# - 目标域的端点地址、协议、端口等连接信息
# - 是否使用中转域（Transit Domain）进行间接通信
#
# 使用场景:
# 1. P2P组网模式：直接配置两个域之间的点对点路由
# 2. 中心化组网模式：通过中转域实现域间通信
# 3. 混合组网：部分域直连，部分域通过中转
#
# 使用方法: create_cluster_domain_route.sh SRC_DOMAIN DEST_DOMAIN DEST_ENDPOINT [TRANSIT_DOMAIN]
# 参数说明:
#   - SRC_DOMAIN: 源域ID，发起通信的域，如 "alice"
#   - DEST_DOMAIN: 目标域ID，接收通信的域，如 "bob"
#   - DEST_ENDPOINT: 目标域的访问端点，格式为 http(s)://ip:port[/path]
#                    例如: "http://192.168.1.100:8080" 或 "https://bob.example.com/api"
#   - TRANSIT_DOMAIN: 中转域ID（可选），如果指定则通过该域转发流量
#
# 示例:
#   ./create_cluster_domain_route.sh alice bob http://10.0.0.2:8080
#   ./create_cluster_domain_route.sh alice bob https://bob.example.com:443/api
#   ./create_cluster_domain_route.sh alice bob http://10.0.0.2:8080 gateway

# 严格模式：遇到错误立即退出，防止错误累积
set -e

# ==================== 参数解析与验证 ====================

# 获取四个命令行参数
SRC_DOMAIN=$1        # 源域ID
DEST_DOMAIN=$2       # 目标域ID
DEST_ENDPOINT=$3     # 目标域端点URL
TRANSIT_DOMAIN=$4    # 中转域ID（可选）

# 定义使用说明信息
usage="$(basename "$0") SRC_DOMAIN DEST_DOMAIN DEST_ENDPOINT(http(s)://ip:port) [TRANSIT_DOMAIN]"

# 验证必需参数是否提供：源域、目标域、目标端点
# 中转域是可选参数，可以为空
if [[ ${SRC_DOMAIN} == "" || ${DEST_DOMAIN} == "" || ${DEST_ENDPOINT} == "" ]]; then
  echo "missing argument: $usage"
  exit 1
fi

# ==================== URL解析与端点信息提取 ====================

# 获取项目根目录路径
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)

# 初始化端点解析变量
HOST=${DEST_ENDPOINT}     # 主机地址（初始化为完整URL，后续会被覆盖）
PORT=80                   # 默认端口：HTTP使用80
PROTOCOL_TLS=false        # 是否使用TLS加密（HTTPS）
HOST_PATH="/"             # URL路径，默认为根路径

# 步骤1：检测协议类型（HTTP vs HTTPS）
# 根据URL前缀判断是否需要TLS加密
if [[ "${DEST_ENDPOINT}" == https://* ]]; then
    PROTOCOL_TLS=true
  else
    PROTOCOL_TLS=false
fi

# 步骤2：移除协议前缀，提取主机:端口/路径部分
# 使用参数扩展 ${var##*://} 删除最长的匹配前缀（包括 ://）
# 例如: "https://bob.example.com:443/api" -> "bob.example.com:443/api"
HOST_PORT_PATH=${DEST_ENDPOINT##*://}

# 步骤3：分离路径和主机:端口
# 检查是否包含路径分隔符 "/"
if [[ "${HOST_PORT_PATH}" == *"/"* ]]; then
  # 包含路径：提取路径部分和主机:端口部分
  # ${HOST_PORT_PATH#*/} 删除第一个 / 之前的内容，得到路径
  # 例如: "bob.example.com:443/api" -> "/api"
  HOST_PATH="/${HOST_PORT_PATH#*/}"
  # ${HOST_PORT_PATH%%/*} 删除第一个 / 之后的内容，得到主机:端口
  # 例如: "bob.example.com:443/api" -> "bob.example.com:443"
  HOST_PORT=${HOST_PORT_PATH%%/*}
else
  # 不包含路径：使用默认根路径，整个字符串为主机:端口
  HOST_PATH="/"
  HOST_PORT="$HOST_PORT_PATH"
fi

# 步骤4：分离主机和端口
# 检查主机:端口字符串中是否包含 ":"
if [[ "${HOST_PORT}" == *":"* ]]; then
  # 包含端口号：分别提取主机和端口
  # ${HOST_PORT##*:} 提取最后一个 : 之后的内容（端口）
  PORT=${HOST_PORT##*:}
  # ${HOST_PORT%%:*} 提取第一个 : 之前的内容（主机）
  HOST=${HOST_PORT%%:*}
else
  # 不包含端口号：使用默认端口
  # 根据协议类型选择默认端口：HTTPS->443, HTTP->80
  if [[ "${DEST_ENDPOINT}" == https://* ]]; then
    PORT=443
  else
    PORT=80
  fi
  HOST="$HOST_PORT"
fi

# ==================== 准备模板变量 ====================

# 设置互联协议类型
# kuscia协议是默认的域间通信协议，支持加密传输和身份认证
INTERCONN_PROTOCOL=kuscia

# ==================== 生成ClusterDomainRoute YAML配置 ====================

# 根据是否指定中转域，选择不同的YAML模板文件
# 模板文件位于 scripts/templates/ 目录下，使用占位符 {{.VAR}} 标记需要替换的变量

if [[ ${TRANSIT_DOMAIN} == "" ]]; then
  # 情况1：直接路由模式（无中转域）
  # 使用标准模板 cluster_domain_route.token.yaml
  # 适用于P2P直连场景，源域直接连接目标域
  
  # 使用 sed 命令替换模板中的占位符：
  # {{.SRC_DOMAIN}}         -> 源域ID
  # {{.DEST_DOMAIN}}        -> 目标域ID
  # {{.HOST}}               -> 目标主机地址
  # {{.PATH}}               -> URL路径（使用 @ 作为sed分隔符避免与 / 冲突）
  # {{.ISTLS}}              -> 是否使用TLS
  # {{.INTERCONN_PROTOCOL}} -> 互联协议类型
  # {{.PORT}}               -> 目标端口号
  CLUSTER_DOMAIN_ROUTE_TEMPLATE=$(sed "s/{{.SRC_DOMAIN}}/${SRC_DOMAIN}/g;
    s/{{.DEST_DOMAIN}}/${DEST_DOMAIN}/g;
    s/{{.HOST}}/${HOST}/g;
    s@{{.PATH}}@${HOST_PATH}@g;
    s/{{.ISTLS}}/${PROTOCOL_TLS}/g;
    s/{{.INTERCONN_PROTOCOL}}/${INTERCONN_PROTOCOL}/g;
    s/{{.PORT}}/${PORT}/g" \
    <"${ROOT}/scripts/templates/cluster_domain_route.token.yaml")
else
  # 情况2：中转路由模式（有中转域）
  # 使用中转模板 cluster_domain_route.token.transit.yaml
  # 适用于中心化组网，流量通过中转域转发
  
  # 额外替换 {{.TRANSIT_DOMAIN}} 占位符
  CLUSTER_DOMAIN_ROUTE_TEMPLATE=$(sed "s/{{.SRC_DOMAIN}}/${SRC_DOMAIN}/g;
    s/{{.DEST_DOMAIN}}/${DEST_DOMAIN}/g;
    s/{{.HOST}}/${HOST}/g;
    s@{{.PATH}}@${HOST_PATH}@g;
    s/{{.ISTLS}}/${PROTOCOL_TLS}/g;
    s/{{.PORT}}/${PORT}/g;
    s/{{.TRANSIT_DOMAIN}}/${TRANSIT_DOMAIN}/g" \
    <"${ROOT}/scripts/templates/cluster_domain_route.token.transit.yaml")
fi
# ==================== 应用配置到Kubernetes集群 ====================

# 将生成的YAML配置通过管道传递给 kubectl apply
# kubectl 会创建或更新 ClusterDomainRoute 资源
# Kuscia的路由控制器会监听该资源并配置实际的网关路由规则
echo "${CLUSTER_DOMAIN_ROUTE_TEMPLATE}" | kubectl apply -f -