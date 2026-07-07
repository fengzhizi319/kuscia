# Kuscia Makefile 子模块详解

> 本文档详细说明 Kuscia 顶层 `Makefile` 如何通过 `-f scripts/make/*.mk` 将构建目标拆分到多个子模块中，以及每个 `.mk` 文件的作用、内容和执行逻辑。

---

## 1. 整体设计：为什么要把 Makefile 拆成多个 `.mk`？

Kuscia 是一个 Go 项目，除了编译 Go 代码外，还需要：

- 生成 CRD、clientset、proto 代码；
- 构建 Docker 镜像（Kuscia 主镜像、deps 镜像、proot 镜像、监控镜像等）；
- 构建文档（Sphinx）；
- 执行各种 linter（Go、YAML、Markdown、License、Shell、拼写）；
- 与 FATE 生态集成。

如果把所有目标都写在一个 `Makefile` 里，文件会非常臃肿，且不同职责的目标容易互相干扰。因此项目采用了**“顶层委托 + 子模块拆分”**的设计：

1. 顶层 `Makefile` 本身**不定义任何具体目标**（除了一个 `_run` 入口）。
2. 顶层 `Makefile` 通过模式规则 `$(MAKECMDGOALS): %: _run`，把用户输入的任意目标转发给 `_run`。
3. `_run` 再调用一次递归 `make`，显式加载 `scripts/make/` 下的各个 `.mk` 文件，并把原始目标传进去执行。

这样每个 `.mk` 只负责一类目标，便于维护、扩展和阅读。

---

## 2. 顶层 Makefile 的转发机制

```makefile
# kuscia/Makefile
_run:
 @$(MAKE) --warn-undefined-variables \
  -f scripts/make/common.mk \
  -f scripts/make/docs.mk \
  -f scripts/make/image.mk \
  -f scripts/make/golang.mk \
  -f scripts/make/lint.mk \
  -f scripts/make/fate.mk \
  $(MAKECMDGOALS)

$(if $(MAKECMDGOALS),$(MAKECMDGOALS): %: _run)
```

### 关键点

| 项 | 说明 |
| --- | --- |
| `$(MAKECMDGOALS)` | 用户输入的目标，例如 `image`、`test`、`help`。 |
| `_run` | 实际执行递归 make 的目标。 |
| `-f <file>` | 显式指定本次递归 make 要读取的 Makefile 文件。可以多次指定，后加载的文件中的定义会覆盖前面同名定义（按 Makefile 规则）。 |
| `--warn-undefined-variables` | 开启未定义变量警告，便于排查拼写错误。 |
| `$(if $(MAKECMDGOALS),...,...)` | 当用户显式指定目标时，为每个目标生成一条模式规则：`%: _run`，即任意目标都先走 `_run`。 |

### 执行流程示例：`make image`

```text
用户输入：make image
   │
   ▼
顶层 Makefile 匹配模式规则 image: _run
   │
   ▼
执行 _run：递归调用 make
   -f common.mk -f docs.mk -f image.mk -f golang.mk -f lint.mk -f fate.mk image
   │
   ▼
子 make 读取 6 个 .mk 文件，合并变量和目标
   │
   ▼
找到 image 目标（定义在 image.mk）并执行
   │
   ▼
image 依赖 build（golang.mk），build 依赖 check_code（golang.mk）
check_code 依赖 fmt、vet（golang.mk）和 verify_error_code（docs.mk）
   │
   ▼
依次执行：fmt → vet → verify_error_code → check_code → build → image
```

---

## 3. 各 `.mk` 文件详解

### 3.1 `common.mk` —— 全局公共变量与工具目标

**文件位置**：`scripts/make/common.mk`

**作用**：

- 设置 Make 使用的 shell（bash + pipefail）。
- 定义项目级公共变量：时间戳、版本号、目标架构、`GOBIN`、默认 `GOOS`。
- 提供日志宏 `log`、`errorLog` 和 `LOG_TARGET`。
- 定义代码生成相关目标。
- 定义 `help` 目标，用于打印分组帮助信息。

#### 3.1.1 关键配置与变量

```makefile
.SECONDARY:
```

- `.SECONDARY` 告诉 Make：不要自动删除中间文件。某些隐式规则会默认删除中间产物，开启此选项可以避免意想不到的清理行为。

```makefile
SHELL:=/bin/bash
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec
```

- 指定 recipe 使用 **bash** 执行，而不是默认的 `/bin/sh`。
- `-o pipefail`：管道中任一命令失败，整个管道返回非零。
- `.SHELLFLAGS = -ec`：`-e` 让 shell 遇到错误立即退出；`-c` 表示从字符串读取命令（Make 默认行为）。

```makefile
DATETIME = $(shell date +"%Y%m%d%H%M%S")
KUSCIA_VERSION_TAG = $(shell git describe --tags --always)
```

- `DATETIME`：生成 `20260707183000` 格式的时间戳，用于镜像 tag。
- `KUSCIA_VERSION_TAG`：通过 `git describe --tags --always` 获取最近的 tag，例如 `v1.2.0b0-21-g09537e5`。

```makefile
ifeq ($(origin ARCH), undefined)
UNAME_M_OUTPUT := $(shell uname -m)
ARCH = $(if $(filter aarch64 arm64,$(UNAME_M_OUTPUT)),arm64,\
       $(if $(filter amd64 x86_64,$(UNAME_M_OUTPUT)),amd64,$(UNAME_M_OUTPUT)))
endif
```

- 如果命令行没有传入 `ARCH`，则根据 `uname -m` 自动推断：
  - `aarch64` / `arm64` → `arm64`
  - `amd64` / `x86_64` → `amd64`
  - 其他 → 保持原样
- 可通过 `make image ARCH=arm64` 覆盖。

```makefile
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

export GOOS=linux
```

- `GOBIN`：Go 工具链安装目录，用于查找 `go-junit-report`、`gocover-cobertura` 等工具。
- `GOOS=linux`：固定为 Linux，确保在 macOS/Windows 上也能交叉编译出 Linux 二进制。

```makefile
LOG_TARGET = echo -e "\033[0;32m==================> Running $@ ============> ... \033[0m"

define log
echo -e "\033[36m==================>$1\033[0m"
endef

define errorLog
echo -e "\033[0;31m==================>$1\033[0m"
endef
```

- `LOG_TARGET`：打印绿色 “Running xxx” 日志。`$@` 代表当前目标名。
- `log` / `errorLog`：青色/红色日志宏，供其他 `.mk` 调用。

#### 3.1.2 主要目标执行步骤

| 目标 | 执行命令 | 说明 |
| --- | --- | --- |
| `manifests` | `bash hack/generate-crds.sh` | 根据 Go types 生成 Kubernetes CRD YAML。 |
| `gen-clientset` | `bash hack/update-codegen.sh` | 生成 CRD 对应的 clientset、informer、lister。 |
| `gen-proto-code` | `bash hack/proto-to-go.sh` | 将 `.proto` 文件编译为 Go 代码。 |
| `generate` | `manifests` → `gen-clientset` → `gen-proto-code` | 一键执行上述三种代码生成。 |
| `help` | `awk` 扫描 `$(MAKEFILE_LIST)` | 解析所有 `.mk` 中的 `##@`（分组）和 `##`（目标描述）注释，输出彩色帮助。 |

**`help` 输出示例**：

```text
Kuscia (Kubernetes-based Secure Collaborative Infra) ...

Usage:
  make <Target> <Option>

Targets:
  Common
    generate        Generate all code that Kuscia needs.
    help            Show this help info.

  Build
    build           build kuscia binary.
    test            Run tests.
    ...
```

**加载顺序**：`common.mk` 必须**第一个**加载，因为后续 `.mk` 会用到 `LOG_TARGET`、`DATETIME`、`ARCH` 等变量。

---

### 3.2 `docs.mk` —— 文档构建与校验

**文件位置**：`scripts/make/docs.mk`

**作用**：

- 使用 Sphinx 构建中文/英文文档。
- 检查文档中的死链（linkinator）。
- 生成和校验错误码国际化文档。
- 版本一致性检查。

#### 3.2.1 核心变量

```makefile
include .VERSION
DOCS_ROOT_DIR        ?= docs
DOCS_SOURCE_DIR      = .
DOCS_OUTPUT_DIR      = _build

SPHINX_BUILD         ?= sphinx-build
SPHINX_AUTOBUILD     ?= sphinx-autobuild
SPHINX_OPTS          ?= -b html
LANGUAGE             ?= zh_CN

VERSION_CHECK_SCRIPT ?= hack/version_check.sh
VERSION_CHECK_DIRS   ?= docs scripts hack
```

- `.VERSION`：引入版本号文件，供 `version_check` 使用。
- `DOCS_ROOT_DIR` / `DOCS_SOURCE_DIR` / `DOCS_OUTPUT_DIR`：文档源目录、输出目录。
- `LANGUAGE`：默认构建中文文档（`zh_CN`）。

#### 3.2.2 主要目标执行步骤

| 目标 | 依赖 | 执行步骤 |
| --- | --- | --- |
| `sphinx-clean` | 无 | 1. 判断 `docs/_build/` 是否存在。<br>2. 若存在则 `rm -rf docs/_build`。 |
| `sphinx-build` | `sphinx-clean` `markdown-check` | 1. 打印日志。<br>2. 输出 `python3`、`pip3`、`sphinx-build` 版本。<br>3. 执行 `sphinx-build -b html -D language=zh_CN docs/. docs/_build`。<br>4. 执行 `make link-check` 检查死链。<br>5. 打印成功日志。 |
| `sphinx-preview` | `sphinx-build` | 执行 `sphinx-autobuild docs/. docs/_build`，启动带热重载的本地预览服务器。 |
| `link-check` | 无 | 1. 判断 `docs/_build/` 是否存在。<br>2. 若存在，执行 `linkinator docs/_build -r --concurrency 25 --skip <忽略列表>`。 |
| `verify_error_code` | 无 | 执行 `bash hack/errorcode/gen_error_code_doc.sh verify proto/api/v1alpha1/errorcode/error_code.proto hack/errorcode/i18n/errorcode.zh-CN.toml`。 |
| `gen_error_code_doc` | `verify_error_code` | 执行 `bash hack/errorcode/gen_error_code_doc.sh doc ... docs/reference/apis/error_code_cn.md`，生成错误码中文文档。 |
| `version_check` | 无 | 执行 `bash hack/version_check.sh --kuscia-version ... --secretflow-version ... --check-dirs ... --mode check`。 |
| `version_fix` | 无 | 与 `version_check` 类似，但 `--mode fix`，自动修复不一致的版本号。 |
| `docs` | `docs-clean` `gen_error_code_doc` `sphinx-build` | 最常用的文档构建入口：先清理，再生成错误码文档，再 Sphinx 构建。 |
| `docs-clean` | `sphinx-clean` | 清理文档构建产物。 |
| `docs-preview` | `docs-clean` `sphinx-preview` | 清理后启动预览服务器。 |
| `docs-link-check` | `link-check` | 检查文档链接。 |

**典型用法**：

```bash
make docs
make docs-clean
make docs-preview
make docs-link-check
make gen_error_code_doc
```

---

### 3.3 `image.mk` —— Docker 镜像构建

**文件位置**：`scripts/make/image.mk`

**作用**：

- 构建 Kuscia 运行所需的各种 Docker 镜像。
- 定义镜像 tag、基础镜像地址、buildx builder 创建/切换逻辑。

#### 3.3.1 核心变量

```makefile
TAG = ${KUSCIA_VERSION_TAG}-${DATETIME}
IMG := secretflow/kuscia:${TAG}

PROOT_IMAGE ?= secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/proot
ENVOY_IMAGE ?= secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-envoy:0.6.2b0
DEPS_IMAGE  ?= secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia-deps:0.7.0b0
```

| 变量 | 说明 | 示例值 |
| --- | --- | --- |
| `TAG` | `${KUSCIA_VERSION_TAG}-${DATETIME}` | `v1.2.0b0-21-g09537e5-20260707183000` |
| `IMG` | Kuscia 主镜像完整名称 | `secretflow/kuscia:v1.2.0b0-21-g09537e5-20260707183000` |
| `PROOT_IMAGE` | proot 镜像名 | `secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/proot` |
| `ENVOY_IMAGE` | Envoy 网关镜像 | `.../kuscia-envoy:0.6.2b0` |
| `DEPS_IMAGE` | 依赖基础镜像 | `.../kuscia-deps:0.7.0b0` |

#### 3.3.2 核心宏：`start_docker_buildx`

```makefile
define start_docker_buildx
 if [ -z "$$(docker buildx inspect kuscia 2>/dev/null)" ]; then \
  echo "create kuscia builder"; \
  docker buildx create --name kuscia --platform linux/arm64,linux/amd64; \
 fi; \
 docker buildx use kuscia
endef
```

执行步骤：

1. `docker buildx inspect kuscia 2>/dev/null`：检查名为 `kuscia` 的 buildx builder 是否存在。
   - 如果存在，输出非空字符串；`[ -z ... ]` 为假，跳过创建。
   - 如果不存在，输出为空或报错（stderr 被重定向）；`[ -z ... ]` 为真，进入 `then`。
2. 打印 `create kuscia builder`。
3. 执行 `docker buildx create --name kuscia --platform linux/arm64,linux/amd64`，创建一个支持双平台的 builder。
4. 执行 `docker buildx use kuscia`，切换到该 builder。

#### 3.3.3 主要目标执行步骤

| 目标 | 依赖 | 执行步骤 |
| --- | --- | --- |
| `proot` | 无 | 1. `export GOARCH=${ARCH}`。<br>2. 调用 `start_docker_buildx`。<br>3. `DOCKER_BUILDKIT=1 docker buildx build -t ${PROOT_IMAGE} -f ./build/dockerfile/proot-build.Dockerfile . --platform linux/${ARCH} --load`。 |
| `deps-image` | 无 | 1. 调用 `start_docker_buildx`。<br>2. `docker buildx build -t ${DEPS_IMAGE} -f ./build/dockerfile/base/kuscia-deps.Dockerfile . --platform linux/${ARCH} --load`。 |
| `image` | `build`（golang.mk） | 1. `export GOARCH=${ARCH}`。<br>2. 设置 `DOCKER_BUILDKIT=1`。<br>3. 调用 `start_docker_buildx`。<br>4. `docker buildx build -t ${IMG} --build-arg KUSCIA_ENVOY_IMAGE=${ENVOY_IMAGE} --build-arg DEPS_IMAGE=${DEPS_IMAGE} -f ./build/dockerfile/kuscia-anolis.Dockerfile . --platform linux/${ARCH} --load`。 |
| `build-monitor` | 无 | `docker build -t secretflow/kuscia-monitor -f ./build/dockerfile/kuscia-monitor.Dockerfile .`（普通 build，非 buildx）。 |

**`make image` 的完整依赖链**：

```text
image (image.mk)
  └── build (golang.mk)
        └── check_code (golang.mk)
              ├── fmt (golang.mk)
              ├── vet (golang.mk)
              └── verify_error_code (docs.mk)
```

所以 `make image` 会先格式化、静态检查、错误码校验，再编译，最后构建 Docker 镜像。

**典型用法**：

```bash
make image
make image ARCH=arm64
make proot
make deps-image
make build-monitor
```

```text

```

---

### 3.4 `golang.mk` —— Go 代码编译与测试

**文件位置**：`scripts/make/golang.mk`

**作用**：

- 负责 Go 代码格式化、静态检查、单元测试、编译、清理。
- 定义 `build` 目标，供 `image.mk` 中的 `image` 目标依赖。
- 提供集成测试入口。

#### 3.4.1 核心变量

```makefile
CMD_EXCLUDE_TESTS = "example|webdemo|testing|test|container"
PKG_EXCLUDE_TESTS = "crd|testing|test"
TEST_SUITE ?= all
```

- `CMD_EXCLUDE_TESTS` / `PKG_EXCLUDE_TESTS`：用于 `go list ... | grep -Ev` 过滤掉不需要跑单元测试的包。
- `TEST_SUITE`：集成测试套件名称，默认 `all`。

#### 3.4.2 主要目标执行步骤

| 目标 | 依赖 | 执行步骤 |
| --- | --- | --- |
| `fmt` | 无 | 执行 `go fmt ./...`，格式化所有 Go 源文件。 |
| `vet` | 无 | 执行 `go vet ./...`，进行静态分析。 |
| `test` | 无 | 1. `rm -rf ./test-results` 并 `mkdir -p test-results`。<br>2. 对 `cmd/...` 包执行 `go test -v ... --parallel 4 -gcflags="all=-N -l" -coverprofile=test-results/cmd.covprofile.out`，结果 `tee` 到 `test-results/cmd.output.txt`。<br>3. 对 `pkg/...` 包执行类似测试，输出到 `test-results/pkg.output.txt`。<br>4. 用 `go-junit-report` 将输出转为 JUnit XML。<br>5. 合并覆盖率文件到 `test-results/coverage.out`。<br>6. 用 `gocover-cobertura` 生成 `test-results/coverage.xml`。 |
| `clean` | 无 | 删除 `test-results`、`build/apps`、`build/framework`、`tmp-crd-code`、`build/linux`。 |
| `build` | `check_code` | 1. 执行 `bash hack/build.sh -t kuscia` 编译 kuscia 二进制。<br>2. `mkdir -p build/linux/${ARCH}`。<br>3. `cp -rp build/apps build/linux/${ARCH}`，整理 Dockerfile 所需的目录结构。 |
| `check_code` | `fmt` `vet` `verify_error_code` | 1. 先执行 `fmt`。<br>2. 执行 `vet`。<br>3. 执行 `verify_error_code`（docs.mk）。<br>4. 打印 “check code FINISH”。 |
| `integration_test` | `image` | 1. `mkdir -p run/test`。<br>2. 从刚构建的 Kuscia 镜像中提取 `/home/kuscia/tests/integration_test.sh` 到 `run/test/` 并赋予执行权限。<br>3. 执行 `run/test/integration_test.sh ${TEST_SUITE}`。 |

**典型用法**：

```bash
make build
make test
make clean
make integration_test TEST_SUITE=center.base
```

---

### 3.5 `lint.mk` —— 多维度代码规范检查

**文件位置**：`scripts/make/lint.mk`

**作用**：

- 聚合所有 linter 工具，统一入口。
- 包括 Go、YAML、Markdown、License 头、Shell、拼写检查。

#### 3.5.1 主要目标执行步骤

| 目标 | 执行步骤 |
| --- | --- |
| `lint-golang` | 1. `golangci-lint --version`。<br>2. `golangci-lint run --out-format=colored-line-number --config=.golangci.yml`。 |
| `lint-yaml` | 1. `yamllint --version`。<br>2. `yamllint --config-file=./scripts/linter/yamllint/.yamllint .`。 |
| `lint-markdown` | 执行 `markdownlint --config ./scripts/linter/markdown/markdown_lint_config.yaml --fix '**/*.md'`，自动修复 Markdown 风格问题。 |
| `lint-codespell-check` | 1. 读取 `scripts/linter/codespell/.codespell.skip`，将换行转为逗号。<br>2. 执行 `codespell --skip <列表> --ignore-words scripts/linter/codespell/.codespell.ignorewords`。 |
| `lint-license-check` | 执行 `license-eye -c scripts/linter/license/.licenserc.yaml header check`，检查 Apache 许可证头。 |
| `lint-license-fix` | 执行 `license-eye -c scripts/linter/license/.licenserc.yaml header fix`，自动补齐许可证头。 |
| `lint-shell-check` | 1. `shellcheck --version`。<br>2. 用 `shellcheck -e SC1091,SC2034` 检查 `./hack/**/*.sh` 和 `./scripts/**/*.sh`。 |
| `check` | 依次执行：`go-check` → `yaml-check` → `shell-check` → `markdown-check` → `codespell-check`。 |

**别名目标**（只做一层转发）：

| 别名 | 实际目标 |
| --- | --- |
| `go-check` | `lint-golang` |
| `yaml-check` | `lint-yaml` |
| `shell-check` | `lint-shell-check` |
| `markdown-check` | `lint-markdown` |
| `codespell-check` | `lint-codespell-check` |
| `license-fix` | `lint-license-fix` |
| `license-check` | `lint-license-check` |

**典型用法**：

```bash
make check
make lint-golang
make lint-yaml
make lint-license-fix
```

---

### 3.6 `fate.mk` —— FATE 生态集成构建

**文件位置**：`scripts/make/fate.mk`

**作用**：

- 构建 Kuscia 与 FATE（联邦学习框架）集成所需的适配镜像。
- 目标代码位于 `thirdparty/fate/`。

#### 3.6.1 核心变量

```makefile
DATETIME = $(shell date +"%Y%m%d%H%M%S")
COMMIT_ID = $(shell git log -1 --pretty="format:%h")
TAG = 0.0.1
DEPLOY_IMG ?= secretflow/fate-deploy-basic:${TAG}
ADAPTER_IMG ?= secretflow/fate-adapter:${TAG}
```

| 变量 | 说明 | 默认值 |
| --- | --- | --- |
| `TAG` | 镜像 tag | `0.0.1` |
| `DEPLOY_IMG` | FATE 部署镜像 | `secretflow/fate-deploy-basic:0.0.1` |
| `ADAPTER_IMG` | FATE 适配镜像 | `secretflow/fate-adapter:0.0.1` |

#### 3.6.2 主要目标执行步骤

| 目标 | 依赖 | 执行步骤 |
| --- | --- | --- |
| `fate-clean` | 无 | `rm -rf ./thirdparty/fate/build/app`。 |
| `fate-build` | `fmt` `vet` | 1. `export GOARCH=amd64`。<br>2. 执行 `bash ./thirdparty/fate/hack/build.sh`。 |
| `fate-adaptor-app-image` | `fate-build` | `docker build -t ${ADAPTER_IMG} -f ./thirdparty/fate/build/dockerfile/kuscia-job-adapter.Dockerfile ./thirdparty/fate`。 |
| `deploy-image` | 无 | `docker build -t ${DEPLOY_IMG} -f ./thirdparty/fate/build/dockerfile/deploy.Dockerfile ./thirdparty/fate`。 |

**典型用法**：

```bash
make fate-build
make fate-adaptor-app-image
make deploy-image
```

---

## 4. 执行逻辑与变量覆盖

### 4.1 变量加载顺序

子 make 按以下顺序读取 `.mk` 文件：

```
common.mk → docs.mk → image.mk → golang.mk → lint.mk → fate.mk
```

- `common.mk` 最先加载，提供 `DATETIME`、`ARCH`、`LOG_TARGET` 等公共变量。
- 后续 `.mk` 可以直接使用这些变量。
- 如果多个 `.mk` 定义了同名变量，**后加载的覆盖先加载的**（除非使用 `?=` 等条件赋值）。
- 命令行传入的变量优先级最高，例如 `make image ARCH=arm64` 会覆盖 `common.mk` 中自动检测的 `ARCH`。

### 4.2 目标合并

所有 `.mk` 中的目标会被合并到同一个 make 名字空间中。因此：

- `image.mk` 的 `image` 目标可以依赖 `golang.mk` 的 `build` 目标。
- `golang.mk` 的 `check_code` 目标可以依赖 `docs.mk` 的 `verify_error_code` 目标。
- `help` 目标在 `common.mk` 中定义，但会扫描所有已加载 `.mk` 中的 `##@` 和 `##` 注释。

### 4.3 为什么 `make image` 会先执行 fmt / vet / verify_error_code？

因为目标依赖链是：

```text
image
  └── build
        └── check_code
              ├── fmt
              ├── vet
              └── verify_error_code
```

Make 会自动按依赖顺序执行，所以你会在日志中依次看到：

```text
Running fmt
Running vet
Running verify_error_code
Running check_code
Running build
Running image
```

如果你只想快速构建镜像、跳过检查，可以手动分步执行：

```bash
# 只编译二进制
bash hack/build.sh -t kuscia

# 直接构建镜像（不触发 fmt/vet）
DOCKER_BUILDKIT=1 docker buildx build \
  --build-arg KUSCIA_ENVOY_IMAGE=... \
  --build-arg DEPS_IMAGE=... \
  -f build/dockerfile/kuscia-anolis.Dockerfile \
  -t secretflow/kuscia:custom-tag \
  --platform linux/amd64 --load .
```

---

## 5. 常用命令速查

| 命令 | 说明 |
| --- | --- |
| `make help` | 查看所有目标及分组帮助。 |
| `make image` | 代码检查 → 编译 → 构建 Kuscia Docker 镜像。 |
| `make image ARCH=arm64` | 指定 arm64 架构构建镜像。 |
| `make build` | 仅编译 kuscia 二进制。 |
| `make test` | 运行单元测试并生成报告。 |
| `make clean` | 清理构建和测试产物。 |
| `make check` | 执行全部 linter 检查。 |
| `make docs` | 构建文档。 |
| `make docs-preview` | 本地预览文档。 |
| `make generate` | 生成 CRD、clientset、proto 代码。 |
| `make integration_test` | 构建镜像后运行集成测试。 |

---

## 6. 小结

| `.mk` 文件 | 主要职责 | 典型目标 |
| --- | --- | --- |
| `common.mk` | 公共变量、shell 设置、代码生成、help | `generate`、`manifests`、`help` |
| `docs.mk` | 文档构建、死链检查、错误码文档、版本校验 | `docs`、`docs-preview`、`gen_error_code_doc` |
| `image.mk` | Docker 镜像构建、buildx builder 管理 | `image`、`deps-image`、`proot`、`build-monitor` |
| `golang.mk` | Go 编译、测试、清理、集成测试 | `build`、`test`、`clean`、`integration_test` |
| `lint.mk` | 各类 linter 检查 | `check`、`lint-golang`、`lint-yaml`、`lint-license-fix` |
| `fate.mk` | FATE 集成镜像构建 | `fate-build`、`fate-adaptor-app-image`、`deploy-image` |

顶层 `Makefile` 通过 `-f` 把这些文件拼接成一个完整的 Makefile 环境，再通过依赖链驱动目标执行。理解每个 `.mk` 的职责后，就可以根据需要单独调用或组合使用这些目标。
