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

package confloader

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/secretflow/kuscia/pkg/agent/config"
	"github.com/secretflow/kuscia/pkg/common"
	cmconfig "github.com/secretflow/kuscia/pkg/confmanager/config"
	dmconfig "github.com/secretflow/kuscia/pkg/datamesh/config"
	kaconfig "github.com/secretflow/kuscia/pkg/kusciaapi/config"
	"github.com/secretflow/kuscia/pkg/utils/nlog"
	tlsutils "github.com/secretflow/kuscia/pkg/utils/tls"
)

// ============================================================================
// Lite节点配置结构
// 用于解析和存储Lite节点的完整配置信息
// ============================================================================
type LiteKusciaConfig struct {
	CommonConfig      `yaml:",inline"`                       // 嵌入通用配置（mode、domainID、domainKeyData等）
	LiteDeployToken   string                      `yaml:"liteDeployToken"`   // Lite节点部署令牌：用于向Master节点注册时的身份认证凭证，由Master颁发，一次性使用
	MasterEndpoint    string                      `yaml:"masterEndpoint"`    // Master节点端点地址：Lite节点连接的Master服务URL，格式为https://<ip>:<port>
	Runtime           string                      `yaml:"runtime"`           // 容器运行时类型：runc（本地容器）、runk（外部K8s）、runp（进程模式）
	Runk              RunkConfig                  `yaml:"runk"`              // Runk运行时配置：仅在runtime=runk时生效，配置外部K8s集群连接信息
	Capacity          config.CapacityCfg          `yaml:"capacity"`          // 资源容量配置：定义节点可用于调度的CPU、内存、Pod数量等资源上限
	ReservedResources config.ReservedResourcesCfg `yaml:"reservedResources"` // 预留资源配置：为系统组件预留的CPU、内存、带宽资源，不参与任务调度
	Image             ImageConfig                 `yaml:"image"`             // 镜像管理配置：控制容器镜像的拉取策略、仓库地址、代理设置等
	AdvancedConfig    `yaml:",inline"`            // 嵌入高级配置（KusciaAPI、ConfManager、DataMesh等）
	GarbageCollection GarbageCollectionConfig `yaml:"garbageCollection,omitempty"` // 垃圾回收配置：控制过期资源的自动清理策略
}

// ============================================================================
// Master节点配置结构
// 用于解析和存储Master节点的完整配置信息
// ============================================================================
type MasterKusciaConfig struct {
	CommonConfig      `yaml:",inline"`            // 嵌入通用配置
	DatastoreEndpoint string `yaml:"datastoreEndpoint"` // 数据存储端点：数据库连接字符串，用于持久化CRD资源和状态信息
	ClusterToken      string `yaml:"clusterToken,omitempty"` // 集群令牌：用于Master节点间的安全通信认证（可选）
	AdvancedConfig    `yaml:",inline"`            // 嵌入高级配置
	GarbageCollection GarbageCollectionConfig `yaml:"garbageCollection,omitempty"` // 垃圾回收配置
}

// ============================================================================
// Autonomy节点配置结构
// 用于解析和存储Autonomy节点的完整配置信息
// Autonomy节点兼具Master和Lite的功能，因此配置包含两者特性
// ============================================================================
type AutonomyKusciaConfig struct {
	CommonConfig      `yaml:",inline"`            // 嵌入通用配置
	Runtime           string                      `yaml:"runtime"`           // 容器运行时类型
	Runk              RunkConfig                  `yaml:"runk"`              // Runk运行时配置
	Capacity          config.CapacityCfg          `yaml:"capacity"`          // 资源容量配置
	ReservedResources config.ReservedResourcesCfg `yaml:"reservedResources"` // 预留资源配置
	Image             ImageConfig                 `yaml:"image"`             // 镜像管理配置
	DatastoreEndpoint string                      `yaml:"datastoreEndpoint"` // 数据存储端点
	AdvancedConfig    `yaml:",inline"`            // 嵌入高级配置
	GarbageCollection GarbageCollectionConfig `yaml:"garbageCollection,omitempty"` // 垃圾回收配置
}

// ============================================================================
// Runk运行时配置结构
// 用于配置对接外部Kubernetes集群的参数
// 仅在 runtime=runk 时生效
// ============================================================================
type RunkConfig struct {
	Namespace      string   `yaml:"namespace"`      // K8s命名空间：任务Pod将被调度到此命名空间中
	DNSServers     []string `yaml:"dnsServers"`     // DNS服务器列表：外部K8s集群的DNS服务器地址
	KubeconfigFile string   `yaml:"kubeconfigFile"` // Kubeconfig文件路径：访问外部K8s集群的认证凭证文件
	EnableLogging  bool     `yaml:"enableLogging"`  // 是否启用日志记录：控制是否收集容器日志
	LogDirectory   string   `yaml:"logDirectory"`   // 日志目录：容器日志存储路径
	LogMaxFiles    int      `yaml:"logMaxFiles"`    // 最大日志文件数：每个容器保留的最大日志文件数量
	LogMaxSize     string   `yaml:"logMaxSize"`     // 单个日志文件最大大小：支持单位Ki/Mi/Gi，如"100Mi"
}

// ============================================================================
// 覆盖K8s Provider配置
// 功能：将RunkConfig中的配置合并到K8sProviderCfg中
// 参数：k8sCfg - 原始K8s配置
// 返回：合并后的K8s配置
// ============================================================================
func (runk RunkConfig) overwriteK8sProviderCfg(k8sCfg config.K8sProviderCfg) config.K8sProviderCfg {
	k8sCfg.Namespace = runk.Namespace
	k8sCfg.KubeconfigFile = runk.KubeconfigFile
	k8sCfg.DNS.Servers = runk.DNSServers
	k8sCfg.EnableLogging = runk.EnableLogging
	k8sCfg.LogDirectory = runk.LogDirectory
	k8sCfg.LogMaxFiles = runk.LogMaxFiles
	k8sCfg.LogMaxSize = runk.LogMaxSize
	return k8sCfg
}

// ============================================================================
// 镜像管理配置结构
// 控制容器镜像的拉取行为和仓库配置
// ============================================================================
type ImageConfig struct {
	PullPolicy      string          `yaml:"pullPolicy"`      // 镜像拉取策略：Always（总是拉取）、IfNotPresent（不存在时拉取）、Never（从不拉取）
	DefaultRegistry string          `yaml:"defaultRegistry"` // 默认镜像仓库：当镜像名称未指定仓库时使用此地址
	Registries      []ImageRegistry `yaml:"registries"`      // 私有仓库列表：配置需要认证的私有镜像仓库
	HTTPProxy       string          `yaml:"httpProxy"`       // HTTP代理地址：用于加速镜像拉取的代理服务，如http://127.0.0.1:8080
}

// ============================================================================
// 镜像仓库配置结构
// 用于配置私有镜像仓库的访问凭证
// ============================================================================
type ImageRegistry struct {
	Name     string `yaml:"name"`     // 仓库名称：标识符，用于区分不同的仓库配置
	Endpoint string `yaml:"endpoint"` // 仓库地址：镜像仓库的访问URL
	UserName string `yaml:"username"` // 用户名：认证凭据
	Password string `yaml:"password"` // 密码：认证凭据
}

// ============================================================================
// 通用配置结构
// 所有节点类型共享的基础配置项
// ============================================================================
type CommonConfig struct {
	Mode          string          `yaml:"mode"`                    // 部署模式：master/lite/autonomy，决定节点角色
	DomainID      string          `yaml:"domainID"`                // 域ID：当前节点的唯一标识符，用于区分不同参与方
	DomainKeyData string          `yaml:"domainKeyData"`           // 域私钥（Base64编码）：用于节点间安全通信和身份认证的RSA私钥
	LogLevel      string          `yaml:"logLevel"`                // 日志级别：DEBUG（调试）、INFO（信息）、WARN（警告）、ERROR（错误）
	Protocol      common.Protocol `yaml:"protocol,omitempty"`      // 通信协议：NOTLS（无加密）、TLS（单向加密）、MTLS（双向加密）
}

// ============================================================================
// 日志轮转配置结构
// 控制日志文件的滚动和清理策略
// ============================================================================
type LogrotateConfig struct {
	MaxFiles      int `yaml:"maxFiles"`      // 最大文件数：保留的日志文件数量上限
	MaxFileSizeMB int `yaml:"maxFileSizeMB"` // 单文件最大大小（MB）：单个日志文件达到此大小后触发轮转
	MaxAgeDays    int `yaml:"maxAgeDays"`    // 最大保留天数（天）：超过此时长的日志文件将被删除
}

// ============================================================================
// 高级配置结构
// 包含各个子系统的详细配置选项
// ============================================================================
type AdvancedConfig struct {
	KusciaAPI             *kaconfig.KusciaAPIConfig   `yaml:"kusciaAPI,omitempty"`             // KusciaAPI配置：管理API服务的端口、证书、SANs等
	ConfManager           *cmconfig.ConfManagerConfig `yaml:"confManager,omitempty"`           // 配置管理器配置：密钥后端存储配置
	DataMesh              *dmconfig.DataMeshConfig    `yaml:"dataMesh,omitempty"`              // DataMesh配置：数据网格服务端口、数据代理列表等
	DomainRoute           DomainRouteConfig           `yaml:"domainRoute,omitempty"`           // 域路由配置：外部TLS设置、CSR数据等
	Agent                 config.AgentConfig          `yaml:"agent,omitempty"`                 // Agent配置：容器运行时、插件、资源限制等
	Debug                 bool                        `yaml:"debug,omitempty"`                 // 调试模式：是否启用pprof性能分析
	DebugPort             int                         `yaml:"debugPort,omitempty"`             // 调试端口：pprof服务监听端口（默认28080）
	EnableWorkloadApprove bool                        `yaml:"enableWorkloadApprove,omitempty"` // 工作负载审批：是否启用任务执行前的手动审批流程
	Logrotate             LogrotateConfig             `yaml:"logrotate,omitempty"`             // 日志轮转配置：全局日志滚动策略
	KusciaAPISans         []string                    `yaml:"kusciaAPISans,omitempty"`         // KusciaAPI SANs：证书 Subject Alternative Names 列表
}

// ============================================================================
// 加载通用配置
// 参数：configFile - 配置文件路径
// 返回：CommonConfig指针和错误信息
// 功能：从YAML文件中读取mode、domainID、domainKeyData等基础配置
// ============================================================================
func LoadCommonConfig(configFile string) (*CommonConfig, error) {

	conf := &CommonConfig{}
	err := loadConfig(configFile, conf)
	return conf, err
}

// ============================================================================
// 加载Lite节点配置
// 参数：configFile - 配置文件路径
// 返回：LiteKusciaConfig指针和错误信息
// 功能：解析Lite节点的完整配置，包括Master端点、部署令牌等
// ============================================================================
func LoadLiteConfig(configFile string) (*LiteKusciaConfig, error) {

	conf := &LiteKusciaConfig{}
	err := loadConfig(configFile, conf)
	return conf, err
}

// ============================================================================
// 加载Master节点配置
// 参数：configFile - 配置文件路径
// 返回：MasterKusciaConfig指针和错误信息
// 功能：解析Master节点的完整配置，包括数据存储端点、集群令牌等
// ============================================================================
func LoadMasterConfig(configFile string) (*MasterKusciaConfig, error) {

	conf := &MasterKusciaConfig{}
	err := loadConfig(configFile, conf)
	return conf, err
}

// ============================================================================
// 加载Autonomy节点配置
// 参数：configFile - 配置文件路径
// 返回：AutonomyKusciaConfig指针和错误信息
// 功能：解析Autonomy节点的完整配置，兼具Master和Lite的特性
// ============================================================================
func LoadAutonomyConfig(configFile string) (*AutonomyKusciaConfig, error) {

	conf := &AutonomyKusciaConfig{}
	err := loadConfig(configFile, conf)
	return conf, err
}

// ============================================================================
// Lite配置覆盖KusciaConfig
// 功能：将LiteKusciaConfig中的配置项合并到统一的KusciaConfig结构中
// 参数：kusciaConfig - 目标KusciaConfig指针，将被修改
// 流程：
//   1. 复制基础配置（LogLevel、DomainID、密钥等）
//   2. 配置KusciaAPI和SANs
//   3. 设置协议类型和ConfManager
//   4. 配置Agent运行时（Runtime、Runk、Capacity等）
//   5. 处理日志轮转配置（优先级：CRI > Advanced > Global）
//   6. 配置预留资源（CPU、Memory、Bandwidth）
//   7. 合并插件配置
//   8. 设置Master端点和CSR数据
//   9. 配置调试模式和镜像管理
//  10. 配置垃圾回收策略
// ============================================================================
func (lite *LiteKusciaConfig) OverwriteKusciaConfig(kusciaConfig *KusciaConfig) {
	// 复制基础配置
	kusciaConfig.LogLevel = lite.LogLevel
	kusciaConfig.DomainID = lite.DomainID
	kusciaConfig.CAKeyData = lite.DomainKeyData  // CA私钥（用于签发证书）
	kusciaConfig.DomainKeyData = lite.DomainKeyData  // 域私钥（用于身份认证）
	
	// 配置KusciaAPI
	if lite.KusciaAPI != nil {
		kusciaConfig.KusciaAPI = lite.KusciaAPI
	}
	if lite.KusciaAPISans != nil {
		kusciaConfig.KusciaAPI.SANs = lite.KusciaAPISans  // 设置证书SANs
	}
	kusciaConfig.Protocol = lite.Protocol  // 通信协议
	kusciaConfig.ConfManager = lite.ConfManager  // 配置管理器
	kusciaConfig.DataMesh = lite.DataMesh  // DataMesh服务
	// 配置Agent特权模式和运行时
	kusciaConfig.Agent.AllowPrivileged = lite.Agent.AllowPrivileged  // 是否允许特权容器
	kusciaConfig.Agent.Provider.Runtime = lite.Runtime  // 容器运行时类型
	kusciaConfig.Agent.Provider.K8s = lite.Runk.overwriteK8sProviderCfg(lite.Agent.Provider.K8s)  // 合并Runk配置
	
	// 覆盖Runk日志轮转配置（优先级处理）
	// LogMaxFiles必须>1，否则使用备用值
	if kusciaConfig.Agent.Provider.K8s.LogMaxFiles <= 1 {
		if lite.AdvancedConfig.Logrotate.MaxFiles > 1 {
			// 优先使用AdvancedConfig中的配置
			kusciaConfig.Agent.Provider.K8s.LogMaxFiles = lite.AdvancedConfig.Logrotate.MaxFiles
		} else {
			// 回退到全局Logrotate配置
			kusciaConfig.Agent.Provider.K8s.LogMaxFiles = kusciaConfig.Logrotate.MaxFiles
		}
	}
	
	// LogMaxSize必须是有效的数量格式（如"100Mi"）
	if _, parseErr := parseMaxSize(kusciaConfig.Agent.Provider.K8s.LogMaxSize); parseErr != nil {
		if lite.AdvancedConfig.Logrotate.MaxFileSizeMB > 0 {
			// 优先使用AdvancedConfig中的配置
			kusciaConfig.Agent.Provider.K8s.LogMaxSize = fmt.Sprintf("%dMi", lite.AdvancedConfig.Logrotate.MaxFileSizeMB)
		} else {
			// 回退到全局Logrotate配置
			kusciaConfig.Agent.Provider.K8s.LogMaxSize = fmt.Sprintf("%dMi", kusciaConfig.Logrotate.MaxFileSizeMB)
		}
	}
	// 配置资源容量
	kusciaConfig.Agent.Capacity = lite.Capacity  // 可调度资源上限
	
	// 如果使用runk运行时且配置了日志目录，设置标准输出路径
	if kusciaConfig.Agent.Provider.Runtime == config.K8sRuntime && kusciaConfig.Agent.Provider.K8s.LogDirectory != "" {
		kusciaConfig.Agent.StdoutPath = kusciaConfig.Agent.Provider.K8s.LogDirectory
	}

	// 配置预留资源（仅当配置了非空值时才覆盖）
	if lite.ReservedResources.CPU != "" {
		kusciaConfig.Agent.ReservedResources.CPU = lite.ReservedResources.CPU  // 预留CPU
	}
	if lite.ReservedResources.Memory != "" {
		kusciaConfig.Agent.ReservedResources.Memory = lite.ReservedResources.Memory  // 预留内存
	}
	if lite.ReservedResources.Bandwidth != "" {
		kusciaConfig.Agent.ReservedResources.Bandwidth = lite.ReservedResources.Bandwidth  // 预留带宽
	}

	// 合并插件配置（按名称匹配替换）
	for _, p := range lite.Agent.Plugins {
		for j, pp := range kusciaConfig.Agent.Plugins {
			if p.Name == pp.Name {
				kusciaConfig.Agent.Plugins[j] = p  // 找到同名插件则替换
				break
			}
		}
	}

	// 配置Master连接信息
	kusciaConfig.Master.Endpoint = lite.MasterEndpoint  // Master端点地址
	
	// 生成CSR数据（用于向Master注册时提交证书签名请求）
	kusciaConfig.DomainRoute.DomainCsrData = GenerateCsrData(lite.DomainID, lite.DomainKeyData, lite.LiteDeployToken)
	
	// 配置调试模式
	kusciaConfig.Debug = lite.Debug
	kusciaConfig.DebugPort = lite.DebugPort
	
	// 配置镜像管理
	kusciaConfig.Image = lite.Image
	kusciaConfig.Image.HTTPProxy = lite.Image.HTTPProxy  // HTTP代理
	
	// 配置垃圾回收
	kusciaConfig.GarbageCollection = lite.GarbageCollection

	// 覆盖日志轮转配置
	overwriteKusciaConfigLogrotate(&kusciaConfig.Logrotate, &lite.AdvancedConfig.Logrotate)
	// 根据日志保留天数计算标准输出GC周期
	kusciaConfig.Agent.StdoutGCDuration = time.Duration(kusciaConfig.Logrotate.MaxAgeDays) * 24 * time.Hour
	// 覆盖Agent CRI日志轮转配置
	overwriteKusciaConfigAgentLogrotate(&kusciaConfig.Agent.Provider.CRI, &lite.Agent.Provider.CRI, &kusciaConfig.Logrotate)
}

// ============================================================================
// Master配置覆盖KusciaConfig
// 功能：将MasterKusciaConfig中的配置项合并到统一的KusciaConfig结构中
// 参数：kusciaConfig - 目标KusciaConfig指针，将被修改
// 流程：
//   1. 复制基础配置（DomainID、LogLevel、密钥等）
//   2. 配置KusciaAPI和SANs
//   3. 设置协议类型和外部TLS
//   4. 配置数据存储端点和集群令牌
//   5. 配置调试模式和工作负载审批
//   6. 配置日志轮转和垃圾回收
// ============================================================================
func (master *MasterKusciaConfig) OverwriteKusciaConfig(kusciaConfig *KusciaConfig) {
	// 复制基础配置
	kusciaConfig.DomainID = master.DomainID
	kusciaConfig.LogLevel = master.LogLevel
	kusciaConfig.CAKeyData = master.DomainKeyData  // CA私钥
	kusciaConfig.DomainKeyData = master.DomainKeyData  // 域私钥
	
	// 配置KusciaAPI
	if master.KusciaAPI != nil {
		kusciaConfig.KusciaAPI = master.KusciaAPI
	}
	if master.KusciaAPISans != nil {
		kusciaConfig.KusciaAPI.SANs = master.KusciaAPISans
	}
	kusciaConfig.Protocol = master.Protocol  // 通信协议
	
	// 配置外部TLS（用于跨域通信）
	if master.DomainRoute.ExternalTLS != nil {
		kusciaConfig.DomainRoute.ExternalTLS = master.DomainRoute.ExternalTLS
	}
	
	// 配置Master特有项
	kusciaConfig.Master.DatastoreEndpoint = master.DatastoreEndpoint  // 数据库连接字符串
	kusciaConfig.Master.ClusterToken = master.ClusterToken  // 集群令牌
	
	// 配置调试和审批
	kusciaConfig.Debug = master.Debug
	kusciaConfig.DebugPort = master.DebugPort
	kusciaConfig.EnableWorkloadApprove = master.AdvancedConfig.EnableWorkloadApprove  // 工作负载审批开关
	kusciaConfig.GarbageCollection = master.GarbageCollection  // 垃圾回收配置

	// 覆盖日志轮转配置
	overwriteKusciaConfigLogrotate(&kusciaConfig.Logrotate, &master.AdvancedConfig.Logrotate)
}

// ============================================================================
// Autonomy配置覆盖KusciaConfig
// 功能：将AutonomyKusciaConfig中的配置项合并到统一的KusciaConfig结构中
// Autonomy节点兼具Master和Lite的功能，因此配置最全面
// 参数：kusciaConfig - 目标KusciaConfig指针，将被修改
// 流程：与Lite类似，但额外包含Master的数据存储配置
// ============================================================================
func (autonomy *AutonomyKusciaConfig) OverwriteKusciaConfig(kusciaConfig *KusciaConfig) {
	// 复制基础配置
	kusciaConfig.LogLevel = autonomy.LogLevel
	kusciaConfig.DomainID = autonomy.DomainID
	kusciaConfig.CAKeyData = autonomy.DomainKeyData
	kusciaConfig.DomainKeyData = autonomy.DomainKeyData
	
	// 配置Agent运行时
	kusciaConfig.Agent.AllowPrivileged = autonomy.Agent.AllowPrivileged
	kusciaConfig.Agent.Provider.Runtime = autonomy.Runtime
	kusciaConfig.Agent.Provider.K8s = autonomy.Runk.overwriteK8sProviderCfg(autonomy.Agent.Provider.K8s)
	
	// 覆盖Runk日志轮转配置（优先级处理）
	// maxFile必须>1，maxFileSizeMB必须可解析
	if kusciaConfig.Agent.Provider.K8s.LogMaxFiles <= 1 {
		if autonomy.AdvancedConfig.Logrotate.MaxFiles > 1 {
			kusciaConfig.Agent.Provider.K8s.LogMaxFiles = autonomy.AdvancedConfig.Logrotate.MaxFiles
		} else {
			// runk日志轮转配置没有默认初始化值，使用kusciaConfig的logrotate配置作为最终备用值
			kusciaConfig.Agent.Provider.K8s.LogMaxFiles = kusciaConfig.Logrotate.MaxFiles
		}
	}
	
	// 如果logMaxSize无效，考虑使用继承值
	if _, parseErr := parseMaxSize(kusciaConfig.Agent.Provider.K8s.LogMaxSize); parseErr != nil {
		if autonomy.AdvancedConfig.Logrotate.MaxFileSizeMB > 0 {
			kusciaConfig.Agent.Provider.K8s.LogMaxSize = fmt.Sprintf("%dMi", autonomy.AdvancedConfig.Logrotate.MaxFileSizeMB)
		} else {
			kusciaConfig.Agent.Provider.K8s.LogMaxSize = fmt.Sprintf("%dMi", kusciaConfig.Logrotate.MaxFileSizeMB)
		}
	}
	
	// 配置资源容量
	kusciaConfig.Agent.Capacity = autonomy.Capacity
	if kusciaConfig.Agent.Provider.Runtime == config.K8sRuntime && kusciaConfig.Agent.Provider.K8s.LogDirectory != "" {
		kusciaConfig.Agent.StdoutPath = kusciaConfig.Agent.Provider.K8s.LogDirectory
	}
	
	// 配置预留资源
	if autonomy.ReservedResources.CPU != "" {
		kusciaConfig.Agent.ReservedResources.CPU = autonomy.ReservedResources.CPU
	}
	if autonomy.ReservedResources.Memory != "" {
		kusciaConfig.Agent.ReservedResources.Memory = autonomy.ReservedResources.Memory
	}
	if autonomy.ReservedResources.Bandwidth != "" {
		kusciaConfig.Agent.ReservedResources.Bandwidth = autonomy.ReservedResources.Bandwidth
	}

	// 合并插件配置
	for _, p := range autonomy.Agent.Plugins {
		for j, pp := range kusciaConfig.Agent.Plugins {
			if p.Name == pp.Name {
				kusciaConfig.Agent.Plugins[j] = p
				break
			}
		}
	}

	// 配置KusciaAPI
	if autonomy.KusciaAPI != nil {
		kusciaConfig.KusciaAPI = autonomy.KusciaAPI
	}
	if autonomy.KusciaAPISans != nil {
		kusciaConfig.KusciaAPI.SANs = autonomy.KusciaAPISans
	}
	kusciaConfig.Protocol = autonomy.Protocol
	kusciaConfig.ConfManager = autonomy.ConfManager
	kusciaConfig.DataMesh = autonomy.DataMesh
	
	// 配置外部TLS
	if autonomy.DomainRoute.ExternalTLS != nil {
		kusciaConfig.DomainRoute.ExternalTLS = autonomy.DomainRoute.ExternalTLS
	}
	
	// 配置Master数据存储
	kusciaConfig.Master.DatastoreEndpoint = autonomy.DatastoreEndpoint
	
	// 配置调试和审批
	kusciaConfig.Debug = autonomy.Debug
	kusciaConfig.DebugPort = autonomy.DebugPort
	kusciaConfig.EnableWorkloadApprove = autonomy.AdvancedConfig.EnableWorkloadApprove
	
	// 配置镜像管理
	kusciaConfig.Image = autonomy.Image
	kusciaConfig.Image.HTTPProxy = autonomy.Image.HTTPProxy
	kusciaConfig.GarbageCollection = autonomy.GarbageCollection

	// 覆盖日志轮转配置
	overwriteKusciaConfigLogrotate(&kusciaConfig.Logrotate, &autonomy.AdvancedConfig.Logrotate)
	kusciaConfig.Agent.StdoutGCDuration = time.Duration(kusciaConfig.Logrotate.MaxAgeDays) * 24 * time.Hour
	overwriteKusciaConfigAgentLogrotate(&kusciaConfig.Agent.Provider.CRI, &autonomy.Agent.Provider.CRI, &kusciaConfig.Logrotate)
}

// ============================================================================
// 覆盖Agent日志轮转配置
// 功能：尝试用kuscia yaml中的logrotate配置覆盖应用（如SecretFlow）的默认日志轮转配置
// 参数：
//   kusciaAgentConfig - Agent CRI配置（将被修改）
//   overwriteAgentLogrotate - CRI级别的日志轮转配置（最高优先级）
//   overwriteLogrotate - 全局日志轮转配置（次高优先级）
// 优先级：CRI配置 > Advanced配置 > Global配置 > 默认值
// ============================================================================
func overwriteKusciaConfigAgentLogrotate(kusciaAgentConfig, overwriteAgentLogrotate *config.CRIProviderCfg, overwriteLogrotate *LogrotateConfig) {
	// kusciaAgentConfig有默认初始值，所以只需要尝试覆盖
	// 优先使用CRI级别的日志轮转配置
	// maxFile必须>1，maxFileSizeMB必须可解析
	if overwriteAgentLogrotate != nil && overwriteAgentLogrotate.ContainerLogMaxFiles > 1 {
		// 最高优先级：CRI配置
		kusciaAgentConfig.ContainerLogMaxFiles = overwriteAgentLogrotate.ContainerLogMaxFiles
	} else if overwriteLogrotate != nil && overwriteLogrotate.MaxFiles > 1 {
		// 次高优先级：全局配置
		kusciaAgentConfig.ContainerLogMaxFiles = overwriteLogrotate.MaxFiles
	}
	
	// 处理ContainerLogMaxSize
	if overwriteAgentLogrotate != nil {
		if _, parseErr := parseMaxSize(overwriteAgentLogrotate.ContainerLogMaxSize); parseErr == nil {
			// CRI配置有效，直接使用
			kusciaAgentConfig.ContainerLogMaxSize = overwriteAgentLogrotate.ContainerLogMaxSize
			return
		}
	}
	if overwriteLogrotate != nil && overwriteLogrotate.MaxFileSizeMB > 0 {
		// 回退到全局配置
		kusciaAgentConfig.ContainerLogMaxSize = fmt.Sprintf("%dMi", overwriteLogrotate.MaxFileSizeMB)
	}

	// 如果仍然无效，使用默认值
	if kusciaAgentConfig.ContainerLogMaxFiles <= 0 {
		kusciaAgentConfig.ContainerLogMaxFiles = config.DefaultLogRotateMaxFiles
	}

	if kusciaAgentConfig.ContainerLogMaxSize == "" {
		kusciaAgentConfig.ContainerLogMaxSize = config.DefaultLogRotateMaxSizeStr
	}
}

// ============================================================================
// 覆盖Kuscia日志轮转配置
// 功能：尝试用kuscia yaml中的logrotate配置覆盖默认配置
// 参数：
//   kusciaConfig - Kuscia全局日志配置（将被修改）
//   overwriteLogrotate - YAML中的日志轮转配置
// ============================================================================
func overwriteKusciaConfigLogrotate(kusciaConfig, overwriteLogrotate *LogrotateConfig) {
	if overwriteLogrotate != nil {
		if overwriteLogrotate.MaxFileSizeMB > 0 {
			kusciaConfig.MaxFileSizeMB = overwriteLogrotate.MaxFileSizeMB  // 单文件最大大小
		}
		if overwriteLogrotate.MaxFiles > 0 {
			kusciaConfig.MaxFiles = overwriteLogrotate.MaxFiles  // 最大文件数
		}
		if overwriteLogrotate.MaxAgeDays > 0 {
			kusciaConfig.MaxAgeDays = overwriteLogrotate.MaxAgeDays  // 最大保留天数
		}
	}
}

// ============================================================================
// 加载配置文件
// 参数：
//   configFile - YAML配置文件路径
//   conf - 目标配置结构指针（任意类型）
// 返回：错误信息
// 功能：读取YAML文件并反序列化到配置结构中
// ============================================================================
func loadConfig(configFile string, conf interface{}) error {
	content, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}
	if err = yaml.Unmarshal(content, conf); err != nil {
		return err
	}
	return nil
}

// ============================================================================
// 生成CSR（证书签名请求）数据
// 参数：
//   domainID - 域ID（作为Common Name）
//   domainKeyData - Base64编码的域私钥
//   deployToken - 部署令牌（嵌入到CSR扩展字段中）
// 返回：PEM格式的CSR字符串
// 功能：
//   1. 解码Base64私钥
//   2. 解析RSA私钥
//   3. 创建CSR模板，包含域ID和部署令牌
//   4. 生成CSR并编码为PEM格式
// 用途：Lite节点向Master注册时提交此CSR以获取证书
// ============================================================================
func GenerateCsrData(domainID, domainKeyData, deployToken string) string {
	if domainKeyData == "" {
		nlog.Fatalf("Domain key data is empty. Please provide a valid domainKeyData.")
	}
	
	// 解码Base64编码的私钥
	domainKeyDataDecoded, err := base64.StdEncoding.DecodeString(domainKeyData)
	if err != nil {
		nlog.Fatalf("Load domain key file error: %v", err.Error())
	}

	// 解析RSA私钥
	key, err := tlsutils.ParseKey(domainKeyDataDecoded, "")
	if err != nil {
		nlog.Fatalf("Decode domain key data error: %v", err.Error())
	}

	// 解析扩展ID（1.2.3.4）
	extensionIDs := strings.Split(common.DomainCsrExtensionID, ".")
	var asn1Id asn1.ObjectIdentifier
	for _, str := range extensionIDs {
		id, convErr := strconv.Atoi(str)
		if convErr != nil {
			nlog.Fatalf("Parse extension ID error: %v", convErr.Error())
		}
		asn1Id = append(asn1Id, id)
	}

	// 创建CSR模板
	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: domainID,  // 使用域ID作为通用名称
		},
		SignatureAlgorithm: x509.SHA256WithRSA,  // 签名算法
		ExtraExtensions: []pkix.Extension{
			{
				Id:    asn1Id,           // 自定义扩展ID
				Value: []byte(deployToken),  // 部署令牌作为扩展值
			},
		},
	}

	// 生成CSR
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, key)
	if err != nil {
		nlog.Fatalf("Create x509 certificate request error: %v", err)
	}

	// 编码为PEM格式
	var reader bytes.Buffer
	if err := pem.Encode(&reader, &pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes}); err != nil {
		nlog.Fatalf("Encode certificate request error: %v", err.Error())
	}
	return reader.String()
}

// ============================================================================
// 解析最大日志大小
// 参数：size - 大小字符串（支持Kubernetes数量格式，如"100Mi"、"1Gi"）
// 返回：字节数和错误信息
// 功能：将人类可读的大小字符串转换为int64字节数
// 来源：复制自kubelet/log/container_log_manager
// ============================================================================
func parseMaxSize(size string) (int64, error) {
	quantity, err := resource.ParseQuantity(size)
	if err != nil {
		return 0, err
	}
	maxSize, ok := quantity.AsInt64()
	if !ok {
		return 0, fmt.Errorf("invalid max log size")
	}
	return maxSize, nil
}
