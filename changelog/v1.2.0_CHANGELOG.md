## 更新日志

本项目的所有显著变更都将记录在此文档中。

变更记录的格式遵循 [保持变更日志](https://keepachangelog.com/zh-CN/1.0.0/) 约定，
同时本项目遵守 [语义化版本控制](https://semver.org/lang/zh-CN/spec/v2.0.0.html) 规范。

### 变更类型

- `新增`：引入新功能。
- `修改`：对现有功能的改进或调整。
- `废弃`：计划在未来移除的功能。
- `移除`：已从项目中移除的功能。
- `修复`：错误或漏洞的修复。
- `安全`：涉及安全漏洞的修复或更新。
- `重大变更`：引入了不兼容的更改，可能需要用户做出相应调整。

### [v1.2.0.dev260313] - 2026-03-13

#### 新增

- **[功能]** 银联 BFIA 协议支持
- **[功能]** Kingbase 数据源支持

#### 修改

- **[无]**

#### 不兼容变更

- **[无]**

#### 修复

- **[问题修复]** 修复 Appimage 中未配置副本数时，运行任务报错的问题

#### 安全

- **[漏洞]** 相关目录访问权限，文件执行权限漏洞修复

---

## Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

### Types of changes

- `Added` for new features.
- `Changed` for changes in existing functionality.
- `Deprecated` for soon-to-be removed features.
- `Removed` for now removed features.
- `Fixed` for any bug fixes.
- `Security` in case of vulnerabilities.
- `Breaking Changed` Breaking for backward-incompatible changes that require user intervention.

### [v1.2.0.dev260313] - 2026-03-13

#### Added

- **[Feature]** Support for China UnionPay BFIA protocol.
- **[Feature]** Kingbase datasource support.

#### Changed

- **[NA]**

#### Breaking Changed

- **[NA]**

#### Fixes

- **[Bugfix]** Fixed an issue where tasks would report errors when replica count was not configured in Appimage.

#### Security

- **[Vulnerability]** Fixed vulnerabilities related to directory access permissions and file execution permissions.
