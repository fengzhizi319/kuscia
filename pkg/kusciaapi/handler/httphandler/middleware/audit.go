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

// Package middleware provides HTTP middleware for the Kuscia API server.
// Package middleware 为 Kuscia API 服务器提供 HTTP 中间件。
package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/secretflow/kuscia/pkg/utils/nlog"
)

var (
	auditLogger = nlog.NewNLog()

	// AuditOperationsTotal counts audit-logged API operations.
	// AuditOperationsTotal 统计审计记录的 API 操作次数。
	AuditOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kuscia_api_audit_operations_total",
			Help: "Total number of audited API operations",
		},
		[]string{"method", "path_prefix", "status_code"},
	)

	// AuditOperationDuration records the duration of audited API operations.
	// AuditOperationDuration 记录审计 API 操作的耗时。
	AuditOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kuscia_api_audit_duration_seconds",
			Help:    "Duration of audited API operations",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms ~ 16s
		},
		[]string{"method", "path_prefix"},
	)
)

// writeMethods are HTTP methods that modify state and require audit logging.
// writeMethods 是需要审计日志的写操作 HTTP 方法。
var writeMethods = map[string]bool{
	"POST":   true,
	"PUT":    true,
	"DELETE": true,
	"PATCH":  true,
}

// AuditLogMiddleware logs all write operations (POST/PUT/DELETE/PATCH) for audit purposes.
// AuditLogMiddleware 记录所有写操作（POST/PUT/DELETE/PATCH）用于审计。
//
// Following Zero-Knowledge principles, only method/path/status/duration are logged.
// Request and response bodies are NEVER logged.
//
// 遵循 Zero-Knowledge 原则，仅记录 method/path/status/duration。
// 绝不记录请求和响应体。
func AuditLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only audit write operations / 仅审计写操作
		if !writeMethods[c.Request.Method] {
			c.Next()
			return
		}

		startTime := time.Now()

		// Process request / 处理请求
		c.Next()

		duration := time.Since(startTime)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path

		// Extract path prefix for metrics (first two segments) / 提取路径前缀用于指标
		pathPrefix := extractPathPrefix(path)

		// Structured audit log (Zero-Knowledge: no request/response body)
		// 结构化审计日志（Zero-Knowledge：不含请求/响应体）
		auditLogger.WithFields(map[string]interface{}{
			"audit":     "true",
			"method":    method,
			"path":      path,
			"status":    statusCode,
			"duration":  duration.String(),
			"client_ip": c.ClientIP(),
		}).Info("API audit event")

		// Record metrics / 记录指标
		statusStr := statusCodeToString(statusCode)
		AuditOperationsTotal.WithLabelValues(method, pathPrefix, statusStr).Inc()
		AuditOperationDuration.WithLabelValues(method, pathPrefix).Observe(duration.Seconds())
	}
}

// extractPathPrefix extracts the first two path segments for metric labeling.
// extractPathPrefix 提取前两个路径段用于指标标签。
//
// Example: /api/v1alpha1/jobs/create -> /api/v1alpha1
func extractPathPrefix(path string) string {
	segments := 0
	end := 0
	for i, ch := range path {
		if ch == '/' {
			segments++
			if segments == 3 {
				end = i
				break
			}
		}
	}
	if end == 0 {
		return path
	}
	return path[:end]
}

// statusCodeToString converts HTTP status code to a label-safe string.
// statusCodeToString 将 HTTP 状态码转换为标签安全的字符串。
func statusCodeToString(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500:
		return "5xx"
	default:
		return "other"
	}
}
