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

// Package start 包含Kuscia启动模块的相关功能和测试
// 该文件包含模块管理器的单元测试，验证模块注册、依赖设置、启动等功能的正确性
package start

import (
	"context" // Context包，用于传递截止时间、取消信号和其他请求范围的值
	"errors"  // Errors包，用于创建和处理错误
	"testing" // Testing包，提供Go语言的测试框架
	"time"    // Time包，提供时间相关功能

	"github.com/stretchr/testify/assert" // Assert包，提供断言功能用于测试
	"k8s.io/apimachinery/pkg/util/wait"  // Wait包，提供等待功能

	"github.com/secretflow/kuscia/cmd/kuscia/modules" // Modules包，提供模块相关功能
	"github.com/secretflow/kuscia/pkg/common"         // Common包，提供公共常量和类型
)

// TestModuleManager_Regist 测试模块管理器的注册功能
// 运行逻辑：
// 1. 创建一个新的模块管理器实例
// 2. 尝试使用nil创建器注册模块，验证应返回false
// 3. 使用有效的创建器注册模块，验证应返回true
// 4. 尝试重复注册同名模块，验证应返回false
func TestModuleManager_Regist(t *testing.T) {
	t.Parallel()            // 并行运行此测试，提高测试效率
	m := NewModuleManager() // 创建新的模块管理器实例
	assert.NotNil(t, m)     // 断言管理器实例不为nil

	// 尝试使用nil创建器注册模块，应该失败
	assert.False(t, m.Regist("m1", nil, common.RunModeAutonomy))

	// 定义一个模块创建器函数，返回mock模块
	creator := func(*modules.ModuleRuntimeConfigs) (modules.Module, error) {
		return &mockModule{}, nil
	}

	// 使用有效的创建器注册模块，应该成功
	assert.True(t, m.Regist("m1", creator, common.RunModeAutonomy))
	// 尝试重复注册同名模块，应该失败
	assert.False(t, m.Regist("m1", creator, common.RunModeAutonomy))
}

// TestModuleManager_SetDependencies 测试模块管理器设置依赖关系的功能
// 运行逻辑：
// 1. 创建模块管理器并注册两个模块
// 2. 尝试为不存在的模块设置依赖，验证应失败
// 3. 为存在的模块设置依赖，验证应成功
// 4. 检查依赖关系是否正确建立
func TestModuleManager_SetDependencies(t *testing.T) {
	t.Parallel()            // 并行运行此测试
	m := NewModuleManager() // 创建新的模块管理器实例
	assert.NotNil(t, m)     // 断言管理器实例不为nil

	// 定义一个模块创建器函数
	creator := func(*modules.ModuleRuntimeConfigs) (modules.Module, error) {
		return &mockModule{}, nil
	}

	// 注册第一个模块
	assert.True(t, m.Regist("m1", creator, common.RunModeAutonomy))

	// 尝试为不存在的模块设置依赖，应该失败
	assert.False(t, m.SetDependencies("m2", "m1"))
	// 尝试为存在的模块设置对不存在模块的依赖，应该失败
	assert.False(t, m.SetDependencies("m1", "m2"))

	// 注册第二个模块
	assert.True(t, m.Regist("m2", creator, common.RunModeAutonomy))
	// 为m1设置对m2的依赖，应该成功
	assert.True(t, m.SetDependencies("m1", "m2"))

	// 类型断言获取具体的模块管理器实现
	kmm, ok := m.(*kusciaModuleManager)
	assert.True(t, ok)    // 断言类型转换成功
	assert.NotNil(t, kmm) // 断言转换后的实例不为nil

	// 验证模块数量
	assert.Len(t, kmm.modules, 2)
	assert.NotNil(t, kmm.modules["m1"]) // 验证m1模块存在
	assert.NotNil(t, kmm.modules["m2"]) // 验证m2模块存在
	// 验证m1模块的依赖数量
	assert.Len(t, kmm.modules["m1"].dependencies, 1)
	// 验证m1模块依赖于m2模块
	assert.Equal(t, "m2", kmm.modules["m1"].dependencies[0])
}

// TestModuleManager_AddReadyHook 测试模块管理器添加就绪钩子的功能
// 运行逻辑：
// 1. 创建模块管理器
// 2. 尝试添加nil钩子或对不存在模块的钩子，验证应失败
// 3. 注册一个模块
// 4. 为该模块添加就绪钩子，验证应成功
func TestModuleManager_AddReadyHook(t *testing.T) {
	t.Parallel()            // 并行运行此测试
	m := NewModuleManager() // 创建新的模块管理器实例
	assert.NotNil(t, m)     // 断言管理器实例不为nil

	// 尝试添加nil钩子到不存在的模块，应该失败
	assert.False(t, m.AddReadyHook(nil, "no-exists"))

	// 定义一个简单的钩子函数
	hook := func(ctx context.Context, modules map[string]modules.Module) error {
		return nil // 简单返回nil表示成功
	}
	// 尝试为不存在的模块添加钩子，应该失败
	assert.False(t, m.AddReadyHook(hook, "no-exists"))

	// 定义模块创建器
	creator := func(*modules.ModuleRuntimeConfigs) (modules.Module, error) {
		return &mockModule{}, nil
	}

	// 注册模块，应该成功
	assert.True(t, m.Regist("m1", creator, common.RunModeAutonomy))
	// 为存在的模块添加就绪钩子，应该成功
	assert.True(t, m.AddReadyHook(hook, "m1"))
}

// getModule 辅助函数，用于从模块管理器中获取指定名称的mock模块实例
// 运行逻辑：
// 1. 将通用模块管理器转换为具体实现
// 2. 检查指定名称的模块是否存在
// 3. 如果模块实例存在且为mockModule类型，则返回该实例
func getModule(t *testing.T, m ModuleManager, name string) *mockModule {
	// 类型断言将通用接口转换为具体实现
	kmm, _ := m.(*kusciaModuleManager)
	assert.NotNil(t, kmm)               // 断言转换成功
	assert.NotNil(t, kmm.modules[name]) // 断言指定名称的模块存在
	// 检查模块实例是否存在
	if kmm.modules[name].instance != nil {
		// 尝试将实例转换为mockModule类型
		if mm, ok := kmm.modules[name].instance.(*mockModule); ok {
			return mm // 转换成功则返回实例
		}
	}

	return nil // 未找到或类型不匹配则返回nil
}

// TestModuleManager_Start_normal 测试模块管理器正常启动功能
// 运行逻辑：
// 1. 创建模块管理器和索引跟踪器
// 2. 定义模块创建器函数
// 3. 注册一个模块
// 4. 启动goroutine模拟模块的正常运行流程（等待模块实例化 -> 标记就绪 -> 关闭运行通道）
// 5. 启动模块管理器并验证执行顺序
func TestModuleManager_Start_normal(t *testing.T) {
	t.Parallel()            // 并行运行此测试
	m := NewModuleManager() // 创建新的模块管理器实例
	assert.NotNil(t, m)     // 断言管理器实例不为nil

	// 创建索引跟踪器，用于跟踪创建、运行和停止的顺序
	idx := &mmIndex{}

	// 定义模块创建器函数，返回带有索引跟踪的mock模块
	creator := func(name string) NewModuleFunc {
		return func(*modules.ModuleRuntimeConfigs) (modules.Module, error) {
			// 创建带有索引跟踪的mock模块
			return createMockModule(name, idx), nil
		}
	}

	// 注册名为"m1"的模块
	assert.True(t, m.Regist("m1", creator("m1"), common.RunModeAutonomy))

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // 测试结束后取消上下文

	// 定义模拟正常模块运行的函数
	normalModule := func(name string) {
		// 等待直到模块实例被创建（轮询检查直到getModule返回非nil）
		assert.NoError(t, wait.PollImmediate(time.Millisecond*50, time.Second, func() (done bool, err error) {
			return getModule(t, m, name) != nil, nil
		}))
		// 标记模块就绪，关闭就绪通道
		close(getModule(t, m, name).readyChan)
		// 等待一段时间
		time.Sleep(100 * time.Millisecond)
		// 关闭运行通道，表示模块运行完成
		close(getModule(t, m, name).runChan)
	}

	// 在单独的goroutine中运行模拟模块
	go normalModule("m1")

	// 启动模块管理器，使用Autonomy运行模式
	assert.NoError(t, m.Start(ctx, common.RunModeAutonomy, nil))
	// 验证创建、运行和停止的顺序计数
	assert.Equal(t, 1, idx.nextCreateOrder) // 应该创建了1个模块
	assert.Equal(t, 1, idx.nextRunOrder)    // 应该运行了1个模块
	assert.Equal(t, 1, idx.nextStopOrder)   // 应该停止了1个模块
}

// TestModuleManager_Start_withdep 测试模块管理器带依赖的启动功能
// 运行逻辑：
// 1. 创建模块管理器和索引跟踪器
// 2. 注册两个模块m1和m2
// 3. 设置m1依赖于m2（启动顺序：m2 -> m1）
// 4. 启动goroutine模拟两个模块的运行流程
// 5. 启动第三个goroutine等待两个模块都就绪后取消上下文
// 6. 启动模块管理器并验证执行顺序和依赖关系
func TestModuleManager_Start_withdep(t *testing.T) {
	t.Parallel()            // 并行运行此测试
	m := NewModuleManager() // 创建新的模块管理器实例
	assert.NotNil(t, m)     // 断言管理器实例不为nil

	// 创建索引跟踪器，用于跟踪创建、运行和停止的顺序
	idx := &mmIndex{}

	// 定义模块创建器函数，返回带有索引跟踪的mock模块
	creator := func(name string) NewModuleFunc {
		return func(*modules.ModuleRuntimeConfigs) (modules.Module, error) {
			// 创建带有索引跟踪的mock模块
			return createMockModule(name, idx), nil
		}
	}

	// 注册两个模块
	assert.True(t, m.Regist("m1", creator("m1"), common.RunModeAutonomy))
	assert.True(t, m.Regist("m2", creator("m2"), common.RunModeAutonomy))

	// 设置依赖关系：m1依赖于m2，意味着m2必须在m1之前启动
	// 启动顺序将是：m2 -> m1
	assert.True(t, m.SetDependencies("m1", "m2"))

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // 测试结束后取消上下文

	// 定义模拟正常模块运行的函数
	normalModule := func(name string) {
		// 等待直到模块实例被创建（轮询检查直到getModule返回非nil）
		assert.NoError(t, wait.PollImmediate(time.Millisecond*50, time.Second, func() (bool, error) {
			return getModule(t, m, name) != nil, nil
		}))
		// 等待一段时间，确保模块正在运行
		time.Sleep(100 * time.Millisecond)
		// 标记模块就绪，关闭就绪通道
		close(getModule(t, m, name).readyChan)
	}

	// 在单独的goroutine中运行两个模块
	go normalModule("m1")
	go normalModule("m2")
	// 第三个goroutine等待两个模块都就绪后取消上下文
	go func() {
		// 类型断言获取具体的模块管理器实现
		kmm, _ := m.(*kusciaModuleManager)
		assert.NotNil(t, kmm)
		assert.NotNil(t, kmm.modules["m1"])
		assert.NotNil(t, kmm.modules["m2"])
		// 等待两个模块的就绪等待都完成
		assert.NoError(t, wait.PollImmediate(time.Millisecond*50, time.Second*2, func() (bool, error) {
			return kmm.modules["m1"].isReadyWaitDone && kmm.modules["m2"].isReadyWaitDone, nil
		}))

		// 所有模块就绪后取消上下文，结束测试
		cancel()
	}()

	// 启动模块管理器，使用Autonomy运行模式
	assert.NoError(t, m.Start(ctx, common.RunModeAutonomy, nil))
	// 验证创建、运行和停止的顺序计数
	assert.Equal(t, 2, idx.nextCreateOrder) // 应该创建了2个模块
	assert.Equal(t, 2, idx.nextRunOrder)    // 应该运行了2个模块
	assert.Equal(t, 2, idx.nextStopOrder)   // 应该停止了2个模块

	// 获取m1模块并验证其执行顺序
	m1 := getModule(t, m, "m1")
	assert.NotNil(t, m1)               // 验证m1存在
	assert.Equal(t, 1, m1.createOrder) // m1的创建顺序
	assert.Equal(t, 1, m1.runOrder)    // m1的运行顺序
	assert.Equal(t, 0, m1.stopOrder)   // m1的停止顺序

	// 获取m2模块并验证其执行顺序
	m2 := getModule(t, m, "m2")
	assert.NotNil(t, m2)               // 验证m2存在
	assert.Equal(t, 0, m2.createOrder) // m2的创建顺序
	assert.Equal(t, 0, m2.runOrder)    // m2的运行顺序
	assert.Equal(t, 1, m2.stopOrder)   // m2的停止顺序
}

// TestModuleManager_Start_readyFailed 测试模块管理器就绪失败的处理
// 运行逻辑：
// 1. 创建模块管理器和索引跟踪器
// 2. 注册两个模块并设置依赖关系
// 3. 启动goroutine模拟m2模块就绪失败
// 4. 启动模块管理器并验证失败处理逻辑（m1因依赖m2失败而不启动）
func TestModuleManager_Start_readyFailed(t *testing.T) {
	t.Parallel()            // 并行运行此测试
	m := NewModuleManager() // 创建新的模块管理器实例
	assert.NotNil(t, m)     // 断言管理器实例不为nil

	// 创建索引跟踪器，用于跟踪创建、运行和停止的顺序
	idx := &mmIndex{}

	// 定义模块创建器函数，返回带有索引跟踪的mock模块
	creator := func(name string) NewModuleFunc {
		return func(*modules.ModuleRuntimeConfigs) (modules.Module, error) {
			// 创建带有索引跟踪的mock模块
			return createMockModule(name, idx), nil
		}
	}

	// 注册两个模块
	assert.True(t, m.Regist("m1", creator("m1"), common.RunModeAutonomy))
	assert.True(t, m.Regist("m2", creator("m2"), common.RunModeAutonomy))

	// 设置依赖关系：m1依赖于m2，意味着m2必须在m1之前就绪
	// 启动顺序将是：m2 -> m1
	assert.True(t, m.SetDependencies("m1", "m2"))

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // 测试结束后取消上下文

	// 定义模拟错误模块运行的函数
	errorModule := func(name string) {
		// 等待直到模块实例被创建
		assert.NoError(t, wait.PollImmediate(time.Millisecond*50, time.Second, func() (bool, error) {
			return getModule(t, m, name) != nil, nil
		}))
		// 设置就绪错误，模拟模块启动失败
		getModule(t, m, name).readyError = errors.New("start failed")
		// 关闭就绪通道
		close(getModule(t, m, name).readyChan)
	}

	// 在单独的goroutine中运行错误模块（m2会就绪失败）
	go errorModule("m2")

	// 启动模块管理器，使用Autonomy运行模式
	// 由于m2就绪失败，m1不会启动（因为它依赖m2）
	assert.NoError(t, m.Start(ctx, common.RunModeAutonomy, nil))
	// 验证创建、运行和停止的顺序计数
	assert.Equal(t, 1, idx.nextCreateOrder) // 只有1个模块被创建（m2）
	assert.Equal(t, 1, idx.nextRunOrder)    // 只有1个模块被运行（m2）
	assert.Equal(t, 1, idx.nextStopOrder)   // 只有1个模块被停止（m2）

	// 验证m1模块为空（因为它依赖于失败的m2，所以没有启动）
	m1 := getModule(t, m, "m1")
	assert.Nil(t, m1) // m1不应该存在，因为它依赖的m2失败了

	// 验证m2模块存在但执行顺序
	m2 := getModule(t, m, "m2")
	assert.NotNil(t, m2)               // m2应该存在
	assert.Equal(t, 0, m2.createOrder) // m2的创建顺序
	assert.Equal(t, 0, m2.runOrder)    // m2的运行顺序
	assert.Equal(t, 0, m2.stopOrder)   // m2的停止顺序
}

// TestModuleManager_Start_runFailed 测试模块管理器运行失败的处理
// 运行逻辑：
// 1. 创建模块管理器和索引跟踪器
// 2. 注册两个模块并设置依赖关系
// 3. 启动goroutine模拟m1模块运行失败，m2模块正常运行
// 4. 启动模块管理器并验证失败处理逻辑
func TestModuleManager_Start_runFailed(t *testing.T) {
	t.Parallel()            // 并行运行此测试
	m := NewModuleManager() // 创建新的模块管理器实例
	assert.NotNil(t, m)     // 断言管理器实例不为nil

	// 创建索引跟踪器，用于跟踪创建、运行和停止的顺序
	idx := &mmIndex{}

	// 定义模块创建器函数，返回带有索引跟踪的mock模块
	creator := func(name string) NewModuleFunc {
		return func(*modules.ModuleRuntimeConfigs) (modules.Module, error) {
			// 创建带有索引跟踪的mock模块
			return createMockModule(name, idx), nil
		}
	}

	// 注册两个模块
	assert.True(t, m.Regist("m1", creator("m1"), common.RunModeAutonomy))
	assert.True(t, m.Regist("m2", creator("m2"), common.RunModeAutonomy))

	// 设置依赖关系：m1依赖于m2，意味着m2必须在m1之前启动
	// 启动顺序将是：m2 -> m1
	assert.True(t, m.SetDependencies("m1", "m2"))

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // 测试结束后取消上下文

	// 定义模拟正常模块运行的函数
	normalModule := func(name string) {
		// 等待直到模块实例被创建
		assert.NoError(t, wait.PollImmediate(time.Millisecond*50, time.Second, func() (bool, error) {
			return getModule(t, m, name) != nil, nil
		}))
		// 等待一段时间
		time.Sleep(100 * time.Millisecond)
		// 标记模块就绪，关闭就绪通道
		close(getModule(t, m, name).readyChan)
	}

	// 定义模拟运行失败模块的函数
	runFailedModule := func(name string) {
		// 等待直到模块实例被创建
		assert.NoError(t, wait.PollImmediate(time.Millisecond*50, time.Second, func() (bool, error) {
			return getModule(t, m, name) != nil, nil
		}))
		// 标记模块就绪，关闭就绪通道
		close(getModule(t, m, name).readyChan)
		// 等待一段时间
		time.Sleep(100 * time.Millisecond)
		// 设置运行错误，模拟模块运行期间失败
		getModule(t, m, name).runError = errors.New("run failed")
		// 关闭运行通道，表示模块运行完成（但失败）
		close(getModule(t, m, name).runChan)
	}

	// 在单独的goroutine中运行失败模块（m1运行失败）和正常模块（m2正常运行）
	go runFailedModule("m1")
	go normalModule("m2")

	// 启动模块管理器，使用Autonomy运行模式
	assert.NoError(t, m.Start(ctx, common.RunModeAutonomy, nil))
	// 验证创建、运行和停止的顺序计数
	assert.Equal(t, 2, idx.nextCreateOrder) // 2个模块都被创建
	assert.Equal(t, 2, idx.nextRunOrder)    // 2个模块都被运行
	assert.Equal(t, 1, idx.nextStopOrder)   // 只有1个模块被停止（m1，因为m2正常运行没有触发停止）

	// 验证m1模块存在但有运行错误
	m1 := getModule(t, m, "m1")
	assert.NotNil(t, m1)               // m1应该存在
	assert.Equal(t, 1, m1.createOrder) // m1的创建顺序
	assert.Equal(t, 1, m1.runOrder)    // m1的运行顺序
	assert.Equal(t, -1, m1.stopOrder)  // m1的停止顺序（-1表示未正常停止）

	// 验证m2模块存在且正常运行
	m2 := getModule(t, m, "m2")
	assert.NotNil(t, m2)               // m2应该存在
	assert.Equal(t, 0, m2.createOrder) // m2的创建顺序
	assert.Equal(t, 0, m2.runOrder)    // m2的运行顺序
	assert.Equal(t, 0, m2.stopOrder)   // m2的停止顺序
}
