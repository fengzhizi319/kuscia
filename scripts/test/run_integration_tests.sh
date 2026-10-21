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

# Wrapper for the Docker based integration test suites in tests/.
#
# The canonical suites live in tests/suite and are driven by
# tests/integration_test.sh. That entry script expects to run from an empty
# directory (the Makefile uses run/) because it copies test assets out of the
# Kuscia image at runtime.
#
# Usage examples:
#   KUSCIA_IMAGE=secretflow/kuscia:1.2.0b0 bash scripts/test/run_integration_tests.sh
#   bash scripts/test/run_integration_tests.sh --suite center.base
#   bash scripts/test/run_integration_tests.sh --list

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd -P)"
INTEGRATION_ENTRY="${ROOT_DIR}/tests/integration_test.sh"
RUN_DIR="${ROOT_DIR}/run"

SUITE="${TEST_SUITE:-all}"
IMAGE="${KUSCIA_IMAGE:-}"
LIST_ONLY=false

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Run the Docker based Kuscia integration test suites.

Options:
  -s, --suite NAME   Suite to run (default: all, or \$TEST_SUITE).
  -i, --image IMAGE  Kuscia image to test (default: \$KUSCIA_IMAGE).
  -l, --list         List available suites for this platform and exit.
  -h, --help         Show this help text.

Suites:
  center.base, p2p.base, center.example, bfia.base,
  center.nsjail, p2p.nsjail

Note:
  bfia.base is skipped on arm64. The nsjail suites are skipped on WSL by the
  upstream runner. Use 'make integration_test' to build the image first.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -s | --suite)
      SUITE="${2:?--suite requires a value}"
      shift
      ;;
    -i | --image)
      IMAGE="${2:?--image requires a value}"
      shift
      ;;
    -l | --list)
      LIST_ONLY=true
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

SUITES=(center.base p2p.base center.example bfia.base center.nsjail p2p.nsjail)
case "$(uname -s -m)" in
  "Linux aarch64" | "Darwin arm64")
    SUITES=(center.base p2p.base center.example center.nsjail p2p.nsjail)
    ;;
esac

if [[ "${LIST_ONLY}" == true ]]; then
  printf '%s\n' "${SUITES[@]}"
  exit 0
fi

if [[ ! -f "${INTEGRATION_ENTRY}" ]]; then
  echo "integration test entry not found: ${INTEGRATION_ENTRY}" >&2
  exit 1
fi

if [[ -z "${IMAGE}" ]]; then
  echo "KUSCIA_IMAGE is not set." >&2
  echo "Set KUSCIA_IMAGE=secretflow/kuscia:<tag> or run 'make image' first." >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker command not found; integration tests require Docker." >&2
  exit 1
fi

if [[ "${SUITE}" != "all" ]]; then
  matched=false
  for candidate in "${SUITES[@]}"; do
    if [[ "${candidate}" == "${SUITE}" ]]; then
      matched=true
      break
    fi
  done
  if [[ "${matched}" == false ]]; then
    echo "unsupported suite '${SUITE}' for this platform." >&2
    echo "available suites: ${SUITES[*]}" >&2
    exit 1
  fi
fi

if ! docker image inspect "${IMAGE}" >/dev/null 2>&1; then
  echo "image '${IMAGE}' is not available locally." >&2
  echo "Build it with 'make image' or pull it before running integration tests." >&2
  exit 1
fi

mkdir -p "${RUN_DIR}/test"
cp "${INTEGRATION_ENTRY}" "${RUN_DIR}/test/integration_test.sh"
chmod +x "${RUN_DIR}/test/integration_test.sh"

echo "==> Running integration suite: ${SUITE}"
echo "==> Image: ${IMAGE}"
echo "==> Work directory: ${RUN_DIR}"

(
  cd "${RUN_DIR}"
  KUSCIA_IMAGE="${IMAGE}" bash ./test/integration_test.sh "${SUITE}"
)
