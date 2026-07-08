#
# Copyright 2025 Ant Group Co., Ltd.
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
# Dockerfile：基于本地 SecretFlow 源码 wheel，构建 Kuscia + SecretFlow 开发一体化镜像
#
# 说明：
#   官方的 kuscia-secretflow.Dockerfile 会把 PyPI 上已发布的 secretflow-lite 包安装进 Kuscia
#   镜像。本文件用于“二次开发”场景：把本地源码构建出来的 SecretFlow dev wheel 安装到 Kuscia
#   镜像中，使节点启动后即可用 runp 模式运行本地 SecretFlow 任务。
#
# 前置条件：
#   1. 已经构建好 Kuscia 镜像（例如 secretflow/kuscia:v1.15.0-dev-20260708120000）
#   2. 已经构建好 SecretFlow dev wheel（例如 secretflow-1.15.0.dev20260708-...-linux_x86_64.whl）
#   3. 把 wheel 文件放到本 Dockerfile 同目录下，构建时通过 --build-arg SF_WHEEL 指定文件名
#
# 构建示例：
#   cd /home/charles/code/sfwork/kuscia
#   cp ../secretflow/docker/dev/secretflow-1.15.0.dev20260708-...-linux_x86_64.whl build/dockerfile/
#   docker build \
#       --build-arg KUSCIA_IMAGE=secretflow/kuscia:v1.15.0-dev-20260708120000 \
#       --build-arg SF_WHEEL=secretflow-1.15.0.dev20260708-...-linux_x86_64.whl \
#       --build-arg SF_VERSION=1.15.0-dev \
#       -f build/dockerfile/kuscia-secretflow-dev.Dockerfile \
#       -t secretflow/kuscia-secretflow-dev:1.15.0-dev \
#       build/dockerfile/
# ====================================================================================================

# ----------------------------------------------------------------------------------------------------
# 构建参数
# ----------------------------------------------------------------------------------------------------
# Kuscia 基础镜像，必须已经存在于本地 Docker
ARG KUSCIA_IMAGE="secretflow/kuscia:latest"

# 提供 Python 3.10 环境的 Anolis 镜像，用于安装本地 wheel。
# 与官方 kuscia-secretflow.Dockerfile 保持一致，确保 glibc 等系统库与 Kuscia 镜像兼容。
ARG PYTHON_IMAGE="secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/anolis8-python:3.10.13"

# 本地 SecretFlow wheel 文件名，需要放在 build context 中
ARG SF_WHEEL="secretflow-1.15.0.dev20260708-py3-none-any.whl"

# 注册到 Kuscia 本地镜像仓库时的镜像 tag
ARG SF_VERSION="1.15.0-dev"

# ----------------------------------------------------------------------------------------------------
# 多阶段构建：阶段 1 —— 在 Anolis Python 基础镜像里安装本地 SecretFlow wheel
# ----------------------------------------------------------------------------------------------------
FROM ${PYTHON_IMAGE} AS python-env

ARG SF_WHEEL
ARG SF_VERSION

# 把 wheel 复制到镜像内固定路径
COPY ${SF_WHEEL} /tmp/secretflow.whl

# 安装本地 wheel 及 kuscia 包（runp 模式调度时需要）。
# 使用阿里云 PyPI 镜像加速依赖下载，安装后清理 pip 缓存以减小体积。
RUN pip install -i https://mirrors.aliyun.com/pypi/simple/ /tmp/secretflow.whl kuscia && \
    rm -rf /root/.cache

# ----------------------------------------------------------------------------------------------------
# 多阶段构建：阶段 2 —— 以 Kuscia 镜像为基础，集成已安装 SecretFlow 的 Python 环境
# ----------------------------------------------------------------------------------------------------
FROM ${KUSCIA_IMAGE}

# 将阶段 1 的 miniconda env 复制到最终镜像，并作为系统默认 Python 环境。
#   /usr/local/bin/ -> python3、pip3 及各种入口脚本
#   /usr/local/lib/ -> Python 标准库与第三方 site-packages
COPY --from=python-env /root/miniconda3/envs/secretflow/bin/ /usr/local/bin/
COPY --from=python-env /root/miniconda3/envs/secretflow/lib/ /usr/local/lib/

# 安装 SecretFlow 运行时所需的系统依赖，并修正从 miniconda 环境复制过来的脚本 shebang。
# Anolis OS 属于 RHEL/CentOS 兼容系，所以用 yum 安装。
#   1. protobuf : SecretFlow 序列化/反序列化依赖
#   2. libnl3   : 网络相关依赖
#   3. libgomp  : OpenMP 运行时库
#   4. grep+sed : 把 /usr/local/bin 下脚本中指向 /root/miniconda3/envs/secretflow 的 shebang
#                替换为 /usr/local，确保脚本在新路径下能正确执行
#   5. rm openssl : 删除 Python 镜像带入的 openssl 可执行文件，避免覆盖 Kuscia 自带的 openssl
RUN yum install -y protobuf libnl3 libgomp && \
    yum clean all && \
    grep -rl '#!/root/miniconda3/envs/secretflow/bin' /usr/local/bin/ | xargs sed -i -e 's/#!\/root\/miniconda3\/envs\/secretflow/#!\/usr\/local/g' && \
    rm -f /usr/local/bin/openssl

# 在 Kuscia 本地镜像仓库中注册 SecretFlow 算法镜像，runp 运行时可直接引用。
# 这里只是写入一条镜像记录，不需要对应 OCI 镜像真实存在。
ARG SF_VERSION
RUN kuscia image --store /home/kuscia/var/images --runtime runp builtin secretflow/sf-dev-anolis8:${SF_VERSION}

WORKDIR /home/kuscia

ENTRYPOINT ["tini", "--"]
