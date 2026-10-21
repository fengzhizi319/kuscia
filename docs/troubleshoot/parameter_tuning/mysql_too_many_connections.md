# 数据库连接数过多

## 问题描述

Kuscia 执行任务报错：

```text
2024-08-30 17:44:15.584 WARN handler/pending_handler.go:128 Failed to update task resource group "xxx-xxx" spec parties info, rpc error: code = Unknown desc = Error 1040 (08004): Too many connections
```

该错误表示 MySQL 当前连接数已达到 `max_connections` 上限，新的连接（包括 Kuscia / Kine 的连接）会被拒绝。常见影响包括任务状态无法更新、节点心跳异常、Kuscia 启动失败。

## 排查

```sql
-- 1. 查看最大连接数与当前连接数
SHOW VARIABLES LIKE 'max_connections';
SHOW STATUS LIKE 'Threads_connected';
SHOW STATUS LIKE 'Max_used_connections';

-- 2. 按用户、来源主机统计连接
SELECT user, host, COUNT(*) AS cnt
FROM information_schema.processlist
GROUP BY user, host
ORDER BY cnt DESC;

-- 3. 查看长时间空闲的连接
SELECT id, user, host, db, command, time, state
FROM information_schema.processlist
WHERE command = 'Sleep'
ORDER BY time DESC
LIMIT 50;

-- 4. 查看连接来自哪些库
SELECT db, COUNT(*) AS cnt
FROM information_schema.processlist
GROUP BY db
ORDER BY cnt DESC;
```

判断方向：

- `Threads_connected` 接近 `max_connections`，且大量连接处于 `Sleep`：可能是连接池未回收或 `wait_timeout` 过长；
- 某个业务库连接数异常增长：检查对应服务的连接池上限与连接泄漏；
- Kuscia / Kine 连接占满：检查节点数量与 Kine 配置，必要时为 Kuscia 使用独立数据库实例。

## 解决方法

1. 检查 MySQL 数据库配置的最大连接数 max_connections

    ```bash
    SHOW VARIABLES LIKE 'max_connections';
    ```

2. 查看当前数据库连接进程

    ```bash
    SHOW PROCESSLIST;
    ```

3. 临时修改数据库配置的最大连接数，此处以 1000 为例

    ```bash
    SET GLOBAL max_connections = 1000;
    ```

   临时修改立即生效，但 MySQL 重启后恢复原值。

4. 修改 MySQL 数据库配置文件 my.cnf，增加最大连接数

    ```ini
    [mysqld]
    max_connections = 1000
    wait_timeout = 600
    interactive_timeout = 600
    ```

   同时建议缩短空闲连接回收时间，避免大量 `Sleep` 连接长期占用配额。

5. 重启 MySQL 服务

    ```bash
    systemctl restart mysqld
    ```

6. 如确认存在异常空闲连接，可在维护窗口清理（谨慎操作，先确认连接归属）：

    ```sql
    KILL <connection_id>;
    ```

## 验证

```sql
SHOW VARIABLES LIKE 'max_connections';
SHOW STATUS LIKE 'Threads_connected';
SHOW STATUS LIKE 'Max_used_connections';
```

```bash
# Kuscia 日志中不再出现 Too many connections
docker exec -it ${USER}-kuscia-master grep -i "too many connections" /home/kuscia/var/logs/kuscia.log | tail
```

再提交一个任务，确认状态可以正常更新。

## 注意事项

- `max_connections` 不宜盲目调得过大：每个连接都会占用内存，过高可能导致 MySQL OOM。
- 优先排查连接泄漏和连接池配置，而不是只调大上限。
- 如果多个 Kuscia 节点共用一个 MySQL 实例，建议为 Kine 使用独立数据库/实例，并限制每节点的连接池大小。
- 修改 `my.cnf` 后必须重启 MySQL 才能持久生效。

## 相关文档

- [MySQL Max Connections](https://dev.mysql.com/doc/refman/8.0/en/server-system-variables.html#sysvar_max_connections)
- [Kine 表问题导致 Kuscia 启动失败](../deployment/kuscia_mysql_kine.md)
- [部署要求](../../deployment/deploy_check.md)
- [部署失败排查](../deployment/deploy_failed.md)
