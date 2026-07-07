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
# Dockerfile：构建 Kuscia 监控组件镜像
#
# 说明：
#   该镜像集成 Prometheus 与 Grafana，用于采集与展示 Kuscia 节点的监控指标。
#   通过多阶段构建，把 Prometheus/Grafana 官方镜像中的二进制、配置与数据目录
#   复制到同一基于 Anolis OS 的镜像中，并预置数据源、Dashboard 与启动脚本。
#
# 容器启动后会执行 /home/init_kuscia_monitor.sh，由该脚本根据环境变量完成
# Prometheus 与 Grafana 的最终配置并拉起相关进程。
# ====================================================================================================

# ----------------------------------------------------------------------------------------------------
# 构建参数
# ----------------------------------------------------------------------------------------------------

# 监控组件在镜像中的根目录
ARG ROOT_DIR="/home"

# Prometheus 官方镜像，仅用于复制 prometheus 二进制
ARG PROM_IMAGE="prom/prometheus:v2.45.3"

# Grafana 官方镜像，仅用于复制 Grafana 安装文件与默认数据
ARG GRAFANA_IMAGE="grafana/grafana:10.3.1"

# ----------------------------------------------------------------------------------------------------
# 多阶段构建：阶段 1 —— 提取 Prometheus 二进制
# ----------------------------------------------------------------------------------------------------
FROM ${PROM_IMAGE} AS prom

# ----------------------------------------------------------------------------------------------------
# 多阶段构建：阶段 2 —— 提取 Grafana 安装目录
# ----------------------------------------------------------------------------------------------------
FROM ${GRAFANA_IMAGE} AS grafana

# ----------------------------------------------------------------------------------------------------
# 多阶段构建：阶段 3 —— 组装最终监控镜像
# ----------------------------------------------------------------------------------------------------
FROM openanolis/anolisos:8.8

# 设置时区为上海
ENV TZ=Asia/Shanghai

# 安装 JSON 处理工具 jq，清理 yum 缓存以减小镜像体积
RUN yum install -y jq &&  yum clean all

# 从 Prometheus 阶段复制 prometheus 可执行文件到 ${ROOT_DIR}/bin
COPY --from=prom /bin/prometheus ${ROOT_DIR}/bin

# 从 Grafana 阶段复制 Grafana 的程序、配置、数据与日志目录。
# 注意：其中数据目录路径沿用原 Dockerfile 中的 /var/lig/grafana/，
#      如需修正请同步调整 init_kuscia_monitor.sh 等脚本中的对应引用。
COPY --from=grafana /usr/share/grafana/ /usr/share/grafana/
COPY --from=grafana /etc/grafana /etc/grafana/
COPY --from=grafana /var/lib/grafana /var/lig/grafana/
COPY --from=grafana /var/log/grafana /var/log/grafana/

# 创建配置目录与 Grafana Dashboard 目录
RUN mkdir -p /home/config
RUN mkdir -p /var/lib/grafana/dashboards/

# 预置 Grafana 数据源配置（指向 Prometheus）
COPY scripts/templates/kuscia-monitor-datasource.yaml /etc/grafana/provisioning/datasources/

# 预置机器监控 Dashboard JSON
COPY scripts/templates/grafana-dashboard-machine.json /var/lib/grafana/dashboards/machine.json

# 复制监控组件初始化脚本
COPY scripts/deploy/init_kuscia_monitor.sh /home

# 将 Prometheus 与 Grafana 的二进制目录加入 PATH，便于直接调用
ENV PATH="${PATH}:${ROOT_DIR}/bin:/bin/aux:/usr/share/grafana/bin"

# 设置默认工作目录
WORKDIR ${ROOT_DIR}

# 赋予初始化脚本可执行权限
RUN chmod +x /home/init_kuscia_monitor.sh

# 容器默认执行初始化脚本；ENTRYPOINT 使用 /bin/bash，CMD 作为参数传入，
# 这样可以在启动时通过覆盖 CMD 方便地进行调试。
CMD ["/home/init_kuscia_monitor.sh"]
ENTRYPOINT ["/bin/bash", "--"]
