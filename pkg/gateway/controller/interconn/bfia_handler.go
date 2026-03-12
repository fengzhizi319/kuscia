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

package interconn

import (
	"fmt"

	envoycluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	grpcreversebridge "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_http1_reverse_bridge/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	kusciaapisv1alpha1 "github.com/secretflow/kuscia/pkg/crd/apis/kuscia/v1alpha1"
	"github.com/secretflow/kuscia/pkg/gateway/xds"
	"github.com/secretflow/kuscia/pkg/utils/nlog"
)

const (
	ptpSourceInst    = "x-ptp-source-inst-id"
	ptpSourceNode    = "x-ptp-source-node-id"
	ptpSessionID     = "x-ptp-session-id"
	ptpInnerPushPath = "/v1/interconn/chan/push"
	ptpOuterPushPath = "/v1/interconn/chan/invoke"

	schedulePath = "/v1/interconn/schedule/"
)

type BFIAHandler struct {
}

func (handler *BFIAHandler) GenerateInternalRoute(dr *kusciaapisv1alpha1.DomainRoute, dp kusciaapisv1alpha1.DomainPort, token string) []*route.Route {
	clusterName := fmt.Sprintf("%s-to-%s-%s", dr.Spec.Source, dr.Spec.Destination, dp.Name)
	transportRouteRouteAction := generateDefaultRouteAction(dr, clusterName)
	transportRouteRouteAction.PrefixRewrite = ptpOuterPushPath
	transportRoute := &route.Route{
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: ptpInnerPushPath,
			},
		},
		Action: &route.Route_Route{
			Route: xds.AddDefaultTimeout(transportRouteRouteAction),
		},
		RequestHeadersToAdd: []*core.HeaderValueOption{
			{
				Header: &core.HeaderValue{
					Key:   interConnProtocolHeader,
					Value: string(kusciaapisv1alpha1.InterConnBFIA),
				},
				AppendAction: core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
			{
				Header: &core.HeaderValue{
					Key:   ptpSourceInst,
					Value: dr.Spec.Source,
				},
				AppendAction: core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
			{
				Header: &core.HeaderValue{
					Key:   ptpSourceNode,
					Value: dr.Spec.Source,
				},
				AppendAction: core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
			{
				Header: &core.HeaderValue{
					Key:   "Kuscia-Token",
					Value: token,
				},
				AppendAction: core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
			{
				Header: &core.HeaderValue{
					Key:   "x-ptp-uri",
					Value: "https://ptp.cn/v1/interconn/chan/push",
				},
				AppendAction: core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
			{
				Header: &core.HeaderValue{
					Key:   "x-ptp-version",
					Value: "1",
				},
				AppendAction: core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
		},
	}

	scheduleRoute := &route.Route{
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: schedulePath,
			},
		},
		Action: &route.Route_Route{
			Route: xds.AddDefaultTimeout(generateDefaultRouteAction(dr, clusterName)),
		},
		RequestHeadersToAdd: []*core.HeaderValueOption{
			{
				Header: &core.HeaderValue{
					Key:   interConnProtocolHeader,
					Value: string(kusciaapisv1alpha1.InterConnBFIA),
				},
				AppendAction: core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
			{
				Header: &core.HeaderValue{
					Key:   "Kuscia-Token",
					Value: token,
				},
				AppendAction: core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
			{
				Header: &core.HeaderValue{
					Key:   "x-target-node-id",
					Value: dr.Spec.Destination,
				},
				AppendAction: core.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			},
		},
	}

	// Add typed_per_filter_config to disable grpc_http1_reverse_bridge
	disable := &grpcreversebridge.FilterConfigPerRoute{
		Disabled: true,
	}
	b, err := proto.Marshal(disable)
	if err != nil {
		nlog.Errorf("Marshal grpc reverse bridge config failed with %v", err)
	} else {
		transportRoute.TypedPerFilterConfig = map[string]*anypb.Any{
			"envoy.filters.http.grpc_http1_reverse_bridge": {
				TypeUrl: "type.googleapis.com/envoy.extensions.filters.http.grpc_http1_reverse_bridge.v3.FilterConfigPerRoute",
				Value:   b,
			},
		}
		scheduleRoute.TypedPerFilterConfig = map[string]*anypb.Any{
			"envoy.filters.http.grpc_http1_reverse_bridge": {
				TypeUrl: "type.googleapis.com/envoy.extensions.filters.http.grpc_http1_reverse_bridge.v3.FilterConfigPerRoute",
				Value:   b,
			},
		}
	}

	return []*route.Route{transportRoute, scheduleRoute}
}

func (handler *BFIAHandler) UpdateDstCluster(dr *kusciaapisv1alpha1.DomainRoute,
	cluster *envoycluster.Cluster) {
	xds.AddTCPHealthCheck(cluster)
}
