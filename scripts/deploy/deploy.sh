#!/bin/bash
# 上一行是 shebang（也称为 hash-bang），由 #! 开头，后接解释器绝对路径 /bin/bash。
# 它的作用是告诉操作系统：当该脚本被赋予可执行权限并直接运行（例如 ./deploy.sh）时，
# 应该调用 /bin/bash 来解析执行本文件。如果没有 shebang，则只能通过 bash deploy.sh 显式执行。
#
# Copyright 2025 Ant Group Co., Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# ==================== 脚本功能说明 ====================
# 该脚本用于在 Docker 环境中部署 Kuscia 的三种节点角色：
#   - Master: 中央控制节点，负责域名管理、任务调度、证书签发等；
#   - Lite:   受控工作节点，需要注册到 Master，执行实际的隐私计算任务；
#   - Autonomy: 自治节点，既包含控制面也包含数据面，可独立运行，也可与其他 Autonomy 节点互联（P2P/CxC）。
#
# 部署流程概述：
#   1. 检查宿主机架构（仅支持 x86_64 / amd64 / arm64）；
#   2. 解析命令行参数与环境变量，初始化镜像、端口、目录等配置；
#   3. 创建 Docker 网络 kuscia-exchange，供容器间通信；
#   4. 按需拉取 Kuscia 与 SecretFlow 镜像；
#   5. 生成或加载 kuscia.yaml 配置文件，启动 Kuscia 容器；
#   6. 探针检测 k3s、Gateway、DataMesh 等关键组件是否就绪；
#   7. 初始化 KusciaAPI 客户端证书；
#   8. 导入 SecretFlow/PSI 引擎镜像并创建 AppImage；
#   9. 创建示例 DomainData（Alice/Bob 数据表），便于快速体验。
#
# 前置条件：
#   - 已安装 Docker，并且当前用户有权限操作 Docker；
#   - 可访问镜像仓库（默认：secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/）；
#   - 目标端口未被占用（默认 1080/80/8082/8083 及自定义主机端口）。

# 严格模式：遇到错误立即退出，防止错误继续传播。
# set -e 是 Bash 的一个选项（option）。启用后，如果脚本中任何一条命令返回非 0 退出状态（即失败），
# shell 会立即终止整个脚本的执行。这样可以在第一个出错点就停下来，避免后续命令在错误状态下继续运行，
# 从而防止产生难以排查的连锁错误。注意：set -e 对某些复合命令、条件判断中的失败不生效。
set -e

# 终端颜色定义，用于输出高亮日志。
# 这里是三个 ANSI 转义序列，分别表示绿色（GREEN）、恢复默认颜色（NC，No Color）、红色（RED）。
# \033 是 ESC 字符的八进制表示；[0;32m 设置前景色为绿色，[0m 重置所有属性，[31m 设置前景色为红色。
# 使用 echo -e 时，-e 选项会解释这些转义字符，使其在支持的终端上显示为彩色文本。
GREEN='\033[0;32m'
NC='\033[0m'
RED='\033[31m'

# ==================== 通用日志与检查函数 ====================

# 打印绿色高亮日志。
# function 是 Bash 中定义函数的保留字，也可以省略 function 直接写 name() { ... }。
# function log() { ... } 这种写法兼容性更好，可读性也更高。
function log() {
  # local 关键字用于声明局部变量，其作用范围仅限于当前函数内部，不会污染脚本全局作用域。
  # var=$1 表示将调用函数时传入的第一个位置参数赋值给变量 var。
  # Bash 函数参数不使用圆括号声明，而是通过 $1、$2、$3 ... 以及 $@、$*、$# 来访问。
  local log_content=$1
  # echo -e 启用对转义字符的解释，将 GREEN 和 NC 中的 ANSI 码渲染为颜色。
  echo -e "${GREEN}${log_content}${NC}"
}

# 检查宿主机 CPU 架构。
# Kuscia 当前支持 x86_64、amd64 与 arm64（arm64 可能有限制，仅做警告）。
function arch_check() {
  # 声明局部变量 arch，用于保存 uname -a 的输出。
  local arch
  # $(uname -a) 是命令替换（command substitution）的一种现代写法，旧写法是 `uname -a`（反引号）。
  # 它会先执行括号内的命令，然后将命令的标准输出替换到当前位置。
  # uname -a 打印系统内核名称、主机名、内核版本、硬件架构等详细信息。
  arch=$(uname -a)
  # [[ ... ]] 是 Bash 扩展的条件表达式，相比 [ ... ]（test 命令）支持更强大的模式匹配和更安全的语法。
  # 例如 [[ $arch == *"ARM"* ]] 中的 == 右侧可以使用通配符 *（glob），判断 $arch 中是否包含 "ARM" 子串。
  # 使用 [[ ... ]] 时不需要对变量加双引号也能防止单词分割和空值问题，但养成加引号的习惯更安全。
  if [[ $arch == *"ARM"* ]] || [[ $arch == *"aarch64"* ]]; then
    echo "Warning: arm64 architecture. Continuing..."
  elif [[ $arch == *"x86_64"* ]]; then
    # 检测到 x86_64 架构时，使用绿色高亮提示继续执行。
    echo -e "${GREEN}x86_64 architecture. Continuing...${NC}"
  elif [[ $arch == *"amd64"* ]]; then
    echo "Warning: amd64 architecture. Continuing..."
  else
    # 如果是不支持的架构，使用红色高亮输出错误信息，并通过 exit 1 以非 0 状态退出脚本。
    # exit 1 中的 1 是退出码（return code），通常 0 表示成功，非 0 表示失败。
    echo -e "${RED}$arch architecture is not supported by kuscia currently${NC}"
    exit 1
  fi
}

# 检查当前用户是否有权限在指定路径创建目录。
# 用于提前发现数据目录、日志目录不可写的问题，避免后续 Docker 挂载或写日志时失败。
function pre_check() {
  # mkdir -p "$1" 中，-p 选项表示递归创建目录，如果目录已存在也不会报错。
  # "$1" 是对第一个参数加双引号，防止路径中包含空格时被错误拆分（单词分割）。
  # 2>/dev/null 表示将命令的标准错误（文件描述符 2）重定向到 /dev/null，即丢弃错误输出。
  # /dev/null 是一个特殊的空设备文件，写入它的数据都会被丢弃；从这里读取会立即得到 EOF。
  # if ! command; then ... fi 表示如果 command 返回非 0（即创建目录失败），则执行 then 分支。
  # ! 是逻辑非运算符，用于反转命令的退出状态。
  if ! mkdir -p "$1" 2>/dev/null; then
    # 创建目录失败时输出红色错误信息，并退出脚本。
    echo -e "${RED}User does not have access to create the directory: $1${NC}"
    exit 1
  fi
}

# ==================== 全局默认配置 ====================

# 若未通过环境变量指定 Kuscia 镜像，则使用默认镜像。
# ${KUSCIA_IMAGE} 是变量扩展（parameter expansion），花括号用于明确变量名的边界。
# 当变量名紧跟其他字符时，必须使用 ${VAR} 形式；即使不紧跟，加花括号也是推荐写法。
if [[ ${KUSCIA_IMAGE} == "" ]]; then
  # 给 KUSCIA_IMAGE 赋值默认镜像地址，格式为 registry/namespace/image:tag。
  KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:latest
fi
# 输出最终使用的 Kuscia 镜像地址，方便用户核对。
log "KUSCIA_IMAGE=${KUSCIA_IMAGE}"

# 若未通过环境变量指定 SecretFlow 引擎镜像，则使用默认镜像。
# 注意这里的条件判断使用了 "$SECRETFLOW_IMAGE" 加双引号，与上一处 ${KUSCIA_IMAGE} 未加引号效果类似，
# 因为 [[ ... ]] 内部通常不需要担心空值展开问题，但加双引号是更严谨的 Shell 脚本习惯。
if [[ "$SECRETFLOW_IMAGE" == "" ]]; then
  SECRETFLOW_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite-anolis8:1.11.0b1
fi
log "SECRETFLOW_IMAGE=${SECRETFLOW_IMAGE}"

# SecretFlow 镜像相关字段。
# SF_IMAGE_REGISTRY 用于保存镜像仓库前缀（registry/namespace），例如 secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow。
# SF_IMAGE_ID 用于保存导入到 containerd 后的镜像 ID，供后续创建 AppImage 使用。
SF_IMAGE_REGISTRY=""
SF_IMAGE_ID=""

# Kuscia 容器内部根目录与证书路径。
# CTR_ROOT 表示容器内 Kuscia 的安装根目录；CTR_CERT_ROOT 是其下存放证书的目录。
CTR_ROOT=/home/kuscia
CTR_CERT_ROOT=${CTR_ROOT}/var/certs

# 各节点角色的内存限制。
# MASTER_MEMORY_LIMIT 是 Master 容器可使用的最大内存；LITE_MEMORY_LIMIT 是 Lite 节点；AUTONOMY_MEMORY_LIMIT 是 Autonomy 节点。
# 这些值会传递给 docker run 的 -m 选项，Docker 会据此限制容器内存使用。
MASTER_MEMORY_LIMIT=2G
LITE_MEMORY_LIMIT=4G
AUTONOMY_MEMORY_LIMIT=6G

# Docker 网络名称，所有 Kuscia 容器加入同一网络以互相通信。
# 使用自定义 bridge 网络（而非默认 bridge）可以让容器通过容器名或 hostname 互相 DNS 解析。
NETWORK_NAME="kuscia-exchange"

# ==================== SecretFlow 镜像信息解析 ====================

# 从 SECRETFLOW_IMAGE 中解析出镜像标签、仓库、注册中心等信息，
# 供后续导入镜像与生成 AppImage 时使用。
#
# 支持的镜像地址格式：
#   - 无注册中心命名空间：repo/name:tag（1 个 /）
#   - 带注册中心命名空间：registry/namespace/name:tag（2 个 /）
function init_sf_image_info() {
  # [ ... ] 是 test 命令的另一种写法，功能与 [[ ... ]] 类似但能力较弱。
  # 在 [ ... ] 中，变量最好加双引号，否则空值可能导致语法错误。
  # 此处判断 SECRETFLOW_IMAGE 是否非空。
  if [ "$SECRETFLOW_IMAGE" != "" ]; then
    # 提取镜像标签（冒号后的部分）。
    # ${VAR##pattern} 是变量扩展中的"从开头删除最长匹配"操作。
    # pattern 是 *:（任意字符后跟冒号），因此 ${SECRETFLOW_IMAGE##*:} 会删除最后一个冒号及其之前的所有内容，
    # 只保留最后一个冒号后的镜像标签，例如 1.11.0b1。
    SF_IMAGE_TAG=${SECRETFLOW_IMAGE##*:}

    # 统计镜像地址中的 "/" 数量，用于判断格式。
    # $( ... ) 是命令替换；tr -cd "/" 表示删除（-d）除 "/" 以外的所有字符（-c 表示 complement，取反）。
    # wc -c 统计剩余字符数，即 "/" 的个数。
    path_separator_count="$(echo "$SECRETFLOW_IMAGE" | tr -cd "/" | wc -c)"

    # 如果 "/" 数量为 1，说明格式为 repo/name:tag。
    if [ "${path_separator_count}" == 1 ]; then
      # ${VAR//pattern/rep} 是"全局替换"操作，将变量中所有匹配 pattern 的子串替换为 rep。
      # 这里 pattern 是 :${SF_IMAGE_TAG}，rep 为空字符串，因此会去掉整个镜像地址中的 :tag 部分，
      # 得到 SF_IMAGE_NAME，例如 secretflow-lite-anolis8。
      SF_IMAGE_NAME=${SECRETFLOW_IMAGE//:${SF_IMAGE_TAG}/}
    elif [ "$path_separator_count" == 2 ]; then
      # 如果 "/" 数量为 2，说明格式为 registry/namespace/name:tag。
      # cut -d "/" -f 1 表示以 "/" 为分隔符，取第 1 个字段，即 registry 部分。
      registry=$(echo "$SECRETFLOW_IMAGE" | cut -d "/" -f 1)
      # 取第 2 个字段，即 namespace 部分。
      bucket=$(echo "$SECRETFLOW_IMAGE" | cut -d "/" -f 2)
      # 取第 3 个字段，即 name:tag 部分。
      name_and_tag=$(echo "$SECRETFLOW_IMAGE" | cut -d "/" -f 3)
      # 再次使用全局替换去掉 :tag，得到纯镜像名。
      name=${name_and_tag//:${SF_IMAGE_TAG}/}
      # 组合 registry 与 namespace 作为镜像仓库前缀。
      SF_IMAGE_REGISTRY="$registry/$bucket"
      # 保存镜像名。
      SF_IMAGE_NAME="$name"
    fi
  fi
}

# 执行 SecretFlow 镜像信息初始化。
# 这里直接调用函数。Bash 中函数调用不需要括号，直接写函数名即可。
init_sf_image_info

# ==================== 容器生命周期辅助函数 ====================

# 判断是否需要启动（或重建）指定名称的 Docker 容器。
# 若容器不存在，返回 0（需要启动）；
# 若容器已存在，则询问用户是否重新创建，根据回答返回 0 或 1。
function need_start_docker_container() {
  # 声明局部变量 ctr，接收调用者传入的容器名。
  local ctr=$1

  # docker ps -a -q -f name=^/"${ctr}"$ 用于精确查询指定名称的容器。
  #   -a 表示列出所有容器，包括已停止的；
  #   -q 表示只输出容器 ID（quiet 模式）；
  #   -f 是 --filter 的缩写，用于过滤；
  #   name=^/"${ctr}"$ 是正则过滤：^/ 表示容器名以 / 开头（docker 内部存储的容器名前缀），"${ctr}"$ 表示以传入名称结尾。
  #   其中 "${ctr}" 对变量加双引号，防止名称中包含空格或特殊字符时被意外拆分。
  # 这样可以避免名称前缀相似的容器被误匹配，例如查找 "alice" 时不会匹配到 "alice-old"。
  # $(...) 将查询结果作为字符串；如果容器不存在，结果为空字符串。
  # [[ ! "..." ]] 判断字符串是否为空。! 表示取反，空字符串在条件中为假，取反后为真，进入 then 分支。
  if [[ ! "$(docker ps -a -q -f name=^/"${ctr}"$)" ]]; then
    # 容器不存在，需要启动。return 0 表示函数返回成功状态。
    return 0
  fi

  # read 是 Bash 内置命令，用于从标准输入读取一行。
  # -r 选项表示禁用反斜杠转义解释；-p 选项用于在读取前输出提示信息。
  # $(log "...") 是命令替换，先执行 log 函数得到带绿色的提示字符串，再作为 -p 的提示内容。
  # 读取到的用户输入保存到变量 yn 中。
  read -rp "$(log "The container '${ctr}' already exists. Do you need to recreate it? [y/n]:")" yn
  # case ... esac 是多分支条件语句，根据变量 yn 的值匹配不同的模式。
  case $yn in
  # [Yy]* 是字符类通配，匹配以 Y 或 y 开头的任意字符串，例如 y、yes、Y、YES。
  [Yy]*)
    log "Remove container ${ctr} ..."
    # docker rm -f 强制删除容器，即使容器正在运行也会被停止并删除。
    docker rm -f "$ctr"
    # 已删除旧容器，需要启动新容器。
    return 0
    ;;
  # *) 是默认分支，匹配所有未被前面分支命中的值。
  *)
    # 保留旧容器，跳过启动。return 1 表示函数返回失败状态。
    return 1
    ;;
  esac
}

# ==================== 探针检测函数 ====================

# 通用 HTTP 探针：在指定容器内通过 curl 访问 endpoint，最多重试 max_retry 次。
# enable_mtls=true 时，使用容器内的 CA 证书与客户端证书进行 mTLS 访问。
# 返回状态码为 200/404/401 时认为探测成功。
function do_http_probe() {
  # 通过 local 声明多个局部变量，分别接收容器名、探测地址、最大重试次数、是否启用 mTLS。
  local ctr=$1
  local endpoint=$2
  local max_retry=$3
  local enable_mtls=$4
  # cert_config 初始未赋值，在 enable_mtls 为 true 时设置成 mTLS 需要的 curl 证书参数。
  local cert_config
  # 判断是否需要启用 mTLS。
  if [[ "$enable_mtls" == "true" ]]; then
    # 设置 curl 的 mTLS 参数：
    #   --cacert 指定 CA 证书，用于验证服务端证书；
    #   --cert 指定客户端证书；
    #   --key 指定客户端私钥。
    # 这里复用了 CA 证书作为客户端证书，是因为 Kuscia 内部使用该证书进行双向认证。
    cert_config="--cacert ${CTR_CERT_ROOT}/ca.crt --cert ${CTR_CERT_ROOT}/ca.crt --key ${CTR_CERT_ROOT}/ca.key"
  fi

  # retry 记录当前重试次数，初始为 0。
  local retry=0
  # while ... do ... done 是循环结构，只要条件为真就重复执行循环体。
  # [[ "$retry" -lt "$max_retry" ]] 判断 retry 是否小于 max_retry；-lt 是"小于"（less than）运算符。
  while [[ "$retry" -lt "$max_retry" ]]; do
    # status_code 保存 curl 返回的 HTTP 状态码。
    local status_code
    # docker exec -it 在运行中的容器内执行命令；
    #   -i 保持 STDIN 打开；
    #   -t 分配伪终端（pseudo-TTY）。
    # curl 参数说明：
    #   -k 允许不验证 SSL 证书；
    #   --write-out '%{http_code}' 在输出结束后打印 HTTP 状态码；
    #   --silent 不显示进度条；
    #   --output /dev/null 将响应体丢弃；
    #   "${endpoint}" 是探测地址；
    #   "${cert_config}" 展开为 mTLS 证书参数（可能为空）。
    # 命令替换 $(...) 捕获 curl 打印的状态码。
    status_code=$(docker exec -it "$ctr" curl -k --write-out '%{http_code}' --silent --output /dev/null "${endpoint}" "${cert_config}")
    # 判断状态码是否为 200、404 或 401。这三种状态都认为服务端已启动并响应请求。
    # || 是逻辑或运算符，只要任一条件为真则整体为真；&& 是逻辑与运算符，需所有条件为真才为真。
    if [[ $status_code -eq 200 || $status_code -eq 404 || $status_code -eq 401 ]]; then
      # 探测成功，函数返回 0。
      return 0
    fi
    # 如果未成功，等待 1 秒后再重试。
    sleep 1
    # retry 自增 1。$(( ... )) 是算术扩展，用于执行整数运算。
    retry=$((retry + 1))
  done

  # 超过最大重试次数仍未成功，返回 1。
  return 1
}

# 探测容器内 k3s API Server 是否就绪（https://127.0.0.1:6443）。
# 超时 60 秒，失败则退出并提示查看 k3s 日志。
function probe_k3s() {
  # domain_ctr 接收要探测的容器名。
  local domain_ctr=$1

  # if ! do_http_probe ...; then ... fi 表示如果 do_http_probe 返回非 0，则进入 then 分支。
  if ! do_http_probe "$domain_ctr" "https://127.0.0.1:6443" 60; then
    # 探测失败时，向标准错误输出错误信息。>&2 将标准输出重定向到标准错误。
    # 1>&2 的简写是 >&2，表示把当前命令的输出（文件描述符 1）重定向到文件描述符 2（标准错误）。
    echo "[Error] Probe k3s in container '$domain_ctr' failed. Please check k3s log in container, path: /home/kuscia/var/logs/k3s.log" >&2
    # 以失败状态退出脚本。
    exit 1
  fi
}

# 探测指定 Namespace 下的 Gateway CRD 是否已经创建。
# 先确保 k3s 就绪，再轮询 Gateway 资源，最多重试 max_retry 秒。
function probe_gateway_crd() {
  # master 是容器名；domain 是 Kubernetes Namespace（在 Kuscia 中通常与 Domain ID 一致）；
  # gw_name 是 Gateway 名称；max_retry 是最大重试次数。
  local master=$1
  local domain=$2
  local gw_name=$3
  local max_retry=$4
  # 先调用 probe_k3s 确保 k3s 已就绪，否则后续 kubectl 命令无法执行。
  probe_k3s "$master"

  # retry 初始化为 0。
  local retry=0
  # 轮询 Gateway 资源。
  while [ "$retry" -lt "$max_retry" ]; do
    # line_num 保存匹配到的 Gateway 数量。
    local line_num
    # docker exec -it "$master" 在 master 容器内执行 kubectl 命令；
    # kubectl get gateways -n "$domain" 获取指定命名空间下的 Gateway 自定义资源；
    # grep -c -i "$gw_name" 统计包含 gw_name（不区分大小写）的行数；
    # xargs 用于去除前后空白字符。
    line_num=$(docker exec -it "$master" kubectl get gateways -n "$domain" | grep -c -i "$gw_name" | xargs)
    # 如果找到且仅找到 1 条记录，认为 Gateway CRD 已就绪。
    if [[ "$line_num" == "1" ]]; then
      # return 后面不加数字时，默认返回上一个命令的退出状态。
      # 这里因为上一条 [[ ... ]] 成功，所以等价于 return 0。
      return
    fi
    # 等待 1 秒后重试。
    sleep 1
    retry=$((retry + 1))
  done
  # 超过最大重试次数，输出错误信息到标准错误并退出。
  echo "[Error] Probe gateway in namespace '$domain' failed. Please check envoy log in container, path: /home/kuscia/var/logs/envoy" >&2
  exit 1
}

# ==================== 网络与存储 ====================

# 创建 Docker 网络 kuscia-exchange，若不存在则创建。
# 所有 Kuscia 容器通过该网络互相发现与通信。
function build_kuscia_network() {
  # docker network ls -q -f name=${NETWORK_NAME} 列出名称匹配的网络，-q 只输出网络 ID。
  # 如果该网络不存在，则返回空字符串，进入 then 分支创建网络。
  if [[ ! "$(docker network ls -q -f name=${NETWORK_NAME})" ]]; then
    # docker network create 创建一个 bridge 类型的自定义网络。
    docker network create "${NETWORK_NAME}"
  fi
}

# ==================== 引擎镜像管理 ====================

# 检查并导入 SecretFlow/PSI 引擎镜像到 Kuscia 容器内。
# 逻辑如下：
#   1. 优先从容器内的 crictl 检查镜像是否已存在；
#   2. 若不存在，检查宿主机 Docker 是否已有该镜像；
#   3. 若宿主机没有，根据 env.list 中的 REGISTRY_ENDPOINT 进行 docker login 并拉取；
#   4. 将镜像保存为 tar 包，再通过 containerd 导入到 Kuscia 容器内。
function check_sf_image() {
  # domain_ctr 是目标 Kuscia 容器名。
  local domain_ctr=$1
  # env_file 指向 ROOT 目录下的 env.list 环境变量文件。
  local env_file=${ROOT}/env.list
  # default_repo 使用从 SECRETFLOW_IMAGE 解析出的仓库前缀。
  local default_repo=${SF_IMAGE_REGISTRY}
  local repo
  # 如果 env.list 存在，则从中读取 REGISTRY_ENDPOINT 作为仓库地址。
  if [ -e "$env_file" ]; then
    # awk -F "=" '/REGISTRY_ENDPOINT/ {print $2}' 以等号为分隔符，匹配 REGISTRY_ENDPOINT 行并输出值。
    repo=$(awk -F "=" '/REGISTRY_ENDPOINT/ {print $2}' "$env_file")
  fi
  # 默认的 sf_image 由镜像名和标签组成。
  local sf_image="${SF_IMAGE_NAME}:${SF_IMAGE_TAG}"
  # 如果 env.list 中指定了仓库，则在镜像名前加上仓库前缀。
  # ${SF_IMAGE_NAME##*/} 是从开头删除最长匹配到 / 的内容，只保留最后一个 / 后面的镜像名。
  # 这样可以去掉可能存在的命名空间前缀，例如 secretflow/secretflow-lite-anolis8 只保留 secretflow-lite-anolis8。
  if [ "$repo" != "" ]; then
    sf_image="${repo}/${SF_IMAGE_NAME##*/}:${SF_IMAGE_TAG}"
  elif [ "$default_repo" != "" ]; then
    sf_image="${default_repo}/${SF_IMAGE_NAME##*/}:${SF_IMAGE_TAG}"
  fi
  # 如果 SECRETFLOW_IMAGE 非空，则直接使用用户指定的完整镜像地址。
  if [ "$SECRETFLOW_IMAGE" != "" ]; then
    sf_image=$SECRETFLOW_IMAGE
  fi

  # 若容器内已存在该镜像，直接获取镜像 ID 并返回。
  # docker exec -it "$domain_ctr" crictl inspecti "$sf_image" 查询容器内 containerd 中是否已有该镜像。
  # >/dev/null 2>&1 将标准输出和标准错误都丢弃；2>&1 表示把标准错误重定向到标准输出当前指向的位置（即 /dev/null）。
  # if command; then ... fi 判断命令是否成功（返回 0）。
  if docker exec -it "$domain_ctr" crictl inspecti "$sf_image" >/dev/null 2>&1; then
    log "Image '${sf_image}' already exists in domain '${DOMAIN_ID}'"
    # 使用 crictl inspecti 获取镜像 ID，并通过 jq 和 tr 清洗输出。
    SF_IMAGE_ID=$(docker exec -it "$domain_ctr" sh -c "crictl inspecti ${sf_image} | jq -r .status.id | tr -d ' \t\n\r'")
    # return 结束函数，返回 0。
    return
  fi

  # has_sf_image 标记宿主机 Docker 是否已有该镜像。
  local has_sf_image=false
  # docker image inspect 检查宿主机本地是否存在该镜像。
  if docker image inspect "${sf_image}" >/dev/null 2>&1; then
    has_sf_image=true
  fi

  if [ "$has_sf_image" == true ]; then
    log "Found the secretflow image '${sf_image}' on host"
  else
    log "Not found the secretflow image '${sf_image}' on host"
    # 如果 env.list 指定了仓库，先执行 docker login。
    if [ "$repo" != "" ]; then
      docker login "$repo"
    fi
    log "Start pulling image '${sf_image}' ..."
    # docker pull 从镜像仓库拉取镜像到宿主机。
    docker pull "${sf_image}"
  fi

  log "Start importing image '${sf_image}' Please be patient..."
  # image_id 保存宿主机 Docker 中该镜像的 ID。
  local image_id
  image_id=$(docker images --filter="reference=${sf_image}" --format "{{.ID}}")
  # image_tar 是生成的 tar 包路径，将镜像地址中的 / 替换为 _ 避免路径问题，并加上镜像 ID。
  local image_tar
  image_tar=/tmp/$(echo "${sf_image}" | sed 's/\//_/g').${image_id}.tar
  # 如果 tar 包不存在，则使用 docker save 将镜像导出为 tar 包。
  if [ ! -e "$image_tar" ]; then
    docker save "$sf_image" -o "$image_tar"
  fi
  # 使用 containerd 的 ctr 命令将 tar 包导入到 Kuscia 容器内的 containerd 镜像仓库。
  #   -a 指定 containerd socket 地址；
  #   -n=k8s.io 指定命名空间为 k8s.io（Kubernetes 默认使用的命名空间）。
  docker exec -it "$domain_ctr" ctr -a=${CTR_ROOT}/containerd/run/containerd.sock -n=k8s.io images import "$image_tar"
  log "Successfully imported image '${sf_image}' to container '${domain_ctr}' ..."

  # 导入完成后，再次获取镜像 ID。
  SF_IMAGE_ID=$(docker exec -it "$domain_ctr" sh -c "crictl inspecti ${sf_image} | jq -r .status.id | tr -d ' \t\n\r'")
}

# 在 Kuscia 容器内调用 create_sf_app_image.sh，根据 SECRETFLOW_IMAGE 注册 AppImage。
# 自动识别应用类型：镜像名以 psi- 开头则注册为 psi，否则注册为 secretflow。
function create_secretflow_app_image() {
  # ctr 接收要操作的 Kuscia 容器名。
  local ctr=$1
  # image_repo 默认使用完整的 SECRETFLOW_IMAGE 地址。
  local image_repo=$SECRETFLOW_IMAGE
  # image_tag 默认设为 latest，后续会根据镜像地址是否包含冒号来重新解析。
  local image_tag=latest

  # 检查 SECRETFLOW_IMAGE 中是否包含冒号，从而判断是否包含标签。
  if [[ "${SECRETFLOW_IMAGE}" == *":"* ]]; then
    # ${VAR%%:*} 是"从结尾删除最短匹配"操作，删除从结尾开始的 : 及其之后的内容，保留冒号前的仓库路径。
    # 例如 secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite-anolis8:1.11.0b1
    # 删除 :1.11.0b1 后得到仓库路径。
    image_repo=${SECRETFLOW_IMAGE%%:*}
    # 已在前面用 ##*: 提取过标签，这里再次提取镜像标签。
    image_tag=${SECRETFLOW_IMAGE##*:}
  fi

  # 通过 awk 提取镜像名的最后一个 / 后的部分，再以 - 分割取第一段，判断应用类型。
  # 例如 secretflow-lite-anolis8 分割后第一段是 secretflow；psi-xxx 分割后第一段是 psi。
  app_type=$(echo "${image_repo}" | awk -F'/' '{print $NF}' | awk -F'-' '{print $1}')
  # 如果 app_type 不是 psi，则统一归为 secretflow 类型。
  if [[ ${app_type} != "psi" ]]; then
    app_type="secretflow"
  fi

  # 在容器内执行 create_sf_app_image.sh 脚本，传入仓库路径、标签、应用类型、镜像 ID。
  docker exec -it "${ctr}" scripts/deploy/create_sf_app_image.sh "${image_repo}" "${image_tag}" "${app_type}" "${SF_IMAGE_ID}"
  log "Create secretflow app image done"
}

# ==================== 组件与数据初始化 ====================

# 探测 DataMesh 健康接口（https://127.0.0.1:8070/healthZ）。
# DataMesh 是 Kuscia 数据平面的核心组件，负责 DomainData 的元数据与访问控制。
function probe_datamesh() {
  # domain_ctr 接收要探测的容器名。
  local domain_ctr=$1
  # 调用 do_http_probe 探测 DataMesh 健康接口，最多重试 30 次，启用 mTLS。
  if ! do_http_probe "$domain_ctr" "https://127.0.0.1:8070/healthZ" 30 true; then
    # 探测失败时输出错误信息到标准错误。
    echo "[Error] Probe datamesh in container '$domain_ctr' failed." >&2
    # 提示用户可以查看容器日志来排查问题。
    echo "You cloud run command that 'docker logs $domain_ctr' to check the log" >&2
  fi
  # 无论成功与否，最终输出探测完成提示。
  log "Probe datamesh successfully"
}

# 创建示例 DomainData 数据表：Alice 与 Bob 的测试数据。
# 用于部署后的快速验证与演示。
function create_domaindata_table() {
  # ctr 接收要操作的 Kuscia 容器名。
  local ctr=$1

  # create domain data table
  # 在容器内执行 Alice 示例数据创建脚本，传入 DOMAIN_ID。
  docker exec -it "${ctr}" scripts/deploy/create_domaindata_alice_table.sh "${DOMAIN_ID}"
  # 在容器内执行 Bob 示例数据创建脚本，传入 DOMAIN_ID。
  docker exec -it "${ctr}" scripts/deploy/create_domaindata_bob_table.sh "${DOMAIN_ID}"
  # 在容器内执行隐私组件测试数据创建脚本。
  docker exec -it "${ctr}" scripts/deploy/create_domaindata_alice_privacy_table.sh "${DOMAIN_ID}"
  docker exec -it "${ctr}" scripts/deploy/create_domaindata_bob_privacy_table.sh "${DOMAIN_ID}"
  log "Create domain data table done"
}

# ==================== Docker 启动参数生成 ====================

# 生成容器启动时的环境变量参数。
# 若存在 env.list 文件，则整体挂载；否则仅传入 REGISTRY_ENDPOINT。
function generate_env_flag() {
  # env_flag 用于保存最终生成的 docker run 环境变量参数字符串。
  local env_flag
  # env_file 指向 ROOT 目录下的 env.list。
  local env_file=${ROOT}/env.list
  # [ -e "$env_file" ] 判断文件是否存在（-e 表示 exists）。
  if [ -e "$env_file" ]; then
    # 如果存在，使用 --env-file 选项整体挂载该环境变量文件。
    env_flag="--env-file $env_file"
  else
    # 如果不存在，仅传入 REGISTRY_ENDPOINT 环境变量。
    env_flag="--env REGISTRY_ENDPOINT=${SF_IMAGE_REGISTRY}"
  fi
  # echo 将生成的参数字符串输出给调用者，调用者通过 $(generate_env_flag) 捕获。
  echo "$env_flag"
}

# 生成容器启动时的挂载参数。
# 挂载 /tmp、数据目录、日志目录到容器内的对应位置。
function generate_mount_flag() {
  # mount_flag 包含三个 -v 挂载：
  #   - /tmp:/tmp 共享宿主机 /tmp，便于脚本间临时文件交互；
  #   - DOMAIN_DATA_DIR:/home/kuscia/var/storage/data 将数据目录挂载到容器内数据存储位置；
  #   - DOMAIN_LOG_DIR:/home/kuscia/var/stdout 将日志目录挂载到容器内标准输出目录。
  local mount_flag="-v /tmp:/tmp -v ${DOMAIN_DATA_DIR}:/home/kuscia/var/storage/data -v ${DOMAIN_LOG_DIR}:/home/kuscia/var/stdout"
  echo "$mount_flag"
}

# 从 kuscia.yaml 中读取 runtime 配置，未配置时默认使用 runc。
# runc 模式下需要 --privileged 参数，runc 也支持 SecretFlow 引擎镜像导入。
function get_runtime() {
  # conf_file 接收 kuscia.yaml 配置文件路径。
  local conf_file=$1
  # runtime 保存解析出的运行时名称。
  local runtime
  # grep '^runtime:' 匹配以 runtime: 开头的行；
  # cut -d':' -f2 以冒号分隔取第二部分（即运行时值）；
  # awk '{$1=$1};1' 去除前后空白（通过 $1=$1 重新赋值触发字段重算，;1 表示打印）；
  # tr -d '\r\n' 删除回车和换行符，避免 Windows 换行影响。
  runtime=$(grep '^runtime:' "${conf_file}" 2>/dev/null | cut -d':' -f2 | awk '{$1=$1};1' | tr -d '\r\n')
  # 如果配置文件中未设置 runtime，则默认使用 runc。
  if [[ $runtime == "" ]]; then
    runtime=runc
  fi
  # 输出运行时名称。
  echo "$runtime"
}

# 创建 Docker Volume，若不存在则创建。
# 用于持久化 containerd 数据，避免容器重建后镜像丢失。
function createVolume() {
  # VOLUME_NAME 接收要创建的卷名称。
  local VOLUME_NAME=$1
  # docker volume ls --format '{{.Name}}' 列出所有卷名称。
  # grep "^${VOLUME_NAME}$" 精确匹配目标卷名（^ 表示行首，$ 表示行尾）。
  # if ! command; then ... fi 如果命令返回非 0（即卷不存在），则创建卷。
  if ! docker volume ls --format '{{.Name}}' | grep "^${VOLUME_NAME}$"; then
    # docker volume create 创建一个命名卷。
    docker volume create "$VOLUME_NAME"
  fi
}

# ==================== 部署函数：Autonomy / Lite / Master ====================

# 部署 Autonomy 节点。
# Autonomy 是独立的 Kuscia 节点，包含完整的控制面与数据面，适合 P2P/CxC 组网。
function deploy_autonomy() {
  # domain_ctr 是本次部署生成的容器名称，格式为 {USER}-kuscia-autonomy-{DOMAIN_ID}。
  # ${USER} 是当前用户名环境变量；${DOMAIN_ID} 是节点域名标识。
  local domain_ctr=${USER}-kuscia-autonomy-${DOMAIN_ID}
  # conf_dir 是配置目录路径，默认与容器名同名。
  local conf_dir=${ROOT}/${domain_ctr}
  # kuscia_config_file 默认指向配置目录下的 kuscia.yaml。
  local kuscia_config_file=${conf_dir}/kuscia.yaml
  # 如果用户通过 -c 参数或环境变量指定了外部配置文件，则优先使用外部配置文件。
  if [[ ${KUSCIA_CONFIG_FILE} != "" ]]; then
    kuscia_config_file=${KUSCIA_CONFIG_FILE}
  fi
  # runtime 保存容器运行时需要使用的容器运行时（runc 或其他）。
  local runtime
  runtime=$(get_runtime "${kuscia_config_file}")
  # 检查宿主机架构是否受支持。
  arch_check
  # need_start_docker_container 返回 0 表示需要启动（或重建）容器。
  if need_start_docker_container "$domain_ctr"; then
    log "Starting container $domain_ctr ..."
    # 生成环境变量参数和挂载参数。
    env_flag=$(generate_env_flag)
    mount_flag=$(generate_mount_flag)

    # 若未提供外部配置文件，则通过 kuscia init 生成默认配置。
    if [[ ${KUSCIA_CONFIG_FILE} == "" ]]; then
      # mkdir -p 创建配置目录。
      mkdir -p "${conf_dir}"
      # docker run -it --rm 启动一个临时容器，执行 kuscia init 命令生成配置。
      #   -i 保持 STDIN 打开；
      #   -t 分配伪终端；
      #   --rm 命令执行完毕后自动删除容器，不保留容器状态。
      # kuscia init --mode Autonomy --domain "${DOMAIN_ID}" 根据 Autonomy 模式和域名生成配置。
      # >"${kuscia_config_file}" 将标准输出重定向到配置文件；
      # 2>&1 将标准错误也重定向到标准输出，即一并写入配置文件；
      # || cat "${kuscia_config_file}" 表示如果 kuscia init 失败，则打印配置文件内容供排错。
      docker run -it --rm "${KUSCIA_IMAGE}" kuscia init --mode Autonomy --domain "${DOMAIN_ID}" >"${kuscia_config_file}" 2>&1 || cat "${kuscia_config_file}"
      # wrap_kuscia_config_file 用于根据 ALLOW_PRIVILEGED 环境变量追加 privileged 相关配置。
      wrap_kuscia_config_file "${kuscia_config_file}"
    fi

    # runc 运行时默认需要特权模式，以支持容器内启动子容器。
    local privileged_flag
    if [[ ${runtime} == "runc" ]]; then
      # privileged_flag 变量值前有一个前导空格，方便后续直接拼接到 docker run 命令中。
      privileged_flag=" --privileged"
    fi

    # 创建 containerd 数据持久化卷。
    createVolume "${domain_ctr}-containerd"

    # docker run 启动 Autonomy 容器。
    #   -d 表示后台运行（detached）；
    #   -i 保持 STDIN 打开；
    #   -t 分配伪终端；
    #   "${privileged_flag}" 展开后可能是空字符串或 " --privileged"；
    #   --name 指定容器名；
    #   --hostname 指定容器内的主机名；
    #   --restart=always 表示容器异常退出或宿主机重启后自动重启；
    #   --network 指定加入的 Docker 网络；
    #   -m 限制容器可使用的最大内存；
    #   -p 将宿主机端口映射到容器端口。
    # 端口映射说明：
    #   1080 是跨域网关（Gateway）端口，用于跨域通信；
    #   80 是内部应用访问端口；
    #   8082 是 KusciaAPI HTTP 端口；
    #   8083 是 KusciaAPI gRPC 端口。
    #   -v 用于挂载卷或目录到容器内。
    docker run -dit "${privileged_flag}" --name="${domain_ctr}" --hostname="${domain_ctr}" --restart=always --network=${NETWORK_NAME} -m ${AUTONOMY_MEMORY_LIMIT} \
      -p "${DOMAIN_HOST_PORT}":1080 \
      -p "${DOMAIN_HOST_INTERNAL_PORT}":80 \
      -p "${KUSCIAAPI_HTTP_PORT}":8082 \
      -p "${KUSCIAAPI_GRPC_PORT}":8083 \
      -v "${domain_ctr}-containerd":${CTR_ROOT}/containerd \
      -v "${kuscia_config_file}":/home/kuscia/etc/conf/kuscia.yaml \
      "${env_flag}" "${mount_flag}" \
      --env NAMESPACE="${DOMAIN_ID}" \
      "${KUSCIA_IMAGE}" bin/kuscia start -c etc/conf/kuscia.yaml

    # 探测 DataMesh 是否就绪。
    probe_datamesh "${domain_ctr}"
    # 初始化 KusciaAPI 客户端证书，便于外部通过 mTLS 访问 KusciaAPI。
    docker exec -it "${domain_ctr}" sh scripts/deploy/init_kusciaapi_client_certs.sh

    log "Container ${domain_ctr} started successfully"
  fi

  # runc 模式下才需要导入 SecretFlow 引擎镜像。
  if [[ ${runtime} == "runc" ]]; then
    check_sf_image "${domain_ctr}"
  fi

  # 注册 SecretFlow/PSI AppImage。
  create_secretflow_app_image "${domain_ctr}"

  # create demo data
  # 创建示例 DomainData 数据表。
  create_domaindata_table "${domain_ctr}"

  log "Autonomy domain '${DOMAIN_ID}' deployed successfully"
}

# 部署 Lite 节点。
# Lite 节点必须指定 MASTER_ENDPOINT 与 DOMAIN_TOKEN，注册到 Master 后作为工作节点运行。
function deploy_lite() {
  # Lite 容器名称格式。
  local domain_ctr=${USER}-kuscia-lite-${DOMAIN_ID}
  local conf_dir=${ROOT}/${domain_ctr}
  local kuscia_config_file=${conf_dir}/kuscia.yaml
  if [[ ${KUSCIA_CONFIG_FILE} != "" ]]; then
    kuscia_config_file=${KUSCIA_CONFIG_FILE}
  fi
  local runtime
  runtime=$(get_runtime "${kuscia_config_file}")

  # 在启动容器前，先通过临时容器访问 Master Endpoint 验证网络连通性。
  # 期望返回 401，表示 HTTPS 端口可达且鉴权生效。
  local HttpResponseCode
  # docker run -it --rm --network=${NETWORK_NAME} 启动临时容器并加入 Kuscia 网络。
  # curl -k 忽略证书验证；-s 静默模式；-o /dev/null 丢弃响应体；-w "%{http_code}" 输出 HTTP 状态码。
  HttpResponseCode=$(docker run -it --rm --network=${NETWORK_NAME} "${KUSCIA_IMAGE}" curl -k -s -o /dev/null -w "%{http_code}" "${MASTER_ENDPOINT}")
  # 检查宿主机架构。
  arch_check

  if need_start_docker_container "$domain_ctr"; then
    log "Starting container $domain_ctr ..."

    env_flag=$(generate_env_flag)
    mount_flag=$(generate_mount_flag)

    # 判断 Master 返回的 HTTP 状态码是否为 401。
    if [[ $HttpResponseCode = "401" ]]; then
      echo -e "${GREEN}Communication with master is normal, response code is 401${NC}"
    else
      # 如果不是 401，说明无法正确连接到 Master，输出红色提示并退出。
      echo -e "${RED}Failed to connect to the master. Please check if the network link to the master is normal. Please refer to the kuscia documentation (https://www.secretflow.org.cn/docs/kuscia/latest/zh-Hans/deployment/deploy_master_lite_cn) for the correct return results${NC}"
      # 再次执行 curl -kvvv 输出详细调试信息，帮助用户排查。
      docker run -it --rm --network=${NETWORK_NAME} "${KUSCIA_IMAGE}" curl -kvvv "${MASTER_ENDPOINT}"
      exit 1
    fi

    # 若未提供外部配置文件，则通过 kuscia init 生成 Lite 配置，需传入 Master 端点与部署 Token。
    if [[ ${KUSCIA_CONFIG_FILE} == "" ]]; then
      mkdir -p "${conf_dir}"
      # kuscia init --mode Lite 生成 Lite 节点配置；--master-endpoint 和 --lite-deploy-token 用于注册到 Master。
      docker run -it --rm "${KUSCIA_IMAGE}" kuscia init --mode Lite --domain "${DOMAIN_ID}" --master-endpoint "${MASTER_ENDPOINT}" --lite-deploy-token "${DOMAIN_TOKEN}" >"${kuscia_config_file}" 2>&1 || cat "${kuscia_config_file}"
      wrap_kuscia_config_file "${kuscia_config_file}"
    fi

    local privileged_flag
    if [[ ${runtime} == "runc" ]]; then
      privileged_flag=" --privileged"
    fi

    createVolume "${domain_ctr}-containerd"

    docker run -dit "${privileged_flag}" --name="${domain_ctr}" --hostname="${domain_ctr}" --restart=always --network=${NETWORK_NAME} -m $LITE_MEMORY_LIMIT \
      -p "${DOMAIN_HOST_PORT}":1080 \
      -p "${DOMAIN_HOST_INTERNAL_PORT}":80 \
      -p "${KUSCIAAPI_HTTP_PORT}":8082 \
      -p "${KUSCIAAPI_GRPC_PORT}":8083 \
      -v "${domain_ctr}-containerd":${CTR_ROOT}/containerd \
      -v "${kuscia_config_file}":/home/kuscia/etc/conf/kuscia.yaml \
      "${env_flag}" "${mount_flag}" \
      --env NAMESPACE="${DOMAIN_ID}" \
      "${KUSCIA_IMAGE}" bin/kuscia start -c etc/conf/kuscia.yaml

    probe_datamesh "$domain_ctr"
    docker exec -it "${domain_ctr}" sh scripts/deploy/init_kusciaapi_client_certs.sh

    log "Lite domain '${DOMAIN_ID}' started successfully"
  fi

  if [[ ${runtime} == "runc" ]]; then
    check_sf_image "${domain_ctr}"
  fi

  log "Lite domain '${DOMAIN_ID}' deployed successfully"
}

# 部署 Master 节点。
# Master 是中心化组网的中控节点，负责 Domain 管理、任务调度、证书与路由管理。
function deploy_master() {
  # Master 容器名称固定为 {USER}-kuscia-master，不携带 DOMAIN_ID。
  local domain_ctr=${USER}-kuscia-master
  # master_domain_id 默认使用用户传入的 DOMAIN_ID。
  local master_domain_id=${DOMAIN_ID}
  local conf_dir=${ROOT}/${domain_ctr}
  local kuscia_config_file=${conf_dir}/kuscia.yaml
  if [[ ${KUSCIA_CONFIG_FILE} != "" ]]; then
    kuscia_config_file=${KUSCIA_CONFIG_FILE}
  fi
  arch_check

  if need_start_docker_container "${domain_ctr}"; then
    log "Starting container ${domain_ctr} ..."

    env_flag=$(generate_env_flag)
    mount_flag=$(generate_mount_flag)

    # 若未提供外部配置文件，则通过 kuscia init 生成 Master 配置。
    if [[ ${KUSCIA_CONFIG_FILE} == "" ]]; then
      mkdir -p "${conf_dir}"
      docker run -it --rm "${KUSCIA_IMAGE}" kuscia init --mode Master --domain "$master_domain_id" >"${kuscia_config_file}" 2>&1 || cat "${kuscia_config_file}"
    fi

    # Master 容器启动参数，不包含 --privileged（Master 通常不需要 privileged）。
    docker run -dit --name="${domain_ctr}" --hostname="${domain_ctr}" --restart=always --network=${NETWORK_NAME} -m ${MASTER_MEMORY_LIMIT} \
      --env NAMESPACE="${master_domain_id}" \
      -p "${DOMAIN_HOST_PORT}":1080 \
      -p "${KUSCIAAPI_HTTP_PORT}":8082 \
      -p "${KUSCIAAPI_GRPC_PORT}":8083 \
      -v "${kuscia_config_file}":/home/kuscia/etc/conf/kuscia.yaml \
      "${env_flag}" "${mount_flag}" \
      "${KUSCIA_IMAGE}" bin/kuscia start -c etc/conf/kuscia.yaml

    # 探测 Gateway CRD 是否创建成功。
    probe_gateway_crd "${domain_ctr}" "${master_domain_id}" "${domain_ctr}" 60
    docker exec -it "${domain_ctr}" sh scripts/deploy/init_kusciaapi_client_certs.sh
    log "Master '${master_domain_id}' started successfully"
  fi
  create_secretflow_app_image "${domain_ctr}"
  log "Master deployed successfully"
}

# ==================== 配置与路径辅助函数 ====================

# 根据 ALLOW_PRIVILEGED 环境变量，在 kuscia.yaml 末尾追加允许特权容器的配置。
# 主要用于 Autonomy/Lite 运行需要 privileged 权限的引擎任务。
function wrap_kuscia_config_file() {
  # kuscia_config_file 接收配置文件路径。
  local kuscia_config_file=$1
  # p2p_protocol 接收第二个参数（当前函数体中未实际使用，保留是为了接口兼容）。
  local p2p_protocol=$2

  # PRIVILEGED_CONFIG 是准备追加到配置文件的 YAML 片段。
  # 注意这里的字符串使用了双引号，内部包含换行，因此展开后是两行 YAML。
  PRIVILEGED_CONFIG="
agent:
  allowPrivileged: true
  "
  # 如果环境变量 ALLOW_PRIVILEGED 为 true，则追加配置。
  if [[ $ALLOW_PRIVILEGED == "true" ]]; then
    # echo -e "$PRIVILEGED_CONFIG" 将变量内容输出；>> 表示以追加方式写入文件。
    # 注意这里使用的是 >> 追加，而不是 > 覆盖，因此不会破坏 kuscia init 生成的原有配置。
    echo -e "$PRIVILEGED_CONFIG" >>"$kuscia_config_file"
  fi
}

# 获取文件或目录的绝对路径。
function get_absolute_path() {
  # $1 是传入的路径，可能是相对路径也可能是绝对路径。
  # 整体结构是 echo $(cd dirname; pwd -P)/basename。
  # dirname -- "$1" 提取路径的目录部分；-- 表示选项结束，防止路径以 - 开头被误解析为选项。
  # cd "$(dirname -- "$1")" 切换到该目录；>/dev/null 隐藏 cd 的输出。
  # pwd -P 打印物理路径（解析所有符号链接），-P 表示 physical。
  # basename -- "$1" 提取路径的文件名部分。
  # 最终输出形如 /absolute/dir/filename 的完整绝对路径。
  echo "$(
    cd "$(dirname -- "$1")" >/dev/null
    pwd -P
  )/$(basename -- "$1")"
}

# ==================== 命令行参数解析 ====================

# 打印使用说明。
# 这里使用了 function 关键字的省略形式 usage() { ... }，与 function usage() { ... } 等价。
usage() {
  # basename "$0" 提取脚本文件名，$0 表示当前脚本本身的路径。
  echo "$(basename "$0") DEPLOY_MODE [OPTIONS]
DEPLOY_MODE:
    autonomy        Deploy a autonomy domain.
    lite            Deploy a lite domain.
    master          Deploy a master.

OPTIONS:
    -c              The host path of kuscia configure file.  It will be mounted into the domain container.
    -d              The data directory used to store domain data. It will be mounted into the domain container.
                    You can set Env 'DOMAIN_DATA_DIR' instead.  Default is '{{ROOT}}/{{DOMAIN_CONTAINER_NAME}}/data'.
    -h              Show this help text.
    -l              The data directory used to store domain logs. It will be mounted into the domain container.
                    You can set Env 'DOMAIN_LOG_DIR' instead.  Default is '{{ROOT}}/{{DOMAIN_CONTAINER_NAME}}/logs'.
    -m              (Only used in lite mode) The master endpoint. You can set Env 'MASTER_ENDPOINT' instead.
    -n              Domain id to be deployed. You can set Env 'DOMAIN_ID' instead.
    -p              The port exposed by domain. You can set Env 'DOMAIN_HOST_PORT' instead.
    -q              (Only used in autonomy or lite mode)The port exposed for internal use by domain. You can set Env 'DOMAIN_HOST_INTERNAL_PORT' instead.
    -r              The install directory. You can set Env 'ROOT' instead. Default is $(pwd).
    -t              (Only used in lite mode) The deploy token. You can set Env 'DOMAIN_TOKEN' instead.
    -k              (Only used in autonomy or master mode)The http port exposed by KusciaAPI , default is 13082. You can set Env 'KUSCIAAPI_HTTP_PORT' instead.
    -g              (Only used in autonomy or master mode)The grpc port exposed by KusciaAPI, default is 13083. You can set Env 'KUSCIAAPI_GRPC_PORT' instead.
    -e              (Only used in autonomy or master mode)The extra subjectAltName for KusciaAPI server cert.
    "
}

# 解析第一个位置参数：部署模式。
# deploy_mode 初始化为空字符串。
deploy_mode=
# case "$1" in ... esac 对第一个位置参数进行多分支匹配。
case "$1" in
# autonomy | lite | master 是或分支，匹配任一值。
autonomy | lite | master)
  # 将匹配到的模式赋值给 deploy_mode。
  deploy_mode=$1
  # shift 将位置参数整体左移一位，$2 变成 $1，$3 变成 $2，依此类推。
  # 这样可以把部署模式从参数列表中移除，后续 getopts 只解析选项参数。
  shift
  ;;
# -h 表示用户请求帮助信息。
-h)
  usage
  # exit 后面无参数时，默认以 0 状态退出，表示正常结束。
  exit
  ;;
# *) 是默认分支，匹配所有无效值。
*)
  echo "deploy_mode is invalid, must be autonomy, lite or master"
  usage
  exit 1
  ;;
esac

# 解析可选参数。
# getopts 是 Bash 内置的命令行选项解析工具。
# 'c:l:n:p:q:m:t:r:d:k:g:e:h' 是选项字符串：
#   每个字母代表一个选项；
#   字母后带冒号 : 表示该选项需要一个参数；
#   不带冒号表示该选项是开关型，不需要参数。
# getopts 会逐次解析选项，每次循环把当前选项存入变量 option。
# OPTARG 是 getopts 自动设置的特殊变量，保存当前选项的参数值。
# OPTIND 是 getopts 自动设置的特殊变量，保存下一个要处理的位置参数的索引。
while getopts 'c:l:n:p:q:m:t:r:d:k:g:e:h' option; do
  case "$option" in
  # -c 选项：指定 kuscia 配置文件路径。
  c)
    # get_absolute_path 将用户传入的相对路径转换为绝对路径，确保 docker run -v 挂载时路径正确。
    KUSCIA_CONFIG_FILE=$(get_absolute_path "$OPTARG")
    ;;
  # -l 选项：指定日志目录。
  l)
    DOMAIN_LOG_DIR=$OPTARG
    ;;
  # -n 选项：指定 Domain ID。
  n)
    DOMAIN_ID=$OPTARG
    ;;
  # -p 选项：指定跨域网关暴露的宿主机端口。
  p)
    DOMAIN_HOST_PORT=$OPTARG
    ;;
  # -q 选项：指定内部应用暴露的宿主机端口。
  q)
    DOMAIN_HOST_INTERNAL_PORT=$OPTARG
    ;;
  # -m 选项：指定 Lite 模式下的 Master 端点。
  m)
    MASTER_ENDPOINT=$OPTARG
    ;;
  # -t 选项：指定 Lite 模式下的部署 Token。
  t)
    DOMAIN_TOKEN=$OPTARG
    ;;
  # -r 选项：指定安装根目录。
  r)
    ROOT=$OPTARG
    ;;
  # -d 选项：指定数据目录。
  d)
    DOMAIN_DATA_DIR=$OPTARG
    ;;
  # -k 选项：指定 KusciaAPI HTTP 端口。
  k)
    KUSCIAAPI_HTTP_PORT=$OPTARG
    ;;
  # -g 选项：指定 KusciaAPI gRPC 端口。
  g)
    KUSCIAAPI_GRPC_PORT=$OPTARG
    ;;
  # -e 选项：指定 KusciaAPI 服务端证书的额外 subjectAltName。
  e)
    KUSCIAAPI_EXTRA_SUBJECT_ALTNAME=$OPTARG
    ;;
  # -h 选项：显示帮助。
  h)
    usage
    exit
    ;;
  # : 表示某个需要参数的选项缺少参数。
  :)
    printf "missing argument for -%s\n" "$OPTARG" >&2
    exit 1
    ;;
  # \? 表示遇到了不认识的选项。
  \?)
    printf "illegal option: -%s\n" "$OPTARG" >&2
    exit 1
    ;;
  esac
done
# shift $((OPTIND - 1)) 将已经解析过的选项参数移除，使剩余的位置参数（如果有）从 $1 开始。
# $(( ... )) 是算术扩展，OPTIND - 1 表示已处理的参数个数。
shift $((OPTIND - 1))

# ==================== 默认值与必填校验 ====================

# 外部暴露的跨域网关端口（1080）必须显式指定。
if [[ ${DOMAIN_HOST_PORT} == "" ]]; then
  # printf 是格式化输出命令，功能比 echo 更强；这里输出错误信息到标准错误。
  printf "empty domain host port\n" >&2
  exit 1
fi

# 内部应用端口（80）默认值。
if [[ ${DOMAIN_HOST_INTERNAL_PORT} == "" ]]; then
  DOMAIN_HOST_INTERNAL_PORT=13081
fi

# KusciaAPI HTTP 端口默认值。
if [[ ${KUSCIAAPI_HTTP_PORT} == "" ]]; then
  KUSCIAAPI_HTTP_PORT=13082
fi

# KusciaAPI gRPC 端口默认值。
if [[ ${KUSCIAAPI_GRPC_PORT} == "" ]]; then
  KUSCIAAPI_GRPC_PORT=13083
fi

# 初始化数据目录、日志目录等配置，并创建 Docker 网络。
function init() {
  # 接收部署模式作为参数。
  local deploy_mode=$1
  # 根据部署模式生成默认的容器名。
  local domain_ctr=${USER}-kuscia-${deploy_mode}-${DOMAIN_ID}
  # Master 模式的容器名比较特殊，固定为 {USER}-kuscia-master，不携带 DOMAIN_ID。
  if [[ ${deploy_mode} == "master" ]]; then
    local domain_ctr=${USER}-kuscia-master
  fi
  # [[ ${ROOT} == "" ]] && ROOT=${PWD} 是简写的 if 语句：
  #   如果 ROOT 为空，则将其设置为当前工作目录 ${PWD}。
  # && 是逻辑与，只有左侧条件为真时才执行右侧命令。
  [[ ${ROOT} == "" ]] && ROOT=${PWD}
  # 如果数据目录未指定，默认使用当前工作目录下的 {容器名}/data。
  [[ ${DOMAIN_DATA_DIR} == "" ]] && DOMAIN_DATA_DIR="${PWD}/${domain_ctr}/data"
  # 如果日志目录未指定，默认使用 ROOT 目录下的 {容器名}/logs。
  [[ ${DOMAIN_LOG_DIR} == "" ]] && DOMAIN_LOG_DIR="${ROOT}/${domain_ctr}/logs"

  # 检查数据目录和日志目录是否可创建/可写。
  pre_check "${DOMAIN_DATA_DIR}"
  pre_check "${DOMAIN_LOG_DIR}"

  # 打印最终配置信息，便于用户核对。
  log "ROOT=${ROOT}"
  log "DOMAIN_ID=${DOMAIN_ID}"
  log "DOMAIN_HOST_PORT=${DOMAIN_HOST_PORT}"
  log "DOMAIN_HOST_INTERNAL_PORT=${DOMAIN_HOST_INTERNAL_PORT}"
  log "DOMAIN_DATA_DIR=${DOMAIN_DATA_DIR}"
  log "DOMAIN_LOG_DIR=${DOMAIN_LOG_DIR}"
  log "KUSCIA_IMAGE=${KUSCIA_IMAGE}"
  log "KUSCIAAPI_HTTP_PORT=${KUSCIAAPI_HTTP_PORT}"
  log "KUSCIAAPI_GRPC_PORT=${KUSCIAAPI_GRPC_PORT}"

  # 创建 Kuscia 容器通信所需的 Docker 网络。
  build_kuscia_network
}

# ==================== 根据部署模式执行安装 ====================

# 对 deploy_mode 进行最终分支处理，执行对应的部署函数。
case ${deploy_mode} in
autonomy)
  # Autonomy 模式必须指定 DOMAIN_ID。
  if [[ ${DOMAIN_ID} == "" ]]; then
    printf "empty domain id\n" >&2
    exit 1
  fi
  # 调用 init 函数初始化目录和网络。
  init "${deploy_mode}"
  # 调用 deploy_autonomy 执行 Autonomy 节点部署。
  deploy_autonomy
  ;;
lite)
  # Lite 模式必须指定 DOMAIN_ID。
  if [[ ${DOMAIN_ID} == "" ]]; then
    printf "empty domain id\n" >&2
    exit 1
  fi
  # Lite 模式必须指定 DOMAIN_TOKEN，用于注册到 Master。
  if [[ ${DOMAIN_TOKEN} == "" ]]; then
    printf "Empty domain token\n" >&2
    exit 1
  fi
  # Lite 模式必须指定 MASTER_ENDPOINT。
  if [[ ${MASTER_ENDPOINT} == "" ]]; then
    printf "Empty master endpoint\n" >&2
    exit 1
  fi

  init "${deploy_mode}"
  deploy_lite
  ;;
master)
  # Master 模式下如果未指定 DOMAIN_ID，默认使用 kuscia-system。
  if [[ ${DOMAIN_ID} == "" ]]; then
    DOMAIN_ID=kuscia-system
  fi
  init "${deploy_mode}"
  deploy_master
  ;;
*)
  # 兜底分支，理论上不会触发，因为前面已经校验过 deploy_mode。
  printf "unsupported network mode: %s\n" "$deploy_mode" >&2
  exit 1
  ;;
esac
