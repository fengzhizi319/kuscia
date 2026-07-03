// Copyright 2024 Ant Group Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// ============================================================================
// 注册节点功能单元测试
// 测试域节点注册、JWT令牌验证、CSR验证等核心功能
// ============================================================================
package controller

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"

	gomonkeyv2 "github.com/agiledragon/gomonkey/v2"

	"github.com/secretflow/kuscia/cmd/kuscia/confloader"
	"github.com/secretflow/kuscia/pkg/gateway/utils"
	"github.com/secretflow/kuscia/pkg/utils/nlog"
	"github.com/secretflow/kuscia/pkg/utils/tls"
	"github.com/secretflow/kuscia/proto/api/v1alpha1/handshake"
)

// ============================================================================
// 全局测试变量
// ============================================================================
var (
	unitTestDeployToken = "FwBvarrLUpfACr00v8AiIbHbFcYguNqvu92XRJ2YysU="  // 测试用部署令牌（Base64编码）
	utAlice             = "alice"                                      // 测试域ID：alice
	utBob               = "bob"                                        // 测试域ID：bob
)

// ============================================================================
// SHA256版本的JWT声明结构
// 用于注册请求的JWT令牌，包含请求内容的哈希值
// ============================================================================
type RegisterJwtClaimsSha256 struct {
	ReqHashSha256 [32]byte `json:"req_hash"`  // 注册请求内容的SHA256哈希值（32字节）
	jwt.RegisteredClaims                        // 嵌入标准JWT声明（过期时间、签发者等）
}

// ============================================================================
// 生成SHA256版本的JWT令牌
// 参数：
//   namespace - 域ID（如"alice"）
//   csrData - CSR数据（PEM格式）
//   prikey - RSA私钥，用于签名JWT
// 返回：
//   req - 注册请求对象
//   token - JWT令牌字符串
//   err - 错误信息
// 功能：
//   1. 创建注册请求（包含域ID、CSR、请求时间）
//   2. 计算请求内容的SHA256哈希
//   3. 创建JWT声明（包含哈希值、过期时间、签发者）
//   4. 使用RSA-SHA256算法签名JWT
// ============================================================================
func generateJwtTokenSha256(namespace, csrData string, prikey *rsa.PrivateKey) (req *handshake.RegisterRequest, token string, err error) {
	// 创建注册请求
	req = &handshake.RegisterRequest{
		DomainId:    namespace,                          // 域ID
		Csr:         base64.StdEncoding.EncodeToString([]byte(csrData)),  // Base64编码的CSR
		RequestTime: int64(time.Now().Nanosecond()),     // 请求时间戳（纳秒）
	}

	// 创建JWT声明
	rjc := &RegisterJwtClaimsSha256{
		ReqHashSha256: getRegisterRequestHashSha256(req),  // 计算请求哈希
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),  // 5分钟后过期
			IssuedAt:  jwt.NewNumericDate(time.Now()),                       // 签发时间
			Issuer:    namespace,  // 签发者=域ID
			Subject:   namespace,  // 主题=域ID
		},
	}
	
	// 使用RSA-SHA256签名JWT
	tokenData := jwt.NewWithClaims(jwt.SigningMethodRS256, rjc)
	token, err = tokenData.SignedString(prikey)
	if err != nil {
		nlog.Errorf("Signed token failed, error: %s.", err.Error())
	}
	return
}

// ============================================================================
// 生成测试密钥对和CSR
// 参数：
//   t - 测试对象
//   namespace - 域ID（作为CSR的Common Name）
// 返回：
//   csr - PEM格式的CSR字符串
//   key - RSA私钥指针
// 功能：
//   1. 生成随机RSA密钥对（2048位）
//   2. 解析密钥数据
//   3. 使用域ID和部署令牌生成CSR
// ============================================================================
func generateTestKey(t *testing.T, namespace string) (csr string, key *rsa.PrivateKey) {
	// 生成密钥数据（Base64编码）
	keyStr, err := tls.GenerateKeyData()
	if err != nil {
		t.Errorf("Generate key data failed, error: %s.", err.Error())
	}
	
	// 解码Base64密钥
	rawKey, _ := base64.StdEncoding.DecodeString(keyStr)
	
	// 解析RSA私钥
	key, err = tls.ParseKey(rawKey, "")
	if err != nil {
		t.Errorf("Parse key data failed, error: %s.", err.Error())
	}
	
	// 生成CSR（包含域ID和部署令牌）
	csr = confloader.GenerateCsrData(namespace, keyStr, unitTestDeployToken)
	return
}

// ============================================================================
// 测试：注册域时服务器不存在的情况
// 场景：模拟HTTP请求，验证在网络不可用时注册流程不会崩溃
// 预期：函数正常返回，不抛出异常
// ============================================================================
func TestRegisterDomain_ServerNotExists(t *testing.T) {
	t.Parallel()  // 并行执行测试，提高测试速度
	
	// 生成测试密钥和CSR
	csr, key := generateTestKey(t, utAlice)

	// Mock HTTP请求函数
	// 目的：避免真实网络调用，仅验证参数传递是否正确
	gomonkeyv2.ApplyFunc(utils.DoHTTPWithRetry, func(i interface{}, out interface{}, hp *utils.HTTPParam, d time.Duration, tm int) error {
		// 验证HTTP方法必须是POST
		assert.Equal(t, http.MethodPost, hp.Method)
		// 验证Kuscia源域ID必须是alice
		assert.Equal(t, utAlice, hp.KusciaSource)
		return nil  // 模拟成功响应
	})

	// 执行注册（由于HTTP被Mock，不会真正发送请求）
	assert.NoError(t, RegisterDomain("alice", "test", csr, key, nil))
}

// ============================================================================
// 测试：验证注册请求的正确性
// 场景：使用正确的域ID、CSR和JWT令牌
// 预期：验证通过，无错误返回
// ============================================================================
func TestVerifyRequest(t *testing.T) {
	t.Parallel()
	
	// 生成测试密钥和CSR（域ID=alice）
	csr, key := generateTestKey(t, utAlice)
	
	// 生成JWT令牌（使用alice的私钥签名）
	req, token, err := generateJwtToken(utAlice, csr, key)
	assert.NoError(t, err, "generateJwtToken failed")
	
	// 验证注册请求（包括JWT签名、CSR内容、域ID匹配等）
	err = verifyRegisterRequest(req, token)
	assert.NoError(t, err, "verifyRegisterRequest failed")
}

// ============================================================================
// 测试：验证CSR的CN（Common Name）与域ID不匹配的情况
// 场景：请求中域ID是alice，但CSR中的CN是bob
// 预期：验证失败，返回错误（防止身份伪造）
// ============================================================================
func TestVerifyCSRcn(t *testing.T) {
	t.Parallel()
	
	// 生成bob域的CSR（但请求声称是alice）
	csr, key := generateTestKey(t, utBob)  // CSR的CN=bob
	
	// 生成JWT令牌（声称来自alice域）
	req, token, err := generateJwtToken(utAlice, csr, key)  // 请求域ID=alice
	assert.NoError(t, err, "generateJwtToken failed")
	
	// 验证注册请求（应该失败，因为CSR的CN与请求域ID不匹配）
	err = verifyRegisterRequest(req, token)
	assert.Error(t, err, "verifyRegisterRequest failed")  // 预期错误
}

// ============================================================================
// 测试：JWT算法兼容性验证
// 场景：验证SHA256和SHA384两种JWT签名算法的互操作性
// 预期：两种算法都能正确验证，支持平滑升级
// ============================================================================
func TestCompatibility(t *testing.T) {
	t.Parallel()
	
	// 生成测试密钥和CSR
	csr, key := generateTestKey(t, utAlice)

	// 测试1：使用SHA384生成JWT，用SHA256验证（向后兼容）
	req, token, err := generateJwtToken(utAlice, csr, key)  // SHA384版本
	assert.NoError(t, err, "generateJwtToken failed")

	err = verifyRegisterRequestSha256(req, token)  // 用SHA256验证
	assert.NoError(t, err, "generateJwtToken failed")

	// 测试2：使用SHA256生成JWT，用SHA384验证（向前兼容）
	req, token, err = generateJwtTokenSha256(utAlice, csr, key)  // SHA256版本
	assert.NoError(t, err, "generateJwtToken failed")
	err = verifyRegisterRequest(req, token)  // 用SHA384验证
	assert.NoError(t, err, "generateJwtToken failed")
}

// ============================================================================
// SHA256版本的注册请求验证函数
// 参数：
//   req - 注册请求对象
//   token - JWT令牌字符串
// 返回：错误信息
// 功能：
//   1. 解析CSR并验证格式
//   2. 验证CSR的CN与请求域ID匹配
//   3. 使用SHA256算法验证JWT签名
// ============================================================================
func verifyRegisterRequestSha256(req *handshake.RegisterRequest, token string) error {
	// Csr in request must be base64 encoded string
	// Raw data must be pem format
	
	// 步骤1：解析CSR（从Base64解码为PEM格式）
	certRequest, err := parseCertRequest(req.Csr)
	if err != nil {
		return fmt.Errorf("parse cert request failed, detail: %s", err.Error())
	}
	
	// 步骤2：验证CSR的CN必须等于域ID（防止身份伪造）
	if err = verifyCSR(certRequest, req.DomainId); err != nil {
		return fmt.Errorf("verify csr failed, detail: %s", err.Error())
	}
	
	// 步骤3：使用JWT验证
	// JWT token must be signed by domain's private key.
	// This handler will verify it by public key in csr.
	if err = verifyJwtTokenSha256(token, certRequest.PublicKey, req); err != nil {
		return fmt.Errorf("verify jwt failed, detail: %s", err.Error())
	}
	return nil
}

// ============================================================================
// SHA256版本的JWT令牌验证函数
// 参数：
//   jwtTokenStr - JWT令牌字符串
//   pubKey - 公钥（从CSR中提取），用于验证JWT签名
//   req - 注册请求对象
// 返回：错误信息
// 功能：
//   1. 解析JWT令牌并提取声明
//   2. 使用公钥验证JWT签名
//   3. 检查令牌是否过期
//   4. 验证请求内容与哈希值匹配（防篡改）
// ============================================================================
func verifyJwtTokenSha256(jwtTokenStr string, pubKey interface{}, req *handshake.RegisterRequest) error {
	// 准备接收JWT声明的结构
	rjc := &RegisterJwtClaimsSha256{}
	
	// 步骤1：解析JWT并验证签名
	// 使用CSR中的公钥验证JWT签名（证明请求确实来自该域）
	jwtToken, err := jwt.ParseWithClaims(jwtTokenStr, rjc, func(token *jwt.Token) (interface{}, error) {
		return pubKey, nil  // 返回公钥用于验证签名
	})
	if err != nil {
		return err
	}
	
	// 步骤2：检查JWT有效性
	if !jwtToken.Valid {
		return fmt.Errorf("%s", "jwt token decrypted fail")
	}
	
	// 步骤3：检查令牌是否过期
	if time.Since(rjc.ExpiresAt.Time) > 0 {
		return fmt.Errorf("%s", "jwt verify error, token expired")
	}
	
	// 步骤4：验证请求内容与哈希值匹配
	// 确保请求在传输过程中未被篡改
	if reflect.DeepEqual(getRegisterRequestHashSha256(req), rjc.ReqHashSha256) {
		return nil  // 哈希匹配，验证通过
	}
	return fmt.Errorf("verify request failed, detail: the request content doesn't match the hash")
}
