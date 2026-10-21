# DataMesh 数据读写

Kuscia DataMesh 提供了基于 gRPC 的数据读写操作，实现了 Arrow Flight 提供的标准的数据服务接口，您可以通过初始化一个 Flight Client 来发起数据的请求。

## 基本概念

| 组件 | 作用 |
| --- | --- |
| DataMesh | Kuscia 内的数据服务，负责解析 DomainData、定位实际存储 |
| DataProxy | 当数据源不能被任务直连时，由 DataProxy 代理读写 |
| Arrow Flight | 数据传输协议，支持 `GetFlightInfo`、`DoGet`、`DoPut`、`DoAction` |
| Ticket / Descriptor | Flight 的服务端句柄与请求描述，由 DataMesh 生成和解析 |

一次典型的读取流程：

1. 客户端连接 DataMesh gRPC 端口（默认 `8071`，mTLS）；
2. 使用 `CommandDomainDataQuery` 构造 `FlightDescriptor`，调用 `GetFlightInfo`；
3. 从返回的 `Endpoint` 中取得实际数据端点与 `Ticket`；
4. 连接该端点并调用 `DoGet`，通过 `flight.NewRecordReader` 读取 Arrow 数据。

写入流程类似，使用 `CommandDomainDataUpdate` 与 `DoPut`，并根据目标数据源选择 CSV / 二进制选项。

## 如何通过Arrow Flight访问DataMesh

FlightServiceClient 是 Apache Arrow Flight 框架中客户端的主要接口，它提供了一系列的方法来与 Flight 服务交互，例如获取数据集信息、上传和下载数据等。在本例中，使用 Go 语言展示如何使用 `flight.FlightServiceClient`。

要使用 FlightServiceClient，您需要先建立一个与 Flight 服务端的 gRPC 连接，然后通过这个连接创建一个 FlightServiceClient 实例。

### 建立连接

```go
func createFlightClient(endpoint string, tlsConfig *tls.Config) (flight.Client, error) {
    // endpoint 可能带有 grpc+tcp:// 前缀，需要去掉
    if parts := strings.Split(endpoint, "//"); len(parts) == 2 {
        endpoint = parts[1]
    }

    dialOpts := []grpc.DialOption{
        grpc.WithKeepaliveParams(keepalive.ClientParameters{
            Time:                time.Minute,
            Timeout:             30 * time.Second,
            PermitWithoutStream: true,
        }),
    }
    if tlsConfig != nil {
        dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
    } else {
        dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
    }

    conn, err := grpc.Dial(endpoint, dialOpts...)
    if err != nil {
        return nil, err
    }
    return flight.NewClientFromConn(conn, nil), nil
}
```

> 生产环境应启用 mTLS，并使用 Kuscia 签发的客户端证书；示例中的 `insecure` 仅用于本地调试。

### 查询数据

![Alt text](../../../imgs/flight_do_get.png)

关键步骤：

```go
// 1. 构造查询命令
cmd := &datamesh.CommandDomainDataQuery{DomaindataId: domainDataID}
desc, err := utils.DescForCommand(cmd)
if err != nil {
    return err
}

// 2. 获取 FlightInfo
info, err := meshClient.GetFlightInfo(ctx, desc)
if err != nil {
    return err
}

// 3. 连接实际数据端点并读取
dpClient, err := createFlightClient(info.Endpoint[0].Location[0].Uri, tlsConfig)
if err != nil {
    return err
}
defer dpClient.Close()

stream, err := dpClient.DoGet(ctx, info.Endpoint[0].Ticket)
if err != nil {
    return err
}
reader, err := flight.NewRecordReader(stream)
if err != nil {
    return err
}
defer reader.Release()

for {
    record, err := reader.Read()
    if err == io.EOF {
        break
    }
    if err != nil {
        return err
    }
    // 处理 Arrow Record
    record.Release()
}
```

### 上传数据

![Alt text](../../../imgs/flight_do_put.png)

关键步骤：

```go
cmd := &datamesh.CommandDomainDataUpdate{
    DomaindataId: domainDataID,
    FileWriteOptions: &datamesh.FileWriteOptions{
        Options: &datamesh.FileWriteOptions_CsvOptions{
            CsvOptions: &datamesh.CSVWriteOptions{FieldDelimiter: ","},
        },
    },
}
desc, err := utils.DescForCommand(cmd)
// GetFlightInfo -> 连接 Endpoint -> DoPut -> Write Arrow Records
```

## DataMesh 支持的数据服务

DataMesh 当前仅支持以下查询能力:

```go
type FlightServiceClient interface {
 GetFlightInfo(ctx context.Context, in *FlightDescriptor, opts ...grpc.CallOption) (*FlightInfo, error)
 // For a given FlightDescriptor, get the Schema as described in Schema.fbs::Schema
 // This is used when a consumer needs the Schema of flight stream. Similar to
 // GetFlightInfo this interface may generate a new flight that was not previously
 // available in ListFlights.
 GetSchema(ctx context.Context, in *FlightDescriptor, opts ...grpc.CallOption) (*SchemaResult, error)
 // Retrieve a single stream associated with a particular descriptor
 // associated with the referenced ticket. A Flight can be composed of one or
 // more streams where each stream can be retrieved using a separate opaque
 // ticket that the flight service uses for managing a collection of streams.
 DoGet(ctx context.Context, in *Ticket, opts ...grpc.CallOption) (FlightService_DoGetClient, error)
 // Push a stream to the flight service associated with a particular
 // flight stream. This allows a client of a flight service to upload a stream
 // of data. Depending on the particular flight service, a client consumer
 // could be allowed to upload a single stream per descriptor or an unlimited
 // number. In the latter, the service might implement a 'seal' action that
 // can be applied to a descriptor once all streams are uploaded.
 DoPut(ctx context.Context, opts ...grpc.CallOption) (FlightService_DoPutClient, error)
 // Flight services can support an arbitrary number of simple actions in
 // addition to the possible ListFlights, GetFlightInfo, DoGet, DoPut
 // operations that are potentially available. DoAction allows a flight client
 // to do a specific action against a flight service. An action includes
 // opaque request and response objects that are specific to the type action
 // being undertaken.
 DoAction(ctx context.Context, in *Action, opts ...grpc.CallOption) (FlightService_DoActionClient, error)
}
```

## 注意事项

在使用 DataMesh（DataProxy）向支持的各种类型的数据源进行输出时，如果目标文件/表不存在，会<span style="color: red;">自动创建</span>。如果输出目标已经存在，均会尝试进行<span style="color: red;">文件覆盖</span> ，具体来说

1. **localfs**：打开输出文件时进行清空。如果目标目录不存在，会自动逐层创建。
2. **OSS**：直接上传文件。需配置**OSS**为允许自动创建目录、允许文件覆盖。
3. **MySQL**：尝试 `DROP IF EXISTS` ，并重新创建表。 如果 `DROP` 失败，继续尝试 `DELETE` 全表。仍失败则返回报错，退出存储。因此建议为 MySQL 配置 `CREATE / INSERT / SELECT / DROP` 权限。如果不能提供 `DROP` 权限，至少需具备 `DELETE` 权限。但注意此时清表速度将会下降。
4. **ODPS**: 经过 DataProxy 服务代理实现

- 读取数据时，需确保提供的 AK/SK 具备表的读取权限（没有 `Download` 权限只能查询低于 1W 行的数据）。
- 写入数据时，需确保提供的 AK/SK 具备表的覆盖写权限；如果需要 DataProxy 自行建表，需确保具备创建表的权限。
- 写入数据时，若表不存在，将创建表（表结构按照 DomainData 的信息来创建）；若任务配置输出信息中包含分区信息，将创建分区表，并创建分区（分区字段类型断言为字符串类型）。
- 写入数据时，若表已存在，任务输出的分区不存在时，将按照任务配置中输出的分区信息创建分区，需保证分区信息正确并可正常创建，若创建失败，将导致失败报错。
- 写入数据时将采用覆盖写（普通表将覆盖整个表，分区表将覆盖分区）。

## 常见问题

| 现象 | 可能原因 | 处理 |
| --- | --- | --- |
| `GetFlightInfo` 返回 not found | DomainData 不存在或未授权 | 检查 DomainData ID、DomainDataGrant 与当前身份 |
| 连接 Endpoint 失败 | 返回的是 DataProxy 地址，客户端无法直达 | 确认网络与证书；必要时通过 DataProxy 所在节点访问 |
| TLS 握手失败 | 未使用 Kuscia CA / 客户端证书 | 配置 `ca.crt`、`client.crt`、`client.key` |
| `DoGet` 读到空数据 | DomainData 路径错误或文件为空 | 检查 DomainData 的 `relative_uri` 与实际文件 |
| 写入后数据被覆盖 | DataMesh 默认覆盖写 | 使用独立输出路径/表，或先备份目标数据 |
| 分区创建失败 | 分区字段类型或值不匹配 | 检查输出分区配置与目标表结构 |

## 相关文档

- [DataMesh 概览](./summary_cn.md)
- [DomainData 参考](../../concepts/domaindata_cn.md)
- [DomainDataGrant 参考](../../concepts/domaindatagrant_cn.md)
- [Kuscia 数据代理](../../../数据代理.md)
- [DataMesh 数据读写示例代码](https://github.com/secretflow/kuscia/tree/main/cmd/example/datamesh)
