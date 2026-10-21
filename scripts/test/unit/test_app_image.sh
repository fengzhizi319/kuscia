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

# Unit tests for the AppImage registration scripts.
#
# A fake kubectl captures the rendered YAML instead of contacting a cluster,
# so these tests run fully offline.

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT_DIR="$(cd "${TEST_DIR}/../../.." && pwd -P)"
CREATE_SF_APP_IMAGE="${ROOT_DIR}/scripts/deploy/create_sf_app_image.sh"
CREATE_SF_APP_IMAGE_LEGACY="${ROOT_DIR}/scripts/deploy/create_secretflow_app_image.sh"
SHUNIT2_PATH="${ROOT_DIR}/tests/vendor/shunit2"

oneTimeSetUp() {
  # Capture PATH after shunit2 has added its lifecycle stub directory.
  ORIGINAL_PATH="${PATH}"
}

setUp() {
  TEST_TMP="$(mktemp -d)"
  mkdir -p "${TEST_TMP}/bin"

  cat > "${TEST_TMP}/bin/kubectl" <<'FAKE_KUBECTL'
#!/bin/bash
cat > "${KUBECTL_CAPTURE:?KUBECTL_CAPTURE is not set}"
FAKE_KUBECTL
  chmod +x "${TEST_TMP}/bin/kubectl"

  export KUBECTL_CAPTURE="${TEST_TMP}/appimage.yaml"
  export PATH="${TEST_TMP}/bin:${ORIGINAL_PATH}"
}

tearDown() {
  export PATH="${ORIGINAL_PATH}"
  unset KUBECTL_CAPTURE
  rm -rf "${TEST_TMP}"
}

run_capture() {
  LAST_OUTPUT="$("$@" 2>&1)" && LAST_STATUS=0 || LAST_STATUS=$?
}

testCreateSfRequiresArguments() {
  run_capture "${CREATE_SF_APP_IMAGE}"
  assertEquals "missing arguments should fail" 1 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "missing argument"
}

testCreateSfRendersSecretflowTemplate() {
  run_capture "${CREATE_SF_APP_IMAGE}" "secretflow/secretflow-anolis8" "1.10.0b0" "secretflow"
  assertEquals "create_sf_app_image.sh failed: ${LAST_OUTPUT}" 0 "${LAST_STATUS}"

  local rendered
  rendered="$(cat "${KUBECTL_CAPTURE}")"
  assertContains "${rendered}" "kind: AppImage"
  assertContains "${rendered}" "name: secretflow-image"
  assertContains "${rendered}" "name: 'secretflow/secretflow-anolis8'"
  assertContains "${rendered}" "tag: '1.10.0b0'"
  assertNotContains "${rendered}" "{{.SF_IMAGE_NAME}}"
  assertNotContains "${rendered}" "{{.SF_IMAGE_TAG}}"
  assertNotContains "${rendered}" "{{.SF_IMAGE_ID}}"
  assertNotContains "${rendered}" "{{.SF_NAME}}"
}

testCreateSfSupportsCustomName() {
  export SF_NAME="custom-app-image"
  run_capture "${CREATE_SF_APP_IMAGE}" "secretflow/secretflow-anolis8" "1.10.0b0" "secretflow"
  unset SF_NAME

  assertEquals "create_sf_app_image.sh failed: ${LAST_OUTPUT}" 0 "${LAST_STATUS}"
  assertContains "$(cat "${KUBECTL_CAPTURE}")" "name: custom-app-image"
}

testCreateSfRendersPsiTemplate() {
  run_capture "${CREATE_SF_APP_IMAGE}" "secretflow/psi-anolis8" "latest" "psi" "sha256:abc123"
  assertEquals "create_sf_app_image.sh failed: ${LAST_OUTPUT}" 0 "${LAST_STATUS}"

  local rendered
  rendered="$(cat "${KUBECTL_CAPTURE}")"
  assertContains "${rendered}" "kind: AppImage"
  assertContains "${rendered}" "name: psi-image"
  assertContains "${rendered}" "id: 'sha256:abc123'"
  assertContains "${rendered}" "name: 'secretflow/psi-anolis8'"
  assertNotContains "${rendered}" "{{.SF_IMAGE_NAME}}"
  assertNotContains "${rendered}" "{{.SF_NAME}}"
}

testLegacyInfersSecretflowType() {
  run_capture "${CREATE_SF_APP_IMAGE_LEGACY}" "secretflow/secretflow-anolis8:1.0.0"
  assertEquals "create_secretflow_app_image.sh failed: ${LAST_OUTPUT}" 0 "${LAST_STATUS}"

  local rendered
  rendered="$(cat "${KUBECTL_CAPTURE}")"
  assertContains "${rendered}" "name: secretflow-image"
  assertContains "${rendered}" "name: 'secretflow/secretflow-anolis8'"
  assertContains "${rendered}" "tag: '1.0.0'"
  assertNotContains "${rendered}" "{{.SF_NAME}}"
}

testLegacyInfersPsiType() {
  run_capture "${CREATE_SF_APP_IMAGE_LEGACY}" "secretflow/psi-anolis8:latest"
  assertEquals "create_secretflow_app_image.sh failed: ${LAST_OUTPUT}" 0 "${LAST_STATUS}"
  assertContains "$(cat "${KUBECTL_CAPTURE}")" "name: psi-image"
}

testLegacyRejectsImageWithoutTag() {
  run_capture "${CREATE_SF_APP_IMAGE_LEGACY}" "secretflow/secretflow-anolis8"
  assertEquals "tagless image should be rejected" 1 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "expected format repo:tag"
}

# shellcheck source=/dev/null
. "${SHUNIT2_PATH}"
