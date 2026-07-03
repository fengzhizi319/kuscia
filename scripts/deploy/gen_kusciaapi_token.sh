#!/bin/bash

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

# 颜色输出控制：用于让脚本的提示信息更易读。
# GREEN：成功/正常输出；RED：错误提示；NC：恢复终端默认颜色。
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

# 第一个参数：域 ID，用于参与签名生成 token。
# 第二个参数：域私钥的 base64 编码内容，脚本会先解码再用于签名。
DOMAIN_ID=$1
DOMAIN_KEY_DATA=$2

# 参数校验：脚本必须且只能接收两个参数，避免由于输入不完整导致生成错误 token。
if [ "$#" -ne 2 ]; then
    echo -e "${RED}Please run the script in this format: $0 \${domainID} \${domainKeyData}${NC}"
    exit 1
fi

# 将 base64 编码的私钥内容解码后写入临时文件。
# 之所以写入文件，是因为 openssl dgst -sign 需要从文件读取私钥。
echo "$DOMAIN_KEY_DATA" | base64 -d > /tmp/domain_key

# 使用域 ID 作为待签名消息，利用域私钥生成 SHA-256 签名。
# 签名结果会先输出到临时文件，后续再做 base64 编码。
echo -n "$DOMAIN_ID" | openssl dgst -sha256 -sign /tmp/domain_key -out /tmp/signture_file

# 将二进制签名结果转成 base64 文本，便于后续截取和输出。
base64 < /tmp/signture_file > /tmp/signture_file_base64

# token 取签名 base64 编码结果的前 32 个字符。
# 注意：这里的实现与服务端约定保持一致，不能随意修改截取长度。
TOKEN=$(head -c 32 /tmp/signture_file_base64)

# 以绿色输出最终 token，方便用户在终端中快速识别。
echo -e "${GREEN}$TOKEN${NC}"

# 清理临时文件，避免敏感材料（私钥、签名中间结果）残留在 /tmp 中。
rm -rf /tmp/domain_key /tmp/signture_file /tmp/signture_file_base64