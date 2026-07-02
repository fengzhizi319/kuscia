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

set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
KUSCIA_HOME="${KUSCIA_HOME:-${ROOT_DIR}/.local-kuscia}"
MODE="${1:-master}"
DOMAIN_ID="${DOMAIN_ID:-${MODE}}"
PID_FILE="${KUSCIA_HOME}/var/kuscia.pid"

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
Usage: $0 [MODE] [--stop]

Start Kuscia locally without Docker.

Supported modes:
  master   : start a local master node (default)
  autonomy : start a local autonomy node

Environment variables:
  KUSCIA_HOME : working directory for local Kuscia (default: ${ROOT_DIR}/.local-kuscia)
  DOMAIN_ID   : domain ID for the node (default: same as MODE)

Examples:
  $0
  DOMAIN_ID=alice $0 autonomy
  $0 --stop
EOF
}

if [[ "$MODE" == "-h" || "$MODE" == "--help" ]]; then
  usage
  exit 0
fi

# Stop mode
if [[ "$MODE" == "--stop" ]]; then
  if [[ -f "$PID_FILE" ]]; then
    local local_pid
    local_pid="$(cat "$PID_FILE")"
    if kill -0 "$local_pid" >/dev/null 2>&1; then
      log_info "Stopping Kuscia (pid: $local_pid)..."
      kill "$local_pid" || true
      sleep 2
      if kill -0 "$local_pid" >/dev/null 2>&1; then
        log_warn "Force killing Kuscia (pid: $local_pid)..."
        kill -9 "$local_pid" || true
      fi
    else
      log_warn "Kuscia process not running."
    fi
    rm -f "$PID_FILE"
  else
    log_warn "PID file not found, trying to kill by process name..."
    pkill -f "kuscia start -c" 2>/dev/null || true
  fi
  log_info "Kuscia stopped."
  exit 0
fi

case "$MODE" in
  master|autonomy)
    ;;
  *)
    log_error "Unsupported mode: $MODE"
    usage
    exit 1
    ;;
esac

# Check dependencies
for cmd in go make gcc git openssl; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    log_error "$cmd command not found. Please install it first."
    exit 1
  fi
done

log_info "Working directory: ${KUSCIA_HOME}"
log_info "Deployment mode: ${MODE}"
log_info "Domain ID: ${DOMAIN_ID}"

# Create directories
mkdir -p "${KUSCIA_HOME}/bin"
mkdir -p "${KUSCIA_HOME}/etc/conf"
mkdir -p "${KUSCIA_HOME}/var/logs"
mkdir -p "${KUSCIA_HOME}/var/storage"
mkdir -p "${KUSCIA_HOME}/var/certs"
mkdir -p "${KUSCIA_HOME}/crds"

# Build Kuscia if not exists
KUSCIA_BINARY="${ROOT_DIR}/build/apps/kuscia/kuscia"
if [[ ! -f "$KUSCIA_BINARY" ]]; then
  log_info "Building Kuscia binary..."
  cd "$ROOT_DIR"
  make build
fi

cp "$KUSCIA_BINARY" "${KUSCIA_HOME}/bin/"

# Copy CRDs
cp "${ROOT_DIR}/crds/v1alpha1"/*.yaml "${KUSCIA_HOME}/crds/"

# Generate domain key
log_info "Generating domain private key..."
DOMAIN_KEY="$(openssl genrsa 2048 2>/dev/null | base64 | tr -d "\n" && echo)"

# Generate config
CONFIG_FILE="${KUSCIA_HOME}/etc/conf/kuscia.yaml"
log_info "Generating config: ${CONFIG_FILE}"

cat > "$CONFIG_FILE" <<EOF
# Deploy mode
mode: ${MODE}
# Domain ID
domainID: ${DOMAIN_ID}
# Domain RSA private key encoded with base64
domainKeyData: ${DOMAIN_KEY}
# Logging level, INFO、DEBUG、WARN
logLevel: INFO
EOF

if [[ "$MODE" == "autonomy" ]]; then
  cat >> "$CONFIG_FILE" <<EOF

# runc or runk or runp
runtime: runp
EOF
fi

# Start Kuscia
log_info "Starting Kuscia..."
nohup "${KUSCIA_HOME}/bin/kuscia" start -c "$CONFIG_FILE" > "${KUSCIA_HOME}/var/logs/kuscia_stdout.log" 2>&1 &
echo $! > "$PID_FILE"

log_info "Kuscia started with pid: $(cat "$PID_FILE")"
log_info "Logs: ${KUSCIA_HOME}/var/logs/kuscia.log"
log_info "Stdout: ${KUSCIA_HOME}/var/logs/kuscia_stdout.log"
log_info "Stop with: $0 --stop"
