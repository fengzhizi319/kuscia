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
# 该脚本用于在指定域中创建Alice方的DomainData资源（数据表）
# DomainData是Kuscia中表示数据的CRD，定义了：
# - 数据的元信息（名称、类型、格式等）
# - 数据的存储位置和访问方式
# - 数据的归属域和权限控制
#
# 使用场景:
# 1. 隐私计算任务前准备数据资源
# 2. 注册本地数据到Kuscia数据网格
# 3. 配置跨域数据共享的源数据
#
# 使用方法: create_domaindata_alice_table.sh DOMAIN_ID
# 参数说明:
#   - DOMAIN_ID: 域ID，指定在哪个域中创建数据资源，如 "alice"
#
# 示例:
#   ./create_domaindata_alice_table.sh alice    # 在alice域中创建示例数据表

# 严格模式：遇到错误立即退出，防止错误累积
set -e

# ==================== 参数解析与验证 ====================

# 获取第一个命令行参数：域ID
# 该参数指定要在哪个域中创建DomainData资源
DOMAIN_ID=$1

# 定义使用说明信息，用于参数缺失时的提示
usage="$(basename "$0") DOMAIN_ID"

# 验证域ID是否为空
# 如果未提供参数，打印使用说明并退出
if [[ ${DOMAIN_ID} == "" ]]; then
  echo "missing argument: $usage"
  exit 1
fi

# ==================== 模板处理与资源配置 ====================

# 获取项目根目录路径
# 通过 BASH_SOURCE[0] 获取当前脚本所在目录，然后向上两级定位到项目根目录
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)

# 读取并处理DomainData YAML模板文件
# 模板文件位于 scripts/templates/domaindata_alice_table.yaml
# 该模板定义了Alice方的示例数据表配置，包括：
# - 数据源类型（如CSV文件、数据库表等）
# - 数据存储路径和格式
# - 数据schema定义（列名、数据类型等）
# - 访问控制和授权信息
#
# 使用 sed 命令将模板中的占位符 {{.DOMAIN_ID}} 替换为实际的域ID
# 例如: 如果 DOMAIN_ID="alice"，则所有 {{.DOMAIN_ID}} 都会被替换为 "alice"
DOMAIN_DATASOURCE_TEMPLATE=$(sed "s/{{.DOMAIN_ID}}/${DOMAIN_ID}/g;" \
  < "${ROOT}/scripts/templates/domaindata_alice_table.yaml")

# ==================== 应用配置到Kubernetes集群 ====================

# 将生成的YAML配置通过管道传递给 kubectl apply
# kubectl 会创建或更新 DomainData 资源
# Kuscia的数据控制器会监听该资源并：
# 1. 验证数据配置的合法性
# 2. 注册数据到数据网格（DataMesh）
# 3. 配置数据访问代理（DataProxy）
# 4. 生成数据元信息和索引
echo "${DOMAIN_DATASOURCE_TEMPLATE}" | kubectl apply -f -