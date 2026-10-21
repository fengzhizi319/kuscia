# Kuscia 脚本测试

本目录提供 Kuscia Shell 脚本的测试入口。测试分为三层，互不依赖，可以单独运行：

> 本文档中的 `bash` 代码块可直接点击运行（VS Code / Qoder 的代码块运行按钮），命令默认在
> **Kuscia 仓库根目录** 执行。

| 层级 | 内容 | 入口 | 依赖 |
| --- | --- | --- | --- |
| 静态检查 | 对所有 `scripts/**/*.sh`、`hack/**/*.sh` 执行 `bash -n`；对约定范围执行 ShellCheck | `run_local_tests.sh --lint-only` | Bash；ShellCheck 可选 |
| 单元测试 | 基于内置 shunit2 的离线测试，使用 fake `kubectl`/`openssl` 隔离外部环境 | `run_local_tests.sh --unit-only` | Bash + 仓库内 `tests/vendor/shunit2` |
| 集成测试 | 在 Docker 中拉起 Kuscia 集群，验证真实任务链路 | `run_integration_tests.sh` | Docker、Kuscia 镜像、网络 |

推荐的统一入口：

```bash
make test-scripts               # 静态检查 + ShellCheck + 单元测试（无需 Docker）
make test-scripts-unit          # 仅单元测试
make test-scripts-lint          # 仅 bash -n + ShellCheck
make test-integration-scripts   # Docker 集成测试
```

## 离线测试

### 运行入口

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
bash scripts/test/run_local_tests.sh --unit-only --filter app_image -v
```

```bash
bash scripts/test/run_local_tests.sh --help
```

| 选项 | 说明 |
| --- | --- |
| `--lint-only` | 仅执行 `bash -n` 与 ShellCheck |
| `--unit-only` | 仅执行 `scripts/test/unit/` 下的单元测试 |
| `--no-shellcheck` | 即使已安装 ShellCheck 也跳过 |
| `--filter PATTERN` | 仅运行路径包含 `PATTERN` 的测试文件 |
| `-v, --verbose` | 打印每个被检查的文件 |
| `-h, --help` | 显示帮助 |

`--filter` 按测试文件路径做子串匹配，例如 `--filter cert` 只会运行
`scripts/test/unit/test_generate_cert.sh`。

### 检查范围

- `bash -n`：递归检查 `scripts/` 与 `hack/` 下的全部 `.sh` 文件，新增脚本无需登记。
- ShellCheck：与 `make shell-check` 保持一致（`scripts/**/*.sh`、`hack/**/*.sh` 的一级子目录），
  并显式加入 `scripts/test/unit/*.sh`，避免新增测试脱离检查。统一忽略 `SC1091,SC2034`。
- ShellCheck 未安装时自动跳过，不会让离线测试失败。

### 现有单元测试

| 文件 | 覆盖内容 |
| --- | --- |
| `unit/test_app_image.sh` | `create_sf_app_image.sh` / `create_secretflow_app_image.sh` 的模板渲染、`SF_NAME` 覆盖、PSI 模板、缺少参数与无 Tag 镜像的拒绝逻辑 |
| `unit/test_generate_cert.sh` | `generate_rsa_key.sh` 输出可解码的私钥；`generate_cert.sh` 的参数校验与 `.key/.csr/.crt` 生成（未安装 openssl 时跳过） |
| `unit/test_cli_contract.sh` | `deploy.sh`、`kuscia.sh`、`add_domain*.sh`、`create_token.sh`、`run_local_kuscia.sh` 的帮助输出、非法模式与必填参数契约 |

### 编写新的单元测试

1. 在 `scripts/test/unit/` 下新建 `test_<feature>.sh`，文件会自动被发现。
2. 通过相对路径定位仓库根目录，并加载仓库内置的 shunit2：

   ```bash
   TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
   ROOT_DIR="$(cd "${TEST_DIR}/../../.." && pwd -P)"
   SHUNIT2_PATH="${ROOT_DIR}/tests/vendor/shunit2"

   # ... 测试函数 ...

   # shellcheck source=/dev/null
   . "${SHUNIT2_PATH}"
   ```

3. 使用 fake 命令隔离外部依赖。例如把临时目录放到 `PATH` 最前面，拦截 `kubectl`：

   ```bash
   cat > "${TEST_TMP}/bin/kubectl" <<'FAKE_KUBECTL'
   #!/bin/bash
   cat > "${KUBECTL_CAPTURE:?KUBECTL_CAPTURE is not set}"
   FAKE_KUBECTL
   export PATH="${TEST_TMP}/bin:${ORIGINAL_PATH}"
   ```

4. **注意 PATH 生命周期**：本仓库内置的 shunit2 会把 `oneTimeSetUp`、`tearDown` 等存根放在临时目录并加入
   `PATH`。如果测试需要修改 `PATH`，请在 `oneTimeSetUp()` 中保存 `PATH`，不要在文件顶层保存，否则
   `setUp()` 恢复 `PATH` 时会丢掉 shunit2 的存根目录，导致 `oneTimeTearDown: command not found`。

5. 常用断言：`assertEquals [message] expected actual`、`assertContains [message] haystack needle`、
   `assertNotContains`、`assertTrue [message] command`、`assertNotNull`。需要按条件跳过时使用
   `startSkipping` 并从测试函数返回。

6. 本地验证：

   ```bash
   bash scripts/test/run_local_tests.sh --unit-only --filter <feature>
   make test-scripts
   ```

## 集成测试

### 前置条件

- 本机已安装 Docker，当前用户有权限访问 Docker。
- 本地已存在 Kuscia 镜像。构建镜像并查看当前可用镜像：

  ```bash
  make image
  docker images --format '{{.Repository}}:{{.Tag}}' --filter reference='secretflow/kuscia:*'
  ```

- 首次运行需要联网下载 `grpcurl` 和 `jq`（若宿主机已安装则直接建软链）。

### 运行方式

构建镜像后，可以用 `make` 直接运行（默认取 `image.mk` 中的 `${IMG}`）：

```bash
make test-integration-scripts TEST_SUITE=center.base
```

也可以显式指定已存在的镜像：

```bash
KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0 \
  bash scripts/test/run_integration_tests.sh --suite center.base
```

```bash
bash scripts/test/run_integration_tests.sh --list
```

```bash
bash scripts/test/run_integration_tests.sh --help
```

| 选项 / 变量 | 说明 |
| --- | --- |
| `-s, --suite NAME` | 运行指定套件，默认 `$TEST_SUITE` 或 `all` |
| `-i, --image IMAGE` | 被测 Kuscia 镜像，默认 `$KUSCIA_IMAGE` |
| `-l, --list` | 列出当前平台可用套件 |
| `-h, --help` | 显示帮助 |
| `KUSCIA_IMAGE` | 被测 Kuscia 镜像（Make 目标有默认值） |
| `TEST_SUITE` | 默认套件名，默认 `all` |

`integration_test.sh` 是兼容入口，参数与包装器一致：

```bash
KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0 \
  bash scripts/test/integration_test.sh --suite center.base
```

### 可用套件

| 套件 | 场景 |
| --- | --- |
| `center.base` | 中心化组网（Master + Alice/Bob Lite），验证 KusciaJob、DomainData、DomainRoute 与 PSI 任务 |
| `p2p.base` | 点对点组网（Alice/Bob Autonomy），验证跨域任务与授权 |
| `center.example` | 最小示例套件，用于验证测试框架本身 |
| `bfia.base` | BFIA 互联协议任务（arm64 平台不提供） |
| `center.nsjail` | 中心化 + nsjail 运行时隔离 |
| `p2p.nsjail` | 点对点 + nsjail 运行时隔离 |

平台差异：

- `bfia.base` 在 arm64（Linux aarch64 / macOS arm64）上不可用，包装脚本会自动从列表中移除。
- `*.nsjail` 套件在 WSL 环境中会被上游 runner 跳过，套件本身仍会出现在 `--list` 中。
- nsjail 套件需要 `ALLOW_PRIVILEGED=true` 启动容器，实际由套件脚本设置。

### 工作方式

`run_integration_tests.sh` 是 `tests/integration_test.sh` 的包装器：

1. 校验镜像存在、Docker 可用、套件属于当前平台；
2. 把 `tests/integration_test.sh` 复制到 `run/test/integration_test.sh`；
3. 在空的 `run/` 目录中执行。runner 会从 Kuscia 镜像中 `docker cp` 出 `tests/suite`、`tests/vendor`
   和 `kuscia.sh`，因此不要在仓库根目录直接执行，否则会污染工作区；
4. `run/` 已加入 `.gitignore`，测试结束后可直接清理。

`make integration_test` 是等价的传统入口：它会先构建镜像，再用同样方式执行
`tests/integration_test.sh`。需要按套件选择、查看列表或复用已构建镜像时，优先使用本目录的包装器。

### 添加集成套件

套件位于 `tests/suite/<mode>/<suite>.sh`，编写方式参见 [tests/suite/README.md](../../tests/suite/README.md)。
新增套件后需要同步更新：

1. `tests/integration_test.sh` 中的套件变量与 `TEST_SUITES` / 平台过滤逻辑；
2. `scripts/test/run_integration_tests.sh` 中的 `SUITES` 列表；
3. 本 README 的套件表。

## CI

`.github/workflows/checks.yml` 在 `secretflow/kuscia-ci:20250314` 容器中执行 `make test-scripts`，
覆盖语法检查、ShellCheck 与全部离线单元测试。集成测试依赖 Docker 与镜像，不在该 workflow 中运行，
需要时请在具备 Docker 的环境中手动执行。
