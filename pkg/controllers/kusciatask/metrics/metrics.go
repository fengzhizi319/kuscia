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
	Succeeded = "succeeded"
	Failed    = "failed"
)

var (
	TaskRequeueCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kuscia_task_requeue_count",
		Help: "Counts number of kuscia tasks requeue",
	}, []string{"task_name"})

	WorkerQueueSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "kuscia_task_worker_queue_size",
		Help: "Size of kusciatask worker queue",
	})

	SyncDurations = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "kuscia_task_sync_durations_seconds",
			Help:       "Sync latency distributions of kuscia tasks.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"condition", "status"},
	)

	TaskResultStats = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kuscia_task_result_stats",
			Help: "Counts number of successful or failed kuscia tasks",
		},
		[]string{"result"},
	)

	// TaskTotal counts total kuscia tasks by status (Pending/Running/Succeeded/Failed).
	// TaskTotal 按状态统计 KusciaTask 总数。
	TaskTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kuscia_task_total",
			Help: "Total number of KusciaTasks by status",
		},
		[]string{"status"},
	)

	// TaskDurationSeconds records the execution duration of tasks by app type.
	// TaskDurationSeconds 按应用类型记录任务执行延迟。
	TaskDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kuscia_task_duration_seconds",
			Help:    "Execution duration of KusciaTask by app type",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1s ~ 4096s
		},
		[]string{"app_type"},
	)
)

func ClearDeadMetrics(key string) {
	TaskRequeueCount.DeleteLabelValues(key)
}

// RecordTaskStatus increments the task total counter for the given status.
// RecordTaskStatus 增加指定状态的 Task 计数。
func RecordTaskStatus(status string) {
	TaskTotal.WithLabelValues(status).Inc()
}

// ObserveTaskDuration records the execution duration of a task.
// ObserveTaskDuration 记录任务执行耗时。
func ObserveTaskDuration(appType string, seconds float64) {
	TaskDurationSeconds.WithLabelValues(appType).Observe(seconds)
}
