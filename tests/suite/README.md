# Kuscia 集成测试套件

套件基于 shunit2 编写，由 `tests/integration_test.sh` 在 Docker 中拉起真实 Kuscia 集群执行。
运行方式与环境要求见 [scripts/test/README.md](../../scripts/test/README.md)。

> 下方“运行套件”中的命令可直接点击运行；编写新套件时的代码片段是套件内片段，不能从仓库根目录
> 独立执行。

运行套件：

```bash
make test-integration-scripts TEST_SUITE=center.base
```

公开全部套件列表：

```bash
bash scripts/test/run_integration_tests.sh --list
```

## 目录结构

```text
tests/suite/
├── core/functions.sh     # 共享函数：启动/停止集群、等待任务、KusciaAPI/DataMesh 调用
├── center/               # 中心化组网（Master + Lite）
│   ├── base.sh           # 核心任务链路
│   ├── example.sh        # 最小示例
│   ├── nsjail.sh         # nsjail 运行时隔离
│   └── kuscia_images.sh
├── p2p/                  # 点对点组网（Autonomy）
│   ├── base.sh
│   ├── nsjail.sh
│   └── kuscia_images.sh
└── bfia/                 # BFIA 互联协议
    └── base.sh
```

## 套件列表

| 套件 | 入口 | 说明 |
| --- | --- | --- |
| `center.base` | `center/base.sh` | 中心化模式：创建 DomainData、授权、提交 PSI 示例任务并校验输出 |
| `p2p.base` | `p2p/base.sh` | 点对点模式：两个 Autonomy 域之间的任务调度与数据交换 |
| `center.example` | `center/example.sh` | 框架自检示例，仅包含一个恒真断言 |
| `bfia.base` | `bfia/base.sh` | 通过 BFIA 接口提交作业（arm64 平台不提供） |
| `center.nsjail` | `center/nsjail.sh` | 以 `ALLOW_PRIVILEGED=true` 启动，验证 nsjail 隔离运行时 |
| `p2p.nsjail` | `p2p/nsjail.sh` | 点对点 + nsjail |

## 编写新套件

1. 在对应模式目录下新建 `<suite>.sh`，以 `set -e` 开头。
2. 加载共享函数（路径相对于执行目录 `run/`）：

   ```bash
   . ./test/suite/core/functions.sh
   ```

3. 使用 `oneTimeSetUp` 启动集群、`oneTimeTearDown` 停止集群；测试函数以 `test_` 开头。
4. 通过 `TEST_SUITE_RUN_ROOT_DIR` 存放临时文件，通过 `TEST_BIN_DIR` 使用下载的 `grpcurl`/`jq`。
5. 在文件末尾加载 shunit2：

   ```bash
   . ./test/vendor/shunit2
   ```

6. 参考 [scripts/test/README.md](../../scripts/test/README.md) 注册新套件（入口脚本、包装脚本与文档三处）。
