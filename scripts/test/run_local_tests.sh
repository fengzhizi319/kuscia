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

# Offline checks and unit tests for shell scripts.
#
# This runner does not need Docker or a running Kuscia cluster. It performs:
#   1. `bash -n` syntax checks for every script under scripts/ and hack/;
#   2. shellcheck (when installed);
#   3. shunit2 based unit tests under scripts/test/unit/.
#
# Usage examples:
#   bash scripts/test/run_local_tests.sh
#   bash scripts/test/run_local_tests.sh --lint-only
#   bash scripts/test/run_local_tests.sh --filter app_image

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd -P)"
SHUNIT2_PATH="${ROOT_DIR}/tests/vendor/shunit2"

RUN_STATIC=true
RUN_SHELLCHECK=true
RUN_UNIT=true
FILTER=""
VERBOSE=false

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Run offline checks and unit tests for shell scripts.

Options:
  --lint-only        Only run syntax checks and shellcheck.
  --unit-only        Only run unit tests under scripts/test/unit/.
  --no-shellcheck    Skip shellcheck even when it is installed.
  --filter PATTERN   Only run unit test files whose path contains PATTERN.
  -v, --verbose      Print every file being checked.
  -h, --help         Show this help text.

Environment:
  The runner uses the bundled shunit2 at tests/vendor/shunit2, so no extra
  test framework is required.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --lint-only)
      RUN_UNIT=false
      ;;
    --unit-only)
      RUN_STATIC=false
      RUN_SHELLCHECK=false
      ;;
    --no-shellcheck)
      RUN_SHELLCHECK=false
      ;;
    --filter)
      FILTER="${2:?--filter requires a value}"
      shift
      ;;
    -v | --verbose)
      VERBOSE=true
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

collect_script_files() {
  local dir
  for dir in "${ROOT_DIR}/scripts" "${ROOT_DIR}/hack"; do
    [[ -d "${dir}" ]] || continue
    find "${dir}" -type f -name '*.sh' -print
  done | sort
}

# Shellcheck uses the same scope as `make shell-check` (one directory level
# below scripts/ and hack/) plus the unit tests, so this gate stays in sync
# with CI and does not fail on files the project does not lint yet.
collect_shellcheck_files() {
  local dir
  for dir in "${ROOT_DIR}/scripts" "${ROOT_DIR}/hack"; do
    [[ -d "${dir}" ]] || continue
    find "${dir}" -mindepth 2 -maxdepth 2 -type f -name '*.sh' -print
  done
  find "${ROOT_DIR}/scripts/test/unit" -maxdepth 1 -type f -name '*.sh' -print
}

mapfile -t SCRIPT_FILES < <(collect_script_files)
mapfile -t SHELLCHECK_FILES < <(collect_shellcheck_files | sort)

if [[ ${#SCRIPT_FILES[@]} -eq 0 ]]; then
  echo "no shell scripts found under ${ROOT_DIR}" >&2
  exit 1
fi

STATIC_FAILURES=0
UNIT_PASSED=0
UNIT_FAILED=0

if [[ "${RUN_STATIC}" == true ]]; then
  echo "==> Syntax check (${#SCRIPT_FILES[@]} files)"
  for file in "${SCRIPT_FILES[@]}"; do
    [[ "${VERBOSE}" == true ]] && echo "  bash -n ${file#"${ROOT_DIR}"/}"
    if ! bash -n "${file}"; then
      echo "syntax error: ${file}" >&2
      STATIC_FAILURES=$((STATIC_FAILURES + 1))
    fi
  done
fi

if [[ "${RUN_SHELLCHECK}" == true ]]; then
  if command -v shellcheck >/dev/null 2>&1; then
    echo "==> shellcheck (${#SHELLCHECK_FILES[@]} files)"
    if [[ ${#SHELLCHECK_FILES[@]} -eq 0 ]]; then
      echo "no files matched the shellcheck scope" >&2
      STATIC_FAILURES=$((STATIC_FAILURES + 1))
    elif ! shellcheck -e SC1091,SC2034 "${SHELLCHECK_FILES[@]}"; then
      STATIC_FAILURES=$((STATIC_FAILURES + 1))
    fi
  else
    echo "==> shellcheck not found, skipped"
  fi
fi

run_unit_tests() {
  if [[ ! -f "${SHUNIT2_PATH}" ]]; then
    echo "shunit2 not found at ${SHUNIT2_PATH}" >&2
    UNIT_FAILED=$((UNIT_FAILED + 1))
    return
  fi

  local -a test_files=()
  local file
  for file in "${SCRIPT_DIR}"/unit/test_*.sh; do
    [[ -e "${file}" ]] || continue
    if [[ -n "${FILTER}" && "${file}" != *"${FILTER}"* ]]; then
      continue
    fi
    test_files+=("${file}")
  done

  if [[ ${#test_files[@]} -eq 0 ]]; then
    echo "==> No unit tests matched filter '${FILTER}'" >&2
    return
  fi

  echo "==> Unit tests (${#test_files[@]} files)"
  for file in "${test_files[@]}"; do
    echo "--- ${file#"${ROOT_DIR}"/}"
    if bash "${file}"; then
      UNIT_PASSED=$((UNIT_PASSED + 1))
    else
      UNIT_FAILED=$((UNIT_FAILED + 1))
    fi
  done
}

if [[ "${RUN_UNIT}" == true ]]; then
  run_unit_tests
fi

echo
echo "============================================================"
echo "scripts test summary"
echo "  static failures: ${STATIC_FAILURES}"
echo "  unit files passed: ${UNIT_PASSED}"
echo "  unit files failed: ${UNIT_FAILED}"
echo "============================================================"

if [[ "${STATIC_FAILURES}" -gt 0 || "${UNIT_FAILED}" -gt 0 ]]; then
  exit 1
fi
