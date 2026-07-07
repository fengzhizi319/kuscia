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
# Dockerfile：构建集成 SecretFlow-lite 的 Kuscia 镜像
#
# 说明：
#   该镜像以 Kuscia 镜像为基础，把 SecretFlow-lite 运行所需的 Python 环境、
#   系统依赖以及 SecretFlow-lite 本身的算法镜像“预置”到 Kuscia 的本地镜像仓库中，
#   最终产出的是一个“Kuscia + SecretFlow-lite”一体化镜像。
#
#   典型使用场景：
#     - 作为 Kuscia 节点运行，同时能直接调度 SecretFlow-lite 任务。
#     - 避免每次启动节点时再去公网拉取 SecretFlow-lite 算法镜像。
#
# 构建示例：
#   docker build \
#       --build-arg KUSCIA_IMAGE=secretflow/kuscia:latest \
#       --build-arg PYTHON_IMAGE=secretflow/anolis8-python:3.10.13 \
#       --build-arg SF_VERSION=1.11.0b1 \
#       -f kuscia-secretflow.Dockerfile \
#       -t secretflow/kuscia-secretflow:1.11.0b1 .
# ====================================================================================================
#不同命令的执行位置，宿主机内还是镜像内
#┌─────────────────────────────────────────────────────────┐
#│                    镜像构建阶段                            │
#│  (docker build)                                         │
#│                                                         │
#│  FROM → 拉取镜像              [宿主机]                     │
#│  RUN  → 在临时容器内执行命令     [镜像内]                    │
#│  COPY → 宿主机文件 → 镜像层     [镜像内]                    │
#│  ENV  → 记录环境变量           [镜像元数据]                 │
#│  CMD  → 记录启动命令           [镜像元数据，此时不执行]        │
#└─────────────────────────────────────────────────────────┘
#                            ↓
#┌─────────────────────────────────────────────────────────┐
#│                    容器运行阶段                           │
#│  (docker run)                                           │
#│                                                         │
#│  CMD/ENTRYPOINT → 实际执行命令  [容器内，此时才运行]          │
#└─────────────────────────────────────────────────────────┘
# ----------------------------------------------------------------------------------------------------
# 构建参数（Build Arguments）
# ----------------------------------------------------------------------------------------------------

# Kuscia 基础镜像，提供 Kuscia 运行时环境、/home/kuscia 目录结构、
# 以及 `kuscia` 命令行工具等。
ARG KUSCIA_IMAGE="secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:latest"

# SecretFlow Python 环境源镜像，仅用于从中复制 Python 解释器、
# 已安装的 site-packages 及动态链接库到最终镜像中。
ARG PYTHON_IMAGE="secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/anolis8-python:3.10.13"

# ----------------------------------------------------------------------------------------------------
# 多阶段构建：阶段 1 —— 提取 Python 环境
# ----------------------------------------------------------------------------------------------------
# 只把 ${PYTHON_IMAGE} 中的 /root/miniconda3/envs/secretflow 目录作为来源，
# 后续会COPY到Kuscia镜像中。这样可以避免把conda安装包等中间产物带入最终镜像。
FROM ${PYTHON_IMAGE} AS python

# ----------------------------------------------------------------------------------------------------
# 多阶段构建：阶段 2 —— 以 Kuscia 镜像为基础，集成 SecretFlow-lite
# ----------------------------------------------------------------------------------------------------
FROM ${KUSCIA_IMAGE}

# 将阶段 1 中的 Python 可执行文件与库复制到最终镜像的 /usr/local 下，
# 使其成为系统默认的 Python 环境。
#   /usr/local/bin/   -> python3、pip3 及各种入口脚本
#   /usr/local/lib/   -> Python 标准库与第三方 site-packages
COPY --from=python /root/miniconda3/envs/secretflow/bin/ /usr/local/bin/
COPY --from=python /root/miniconda3/envs/secretflow/lib/ /usr/local/lib/

# 安装 SecretFlow-lite 运行时所需的系统依赖，并修正从 miniconda 环境复制过来的脚本 shebang。
# Anolis OS 是龙蜥操作系统，属于 RHEL/CentOS 兼容系，所以要用 yum 安装依赖。不能用 apt-get 安装依赖。
# 步骤说明：
#   1. protobuf    : SecretFlow 序列化/反序列化依赖
#   2. libnl3      : 网络相关依赖
#   3. libgomp     : OpenMP 运行时库，部分数值计算库需要
#   4. grep + sed  : 把 /usr/local/bin 下脚本中指向 /root/miniconda3/envs/secretflow 的 shebang
#                   统一替换为 /usr/local，确保脚本在新路径下能正确执行
#   5. rm openssl  : 删除从 Python 镜像带入的 openssl 可执行文件，避免覆盖 Kuscia 镜像自带的 openssl，
#                   防止 TLS 证书路径、版本不一致导致的运行时问题
RUN yum install -y protobuf libnl3 libgomp && \
    yum clean all && \
    grep -rl '#!/root/miniconda3/envs/secretflow/bin' /usr/local/bin/ | xargs sed -i -e 's/#!\/root\/miniconda3\/envs\/secretflow/#!\/usr\/local/g' && \
    rm /usr/local/bin/openssl

# SecretFlow-lite 的版本号，可通过 --build-arg SF_VERSION=xxx 覆盖
ARG SF_VERSION="1.11.0b1"

# 安装 SecretFlow-lite Python 包，并把它对应的算法镜像注册到 Kuscia 本地镜像仓库中：
#   1. pip install secretflow-lite==${SF_VERSION}
#      使用阿里云 PyPI 镜像加速安装，安装后清理 pip 缓存以减小镜像体积。
#   2. kuscia image --store /home/kuscia/var/images --runtime runp builtin <image>
#      将 SecretFlow-lite 算法镜像标记为 Kuscia runp 运行时的内置镜像，
#      这样节点启动后无需再从远端拉取即可直接运行 SecretFlow-lite 任务。
#   3. 这里执行两次 `kuscia image builtin`，分别使用短名称和完整仓库路径，
#      兼容 Kuscia 内部可能以不同格式引用同一镜像的场景。
RUN pip install secretflow-lite==${SF_VERSION} --extra-index-url https://mirrors.aliyun.com/pypi/simple/ && rm -rf /root/.cache && \
    kuscia image --store /home/kuscia/var/images --runtime runp builtin secretflow/secretflow-lite-anolis8:${SF_VERSION} && \
    kuscia image --store /home/kuscia/var/images --runtime runp builtin secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite-anolis8:${SF_VERSION}

# 设置默认工作目录为 Kuscia 主目录
WORKDIR /home/kuscia

# 使用 tini 作为 init 进程，负责回收僵尸进程并正确转发信号给业务进程
ENTRYPOINT ["tini", "--"]
