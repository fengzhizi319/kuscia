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
# Dockerfile：构建 Kuscia 主运行镜像（基于 Anolis OS）
#
# 说明：
#   本镜像以 Anolis OS 为基础，集成 Kuscia 运行时所需的二进制、依赖工具、Envoy 网关、
#   node_exporter 监控组件以及 Kuscia 自身的配置、脚本与测试数据，最终生成可直接作为
#   Kuscia Master / Lite / Autonomy 节点运行的镜像。
#
# 多阶段构建说明：
#   - deps:            从 kuscia-deps 镜像复制预编译的工具链与依赖二进制
#   - node_exporter:   从 Prometheus node-exporter 镜像复制监控采集器
#   - kuscia_envoy:    从 Kuscia Envoy 镜像复制网关代理二进制
#   - 最终镜像:        基于 Anolis OS 汇总上述产物与源码编译出的 kuscia 二进制
# ====================================================================================================

# ----------------------------------------------------------------------------------------------------
# 构建参数：均可通过 docker build --build-arg <KEY>=<VALUE> 覆盖
# ----------------------------------------------------------------------------------------------------

# Kuscia 依赖镜像，包含 k3s、crictl、kubectl、cni 插件等预编译工具
ARG DEPS_IMAGE="secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-deps:0.7.0b0"

# Kuscia Envoy 网关镜像
ARG KUSCIA_ENVOY_IMAGE="secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-envoy:0.6.2b0"

# Prometheus node-exporter 镜像，用于节点监控指标采集
ARG PROM_NODE_EXPORTER="secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/node-exporter:v1.9.1"

# 最终基础系统镜像：Anolis OS 23
ARG BASE_IMAGE="secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/anolisos:23"

# ----------------------------------------------------------------------------------------------------
# 多阶段构建：阶段 1 —— 提取 deps 工具链
# ----------------------------------------------------------------------------------------------------
FROM ${DEPS_IMAGE} AS deps

# ----------------------------------------------------------------------------------------------------
# 多阶段构建：阶段 2 —— 提取 node_exporter
# ----------------------------------------------------------------------------------------------------
FROM ${PROM_NODE_EXPORTER} AS node_exporter

# ----------------------------------------------------------------------------------------------------
# 多阶段构建：阶段 3 —— 提取 Envoy
# ----------------------------------------------------------------------------------------------------
FROM ${KUSCIA_ENVOY_IMAGE} AS kuscia_envoy

# ----------------------------------------------------------------------------------------------------
# 多阶段构建：阶段 4 —— 组装最终 Kuscia 镜像
# ----------------------------------------------------------------------------------------------------
FROM ${BASE_IMAGE}

# 设置时区为上海，保证容器内日志与调度时间符合国内习惯
ENV TZ=Asia/Shanghai

# Docker buildx 会自动注入的目标平台与架构变量，用于定位构建产物
ARG TARGETPLATFORM
ARG TARGETARCH

# Kuscia 主目录，后续所有目录、权限、环境变量均围绕此处展开
ARG HOME_DIR="/home/kuscia"
ENV HOME=${HOME_DIR}

# 安装运行时所需的系统工具并创建 Kuscia 目录结构：
#   - openssl:     TLS/证书相关操作
#   - net-tools:   ifconfig/netstat 等网络调试工具
#   - which:       查找命令路径
#   - jq:          JSON 处理
#   - logrotate:   日志轮转
#   - iproute:     ip/ss 等现代网络工具
#   - procps-ng:   ps/top 等进程工具
#   - libcap:      setcap 能力管理
#   - gzip:        压缩/解压
RUN yum install -y openssl net-tools which jq logrotate iproute procps-ng libcap gzip && \
    yum clean all && \
    mkdir -p ${HOME_DIR}/bin && \
    mkdir -p /bin/aux && \
    mkdir -p ${HOME_DIR}/scripts && \
    mkdir -p ${HOME_DIR}/etc/conf && \
    mkdir -p ${HOME_DIR}/etc/cni && \
    mkdir -p ${HOME_DIR}/crds && \
    mkdir -p ${HOME_DIR}/var/storage/data && \
    mkdir -p ${HOME_DIR}/var/k3s/server/db && \
    mkdir -p ${HOME_DIR}/var/images && \
    mkdir -p ${HOME_DIR}/pause

# 创建非 root 用户 kuscia 与同名用户组，并正确设置 /home/kuscia 的属主与权限。
# g+rwxs 中的 s 位确保后续在目录下新建的文件继承 kuscia 组，便于多进程协作。
RUN useradd -ms /bin/bash kuscia && \
    usermod -aG kuscia kuscia && \
    chown -R kuscia:kuscia /home/kuscia && \
    chgrp kuscia /home/kuscia && \
    chmod -R g+rwxs /home/kuscia

# 从 deps 镜像复制预编译工具链到 ${HOME_DIR}/bin
COPY --chown=kuscia:kuscia --from=deps /image/home/kuscia/bin ${HOME_DIR}/bin

# 从 node_exporter 镜像复制监控采集器到 ${HOME_DIR}/bin
COPY --chown=kuscia:kuscia --from=node_exporter /bin/node_exporter ${HOME_DIR}/bin

# 创建常用命令的符号链接，统一调用方式：
#   - k3s 同时承担 crictl、kubectl 的角色
#   - cni 插件统一以 bridge/flannel/host-local/loopback/portmap 等名称暴露
RUN pushd ${HOME_DIR}/bin && \
    ln -s k3s crictl && \
    ln -s k3s kubectl && \
    ln -s cni bridge && \
    ln -s cni flannel && \
    ln -s cni host-local && \
    ln -s cni loopback && \
    ln -s cni portmap && \
    popd

# 复制源码构建产物与配置文件到最终镜像（均以 kuscia 用户身份）
COPY --chown=kuscia:kuscia build/${TARGETPLATFORM}/apps/kuscia/kuscia ${HOME_DIR}/bin
COPY --chown=kuscia:kuscia build/pause/pause-${TARGETARCH}.tar ${HOME_DIR}/pause/pause.tar
COPY --chown=kuscia:kuscia crds/v1alpha1 ${HOME_DIR}/crds/v1alpha1
COPY --chown=kuscia:kuscia etc/conf ${HOME_DIR}/etc/conf
COPY --chown=kuscia:kuscia etc/cni ${HOME_DIR}/etc/cni
COPY --chown=kuscia:kuscia tests ${HOME_DIR}/tests
COPY --chown=kuscia:kuscia tests/data ${HOME_DIR}/var/storage/data
COPY --chown=kuscia:kuscia scripts ${HOME_DIR}/scripts
COPY --chown=kuscia:kuscia thirdparty/*/scripts ${HOME_DIR}/scripts

# 从 Envoy 阶段复制网关二进制
COPY --chown=kuscia:kuscia --from=kuscia_envoy /home/kuscia/bin/envoy ${HOME_DIR}/bin

# 将 Kuscia 相关二进制目录加入 PATH，并设置默认工作目录
ENV PATH="${PATH}:${HOME_DIR}/bin:/bin/aux"
WORKDIR ${HOME_DIR}

# 为非 root 用户授予 kuscia 与 envoy 二进制绑定低端口（0, 1024] 的能力，
# 这样 Kuscia 以 kuscia 用户运行时也能监听 80/443 等端口。
RUN setcap cap_net_bind_service=+ep /home/kuscia/bin/kuscia && \
    setcap cap_net_bind_service=+ep /home/kuscia/bin/envoy

# 使用 tini 作为 init 进程，负责回收僵尸进程并转发信号
ENTRYPOINT ["tini", "--"]
