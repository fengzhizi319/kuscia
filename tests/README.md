# Kuscia 测试资产

本目录存放 Kuscia 的集成测试资产与 Go 测试工具。脚本本身的静态检查与离线单元测试位于
`scripts/test/`，请先阅读 [scripts/test/README.md](../scripts/test/README.md)。

> 本文档中的 `bash` 代码块可直接点击运行，命令默认在 **Kuscia 仓库根目录** 执行。

| 路径 | 用途 |
| --- | --- |
| `integration_test.sh` | 集成测试入口，需从空目录运行；通常由 `scripts/test/run_integration_tests.sh` 调用 |
| `suite/` | shunit2 集成套件（center / p2p / bfia / core），说明见 [suite/README.md](suite/README.md) |
| `data/` | 示例数据（Alice/Bob、隐私组件、SCQL、逻辑回归输入） |
| `util/` | Go 测试辅助代码（Kubernetes / 通用工具） |
| `vendor/shunit2` | 仓库内置的 shunit2 框架，离线单元测试与集成套件共用 |

## 运行集成测试

推荐使用包装脚本，它会处理平台差异、镜像校验和临时工作目录：

```bash
make test-integration-scripts TEST_SUITE=center.base
```

也可以显式指定镜像：

```bash
KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0 \
  bash scripts/test/run_integration_tests.sh --suite center.base
```

直接执行 `tests/integration_test.sh` 时，请先进入一个空目录（例如 `run/`），因为入口脚本会从
Kuscia 镜像中 `docker cp` 出套件并下载 `grpcurl`、`jq`：

```bash
mkdir -p run && cd run
KUSCIA_IMAGE=secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:1.2.0b0 \
  bash ../tests/integration_test.sh all
```
