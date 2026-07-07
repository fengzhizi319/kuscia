# ====================================================================================================
# image.mk - Docker 镜像构建模块
# ====================================================================================================
# 本文件定义了 Kuscia 项目所有与 Docker 镜像相关的构建目标。
# 通过 `make image`、`make deps-image` 等命令调用。
# 被主 Makefile 通过 `-f scripts/make/image.mk` 加载。
# ====================================================================================================

# ========================================== image.mk ===============================================
# All make targets related to image should be defined here.
# ========================================== image.mk ===============================================
#make image
#    │
#    ├──► 先执行 build 目标（编译 Go 代码）
#    │       └──► 生成可执行二进制文件
#    │
#    └──► 执行 image 目标
#            ├──► start_docker_buildx（检查/创建 buildx builder）
#            │
#            └──► docker buildx build
#                    ├──► FROM ${DEPS_IMAGE}（基础依赖镜像）
#                    ├──► COPY --from=${ENVOY_IMAGE}（拷贝 Envoy 二进制）
#                    └──► COPY 编译好的 Go 二进制（来自 build 目标）
#                    └──► 生成最终镜像: secretflow/kuscia:v0.7.0-202507071200

#关键技术点
# -BuildKit: DOCKER_BUILDKIT=1 启用 Docker 下一代构建引擎，支持并行构建、缓存导出、多平台等高级特性。
# -Buildx: Docker 的扩展构建工具，支持跨平台构建（在 x86 机器上构建 arm64 镜像）。
# -多阶段构建: 通过 --build-arg 传入基础镜像地址，Dockerfile 中可以使用 FROM ${DEPS_IMAGE} 和 COPY --from=${ENVOY_IMAGE} 实现多阶段构建，减小最终镜像体积。
# -Anolis OS: kuscia-anolis.Dockerfile 基于龙蜥操作系统（Anolis OS），这是阿里/蚂蚁主导的开源 Linux 发行版，兼容 CentOS/RHEL 生态。
# ====================================================================================================
# 变量定义区
# ====================================================================================================

# TAG: 镜像标签，由版本号 + 时间戳组成
# 例如: v0.7.0-202507071200
# ${KUSCIA_VERSION_TAG} 和 ${DATETIME} 在 common.mk 中定义
TAG = ${KUSCIA_VERSION_TAG}-${DATETIME}

# IMG: 主镜像的完整名称
# 格式: 仓库名/镜像名:标签
# 例如: secretflow/kuscia:v0.7.0-202507071200
IMG := secretflow/kuscia:${TAG}

# ====================================================================================================
# 基础镜像地址（可在命令行覆盖，如 make image PROOT_IMAGE=xxx）
# 这些镜像作为构建时的依赖基础镜像
# ====================================================================================================

# PROOT_IMAGE: proot 工具镜像（用于在非 root 环境下运行程序）
# proot 是一个用户空间模拟器，可以在没有 root 权限的情况下模拟 chroot 环境
PROOT_IMAGE ?= secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/proot

# ENVOY_IMAGE: Kuscia 的 Envoy 代理镜像（服务网格/流量代理组件）
# 版本 0.6.2b0 中的 b0 表示 beta 版本
ENVOY_IMAGE ?= secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-envoy:0.6.2b0

# DEPS_IMAGE: Kuscia 依赖基础镜像（包含编译好的依赖库）
# 这个镜像通常很大，包含了所有第三方依赖，避免每次构建都重新下载
DEPS_IMAGE ?= secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-deps:0.7.0b0

# ====================================================================================================
# 宏定义（Makefile 函数）
# ====================================================================================================

# start_docker_buildx: 检查并启动 docker buildx 构建器
#
# 功能说明:
#   1. 检查名为 "kuscia" 的 buildx builder 是否存在
#   2. 如果不存在，创建一个支持多平台(linux/arm64, linux/amd64)的 builder
#   3. 切换到 kuscia builder 进行后续构建
#
# 使用场景:
#   buildx 是 Docker 的高级构建工具，支持多平台构建、缓存优化等
#   这里确保 builder 存在且被激活，避免每次手动检查
define start_docker_buildx
	# 检查是否存在名为 kuscia 的 buildx builder。
	# 如果不存在，则创建一个支持 linux/arm64 与 linux/amd64 双平台的 builder，
	# 然后切换到 kuscia builder。
	if [ -z "$$(docker buildx inspect kuscia 2>/dev/null)" ]; then \
		echo "create kuscia builder"; \
		docker buildx create --name kuscia --platform linux/arm64,linux/amd64; \
	fi; \
	docker buildx use kuscia
endef

# ====================================================================================================
# 目标定义区（##@ Image 表示这些目标属于 "Image" 分类，make help 会按此分组显示）
# ====================================================================================================

##@ Image

# ====================================================================================================
# 目标: proot
# 用途: 构建 proot 工具镜像
# ====================================================================================================

.PHONY: proot
# 设置 GOARCH 环境变量，与 ARCH 保持一致
# 注释说明: 可以通过 export ARCH=amd64 或 export ARCH=arm64 指定架构
# 如果不指定，ARCH 会自动检测（通过 common.mk 中的逻辑）
proot: export GOARCH=${ARCH}
proot:
	# DOCKER_BUILDKIT=1: 启用 Docker 的 BuildKit 构建引擎（更快、功能更强大）
	DOCKER_BUILDKIT=1
	# @$(call start_docker_buildx): 调用上面定义的宏，确保 buildx builder 就绪
	# @ 表示静默执行，不显示命令本身
	@$(call start_docker_buildx)
	# docker buildx build: 使用 buildx 构建镜像
	#   -t ${PROOT_IMAGE}: 打标签为 PROOT_IMAGE 变量指定的地址
	#   -f ./build/dockerfile/proot-build.Dockerfile: 指定 Dockerfile 路径
	#   .: 构建上下文（当前目录）
	#   --platform linux/${ARCH}: 指定目标平台（amd64 或 arm64）
	#   --load: 将构建结果加载到本地 Docker 守护进程（而不是推送到仓库）
	docker buildx build -t ${PROOT_IMAGE} -f ./build/dockerfile/proot-build.Dockerfile . --platform linux/${ARCH} --load

# ====================================================================================================
# 目标: deps-image
# 用途: 构建 Kuscia 依赖基础镜像
# ====================================================================================================

.PHONY: deps-image
deps-image:
	# 启用 BuildKit
	DOCKER_BUILDKIT=1
	# 确保 buildx builder 就绪
	@$(call start_docker_buildx)
	# 构建依赖镜像，使用 kuscia-deps.Dockerfile
	# 这个镜像通常包含所有编译依赖，体积较大但构建主镜像时复用
	docker buildx build -t ${DEPS_IMAGE} -f ./build/dockerfile/base/kuscia-deps.Dockerfile . --platform linux/${ARCH} --load

# ====================================================================================================
# 目标: image（主目标）
# 用途: 构建 Kuscia 主 Docker 镜像
# ====================================================================================================

.PHONY: image
# 设置 GOARCH 与 ARCH 一致（影响 Go 编译的架构）
image: export GOARCH=${ARCH}
# 目标注释: make help 会显示 "Build docker image with the manager."
image: ## Build docker image with the manager.
# 依赖: 先执行 build 目标（编译 Go 二进制文件）
# 这意味着在构建 Docker 镜像之前，会先编译好所有可执行文件
image: build
image:
	# 启用 BuildKit
	DOCKER_BUILDKIT=1
	# 确保 buildx builder 就绪
	@$(call start_docker_buildx)
	# 构建主镜像，关键参数说明:
	#   --build-arg KUSCIA_ENVOY_IMAGE=${ENVOY_IMAGE}: 将 Envoy 镜像地址作为构建参数传入 Dockerfile
	#   --build-arg DEPS_IMAGE=${DEPS_IMAGE}: 将依赖镜像地址作为构建参数传入 Dockerfile
	#   Dockerfile 中可以通过 ARG 指令接收这些参数，用于 FROM 基础镜像
	#   -f ./build/dockerfile/kuscia-anolis.Dockerfile: 使用基于 Anolis OS 的 Dockerfile
	docker buildx build -t ${IMG} --build-arg KUSCIA_ENVOY_IMAGE=${ENVOY_IMAGE} --build-arg DEPS_IMAGE=${DEPS_IMAGE} -f ./build/dockerfile/kuscia-anolis.Dockerfile . --platform linux/${ARCH} --load

# ====================================================================================================
# 目标: build-monitor
# 用途: 构建 Kuscia 监控组件镜像
# ====================================================================================================

.PHONY: build-monitor
build-monitor:
	# 使用普通 docker build（非 buildx），可能用于本地快速构建
	# 标签固定为 secretflow/kuscia-monitor
	# 使用 kuscia-monitor.Dockerfile
	docker build -t secretflow/kuscia-monitor -f ./build/dockerfile/kuscia-monitor.Dockerfile .