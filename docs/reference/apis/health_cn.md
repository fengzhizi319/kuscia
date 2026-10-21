# Health

Health 提供了服务的健康检查，你可以借助这些 API 了解服务的健康状态。
你可以从 [这里](https://github.com/secretflow/kuscia/tree/main/proto/api/v1alpha1/kusciaapi/health.proto) 找到对应的 protobuf 文件。

## 接口总览

| 方法名                 | 请求类型          | 响应类型     | 描述     |
|---------------------|---------------|----------------|--------|
| [healthZ](#healthz) | HealthRequest | HealthResponse | 服务是否健康 |

## 接口详情

(healthz)=

### 健康检查

#### 说明

`healthZ` 返回当前 KusciaAPI 及其依赖组件的就绪状态。`data.ready` 为 `true` 表示节点可以接收业务请求。

- 该接口只用于健康检查，不应承载业务查询；
- 接口使用 KusciaAPI 的 mTLS 与 Token 认证；
- 建议用于 K8s `readinessProbe`、负载均衡健康检查或部署后的冒烟验证。

#### HTTP 路径

```text
POST /healthZ
```

#### 请求（HealthRequest）

| 字段     | 类型                                           | 选填 | 描述      |
|--------|----------------------------------------------|----|---------|
| header | [RequestHeader](summary_cn.md#requestheader) | 可选 | 自定义请求内容 |

请求头：

| Header | 是否必填 | 说明 |
| --- | --- | --- |
| `Token` | 是 | KusciaAPI Token，位于 `/home/kuscia/var/certs/token` |
| `Content-Type` | 是 | 固定为 `application/json` |

#### 响应（HealthResponse）

| 字段     | 类型                                           | 描述   |
|------------|--------------------------------|------|
| status     | [Status](summary_cn.md#status) | 状态信息 |
| data       | HealthResponseData             |      |
| data.ready | bool                           | 是否就绪 |

#### 请求示例

发起请求：

```sh
# execution example in container
export CTR_CERTS_ROOT=/home/kuscia/var/certs
curl -k -X POST 'https://localhost:8082/healthZ' \
 --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
 --header 'Content-Type: application/json' \
 --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
 --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
 --cacert ${CTR_CERTS_ROOT}/ca.crt \
 -d '{}'
```

请求响应成功结果：

```json
{
  "status": {
    "code": 0,
    "message": "success",
    "details": []
  },
  "data": {
    "ready": true
  }
}
```

只取就绪状态：

```bash
curl -k -s -X POST 'https://localhost:8082/healthZ' \
  --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
  --header 'Content-Type: application/json' \
  --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
  --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
  --cacert ${CTR_CERTS_ROOT}/ca.crt \
  -d '{}' | jq -r '.data.ready'
```

#### 在 K8s 中使用

```yaml
readinessProbe:
  httpGet:
    path: /healthZ
    port: 8082
    scheme: HTTPS
  initialDelaySeconds: 10
  periodSeconds: 10
```

> 如果探针无法携带客户端证书，请在 Service 或网格层完成 mTLS 终止，或改用节点内 `kubectl`/进程检查作为就绪判断。

## 常见问题

| 现象 | 可能原因 | 处理 |
| --- | --- | --- |
| 401 / 403 | Token 缺失或错误 | 检查 `/home/kuscia/var/certs/token` 与请求头 |
| TLS 握手失败 | 证书路径错误或证书过期 | 使用容器内预置证书，检查有效期 |
| `ready` 为 `false` | 依赖组件尚未就绪 | 查看 `/home/kuscia/var/logs/kuscia.log`，等待 DataMesh / Gateway 就绪 |
| 连接被拒绝 | KusciaAPI 未启动或端口未映射 | 检查 `kubectl get po` 与端口映射 |
| 404 | 请求路径或方法错误 | 确认使用 `POST /healthZ` |

## 相关文档

- [KusciaAPI 概览](summary_cn.md)
- [Kuscia 部署验证](../../deployment/kuscia_deployment_instructions.md#验证部署)
- [Kuscia 端口介绍](../../deployment/kuscia_ports_cn.md)
- [错误码说明](error_code_cn.md)
