# OSS（对象存储服务）Endpoint 配置问题

## 问题描述

Kuscia 访问百度云对象存储服务 BOS 时报错，错误信息如下：

```text
botocore.exceptions.ClientError: An error occurred (400) when calling the ListObjectsV2 operation: Bad Request
```

该错误通常出现在以下场景：

- 通过 Kuscia API 注册 `type: oss` 的 DomainDataSource；
- 任务通过 DataMesh 读取 OSS/BOS 数据；
- Endpoint 使用了云厂商的通用域名，而不是兼容 S3 的域名。

## 原因分析

Kuscia 的 OSS 数据源使用 AWS S3 标准协议访问对象存储。部分云厂商同时提供两种 Endpoint：

- 通用对象存储 Endpoint，例如 `bcebos.xxx.com`；
- 兼容 S3 协议的 Endpoint，例如 `s3.bcebos.xxx.com`。

如果使用通用 Endpoint，客户端发出的 S3 签名请求格式与厂商预期不一致，`ListObjectsV2` 等操作会返回 400。

## 解决方案

使用 Kuscia API 注册数据源时，需要带上 `s3` 前缀来标明客户端将使用 AWS S3 标准协议连接 BOS。即如果 BOS 的地址为 `bcebos.xxx.com`，需要使用 `s3.bcebos.xxx.com` 作为 Endpoint。示例如下所示：

```bash
export CTR_CERTS_ROOT=/home/kuscia/var/certs
curl -k -X POST 'https://localhost:8082/api/v1/domaindatasource/create' \
 --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
 --header 'Content-Type: application/json' \
 --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
 --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
 --cacert ${CTR_CERTS_ROOT}/ca.crt \
 -d '{
  "domain_id": "alice",
  "datasource_id":"demo-oss-datasource",
  "type":"oss",
  "name": "DemoDataSource",
  "info": {
      "oss": {
          "endpoint": "https://s3.bcebos.xxx.com",
          "bucket": "secretflow",
          "prefix": "kuscia/",
          "access_key_id":"ak-xxxx",
          "access_key_secret" :"sk-xxxx"
      }
  },
  "access_directly": true
}'
```

> 注意：`endpoint` 需要包含 `http://` 或 `https://` 协议头，且不要附加 bucket 名称；bucket 与 prefix 分别在 `bucket`、`prefix` 字段中配置。

## 验证

1. 查询数据源，确认配置已生效：

    ```bash
    curl -k -X POST 'https://localhost:8082/api/v1/domaindatasource/query' \
     --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
     --header 'Content-Type: application/json' \
     --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
     --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
     --cacert ${CTR_CERTS_ROOT}/ca.crt \
     -d '{
      "domain_id": "alice",
      "datasource_id": "demo-oss-datasource"
    }'
    ```

2. 基于该数据源创建 DomainData 并运行一个最小任务，确认 DataMesh 可以正常读取对象存储中的文件。

## 各云平台 Endpoint 参考

不同云平台提供的存储服务 Endpoint 有所差异，需要根据具体平台的要求进行适配。

百度云 BOS 关于兼容 AWS S3 的文档可以参考[这里](https://cloud.baidu.com/doc/BOS/s/ojwvyq973)。

| 类型 | Java SDK 是否带 s3 前缀 | Python SDK 是否带 s3 前缀 |
| :---: | :---: | :---: |
| 阿里云 OSS | 带 | 带和不带前缀都支持 |
| 百度 BOS | 带 | 带 |
| 华为 OBS | 不带 | 不带 |
| 开源 minio | 不带 | 不带 |

## 相关文档

- [DomainDataSource 概念](../../reference/concepts/domaindatasource_cn.md)
- [DomainDataSource API](../../reference/apis/domaindatasource_cn.md)
- [DataMesh 数据读写](../../reference/apis/datamesh/datacrud_cn.md)
- [Kuscia 配置文件](../../deployment/kuscia_config_cn.md)
