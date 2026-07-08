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

# ========================================== golang.mk ===============================================
# All make targets related to golang should be defined here.
# 本文件集中定义所有与 Golang 编译、测试、代码检查相关的 Makefile 目标。
# 它通过顶层 Makefile 以 `-f scripts/make/golang.mk` 的方式被加载。
# ========================================== golang.mk ===============================================

# =============================================================================
# Makefile 语法小注（本文件会用到的部分）
# =============================================================================
# 1. 变量赋值
#    =   递归展开变量，使用时才求值
#    :=  简单展开变量，定义时立即求值
#    ?=  若环境变量/命令行未设置，则使用默认值
#
# 2. .PHONY
#    声明该目标不对应真实文件，避免目录里存在同名文件时 make 误判为已最新。
#
# 3. ##@ Category
#    分类标记。make help 会把它下面的目标归类显示，例如 "Build"。
#
# 4. 目标多行写法
#    target: ## 帮助文本
#    target: prerequisites
#    target:
#        recipe
#    第一行 ## 后面的文字会被 make help 抓取；
#    后续几行都是同一个目标的前提条件或 recipe，功能上等价于写成一行。
#
# 5. 多个前提条件拆行
#    target: dep1 dep2
#    target: dep3
#    make 会把它们合并成 dep1 dep2 dep3。
#
# 6. @ 前缀
#    recipe 行以 @ 开头时，make 执行前不会回显这条命令本身。
#
# 7. $(LOG_TARGET)
#    定义在 scripts/make/common.mk 中的宏，用于打印 "==> Running xxx" 的目标头。
#
# 8. recipe 中的 $$
#    Makefile 里 $ 有特殊含义。想让 shell 看到 $ 必须进行转义，写成 $$。
#    例如 $$(go list ./cmd/...) 实际执行时变成 $(go list ./cmd/...)，
#    由 bash 做命令替换。
# =============================================================================

# CMD_EXCLUDE_TESTS / PKG_EXCLUDE_TESTS
# 用于 go test 时过滤掉不需要运行的测试包，值是传给 grep -Ev 的正则字符串。
# 这里把 example、webdemo、testing、test、container 等示例/测试辅助包排除。
CMD_EXCLUDE_TESTS = "example|webdemo|testing|test|container"
PKG_EXCLUDE_TESTS = "crd|testing|test"

# TEST_SUITE 用于 integration_test 目标，指定要跑哪个集成测试套件。
# ?= 表示如果命令行没有传入 TEST_SUITE=xxx，则默认使用 "all"。
TEST_SUITE ?= all

##@ Build

# -----------------------------------------------------------------------------
# 目标：fmt
# 作用：对全部 Go 源码执行 go fmt，统一代码格式。
# -----------------------------------------------------------------------------
.PHONY: fmt
fmt: # Run go fmt against code.
	@$(LOG_TARGET)
	go fmt ./...


# -----------------------------------------------------------------------------
# 目标：vet
# 作用：对全部 Go 源码执行 go vet，捕捉常见错误。
# -----------------------------------------------------------------------------
.PHONY: vet
vet: # Run go vet against code.
	@$(LOG_TARGET)
	go vet ./...


# -----------------------------------------------------------------------------
# 目标：test
# 作用：运行 cmd/ 与 pkg/ 下的单元测试，并生成 JUnit XML 与覆盖率报告。
# 说明：
#   - --parallel 4 设置测试并行度。
#   - -gcflags="all=-N -l" 关闭编译优化，便于调试。
#   - tee 把 go test 输出同时显示到终端并写入文件，供 go-junit-report 转换。
#   - awk 去重逻辑：覆盖率文件按 sort -r 倒序后，相同记录只保留第一条。
# -----------------------------------------------------------------------------
.PHONY: test
test: ## Run tests.
test:
	@$(LOG_TARGET)
	rm -rf ./test-results
	mkdir -p test-results

	go test -v $$(go list ./cmd/... | grep -Ev ${CMD_EXCLUDE_TESTS}) \
		--parallel 4 -gcflags="all=-N -l" \
		-coverprofile=test-results/cmd.covprofile.out | tee test-results/cmd.output.txt
	go test -v $$(go list ./pkg/... | grep -Ev ${PKG_EXCLUDE_TESTS}) \
		--parallel 4 -gcflags="all=-N -l" \
		-coverprofile=test-results/pkg.covprofile.out | tee test-results/pkg.output.txt

	cat ./test-results/cmd.output.txt | go-junit-report > ./test-results/TEST-cmd.xml
	cat ./test-results/pkg.output.txt | go-junit-report > ./test-results/TEST-pkg.xml

	echo "mode: set" > ./test-results/coverage.out && cat ./test-results/*.covprofile.out | grep -v mode: | sort -r | awk '{if($$1 != last) {print $$0;last=$$1}}' >> ./test-results/coverage.out
	cat ./test-results/coverage.out | gocover-cobertura > ./test-results/coverage.xml


# -----------------------------------------------------------------------------
# 目标：clean
# 作用：清理构建产物与测试结果，方便重新构建。
# 命令前加 - 表示即使该命令失败（如目录不存在），make 也不中断。
# -----------------------------------------------------------------------------
.PHONY: clean
clean: ## clean build and test product.
clean:
	@$(LOG_TARGET)
	-rm -rf ./test-results
	-rm -rf ./build/apps
	-rm -rf ./build/framework
	-rm -rf ./tmp-crd-code
	-rm -rf ./build/linux


# -----------------------------------------------------------------------------
# 目标：build
# 作用：编译 Kuscia 主二进制，并把产物整理到 Docker 构建上下文需要的目录。
# 依赖关系：
#   build -> check_code -> fmt / vet / verify_error_code
#   verify_error_code 定义在 scripts/make/docs.mk 中，用于校验错误码 i18n 配置。
# recipe 说明：
#   1. 调用 hack/build.sh 完成真正的 go build，生成 build/apps/kuscia/kuscia。
#   2. Dockerfile 中 COPY 的源路径是 build/linux/${ARCH}/apps，
#      因此编译后需要把 build/apps 复制到 build/linux/${ARCH}/ 下。
# -----------------------------------------------------------------------------
.PHONY: build
build: ## build kuscia binary.
build: check_code
build:
	@$(LOG_TARGET)
	bash hack/build.sh -t kuscia
	mkdir -p build/linux/${ARCH}
	cp -rp build/apps build/linux/${ARCH}




# -----------------------------------------------------------------------------
# 目标：check_code
# 作用：聚合所有代码检查目标（fmt、vet、verify_error_code），作为 build 的前置条件。
# 注意：
#   check_code: fmt vet
#   check_code: verify_error_code
#   这两行会被 make 合并成同一个前提列表 fmt vet verify_error_code。
#   这些子目标之间没有先后依赖，make 会并行/按需执行它们。
# -----------------------------------------------------------------------------
.PHONY: check_code
check_code: ## check code format.
check_code: fmt vet
check_code: verify_error_code
check_code:
	@$(LOG_TARGET)
	@$(call log,  "check code FINISH")


# -----------------------------------------------------------------------------
# 目标：integration_test
# 作用：基于已经构建好的 Kuscia 镜像运行集成测试。
# 依赖 image 目标（定义在 scripts/make/image.mk），所以执行前会先确保 Docker 镜像已构建。
# ${IMG} 变量在 image.mk 中定义，格式为 secretflow/kuscia:<version>-<datetime>。
# TEST_SUITE 可通过命令行传入，例如：make integration_test TEST_SUITE=center.base。
# -----------------------------------------------------------------------------
.PHONY: integration_test
integration_test: ## Run Integration Test
integration_test: image
integration_test:
	mkdir -p run/test
	cd run && KUSCIA_IMAGE=${IMG} docker run --rm ${IMG} cat /home/kuscia/tests/integration_test.sh > ./test/integration_test.sh && chmod u+x ./test/integration_test.sh
	cd run && KUSCIA_IMAGE=${IMG} ./test/integration_test.sh ${TEST_SUITE}
