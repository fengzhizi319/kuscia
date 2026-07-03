// Copyright 2024 Ant Group Co., Ltd.
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

package start

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/secretflow/kuscia/cmd/kuscia/modules"
	"github.com/secretflow/kuscia/pkg/common"
	"github.com/secretflow/kuscia/pkg/utils/lock"
	"github.com/secretflow/kuscia/pkg/utils/nlog"
)

const moduleDefaultExitTimeout = time.Second * 3

type ModuleReadyHook func(ctx context.Context, modules map[string]modules.Module) error
type ModuleManager interface {
	// regist a new module
	Regist(name string, creator NewModuleFunc, modes ...common.RunModeType) bool
	// set module dependency modules
	SetDependencies(name string, dependencies ...string) bool

	// when [modules] are all ready, hook will called
	AddReadyHook(hook ModuleReadyHook, modules ...string) bool

	// start to run all modules
	Start(ctx context.Context, mode common.RunModeType, conf *modules.ModuleRuntimeConfigs) error
}

type kusciaModuleReadyHook struct {
	// hook modules. when all modules are ready. hook function will called
	modules []string

	hook ModuleReadyHook

	isCalled bool

	// runtime context and cancel function
	ctx    context.Context
	cancel context.CancelFunc
}

type kusciaModuleManager struct {
	modules map[string]*moduleInfo

	readyHooks []*kusciaModuleReadyHook

	// runtime context and cancel function
	ctx    context.Context
	cancel context.CancelFunc

	// all modules had finished
	wg *sync.WaitGroup

	// newly created module use this chan to notify
	newlyModuleCh chan *moduleInfo

	// newly ready module use this chan to notify
	readyModuleCh chan *moduleInfo

	// run failed module use this chan to notify
	runFailedModuleCh chan *moduleInfo
}

func NewModuleManager() ModuleManager {
	return &kusciaModuleManager{
		modules:           map[string]*moduleInfo{},
		newlyModuleCh:     make(chan *moduleInfo, 100),
		readyModuleCh:     make(chan *moduleInfo, 100),
		runFailedModuleCh: make(chan *moduleInfo, 100),
		wg:                &sync.WaitGroup{},
	}
}

func (kmm *kusciaModuleManager) Regist(name string, creator NewModuleFunc, modes ...common.RunModeType) bool {
	if creator == nil {
		nlog.Errorf("module %s creator is nil", name)
		return false
	}

	if _, ok := kmm.modules[name]; ok {
		nlog.Errorf("module(%s) had exists", name)
		return false
	}

	mset := map[common.RunModeType]bool{}
	for _, m := range modes {
		mset[m] = true
	}

	kmm.modules[name] = &moduleInfo{
		name:     name,
		creator:  creator,
		modes:    mset,
		instance: nil,
	}
	return true
}

func (kmm *kusciaModuleManager) SetDependencies(name string, dependencies ...string) bool {
	if module, ok := kmm.modules[name]; ok {
		for _, d := range dependencies {
			if _, ok = kmm.modules[d]; !ok {
				nlog.Errorf("invalidate module %s", d)
				return false
			}
		}
		module.dependencies = append(module.dependencies, dependencies...)
		return true
	}
	nlog.Errorf("invalidate module %s", name)
	return false
}

func (kmm *kusciaModuleManager) AddReadyHook(hook ModuleReadyHook, modules ...string) bool {
	if hook == nil {
		nlog.Errorf("input hook function is nil")
		return false
	}
	for _, m := range modules {
		if _, ok := kmm.modules[m]; !ok {
			nlog.Errorf("invalidate module name %s", m)
			return false
		}
	}

	kmm.readyHooks = append(kmm.readyHooks, &kusciaModuleReadyHook{
		modules:  modules,
		hook:     hook,
		isCalled: false,
	})

	return true
}

// Start 启动所有注册的模块，按依赖关系顺序初始化并运行
//
// 执行流程：
// 1. 根据运行模式筛选需要启动的模块，构建依赖关系图
// 2. 启动无依赖的模块（立即可以运行的模块）
// 3. 进入主循环，监听模块就绪、失败、退出等事件
// 4. 当模块就绪后，检查是否有依赖该模块的其他模块可以启动
// 5. 触发已满足依赖条件的 Ready Hook 回调函数
// 6. 所有模块启动完成后，等待退出信号或模块失败
// 7. 收到退出信号时，执行优雅退出流程（按依赖逆序停止模块）
//
// 参数说明：
//   - ctx: 父级上下文，用于接收外部退出信号
//   - mode: 运行模式（master/lite/autonomy），决定启动哪些模块
//   - conf: 模块运行时配置，包含各种路径、端口、证书等信息
//
// 返回值：
//   - error: 启动失败或运行过程中出错时返回错误信息
func (kmm *kusciaModuleManager) Start(ctx context.Context, mode common.RunModeType, conf *modules.ModuleRuntimeConfigs) error {
	// 步骤1：根据运行模式筛选需要启动的模块，并构建反向依赖图
	// modules: 需要启动的模块集合（map[name]*moduleInfo）
	// moduleReverseDeps: 反向依赖图，key是模块名，value是依赖该模块的模块列表
	// 例如：如果 agent 依赖 k3s，则 reverseDep["k3s"] = ["agent"]
	modules, moduleReverseDeps := kmm.initNeedStartModules(mode)

	// 创建独立的上下文用于管理模块生命周期
	// 注意：不使用传入的 ctx 作为父上下文，避免父上下文取消时所有模块立即被取消
	// 而是使用 background context，通过 kmm.cancel() 手动控制模块的优雅退出
	kmm.ctx, kmm.cancel = context.WithCancel(context.Background())
	defer kmm.cancel()

	// 步骤2：启动所有无依赖的模块（即 dependencies 为空的模块）
	// 这些模块可以立即启动，不需要等待其他模块就绪
	for _, mc := range modules {
		if mc.isModuleDepReady(modules) {
			nlog.Infof("module(%s) no dep so start", mc.name)
			if err := kmm.runModule(kmm.ctx, mc, conf); err != nil {
				return err
			}
		}
	}

	// 步骤3：主事件循环，处理模块就绪、失败、退出等事件
	for {
		var readyModule *moduleInfo
		select {
		// 事件1：新模块创建完成，开始等待其就绪
		case newCreateModule := <-kmm.newlyModuleCh:
			// 在协程中等待模块就绪（WaitReady 会阻塞直到模块内部发出就绪信号）
			go func() {
				nlog.Infof("new created module: %v", newCreateModule.name)
				// WaitReady 会阻塞直到模块调用 ready 方法或超时
				newCreateModule.readyError = newCreateModule.instance.WaitReady(ctx)
				// 将就绪结果发送回主循环处理
				kmm.readyModuleCh <- newCreateModule
			}()

		// 事件2：模块就绪（WaitReady 返回）
		case readyModule = <-kmm.readyModuleCh:
			// 标记该模块的就绪等待已完成（只有主协程可以设置此标志）
			readyModule.isReadyWaitDone = true

			// 检查模块是否就绪失败
			if readyModule.readyError != nil {
				nlog.Errorf("[Module] %s wait ready failed with %s, so start gracefulExit", readyModule.name, readyModule.readyError.Error())
				// 模块就绪失败，触发优雅退出流程
				return kmm.gracefulExit(modules, moduleReverseDeps)
			}
			nlog.Infof("[Module] %s is ready now", readyModule.name)

		// 事件3：模块运行失败（Run 方法返回错误）
		case <-kmm.runFailedModuleCh:
			// 某个模块运行失败，触发优雅退出
			return kmm.gracefulExit(modules, moduleReverseDeps)

		// 事件4：接收到外部退出信号（如 Ctrl+C、SIGTERM）
		case <-ctx.Done():
			// 正常退出，执行优雅关闭流程
			nlog.Infof("Got process exit signal, so start gracefulExit")
			return kmm.gracefulExit(modules, moduleReverseDeps)
		}

		// 步骤4：检查并触发已满足条件的 Ready Hook 回调
		// Hook 会在其依赖的所有模块都就绪后被调用
		if err := kmm.callbackHookFunctions(modules); err != nil {
			nlog.Infof("Callback hook function, so start gracefulExit")
			// Hook 执行失败，也需要优雅退出
			return kmm.gracefulExit(modules, moduleReverseDeps)
		}

		// 如果没有模块就绪，继续等待下一个事件
		if readyModule == nil {
			continue
		}

		// 步骤5：检查是否有模块依赖当前就绪的模块，如果有且其所有依赖都已就绪，则启动该模块
		// 这是依赖驱动启动的核心逻辑
		for _, name := range moduleReverseDeps[readyModule.name] {
			mc := modules[name]
			// 只处理尚未启动的模块
			if mc.instance == nil {
				// 检查该模块的所有依赖是否都已就绪
				if mc.isModuleDepReady(modules) {
					// 所有依赖都已就绪，可以启动该模块
					if err := kmm.runModule(kmm.ctx, mc, conf); err != nil {
						return err
					}
				}
			}
		}

		// 步骤6：检查是否所有模块都已启动完成
		if kmm.isAllModulesStarted(modules) {
			break
		}
	}

	// 步骤7：所有模块启动成功，打印成功日志
	nlog.Info("[Module] all modules are startup")
	nlog.Info("++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++")
	nlog.Info("Kuscia started success")
	nlog.Info("++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++")

	// 步骤8：等待退出信号或模块运行结束
	select {
	// 情况1：接收到外部退出信号
	case <-ctx.Done():
		nlog.Infof("Got process exit signal, so start gracefulExit")
		return kmm.gracefulExit(modules, moduleReverseDeps)

	// 情况2：某个模块运行失败
	case <-kmm.runFailedModuleCh:
		return kmm.gracefulExit(modules, moduleReverseDeps)

	// 情况3：所有模块正常运行结束（后台 goroutine 全部完成）
	case <-lock.NewWaitGroupChannel(kmm.wg):
		nlog.Infof("All module are finished")
	}

	return nil
}

// callbackHookFunctions 检查并触发所有已满足条件的 Ready Hook 回调函数
//
// Hook 机制说明：
// - Hook 是一个回调函数，在其依赖的所有模块都就绪后被调用
// - 用于实现模块间的协调逻辑，例如：当 k3s 和 controllers 都就绪后，开始注册 CRD
// - 每个 Hook 只会被调用一次（通过 isCalled 标志保证）
// - 如果 Hook 执行失败，会触发整个系统的优雅退出
//
// 执行逻辑：
// 1. 遍历所有注册的 Hook
// 2. 跳过已经调用过的 Hook 或无效的 Hook
// 3. 检查 Hook 依赖的所有模块是否都已就绪（isReadyWaitDone == true）
// 4. 如果所有依赖模块都已就绪，则执行 Hook 回调函数
// 5. 标记 Hook 为已调用，并保存其上下文用于后续取消
//
// 参数：
//   - ms: 所有需要启动的模块映射
//
// 返回值：
//   - error: Hook 执行失败时返回错误，触发系统退出
func (kmm *kusciaModuleManager) callbackHookFunctions(ms map[string]*moduleInfo) error {
	for _, hf := range kmm.readyHooks {
		// 跳过已经调用过的 Hook 或未设置回调函数的 Hook
		if hf.isCalled || hf.hook == nil {
			continue
		}

		// 检查 Hook 依赖的所有模块是否都已就绪
		isAllReady := true
		depModules := map[string]modules.Module{}
		for _, dep := range hf.modules {
			if m, ok := ms[dep]; ok {
				// 收集依赖模块实例
				depModules[dep] = m.instance
				// 只要有一个模块未就绪，就标记为未就绪
				if !m.isReadyWaitDone {
					isAllReady = false
					break
				}
			}
		}

		// 如果所有依赖模块都已就绪，执行 Hook 回调
		if isAllReady {
			// 标记为已调用，确保不会被重复执行
			hf.isCalled = true

			// 创建 Hook 的独立上下文，用于后续取消
			hf.ctx, hf.cancel = context.WithCancel(kmm.ctx)

			// 执行 Hook 回调函数
			// 典型用途：CRD 注册、健康检查、服务发现等
			if err := hf.hook(hf.ctx, depModules); err != nil {
				nlog.Warnf("Hook function callback failed with %s", err.Error())
				return err
			}
		}

	}

	return nil
}

// cancelCallbacks 取消与即将退出的模块相关的 Hook 回调
//
// 用途：
// - 在优雅退出过程中，当某些模块准备退出时，需要取消依赖这些模块的 Hook
// - 防止 Hook 继续运行导致访问已停止的模块
//
// 执行逻辑：
// 1. 遍历所有已调用的 Hook
// 2. 检查 Hook 依赖的模块中是否有即将退出的模块
// 3. 如果有，则取消该 Hook 的上下文，停止其执行
// 4. 重置 isCalled 标志，允许 Hook 在下次启动时重新执行
//
// 参数：
//   - canExitModules: 可以退出的模块集合（key=模块名, value=是否已标记退出）
func (kmm *kusciaModuleManager) cancelCallbacks(canExitModules map[string]bool) {
	for _, hf := range kmm.readyHooks {
		// 只处理已经调用过的 Hook
		if !hf.isCalled {
			continue
		}

		// 检查 Hook 依赖的模块中是否有即将退出的
		isAllCanExit := false
		for _, dep := range hf.modules {
			if _, ok := canExitModules[dep]; ok {
				// 只要有一个依赖模块要退出，就取消该 Hook
				isAllCanExit = true
				break
			}
		}

		// 取消 Hook 的执行
		if isAllCanExit {
			// 重置标志，允许下次启动时重新调用
			hf.isCalled = false
			// 取消上下文，Hook 内部的 goroutine 会收到取消信号并退出
			hf.cancel()
		}
	}
}

// initNeedStartModules 根据运行模式筛选需要启动的模块，并构建依赖关系图
//
// 这是模块启动的核心预处理函数，负责：
// 1. 根据运行模式（master/lite/autonomy）过滤出需要启动的模块
// 2. 构建反向依赖图，用于后续的依赖驱动启动
//
// 执行逻辑：
// 步骤1：遍历所有注册的模块，检查当前运行模式是否在模块的支持模式列表中
//
//	如果是，则将该模块加入 startModules 集合
//
// 步骤2：初始化反向依赖图 reverseDep
//   - key: 模块名称
//   - value: 依赖该模块的模块列表
//   - 例如：如果 agent 依赖 k3s，则 reverseDep["k3s"] = ["agent"]
//
// 步骤3：遍历所有需要启动的模块，填充反向依赖关系
//
//	对于模块 A 的每个依赖 D，如果 D 也在 startModules 中，
//	则将 A 添加到 reverseDep[D] 的列表中
//
// 示例：
//
//	假设模块依赖关系：agent -> k3s, controllers -> k3s
//	则 reverseDep 为：
//	{
//	  "k3s": ["agent", "controllers"],
//	  "agent": [],
//	  "controllers": []
//	}
//
// 参数：
//   - mode: 运行模式（master/lite/autonomy）
//
// 返回值：
//   - map[string]*moduleInfo: 需要启动的模块集合
//   - map[string][]string: 反向依赖图
func (kmm *kusciaModuleManager) initNeedStartModules(mode common.RunModeType) (map[string]*moduleInfo, map[string][]string) {
	// 步骤1：筛选需要启动的模块
	startModules := map[string]*moduleInfo{}
	for _, mc := range kmm.modules {
		// 检查当前运行模式是否在模块的支持模式列表中
		if _, ok := mc.modes[mode]; ok {
			startModules[mc.name] = mc
		}
	}

	// 步骤2：初始化反向依赖图
	reverseDep := map[string][]string{}
	for _, mc := range startModules {
		// 为每个模块初始化一个空列表
		reverseDep[mc.name] = []string{}
	}

	// 步骤3：填充反向依赖关系
	for _, mc := range startModules {
		for _, dep := range mc.dependencies {
			// 只关心也在 startModules 中的依赖（跨模式的依赖忽略）
			if _, ok := startModules[dep]; ok {
				// 将当前模块添加到其依赖的反向依赖列表中
				reverseDep[dep] = append(reverseDep[dep], mc.name)
			}
		}
	}

	return startModules, reverseDep
}

func (kmm *kusciaModuleManager) isAllModulesStarted(modules map[string]*moduleInfo) bool {
	allModuleStarted := true
	for _, mc := range modules {
		if mc.instance == nil || !mc.isReadyWaitDone {
			allModuleStarted = false
		}
	}
	return allModuleStarted
}

// runModule 创建并启动单个模块
//
// 这是模块启动的核心函数，负责：
// 1. 调用模块的构造函数创建模块实例
// 2. 创建模块的独立上下文
// 3. 在后台 goroutine 中运行模块
// 4. 监控模块的就绪状态和运行状态
//
// 执行流程：
// 步骤1：调用模块构造函数（creator 函数）创建模块实例
//
//	如果创建失败，直接返回错误
//
// 步骤2：为模块创建独立的上下文（继承自 kmm.ctx）
//
//	用于控制模块的生命周期（启动、停止）
//
// 步骤3：将新创建的模块发送到 newlyModuleCh 通道
//
//	通知主循环开始等待模块就绪
//
// 步骤4：在后台 goroutine 中运行模块
//   - 调用模块的 Run 方法（通常会阻塞直到模块退出）
//   - 如果 Run 返回错误，发送失败通知到 runFailedModuleCh
//   - 如果 Run 正常退出，记录成功日志
//   - 运行结束后，减少 WaitGroup 计数
//
// 并发模型：
//   - 主协程：负责模块的创建、启动、依赖管理
//   - 模块协程：每个模块在自己的 goroutine 中运行
//   - 就绪等待协程：每个模块有一个协程专门等待其就绪
//
// 参数：
//   - ctx: 父级上下文（通常是 kmm.ctx）
//   - mc: 模块信息结构，包含模块名称、构造函数、依赖等
//   - conf: 模块运行时配置
//
// 返回值：
//   - error: 模块创建失败时返回错误
func (kmm *kusciaModuleManager) runModule(ctx context.Context, mc *moduleInfo, conf *modules.ModuleRuntimeConfigs) error {
	nlog.Infof("Try to start module %s", mc.name)

	// 步骤1：调用模块构造函数创建实例
	var err error
	mc.instance, err = mc.creator(conf)
	if err != nil {
		nlog.Errorf("[Module] %s init failed with error: %s", mc.name, err.Error())
		return err
	}
	nlog.Infof("[Module] %s is created", mc.name)

	// 步骤2：创建模块的独立上下文
	mc.ctx, mc.cancel = context.WithCancel(ctx)

	// 步骤3：通知主循环有新模块创建，需要等待其就绪
	kmm.newlyModuleCh <- mc

	// 步骤4：在后台 goroutine 中运行模块
	// 增加 WaitGroup 计数，用于等待所有模块运行结束
	kmm.wg.Add(1)
	// 增加 finishWG 计数，用于优雅退出时等待模块完全停止
	mc.finishWG.Add(1)
	go func() {
		// 确保在 goroutine 结束时减少计数
		defer kmm.wg.Done()
		defer mc.finishWG.Done()

		// 运行模块（通常会阻塞直到模块退出）
		err := mc.instance.Run(mc.ctx)
		if err != nil {
			// 模块运行失败，通知主循环触发优雅退出
			nlog.Infof("[Module] %s is finished with err=%s", mc.name, err.Error())
			kmm.runFailedModuleCh <- mc
		} else {
			// 模块正常运行结束
			nlog.Infof("[Module] %s is successful finished", mc.name)
		}
	}()

	return nil
}

// stepExit 执行一层模块的退出操作
//
// 用途：
// - 同时停止所有标记为"待退出"（false）的模块
// - 等待这些模块完全退出或超时
//
// 执行流程：
// 步骤1：遍历 canExitModules，找到所有标记为 false 的模块
//
//	对这些模块执行：
//	a. 调用模块的 cancel() 函数，发送退出信号
//	b. 标记为 true（已开始退出）
//	c. 在后台 goroutine 中等待模块的 finishWG 完成
//
// 步骤2：等待所有模块退出完成
//
//	使用 WaitGroup 等待所有模块的 finishWG
//	设置超时时间为 moduleDefaultExitTimeout（默认3秒）
//	- 如果超时，返回错误，进程强制退出
//	- 如果正常完成，记录日志继续
//
// 并发模型：
//   - 主协程：发起退出信号，等待所有模块退出
//   - 多个后台协程：每个模块一个协程，等待其 finishWG
//
// 参数：
//   - canExitModules: 模块退出状态映射（false=待退出, true=已退出）
//
// 返回值：
//   - error: 超时或其他错误时返回
func (kmm *kusciaModuleManager) stepExit(canExitModules map[string]bool) error {
	wg := sync.WaitGroup{}

	// 步骤1：对所有待退出的模块发起退出信号
	for name, exited := range canExitModules {
		// 跳过已经退出的模块
		if exited {
			continue
		}

		mc := kmm.modules[name]

		nlog.Infof("[Module] %s notified to exit...", name)

		// a. 调用 cancel 发送退出信号
		mc.cancel()

		// b. 标记为已开始退出
		canExitModules[name] = true

		// c. 在后台协程中等待模块完全退出
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			// 等待模块的 finishWG 完成
			// finishWG 在 runModule 的 goroutine 结束时 Done
			mc.finishWG.Wait()
		}(name)
	}

	// 步骤2：等待所有模块退出完成或超时
	select {
	// 超时情况：3秒内有模块未退出
	case <-time.After(moduleDefaultExitTimeout):
		return errors.New("some modules are not graceful exit, so exit process atonce")

	// 正常情况：所有模块都已退出
	case <-lock.NewWaitGroupChannel(&wg):
		nlog.Infof("Current step modules are finished now")
	}

	return nil
}

// gracefulExit 执行模块的优雅退出流程
//
// 核心思想：
// - 按照依赖关系的逆序停止模块（先停止叶子模块，再停止被依赖的模块）
// - 例如：agent 依赖 k3s，则先停止 agent，再停止 k3s
// - 这样可以确保被依赖的模块在其依赖者都停止后才停止
//
// 退出策略：
// 1. 分层退出：将模块按依赖关系分成多个层次
//   - 第1层：没有依赖的模块（可以立即退出）
//   - 第2层：依赖第1层的模块（第1层退出后可以退出）
//   - 第N层：依此类推...
//
// 2. 每层并行退出：同一层的模块可以同时退出（它们之间没有依赖关系）
//
// 3. 超时控制：每层退出有超时时间（默认3秒），超时后强制退出进程
//
// 执行流程：
// 步骤1：初始化 canExitModules 映射，标记哪些模块可以退出
//   - 没有依赖的模块（len(reverseDep[k]) == 0）初始化为 false（待退出）
//   - 未启动的模块（instance == nil）初始化为 true（已退出）
//
// 步骤2：循环执行分层退出
//
//	a. 取消与即将退出模块相关的 Hook 回调
//	b. 调用 stepExit 停止当前层的所有模块
//	c. 检查是否有新的模块可以退出（其依赖的模块都已退出）
//	d. 如果没有新模块可以退出，跳出循环（可能有循环依赖）
//
// 步骤3：验证是否所有模块都已退出
//
//	如果不是，可能存在循环依赖，记录警告日志
//
// 参数：
//   - modules: 所有需要启动的模块集合
//   - reverseDep: 反向依赖图
//
// 返回值：
//   - error: 退出超时或其他错误时返回
func (kmm *kusciaModuleManager) gracefulExit(modules map[string]*moduleInfo, reverseDep map[string][]string) error {
	nlog.Infof("GracefulExit started...")

	// canExitModules 跟踪每个模块的退出状态
	// true: 模块已退出或可以退出
	// false: 模块正在退出或等待退出
	canExitModules := map[string]bool{}

	// 步骤1：标记没有依赖的模块为"待退出"（false）
	// 这些模块是第一层，可以立即开始退出
	for k, v := range reverseDep {
		if len(v) == 0 {
			canExitModules[k] = false
		}
	}

	// 标记所有未启动的模块为"已退出"（true）
	// 这些模块不需要退出操作
	for _, mc := range modules {
		if mc.instance == nil {
			canExitModules[mc.name] = true
		}
	}

	// 步骤2：循环执行分层退出
	for {
		// a. 取消与即将退出模块相关的 Hook 回调
		kmm.cancelCallbacks(canExitModules)

		// b. 停止当前层的所有模块（那些标记为 false 的模块）
		if err := kmm.stepExit(canExitModules); err != nil {
			return err
		}

		// c. 检查是否有新的模块可以退出
		// 如果一个模块的所有依赖模块都已退出（reverseDep 中的所有模块都是 true），
		// 则该模块现在可以退出了
		hasNewModule := false
		for _, mc := range modules {
			if kmm.isModuleCanExit(mc, canExitModules, reverseDep[mc.name]) {
				// 标记为待退出
				canExitModules[mc.name] = false
				hasNewModule = true
			}
		}

		// d. 如果没有新模块可以退出，退出循环
		if !hasNewModule {
			break
		}
	}

	// 步骤3：验证是否所有模块都已处理
	if len(canExitModules) != len(modules) {
		nlog.Warnf("No new module can exit, may be circular dependency")
	}

	return nil
}

// isModuleCanExit 判断一个模块是否可以退出
//
// 判断条件：
// 1. 该模块尚未被标记为可退出（不在 canExitModules 中）
// 2. 该模块的所有反向依赖（依赖该模块的其他模块）都已退出
//
// 例如：
//   - 如果 agent 依赖 k3s，则 reverseDeps["k3s"] = ["agent"]
//   - 要判断 k3s 是否可以退出，需要检查 agent 是否已退出
//   - 只有 agent 已退出（canExitModules["agent"] == true），k3s 才能退出
//
// 参数：
//   - mc: 要判断的模块信息
//   - canExitModules: 当前各模块的退出状态
//   - reverseDeps: 依赖该模块的模块列表（反向依赖）
//
// 返回值：
//   - bool: true 表示该模块可以退出，false 表示还不能退出
func (kmm *kusciaModuleManager) isModuleCanExit(mc *moduleInfo, canExitModules map[string]bool, reverseDeps []string) bool {
	// 条件1：该模块尚未被标记为可退出
	if _, ok := canExitModules[mc.name]; ok {
		return false
	}

	// 条件2：检查所有反向依赖是否都已退出
	canExit := true
	for _, dep := range reverseDeps {
		if isExited, ok := canExitModules[dep]; ok {
			// 如果有一个依赖模块还未退出，则当前模块不能退出
			if !isExited {
				canExit = false
				break
			}
		}
	}

	return canExit
}
