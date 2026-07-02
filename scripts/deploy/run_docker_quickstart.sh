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

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

KUSCIA_IMAGE="${KUSCIA_IMAGE:-secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/kuscia:latest}"
MODE="${1:-p2p}"

log_info() {
  echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
  echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
  echo -e "${RED}[ERROR]${NC} $*" >&2
}

usage() {
  cat <<EOF
Usage: $0 [MODE]

Quick start Kuscia cluster in Docker mode.

Supported modes:
  p2p    : peer-to-peer, 2 Autonomy nodes (default)
  center : centralized, 1 Master + 2 Lite nodes
  cxc    : center x center
  cxp    : center x p2p

Environment variables:
  KUSCIA_IMAGE : Kuscia image to use (default: ${KUSCIA_IMAGE})

Examples:
  $0
  $0 center
  KUSCIA_IMAGE=secretflow/kuscia:1.2.0b0 $0 p2p
EOF
}

if [[ "$MODE" == "-h" || "$MODE" == "--help" ]]; then
  usage
  exit 0
fi

case "$MODE" in
  p2p|center|cxc|cxp)
    ;;
  *)
    log_error "Unsupported mode: $MODE"
    usage
    exit 1
    ;;
esac

log_info "Using Kuscia image: ${KUSCIA_IMAGE}"
log_info "Deployment mode: ${MODE}"

# Check Docker
if ! command -v docker >/dev/null 2>&1; then
  log_error "docker command not found. Please install Docker 20.10+ first."
  exit 1
fi

# Pull image
log_info "Pulling Kuscia image..."
docker pull "${KUSCIA_IMAGE}"

# Extract deploy script
SCRIPT_NAME="kuscia.sh"
if [[ -f "${SCRIPT_NAME}" ]]; then
  log_warn "${SCRIPT_NAME} already exists, will overwrite."
fi

log_info "Extracting deploy script from image..."
docker run --rm "${KUSCIA_IMAGE}" cat /home/kuscia/scripts/deploy/kuscia.sh > "${SCRIPT_NAME}"
chmod u+x "${SCRIPT_NAME}"

# Start cluster
log_info "Starting Kuscia cluster in ${MODE} mode..."
"./${SCRIPT_NAME}" "${MODE}"

log_info "Kuscia ${MODE} cluster started successfully."
log_info "You can verify with: docker ps"
log_info "Run example job with: docker exec -it \${USER}-kuscia-autonomy-alice scripts/user/create_example_job.sh"
