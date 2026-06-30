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

//nolint:dupl
package main

import (
	"fmt"
	"os"

	_ "github.com/coredns/caddy/onevent"  // 导入CoreDNS事件插件，用于处理事件
	_ "github.com/coredns/coredns/plugin/acl"  // 导入ACL插件，用于访问控制
	_ "github.com/coredns/coredns/plugin/any"  // 导入ANY插件，处理ANY查询
	_ "github.com/coredns/coredns/plugin/auto"  // 导入AUTO插件，用于自动配置
	_ "github.com/coredns/coredns/plugin/autopath"  // 导入AUTOPATH插件，自动解析路径
	_ "github.com/coredns/coredns/plugin/bind"  // 导入BIND插件，处理BIND相关功能
	_ "github.com/coredns/coredns/plugin/bufsize"  // 导入BUFSIZE插件，设置缓冲区大小
	_ "github.com/coredns/coredns/plugin/cache"  // 导入CACHE插件，提供DNS缓存功能
	_ "github.com/coredns/coredns/plugin/cancel"  // 导入CANCEL插件，取消查询
	_ "github.com/coredns/coredns/plugin/chaos"  // 导入CHAOS插件，处理CHAOSt查询
	_ "github.com/coredns/coredns/plugin/debug"  // 导入DEBUG插件，提供调试功能
	_ "github.com/coredns/coredns/plugin/dnssec"  // 导入DNSSEC插件，处理DNS安全扩展
	_ "github.com/coredns/coredns/plugin/dnstap"  // 导入DNSTAP插件，提供DNS数据输出
	_ "github.com/coredns/coredns/plugin/erratic"  // 导入ERRATIC插件，模拟错误行为
	_ "github.com/coredns/coredns/plugin/errors"  // 导入ERRORS插件，处理错误
	_ "github.com/coredns/coredns/plugin/file"  // 导入FILE插件，从文件读取数据
	_ "github.com/coredns/coredns/plugin/forward"  // 导入FORWARD插件，转发DNS查询
	_ "github.com/coredns/coredns/plugin/health"  // 导入HEALTH插件，健康检查
	_ "github.com/coredns/coredns/plugin/hosts"  // 导入HOSTS插件，处理hosts文件
	_ "github.com/coredns/coredns/plugin/loadbalance"  // 导入LOADBALANCE插件，负载均衡
	_ "github.com/coredns/coredns/plugin/log"  // 导入LOG插件，日志记录
	_ "github.com/coredns/coredns/plugin/loop"  // 导入LOOP插件，循环检测
	_ "github.com/coredns/coredns/plugin/metadata"  // 导入METADATA插件，提供元数据
	_ "github.com/coredns/coredns/plugin/metrics"  // 导入METRICS插件，提供指标
	_ "github.com/coredns/coredns/plugin/nsid"  // 导入NSID插件，提供名称服务器ID
	_ "github.com/coredns/coredns/plugin/pprof"  // 导入PPROF插件，性能分析
	_ "github.com/coredns/coredns/plugin/ready"  // 导入READY插件，准备状态检查
	_ "github.com/coredns/coredns/plugin/reload"  // 导入RELOAD插件，热重载
	_ "github.com/coredns/coredns/plugin/rewrite"  // 导入REWRITE插件，查询重写
	_ "github.com/coredns/coredns/plugin/root"  // 导入ROOT插件，根服务器配置
	_ "github.com/coredns/coredns/plugin/route53"  // 导入ROUTE53插件，AWS Route53集成
	_ "github.com/coredns/coredns/plugin/secondary"  // 导入SECONDARY插件，辅助服务器
	_ "github.com/coredns/coredns/plugin/sign"  // 导入SIGN插件，DNS记录签名
	_ "github.com/coredns/coredns/plugin/template"  // 导入TEMPLATE插件，模板处理
	_ "github.com/coredns/coredns/plugin/transfer"  // 导入TRANSFER插件，区域传输
	_ "github.com/coredns/coredns/plugin/whoami"  // 导入WHOAMI插件，返回客户端信息
	"github.com/spf13/cobra"  // Cobra库，用于构建现代CLI应用程序
	"github.com/spf13/pflag"  // POSIX风格的命令行标志解析库
	kubectlcmd "k8s.io/kubectl/pkg/cmd"  // Kubernetes kubectl命令实现

	"github.com/secretflow/kuscia/cmd/kuscia/container"  // Kuscia容器管理子命令
	"github.com/secretflow/kuscia/cmd/kuscia/diagnose"   // Kuscia诊断子命令
	"github.com/secretflow/kuscia/cmd/kuscia/image"      // Kuscia镜像管理子命令
	"github.com/secretflow/kuscia/cmd/kuscia/kusciainit"  // Kuscia初始化子命令
	"github.com/secretflow/kuscia/cmd/kuscia/start"      // Kuscia启动子命令
	_ "github.com/secretflow/kuscia/pkg/agent/middleware/plugins"  // Kuscia代理中间件插件
	"github.com/secretflow/kuscia/pkg/utils/meta"         // Kuscia元数据工具
	"github.com/secretflow/kuscia/pkg/utils/signals"      // Kuscia信号处理工具
)

// main.go 是 Kuscia 项目的主入口文件，定义了根命令和多个子命令
// 使用 cobra 库处理命令行参数

// main 函数是整个 Kuscia 应用程序的入口点
// 运行逻辑：
// 1. 创建根命令对象，配置基本信息（名称、版本、帮助文本等）
// 2. 清理可能冲突的全局标志
// 3. 初始化信号处理上下文，用于优雅处理系统信号
// 4. 注册所有可用的子命令
// 5. 执行根命令，等待用户输入并处理
func main() {
	// 创建根命令对象，配置命令的基本属性
	rootCmd := &cobra.Command{
		// Use: 命令名称
		Use: "kuscia",
		// Long: 命令详细描述
		Long: `kuscia is a root cmd, please select subcommand you want`,
		// Version: 版本信息，从meta包获取
		Version: meta.KusciaVersionString(),
		// CompletionOptions: 配置命令自动补全选项，禁用默认命令
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		// RunE: 执行函数，当用户直接运行根命令而不带子命令时的处理逻辑
		RunE: func(cmd *cobra.Command, args []string) error {
			// 当用户只运行 kuscia 命令而没有指定子命令时，显示帮助信息
			return cmd.Help()
		},
	}
	// 由于调度器在Kuscia进程初始化时可能会注册版本标志，这里清除全局命令行标志以避免冲突
	pflag.CommandLine = nil
	// 创建信号处理上下文，设置信号处理器来捕获系统信号（如SIGTERM、SIGINT）
	// 这样可以确保程序能够优雅地关闭并释放资源
	ctx := signals.NewKusciaContextWithStopCh(signals.SetupSignalHandler())
	// 向根命令注册 image 子命令，用于镜像管理功能
	rootCmd.AddCommand(image.NewImageCommand(ctx))
	// 向根命令注册 container 子命令，用于容器管理功能
	rootCmd.AddCommand(container.NewContainerCommand(ctx))
	// 向根命令注册 start 子命令，这是最核心的命令，用于启动 Kuscia 实例
	rootCmd.AddCommand(start.NewStartCommand(ctx))
	// 向根命令注册 diagnose 子命令，用于诊断和故障排除
	rootCmd.AddCommand(diagnose.NewDiagnoseCommand(ctx))
	// 向根命令注册 kusciainit 子命令，用于初始化 Kuscia 环境
	rootCmd.AddCommand(kusciainit.NewInitCommand(ctx))
	// 向根命令注册 kubectl 命令，提供原生 kubectl 功能
	rootCmd.AddCommand(kubectlcmd.NewDefaultKubectlCommand())
	// 向根命令注册 kernel-check 子命令，用于检查内核参数是否满足要求
	rootCmd.AddCommand(NewKernelCheckCommand(ctx))
	// 执行根命令，这会解析命令行参数并执行相应的子命令
	// 如果执行过程中出现错误，则打印错误信息并以错误码退出
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
