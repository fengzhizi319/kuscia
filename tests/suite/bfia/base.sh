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

set -e

echo "test suite env: TEST_SUITE_RUN_ROOT_DIR=${TEST_SUITE_RUN_ROOT_DIR}"
echo "test suite env: TEST_BIN_DIR=${TEST_BIN_DIR}"

TEST_SUITE_BFIA_TEST_RUN_DIR=${TEST_SUITE_RUN_ROOT_DIR}/bfia

mkdir -p "${TEST_SUITE_BFIA_TEST_RUN_DIR}"

. ./test/suite/core/functions.sh

function oneTimeSetUp() {
  start_bfia "${TEST_SUITE_BFIA_TEST_RUN_DIR}"
  sleep 10
}

function oneTimeTearDown() {
  stop_bfia
}

function test_bfia_job() {
  local job_id=job-ic-ecdh
  
  # Create BFIA job via HTTP API inside alice container
  docker exec -it ${AUTONOMY_ALICE_CONTAINER} bash -c 'curl -X POST "http://127.0.0.1:8084/v1/interconn/schedule/job/create" \
    --header "Content-Type: application/json" \
    -d "{
      \"job_id\": \"'${job_id}'\",
      \"dag\": {
        \"version\": \"1.0.0\",
        \"components\": [{
          \"code\": \"ic-ecdh\",
          \"name\": \"ic_psi_ecdh_1\",
          \"module_name\": \"ic-ecdh\",
          \"componentName\": \"ic-ecdh\",
          \"provider\": \"morse\",
          \"version\": \"1.0.0\",
          \"input\": [],
          \"output\": [
            {\"type\": \"dataset\", \"key\": \"data\"},
            {\"type\": \"report\", \"key\": \"summary\"}
          ]
        }]
      },
      \"config\": {
        \"role\": {
          \"host\": [\"bob\"],
          \"guest\": [\"alice\"]
        },
        \"initiator\": {
          \"role\": \"guest\",
          \"node_id\": \"alice\"
        },
        \"job_params\": {
          \"common\": {\"sync_type\": \"poll\"},
          \"guest\": {\"0\": {\"resources\": {\"cpu\": -1, \"memory\": -1, \"disk\": -1}}},
          \"host\": {\"0\": {\"resources\": {\"cpu\": -1, \"memory\": -1, \"disk\": -1}}},
          \"arbiter\": {}
        },
        \"task_params\": {
          \"host\": {
            \"0\": {
              \"ic_psi_ecdh_1\": {
                \"rank\": 1,
                \"field_names\": \"id\",
                \"name\": \"breast_hetero_host.csv\",
                \"namespace\": \"data\"
              }
            }
          },
          \"arbiter\": {},
          \"guest\": {
            \"0\": {
              \"ic_psi_ecdh_1\": {
                \"namespace\": \"data\",
                \"name\": \"breast_hetero_guest.csv\",
                \"rank\": 0,
                \"field_names\": \"id\"
              }
            }
          },
          \"common\": {
            \"ic_psi_ecdh_1\": {
              \"result_to_rank\": -1,
              \"algo\": \"ecdh_psi\",
              \"protocol_families\": \"ecc\",
              \"curve_type\": \"curve25519\",
              \"hash_type\": \"sha_256\",
              \"hash2curve_strategy\": \"direct_hash_as_point_x\",
              \"point_octet_format\": \"uncompressed\",
              \"bit_length_after_truncated\": -1
            }
          }
        },
        \"version\": \"1.0.0\"
      }
    }"'
  
  # Wait for job completion using the new job ID format
  assertEquals "Kuscia job failed" "Succeeded" "$(wait_kuscia_job_until "${AUTONOMY_ALICE_CONTAINER}" 600 ${job_id})"

  # Get kusciatask name by kusciajob
  local kt_name=$(docker exec -it ${AUTONOMY_ALICE_CONTAINER} kubectl get kt -n cross-domain -o jsonpath='{.items[?(@.metadata.annotations.kuscia\.secretflow/job-id=="'${job_id}'")].metadata.name}')
  
  # Verify output files exist (adjust path based on new job structure)
  assertEquals "Kuscia report file exist" "Y" "$(exist_container_file "${AUTONOMY_ALICE_CONTAINER}" var/storage/${job_id}-guest-0/${kt_name}-data)"
  
  unset job_id kt_name
}

. ./test/vendor/shunit2
