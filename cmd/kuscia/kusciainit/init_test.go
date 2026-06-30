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

package kusciainit

import (
	"context"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/secretflow/kuscia/cmd/kuscia/confloader"
	"github.com/secretflow/kuscia/pkg/utils/paths"
	"github.com/secretflow/kuscia/pkg/utils/tls"
)

func TestKusciaInitCommand_ConfigConvert_Master(t *testing.T) {
	t.Parallel()
	// 1) 先生成一份临时 RSA 私钥文件，模拟用户在命令行中通过 --domain-key-file 传入密钥文件的场景。
	//    这里使用真实的私钥文件，而不是直接拼接字符串，确保转换逻辑和实际初始化流程一致。
	domainKeyFile := path.Join(t.TempDir(), "domain.key")
	assert.Nil(t, tls.GeneratePrivateKeyToFile(domainKeyFile))

	// 2) 读取文件内容并转成 base64 编码的 DomainKeyData。
	//    Init 命令最终不会保留文件路径，而是把文件中的 key 内容加载到配置体中。
	domainKeyData, err := loadDomainKeyData(domainKeyFile)
	assert.Nil(t, err)

	// 3) 构造 init 命令的输入参数，模拟 Master 模式下的启动配置。
	//    这些字段会在 convert2KusciaConfig 中被映射到 confloader.MasterKusciaConfig。
	input := InitConfig{
		Mode:                  "Master",
		DomainID:              "kuscia-system",
		DomainKeyFile:         domainKeyFile,
		LogLevel:              "INFO",
		Protocol:              "NOTLS",
		EnableWorkloadApprove: true,
	}

	dst := confloader.MasterKusciaConfig{
		CommonConfig: confloader.CommonConfig{
			Mode:          "Master",
			DomainID:      "kuscia-system",
			DomainKeyData: domainKeyData,
			LogLevel:      "INFO",
			Protocol:      "NOTLS",
		},
		AdvancedConfig: confloader.AdvancedConfig{
			EnableWorkloadApprove: true,
		},
	}

	// 4) 构造期望值，确认命令转换后每个字段都被正确映射。
	//    重点检查 DomainKeyData 是否被正确写入 CommonConfig，而不是只保留文件路径。
	var exceptValue interface{}
	exceptValue = dst

	assert.EqualValues(t, exceptValue, input.convert2KusciaConfig())
}

func TestKusciaInitCommand_ConfigConvert_Lite(t *testing.T) {
	t.Parallel()
	// 1) 为 Lite 模式准备临时私钥文件。
	domainKeyFile := path.Join(t.TempDir(), "domain.key")
	assert.Nil(t, tls.GeneratePrivateKeyToFile(domainKeyFile))

	// 2) 将文件内容转换成配置需要的 base64 key 数据。
	domainKeyData, err := loadDomainKeyData(domainKeyFile)
	assert.Nil(t, err)

	// 3) 构造 Lite 模式的命令输入：除了基础域信息外，还需要 lite 专有字段，例如 deploy token、master endpoint 和 runtime。
	input := InitConfig{
		Mode:                  "Lite",
		DomainID:              "alice",
		LogLevel:              "INFO",
		DomainKeyFile:         domainKeyFile,
		LiteDeployToken:       "test",
		MasterEndpoint:        "https://master.svc:1080",
		Runtime:               "runc",
		Protocol:              "NOTLS",
		EnableWorkloadApprove: false,
	}

	dst := confloader.LiteKusciaConfig{
		CommonConfig: confloader.CommonConfig{
			Mode:          "Lite",
			DomainID:      "alice",
			DomainKeyData: domainKeyData,
			LogLevel:      "INFO",
			Protocol:      "NOTLS",
		},
		LiteDeployToken: "test",
		MasterEndpoint:  "https://master.svc:1080",
		Runtime:         "runc",
		AdvancedConfig: confloader.AdvancedConfig{
			EnableWorkloadApprove: false,
		},
	}

	// 4) 断言转换结果和预期完全一致，尤其验证 lite 模式下专有字段是否被带入。
	var exceptValue interface{}
	exceptValue = dst

	assert.EqualValues(t, exceptValue, input.convert2KusciaConfig())
}

func TestKusciaInitCommand_ConfigConvert_Autonomy(t *testing.T) {
	t.Parallel()
	// 1) 生成自治模式所需的临时私钥文件。
	domainKeyFile := path.Join(t.TempDir(), "domain.key")
	assert.Nil(t, tls.GeneratePrivateKeyToFile(domainKeyFile))

	// 2) 读取并编码 key 内容，供后续写入 CommonConfig.DomainKeyData。
	domainKeyData, err := loadDomainKeyData(domainKeyFile)
	assert.Nil(t, err)

	// 3) 构造自治模式输入。
	//    和 Master / Lite 不同，Autonomy 模式下只需要基础域信息和 runtime，不需要额外的 deploy token 或 master endpoint。
	input := InitConfig{
		Mode:          "Autonomy",
		DomainID:      "alice",
		LogLevel:      "INFO",
		DomainKeyFile: domainKeyFile,
		Runtime:       "runc",
		Protocol:      "NOTLS",
	}

	// 4) 断言转换结果，确保自治模式下的字段映射没有遗漏。
	dst := confloader.AutonomyKusciaConfig{
		CommonConfig: confloader.CommonConfig{
			Mode:          "Autonomy",
			DomainID:      "alice",
			DomainKeyData: domainKeyData,
			LogLevel:      "INFO",
			Protocol:      "NOTLS",
		},
		Runtime: "runc",
	}

	var exceptValue interface{}
	exceptValue = dst

	assert.EqualValues(t, exceptValue, input.convert2KusciaConfig())
}

func TestLoadDomainKeyData_FileExists(t *testing.T) {
	t.Parallel()
	// 1) 准备一个真实可解析的 RSA 私钥文件。
	validKeyFile := path.Join(t.TempDir(), "domain.key")
	assert.Nil(t, tls.GeneratePrivateKeyToFile(validKeyFile))

	// 2) loadDomainKeyData 应该能读取并转成可写入配置的 key data。
	_, err := loadDomainKeyData(validKeyFile)
	assert.Nil(t, err)
}

func TestLoadDomainKeyData_FileNotExists(t *testing.T) {
	t.Parallel()

	// 不存在的文件路径应返回错误，避免初始化阶段误以为空内容有效。
	_, err := loadDomainKeyData(path.Join(t.TempDir(), "empty.key"))
	assert.NotNil(t, err)
}

func TestLoadDomainKeyData_InvalidateFileExists(t *testing.T) {
	t.Parallel()
	// 写入一段非法内容，模拟 key 文件被污染或格式错误的场景。
	invalidKeyFile := path.Join(t.TempDir(), "invalid-domain.key")
	assert.Nil(t, paths.WriteFile(invalidKeyFile, []byte("test")))

	// 非法 key 文件应该被拒绝，防止生成不可用配置。
	_, err := loadDomainKeyData(invalidKeyFile)
	assert.NotNil(t, err)
}

func TestLoadDomainKeyData_EmptyFile(t *testing.T) {
	t.Parallel()

	// 空路径表示用户没有传入 domain-key-file，这里应当允许并返回空值，而不是报错。
	_, err := loadDomainKeyData("")
	assert.Nil(t, err)
}

func TestNewInitCommand(t *testing.T) {
	// 这里只检查命令对象本身是否正确构造，确保 init 命令的入口、描述和参数定义都存在。
	ctx := context.Background()
	cmd := NewInitCommand(ctx)

	// 1) 基本元信息：命令名和简短描述。
	assert.Equal(t, "init", cmd.Use)
	assert.Equal(t, "Init means init Kuscia config", cmd.Short)

	// 2) 检查关键 flag 是否注册，保证后续命令行参数能正常解析到 InitConfig。
	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("mode"))
	assert.NotNil(t, flags.Lookup("domain"))
	assert.NotNil(t, flags.Lookup("runtime"))
	assert.NotNil(t, flags.Lookup("domain-key-file"))
	assert.NotNil(t, flags.Lookup("log-level"))
	assert.NotNil(t, flags.Lookup("lite-deploy-token"))
	assert.NotNil(t, flags.Lookup("master-endpoint"))
	assert.NotNil(t, flags.Lookup("datastore-endpoint"))
	assert.NotNil(t, flags.Lookup("protocol"))
	assert.NotNil(t, flags.Lookup("enable-workload-approve"))
}
