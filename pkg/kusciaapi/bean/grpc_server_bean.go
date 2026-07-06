// Copyright 2023 Ant Group Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package bean 实现了 KusciaAPI 模块中各类 Bean（HTTP Server、gRPC Server、Controller 等）的初始化与生命周期管理。
//
// 本文件负责创建并启动 KusciaAPI 的 gRPC 服务端，是所有外部/内部 gRPC 请求的统一入口。
// 它注册的服务包括：JobService、DomainService、DomainRouteService、DomainDataService、
// DomainDataSourceService、DomainDataGrantService、ServingService、CertificateService、
// ConfigService、AppImageService、LogService、HealthService 等。
package bean

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	"github.com/secretflow/kuscia/pkg/common"
	cmservice "github.com/secretflow/kuscia/pkg/confmanager/service"
	"github.com/secretflow/kuscia/pkg/kusciaapi/config"
	"github.com/secretflow/kuscia/pkg/kusciaapi/handler/grpchandler"
	"github.com/secretflow/kuscia/pkg/kusciaapi/service"
	"github.com/secretflow/kuscia/pkg/kusciaapi/utils"
	"github.com/secretflow/kuscia/pkg/utils/nlog"
	tlsutil "github.com/secretflow/kuscia/pkg/utils/tls"
	"github.com/secretflow/kuscia/pkg/web/errorcode"
	"github.com/secretflow/kuscia/pkg/web/framework"
	frameworkconfig "github.com/secretflow/kuscia/pkg/web/framework/config"
	"github.com/secretflow/kuscia/pkg/web/interceptor"
	pberrorcode "github.com/secretflow/kuscia/proto/api/v1alpha1/errorcode"
	"github.com/secretflow/kuscia/proto/api/v1alpha1/kusciaapi"
)

// grpcServerBean 是 KusciaAPI gRPC 服务端的 Bean 实现。
//
// 它实现了 framework.ConfBeanRegistry 中定义的生命周期接口（Validate / Init / Start / ServerName），
// 由框架在启动时统一调度。
type grpcServerBean struct {
	// config 保存 KusciaAPI 的全局配置，包括监听端口、TLS/Token/协议、拦截器日志等。
	config *config.KusciaAPIConfig

	// cmConfigService 是 ConfigManager 的配置服务，部分服务（如 DomainData、DomainDataSource）
	// 在创建 Service 实现时需要依赖它来获取/管理配置信息。
	cmConfigService cmservice.IConfigService
}

// NewGrpcServerBean 创建一个新的 gRPC Server Bean 实例。
//
// 参数说明：
//   - config：KusciaAPI 配置对象，包含端口、TLS、Token、协议等。
//   - cmConfigService：ConfigManager 配置服务接口。
//
// 返回的 grpcServerBean 只做了字段赋值，真正的 gRPC Server 创建和启动在 Start 方法中完成。
func NewGrpcServerBean(config *config.KusciaAPIConfig, cmConfigService cmservice.IConfigService) *grpcServerBean { // nolint: golint
	return &grpcServerBean{
		config:          config,
		cmConfigService: cmConfigService,
	}
}

// Validate 用于校验 Bean 配置是否合法。
//
// 当前 gRPC Server Bean 没有需要额外校验的字段，因此为空实现。
// 框架会在启动阶段调用所有 Bean 的 Validate 方法。
func (s *grpcServerBean) Validate(errs *errorcode.Errs) {
}

// Init 用于在 Bean 启动前完成初始化工作。
//
// 当前 gRPC Server Bean 没有需要提前初始化的资源，因此直接返回 nil。
// 真正的监听和注册都在 Start 中完成。
func (s *grpcServerBean) Init(e framework.ConfBeanRegistry) error {
	return nil
}

// Start 创建并启动 KusciaAPI gRPC 服务端。
//
// 执行流程：
//  1. 初始化 gRPC ServerOption：连接超时、panic 恢复拦截器、最大接收消息大小（256MB）。
//  2. 如果配置了 TLS，根据协议类型（TLS / mTLS）构建服务端 TLS 配置并注入到 ServerOption。
//  3. 监听配置中指定的 gRPC 端口（默认 8083）。
//  4. 依次添加日志拦截器、Token 认证拦截器（如果启用 Token 认证）、Master 角色拦截器。
//  5. 创建 grpc.Server 并注册所有 KusciaAPI gRPC 服务及其 Handler。
//  6. 注册 gRPC reflection 服务，便于 grpcurl 等工具调试。
//  7. 阻塞调用 server.Serve(lis) 开始对外提供服务。
//
// 注意：该方法会一直阻塞，直到服务停止或发生致命错误。
func (s *grpcServerBean) Start(ctx context.Context, e framework.ConfBeanRegistry) error {
	// 基础 ServerOption：
	// - ConnectionTimeout：连接建立超时时间
	// - ChainUnaryInterceptor / StreamInterceptor：为 unary 和 stream 请求都加上 panic 恢复拦截器，
	//   避免单个请求 panic 导致整个 gRPC Server 崩溃。
	// - MaxRecvMsgSize：最大接收消息大小设置为 256MB，以支持较大的任务配置或 DistData 元数据。
	opts := []grpc.ServerOption{
		grpc.ConnectionTimeout(time.Duration(s.config.ConnectTimeout) * time.Second),
		grpc.ChainUnaryInterceptor(interceptor.UnaryRecoverInterceptor(pberrorcode.ErrorCode_KusciaAPIErrForUnexpected)),
		grpc.StreamInterceptor(interceptor.StreamRecoverInterceptor(pberrorcode.ErrorCode_KusciaAPIErrForUnexpected)),
		grpc.MaxRecvMsgSize(256 * 1024 * 1024), // 256MB
	}

	// 如果启用了 TLS，则构建服务端 TLS 配置并加入 ServerOption。
	// protocol 可以是 common.TLS 或 common.MTLS：
	// - TLS：仅服务端出示证书，客户端可选校验（典型外部 API 场景）。
	// - mTLS：双向 TLS，客户端也必须提供证书并由 RootCA 校验（高安全场景）。
	if s.config.TLS != nil {
		serverTLSConfig, err := buildServerTLSConfig(s.config.TLS, s.config.Protocol)
		if err != nil {
			nlog.Fatalf("Failed to init server tls config: %v", err)
		}
		creds := credentials.NewTLS(serverTLSConfig)
		opts = append(opts, grpc.Creds(creds))
	}

	// 根据配置中的 GRPCPort 监听 TCP 端口，默认 8083。
	addr := fmt.Sprintf(":%d", s.config.GRPCPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		nlog.Fatalf("failed to listen on addr[%s]: %v", addr, err)
	}

	// 添加 gRPC 请求日志拦截器，记录请求/响应信息用于调试和审计。
	opts = append(opts, grpc.ChainUnaryInterceptor(interceptor.GrpcServerLoggingInterceptor(*s.config.InterceptorLog)))

	// 如果配置了 Token 认证，读取 token 文件内容，并为 unary 和 stream 请求分别添加 Token 校验拦截器。
	// 未通过 Token 校验的请求会被直接拒绝。
	tokenConfig := s.config.Token
	if s.config.Token != nil {
		token, err := utils.ReadToken(*tokenConfig)
		if err != nil {
			return err
		}
		tokenInterceptor := grpc.ChainUnaryInterceptor(interceptor.GrpcServerTokenInterceptor(token))
		opts = append(opts, tokenInterceptor)
		tokenStreamInterceptor := grpc.ChainStreamInterceptor(interceptor.GrpcStreamServerTokenInterceptor(token))
		opts = append(opts, tokenStreamInterceptor)
	}

	// 添加 Master 角色拦截器，用于在 autonomy / master 模式下限制某些 API 只能由主节点调用。
	opts = append(opts, grpc.ChainUnaryInterceptor(interceptor.GrpcServerMasterRoleInterceptor()))
	opts = append(opts, grpc.ChainStreamInterceptor(interceptor.GrpcStreamServerMasterRoleInterceptor()))

	// 使用前面组装好的 opts 创建 grpc.Server 实例，
	// 然后把 KusciaAPI 定义的各服务接口注册到该 Server 上。
	//
	// 注册关系：
	// - JobService：作业管理（创建/查询/停止/监听 Job）。
	// - DomainService：Domain（节点/命名空间）管理。
	// - DomainRouteService：跨域路由管理。
	// - HealthService：健康检查。
	// - DomainDataService：数据表（DomainData）CRUD。
	// - DomainDataSourceService：数据源（DomainDataSource）CRUD。
	// - ServingService：在线服务管理。
	// - DomainDataGrantService：跨域数据授权管理。
	// - CertificateService：证书管理。
	// - ConfigService：配置管理（依赖 cmConfigService）。
	// - AppImageService：应用镜像管理。
	// - LogService：日志查询。
	server := grpc.NewServer(opts...)
	kusciaapi.RegisterJobServiceServer(server, grpchandler.NewJobHandler(service.NewJobService(s.config)))
	kusciaapi.RegisterDomainServiceServer(server, grpchandler.NewDomainHandler(service.NewDomainService(s.config)))
	kusciaapi.RegisterDomainRouteServiceServer(server, grpchandler.NewDomainRouteHandler(service.NewDomainRouteService(s.config)))
	kusciaapi.RegisterHealthServiceServer(server, grpchandler.NewHealthHandler(service.NewHealthService()))
	kusciaapi.RegisterDomainDataServiceServer(server, grpchandler.NewDomainDataHandler(service.NewDomainDataService(s.config, s.cmConfigService)))
	kusciaapi.RegisterDomainDataSourceServiceServer(server, grpchandler.NewDomainDataSourceHandler(service.NewDomainDataSourceService(s.config, s.cmConfigService)))
	kusciaapi.RegisterServingServiceServer(server, grpchandler.NewServingHandler(service.NewServingService(s.config)))
	kusciaapi.RegisterDomainDataGrantServiceServer(server, grpchandler.NewDomainDataGrantHandler(service.NewDomainDataGrantService(s.config)))
	kusciaapi.RegisterCertificateServiceServer(server, grpchandler.NewCertificateHandler(newCertService(s.config)))
	kusciaapi.RegisterConfigServiceServer(server, grpchandler.NewConfigHandler(service.NewConfigService(s.config, s.cmConfigService)))
	kusciaapi.RegisterAppImageServiceServer(server, grpchandler.NewAppImageHandler(service.NewAppImageService(s.config)))
	kusciaapi.RegisterLogServiceServer(server, grpchandler.NewLogHandler(service.NewLogService(s.config)))

	// 注册 gRPC reflection 服务，方便使用 grpcurl、grpcui 等工具动态发现服务和方法。
	reflection.Register(server)
	nlog.Infof("grpc server listening on %s", addr)

	// 开始监听并处理 gRPC 请求。该方法阻塞，直到 server.Stop() 被调用或发生不可恢复的错误。
	return server.Serve(lis)
}

// ServerName 返回该 Bean 的名称，用于框架日志和调试。
func (s *grpcServerBean) ServerName() string {
	return "kusciaAPIGrpcServer"
}

// buildServerTLSConfig 根据 TLS 配置和通信协议构建 tls.Config。
//
// 参数说明：
//   - config：TLS 服务端配置，包含 RootCA、ServerCert、ServerKey 等。
//   - protocol：通信协议，common.MTLS 表示双向 TLS，其他表示单向 TLS。
//
// 逻辑说明：
//   - 当 protocol == common.MTLS 时，需要传入 RootCA 用于校验客户端证书，实现双向认证。
//   - 其他协议（如 common.TLS）只校验服务端证书，不强制校验客户端证书。
//
// 如果 config 为 nil，则返回错误。
func buildServerTLSConfig(config *frameworkconfig.TLSServerConfig, protocol common.Protocol) (*tls.Config, error) {

	if config == nil {
		return nil, fmt.Errorf("tls config is empty")
	}
	if protocol == common.MTLS {
		return tlsutil.BuildServerTLSConfig(config.RootCA, config.ServerCert, config.ServerKey)
	}
	return tlsutil.BuildServerTLSConfig(nil, config.ServerCert, config.ServerKey)
}
