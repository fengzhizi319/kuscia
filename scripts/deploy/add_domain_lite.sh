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

# ==================== 脚本功能说明 ====================
# 该脚本用于向Kuscia集群中添加一个Lite模式的域（Domain）
# Lite模式是简化版的域配置，主要用于：
# 1. 轻量级节点快速接入
# 2. 自动化部署场景中的子节点注册
# 3. 不需要复杂证书配置的测试环境
#
# 与 add_domain.sh 的区别：
# - 不要求提供TLS证书文件
# - 不支持P2P模式和互联协议配置
# - 自动等待并返回部署令牌（deploy token）用于后续节点激活
#
# 使用方法: add_domain_lite.sh DOMAIN_ID [MASTER_DOMAIN_ID]
# 参数说明:
#   - DOMAIN_ID: 域的唯一标识符（必需），如 "lite-node-1"
#   - MASTER_DOMAIN_ID: 主域ID（可选），指定该Lite域归属的主域
#
# 执行流程:
# 1. 创建空的Domain资源（不含证书）
# 2. 等待Kuscia控制器生成部署令牌
# 3. 输出令牌供后续节点初始化使用
#
# 示例:
#   ./add_domain_lite.sh lite-alice              # 添加Lite域，无主域
#   ./add_domain_lite.sh lite-bob alice          # 添加Lite域，归属于alice主域

# 严格模式：遇到错误立即退出，防止错误累积
set -e

# ==================== 参数解析与验证 ====================

# 定义使用说明信息，用于参数缺失时的提示
usage="$(basename "$0") DOMAIN_ID [MASTER_DOMAIN_ID]"

# 获取第一个参数：域ID（必需参数）
DOMAIN_ID=$1

# 验证域ID是否为空，如果为空则打印使用说明并退出
if [[ ${DOMAIN_ID} == "" ]]; then
  echo "missing argument: $usage"
  exit 1
fi

# 获取第二个参数：主域ID（可选参数）
# Lite域通常需要指定一个主域来进行集中管理和认证
# 如果未指定，则为空值，表示独立域或由系统自动分配
MASTER_DOMAIN_ID=$2

# ==================== 创建Lite Domain资源 ====================

# 构建简化的Domain YAML配置并应用到Kubernetes集群
# Lite模式的Domain特点：
# - spec.cert: 留空，不使用预置证书
# - spec.role: 留空，由系统根据上下文自动判断
# - spec.master: 指向主域，建立层级关系
# - authCenter: 保持标准的Token认证和RSA密钥生成配置
#
# 工作流程：
# 1. kubectl apply 创建/更新Domain资源
# 2. Kuscia的Domain控制器监听到新Domain
# 3. 控制器自动生成部署令牌（deploy token）并写入status.deployTokenStatuses
# 4. 输出重定向到 /dev/null，隐藏kubectl的成功消息
# 利用 Here Document（<<）和 管道 将内联的 YAML 配置直接传递给 kubectl apply
# 这样可以在脚本中动态生成YAML，而无需单独创建文件
echo "
apiVersion: kuscia.secretflow/v1alpha1
kind: Domain
metadata:
  annotations:
    domain/${DOMAIN_ID}: kuscia.secretflow/domain-type=embedded
  name: ${DOMAIN_ID}
spec:
  cert:
  role:
  master: ${MASTER_DOMAIN_ID}
  authCenter:
    authenticationType: Token
    tokenGenMethod: RSA-GEN
" | kubectl apply -f - > /dev/null

# ==================== 等待部署令牌生成 ====================

# 定义等待函数：轮询查询Domain状态，直到获取到可用的部署令牌
# 部署令牌是Lite节点激活的关键凭证，用于后续的节点初始化和认证
#
# 参数:
#   domain_id: 要查询的域ID
#
# 工作原理:
# 1. 通过 kubectl get domain 获取Domain资源的JSON格式状态
# 2. 使用 jsonpath 提取 deployTokenStatuses 数组中 state=="unused" 的token
# 3. "unused" 状态表示令牌已生成但尚未被使用，可用于节点激活
# 4. 如果令牌为空，则等待1秒后重试，最多重试60次（约1分钟）
# 5. 一旦获取到令牌或达到最大重试次数，函数返回
function wait_csr_token() {
  local domain_id=$1
  local retry=0
  local max_retry=60
    sleep 1
  # 这是一个带超时的轮询函数：每秒查询一次 K8s Domain 资源，用 JSONPath 过滤出未使用的部署令牌，获取成功后立即返回，60 秒未获取则放弃。
  while [ $retry -lt $max_retry ]; do
    # 从Domain状态的deployTokenStatuses中提取未使用的令牌
    # jsonpath表达式解释:
    # .status.deployTokenStatuses - 访问部署令牌状态数组
    # [?(@.state=="unused")]     - 过滤出状态为"unused"的元素
    # .token                       - 提取token字段值
    csrToken=$(kubectl get domain "${domain_id}" -o=jsonpath='{.status.deployTokenStatuses[?(@.state=="unused")].token}')
    if [[ $csrToken != "" ]]; then
      return
    fi
    sleep 1
    retry=$((retry + 1))
  done
}

# ==================== 执行等待并输出令牌 ====================

# 调用等待函数，阻塞直到获取到部署令牌或超时
wait_csr_token "${DOMAIN_ID}"

# 将获取到的部署令牌输出到标准输出
# 调用者可以捕获此输出来进行后续的节点初始化操作
# 例如: TOKEN=$(./add_domain_lite.sh lite-node alice)
#       echo "Deploy token: $TOKEN"
echo "${csrToken}"
