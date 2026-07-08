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
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/secretflow/kuscia/pkg/common"
	"github.com/secretflow/kuscia/pkg/crd/apis/kuscia/v1alpha1"
	"github.com/secretflow/kuscia/pkg/datamesh/config"
	"github.com/secretflow/kuscia/pkg/utils/nlog"
	"github.com/secretflow/kuscia/pkg/utils/resources"
	"github.com/secretflow/kuscia/pkg/web/utils"
	pbv1alpha1 "github.com/secretflow/kuscia/proto/api/v1alpha1"
	"github.com/secretflow/kuscia/proto/api/v1alpha1/datamesh"
	"github.com/secretflow/kuscia/proto/api/v1alpha1/errorcode"
)

const (
	dataTypeString = "string"
	dataTypeStr    = "str"
)

// IDomainDataService 定义了管理 DomainData（域名数据）的接口。
// 它包含创建、查询、更新和删除 DomainData 的操作。
type IDomainDataService interface {
	CreateDomainData(ctx context.Context, request *datamesh.CreateDomainDataRequest) *datamesh.CreateDomainDataResponse
	QueryDomainData(ctx context.Context, request *datamesh.QueryDomainDataRequest) *datamesh.QueryDomainDataResponse
	UpdateDomainData(ctx context.Context, request *datamesh.UpdateDomainDataRequest) *datamesh.UpdateDomainDataResponse
	DeleteDomainData(ctx context.Context, request *datamesh.DeleteDomainDataRequest) *datamesh.DeleteDomainDataResponse
}

// domainDataService 实现了 IDomainDataService 接口。
type domainDataService struct {
	conf *config.DataMeshConfig
}

// NewDomainDataService 创建一个 domainDataService 的新实例。
func NewDomainDataService(config *config.DataMeshConfig) IDomainDataService {
	return &domainDataService{
		conf: config,
	}
}

// CreateDomainData 处理创建 DomainData 的请求。
// 执行逻辑如下：
// 1. 如果请求中指定了 DomaindataId，首先校验其是否符合 Kubernetes 资源名称的规范。
// 2. 校验通过后，查询 K8s 集群中是否已存在该 ID 的 DomainData：
//   - 如果已存在，则将本次“创建”请求转换为“更新”请求，并调用 UpdateDomainData 进行更新，随后返回结果。
//
// 3. 对请求参数进行归一化处理（若名称、ID、数据源ID、Vendor 等为空，则设置默认值或自动生成）。
// 4. 获取并校验对应的数据源（DataSource）：
//   - 如果数据源的 AccessDirectly 为 false，则需额外校验数据列的类型是否与数据源类型匹配。
//   - 如果数据源不是文件系统（FS）类型，则将 FileFormat 强制设为 UNKNOWN。
//
// 5. 构建 Kuscia CRD 中的 DomainData 对象，设置相应的 labels、annotations 以及 Spec 内容。
// 6. 调用 Kuscia Client 向 Kubernetes 集群中写入该 DomainData 资源。
func (s domainDataService) CreateDomainData(ctx context.Context, request *datamesh.CreateDomainDataRequest) *datamesh.CreateDomainDataResponse {
	// check whether domainData  is existed
	if request.DomaindataId != "" {
		// do k8s validate
		if err := resources.ValidateK8sName(request.DomaindataId, "domaindata_id"); err != nil {
			return &datamesh.CreateDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(errorcode.ErrorCode_DataMeshErrRequestInvalidate, err.Error()),
			}
		}
		domainData, err := s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(s.conf.KubeNamespace).Get(ctx, request.DomaindataId, metav1.GetOptions{})
		if err == nil && domainData != nil {
			// update domainData
			resp := s.UpdateDomainData(ctx, convert2UpdateReq(request))
			return convert2CreateResp(resp, request.DomaindataId)
		}
	}

	// normalization request
	s.normalizationCreateRequest(request)

	// check datasource
	datasource, err := s.checkDataSource(ctx, request.DatasourceId, request.Columns)
	if err != nil {
		return &datamesh.CreateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.ErrorCode_DataMeshErrGetDomainDataSourceFromKubeFailed, err.Error()),
		}
	}
	if !isFSDataSource(datasource.Spec.Type) {
		request.FileFormat = pbv1alpha1.FileFormat_UNKNOWN
	}

	// build kuscia domain
	Labels := make(map[string]string)
	Labels[common.LabelDomainDataType] = request.Type
	Labels[common.LabelDomainDataVendor] = request.Vendor
	Labels[common.LabelInterConnProtocolType] = "kuscia"

	annotations := make(map[string]string)
	annotations[common.InitiatorAnnotationKey] = request.DomaindataId

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
			Vendor:      request.Vendor,
			Author:      s.conf.KubeNamespace,
			FileFormat:  common.Convert2KubeFileFormat(request.FileFormat),
		},
	}

	// create kuscia domain
	_, err = s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(s.conf.KubeNamespace).Create(ctx, kusciaDomainData, metav1.CreateOptions{})
	if err != nil {
		nlog.Errorf("CreateDomainData failed, error: %s", err.Error())
		return &datamesh.CreateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.ErrorCode_DataMeshErrCreateDomainData, err.Error()),
		}
	}
	return &datamesh.CreateDomainDataResponse{
		Status: utils.BuildSuccessResponseStatus(),
		Data: &datamesh.CreateDomainDataResponseData{
			DomaindataId: request.DomaindataId,
		},
	}
}

// QueryDomainData 处理查询 DomainData 的请求。
// 执行逻辑如下：
// 1. 调用 Kuscia Client 从 Kubernetes 指定 Namespace 中获取与 request.DomaindataId 对应的 DomainData 资源。
// 2. 如果查询失败，返回包含对应错误码的 Status。
// 3. 如果查询成功，将 Kubernetes 资源数据转换并映射为 protobuf 的 datamesh.DomainData 并返回。
func (s domainDataService) QueryDomainData(ctx context.Context, request *datamesh.QueryDomainDataRequest) *datamesh.QueryDomainDataResponse {
	// get kuscia domain
	kusciaDomainData, err := s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(s.conf.KubeNamespace).Get(ctx, request.DomaindataId, metav1.GetOptions{})
	if err != nil {
		nlog.Errorf("QueryDomainData failed, error: %s", err.Error())
		return &datamesh.QueryDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.ErrorCode_DataMeshErrQueryDomainData, err.Error()),
		}
	}
	// build domain response
	return &datamesh.QueryDomainDataResponse{
		Status: utils.BuildSuccessResponseStatus(),
		Data: &datamesh.DomainData{
			DomaindataId: kusciaDomainData.Name,
			Name:         kusciaDomainData.Spec.Name,
			Type:         kusciaDomainData.Spec.Type,
			RelativeUri:  kusciaDomainData.Spec.RelativeURI,
			DatasourceId: kusciaDomainData.Spec.DataSource,
			Attributes:   kusciaDomainData.Spec.Attributes,
			Partition:    common.Convert2PbPartition(kusciaDomainData.Spec.Partition),
			Columns:      common.Convert2PbColumn(kusciaDomainData.Spec.Columns),
			Vendor:       kusciaDomainData.Spec.Vendor,
			FileFormat:   common.Convert2PbFileFormat(kusciaDomainData.Spec.FileFormat),
			Author:       kusciaDomainData.Spec.Author,
		},
	}
}

// UpdateDomainData 处理更新 DomainData 的请求。
// 执行逻辑如下：
// 1. 从 K8s 中获取原有的 DomainData 数据。如果获取失败则直接返回错误。
// 2. 对更新请求参数进行归一化处理。由于更新请求中某些字段可能未设置，此时需使用原有 DomainData 规格中的值进行填充，避免空字段覆盖旧数据。
// 3. 检查数据源是否变更。如果数据源 ID 与旧数据源 ID 不同，则需要重新加载 and 校验数据源，非文件系统（FS）数据源同样强制设置 FileFormat 为 UNKNOWN。
// 4. 拷贝原有的 labels，并根据最新的 Type 和 Vendor 进行更新，构建全新的 modifiedDomainData 资源描述结构，设置 ResourceVersion 为原资源版本号。
// 5. 调用 MergeDomainData 函数计算原始数据与修改数据的差异，并生成 Merge Patch 字节切片。
// 6. 调用 Kuscia Client 的 Patch 方法，通过 MergePatchType 方式局部更新 K8s 集群中的 DomainData。
func (s domainDataService) UpdateDomainData(ctx context.Context, request *datamesh.UpdateDomainDataRequest) *datamesh.UpdateDomainDataResponse {
	// get original domainData from k8s
	originalDomainData, err := s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(s.conf.KubeNamespace).Get(ctx, request.DomaindataId, metav1.GetOptions{})
	if err != nil {
		nlog.Errorf("UpdateDomainData failed, error: %s", err.Error())
		return &datamesh.UpdateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.ErrorCode_DataMeshErrGetDomainDataFromKubeFailed, err.Error()),
		}
	}

	// normalize request
	s.normalizationUpdateRequest(request, originalDomainData.Spec)

	// check DataSource
	if request.DatasourceId != originalDomainData.Spec.DataSource {
		datasource, checkErr := s.checkDataSource(ctx, request.DatasourceId, request.Columns)
		if checkErr != nil {
			nlog.Errorf("Query DataSource %s of DomainData %s fail: %v", request.DatasourceId, request.DomaindataId, checkErr)
			return &datamesh.UpdateDomainDataResponse{
				Status: utils.BuildErrorResponseStatus(errorcode.ErrorCode_DataMeshErrGetDomainDataSourceFromKubeFailed, checkErr.Error()),
			}
		}
		if !isFSDataSource(datasource.Spec.Type) {
			request.FileFormat = pbv1alpha1.FileFormat_UNKNOWN
		}
	}

	// build modified domainData
	labels := make(map[string]string)
	for key, value := range originalDomainData.Labels {
		labels[key] = value
	}
	labels[common.LabelDomainDataType] = request.Type
	labels[common.LabelDomainDataVendor] = request.Vendor

	// build modified domainData
	modifiedDomainData := &v1alpha1.DomainData{
		ObjectMeta: metav1.ObjectMeta{
			Name:            request.DomaindataId,
			ResourceVersion: originalDomainData.ResourceVersion,
			Labels:          labels,
		},
		Spec: v1alpha1.DomainDataSpec{
			RelativeURI: request.RelativeUri,
			Name:        request.Name,
			Type:        request.Type,
			DataSource:  request.DatasourceId,
			Attributes:  request.Attributes,
			Partition:   common.Convert2KubePartition(request.Partition),
			Columns:     common.Convert2KubeColumn(request.Columns),
			Vendor:      request.Vendor,
			FileFormat:  common.Convert2KubeFileFormat(request.FileFormat),
			Author:      s.conf.KubeNamespace,
		},
	}
	// merge modifiedDomainData to originalDomainData
	patchBytes, originalBytes, modifiedBytes, err := MergeDomainData(originalDomainData, modifiedDomainData)
	if err != nil {
		nlog.Errorf("Merge DomainData failed, request: %+v,error: %s.",
			request, err.Error())
		return &datamesh.UpdateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.ErrorCode_DataMeshErrMergeDomainDataFailed, err.Error()),
		}
	}
	nlog.Debugf("Update DomainData request: %+v, patchBytes: %s, originalDomainData: %s, modifiedDomainData: %s",
		request, patchBytes, originalBytes, modifiedBytes)
	// patch the merged domainData
	_, err = s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(originalDomainData.Namespace).Patch(ctx, originalDomainData.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		// todo: retry if conflict
		nlog.Debugf("Patch DomainData failed, request: %+v, patchBytes: %s, originalDomainData: %s, modifiedDomainData: %s, error: %s",
			request, patchBytes, originalBytes, modifiedBytes, err.Error())
		return &datamesh.UpdateDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.ErrorCode_DataMeshErrPatchDomainDataFailed, err.Error()),
		}
	}
	// construct the response
	return &datamesh.UpdateDomainDataResponse{
		Status: utils.BuildSuccessResponseStatus(),
	}
}

// DeleteDomainData 处理删除 DomainData 的请求。
// 执行逻辑如下：
// 1. 打印一条 Warn 级别的日志记录删除操作。
// 2. 调用 Kuscia Client 的 Delete 方法在指定 Namespace 下物理删除该 DomainData 资源。
func (s domainDataService) DeleteDomainData(ctx context.Context, request *datamesh.DeleteDomainDataRequest) *datamesh.DeleteDomainDataResponse {
	// record the delete operation
	nlog.Warnf("Delete domainDataID %s", request.DomaindataId)
	// delete kuscia domainData
	err := s.conf.KusciaClient.KusciaV1alpha1().DomainDatas(s.conf.KubeNamespace).Delete(ctx, request.DomaindataId, metav1.DeleteOptions{})
	if err != nil {
		nlog.Errorf("Delete domainData: %s failed, detail: %s", request.DomaindataId, err.Error())
		return &datamesh.DeleteDomainDataResponse{
			Status: utils.BuildErrorResponseStatus(errorcode.ErrorCode_DataMeshErrDeleteDomainDataFailed, err.Error()),
		}
	}
	return &datamesh.DeleteDomainDataResponse{
		Status: utils.BuildSuccessResponseStatus(),
	}
}

// normalizationCreateRequest 规范化 CreateDomainData 请求。
// 如果名称为空，则从相对 URI（RelativeUri）中提取最后一段作为名称。
// 如果 DomaindataId 为空，则根据名称自动生成一个唯一的 DomainDataID。
// 如果数据源 ID 为空，则采用系统默认数据源 ID（common.DefaultDataSourceID）。
// 如果 Vendor 为空，则采用系统默认的 Vendor。
func (s domainDataService) normalizationCreateRequest(request *datamesh.CreateDomainDataRequest) {
	// normalization domaindata name
	if request.Name == "" {
		uris := strings.Split(request.RelativeUri, DomainDataURIDelimiter)
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
	if request.Vendor == "" {
		request.Vendor = common.DefaultDomainDataVendor
	}
}

// normalizationUpdateRequest 规范化 UpdateDomainData 请求。
// 更新操作中，如果请求字段为空，则需要将原有 DomainData 中的对应值填充到请求中，以防在更新时原有数据被空字段覆盖。
func (s domainDataService) normalizationUpdateRequest(request *datamesh.UpdateDomainDataRequest, data v1alpha1.DomainDataSpec) {
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

// checkDataSource 获取并检查指定数据源的合法性。
// 首先从 Kubernetes 获取对应的 DomainDataSource，如果其 Spec 中的 AccessDirectly（是否直连）为 false，
// 说明需要通过中间服务接入（如 DataProxy），则调用 CheckColType 校验字段类型是否符合该数据源类型的约束。
func (s domainDataService) checkDataSource(ctx context.Context, dsID string,
	cols []*pbv1alpha1.DataColumn) (*v1alpha1.DomainDataSource, error) {
	datasource, err := s.conf.KusciaClient.KusciaV1alpha1().DomainDataSources(s.conf.KubeNamespace).Get(ctx,
		dsID, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("query DataSource %s of DomainData fail: %v", dsID, err)
	}

	if !datasource.Spec.AccessDirectly {
		return datasource, CheckColType(cols, datasource.Spec.Type)
	}
	return datasource, nil
}

// convert2UpdateReq 将 CreateDomainDataRequest 映射转换为 UpdateDomainDataRequest。
// 用于在创建已存在的 DomainData 时重定向请求到 Update 接口。
func convert2UpdateReq(createReq *datamesh.CreateDomainDataRequest) (updateReq *datamesh.UpdateDomainDataRequest) {
	updateReq = &datamesh.UpdateDomainDataRequest{
		Header:       createReq.Header,
		DomaindataId: createReq.DomaindataId,
		Name:         createReq.Name,
		Type:         createReq.Type,
		RelativeUri:  createReq.RelativeUri,
		DatasourceId: createReq.DatasourceId,
		Attributes:   createReq.Attributes,
		Partition:    createReq.Partition,
		Columns:      createReq.Columns,
		Vendor:       createReq.Vendor,
		FileFormat:   createReq.FileFormat,
	}
	return
}

// convert2CreateResp 将 UpdateDomainDataResponse 转换映射为 CreateDomainDataResponse 并回填 DomainData ID。
func convert2CreateResp(updateResp *datamesh.UpdateDomainDataResponse, domainDataID string) (createResp *datamesh.CreateDomainDataResponse) {
	createResp = &datamesh.CreateDomainDataResponse{
		Status: updateResp.Status,
		Data: &datamesh.CreateDomainDataResponseData{
			DomaindataId: domainDataID,
		},
	}
	return
}

// CheckColType 根据数据源类型校验列的数据类型。
// 1. 将列类型统一转换为小写。
// 2. 如果数据源类型是 MySQL，只允许特定的基本类型（如 int, uint, float, bool, string/str），其他类型一概报错。
// 3. 如果是其他类型（如 DataProxy），则尝试将其映射为 Arrow 类型，如果不匹配则认为类型对于 DataProxy 无效。
func CheckColType(cols []*pbv1alpha1.DataColumn, dsType string) error {
	for _, col := range cols {
		col.Type = strings.ToLower(col.Type)
		if dsType == common.DomainDataSourceTypeMysql {
			switch col.Type {
			case "int", "int8", "int16", "int32", "int64":
			case "uint", "uint8", "uint16", "uint32", "uint64":
			case "float", "float32", "float64":
			case "bool":
			case dataTypeString, dataTypeStr:
				return nil
			default:
				err := fmt.Errorf("Col[%s].Type=%s is invalid for mysql", col.Name, col.Type)
				nlog.Error(err)
				return err
			}

			return nil
		}

		arrowType := common.Convert2ArrowColumnType(col.Type)
		if arrowType == nil {
			err := fmt.Errorf("Col[%s].Type=%s is invalid for DataProxy", col.Name, col.Type)
			nlog.Error(err)
			return err
		}
	}
	return nil
}
