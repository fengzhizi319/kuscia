# Kuscia 脚本目录

本目录集中存放 Kuscia 的部署、运维、模板、监控与测试脚本。所有脚本均以 Bash 编写并带有 Apache-2.0
许可证头，部署类脚本支持 `-h/--help`。更完整的部署背景可参考
[构建与部署文档](../docs/development/build_kuscia_cn.md)，脚本实现细节可参考
[部署脚本执行逻辑](deploy/KUSCIA_SH_EXECUTION_LOGIC.md)。

> **直接运行说明**：本文档中的 `bash` 代码块都是完整命令，可在 VS Code / Qoder 等支持
> Markdown 代码块运行的编辑器中点击运行按钮直接执行。除特别注明“容器内执行”外，命令默认在
> **Kuscia 仓库根目录** 执行。若从 clwork 工作区根目录浏览本文档，先进入 Kuscia 目录：

```bash
cd kuscia
pwd
```

## 目录结构

| 路径 | 用途 |
| --- | --- |
| `deploy/` | Docker 部署、域管理、路由、证书、AppImage、示例数据与监控脚本 |
| `make/` | Makefile 子模块：`common.mk`、`golang.mk`、`lint.mk`、`test.mk`、`image.mk`、`docs.mk`、`fate.mk` |
| `test/` | 脚本测试：离线语法检查、ShellCheck、shunit2 单元测试、Docker 集成测试入口 |
| `templates/` | Kuscia CRD、AppImage、路由、监控、SCQL 与示例任务模板 |
| `tools/register_app_image/` | 面向多节点部署的 AppImage 批量注册工具 |
| `user/` | 示例 KusciaJob 与 DataProxy 提交脚本 |
| `linter/` | codespell、license、markdown、yamllint 检查配置 |
| `yamllint/` | yamllint 依赖清单 |
| `build-kuscia-secretflow-images.sh` | 从源码构建 Kuscia 与 SecretFlow 联调镜像 |
| `run_local_kuscia.sh` | 不依赖 Docker，从源码启动本地 Master/Autonomy 节点 |

## 脚本速查

### 部署与生命周期

| 脚本 | 执行位置 | 功能 |
| --- | --- | --- |
| `deploy/deploy.sh` | 宿主机 | 部署单个 `master`、`lite` 或 `autonomy` 域 |
| `deploy/kuscia.sh` | 宿主机 / 容器内 | `center`、`p2p`、`cxc`、`cxp` 组网，以及 `start`、`upgrade`、`monitor` |
| `deploy/start_standalone.sh` | 宿主机 | 以固定端口启动 localhost 可访问的 center/p2p 集群 |
| `deploy/run_docker_quickstart.sh` | 宿主机 | 拉取镜像、提取 `kuscia.sh` 并一键启动集群 |
| `deploy/stop.sh` | 宿主机 | 停止 Kuscia 容器，保留容器与卷 |
| `deploy/uninstall.sh` | 宿主机 | 删除容器、卷与网络（不可逆） |
| `deploy/set_kernel_params.sh` | 宿主机 root | 调整 TCP backlog、TIME_WAIT 等内核参数 |
| `build-kuscia-secretflow-images.sh` | 宿主机 | 构建 Kuscia/SecretFlow 镜像，支持推送与导出 tar |
| `run_local_kuscia.sh` | 宿主机 | 从源码启动本地节点，用于非 Docker 开发 |

### 域、路由与证书

| 脚本 | 执行位置 | 功能 |
| --- | --- | --- |
| `deploy/add_domain.sh` | Master 容器 | 创建带证书的 Domain，支持 P2P 角色与互联协议 |
| `deploy/add_domain_lite.sh` | Master 容器 | 创建 Lite Domain 并等待 deploy token |
| `deploy/create_token.sh` | Master 容器 | 为 Domain 的 ServiceAccount 签发长期 Token |
| `deploy/join_to_host.sh` | 域容器 | 创建本域到宿主域的 DomainRoute，支持 MTLS/Token/None |
| `deploy/create_cluster_domain_route.sh` | Master 容器 | 创建 ClusterDomainRoute 跨域路由 |
| `deploy/create_reverse_tunnel_test_cluster.sh` | 宿主机 | 创建基于反向隧道的 P2P 测试集群（Docker Swarm） |
| `deploy/generate_rsa_key.sh` | 任意 Bash 环境 | 生成 base64 编码的域私钥 |
| `deploy/generate_cert.sh` | 任意 Bash 环境 | 由域私钥生成自签 `.key/.csr/.crt` |
| `deploy/gen_kusciaapi_token.sh` | 任意 Bash 环境 | 用域私钥为 Domain ID 生成 KusciaAPI Token |
| `deploy/init_kusciaapi_client_certs.sh` | 节点容器 | 基于 CA 生成 KusciaAPI 客户端证书（PKCS#8） |

### AppImage、数据与示例任务

| 脚本 | 执行位置 | 功能 |
| --- | --- | --- |
| `deploy/create_sf_app_image.sh` | 可访问集群的 kubectl 环境 | 分别传入镜像名、Tag、类型、ID 注册 AppImage |
| `deploy/create_secretflow_app_image.sh` | 可访问集群的 kubectl 环境 | 从 `repo:tag` 推断类型并注册 AppImage |
| `deploy/register_app_image.sh` | 宿主机 | 向运行中的 Kuscia 容器注册 AppImage，可导入镜像 |
| `tools/register_app_image/register_app_image.sh` | 宿主机 | 按部署模式批量向 Master/Lite 注册 AppImage |
| `deploy/create_domaindata_alice_table.sh` | Alice 域容器 | 创建 Alice 示例数据表 |
| `deploy/create_domaindata_bob_table.sh` | Bob 域容器 | 创建 Bob 示例数据表 |
| `deploy/create_domaindata_alice_privacy_table.sh` | Alice 域容器 | 创建隐私组件测试表（PII、准标识符、敏感属性） |
| `deploy/create_domaindata_bob_privacy_table.sh` | Bob 域容器 | 创建 Bob 隐私组件测试表 |
| `deploy/init_example_data.sh` | 域容器 | 通过 KusciaAPI 创建示例数据并授权给对方域 |
| `deploy/create_secretpad_svc.sh` | Master 容器 | 为 SecretPad 容器创建 Kubernetes Service |
| `user/create_example_job.sh` | Master/Autonomy 容器 | 提交 PSI、PSI_WITH_DP、NSJAIL_PSI 示例任务 |
| `user/create_dp.sh` | Master/Autonomy 容器 | 创建 DataProxy Serving 示例 |

### 监控与本地运行

| 脚本 | 执行位置 | 功能 |
| --- | --- | --- |
| `deploy/kuscia.sh monitor` | Master/Autonomy 容器 | 在集群内部署 Prometheus/Grafana 监控组件 |
| `deploy/start_monitor.sh` | 宿主机 | 以独立容器启动 Kuscia Monitor |
| `deploy/init_kuscia_monitor.sh` | 监控容器 | 监控镜像入口，初始化 Prometheus 数据源与 Grafana 面板 |
| `run_local_kuscia.sh` | 宿主机 | 从源码构建并启动本地 Master/Autonomy |

### 测试

| 脚本 | 执行位置 | 功能 |
| --- | --- | --- |
| `test/run_local_tests.sh` | 宿主机 | `bash -n` + ShellCheck + shunit2 单元测试，无需 Docker |
| `test/run_integration_tests.sh` | 宿主机 | Docker 集成测试包装器，支持套件选择与平台过滤 |
| `test/integration_test.sh` | 宿主机 | 兼容入口，等价于 `run_integration_tests.sh` |
| `test/unit/test_app_image.sh` | 宿主机 | AppImage 模板渲染与参数校验测试 |
| `test/unit/test_generate_cert.sh` | 宿主机 | 私钥、证书生成与参数校验测试 |
| `test/unit/test_cli_contract.sh` | 宿主机 | 部署脚本帮助、模式与必填参数的 CLI 契约测试 |

### 构建与代码生成

| 脚本 | 执行位置 | 功能 |
| --- | --- | --- |
| `hack/build.sh` | 宿主机 | 编译 `kuscia` / `transport` 二进制到 `build/apps/` |
| `hack/generate-crds.sh` | 宿主机 | 用 controller-gen 生成 `crds/v1alpha1` |
| `hack/update-codegen.sh` | 宿主机 | 生成 CRD clientset、informer、lister、deepcopy |
| `hack/generate-groups.sh` | 宿主机 | 通用 client-gen 包装器，由 `update-codegen.sh` 调用 |
| `hack/proto-to-go.sh` | 宿主机 | 由 `.proto` 生成 Go/gRPC 代码 |
| `hack/version_check.sh` | 宿主机 | 检查或修正 Markdown 中的 Kuscia/SecretFlow 版本号 |
| `hack/errorcode/gen_error_code_doc.sh` | 宿主机 | 由错误码 proto 生成/校验错误码文档 |
| `hack/k3s/build.sh` | 宿主机 | 克隆并构建内嵌 K3s 二进制 |
| `hack/k3s/build-kuscia-k3s-binary.sh` | K3s 源码目录 | 被 K3s Makefile 调用，注入 Kuscia 定制构建参数 |

## 执行环境约定

不同脚本依赖的运行时环境不同，执行前请先确认位置：

| 位置 | 判断依据 | 典型脚本 |
| --- | --- | --- |
| 宿主机 | 需要 `docker`，不使用集群内 `kubectl` | `deploy.sh`、`kuscia.sh center/p2p/...`、`stop.sh`、`start_monitor.sh` |
| Master 容器 | 脚本中的 `kubectl` 指向 Master 的 K3s | `add_domain.sh`、`add_domain_lite.sh`、`create_token.sh`、`create_cluster_domain_route.sh` |
| 域容器 | 脚本中的 `kubectl` 指向该域的 K3s | `create_domaindata_*.sh`、`create_example_job.sh`、`init_example_data.sh` |
| 节点容器 | 依赖 `/home/kuscia/var/certs` 下的 CA | `init_kusciaapi_client_certs.sh` |
| 监控容器 | 监控镜像入口 | `init_kuscia_monitor.sh` |

进入容器的命令（容器名默认以当前系统用户名为前缀，即 `${USER}-kuscia-*`）：

```bash
docker exec -it "${USER}-kuscia-master" bash
```

```bash
docker exec -it "${USER}-kuscia-lite-alice" bash
```

```bash
docker exec -it "${USER}-kuscia-autonomy-alice" bash
```

如果容器由其他用户创建，请把命令中的 `${USER}` 替换为实际用户名。部分脚本支持通过
`KUSCIA_CTR_PREFIX` 覆盖容器名前缀。

## 快速开始

### Docker 一键组网

中心化组网（1 Master + 2 Lite）：

```bash
bash scripts/deploy/kuscia.sh center
```

点对点组网（2 Autonomy）：

```bash
bash scripts/deploy/kuscia.sh p2p
```

确认容器状态：

```bash
docker ps --format 'table {{.Names}}\t{{.Status}}'
```

提交示例 PSI 任务（中心化模式在 Master 容器执行，P2P 模式在 Alice Autonomy 容器执行）：

```bash
docker exec -it "${USER}-kuscia-master" scripts/user/create_example_job.sh PSI
```

```bash
docker exec -it "${USER}-kuscia-autonomy-alice" scripts/user/create_example_job.sh PSI
```

查看任务状态：

```bash
docker exec -it "${USER}-kuscia-master" kubectl get kj -n cross-domain
```

### 交互式快速启动

`run_docker_quickstart.sh` 会自动拉取镜像、从镜像中提取 `kuscia.sh` 到当前目录并启动集群：

```bash
bash scripts/deploy/run_docker_quickstart.sh center
```

```bash
bash scripts/deploy/run_docker_quickstart.sh p2p
```

### 不使用 Docker 本地启动

需要预装 `go`、`make`、`gcc`、`git`、`openssl`，部分环境还需 root 权限监听 53 端口：

```bash
bash scripts/run_local_kuscia.sh master
```

```bash
DOMAIN_ID=alice bash scripts/run_local_kuscia.sh autonomy
```

```bash
bash scripts/run_local_kuscia.sh --stop
```

### 从源码构建镜像

```bash
bash scripts/build-kuscia-secretflow-images.sh -p all -v 1.15.0-dev
```

## 部署与生命周期

### deploy.sh：部署单个域

支持 `master`、`lite`、`autonomy` 三种模式。Master 容器名为 `${USER}-kuscia-master`；Lite 与
Autonomy 容器名为 `${USER}-kuscia-<mode>-<domain-id>`。

| 选项 | 说明 |
| --- | --- |
| `-n` | Domain ID。`master` 默认 `kuscia-system`，`lite`/`autonomy` 必填 |
| `-p` | 跨域网关映射到宿主机的端口，必填 |
| `-q` | 内部应用端口映射，默认 `13081`（lite/autonomy） |
| `-m` | Master 端点，仅 `lite` 必填，例如 `https://1.2.3.4:18080` |
| `-t` | Lite 部署 Token，仅 `lite` 必填 |
| `-r` | 安装根目录，默认当前目录 |
| `-d` | 数据目录，默认 `<ROOT>/<容器名>/data` |
| `-l` | 日志目录，默认 `<ROOT>/<容器名>/logs` |
| `-c` | 宿主机上的 `kuscia.yaml` 路径，会挂载进容器 |
| `-k` | KusciaAPI HTTP 端口，默认 `13082` |
| `-g` | KusciaAPI gRPC 端口，默认 `13083` |
| `-e` | KusciaAPI 服务端证书的额外 subjectAltName（master/autonomy） |
| `-h` | 显示帮助 |

对应环境变量：`DOMAIN_ID`、`DOMAIN_HOST_PORT`、`DOMAIN_HOST_INTERNAL_PORT`、`MASTER_ENDPOINT`、
`DOMAIN_TOKEN`、`ROOT`、`DOMAIN_DATA_DIR`、`DOMAIN_LOG_DIR`、`KUSCIAAPI_HTTP_PORT`、
`KUSCIAAPI_GRPC_PORT`。

查看帮助：

```bash
bash scripts/deploy/deploy.sh -h
```

部署 Master：

```bash
bash scripts/deploy/deploy.sh master -n kuscia-system -p 18080
```

在 Master 中注册 Lite 域并取得部署 Token：

```bash
ALICE_TOKEN="$(docker exec -i "${USER}-kuscia-master" scripts/deploy/add_domain_lite.sh alice kuscia-system | tr -d '\r\n')"
printf 'ALICE_TOKEN=%s\n' "${ALICE_TOKEN}"
```

部署 Lite（与 Master 同机时 `MASTER_ENDPOINT` 使用 `127.0.0.1`，跨机时改为 Master 的地址）：

```bash
MASTER_ENDPOINT=https://127.0.0.1:18080
ALICE_TOKEN="$(docker exec -i "${USER}-kuscia-master" scripts/deploy/add_domain_lite.sh alice kuscia-system | tr -d '\r\n')"
bash scripts/deploy/deploy.sh lite -n alice -p 28080 -m "${MASTER_ENDPOINT}" -t "${ALICE_TOKEN}"
```

部署 Autonomy：

```bash
bash scripts/deploy/deploy.sh autonomy -n alice -p 11080
```

### kuscia.sh：多模式集群与运维

| 模式 | 拓扑 | 默认容器名 |
| --- | --- | --- |
| `center` | 1 Master + Alice/Bob 2 Lite | `${USER}-kuscia-master`、`${USER}-kuscia-lite-alice`、`${USER}-kuscia-lite-bob` |
| `p2p` | 2 Autonomy 对等互联 | `${USER}-kuscia-autonomy-alice`、`${USER}-kuscia-autonomy-bob` |
| `cxc` | 2 套 Master+Lite 互联 | `${USER}-kuscia-master-cxc-alice`、`${USER}-kuscia-master-cxc-bob`、`${USER}-kuscia-lite-cxc-alice`、`${USER}-kuscia-lite-cxc-bob` |
| `cxp` | Center 与 P2P 混合 | `${USER}-kuscia-master-cxp-alice`、`${USER}-kuscia-lite-cxp-alice`、`${USER}-kuscia-autonomy-cxp-bob` |
| `start` | 按配置文件启动单个节点 | 由 `--name` 与配置决定 |
| `upgrade` | 用新镜像重建已有容器 | 参数为容器名 |
| `monitor` | 在域内安装监控组件 | `monitor` Pod |

常用选项：

| 选项 | 说明 |
| --- | --- |
| `-a` | 导入 SecretFlow 镜像，`none` 表示不导入 |
| `-c` | 宿主机配置文件路径 |
| `-d` / `-l` / `-s` | 数据、日志、K3s 数据目录 |
| `-m` | 内存限制，例如 `4GiB`；`-1` 表示不限制 |
| `-p` / `-q` | 跨域网关与内部端口 |
| `-t` | 开启网关路由转发能力 |
| `-k` / `-g` / `-x` | KusciaAPI HTTP、gRPC 与 Metrics 端口 |
| `-P` | 互联协议：`kuscia` 或 `bfia` |
| `--expose-ports` | 为集群模式暴露端口 |
| `--cluster` | `start` 模式使用 overlay 集群网络 |
| `--rootless` | 以非 root 用户运行（受运行时限制） |
| `--data-proxy` | 同时部署 DataProxy |
| `--runp` / `--runk` | 指定运行时 / 监控 Lite 采用 runk |
| `-h, --help` | 显示帮助 |

查看帮助：

```bash
bash scripts/deploy/kuscia.sh --help
```

启动 center、p2p、cxc、cxp：

```bash
bash scripts/deploy/kuscia.sh center
```

```bash
bash scripts/deploy/kuscia.sh p2p -P kuscia
```

```bash
bash scripts/deploy/kuscia.sh cxc -t
```

```bash
bash scripts/deploy/kuscia.sh cxp -t
```

按配置文件启动单节点（`start` 模式）：

```bash
bash scripts/deploy/kuscia.sh start -c ./kuscia.yaml -p 28080 -k 28081 -g 28082 -q 28083 -x 28084
```

升级已有容器（参数为容器名，会保留卷、端口、网络与内存配置）：

```bash
KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0 \
  bash scripts/deploy/kuscia.sh upgrade "${USER}-kuscia-lite-alice"
```

### start_standalone.sh：localhost 固定端口集群

与 `kuscia.sh` 不同，该脚本默认把端口映射到宿主机，适合本地联调。

| 模式 | Master / 节点端口映射 |
| --- | --- |
| `center`（默认） | Master `18080->1080`、`18082->8082`、`18083->8083`；Alice `28080~28083`；Bob `38080~38083` |
| `p2p` | Alice `11081->80`、`11082->8082`、`11083->8083`；Bob `12081->80`、`12082->8082`、`12083->8083` |
| `cxc` / `cxp` | 第一套网络 Master `8080/8082/8083`；第二套 Master 使用 `18080` 起始；Lite 使用 `28080`、`38080` 起始的端口段 |

常用选项：

| 选项 | 说明 |
| --- | --- |
| `-p kuscia\|bfia` | 互联协议，默认 `kuscia`；center 模式仅支持 `kuscia` |
| `-P NOTLS\|TLS\|MTLS` | KusciaAPI/网关协议，默认 `TLS` |
| `-t` | 开启 transit 转发 |
| `-h` | 显示帮助 |

可通过 `KUSCIA_GATEWAY_HOST_PORT`、`KUSCIA_API_HTTP_HOST_PORT`、`KUSCIA_API_GRPC_HOST_PORT`、
`KUSCIA_ALICE_HOST_PORT_BASE`、`KUSCIA_BOB_HOST_PORT_BASE` 覆盖端口。

```bash
bash scripts/deploy/start_standalone.sh center
```

```bash
bash scripts/deploy/start_standalone.sh p2p -p bfia
```

```bash
bash scripts/deploy/start_standalone.sh cxc -t
```

### run_docker_quickstart.sh：自动提取部署脚本

默认模式为 `p2p`，支持 `p2p`、`center`、`cxc`、`cxp`。脚本会在当前目录生成 `kuscia.sh`。

```bash
bash scripts/deploy/run_docker_quickstart.sh
```

```bash
KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0 \
  bash scripts/deploy/run_docker_quickstart.sh center
```

### stop.sh：停止容器

按容器名前缀停止，不删除容器、卷与网络。

```bash
bash scripts/deploy/stop.sh center
```

```bash
bash scripts/deploy/stop.sh p2p
```

```bash
bash scripts/deploy/stop.sh all
```

### uninstall.sh：清理容器、卷与网络

**警告：该操作不可逆，会删除 Kuscia 容器的本地数据卷。** 脚本在存在运行中容器时会要求确认。

```bash
bash scripts/deploy/uninstall.sh center
```

```bash
bash scripts/deploy/uninstall.sh p2p
```

```bash
bash scripts/deploy/uninstall.sh all
```

### set_kernel_params.sh：内核网络参数

需要 root 权限，写入 `/proc/sys`：

- `net.ipv4.tcp_max_syn_backlog=2048`
- `net.core.somaxconn=2048`
- `net.ipv4.tcp_retries2=5`
- `net.ipv4.tcp_slow_start_after_idle=0`
- `net.ipv4.tcp_tw_reuse=1`
- `fs.file-max=102400`

```bash
sudo bash scripts/deploy/set_kernel_params.sh
```

验证：

```bash
sysctl net.core.somaxconn net.ipv4.tcp_max_syn_backlog net.ipv4.tcp_tw_reuse
```

### build-kuscia-secretflow-images.sh：构建联调镜像

| 选项 | 说明 |
| --- | --- |
| `-p, --project` | `kuscia`、`secretflow` 或 `all`，默认 `all` |
| `-v, --version` | 镜像版本；Kuscia 默认取 `git describe`，SecretFlow 默认 `dev-<datetime>` |
| `-a, --arch` | Kuscia 目标架构：`amd64` / `arm64` |
| `-r, --registry` | 镜像仓库前缀 |
| `-l, --latest` | 额外打 `latest` 标签 |
| `--push` | 构建后推送（需先 `docker login`） |
| `-t, --tar` | 构建后导出 tar |
| `--sf-wheel` | 使用预构建的 SecretFlow wheel |

```bash
bash scripts/build-kuscia-secretflow-images.sh -p kuscia
```

```bash
bash scripts/build-kuscia-secretflow-images.sh -p secretflow -v 1.15.0-dev
```

```bash
bash scripts/build-kuscia-secretflow-images.sh -p all -v 1.15.0-dev --tar
```

```bash
bash scripts/build-kuscia-secretflow-images.sh -p all \
  -r registry.example.com/secretflow -v 1.15.0-dev --push
```

## 域、互联与证书

### add_domain.sh：创建带证书的 Domain

用法：`add_domain.sh DOMAIN_ID [ROLE] [INTERCONN_PROTOCOL] [MASTER_DOMAIN_ID]`

- `ROLE=p2p` 时创建 `partner` 角色，并写入 `interConnProtocols`；
- 需要把 `<DOMAIN_ID>.domain.crt` 放到容器的 `/home/kuscia/var/certs/`；
- `MASTER_DOMAIN_ID` 默认等于 `DOMAIN_ID`。

```bash
docker cp ./bob.domain.crt "${USER}-kuscia-master":/home/kuscia/var/certs/bob.domain.crt
docker exec -it "${USER}-kuscia-master" scripts/deploy/add_domain.sh bob p2p kuscia kuscia-system
```

### add_domain_lite.sh：创建 Lite Domain 并获取 Token

用法：`add_domain_lite.sh DOMAIN_ID [MASTER_DOMAIN_ID]`。脚本会轮询 Domain 状态，最多等待约 60 秒，
并输出未使用的 deploy token。

```bash
docker exec -it "${USER}-kuscia-master" scripts/deploy/add_domain_lite.sh alice kuscia-system
```

Token 遗失时可重新查询：

```bash
docker exec -it "${USER}-kuscia-master" kubectl get domain alice \
  -o=jsonpath='{.status.deployTokenStatuses[?(@.state=="unused")].token}'
```

### create_token.sh：签发 ServiceAccount Token

为指定 Domain 的 ServiceAccount 签发有效期 `87600h`（约 10 年）的 Token：

```bash
docker exec -it "${USER}-kuscia-master" scripts/deploy/create_token.sh alice
```

### join_to_host.sh：建立域间路由

用法：`join_to_host.sh SELF_DOMAIN_ID HOST_DOMAIN_ID HOST_ENDPOINT [OPTIONS]`

| 选项 | 说明 |
| --- | --- |
| `-p` | 互联协议，默认 `kuscia` |
| `-a` | 认证方式：`MTLS`（默认）、`Token`、`None` |
| `-t` | `Token` 认证时使用的 Token |
| `-x` | transit 中转域 |
| `-i` | 是否需要 interop 配置，默认 `true` |
| `-k` | HTTPS 时允许不安全连接 |
| `-h` | 显示帮助 |

MTLS 互联：

```bash
docker exec -it "${USER}-kuscia-autonomy-alice" \
  scripts/deploy/join_to_host.sh alice bob https://192.168.1.10:24869
```

Token 认证：

```bash
BOB_TOKEN=replace-with-token
docker exec -it "${USER}-kuscia-autonomy-alice" \
  scripts/deploy/join_to_host.sh alice bob https://192.168.1.10:24869 -a Token -t "${BOB_TOKEN}"
```

### create_cluster_domain_route.sh：创建 ClusterDomainRoute

用法：`create_cluster_domain_route.sh SRC_DOMAIN DEST_DOMAIN DEST_ENDPOINT [TRANSIT_DOMAIN]`

```bash
docker exec -it "${USER}-kuscia-master" \
  scripts/deploy/create_cluster_domain_route.sh alice bob http://"${USER}-kuscia-lite-bob":1080
```

经中转域路由：

```bash
docker exec -it "${USER}-kuscia-master" \
  scripts/deploy/create_cluster_domain_route.sh alice bob https://192.168.1.10:24869 master-cxc-alice
```

### create_reverse_tunnel_test_cluster.sh：反向隧道测试集群

脚本会执行以下操作：

- 初始化 Docker Swarm，创建 overlay 网络 `kuscia-swarm-exchange`；
- 以 Docker Stack 名称 `kuscia-autonomy` 部署 Alice（3 副本）与 Bob（1 副本）；
- 启动两个 MySQL 8.0 容器作为 Kine 存储（端口 `13307`、`13308`）；
- 交换 TLS 证书、清理容器 iptables、创建 Domain 与跨域路由；
- 注册示例数据与 SecretFlow AppImage。

前置条件：Docker Swarm 可用；`KUSCIA_IMAGE` 必填；`IMAGE` 为 Stack 中实际使用的镜像，未设置时
默认与 `KUSCIA_IMAGE` 相同。

```bash
export KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0
bash scripts/deploy/create_reverse_tunnel_test_cluster.sh
```

清理测试 Stack：

```bash
docker stack rm kuscia-autonomy
docker network rm kuscia-swarm-exchange
```

### generate_rsa_key.sh：生成域私钥

输出 base64 编码的 2048 位 RSA 私钥。脚本带有彩色提示文本，取第 3 行即为私钥：

```bash
DOMAIN_KEY="$(bash scripts/deploy/generate_rsa_key.sh | sed -n '3p')"
printf '%s\n' "${DOMAIN_KEY}"
```

### generate_cert.sh：生成自签证书

用法：`generate_cert.sh DOMAIN_ID DOMAIN_KEY_DATA`，在当前目录生成
`<DOMAIN_ID>.domain.key`、`.csr`、`.crt`，有效期 36500 天。

```bash
DOMAIN_KEY="$(bash scripts/deploy/generate_rsa_key.sh | sed -n '3p')"
bash scripts/deploy/generate_cert.sh alice "${DOMAIN_KEY}"
openssl x509 -in alice.domain.crt -noout -subject -dates
```

### gen_kusciaapi_token.sh：生成 KusciaAPI Token

用法：`gen_kusciaapi_token.sh DOMAIN_ID DOMAIN_KEY_DATA`。Token 取 SHA-256 签名 base64 结果的前
32 个字符，与服务端约定一致。

```bash
DOMAIN_KEY="$(bash scripts/deploy/generate_rsa_key.sh | sed -n '3p')"
KUSCIA_TOKEN="$(bash scripts/deploy/gen_kusciaapi_token.sh alice "${DOMAIN_KEY}")"
printf 'KUSCIA_TOKEN=%s\n' "${KUSCIA_TOKEN}"
```

### init_kusciaapi_client_certs.sh：生成 KusciaAPI 客户端证书

读取 `${ROOT}/var/certs/ca.crt` 与 `ca.key`，生成 PKCS#8 客户端私钥、CSR 与由 CA 签发的证书。
在节点容器内执行时 `${ROOT}` 为 `/home/kuscia`：

```bash
docker exec -it "${USER}-kuscia-master" scripts/deploy/init_kusciaapi_client_certs.sh
```

生成结果位于 `/home/kuscia/var/certs/`：

```bash
docker exec -it "${USER}-kuscia-master" ls -l /home/kuscia/var/certs/kusciaapi-client.*
```

## AppImage、数据与示例任务

### create_sf_app_image.sh：分参数注册 AppImage

用法：`create_sf_app_image.sh SF_IMAGE_NAME SF_IMAGE_TAG [APP_TYPE] [SF_IMAGE_ID]`

- `APP_TYPE` 决定模板 `scripts/templates/app_image.<APP_TYPE>.yaml`，默认 `secretflow`；
- `SF_IMAGE_ID` 主要用于 PSI 模板；
- 可通过 `SF_NAME` 环境变量覆盖 AppImage 资源名，默认 `secretflow-image`；
- 需要 `kubectl` 能访问目标集群，通常在 Master 容器内执行。

注册 SecretFlow：

```bash
docker exec -it "${USER}-kuscia-master" \
  scripts/deploy/create_sf_app_image.sh \
  secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite-anolis8 \
  1.11.0b1 secretflow
```

注册 PSI 并指定资源名与镜像 ID：

```bash
docker exec -it "${USER}-kuscia-master" \
  env SF_NAME=psi-image \
  scripts/deploy/create_sf_app_image.sh \
  secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/psi-anolis8 \
  1.11.0b1 psi sha256:replace-with-image-id
```

查看结果：

```bash
docker exec -it "${USER}-kuscia-master" kubectl get appimage
```

### create_secretflow_app_image.sh：从 `repo:tag` 推断类型

用法：`create_secretflow_app_image.sh SF_IMAGE_NAME`，镜像必须包含 Tag。镜像名最后一段以 `psi-`
开头时注册为 PSI，其余注册为 SecretFlow。同样支持 `SF_NAME` 覆盖资源名。

```bash
docker exec -it "${USER}-kuscia-master" \
  scripts/deploy/create_secretflow_app_image.sh \
  secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite-anolis8:1.11.0b1
```

```bash
docker exec -it "${USER}-kuscia-master" \
  env SF_NAME=psi-image \
  scripts/deploy/create_secretflow_app_image.sh \
  secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/psi-anolis8:1.11.0b1
```

### deploy/register_app_image.sh：向运行中的容器注册

| 选项 | 说明 |
| --- | --- |
| `-c` | Kuscia 容器名，必填 |
| `-i` | 应用镜像地址，必填 |
| `-n` | AppImage 资源名，默认 `secretflow-image` |
| `-f` | 自定义 AppImage 模板路径 |
| `-m` | 使用内置模板，并通过宿主机 `kubectl` 应用 |
| `--import` | 将宿主机镜像导入容器（`docker save` + `kuscia image load`） |
| `-h` | 显示帮助 |

使用内置模板并导入镜像：

```bash
bash scripts/deploy/register_app_image.sh \
  -c "${USER}-kuscia-autonomy-alice" \
  -i secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite-anolis8:1.11.0b1 \
  -m -n secretflow-image --import
```

使用自定义模板：

```bash
bash scripts/deploy/register_app_image.sh \
  -c "${USER}-kuscia-autonomy-alice" \
  -i registry.example.com/team/custom-engine:1.0.0 \
  -f ./my-app-image.yaml --import
```

### tools/register_app_image/register_app_image.sh：按部署模式批量注册

| 选项 | 说明 |
| --- | --- |
| `-i` | 应用镜像地址，必填 |
| `-m` | 部署模式：`center` 或 `p2p`，默认 `p2p` |
| `-d` | 域 ID 列表，默认 `alice,bob` |
| `-u` | 部署用户，默认当前 `${USER}` |
| `-n` | AppImage 资源名，未指定时自动生成 |
| `-f` | 自定义模板路径 |
| `-h` | 显示帮助 |

```bash
bash scripts/tools/register_app_image/register_app_image.sh \
  -u "${USER}" -m center -d alice,bob -n secretflow-image \
  -f scripts/tools/register_app_image/secretflow-image.yaml \
  -i secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite-anolis8:1.11.0b1
```

### create_domaindata_*.sh：创建示例 DomainData

四个脚本用法一致：`create_domaindata_<domain>_<kind>.sh DOMAIN_ID`，通过 `kubectl apply` 创建
DomainData CRD。普通表用于 PSI 示例，隐私表包含 PII、准标识符、敏感属性与数值列，用于
K-匿名、差分隐私、脱敏、L-Diversity 与 LDP 测试。

```bash
docker exec -it "${USER}-kuscia-lite-alice" scripts/deploy/create_domaindata_alice_table.sh alice
docker exec -it "${USER}-kuscia-lite-bob" scripts/deploy/create_domaindata_bob_table.sh bob
```

```bash
docker exec -it "${USER}-kuscia-lite-alice" scripts/deploy/create_domaindata_alice_privacy_table.sh alice
docker exec -it "${USER}-kuscia-lite-bob" scripts/deploy/create_domaindata_bob_privacy_table.sh bob
```

### init_example_data.sh：创建数据并跨域授权

通过 `https://127.0.0.1:8082` 调用 KusciaAPI 创建 `alice-table` 或 `bob-table`，并在 DataMesh
上把数据授权给对方域。必须在对应域的容器内执行，因为脚本依赖容器内 `var/certs` 与本地 API 端口。

```bash
docker exec -it "${USER}-kuscia-lite-alice" scripts/deploy/init_example_data.sh alice
docker exec -it "${USER}-kuscia-lite-bob" scripts/deploy/init_example_data.sh bob
```

### create_secretpad_svc.sh：创建 SecretPad Service

用法：`create_secretpad_svc.sh SECRET_PAD_CTR_NAME [DOMAIN_ID]`。第一个参数是运行中的
SecretPad 容器名，第二个参数用于模板中的域名/命名空间占位符。

```bash
docker exec -it "${USER}-kuscia-master" scripts/deploy/create_secretpad_svc.sh secretpad alice
```

### user/create_example_job.sh：提交示例任务

用法：`create_example_job.sh [PSI|PSI_WITH_DP|NSJAIL_PSI] [JOB_NAME]`

- 默认 `PSI`，任务名默认 `secretflow-task-<时间戳>`；
- `PSI_WITH_DP` 使用带 DataProxy 的输入配置；
- `NSJAIL_PSI` 需要部署时设置 `ALLOW_PRIVILEGED=true`；
- 任务提交到 `cross-domain` 命名空间。

```bash
docker exec -it "${USER}-kuscia-master" scripts/user/create_example_job.sh PSI
```

```bash
docker exec -it "${USER}-kuscia-autonomy-alice" scripts/user/create_example_job.sh PSI_WITH_DP my-psi-job
```

查看任务：

```bash
docker exec -it "${USER}-kuscia-master" kubectl get kj -n cross-domain
```

### user/create_dp.sh：创建 DataProxy Serving

用法：`create_dp.sh DOMAIN_ID`，通过 `kubectl apply` 创建 `dataproxy` KusciaDeployment。

```bash
docker exec -it "${USER}-kuscia-master" scripts/user/create_dp.sh alice
```

## 监控

Kuscia 提供两种监控部署方式：集群内监控（`kuscia.sh monitor`）与独立容器监控
（`start_monitor.sh`）。

### kuscia.sh monitor：集群内监控

在 Master 容器内为指定 Lite 域部署监控：

```bash
docker exec -it "${USER}-kuscia-master" scripts/deploy/kuscia.sh monitor alice
```

在 Autonomy 容器内为自身部署监控：

```bash
docker exec -it "${USER}-kuscia-autonomy-alice" scripts/deploy/kuscia.sh monitor
```

部署成功后，可在对应 Lite 容器查询 Prometheus targets：

```bash
docker exec -it "${USER}-kuscia-lite-alice" \
  curl -kv -H 'Host: prometheus.alice.svc' -H 'Kuscia-Source: alice' \
  http://localhost:80/api/v1/targets
```

### start_monitor.sh：独立监控容器

用法：`start_monitor.sh MODE [NETWORK]`，`MODE` 为 `center` 或 `p2p`，`NETWORK` 默认
`kuscia-exchange`。容器已存在时会询问是否重建。

| 模式 | 容器 | 宿主机端口 |
| --- | --- | --- |
| `center` | `${USER}-kuscia-monitor-center` | `9090->9090`、`3000->3000` |
| `p2p` | `${USER}-kuscia-monitor-alice` | `9089->9090`、`3000->3000` |
| `p2p` | `${USER}-kuscia-monitor-bob` | `9090->9090`、`3001->3000` |

```bash
bash scripts/deploy/start_monitor.sh center
```

```bash
bash scripts/deploy/start_monitor.sh p2p kuscia-exchange
```

访问 Grafana：`http://127.0.0.1:3000`（p2p 的 Bob 使用 `3001`），默认账号 `admin/admin`。
宿主机侧监控配置写入 `$HOME/kuscia/<容器名>/prometheus.yml`。

### init_kuscia_monitor.sh：监控镜像入口

该脚本是 `secretflow/kuscia-monitor` 镜像的启动入口，一般由 `start_monitor.sh` 或
`kuscia.sh monitor` 自动调用，无需手动执行。它负责：

1. 启动 Prometheus（`9090`，配置 `/home/config/prometheus.yml`）；
2. 启动 Grafana（`3000`）；
3. 查询 Grafana 中 `Kuscia-monitor` 数据源并写入 `machine.json` 的 `{{Kuscia-datasource}}`；
4. 启用 Grafana dashboard provisioning 后重启 Grafana。

排障时查看日志：

```bash
docker logs -f "${USER}-kuscia-monitor-center"
```

## 本地源码运行

### run_local_kuscia.sh

不依赖 Docker，直接在宿主机编译并运行 Kuscia：

| 参数 / 环境变量 | 说明 |
| --- | --- |
| `master` | 启动本地 Master，默认模式 |
| `autonomy` | 启动本地 Autonomy |
| `--stop` | 停止由 PID 文件记录的进程 |
| `KUSCIA_HOME` | 工作目录，默认 `<repo>/.local-kuscia` |
| `DOMAIN_ID` | 域 ID，默认与模式相同 |
| `-h, --help` | 显示帮助 |

```bash
bash scripts/run_local_kuscia.sh master
```

```bash
DOMAIN_ID=alice bash scripts/run_local_kuscia.sh autonomy
```

```bash
bash scripts/run_local_kuscia.sh --stop
```

运行文件位于 `${KUSCIA_HOME:-.local-kuscia}`，该目录已被 `.gitignore` 忽略。

## 测试

脚本测试分为三层，统一说明见 [scripts/test/README.md](test/README.md)。

### 离线检查与单元测试

```bash
bash scripts/test/run_local_tests.sh
```

```bash
bash scripts/test/run_local_tests.sh --lint-only
```

```bash
bash scripts/test/run_local_tests.sh --unit-only
```

```bash
bash scripts/test/run_local_tests.sh --unit-only --filter cert -v
```

| 选项 | 说明 |
| --- | --- |
| `--lint-only` | 仅执行 `bash -n` 与 ShellCheck |
| `--unit-only` | 仅执行 shunit2 单元测试 |
| `--no-shellcheck` | 跳过 ShellCheck |
| `--filter PATTERN` | 仅运行路径包含 `PATTERN` 的单元测试文件 |
| `-v, --verbose` | 输出每个被检查的文件 |
| `-h, --help` | 显示帮助 |

ShellCheck 范围与 `make shell-check` 保持一致（`scripts/**/*.sh`、`hack/**/*.sh`），并额外检查
`scripts/test/unit/*.sh`；统一忽略 `SC1091,SC2034`。

### Docker 集成测试

```bash
KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0 \
  bash scripts/test/run_integration_tests.sh --suite center.base
```

```bash
KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0 \
  bash scripts/test/run_integration_tests.sh
```

```bash
bash scripts/test/run_integration_tests.sh --list
```

| 选项 / 变量 | 说明 |
| --- | --- |
| `-s, --suite NAME` | 套件名，默认 `$TEST_SUITE` 或 `all` |
| `-i, --image IMAGE` | 被测镜像，默认 `$KUSCIA_IMAGE` |
| `-l, --list` | 列出当前平台可用套件 |
| `KUSCIA_IMAGE` | 被测 Kuscia 镜像 |
| `TEST_SUITE` | 默认套件名 |

套件：`center.base`、`p2p.base`、`center.example`、`bfia.base`、`center.nsjail`、`p2p.nsjail`。
`bfia.base` 在 arm64 上自动移除；`*.nsjail` 在 WSL 上会被上游 runner 跳过。

`test/integration_test.sh` 是兼容入口，参数与包装器一致：

```bash
KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0 \
  bash scripts/test/integration_test.sh --suite p2p.base
```

### 单元测试文件

| 文件 | 覆盖内容 |
| --- | --- |
| `unit/test_app_image.sh` | 模板渲染、`SF_NAME` 覆盖、PSI 模板、缺参与无 Tag 镜像拒绝 |
| `unit/test_generate_cert.sh` | 私钥可解码、证书生成、参数校验（无 openssl 时跳过） |
| `unit/test_cli_contract.sh` | `deploy.sh`、`kuscia.sh`、`add_domain*.sh`、`create_token.sh`、`run_local_kuscia.sh` 的 CLI 契约 |

直接运行单个测试文件（shunit2 由仓库内置）：

```bash
bash scripts/test/unit/test_app_image.sh
```

### Make 测试目标

```bash
make test-scripts
```

```bash
make test-scripts-lint
```

```bash
make test-scripts-unit
```

```bash
make test-integration-scripts TEST_SUITE=center.base
```

## Make 模块

顶层 `Makefile` 通过 `scripts/make/*.mk` 组织所有目标。查看全部目标：

```bash
make help
```

| 模块 | 主要目标 | 说明 |
| --- | --- | --- |
| `common.mk` | `generate`、`manifests`、`gen-clientset`、`gen-proto-code`、`help` | 代码生成与公共变量 |
| `golang.mk` | `fmt`、`vet`、`test`、`build`、`integration_test`、`clean` | Go 构建、单测与集成测试 |
| `image.mk` | `image`、`build-monitor`、`deps-image`、`proot` | Docker 镜像构建 |
| `lint.mk` | `check`、`go-check`、`yaml-check`、`shell-check`、`markdown-check`、`codespell-check`、`license-check` | 各类静态检查 |
| `docs.mk` | `docs`、`docs-clean`、`docs-preview`、`docs-link-check`、`version_check`、`version_fix` | 文档构建与版本校验 |
| `fate.mk` | `fate-build`、`fate-adaptor-app-image`、`deploy-image`、`fate-clean` | FATE 适配器构建 |
| `test.mk` | `test-scripts`、`test-scripts-lint`、`test-scripts-unit`、`test-integration-scripts` | Shell 脚本测试 |

常用命令：

```bash
make build
```

```bash
make image
```

```bash
make test
```

```bash
make shell-check
```

```bash
make check
```

```bash
make docs
```

```bash
make docs-preview
```

```bash
make version_check
```

## 构建与代码生成（hack/）

`hack/` 位于 **仓库根目录**（与 `scripts/` 同级），承载二进制编译、CRD/clientset/protobuf 代码生成、
版本一致性与错误码文档生成。以下命令均在 Kuscia 仓库根目录执行。

### hack/build.sh：编译二进制

| 选项 | 说明 |
| --- | --- |
| `-t kuscia` | 编译 `build/apps/kuscia/kuscia` |
| `-t transport` | 编译 `build/apps/transport/transport` |
| `-h` | 显示帮助 |

构建时通过 `-ldflags` 注入 `git describe --tags --always` 得到的版本号。必须显式指定 `-t`：

```bash
bash hack/build.sh -t kuscia
```

```bash
bash hack/build.sh -t transport
```

```bash
bash hack/build.sh -h
```

Make 等价入口：

```bash
make build
```

### hack/generate-crds.sh：生成 CRD

脚本会在缺少 `controller-gen` 时自动安装 v0.15.0，生成结果写入 `crds/v1alpha1/`：

```bash
bash hack/generate-crds.sh
```

```bash
make manifests
```

### hack/update-codegen.sh：生成 clientset / informer / lister

调用 `hack/generate-groups.sh all` 把代码生成到临时目录，再复制到 `pkg/crd`。首次运行会安装
Kubernetes code-generator 工具：

```bash
bash hack/update-codegen.sh
```

```bash
make gen-clientset
```

`hack/generate-groups.sh` 是通用生成器包装器，通常由 `update-codegen.sh` 调用；查看参数：

```bash
bash hack/generate-groups.sh --help
```

### hack/proto-to-go.sh：生成 protobuf Go 代码

依赖 `protoc`；缺少 `protoc-gen-go`、`protoc-gen-go-grpc` 时会自动 `go install`：

```bash
bash hack/proto-to-go.sh
```

```bash
make gen-proto-code
```

一次生成 CRD、clientset 与 protobuf 代码：

```bash
make generate
```

### hack/version_check.sh：版本一致性检查

| 选项 | 说明 |
| --- | --- |
| `--kuscia-version VERSION` | 期望的 Kuscia 版本，必填 |
| `--secretflow-version VERSION` | 期望的 SecretFlow 版本，必填 |
| `--check-dirs DIRS` | 检查目录，默认 `docs scripts hack` |
| `--mode check\|fix` | 检查或直接修正，默认 `check` |
| `--verbose` | 输出每个文件的检查过程 |
| `-h, --help` | 显示帮助 |

版本号来自仓库根目录的 `.VERSION`。下面示例只检查快速开始文档，可直接运行：

```bash
bash hack/version_check.sh \
  --kuscia-version 1.2.0b0 \
  --secretflow-version 1.11.0b1 \
  --check-dirs docs/getting_started
```

```bash
bash hack/version_check.sh --help
```

Make 入口读取 `.VERSION` 并扫描默认目录：

```bash
make version_check
```

`--mode fix` / `make version_fix` 会直接改写文件，执行前请先提交或备份。

### hack/errorcode/gen_error_code_doc.sh：错误码文档

| 模式 | 说明 |
| --- | --- |
| `verify` | 校验 proto 中的错误码与 i18n 文件是否一一对应 |
| `doc` | 由 proto + i18n 生成 Markdown |
| `append` | 把 proto 中缺失的错误码追加到 i18n 文件 |

```bash
bash hack/errorcode/gen_error_code_doc.sh verify \
  proto/api/v1alpha1/errorcode/error_code.proto \
  hack/errorcode/i18n/errorcode.zh-CN.toml
```

```bash
bash hack/errorcode/gen_error_code_doc.sh doc \
  proto/api/v1alpha1/errorcode/error_code.proto \
  hack/errorcode/i18n/errorcode.zh-CN.toml \
  docs/reference/apis/error_code_cn.md
```

Make 入口：

```bash
make gen_error_code_doc
```

### hack/k3s/build.sh：构建内嵌 K3s

克隆固定版本 K3s 源码到 `build/k3s`，替换 kine 版本并构建 Kuscia 定制 K3s 二进制（输出位于
`build/k3s/bin/k3s`）。该过程需要联网和较长时间：

```bash
bash hack/k3s/build.sh
```

`hack/k3s/build-kuscia-k3s-binary.sh` 由 K3s 源码中的 Makefile 通过 dapper 调用，用于注入
Kuscia 的版本信息与构建标签，通常不需要手动执行。

## 模板目录

`templates/` 中的 YAML 通过 `sed` 替换占位符后传给 `kubectl apply -f -`。新增模板时需同步更新
对应脚本的替换逻辑，并在 `scripts/test/unit/` 中添加渲染测试，避免未替换的占位符导致
`kubectl` 解析失败。

| 模板 | 用途 |
| --- | --- |
| `kuscia-master.yaml` / `kuscia-lite.yaml` / `kuscia-autonomy.yaml` | 节点 Deployment 模板 |
| `domain.yaml` / `domain_datasource.yaml` | Domain 与 DomainDataSource |
| `domain_route.mtls.yaml` / `domain_route.token.yaml` | DomainRoute（MTLS/Token） |
| `cluster_domain_route.mtls.yaml` / `cluster_domain_route.token.yaml` / `cluster_domain_route.token.transit.yaml` | ClusterDomainRoute（直连/中转） |
| `app_image.secretflow.yaml` / `app_image.psi.yaml` / `app_image.dataproxy.yaml` / `app_image.serving.yaml` | AppImage 模板 |
| `dataproxy.yaml` | DataProxy KusciaDeployment |
| `domaindata_alice_table.yaml` / `domaindata_bob_table.yaml` | PSI 示例数据 |
| `domaindata_alice_privacy_table.yaml` / `domaindata_bob_privacy_table.yaml` | 隐私组件测试数据 |
| `domaindata_alice_dp_table.yaml` / `domaindata_bob_dp_table.yaml` | DataProxy 示例数据 |
| `job.2pc_balanced_psi.yaml` | 2PC Balanced PSI 任务 |
| `task_input_config.2pc_balanced_psi.json` / `..._dp.json` | 任务输入配置 |
| `secretpad_svc.yaml` | SecretPad Service |
| `datamesh_svc.yaml` | DataMesh Service |
| `interop_config.yaml` | 互联协议配置 |
| `prometheus.yml` / `kuscia-monitor-datasource.yaml` / `grafana-dashboard-machine.json` | 监控配置与面板 |
| `monitor/base`、`monitor/runc`、`monitor/runk` | 集群内监控 Kustomize 模板 |
| `scql/broker_alice.yaml`、`broker_bob.yaml`、`broker_master.yaml` | SCQL Broker 模板 |

常见占位符：

| 占位符 | 含义 |
| --- | --- |
| `{{.SF_IMAGE_NAME}}` | 镜像仓库地址（不含 Tag） |
| `{{.SF_IMAGE_TAG}}` | 镜像 Tag |
| `{{.SF_IMAGE_ID}}` | 镜像 ID（PSI 等模板使用） |
| `{{.SF_NAME}}` | AppImage 资源名，可通过 `SF_NAME` 覆盖 |
| `{{.IMAGE_NAME}}` / `{{.IMAGE_TAG}}` | DataProxy/Kuscia 模板使用的镜像字段 |
| `{{.DOMAIN_ID}}` | 域标识 |
| `{{.TASK_ID}}`、`{{.JOB_NAME}}` 等 | 任务运行时由 Kuscia 或脚本注入，不属于部署期替换 |

## Linter 配置

| 路径 | 用途 |
| --- | --- |
| `linter/markdown/markdown_lint_config.yaml` | markdownlint 规则 |
| `linter/yamllint/.yamllint` | yamllint 规则 |
| `linter/license/.licenserc.yaml` | Apache-2.0 许可证头检查 |
| `linter/codespell/.codespell.skip` / `.ignorewords` | codespell 跳过项与忽略词 |
| `yamllint/requirements.txt` | yamllint 依赖清单 |

对应命令：

```bash
make markdown-check
```

```bash
make yaml-check
```

```bash
make license-check
```

```bash
make codespell-check
```

## 开发约定

- 所有脚本使用 `#!/bin/bash`，并以 `set -e`（必要时 `set -euo pipefail`）开头。
- 提交前运行 `make shell-check` 与 `make test-scripts`；ShellCheck 统一忽略 `SC1091,SC2034`。
- 外部命令应可被测试替换（例如通过 `PATH` 注入 fake `kubectl`/`docker`），不要写死集群地址。
- 新增脚本无需修改 Makefile：`run_local_tests.sh` 会自动发现 `scripts/**/*.sh` 并执行语法检查，
  但需要为新增的 ShellCheck 范围补充 `lint.mk` 与测试 runner 的路径。
- 新增单元测试放在 `scripts/test/unit/test_<feature>.sh`，会自动被 `--unit-only` 发现。
- 修改部署脚本的参数或输出时，同步更新本 README 与 `scripts/test/unit/test_cli_contract.sh`。
