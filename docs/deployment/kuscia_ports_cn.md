# Kuscia 端口介绍

在实际场景中，为了确保 Kuscia 运行在一个安全的网络环境中，用户需要根据本地网络防火墙，管理 Kuscia 对外暴露给合作机构的端口。

如果采用 Docker 方式部署，那么在部署的过程中，为了能够跨域和域内访问 Kuscia 节点，需要将 Kuscia 节点的部分内部端口通过端口映射的方式暴露在宿主机上。

下面是需要用户感知的端口信息，按是否需要暴露给合作方，分为两类：

* 是。该类端口需要通过公网或专线能够让合作方访问。
* 否。该类端口仅需局域网内使用，无需暴露给合作方。

## 端口总览

| 协议 | 端口号 | 说明 | 是否需要暴露给合作方 | 部署脚本对应参数 |
| --- | --- | --- | --- | --- |
| HTTP/HTTPS | 1080 | 节点之间的认证鉴权端口。在创建节点之间路由时需要指定，可参考[创建节点路由](../reference/apis/domainroute_cn.md#请求createdomainrouterequest) | 是 | `-p` |
| HTTP | 80 | 访问节点中应用的端口。例如：可通过此端口访问 Serving 服务进行预测打分，可参考[使用 SecretFlow Serving 进行预测](../tutorial/run_sf_serving_with_api_cn.md#使用-secretflow-serving-进行预测) | 否 | `-q` |
| HTTP/HTTPS | 8082 | 节点 KusciaAPI 的 HTTP 访问端口，可参考[如何使用 KusciaAPI](../reference/apis/summary_cn.md#如何使用-kuscia-api) | 否 | `-k` |
| GRPC/GRPCS | 8083 | 节点 KusciaAPI 的 gRPC 访问端口，可参考[如何使用 KusciaAPI](../reference/apis/summary_cn.md#如何使用-kuscia-api) | 否 | `-g` |
| HTTP | 9091 | 节点 Metrics 指标采集端口，可参考 [Kuscia 监控](./kuscia_monitor.md) | 否 | `-x` |

以下端口通常只在节点内部或本地调试时使用，**不建议暴露给合作方**：

| 协议 | 端口号 | 说明 | 使用建议 |
| --- | --- | --- | --- |
| HTTP | 8070 | DataMesh HTTP（mTLS） | 仅容器/集群内访问；本地调试时可绑定 `127.0.0.1` |
| gRPC | 8071 | DataMesh gRPC（mTLS） | 同上 |
| HTTP | 8092 | KusciaAPI 内部 HTTP 端口 | 仅节点内部组件访问 |
| HTTP | 8050 | Reporter HTTP | 仅节点内部访问 |
| HTTP | 8060 | ConfManager HTTP | 仅节点内部访问 |
| gRPC | 8061 | ConfManager gRPC | 仅节点内部访问 |
| gRPC | 9090 | Transport gRPC | 仅节点内部访问 |

> **提示**：上表中的内部端口由 Kuscia 组件自动使用，一般不需要在 `docker run` 中额外映射。需要从宿主机直接调试 DataMesh 时，再按需映射到 `127.0.0.1`。

## Docker 端口映射

Docker 部署脚本通过以下参数把容器端口映射到宿主机：

| 参数 | 容器端口 | 说明 |
| --- | --- | --- |
| `-p` | 1080 | 跨域网关，必须让合作方访问 |
| `-q` | 80 | 节点应用入口 |
| `-k` | 8082 | KusciaAPI HTTP |
| `-g` | 8083 | KusciaAPI gRPC |
| `-x` | 9091 | Metrics |

示例：

```bash
# 宿主机 11080 -> 容器 1080（跨域网关）
# 宿主机 11081 -> 容器 80（应用入口）
# 宿主机 18082 -> 容器 8082（KusciaAPI HTTP）
# 宿主机 18083 -> 容器 8083（KusciaAPI gRPC）
./kuscia.sh start -c autonomy_alice.yaml -p 11080 -q 11081 -k 18082 -g 18083
```

中心化模式下，Master 与各 Lite 的宿主机端口应错开规划，例如：

| 节点 | `-p`（1080） | `-q`（80） | `-k`（8082） | `-g`（8083） |
| --- | --- | --- | --- | --- |
| Master | 18080 | 18081 | 18082 | 18083 |
| Lite Alice | 28080 | 28081 | 28082 | 28083 |
| Lite Bob | 38080 | 38081 | 38082 | 38083 |

以上仅为示例，请以实际部署脚本和网络规划为准。

## 安全暴露建议

1. **只暴露 1080**：跨域通信只需要 1080；其余端口默认不应对公网开放。
2. **管理端口绑定内网或回环地址**：例如 `-p 127.0.0.1:8082:8082`，避免 KusciaAPI 直接暴露在公网。
3. **Metrics 只对监控系统开放**：9091 无业务鉴权，建议通过内网、NetworkPolicy 或安全组限制来源。
4. **启用 TLS**：跨域与 KusciaAPI 在生产环境应使用 `tls` / `mtls`；证书由 Kuscia 统一签发和轮转。
5. **不要绕过防火墙**：Docker 发布端口会写入 `DOCKER-USER` 链，可能绕过 firewalld / ufw。生产环境请同时配置云安全组或 `DOCKER-USER` 规则。

防火墙示例：

```bash
# 仅放行跨域网关
sudo firewall-cmd --permanent --add-port=1080/tcp
sudo firewall-cmd --reload

# 如需限制来源网段
sudo firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="10.0.0.0/8" port port="1080" protocol="tcp" accept'
```

## 端口检查

部署前检查端口占用：

```bash
ss -lntp | grep -E ':(1080|80|8082|8083|9091)\b'
```

部署后检查映射与连通性：

```bash
# 查看容器端口映射
docker port ${USER}-kuscia-autonomy-alice

# 检查跨域网关是否可达（在合作方机器执行）
curl -vk --connect-timeout 5 https://<peer-host>:1080/

# 检查 KusciaAPI 健康状态（在节点容器内执行）
docker exec -it ${USER}-kuscia-autonomy-alice bash -c '
export CTR_CERTS_ROOT=/home/kuscia/var/certs
curl -k -s -X POST "https://localhost:8082/healthZ" \
  --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
  --header "Content-Type: application/json" \
  --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
  --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
  --cacert ${CTR_CERTS_ROOT}/ca.crt \
  -d "{}"'
```

K8s 部署使用 Service / Ingress 暴露端口，检查命令：

```bash
kubectl get svc -n kuscia-master
kubectl get ingress -A
```

## 相关文档

* [部署要求](./deploy_check.md)
* [网络要求](./networkrequirements.md)
* [Kuscia 部署指引](./kuscia_deployment_instructions.md)
* [Kuscia 监控](./kuscia_monitor.md)
* [网络连通性排查](../troubleshoot/network/network_troubleshoot.md)
