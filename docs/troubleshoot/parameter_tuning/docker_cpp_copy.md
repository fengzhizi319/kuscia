# Docker 24.0 环境中 C++17 文件复制权限问题

## 问题描述

在部署了 Docker 24.0 的环境中使用 C++17 标准的 `std::filesystem::copy` 系统调用时，会遇到 `permission denied` 的权限问题。该问题通常发生在容器内对 **hostPath 挂载目录**中的文件执行复制时。

典型报错：

```text
terminate called after throwing an instance of 'std::filesystem::__cxx11::filesystem_error'
  what():  filesystem error: cannot copy: Permission denied
```

## 原因分析

这是由于 Docker 24.0 版本的 HostPath 挂载默认文件的权限为 0600，不支持 0200。然而，在 C++17 标准的 `std::filesystem::copy` 系统调用流程中，该函数首先会创建一个具有仅写权限（0200）的目标文件，然后将源文件的内容写入该目标文件，最后复制源文件 mode 至目标文件，mode 复制到目标文件失败，从而导致目标文件权限报错。

### 文件权限介绍

文件权限种类：

- 读取权限（4）：允许读取文件内容。
- 写入权限（2）：允许写入文件内容。
- 执行权限（1）：允许执行文件。
- 特殊权限（0）：禁止访问文件或者特殊权限位。

文件权限用户：

- 文件所有者：拥有文件的权限。
- 文件所属组：拥有文件权限的组。
- 其他用户：拥有文件权限的其他用户。

在 Linux 系统中，可以通过 `ls -la` 命令查看文件权限，例如：`-rw-------` 表示文件权限为 `0600`。其中，第一个 `-` 表示文件，其他类型包括如 d 代表目录、l 代表链接等。`rw-` 表示文件所有者具有读写权限，`---` 表示文件所属组无权限，`---` 表示其他用户无权限。

## 排查

```bash
# 确认 Docker 版本
docker version --format '{{.Server.Version}}'

# 查看挂载目录与文件权限
docker inspect -f '{{range .Mounts}}{{.Source}} -> {{.Destination}} ({{.Mode}}){{"\n"}}{{end}}' <container>
ls -la <host-path>

# 在容器内复现
docker exec -it <container> sh -c 'ls -la /path/in/container && touch /path/in/container/test'
```

如果宿主机文件为 `0600`、容器内目标文件创建后无法完成 mode 复制，即可确认是该问题。

## 推荐解决方案

推荐的解决方法是使用 C++ 的 `fstream` 库以编程方式复制文件内容，并在创建新文件时指定 `0600` 权限，然后将源文件内容写到目标文件。这样，新文件对拥有者提供读写权限，从而避免了权限问题。

示例：

```cpp
#include <fstream>
#include <sys/stat.h>

void CopyFileWithMode(const std::string& src, const std::string& dst) {
  std::ifstream in(src, std::ios::binary);
  std::ofstream out(dst, std::ios::binary | std::ios::trunc);
  out << in.rdbuf();
  out.close();
  chmod(dst.c_str(), 0600);
}
```

如果不方便修改业务代码，也可以考虑：

1. 在复制前显式创建目标文件并设置权限：

   ```cpp
   int fd = open(dst.c_str(), O_CREAT | O_WRONLY, 0600);
   close(fd);
   std::filesystem::copy_file(src, dst, std::filesystem::copy_options::overwrite_existing);
   ```

2. 避免把需要复制的文件放在 hostPath，改用 Docker Volume；
3. 升级/回退 Docker 版本，并在测试环境验证挂载权限行为。

## 验证

```bash
# 容器内重新执行复制逻辑
docker exec -it <container> <your-command>

# 检查目标文件权限
docker exec -it <container> ls -la /path/in/container
```

目标文件权限应为 `-rw-------` 或业务期望的权限，且不再出现 `Permission denied`。

## 注意事项

- 该问题与容器是否以 root 运行无直接关系，核心是 hostPath 挂载文件的 mode 与复制流程不兼容。
- 修改文件权限时避免使用 `0777`，应遵循最小权限原则。
- 如果复制的是目录，需要递归处理并分别设置文件与目录权限。

## 相关文档

- [部署要求](../../deployment/deploy_check.md)
- [Docker 官方安装文档](https://docs.docker.com/engine/install/)
- [部署失败排查](../deployment/deploy_failed.md)
