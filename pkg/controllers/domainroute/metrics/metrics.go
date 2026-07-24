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

// Package metrics provides Prometheus metrics for the DomainRoute controller.
// Package metrics 为 DomainRoute 控制器提供 Prometheus 指标。
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// DomainRouteTotal counts total domain routes by status (Ready/NotReady/Pending).
	// DomainRouteTotal 按状态统计 DomainRoute 总数（Ready/NotReady/Pending）。
	DomainRouteTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kuscia_domainroute_total",
			Help: "Total number of DomainRoutes by status",
		},
		[]string{"status"},
	)

	// DomainRouteReconcileErrors counts reconcile errors for domain routes.
	// DomainRouteReconcileErrors 统计 DomainRoute reconcile 错误次数。
	DomainRouteReconcileErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kuscia_domainroute_reconcile_errors_total",
			Help: "Total number of DomainRoute reconcile errors",
		},
		[]string{"reason"},
	)

	// DomainRouteStatusTransitions counts status transitions for domain routes.
	// DomainRouteStatusTransitions 统计 DomainRoute 状态转换次数。
	DomainRouteStatusTransitions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kuscia_domainroute_status_transitions_total",
			Help: "Total number of DomainRoute status transitions",
		},
		[]string{"from", "to"},
	)
)

// RecordDomainRouteStatus increments the domain route counter for the given status.
// RecordDomainRouteStatus 增加指定状态的 DomainRoute 计数。
func RecordDomainRouteStatus(status string) {
	DomainRouteTotal.WithLabelValues(status).Inc()
}

// RecordReconcileError increments the reconcile error counter.
// RecordReconcileError 增加 reconcile 错误计数。
func RecordReconcileError(reason string) {
	DomainRouteReconcileErrors.WithLabelValues(reason).Inc()
}

// RecordStatusTransition records a status transition from one state to another.
// RecordStatusTransition 记录从一个状态到另一个状态的转换。
func RecordStatusTransition(from, to string) {
	DomainRouteStatusTransitions.WithLabelValues(from, to).Inc()
}
package metrics
