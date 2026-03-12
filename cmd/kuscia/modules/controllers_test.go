// Copyright 2026 Ant Group Co., Ltd.
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

package modules

import (
	"testing"

	"github.com/secretflow/kuscia/cmd/kuscia/confloader"
	"github.com/secretflow/kuscia/pkg/common"
	"github.com/secretflow/kuscia/pkg/utils/kubeconfig"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"

	kusciafake "github.com/secretflow/kuscia/pkg/crd/clientset/versioned/fake"
	extensionfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
)

func TestNewControllersModule(t *testing.T) {
	config := &ModuleRuntimeConfigs{
		KusciaConfig: confloader.KusciaConfig{
			RootDir:  "/tmp/test",
			DomainID: "test-domain",
			RunMode:  common.RunModeMaster,
			GarbageCollection: confloader.GarbageCollectionConfig{
				KusciaDomainDataGC: confloader.KusciaDomainDataGCConfig{
					Enable:        ptr.To(true),
					DurationHours: 168,
				},
				KusciaJobGC: confloader.KusciaJobGCConfig{
					DurationHours: 720,
				},
			},
			EnableWorkloadApprove: true,
		},
		Clients: &kubeconfig.KubeClients{
			KubeClient:       fake.NewSimpleClientset(),
			KusciaClient:     kusciafake.NewSimpleClientset(),
			ExtensionsClient: extensionfake.NewSimpleClientset(),
		},
	}

	module, err := NewControllersModule(config)
	assert.NoError(t, err)
	assert.NotNil(t, module)
}
