#!/bin/bash
#
# Copyright 2025 Ant Group Co., Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

# CLI contract tests: help output, unknown-mode rejection and required
# arguments. These paths exit before touching Docker or kubectl.

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT_DIR="$(cd "${TEST_DIR}/../../.." && pwd -P)"
DEPLOY_SH="${ROOT_DIR}/scripts/deploy/deploy.sh"
KUSCIA_SH="${ROOT_DIR}/scripts/deploy/kuscia.sh"
ADD_DOMAIN_SH="${ROOT_DIR}/scripts/deploy/add_domain.sh"
ADD_DOMAIN_LITE_SH="${ROOT_DIR}/scripts/deploy/add_domain_lite.sh"
CREATE_TOKEN_SH="${ROOT_DIR}/scripts/deploy/create_token.sh"
RUN_LOCAL_SH="${ROOT_DIR}/scripts/run_local_kuscia.sh"
SHUNIT2_PATH="${ROOT_DIR}/tests/vendor/shunit2"

oneTimeSetUp() {
  # Capture PATH after shunit2 has added its lifecycle stub directory.
  ORIGINAL_PATH="${PATH}"
}

setUp() {
  TEST_TMP="$(mktemp -d)"
  mkdir -p "${TEST_TMP}/bin"

  # Fail the test if any contract path unexpectedly reaches docker/kubectl.
  cat > "${TEST_TMP}/bin/docker" <<'FAKE_DOCKER'
#!/bin/bash
echo "unexpected docker invocation: $*" >&2
exit 99
FAKE_DOCKER
  cat > "${TEST_TMP}/bin/kubectl" <<'FAKE_KUBECTL'
#!/bin/bash
echo "unexpected kubectl invocation: $*" >&2
exit 99
FAKE_KUBECTL
  chmod +x "${TEST_TMP}/bin/docker" "${TEST_TMP}/bin/kubectl"

  export PATH="${TEST_TMP}/bin:${ORIGINAL_PATH}"
}

tearDown() {
  export PATH="${ORIGINAL_PATH}"
  rm -rf "${TEST_TMP}"
}

run_capture() {
  LAST_OUTPUT="$("$@" 2>&1)" && LAST_STATUS=0 || LAST_STATUS=$?
}

testDeployHelpExitsZero() {
  run_capture bash "${DEPLOY_SH}" -h
  assertEquals "deploy.sh -h should succeed: ${LAST_OUTPUT}" 0 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "DEPLOY_MODE:"
  assertContains "${LAST_OUTPUT}" "autonomy"
}

testDeployRejectsUnknownMode() {
  run_capture bash "${DEPLOY_SH}" bogus
  assertEquals "unknown deploy mode should fail" 1 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "deploy_mode is invalid"
}

testKusciaHelpExitsZero() {
  run_capture bash "${KUSCIA_SH}" --help
  assertEquals "kuscia.sh --help should succeed: ${LAST_OUTPUT}" 0 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "DEPLOY_MODE:"
  assertContains "${LAST_OUTPUT}" "center"
}

testKusciaRejectsUnknownMode() {
  run_capture bash "${KUSCIA_SH}" bogus
  assertEquals "unknown network mode should fail" 1 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "unsupported network mode"
}

testAddDomainRequiresDomainId() {
  run_capture bash "${ADD_DOMAIN_SH}"
  assertEquals "missing DOMAIN_ID should fail" 1 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "missing argument: add_domain.sh DOMAIN_ID"
}

testAddDomainLiteRequiresDomainId() {
  run_capture bash "${ADD_DOMAIN_LITE_SH}"
  assertEquals "missing DOMAIN_ID should fail" 1 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "missing argument: add_domain_lite.sh DOMAIN_ID"
}

testCreateTokenRequiresDomainId() {
  run_capture bash "${CREATE_TOKEN_SH}"
  assertEquals "missing DOMAIN_ID should fail" 1 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "missing argument: create_token.sh DOMAIN_ID"
}

testRunLocalHelpExitsZero() {
  run_capture bash "${RUN_LOCAL_SH}" --help
  assertEquals "run_local_kuscia.sh --help should succeed: ${LAST_OUTPUT}" 0 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "Usage:"
  assertContains "${LAST_OUTPUT}" "master"
}

testRunLocalRejectsUnknownMode() {
  run_capture bash "${RUN_LOCAL_SH}" bogus
  assertEquals "unknown local mode should fail" 1 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "Unsupported mode: bogus"
}

# shellcheck source=/dev/null
. "${SHUNIT2_PATH}"
