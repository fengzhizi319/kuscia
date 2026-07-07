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
# Dockerfile：静态编译 PRoot 二进制
#
# 说明：
#   PRoot 是一个用户态的 chroot / root 模拟工具，Kuscia 使用它在不依赖 root 权限的情况下
#   为任务容器提供隔离执行环境。本镜像基于 GCC 7.4.0 编译环境，从 PRoot 官方仓库克隆源码，
#   应用 Kuscia 所需的补丁后，静态编译出 proot 可执行文件。
#
# 使用方式：
#   docker build -t kuscia-proot-builder -f build/dockerfile/proot-build.Dockerfile .
#   # 如需取出编译产物，可创建临时容器复制 /root/proot/src/proot
# ====================================================================================================

# 使用带有完整 GCC 工具链的镜像作为构建环境，便于静态编译 C 项目
FROM secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/gcc:7.4.0

# 从 PRoot 官方 GitHub 仓库克隆源码到 /root/proot
RUN git clone https://github.com/proot-me/proot.git /root/proot

# 将本地补丁目录复制到镜像中。
# 补丁位于 kuscia/hack/proot/patch/，用于修复 PRoot 在 Kuscia 场景下遇到的特定路径处理问题。
COPY hack/proot/patch /tmp

# 进入源码目录，依次执行：
#   1. git apply /tmp/*.patch  —— 应用所有本地补丁
#   2. LDFLAGS="${LDFLAGS} -static" make -C src proot
#      以静态链接方式编译 src/proot，确保生成的二进制不依赖容器内的动态库，
#      方便后续复制到更精简的运行镜像中直接使用。
RUN cd /root/proot && \
    git apply /tmp/*.patch && \
    LDFLAGS="${LDFLAGS} -static" make -C src proot
