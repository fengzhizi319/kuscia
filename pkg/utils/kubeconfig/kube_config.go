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

package kubeconfig

import (
	"fmt"
	"time"

	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	kusciaclientset "github.com/secretflow/kuscia/pkg/crd/clientset/versioned"
)

// KubeClients 包含多种 Kubernetes 客户端实例，用于与内置 K3s 集群进行交互
// 在 Kuscia 架构中，内置 K3s 作为本地 Kubernetes 实例运行，这些客户端用于管理资源
type KubeClients struct {
	// KubeClient 是标准 Kubernetes 资源的客户端，用于管理原生 Kubernetes 对象如 Pod、Service 等
	KubeClient kubernetes.Interface
	// KusciaClient 是 Kuscia 自定义资源的客户端，用于管理 Domain、DomainData 等自定义资源对象
	KusciaClient kusciaclientset.Interface
	// ExtensionsClient 是 API 扩展资源的客户端，主要用于管理 CRD（Custom Resource Definition）
	ExtensionsClient apiextensionsclientset.Interface
	// Kubeconfig 存储连接到 K3s 集群的配置信息
	Kubeconfig *restclient.Config
}

// CreateClientSets 根据给定的配置创建多个 Kubernetes 客户端实例
// 此函数用于初始化与内置 K3s 集群的连接，创建标准 Kubernetes 客户端、Kuscia 自定义资源客户端和 API 扩展客户端
func CreateClientSets(config *restclient.Config) (*KubeClients, error) {
	// 创建标准 Kubernetes 客户端，用于与内置 K3s 集群中的原生资源交互
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error building kubernetes client set, detail-> %v", err)
	}

	// 创建 Kuscia 自定义资源客户端，用于管理 Domain、DomainData 等特定于 Kuscia 的资源
	// 在内置 K3s 中，这些是 Kuscia 系统的核心资源类型
	kusciaClient, err := kusciaclientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error building domain client set, detail-> %v", err)
	}

	// 创建 API 扩展客户端，用于管理 CRD（Custom Resource Definition）等扩展资源
	// 在 Kuscia 架构中，此客户端用于处理自定义资源定义
	extensionClient, err := apiextensionsclientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error building apiextensions kubernetes client set, detail-> %v", err)
	}
	return &KubeClients{
		KubeClient:       kubeClient,
		KusciaClient:     kusciaClient,
		ExtensionsClient: extensionClient,
		Kubeconfig:       config,
	}, nil
}

// CreateClientSetsFromToken 使用提供的认证令牌和主服务器 URL 创建 Kubernetes 客户端集合
// 此函数允许通过令牌认证连接到内置 K3s 集群，适用于需要基于令牌的身份验证场景
func CreateClientSetsFromToken(token, masterURL string) (*KubeClients, error) {
	// 从令牌和主服务器 URL 构建客户端配置，这是连接到内置 K3s 的第一步
	kubeConfig, err := BuildClientConfigFromToken(token, masterURL)
	if err != nil {
		return nil, fmt.Errorf("error building kubernetes client config from token, detail-> %v", err)
	}
	// 使用构建好的配置创建完整的客户端集合
	return CreateClientSets(kubeConfig)
}

// CreateClientSetsFromKubeconfig 从指定的 kubeconfig 文件路径创建 Kubernetes 客户端集合
// 此函数使用默认的性能参数连接到内置 K3s 集群，适用于常规的 Kuscia 运行时环境
func CreateClientSetsFromKubeconfig(kubeconfigPath, masterURL string) (*KubeClients, error) {
	// 使用默认的 QPS(500)、Burst(1000) 和无超时设置连接到内置 K3s 集群
	return CreateClientSetsFromKubeconfigWithOptions(kubeconfigPath, masterURL, 500, 1000, 0)
}

// CreateClientSetsFromKubeconfigWithOptions 从指定的 kubeconfig 文件创建 Kubernetes 客户端集合，并支持性能参数配置
// 此函数允许用户指定 QPS、Burst 和超时等参数，以优化与内置 K3s 集群的通信性能
func CreateClientSetsFromKubeconfigWithOptions(kubeconfigPath, masterURL string, QPS float32, Burst int, Timeout time.Duration) (*KubeClients, error) {
	// 从 kubeconfig 文件和主服务器 URL 构建客户端配置，这是连接到内置 K3s 的关键步骤
	kubeConfig, err := BuildClientConfigFromKubeconfig(kubeconfigPath, masterURL)
	if err != nil {
		return nil, fmt.Errorf("error building kubernetes client config from token, detail-> %v", err)
	}
	// 如果指定了非零的 QPS（每秒查询数），则更新配置中的 QPS 参数
	// 这可以控制与内置 K3s API 服务器的通信频率
	if QPS != 0 {
		kubeConfig.QPS = QPS
	}
	// 如果指定了非零的突发请求量，则更新配置中的 Burst 参数
	// 这允许在短时间内发送更多请求，提高性能
	if Burst != 0 {
		kubeConfig.Burst = Burst
	}
	// 如果指定了非零超时时间，则更新配置中的超时参数
	if Timeout != 0 {
		kubeConfig.Timeout = Timeout
	}
	// 使用最终的配置创建客户端集合
	return CreateClientSets(kubeConfig)
}

// CreateKubeClientFromKubeconfigWithOptions 从指定的 kubeconfig 文件创建标准 Kubernetes 客户端，并支持性能参数配置
// 此函数专门创建标准 Kubernetes 客户端，不包括 Kuscia 自定义资源客户端，适用于仅需访问原生 Kubernetes 资源的场景
func CreateKubeClientFromKubeconfigWithOptions(kubeconfigPath, masterURL string, QPS float32, Burst int, Timeout time.Duration) (kubernetes.Interface, error) {
	// 从 kubeconfig 文件和主服务器 URL 构建客户端配置，这是连接到内置 K3s 的第一步
	kubeConfig, err := BuildClientConfigFromKubeconfig(kubeconfigPath, masterURL)
	if err != nil {
		return nil, fmt.Errorf("error building kubernetes client config from token, detail-> %v", err)
	}
	// 如果指定了非零的 QPS（每秒查询数），则更新配置中的 QPS 参数
	// 这可以控制与内置 K3s API 服务器的通信频率
	if QPS != 0 {
		kubeConfig.QPS = QPS
	}
	// 如果指定了非零的突发请求量，则更新配置中的 Burst 参数
	// 这允许在短时间内发送更多请求，提高性能
	if Burst != 0 {
		kubeConfig.Burst = Burst
	}
	// 如果指定了非零超时时间，则更新配置中的超时参数
	if Timeout != 0 {
		kubeConfig.Timeout = Timeout
	}
	// 使用配置创建标准 Kubernetes 客户端
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("error building kubernetes client set, detail-> %v", err)
	}

	return kubeClient, nil
}

// BuildClientConfigFromKubeconfig 从指定的 kubeconfig 文件和主服务器 URL 构建客户端配置
// 此函数用于从 kubeconfig 文件中解析并构建与内置 K3s 集群通信所需的配置对象
// 在 Kuscia 架构中，内置 K3s 作为轻量级 Kubernetes 实现运行在本地，此函数负责建立与它的连接配置
func BuildClientConfigFromKubeconfig(kubeconfigPath, masterURL string) (*restclient.Config, error) {
	// 使用 clientcmd.BuildConfigFromFlags 从主服务器 URL 和 kubeconfig 文件路径构建配置
	// 这是连接到内置 K3s 集群的关键步骤，它会解析 kubeconfig 文件中的认证和集群信息
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("build config from flags failed, detail-> %v", err)
	}

	return cfg, nil
}

// BuildClientConfigFromToken 使用提供的认证令牌和主服务器 URL 构建客户端配置
// 此函数用于通过令牌认证的方式连接到内置 K3s 集群，通常在自动化或服务间通信场景中使用
// 在 Kuscia 架构中，当需要程序化地与内置 K3s 交互时，这种认证方式非常有用
func BuildClientConfigFromToken(token, masterURL string) (*restclient.Config, error) {
	// 使用 clientcmd.NewNonInteractiveDeferredLoadingClientConfig 创建非交互式客户端配置
	// 通过 ConfigOverrides 设置认证令牌和集群信息，InsecureSkipTLSVerify=true 表示跳过 TLS 验证
	// 这在本地开发或内部服务通信中常用，但在生产环境中应谨慎使用
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{},
		&clientcmd.ConfigOverrides{
			AuthInfo:    clientcmdapi.AuthInfo{Token: token},
			ClusterInfo: clientcmdapi.Cluster{Server: masterURL, InsecureSkipTLSVerify: true},
		}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build config from token failed, detail-> %v", err)
	}
	// 显式设置承载令牌，确保配置正确包含认证信息
	cfg.BearerToken = token

	return cfg, nil
}
