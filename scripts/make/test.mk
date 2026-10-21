#
# Copyright 2025 Ant Group Co., Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#   https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# ========================================== test.mk ===============================================
# All make targets related to shell script tests should be defined here.
# Go tests live in golang.mk (`make test` / `make integration_test`).
# ========================================== test.mk ===============================================

TEST_SUITE ?= all

##@ Test

.PHONY: test-scripts
test-scripts: ## Run offline shell script checks and unit tests (no Docker needed).
	@$(LOG_TARGET)
	bash scripts/test/run_local_tests.sh


.PHONY: test-scripts-lint
test-scripts-lint: ## Run bash syntax checks and shellcheck for shell scripts.
	@$(LOG_TARGET)
	bash scripts/test/run_local_tests.sh --lint-only


.PHONY: test-scripts-unit
test-scripts-unit: ## Run shunit2 unit tests for shell scripts only.
	@$(LOG_TARGET)
	bash scripts/test/run_local_tests.sh --unit-only


# 默认使用 image.mk 的 ${IMG}（即 `make image` 的产物）；
# 也可以通过环境变量 KUSCIA_IMAGE 指定已存在的镜像。
.PHONY: test-integration-scripts
test-integration-scripts: ## Run Docker based integration suites via scripts/test/run_integration_tests.sh.
	@$(LOG_TARGET)
	KUSCIA_IMAGE="$${KUSCIA_IMAGE:-$(IMG)}" bash scripts/test/run_integration_tests.sh --suite $(TEST_SUITE)
