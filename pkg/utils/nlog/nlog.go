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

package nlog

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type NLog struct {
	logWriter LogWriter
	formatter Formatter
	ctx       context.Context
	fields    map[string]interface{}
}

func (n *NLog) WithCtx(ctx context.Context) *NLog {
	ret := &NLog{ctx: ctx, logWriter: n.logWriter, formatter: n.formatter, fields: n.fields}
	if ctx == nil {
		ret.ctx = context.Background()
	}
	return ret
}

// WithFields returns a new NLog with additional structured fields.
// WithFields 返回一个携带额外结构化字段的新 NLog 实例。
//
// Fields are prepended to log messages in key=value format.
// 字段以 key=value 格式前置到日志消息中。
//
// Example / 示例:
//
//	log.WithFields(map[string]interface{}{"job_id": "job-123", "task_id": "task-456"}).Infof("reconcile started")
//	// Output: job_id=job-123 task_id=task-456 reconcile started
func (n *NLog) WithFields(fields map[string]interface{}) *NLog {
	merged := make(map[string]interface{}, len(n.fields)+len(fields))
	for k, v := range n.fields {
		merged[k] = v
	}
	for k, v := range fields {
		merged[k] = v
	}
	return &NLog{ctx: n.ctx, logWriter: n.logWriter, formatter: n.formatter, fields: merged}
}

// WithField returns a new NLog with a single additional field.
// WithField 返回一个携带单个额外字段的新 NLog 实例。
func (n *NLog) WithField(key string, value interface{}) *NLog {
	return n.WithFields(map[string]interface{}{key: value})
}

// formatFields renders fields as sorted key=value pairs.
// formatFields 将字段渲染为排序后的 key=value 对。
func (n *NLog) formatFields() string {
	if len(n.fields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(n.fields))
	for k := range n.fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, n.fields[k]))
	}
	return strings.Join(parts, " ") + " "
}

func (n *NLog) Infof(format string, args ...interface{}) {
	n.logWriter.Info(n.formatter.Format(n.ctx, n.formatFields()+fmt.Sprintf(format, args...)))
}

func (n *NLog) Info(args ...interface{}) {
	n.logWriter.Info(n.formatter.Format(n.ctx, n.formatFields()+fmt.Sprint(args...)))
}

func (n *NLog) Debugf(format string, args ...interface{}) {
	n.logWriter.Debug(n.formatter.Format(n.ctx, n.formatFields()+fmt.Sprintf(format, args...)))
}

func (n *NLog) Debug(args ...interface{}) {
	n.logWriter.Debug(n.formatter.Format(n.ctx, n.formatFields()+fmt.Sprint(args...)))
}

func (n *NLog) Warnf(format string, args ...interface{}) {
	n.logWriter.Warn(n.formatter.Format(n.ctx, n.formatFields()+fmt.Sprintf(format, args...)))
}

func (n *NLog) Warn(args ...interface{}) {
	n.logWriter.Warn(n.formatter.Format(n.ctx, n.formatFields()+fmt.Sprint(args...)))
}

func (n *NLog) Errorf(format string, args ...interface{}) {
	n.logWriter.Error(n.formatter.Format(n.ctx, n.formatFields()+fmt.Sprintf(format, args...)))
}

func (n *NLog) Error(args ...interface{}) {
	n.logWriter.Error(n.formatter.Format(n.ctx, n.formatFields()+fmt.Sprint(args...)))
}

func (n *NLog) Fatalf(format string, args ...interface{}) {
	n.logWriter.Fatal(n.formatter.Format(n.ctx, n.formatFields()+fmt.Sprintf(format, args...)))
}

func (n *NLog) Fatal(args ...interface{}) {
	n.logWriter.Fatal(n.formatter.Format(n.ctx, n.formatFields()+fmt.Sprint(args...)))
}

func (n *NLog) Write(p []byte) (int, error) {
	return n.logWriter.Write(p)
}

func NewNLog(ops ...Option) *NLog {
	n := &NLog{logWriter: GetDefaultLogWriter(), formatter: NewDefaultFormatter(), ctx: context.Background()}
	for _, o := range ops {
		if o != nil {
			o.apply(n)
		}
	}
	return n
}
