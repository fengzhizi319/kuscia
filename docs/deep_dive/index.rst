深入解析
==========

本目录收录跨 SecretPad、Kuscia 与 SecretFlow 的中文长文。内容以源码路径、CRD 字段和端到端时序为主，适合在熟悉 :doc:`../getting_started/quickstart_cn` 与 :doc:`../reference/architecture_cn` 后阅读。

阅读建议
--------

1. 先读 :doc:`../reference/architecture_cn`，建立控制面、节点、组网模式和核心对象的整体认知。
2. 再按问题域选择长文：调度看「任务调度」，网络看「组网模式」，数据看「数据代理」与「通信与数据注册流程」。
3. 需要跟代码时，先定位长文中的源码索引，再结合 :doc:`../development/build_kuscia_cn` 编译与调试。

.. toctree::
    :maxdepth: 1

    ../设计文档
    ../任务调度
    ../组网模式
    ../数据代理
    ../通信与数据注册流程
    ../联邦逻辑回归全流程
