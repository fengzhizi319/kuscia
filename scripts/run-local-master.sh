#!/bin/bash
#
# 本地运行 Kuscia Master（不依赖 Docker 容器）
#
# 用法：
#   bash scripts/run-local-master.sh [KUSCIA_HOME_DIR]
#
# 说明：
#   1. 需要 root 权限（或能 sudo），因为 Kuscia 内部的 CoreDNS 会监听 53 端口。
#   2. 默认 KUSCIA_HOME 为当前目录下的 .local-kuscia。
#   3. 首次运行会从 KUSCIA_IMAGE 镜像中提取 bin/、etc/、crds/ 等依赖，
#      并替换成本地编译的 kuscia 二进制。
#   4. CoreDNS 默认监听 127.0.0.1:53，以避免与宿主机 systemd-resolved/WSL DNS 冲突。
#   5. 脚本会先以普通用户身份构建 kuscia 二进制，然后通过 sudo 启动 master。

set -e

KUSCIA_HOME="${1:-${KUSCIA_HOME:-$(pwd)/.local-kuscia}}"
KUSCIA_IMAGE="${KUSCIA_IMAGE:-secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0}"
DOMAIN="${DOMAIN:-kuscia-master}"
PROTOCOL="${PROTOCOL:-NOTLS}"

# 如果当前不是 root，先构建二进制，然后使用 sudo 重新执行本脚本
if [ "$(id -u)" -ne 0 ]; then
  echo "==> 当前用户非 root，先以普通用户构建 kuscia 二进制..."
  bash hack/build.sh -t kuscia

  echo "==> 使用 sudo 启动本地 Kuscia master..."
  exec sudo KUSCIA_HOME="${KUSCIA_HOME}" KUSCIA_IMAGE="${KUSCIA_IMAGE}" DOMAIN="${DOMAIN}" PROTOCOL="${PROTOCOL}" PATH="${PATH}" bash "$0" "${KUSCIA_HOME}"
fi

# 以下是 root 权限执行的逻辑
echo "==> KUSCIA_HOME=${KUSCIA_HOME}"
echo "==> KUSCIA_IMAGE=${KUSCIA_IMAGE}"

# 1. 准备依赖目录
echo "==> 准备本地依赖目录..."
mkdir -p "${KUSCIA_HOME}"

if [ ! -f "${KUSCIA_HOME}/bin/k3s" ]; then
  echo "==> 从镜像 ${KUSCIA_IMAGE} 提取依赖..."
  docker run --rm -v "${KUSCIA_HOME}:/out" --user "$(id -u):$(id -g)" "${KUSCIA_IMAGE}" bash -c '
    cp -a /home/kuscia/bin /home/kuscia/crds /home/kuscia/etc /home/kuscia/scripts /home/kuscia/pause /out/
    mkdir -p /out/var/storage/data /out/var/logs /out/var/certs /out/var/tmp /out/var/stdout
  '
fi

# 2. 替换成本地编译的 kuscia 二进制
cp -f build/apps/kuscia/kuscia "${KUSCIA_HOME}/bin/kuscia"

# 3. 将配置模板中的 /home/kuscia 替换为实际 KUSCIA_HOME
find "${KUSCIA_HOME}/etc" -type f -exec sed -i "s|/home/kuscia|${KUSCIA_HOME}|g" {} +

# 4. 限制 CoreDNS 只监听 127.0.0.1:53，避免与 systemd-resolved / WSL DNS 冲突
COREFILE="${KUSCIA_HOME}/etc/conf/corefile"
if [ -f "${COREFILE}" ] && ! grep -q "bind 127.0.0.1" "${COREFILE}"; then
  echo "==> 修改 CoreDNS 监听地址为 127.0.0.1:53..."
  sed -i 's/^\.:53 {/.:53 {\n    bind 127.0.0.1/' "${COREFILE}"
fi

# 5. 检查 53 端口是否被其他进程占用
if ss -tlnp 2>/dev/null | grep -q '127.0.0.1:53\|0.0.0.0:53 '; then
  echo "⚠️  警告：53 端口已被占用。常见占用进程："
  echo "    - systemd-resolved（可执行：systemctl stop systemd-resolved）"
  echo "    - WSL 虚拟网卡 DNS（通常监听 10.255.255.254:53，可忽略）"
  echo "    如果启动后继续报错 'bind: address already in use'，请先释放 127.0.0.1:53。"
fi

# 6. 生成 master 配置文件
CONFIG_FILE="${KUSCIA_HOME}/etc/conf/kuscia.yaml"
echo "==> 生成 master 配置文件: ${CONFIG_FILE}"
KUSCIA_HOME="${KUSCIA_HOME}" "${KUSCIA_HOME}/bin/kuscia" init \
  --mode master \
  --domain "${DOMAIN}" \
  --protocol "${PROTOCOL}" > "${CONFIG_FILE}"

# 7. 启动 master
echo "==> 启动本地 Kuscia master..."
echo "    日志目录: ${KUSCIA_HOME}/var/logs"
echo ""
KUSCIA_HOME="${KUSCIA_HOME}" "${KUSCIA_HOME}/bin/kuscia" start --config "${CONFIG_FILE}"
