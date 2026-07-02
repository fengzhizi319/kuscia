#!/bin/bash
#
# 本地运行 Kuscia Master（不依赖 Docker 容器）
#
# 用法：
#   bash scripts/run-local-master.sh [KUSCIA_HOME_DIR]
#
# 说明：
#   1. 需要当前用户有 root 权限（或能 sudo），因为 Kuscia 内部的 CoreDNS 会监听 53 端口。
#   2. 默认 KUSCIA_HOME 为当前目录下的 .local-kuscia。
#   3. 首次运行会从 KUSCIA_IMAGE 镜像中提取 bin/、etc/、crds/ 等依赖，并替换成本地编译的 kuscia 二进制。

set -e

KUSCIA_HOME="${1:-${KUSCIA_HOME:-$(pwd)/.local-kuscia}}"
KUSCIA_IMAGE="${KUSCIA_IMAGE:-secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0}"
DOMAIN="${DOMAIN:-kuscia-master}"
PROTOCOL="${PROTOCOL:-NOTLS}"

if [ "$(id -u)" -ne 0 ]; then
  echo "⚠️  警告：Kuscia 本地运行需要绑定 53/80 等特权端口，建议以 root 用户执行本脚本。"
  echo "   当前用户 UID=$(id -u)，如后续出现 'permission denied'，请切换为 root 或使用 sudo。"
fi

echo "==> KUSCIA_HOME=${KUSCIA_HOME}"
echo "==> KUSCIA_IMAGE=${KUSCIA_IMAGE}"

# 1. 构建本地 kuscia 二进制
echo "==> 构建本地 kuscia 二进制..."
bash hack/build.sh -t kuscia

# 2. 准备依赖目录
echo "==> 准备本地依赖目录..."
mkdir -p "${KUSCIA_HOME}"

if [ ! -f "${KUSCIA_HOME}/bin/k3s" ]; then
  echo "==> 从镜像 ${KUSCIA_IMAGE} 提取依赖..."
  docker run --rm -v "${KUSCIA_HOME}:/out" --user "$(id -u):$(id -g)" "${KUSCIA_IMAGE}" bash -c '
    cp -a /home/kuscia/bin /home/kuscia/crds /home/kuscia/etc /home/kuscia/scripts /home/kuscia/pause /out/
    mkdir -p /out/var/storage/data /out/var/logs /out/var/certs /out/var/tmp /out/var/stdout
  '
fi

# 3. 替换成本地编译的 kuscia 二进制
cp -f build/apps/kuscia/kuscia "${KUSCIA_HOME}/bin/kuscia"

# 4. 将配置模板中的 /home/kuscia 替换为实际 KUSCIA_HOME
find "${KUSCIA_HOME}/etc" -type f -exec sed -i "s|/home/kuscia|${KUSCIA_HOME}|g" {} +

# 5. 生成 master 配置文件
CONFIG_FILE="${KUSCIA_HOME}/etc/conf/kuscia.yaml"
echo "==> 生成 master 配置文件: ${CONFIG_FILE}"
KUSCIA_HOME="${KUSCIA_HOME}" "${KUSCIA_HOME}/bin/kuscia" init \
  --mode master \
  --domain "${DOMAIN}" \
  --protocol "${PROTOCOL}" > "${CONFIG_FILE}"

# 6. 启动 master
echo "==> 启动本地 Kuscia master..."
echo "    日志目录: ${KUSCIA_HOME}/var/logs"
echo ""
KUSCIA_HOME="${KUSCIA_HOME}" "${KUSCIA_HOME}/bin/kuscia" start --config "${CONFIG_FILE}"
