# Log

## 接口总览

| 方法名 | 请求类型 | 响应类型 | 描述 |
| --- | --- | --- | --- |
| 查询日志 | QueryLogRequest | QueryLogResponse（流式） | 查询某个 Kuscia 任务的运行日志 |

## 接口详情

### 查询日志

#### 说明

查询某个 Kuscia 任务的运行日志

- 仅支持 Lite 和 Autonomy节点，不支持 Master 节点
- 仅支持查询任务本方节点的日志，即任务如果有 Alice，Bob 多方参与，调用 Alice 节点的接口只会查询 Alice 节点的运行日志
- 调用查询日志接口时，对于重启了多次的 pod 容器，会查询最新一次重启的 pod 容器的日志文件并返回

#### HTTP 路径

```text
POST /api/v1/log/task/query
```

#### 请求（QueryLogRequest）

| 字段              | 类型                                           | 选填 | 描述                                                                                                                         |
|-----------------|----------------------------------------------|----|----------------------------------------------------------------------------------------------------------------------------|
| header          | [RequestHeader](summary_cn.md#requestheader) | 可选 | 自定义请求内容                                                                                                                    |
| task_id          | string                                       | 必填 | TaskID |
| replica_idx       | int                                       | 可选 | Task对应的Pod副本索引(从0开始)；默认不填时，单副本时直接展示，多副本时选择第一个副本展示                                                                                                                   |
| container | string                                        | 可选 | 容器名，默认不填时，Task对应的Pod只有一个容器时展示，存在多个容器时报错                                                                         |
| follow           | bool                              | 可选 | 是否跟踪pod日志，默认为false（不跟踪）                                                                                                                       |

字段说明：

- `task_id` 可通过 `kubectl get task` 或 KusciaAPI 的任务查询接口获得；
- 多副本任务（如 Serving）需要指定 `replica_idx`，否则默认读取第一个副本；
- Pod 内存在多个容器时必须指定 `container`，否则接口返回错误；
- `follow=true` 时连接保持打开，持续返回新日志，适合调试运行中的任务。

#### 响应（QueryLogResponse）

流式返回响应结果，每次返回的结果如下：

| 字段                   | 类型                                    | 描述     |
|----------------------|---------------------------------------|--------|
| status | [Status](summary_cn.md#status) | 状态信息 |
| log               | string        | 正确时返回日志内容（每次返回多行），错误时返回错误信息。  |

响应是**流式**的：每行是一段 JSON，客户端需要逐行解析，直到连接关闭。

#### 请求示例

发起请求

```sh
# Execute the example inside the container
export CTR_CERTS_ROOT=/home/kuscia/var/certs
curl -v -k -X POST 'https://localhost:8082/api/v1/log/task/query' \
 --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
 --header 'Content-Type: application/json' \
 --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
 --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
 --cacert ${CTR_CERTS_ROOT}/ca.crt \
 -d '{
  "task_id": "secretflow-task-20241101160338-single-psi",
  "replica_idx": 0,
  "container": "secretflow",
  "follow": false
}'
```

请求响应成功结果：

```text
{"status":{"message":"success"},"log":"abcd"}
{"status":{"message":"success"},"log":"efgh"}
```

跟踪运行中的任务日志：

```bash
# -N 关闭 curl 缓冲，实时输出
curl -N -k -X POST 'https://localhost:8082/api/v1/log/task/query' \
 --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
 --header 'Content-Type: application/json' \
 --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
 --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
 --cacert ${CTR_CERTS_ROOT}/ca.crt \
 -d '{"task_id":"<task-id>","follow":true}'
```

只提取日志内容：

```bash
... | jq -r 'select(.log != null) | .log'
```

## 常见问题

| 现象 | 可能原因 | 处理 |
| --- | --- | --- |
| `container is required` | Pod 内有多个容器 | 指定 `container`，可通过 `kubectl get po -o jsonpath='{.spec.containers[*].name}'` 查看 |
| 日志为空 | 任务尚未开始、容器已清理或副本索引错误 | 确认任务状态与 `replica_idx`，检查 Pod 是否仍在运行 |
| 连接长时间无输出 | `follow=true` 且暂无新日志 | 属于正常现象；可先使用 `follow=false` 获取历史日志 |
| 401 / 403 | Token 或证书错误 | 使用容器内 `/home/kuscia/var/certs` 下的证书与 Token |
| 仅能看到本节点日志 | 接口设计如此 | 分别到各参与方节点查询 |

## 相关文档

- [KusciaAPI 概览](summary_cn.md)
- [错误码说明](error_code_cn.md)
- [任务失败排查](../../troubleshoot/runtask/run_job_failed.md)
- [Kuscia 部署验证](../../deployment/kuscia_deployment_instructions.md#验证部署)
