/*
 * Copyright 2024 Ant Group Co., Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package start

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	clientsetfake "k8s.io/client-go/kubernetes/fake"

	"github.com/secretflow/kuscia/cmd/kuscia/confloader"
	"github.com/secretflow/kuscia/cmd/kuscia/modules"
	"github.com/secretflow/kuscia/pkg/common"
	kusciaclientsetfake "github.com/secretflow/kuscia/pkg/crd/clientset/versioned/fake"
	"github.com/secretflow/kuscia/pkg/utils/kubeconfig"
	"github.com/secretflow/kuscia/pkg/utils/nlog"
	"github.com/secretflow/kuscia/pkg/utils/paths"
	"github.com/secretflow/kuscia/pkg/utils/tls"
)

func Test_Start_Autonomy(t *testing.T) {
	autonomy := common.RunModeAutonomy
	// 1) 生成一份临时的域密钥，用于模拟自治模式下的 Kuscia 启动环境。
	//    这里不依赖真实密钥文件，而是直接在测试中构造配置，保证测试可重复、可隔离。
	domainKeyData, err := tls.GenerateKeyData()
	assert.NoError(t, err)
	kusciaConfig := &confloader.AutonomyKusciaConfig{
		CommonConfig: confloader.CommonConfig{
			Mode:          autonomy,
			DomainID:      "alice",
			DomainKeyData: domainKeyData,
			Protocol:      common.TLS,
		},
	}

	// 2) 将配置序列化到临时目录中的 kuscia.yaml。
	//    这样后续的启动流程会像真实部署一样，从文件读取配置，而不是直接依赖内存对象。
	data, err := yaml.Marshal(kusciaConfig)
	assert.NoError(t, err)
	dir := t.TempDir()
	common.DefaultKusciaHomePath = func() string {
		return dir
	}

	filename := filepath.Join(dir, "kuscia.yaml")
	assert.NoError(t, os.WriteFile(filename, data, 0600))
	defer func() {
		assert.NoError(t, os.Remove(filename))
	}()

	// 3) 准备运行目录所需的静态资源。
	//    start 流程会依赖 etc/ 和 crds/ 下的配置文件，因此测试需要把仓库中的模板复制到临时 home 目录。
	workDir := GetWorkDir()
	assert.NoError(t, paths.CopyDirectory(filepath.Join(workDir, "etc"), common.DefaultKusciaHomePath()+"/etc"))
	assert.NoError(t, paths.CopyDirectory(filepath.Join(workDir, "crds"), common.DefaultKusciaHomePath()+"/crds"))

	// 4) 构造启动上下文和模块运行配置。
	//    这里注入 fake kube client / fake kuscia client，避免测试依赖真实集群。
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	kusciaConf, err := confloader.ReadConfig(filename)
	assert.NoError(t, err)
	conf := modules.NewModuleRuntimeConfigs(runCtx, kusciaConf)
	conf.Clients = &kubeconfig.KubeClients{
		KubeClient:   clientsetfake.NewSimpleClientset(),
		KusciaClient: kusciaclientsetfake.NewSimpleClientset(),
	}
	defer conf.Close()
	mm := NewModuleManager()

	// 5) 注册自治模式下需要启动的模块。
	//    这里最小化只注册 coredns，目的是验证模块管理器在自治模式下能正常拉起依赖链路。
	assert.True(t, mm.Regist("coredns", modules.NewCoreDNS, autonomy))

	errCh := make(chan error)

	// 6) 在后台启动模块管理器。
	//    如果启动过程中出现错误，会写入 errCh；否则会一直运行，直到测试上下文结束或超时观察通过。
	go func() {
		errCh <- mm.Start(runCtx, autonomy, conf)
	}()

	// 7) 等待模块启动结果。
	//    - 如果快速返回错误，说明启动流程存在异常；
	//    - 如果 5 秒内没有错误，则认为核心模块已经成功进入运行态。
	select {
	case err := <-errCh:
		nlog.Errorf("Error occurred during start -> %v", err)
	case <-time.Tick(5 * time.Second):
		nlog.Info("all modules are running")
	}

	// 8) 验证启动副作用。
	//    start 流程会生成 tmp 目录下的 resolv.conf 备份，用于后续网络解析使用。
	assert.FileExists(t, filepath.Join(conf.RootDir, common.TmpPrefix, "resolv.conf"))
}

func GetWorkDir() string {
	// runtime.Caller 返回当前测试文件的绝对路径，随后通过截断到 cmd 目录之前的部分，得到仓库根目录。
	// 这样无论测试从哪个工作目录启动，都能稳定定位到 etc/、crds/ 等资源目录。
	_, filename, _, _ := runtime.Caller(0)
	nlog.Infof("path is %s", filename)
	return strings.SplitN(filename, "cmd", 2)[0]
}
