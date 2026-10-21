# Kuscia 文档总览

本文档是 `kuscia/docs` 目录的导航与内容索引，说明文档的种类划分、每篇文档的具体内容、推荐阅读路径，以及文档构建与维护方式。

- **文档框架**：Sphinx 8 + MyST-Parser，配置见 [`conf.py`](./conf.py)，依赖见 [`requirements.txt`](./requirements.txt)。
- **源语言**：简体中文（`zh_CN`），英文翻译位于 `locales/en/LC_MESSAGES/`。
- **文档贡献**：构建、预览、翻译流程见 `CONTRIBUTING.md`（位于本目录，因 `conf.py` 的 `exclude_patterns` 配置不进入 Sphinx 输出）。
- **版本日志**：见 [`change_log.rst`](./change_log.rst)，实际内容引入 `../changelog/v1.2.0_CHANGELOG.md`。

---

## 1. 文档体系与种类

按照内容定位，`kuscia/docs` 中的文档可分为以下几类。整体上对应「教程（Tutorial）— 指南（How-to）— 参考（Reference）— 解释（Explanation）」四种文档类型，并额外包含排障、开发与元文档。

| 种类 | 目录 | 定位 | 主要读者 |
| --- | --- | --- | --- |
| 入门教程 | `getting_started/` | 从零部署集群并跑通第一个作业 | 新用户、PoC 验证人员 |
| 参考手册 | `reference/` | 架构、核心概念、Kuscia API、DataMesh API 的权威定义 | 所有读者 |
| 部署与运维 | `deployment/` | Docker/K8s 部署、配置、端口、监控、日志、诊断 | 实施人员、运维人员 |
| 操作指南 | `tutorial/` | 面向具体任务场景的步骤式教程 | 应用开发者、算法工程师 |
| 开发文档 | `development/` | 构建、CLI、CRD 代码生成、Makefile、扩展数据源 | Kuscia 开发者 |
| 故障排查 | `troubleshoot/` | 按概念、部署、网络、任务、参数调优组织的排障手册 | 运维人员、开发者 |
| 深度解析长文 | `docs/` 根目录 | 跨 SecretPad/Kuscia/SecretFlow 的端到端原理与源码分析 | 架构师、高级开发者 |
| 贡献与版本 | 根目录 | 文档构建翻译规范、版本更新日志 | 文档贡献者 |

---

## 2. 推荐阅读路径

### 2.1 第一次接触 Kuscia

1. [概述](./reference/overview.md)：了解 Kuscia 解决什么问题、适合什么场景。
2. [快速开始](./getting_started/quickstart_cn.md)：用 Docker 一键拉起集群并运行 PSI 示例。
3. [了解 KusciaJob](./getting_started/run_secretflow_cn.md)：理解作业、任务与 DAG 的基本用法。
4. [架构](./reference/architecture_cn.md)：建立控制平面、节点、组网模式的整体认知。

### 2.2 准备部署到测试/生产环境

1. [部署要求](./deployment/deploy_check.md)：核对 Docker/K8s/MySQL 版本、OS、内核、资源与网络。
2. [部署指引](./deployment/kuscia_deployment_instructions.md)：选择 Docker 或 K8s、中心化或点对点模式。
3. 按环境阅读：
   - Docker：[多机部署中心化集群](./deployment/Docker_deployment_kuscia/deploy_master_lite_cn.md) / [多机部署点对点集群](./deployment/Docker_deployment_kuscia/deploy_p2p_cn.md)
   - K8s：[部署中心化集群](./deployment/K8s_deployment_kuscia/K8s_master_lite_cn.md) / [部署点对点集群](./deployment/K8s_deployment_kuscia/K8s_p2p_cn.md)
4. [Kuscia 配置文件](./deployment/kuscia_config_cn.md)、[端口介绍](./deployment/kuscia_ports_cn.md)、[网络要求](./deployment/networkrequirements.md)。
5. [运维操作](./deployment/operation_cn.md)、[监控](./deployment/kuscia_monitor.md)、[日志说明](./deployment/logdescription.md)。

### 2.3 集成 Kuscia API

1. [Kuscia API 概览](./reference/apis/summary_cn.md)：请求/响应约定与调用方式。
2. [使用 Kuscia API 运行 SecretFlow 作业](./tutorial/run_sf_job_with_api_cn.md)。
3. [使用 Kuscia API 运行 SecretFlow Serving](./tutorial/run_sf_serving_with_api_cn.md)。
4. [使用 Kuscia API 部署 DataProxy](./tutorial/run_dp_on_kuscia_cn.md)。
5. 按需查阅 [Domain](./reference/apis/domain_cn.md)、[DomainRoute](./reference/apis/domainroute_cn.md)、[DomainData](./reference/apis/domaindata_cn.md)、[KusciaJob](./reference/apis/kusciajob_cn.md)、[Serving](./reference/apis/serving_cn.md) 等接口详情。

### 2.4 开发或二次开发 Kuscia

1. [构建命令](./development/build_kuscia_cn.md)：搭建开发环境并编译。
2. [Kuscia / SecretFlow 源码二次开发镜像打包指南](./development/build_kuscia_secretflow_images_cn.md)：从源码构建、推送和导出 Kuscia 与 SecretFlow 镜像，并注册 AppImage。
3. [Cobra 命令执行逻辑说明](./development/cobra_execution_explanation.md)：理解 CLI 入口。
4. [Kubernetes CRD 代码生成完全指南](./development/code_generation_guide.md)：修改 CRD 后如何生成代码。
5. [Makefile 子模块详解](./development/makefile_submodules.md)：理解构建目标与各 `.mk` 文件。
6. [注册自定义算法镜像](./development/register_custom_image.md)、[扩展其他数据源](./development/add_datamesh_io.md)。
7. [Kuscia 中 Kubernetes 配置与运行模式详解](./development/kuscia_kubernetes_architecture.md)：深入 runc/runk/runp 与嵌入式 K3s。

### 2.5 排查问题

1. [常见问题入口](./troubleshoot/index.rst)：按概念、部署、网络、任务、参数调优分类。
2. [部署失败日志排查](./troubleshoot/deployment/deploy_failed.md) / [授权错误排查](./troubleshoot/network/network_authorization_check.md) / [任务运行网络错误排查](./troubleshoot/network/network_troubleshoot.md) / [作业运行失败](./troubleshoot/runtask/run_job_failed.md)。
3. [内核参数](./troubleshoot/parameter_tuning/kernel_params.md) 与 [Kuscia 诊断工具](./deployment/diagnose_tool.md)。

### 2.6 研究内部原理

1. [Kuscia 架构](./reference/architecture_cn.md) 与 [调度架构与数据流](./reference/kuscia_scheduling_architecture_cn.md)。
2. 根目录深度长文：[设计文档](./设计文档.md)、[任务调度](./任务调度.md)、[组网模式](./组网模式.md)、[数据代理](./数据代理.md)、[通信与数据注册流程](./通信与数据注册流程.md)、[联邦逻辑回归全流程](./联邦逻辑回归全流程.md)。

---

## 3. 入门教程（`getting_started/`）

| 文档 | 具体内容 | 适合读者 |
| --- | --- | --- |
| [快速开始](./getting_started/quickstart_cn.md) | 介绍中心化（center）、点对点（p2p）、中心化互联（cxc）、中心化-P2P 混合（cxp）四种 Docker 组网方式；给出环境要求、一键启动命令、两方 PSI 示例作业和状态查询方法。 | 首次接触 Kuscia 的用户 |
| [了解 KusciaJob](./getting_started/run_secretflow_cn.md) | 在已部署集群上准备数据和 KusciaJob YAML，提交作业、查看运行状态、删除作业；说明 KusciaJob 与 KusciaTask 的关系。 | 已跑通快速开始的用户 |
| [如何本地启动 Kuscia](./getting_started/如何本地启动Kuscia.md) | 面向本地 Linux 的完整启动手册：目录与概述、Docker 和 Go 的前置安装、部署模式简介、Docker 模式快速启动（推荐点对点）。 | 本地开发与测试人员 |

---

## 4. 参考手册（`reference/`）

### 4.1 总览与架构

| 文档 | 具体内容 |
| --- | --- |
| [概述](./reference/overview.md) | 从隐私计算落地问题出发介绍 Kuscia 定位；PoC 验证、生产部署、引擎接入三类使用场景；Kuscia 从手动拉起到 Coordinator 再到 Kuscia 编排的演进；产品层/框架层/引擎层分层；为什么需要 Kuscia。 |
| [架构](./reference/architecture_cn.md) | 控制平面组件（K3s、Kuscia Controllers、Kuscia Storage、Envoy、互联互通适配器）与节点组件（Lite/Autonomy 中的 Agent、NetworkMesh、DataMesh）；中心化与 P2P 组网、部署模式、作业编排与任务调度概览。 |
| [Kuscia 隐私计算调度架构与数据流详解](./reference/kuscia_scheduling_architecture_cn.md) | 从 SecretPad 请求到 KusciaAPI、Job Service、KusciaJob CR、Job Controller、Task Controller、Pod、Ray、SecretFlow 的完整模块架构、数据结构、数据流时序图和关键配置项。 |
| [DomainData 详细说明](./reference/domaindata_specification.md) | DomainData 的数据结构、YAML 示例、存储机制（CRD/etcd、大 value 存储、压缩与备份恢复）、API 接口、DomainDataGrant 授权与跨域同步、典型使用场景。 |
| [scripts/deploy 目录脚本功能详解](./reference/scripts_deploy_guide.md) | `scripts/deploy` 下部署启动类脚本（`kuscia.sh`、`deploy.sh` 等）与资源配置类脚本的功能、参数、模式选择和执行示例。 |
| [设置 K3S API 白名单](./reference/api_whitelist_config_cn.md) | K3s API 白名单的用途，以及两种配置方式与示例。 |

### 4.2 核心概念（`reference/concepts/`）

| 文档 | 具体内容 |
| --- | --- |
| [Domain](./reference/concepts/domain_cn.md) | Domain（节点/域）的用例、创建/更新/清理方式，以及不同角色节点的配置参考。 |
| [DomainRoute](./reference/concepts/domainroute_cn.md) | DomainRoute 路由规则：中心化集群中 Lite 访问 Master、Lite 节点之间互访、P2P 集群 Autonomy 节点之间互访，以及协议、路径重写等进阶配置。 |
| [DomainData](./reference/concepts/domaindata_cn.md) | DomainData 的用例、创建/更新/清理、在 Domain 侧管理数据的方法与字段参考。 |
| [DomainDataGrant](./reference/concepts/domaindatagrant_cn.md) | 数据授权的创建、更新、清理，以及在 Domain 侧管理授权记录。 |
| [DomainDataSource](./reference/concepts/domaindatasource_cn.md) | DomainDataSource 的用例、创建/更新/清理，用于描述 localfs、OSS、MySQL 等实际数据源。 |
| [KusciaJob](./reference/concepts/kusciajob_cn.md) | KusciaJob 的状态说明、创建、DAG 调度机制、停止与清理。 |
| [KusciaTask](./reference/concepts/kusciatask_cn.md) | KusciaTask 的状态说明、创建方式与字段参考；KusciaTask 是作业 DAG 中的任务单元。 |
| [KusciaDeployment](./reference/concepts/kusciadeployment_cn.md) | KusciaDeployment 的状态、创建、查看与清理，用于 Serving 等常驻任务流。 |
| [InteropConfig](./reference/concepts/interopconfig_cn.md) | 互联互通协议配置的创建、更新与清理。 |
| [AppImage](./reference/concepts/appimage_cn.md) | AppImage（算法镜像）的用例、创建、更新、清理与字段参考。 |

### 4.3 Kuscia API（`reference/apis/`）

| 文档 | 具体内容 |
| --- | --- |
| [概览](./reference/apis/summary_cn.md) | K8s API 与 Kuscia API 的区别；HTTP/gRPC 请求与响应约定、认证方式、错误处理；Python 调用示例。 |
| [Domain](./reference/apis/domain_cn.md) | Domain 相关接口：创建、更新、删除、查询等请求与响应示例。 |
| [DomainRoute](./reference/apis/domainroute_cn.md) | DomainRoute 相关接口的请求参数、响应结构与示例。 |
| [DomainData](./reference/apis/domaindata_cn.md) | DomainData 的创建、更新、删除、查询、批量查询等接口。 |
| [DomainDataGrant](./reference/apis/domaindatagrant_cn.md) | DomainDataGrant 的创建、更新、删除、查询等接口。 |
| [DomainDataSource](./reference/apis/domaindatasource_cn.md) | DomainDataSource 的增删改查接口与各数据源参数。 |
| [KusciaJob](./reference/apis/kusciajob_cn.md) | KusciaJob 的创建、查询、停止、删除等接口，包含 `taskInputConfig` 说明。 |
| [Serving](./reference/apis/serving_cn.md) | Serving 的创建、更新、删除、查询接口，用于在线预测服务。 |
| [AppImage](./reference/apis/appimage_cn.md) | AppImage 的注册、查询、更新、删除接口。 |
| [Config](./reference/apis/config_cn.md) | 节点配置相关接口，如证书、Token 等信息的读取与配置。 |
| [Log](./reference/apis/log_cn.md) | 获取任务/容器日志的接口。 |
| [Health](./reference/apis/health_cn.md) | 健康检查接口。 |
| [Error Code](./reference/apis/error_code_cn.md) | Kuscia API 错误码定义与说明。 |

### 4.4 DataMesh API（`reference/apis/datamesh/`）

| 文档 | 具体内容 |
| --- | --- |
| [概览](./reference/apis/datamesh/summary_cn.md) | DataMesh API 的使用场景、接口约定和访问方式。 |
| [DomainData](./reference/apis/datamesh/domaindata_cn.md) | DataMesh 侧 DomainData 的查询等接口。 |
| [DomainDataSource](./reference/apis/datamesh/domaindatasource_cn.md) | DataMesh 侧 DomainDataSource 的接口。 |
| [DataMesh 数据读写](./reference/apis/datamesh/datacrud_cn.md) | 通过 Arrow Flight 访问 DataMesh 的方法、支持的数据服务与注意事项。 |

> 说明：DataMesh 文档通过 [`reference/index.rst`](./reference/index.rst) 的 toctree 挂载；[`reference/apis/index.rst`](./reference/apis/index.rst) 中的 DataMesh 条目目前被注释掉，以避免重复导航。

---

## 5. 部署与运维（`deployment/`）

### 5.1 部署总览、要求与运行模式

| 文档 | 具体内容 |
| --- | --- |
| [Kuscia 部署指引](./deployment/kuscia_deployment_instructions.md) | Docker 与 K8s 两种部署方式的选择入口、前置准备、部署验证方法和排障链接。 |
| [部署要求](./deployment/deploy_check.md) | Docker、K8s、MySQL 版本要求，处理器架构、操作系统、内核版本、资源与网络要求。 |
| [使用 RunP 模式部署节点](./deployment/deploy_with_runp_cn.md) | 生成配置文件并指定 `runtime=runp`，以及对应的部署流程。 |

### 5.2 Docker 多机部署（`deployment/Docker_deployment_kuscia/`）

| 文档 | 具体内容 |
| --- | --- |
| [多机部署点对点集群](./deployment/Docker_deployment_kuscia/deploy_p2p_cn.md) | 基于 TOKEN 认证部署 Autonomy 节点；交换 `domain.crt` 证书、建立双向路由、验证连通性。 |
| [多机部署中心化集群](./deployment/Docker_deployment_kuscia/deploy_master_lite_cn.md) | 部署 Master 与 Lite 节点，获取 Lite 部署 Token、建立节点授权并验证。 |
| [升级 Kuscia 版本](./deployment/Docker_deployment_kuscia/deploy_upgrade_kuscia_cn.md) | 升级 Kuscia 的步骤与注意事项。 |
| [Docker 集群部署-组网模式](./deployment/Docker_deployment_kuscia/deploy_clustered_cn.md) | 使用 Docker overlay 网络跨主机组网，配合 Nginx 代理的集群部署方式与参数示例。 |
| [非 root 用户部署节点](./deployment/Docker_deployment_kuscia/docker_deploy_kuscia_with_rootless.md) | 以非 root 用户运行 Docker 节点的部署方法。 |
| [DataProxy 部署](./deployment/Docker_deployment_kuscia/deploy_dataproxy_cn.md) | DataProxy 的部署步骤与验证方法。 |

### 5.3 K8s 集群部署（`deployment/K8s_deployment_kuscia/`）

| 文档 | 具体内容 |
| --- | --- |
| [部署中心化集群](./deployment/K8s_deployment_kuscia/K8s_master_lite_cn.md) | 在 K8s 上部署 Master 与 Lite、获取 Token、建立双向授权、验证部署并运行任务。 |
| [部署点对点集群](./deployment/K8s_deployment_kuscia/K8s_p2p_cn.md) | 在 K8s 上部署 Autonomy 节点、交换证书、建立路由、验证并运行任务。 |
| [非 root 用户部署节点](./deployment/K8s_deployment_kuscia/k8s_deploy_kuscia_with_rootless.md) | K8s 环境下非 root 部署方式及所需变更。 |

### 5.4 本地联合部署

| 文档 | 具体内容 |
| --- | --- |
| [Kuscia 与 DataMesh 本地部署指南](./deployment/local_deploy_kuscia_datamesh.md) | 单机 Linux/WSL2 上的部署指南：环境要求、一键体验集群、DataMesh 说明、自定义多节点（暴露端口）、源码/二进制启动、验证与停止。 |
| [SecretPad + Kuscia + DataMesh 本地联合部署指南](./deployment/local_deploy_secretpad_kuscia_datamesh.md) | 三组件本地联调：架构与通信关系、All-in-one 容器化部署、Kuscia 容器 + SecretPad 源码联调、DataMesh 交互验证。 |

### 5.5 运维、监控与诊断

| 文档 | 具体内容 |
| --- | --- |
| [运维操作](./deployment/operation_cn.md) | 宿主机、Master 容器、节点容器上的常用运维命令，以及安全建议。 |
| [Kuscia 诊断工具](./deployment/diagnose_tool.md) | `kuscia diagnose network` 与 `kuscia diagnose log` 的使用方法。 |
| [Kuscia 日志说明](./deployment/logdescription.md) | Kuscia 目录结构、日志文件分类与示例。 |
| [Kuscia 监控](./deployment/kuscia_monitor.md) | 监控能力、监控配置方法与 Kuscia 监控指标项。 |
| [Kuscia 引擎指标监控](./deployment/kuscia_engine_monitor.md) | 引擎侧指标配置与集群侧配置。 |
| [Kuscia 网络要求](./deployment/networkrequirements.md) | 参数要求、网络联通说明与 Nginx 代理参数配置示例。 |
| [Kuscia 配置文件](./deployment/kuscia_config_cn.md) | `kuscia.yaml` 字段说明：部署模式、DomainID、私钥、通信协议、日志、Token、Master 地址、runtime、资源容量、Agent 镜像等。 |
| [Kuscia 端口介绍](./deployment/kuscia_ports_cn.md) | 1080、80、8082、8083、9091 等端口的用途、是否需要暴露给合作方，以及对应的部署脚本参数。 |

---

## 6. 操作指南（`tutorial/`）

| 文档 | 具体内容 |
| --- | --- |
| [使用 Kuscia API 运行 SecretFlow 作业](./tutorial/run_sf_job_with_api_cn.md) | 准备节点、证书与 Token、数据；配置并提交 KusciaJob；查询状态、删除作业；算子参数说明与 HTTP 错误处理。 |
| [使用 Kuscia API 运行 SecretFlow Serving](./tutorial/run_sf_serving_with_api_cn.md) | 准备 Serving 应用镜像模板，提交、查询、预测、更新和删除 SecretFlow Serving。 |
| [使用 Kuscia API 部署 DataProxy](./tutorial/run_dp_on_kuscia_cn.md) | 获取证书和 Token，在 K8s 模式与 Docker 模式下部署 DataProxy。 |
| [在 Kuscia 上运行 SCQL 联合分析任务](./tutorial/run_scql_on_kuscia_cn.md) | 准备节点、证书 Token 和数据，部署 SCQL，并使用 SCQL 发起联合分析查询。 |
| [运行互联互通银联 BFIA 协议作业](./tutorial/run_bfia_job_cn.md) | 准备环境、提交 BFIA 协议作业、查看 KusciaJob 状态与 ECDH PSI 算子结果。 |
| [运行 FATE 作业](./tutorial/run_fate_cn.md) | 准备节点与 FATE 集群，构建 FATE 部署镜像和 AppImage，验证 FATE 服务并查看部署日志。 |
| [升级引擎镜像](./tutorial/upgrade_engine.md) | 在 Kuscia 中导入并升级引擎镜像。 |
| [使用自定义镜像仓库](./tutorial/custom_registry.md) | 配置自定义镜像仓库，镜像仓库与 AppImage 的搭配使用，以及镜像拉取失败排查。 |
| [给自定义应用渲染配置文件](./tutorial/config_render.md) | 自定义应用的配置示例与配置文件渲染机制。 |
| [配置 Path Rewrite](./tutorial/kuscia_gateway_with_path.md) | 多机部署下配置 Path Rewrite，以及通过 KusciaAPI 配置的方法。 |
| [自定义 Service 路由](./tutorial/user_defined_service_route.md) | 为 Kuscia 中的 Service 自定义路由的示例与访问方法。 |
| [安全加固方案](./tutorial/security_plan_cn.md) | 使用 NsJail 进程安全沙箱与 Kata 安全容器运行 SecretFlow 任务，降低未知代码执行风险。 |

---

## 7. 开发文档（`development/`）

| 文档 | 具体内容 |
| --- | --- |
| [构建命令](./development/build_kuscia_cn.md) | 开发环境搭建与 Kuscia 构建命令。 |
| [Kuscia / SecretFlow 源码二次开发镜像打包指南](./development/build_kuscia_secretflow_images_cn.md) | 一键打包脚本与 `make image` 的差异、Kuscia/SecretFlow 镜像构建、AppImage 注册、端口映射，以及 AppImage → KusciaJob 的调度原理。 |
| [Cobra 命令执行逻辑说明](./development/cobra_execution_explanation.md) | `Execute()`、`ExecuteC()`、`execute()` 的调用链，Run 钩子执行顺序，以及对阅读 Kuscia CLI 代码的建议。 |
| [注册自定义算法镜像](./development/register_custom_image.md) | 准备工具脚本、准备 AppImage、加载镜像到节点、注册镜像并使用自定义镜像运行作业。 |
| [Kubernetes CRD 代码生成完全指南](./development/code_generation_guide.md) | Kubernetes 与 CRD 基础、CRD YAML 与客户端代码生成、应用 CRD、验证注册与资源操作。 |
| [Kuscia 中 Kubernetes 配置与运行模式详解](./development/kuscia_kubernetes_architecture.md) | K8s 与 K3s 的区别、runc/runk/runp 三种运行模式、嵌入式 K3s 架构、K8s 配置适用性、命名空间隔离、网络与服务发现、容器运行时、场景对比、镜像体系与 FAQ。 |
| [如何扩展其他数据源](./development/add_datamesh_io.md) | DataMesh IO 扩展流程、业务要求、添加步骤与注意事项。 |
| [golangci-lint 修复汇总](./development/lint-fixes-summary.md) | `govet inline`、`goconst` 等 lint 问题的修复方式、验证结果与注意事项。 |
| [Kuscia Makefile 子模块详解](./development/makefile_submodules.md) | 顶层 Makefile 的转发机制，`common.mk`、`docs.mk`、`image.mk`、`golang.mk`、`lint.mk`、`fate.mk` 的职责，变量覆盖规则与常用命令。 |

---

## 8. 故障排查（`troubleshoot/`）

### 8.1 概念答疑（`troubleshoot/concept/`）

| 文档 | 具体内容 |
| --- | --- |
| [Kuscia 与 Ray 的区别](./troubleshoot/concept/kuscia_vs_ray.md) | Kuscia 与 Ray 在定位、资源管理、跨域通信等方面的差异，以及 Kuscia 额外解决的问题。 |
| [Protocol 通信协议](./troubleshoot/concept/protocol_describe.md) | NOTLS/TLS/MTLS 协议对应的 KusciaAPI 请求差异、节点授权差异，以及修改协议的影响和解决方法。 |

### 8.2 部署失败（`troubleshoot/deployment/`）

| 文档 | 具体内容 |
| --- | --- |
| [部署失败日志排查](./troubleshoot/deployment/deploy_failed.md) | 分别登录 Master、Alice、Bob 容器查看 Kuscia 与 K3s 错误日志的排查步骤。 |
| [Lite 节点证书丢失后重新部署](./troubleshoot/deployment/private_key_loss.md) | 节点私钥/证书丢失后的重新部署流程。 |
| [Kine 表问题导致启动失败](./troubleshoot/deployment/kuscia_mysql_kine.md) | Kine 表不存在等数据库问题导致 Kuscia 启动失败的排查。 |
| [Create fsnotify failed](./troubleshoot/deployment/create_fsnotify_failed.md) | 启动时报 `Create fsnotify failed` 的问题描述与解决方案。 |
| [WSL 注意事项](./troubleshoot/deployment/wsl_start_docker.md) | 在 WSL 中启动 Docker 的注意事项。 |
| [FATE 部署失败](./troubleshoot/deployment/FATE_deploy_failed.md) | FATE 部署时 Pod 镜像拉取失败的排查。 |

### 8.3 网络连接（`troubleshoot/network/`）

| 文档 | 具体内容 |
| --- | --- |
| [授权错误排查](./troubleshoot/network/network_authorization_check.md) | 节点授权失败的排查步骤。 |
| [任务运行网络错误排查](./troubleshoot/network/network_troubleshoot.md) | 通过 traceid 定位问题请求、理解 Gateway 日志格式，并对照正常请求日志排查网络错误。 |

### 8.4 任务运行（`troubleshoot/runtask/`）

| 文档 | 具体内容 |
| --- | --- |
| [作业运行失败](./troubleshoot/runtask/run_job_failed.md) | 作业运行失败的通用排查步骤。 |
| [FATE 作业运行失败](./troubleshoot/runtask/FATE_run_job_failed.md) | FATE 作业运行失败的日志排查方法。 |

### 8.5 参数调优（`troubleshoot/parameter_tuning/`）

| 文档 | 具体内容 |
| --- | --- |
| [内核参数](./troubleshoot/parameter_tuning/kernel_params.md) | 通过 Node 的 `Kernel-Params` Condition 判断内核参数是否满足高并发要求，并给出调整方法。 |
| [Docker 24.0 C++17 文件复制权限问题](./troubleshoot/parameter_tuning/docker_cpp_copy.md) | 问题描述、文件权限说明与推荐解决方案。 |
| [数据库连接数过多](./troubleshoot/parameter_tuning/mysql_too_many_connections.md) | MySQL 连接数过多的原因与解决方法。 |
| [MySQL optimizer_switch 参数优化](./troubleshoot/parameter_tuning/mysql_derived_merge.md) | 派生表合并相关参数的问题、解决方案、原因分析与参数介绍。 |
| [K8s 部署模式下线程限制](./troubleshoot/parameter_tuning/k8s_ulimit_check.md) | SecretFlow 应用线程受限的原因分析与解决方案。 |
| [Docker 节点 Memory 扩容](./troubleshoot/parameter_tuning/docker_memory_limit.md) | 对已部署节点进行内存扩容的步骤。 |
| [MTU 配置导致的网络异常](./troubleshoot/parameter_tuning/change_pod_mtu.md) | MTU 简介、问题现象与调整方法。 |
| [OSS Endpoint 配置问题](./troubleshoot/parameter_tuning/kusciaapi_store_endpoint.md) | 对象存储 Endpoint 配置错误的现象、解决方案与原因分析。 |

---

## 9. 深度解析长文（`docs/` 根目录）

这 6 篇文档篇幅较长，聚焦源码级流程与跨组件协作，适合在熟悉基础概念后深入阅读。它们通过 [`deep_dive/index.rst`](./deep_dive/index.rst) 挂载，并在 [`index.md`](./index.md) 的「深入解析」章节中提供入口。

| 文档 | 篇幅与结构 | 具体内容 |
| --- | --- | --- |
| [设计文档](./设计文档.md) | 约 360 KB，14 章 | Kuscia 架构及各模块设计全景：总体分层、部署模式与组网、核心模块、运行时与资源隔离、任务调度与数据流、网络与安全、DataMesh、API 层、运维监控诊断、嵌入式 K3s、CRD 代码生成、镜像体系与附录。 |
| [任务调度](./任务调度.md) | 约 126 KB，6 章 | 典型任务调度全流程；后端提交的 `CreateJob` 与 `taskInputConfig` 格式；状态机；P2P 资源同步；按 DAG 创建 KusciaTask（DAG 校验、TaskID、就绪任务、并发度、Job 状态）；TaskResourceGroup/Pod/Service/ConfigMap 创建；资源预留；Pod 绑定与 All-or-Nothing 调度；完整配置传递链路、排障与最佳实践。 |
| [组网模式](./组网模式.md) | 约 43 KB，5 章 | 节点角色与部署方式；模块化架构、模块启动矩阵、控制面/数据面职责、runc/runk/runp 差异、配置结构；Master-Lite、P2P、混合模式的认证与证书流程；密钥证书生命周期与跨域通信安全；中心化与 P2P 组网流程及路由管理。 |
| [数据代理](./数据代理.md) | 约 17 KB，10 章 | DataProxy 的定位与使用场景、架构与工作流程、支持的数据源、`dataMesh` 配置、自动/手动部署方式、DomainDataSource 关联、自定义插件开发、常见问题与最佳实践。 |
| [通信与数据注册流程](./通信与数据注册流程.md) | 约 77 KB，11 章 | SecretPad→Kuscia 的数据注册全链路：DomainDataSource 与 DomainData 两种形态；LOCAL CSV 上传及 OSS/MySQL/ODPS/HTTP 注册；Kuscia gRPC Handler 与 CR 创建；DataMesh 元数据、路径解析、CSV↔Arrow Flight 转换；接口汇总、存储方式、完整时序图、源码索引与附录。 |
| [联邦逻辑回归全流程](./联邦逻辑回归全流程.md) | 约 57 KB，11 章 | 从 SecretPad 画布到 SecretFlow 执行的联邦逻辑回归端到端流程：数据准备、前端 DAG 编排、后端 JobChain/JobRender/KusciaJobConverter/TaskInputConfig/ProjectJob、Kuscia 调度、容器内预处理与 `ss_sgd_train` MPC 训练、状态同步与结果展示、完整时序图、源码索引与默认参数。 |

---

## 10. 文档贡献与版本记录

| 文档 | 具体内容 |
| --- | --- |
| `CONTRIBUTING.md` | 文档贡献指南（中英文）：Python 虚拟环境准备、`secretflow-doctools` 构建与本地预览、`update-translations` 翻译同步、文件清理与问题反馈。该文件通过 `conf.py` 的 `exclude_patterns` 排除，不进入 Sphinx 输出。 |
| [change_log.rst](./change_log.rst) | 版本更新日志入口，引入 `../changelog/v1.2.0_CHANGELOG.md`。 |
| [update_po.sh](./update_po.sh) | 使用 `sphinx-build -b gettext` 与 `sphinx-intl update` 重新生成 `locales/` 下英文 `.po` 文件的辅助脚本。 |

---

## 11. Sphinx 导航现状

### 11.1 导航结构

文档站点以 [`index.md`](./index.md) 为根，通过隐藏 toctree 挂载各分类索引；分类内部再由各自的 `index.rst` 组织：

```text
index.md（首页）
├── README.md                  # 本文件：文档总览
├── getting_started/index      # 入门教程
├── reference/index            # 参考手册（concepts / apis / datamesh）
├── deployment/index           # 部署与运维
├── tutorial/index             # 操作指南
├── development/index          # 开发文档
├── troubleshoot/index         # 故障排查
├── deep_dive/index            # 深度解析长文
└── change_log                 # 版本更新日志
```

### 11.2 已纳入 toctree 的文档

- 入门：`quickstart_cn`、`run_secretflow_cn`、`如何本地启动Kuscia`。
- 参考：`overview`、`architecture_cn`、`kuscia_scheduling_architecture_cn`、`domaindata_specification`、`api_whitelist_config_cn`、`scripts_deploy_guide`、全部 `concepts/` 文档、全部 `apis/` 文档、`apis/datamesh/` 文档。
- 部署：`kuscia_deployment_instructions`、`deploy_check`、Docker 6 篇、K8s 3 篇、`deploy_with_runp_cn`、`local_deploy_kuscia_datamesh`、`local_deploy_secretpad_kuscia_datamesh`、`operation_cn`、`networkrequirements`、`diagnose_tool`、`logdescription`、`kuscia_monitor`、`kuscia_engine_monitor`、`kuscia_config_cn`、`kuscia_ports_cn`。
- 指南：`run_sf_job_with_api_cn`、`run_sf_serving_with_api_cn`、`run_dp_on_kuscia_cn`、`run_scql_on_kuscia_cn`、`run_bfia_job_cn`、`run_fate_cn`、`upgrade_engine`、`custom_registry`、`config_render`、`kuscia_gateway_with_path`、`user_defined_service_route`、`security_plan_cn`。
- 开发：`build_kuscia_cn`、`build_kuscia_secretflow_images_cn`、`makefile_submodules`、`cobra_execution_explanation`、`register_custom_image`、`add_datamesh_io`、`code_generation_guide`、`kuscia_kubernetes_architecture`、`lint-fixes-summary`。
- 排障：`concept/` 2 篇、`deployment/` 6 篇、`network/` 2 篇、`runtask/` 2 篇、`parameter_tuning/` 8 篇。
- 深度解析：`设计文档`、`任务调度`、`组网模式`、`数据代理`、`通信与数据注册流程`、`联邦逻辑回归全流程`，统一由 [`deep_dive/index.rst`](./deep_dive/index.rst) 挂载。

### 11.3 文档编写约定与注意事项

1. **锚点写法**：MyST 中目标锚点使用 `(label)=`（如 `(configtemplate)=`），不要写成 `{#label}` 或 `#(label)=`；后者不会生成有效锚点。
2. **跨文档链接**：Sphinx 的 MyST 交叉引用仅对**标题 slug** 做跨文档解析，链接其他文档的章节时请使用归一化后的标题 slug（如 `other.md#配置项详解`），不要依赖显式 label。
3. **相对路径**：按当前文件所在目录计算层级；`troubleshoot/` 与 `tutorial/` 同为一级目录，互相引用使用 `../` 而非 `../../`。
4. **文件命名**：新增文档使用小写 ASCII 加下划线（如 `build_kuscia_cn.md`），避免空格和中文文件名，便于链接、检索和翻译。
5. **构建校验**：提交前执行 `secretflow-doctools build --lang zh_CN --lang en` 或下方的 `sphinx-build -b dummy`，目标为 **0 warning**。

---

## 12. 文档构建、翻译与维护

### 12.1 本地构建与预览

```sh
cd kuscia/docs

python -m venv .venv
source .venv/bin/activate
python -m pip install -r requirements.txt

# 构建中文和英文文档
secretflow-doctools build --lang zh_CN --lang en

# 本地预览
secretflow-doctools preview
```

### 12.2 更新翻译

```sh
secretflow-doctools update-translations --lang en
# 或使用仓库脚本
bash update_po.sh
```

修改现有正文后必须同步更新 `locales/` 中对应的 `.po` 文件，否则发布后可能出现中英文混排。

### 12.3 新增文档的约定

1. 将文档放到对应分类目录；跨组件、源码级的专题长文可放在 `docs/` 根目录。
2. 在分类的 `index.rst` 中把文档加入 `toctree`，否则不会出现在文档站点导航中。
3. 图片放在 `imgs/` 下，使用相对路径引用。
4. 使用 MyST Markdown 语法；标题层级保持 `#` → `##` → `###`。
5. 新增或修改正文后，同步更新英文翻译。
6. 提交前运行一次 `secretflow-doctools build --lang zh_CN --lang en`，确认没有断链或 toctree 警告。

### 12.4 文档维护建议

- 修改 API proto 或 CRD 字段时，同步更新 `reference/apis/` 与 `reference/concepts/` 中对应文档。
- 部署脚本参数变化时，同步更新 `deployment/kuscia_ports_cn.md`、`deployment/kuscia_config_cn.md` 与 `reference/scripts_deploy_guide.md`。
- 新增排障案例时，按「概念 / 部署 / 网络 / 任务 / 参数调优」归类，并在对应 `index.rst` 中登记。
- 新增长文时，放入 `docs/` 根目录并登记到 `deep_dive/index.rst`，同时在 `index.md` 的「深入解析」章节添加入口。
- 调整文档目录结构后，同步更新本文件的分类表、第 11 节导航结构与相关相对链接。
- 每次合并前运行一次 `sphinx-build -b dummy -E -a`，确认无 `myst.xref_missing`、重复标签或 toctree 告警。
