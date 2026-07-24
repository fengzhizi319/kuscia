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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	// Succeeded label for JobResultStats.
	Succeeded = "succeeded"
	// Failed label for JobResultStats.
	Failed = "failed"
)

var (
	// JobRequeueCount record the count of a kuscia job requeue.
	JobRequeueCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kuscia_job_requeue_count",
		Help: "Counts number of KusciaJob requeue",
	}, []string{"job_name"})

	// JobWorkerQueueSize record the kuscia job worker queue size.
	JobWorkerQueueSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "kuscia_job_worker_queue_size",
		Help: "Size of KusciaJob worker queue",
	})

	// JobSyncDurations record sync handle time duration of kuscia jobs.
	JobSyncDurations = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "kuscia_job_sync_durations_seconds",
			Help:       "Sync latency distributions of kuscia jobs.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"phase", "result"},
	)

	// JobResultStats record the count of succeeded or failed kuscia jobs.
	JobResultStats = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kuscia_job_result_stats",
			Help: "Counts number of succeeded or failed kuscia jobs",
		},
		[]string{"result"},
	)

	// JobTotal counts total kuscia jobs by status (Pending/Running/Succeeded/Failed).
	// JobTotal 按状态统计 KusciaJob 总数（Pending/Running/Succeeded/Failed）。
	JobTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kuscia_job_total",
			Help: "Total number of KusciaJobs by status",
		},
		[]string{"status"},
	)

	// JobDurationSeconds records the end-to-end duration of job execution.
	// JobDurationSeconds 记录任务端到端执行延迟。
	JobDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kuscia_job_duration_seconds",
			Help:    "End-to-end duration of KusciaJob execution",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s ~ 4096s
		},
		[]string{"app_type"},
	)

	// ReconcileDurationSeconds records reconcile loop duration per controller.
	// ReconcileDurationSeconds 记录每个控制器的 reconcile 循环耗时。
	ReconcileDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kuscia_reconcile_duration_seconds",
			Help:    "Duration of reconcile loop per controller",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms ~ 16s
		},
		[]string{"controller"},
	)
)

// ClearDeadMetrics clear requeue count of a kuscia job.
func ClearDeadMetrics(key string) {
	JobRequeueCount.DeleteLabelValues(key)
}

// RecordJobStatus increments the job total counter for the given status.
// RecordJobStatus 增加指定状态的 Job 计数。
func RecordJobStatus(status string) {
	JobTotal.WithLabelValues(status).Inc()
}

// ObserveJobDuration records the duration of a job execution.
// ObserveJobDuration 记录任务执行耗时。
func ObserveJobDuration(appType string, seconds float64) {
	JobDurationSeconds.WithLabelValues(appType).Observe(seconds)
}

// ObserveReconcileDuration records the duration of a reconcile loop.
// ObserveReconcileDuration 记录 reconcile 循环耗时。
func ObserveReconcileDuration(controller string, seconds float64) {
	ReconcileDurationSeconds.WithLabelValues(controller).Observe(seconds)
}
