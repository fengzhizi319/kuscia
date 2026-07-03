#!/bin/bash
#
# Copyright 2025 Ant Group Co., Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# ==================== 脚本功能说明 ====================
# 该脚本用于在 Docker 环境中部署 Kuscia 的三种节点角色：
#   - Master: 中央控制节点，负责域名管理、任务调度、证书签发等；
#   - Lite:   受控工作节点，需要注册到 Master，执行实际的隐私计算任务；
#   - Autonomy: 自治节点，既包含控制面也包含数据面，可独立运行，也可与其他 Autonomy 节点互联（P2P/CxC）。
#
# 部署流程概述：
#   1. 检查宿主机架构（仅支持 x86_64 / amd64 / arm64）；
#   2. 解析命令行参数与环境变量，初始化镜像、端口、目录等配置；
#   3. 创建 Docker 网络 kuscia-exchange，供容器间通信；
#   4. 按需拉取 Kuscia 与 SecretFlow 镜像；
#   5. 生成或加载 kuscia.yaml 配置文件，启动 Kuscia 容器；
#   6. 探针检测 k3s、Gateway、DataMesh 等关键组件是否就绪；
#   7. 初始化 KusciaAPI 客户端证书；
#   8. 导入 SecretFlow/PSI 引擎镜像并创建 AppImage；
#   9. 创建示例 DomainData（Alice/Bob 数据表），便于快速体验。
#
# 前置条件：
#   - 已安装 Docker，并且当前用户有权限操作 Docker；
#   - 可访问镜像仓库（默认：secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/）；
#   - 目标端口未被占用（默认 1080/80/8082/8083 及自定义主机端口）。

# 严格模式：遇到错误立即退出，防止错误继续传播
set -e

# 终端颜色定义，用于输出高亮日志
GREEN='\033[0;32m'
NC='\033[0m'
RED='\033[31m'

# ==================== 通用日志与检查函数 ====================

# 打印绿色高亮日志
function log() {
  local log_content=$1
  echo -e "${GREEN}${log_content}${NC}"
}

# 检查宿主机 CPU 架构。
# Kuscia 当前支持 x86_64、amd64 与 arm64（arm64 可能有限制，仅做警告）。
function arch_check() {
  local arch
  arch=$(uname -a)
  if [[ $arch == *"ARM"* ]] || [[ $arch == *"aarch64"* ]]; then
    echo "Warning: arm64 architecture. Continuing..."
  elif [[ $arch == *"x86_64"* ]]; then
    echo -e "${GREEN}x86_64 architecture. Continuing...${NC}"
  elif [[ $arch == *"amd64"* ]]; then
    echo "Warning: amd64 architecture. Continuing..."
  else
    echo -e "${RED}$arch architecture is not supported by kuscia currently${NC}"
    exit 1
  fi
}

# 检查当前用户是否有权限在指定路径创建目录。
# 用于提前发现数据目录、日志目录不可写的问题。
function pre_check() {
  if ! mkdir -p "$1" 2>/dev/null; then
    echo -e "${RED}User does not have access to create the directory: $1${NC}"
    exit 1
  fi
}

# ==================== 全局默认配置 ====================

# 若未通过环境变量指定 Kuscia 镜像，则使用默认镜像
if [[ ${KUSCIA_IMAGE} == "" ]]; then
  KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:latest
fi
log "KUSCIA_IMAGE=${KUSCIA_IMAGE}"

# 若未通过环境变量指定 SecretFlow 引擎镜像，则使用默认镜像
if [[ "$SECRETFLOW_IMAGE" == "" ]]; then
  SECRETFLOW_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite-anolis8:1.11.0b1
fi
log "SECRETFLOW_IMAGE=${SECRETFLOW_IMAGE}"

# SecretFlow 镜像相关字段
SF_IMAGE_REGISTRY=""
SF_IMAGE_ID=""

# Kuscia 容器内部根目录与证书路径
CTR_ROOT=/home/kuscia
CTR_CERT_ROOT=${CTR_ROOT}/var/certs

# 各节点角色的内存限制
MASTER_MEMORY_LIMIT=2G
LITE_MEMORY_LIMIT=4G
AUTONOMY_MEMORY_LIMIT=6G

# Docker 网络名称，所有 Kuscia 容器加入同一网络以互相通信
NETWORK_NAME="kuscia-exchange"

# ==================== SecretFlow 镜像信息解析 ====================

# 从 SECRETFLOW_IMAGE 中解析出镜像标签、仓库、注册中心等信息，
# 供后续导入镜像与生成 AppImage 时使用。
#
# 支持的镜像地址格式：
#   - 无注册中心命名空间：repo/name:tag（1 个 /）
#   - 带注册中心命名空间：registry/namespace/name:tag（2 个 /）
function init_sf_image_info() {
  if [ "$SECRETFLOW_IMAGE" != "" ]; then
    # 提取镜像标签（冒号后的部分）
    SF_IMAGE_TAG=${SECRETFLOW_IMAGE##*:}

    # 统计镜像地址中的 "/" 数量，用于判断格式
    path_separator_count="$(echo "$SECRETFLOW_IMAGE" | tr -cd "/" | wc -c)"

    if [ "${path_separator_count}" == 1 ]; then
      # 格式为 repo/name:tag，直接去掉标签得到仓库名
      SF_IMAGE_NAME=${SECRETFLOW_IMAGE//:${SF_IMAGE_TAG}/}
    elif [ "$path_separator_count" == 2 ]; then
      # 格式为 registry/namespace/name:tag，分别提取注册中心与镜像名
      registry=$(echo "$SECRETFLOW_IMAGE" | cut -d "/" -f 1)
      bucket=$(echo "$SECRETFLOW_IMAGE" | cut -d "/" -f 2)
      name_and_tag=$(echo "$SECRETFLOW_IMAGE" | cut -d "/" -f 3)
      name=${name_and_tag//:${SF_IMAGE_TAG}/}
      SF_IMAGE_REGISTRY="$registry/$bucket"
      SF_IMAGE_NAME="$name"
    fi
  fi
}

# 执行 SecretFlow 镜像信息初始化
init_sf_image_info

# ==================== 容器生命周期辅助函数 ====================

# 判断是否需要启动（或重建）指定名称的 Docker 容器。
# 若容器不存在，返回 0（需要启动）；
# 若容器已存在，则询问用户是否重新创建，根据回答返回 0 或 1。
function need_start_docker_container() {
  local ctr=$1

  if [[ ! "$(docker ps -a -q -f name=^/"${ctr}"$)" ]]; then
    # 容器不存在，需要启动
    return 0
  fi

  read -rp "$(log "The container '${ctr}' already exists. Do you need to recreate it? [y/n]:")" yn
  case $yn in
  [Yy]*)
    log "Remove container ${ctr} ..."
    docker rm -f "$ctr"
    # 已删除旧容器，需要启动新容器
    return 0
    ;;
  *)
    # 保留旧容器，跳过启动
    return 1
    ;;
  esac
}

# ==================== 探针检测函数 ====================

# 通用 HTTP 探针：在指定容器内通过 curl 访问 endpoint，最多重试 max_retry 次。
# enable_mtls=true 时，使用容器内的 CA 证书与客户端证书进行 mTLS 访问。
# 返回状态码为 200/404/401 时认为探测成功。
function do_http_probe() {
  local ctr=$1
  local endpoint=$2
  local max_retry=$3
  local enable_mtls=$4
  local cert_config
  if [[ "$enable_mtls" == "true" ]]; then
    cert_config="--cacert ${CTR_CERT_ROOT}/ca.crt --cert ${CTR_CERT_ROOT}/ca.crt --key ${CTR_CERT_ROOT}/ca.key"
  fi

  local retry=0
  while [[ "$retry" -lt "$max_retry" ]]; do
    local status_code
    status_code=$(docker exec -it "$ctr" curl -k --write-out '%{http_code}' --silent --output /dev/null "${endpoint}" "${cert_config}")
    if [[ $status_code -eq 200 || $status_code -eq 404 || $status_code -eq 401 ]]; then
      return 0
    fi
    sleep 1
    retry=$((retry + 1))
  done

  return 1
}

# 探测容器内 k3s API Server 是否就绪（https://127.0.0.1:6443）。
# 超时 60 秒，失败则退出并提示查看 k3s 日志。
function probe_k3s() {
  local domain_ctr=$1

  if ! do_http_probe "$domain_ctr" "https://127.0.0.1:6443" 60; then
    echo "[Error] Probe k3s in container '$domain_ctr' failed. Please check k3s log in container, path: /home/kuscia/var/logs/k3s.log" >&2
    exit 1
  fi
}

# 探测指定 Namespace 下的 Gateway CRD 是否已经创建。
# 先确保 k3s 就绪，再轮询 Gateway 资源，最多重试 max_retry 秒。
function probe_gateway_crd() {
  local master=$1
  local domain=$2
  local gw_name=$3
  local max_retry=$4
  probe_k3s "$master"

  local retry=0
  while [ "$retry" -lt "$max_retry" ]; do
    local line_num
    line_num=$(docker exec -it "$master" kubectl get gateways -n "$domain" | grep -c -i "$gw_name" | xargs)
    if [[ "$line_num" == "1" ]]; then
      return
    fi
    sleep 1
    retry=$((retry + 1))
  done
  echo "[Error] Probe gateway in namespace '$domain' failed. Please check envoy log in container, path: /home/kuscia/var/logs/envoy" >&2
  exit 1
}

# ==================== 网络与存储 ====================

# 创建 Docker 网络 kuscia-exchange，若不存在则创建。
# 所有 Kuscia 容器通过该网络互相发现与通信。
function build_kuscia_network() {
  if [[ ! "$(docker network ls -q -f name=${NETWORK_NAME})" ]]; then
    docker network create "${NETWORK_NAME}"
  fi
}

# ==================== 引擎镜像管理 ====================

# 检查并导入 SecretFlow/PSI 引擎镜像到 Kuscia 容器内。
# 逻辑如下：
#   1. 优先从容器内的 crictl 检查镜像是否已存在；
#   2. 若不存在，检查宿主机 Docker 是否已有该镜像；
#   3. 若宿主机没有，根据 env.list 中的 REGISTRY_ENDPOINT 进行 docker login 并拉取；
#   4. 将镜像保存为 tar 包，再通过 containerd 导入到 Kuscia 容器内。
function check_sf_image() {
  local domain_ctr=$1
  local env_file=${ROOT}/env.list
  local default_repo=${SF_IMAGE_REGISTRY}
  local repo
  if [ -e "$env_file" ]; then
    repo=$(awk -F "=" '/REGISTRY_ENDPOINT/ {print $2}' "$env_file")
  fi
  local sf_image="${SF_IMAGE_NAME}:${SF_IMAGE_TAG}"
  if [ "$repo" != "" ]; then
    sf_image="${repo}/${SF_IMAGE_NAME##*/}:${SF_IMAGE_TAG}"
  elif [ "$default_repo" != "" ]; then
    sf_image="${default_repo}/${SF_IMAGE_NAME##*/}:${SF_IMAGE_TAG}"
  fi
  if [ "$SECRETFLOW_IMAGE" != "" ]; then
    sf_image=$SECRETFLOW_IMAGE
  fi

  # 若容器内已存在该镜像，直接获取镜像 ID 并返回
  if docker exec -it "$domain_ctr" crictl inspecti "$sf_image" >/dev/null 2>&1; then
    log "Image '${sf_image}' already exists in domain '${DOMAIN_ID}'"
    SF_IMAGE_ID=$(docker exec -it "$domain_ctr" sh -c "crictl inspecti ${sf_image} | jq -r .status.id | tr -d ' \t\n\r'")
    return
  fi

  local has_sf_image=false
  if docker image inspect "${sf_image}" >/dev/null 2>&1; then
    has_sf_image=true
  fi

  if [ "$has_sf_image" == true ]; then
    log "Found the secretflow image '${sf_image}' on host"
  else
    log "Not found the secretflow image '${sf_image}' on host"
    if [ "$repo" != "" ]; then
      docker login "$repo"
    fi
    log "Start pulling image '${sf_image}' ..."
    docker pull "${sf_image}"
  fi

  log "Start importing image '${sf_image}' Please be patient..."
  local image_id
  image_id=$(docker images --filter="reference=${sf_image}" --format "{{.ID}}")
  local image_tar
  image_tar=/tmp/$(echo "${sf_image}" | sed 's/\//_/g').${image_id}.tar
  if [ ! -e "$image_tar" ]; then
    docker save "$sf_image" -o "$image_tar"
  fi
  docker exec -it "$domain_ctr" ctr -a=${CTR_ROOT}/containerd/run/containerd.sock -n=k8s.io images import "$image_tar"
  log "Successfully imported image '${sf_image}' to container '${domain_ctr}' ..."

  SF_IMAGE_ID=$(docker exec -it "$domain_ctr" sh -c "crictl inspecti ${sf_image} | jq -r .status.id | tr -d ' \t\n\r'")
}

# 在 Kuscia 容器内调用 create_sf_app_image.sh，根据 SECRETFLOW_IMAGE 注册 AppImage。
# 自动识别应用类型：镜像名以 psi- 开头则注册为 psi，否则注册为 secretflow。
function create_secretflow_app_image() {
  local ctr=$1
  local image_repo=$SECRETFLOW_IMAGE
  local image_tag=latest

  if [[ "${SECRETFLOW_IMAGE}" == *":"* ]]; then
    image_repo=${SECRETFLOW_IMAGE%%:*}
    image_tag=${SECRETFLOW_IMAGE##*:}
  fi

  app_type=$(echo "${image_repo}" | awk -F'/' '{print $NF}' | awk -F'-' '{print $1}')
  if [[ ${app_type} != "psi" ]]; then
    app_type="secretflow"
  fi

  docker exec -it "${ctr}" scripts/deploy/create_sf_app_image.sh "${image_repo}" "${image_tag}" "${app_type}" "${SF_IMAGE_ID}"
  log "Create secretflow app image done"
}

# ==================== 组件与数据初始化 ====================

# 探测 DataMesh 健康接口（https://127.0.0.1:8070/healthZ）。
# DataMesh 是 Kuscia 数据平面的核心组件，负责 DomainData 的元数据与访问控制。
function probe_datamesh() {
  local domain_ctr=$1
  if ! do_http_probe "$domain_ctr" "https://127.0.0.1:8070/healthZ" 30 true; then
    echo "[Error] Probe datamesh in container '$domain_ctr' failed." >&2
    echo "You cloud run command that 'docker logs $domain_ctr' to check the log" >&2
  fi
  log "Probe datamesh successfully"
}

# 创建示例 DomainData 数据表：Alice 与 Bob 的测试数据。
# 用于部署后的快速验证与演示。
function create_domaindata_table() {
  local ctr=$1

  # create domain data table
  docker exec -it "${ctr}" scripts/deploy/create_domaindata_alice_table.sh "${DOMAIN_ID}"
  docker exec -it "${ctr}" scripts/deploy/create_domaindata_bob_table.sh "${DOMAIN_ID}"
  log "Create domain data table done"
}

# ==================== Docker 启动参数生成 ====================

# 生成容器启动时的环境变量参数。
# 若存在 env.list 文件，则整体挂载；否则仅传入 REGISTRY_ENDPOINT。
function generate_env_flag() {
  local env_flag
  local env_file=${ROOT}/env.list
  if [ -e "$env_file" ]; then
    env_flag="--env-file $env_file"
  else
    env_flag="--env REGISTRY_ENDPOINT=${SF_IMAGE_REGISTRY}"
  fi
  echo "$env_flag"
}

# 生成容器启动时的挂载参数。
# 挂载 /tmp、数据目录、日志目录到容器内的对应位置。
function generate_mount_flag() {
  local mount_flag="-v /tmp:/tmp -v ${DOMAIN_DATA_DIR}:/home/kuscia/var/storage/data -v ${DOMAIN_LOG_DIR}:/home/kuscia/var/stdout"
  echo "$mount_flag"
}

# 从 kuscia.yaml 中读取 runtime 配置，未配置时默认使用 runc。
# runc 模式下需要 --privileged 参数，runc 也支持 SecretFlow 引擎镜像导入。
function get_runtime() {
  local conf_file=$1
  local runtime
  runtime=$(grep '^runtime:' "${conf_file}" 2>/dev/null | cut -d':' -f2 | awk '{$1=$1};1' | tr -d '\r\n')
  if [[ $runtime == "" ]]; then
    runtime=runc
  fi
  echo "$runtime"
}

# 创建 Docker Volume，若不存在则创建。
# 用于持久化 containerd 数据，避免容器重建后镜像丢失。
function createVolume() {
  local VOLUME_NAME=$1
  if ! docker volume ls --format '{{.Name}}' | grep "^${VOLUME_NAME}$"; then
    docker volume create "$VOLUME_NAME"
  fi
}

# ==================== 部署函数：Autonomy / Lite / Master ====================

# 部署 Autonomy 节点。
# Autonomy 是独立的 Kuscia 节点，包含完整的控制面与数据面，适合 P2P/CxC 组网。
function deploy_autonomy() {
  local domain_ctr=${USER}-kuscia-autonomy-${DOMAIN_ID}
  local conf_dir=${ROOT}/${domain_ctr}
  local kuscia_config_file=${conf_dir}/kuscia.yaml
  if [[ ${KUSCIA_CONFIG_FILE} != "" ]]; then
    kuscia_config_file=${KUSCIA_CONFIG_FILE}
  fi
  local runtime
  runtime=$(get_runtime "${kuscia_config_file}")
  arch_check
  if need_start_docker_container "$domain_ctr"; then
    log "Starting container $domain_ctr ..."
    env_flag=$(generate_env_flag)
    mount_flag=$(generate_mount_flag)

    # 若未提供外部配置文件，则通过 kuscia init 生成默认配置
    if [[ ${KUSCIA_CONFIG_FILE} == "" ]]; then
      mkdir -p "${conf_dir}"
      docker run -it --rm "${KUSCIA_IMAGE}" kuscia init --mode Autonomy --domain "${DOMAIN_ID}" >"${kuscia_config_file}" 2>&1 || cat "${kuscia_config_file}"
      wrap_kuscia_config_file "${kuscia_config_file}"
    fi

    # runc 运行时默认需要特权模式，以支持容器内启动子容器
    local privileged_flag
    if [[ ${runtime} == "runc" ]]; then
      privileged_flag=" --privileged"
    fi

    createVolume "${domain_ctr}-containerd"

    docker run -dit "${privileged_flag}" --name="${domain_ctr}" --hostname="${domain_ctr}" --restart=always --network=${NETWORK_NAME} -m ${AUTONOMY_MEMORY_LIMIT} \
      -p "${DOMAIN_HOST_PORT}":1080 \
      -p "${DOMAIN_HOST_INTERNAL_PORT}":80 \
      -p "${KUSCIAAPI_HTTP_PORT}":8082 \
      -p "${KUSCIAAPI_GRPC_PORT}":8083 \
      -v "${domain_ctr}-containerd":${CTR_ROOT}/containerd \
      -v "${kuscia_config_file}":/home/kuscia/etc/conf/kuscia.yaml \
      "${env_flag}" "${mount_flag}" \
      --env NAMESPACE="${DOMAIN_ID}" \
      "${KUSCIA_IMAGE}" bin/kuscia start -c etc/conf/kuscia.yaml

    probe_datamesh "${domain_ctr}"
    docker exec -it "${domain_ctr}" sh scripts/deploy/init_kusciaapi_client_certs.sh

    log "Container ${domain_ctr} started successfully"
  fi

  # runc 模式下才需要导入 SecretFlow 引擎镜像
  if [[ ${runtime} == "runc" ]]; then
    check_sf_image "${domain_ctr}"
  fi

  create_secretflow_app_image "${domain_ctr}"

  # create demo data
  create_domaindata_table "${domain_ctr}"

  log "Autonomy domain '${DOMAIN_ID}' deployed successfully"
}

# 部署 Lite 节点。
# Lite 节点必须指定 MASTER_ENDPOINT 与 DOMAIN_TOKEN，注册到 Master 后作为工作节点运行。
function deploy_lite() {
  local domain_ctr=${USER}-kuscia-lite-${DOMAIN_ID}
  local conf_dir=${ROOT}/${domain_ctr}
  local kuscia_config_file=${conf_dir}/kuscia.yaml
  if [[ ${KUSCIA_CONFIG_FILE} != "" ]]; then
    kuscia_config_file=${KUSCIA_CONFIG_FILE}
  fi
  local runtime
  runtime=$(get_runtime "${kuscia_config_file}")

  # 在启动容器前，先通过临时容器访问 Master Endpoint 验证网络连通性。
  # 期望返回 401，表示 HTTPS 端口可达且鉴权生效。
  local HttpResponseCode
  HttpResponseCode=$(docker run -it --rm --network=${NETWORK_NAME} "${KUSCIA_IMAGE}" curl -k -s -o /dev/null -w "%{http_code}" "${MASTER_ENDPOINT}")
  arch_check

  if need_start_docker_container "$domain_ctr"; then
    log "Starting container $domain_ctr ..."

    env_flag=$(generate_env_flag)
    mount_flag=$(generate_mount_flag)

    if [[ $HttpResponseCode = "401" ]]; then
      echo -e "${GREEN}Communication with master is normal, response code is 401${NC}"
    else
      echo -e "${RED}Failed to connect to the master. Please check if the network link to the master is normal. Please refer to the kuscia documentation (https://www.secretflow.org.cn/docs/kuscia/latest/zh-Hans/deployment/deploy_master_lite_cn) for the correct return results${NC}"
      docker run -it --rm --network=${NETWORK_NAME} "${KUSCIA_IMAGE}" curl -kvvv "${MASTER_ENDPOINT}"
      exit 1
    fi

    # 若未提供外部配置文件，则通过 kuscia init 生成 Lite 配置，需传入 Master 端点与部署 Token
    if [[ ${KUSCIA_CONFIG_FILE} == "" ]]; then
      mkdir -p "${conf_dir}"
      docker run -it --rm "${KUSCIA_IMAGE}" kuscia init --mode Lite --domain "${DOMAIN_ID}" --master-endpoint "${MASTER_ENDPOINT}" --lite-deploy-token "${DOMAIN_TOKEN}" >"${kuscia_config_file}" 2>&1 || cat "${kuscia_config_file}"
      wrap_kuscia_config_file "${kuscia_config_file}"
    fi

    local privileged_flag
    if [[ ${runtime} == "runc" ]]; then
      privileged_flag=" --privileged"
    fi

    createVolume "${domain_ctr}-containerd"

    docker run -dit "${privileged_flag}" --name="${domain_ctr}" --hostname="${domain_ctr}" --restart=always --network=${NETWORK_NAME} -m $LITE_MEMORY_LIMIT \
      -p "${DOMAIN_HOST_PORT}":1080 \
      -p "${DOMAIN_HOST_INTERNAL_PORT}":80 \
      -p "${KUSCIAAPI_HTTP_PORT}":8082 \
      -p "${KUSCIAAPI_GRPC_PORT}":8083 \
      -v "${domain_ctr}-containerd":${CTR_ROOT}/containerd \
      -v "${kuscia_config_file}":/home/kuscia/etc/conf/kuscia.yaml \
      "${env_flag}" "${mount_flag}" \
      --env NAMESPACE="${DOMAIN_ID}" \
      "${KUSCIA_IMAGE}" bin/kuscia start -c etc/conf/kuscia.yaml

    probe_datamesh "$domain_ctr"
    docker exec -it "${domain_ctr}" sh scripts/deploy/init_kusciaapi_client_certs.sh

    log "Lite domain '${DOMAIN_ID}' started successfully"
  fi

  if [[ ${runtime} == "runc" ]]; then
    check_sf_image "${domain_ctr}"
  fi

  log "Lite domain '${DOMAIN_ID}' deployed successfully"
}

# 部署 Master 节点。
# Master 是中心化组网的中控节点，负责 Domain 管理、任务调度、证书与路由管理。
function deploy_master() {
  local domain_ctr=${USER}-kuscia-master
  local master_domain_id=${DOMAIN_ID}
  local conf_dir=${ROOT}/${domain_ctr}
  local kuscia_config_file=${conf_dir}/kuscia.yaml
  if [[ ${KUSCIA_CONFIG_FILE} != "" ]]; then
    kuscia_config_file=${KUSCIA_CONFIG_FILE}
  fi
  arch_check

  if need_start_docker_container "${domain_ctr}"; then
    log "Starting container ${domain_ctr} ..."

    env_flag=$(generate_env_flag)
    mount_flag=$(generate_mount_flag)

    # 若未提供外部配置文件，则通过 kuscia init 生成 Master 配置
    if [[ ${KUSCIA_CONFIG_FILE} == "" ]]; then
      mkdir -p "${conf_dir}"
      docker run -it --rm "${KUSCIA_IMAGE}" kuscia init --mode Master --domain "$master_domain_id" >"${kuscia_config_file}" 2>&1 || cat "${kuscia_config_file}"
    fi

    docker run -dit --name="${domain_ctr}" --hostname="${domain_ctr}" --restart=always --network=${NETWORK_NAME} -m ${MASTER_MEMORY_LIMIT} \
      --env NAMESPACE="${master_domain_id}" \
      -p "${DOMAIN_HOST_PORT}":1080 \
      -p "${KUSCIAAPI_HTTP_PORT}":8082 \
      -p "${KUSCIAAPI_GRPC_PORT}":8083 \
      -v "${kuscia_config_file}":/home/kuscia/etc/conf/kuscia.yaml \
      "${env_flag}" "${mount_flag}" \
      "${KUSCIA_IMAGE}" bin/kuscia start -c etc/conf/kuscia.yaml

    probe_gateway_crd "${domain_ctr}" "${master_domain_id}" "${domain_ctr}" 60
    docker exec -it "${domain_ctr}" sh scripts/deploy/init_kusciaapi_client_certs.sh
    log "Master '${master_domain_id}' started successfully"
  fi
  create_secretflow_app_image "${domain_ctr}"
  log "Master deployed successfully"
}

# ==================== 配置与路径辅助函数 ====================

# 根据 ALLOW_PRIVILEGED 环境变量，在 kuscia.yaml 末尾追加允许特权容器的配置。
# 主要用于 Autonomy/Lite 运行需要 privileged 权限的引擎任务。
function wrap_kuscia_config_file() {
  local kuscia_config_file=$1
  local p2p_protocol=$2

  PRIVILEGED_CONFIG="
agent:
  allowPrivileged: true
  "
  if [[ $ALLOW_PRIVILEGED == "true" ]]; then
    echo -e "$PRIVILEGED_CONFIG" >>"$kuscia_config_file"
  fi
}

# 获取文件或目录的绝对路径。
function get_absolute_path() {
  echo "$(
    cd "$(dirname -- "$1")" >/dev/null
    pwd -P
  )/$(basename -- "$1")"
}

# ==================== 命令行参数解析 ====================

# 打印使用说明
usage() {
  echo "$(basename "$0") DEPLOY_MODE [OPTIONS]
DEPLOY_MODE:
    autonomy        Deploy a autonomy domain.
    lite            Deploy a lite domain.
    master          Deploy a master.

OPTIONS:
    -c              The host path of kuscia configure file.  It will be mounted into the domain container.
    -d              The data directory used to store domain data. It will be mounted into the domain container.
                    You can set Env 'DOMAIN_DATA_DIR' instead.  Default is '{{ROOT}}/{{DOMAIN_CONTAINER_NAME}}/data'.
    -h              Show this help text.
    -l              The data directory used to store domain logs. It will be mounted into the domain container.
                    You can set Env 'DOMAIN_LOG_DIR' instead.  Default is '{{ROOT}}/{{DOMAIN_CONTAINER_NAME}}/logs'.
    -m              (Only used in lite mode) The master endpoint. You can set Env 'MASTER_ENDPOINT' instead.
    -n              Domain id to be deployed. You can set Env 'DOMAIN_ID' instead.
    -p              The port exposed by domain. You can set Env 'DOMAIN_HOST_PORT' instead.
    -q              (Only used in autonomy or lite mode)The port exposed for internal use by domain. You can set Env 'DOMAIN_HOST_INTERNAL_PORT' instead.
    -r              The install directory. You can set Env 'ROOT' instead. Default is $(pwd).
    -t              (Only used in lite mode) The deploy token. You can set Env 'DOMAIN_TOKEN' instead.
    -k              (Only used in autonomy or master mode)The http port exposed by KusciaAPI , default is 13082. You can set Env 'KUSCIAAPI_HTTP_PORT' instead.
    -g              (Only used in autonomy or master mode)The grpc port exposed by KusciaAPI, default is 13083. You can set Env 'KUSCIAAPI_GRPC_PORT' instead.
    -e              (Only used in autonomy or master mode)The extra subjectAltName for KusciaAPI server cert.
    "
}

# 解析第一个位置参数：部署模式
deploy_mode=
case "$1" in
autonomy | lite | master)
  deploy_mode=$1
  shift
  ;;
-h)
  usage
  exit
  ;;
*)
  echo "deploy_mode is invalid, must be autonomy, lite or master"
  usage
  exit 1
  ;;
esac

# 解析可选参数
while getopts 'c:l:n:p:q:m:t:r:d:k:g:e:h' option; do
  case "$option" in
  c)
    KUSCIA_CONFIG_FILE=$(get_absolute_path "$OPTARG")
    ;;
  l)
    DOMAIN_LOG_DIR=$OPTARG
    ;;
  n)
    DOMAIN_ID=$OPTARG
    ;;
  p)
    DOMAIN_HOST_PORT=$OPTARG
    ;;
  q)
    DOMAIN_HOST_INTERNAL_PORT=$OPTARG
    ;;
  m)
    MASTER_ENDPOINT=$OPTARG
    ;;
  t)
    DOMAIN_TOKEN=$OPTARG
    ;;
  r)
    ROOT=$OPTARG
    ;;
  d)
    DOMAIN_DATA_DIR=$OPTARG
    ;;
  k)
    KUSCIAAPI_HTTP_PORT=$OPTARG
    ;;
  g)
    KUSCIAAPI_GRPC_PORT=$OPTARG
    ;;
  e)
    KUSCIAAPI_EXTRA_SUBJECT_ALTNAME=$OPTARG
    ;;
  h)
    usage
    exit
    ;;
  :)
    printf "missing argument for -%s\n" "$OPTARG" >&2
    exit 1
    ;;
  \?)
    printf "illegal option: -%s\n" "$OPTARG" >&2
    exit 1
    ;;
  esac
done
shift $((OPTIND - 1))

# ==================== 默认值与必填校验 ====================

# 外部暴露的跨域网关端口（1080）必须显式指定
if [[ ${DOMAIN_HOST_PORT} == "" ]]; then
  printf "empty domain host port\n" >&2
  exit 1
fi

# 内部应用端口（80）默认值
if [[ ${DOMAIN_HOST_INTERNAL_PORT} == "" ]]; then
  DOMAIN_HOST_INTERNAL_PORT=13081
fi

# KusciaAPI HTTP 端口默认值
if [[ ${KUSCIAAPI_HTTP_PORT} == "" ]]; then
  KUSCIAAPI_HTTP_PORT=13082
fi

# KusciaAPI gRPC 端口默认值
if [[ ${KUSCIAAPI_GRPC_PORT} == "" ]]; then
  KUSCIAAPI_GRPC_PORT=13083
fi

# 初始化数据目录、日志目录等配置，并创建 Docker 网络
function init() {
  local deploy_mode=$1
  local domain_ctr=${USER}-kuscia-${deploy_mode}-${DOMAIN_ID}
  if [[ ${deploy_mode} == "master" ]]; then
    local domain_ctr=${USER}-kuscia-master
  fi
  [[ ${ROOT} == "" ]] && ROOT=${PWD}
  [[ ${DOMAIN_DATA_DIR} == "" ]] && DOMAIN_DATA_DIR="${PWD}/${domain_ctr}/data"
  [[ ${DOMAIN_LOG_DIR} == "" ]] && DOMAIN_LOG_DIR="${ROOT}/${domain_ctr}/logs"

  pre_check "${DOMAIN_DATA_DIR}"
  pre_check "${DOMAIN_LOG_DIR}"

  log "ROOT=${ROOT}"
  log "DOMAIN_ID=${DOMAIN_ID}"
  log "DOMAIN_HOST_PORT=${DOMAIN_HOST_PORT}"
  log "DOMAIN_HOST_INTERNAL_PORT=${DOMAIN_HOST_INTERNAL_PORT}"
  log "DOMAIN_DATA_DIR=${DOMAIN_DATA_DIR}"
  log "DOMAIN_LOG_DIR=${DOMAIN_LOG_DIR}"
  log "KUSCIA_IMAGE=${KUSCIA_IMAGE}"
  log "KUSCIAAPI_HTTP_PORT=${KUSCIAAPI_HTTP_PORT}"
  log "KUSCIAAPI_GRPC_PORT=${KUSCIAAPI_GRPC_PORT}"

  build_kuscia_network
}

# ==================== 根据部署模式执行安装 ====================

case ${deploy_mode} in
autonomy)
  if [[ ${DOMAIN_ID} == "" ]]; then
    printf "empty domain id\n" >&2
    exit 1
  fi
  init "${deploy_mode}"
  deploy_autonomy
  ;;
lite)
  if [[ ${DOMAIN_ID} == "" ]]; then
    printf "empty domain id\n" >&2
    exit 1
  fi
  if [[ ${DOMAIN_TOKEN} == "" ]]; then
    printf "Empty domain token\n" >&2
    exit 1
  fi
  if [[ ${MASTER_ENDPOINT} == "" ]]; then
    printf "Empty master endpoint\n" >&2
    exit 1
  fi

  init "${deploy_mode}"
  deploy_lite
  ;;
master)
  if [[ ${DOMAIN_ID} == "" ]]; then
    DOMAIN_ID=kuscia-system
  fi
  init "${deploy_mode}"
  deploy_master
  ;;
*)
  printf "unsupported network mode: %s\n" "$deploy_mode" >&2
  exit 1
  ;;
esac
