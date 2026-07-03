#!/bin/bash
#
# Copyright 2025 Ant Group Co., Ltd.
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

# 严格模式：遇到错误立即退出，防止错误累积
set -e

# ==================== 使用说明 ====================
# 该脚本用于向Kuscia集群中添加一个新的域（Domain）
# Domain是Kuscia中表示参与方的核心资源，代表一个独立的组织或节点
# 
# 使用方法: add_domain.sh DOMAIN_ID [ROLE] [INTERCONN_PROTOCOL] [MASTER_DOMAIN_ID]
# 参数说明:
#   - DOMAIN_ID: 域的唯一标识符（必需），如 "alice", "bob"
#   - ROLE: 域的角色类型（可选），值为 "p2p" 时表示对等节点角色
#   - INTERCONN_PROTOCOL: 互联协议类型（可选），默认为 "kuscia"
#   - MASTER_DOMAIN_ID: 主域ID（可选），默认为DOMAIN_ID本身
#
# 示例:
#   ./add_domain.sh alice                    # 添加普通域 alice
#   ./add_domain.sh bob p2p                  # 添加P2P模式的域 bob
#   ./add_domain.sh charlie p2p grpc master1 # 指定互联协议和主域

usage="$(basename "$0") DOMAIN_ID [ROLE] [INTERCONN_PROTOCOL] [MASTER_DOMAIN_ID]"

# ==================== 参数解析与验证 ====================

# 获取第一个参数：域ID（必需参数）
DOMAIN_ID=$1
if [[ ${DOMAIN_ID} == "" ]]; then
  echo "missing argument: $usage"
  exit 1
fi

# 处理第二个参数：角色类型
# 如果指定为 "p2p"，则将角色设置为 "partner"（合作伙伴/对等节点）
# partner 角色表示该域在P2P组网模式下作为对等方参与
ROLE=
if [[ $2 == p2p ]]; then
  ROLE=partner
fi

# 处理第三个参数：互联协议类型
# 支持多种跨域通信协议，如 "kuscia"（默认）、"grpc" 等
# 该协议决定了不同域之间的通信方式和数据交换格式
INTERCONN_PROTOCOL=$3
[ "${INTERCONN_PROTOCOL}" != "" ] || INTERCONN_PROTOCOL="kuscia"

# 处理第四个参数：主域ID
# 在中心化组网模式中，需要指定一个主域（Master Domain）来协调其他域
# 如果未指定，则默认将当前域设置为自己的主域（适用于独立域或主域自身）
MASTER_DOMAIN_ID=$4
if [[ $MASTER_DOMAIN_ID == "" ]]; then
  MASTER_DOMAIN_ID=$DOMAIN_ID
fi

# ==================== 证书准备 ====================

# 获取项目根目录路径
# 通过 BASH_SOURCE[0] 获取当前脚本所在目录，然后向上两级定位到项目根目录
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)

# 读取并编码域证书文件
# 每个域都需要一个TLS证书用于安全通信和身份认证
# 证书文件位于 var/certs/{DOMAIN_ID}.domain.crt
# 使用 base64 编码并将结果转换为单行字符串，以便嵌入到YAML中
CERT=$(base64 "$ROOT"/var/certs/"${DOMAIN_ID}".domain.crt | tr -d "\n")

# ==================== 构建Domain YAML模板 ====================

# 创建Kubernetes Domain资源的YAML模板
# Domain CRD定义了域的元数据和配置信息，包括：
# - metadata.name: 域的唯一标识
# - spec.cert: TLS证书（base64编码）
# - spec.role: 域的角色（空表示中心节点，"partner"表示对等节点）
# - spec.master: 主域ID，用于确定域的归属关系
# - spec.authCenter: 认证中心配置，使用Token认证和RSA密钥生成方式
DOMAIN_TEMPLATE="
apiVersion: kuscia.secretflow/v1alpha1
kind: Domain
metadata:
  annotations:
    domain/${DOMAIN_ID}: kuscia.secretflow/domain-type=embedded
  name: ${DOMAIN_ID}
spec:
  cert: ${CERT}
  role: ${ROLE}
  master: ${MASTER_DOMAIN_ID}
  authCenter:
    authenticationType: Token
    tokenGenMethod: RSA-GEN
"
# ==================== 条件字段追加 ====================

# 如果第二个参数为 "p2p"，则添加互联协议配置
# P2P模式需要明确指定使用的互联协议列表，以支持对等节点间的直接通信
# 使用 printf 格式化字符串，确保缩进正确（2个空格）
if [ "$2" == "p2p" ]; then
  APPEND_LINE=$(printf "\n%*sinterConnProtocols: [ '${INTERCONN_PROTOCOL}' ]" "2")
  DOMAIN_TEMPLATE="${DOMAIN_TEMPLATE}${APPEND_LINE}"
fi

# ==================== 应用配置到Kubernetes集群 ====================

# 通过 kubectl 将生成的YAML配置应用到Kubernetes集群
# 使用管道将YAML内容传递给 kubectl apply，实现声明式资源配置
# 如果Domain已存在则更新，不存在则创建
echo "${DOMAIN_TEMPLATE}" | kubectl apply -f -
