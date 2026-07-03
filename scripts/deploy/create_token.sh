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
# 该脚本用于为指定的 Kuscia 域（Domain）创建一个长期有效的 Kubernetes ServiceAccount Token。
#
# 在 Kuscia 中，每个 Domain 对应一个 Kubernetes Namespace，并且在该 Namespace 下
# 会创建一个与 Domain 同名的 ServiceAccount。本脚本通过 kubectl 为该 ServiceAccount
# 签发一个 Token，供 Lite 节点或外部组件加入 Master 时使用。
#
# 使用场景：
#   1. Master 节点创建 Domain 后，需要生成 Deploy Token 供 Lite 节点注册；
#   2. 手动生成一个长期 Token 用于脚本化部署、CI/CD 或调试；
#   3. 替代旧的 UID-RSA 方式，配合 RSA-GEN 认证模式完成节点接入。
#
# 前置条件：
#   - 当前环境已配置 kubectl，且能够访问目标 Kuscia 集群；
#   - 指定的 DOMAIN_ID 必须已经存在，并且对应 Namespace 下已创建同名 ServiceAccount。
#
# 使用方法：
#   create_token.sh <DOMAIN_ID>
#
# 参数说明：
#   DOMAIN_ID: Kuscia 域标识，例如 "alice"、"bob" 或 "root-kuscia-lite-alice" 等。
#
# 示例：
#   ./create_token.sh alice
#   ./create_token.sh bob

# 严格模式：遇到错误立即退出，防止错误继续传播
set -e

# 定义使用说明，参数缺失时向用户展示
usage="$(basename "$0") DOMAIN_ID"

# 第一个参数：Kuscia 域标识
DOMAIN_ID=$1

# 验证 DOMAIN_ID 是否为空
if [[ ${DOMAIN_ID} == "" ]]; then
  echo "missing argument: $usage"
  exit 1
fi

# 计算项目根目录（当前脚本位于 scripts/deploy，向上回退两级到达项目根目录）
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)

# ==================== 创建 ServiceAccount Token ====================

# 使用 kubectl create token 为指定 Domain 的 ServiceAccount 签发 Token。
#
# 参数说明：
#   $DOMAIN_ID:        ServiceAccount 名称，与 Domain ID 相同；
#   -n $DOMAIN_ID:     指定该 ServiceAccount 所在的 Namespace，同样与 Domain ID 相同；
#   --duration 87600h: Token 有效期，87600 小时约合 10 年，满足长期部署需求。
#
# 输出：
#   命令执行成功后，会在标准输出打印签发的 JWT Token，可直接复制用于 Lite 节点部署
#   或写入部署脚本的 KUSCIA_TOKEN 环境变量。
#
# 注意：
#   - 该 Token 绑定到对应 Namespace 的 ServiceAccount 权限；
#   - 若 Domain/ServiceAccount 不存在，命令会失败，请先通过 Master 创建 Domain。
kubectl create token "$DOMAIN_ID" --duration 87600h -n "$DOMAIN_ID"

# ==================== 调试示例（已注释） ====================

# 以下命令可用于验证生成的 Token 是否能访问对应 Namespace 的 Pod 资源。
# 将 ${TOKEN} 替换为上方命令输出的实际 Token 即可测试。
#
# curl -k https://127.0.0.1:6443/api/v1/namespaces/${DOMAIN_ID}/pods \
#      -H "Authorization: Bearer ${TOKEN}"
