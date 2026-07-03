#!/bin/bash
#
# Copyright 2025 Ant Group Co., Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# ==================== 脚本功能说明 ====================
# 该脚本用于创建基于反向隧道（Reverse Tunnel）的Kuscia测试集群
# 主要用于测试P2P组网模式下的反向隧道通信机制
#
# 核心特性:
# 1. 多副本部署：Alice域3个副本，Bob域1个副本，模拟分布式环境
# 2. 反向隧道：通过REVERSE-TUNNEL方式实现跨域通信，无需目标域暴露公网IP
# 3. 自动化配置：自动交换证书、创建路由、注册数据和应用镜像
# 4. Docker Swarm：使用Swarm模式管理多副本容器和网络
#
# 架构说明:
# - Alice域：3个副本容器，端口映射 14869->1080
# - Bob域：1个副本容器，端口映射 24869->1080
# - MySQL存储：每个域独立的MySQL数据库作为数据存储后端
# - Overlay网络：kuscia-swarm-exchange提供跨容器通信
#
# 执行流程:
# 1. 创建Docker Swarm和Overlay网络
# 2. 生成Alice和Bob的配置文件
# 3. 部署Docker Stack（多副本容器）
# 4. 交换TLS证书并清理iptables规则
# 5. 创建Domain资源和跨域路由
# 6. 配置反向隧道传输方式
# 7. 注册示例数据和SecretFlow应用镜像
#
# 使用方法: ./create_reverse_tunnel_test_cluster.sh
# 前置条件:
#   - Docker已安装并启动
#   - KUSCIA_IMAGE环境变量已设置（Kuscia镜像名称）
#   - IMAGE环境变量已设置（可选，默认使用KUSCIA_IMAGE）

# ==================== 辅助函数：获取Docker容器列表 ====================

# 根据前缀名称查找正在运行的Docker容器ID
# 参数:
#   prefix: 容器名称前缀，如 "kuscia-autonomy-alice"
# 返回:
#   匹配的容器ID列表（空格分隔）
function docker_get_ctrs() {
  local prefix=$1
  # docker ps 列出所有运行中的容器
  # grep 过滤包含指定前缀的行
  # awk '{print $1}' 提取第一列（容器ID）
  docker ps | grep "$prefix" | awk '{print $1}'
}

# ==================== 核心函数：构建副本间连接和配置 ====================

# 该函数完成以下关键任务：
# 1. 交换Alice和Bob域的TLS证书
# 2. 清理容器内的iptables规则（避免网络隔离）
# 3. 创建Domain资源和跨域路由
# 4. 配置反向隧道传输方式
# 5. 注册示例数据和SecretFlow应用镜像
function build_replica_conn() {
  # 获取本机IP地址（第一个网卡）
  local hostname
  hostname=$(hostname -I | awk '{print $1}')

  # 获取Alice和Bob域的所有容器ID
  # read -ra 将空格分隔的字符串读入数组
  read -ra alice_ctrs <<< "$(docker_get_ctrs kuscia-autonomy-alice)"
  read -ra bob_ctrs <<< "$(docker_get_ctrs kuscia-autonomy-bob)"

  # 从多个副本中选择一个主容器进行操作
  # 选择第一个容器作为代表执行配置命令
  local alice_choice_ctr=${alice_ctrs[0]}
  local bob_choice_ctr=${bob_ctrs[0]}

  # --- 步骤1：交换TLS证书 ---
  # 从Alice主容器复制其证书到本地
  docker cp "$alice_choice_ctr":/home/kuscia/var/certs/domain.crt alice.domain.crt
  
  # 将Alice证书复制到所有Bob容器中，使其能够验证Alice的身份
  for ctr in "${bob_ctrs[@]}"; do
    docker cp alice.domain.crt "$ctr":/home/kuscia/var/certs/alice.domain.crt
    # 清理Bob容器的iptables规则，确保网络连通性
    rm_iptables "$ctr"
  done

  # 从Bob主容器复制其证书到本地
  docker cp "$bob_choice_ctr":/home/kuscia/var/certs/domain.crt bob.domain.crt
  
  # 将Bob证书复制到所有Alice容器中，使其能够验证Bob的身份
  for ctr in "${alice_ctrs[@]}"; do
    docker cp bob.domain.crt "$ctr":/home/kuscia/var/certs/bob.domain.crt
    # 清理Alice容器的iptables规则，确保网络连通性
    rm_iptables "$ctr"
  done

  # --- 步骤2：创建Domain资源 ---
  # 在Alice容器中注册Bob域（P2P模式）
  docker exec -it "${alice_choice_ctr}" scripts/deploy/add_domain.sh bob p2p
  # 在Bob容器中注册Alice域（P2P模式）
  docker exec -it "${bob_choice_ctr}" scripts/deploy/add_domain.sh alice p2p
  
  # --- 步骤3：加入主机网络 ---
  # 配置Alice连接到Bob的服务端点（通过主机IP和端口24869）
  docker exec -it "${alice_choice_ctr}" scripts/deploy/join_to_host.sh alice bob https://"$hostname":24869
  # 配置Bob连接到Alice的服务端点（通过主机IP和端口14869）
  docker exec -it "${bob_choice_ctr}" scripts/deploy/join_to_host.sh bob alice https://"$hostname":14869
  
  # --- 步骤4：配置反向隧道 ---
  # 修改ClusterDomainRoute资源，将传输方式设置为REVERSE-TUNNEL
  # 反向隧道允许目标域主动建立连接到源域，无需目标域暴露公网IP
  # --type=merge 表示合并更新，只修改spec.transit.transitMethod字段
  docker exec -it "${alice_choice_ctr}" kubectl patch cdr alice-bob --type=merge -p '{"spec":{"transit":{"transitMethod":"REVERSE-TUNNEL"}}}'
  docker exec -it "${bob_choice_ctr}" kubectl patch cdr alice-bob --type=merge -p '{"spec":{"transit":{"transitMethod":"REVERSE-TUNNEL"}}}'
  
  # --- 步骤5：创建示例数据 ---
  # 在Alice域中创建示例数据表
  create_domaindata_alice_table "${alice_choice_ctr}" alice
  # 在Bob域中创建示例数据表
  create_domaindata_bob_table "${bob_choice_ctr}" bob
  
  # --- 步骤6：创建数据授权 ---
  # 配置Alice到Bob的数据共享授权
  create_domaindatagrant_alice2bob "${alice_choice_ctr}"
  # 配置Bob到Alice的数据共享授权
  create_domaindatagrant_bob2alice "${bob_choice_ctr}"
  
  # --- 步骤7：注册SecretFlow应用镜像 ---
  # 在Alice域中注册SecretFlow应用镜像（用于隐私计算任务）
  create_secretflow_app_image "${alice_choice_ctr}"
  # 在Bob域中注册SecretFlow应用镜像
  create_secretflow_app_image "${bob_choice_ctr}"
}

# ==================== 辅助函数：清理iptables规则 ====================

# 清理指定容器内的iptables防火墙规则
# 目的：移除可能阻止容器间通信的网络隔离规则
# 参数:
#   ctr: Docker容器ID或名称
function rm_iptables() {
  local ctr
  local pid
  local name

  ctr=$1
  
  # 获取容器的进程ID（PID），用于进入其网络命名空间
  pid=$(docker inspect --format '{{.State.Pid}}' "$ctr")
  
  # 获取容器名称并进行格式化转换
  # cut -b 2- 删除开头的 '/'
  # cut -d '.' -f1,2 提取前两部分（去除后缀）
  # sed 's/_/-/g' 将下划线替换为连字符
  name=$(docker inspect --format '{{.Name}}' "$ctr" | cut -b 2- | cut -d '.' -f1,2 | sed 's/_/-/g')
  
  # 查看当前iptables规则（详细输出）
  # nsenter -t $pid -n 进入容器的网络命名空间执行命令
  nsenter -t "$pid" -n iptables -L -n -v
  
  # 清空INPUT链的所有规则（允许所有入站流量）
  nsenter -t "$pid" -n iptables -F INPUT
  
  # 清空OUTPUT链的所有规则（允许所有出站流量）
  nsenter -t "$pid" -n iptables -F OUTPUT

  # 注释掉的命令：设置容器hostname（当前未启用）
  # nsenter -t $pid --uts hostname $name
}

# ==================== 辅助函数：创建Docker网络 ====================

# 创建Docker Swarm模式和Overlay网络
# Overlay网络允许不同宿主机上的容器相互通信
function create_network() {
  # 获取本机IP地址
  local hostname
  hostname=$(hostname -I | awk '{print $1}')

  # 定义网络名称
  network_name="kuscia-swarm-exchange"
  
  # 检查网络是否已存在
  # docker network ls 列出所有网络
  # grep -c 统计匹配的行数
  exists=$(docker network ls | grep -c $network_name)
  
  if [ "$exists" != "1" ]; then
    # 初始化Docker Swarm模式
    # --advertise-addr 指定Swarm对外通告的地址
    docker swarm init --advertise-addr "$hostname"
    
    # 创建Overlay网络
    # -d overlay 使用overlay驱动
    # --subnet 12.0.0.0/8 指定子网范围
    # --attachable 允许独立容器附加到此网络
    docker network create -d overlay --subnet 12.0.0.0/8 --attachable $network_name
  else
    echo "network $network_name exists!"
  fi
}

# ==================== 辅助函数：生成Kuscia配置文件 ====================

# 为Alice和Bob域生成Kuscia启动配置文件
# 配置文件包含域ID、运行时、日志级别、数据存储等关键参数
function create_kuscia_yaml() {
  # 获取本机IP地址（用于MySQL连接）
  local hostname
  hostname=$(hostname -I | awk '{print $1}')

  # --- 生成Alice域配置 ---
  # 如果alice目录不存在则创建
  if [ ! -d alice ]; then
    mkdir alice
  fi
  
  # 运行Kuscia初始化命令生成配置文件
  # --mode autonomy 自治模式（无中心节点）
  # --domain "alice" 域ID
  # --runtime "runp" 使用RunP运行时（支持隐私计算）
  # --log-level "DEBUG" 调试级别日志
  # --datastore-endpoint 指定MySQL数据库连接字符串
  # mysql://root:password@tcp($hostname:13307)/kine
  #   - 用户名: root
  #   - 密码: password
  #   - 地址: $hostname:13307（主机端口13307映射到MySQL的3306）
  #   - 数据库名: kine
  docker run -it --rm "${KUSCIA_IMAGE}" kuscia init --mode autonomy --domain "alice" --runtime "runp" --log-level "DEBUG" --datastore-endpoint "mysql://root:password@tcp($hostname:13307)/kine" > alice/kuscia.yaml 2>&1 || cat alice/kuscia.yaml

  # --- 生成Bob域配置 ---
  # 如果bob目录不存在则创建
  if [ ! -d bob ]; then
    mkdir bob
  fi
  
  # 运行Kuscia初始化命令生成Bob的配置文件
  # 注意：Bob使用不同的MySQL端口（13308）以避免冲突
  docker run -it --rm "${KUSCIA_IMAGE}" kuscia init --mode autonomy --domain "bob" --runtime "runp" --log-level "DEBUG" --datastore-endpoint "mysql://root:password@tcp($hostname:13308)/kine" > bob/kuscia.yaml 2>&1 || cat bob/kuscia.yaml
}

# ==================== 辅助函数：创建Docker Stack部署文件并启动 ====================

# 生成Docker Compose格式的Stack配置文件并部署
# 定义了Alice和Bob的多副本服务以及各自的MySQL数据库
function create_load() {
  # 获取脚本所在目录的绝对路径
  script_dir=$(realpath "$(dirname "$0")")
  
  # 使用heredoc生成Docker Stack配置文件
  cat << EOF > kuscia-autonomy.yaml
  version: '3.8'

  services:
    # --- Alice域服务（3个副本）---
    kuscia-autonomy-alice:
      image: $IMAGE                                    # 使用指定的Kuscia镜像
      command:
        - bin/kuscia                                   # 启动命令
        - start                                        # 启动子命令
        - -c                                           # 配置文件参数
        - etc/conf/kuscia.yaml                         # 配置文件路径
      environment:
        NAMESPACE: alice                               # Kubernetes命名空间
        PATH: '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:home/kuscia/tmp/bin:/home/kuscia/bin:/bin/aux'
      volumes:
        - /tmp:/tmp                                    # 挂载临时目录
        - $script_dir/alice/kuscia.yaml:/home/kuscia/etc/conf/kuscia.yaml  # 挂载配置文件
      ports:
        - "14869:1080/tcp"                             # 端口映射：主机14869 -> 容器1080
      networks:
        - kuscia-swarm-exchange                        # 连接到Overlay网络
      depends_on:
        - mysql-alice                                  # 依赖MySQL服务先启动
      deploy:
        replicas: 3                                    # 部署3个副本（分布式测试）

    # --- Bob域服务（1个副本）---
    kuscia-autonomy-bob:
      image: $IMAGE
      command:
        - bin/kuscia
        - start
        - -c
        - etc/conf/kuscia.yaml
      environment:
        NAMESPACE: bob
        PATH: '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:home/kuscia/tmp/bin:/home/kuscia/bin:/bin/aux'
      volumes:
        - /tmp:/tmp
        - $script_dir/bob/kuscia.yaml:/home/kuscia/etc/conf/kuscia.yaml
      ports:
        - "24869:1080/tcp"                             # 端口映射：主机24869 -> 容器1080
      networks:
      networks:
        - kuscia-swarm-exchange
      depends_on:
        - mysql-bob
      deploy:
        replicas: 1                                    # 部署1个副本

    # --- Alice的MySQL数据库 ---
    mysql-alice:
      image: mysql:8.0                                 # MySQL 8.0镜像
      environment:
        MYSQL_ROOT_PASSWORD: password                  # root用户密码
        MYSQL_DATABASE: kine                           # 初始创建的数据库名
        MYSQL_USER: user                               # 普通用户名
        MYSQL_PASSWORD: password                       # 普通用户密码
      ports:
        - "13307:3306"                                 # 端口映射：主机13307 -> 容器3306
      networks:
        - kuscia-swarm-exchange

    # --- Bob的MySQL数据库 ---
    mysql-bob:
      image: mysql:8.0
      environment:
        MYSQL_ROOT_PASSWORD: password
        MYSQL_DATABASE: kine
        MYSQL_USER: user
        MYSQL_PASSWORD: password
      ports:
        - "13308:3306"                                 # 端口映射：主机13308 -> 容器3306
      networks:
        - kuscia-swarm-exchange

  networks:
    kuscia-swarm-exchange:
      name: kuscia-swarm-exchange
      external: true                                   # 使用外部已创建的网络
EOF
  
  # 部署Docker Stack
  # -c 指定配置文件
  # kuscia-autonomy 是Stack的名称（会作为服务名前缀）
  docker stack deploy -c kuscia-autonomy.yaml kuscia-autonomy
}

# ==================== 辅助函数：清理副本 ====================

# 移除Docker Stack及其所有相关服务
# 用于重新部署前的清理工作
function clean_replica() {
  # docker stack rm 会停止并删除Stack中的所有服务
  docker stack rm kuscia-autonomy
}

# ==================== 主函数：执行完整部署流程 ====================

# 按顺序执行所有步骤来创建完整的测试集群
function run_replica() {
  # 步骤1：创建Docker Swarm和Overlay网络
  create_network
  
  # 步骤2：清理旧的Stack（如果存在）
  clean_replica
  
  # 步骤3：等待清理完成
  sleep 10
  
  # 步骤4：生成Kuscia配置文件
  create_kuscia_yaml
  
  # 步骤5：部署Docker Stack（启动所有容器）
  create_load
  
  # 步骤6：等待容器启动和服务就绪
  # 60秒足够让MySQL初始化、Kuscia启动并完成CRD注册
  sleep 60
  
  # 步骤7：构建副本间连接和配置（证书交换、路由配置等）
  build_replica_conn
}

# ==================== 脚本入口 ====================
# 执行主函数，开始部署流程
run_replica