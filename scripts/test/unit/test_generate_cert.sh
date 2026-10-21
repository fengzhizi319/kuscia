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

# Unit tests for generate_rsa_key.sh and generate_cert.sh.
#
# Certificates are generated in a throw-away directory and only validated
# with the local openssl binary, so these tests run fully offline.

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT_DIR="$(cd "${TEST_DIR}/../../.." && pwd -P)"
GENERATE_RSA_KEY="${ROOT_DIR}/scripts/deploy/generate_rsa_key.sh"
GENERATE_CERT="${ROOT_DIR}/scripts/deploy/generate_cert.sh"
SHUNIT2_PATH="${ROOT_DIR}/tests/vendor/shunit2"

setUp() {
  TEST_TMP="$(mktemp -d)"
}

tearDown() {
  rm -rf "${TEST_TMP}"
}

run_capture() {
  LAST_OUTPUT="$("$@" 2>&1)" && LAST_STATUS=0 || LAST_STATUS=$?
}

run_in_tmp() {
  LAST_OUTPUT="$(cd "${TEST_TMP}" && "$@" 2>&1)" && LAST_STATUS=0 || LAST_STATUS=$?
}

require_openssl() {
  if ! command -v openssl >/dev/null 2>&1; then
    startSkipping
    return 1
  fi
  return 0
}

strip_ansi() {
  printf '%s\n' "$1" | sed $'s/\033\\[[0-9;]*m//g'
}

testGenerateRsaKeyEmitsBase64PrivateKey() {
  if ! require_openssl; then
    return 0
  fi

  run_capture bash "${GENERATE_RSA_KEY}"
  assertEquals "generate_rsa_key.sh failed: ${LAST_OUTPUT}" 0 "${LAST_STATUS}"

  local cleaned key_b64
  cleaned="$(strip_ansi "${LAST_OUTPUT}")"
  assertContains "${cleaned}" "Generate domain private key configuration"

  key_b64="$(printf '%s\n' "${cleaned}" | sed -n '3p')"
  assertNotNull "base64 key line is missing" "${key_b64}"

  printf '%s' "${key_b64}" | base64 -d > "${TEST_TMP}/key.pem"
  assertTrue "decoded output should be a PEM private key" "grep -q 'BEGIN .*PRIVATE KEY' '${TEST_TMP}/key.pem'"

  if openssl pkey -in "${TEST_TMP}/key.pem" -noout >/dev/null 2>&1; then
    key_status=0
  else
    key_status=$?
  fi
  assertEquals "openssl pkey should accept the generated key" 0 "${key_status}"
}

testGenerateCertRequiresDomainId() {
  run_capture bash "${GENERATE_CERT}"
  assertEquals "missing DOMAIN_ID should fail" 1 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "missing argument: DOMAIN_ID"
}

testGenerateCertRequiresKeyData() {
  run_capture bash "${GENERATE_CERT}" alice
  assertEquals "missing DOMAIN_KEY_DATA should fail" 1 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "missing argument: DOMAIN_KEY_DATA"
}

testGenerateCertCreatesDomainCertificate() {
  if ! require_openssl; then
    return 0
  fi

  local key_data cert_output
  key_data="$(openssl genrsa 2048 2>/dev/null | base64 | tr -d '\n')"
  assertNotNull "failed to generate a test RSA key" "${key_data}"

  run_in_tmp bash "${GENERATE_CERT}" alice "${key_data}"
  cert_output="${LAST_OUTPUT}"
  assertEquals "generate_cert.sh failed: ${cert_output}" 0 "${LAST_STATUS}"

  assertTrue "private key file should exist" "[[ -s '${TEST_TMP}/alice.domain.key' ]]"
  assertTrue "CSR file should exist" "[[ -s '${TEST_TMP}/alice.domain.csr' ]]"
  assertTrue "certificate file should exist" "[[ -s '${TEST_TMP}/alice.domain.crt' ]]"
  assertContains "${cert_output}" "BEGIN CERTIFICATE"

  run_in_tmp openssl x509 -in alice.domain.crt -noout -subject
  assertEquals "openssl x509 failed: ${LAST_OUTPUT}" 0 "${LAST_STATUS}"
  assertContains "${LAST_OUTPUT}" "alice"
}

# shellcheck source=/dev/null
. "${SHUNIT2_PATH}"
