#!/usr/bin/env bash
#
# Copyright 2026 Ant Group Co., Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#   https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# ====================================================================================================
# 统一打包脚本：基于源码构建 Kuscia 与 SecretFlow 的 Docker 镜像
#
# 说明：
#   本脚本位于 sfwork 工作区 kuscia/scripts/ 目录下，负责把同级的 kuscia/ 与 secretflow/
#   两个子项目分别构建成 Docker 镜像。构建产物可直接加载到本地 Docker，也可推送到远端
#   仓库或导出为 tar 包。
#
#   脚本会自动向上查找同时包含 kuscia/ 与 secretflow/ 的目录作为工作区根目录，
#   因此无论放在 sfwork/scripts/ 还是 kuscia/scripts/ 下都能正常运行。
#
# 前置依赖：
#   - bash（建议 4.0+）
#   - docker，并已启用 buildx（Kuscia 镜像需要跨架构构建能力）
#   - git（用于获取 Kuscia 版本号）
#   - SecretFlow 构建时若未指定 --sf-wheel，需要本地存在 secretflow/release-ci:latest
#
# 使用方式：
#   cd /home/charles/code/sfwork
#   bash kuscia/scripts/build-kuscia-secretflow-images.sh [OPTIONS]
#
# 示例：
#   # 只构建 Kuscia
#   bash kuscia/scripts/build-kuscia-secretflow-images.sh -p kuscia
#
#   # 只构建 SecretFlow 开发镜像
#   bash kuscia/scripts/build-kuscia-secretflow-images.sh -p secretflow -v 1.15.0-dev
#
#   # 构建全部并导出 tar
#   bash kuscia/scripts/build-kuscia-secretflow-images.sh -p all -v 1.15.0-dev --tar
#
#   # 指定仓库并推送
#   bash kuscia/scripts/build-kuscia-secretflow-images.sh -p all \
#       -r registry.example.com/secretflow -v 1.15.0-dev --push
# ====================================================================================================

# -e  任一命令失败即退出
# -u  使用未定义变量时报错
# -o pipefail  管道中任一命令失败即返回非零退出码
set -euo pipefail

# 记录用户调用脚本时所在的目录，tar 包会输出到该目录
OUTPUT_DIR="$(pwd -P)"

# 计算脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"

# 自动向上查找 sfwork 工作区根目录：要求该目录下同时存在 kuscia/ 与 secretflow/
ROOT_DIR="${SCRIPT_DIR}"
while [[ "${ROOT_DIR}" != "/" ]]; do
    if [[ -d "${ROOT_DIR}/kuscia" && -d "${ROOT_DIR}/secretflow" ]]; then
        break
    fi
    ROOT_DIR="$(dirname "${ROOT_DIR}")"
done

if [[ ! -d "${ROOT_DIR}/kuscia" || ! -d "${ROOT_DIR}/secretflow" ]]; then
    echo "[ERROR] Cannot find sfwork workspace root containing both kuscia/ and secretflow/." >&2
    exit 1
fi

KUSCIA_DIR="${ROOT_DIR}/kuscia"
SF_DIR="${ROOT_DIR}/secretflow"

# 默认参数：
#   PROJECT    要构建的项目，可选 kuscia / secretflow / all
#   VERSION    镜像版本号，为空时分别使用默认值
#   ARCH       Kuscia 目标架构，为空时使用宿主机架构
#   REGISTRY   镜像仓库前缀，用于生成完整镜像名
#   LATEST     是否额外打 latest 标签
#   PUSH       是否构建后推送镜像
#   SAVE_TAR   是否导出 tar 包
#   SF_WHEEL   是否使用预编译的 SecretFlow wheel 包
PROJECT="all"
VERSION=""
ARCH=""
REGISTRY=""
LATEST=0
PUSH=0
SAVE_TAR=0
SF_WHEEL=""

# Kuscia 官方基础镜像（与 kuscia/scripts/make/image.mk 保持一致）。
# 可通过环境变量覆盖，便于内网或自定义镜像源场景。
ENVOY_IMAGE="${ENVOY_IMAGE:-secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-envoy:0.6.2b0}"
DEPS_IMAGE="${DEPS_IMAGE:-secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-deps:0.7.0b0}"

# SecretFlow wheel 构建容器镜像，可通过环境变量指向私有镜像仓库或本地镜像。
RELEASE_CI_IMAGE="${RELEASE_CI_IMAGE:-secretflow/release-ci:latest}"

# ====================================================================================================
# 工具函数
# ====================================================================================================

# 打印帮助信息并退出
usage() {
    cat <<EOF
Usage: bash $(basename "$0") [OPTIONS]

Build Kuscia / SecretFlow Docker images from source.

Options:
  -h, --help                Show this help message.
  -p, --project PROJECT     Project to build: kuscia | secretflow | all (default: all).
  -v, --version VERSION     Version / tag for images.
                            Kuscia default:  git describe --tags --always
                            SecretFlow default: dev-<datetime>
  -a, --arch ARCH           Target architecture for Kuscia: amd64 | arm64 (default: host arch).
  -r, --registry REGISTRY   Registry prefix, e.g. registry.example.com/secretflow.
  -l, --latest              Also tag the image as latest.
      --push                Push images after build (requires docker login).
  -t, --tar                 Save images as tar files after build.
      --sf-wheel PATH       Use a pre-built SecretFlow wheel instead of building in container.

Examples:
  bash $(basename "$0") -p kuscia
  bash $(basename "$0") -p secretflow -v 1.15.0-dev
  bash $(basename "$0") -p all -v 1.15.0-dev --tar
  bash $(basename "$0") -p all -r registry.example.com/secretflow -v 1.15.0-dev --push
EOF
    exit 0
}

# 绿色 INFO 日志，输出到标准输出
log_info() {
    echo -e "\033[0;32m[INFO]\033[0m $*"
}

# 红色 ERROR 日志，输出到标准错误
log_error() {
    echo -e "\033[0;31m[ERROR]\033[0m $*" >&2
}

# 处理镜像仓库前缀：去除末尾斜杠，并在非空时追加一个斜杠，便于拼接完整镜像名。
# 例如：registry.example.com/secretflow  ->  registry.example.com/secretflow/
registry_prefix() {
    local r="${1:-}"
    r="${r%/}"
    if [[ -n "${r}" ]]; then
        echo "${r}/"
    fi
}

# 根据 uname -m 输出把机器架构归一化为 amd64 / arm64，未知架构保持原样返回。
detect_arch() {
    local uname_m
    uname_m=$(uname -m)
    case "${uname_m}" in
        x86_64)
            echo "amd64"
            ;;
        aarch64 | arm64)
            echo "arm64"
            ;;
        *)
            echo "${uname_m}"
            ;;
    esac
}

# 进入 Kuscia 目录，使用 git describe 获取最近的 tag，失败则返回 dev。
# 该版本号用于 Kuscia 镜像 tag 的组成部分。
kuscia_version() {
    cd "${KUSCIA_DIR}"
    git describe --tags --always 2>/dev/null || echo "dev"
}

# 为指定镜像额外生成 latest 标签。
# 例如 secretflow/kuscia:v1.0-20260101120000 -> secretflow/kuscia:latest
tag_latest() {
    local img="$1"
    local latest_tag="${img%:*}:latest"
    docker tag "${img}" "${latest_tag}"
    echo "${latest_tag}"
}

# 推送单个镜像到已登录的 Docker 仓库
push_image() {
    local img="$1"
    log_info "Pushing ${img} ..."
    docker push "${img}"
}

# 把单个镜像保存为 tar 包，输出到用户调用脚本时所在的目录（OUTPUT_DIR）。
# 文件名中把 / 与 : 都替换为 _，避免路径或文件系统特殊字符问题。
save_image_tar() {
    local img="$1"
    local filename
    filename="${img//\//_}"
    filename="${filename//:/_}.tar"
    local out_file="${OUTPUT_DIR}/${filename}"
    log_info "Saving ${img} to ${out_file} ..."
    docker save -o "${out_file}" "${img}"
}

# ====================================================================================================
# Kuscia 镜像构建
# ====================================================================================================

build_kuscia() {
    log_info "========== Building Kuscia image =========="

    # 切换到 Kuscia 源码目录，后续编译与构建命令均在该目录下执行
    cd "${KUSCIA_DIR}"

    # 确定 Kuscia 版本：命令行未指定时使用 git 描述信息，否则使用命令行传入值
    local kuscia_version
    if [[ -z "${VERSION}" ]]; then
        kuscia_version=$(kuscia_version)
    else
        kuscia_version="${VERSION}"
    fi

    # 镜像 tag 由“版本-构建时间”组成，保证每次构建tag唯一，避免覆盖历史镜像
    local tag="${kuscia_version}-${DATETIME}"
    local image_name
    image_name="$(registry_prefix "${REGISTRY}")secretflow/kuscia:${tag}"

    log_info "Kuscia version: ${kuscia_version}"
    log_info "Target arch:    ${ARCH}"
    log_info "Image name:     ${image_name}"

    # 编译 kuscia 二进制：使用 kuscia/hack/build.sh 交叉编译到 linux/${ARCH}
    log_info "Compiling kuscia binary for linux/${ARCH} ..."
    GOOS=linux GOARCH="${ARCH}" bash hack/build.sh -t kuscia

    # Kuscia Dockerfile 期望在 build/linux/${ARCH}/ 目录下找到 apps 等构建产物，
    # 因此将编译输出 build/apps 复制到该目录结构中。
    mkdir -p "build/linux/${ARCH}"
    cp -rp build/apps "build/linux/${ARCH}/"

    # 准备 docker buildx builder：若名为 sfwork-kuscia 的 builder 不存在则创建一个，
    # 支持 linux/amd64 与 linux/arm64 两种平台。
    if ! docker buildx inspect sfwork-kuscia >/dev/null 2>&1; then
        log_info "Creating docker buildx builder 'sfwork-kuscia' ..."
        docker buildx create --name sfwork-kuscia --platform linux/amd64,linux/arm64 >/dev/null
    fi

    # 使用 buildx 构建 Kuscia 镜像：
    #   --load      将构建结果加载到本地 Docker（默认 buildx 不加载）
    #   --platform  指定目标平台
    #   通过 --build-arg 注入 Envoy 与 Deps 基础镜像
    log_info "Building Kuscia Docker image ..."
    DOCKER_BUILDKIT=1 docker buildx build --builder sfwork-kuscia \
        --build-arg KUSCIA_ENVOY_IMAGE="${ENVOY_IMAGE}" \
        --build-arg DEPS_IMAGE="${DEPS_IMAGE}" \
        -f build/dockerfile/kuscia-anolis.Dockerfile \
        -t "${image_name}" \
        --platform "linux/${ARCH}" \
        --load \
        .

    # 将最终镜像名保存到全局变量，用于最后的构建摘要
    KUSCIA_IMAGE="${image_name}"

    # 若开启 latest 标签，先打 latest 标签，并在需要推送时一并推送
    if [[ "${LATEST}" -eq 1 ]]; then
        local latest
        latest=$(tag_latest "${image_name}")
        log_info "Tagged ${latest}"
        if [[ "${PUSH}" -eq 1 ]]; then
            push_image "${latest}"
        fi
    fi

    # 推送主版本镜像
    if [[ "${PUSH}" -eq 1 ]]; then
        push_image "${image_name}"
    fi

    # 导出 tar 包
    if [[ "${SAVE_TAR}" -eq 1 ]]; then
        save_image_tar "${image_name}"
    fi

    log_info "Kuscia image built successfully: ${image_name}"
}

# ====================================================================================================
# SecretFlow 开发镜像构建
# ====================================================================================================

build_secretflow() {
    log_info "========== Building SecretFlow dev image =========="

    # 切换到 SecretFlow 源码目录
    cd "${SF_DIR}"

    # 确定 SecretFlow 版本：命令行未指定时使用 dev-<构建时间>
    local sf_version
    if [[ -z "${VERSION}" ]]; then
        sf_version="dev-${DATETIME}"
    else
        sf_version="${VERSION}"
    fi

    local image_name
    image_name="$(registry_prefix "${REGISTRY}")secretflow/sf-dev-ubuntu:${sf_version}"

    log_info "SecretFlow version: ${sf_version}"
    log_info "Image name:         ${image_name}"

    # 清理函数：构建 SecretFlow 镜像时会在 docker/dev/ 目录下生成临时文件
    # （nsjail 配置、condarc、yml 模板、wheel 包等），退出前必须清理，
    # 避免污染源码目录或影响下一次构建。
    cleanup_sf_dev() {
        rm -rf "${SF_DIR}/docker/dev/.nsjail" \
               "${SF_DIR}/docker/dev/.condarc" \
               "${SF_DIR}/docker/dev/"*.yml \
               "${SF_DIR}/docker/dev/"*.whl
    }
    # 构建开始前先做一次清理，确保目录干净
    cleanup_sf_dev
    # 注册 trap：脚本退出（包括成功、失败、中断）时自动清理临时文件
    trap cleanup_sf_dev EXIT

    # 准备 wheel 包：
    #   方式一：用户通过 --sf-wheel 指定预编译 wheel，直接复制到 docker/dev/ 下
    #   方式二：在 secretflow/release-ci:latest 容器内编译 wheel（需要本地已有该镜像）
    if [[ -n "${SF_WHEEL}" ]]; then
        if [[ ! -f "${SF_WHEEL}" ]]; then
            log_error "Specified SecretFlow wheel not found: ${SF_WHEEL}"
            exit 1
        fi
        log_info "Using pre-built wheel: ${SF_WHEEL}"
        cp "${SF_WHEEL}" docker/dev/
    else
        # 检查本地是否存在 release-ci 镜像，该镜像负责在隔离环境中编译 SecretFlow wheel
        if docker image inspect "${RELEASE_CI_IMAGE}" >/dev/null 2>&1; then
            log_info "Building SecretFlow wheel inside ${RELEASE_CI_IMAGE} ..."
            docker run -i --rm \
                -e SF_BUILD_DOCKER_NAME="sf-dev-ubuntu:${sf_version}" \
                --mount "type=bind,source=${SF_DIR},target=/home/admin/src" \
                -w /home/admin \
                --cap-add=SYS_PTRACE \
                --security-opt seccomp=unconfined \
                --cap-add=NET_ADMIN \
                --privileged=true \
                "${RELEASE_CI_IMAGE}" \
                /home/admin/src/docker/dev/entry.sh
        else
            log_error "${RELEASE_CI_IMAGE} is not available locally."
            log_error "Please either pull it first:"
            log_error "  docker pull ${RELEASE_CI_IMAGE}"
            log_error "Or use a different image by setting RELEASE_CI_IMAGE=<image>"
            log_error "Or build the wheel manually in a Python 3.10 environment and pass it with --sf-wheel."
            exit 1
        fi
    fi

    # 校验 wheel 包是否已成功生成：docker/dev/ 目录下必须存在至少一个 .whl 文件
    local wheel_count
    wheel_count=$(ls -1 docker/dev/*.whl 2>/dev/null | wc -l)
    if [[ "${wheel_count}" -eq 0 ]]; then
        log_error "No SecretFlow wheel found in secretflow/docker/dev/"
        exit 1
    fi

    # 若存在多个 wheel（例如多次构建残留），只保留最新的一个，避免 Dockerfile 中
    # COPY *.whl + pip install /tmp/*.whl 因版本冲突而失败。
    if [[ "${wheel_count}" -gt 1 ]]; then
        log_info "Multiple SecretFlow wheels found, keeping only the latest one."
        local latest_wheel
        latest_wheel=$(ls -t docker/dev/secretflow-*.whl 2>/dev/null | head -n 1)
        find docker/dev -maxdepth 1 -name 'secretflow-*.whl' ! -name "$(basename "${latest_wheel}")" -delete
    fi

    # 准备 SecretFlow 镜像构建所需的运行时配置文件与模板：
    #   .nsjail   沙箱配置
    #   .condarc  conda 源配置
    #   *.yml     部署与配置模板
    cp -r docker/release/.nsjail docker/dev/
    cp docker/release/.condarc docker/dev/
    cp docker/*.yml docker/dev/

    # 构建 SecretFlow 开发镜像。
    # 通过 --build-arg 传入配置模板与部署模板的内容。
    log_info "Building SecretFlow dev Docker image ..."
    docker build -f docker/dev/Dockerfile \
        -t "${image_name}" \
        --build-arg config_templates="$(cat docker/config_templates.yml)" \
        --build-arg deploy_templates="$(cat docker/deploy_templates.yml)" \
        docker/dev

    # 保存最终镜像名
    SF_DEV_IMAGE="${image_name}"

    # latest 标签处理
    if [[ "${LATEST}" -eq 1 ]]; then
        local latest
        latest=$(tag_latest "${image_name}")
        log_info "Tagged ${latest}"
        if [[ "${PUSH}" -eq 1 ]]; then
            push_image "${latest}"
        fi
    fi

    # 推送主版本镜像
    if [[ "${PUSH}" -eq 1 ]]; then
        push_image "${image_name}"
    fi

    # 导出 tar 包
    if [[ "${SAVE_TAR}" -eq 1 ]]; then
        save_image_tar "${image_name}"
    fi

    # 构建成功后主动取消 trap 并再次清理临时文件，保持源码目录干净
    trap - EXIT
    cleanup_sf_dev

    log_info "SecretFlow dev image built successfully: ${image_name}"
}

# ====================================================================================================
# 参数解析
# ====================================================================================================

# 逐个解析命令行参数，长/短选项均支持。
while [[ "$#" -gt 0 ]]; do
    case "$1" in
        -h | --help)
            usage
            ;;
        -p | --project)
            PROJECT="$2"
            shift 2
            ;;
        -v | --version)
            VERSION="$2"
            shift 2
            ;;
        -a | --arch)
            ARCH="$2"
            shift 2
            ;;
        -r | --registry)
            REGISTRY="$2"
            shift 2
            ;;
        -l | --latest)
            LATEST=1
            shift
            ;;
        --push)
            PUSH=1
            shift
            ;;
        -t | --tar)
            SAVE_TAR=1
            shift
            ;;
        --sf-wheel)
            SF_WHEEL="$2"
            shift 2
            ;;
        *)
            log_error "Unknown argument: $1"
            usage
            ;;
    esac
done

# 校验 project 参数必须是允许的三个值之一
if [[ "${PROJECT}" != "kuscia" && "${PROJECT}" != "secretflow" && "${PROJECT}" != "all" ]]; then
    log_error "Invalid project: ${PROJECT}. Must be one of: kuscia, secretflow, all."
    exit 1
fi

# 若未指定架构，则自动探测宿主机架构作为 Kuscia 构建目标架构
if [[ -z "${ARCH}" ]]; then
    ARCH=$(detect_arch)
fi

# 生成构建时间戳，用于镜像 tag 与 SecretFlow 默认版本号，保证唯一性
DATETIME=$(date +"%Y%m%d%H%M%S")

# 用于在 build 函数外保存最终镜像名的全局变量，初始为空
KUSCIA_IMAGE=""
SF_DEV_IMAGE=""

# ====================================================================================================
# 主流程
# ====================================================================================================

# 根据用户选择调用对应的构建函数
if [[ "${PROJECT}" == "kuscia" || "${PROJECT}" == "all" ]]; then
    build_kuscia
fi

if [[ "${PROJECT}" == "secretflow" || "${PROJECT}" == "all" ]]; then
    build_secretflow
fi

# 输出构建摘要：列出最终镜像名、latest 标签、推送与导出状态
log_info "========== Build summary =========="
if [[ -n "${KUSCIA_IMAGE}" ]]; then
    echo "  Kuscia image:      ${KUSCIA_IMAGE}"
fi
if [[ -n "${SF_DEV_IMAGE}" ]]; then
    echo "  SecretFlow image:  ${SF_DEV_IMAGE}"
fi
if [[ "${LATEST}" -eq 1 ]]; then
    echo "  Also tagged as:    latest"
fi
if [[ "${PUSH}" -eq 1 ]]; then
    echo "  Pushed to registry."
fi
if [[ "${SAVE_TAR}" -eq 1 ]]; then
    echo "  Saved as tar files in: ${OUTPUT_DIR}"
fi
