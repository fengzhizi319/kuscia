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

// Package start 定义了Kuscia的启动命令和启动逻辑
// 该包负责解析配置、初始化模块管理器、注册各个功能模块，并启动Kuscia实例
package start

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"  // Cobra库，用于构建现代CLI应用程序

	"github.com/secretflow/kuscia/cmd/kuscia/conflistener"  // 配置文件监听器，用于热重载配置
	"github.com/secretflow/kuscia/cmd/kuscia/confloader"   // 配置加载器，用于加载和解析配置文件
	"github.com/secretflow/kuscia/cmd/kuscia/modules"      // 模块管理器，用于管理Kuscia的各种功能模块
	"github.com/secretflow/kuscia/cmd/kuscia/utils"        // 工具函数库，提供各种实用工具
	"github.com/secretflow/kuscia/pkg/agent/config"        // 代理配置相关
	"github.com/secretflow/kuscia/pkg/common"              // 通用常量和类型定义
	"github.com/secretflow/kuscia/pkg/utils/nlog"          // 日志工具
	"github.com/secretflow/kuscia/pkg/utils/runtime"       // 运行时工具，用于检查运行时权限
)

// startConfig 启动配置结构体，包含启动所需的配置参数
type startConfig struct {
	configFile        string  // 配置文件路径
	logLevelHotReload bool    // 是否启用日志级别热重载
}

// NewStartCommand 创建启动命令
// 该函数定义了start子命令的结构、参数和执行逻辑
func NewStartCommand(ctx context.Context) *cobra.Command {

	// 初始化启动配置结构体
	startConfigs := startConfig{}

	// 创建Cobra命令对象，定义命令的基本属性
	cmd := &cobra.Command{
		// Use: 定义命令的使用方式
		Use: "start",
		// Short: 简短的命令描述
		Short: "Start means running Kuscia",
		// Long: 详细的命令描述
		Long: `Start Kuscia with multi-mode from config file, Lite, Master or Autonomy`,
		// SilenceUsage: 设置为true可禁止在出错时自动打印使用说明
		SilenceUsage: true,
		// RunE: 定义命令的执行逻辑
		RunE: func(cmd *cobra.Command, args []string) error {

			// 检查是否启用了日志级别热重载功能
			if startConfigs.logLevelHotReload {
				// 启动配置文件监听器，监控配置文件变更并热重载日志级别
				conflistener.LogLevelHotReloadListener(ctx, startConfigs.configFile)
				nlog.Infof("Kuscia hot reload is enabled, config file path: %s", startConfigs.configFile)
			}

			// 调用Start函数启动Kuscia实例
			return Start(ctx, startConfigs.configFile)
		},
	}
	// 定义命令行参数：配置文件路径
	// -c 或 --config 指定配置文件路径，默认为 "etc/config/kuscia.yaml"
	cmd.Flags().StringVarP(&startConfigs.configFile, "config", "c", "etc/config/kuscia.yaml", "load config from file.")

	// 定义命令行参数：是否启用日志级别热重载
	// -l 或 --loglevel-hot-reload 启用配置更新功能
	// Must be enabled with -l=false or --loglevel-hot-reload=true. See: https://github.com/spf13/cobra/issues/1657
	cmd.Flags().BoolVarP(&startConfigs.logLevelHotReload, "loglevel-hot-reload", "l", true, "enable kuscia configuration update, eg '-d=false'")

	// 返回创建的命令对象
	return cmd
}

// Start 启动Kuscia实例的主函数
// 该函数负责加载配置、初始化模块管理器、注册各功能模块、设置依赖关系，并最终启动所有模块
func Start(ctx context.Context, configFile string) error {

	// 1. 加载通用配置
	// 从配置文件中解析出通用配置信息，包括域名、运行模式等
	commonConfig, err := confloader.LoadCommonConfig(configFile)
	if err != nil {
		// 如果配置加载失败，记录致命错误并退出
		nlog.Fatalf("Load kuscia common config failed. %v", err)
	}

	// 检查DomainID是否合法，不能是不支持的值
	if commonConfig.DomainID == common.UnSupportedDomainID {
		nlog.Fatalf("Domain id can't be 'master', please check input config file(%s)", configFile)
	}

	// 2. 加载完整配置
	// 从配置文件中加载完整的Kuscia配置信息
	kusciaConf, err := confloader.ReadConfig(configFile)
	if err != nil {
		nlog.Fatalf("Load kuscia config failed. %v", err.Error())
	}
	// 3. 创建模块运行时配置
	// 根据加载的配置创建模块管理器所需的运行时配置
	conf := modules.NewModuleRuntimeConfigs(ctx, kusciaConf)
	// 在函数结束时关闭配置资源
	defer conf.Close()

	// 4. 检查运行时权限
	// 如果使用容器运行时且没有特权模式，记录错误并返回
	if conf.Agent.Provider.Runtime == config.ContainerRuntime && !runtime.Permission.HasPrivileged() {
		nlog.Errorf("Runc must run with privileged mode")
		nlog.Errorf("Please run kuscia like: docker run --privileged secretflow/kuscia")
		return errors.New("permission is error")
	}

	// 5. 设置性能分析工具
	// 如果启用了调试模式，设置pprof性能分析工具
	utils.SetupPprof(conf.Debug, conf.DebugPort)
	// 检查是否具有设置OOM分数的权限，如果有则设置
	if runtime.Permission.HasSetOOMScorePermission() {
		modules.SetKusciaOOMScore()
	}

	// 6. 定义运行模式常量
	// 定义三种运行模式：Master、Lite、Autonomy
	master, lite, autonomy := common.RunModeMaster, common.RunModeLite, common.RunModeAutonomy

	// 7. 创建模块管理器
	// 初始化模块管理器，用于管理Kuscia的所有功能模块
	mm := NewModuleManager()
	// 8. 注册各功能模块及其支持的运行模式
	// 根据不同运行模式注册对应的模块
	mm.Regist("coredns", modules.NewCoreDNS, autonomy, lite, master)  // CoreDNS模块，支持所有模式
	mm.Regist("k3s", modules.NewK3s, autonomy, master)              // K3s模块，仅支持Autonomy和Master模式
	mm.Regist("agent", modules.NewAgent, autonomy, lite)             // Agent模块，支持Autonomy和Lite模式
	mm.Regist("envoy", modules.NewEnvoy, autonomy, lite, master)     // Envoy模块，支持所有模式
	// 如果启用了containerd，则注册containerd模块
	if conf.EnableContainerd {
		mm.Regist("containerd", modules.NewContainerd, autonomy, lite)
	}

	// 继续注册其他模块
	mm.Regist("config", modules.NewConfManager, autonomy, lite, master)        // 配置管理模块
	mm.Regist("controllers", modules.NewControllersModule, autonomy, master)   // 控制器模块
	mm.Regist("datamesh", modules.NewDataMesh, autonomy, lite)                 // 数据网格模块
	mm.Regist("domainroute", modules.NewDomainRoute, autonomy, master, lite)   // 域路由模块
	mm.Regist("interconn", modules.NewInterConn, autonomy, master)             // 互联模块
	mm.Regist("kusciaapi", modules.NewKusciaAPI, autonomy, lite, master)      // Kuscia API模块
	mm.Regist("metricexporter", modules.NewMetricExporter, autonomy, lite, master)  // 指标导出模块
	mm.Regist("nodeexporter", modules.NewNodeExporter, autonomy, lite, master)      // 节点指标导出模块
	mm.Regist("ssexporter", modules.NewSsExporter, autonomy, lite, master)         // 系统状态导出模块
	mm.Regist("scheduler", modules.NewScheduler, autonomy, master)                 // 调度器模块
	mm.Regist("transport", modules.NewTransport, autonomy, lite)                   // 传输模块
	mm.Regist("reporter", modules.NewReporter, autonomy, master)                   // 报告模块
	mm.Regist("diagnose", modules.NewDiagnose, autonomy, lite, master)             // 诊断模块

	// 9. 设置模块间的依赖关系
	// 定义各模块之间的依赖关系，确保模块按正确顺序启动
	if conf.EnableContainerd {
		// 如果启用了containerd，agent依赖containerd
		mm.SetDependencies("agent", "envoy", "k3s", "kusciaapi", "containerd")
	} else {
		// 否则agent不依赖containerd
		mm.SetDependencies("agent", "envoy", "k3s", "kusciaapi")
	}
	mm.SetDependencies("envoy", "k3s")                    // envoy依赖k3s
	mm.SetDependencies("controllers", "k3s")              // controllers依赖k3s
	mm.SetDependencies("config", "k3s", "envoy", "domainroute", "controllers")  // config依赖多个模块
	mm.SetDependencies("datamesh", "k3s", "config", "envoy", "domainroute")    // datamesh依赖多个模块
	mm.SetDependencies("domainroute", "k3s")              // domainroute依赖k3s
	mm.SetDependencies("interconn", "k3s")                // interconn依赖k3s
	mm.SetDependencies("kusciaapi", "k3s", "config", "domainroute")           // kusciaapi依赖多个模块
	mm.SetDependencies("scheduler", "k3s")                // scheduler依赖k3s
	mm.SetDependencies("ssexporter", "envoy")             // ssexporter依赖envoy
	mm.SetDependencies("metricexporter", "agent", "envoy", "ssexporter", "nodeexporter")  // metricexporter依赖多个模块
	mm.SetDependencies("transport", "envoy")              // transport依赖envoy
	mm.SetDependencies("k3s", "coredns")                  // k3s依赖coredns
	mm.SetDependencies("reporter", "k3s", "kusciaapi")    // reporter依赖k3s和kusciaapi
	mm.SetDependencies("diagnose", "k3s")                 // diagnose依赖k3s

	// 10. 添加就绪钩子
	// 定义在特定模块就绪后执行的钩子函数
	mm.AddReadyHook(func(ctx context.Context, mdls map[string]modules.Module) error {
		nlog.Info("Start... coredns controllers")  // 记录启动CoreDNS控制器的日志
		// 获取coredns模块实例
		cdsModule, ok := mdls["coredns"].(*modules.CorednsModule)
		// 如果模块存在且类型正确，启动CoreDNS控制器
		if ok && cdsModule != nil {
			// 使用Kubernetes客户端启动CoreDNS控制器
			cdsModule.StartControllers(ctx, conf.Clients.KubeClient)
			return nil  // 返回nil表示执行成功
		}
		// 如果模块类型不正确，返回错误
		return errors.New("coredns module type is invalid")
	}, "k3s", "coredns", "envoy", "domainroute")  // 指定钩子函数在这些模块就绪后执行

	// 11. 启动模块管理器
	// 根据配置的运行模式启动所有注册的模块
	// 将运行模式转换为小写并与模块管理器配置匹配
	err = mm.Start(ctx, strings.ToLower(commonConfig.Mode), conf)

	// 12. 记录关闭日志
	// 在Kuscia实例关闭时记录日志
	nlog.Infof("Kuscia Instance [%s] shut down", commonConfig.DomainID)
	// 返回启动过程中可能出现的错误
	return err
}
