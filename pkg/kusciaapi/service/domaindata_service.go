// Copyright 2023 Ant Group Co., Ltd.
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

//nolint:dupl
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"github.com/secretflow/kuscia/pkg/common"
	cmservice "github.com/secretflow/kuscia/pkg/confmanager/service"
	"github.com/secretflow/kuscia/pkg/crd/apis/kuscia/v1alpha1"
	"github.com/secretflow/kuscia/pkg/datamesh/dataserver/io/builtin"
	"github.com/secretflow/kuscia/pkg/datamesh/metaserver/service"
	"github.com/secretflow/kuscia/pkg/kusciaapi/config"
	"github.com/secretflow/kuscia/pkg/kusciaapi/constants"
	"github.com/secretflow/kuscia/pkg/kusciaapi/errorcode"
	"github.com/secretflow/kuscia/pkg/utils/nlog"
	"github.com/secretflow/kuscia/pkg/utils/resources"
	"github.com/secretflow/kuscia/pkg/utils/tls"
	consts "github.com/secretflow/kuscia/pkg/web/constants"
	"github.com/secretflow/kuscia/pkg/web/utils"
	pbv1alpha1 "github.com/secretflow/kuscia/proto/api/v1alpha1"
	"github.com/secretflow/kuscia/proto/api/v1alpha1/confmanager"
	pberrorcode "github.com/secretflow/kuscia/proto/api/v1alpha1/errorcode"
	"github.com/secretflow/kuscia/proto/api/v1alpha1/kusciaapi"
)

// IDomainDataService 定义了 DomainData(数据资产)元数据管理的对外服务接口。
// DomainData 是 kuscia 中对"一份数据"的元数据抽象(指向具体数据源中的文件/表等),
// 其本身以 k8s 自定义资源(CRD)的形式存储,namespace 对应 domain_id。
type IDomainDataService interface {
	// CreateDomainData 创建一条 DomainData;若指定的 domaindata_id 已存在,则退化为更新语义。
	CreateDomainData(ctx context.Context, request *kusciaapi.CreateDomainDataRequest) *kusciaapi.CreateDomainDataResponse
	// UpdateDomainData 更新一条已存在的 DomainData(支持部分字段更新)。
	UpdateDomainData(ctx context.Context, request *kusciaapi.UpdateDomainDataRequest) *kusciaapi.UpdateDomainDataResponse
	// DeleteDomainData 只删除 DomainData 元数据,不处理底层原始数据。
	DeleteDomainData(ctx context.Context, request *kusciaapi.DeleteDomainDataRequest) *kusciaapi.DeleteDomainDataResponse
	// DeleteDomainDataAndRaw 删除 DomainData 元数据,同时清理其指向的原始数据(文件/对象/数据库表)。
	DeleteDomainDataAndRaw(ctx context.Context, request *kusciaapi.DeleteDomainDataRequest) *kusciaapi.DeleteDomainDataResponse
	// QueryDomainData 按 domain_id + domaindata_id 查询单条详情。
	QueryDomainData(ctx context.Context, request *kusciaapi.QueryDomainDataRequest) *kusciaapi.QueryDomainDataResponse
	// BatchQueryDomainData 按多个 domain_id + domaindata_id 批量查询,容忍部分不存在。
	BatchQueryDomainData(ctx context.Context, request *kusciaapi.BatchQueryDomainDataRequest) *kusciaapi.BatchQueryDomainDataResponse
	// ListDomainData 按 domain_id(可选按类型/vendor 过滤)列出该 domain 下所有 DomainData。
	ListDomainData(ctx context.Context, request *kusciaapi.ListDomainDataRequest) *kusciaapi.ListDomainDataResponse
}

// domainDataService 是 IDomainDataService 的默认实现。
// conf 提供 KusciaClient(访问 apiserver)、RunMode/Initiator(lite 模式判断)、DomainKey(解密密钥)等依赖;
// configService 用于从 config manager 查询数据源连接信息(可为空,表示不支持该能力)。
type domainDataService struct {
	conf          *config.KusciaAPIConfig
	configService cmservice.IConfigService
}

// NewDomainDataService 构造 domainDataService 实例。
func NewDomainDataService(config *config.KusciaAPIConfig, configService cmservice.IConfigService) IDomainDataService {
	return &domainDataService{
		conf:          config,
		configService: configService,
	}
}

// CreateDomainData 创建一条 DomainData(数据资产)记录。
//
// 执行逻辑:
//  1. 基础字段校验:domain_id、type、relative_uri 不能为空,domain_id 需符合 k8s 命名规范。
//  2. lite 模式下校验:若当前是 kuscia lite 部署,只能操作自己 domain 下的数据。
//  3. 若请求携带了 domaindata_id,先按 k8s 规范校验其命名,再去 apiserver 查询是否已存在:
//     - 若已存在,则退化为"更新"语义,调用 UpdateDomainData 并转换响应返回(即 Create 接口的幂等/upsert 行为)。
//  4. 鉴权:authHandler 校验调用方(domain 角色)是否只操作自己的 DomainData。
//  5. 归一化请求参数:填充 name、domaindata_id、datasource_id、vendor 默认值,并处理 relative_uri 前导斜杠。
//  6. 若指定了 datasource_id,校验对应的 DomainDataSource 是否存在。
//  7. 组装 k8s 自定义资源 DomainData(打上类型、vendor、互联协议等 label,以及 initiator 注解)。
//  8. 调用 KusciaClient 在 apiserver 中创建该 DomainData 资源。
//  9. 创建成功返回 domaindata_id;创建失败则转换为 kusciaapi 错误码返回。
func (s domainDataService) CreateDomainData(ctx context.Context, request *kusciaapi.CreateDomainDataRequest) *kusciaapi.CreateDomainDataResponse {
	// 1. 校验 domain_id 不能为空
	if request.DomainId == "" {
		return &kusciaapi.CreateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, "request domain id can not be empty"),
		}
	}
	// 校验 domain_id 是否符合 k8s 资源命名规范(会作为 namespace 使用)
	if err := resources.ValidateK8sName(request.DomainId, "domain_id"); err != nil {
		return &kusciaapi.CreateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, err.Error()),
		}
	}
	// 2. 校验数据类型 type 不能为空
	if request.Type == "" {
		return &kusciaapi.CreateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, "request type can not be empty"),
		}
	}
	// 3. 校验相对路径 relative_uri 不能为空
	if request.RelativeUri == "" {
		return &kusciaapi.CreateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, "request relative_uri can not be empty"),
		}
	}
	// 4. lite 模式下,只允许操作 initiator 自己的 domain 数据
	if err := s.validateRequestWhenLite(request); err != nil {
		return &kusciaapi.CreateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, err.Error()),
		}
	}

	// 5. 若显式指定了 domaindata_id,先检查该资源是否已存在,存在则走"更新"逻辑,实现 upsert 语义
	if request.DomaindataId != "" {
		// 先做 k8s 命名合法性校验(会作为资源 name 使用)
		if err := resources.ValidateK8sName(request.DomaindataId, "domaindata_id"); err != nil {
			return &kusciaapi.CreateDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, err.Error()),
			}
		}
		domainData, err := s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(request.DomainId).Get(ctx, request.DomaindataId, metav1.GetOptions{})
		if err == nil && domainData != nil {
			// 资源已存在:转换为 UpdateDomainData 请求并复用更新逻辑,再把响应转换回 Create 响应格式
			resp := s.UpdateDomainData(ctx, convert2UpdateReq(request))
			return convert2CreateResp(resp, request.DomaindataId)
		}
	}
	// 6. 鉴权:domain 角色只能操作自己的 DomainData
	if err := s.authHandler(ctx, request); err != nil {
		return &kusciaapi.CreateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrAuthFailed, err.Error()),
		}
	}
	// 7. 归一化请求:补全 name/domaindata_id/datasource_id/vendor 默认值,规整 relative_uri
	s.normalizationCreateRequest(request)

	// 8. 若指定了数据源,需要确认该数据源在当前 domain 下确实存在
	if len(request.DatasourceId) > 0 {
		kusciaErrorCode, msg := CheckDomainDataSourceExists(s.conf.KusciaClient, request.DomainId, request.DatasourceId)

		if pberrorcode.ErrorCode_SUCCESS != kusciaErrorCode {
			return &kusciaapi.CreateDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(kusciaErrorCode, msg),
			}
		}
	}

	// vendor 以请求传入的值为准(经过归一化后已有默认值兜底)
	customVendor := request.Vendor

	// 9. 组装 k8s 资源上的 label,用于后续按类型/vendor/互联协议筛选查询
	Labels := map[string]string{
		common.LabelDomainDataType:        request.Type,
		common.LabelDomainDataVendor:      customVendor,
		common.LabelInterConnProtocolType: "kuscia",
	}

	// 标记该 DomainData 的发起方(创建者所属 domain)
	annotations := map[string]string{
		common.InitiatorAnnotationKey: request.DomainId,
	}

	// 10. 构造 DomainData 自定义资源对象
	kusciaDomainData := &v1alpha1.DomainData{
		ObjectMeta: metav1.ObjectMeta{
			Name:        request.DomaindataId,
			Labels:      Labels,
			Annotations: annotations,
		},
		Spec: v1alpha1.DomainDataSpec{
			RelativeURI: request.RelativeUri,
			Name:        request.Name,
			Type:        request.Type,
			DataSource:  request.DatasourceId,
			Attributes:  request.Attributes,
			Partition:   common.Convert2KubePartition(request.Partition),
			Columns:     common.Convert2KubeColumn(request.Columns),
			Vendor:      customVendor,
			Author:      request.DomainId,
			FileFormat:  common.Convert2KubeFileFormat(request.FileFormat),
		},
	}
	// 11. 调用 apiserver 在指定 domain(namespace) 下创建该 DomainData 资源
	_, err := s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(request.DomainId).Create(ctx, kusciaDomainData, metav1.CreateOptions{})
	if err != nil {
		nlog.Errorf("CreateDomainData failed, error: %s", err.Error())
		return &kusciaapi.CreateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.CreateDomainDataErrorCode(err, pberrorcode.ErrorCode_KusciaAPIErrCreateDomainDataFailed), err.Error()),
		}
	}
	// 12. 创建成功,返回最终生效的 domaindata_id
	return &kusciaapi.CreateDomainDataResponse{
		Status: utils.BuildSuccessResponseStatus(),
		Data: &kusciaapi.CreateDomainDataResponseData{
			DomaindataId: request.DomaindataId,
		},
	}
}

// UpdateDomainData 更新一条已存在的 DomainData 记录。
//
// 执行逻辑:
//  1. 基础校验:domaindata_id、domain_id 不能为空。
//  2. lite 模式校验:只能操作自己 domain 下的数据。
//  3. 鉴权:domain 角色只能操作自己的 DomainData。
//  4. 从 apiserver 获取原始 DomainData 资源(作为 merge patch 的基准)。
//  5. 归一化更新请求:请求中未传的字段,用原始资源里的值兜底填充(实现"部分更新"语义)。
//  6. 若变更了 datasource_id,校验新的数据源是否存在。
//  7. 构造"期望状态"的 DomainData 对象(修改后的 label 及 spec)。
//  8. 用 MergeDomainData 计算出 JSON merge patch(原始对象 vs 修改后对象的差异)。
//  9. 调用 apiserver Patch 接口,将差异应用到该资源上。
//  10. 返回成功或失败响应。
func (s domainDataService) UpdateDomainData(ctx context.Context, request *kusciaapi.UpdateDomainDataRequest) *kusciaapi.UpdateDomainDataResponse {
	// 1. 基础字段校验
	if request.DomaindataId == "" || request.DomainId == "" {
		return &kusciaapi.UpdateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, "domain id and domaindata id can not be empty"),
		}
	}

	// 2. lite 模式下只能操作自己 domain 的数据
	if err := s.validateRequestWhenLite(request); err != nil {
		return &kusciaapi.UpdateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, err.Error()),
		}
	}
	// 3. 鉴权
	if err := s.authHandler(ctx, request); err != nil {
		return &kusciaapi.UpdateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrAuthFailed, err.Error()),
		}
	}
	// 4. 获取原始资源,用于后续 merge patch 计算
	originalDomainData, err := s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(request.DomainId).Get(ctx, request.DomaindataId, metav1.GetOptions{})
	if err != nil {
		nlog.Errorf("UpdateDomainData failed, error: %s", err.Error())
		return &kusciaapi.UpdateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.GetDomainDataErrorCode(err, pberrorcode.ErrorCode_KusciaAPIErrGetDomainDataFailed), err.Error()),
		}
	}

	// 5. 请求中未携带的字段,使用原始资源的值兜底,避免"部分更新"把其他字段清空
	s.normalizationUpdateRequest(request, originalDomainData.Spec)
	// 6. 若变更了数据源,需要校验新数据源是否存在
	if len(request.DatasourceId) > 0 {
		kusciaErrorCode, msg := CheckDomainDataSourceExists(s.conf.KusciaClient, request.DomainId, request.DatasourceId)

		if pberrorcode.ErrorCode_SUCCESS != kusciaErrorCode {
			return &kusciaapi.UpdateDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(kusciaErrorCode, msg),
			}
		}
	}

	// 7. 复制原有 label,再覆盖类型和 vendor,构造"期望状态"的 label
	labels := make(map[string]string)
	for key, value := range originalDomainData.Labels {
		labels[key] = value
	}

	// vendor 以请求传入的值为准(经过归一化后已有默认值兜底)
	customVendor := request.Vendor

	labels[common.LabelDomainDataType] = request.Type
	labels[common.LabelDomainDataVendor] = customVendor
	// 构造修改后的 DomainData 对象,ResourceVersion 保持与原始资源一致,便于乐观锁校验
	modifiedDomainData := &v1alpha1.DomainData{
		ObjectMeta: metav1.ObjectMeta{
			Name:            request.DomaindataId,
			Labels:          labels,
			ResourceVersion: originalDomainData.ResourceVersion,
		},
		Spec: v1alpha1.DomainDataSpec{
			RelativeURI: request.RelativeUri,
			Name:        request.Name,
			Type:        request.Type,
			DataSource:  request.DatasourceId,
			Attributes:  request.Attributes,
			Partition:   common.Convert2KubePartition(request.Partition),
			Columns:     common.Convert2KubeColumn(request.Columns),
			Vendor:      customVendor,
			Author:      request.DomainId,
			FileFormat:  common.Convert2KubeFileFormat(request.FileFormat),
		},
	}
	// 8. 计算 original -> modified 的 JSON merge patch
	patchBytes, originalBytes, modifiedBytes, err := service.MergeDomainData(originalDomainData, modifiedDomainData)
	if err != nil {
		nlog.Errorf("Merge DomainData failed, request: %+v,error: %s.",
			request, err.Error())
		return &kusciaapi.UpdateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrMergeDomainDataFailed, err.Error()),
		}
	}
	nlog.Debugf("Update DomainData request: %+v, patchBytes: %s, originalDomainData: %s, modifiedDomainData: %s.",
		request, patchBytes, originalBytes, modifiedBytes)
	// 9. 将 merge patch 应用到 apiserver 中的资源上
	_, err = s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(originalDomainData.Namespace).Patch(ctx, originalDomainData.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		nlog.Debugf("Patch DomainData failed, request: %+v, patchBytes: %s, originalDomainData: %s, modifiedDomainData: %s, error: %s.",
			request, patchBytes, originalBytes, modifiedBytes, err.Error())
		return &kusciaapi.UpdateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.GetDomainDataErrorCode(err, pberrorcode.ErrorCode_KusciaAPIErrPatchDomainDataFailed), err.Error()),
		}
	}
	// 10. 更新成功
	return &kusciaapi.UpdateDomainDataResponse{
		Status: utils.BuildSuccessResponseStatus(),
	}
}

// deleteLocalFsFile 删除本地文件系统中的原始数据文件。
//
// 执行逻辑:
//  1. 安全校验:base path 必须位于允许的本地数据源根目录(DefaultDomainDataSourceLocalFSPath)下,
//     防止误配置导致越权删除任意目录。
//  2. 安全校验:对 relativeUri 做 Clean 处理,禁止出现 ".." 路径穿越,防止跳出 base path 删除其他文件。
//  3. 拼接得到目标文件的绝对路径,检查文件是否存在:
//     - 存在则删除;
//     - 不存在则忽略(视为已删除,不报错);
//     - 其他 stat 错误则返回错误。
func deleteLocalFsFile(path, relativeUri string) error {
	// Validate the base path to ensure it is within the allowed directory
	if !strings.Contains(path, common.DefaultDomainDataSourceLocalFSPath) {
		return fmt.Errorf("invalid base path: %s, must be within %s", path, common.DefaultDomainDataSourceLocalFSPath)
	}

	// Validate the relativeUri to ensure it does not escape the base path
	cleanRelativeUri := filepath.Clean(relativeUri)
	if strings.HasPrefix(cleanRelativeUri, "..") || strings.Contains(cleanRelativeUri, "../") {
		return fmt.Errorf("invalid relative URI: %s, must not escape the base path", relativeUri)
	}
	var err error
	// Check if the file exists
	filePath := filepath.Join(path, relativeUri)
	if _, err = os.Stat(filePath); err == nil {
		if err = os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to delete file %s: %v", filePath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check file %s: %v", filePath, err)
	}
	nlog.Infof("File %s deleted successfully", filePath)
	return nil
}

// deleteOSSFile 删除对象存储(S3 兼容协议,如 OSS/MinIO)中的原始数据文件。
//
// 执行逻辑:
//  1. 使用 AK/SK 构造 AWS S3 session(S3ForcePathStyle 由 virtualHost 取反决定,兼容路径风格/虚拟主机风格两种寻址方式)。
//  2. 先用 HeadObject 探测对象是否存在:
//     - 若返回 404,说明对象不存在,视为已删除,直接返回成功;
//     - 若是其他错误,返回错误。
//  3. 调用 DeleteObject 删除该对象。
func deleteOSSFile(accessKey, secretKey, endpoint, bucketName, prefix, relativeURI string, virtualHost bool) error {

	nlog.Debugf("Open oss remote endpoint(%s), bucket(%s), relativeURI(%s)", endpoint, bucketName, relativeURI)
	// Create a new session
	sess, err := session.NewSession(&aws.Config{
		Credentials:      credentials.NewStaticCredentials(accessKey, secretKey, ""),
		Endpoint:         &endpoint,
		Region:           aws.String("us-west-2"),
		S3ForcePathStyle: pointer.Bool(!virtualHost),
	})

	if err != nil {
		nlog.Errorf("OSS(%s) create session failed with error: %s", endpoint, err.Error())
		return fmt.Errorf("failed create oss %s session failed with error: %s", endpoint, err.Error())
	}
	client := s3.New(sess)

	// Check if the object exists
	_, err = client.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(filepath.ToSlash(filepath.Join(prefix, relativeURI))),
	})
	if err != nil {
		if aerr, ok := err.(s3.RequestFailure); ok && aerr.StatusCode() == 404 {
			nlog.Infof("Object %s does not exist in bucket %s", path.Join(prefix, relativeURI), bucketName)
			return nil
		}
		nlog.Errorf("Error occurred while checking existence of object %s in bucket %s: %v", path.Join(prefix, relativeURI), bucketName, err)
		return fmt.Errorf("failed to check existence of object %s in bucket %s: %v", path.Join(prefix, relativeURI), bucketName, err)
	}

	// Delete the object from the bucket
	_, err = client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(filepath.ToSlash(filepath.Join(prefix, relativeURI))),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object %s from bucket %s: %v", path.Join(prefix, relativeURI), bucketName, err)
	}
	nlog.Infof("Successfully deleted OSS file: %s", relativeURI)
	return nil
}

// deleteMysqlTable 删除 MySQL 数据源中对应的原始数据表(DomainData 以表形式存储的场景)。
//
// 执行逻辑:
//  1. 拼接 DSN 并打开数据库连接(使用 database/sql + mysql 驱动)。
//  2. 执行 `DROP TABLE IF EXISTS` 删除对应的表,relativeURI 即表名。
func deleteMysqlTable(user, passwd, endpoint, database, relativeURI string) error {
	nlog.Debugf("Open MySQL Session database(%s), user(%s)", database, user)

	// Build the connection string
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s", user, passwd, endpoint, database)

	// Open a connection to the database
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Drop the table
	query := fmt.Sprintf("DROP TABLE IF EXISTS `%s`", relativeURI)
	_, err = db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to drop table %s: %v", relativeURI, err)
	}

	nlog.Infof("Successfully dropped table: %s", relativeURI)
	return nil
}

// deletePostgresqlTable 删除 PostgreSQL 数据源中对应的原始数据表。
//
// 执行逻辑:
//  1. 解析 endpoint,兼容 "host:port?params" 形式,拆出连接参数(params)、host、port
//     (若 endpoint 中缺省端口,则退回默认 Postgresql 端口)。
//  2. 拼接 DSN 并打开数据库连接(使用 database/sql + postgres 驱动)。
//  3. 执行 `DROP TABLE IF EXISTS` 删除对应的表,relativeURI 即表名。
func deletePostgresqlTable(user, passwd, endpoint, database, relativeURI string) error {
	nlog.Debugf("Open Postgresql Session database(%s), user(%s)", database, user)
	params := ""
	// Replace 127.0.0.1:5432?sslmode=disable&xxx=xxx with ? Split the host, port, and link string parameters
	if strings.Contains(endpoint, "?") {
		params = endpoint[strings.Index(endpoint, "?")+1:]
		endpoint = endpoint[:strings.Index(endpoint, "?")]
	}

	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		if addrErr, ok := err.(*net.AddrError); ok && addrErr.Err == "missing port in address" {
			host = endpoint
			port = builtin.PostgresqlPort
			err = nil
		} else {
			nlog.Errorf("Endpoint \"%s\" can't be resolved with net.SplitHostPort()", endpoint)
			return err
		}
	}

	dsn := ""
	if params != "" {
		params = strings.ReplaceAll(params, "&", " ")
		dsn = fmt.Sprintf("user=%s password=%s host=%s dbname=%s port=%s %s", user, passwd, host, database, port, params)
	} else {
		dsn = fmt.Sprintf("user=%s password=%s host=%s dbname=%s port=%s", user, passwd, host, database, port)
	}

	// Open a connection to the database
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Drop the table
	query := fmt.Sprintf("DROP TABLE IF EXISTS \"%s\"", relativeURI)
	_, err = db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to drop table %s: %v", relativeURI, err)
	}

	nlog.Infof("Successfully dropped table: %s", relativeURI)
	return nil
}

// getDsInfoByKey 通过 config manager 提供的 key,查询并解码数据源连接信息(如 AK/SK、账号密码等)。
//
// 执行逻辑:
//  1. 若未注入 configService,直接返回错误(说明当前环境不支持该能力)。
//  2. 调用 configService.QueryConfig 按 key 查询配置内容。
//  3. 查询失败(非成功状态码)时返回错误。
//  4. 查询成功后,按 sourceType 解码出具体的 DataSourceInfo 结构体。
func (s domainDataService) getDsInfoByKey(ctx context.Context, sourceType string, infoKey string) (*kusciaapi.DataSourceInfo, error) {
	if s.configService == nil {
		return nil, fmt.Errorf("cm config service is empty, skip get datasource info by key")
	}

	response := s.configService.QueryConfig(ctx, &confmanager.QueryConfigRequest{Key: infoKey})
	if !utils.IsSuccessCode(response.Status.Code) {
		nlog.Errorf("Query info key failed, code: %d, message: %s", response.Status.Code, response.Status.Message)
		return nil, fmt.Errorf("query info key failed: %v", response.Status.Message)
	}
	info, err := decodeDataSourceInfo(sourceType, response.Value)
	if err != nil {
		nlog.Errorf("Decode datasource info for key %s failed: %v", infoKey, err)
	}
	return info, err
}

// getDomainDataSourceById 根据 namespace(domain) 和 datasourceId 从 apiserver 获取 DomainDataSource 资源。
//
// 执行逻辑:
//  1. 调用 KusciaClient 查询指定资源。
//  2. 若资源不存在,返回带有明确语义的"数据源不存在"错误。
//  3. 其他错误统一包装后返回。
func (s domainDataService) getDomainDataSourceById(namespace string, datasourceId string) (*v1alpha1.DomainDataSource, error) {
	domaindatasource, err := s.conf.KusciaClient.KusciaV1alpha1().DomainDataSources(namespace).Get(context.Background(), datasourceId, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("domain %v data source %v doesn't exist", namespace, datasourceId)
		}
		return nil, fmt.Errorf("query DataSource %s of DomainData fail: %v", datasourceId, err)
	}
	return domaindatasource, nil
}

// decryptInfo 使用 domain 私钥(OAEP)解密 DomainDataSource 中加密存储的连接信息,并反序列化为 DataSourceInfo。
//
// 执行逻辑:
//  1. 用 s.conf.DomainKey 对 cipherInfo 做 RSA-OAEP 解密,得到明文 JSON 字节。
//  2. 将明文 JSON 反序列化为 kusciaapi.DataSourceInfo 结构体。
func (s domainDataService) decryptInfo(cipherInfo string) (*kusciaapi.DataSourceInfo, error) {
	plaintext, err := tls.DecryptOAEP(s.conf.DomainKey, cipherInfo)
	if err != nil {
		return nil, fmt.Errorf("decrypt data source info failed, %v", err)
	}
	info := &kusciaapi.DataSourceInfo{}
	if err = json.Unmarshal(plaintext, info); err != nil {
		return nil, fmt.Errorf("unmarshal data source info failed, %v", err)
	}
	return info, nil
}

// DeleteDomainDataAndRaw 删除 DomainData 元数据记录,同时删除其对应的原始数据(文件/对象/数据库表)。
//
// 执行逻辑:
//  1. 基础校验:domaindata_id、domain_id 不能为空;lite 模式只能操作自己的数据;鉴权。
//  2. 记录一条 warn 级别的审计日志,标记这是一次破坏性操作。
//  3. 从 apiserver 获取该 DomainData,取出其 datasource_id 和 relative_uri(原始数据的定位信息)。
//  4. 根据 datasource_id 查询对应的 DomainDataSource 资源(不存在则报错)。
//  5. 解密数据源连接信息:
//     - 若数据源配置了 InfoKey,说明连接信息存放在 config manager,通过 getDsInfoByKey 查询解码;
//     - 否则,连接信息以加密形式内嵌在数据源资源的 Data 字段中,通过 decryptInfo 用 domain 私钥解密。
//  6. 根据数据源类型(localfs/oss/mysql/postgresql)分别调用对应的删除函数清理原始数据;
//     不支持的类型直接返回错误,不做任何删除。
//  7. 原始数据删除成功后,再删除 apiserver 中的 DomainData 元数据记录。
//  8. 任一步骤失败都会提前返回错误响应,只有全部成功才返回成功响应。
func (s domainDataService) DeleteDomainDataAndRaw(ctx context.Context, request *kusciaapi.DeleteDomainDataRequest) *kusciaapi.DeleteDomainDataResponse {
	var err error
	// 1. 基础字段校验
	if request.DomaindataId == "" || request.DomainId == "" {
		return &kusciaapi.DeleteDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, "domain id and domaindata id can not be empty"),
		}
	}
	// lite 模式下只能操作自己 domain 的数据
	if err = s.validateRequestWhenLite(request); err != nil {
		return &kusciaapi.DeleteDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, err.Error()),
		}
	}
	// 鉴权
	if err = s.authHandler(ctx, request); err != nil {
		return &kusciaapi.DeleteDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrAuthFailed, err.Error()),
		}
	}
	// 2. 记录审计日志(带原始数据删除,属于破坏性/不可逆操作)
	nlog.Warnf("Delete domainID: %s domainDataID: %s", request.DomainId, request.DomaindataId)
	// 3. 获取 DomainData 元数据,拿到数据源 id 与相对路径
	domainData, err := s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(request.DomainId).Get(ctx, request.DomaindataId, metav1.GetOptions{})
	if err != nil {
		nlog.Errorf("Failed to get DomainData with ID %s: %v", request.DomaindataId, err)
		return &kusciaapi.DeleteDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.GetDomainDataErrorCode(err, pberrorcode.ErrorCode_KusciaAPIErrGetDomainDataFailed), err.Error()),
		}
	}

	// Extract datasourceId and relativeUri from the DomainData object
	datasourceId := domainData.Spec.DataSource
	relativeUri := domainData.Spec.RelativeURI

	// 4. 查询数据源资源
	domaindatasource, err := s.getDomainDataSourceById(domainData.Namespace, datasourceId)

	if err != nil {
		return &kusciaapi.DeleteDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.GetDomainDataSourceErrorCode(err, pberrorcode.ErrorCode_KusciaAPIErrDomainDataSourceNotExists), err.Error()),
		}
	}
	var domaindatasourceInfo, info *kusciaapi.DataSourceInfo
	// 5. 解密/解码数据源连接信息(两种存储方式二选一)
	if len(domaindatasource.Spec.InfoKey) != 0 {
		// 方式一:连接信息托管在 config manager,按 key 查询
		info, err = s.getDsInfoByKey(ctx, domaindatasource.Spec.Type, domaindatasource.Spec.InfoKey)
		if err != nil {
			nlog.Errorf("Failed to get DomainDataSource info by key %s: %v", domaindatasource.Spec.InfoKey, err)
			return &kusciaapi.DeleteDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrDeleteDomainDataFailed, err.Error()),
			}
		}
		domaindatasourceInfo = info
	} else {
		// 方式二:连接信息以加密形式内嵌在数据源资源的 Data 字段中
		encryptedContent, exist := domaindatasource.Spec.Data[encryptedInfo]
		if !exist {
			nlog.Errorf("DomainDataSource %s does not have encrypted info", datasourceId)
			return &kusciaapi.DeleteDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrDeleteDomainDataFailed, "DomainDataSource does not have encrypted info"),
			}
		}
		// 用 domain 私钥解密
		domaindatasourceInfo, err = s.decryptInfo(encryptedContent)
		if err != nil {
			nlog.Errorf("Failed to decrypt DomainDataSource info: %v", err)
			return &kusciaapi.DeleteDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrDeleteDomainDataFailed, err.Error()),
			}
		}
	}
	// 6. 按数据源类型分发到对应的原始数据删除逻辑
	switch domaindatasource.Spec.Type {
	case "localfs":
		if err = deleteLocalFsFile(domaindatasourceInfo.Localfs.Path, relativeUri); err != nil {
			nlog.Errorf("Failed to delete local file at %s %s: %v", domaindatasourceInfo.Localfs.Path, relativeUri, err)
			return &kusciaapi.DeleteDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrDeleteDomainDataFailed, err.Error()),
			}
		}
	case "oss":
		if err = deleteOSSFile(domaindatasourceInfo.Oss.AccessKeyId, domaindatasourceInfo.Oss.AccessKeySecret, domaindatasourceInfo.Oss.Endpoint, domaindatasourceInfo.Oss.Bucket, domaindatasourceInfo.Oss.Prefix, relativeUri, domaindatasourceInfo.Oss.Virtualhost); err != nil {
			nlog.Errorf("Failed to delete OSS file at %s: %v", relativeUri, err)
			return &kusciaapi.DeleteDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrDeleteDomainDataFailed, err.Error()),
			}
		}
	case "mysql":
		if err = deleteMysqlTable(domaindatasourceInfo.Database.User, domaindatasourceInfo.Database.Password, domaindatasourceInfo.Database.Endpoint, domaindatasourceInfo.Database.Database, relativeUri); err != nil {
			nlog.Errorf("Failed to delete MySQL table at %s: %v", relativeUri, err)
			return &kusciaapi.DeleteDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrDeleteDomainDataFailed, err.Error()),
			}
		}

	case "postgresql":
		if err = deletePostgresqlTable(domaindatasourceInfo.Database.User, domaindatasourceInfo.Database.Password, domaindatasourceInfo.Database.Endpoint, domaindatasourceInfo.Database.Database, relativeUri); err != nil {
			nlog.Errorf("Failed to delete PostgreSQL table at %s: %v", relativeUri, err)
			return &kusciaapi.DeleteDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrDeleteDomainDataFailed, err.Error()),
			}
		}
	default:
		// 不支持的数据源类型:不做任何删除,直接报错
		nlog.Warnf("Unsupported domainDataSource type: %s", domaindatasource.Spec.Type)
		return &kusciaapi.DeleteDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrDeleteDomainDataFailed, "unsupported domainDataSource type"),
		}
	}
	// 7. 原始数据清理成功后,再删除 DomainData 元数据记录
	err = s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(request.DomainId).Delete(ctx, request.DomaindataId, metav1.DeleteOptions{})
	if err != nil {
		nlog.Errorf("Delete domainData: %s failed, detail: %s", request.DomaindataId, err.Error())
		return &kusciaapi.DeleteDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.GetDomainDataErrorCode(err, pberrorcode.ErrorCode_KusciaAPIErrDeleteDomainDataFailed), err.Error()),
		}
	}
	// 8. 全部成功
	return &kusciaapi.DeleteDomainDataResponse{
		Status: utils.BuildSuccessResponseStatus(),
	}
}

// DeleteDomainData 仅删除 DomainData 元数据记录,不处理底层原始数据(与 DeleteDomainDataAndRaw 相对)。
//
// 执行逻辑:
//  1. 基础校验:domaindata_id、domain_id 不能为空。
//  2. lite 模式校验、鉴权。
//  3. 记录审计日志。
//  4. 直接调用 apiserver 删除该 DomainData 资源。
func (s domainDataService) DeleteDomainData(ctx context.Context, request *kusciaapi.DeleteDomainDataRequest) *kusciaapi.DeleteDomainDataResponse {
	// 1. 基础字段校验
	if request.DomaindataId == "" || request.DomainId == "" {
		return &kusciaapi.DeleteDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, "domain id and domaindata id can not be empty"),
		}
	}
	// 2. lite 模式校验
	if err := s.validateRequestWhenLite(request); err != nil {
		return &kusciaapi.DeleteDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, err.Error()),
		}
	}
	// 鉴权
	if err := s.authHandler(ctx, request); err != nil {
		return &kusciaapi.DeleteDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrAuthFailed, err.Error()),
		}
	}
	// 3. 审计日志
	nlog.Warnf("Delete domainID: %s domainDataID: %s", request.DomainId, request.DomaindataId)
	// 4. 只删除 apiserver 中的元数据资源,原始数据不受影响
	err := s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(request.DomainId).Delete(ctx, request.DomaindataId, metav1.DeleteOptions{})
	if err != nil {
		nlog.Errorf("Delete domainData: %s failed, detail: %s", request.DomaindataId, err.Error())
		return &kusciaapi.DeleteDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.GetDomainDataErrorCode(err, pberrorcode.ErrorCode_KusciaAPIErrDeleteDomainDataFailed), err.Error()),
		}
	}
	return &kusciaapi.DeleteDomainDataResponse{
		Status: utils.BuildSuccessResponseStatus(),
	}
}

// QueryDomainData 按 domain_id + domaindata_id 查询单条 DomainData 详情。
//
// 执行逻辑:
//  1. 基础校验:domain_id、domaindata_id 不能为空。
//  2. lite 模式校验、鉴权。
//  3. 从 apiserver 获取该资源。
//  4. 将 k8s DomainData 对象转换为 kusciaapi.DomainData 对外响应结构体返回。
func (s domainDataService) QueryDomainData(ctx context.Context, request *kusciaapi.QueryDomainDataRequest) *kusciaapi.QueryDomainDataResponse {
	// 1. 基础字段校验
	if request.Data == nil || request.Data.DomaindataId == "" || request.Data.DomainId == "" {
		return &kusciaapi.QueryDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, "domain id and domaindata id can not be empty"),
		}
	}
	// 2. lite 模式校验
	if err := s.validateRequestWhenLite(request.Data); err != nil {
		return &kusciaapi.QueryDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, err.Error()),
		}
	}
	// 鉴权
	if err := s.authHandler(ctx, request.Data); err != nil {
		return &kusciaapi.QueryDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrAuthFailed, err.Error()),
		}
	}
	// 3. 查询 apiserver 中的资源
	kusciaDomainData, err := s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(request.Data.DomainId).Get(ctx, request.Data.DomaindataId, metav1.GetOptions{})
	if err != nil {
		nlog.Errorf("QueryDomainData failed, error: %s", err.Error())
		return &kusciaapi.QueryDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.GetDomainDataErrorCode(err, pberrorcode.ErrorCode_KusciaAPIErrGetDomainDataFailed), err.Error()),
		}
	}
	// 4. 转换为对外响应结构体
	return &kusciaapi.QueryDomainDataResponse{
		Status: utils.BuildSuccessResponseStatus(),
		Data: &kusciaapi.DomainData{
			DomaindataId: kusciaDomainData.Name,
			DomainId:     kusciaDomainData.Namespace,
			Name:         kusciaDomainData.Spec.Name,
			Type:         kusciaDomainData.Spec.Type,
			RelativeUri:  kusciaDomainData.Spec.RelativeURI,
			DatasourceId: kusciaDomainData.Spec.DataSource,
			Attributes:   kusciaDomainData.Spec.Attributes,
			Partition:    common.Convert2PbPartition(kusciaDomainData.Spec.Partition),
			Columns:      common.Convert2PbColumn(kusciaDomainData.Spec.Columns),
			Vendor:       kusciaDomainData.Spec.Vendor,
			Status:       constants.DomainDataStatusAvailable,
			Author:       kusciaDomainData.Spec.Author,
			FileFormat:   common.Convert2PbFileFormat(kusciaDomainData.Spec.FileFormat),
		},
	}
}

// BatchQueryDomainData 批量查询多条 DomainData 详情(按 domain_id + domaindata_id 列表)。
//
// 执行逻辑:
//  1. 遍历请求列表,逐条做基础字段校验(domain_id/domaindata_id 不能为空)、lite 模式校验、鉴权;
//     任一条不合法则整体请求失败返回。
//  2. 逐条从 apiserver 查询对应的 DomainData:
//     - 若某条不存在(NotFound),跳过该条,不影响其他条目的结果(容忍部分缺失);
//     - 其他错误则整体失败返回。
//  3. 将查询到的资源转换为响应结构体列表返回。
func (s domainDataService) BatchQueryDomainData(ctx context.Context, request *kusciaapi.BatchQueryDomainDataRequest) *kusciaapi.BatchQueryDomainDataResponse {
	// 1. 逐条校验请求
	for _, v := range request.Data {
		if v.GetDomainId() == "" || v.GetDomaindataId() == "" {
			return &kusciaapi.BatchQueryDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, "domain id and domaindata id can not be empty"),
			}
		}
		// check the request when this is kuscia lite api
		if err := s.validateRequestWhenLite(v); err != nil {
			return &kusciaapi.BatchQueryDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, err.Error()),
			}
		}
		// auth pre handler
		if err := s.authHandler(ctx, v); err != nil {
			return &kusciaapi.BatchQueryDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrAuthFailed, err.Error()),
			}
		}
	}
	// 2. 逐条查询,组装结果列表
	respDatas := make([]*kusciaapi.DomainData, 0, len(request.Data))
	for _, v := range request.Data {
		kusciaDomainData, err := s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(v.DomainId).Get(ctx, v.DomaindataId, metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				// 该条不存在,跳过,不阻断整体查询
				continue
			}
			nlog.Errorf("QueryDomainData failed, error: %s", err.Error())
			return &kusciaapi.BatchQueryDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrGetDomainDataFailed, err.Error()),
			}
		}
		domainData := kusciaapi.DomainData{
			DomaindataId: kusciaDomainData.Name,
			DomainId:     kusciaDomainData.Namespace,
			Name:         kusciaDomainData.Spec.Name,
			Type:         kusciaDomainData.Spec.Type,
			RelativeUri:  kusciaDomainData.Spec.RelativeURI,
			DatasourceId: kusciaDomainData.Spec.DataSource,
			Attributes:   kusciaDomainData.Spec.Attributes,
			Partition:    common.Convert2PbPartition(kusciaDomainData.Spec.Partition),
			Columns:      common.Convert2PbColumn(kusciaDomainData.Spec.Columns),
			Vendor:       kusciaDomainData.Spec.Vendor,
			Status:       constants.DomainDataStatusAvailable,
			Author:       kusciaDomainData.Spec.Author,
			FileFormat:   common.Convert2PbFileFormat(kusciaDomainData.Spec.FileFormat),
		}
		respDatas = append(respDatas, &domainData)
	}
	// 3. 返回聚合结果
	return &kusciaapi.BatchQueryDomainDataResponse{
		Status: utils.BuildSuccessResponseStatus(),
		Data: &kusciaapi.DomainDataList{
			DomaindataList: respDatas,
		},
	}
}

// ListDomainData 按 domain_id 列出该 domain 下的 DomainData,支持按类型/vendor 过滤。
//
// 执行逻辑:
//  1. 基础校验:domain_id 不能为空;lite 模式校验、鉴权。
//  2. 根据请求的 domaindata_type、domaindata_vendor 构造 k8s label selector(可组合 AND)。
//  3. 调用 apiserver List 接口按 selector 查询该 namespace 下的所有 DomainData
//     (TODO: 尚未支持分页 limit/continue,一次性拉取全部)。
//  4. 将查询结果逐条转换为响应结构体列表返回。
func (s domainDataService) ListDomainData(ctx context.Context, request *kusciaapi.ListDomainDataRequest) *kusciaapi.ListDomainDataResponse {
	// 1. 基础字段校验
	if request.Data == nil || request.Data.DomainId == "" {
		return &kusciaapi.ListDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, "domain id can not be empty"),
		}
	}
	// lite 模式校验
	if err := s.validateRequestWhenLite(request.Data); err != nil {
		return &kusciaapi.ListDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrRequestValidate, err.Error()),
		}
	}
	// 鉴权
	if err := s.authHandler(ctx, request.Data); err != nil {
		return &kusciaapi.ListDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrAuthFailed, err.Error()),
		}
	}
	// 2. 按类型/vendor 构造 label selector,支持组合过滤
	var (
		selector    fields.Selector
		selectorStr string
	)
	if request.Data.DomaindataType != "" {
		typeSelector := fields.OneTermEqualSelector(common.LabelDomainDataType, request.Data.DomaindataType)
		selector = typeSelector
		selectorStr = selector.String()
	}
	if request.Data.DomaindataVendor != "" {
		vendorSelector := fields.OneTermEqualSelector(common.LabelDomainDataVendor, request.Data.DomaindataVendor)
		if selector != nil {
			selector = fields.AndSelectors(selector, vendorSelector)
		} else {
			selector = vendorSelector
		}
		selectorStr = selector.String()
	}
	// 3. 调用 apiserver 列出符合条件的资源
	// todo support limit and continue
	dataList, err := s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(request.Data.DomainId).List(ctx, metav1.ListOptions{
		TypeMeta:       metav1.TypeMeta{},
		LabelSelector:  selectorStr,
		TimeoutSeconds: nil,
		Limit:          0,
		Continue:       "",
	})
	if err != nil {
		nlog.Errorf("List DomainData failed, error: %s", err.Error())
		return &kusciaapi.ListDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(pberrorcode.ErrorCode_KusciaAPIErrListDomainDataFailed, err.Error()),
		}
	}
	// 4. 转换为响应结构体列表
	respDatas := make([]*kusciaapi.DomainData, len(dataList.Items))
	for i, v := range dataList.Items {
		domaindata := kusciaapi.DomainData{
			DomaindataId: v.Name,
			DomainId:     v.Namespace,
			Name:         v.Spec.Name,
			Type:         v.Spec.Type,
			RelativeUri:  v.Spec.RelativeURI,
			DatasourceId: v.Spec.DataSource,
			Attributes:   v.Spec.Attributes,
			Partition:    common.Convert2PbPartition(v.Spec.Partition),
			Columns:      common.Convert2PbColumn(v.Spec.Columns),
			Vendor:       v.Spec.Vendor,
			Status:       constants.DomainDataStatusAvailable,
			Author:       v.Spec.Author,
			FileFormat:   common.Convert2PbFileFormat(v.Spec.FileFormat),
		}
		respDatas[i] = &domaindata
	}
	return &kusciaapi.ListDomainDataResponse{
		Status: utils.BuildSuccessResponseStatus(),
		Data: &kusciaapi.DomainDataList{
			DomaindataList: respDatas,
		},
	}
}

// convert2UpdateReq 将 CreateDomainDataRequest 转换为 UpdateDomainDataRequest,
// 用于 CreateDomainData 中检测到资源已存在时,复用 UpdateDomainData 的逻辑(实现 upsert)。
func convert2UpdateReq(createReq *kusciaapi.CreateDomainDataRequest) (updateReq *kusciaapi.UpdateDomainDataRequest) {
	updateReq = &kusciaapi.UpdateDomainDataRequest{
		Header:       createReq.Header,
		DomainId:     createReq.DomainId,
		DomaindataId: createReq.DomaindataId,
		Name:         createReq.Name,
		Type:         createReq.Type,
		RelativeUri:  createReq.RelativeUri,
		DatasourceId: createReq.DatasourceId,
		Attributes:   createReq.Attributes,
		Partition:    createReq.Partition,
		Columns:      createReq.Columns,
		FileFormat:   createReq.FileFormat,
	}
	return
}

// convert2CreateResp 将 UpdateDomainDataResponse 转换回 CreateDomainDataResponse,
// 复用其 Status,并补齐 Create 接口特有的 domaindata_id 字段。
func convert2CreateResp(updateResp *kusciaapi.UpdateDomainDataResponse, domainDataID string) (createResp *kusciaapi.CreateDomainDataResponse) {
	createResp = &kusciaapi.CreateDomainDataResponse{
		Status: updateResp.Status,
		Data: &kusciaapi.CreateDomainDataResponseData{
			DomaindataId: domainDataID,
		},
	}
	return
}

// normalizationCreateRequest 对 CreateDomainData 请求做归一化/补全处理。
//
// 执行逻辑:
//  1. name 为空时,取 relative_uri 最后一段路径作为默认 name。
//  2. domaindata_id 为空时,基于 name 生成一个确定性的 id。
//  3. datasource_id 为空时,使用默认数据源 id。
//  4. vendor 为空时,使用默认 vendor。
//  5. 去除 relative_uri 的前导路径分隔符,避免与数据源 path/prefix 拼接时产生错误路径
//     (例如 datasource path="/home/admin"、prefix="/test/"、relativeURI="/data/a.csv" 拼接时,
//     若不去除前导斜杠,filepath.Join 会导致最终路径丢失 prefix 部分)。
func (s domainDataService) normalizationCreateRequest(request *kusciaapi.CreateDomainDataRequest) {
	// normalization domaindata name
	if request.Name == "" {
		uris := strings.Split(request.RelativeUri, "/")
		if len(uris) > 0 {
			request.Name = uris[len(uris)-1]
		}
	}
	// normalization domaindata id
	if request.DomaindataId == "" {
		request.DomaindataId = common.GenDomainDataID(request.Name)
	}
	//fill default datasource id
	if request.DatasourceId == "" {
		request.DatasourceId = common.DefaultDataSourceID
	}
	//fill default vendor
	if request.Vendor == "" {
		request.Vendor = common.DefaultDomainDataVendor
	}
	// truncate slash
	// fix Issue:
	// datasource-path: "/home/admin" ; datasource-prefix: "/test/" ; domainData-relativeURI: "/data/alice.csv" -> "/test/data/alice.csv"
	// os.path.join(path,prefix,uri) would be /data/alice.csv , this is not expect, expect is /home/admin/test/data/alice.csv -> /home/admin/test/data/alice.csv
	// so trim the prefix filepath.Separator
	request.RelativeUri = strings.TrimPrefix(request.RelativeUri, string(filepath.Separator))
}

// normalizationUpdateRequest 对 UpdateDomainData 请求做归一化处理,实现"部分更新"语义:
// 请求中未携带(零值)的字段,用原始资源(data)中的现有值兜底填充,避免覆盖成空值。
func (s domainDataService) normalizationUpdateRequest(request *kusciaapi.UpdateDomainDataRequest, data v1alpha1.DomainDataSpec) {
	if request.Name == "" {
		request.Name = data.Name
	}
	if request.Type == "" {
		request.Type = data.Type
	}
	if request.RelativeUri == "" {
		request.RelativeUri = data.RelativeURI
	}
	if request.DatasourceId == "" {
		request.DatasourceId = data.DataSource
	}
	if len(request.Columns) == 0 {
		request.Columns = common.Convert2PbColumn(data.Columns)
	}
	if request.Partition == nil {
		request.Partition = common.Convert2PbPartition(data.Partition)
	}
	if len(request.Attributes) == 0 {
		request.Attributes = data.Attributes
	}
	if request.Vendor == "" {
		request.Vendor = data.Vendor
	}
	if request.FileFormat == pbv1alpha1.FileFormat_UNKNOWN {
		request.FileFormat = common.Convert2PbFileFormat(data.FileFormat)
	}
}

// authHandler 鉴权处理:从 ctx 中解析出调用方角色和所属 domain。
// 若调用方角色是 domain(而非 master 等更高权限角色),则只允许操作自己 domain 下的资源,
// 请求中的 domain_id 必须与调用方 domain 一致,否则拒绝。
func (s domainDataService) authHandler(ctx context.Context, request RequestWithDomainID) error {
	role, domainID := GetRoleAndDomainFromCtx(ctx)
	if role == consts.AuthRoleDomain && request.GetDomainId() != domainID {
		return fmt.Errorf("domain's kusciaAPI could only operate its own DomainData, request.DomainID must be %s not %s", domainID, request.GetDomainId())
	}
	return nil
}

// validateRequestWhenLite 在 kuscia lite 部署模式下,限制只能操作 initiator 自身所属 domain 的数据,
// 防止 lite 节点越权操作其他 domain 的数据(lite 模式通常只服务单一 domain)。
func (s domainDataService) validateRequestWhenLite(request RequestWithDomainID) error {
	if s.conf.RunMode == common.RunModeLite && request.GetDomainId() != s.conf.Initiator {
		return fmt.Errorf("kuscia lite api could only operate it's own domaindata, the domainid of request must be %s, not %s", s.conf.Initiator, request.GetDomainId())
	}
	return nil
}
