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

# -----------------------------------------------------------------------------
# Shell 严格模式设置，提升脚本健壮性
#   -e  (errexit)          任一命令返回非零退出码时立即终止整个脚本
#   -u  (nounset)          使用未定义变量时报错，防止拼写错误导致意外行为
#   -o pipefail            管道中任一命令失败即返回非零退出码，而非仅看最后一个命令
# -----------------------------------------------------------------------------
set -euo pipefail


# -----------------------------------------------------------------------------
# 记录用户调用脚本时所在的目录（当前工作目录，Current Working Directory）
# -P 选项返回物理路径（Physical path），自动解析并去除所有符号链接，确保路径真实可靠
# 无论用户从哪个目录调用本脚本，tar 包的最终输出都将放置在此目录下
# -----------------------------------------------------------------------------
OUTPUT_DIR="$(pwd -P)"

# -----------------------------------------------------------------------------
# 计算脚本自身所在的目录（Script Directory）
# ${BASH_SOURCE[0]} 获取当前脚本文件的路径（兼容 source 和 bash 直接执行两种方式）
# dirname 提取其所在目录，cd 进入后 pwd -P 获取绝对物理路径
# 这样即使脚本通过相对路径、绝对路径或符号链接被调用，也能准确定位脚本真实位置
# -----------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"

# -----------------------------------------------------------------------------
# 自动向上查找 sfwork 工作区根目录（Workspace Root Directory）
# 判定标准：该目录下必须同时存在 kuscia/ 和 secretflow/ 两个子目录
# 从脚本所在目录开始，逐级向父目录遍历，直到根目录 "/" 为止
# 这种向上查找机制使得脚本可以放在工作区任意层级子目录中，都能自动定位根目录
# -----------------------------------------------------------------------------
ROOT_DIR="${SCRIPT_DIR}"
while [[ "${ROOT_DIR}" != "/" ]]; do
    # 检查当前候选目录是否同时包含 kuscia/ 和 secretflow/ 两个子目录
    if [[ -d "${ROOT_DIR}/kuscia" && -d "${ROOT_DIR}/secretflow" ]]; then
        # 找到匹配的工作区根目录，立即跳出循环
        break
    fi
    # 未找到则移动到父目录，继续下一轮查找
    # dirname 会去掉路径最后一个组件，实现向上级目录移动
    ROOT_DIR="$(dirname "${ROOT_DIR}")"
done

# -----------------------------------------------------------------------------
# 最终校验：循环结束后再次确认是否成功定位到工作区根目录
# 如果遍历到根目录仍未同时发现 kuscia/ 和 secretflow/，则判定查找失败
# 此时脚本无法确定构建所需的源码位置，必须终止执行
# -----------------------------------------------------------------------------
if [[ ! -d "${ROOT_DIR}/kuscia" || ! -d "${ROOT_DIR}/secretflow" ]]; then
    # 将错误信息输出到标准错误流（>&2），而非标准输出，便于调用方区分错误日志
    echo "[ERROR] Cannot find sfwork workspace root containing both kuscia/ and secretflow/." >&2
    # 以非零退出码（1）终止脚本，提示调用方工作区定位失败，可被 CI/CD 或外层脚本捕获
    exit 1
fi

# 根据找到的根目录，分别设置 Kuscia 和 SecretFlow 的源码子目录路径
KUSCIA_DIR="${ROOT_DIR}/kuscia"
SF_DIR="${ROOT_DIR}/secretflow"

# -----------------------------------------------------------------------------
# 默认参数定义
#   PROJECT    要构建的项目，可选 kuscia / secretflow / all（默认构建全部）
#   VERSION    镜像版本号，为空时分别使用各自默认值（Kuscia 用 git tag，SecretFlow 用 dev-时间戳）
#   ARCH       Kuscia 目标架构，为空时自动探测宿主机架构（amd64 / arm64）
#   REGISTRY   镜像仓库前缀，用于生成完整镜像名（如 registry.example.com/secretflow/）
#   LATEST     是否额外打 latest 标签（布尔值：0=否，1=是）
#   PUSH       是否构建后推送镜像到远端仓库（布尔值：0=否，1=是）
#   SAVE_TAR   是否导出 tar 包到 OUTPUT_DIR（布尔值：0=否，1=是）
#   SF_WHEEL   是否使用预编译的 SecretFlow wheel 包路径（空字符串表示需要现场编译）
# -----------------------------------------------------------------------------
PROJECT="all"
VERSION=""
ARCH=""
REGISTRY=""
LATEST=0
PUSH=0
SAVE_TAR=0
SF_WHEEL=""

# -----------------------------------------------------------------------------
# Kuscia 官方基础镜像定义（与 kuscia/scripts/make/image.mk 保持一致）
# 通过环境变量覆盖默认值，便于内网环境、自定义镜像源或离线场景使用
#   ENVOY_IMAGE  : Kuscia 依赖的 Envoy 代理基础镜像
#   DEPS_IMAGE   : Kuscia 依赖的系统层基础镜像
# -----------------------------------------------------------------------------
ENVOY_IMAGE="${ENVOY_IMAGE:-secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-envoy:0.6.2b0}"
DEPS_IMAGE="${DEPS_IMAGE:-secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-deps:0.7.0b0}"

# -----------------------------------------------------------------------------
# SecretFlow wheel 构建容器镜像定义
# 可通过环境变量 RELEASE_CI_IMAGE 指向私有镜像仓库或本地已导入的镜像
# 该镜像负责在隔离容器环境中编译 SecretFlow 的 Python wheel 包
# -----------------------------------------------------------------------------
RELEASE_CI_IMAGE="${RELEASE_CI_IMAGE:-secretflow/release-ci:latest}"

# ====================================================================================================
# 工具函数（Utility Functions）
# ====================================================================================================

# -----------------------------------------------------------------------------
# 打印帮助信息并退出
# 使用 here-document（<<EOF）输出多行说明文本，包含用法、选项说明和示例
# 调用后直接 exit 0（成功退出），不执行后续构建逻辑
# -----------------------------------------------------------------------------
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

# -----------------------------------------------------------------------------
# 绿色 INFO 日志，输出到标准输出（stdout）
# 使用 ANSI 转义序列 \033[0;32m 设置绿色前景色，\033[0m 重置颜色
# 便于在终端中快速区分日志级别
# -----------------------------------------------------------------------------
log_info() {
    echo -e "\033[0;32m[INFO]\033[0m $*"
}

# -----------------------------------------------------------------------------
# 红色 ERROR 日志，输出到标准错误（stderr）
# 使用 >&2 重定向到标准错误流，确保错误信息能被管道或日志系统正确捕获
# ANSI 转义序列 \033[0;31m 设置红色前景色
# -----------------------------------------------------------------------------
log_error() {
    echo -e "\033[0;31m[ERROR]\033[0m $*" >&2
}

# -----------------------------------------------------------------------------
# 处理镜像仓库前缀（Registry Prefix）
# 功能：去除末尾斜杠，并在非空时追加一个斜杠，便于后续拼接完整镜像名
# 例如：registry.example.com/secretflow  ->  registry.example.com/secretflow/
# 若输入为空，则输出空字符串，不影响镜像名拼接
# -----------------------------------------------------------------------------
registry_prefix() {
    local r="${1:-}"          # 读取第一个参数，未提供则默认空字符串
    r="${r%/}"                # 去除变量 r 末尾的斜杠（如果有的话）
    if [[ -n "${r}" ]]; then  # 若处理后的字符串非空
        echo "${r}/"           # 输出时追加一个斜杠，保证后续拼接格式正确
    fi
}

# -----------------------------------------------------------------------------
# 探测宿主机架构并归一化为标准 Docker 平台标识
# 根据 uname -m 的输出映射：
#   x86_64        -> amd64
#   aarch64/arm64 -> arm64
#   其他          -> 保持原样返回
# 返回值用于 Kuscia 的 GOARCH 交叉编译和 Docker buildx 的 --platform 参数
# -----------------------------------------------------------------------------
detect_arch() {
    local uname_m
    uname_m=$(uname -m)       # 获取内核报告的机器硬件名称
    case "${uname_m}" in
        x86_64)
            echo "amd64"      # Intel/AMD 64位架构的标准命名
            ;;
        aarch64 | arm64)
            echo "arm64"      # ARM 64位架构的标准命名（兼容两种常见写法）
            ;;
        *)
            echo "${uname_m}" # 未知架构保持原样，避免误转换
            ;;
    esac
}

# -----------------------------------------------------------------------------
# 获取 Kuscia 版本号
# 进入 Kuscia 源码目录，使用 git describe 获取最近的 tag 和 commit 缩写
# 若目录不是 git 仓库或没有 tag，则回退到默认值 "dev"
# 该版本号用于 Kuscia 镜像 tag 的组成部分，保证可追溯性
# -----------------------------------------------------------------------------
kuscia_version() {
    cd "${KUSCIA_DIR}"
    git describe --tags --always 2>/dev/null || echo "dev"
}

# -----------------------------------------------------------------------------
# 为指定镜像额外生成 latest 标签
# 逻辑：将镜像名中最后一个冒号及之后的内容（即 tag）替换为 "latest"
# 例如 secretflow/kuscia:v1.0-20260101120000 -> secretflow/kuscia:latest
# 然后执行 docker tag 命令在本地创建新标签
# -----------------------------------------------------------------------------
tag_latest() {
    local img="$1"
    local latest_tag="${img%:*}:latest"  # %:* 删除从右起第一个冒号及之后的所有字符
    docker tag "${img}" "${latest_tag}"
    echo "${latest_tag}"
}

# -----------------------------------------------------------------------------
# 推送单个镜像到已登录的 Docker 仓库
# 调用 docker push 命令，并在推送前打印日志便于追踪构建进度
# 注意：调用前必须已执行 docker login 登录目标仓库，否则推送会失败
# -----------------------------------------------------------------------------
push_image() {
    local img="$1"
    log_info "Pushing ${img} ..."
    docker push "${img}"
}

# -----------------------------------------------------------------------------
# 把单个镜像保存为 tar 包，输出到用户调用脚本时所在的目录（OUTPUT_DIR）
# 文件名处理：
#   1. 将镜像名中的 / 替换为 _（避免创建子目录）
#   2. 将 : 替换为 _（避免文件系统特殊字符问题）
#   3. 追加 .tar 后缀
# 例如：secretflow/kuscia:v1.0  ->  secretflow_kuscia_v1.0.tar
# 最终使用 docker save 命令导出镜像二进制数据到文件
# -----------------------------------------------------------------------------
save_image_tar() {
    local img="$1"
    local filename
    filename="${img//\//_}"      # 全局替换 / 为 _
    filename="${filename//:/_}.tar"  # 全局替换 : 为 _，并追加 .tar
    local out_file="${OUTPUT_DIR}/${filename}"
    log_info "Saving ${img} to ${out_file} ..."
    docker save -o "${out_file}" "${img}"
}

# ====================================================================================================
# Kuscia 镜像构建（Build Kuscia Image）
# ====================================================================================================

build_kuscia() {
    log_info "========== Building Kuscia image =========="

    # 切换到 Kuscia 源码目录，后续编译与构建命令均在该目录下执行
    cd "${KUSCIA_DIR}"

    # 确定 Kuscia 版本：命令行未指定 (-v) 时，调用 kuscia_version() 从 git 获取
    # 若用户指定了版本，则直接使用用户输入值
    local kuscia_version
    if [[ -z "${VERSION}" ]]; then
        kuscia_version=$(kuscia_version)
    else
        kuscia_version="${VERSION}"
    fi

    # 镜像 tag 由"版本-构建时间"组成，格式如 v1.0-20260101120000
    # 保证每次构建 tag 唯一，避免覆盖历史镜像，便于回滚和审计
    local tag="${kuscia_version}-${DATETIME}"
    local image_name
    # 拼接完整镜像名：仓库前缀 + 镜像路径 + tag
    image_name="$(registry_prefix "${REGISTRY}")secretflow/kuscia:${tag}"

    log_info "Kuscia version: ${kuscia_version}"
    log_info "Target arch:    ${ARCH}"
    log_info "Image name:     ${image_name}"

    # -----------------------------------------------------------------------------
    # 编译 kuscia 二进制文件
    # 使用 Kuscia 自带的 hack/build.sh 脚本进行交叉编译
    # 通过环境变量 GOOS=linux GOARCH=${ARCH} 指定目标平台，确保在任意宿主机上都能生成 Linux 可执行文件
    # -t kuscia 参数告诉 build.sh 只编译 kuscia 主程序
    # -----------------------------------------------------------------------------
    log_info "Compiling kuscia binary for linux/${ARCH} ..."
    GOOS=linux GOARCH="${ARCH}" bash hack/build.sh -t kuscia

    # -----------------------------------------------------------------------------
    # 准备 Docker 构建上下文所需的目录结构
    # Kuscia 的 Dockerfile 期望在 build/linux/${ARCH}/ 目录下找到 apps 等构建产物
    # 因此将编译输出 build/apps 复制到该目录结构中，确保 docker build 时能找到文件
    # -----------------------------------------------------------------------------
    mkdir -p "build/linux/${ARCH}"
    cp -rp build/apps "build/linux/${ARCH}/"

    # -----------------------------------------------------------------------------
    # 准备 docker buildx builder 实例
    # 检查名为 sfwork-kuscia 的 builder 是否已存在
    # 若不存在则创建一个，显式支持 linux/amd64 与 linux/arm64 两种平台
    # buildx builder 是 Docker 多架构构建的基础设施，支持跨平台镜像编译
    # >/dev/null 2>&1 抑制 inspect 命令的正常输出和错误输出，保持终端整洁
    # -----------------------------------------------------------------------------
    if ! docker buildx inspect sfwork-kuscia >/dev/null 2>&1; then
        log_info "Creating docker buildx builder 'sfwork-kuscia' ..."
        docker buildx create --name sfwork-kuscia --platform linux/amd64,linux/arm64 >/dev/null
    fi

    # -----------------------------------------------------------------------------
    # 使用 buildx 构建 Kuscia 镜像
    # 参数说明：
    #   --builder sfwork-kuscia    使用上面创建/检查的 builder 实例
    #   --build-arg                向 Dockerfile 注入构建参数（Envoy 和 Deps 基础镜像地址）
    #   -f                         指定 Dockerfile 路径
    #   -t                         设置镜像标签
    #   --platform                 指定目标平台（如 linux/amd64）
    #   --load                     将构建结果加载到本地 Docker 守护进程（默认 buildx 不加载）
    #   .                          构建上下文为当前目录（KUSCIA_DIR）
    # DOCKER_BUILDKIT=1 显式启用 BuildKit 构建引擎，提升构建效率和功能
    # -----------------------------------------------------------------------------
    log_info "Building Kuscia Docker image ..."
    DOCKER_BUILDKIT=1 docker buildx build --builder sfwork-kuscia \
        --build-arg KUSCIA_ENVOY_IMAGE="${ENVOY_IMAGE}" \
        --build-arg DEPS_IMAGE="${DEPS_IMAGE}" \
        -f build/dockerfile/kuscia-anolis.Dockerfile \
        -t "${image_name}" \
        --platform "linux/${ARCH}" \
        --load \
        .

    # 将最终镜像名保存到全局变量，用于最后的构建摘要输出
    KUSCIA_IMAGE="${image_name}"

    # -----------------------------------------------------------------------------
    # 若用户开启 --latest 选项，额外创建 latest 标签
    # 并在开启 --push 时一并推送 latest 标签到仓库
    # latest 标签通常用于指向当前最稳定的版本，但需谨慎使用避免生产环境误更新
    # -----------------------------------------------------------------------------
    if [[ "${LATEST}" -eq 1 ]]; then
        local latest
        latest=$(tag_latest "${image_name}")
        log_info "Tagged ${latest}"
        if [[ "${PUSH}" -eq 1 ]]; then
            push_image "${latest}"
        fi
    fi

    # 若开启 --push 选项，推送主版本镜像（带具体版本 tag）
    if [[ "${PUSH}" -eq 1 ]]; then
        push_image "${image_name}"
    fi

    # 若开启 --tar 选项，导出当前镜像为 tar 包到 OUTPUT_DIR
    if [[ "${SAVE_TAR}" -eq 1 ]]; then
        save_image_tar "${image_name}"
    fi

    log_info "Kuscia image built successfully: ${image_name}"
}

# ====================================================================================================
# SecretFlow 开发镜像构建（Build SecretFlow Dev Image）
# ====================================================================================================

build_secretflow() {
    log_info "========== Building SecretFlow dev image =========="

    # 切换到 SecretFlow 源码目录，后续所有文件操作和构建均基于此目录
    cd "${SF_DIR}"

    # 确定 SecretFlow 版本号：命令行未指定时，使用 dev-构建时间戳 作为默认值
    # 例如：dev-20260101120000，表示这是开发版本，且包含构建时间信息
    local sf_version
    if [[ -z "${VERSION}" ]]; then
        sf_version="dev-${DATETIME}"
    else
        sf_version="${VERSION}"
    fi

    local image_name
    # 拼接完整镜像名：仓库前缀 + 镜像路径 + tag
    image_name="$(registry_prefix "${REGISTRY}")secretflow/sf-dev-ubuntu:${sf_version}"

    log_info "SecretFlow version: ${sf_version}"
    log_info "Image name:         ${image_name}"

    # -----------------------------------------------------------------------------
    # 定义清理函数：构建 SecretFlow 镜像时会在 docker/dev/ 目录下生成临时文件
    # 包括 nsjail 配置、condarc、yml 模板、wheel 包等，构建结束后必须清理
    # 避免污染源码目录或影响下一次构建（如旧 wheel 包导致版本冲突）
    # -----------------------------------------------------------------------------
    cleanup_sf_dev() {
        rm -rf "${SF_DIR}/docker/dev/.nsjail" \
               "${SF_DIR}/docker/dev/.condarc" \
               "${SF_DIR}/docker/dev/"*.yml \
               "${SF_DIR}/docker/dev/"*.whl
    }
    # 构建开始前先执行一次清理，确保 docker/dev/ 目录处于干净状态
    cleanup_sf_dev
    # 注册 trap：捕获 EXIT 信号（脚本正常退出、报错退出、被中断时均触发），自动执行清理
    # 这是防御性编程，确保即使构建中途失败，也不会遗留临时文件
    trap cleanup_sf_dev EXIT

    # -----------------------------------------------------------------------------
    # 准备 SecretFlow 的 wheel 包（Python 二进制分发包）
    # 两种获取方式：
    #   方式一：用户通过 --sf-wheel 指定预编译 wheel 文件路径，脚本直接复制到 docker/dev/
    #   方式二：在 secretflow/release-ci:latest 容器内现场编译 wheel
    #           需要本地已有该镜像，通过 docker run 挂载源码目录进行隔离编译
    # -----------------------------------------------------------------------------
    if [[ -n "${SF_WHEEL}" ]]; then
        # 用户指定了预编译 wheel 路径，先校验文件是否存在
        if [[ ! -f "${SF_WHEEL}" ]]; then
            log_error "Specified SecretFlow wheel not found: ${SF_WHEEL}"
            exit 1
        fi
        log_info "Using pre-built wheel: ${SF_WHEEL}"
        cp "${SF_WHEEL}" docker/dev/
    else
        # 检查本地 Docker 中是否存在 release-ci 镜像
        # docker image inspect 会返回镜像的详细元数据，若镜像不存在则返回非零退出码
        if docker image inspect "${RELEASE_CI_IMAGE}" >/dev/null 2>&1; then
            log_info "Building SecretFlow wheel inside ${RELEASE_CI_IMAGE} ..."
            # 在 release-ci 容器内编译 wheel：
            #   -i --rm          交互模式运行，容器退出后自动删除（不残留容器实例）
            #   -e                设置环境变量，传入镜像名称供容器内脚本使用
            #   --mount type=bind 将宿主机 SF_DIR 挂载到容器内 /home/admin/src，实现源码共享
            #   -w /home/admin    设置容器内工作目录
            #   --cap-add / --security-opt / --privileged
            #                     授予容器额外权限，满足编译过程中可能的系统调用需求
            #   /home/admin/src/docker/dev/entry.sh
            #                     容器启动后执行的入口脚本，负责实际的 wheel 编译
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
            # release-ci 镜像不存在时，给出清晰的错误提示和解决建议
            log_error "${RELEASE_CI_IMAGE} is not available locally."
            log_error "Please either pull it first:"
            log_error "  docker pull ${RELEASE_CI_IMAGE}"
            log_error "Or use a different image by setting RELEASE_CI_IMAGE=<image>"
            log_error "Or build the wheel manually in a Python 3.10 environment and pass it with --sf-wheel."
            exit 1
        fi
    fi

    # -----------------------------------------------------------------------------
    # 校验 wheel 包是否已成功生成
    # 统计 docker/dev/ 目录下 .whl 文件数量，必须至少存在一个
    # ls -1 每行输出一个文件名，wc -l 统计行数
    # 2>/dev/null 抑制 ls 在无匹配文件时的错误输出
    # -----------------------------------------------------------------------------
    local wheel_count
    wheel_count=$(ls -1 docker/dev/*.whl 2>/dev/null | wc -l)
    if [[ "${wheel_count}" -eq 0 ]]; then
        log_error "No SecretFlow wheel found in secretflow/docker/dev/"
        exit 1
    fi

    # -----------------------------------------------------------------------------
    # 若存在多个 wheel 文件（例如多次构建残留或目录未清理干净），只保留最新的一个
    # 原因：Dockerfile 中通常使用 COPY *.whl + pip install /tmp/*.whl
    #       多个 wheel 可能导致 pip 因版本冲突而安装失败
    # 实现：ls -t 按修改时间倒序排列，head -n 1 取第一个（即最新）
    #       find 删除所有不匹配最新文件名的 secretflow-*.whl
    # -----------------------------------------------------------------------------
    if [[ "${wheel_count}" -gt 1 ]]; then
        log_info "Multiple SecretFlow wheels found, keeping only the latest one."
        local latest_wheel
        latest_wheel=$(ls -t docker/dev/secretflow-*.whl 2>/dev/null | head -n 1)
        # -maxdepth 1 限制只查找当前目录，不递归子目录
        # ! -name 取反匹配，保留最新文件，删除其余
        find docker/dev -maxdepth 1 -name 'secretflow-*.whl' ! -name "$(basename "${latest_wheel}")" -delete
    fi

    # -----------------------------------------------------------------------------
    # 准备 SecretFlow 镜像构建所需的运行时配置文件与模板
    # 从 docker/release/ 目录复制到 docker/dev/ 目录，供 Dockerfile 使用：
    #   .nsjail   : 沙箱配置文件，用于限制运行时进程权限
    #   .condarc  : conda 包管理器源配置，加速依赖安装
    #   *.yml     : 部署与配置模板，用于镜像内服务编排
    # -----------------------------------------------------------------------------
    cp -r docker/release/.nsjail docker/dev/
    cp docker/release/.condarc docker/dev/
    cp docker/*.yml docker/dev/

    # -----------------------------------------------------------------------------
    # 构建 SecretFlow 开发镜像
    # 使用标准 docker build（非 buildx），因为 SecretFlow 通常只需宿主机架构
    # 通过 --build-arg 传入配置模板和部署模板的文件内容（使用 $(cat ...) 命令替换）
    # 构建上下文指定为 docker/dev/ 目录，Dockerfile 位于该目录下
    # -----------------------------------------------------------------------------
    log_info "Building SecretFlow dev Docker image ..."
    docker build -f docker/dev/Dockerfile \
        -t "${image_name}" \
        --build-arg config_templates="$(cat docker/config_templates.yml)" \
        --build-arg deploy_templates="$(cat docker/deploy_templates.yml)" \
        docker/dev

    # 保存最终镜像名到全局变量，用于构建摘要
    SF_DEV_IMAGE="${image_name}"

    # -----------------------------------------------------------------------------
    # 若开启 --latest，额外创建 latest 标签并推送（若同时开启 --push）
    # -----------------------------------------------------------------------------
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

    # -----------------------------------------------------------------------------
    # 构建成功后的清理工作
    # 1. 取消之前注册的 EXIT trap，避免脚本正常退出时再次执行清理（虽然重复清理无害）
    # 2. 手动调用 cleanup_sf_dev 删除临时文件，保持源码目录干净
    # -----------------------------------------------------------------------------
    trap - EXIT
    cleanup_sf_dev

    log_info "SecretFlow dev image built successfully: ${image_name}"
}

# ====================================================================================================
# 命令行参数解析（Argument Parsing）
# ====================================================================================================

# 使用 while 循环逐个解析命令行参数，支持长选项（--help）和短选项（-h）
# shift 命令用于移动参数位置，处理完一个参数后移除，继续下一个
while [[ "$#" -gt 0 ]]; do
    case "$1" in
        -h | --help)
            usage           # 显示帮助信息并退出
            ;;
        -p | --project)
            PROJECT="$2"    # 读取下一个参数作为项目名
            shift 2         # 移动两个位置（选项名 + 选项值）
            ;;
        -v | --version)
            VERSION="$2"    # 读取版本号
            shift 2
            ;;
        -a | --arch)
            ARCH="$2"       # 读取目标架构
            shift 2
            ;;
        -r | --registry)
            REGISTRY="$2"   # 读取仓库前缀
            shift 2
            ;;
        -l | --latest)
            LATEST=1        # 布尔标志，无需额外参数，只需移动一个位置
            shift
            ;;
        --push)
            PUSH=1          # 布尔标志
            shift
            ;;
        -t | --tar)
            SAVE_TAR=1      # 布尔标志
            shift
            ;;
        --sf-wheel)
            SF_WHEEL="$2"   # 读取预编译 wheel 文件路径
            shift 2
            ;;
        *)
            # 遇到未知参数，打印错误并显示帮助信息
            log_error "Unknown argument: $1"
            usage
            ;;
    esac
done

# -----------------------------------------------------------------------------
# 参数校验：project 必须是允许的三个值之一
# 若输入非法值，打印错误并退出
# -----------------------------------------------------------------------------
if [[ "${PROJECT}" != "kuscia" && "${PROJECT}" != "secretflow" && "${PROJECT}" != "all" ]]; then
    log_error "Invalid project: ${PROJECT}. Must be one of: kuscia, secretflow, all."
    exit 1
fi

# -----------------------------------------------------------------------------
# 若用户未指定架构（-a），则自动探测宿主机架构作为 Kuscia 构建目标架构
# 确保默认行为在任意机器上都能正常工作
# -----------------------------------------------------------------------------
if [[ -z "${ARCH}" ]]; then
    ARCH=$(detect_arch)
fi

# -----------------------------------------------------------------------------
# 生成构建时间戳，格式为年月日时分秒（如 20260101120000）
# 用于：
#   1. Kuscia 镜像 tag 的组成部分（保证唯一性）
#   2. SecretFlow 默认版本号（dev-时间戳）
# -----------------------------------------------------------------------------
DATETIME=$(date +"%Y%m%d%H%M%S")

# -----------------------------------------------------------------------------
# 全局变量，用于在 build 函数外保存最终镜像名
# 初始为空字符串，构建成功后分别赋值
# 用于最后统一输出构建摘要
# -----------------------------------------------------------------------------
KUSCIA_IMAGE=""
SF_DEV_IMAGE=""

# ====================================================================================================
# 主流程（Main Execution）
# ====================================================================================================

# 根据用户选择（-p 参数）调用对应的构建函数
# 若选择 "all"，则两个构建函数都会执行，依次构建 Kuscia 和 SecretFlow
if [[ "${PROJECT}" == "kuscia" || "${PROJECT}" == "all" ]]; then
    build_kuscia
fi

if [[ "${PROJECT}" == "secretflow" || "${PROJECT}" == "all" ]]; then
    build_secretflow
fi

# -----------------------------------------------------------------------------
# 构建摘要输出（Build Summary）
# 列出本次构建的关键信息，便于用户核对和记录：
#   - 最终镜像名（含 tag）
#   - 是否额外打了 latest 标签
#   - 是否已推送到远端仓库
#   - tar 包保存路径（若开启）
# -----------------------------------------------------------------------------
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