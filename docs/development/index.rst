开发 Kuscia
================

本目录面向参与 Kuscia 开发或二次开发的读者，覆盖本地构建、镜像打包、CLI 与代码生成、运行时架构和扩展开发。阅读前请先按 :doc:`/getting_started/quickstart_cn` 跑通一次部署，并准备好 Go、Docker、Protoc 等工具链。

推荐阅读路径
----------------

1. **首次构建**：:doc:`build_kuscia_cn` → :doc:`makefile_submodules`，先编译出二进制，再理解 ``make`` 目标与变量覆盖。
2. **制作交付镜像**：:doc:`build_kuscia_secretflow_images_cn` → :doc:`register_custom_image`，从源码构建 Kuscia / SecretFlow 镜像并注册为 AppImage。
3. **阅读源码**：:doc:`cobra_execution_explanation` → :doc:`code_generation_guide` → :doc:`kuscia_kubernetes_architecture`，依次理解命令入口、CRD 代码生成和运行时模式。
4. **扩展能力**：:doc:`add_datamesh_io`；历史 lint 修复记录见 :doc:`lint-fixes-summary`。

文档分类
----------------

**构建与打包**

- :doc:`build_kuscia_cn`：开发环境依赖、``make build`` / ``make image``、文档构建与集成测试。
- :doc:`makefile_submodules`：顶层 Makefile 的转发机制，``common.mk``、``image.mk``、``golang.mk``、``lint.mk`` 等子模块的职责与常用命令。
- :doc:`build_kuscia_secretflow_images_cn`：基于本地源码构建 Kuscia 与 SecretFlow 镜像、推送/导出镜像、接入 Kuscia 并注册 AppImage 的完整流程。

**CLI 与代码生成**

- :doc:`cobra_execution_explanation`：Cobra ``Execute()`` / ``execute()`` 调用链与 Run 钩子执行顺序。
- :doc:`code_generation_guide`：Kubernetes CRD 基础、CRD YAML 与客户端代码生成、应用与验证。

**运行时架构**

- :doc:`kuscia_kubernetes_architecture`：K8s 与 K3s 的区别、runc/runk/runp 三种运行模式、嵌入式 K3s、网络与存储配置。

**扩展开发**

- :doc:`register_custom_image`：准备 AppImage 模板、加载自定义算法镜像到节点并提交作业。
- :doc:`add_datamesh_io`：为 DataMesh 扩展新的数据源读写实现。

**历史记录**

- :doc:`lint-fixes-summary`：``govet inline``、``goconst`` 等 golangci-lint 问题的修复方式与验证结果。

.. toctree::
    :maxdepth: 2

    build_kuscia_cn
    makefile_submodules
    build_kuscia_secretflow_images_cn
    cobra_execution_explanation
    code_generation_guide
    kuscia_kubernetes_architecture
    register_custom_image
    add_datamesh_io
    lint-fixes-summary
