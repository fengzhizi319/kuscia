# Lite 节点遗漏证书之后如何重新部署

## 前言

出于安全性的考虑，如果 master 已经有了节点的证书并且机构侧节点已经部署完成，此时节点证书丢失且想要复用节点，需要执行下文步骤。

适用场景：

- Lite 节点容器/数据目录被误删，但 Master 上仍保留该 Domain 的注册信息和旧证书；
- 希望继续使用原 DomainID，而不是在 Master 上重新注册一个新节点。

> **风险提示**：清空证书会立即中断该节点与 Master 的现有认证关系，必须尽快完成重新部署。操作前请备份 Master 数据，并与合作方确认维护窗口。

## 重新部署节点

### 步骤一：查询节点当前信息

在 Master 容器内查询目标节点的角色与证书状态，后续更新时需要沿用相同的 `role`：

```bash
docker exec -it ${USER}-kuscia-master bash -c '
export CTR_CERTS_ROOT=/home/kuscia/var/certs
curl -k -s -X POST "https://localhost:8082/api/v1/domain/query" \
  --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
  --header "Content-Type: application/json" \
  --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
  --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
  --cacert ${CTR_CERTS_ROOT}/ca.crt \
  -d "{\"domain_id\":\"alice\"}"
'
```

记录返回中的 `role`、`master_domain_id` 和 `cert` 字段。

### 步骤二：清空 Master 上的旧证书

在 master 节点上删除 lite 之前的节点证书，证书字段设置为空。详情参考[这里](../../reference/apis/domain_cn.md#更新节点)

示例如下（`domain_id`、`role`、`master_domain_id` 按上一步的实际值填写）：

```bash
docker exec -it ${USER}-kuscia-master bash -c '
export CTR_CERTS_ROOT=/home/kuscia/var/certs
curl -k -s -X POST "https://localhost:8082/api/v1/domain/update" \
  --header "Token: $(cat ${CTR_CERTS_ROOT}/token)" \
  --header "Content-Type: application/json" \
  --cert ${CTR_CERTS_ROOT}/kusciaapi-server.crt \
  --key ${CTR_CERTS_ROOT}/kusciaapi-server.key \
  --cacert ${CTR_CERTS_ROOT}/ca.crt \
  -d "{\"domain_id\":\"alice\",\"role\":\"partner\",\"master_domain_id\":\"kuscia-system\",\"cert\":\"\"}"
'
```

期望返回：

```json
{"status":{"code":0,"message":"success","details":[]}}
```

再次查询节点，确认 `cert` 已为空。

### 步骤三：重新签发 Token 并部署节点

向 master 重新申请 Token 并部署节点。详情参考[这里](../../deployment/Docker_deployment_kuscia/deploy_master_lite_cn.md#部署-lite-节点-alice)

```bash
# 在 Master 上重新获取未使用的部署 Token
export ALICE_TOKEN=$(docker exec -it ${USER}-kuscia-master \
  kubectl get domain alice -o=jsonpath='{.status.deployTokenStatuses[?(@.state=="unused")].token}')
echo "Alice 的部署 Token: ${ALICE_TOKEN}"
```

如果 Token 不存在或已使用，先重新注册节点：

```bash
docker exec -i ${USER}-kuscia-master sh scripts/deploy/add_domain_lite.sh alice
```

然后按部署文档生成配置并启动：

```bash
docker run -it --rm ${KUSCIA_IMAGE} kuscia init \
  --mode lite --domain "alice" \
  --master-endpoint "https://<master-ip>:<port>" \
  --lite-deploy-token "${ALICE_TOKEN}" > lite_alice.yaml

./kuscia.sh start -c lite_alice.yaml -p 28080 -k 28081
```

## 验证

```bash
# 节点重新上线
docker exec -it ${USER}-kuscia-master kubectl get domain alice -o yaml

# 证书已重新签发且非空
docker exec -it ${USER}-kuscia-master kubectl get domain alice \
  -o jsonpath='{.status.certStatus}'

# 路由与任务恢复正常
docker exec -it ${USER}-kuscia-master kubectl get domainroutes
docker exec -it ${USER}-kuscia-master kubectl get kj -A
```

再提交一个最小任务，确认跨域通信恢复。

## 注意事项

- 清空证书后，旧节点的所有跨域路由会失效；重新部署后需确认相关 `DomainRoute` 仍为 `Ready`。
- 如果 **Master 自身的证书或私钥丢失**，无法通过上述方式恢复集群身份，需要重建集群并重新与各机构组网。
- 建议开启证书目录的定期备份，并将 `var/certs` 纳入主机级备份策略。
- Token 属于敏感凭据，不要写入脚本仓库或聊天记录。

## 相关文档

- [更新节点 API](../../reference/apis/domain_cn.md#更新节点)
- [多机部署中心化集群](../../deployment/Docker_deployment_kuscia/deploy_master_lite_cn.md)
- [Kuscia 部署指引](../../deployment/kuscia_deployment_instructions.md)
- [网络连通性排查](../network/network_troubleshoot.md)
- [升级 Kuscia 版本](../../deployment/Docker_deployment_kuscia/deploy_upgrade_kuscia_cn.md)
