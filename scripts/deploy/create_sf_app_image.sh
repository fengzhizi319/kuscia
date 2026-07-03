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
# 该脚本用于在 Kuscia 集群中注册 SecretFlow/PSI 应用镜像（AppImage）。
#
# AppImage 是 Kuscia 定义的一种 CRD（Custom Resource Definition），
# 描述了一个可被执行的隐私计算应用，包含：
#   - 容器镜像的仓库地址、标签、镜像 ID
#   - 应用类型（secretflow / psi / 其他自定义类型）
#   - 部署模板：容器启动命令、端口、ConfigVolume 挂载、重启策略等
#   - 配置模板：任务运行时需要注入的 task-config.conf
#
# 与 create_secretflow_app_image.sh 的区别：
#   - create_secretflow_app_image.sh 只接收一个 "repo:tag" 参数，自动解析类型；
#   - 本脚本将镜像名称、标签、应用类型、镜像 ID 作为独立参数传入，
#     更加灵活，可直接复用 scripts/templates/app_image.<APP_TYPE>.yaml 模板。
#
# 使用场景：
#   1. 在提交 KusciaJob 前，先向集群注册可用的应用镜像；
#   2. 为同一镜像注册不同类型（secretflow、psi 等）的 AppImage；
#   3. 在 CI/CD 或本地部署流程中批量注册应用镜像。
#
# 前置条件：
#   - 当前环境已配置 kubectl，并且能够访问目标 Kuscia 集群；
#   - 模板文件 scripts/templates/app_image.<APP_TYPE>.yaml 必须存在。
#
# 使用方法：
#   create_sf_app_image.sh <SF_IMAGE_NAME> <SF_IMAGE_TAG> [APP_TYPE] [SF_IMAGE_ID]
#
# 参数说明：
#   SF_IMAGE_NAME: 容器镜像仓库名称，不含标签。
#                  例如："secretflow/secretflow-anolis8"
#   SF_IMAGE_TAG:  容器镜像标签。
#                  例如："1.10.0b0"、"latest"
#   APP_TYPE:      应用类型，决定使用哪个模板文件。
#                  可选值：secretflow（默认）、psi 或任意自定义值。
#                  模板文件路径：scripts/templates/app_image.${APP_TYPE}.yaml
#   SF_IMAGE_ID:   容器镜像 ID（可选），主要用于 PSI 模板中的 spec.image.id 字段。
#
# 示例：
#   ./create_sf_app_image.sh secretflow/secretflow-anolis8 1.10.0b0 secretflow
#   ./create_sf_app_image.sh secretflow/psi-anolis8 latest psi sha256:xxx

# 严格模式：遇到错误立即退出，防止错误继续传播
set -e

# ==================== 参数解析 ====================

# 第一个参数：容器镜像仓库名称（不含标签）
SF_IMAGE_NAME=$1

# 第二个参数：容器镜像标签
SF_IMAGE_TAG=$2

# 第三个参数：应用类型，未提供时默认使用 secretflow
APP_TYPE=$3

# 第四个参数：容器镜像 ID，PSI 等模板可能会使用
SF_IMAGE_ID=$4

# 如果未传入 APP_TYPE，则默认注册为 secretflow 类型
if [[ ${APP_TYPE} == "" ]]; then
  APP_TYPE="secretflow"
fi

# 定义使用说明，参数缺失时向用户展示
usage="$(basename "$0") SF_IMAGE_NAME SF_IMAGE_TAG [APP_TYPE] [SF_IMAGE_ID]"

# 验证必填参数：镜像名称和标签不能为空
if [[ ${SF_IMAGE_NAME} == "" || ${SF_IMAGE_TAG} == "" ]]; then
  echo "missing argument: $usage"
  exit 1
fi

# ==================== 计算项目根目录 ====================

# 获取脚本所在目录的绝对路径，再向上回退两级到达项目根目录
# dirname "${BASH_SOURCE[0]}"  -> 脚本所在目录：scripts/deploy
# /../..                       -> 项目根目录
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)

# ==================== 生成 AppImage YAML 配置 ====================

# 读取对应应用类型的 YAML 模板，并使用 sed 替换其中的占位符。
#
# 模板文件位置：
#   ${ROOT}/scripts/templates/app_image.secretflow.yaml
#   ${ROOT}/scripts/templates/app_image.psi.yaml
#   ${ROOT}/scripts/templates/app_image.<APP_TYPE>.yaml
#
# 模板中的占位符及替换规则：
#   {{.SF_IMAGE_NAME}} -> 容器镜像仓库名称（本脚本用单引号包裹，保留字符串原样）
#   {{.SF_IMAGE_TAG}}  -> 容器镜像标签
#   {{.SF_IMAGE_ID}}   -> 容器镜像 ID
#
# 注意：sed 使用 "!" 作为分隔符，而不是默认的 "/"，
# 这样可以避免镜像名称中包含的 "/" 与 sed 分隔符冲突。
APP_IMAGE_TEMPLATE=$(sed "s!{{.SF_IMAGE_NAME}}!'${SF_IMAGE_NAME}'!g;
  s!{{.SF_IMAGE_TAG}}!'${SF_IMAGE_TAG}'!g;
  s!{{.SF_IMAGE_ID}}!'${SF_IMAGE_ID}'!g" \
  < "${ROOT}/scripts/templates/app_image.${APP_TYPE}.yaml")

# ==================== 应用配置到 Kubernetes 集群 ====================

# 将生成的 AppImage YAML 通过管道传给 kubectl apply，
# 在目标 Kuscia 集群中创建或更新 AppImage 资源。
#
# 执行后，Kuscia 应用控制器会：
#   1. 校验 AppImage 配置的合法性；
#   2. 将该应用注册到集群的应用目录；
#   3. 后续 KusciaJob 可以通过 appImageRef 引用该应用来启动任务 Pod。
#
# 注意：本脚本直接使用宿主机的 kubectl 访问集群；
# 如果需要向 Docker 容器内的 Kuscia 注册镜像，请参考 register_app_image.sh。
echo "${APP_IMAGE_TEMPLATE}" | kubectl apply -f -
