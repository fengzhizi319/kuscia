# Kuscia / SecretFlow 源码二次开发 Docker 镜像打包指南

> 目标：基于当前 `sfwork` 工作区里的 `kuscia/` 与 `secretflow/` 源码，完成二次开发后的 Docker 镜像构建，并能够将自定义 SecretFlow 镜像注册到 Kuscia 集群中使用。
>
> 本指南参考了官方 Kuscia 镜像（`secretflow/kuscia`）与 SecretFlow 官方开发镜像（`sf-dev-ubuntu` / `secretflow-ubuntu`）的构建方式。

---

## 1. 前置条件

### 1.1 通用环境

- Linux 开发机（推荐 Ubuntu / Anolis / CentOS）。
- 已安装 Docker Engine，并启用 **Buildx**（Kuscia 镜像默认使用 `docker buildx`）。
- 能访问公网或内部镜像仓库，以下基础镜像需要可被拉取：
  - `secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/anolisos:23`
  - `secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-deps:0.7.0b0`
  - `secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-envoy:0.6.2b0`
  - `secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/node-exporter:v1.9.1`
  - `secretflow/ubuntu-base-ci:20250228`
  - `secretflow/release-ci:latest`（SecretFlow 开发镜像官方构建容器，可选）
- 目标仓库可写（如果启用 `--push`）。

### 1.2 Kuscia 编译依赖

- Go 1.24.7（与 `kuscia/go.mod` 保持一致）。
- 推荐宿主机架构与目标架构一致；若需要交叉编译，请自行设置 `GOOS=linux GOARCH=<arch> CGO_ENABLED=0`。

### 1.3 SecretFlow 编译依赖

- Python 3.10 或 3.11 环境（用于本地构建 wheel）。
- `pip install build` 或 `pdm`。
- 若使用官方 `secretflow/release-ci` 容器构建 wheel，则宿主机无需安装 Python 构建环境。

---

## 2. 快速开始：一键打包脚本

项目根目录已提供统一打包脚本：

```bash
bash kuscia/scripts/build-kuscia-secretflow-images.sh [OPTIONS]
```

### 2.1 常用示例

```bash
# 1. 只构建 Kuscia 镜像（默认使用当前宿主机架构）
bash kuscia/scripts/build-kuscia-secretflow-images.sh -p kuscia

# 2. 只构建 SecretFlow 开发镜像
bash kuscia/scripts/build-kuscia-secretflow-images.sh -p secretflow -v 1.15.0-dev

# 3. 同时构建 Kuscia + SecretFlow 开发镜像，并导出 tar
bash kuscia/scripts/build-kuscia-secretflow-images.sh -p all -v 1.15.0-dev --tar

# 4. 指定镜像仓库前缀并推送
bash kuscia/scripts/build-kuscia-secretflow-images.sh -p all \
  -r registry.example.com/secretflow \
  -v 1.15.0-dev \
  --push

# 5. 使用已经编译好的 SecretFlow wheel 包构建镜像
bash kuscia/scripts/build-kuscia-secretflow-images.sh -p secretflow \
  -v 1.15.0-dev \
  --sf-wheel /path/to/secretflow-1.15.0.dev...-cp310-cp310-linux_x86_64.whl
```

### 2.2 脚本参数说明

| 参数 | 说明 | 默认值 |
|---|---|---|
| `-h, --help` | 显示帮助信息 | - |
| `-p, --project` | 构建项目：`kuscia`、`secretflow`、`all` | `all` |
| `-v, --version` | 镜像版本号 / Tag | Kuscia 使用 `git describe`；SecretFlow 使用 `dev-<date>` |
| `-a, --arch` | Kuscia 目标架构：`amd64` / `arm64` | 自动检测宿主机架构 |
| `-r, --registry` | 镜像仓库前缀，例如 `registry.example.com/secretflow` | 无（本地镜像名） |
| `-l, --latest` | 额外给镜像打 `latest` Tag | 否 |
| `--push` | 构建完成后推送镜像 | 否 |
| `-t, --tar` | 将构建好的镜像导出为 `.tar` | 否 |
| `--sf-wheel` | 使用指定的 SecretFlow wheel 包构建开发镜像 | 自动构建 |

> 脚本位置：`kuscia/scripts/build-kuscia-secretflow-images.sh`
>
> 注意：
> - Kuscia 镜像默认使用 `docker buildx` 构建，脚本会自动创建名为 `sfwork-kuscia` 的 builder。
> - SecretFlow 开发镜像默认依赖 `secretflow/release-ci:latest` 容器构建 wheel；如果该镜像不可用，可通过 `--sf-wheel` 传入预先构建好的 wheel 包。

---

## 3. Kuscia 镜像构建（源码方式）

### 3.1 官方构建方式

Kuscia 的构建入口在 `kuscia/Makefile`，所有镜像相关逻辑在 `kuscia/scripts/make/image.mk`。

```bash
cd /home/charles/code/sfwork/kuscia

# 构建当前宿主机架构镜像
make image

# 指定架构
make image ARCH=amd64

# 指定自定义镜像名 / 仓库
make image IMG=myregistry.example.com/secretflow/kuscia:$(git describe --tags --always)-$(date +%Y%m%d%H%M%S)
```

构建产物：

- 本地产物：`build/apps/kuscia/kuscia`、`build/linux/${ARCH}/apps/kuscia/kuscia`。
- Docker 镜像：`secretflow/kuscia:<git-describe>-<datetime>`。

### 3.2 关键文件说明

| 文件 | 作用 |
|---|---|
| `kuscia/Makefile` | 顶层 Makefile，汇总各子模块 |
| `kuscia/scripts/make/image.mk` | 镜像 Tag、Buildx、Build Arg 定义 |
| `kuscia/scripts/make/golang.mk` | `build` 目标：编译二进制并整理产物目录 |
| `kuscia/hack/build.sh` | 实际执行 `go build` 的脚本 |
| `kuscia/build/dockerfile/kuscia-anolis.Dockerfile` | Kuscia 主镜像 Dockerfile |
| `kuscia/build/dockerfile/base/kuscia-deps.Dockerfile` | 含 k3s、containerd、CNI 等依赖的基础镜像 |
| `kuscia/build/pause/pause-{amd64,arm64}.tar` | K3s Pod 基础镜像，仓库中已预置 |

### 3.3 手动分步构建（跳过 `make` 的代码检查）

如果你只想快速出包，不想触发 `go fmt`、`go vet`、`verify_error_code`，可以按以下步骤手动构建：

```bash
cd /home/charles/code/sfwork/kuscia

ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')
VERSION=$(git describe --tags --always)
DATETIME=$(date +"%Y%m%d%H%M%S")
IMAGE="secretflow/kuscia:${VERSION}-${DATETIME}"

# 1. 编译二进制（交叉编译时设置 GOARCH）
GOOS=linux GOARCH="${ARCH}" bash hack/build.sh -t kuscia

# 2. 整理 Dockerfile 需要的目录结构
mkdir -p "build/linux/${ARCH}"
cp -rp build/apps "build/linux/${ARCH}/"

# 3. 创建并使用 buildx builder（若不存在）
if ! docker buildx inspect kuscia >/dev/null 2>&1; then
    docker buildx create --name kuscia --platform linux/amd64,linux/arm64
fi
docker buildx use kuscia

# 4. 构建镜像
DOCKER_BUILDKIT=1 docker buildx build \
  --build-arg KUSCIA_ENVOY_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-envoy:0.6.2b0 \
  --build-arg DEPS_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-deps:0.7.0b0 \
  -f build/dockerfile/kuscia-anolis.Dockerfile \
  -t "${IMAGE}" \
  --platform "linux/${ARCH}" --load .
```

### 3.4 二次开发常见修改点

| 修改内容 | 需要重新构建的部分 |
|---|---|
| Go 源码修改 | 只需重新 `make image` 或 `hack/build.sh` + `docker buildx build` |
| CRD / 配置模板 | 修改 `crds/v1alpha1/`、`etc/conf/` 后重新构建镜像 |
| 新增二进制依赖 | 修改 `build/dockerfile/base/kuscia-deps.Dockerfile` 并重新构建 deps 镜像 |
| Envoy 版本 | 修改 `KUSCIA_ENVOY_IMAGE` build arg |

---

## 4. SecretFlow 镜像构建（源码方式）

SecretFlow 的 Docker 构建分为两类：

1. **开发镜像（`sf-dev-ubuntu`）**：基于当前源码构建 wheel 后安装，适合二次开发验证。
2. **发布镜像（`secretflow-ubuntu`）**：基于已发布到 PyPI / test.pypi.org 的版本安装，适合生产复现。

二次开发请优先使用 **开发镜像**。

### 4.1 官方开发镜像构建

```bash
cd /home/charles/code/sfwork/secretflow/docker/dev

# 构建 sf-dev-ubuntu:<version>
bash build.sh -v 1.15.0-dev

# 构建并保存为 tar
bash build.sh -v 1.15.0-dev -t

# 推送到指定仓库并标记 latest
bash build.sh -v 1.15.0-dev -r myregistry.example.com/secretflow -u -l
```

### 4.2 关键文件说明

| 文件 | 作用 |
|---|---|
| `secretflow/pyproject.toml` | Python 包元数据、构建后端 `pdm-backend` |
| `secretflow/secretflow/version.py` | 版本号占位，构建时由 `pdm_build.py` 替换 |
| `secretflow/docker/dev/Dockerfile` | 开发镜像 Dockerfile，安装本地 wheel |
| `secretflow/docker/dev/build.sh` | 官方开发镜像构建脚本，使用 `release-ci` 容器构建 wheel |
| `secretflow/docker/dev/entry.sh` | `release-ci` 容器内执行：创建 conda 环境、编译 wheel |
| `secretflow/docker/release/ubuntu.Dockerfile` | 发布镜像 Dockerfile，从 PyPI 安装 |
| `secretflow/docker/config_templates.yml` | Kuscia 应用配置模板（镜像 LABEL） |
| `secretflow/docker/deploy_templates.yml` | Kuscia 应用部署模板（镜像 LABEL） |

### 4.3 手动分步构建（不依赖 release-ci）

如果你已经有构建好的 wheel，或希望完全在本地控制构建过程：

```bash
cd /home/charles/code/sfwork/secretflow

VERSION="1.15.0-dev"
IMAGE="secretflow/sf-dev-ubuntu:${VERSION}"

# 1. 生成 wheel（要求在 Python 3.10 环境下执行）
pip install build
python -m build --wheel

# 2. 将产物与 nsjail / 模板文件复制到 docker/dev/
cp dist/secretflow-*.whl docker/dev/
cp -r docker/release/.nsjail docker/dev/
cp docker/release/.condarc docker/dev/
cp docker/*.yml docker/dev/

# 3. 构建镜像
docker build -f docker/dev/Dockerfile -t "${IMAGE}" \
  --build-arg config_templates="$(cat docker/config_templates.yml)" \
  --build-arg deploy_templates="$(cat docker/deploy_templates.yml)" \
  docker/dev

# 4. 清理临时文件
rm -rf docker/dev/.nsjail docker/dev/.condarc docker/dev/*.yml docker/dev/*.whl
```

> 注意：若宿主机 Python 版本不是 3.10，建议通过 `secretflow/release-ci` 容器或 Python 3.10 容器执行 `python -m build`，否则生成的 wheel 可能无法安装到镜像的 Python 3.10 环境中。

### 4.4 二次开发常见修改点

| 修改内容 | 需要重新构建的部分 |
|---|---|
| Python 源码修改 | 重新生成 wheel 后构建 `sf-dev-ubuntu` 镜像 |
| 组件 / pipeline 模板 | 更新 `docker/config_templates.yml`、`docker/deploy_templates.yml` 后重新构建 |
| nsjail 沙箱配置 | 修改 `docker/release/.nsjail/` 后重新构建 |
| 依赖版本 | 修改 `pyproject.toml` 并重新生成 wheel |

---

## 5. 将自定义 SecretFlow 镜像接入 Kuscia

Kuscia 不直接运行任意容器，而是通过 **AppImage** CRD 注册应用镜像。注册后，在提交 KusciaJob 时通过 `appImageRef` 引用。

### 5.1 使用官方脚本注册

Kuscia 已提供注册脚本：

```bash
cd /home/charles/code/sfwork/kuscia

# 注册 SecretFlow 类型应用镜像
./scripts/deploy/create_sf_app_image.sh \
  secretflow/sf-dev-ubuntu \
  1.15.0-dev \
  secretflow
```

参数说明：

- `SF_IMAGE_NAME`：镜像仓库名（不含 Tag）。
- `SF_IMAGE_TAG`：镜像 Tag。
- `APP_TYPE`：应用类型，SecretFlow 使用 `secretflow`。

### 5.2 模板说明

注册脚本读取模板：

```text
kuscia/scripts/templates/app_image.secretflow.yaml
```

模板关键字段：

- `spec.image.name` / `spec.image.tag`：容器镜像。
- `spec.deployTemplates[].spec.containers[].args`：启动命令，默认执行 `python -m secretflow.kuscia.entry ./kuscia/task-config.conf`。
- `spec.configTemplates`：任务配置模板，Kuscia 在启动任务时注入 `task-config.conf`。

如果你的自定义镜像需要修改启动命令（例如使用 nsjail），可复制该模板并自定义，然后直接 `kubectl apply -f`。

### 5.3 在 KusciaJob 中引用

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: my-job
spec:
  initiator: alice
  scheduleMode: Strict
  appImageRef: sf-dev-ubuntu          # AppImage 的 metadata.name
  tasks:
    - appImage: sf-dev-ubuntu
      parties:
        - domainID: alice
          configTemplate: secretflow
        - domainID: bob
          configTemplate: secretflow
```

> `appImageRef` 对应的是 `AppImage.metadata.name`，不是镜像名。同一个 AppImage 可以被多个 KusciaJob 复用，不需要每个任务都新建一个 AppImage。

---

## 6. Kuscia + SecretFlow 一体化镜像（可选）

官方还提供了一个一体化镜像 `kuscia-secretflow`，将 Kuscia 运行时与 `secretflow-lite` 直接打包，适合快速部署。

### 6.1 官方一体化镜像

```bash
cd /home/charles/code/sfwork/kuscia

# 先构建 Kuscia 镜像
make image

# 构建一体化镜像（默认从 PyPI 安装 secretflow-lite）
docker build \
  --build-arg KUSCIA_IMAGE=secretflow/kuscia:<your-kuscia-tag> \
  --build-arg SF_VERSION=1.11.0b1 \
  -f build/dockerfile/kuscia-secretflow.Dockerfile \
  -t secretflow/kuscia-secretflow:1.11.0b1 .
```

### 6.2 基于自定义 SecretFlow 的一体化镜像

如果你想把二次开发后的 SecretFlow 直接打进 Kuscia 镜像，建议新建一个 Dockerfile，例如：

```dockerfile
ARG KUSCIA_IMAGE=secretflow/kuscia:latest
ARG SF_IMAGE=secretflow/sf-dev-ubuntu:1.15.0-dev

FROM ${SF_IMAGE} AS sf
FROM ${KUSCIA_IMAGE}

USER root
# 将 SecretFlow 开发镜像中的 Python 环境整体复制到 Kuscia 镜像
COPY --from=sf /usr/local/bin/ /usr/local/bin/
COPY --from=sf /usr/local/lib/ /usr/local/lib/

# 修复 shebang 路径（如果原镜像使用 miniconda，需要相应调整）
RUN grep -rl '#!/root/miniconda3/envs/secretflow/bin' /usr/local/bin/ 2>/dev/null | \
    xargs -r sed -i -e 's|#!/root/miniconda3/envs/secretflow|#!/usr/local|g' || true

# 安装运行时依赖
RUN yum install -y protobuf libnl3 libgomp && yum clean all

USER kuscia
WORKDIR /home/kuscia
ENTRYPOINT ["tini", "--"]
```

构建命令：

```bash
docker build \
  --build-arg KUSCIA_IMAGE=secretflow/kuscia:<tag> \
  --build-arg SF_IMAGE=secretflow/sf-dev-ubuntu:<tag> \
  -f kuscia-secretflow-custom.Dockerfile \
  -t secretflow/kuscia-secretflow-custom:<tag> .
```

---

## 7. 常见问题排查

### 7.1 `docker buildx` 相关

- 报错 `multiple platforms feature is currently not supported for docker driver`：
  - 需要先创建并启用 buildx builder：`docker buildx create --name kuscia --use`。
- 报错 `buildx` 不存在：
  - 升级 Docker 到较新版本，或使用 `docker build` 单平台构建。

### 7.2 Kuscia 构建

- 报错 `build/pause/pause-${TARGETARCH}.tar: no such file or directory`：
  - 检查 `build/pause/pause-amd64.tar` 与 `build/pause/pause-arm64.tar` 是否存在。
- 报错 `build/linux/amd64/apps/kuscia/kuscia: no such file or directory`：
  - 先执行 `hack/build.sh -t kuscia` 并确认产物路径正确。

### 7.3 SecretFlow 构建

- 报错 `No matching distribution found for secretflow==xxx`：
  - 发布镜像需要从 PyPI 安装，确认版本已发布；开发镜像使用本地 wheel 构建。
- 报错 `*.whl is not a supported wheel on this platform`：
  - wheel 与镜像内 Python 版本不匹配。开发镜像基于 Python 3.10，请使用 Python 3.10 构建 wheel。

### 7.4 AppImage 注册

- 报错 `error: unable to recognize "kuscia.secretflow/v1alpha1"`：
  - 确认当前 `kubectl` 上下文已指向 Kuscia 集群。
- 任务启动后容器拉取失败：
  - 确认 Kuscia 节点能访问镜像仓库，或已将镜像 `docker save` 后 `docker load` 到 Kuscia 容器内。

---

## 8. 本地运行与 Docker 运行的端口处理

Kuscia 的默认端口分为两类：**容器内部固定端口** 和 **宿主机映射端口**。二次开发验证时，经常需要在“无 Docker 本地运行”和“Docker 容器运行”之间切换，搞清楚这两类端口的对应关系可以避免连接失败或端口冲突。

### 8.1 Kuscia 容器内部默认端口

这些端口写在 Kuscia 代码与配置里，不管你是用官方镜像还是自编译镜像，只要容器内运行 `bin/kuscia start`，默认都会监听：

| 服务 | 内部端口 | 说明 |
|---|---|---|
| Gateway（跨域网关） | 1080 | 对外/跨域通信入口 |
| Gateway（域内应用网关） | 80 | 域内 Pod 应用流量入口 |
| KusciaAPI HTTP | 8082 | 外部 HTTP API |
| KusciaAPI gRPC | 8083 | 外部 gRPC API（SecretPad 默认使用） |
| KusciaAPI HTTP internal | 8092 | 容器内内部 HTTP API |
| DataMesh HTTP | 8070 | 数据服务 HTTP |
| DataMesh gRPC | 8071 | 数据服务 gRPC |
| ConfManager HTTP | 8060 | 配置管理 HTTP |
| ConfManager gRPC | 8061 | 配置管理 gRPC |
| Reporter HTTP | 8050 | 指标上报 |
| Transport gRPC | 9090 | 跨域传输 |
| CoreDNS | 53 | 域内 DNS |
| Envoy xDS | 10001 | Envoy 动态配置 |

> 容器内部端口通常不需要改；如果你想暴露给宿主机或让外部组件连接，改的是**宿主机映射端口**。

### 8.2 Docker 运行时的端口映射

#### `start_standalone.sh` 默认端口

`kuscia/scripts/deploy/start_standalone.sh` 用于快速拉起本地体验集群，默认会把容器内部端口映射到宿主机。以 `center` 模式为例：

| 节点 | 宿主机端口 | 容器内部端口 |
|---|---|---|
| master | 18080 | 1080 |
| | 18082 | 8082 |
| | 18083 | 8083 |
| alice-lite | 28080 | 1080 |
| | 28081 | 80 |
| | 28082 | 8082 |
| | 28083 | 8083 |
| bob-lite | 38080 | 1080 |
| | 38081 | 80 |
| | 38082 | 8082 |
| | 38083 | 8083 |

`p2p` 模式默认映射：

| 节点 | 宿主机端口 | 容器内部端口 |
|---|---|---|
| alice-autonomy | 11081 | 80 |
| | 11082 | 8082 |
| | 11083 | 8083 |
| bob-autonomy | 12081 | 80 |
| | 12082 | 8082 |
| | 12083 | 8083 |

#### `deploy.sh` 默认端口

生产部署脚本 `kuscia/scripts/deploy/deploy.sh` 允许自定义映射，默认值如下：

| 用途 | 默认宿主机端口 | 容器内部端口 | 参数/环境变量 |
|---|---|---|---|
| 跨域网关 | 需显式指定 | 1080 | `-p` / `DOMAIN_HOST_PORT` |
| 域内应用网关 | 13081 | 80 | `-q` / `DOMAIN_HOST_INTERNAL_PORT` |
| KusciaAPI HTTP | 13082 | 8082 | `-k` / `KUSCIAAPI_HTTP_PORT` |
| KusciaAPI gRPC | 13083 | 8083 | `-g` / `KUSCIAAPI_GRPC_PORT` |

示例：

```bash
bash scripts/deploy/deploy.sh autonomy -d alice \
  -p 11080 -q 11081 -k 11082 -g 11083
```

### 8.3 无 Docker 本地运行时的端口

`run-all-no-docker.sh` 直接把 Kuscia、SecretPad 后端、SecretPad 前端运行在宿主机上，不存在“宿主机→容器”映射，端口就是服务实际监听端口：

| 服务 | 地址 | 说明 |
|---|---|---|
| Kuscia API gRPC | 127.0.0.1:8083 | 与容器内默认端口一致 |
| Kuscia Envoy 内部 | 127.0.0.1:80 | SecretPad 通过该地址访问网关 |
| SecretPad 后端 HTTP | 127.0.0.1:8080 | dev 模式 `http-port` |
| SecretPad 后端 HTTPS | 127.0.0.1:8443 | `server.port` |
| SecretPad 内部 API | 127.0.0.1:9001 | `server.http-port-inner` |
| SecretPad 前端 dev | 127.0.0.1:8000 | Umi dev server |
| CoreDNS | 127.0.0.1:53 | 需要 root |

对应环境变量：

```bash
export KUSCIA_API_ADDRESS=127.0.0.1
export KUSCIA_GW_ADDRESS=127.0.0.1:80
export KUSCIA_PROTOCOL=notls
```

前端代理配置 `secretpad/frontend-src/apps/platform/.env`：

```bash
PROXY_URL=http://127.0.0.1:8080
```

### 8.4 端口冲突与特权端口

- **80 和 53 是特权端口**，无 Docker 模式下需要用 `sudo` 启动；Docker 模式下可以通过映射到高位宿主机端口（如 `13081`）来避免。
- 启动前检查端口占用：

```bash
ss -tln | grep -E ':(53|80|8080|8082|8083|8443|9001|8000)\b'
```

- 无 Docker 模式下若 `53` 被 `systemd-resolved` 占用，可临时停止：

```bash
sudo systemctl stop systemd-resolved
```

- Docker 模式下若宿主机端口冲突，修改 `start_standalone.sh` 源码中的端口（该脚本暂不支持命令行改端口），或使用 `deploy.sh` 的 `-p/-q/-k/-g` 参数。

### 8.5 SecretPad 接入 Kuscia 容器时的注意点

当 SecretPad 后端运行在宿主机、Kuscia 运行在 Docker 容器时，`KUSCIA_GW_ADDRESS` 和 gRPC 地址要指向**宿主机上映射后的端口**，而不是容器内部端口。

例如使用 `deploy.sh` 默认值：

```bash
export KUSCIA_API_ADDRESS=127.0.0.1
export KUSCIA_GW_ADDRESS=127.0.0.1:13081
export KUSCIA_PROTOCOL=notls   # 生产环境请使用 tls/mtls
```

KusciaAPI gRPC 默认对应 `13083`。

如果启用了 mTLS，还需要把 Kuscia 容器内的客户端证书挂载到 SecretPad 的 `config/certs/` 目录，并配置 `KUSCIA_PROTOCOL` 为相应协议。

> **注意**：SecretFlow 任务容器运行在 Kuscia 容器内部，它们访问 DataMesh / KusciaAPI 时使用的是**容器内部端口**（如 8071、8083）和 CoreDNS 域名，不需要关心宿主机映射。

## 9. 整体关系与部署形态总结

本节把前面关于镜像、域内通信、端口映射的内容做一个整体梳理，方便二次开发时快速定位自己该构建/重启哪一部分。

### 9.1 Kuscia 与 SecretFlow 的镜像关系

- **默认是两个镜像**：
  - `secretflow/kuscia`：Kuscia 运行时（Go 二进制 + k3s/Envoy/containerd/CNI 等）。
  - `secretflow/sf-dev-ubuntu` 或 `secretflow/secretflow-lite-anolis8`：SecretFlow 算法镜像，通过 AppImage 注册到 Kuscia，由 Kuscia 调度执行。
- **可选一体化镜像**：
  - `secretflow/kuscia-secretflow`：把 Kuscia 运行时和 SecretFlow 运行环境打进同一个镜像，适合快速部署或离线环境，详见本文第 6 节。

### 9.2 Kuscia 域内通信形态

Kuscia 当前是 **“一个域一个容器、一个 Kuscia 进程”** 的架构：

- 内部模块（kusciaapi、datamesh、confmanager、transport、envoy、scheduler、domainroute 等）在同一个进程内以 goroutine/子进程方式运行。
- 域内模块通信方式：
  1. **本地回环固定端口**：`127.0.0.1:8083`、`127.0.0.1:80` 等。
  2. **共享 K3s 控制平面**：所有模块通过同一个 kubeconfig 读写 CRD。
  3. **Envoy + xDS**：`domainroute` 模块在 `127.0.0.1:10001` 提供 xDS，Envoy 拉取路由配置。
  4. **CoreDNS**：为域内 Pod 提供 `*.svc` 域名解析。
  5. **CRD 状态同步**：Domain、DomainRoute、Gateway、Service、Endpoints、KusciaJob、KusciaTask 等。

> 目前没有官方支持把各模块拆成独立镜像/独立容器。如果想拆分，需要为每个模块提供独立启动入口、替换所有 `127.0.0.1` 硬编码、共享 K3s/证书/数据存储、让 xDS/CoreDNS 可被跨容器访问，并重新实现跨容器依赖编排。

### 9.3 本地运行 vs Docker 运行的端口映射关系

| 运行方式 | KusciaAPI gRPC | Envoy 内部 | SecretPad HTTP | SecretPad HTTPS | 前端 |
|---|---|---|---|---|---|
| 无 Docker 本地运行 | `127.0.0.1:8083` | `127.0.0.1:80` | `127.0.0.1:8080` | `127.0.0.1:8443` | `127.0.0.1:8000` |
| `deploy.sh` 默认 Docker | `127.0.0.1:13083` | `127.0.0.1:13081` | — | — | — |
| `start_standalone.sh` center | master: `18083`、alice-lite: `28083`、bob-lite: `38083` | alice/bob-lite: `28081`/`38081` | — | — | — |

核心原则：

- **容器内部端口固定**（如 8083、80、1080），通常不要改。
- **宿主机映射端口可变**。外部组件（如宿主机上的 SecretPad）连接 Kuscia 时，必须使用**宿主机上映射后的端口**。
- **任务容器内部**访问 DataMesh / KusciaAPI 时，使用容器内部端口和 CoreDNS 域名，不需要关心宿主机映射。

### 9.4 二次开发时该重启/重构建什么

| 修改内容 | 需要重新构建/注册的镜像 | 需要重启的服务 |
|---|---|---|
| Kuscia Go 源码 | `secretflow/kuscia` | Kuscia 容器/进程 |
| SecretFlow Python 源码 | `secretflow/sf-dev-ubuntu` | 重新注册 AppImage |
| AppImage / 模板 | — | 重新 `create_sf_app_image.sh` 或 `kubectl apply` |
| 端口/网络配置 | — | 重启对应 Kuscia 容器并检查映射 |

## 10. AppImage → KusciaJob 调度与运行原理详解

本节详细说明 Kuscia 如何通过 AppImage CRD 把 SecretFlow 镜像注册进来，以及在提交 KusciaJob 后，Kuscia 如何解析 AppImage、生成任务 Pod、拉取镜像并启动 SecretFlow 进程。

### 10.1 涉及的 CRD 与核心类型

#### AppImage

- CRD YAML：`crds/v1alpha1/kuscia.secretflow_appimages.yaml`
- Go types：`pkg/crd/apis/kuscia/v1alpha1/appimage_types.go`
- 作用域：Cluster，short name `aimg`

核心字段：

```go
type AppImageSpec struct {
    Image           AppImageInfo      `json:"image"`
    ConfigTemplates map[string]string `json:"configTemplates,omitempty"`
    DeployTemplates []DeployTemplate  `json:"deployTemplates"`
}

type AppImageInfo struct {
    Name string `json:"name"`
    Tag  string `json:"tag"`
    ID   string `json:"id,omitempty"`   // 可选 sha256 校验
    Sign string `json:"sign,omitempty"`
}

type DeployTemplate struct {
    Name     string  `json:"name"`
    Role     string  `json:"role,omitempty"`
    Replicas *int32  `json:"replicas,omitempty"`
    Spec     PodSpec `json:"spec"`
}
```

- `Image.Name` / `Image.Tag`：任务容器实际使用的 OCI 镜像。
- `ConfigTemplates`：任务配置模板，最终会被渲染成 `task-config.conf` 等文件挂载到容器。
- `DeployTemplates`：定义容器规格（命令、参数、端口、资源、挂载等），支持按 `Role` 选择不同模板。

#### KusciaJob

- CRD YAML：`crds/v1alpha1/kuscia.secretflow_kusciajobs.yaml`
- Go types：`pkg/crd/apis/kuscia/v1alpha1/kusciajob_types.go`

```go
type KusciaJobSpec struct {
    Initiator      string
    ScheduleMode   KusciaJobScheduleMode // Strict / BestEffort
    MaxParallelism *int
    Tasks          []KusciaTaskTemplate
}

type KusciaTaskTemplate struct {
    Alias           string
    TaskID          string
    Dependencies    []string
    AppImage        string          // 引用 AppImage.metadata.name
    TaskInputConfig string
    ScheduleConfig  *ScheduleConfig
    Priority        int
    Parties         []Party
}

type Party struct {
    DomainID  string
    Role      string
    Resources *corev1.ResourceRequirements
}
```

#### KusciaTask

- Go types：`pkg/crd/apis/kuscia/v1alpha1/kusciatask_types.go`

```go
type PartyInfo struct {
    DomainID    string
    AppImageRef string   // 从 KusciaJob 的 AppImage 字段复制
    Role        string
    Template    PartyTemplate
}
```

### 10.2 AppImage 注册流程

注册脚本：

- `scripts/deploy/create_sf_app_image.sh`
- 模板：`scripts/templates/app_image.secretflow.yaml`

脚本读取模板并把占位符替换成镜像名/Tag，然后通过 `kubectl apply` 写入 K3s API：

```bash
APP_IMAGE_TEMPLATE=$(sed "s!{{.SF_IMAGE_NAME}}!'${SF_IMAGE_NAME}'!g;
  s!{{.SF_IMAGE_TAG}}!'${SF_IMAGE_TAG}'!g;
  s!{{.SF_IMAGE_ID}}!'${SF_IMAGE_ID}'!g" \
  < "${ROOT}/scripts/templates/app_image.${APP_TYPE}.yaml")

echo "${APP_IMAGE_TEMPLATE}" | kubectl apply -f -
```

SecretFlow 模板关键片段：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: AppImage
metadata:
  name: {{.SF_NAME}}
spec:
  configTemplates:
    task-config.conf: |
      {
        "task_id": "{{.TASK_ID}}",
        "task_input_config": "{{.TASK_INPUT_CONFIG}}",
        "task_cluster_def": "{{.TASK_CLUSTER_DEFINE}}",
        "task_progress_url":"http://reporter.master.svc/report/progress?task_id={{.TASK_ID}}",
        "allocated_ports": "{{.ALLOCATED_PORTS}}"
      }
  deployTemplates:
    - name: secretflow
      role: secretflow
      replicas: 1
      spec:
        containers:
          - command: [sh]
            args:
              - -c
              - "python -m secretflow.kuscia.entry ./kuscia/task-config.conf"
            configVolumeMounts:
              - mountPath: /work/kuscia/task-config.conf
                subPath: task-config.conf
            name: secretflow
            ports:
              - name: spu
                protocol: GRPC
                scope: Cluster
            workingDir: /work
        restartPolicy: Never
  image:
    name: {{.SF_IMAGE_NAME}}
    tag: {{.SF_IMAGE_TAG}}
```

注册完成后，AppImage 对象就存在 K3s 中，供后续 KusciaJob 引用。

> **AppImage 不是按任务创建的**：
>
> 一个 AppImage 对应**一种应用镜像模板**（如 `secretflow/sf-dev-ubuntu:1.15.0-dev`），通常注册一次即可被多个 KusciaJob 复用。KusciaJob 通过 `appImageRef` 字段引用 AppImage 的 `metadata.name`，而任务专属配置（输入数据、算法参数、参与方等）放在 `KusciaJob.spec.tasks[].taskInputConfig` 中，最终由 `config-render` 注入到任务容器。
>
> 只有以下情况才需要新建/更新 AppImage：
> - SecretFlow 镜像版本升级；
> - 需要不同的启动命令、端口、资源或挂载模板；
> - 新增其他类型的算法引擎镜像。
>
> 例如，注册一次 `sf-dev-ubuntu` 后，可以提交无数个 PSI、LR、NN 等不同算法的 KusciaJob，只要它们都使用同一个 SecretFlow 镜像。

### 10.2.1 配置模板与部署模板是否多余？

**不是多余的。** AppImage 不只是“镜像名+Tag”的别名，它还声明了“这张镜像该怎么跑、需要什么样的配置文件”。这两个字段缺一不可：

1. **ConfigTemplates —— 配置模板（静态模板）**
   - 定义镜像运行时需要哪些配置文件，以及这些文件的格式/占位符。
   - 占位符（如 `{{.TASK_ID}}`、`{{.TASK_INPUT_CONFIG}}`）在任务调度时由 Kuscia 根据 `KusciaJob` 中的实际值渲染。
   - 对于 SecretFlow，模板里的 `task-config.conf` 约定了 SecretFlow 入口期望读取哪些字段；不同的算法引擎可能使用完全不同的配置格式。
   - 如果不定义 ConfigTemplates，agent 侧的 `config-render` 钩子就不知道要生成什么文件、挂载到哪里。

2. **DeployTemplates —— 部署模板（Pod/容器规格）**
   - 定义容器启动命令、参数、工作目录、端口、资源、挂载、重启策略等。
   - 不同应用镜像的启动方式不同：SecretFlow 是 `python -m secretflow.kuscia.entry ./kuscia/task-config.conf`，其他引擎可能是别的命令。
   - 支持按 `Role` 选择不同模板，同一个 AppImage 可以给 server/client 或 alice/bob 配置不同端口/资源。
   - Kuscia 最终会根据 DeployTemplates 生成 Kubernetes Pod，交给 agent 执行。

对比关系：

| 内容 | 存在哪里 | 是否随任务变化 |
|---|---|---|
| 镜像名/Tag | `AppImage.spec.image` | 否 |
| 配置文件格式/占位符 | `AppImage.spec.configTemplates` | 否 |
| 容器启动方式/端口/资源 | `AppImage.spec.deployTemplates` | 否 |
| 本次任务的具体输入、参数、参与方 | `KusciaJob.spec.tasks[].taskInputConfig` 等 | 是 |

所以 `AppImage` 解决的是“这个镜像怎么跑”，`KusciaJob` 解决的是“这次任务跑什么”。两者职责分离，配置模板和部署模板正是 AppImage 能够被多个 KusciaJob 复用的关键：同一套模板可以被无数个 KusciaJob 共享，而每个 KusciaJob 只需要注入自己的任务变量。

> 换句话说，可以把 `AppImage` 理解为一份**“镜像使用说明书”**：`image` 字段说明用哪个镜像，`deployTemplates` 说明怎么启动容器，`configTemplates` 说明需要生成哪些配置文件以及配置文件的格式。Kuscia 按照这份说明书，把 `KusciaJob` 里的任务变量填进去，最终把容器跑起来。

### 10.3 KusciaJob 提交后的调度流程

主要控制器：

| 控制器 | 路径 | 职责 |
|---|---|---|
| KusciaJob controller | `pkg/controllers/kusciajob/controller.go` | 调和 KusciaJob，驱动状态机 |
| RunningHandler / scheduler | `pkg/controllers/kusciajob/handler/running.go`、`scheduler.go` | 计算可运行子任务并创建 KusciaTask |
| KusciaTask controller | `pkg/controllers/kusciatask/controller.go` | 调和 KusciaTask |
| PendingHandler | `pkg/controllers/kusciatask/handler/pending_handler.go` | 根据 AppImage 构建 Pod/Service/ConfigMap |
| TaskResourceGroup controller | `pkg/controllers/taskresourcegroup/` | 跨域资源协调 |
| Kuscia scheduler plugin | `pkg/scheduler/kusciascheduling/` | 等待所有参与方 Pod 都 Ready 后再放行 |

流程：

1. 用户创建 `KusciaJob`（通常在 `cross-domain` namespace）。
2. `KusciaJob` controller 进入 `RunningHandler.handleRunning`，计算依赖已完成的就绪子任务。
3. `buildWillStartKusciaTask` 为每个就绪子任务创建一个 `KusciaTask` CR。关键代码（`pkg/controllers/kusciajob/handler/scheduler.go`）：

```go
taskPartyInfos[i] = kusciaapisv1alpha1.PartyInfo{
    DomainID:    p.DomainID,
    AppImageRef: template.AppImage,   // <-- 这里把 KusciaJob 的 appImage 赋给 Party
    Role:        p.Role,
    Template:    tpl,
    ...
}
```

4. `KusciaTask` controller 收到新的 `KusciaTask`，调用 `PendingHandler`。
5. `PendingHandler.buildPartyKitInfo` 根据每个 `Party.AppImageRef` 从 Informer 缓存中读取 `AppImage`：

```go
appImage, err := h.appImagesLister.Get(party.AppImageRef)
if err != nil {
    return nil, fmt.Errorf("failed to get appImage %q from cache, %v", party.AppImageRef, err)
}

baseDeployTemplate, err := utilsres.SelectDeployTemplate(appImage.Spec.DeployTemplates, party.Role)
...
kit.Image = fmt.Sprintf("%s:%s", appImage.Spec.Image.Name, appImage.Spec.Image.Tag)
kit.ImageID = appImage.Spec.Image.ID
kit.DeployTemplate = deployTemplate
kit.ConfigTemplates = appImage.Spec.ConfigTemplates
```

`SelectDeployTemplate` 在 `pkg/utils/resources/appimage.go` 中实现：先按 `Role` 精确匹配，否则使用默认模板。

6. `generatePod` 把 `kit.Image` 注入到容器定义中：

```go
resCtr := v1.Container{
    Name:            ctr.Name,
    Image:           partyKit.Image,   // 来自 AppImage.Spec.Image
    Command:         ctr.Command,
    Args:            ctr.Args,
    WorkingDir:      ctr.WorkingDir,
    Env:             ctr.Env,
    ...
    ImagePullPolicy: ctr.ImagePullPolicy,
}
if ctr.ImagePullPolicy == "" {
    ctr.ImagePullPolicy = v1.PullIfNotPresent
}
```

7. Pod 被打上 `NodeSelector`：`kuscia.secretflow/namespace: <domainID>`，由对应域的 Kuscia agent 节点接收。
8. Kuscia scheduler plugin（`KusciaScheduling.Permit`）会等待所有参与方的 Pod 都进入 reserved 状态才放行。

### 10.4 任务配置的渲染与挂载

任务配置通过两套 ConfigMap 协作完成：

1. **模板 ConfigMap**：保存 AppImage 中 `configTemplates` 的原始模板字符串。名字为 `<task-name>-configtemplate`。
2. **取值 ConfigMap**：由 `generateKusciaConfigMap` 生成，保存运行时值，如 `TASK_ID`、`TASK_INPUT_CONFIG`、`TASK_CLUSTER_DEFINE`、`ALLOCATED_PORTS`。名字为 `<task-name>-kuscia-gen-conf`。

Pod 中把模板 ConfigMap 挂载成 volume，并通过 annotation 标记取值 ConfigMap：

```go
pod.Annotations[common.ConfigTemplateValueAnnotationKey] = confMap.Name
pod.Annotations[common.ConfigTemplateVolumesAnnotationKey] = configTemplateVolumeName
pod.Spec.Volumes = append(pod.Spec.Volumes, v1.Volume{
    Name: configTemplateVolumeName,
    VolumeSource: v1.VolumeSource{
        ConfigMap: &v1.ConfigMapVolumeSource{
            LocalObjectReference: v1.LocalObjectReference{
                Name: partyKit.ConfigTemplatesCMName,
            },
        },
    },
})
```

真正渲染发生在 agent 侧：`pkg/agent/middleware/plugins/hook/configrender/config_render.go` 在 `PointMakeMounts` 钩子触发时，读取取值 ConfigMap，用 `text/template` 替换模板中的 `{{.TASK_ID}}` 等占位符，生成最终文件再挂载到容器。

容器启动后执行的命令由 AppImage 模板定义：

```bash
sh -c "python -m secretflow.kuscia.entry ./kuscia/task-config.conf"
```

### 10.5 镜像拉取与运行时模式

Kuscia agent 支持三种运行时，配置在 `etc/conf/kuscia.yaml`：

```yaml
runtime: runc   # runc / runk / runp
```

| 运行时 | 说明 | 镜像来源 |
|---|---|---|
| `runc` | 通过 containerd/CRI 启动容器（默认） | 由 containerd 从镜像仓库拉取 `AppImage.Spec.Image` |
| `runk` | 在机构自有 Kubernetes 集群中创建 Pod | 由该集群的容器运行时拉取 |
| `runp` | 直接在当前 Kuscia 容器内以进程方式运行 | 读取本地 OCI store `/home/kuscia/var/images`，需要预先用 `kuscia image load` 或 `builtin` 导入 |

默认镜像拉取策略：

```go
if ctr.ImagePullPolicy == "" {
    ctr.ImagePullPolicy = v1.PullIfNotPresent
}
```

本地镜像存储默认路径：

```go
const defaultLocalImagePath = "var/images"

func DefaultImageStoreDir() string {
    return path.Join(common.DefaultKusciaHomePath(), defaultLocalImagePath)
}
```

### 10.5.1 runc 模式下，容器里的 Kuscia 如何拉起另一个容器？

这是 Kuscia 最容易让人困惑的地方之一：Kuscia 自己跑在 Docker 容器里，凭什么还能再拉起任务容器？答案是：**Kuscia 容器以 privileged 模式运行，内部又启动了一个独立的 containerd，利用 Linux 内核能力直接创建 runc 容器。这些任务容器在宿主机内核看来与 Kuscia 容器是“兄弟”关系，而不是嵌套 Docker 容器。**

#### 前提：privileged 权限检查

Kuscia 启动时会检查当前是否拥有特权能力。代码在 `cmd/kuscia/start/start.go`：

```go
if conf.Agent.Provider.Runtime == config.ContainerRuntime && !runtime.Permission.HasPrivileged() {
    nlog.Errorf("Runc must run with privileged mode")
    nlog.Errorf("Please run kuscia like: docker run --privileged secretflow/kuscia")
    return errors.New("permission is error")
}
```

`pkg/utils/runtime/permission.go` 读取 `/proc/self/status` 的 `CapEff` 字段，判断是否具备完整的特权 capability 掩码（`CAP_PRIVILEGED = 0x3ffffffff`）。只有 `--privileged` 才能让容器内的 containerd/runc 完成挂载 cgroup、创建 network namespace、操作 `/proc/sys` 等操作。

#### 1. 容器内启动 containerd

当 `runtime: runc` 时，`cmd/kuscia/modules/runtime.go` 会设置 `EnableContainerd = true`，并让 `agent` 模块依赖 `containerd` 模块。`cmd/kuscia/modules/containerd.go` 负责启动 containerd：

```go
func (s *containerdModule) Run(ctx context.Context) error {
    configPath := filepath.Join(s.Root, pkgcom.ConfPrefix, "containerd.toml")
    configPathTmpl := filepath.Join(s.Root, pkgcom.ConfPrefix, "containerd.toml.tmpl")
    if err := common.RenderConfig(configPathTmpl, configPath, s); err != nil {
        return err
    }
    ...
    sp := supervisor.NewSupervisor("containerd", nil, -1)
    return sp.Run(ctx, func(ctx context.Context) supervisor.Cmd {
        cmd := exec.Command(filepath.Join(s.Root, "bin/containerd"), args...)
        ...
    })
}
```

它会渲染 `etc/conf/containerd.toml.tmpl`，生成 `/home/kuscia/etc/conf/containerd.toml`，其中指定了 containerd 的 root/state 和 gRPC socket：

```toml
root = "{{.Root}}/containerd/root"
state = "{{.Root}}/containerd/run"

[grpc]
  address = "{{.Socket}}"
```

Socket 默认路径为：

```go
func ContainerdSocket() string {
    return path.Join(DefaultKusciaHomePath(), "/containerd/run/containerd.sock")
}
// /home/kuscia/containerd/run/containerd.sock
```

也就是说，**Kuscia 容器内部跑了一个完整的 containerd 守护进程**，它的数据目录在 `/home/kuscia/containerd/` 下（通常由 Docker volume 持久化）。

#### 2. Agent 通过 CRI 调用 containerd

Agent 启动时，`cmd/kuscia/modules/agent.go` 把 CRI endpoint 强制指向内部 containerd socket：

```go
conf.Provider.CRI.RemoteImageEndpoint = fmt.Sprintf("unix://%s", i.ContainerdSock)
conf.Provider.CRI.RemoteRuntimeEndpoint = fmt.Sprintf("unix://%s", i.ContainerdSock)
```

`pkg/agent/provider/pod/cri_provider.go` 创建远程 CRI 客户端：

```go
case config.ContainerRuntime:
    remoteRuntimeService, err = remote.NewRemoteRuntimeService(
        dep.CRIProviderCfg.RemoteRuntimeEndpoint,
        dep.CRIProviderCfg.RuntimeRequestTimeout, nil, nil)
    remoteImageService, err = remote.NewRemoteImageService(
        dep.CRIProviderCfg.RemoteImageEndpoint,
        dep.CRIProviderCfg.RuntimeRequestTimeout, nil, nil)
```

然后 Kuscia 就像 kubelet 一样，通过 CRI gRPC 接口让 containerd 干活：

- 创建 Pod 沙箱：`pkg/agent/kuberuntime/kuberuntime_sandbox.go`
  ```go
  podSandBoxID, err := m.runtimeService.RunPodSandbox(ctx, podSandboxConfig, runtimeHandler)
  ```

- 拉取镜像：`pkg/agent/kuberuntime/kuberuntime_image.go`
  ```go
  imageRef, err := m.imageService.PullImage(ctx, imgSpec, nil, podSandboxConfig)
  ```

- 创建并启动容器：`pkg/agent/kuberuntime/kuberuntime_container.go`
  ```go
  containerID, err := m.runtimeService.CreateContainer(ctx, podSandboxID, containerConfig, podSandboxConfig)
  err = m.runtimeService.StartContainer(ctx, containerID)
  ```

#### 3. 任务容器与 Kuscia 容器的关系

因为 Kuscia 容器是 privileged 的，内部的 containerd 可以直接操作宿主机的 cgroup、网络命名空间、mount 命名空间等内核资源。containerd 通过 `runc` 创建出来的任务容器：

- **不是 Docker 嵌套容器**：它们不是 `docker run` 出来的，宿主机 Docker daemon 看不到它们。
- **是宿主机内核上的普通 Linux 容器**：与 Kuscia 容器在同一层级，只是由 Kuscia 内部的 containerd 管理。
- **拥有独立的 PID/network/mount namespace**：和 Kuscia 容器本身隔离。

containerd 配置中使用了 CNI 插件：

```toml
[plugins."io.containerd.grpc.v1.cri".cni]
  bin_dir = "{{.Root}}/bin"
  conf_dir = "{{.Root}}/etc/cni/net.d"
```

任务容器会被分配到 `cni0` 网桥（子网 `10.88.0.0/16`），与 Kuscia 容器内部的网络命名空间无关。

#### 4. 为什么 runp 不需要这些？

`runp` 直接在当前 Kuscia 容器内以进程方式运行 SecretFlow，不经过 containerd/runc，因此不需要 privileged，也不需要额外的容器隔离。但它共享 Kuscia 容器的 PID/network namespace，隔离性比 runc 弱。

#### 小结

| 运行时 | Kuscia 容器内是否启动 containerd | 任务是否是独立 Linux 容器 | 是否需要 privileged |
|---|---|---|---|
| `runc` | 是 | 是（宿主机内核上的兄弟容器） | 是 |
| `runp` | 否 | 否（Kuscia 容器内的进程） | 否 |
| `runk` | 否（复用外部 K8s） | 是（外部 K8s Pod） | 否 |

所以，`runc` 模式下“容器里的 Kuscia 拉起另一个容器”的本质是：**privileged Kuscia 容器内部运行了一个 containerd，containerd 直接在宿主机内核上创建 runc 容器，这些任务容器与 Kuscia 容器是平级的，只是由 Kuscia 内部管理。**

### 10.6 `kuscia-secretflow` 一体化镜像的特别之处

`kuscia-secretflow` 把 SecretFlow 运行环境打包进 Kuscia 镜像，并在构建时把镜像注册到本地 OCI store：

```dockerfile
ARG SF_VERSION="1.11.0b1"
RUN pip install secretflow-lite==${SF_VERSION} ... && \
    kuscia image --store /home/kuscia/var/images --runtime runp builtin \
      secretflow/secretflow-lite-anolis8:${SF_VERSION}
```

`kuscia image builtin` 的实现在：

- CLI：`cmd/kuscia/image/cmd/builtin.go`
- Store：`pkg/agent/local/store/oci_store.go#RegisterImage`

它在 `/home/kuscia/var/images/repositories` 下写入一个带 tag 的 OCI 镜像记录，使得 `runp` 运行时可以直接找到该镜像而无需从仓库拉取。

部署脚本 `scripts/deploy/kuscia.sh` 检测到镜像内已有内置镜像且 `runtime=runp` 时，会自动在容器内重新注册：

```bash
if [[ -n "$(docker run --rm "$KUSCIA_IMAGE" bash -c 'ls -A /home/kuscia/var/images')" ]] && [[ ${runtime} == "runp" ]]; then
   builtin_images=$(docker run --rm "$KUSCIA_IMAGE" bash -c 'kuscia image ls --runtime runp | sed "1d" | awk "{print \$1\":\"\$2}"')
   for image in $builtin_images; do
     docker exec -it "${domain_ctr}" bash -c "kuscia image --store /home/kuscia/var/images --runtime runp builtin '${image}'"
   done
fi
```

注意：无论镜像是分开还是一体化，**AppImage CRD 都需要单独注册**（例如通过 `create_sf_app_image.sh`），KusciaJob 通过 `appImageRef` 引用的是 AppImage 名字，而不是镜像名字。

### 10.7 调试时如何追踪

- 查看 AppImage：`kubectl get appimage <name> -o yaml`
- 查看 KusciaJob 生成的 KusciaTask：`kubectl get kusciatask -n cross-domain`
- 查看生成的 Pod：`kubectl get pod -n <domain-namespace>`
- 查看任务容器日志：`kubectl logs <pod-name> -n <domain-namespace>`
- 查看 config-render 日志：在 agent 日志中搜索 `fillTemplateValueFromConfigMap`
- 查看本地镜像：`kuscia image ls --runtime runp`

## 11. 附录：官方参考路径

| 项目 | 关键文件 |
|---|---|
| Kuscia | `kuscia/Makefile` |
| | `kuscia/scripts/make/image.mk` |
| | `kuscia/scripts/make/golang.mk` |
| | `kuscia/hack/build.sh` |
| | `kuscia/build/dockerfile/kuscia-anolis.Dockerfile` |
| | `kuscia/build/dockerfile/base/kuscia-deps.Dockerfile` |
| | `kuscia/build/dockerfile/kuscia-secretflow.Dockerfile` |
| | `kuscia/scripts/deploy/create_sf_app_image.sh` |
| | `kuscia/scripts/templates/app_image.secretflow.yaml` |
| SecretFlow | `secretflow/pyproject.toml` |
| | `secretflow/docker/dev/Dockerfile` |
| | `secretflow/docker/dev/build.sh` |
| | `secretflow/docker/dev/entry.sh` |
| | `secretflow/docker/release/ubuntu.Dockerfile` |
| | `secretflow/docker/release/build.sh` |
| | `secretflow/docker/config_templates.yml` |
| | `secretflow/docker/deploy_templates.yml` |
| AppImage / KusciaJob 调度 | `pkg/crd/apis/kuscia/v1alpha1/appimage_types.go` |
| | `pkg/crd/apis/kuscia/v1alpha1/kusciajob_types.go` |
| | `pkg/crd/apis/kuscia/v1alpha1/kusciatask_types.go` |
| | `pkg/utils/resources/appimage.go` |
| | `pkg/controllers/kusciajob/handler/scheduler.go` |
| | `pkg/controllers/kusciatask/handler/pending_handler.go` |
| | `pkg/agent/middleware/plugins/hook/configrender/config_render.go` |
| | `pkg/agent/config/agent_config.go` |
| | `pkg/agent/local/store/oci_store.go` |
| | `cmd/kuscia/image/cmd/builtin.go` |
| runc 运行时 | `cmd/kuscia/modules/containerd.go` |
| | `cmd/kuscia/modules/runtime.go` |
| | `cmd/kuscia/modules/agent.go` |
| | `cmd/kuscia/start/start.go` |
| | `pkg/utils/runtime/permission.go` |
| | `pkg/agent/provider/pod/cri_provider.go` |
| | `pkg/agent/kuberuntime/kuberuntime_sandbox.go` |
| | `pkg/agent/kuberuntime/kuberuntime_container.go` |
| | `pkg/agent/kuberuntime/kuberuntime_image.go` |
| | `etc/conf/containerd.toml.tmpl` |