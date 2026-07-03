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
# 该脚本用于在Kuscia集群中注册SecretFlow应用镜像（AppImage）
# AppImage是Kuscia中表示可执行应用的CRD，定义了：
# - 应用的容器镜像地址和标签
# - 应用类型（secretflow或psi）
# - 应用的元数据和配置信息
#
# 使用场景:
# 1. 隐私计算任务前注册可用的应用镜像
# 2. 管理不同版本的SecretFlow/PSI应用
# 3. 为KusciaJob提供可执行的应用模板
#
# 使用方法: create_secretflow_app_image.sh SF_IMAGE_NAME
# 参数说明:
#   - SF_IMAGE_NAME: SecretFlow镜像的完整名称，格式为 repo:tag
#                    例如: "secretflow/secretflow-anolis8:1.0.0"
#                          "secretflow/psi-anolis8:latest"
#
# 示例:
#   ./create_secretflow_app_image.sh secretflow/secretflow-anolis8:1.0.0
#   ./create_secretflow_app_image.sh secretflow/psi-anolis8:latest

# 严格模式：遇到错误立即退出，防止错误累积
set -e

# ==================== 参数解析与验证 ====================

# 获取第一个命令行参数：SecretFlow镜像名称
SF_IMAGE_NAME=$1

# 定义使用说明信息，用于参数缺失时的提示
usage="$(basename "$0") SF_IMAGE_NAME"

# 验证镜像名称是否为空
# 如果未提供参数，打印使用说明并退出
if [[ ${SF_IMAGE_NAME} == "" ]]; then
  echo "missing argument: $usage"
  exit 1
fi

# ==================== 解析镜像名称和标签 ====================

# 检查镜像名称是否包含标签分隔符 ":"
# 格式通常为: repository:tag （如 secretflow/secretflow-anolis8:1.0.0）
if [[ "${SF_IMAGE_NAME}" == *":"* ]]; then
  # 提取镜像仓库地址（冒号前的部分）
  # ${SF_IMAGE_NAME%%:*} 删除最后一个 : 及其后面的内容
  # 例如: "secretflow/secretflow-anolis8:1.0.0" -> "secretflow/secretflow-anolis8"
  IMAGE_REPO=${SF_IMAGE_NAME%%:*}
  
  # 提取镜像标签（冒号后的部分）
  # ${SF_IMAGE_NAME##*:} 删除最后一个 : 及其前面的内容
  # 例如: "secretflow/secretflow-anolis8:1.0.0" -> "1.0.0"
  IMAGE_TAG=${SF_IMAGE_NAME##*:}
fi

# ==================== 识别应用类型 ====================

# 从镜像仓库地址中提取应用类型
# 处理流程:
# 1. awk -F'/' '{print $NF}' - 以 / 为分隔符，提取最后一部分
#    例如: "secretflow/secretflow-anolis8" -> "secretflow-anolis8"
# 2. awk -F'-' '{print $1}' - 以 - 为分隔符，提取第一部分作为应用类型
#    例如: "secretflow-anolis8" -> "secretflow"
#           "psi-anolis8" -> "psi"
APP_TYPE=$(echo "${IMAGE_REPO}" | awk -F'/' '{print $NF}' | awk -F'-' '{print $1}')

# 判断应用类型
# 如果提取的类型不是 "psi"，则默认为 "secretflow"
# 这支持两种主要的隐私计算应用类型：
# - psi: 私有集合交集（Private Set Intersection）应用
# - secretflow: SecretFlow框架应用（联邦学习、多方安全计算等）
if [[ ${APP_TYPE} != "psi" ]]; then
  APP_TYPE="secretflow"
fi

# ==================== 生成AppImage YAML配置 ====================

# 获取项目根目录路径
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)

# 读取并处理AppImage YAML模板文件
# 根据应用类型选择不同的模板文件：
# - scripts/templates/app_image.psi.yaml (PSI应用)
# - scripts/templates/app_image.secretflow.yaml (SecretFlow应用)
#
# 使用 sed 命令替换模板中的占位符：
# {{.SF_IMAGE_NAME}} -> 镜像仓库地址（加单引号）
# {{.SF_IMAGE_TAG}}  -> 镜像标签（加单引号）
# {{.SF_IMAGE_ID}}   -> 镜像ID（加单引号，当前可能为空或未定义）
#
# 注意：使用 ! 作为sed分隔符而不是 /，避免与镜像地址中的 / 冲突
APP_IMAGE_TEMPLATE=$(sed "s!{{.SF_IMAGE_NAME}}!'${IMAGE_REPO}'!g;
  s!{{.SF_IMAGE_TAG}}!'${IMAGE_TAG}'!g;
  s!{{.SF_IMAGE_ID}}!'${SF_IMAGE_ID}'!g" \
  < "${ROOT}/scripts/templates/app_image.${APP_TYPE}.yaml")

# ==================== 应用配置到Kubernetes集群 ====================

# 将生成的YAML配置通过管道传递给 kubectl apply
# kubectl 会创建或更新 AppImage 资源
# Kuscia的应用控制器会监听该资源并：
# 1. 验证镜像配置的合法性
# 2. 注册应用到应用目录
# 3. 为后续的KusciaJob提供可调用的应用模板
echo "${APP_IMAGE_TEMPLATE}" | kubectl apply -f -