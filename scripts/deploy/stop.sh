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
# 该脚本用于停止本地由 start_standalone.sh 或同类脚本启动的 Kuscia Docker 容器。
#
# 支持按网络模式批量停止：
#   - center: 停止中心化模式容器（Master + Lite），不包含 P2P Autonomy 容器
#   - p2p:    停止 P2P 模式容器（Autonomy）
#   - all:    停止所有 Kuscia 相关容器（默认）
#
# 匹配规则基于容器名称前缀：
#   - 通用前缀：${USER}-kuscia，例如 charles-kuscia-master、charles-kuscia-lite-alice
#   - P2P 前缀：${USER}-kuscia-autonomy，例如 charles-kuscia-autonomy-alice
#
# 注意：本脚本仅执行 docker stop，不会删除容器，也不会清理网络或数据卷。
#       如需彻底清理，请使用 docker rm 或相关清理脚本。
#
# 使用示例：
#   ./stop.sh           # 停止所有 Kuscia 容器
#   ./stop.sh center    # 仅停止中心化模式容器
#   ./stop.sh p2p       # 仅停止 P2P 模式容器
#   ./stop.sh -h        # 显示帮助信息

# 严格模式：遇到错误立即退出
set -e

# 终端颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

# 容器名称前缀定义
# CTR_PREFIX  匹配所有由 start_standalone.sh 创建的 Kuscia 容器
CTR_PREFIX="${USER}-kuscia"
# P2P_PREFIX  专门匹配 P2P 模式下的 Autonomy 容器
P2P_PREFIX="${USER}-kuscia-autonomy"

# ==================== 帮助信息 ====================
function print_usage() {
  echo "$(basename "$0") NETWORK_MODE [OPTIONS]
  NETWORK_MODE:
  center                  stop centralized network mode containers
  p2p                     stop p2p network mode containers
  all                     stop all containers (default)

  Common Options:
  -h                      show this help text"
}

# ==================== 日志输出函数 ====================

# 绿色日志：正常信息
function log() {
  local log_content=$1
  echo -e "${GREEN}${log_content}${NC}"
}

# 红色日志：错误信息，输出到 stderr
function log_error() {
  local log_content=$1
  echo -e "${RED}${log_content}${NC}" >&2
}

# 黄色日志：提示/警告信息
function log_hint(){
  local log_content=$1
  echo -e "${YELLOW}${log_content}${NC}"
}

# ==================== 容器列表获取 ====================

# 根据指定的容器类型，获取当前正在运行的 Kuscia 容器名称列表。
#
# 参数：
#   container_type: p2p / center / all
#
# 匹配逻辑：
#   - p2p:   名称以 P2P_PREFIX（${USER}-kuscia-autonomy）开头的运行中容器
#   - center: 名称以 CTR_PREFIX（${USER}-kuscia）开头，但排除 P2P 容器
#   - all:    所有名称以 CTR_PREFIX 开头的运行中容器
function get_running_container_list() {
  local container_type=$1

  case "$container_type" in
      "p2p")
          container_list=$(docker ps --format '{{.Names}}' -f name=^"${P2P_PREFIX}")
          ;;
      "center")
          container_list=$(docker ps --format '{{.Names}}' -f name=^"${CTR_PREFIX}" | grep -v ^"${P2P_PREFIX}")
          ;;
      "all")
          container_list=$(docker ps --format '{{.Names}}' -f name=^"${CTR_PREFIX}")
          ;;
  esac

  echo "$container_list"
}

# ==================== 停止容器 ====================

# 停止指定类型的 Kuscia 容器。
# 若该类型下没有运行中的容器，则打印黄色提示；
# 否则调用 docker stop 批量停止，并打印绿色成功信息。
function stop_container() {
  local container_type=$1

  container_list=$(get_running_container_list "$container_type")

  if [ -z "$container_list" ]; then
      case "$container_type" in
          "p2p")
              log_hint "No Kuscia p2p containers running!"
              ;;
          "center")
              log_hint "No Kuscia center containers running!"
              ;;
          "all")
              log_hint "No Kuscia containers running!"
              ;;
      esac
  else
      log "Stopping Kuscia $container_type containers ..."
      docker stop "$container_list"
      log "Kuscia $container_type containers stopped successfully!"
  fi
}

# ==================== 入口逻辑 ====================

# 无参数时默认停止所有 Kuscia 容器
if [ $# -eq 0 ]; then
  stop_container "all"
elif [ "$1" == "-h" ]; then
  # 显示帮助信息
  print_usage
  exit 0
else
  # 根据第一个参数分派到对应的停止类型
  case "$1" in
  "p2p")
      stop_container "p2p"
      ;;
  "center")
      stop_container "center"
      ;;
  "all")
      stop_container "all"
      ;;
  *)
      log_error "Invalid network mode: $1"
      print_usage
      exit 1
      ;;
  esac
fi
