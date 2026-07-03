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
# 该脚本用于向运行中的 Kuscia Docker 容器注册应用镜像（AppImage）。
#
# 与 create_sf_app_image.sh 的区别：
#   - create_sf_app_image.sh 在宿主机上直接执行 kubectl apply；
#   - 本脚本面向已启动的 Kuscia Docker 容器，通过 docker exec 操作容器内部环境，
#     并支持将宿主机上的引擎镜像导入到容器内。
#
# 主要能力：
#   1. 使用自定义 AppImage 模板文件注册应用（-f 模式）；
#   2. 使用项目内置模板自动注册 secretflow / psi / dataproxy / kuscia 等应用（-m 模式）；
#   3. 可选地通过 --import 将引擎镜像从宿主机导入到 Kuscia 容器内。
#
# 使用场景：
#   - 本地或生产环境中，向已部署的 Kuscia 节点添加新的引擎镜像；
#   - CI/CD 流水线中批量注册自定义应用镜像；
#   - 当 Kuscia 容器无法直接访问外部镜像仓库时，使用 --import 离线导入。
#
# 前置条件：
#   - Docker 已安装，当前用户可执行 docker 命令；
#   - 目标 Kuscia 容器已经启动；
#   - 使用 -m 模式时，宿主机 kubectl 已配置并可访问对应 Kuscia 集群。

# 严格模式：遇到错误立即退出，防止错误继续传播
set -e

# 终端颜色定义，用于输出高亮日志
GREEN='\033[0;32m'
NC='\033[0m'
RED='\033[31m'

# 项目根目录，用于定位 scripts/templates/ 下的 AppImage 模板
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)

# 使用说明
usage="$(basename "$0") [OPTIONS]

OPTIONS:
    -h    [optional] show this help text
    -c    [mandatory] kuscia container name
    -i    [mandatory] app docker image info
    -f    [optional] kuscia appImage template file full path. the recommended template file naming rule is {appImage name}.yaml under the same directory as tool script. otherwise the file full path must be specified
example:
 ./register_app_image.sh -c root-kuscia-autonomy-alice -f ./secretflow-image.yaml -i secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite-anolis8:latest --import
"

# ==================== 预处理特殊选项 --import ====================

# getopts 不直接支持长选项，因此先遍历所有参数，
# 若发现 --import，则设置 IMPORT=true，并将其从参数列表中移除。
NEW_ARGS=()
IMPORT=false
for arg in "$@"; do
  case $arg in
    --import)
      IMPORT=true
      ;;
    *)
       NEW_ARGS+=("$arg")
      ;;
  esac
done
set -- "${NEW_ARGS[@]}"

# ==================== 解析命令行参数 ====================

# getopts 选项说明：
#   h         显示帮助信息
#   c:        Kuscia 容器名称（必填）
#   i:        应用 Docker 镜像地址（必填）
#   m         使用默认模板注册（deploy 模式），无需参数
#   f:        自定义 AppImage 模板文件路径（可选）
#   n:        AppImage 名称，默认 secretflow-image
while getopts 'hc:i:mf:n:' option; do
  case "$option" in
  h)
    echo "$usage"
    exit 0
    ;;
  c)
    KUSCIA_CONTAINER_NAME=$OPTARG
    ;;
  i)
    IMAGE=$OPTARG
    ;;
  m)
    DEPLOY=true
    ;;
  f)
    APP_IMAGE_FILE=$OPTARG
    ;;
  n)
    SF_NAME=$OPTARG
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

# ==================== 默认值与镜像地址规范化 ====================

# 若未指定 AppImage 名称，默认使用 secretflow-image
if [[ -z "${SF_NAME}" ]]; then
  SF_NAME="secretflow-image"
fi

# 规范化镜像地址：
#   - 若镜像名中只有一个 "/"，视为 docker.io 下的用户仓库，补充前缀 docker.io/；
#   - 若镜像名中没有 "/"，视为 docker.io/library 下的官方镜像，补充前缀 docker.io/library/；
# 这样可确保后续 docker pull / docker save / kuscia image 操作使用完整镜像地址。
count=$(echo "${IMAGE}" | grep -o "/" | wc -l)
if [[ ${count} -eq 1 ]]; then
  IMAGE="docker.io/${IMAGE}"
elif [[ ${count} -eq 0 ]]; then
  IMAGE="docker.io/library/${IMAGE}"
fi

# ==================== 镜像导入函数 ====================

# 将指定的引擎镜像从宿主机导入到 Kuscia 容器内部。
# 导入流程：
#   1. 检查镜像是否已存在于 Kuscia 容器内（通过 kuscia image list）；
#   2. 若不存在，检查宿主机 Docker 是否已有该镜像，无则拉取；
#   3. 使用 docker save 将镜像保存为 tar 包（存放在 Kuscia 容器 /home/kuscia/var/images 的宿主机挂载目录）；
#   4. 通过 kuscia image load 将 tar 包导入容器；
#   5. 校验导入结果，并清理临时 tar 包。
function import_engine_image() {
  if docker exec -i "${KUSCIA_CONTAINER_NAME}" bash -c "kuscia image list 2>&1 | awk '{print \$1\":\"\$2}' | grep -q \"^${IMAGE}$\""; then
     echo -e "${GREEN}Image '${IMAGE}' already exists in container ${KUSCIA_CONTAINER_NAME}${NC}"
  else
     if docker image inspect "${IMAGE}" >/dev/null 2>&1; then
        echo -e "${GREEN}Found the engine image '${IMAGE}' on host${NC}"
     else
        echo -e "${GREEN}Not found the engine image '${IMAGE}' on host${NC}"
        echo -e "${GREEN}Start pulling image '${IMAGE}' ...${NC}"
        docker pull "${IMAGE}"
     fi
     # 生成随机文件名，避免并发导入时冲突
     local image_random
     image_random="image_$(head /dev/urandom | base64 | tr -dc A-Za-z0-9 | head -c 8)"
     echo -e "${GREEN}Start importing image '${IMAGE}' Please be patient...${NC}"

     local image_tar=${DOMAIN_IMAGE_WORK_DIR}/${image_random}.tar
     docker save "${IMAGE}" -o "${image_tar}"
     docker exec -it "${KUSCIA_CONTAINER_NAME}" kuscia image load -i /home/kuscia/var/images/"${image_random}".tar
     if docker exec -i "${KUSCIA_CONTAINER_NAME}" bash -c "kuscia image list 2>&1 | awk '{print \$1\":\"\$2}' | grep -q \"^${IMAGE}$\""; then
        echo -e "${GREEN}image ${IMAGE} import successfully${NC}"
     else
        echo -e "${RED}error: ${IMAGE} import failed${NC}"
     fi
     rm -rf "${image_tar}"
  fi
}

# ==================== 自定义模板注册函数 ====================

# 根据用户提供的自定义 AppImage 模板文件，替换镜像名、标签与 AppImage 名称后，
# 在 Kuscia 容器内创建 AppImage 资源。
#
# 处理逻辑：
#   1. 从 IMAGE 参数解析镜像仓库与标签；
#   2. 找到模板中 "  image:" 所在行号，保留其上方内容；
#   3. 追加新的 image.name 与 image.tag 字段；
#   4. 替换 {{.SF_NAME}} 占位符；
#   5. 将生成的 YAML 复制到容器内并执行 kubectl apply。
function apply_appimage_crd() {
  local image_repo
  local image_tag
  image_repo=$(echo "${IMAGE}" | awk -F ":" '{print $1}')
  image_tag=$(echo "${IMAGE}" | awk -F ":" '{print $2}')
  if [[ ${image_tag} = "" ]]; then
    image_tag="latest"
  fi
  if [[ ! -f $APP_IMAGE_FILE ]]; then
    echo -e "${RED}${APP_IMAGE_FILE} does not exist, register fail${NC}"
  else
    # 定位模板中 image 字段的起始行，保留其上方内容
    image_line=$(awk '/^  image:/{print NR; exit}' "$APP_IMAGE_FILE")
    head -n "$((image_line - 1))" "$APP_IMAGE_FILE" > engine_appimage.yaml
    # 追加新的 image 字段，使用实际镜像仓库与标签
    echo -e "  image:\n    name: ${image_repo}\n    tag: ${image_tag}" >> engine_appimage.yaml
    # 若模板中包含 {{.SF_NAME}}，替换为指定的 AppImage 名称
    if grep -q "{{.SF_NAME}}" engine_appimage.yaml; then
        sed -i "s/{{.SF_NAME}}/${SF_NAME}/g" engine_appimage.yaml
    fi
    docker cp engine_appimage.yaml "${KUSCIA_CONTAINER_NAME}":/home/kuscia
    docker exec -i "${KUSCIA_CONTAINER_NAME}" kubectl apply -f engine_appimage.yaml && rm -rf engine_appimage.yaml
    rm -rf engine_appimage.yaml
  fi
}

# ==================== 默认模板注册函数 ====================

# 根据镜像名称自动识别应用类型，并使用项目内置模板注册 AppImage。
#
# 应用类型识别规则：
#   - 镜像名最后一段以 psi- 开头 -> psi
#   - 以 dataproxy- 开头 -> dataproxy
#   - 以 kuscia- 开头 -> kuscia
#   - 其他 -> secretflow（默认）
#
# 模板占位符：
#   - secretflow / psi 模板使用 {{.SF_IMAGE_NAME}}、{{.SF_IMAGE_TAG}}、{{.SF_NAME}}
#   - dataproxy / kuscia 模板使用 {{.IMAGE_NAME}}、{{.IMAGE_TAG}}
#
# 注意：本函数通过宿主机 kubectl 应用生成的 YAML，而非在容器内执行。
function register_default_app_image() {
  local image_repo
  local image_tag
  image_repo=$(echo "${IMAGE}" | awk -F ":" '{print $1}')
  image_tag=$(echo "${IMAGE}" | awk -F ":" '{print $2}')
  if [[ ${image_tag} = "" ]]; then
    image_tag="latest"
  fi
  local app_type
  app_type=$(echo "${image_repo}" | awk -F'/' '{print $NF}' | awk -F'-' '{print $1}')
  if [[ ${app_type} != "psi" ]] && [[ ${app_type} != "dataproxy" ]] && [[ ${app_type} != "kuscia" ]]; then
     app_type="secretflow"
  fi
  if [[ ${app_type} == "secretflow" ]] || [[ ${app_type} == "psi" ]]; then
    app_image_template=$(sed "s!{{.SF_IMAGE_NAME}}!'${image_repo}'!g;
    s!{{.SF_IMAGE_TAG}}!'${image_tag}'!g;
    s!{{.SF_NAME}}!'${SF_NAME}'!g" \
    < "${ROOT}/scripts/templates/app_image.${app_type}.yaml")
  else
    app_image_template=$(sed "s!{{.IMAGE_NAME}}!'${image_repo}'!g;
    s!{{.IMAGE_TAG}}!'${image_tag}'!g" \
    < "${ROOT}/scripts/templates/app_image.${app_type}.yaml")
  fi
  echo "${app_image_template}" | kubectl apply -f -
}

# ==================== 主注册流程 ====================

# 向指定的 Kuscia 容器注册应用镜像。
# 执行步骤：
#   1. 通过 docker inspect 获取容器 /home/kuscia/var/images 在宿主机的挂载路径；
#      macOS 下需要去掉 /host_mnt 前缀；
#   2. 校验 KUSCIA_CONTAINER_NAME 与 IMAGE 是否为空；
#   3. 校验目标容器是否存在；
#   4. 若指定 --import，先导入引擎镜像；
#   5. 若提供自定义模板文件，则执行 apply_appimage_crd。
function register_app_image() {
  DOMAIN_IMAGE_WORK_DIR=$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/home/kuscia/var/images"}}{{.Source}}{{end}}{{end}}' "$KUSCIA_CONTAINER_NAME")
  if [[ $(uname) = Darwin ]]; then
     DOMAIN_IMAGE_WORK_DIR=$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/home/kuscia/var/images"}}{{.Source}}{{end}}{{end}}' "$KUSCIA_CONTAINER_NAME" | sed 's|/host_mnt||')
  fi
  if [[ -z "${KUSCIA_CONTAINER_NAME}" ]] || [[ -z "${IMAGE}" ]]; then
     echo -e "${RED}KUSCIA_CONTAINER_NAME and IMAGE must not be empty.${NC}"
     echo -e "${RED}$usage${NC}"
  else
    if ! docker ps -a --format '{{.Names}}' | grep -q "^${KUSCIA_CONTAINER_NAME}$"; then
      echo -e "${RED}Container ${KUSCIA_CONTAINER_NAME} does not exist.${NC}"
    else
      if [[ "${IMPORT}" == "true" ]]; then
         import_engine_image
      fi
      if [[ "${APP_IMAGE_FILE}" != "" ]]; then
         apply_appimage_crd
      fi
    fi
  fi
}

# ==================== 入口分支 ====================

# 若指定 -m 参数，进入 deploy 模式，使用默认模板注册 AppImage；
# 否则进入普通注册模式，向指定容器导入镜像并/或应用自定义模板。
if [[ "${DEPLOY}" = "true" ]]; then
  register_default_app_image
else
  register_app_image
fi
