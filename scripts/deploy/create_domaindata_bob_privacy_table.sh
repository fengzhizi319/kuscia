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

# 脚本功能：在Kuscia系统中创建Bob方的隐私组件测试DomainData资源对象
# 该脚本会根据模板文件创建一个指定域ID的Bob隐私表数据定义，并将其应用到Kubernetes集群中
# 隐私表包含 PII 字段（name/id_card/mobile）、准标识符（age/zipcode）、
# 敏感属性（department）和数值列（bonus/has_disease），用于测试
# K-匿名、差分隐私、数据脱敏、L-多样性、本地差分隐私等隐私组件。

# 设置脚本选项：遇到错误时立即退出
set -e

# 获取第一个命令行参数作为域ID
DOMAIN_ID=$1

# 定义使用方法提示信息
usage="$(basename "$0") DOMAIN_ID"

# 检查是否提供了域ID参数，如果没有则显示使用方法并退出
if [[ ${DOMAIN_ID} == "" ]]; then
  echo "missing argument: $usage"
  exit 1
fi

# 计算项目根目录路径
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)

# 使用sed命令替换模板中的占位符{{.DOMAIN_ID}}为实际的域ID值
# 读取domaindata_bob_privacy_table.yaml模板文件并进行变量替换
DOMAIN_DATASOURCE_TEMPLATE=$(sed "s/{{.DOMAIN_ID}}/${DOMAIN_ID}/g;" \
  < "${ROOT}/scripts/templates/domaindata_bob_privacy_table.yaml")

# 将处理后的模板内容通过管道传递给 kubectl apply 命令
# 在Kubernetes集群中创建或更新DomainData资源对象
echo "${DOMAIN_DATASOURCE_TEMPLATE}" | kubectl apply -f -
