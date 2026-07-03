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
# 该脚本用于为SecretPad容器创建Kubernetes Service资源
# SecretPad是SecretFlow的可视化操作界面，提供：
# - 隐私计算任务的图形化配置和管理
# - 数据资源的浏览和授权管理
# - 任务执行状态的监控和结果查看
#
# Service的作用:
# - 为SecretPad Pod提供稳定的网络访问端点
# - 实现负载均衡（如果有多个副本）
# - 使SecretPad可以通过域名或IP被外部访问
#
# 使用方法: create_secretpad_svc.sh SECRET_PAD_CTR_NAME DOMAIN_ID
# 参数说明:
#   - SECRET_PAD_CTR_NAME: SecretPad容器名称，如 "secretpad"
#   - DOMAIN_ID: 域ID，指定SecretPad所属的域，如 "alice"（可选）
#
# 示例:
#   ./create_secretpad_svc.sh secretpad alice    # 为alice域的secretpad创建Service
#   ./create_secretpad_svc.sh secretpad          # 不指定域ID

# 严格模式：遇到错误立即退出，防止错误累积
set -e

# ==================== 参数解析与验证 ====================

# 获取第一个命令行参数：SecretPad容器名称
SECRET_PAD_CTR_NAME=$1

# 获取第二个命令行参数：域ID（可选）
DOMAIN_ID=$2

# 定义使用说明信息，用于参数缺失时的提示
usage="$(basename "$0") SECRET_PAD_CTR_NAME DOMAIN_ID"

# 验证必需参数：SecretPad容器名称不能为空
# 域ID是可选参数，可以为空
if [[ ${SECRET_PAD_CTR_NAME} == "" ]]; then
  echo "missing argument: $usage"
  exit 1
fi

# ==================== 生成Service YAML配置 ====================

# 获取项目根目录路径
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)

# 读取并处理SecretPad Service YAML模板文件
# 模板文件位于 scripts/templates/secretpad_svc.yaml
# 该模板定义了Kubernetes Service的配置，包括：
# - Service类型（ClusterIP/NodePort/LoadBalancer）
# - 端口映射规则
# - 选择器（selector）用于关联目标Pod
# - 会话亲和性设置
#
# 使用 sed 命令替换模板中的占位符：
# {{.SECRET_PAD_CTR_NAME}} -> SecretPad容器名称
# {{.DOMAIN}}              -> 域ID（用于命名空间或标签选择器）
SECRET_PAD_SVC_TEMPLATE=$(sed "s/{{.SECRET_PAD_CTR_NAME}}/${SECRET_PAD_CTR_NAME}/g;
 s/{{.DOMAIN}}/${DOMAIN_ID}/g;" \
  < "${ROOT}/scripts/templates/secretpad_svc.yaml")

# ==================== 应用配置到Kubernetes集群 ====================

# 将生成的YAML配置通过管道传递给 kubectl apply
# kubectl 会创建或更新 Service 资源
# Kubernetes的服务控制器会：
# 1. 分配虚拟IP地址（ClusterIP）
# 2. 配置iptables或IPVS规则实现流量转发
# 3. 根据selector自动发现并关联匹配的Pod
# 4. 提供负载均衡和健康检查功能
echo "${SECRET_PAD_SVC_TEMPLATE}" | kubectl apply -f -