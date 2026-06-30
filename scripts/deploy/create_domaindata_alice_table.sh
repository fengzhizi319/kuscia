#!/bin/bash
#
# Copyright 2023 Ant Group Co., Ltd.
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

# Exit immediately if any command fails
set -e

# Store the first command-line argument as DOMAIN_ID
# This represents the identifier for the domain where the DomainData will be created
DOMAIN_ID=$1

# Define usage message for the script
usage="$(basename "$0") DOMAIN_ID"

# Check if DOMAIN_ID argument was provided
# If not provided, display error message and exit with status 1
if [[ ${DOMAIN_ID} == "" ]]; then
  echo "missing argument: $usage"
  exit 1
fi

# Determine the root directory of the kuscia project
# This navigates to the parent directory of the current script's location
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)

# Read the domaindata template file and replace placeholder with actual domain ID
# The template file contains YAML definition for DomainData resource
# The placeholder {{.DOMAIN_ID}} in the template will be replaced with the actual DOMAIN_ID value
DOMAIN_DATASOURCE_TEMPLATE=$(sed "s/{{.DOMAIN_ID}}/${DOMAIN_ID}/g;" \
  < "${ROOT}/scripts/templates/domaindata_alice_table.yaml")

# Apply the processed YAML template to Kubernetes
# This creates or updates the DomainData resource in the cluster
# The kubectl apply command handles the actual creation/update of the resource
echo "${DOMAIN_DATASOURCE_TEMPLATE}" | kubectl apply -f -