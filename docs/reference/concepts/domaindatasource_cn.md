# DomainDataSource

Kuscia 通过 DomainDataSource 来管理数据源信息，数据源是指存储数据的地方，其可以是本地文件路径、对象存储服务 OSS（或支持标准 AWS S3 协议的对象存储服务）、MySQL 等。DomainDataSource 对象用于描述数据源的元数据（MetaData），
如一个描述 OSS 数据源的 DomainDataSource 对象会包含：访问 OSS 所需的 Endpoints、Bucket、AK、SK 等信息（敏感信息会加密保存在 DomainDataSource 的 encryptedInfo 字段中）。一个描述本地文件路径数据源的 DomainDataSource 对象会包含：本地文件数据源的绝对路径（如下示例中的 uri 字段）；

## 对象结构

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `metadata.name` | string | 数据源在 Kuscia 中的唯一标识（CRD 名称） |
| `metadata.namespace` | string | 所属节点（Domain），例如 `alice` |
| `metadata.labels` | map | 建议包含 `kuscia.secretflow/domaindatasource-type`，便于按类型查询 |
| `spec.name` | string | 数据源展示名称，可重复 |
| `spec.type` | string | 数据源类型：`localfs`、`oss`、`mysql`、`odps` |
| `spec.uri` | string | 数据源位置摘要；localfs 为绝对路径，OSS 为 bucket/prefix |
| `spec.accessDirectly` | bool | 任务是否可直连数据源；`false` 时通过 DataProxy 访问 |
| `spec.data.encryptedInfo` | string | 加密后的连接信息（OSS AK/SK、MySQL DSN 等） |
| `spec.info` | object | 明文连接信息，仅部分类型/场景使用，生产环境应避免 |

以下是一个描述本地文件路径（localfs）数据源的 DomainDataSource CRD 的示例：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainDataSource
metadata:
  labels:
    kuscia.secretflow/domaindatasource-type: localfs
  name: default-data-source
  namespace: alice
spec:
  accessDirectly: true
  data:
    encryptedInfo: joPdxNPMZJk6DiD/M1KkjSSY58BnLxaOM41z3uB5SmkjpDJ14+4qXqdsWRXAqMoPBdxRVDa/GhdUKnr/hL7eBZFMVivFCtCqMTaBJunUYsH6U168wzWaYrERme8BmNChETY3HdIeM4wP7o72+ctDoPDASuAWZNoJ5hxnYfYcpxJ3YoG3STf2DzqYmIZeAWxlneJ32wWFtSBlo1hzIvxuiuwHXZaq7h77a/+H7s1paUpio8wTkqJohID4k37pX3thuit9OUEsqxDxl5SEm+qq9HeVC8XscuKVs3nQw/cmSg4LavtAQWqrk7qjqYWmd370z6cQjuHOCbX1gZ2UbCjpnw==
  name: default-data-source
  type: localfs
  uri: /home/kuscia/var/storage/data
```

DomainDataSource `metadata` 的子字段详细介绍如下：

- `labels`：标签在 K3s 中用于支持高效的查询和监听操作，参考：[标签和选择算符](https://kubernetes.io/zh-cn/docs/concepts/overview/working-with-objects/labels/)。
- `name`：表示数据源的标识，用于唯一标识数据源。
- `namespace`：表示 DomainDataSource 所属的命名空间，即所属的节点，当前示例为 `alice`。

DomainDataSource `spec` 的子字段详细介绍如下：

- `accessDirectly`：隐私计算应用（如 SecretFlow ）是否可直连访问数据源的标志位，true：应用直连访问数据源（不经过 DataProxy），false: 应用可通过 DataProxy 访问数据源。当前默认设置为 false，使用 odps 类型时目前必须经过 DataProxy。
- `encryptedInfo`：加密存储访问数据源所需的信息，如数据源为 MySQL 时，此字段会加密保存 MySQL 的链接串。
- `name`：数据源的名称，可重复，注意区别于 metadata 中的 name 字段。
- `type`：表示数据源的类型，类型包括：localfs, oss, mysql。
- `uri`：表示数据源的概括信息，如 localfs 此处为绝对路径（如 /home/kuscia/var/storage/data），如 OSS 此处为 bucket 与 prefix 拼接的路径（alice_bucket/kuscia/data/）。

## 支持的数据源类型

| 类型 | `accessDirectly` | 说明 | 典型配置字段 |
| --- | --- | --- | --- |
| `localfs` | 通常为 `true` | 节点本地目录，任务可直接读取 | `uri`（绝对路径） |
| `oss` | 建议 `true` | S3 兼容对象存储 | endpoint、bucket、prefix、AK/SK、virtualhost |
| `mysql` | 建议 `true` | MySQL 数据库 | endpoint、user、password、database |
| `odps` | 必须 `false` | 阿里云 MaxCompute，经 DataProxy 访问 | endpoint、project、AK/SK、partition |

## 加密信息

`encryptedInfo` 使用目标 Domain 的公钥加密，因此：

- **推荐通过 KusciaAPI 创建/更新**，由服务端完成加密；
- 不要手工拼接或修改 `encryptedInfo`；
- 更换 Domain 证书后，旧的 `encryptedInfo` 可能无法解密，需要重新创建数据源；
- 敏感信息不要写入 `spec.info` 等明文字段，也不要提交到代码仓库。

## 用例

以下是一些 DomainDataSource 的典型用例：

- 创建 DomainDataSource
- 更新 DomainDataSource
- 清理 DomainDataSource

## 创建 DomainDataSource

因 DomainDataSource 中含有 encryptedInfo 字段，其需要使用 Domain 节点的公钥进行加密，推荐使用 Kuscia API 进行创建，[参考 kuscia API 文档](../apis/domaindatasource_cn.md#创建数据源)。

API 支持 localfs、OSS、MySQL、ODPS 四种示例：

- [本地文件数据源示例](../apis/domaindatasource_cn.md#创建本地文件数据源示例)
- [对象存储数据源示例](../apis/domaindatasource_cn.md#创建对象存储服务数据源示例)
- [MySQL 数据源示例](../apis/domaindatasource_cn.md#创建-mysql-数据库数据源示例)
- [ODPS 数据源示例](../apis/domaindatasource_cn.md#创建-odps-数据库数据源示例)

创建后检查：

```bash
kubectl get domaindatasource -n alice
kubectl describe domaindatasource demo-localfs-datasource -n alice
```

## 更新 DomainDataSource

因 DomainDataSource 中含有 encryptedInfo 字段，其需要使用 Domain 节点的公钥进行加密，推荐使用 Kuscia API 进行更新，[参考 kuscia API 文档](../apis/domaindatasource_cn.md#更新数据源)。

更新时注意：

- 修改连接信息后，依赖该数据源的 DomainData 仍指向原 `datasource_id`，无需重建 DomainData；
- 更新不会迁移已有数据；
- 如果仅修改 `accessDirectly`，请确认任务镜像具备直连数据源的能力。

## 清理 DomainDataSource

下面以删除 alice Domain中以 `demo-localfs-datasource` 为数据源标识的 DomainDataSource 为例：
注意：清理 DomainDataSource 并不会清除真实的数据源，只是从 Kuscia 中删除 DomainDataSource CRD 对象。

1. 运行以下命令清理 DomainDataSource。

    ```shell
    kubectl delete domaindatasource demo-localfs-datasource -n alice
    ```

2. 检查 Alice 下的 DomainDataSource 是否已被清理。

    ```shell
    kubectl get domaindatasource demo-localfs-datasource -n alice
    Error from server (NotFound): domaindatasources.kuscia.secretflow "demo-localfs-datasource" not found
    ```

## 注意事项

- 删除 DomainDataSource 前先确认没有 DomainData 或运行中的任务依赖它；
- `accessDirectly=false` 时，任务需要通过 DataProxy 访问数据源，需确保 DataProxy 已部署且网络可达；
- ODPS 数据源必须经过 DataProxy，不能设置 `accessDirectly=true`；
- 生产环境建议为每个数据源配置最小权限的 AK/SK 或数据库账号。

## 相关文档

- [DomainDataSource API](../apis/domaindatasource_cn.md)
- [DomainData](./domaindata_cn.md)
- [Kuscia 数据代理](../../数据代理.md)
- [DataMesh 数据读写](../apis/datamesh/datacrud_cn.md)
