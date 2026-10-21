# Kine 表问题导致 Kuscia 启动失败

## 前言

Kuscia 在中心化或点对点模式下使用内嵌 K3s；当配置了外部 `datastoreEndpoint` 时，K3s 通过 [Kine](https://github.com/k3s-io/kine) 把 Kubernetes 数据写入 MySQL / PostgreSQL。Kine 表缺失、权限不足或多个节点共用同一张表，都会导致 Kuscia 启动失败。

## 常见现象

### Kine 表不存在

Kuscia 启动失败可能是 Kine 表在数据库中不存在导致的，可以使用如下方法解决：

1. 部署 Kuscia 前先手动创建好 Kine 表，建表语句参考 [kine](https://github.com/secretflow/kuscia/blob/main/hack/k8s/kine.sql)。
2. 提供的数据库账号有建表权限（账号具有 `DDL+DML` 权限），并且数据表不存在，kuscia 会尝试自动建表，如果创建失败 kuscia 会启动失败。

启动日志中通常可以看到类似报错：

```text
failed to create kine table
Error 1146 (42S02): Table 'kine.kine' doesn't exist
```

### Kine 多节点混用导致启动失败

Kuscia 启动失败可能是多个节点使用同一张表导致的，报错示例如下：

```bash
2024-06-27 19:22:53.070 ERROR modules/k3s.go:283 context canceled
2024-06-27 19:22:53.070 FATAL modules/modules.go:181 error building kubernetes client config from token, detail-> build config from flags failed, detail-> stat /home/kuscia/etc/kubeconfig: no such file or directory
```

可以使用如下方法解决：

1. 确保一个节点（Domain） 使用一张 Kine 表。
2. 复用 Kine 表需保证 DomainID 与之前的节点保持一致。

## 排查

```bash
# 1. 确认 Kuscia 使用的 datastoreEndpoint
docker exec -it ${USER}-kuscia-master cat /home/kuscia/etc/conf/kuscia.yaml | grep -A2 datastore

# 2. 登录数据库，确认库/表是否存在
mysql -h <host> -P 3306 -u <user> -p
```

```sql
SHOW DATABASES;
USE kine;
SHOW TABLES;
SHOW CREATE TABLE kine\G
```

```bash
# 3. 确认账号权限
mysql -h <host> -u <user> -p -e "SHOW GRANTS FOR CURRENT_USER();"

# 4. 确认是否有多个节点使用了同一个 DSN
#    在每个节点的 kuscia.yaml 中比对 datastoreEndpoint
```

## 解决方案

### 1. 创建独立数据库或表

为每个 Domain 使用独立的数据库（Kine 会在库内创建 `kine` 表）：

```sql
CREATE DATABASE IF NOT EXISTS kine_alice CHARACTER SET utf8mb4;
CREATE USER IF NOT EXISTS 'kuscia'@'%' IDENTIFIED BY '<password>';
GRANT ALL PRIVILEGES ON kine_alice.* TO 'kuscia'@'%';
FLUSH PRIVILEGES;
```

也可以使用仓库中的建表语句手动创建：

```bash
mysql -h <host> -u <user> -p kine_alice < hack/k8s/kine.sql
```

### 2. 配置独立的 datastoreEndpoint

```yaml
# kuscia.yaml
datastoreEndpoint: "mysql://kuscia:<password>@tcp(<host>:3306)/kine_alice"
```

每个节点的库名应不同，例如 `kine_alice`、`kine_bob`、`kine_master`。

### 3. 重新启动节点

```bash
docker restart ${USER}-kuscia-master
docker logs -f ${USER}-kuscia-master
```

## 验证

```bash
# Kuscia 进程正常
docker exec -it ${USER}-kuscia-master kubectl get po -A

# K3s 已连接外部数据库
docker exec -it ${USER}-kuscia-master kubectl get nodes

# 数据库中已存在 kine 表且有数据
mysql -h <host> -u <user> -p -e "SELECT COUNT(*) FROM kine_alice.kine;"
```

Kuscia 能持续运行且 `kubectl get nodes` 正常返回，说明 Kine 配置正确。

## 注意事项

- Kine 表保存的是整个 K3s 控制面数据，**删除表等同于清空集群控制面**，操作前务必确认库名并做好备份。
- 不同节点共用同一张表会造成数据互相覆盖，集群表现异常且难以恢复。
- 生产环境建议为 Kine 使用独立数据库实例或独立 Schema，并配置定期备份。
- 连接数问题参见[数据库连接数过多](../parameter_tuning/mysql_too_many_connections.md)。

## 相关文档

- [Kine 建表语句](https://github.com/secretflow/kuscia/blob/main/hack/k8s/kine.sql)
- [部署要求](../../deployment/deploy_check.md)
- [数据库连接数过多](../parameter_tuning/mysql_too_many_connections.md)
- [部署失败排查](./deploy_failed.md)
