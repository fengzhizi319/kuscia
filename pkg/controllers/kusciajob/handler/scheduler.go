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

// Package handler 实现了 KusciaJob 控制器的状态机与调度逻辑。
//
// scheduler.go 专注于 KusciaJob 的编排调度：
//   - 处理 Job 的 stage 命令（start/stop/restart/cancel/suspend）；
//   - 校验任务依赖、检测 DAG 环；
//   - 根据任务依赖与 MaxParallelism 计算当前可启动的子任务；
//   - 将可启动的子任务转换为 KusciaTask CR 并创建；
//   - 根据所有子任务状态汇总 KusciaJob 的 Phase（Pending/Running/Succeeded/Failed/Cancelled 等）。
//
// 核心调度模型：一个 KusciaJob 包含多个 KusciaTaskTemplate（按 alias 唯一标识），
// Controller 持续 reconcile，把“依赖已全部 Succeeded 且尚未创建”的 task 逐个实例化为 KusciaTask CR。
//
//nolint:dupl
package handler

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/uuid"
	corelisters "k8s.io/client-go/listers/core/v1"

	"github.com/secretflow/kuscia/pkg/crd/apis/kuscia/v1alpha1"

	"github.com/secretflow/kuscia/pkg/common"
	kusciaapisv1alpha1 "github.com/secretflow/kuscia/pkg/crd/apis/kuscia/v1alpha1"

	"github.com/secretflow/kuscia/pkg/crd/clientset/versioned"
	kuscialistersv1alpha1 "github.com/secretflow/kuscia/pkg/crd/listers/kuscia/v1alpha1"
	intercommon "github.com/secretflow/kuscia/pkg/interconn/kuscia/common"
	"github.com/secretflow/kuscia/pkg/utils/nlog"
	utilsres "github.com/secretflow/kuscia/pkg/utils/resources"
)

const (
	updateRetries = 3
)

// JobScheduler 负责 KusciaJob 的编排调度，直到 Job 进入终态（Succeeded/Failed/Cancelled/ApprovalReject）。
//
// 主要职责：
//   - 处理 stage 命令（start/stop/restart/cancel/suspend）；
//   - 审批流程管理（allApprovalAccept / someApprovalReject）；
//   - 任务依赖校验与 DAG 环检测；
//   - 根据子任务状态计算 Job Phase。
type JobScheduler struct {
	// kusciaClient 用于读写 KusciaJob / KusciaTask 等 CRD。
	kusciaClient versioned.Interface
	// kusciaTaskLister 用于缓存查询已创建的 KusciaTask。
	kusciaTaskLister kuscialistersv1alpha1.KusciaTaskLister
	// domainLister 用于区分本方 domain 与 partner domain。
	domainLister kuscialistersv1alpha1.DomainLister
	// namespaceLister 用于读取 domain namespace 及标签（如 domain role）。
	namespaceLister corelisters.NamespaceLister
	// enableWorkloadApprove 控制是否需要工作负载审批。
	enableWorkloadApprove bool
}

// NewJobScheduler 创建并返回 JobScheduler 实例。
func NewJobScheduler(deps *Dependencies) *JobScheduler {
	return &JobScheduler{
		kusciaClient:          deps.KusciaClient,
		kusciaTaskLister:      deps.KusciaTaskLister,
		namespaceLister:       deps.NamespaceLister,
		domainLister:          deps.DomainLister,
		enableWorkloadApprove: deps.EnableWorkloadApprove,
	}
}

// handleStageCommand 处理 KusciaJob 的 stage 命令。
//
// stage 命令通过 job.Labels 中的 kuscia.secretflow/job-stage 传递，
// 典型值包括 start、stop、restart、cancel、suspend。
// 该方法根据命令类型分发到对应的处理器，并返回是否更新了 job status。
func (h *JobScheduler) handleStageCommand(now metav1.Time, job *kusciaapisv1alpha1.KusciaJob) (hasReconciled bool, err error) {
	stageCmd, cmdTrigger, ok := h.getStageCmd(job)
	if !ok {
		// 没有 stage 命令，直接返回。
		return
	}

	nlog.Infof("Job %s Handle stage trigger: %s send command: %s.", job.Name, cmdTrigger, stageCmd)
	switch kusciaapisv1alpha1.JobStage(stageCmd) {
	case kusciaapisv1alpha1.JobStartStage:
		return h.handleStageCmdStart(job)
	case kusciaapisv1alpha1.JobStopStage:
		return h.handleStageCmdStop(now, job)
	case kusciaapisv1alpha1.JobCancelStage:
		return h.handleStageCmdCancelled(now, job)
	case kusciaapisv1alpha1.JobRestartStage:
		return h.handleStageCmdRestart(now, job)
	case kusciaapisv1alpha1.JobSuspendStage:
		return h.handleStageCmdSuspend(now, job)
	}
	return
}

// handleStageCmdStart 处理 job-start 阶段命令。
//
// 流程：
//   - 如果是 BFIA 资源且本方是 initiator，则跳过（BFIA Job Controller 负责 initiator 侧启动逻辑）；
//   - 否则把本方所有参与 party 的 stage 状态置为 JobStartStageSucceeded。
func (h *JobScheduler) handleStageCmdStart(job *kusciaapisv1alpha1.KusciaJob) (hasReconciled bool, err error) {
	// BFIA 模式下 initiator 的 start 流程由 bfia job controller 处理，这里跳过。
	if utilsres.SelfClusterAsInitiator(h.namespaceLister, job.Spec.Initiator, job.Annotations) && utilsres.IsBFIAResource(job) {
		return false, nil
	}

	// 获取本方参与的 party 列表。
	ownP, _, _ := h.getAllParties(job)
	if job.Status.StageStatus == nil {
		job.Status.StageStatus = make(map[string]kusciaapisv1alpha1.JobStagePhase)
	}
	// 将本方每个 party 的 stage 状态标记为 start 成功。
	for p := range ownP {
		if v, ok := job.Status.StageStatus[p]; ok && v == kusciaapisv1alpha1.JobStartStageSucceeded {
			continue
		}
		job.Status.StageStatus[p] = kusciaapisv1alpha1.JobStartStageSucceeded
		hasReconciled = true
		// 打印命令执行日志。
		cmd, cmdTrigger, _ := h.getStageCmd(job)
		nlog.Infof("Job: %s party: %s execute the cmd: %s, set own party: %s stage to 'startSuccess'.", job.Name, cmdTrigger, cmd, p)
	}
	return hasReconciled, nil
}

// handleStageCmdRestart 处理 job-restart 阶段命令。
//
// 分三种情况处理：
//  1. Job 处于 Failed/Suspended：先重置为 Running，删除未成功的 task，标记本方 restart 成功/失败；
//  2. Job 处于 Running：等待所有 party restart 完成，仅由 initiator 把 stage 重新置为 start，
//     以便后续再次进入正常调度；
//  3. 其他 Phase：报错并忽略。
func (h *JobScheduler) handleStageCmdRestart(now metav1.Time, job *kusciaapisv1alpha1.KusciaJob) (hasReconciled bool, err error) {
	// 打印命令日志。
	cmd, cmdTrigger, _ := h.getStageCmd(job)
	nlog.Infof("Job: %s party: %s execute the cmd: %s.", job.Name, cmdTrigger, cmd)
	// 如果 Job 校验失败，则不允许 restart。
	validateCond, exist := utilsres.GetKusciaJobCondition(&job.Status, kusciaapisv1alpha1.JobValidated, false)
	if exist && validateCond.Status == corev1.ConditionFalse {
		nlog.Infof("Job %s condition %q status is false, skip restarting it", job.Name, kusciaapisv1alpha1.JobValidated)
		return false, nil
	}
	// 情况 1：Job 处于 Failed 或 Suspended，需要清理后重启。
	if job.Status.Phase == kusciaapisv1alpha1.KusciaJobFailed || job.Status.Phase == kusciaapisv1alpha1.KusciaJobSuspended {
		// 把 Job Phase 重置为 Running，并清空完成时间。
		job.Status.Phase = kusciaapisv1alpha1.KusciaJobRunning
		job.Status.CompletionTime = nil
		partyStage := kusciaapisv1alpha1.JobRestartStageSucceeded
		job.Status.Message = fmt.Sprintf("This job is restarted by %s", cmdTrigger)
		job.Status.Reason = fmt.Sprintf("Party: %s execute the cmd: %s.", cmdTrigger, cmd)
		// 删除未成功（非 Succeeded）的 task，为重启做准备。
		if deleteErr := h.deleteNotSuccessTasks(job); deleteErr != nil {
			// 删除失败则标记 restart 失败。
			partyStage = kusciaapisv1alpha1.JobRestartStageFailed
			job.Status.Message = fmt.Sprintf("Restart this job and delete the 'failed' phase task failed, error: %s.", deleteErr.Error())
		}
		// 设置本方每个 party 的 restart 阶段状态。
		ownP, _, _ := h.getAllParties(job)
		if job.Status.StageStatus == nil {
			job.Status.StageStatus = make(map[string]kusciaapisv1alpha1.JobStagePhase)
		}
		for p := range ownP {
			job.Status.StageStatus[p] = partyStage
		}
		return true, nil
	}
	// 情况 2：Job 正在 Running，等待所有 party 都完成 restart 后，由 initiator 切回 start stage。
	if job.Status.Phase == kusciaapisv1alpha1.KusciaJobRunning {
		isComplete := h.isAllPartyRestartComplete(job)
		if !isComplete {
			// 还有 party 未完成 restart，继续等待。
			return true, nil
		}
		// Job 可能被多次 restart，因此需要把 stage 重新置为 start 才能再次调度。
		// 为避免冲突，只有 initiator 负责设置 stage。
		if ok, _ := intercommon.SelfClusterIsInitiator(h.domainLister, job); ok {
			if err = h.setJobStage(job.Name, job.Spec.Initiator, string(kusciaapisv1alpha1.JobStartStage)); err != nil {
				nlog.Errorf("Set job stage to 'start' failed, error: %s.", err.Error())
				return true, err
			}
			return true, nil
		}
		return false, nil
	}
	// 情况 3：非预期 Phase，记录错误。
	nlog.Errorf("Unexpected phase: %s of job: %s.", job.Status.Phase, job.Name)
	return false, nil
}

// handleStageCmdStop 处理 job-stop 阶段命令。
//
// 流程：
//   - 若 Job 已处于终态（Cancelled 或已标记完成时间的 Failed），直接返回；
//   - 停止所有运行中的 task；
//   - 将本方 party 的 stage 状态置为 JobStopStageSucceeded；
//   - 将 Job Phase 置为 Failed，并记录完成时间。
func (h *JobScheduler) handleStageCmdStop(now metav1.Time, job *kusciaapisv1alpha1.KusciaJob) (bool, error) {
	if job.Status.Phase == kusciaapisv1alpha1.KusciaJobCancelled ||
		(job.Status.Phase == kusciaapisv1alpha1.KusciaJobFailed && job.Status.CompletionTime != nil) {
		return false, nil
	}
	// 打印命令日志。
	cmd, cmdTrigger, _ := h.getStageCmd(job)
	nlog.Infof("Job: %s party: %s execute the cmd: %s.", job.Name, cmdTrigger, cmd)
	stageStatus := kusciaapisv1alpha1.JobStopStageSucceeded
	ownP, _, _ := h.getAllParties(job)
	if job.Status.StageStatus == nil {
		job.Status.StageStatus = make(map[string]kusciaapisv1alpha1.JobStagePhase)
	}
	// 停止所有运行中的 task。
	if err := h.stopTasks(now, job); err != nil {
		nlog.Errorf("Stop 'runnning' task of job: %s failed, error: %s.", job.Name, err.Error())
		return false, err
	}
	// 标记本方 stop 阶段完成。
	for p := range ownP {
		job.Status.StageStatus[p] = stageStatus
	}
	// 将 Job 置为 Failed，并把正在运行的 task 也标记为 Failed。
	setRunningTaskStatusToFailed(&job.Status)
	reason := fmt.Sprintf("Party: %s execute the cmd: %s.", cmdTrigger, cmd)
	setKusciaJobStatus(now, &job.Status, kusciaapisv1alpha1.KusciaJobFailed, reason, reason)
	if job.Status.CompletionTime == nil {
		job.Status.CompletionTime = &now
	}
	return true, nil
}

// handleStageCmdCancelled 处理 job-cancel 阶段命令。
//
// 流程与 stop 类似，但最终把 Job Phase 置为 Cancelled。
func (h *JobScheduler) handleStageCmdCancelled(now metav1.Time, job *kusciaapisv1alpha1.KusciaJob) (bool, error) {
	if job.Status.Phase == kusciaapisv1alpha1.KusciaJobCancelled && job.Status.CompletionTime != nil {
		return false, nil
	}
	// 打印命令日志。
	cmd, cmdTrigger, _ := h.getStageCmd(job)
	nlog.Infof("Job: %s party: %s execute the cmd: %s.", job.Name, cmdTrigger, cmd)
	stageStatus := kusciaapisv1alpha1.JobCancelStageSucceeded
	// 获取本方 party。
	ownP, _, _ := h.getAllParties(job)
	if job.Status.StageStatus == nil {
		job.Status.StageStatus = make(map[string]kusciaapisv1alpha1.JobStagePhase)
	}
	// 停止运行中的 task。
	if err := h.stopTasks(now, job); err != nil {
		nlog.Errorf("Stop 'runnning' task of job: %s failed, error: %s.", job.Name, err.Error())
		return false, err
	}
	// 标记本方 cancel 阶段完成。
	for p := range ownP {
		job.Status.StageStatus[p] = stageStatus
	}
	// 将 Job 置为 Cancelled，并把正在运行的 task 标记为 Failed。
	setRunningTaskStatusToFailed(&job.Status)
	reason := fmt.Sprintf("Party: %s execute the cmd: %s.", cmdTrigger, cmd)
	setKusciaJobStatus(now, &job.Status, kusciaapisv1alpha1.KusciaJobCancelled, reason, reason)
	if job.Status.CompletionTime == nil {
		job.Status.CompletionTime = &now
	}
	return true, nil
}

// handleStageCmdSuspend 处理 job-suspend 阶段命令。
//
// 仅当 Job 处于 Running 时才允许 suspend：
//   - 停止运行中的 task；
//   - 标记本方 suspend 阶段完成；
//   - 将 Job Phase 置为 Suspended。
func (h *JobScheduler) handleStageCmdSuspend(now metav1.Time, job *kusciaapisv1alpha1.KusciaJob) (hasReconciled bool, err error) {
	if job.Status.Phase != kusciaapisv1alpha1.KusciaJobRunning {
		return false, nil
	}
	// 打印命令日志。
	cmd, cmdTrigger, _ := h.getStageCmd(job)
	nlog.Infof("Job: %s party: %s execute the cmd: %s.", job.Name, cmdTrigger, cmd)
	stageStatus := kusciaapisv1alpha1.JobSuspendStageSucceeded
	ownP, _, _ := h.getAllParties(job)
	if job.Status.StageStatus == nil {
		job.Status.StageStatus = make(map[string]kusciaapisv1alpha1.JobStagePhase)
	}
	// 停止运行中的 task。
	if err = h.stopTasks(now, job); err != nil {
		nlog.Errorf("Stop 'runnning' task of job: %s failed, error: %s.", job.Name, err.Error())
		return false, err
	}
	// 标记本方 suspend 阶段完成。
	for p := range ownP {
		job.Status.StageStatus[p] = stageStatus
	}
	// 将 Job 置为 Suspended。
	reason := fmt.Sprintf("Party: %s execute the cmd: %s.", cmdTrigger, cmd)
	setKusciaJobStatus(now, &job.Status, kusciaapisv1alpha1.KusciaJobSuspended, reason, "")
	return true, nil
}

// getStageCmd 从 job.Labels 中提取当前 stage 命令及其触发方。
func (h *JobScheduler) getStageCmd(job *kusciaapisv1alpha1.KusciaJob) (stageCmd, cmdTrigger string, ok bool) {
	stageCmd, ok = job.Labels[common.LabelJobStage]
	cmdTrigger = job.Labels[common.LabelJobStageTrigger]
	return
}

// updateJobTime 更新 Job 的时间戳。
//
// 若 StartTime 为空则设为当前时间；LastReconcileTime 总是更新。
// 对于 ApprovalReject 状态，若尚未设置 CompletionTime 也一并设置。
func updateJobTime(now metav1.Time, job *kusciaapisv1alpha1.KusciaJob) {
	if job.Status.StartTime == nil {
		job.Status.StartTime = &now
	}
	job.Status.LastReconcileTime = &now

	if job.Status.Phase == kusciaapisv1alpha1.KusciaJobApprovalReject &&
		job.Status.CompletionTime == nil {
		job.Status.CompletionTime = &now
	}
}

// validateJob 校验 KusciaJob 的合法性。
//
// 若已经校验通过（JobValidated condition 为 True），直接返回。
// 否则执行 kusciaJobValidate：
//   - 校验通过：设置 JobValidated condition 为 True；
//   - 校验失败：设置 JobValidated condition 为 False，并将 Job Phase 置为 Failed。
func (h *JobScheduler) validateJob(now metav1.Time, job *kusciaapisv1alpha1.KusciaJob) (needUpdateStatus, validatePass bool) {
	jobValidatedCond, _ := utilsres.GetKusciaJobCondition(&job.Status, kusciaapisv1alpha1.JobValidated, true)
	// 已校验通过，无需重复。
	if jobValidatedCond.Status == corev1.ConditionTrue {
		return false, true
	}
	// 执行具体校验逻辑。
	err := h.kusciaJobValidate(job)
	if err == nil {
		// 校验通过。
		utilsres.SetKusciaJobCondition(now, jobValidatedCond, corev1.ConditionTrue, "", "")
		return true, true
	}
	// 校验失败，记录失败原因。
	utilsres.SetKusciaJobCondition(now, jobValidatedCond, corev1.ConditionFalse, string(v1alpha1.ValidateFailed), fmt.Sprintf("Validate job failed, %v", err.Error()))
	setKusciaJobStatus(now, &job.Status, kusciaapisv1alpha1.KusciaJobFailed, string(v1alpha1.ValidateFailed), "")
	return true, false
}

// allApprovalAccept 判断所有参与方是否都已接受该 Job。
//
// initiator 默认视为已接受；其他 party 需要在 job.Status.ApproveStatus 中标记为 JobAccepted。
func (h *JobScheduler) allApprovalAccept(job *kusciaapisv1alpha1.KusciaJob) (ture bool, err error) {
	for _, party := range utilsres.GetJobParties(job) {
		domain, err := h.domainLister.Get(party.DomainID)
		if err != nil {
			nlog.Errorf("Check approval failed, error: %s.", err.Error())
			return false, err
		}
		// initiator 自动视为已接受。
		if party.DomainID == job.Spec.Initiator {
			continue
		}
		// 其他 party 必须显式 Accepted。
		if result, ok := job.Status.ApproveStatus[domain.Name]; ok && result == kusciaapisv1alpha1.JobAccepted {
			continue
		}
		return false, nil
	}
	return true, nil
}

// someApprovalReject 判断是否存在参与方拒绝该 Job。
//
// 只要任一非 initiator party 在 ApproveStatus 中标记为 JobRejected，即返回 true。
func (h *JobScheduler) someApprovalReject(job *kusciaapisv1alpha1.KusciaJob) (ture bool, rejectParty string, err error) {
	for _, party := range utilsres.GetJobParties(job) {
		domain, err := h.domainLister.Get(party.DomainID)
		if err != nil {
			nlog.Errorf("Check approval failed, error: %s.", err.Error())
			return false, "", err
		}
		// initiator 自动视为已接受，跳过。
		if party.DomainID == job.Spec.Initiator {
			continue
		}
		// 发现拒绝。
		if result, ok := job.Status.ApproveStatus[domain.Name]; ok && result == kusciaapisv1alpha1.JobRejected {
			return true, domain.Name, nil
		}
	}
	return false, "", nil
}

// allPartyCreateSuccess 判断所有参与方是否都已完成 create/start 阶段。
func (h *JobScheduler) allPartyCreateSuccess(job *kusciaapisv1alpha1.KusciaJob) (ture bool, err error) {
	for _, party := range utilsres.GetJobParties(job) {
		domain, err := h.domainLister.Get(party.DomainID)
		if err != nil {
			nlog.Errorf("Check party create success status failed, error: %s.", err.Error())
			return false, err
		}
		// create 或 start 任一阶段成功即视为该 party 已就绪。
		if stageStatus, ok := job.Status.StageStatus[domain.Name]; ok && stageStatus == kusciaapisv1alpha1.JobCreateStageSucceeded ||
			stageStatus == kusciaapisv1alpha1.JobStartStageSucceeded {
			continue
		}
		return false, nil
	}
	return true, nil
}

// somePartyCreateFailed 判断是否有参与方 create 阶段失败。
func (h *JobScheduler) somePartyCreateFailed(job *kusciaapisv1alpha1.KusciaJob) (ture bool, rejectParty string, err error) {
	for _, party := range utilsres.GetJobParties(job) {
		domain, err := h.domainLister.Get(party.DomainID)
		if err != nil {
			nlog.Errorf("Check party create fail status failed, error: %s.", err.Error())
			return false, "", err
		}
		if stageStatus, ok := job.Status.StageStatus[domain.Name]; ok && stageStatus == kusciaapisv1alpha1.JobCreateStageFailed {
			return true, domain.Name, nil
		}
	}
	return false, "", nil
}

// somePartyStartFailed 判断是否有参与方 start/create 阶段失败。
func (h *JobScheduler) somePartyStartFailed(job *kusciaapisv1alpha1.KusciaJob) (ture bool, failedParty string, err error) {
	for _, party := range utilsres.GetJobParties(job) {
		domain, err := h.domainLister.Get(party.DomainID)
		if err != nil {
			nlog.Errorf("Check party create fail status failed, error: %s.", err.Error())
			return false, "", err
		}
		// start 或 create 任一阶段失败都视为失败。
		if stageStatus, ok := job.Status.StageStatus[domain.Name]; ok && stageStatus == kusciaapisv1alpha1.JobStartStageFailed ||
			stageStatus == kusciaapisv1alpha1.JobCreateStageFailed {
			return true, domain.Name, nil
		}
	}
	return false, "", nil
}

// isAllPartyRestartComplete 判断所有参与方是否都已完成 restart 阶段（成功或失败）。
func (h *JobScheduler) isAllPartyRestartComplete(job *kusciaapisv1alpha1.KusciaJob) bool {
	for _, party := range utilsres.GetJobParties(job) {
		stageStatus, ok := job.Status.StageStatus[party.DomainID]
		if !ok {
			return false
		}
		if stageStatus != kusciaapisv1alpha1.JobRestartStageFailed && stageStatus != kusciaapisv1alpha1.JobRestartStageSucceeded {
			nlog.Infof("Party: %s is not complete to handle 'restart' stage.", party.DomainID)
			return false
		}
	}
	return true
}

// getAllParties 根据 job.Spec.Tasks 汇总所有参与方，并按 domain role 区分为本方与外部 partner。
//
// 返回：
//   - ownParties：由本 Kuscia 集群负责的 domain（非 Partner）；
//   - otherParties：外部 partner domain；
//   - err：查询 domain 失败时返回错误。
func (h *JobScheduler) getAllParties(job *kusciaapisv1alpha1.KusciaJob) (ownParties, otherParties map[string]kusciaapisv1alpha1.Party, err error) {
	partyMap := make(map[string]kusciaapisv1alpha1.Party)
	ownParties = make(map[string]kusciaapisv1alpha1.Party)
	otherParties = make(map[string]kusciaapisv1alpha1.Party)
	// 汇总所有 task 中的 party，按 domain_id 去重。
	for _, t := range job.Spec.Tasks {
		for _, p := range t.Parties {
			partyMap[p.DomainID] = p
		}
	}
	// 根据 namespace/domain 的 role 进行分类。
	for k, v := range partyMap {
		domain, err := h.domainLister.Get(k)
		if err != nil {
			nlog.Errorf("getAllParties failed, get domain: %s, error: %s.", k, err.Error())
			return nil, nil, err
		}
		if domain.Spec.Role == kusciaapisv1alpha1.Partner {
			otherParties[k] = v
			continue
		}
		ownParties[k] = v
	}
	return
}

// stopTasks 停止 KusciaJob 下所有尚未结束的任务。
//
// 遍历 job.Status.TaskStatus：
//   - 跳过已 Failed/Succeeded 的 task；
//   - 对运行中的 task，先通过 lister 查询，缓存未命中再向 apiserver 二次确认；
//   - 将 task 及其本方 party 的 phase 置为 Failed，并更新到 apiserver。
func (h *JobScheduler) stopTasks(now metav1.Time, kusciaJob *kusciaapisv1alpha1.KusciaJob) error {
	for taskID, phase := range kusciaJob.Status.TaskStatus {
		if phase == kusciaapisv1alpha1.TaskFailed || phase == kusciaapisv1alpha1.TaskSucceeded {
			continue
		}

		kt, err := h.kusciaTaskLister.KusciaTasks(common.KusciaCrossDomain).Get(taskID)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				// 缓存未命中时二次确认，可能是被手动删除。
				kt, err = h.kusciaClient.KusciaV1alpha1().KusciaTasks(common.KusciaCrossDomain).Get(context.Background(), taskID, metav1.GetOptions{})
				if err != nil {
					if k8serrors.IsNotFound(err) {
						nlog.Errorf("Kuscia task %v not found, skip stopping", taskID)
						continue
					}
					nlog.Errorf("Get kuscia task %v failed, so skip stopping this task", taskID)
					return err
				}
			} else {
				nlog.Errorf("Get kuscia task %v failed, so skip stopping this task", taskID)
				return err
			}
		}

		copyKt := kt.DeepCopy()
		setKusciaTaskStatus(now, &copyKt.Status, kusciaapisv1alpha1.TaskFailed, "KusciaJobStopped", "Job was stopped")
		// 将本方 party 的 task 状态也置为 Failed；外部 BFIA partner 跳过。
		for _, party := range copyKt.Spec.Parties {
			if utilsres.IsOuterBFIAInterConnDomain(h.namespaceLister, party.DomainID) {
				continue
			}
			isPartner, checkErr := utilsres.IsPartnerDomain(h.namespaceLister, party.DomainID)
			if checkErr != nil {
				return checkErr
			}
			if isPartner {
				continue
			}
			found := false
			for i := range copyKt.Status.PartyTaskStatus {
				if copyKt.Status.PartyTaskStatus[i].DomainID == party.DomainID && copyKt.Status.PartyTaskStatus[i].Role == party.Role {
					found = true
					copyKt.Status.PartyTaskStatus[i].Phase = kusciaapisv1alpha1.TaskFailed
				}
			}

			if !found {
				partyTaskStatus := kusciaapisv1alpha1.PartyTaskStatus{DomainID: party.DomainID, Role: party.Role, Phase: kusciaapisv1alpha1.TaskFailed}
				copyKt.Status.PartyTaskStatus = append(copyKt.Status.PartyTaskStatus, partyTaskStatus)
			}
		}

		if err = utilsres.UpdateKusciaTaskStatus(h.kusciaClient, kt, copyKt); err != nil {
			return err
		}
	}
	return nil
}

// deleteNotSuccessTasks 删除 KusciaJob 下所有未成功的 task（用于 restart 时清理）。
//
// 仅保留 Succeeded 的 task；其他 task 从 apiserver 删除，并从 job.Status.TaskStatus 中移除。
func (h *JobScheduler) deleteNotSuccessTasks(kusciaJob *kusciaapisv1alpha1.KusciaJob) error {
	var tasks []string
	for taskID, phase := range kusciaJob.Status.TaskStatus {
		if phase == kusciaapisv1alpha1.TaskSucceeded {
			continue
		}
		tasks = append(tasks, taskID)

		kt, err := h.kusciaTaskLister.KusciaTasks(common.KusciaCrossDomain).Get(taskID)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			nlog.Warnf("Get kuscia task %v failed, so skip delete this task, error: %s.", taskID, err.Error())
			return err
		}
		err = h.kusciaClient.KusciaV1alpha1().KusciaTasks(common.KusciaCrossDomain).Delete(context.Background(), kt.Name, metav1.DeleteOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			nlog.Warnf("Delete kuscia task %v failed, so skip delete this task, error: %s.", taskID, err.Error())
			return err
		}
	}
	// 清理 job.Status.TaskStatus 中对应条目。
	for _, v := range tasks {
		delete(kusciaJob.Status.TaskStatus, v)
	}
	return nil
}

// kusciaJobValidate 校验 KusciaJob 的合法性。
//
// 校验项：
//  1. initiator 对应的 namespace 必须存在；
//  2. Job 至少包含一个 task；
//  3. 所有 task 的 dependencies 必须指向存在的 task；
//  4. task 依赖图不能存在环。
func (h *JobScheduler) kusciaJobValidate(kusciaJob *kusciaapisv1alpha1.KusciaJob) error {
	if _, err := h.namespaceLister.Get(kusciaJob.Spec.Initiator); err != nil {
		return fmt.Errorf("can't find initiator namespace %v under cluster, %v", kusciaJob.Spec.Initiator, err)
	}

	if len(kusciaJob.Spec.Tasks) == 0 {
		return fmt.Errorf("kuscia job should include at least one of task")
	}

	if err := kusciaJobDependenciesExits(kusciaJob); err != nil {
		return err
	}
	return kusciaJobHasTaskCycle(kusciaJob)
}

// annotateKusciaJob 对 KusciaJob 进行预处理，更新 labels 与 annotations。
//
// 主要设置两类信息：
//  1. 本方是否为 initiator（SelfClusterAsInitiatorAnnotationKey）；
//  2. 互联互通协议相关标签（BFIA/Kuscia/self party 列表）。
func (h *JobScheduler) annotateKusciaJob(job *kusciaapisv1alpha1.KusciaJob, ownParties map[string]kusciaapisv1alpha1.Party) (hasUpdate bool, err error) {
	if job.Annotations != nil {
		if _, ok := job.Annotations[common.InterConnSelfPartyAnnotationKey]; ok {
			return false, nil
		}

		// for job which initiator doesn't participate
		// and will not annotate kuscia.secretflow/interconn-self-parties
		// so, choose kuscia.secretflow/self-cluster-as-initiator to decide if need return
		if len(ownParties) == 0 {
			if _, ok := job.Annotations[common.SelfClusterAsInitiatorAnnotationKey]; ok {
				return false, nil
			}
		}
	}
	// 标记本方是否作为 initiator。
	if err = h.annotateSelfClusterAsInitiator(job); err != nil {
		return false, err
	}
	// 标记互联互通协议与参与方列表。
	if err = h.annotateInterConn(job); err != nil {
		return false, err
	}

	// 将更新后的 annotations 同步回 apiserver。
	update := func(kusciaJob *kusciaapisv1alpha1.KusciaJob) {
		mergeAnnotations(kusciaJob, job)
	}

	hasUpdated := func(kusciaJob *kusciaapisv1alpha1.KusciaJob) bool {
		if job.Annotations != nil {
			if _, ok := job.Annotations[common.SelfClusterAsInitiatorAnnotationKey]; ok {
				return true
			}
		}
		return false
	}
	return true, utilsres.UpdateKusciaJob(h.kusciaClient, job, hasUpdated, update, updateRetries)
}

func mergeAnnotations(originJob, currentJob *kusciaapisv1alpha1.KusciaJob) {
	if originJob.Annotations == nil {
		originJob.Annotations = make(map[string]string)
	}
	for k, v := range currentJob.Annotations {
		originJob.Annotations[k] = v
	}
}

// annotateSelfClusterAsInitiator 标记本方集群是否作为 initiator 参与该 Job。
//
// 若 initiator 对应的 domain role 为 Partner，则本方为 false；否则为 true，并记录 initiator 名称。
func (h *JobScheduler) annotateSelfClusterAsInitiator(job *kusciaapisv1alpha1.KusciaJob) (err error) {
	ns, err := h.namespaceLister.Get(job.Spec.Initiator)
	if err != nil {
		nlog.Errorf("setSelfClusterAsInitiator failed, Get domain error: %s.", err.Error())
		return err
	}
	if job.Annotations == nil {
		job.Annotations = make(map[string]string)
	}
	if ns.Labels[common.LabelDomainRole] == string(kusciaapisv1alpha1.Partner) {
		job.Annotations[common.SelfClusterAsInitiatorAnnotationKey] = "false"
		return
	}
	job.Annotations[common.SelfClusterAsInitiatorAnnotationKey] = "true"
	job.Annotations[common.InitiatorAnnotationKey] = job.Spec.Initiator
	return
}

// annotateInterConn 根据参与方的 domain role 与互联互通协议，为 KusciaJob 设置互联标签。
//
// 逻辑：
//   - 将 partner domain 按协议分为 BFIA 列表与 Kuscia 列表；
//   - 将非 partner（本方） domain 加入 selfDomainList；
//   - 不允许同一个 Job 同时混用 BFIA 与 Kuscia 协议；
//   - 仅当本方是 initiator 时才需要标记 partner 列表。
func (h *JobScheduler) annotateInterConn(job *kusciaapisv1alpha1.KusciaJob) (err error) {
	var (
		bfiaDomainList   []string
		kusciaDomainList []string
		selfDomainList   []string
	)

	for _, party := range utilsres.GetJobParties(job) {
		ns, err := h.namespaceLister.Get(party.DomainID)
		if err != nil {
			nlog.Errorf("labelInterConn failed to get domain %v namespace, %v", party.DomainID, err)
			return err
		}
		if ns.Labels != nil && ns.Labels[common.LabelDomainRole] == string(kusciaapisv1alpha1.Partner) {
			// 只有 initiator 方才需要为 partner 打互联标签。
			if !utilsres.SelfClusterAsInitiator(h.namespaceLister, job.Spec.Initiator, job.Annotations) {
				continue
			}
			switch ns.Labels[common.LabelInterConnProtocols] {
			case string(kusciaapisv1alpha1.InterConnBFIA):
				bfiaDomainList = append(bfiaDomainList, ns.Name)
			case string(kusciaapisv1alpha1.InterConnKuscia):
				kusciaDomainList = append(kusciaDomainList, ns.Name)
			default:
				kusciaDomainList = append(kusciaDomainList, ns.Name)
			}
		} else {
			selfDomainList = append(selfDomainList, ns.Name)
		}
	}

	// BFIA 与 Kuscia 协议不能混用。
	if len(bfiaDomainList) > 0 && len(kusciaDomainList) > 0 {
		return fmt.Errorf("can't create a job with partners using both bfia and kuscia protocol")
	}

	if len(bfiaDomainList) > 0 {
		value := domainListToString(bfiaDomainList)
		job.Annotations[common.InterConnBFIAPartyAnnotationKey] = value
		if job.Labels == nil {
			job.Labels = make(map[string]string)
		}
		job.Labels[common.LabelInterConnProtocolType] = string(kusciaapisv1alpha1.InterConnBFIA)
	}
	if len(kusciaDomainList) > 0 {
		value := domainListToString(kusciaDomainList)
		job.Annotations[common.InterConnKusciaPartyAnnotationKey] = value
	}
	if len(selfDomainList) > 0 {
		value := domainListToString(selfDomainList)
		job.Annotations[common.InterConnSelfPartyAnnotationKey] = value
	}
	return
}

// setJobStage 更新 KusciaJob 的 stage 标签，并递增 stage version。
//
// 通过修改 job.Labels 触发下一次 reconcile，使 Job 进入新的 stage 处理逻辑。
func (h *JobScheduler) setJobStage(jobID string, trigger, stage string) (err error) {
	job, err := h.kusciaClient.KusciaV1alpha1().KusciaJobs(common.KusciaCrossDomain).Get(context.Background(), jobID, metav1.GetOptions{})
	if err != nil {
		nlog.Errorf("Get job: %s failed, error: %s.", jobID, err.Error())
		return err
	}
	if job.Labels == nil {
		job.Labels = make(map[string]string)
	}
	job.Labels[common.LabelJobStage] = stage
	job.Labels[common.LabelJobStageTrigger] = trigger
	// 递增 stage version，避免重复处理同一 stage 命令。
	jobVersion := "1"
	if v, ok := job.Labels[common.LabelJobStageVersion]; ok {
		if iV, convErr := strconv.Atoi(v); convErr == nil {
			jobVersion = strconv.Itoa(iV + 1)
		}
	}
	job.Labels[common.LabelJobStageVersion] = jobVersion
	_, err = h.kusciaClient.KusciaV1alpha1().KusciaJobs(common.KusciaCrossDomain).Update(context.Background(), job, metav1.UpdateOptions{})
	if err != nil {
		nlog.Errorf("Update job: %s failed, error: %s.", job.Name, err.Error())
		return err
	}
	return nil
}

// domainListToString 将 domain 列表用下划线拼接成 label 值。
func domainListToString(domains []string) (labelValue string) {
	for _, v := range domains {
		if labelValue == "" {
			labelValue = v
			continue
		}
		labelValue += "_" + v
	}
	return
}

// setJobTaskID 为 KusciaJob 中尚未设置 TaskID 的子任务生成唯一 ID。
//
// 生成的 TaskID 格式为：{jobName}-{uuid 后缀}，并立即更新到 apiserver。
func (h *JobScheduler) setJobTaskID(kusciaJob *kusciaapisv1alpha1.KusciaJob) (bool, error) {
	needSetTaskID := false
	for i := range kusciaJob.Spec.Tasks {
		if kusciaJob.Spec.Tasks[i].TaskID == "" {
			needSetTaskID = true
		}
	}

	if !needSetTaskID {
		return false, nil
	}

	update := func(kusciaJob *kusciaapisv1alpha1.KusciaJob) {
		for i := range kusciaJob.Spec.Tasks {
			if kusciaJob.Spec.Tasks[i].TaskID == "" {
				kusciaJob.Spec.Tasks[i].TaskID = generateTaskID(kusciaJob.Name)
			}
		}
	}

	update(kusciaJob)

	hasUpdated := func(kusciaJob *kusciaapisv1alpha1.KusciaJob) bool {
		for i := range kusciaJob.Spec.Tasks {
			if kusciaJob.Spec.Tasks[i].TaskID == "" {
				return false
			}
		}
		return true
	}
	return true, utilsres.UpdateKusciaJob(h.kusciaClient, kusciaJob, hasUpdated, update, updateRetries)
}

// isBFIAInterConnJob 判断 Job 是否使用 BFIA 协议进行互联互通。
func isBFIAInterConnJob(nsLister corelisters.NamespaceLister, kusciaJob *kusciaapisv1alpha1.KusciaJob) bool {
	if kusciaJob.Annotations != nil {
		if val, ok := kusciaJob.Annotations[common.InterConnBFIAPartyAnnotationKey]; ok && val != "" {
			return true
		}
	}
	if kusciaJob.Labels != nil {
		if interConnType, ok := kusciaJob.Labels[common.LabelInterConnProtocolType]; ok && interConnType == string(kusciaapisv1alpha1.InterConnBFIA) {
			return true
		}
	}
	return false
}

// isInterConnJob 判断 Job 是否为互联互通类型（BFIA 或 Kuscia 协议）。
func isInterConnJob(kusciaJob *kusciaapisv1alpha1.KusciaJob) bool {
	_, existBFIA := kusciaJob.Annotations[common.InterConnBFIAPartyAnnotationKey]
	_, existKuscia := kusciaJob.Annotations[common.InterConnKusciaPartyAnnotationKey]
	_, existInterconn := kusciaJob.Labels[common.LabelInterConnProtocolType]
	if existBFIA || existKuscia || existInterconn {
		return true
	}
	return false
}

// kusciaJobHasTaskCycle 检测 KusciaJob 的任务依赖图中是否存在环。
//
// 算法：拓扑排序思想。循环剥离当前入度为 0（无依赖）的 task，
// 若某轮无法剥离任何 task 但仍有剩余 task，则存在环。
func kusciaJobHasTaskCycle(kusciaJob *kusciaapisv1alpha1.KusciaJob) error {
	copyKusciaJob := kusciaJob.DeepCopy()
	for {
		// 步骤 1：收集本轮所有无依赖的 task。
		removeSubtasks := make(map[string]bool, 0)
		copyKusciaJob.Spec.Tasks = kusciaTaskTemplateFilter(copyKusciaJob.Spec.Tasks,
			func(t kusciaapisv1alpha1.KusciaTaskTemplate, i int) bool {
				noDependencies := len(t.Dependencies) == 0
				if noDependencies {
					removeSubtasks[t.Alias] = true
				}
				return !noDependencies
			})

		// 步骤 2：从剩余 task 的依赖列表中移除已剥离的 task。
		for i, t := range copyKusciaJob.Spec.Tasks {
			copyKusciaJob.Spec.Tasks[i].Dependencies = stringFilter(t.Dependencies,
				func(taskName string, i int) bool {
					_, exist := removeSubtasks[taskName]
					return !exist
				})
		}

		// 若本轮没有 task 被剥离：
		// - 剩余 task 为空则无环；
		// - 剩余 task 非空则存在环。
		if len(removeSubtasks) == 0 {
			for _, t := range copyKusciaJob.Spec.Tasks {
				if len(t.Dependencies) != 0 {
					return fmt.Errorf("validate failed: cycled dependencies")
				}
			}
			break
		}
	}

	return nil
}

// kusciaJobDependenciesExits 检查所有 task 的 dependencies 是否指向了真实存在的 task alias。
func kusciaJobDependenciesExits(kusciaJob *kusciaapisv1alpha1.KusciaJob) error {
	copyKusciaJob := kusciaJob.DeepCopy()
	taskIDSet := make(map[string]bool, 0)
	for _, t := range copyKusciaJob.Spec.Tasks {
		taskIDSet[t.Alias] = true
	}

	for _, t := range copyKusciaJob.Spec.Tasks {
		for _, d := range t.Dependencies {
			if _, exits := taskIDSet[d]; !exits {
				return fmt.Errorf("validate failed: task %s has not exist dependency task %s", t.Alias, d)
			}
		}
	}

	return nil
}

// buildJobSubTaskStatus 根据已创建的 KusciaTask 列表，构建 Job 子任务状态映射。
//
// 返回两个 map：
//   - 以 task alias 为 key；
//   - 以 task id（KusciaTask name）为 key。
func buildJobSubTaskStatus(currentSubTasks []*kusciaapisv1alpha1.KusciaTask, job *kusciaapisv1alpha1.KusciaJob) (map[string]kusciaapisv1alpha1.KusciaTaskPhase, map[string]kusciaapisv1alpha1.KusciaTaskPhase) {
	subTaskStatusWithAlias := make(map[string]kusciaapisv1alpha1.KusciaTaskPhase, 0)
	subTaskStatusWithID := make(map[string]kusciaapisv1alpha1.KusciaTaskPhase, 0)
	for idx := range currentSubTasks {
		for _, task := range job.Spec.Tasks {
			if task.TaskID == currentSubTasks[idx].Name {
				if currentSubTasks[idx].Status.Phase == "" {
					currentSubTasks[idx].Status.Phase = kusciaapisv1alpha1.TaskPending
				}
				subTaskStatusWithAlias[task.Alias] = currentSubTasks[idx].Status.Phase
				subTaskStatusWithID[task.TaskID] = currentSubTasks[idx].Status.Phase
			}
		}
	}
	return subTaskStatusWithAlias, subTaskStatusWithID
}

// buildJobStatus 根据计算出的 currentJobStatusPhase 更新 KusciaJobStatus。
//
// 若 Phase 变为 Succeeded 且 CompletionTime 为空，则设置完成时间。
func buildJobStatus(now metav1.Time,
	kjStatus *kusciaapisv1alpha1.KusciaJobStatus,
	currentJobStatusPhase kusciaapisv1alpha1.KusciaJobPhase) bool {
	needUpdate := false
	if kjStatus.Phase != currentJobStatusPhase {
		needUpdate = true
		kjStatus.Phase = currentJobStatusPhase
	}

	if currentJobStatusPhase == kusciaapisv1alpha1.KusciaJobSucceeded {
		if kjStatus.CompletionTime == nil {
			needUpdate = true
			kjStatus.CompletionTime = &now
		}
	}

	return needUpdate
}

// jobStatusPhaseFrom 根据当前子任务状态汇总 KusciaJob 的整体 Phase。
//
// 关键概念：
//   - Finished task：task 状态为 Succeeded 或 Failed；
//   - Ready task：依赖已全部 Succeeded，但尚未创建 KusciaTask；
//   - Critical task：非 tolerable 的关键任务。
//
// 状态转换规则：
//   - 没有任何子任务被创建：Running（Job 已提交，等待调度）；
//   - 所有子任务 Finished 且所有 Critical 子任务 Succeeded：Succeeded；
//   - Strict 模式：任一 Critical 子任务 Failed → Failed；
//   - BestEffort 模式：
//   - 互联互通 Job：任一子任务 Failed → Failed；
//   - 普通 Job：没有 Ready 也没有 Running 子任务，且存在 Critical 子任务 Failed → Failed；
//   - 其他情况：Running。
func jobStatusPhaseFrom(job *kusciaapisv1alpha1.KusciaJob, currentSubTasksStatus map[string]kusciaapisv1alpha1.KusciaTaskPhase) (phase kusciaapisv1alpha1.KusciaJobPhase) {
	tasks := currentTaskMapFrom(job, currentSubTasksStatus)

	// 所有子任务都还没创建，Job 视为 Running（等待调度）。
	if tasks.AllMatch(taskNotExists) {
		return kusciaapisv1alpha1.KusciaJobRunning
	}

	criticalTasks := tasks.criticalTaskMap()
	readyTasks := tasks.readyTaskMap()
	nlog.Infof("JobStatusPhaseFrom readyTasks=%+v, tasks=%+v, kusciaJobId=%s",
		readyTasks.ToShortString(), tasks.ToShortString(), job.Name)

	// 所有子任务结束且所有关键子任务成功，Job 成功。
	if tasks.AllMatch(taskFinished) && criticalTasks.AllMatch(taskSucceeded) {
		return kusciaapisv1alpha1.KusciaJobSucceeded
	}

	switch job.Spec.ScheduleMode {
	case kusciaapisv1alpha1.KusciaJobScheduleModeStrict:
		// Strict 模式：任一关键任务失败，整个 Job 失败。
		if criticalTasks.AnyMatch(taskFailed) {
			return kusciaapisv1alpha1.KusciaJobFailed
		}
	case kusciaapisv1alpha1.KusciaJobScheduleModeBestEffort:
		// 互联互通 Job：任一任务失败即整体失败。
		if isInterConnJob(job) {
			if tasks.AnyMatch(taskFailed) {
				nlog.Infof("Interconn jobStatusPhaseFrom failed readyTasks=%+v, tasks=%+v, kusciaJobId=%s",
					readyTasks.ToShortString(), tasks.ToShortString(), job.Name)
				return kusciaapisv1alpha1.KusciaJobFailed
			}
		} else {
			// 普通 BestEffort：没有可调度/运行中的任务，且关键任务失败，则 Job 失败。
			if len(readyTasks) == 0 && !tasks.AnyMatch(taskRunning) && criticalTasks.AnyMatch(taskFailed) {
				nlog.Infof("JobStatusPhaseFrom failed readyTasks=%+v, tasks=%+v, kusciaJobId=%s",
					readyTasks.ToShortString(), tasks.ToShortString(), job.Name)
				return kusciaapisv1alpha1.KusciaJobFailed
			}
		}
	default:
		// 不可达，提交时应该已校验模式。
		return kusciaapisv1alpha1.KusciaJobFailed
	}

	// 默认情况 Job 仍在运行。
	return kusciaapisv1alpha1.KusciaJobRunning
}

// ShouldReconcile 判断 KusciaJob 是否需要继续 reconcile。
//
// 处于终态（ApprovalReject / Cancelled / Succeeded）且已设置 CompletionTime 的 Job 无需再 reconcile。
func ShouldReconcile(job *kusciaapisv1alpha1.KusciaJob) bool {
	if (job.Status.Phase == kusciaapisv1alpha1.KusciaJobApprovalReject ||
		job.Status.Phase == kusciaapisv1alpha1.KusciaJobCancelled ||
		job.Status.Phase == kusciaapisv1alpha1.KusciaJobSucceeded) &&
		job.Status.CompletionTime != nil {
		return false
	}
	return true
}

// readyTasksOf 返回当前已经“就绪”但尚未创建的子任务列表。
//
// 就绪条件：task 的 dependencies 全部 Succeeded，且该 task 还没有对应的 KusciaTask。
// 注意：不考虑 ScheduleMode，只要依赖满足就视为 ready。
func readyTasksOf(kusciaJob *kusciaapisv1alpha1.KusciaJob, currentTasks map[string]kusciaapisv1alpha1.KusciaTaskPhase) []kusciaapisv1alpha1.KusciaTaskTemplate {
	if currentTasks == nil {
		currentTasks = make(map[string]kusciaapisv1alpha1.KusciaTaskPhase, 0)
	}

	// 深拷贝，避免修改原始 Job。
	copyKusciaJob := kusciaJob.DeepCopy()
	// 从每个 task 的依赖中移除已经 Succeeded 的依赖，
	// 之后 dependencies 为空的 task 就是已经可创建或失败的。
	for i, t := range copyKusciaJob.Spec.Tasks {
		copyKusciaJob.Spec.Tasks[i].Dependencies = stringFilter(t.Dependencies,
			func(t string, i int) bool {
				return !(currentTasks[t] == kusciaapisv1alpha1.TaskSucceeded)
			})
	}
	noDependenciesTasks := kusciaTaskTemplateFilter(copyKusciaJob.Spec.Tasks,
		func(t kusciaapisv1alpha1.KusciaTaskTemplate, i int) bool {
			return len(t.Dependencies) == 0 && t.TaskID != ""
		})

	// 在 dependencies 为空的 task 中，筛选出尚未创建 KusciaTask 的 task。
	readyTasks := kusciaTaskTemplateFilter(noDependenciesTasks,
		func(t kusciaapisv1alpha1.KusciaTaskTemplate, i int) bool {
			_, exist := currentTasks[t.Alias]
			return !exist
		})

	if len(readyTasks) == 0 {
		return nil
	}

	// 按优先级降序排列，优先级高的先调度。
	sort.Slice(readyTasks, func(i, j int) bool {
		return readyTasks[i].Priority > readyTasks[j].Priority
	})

	// 返回原始 Job 中的 task 模板（保留完整配置）。
	kusciaJobTaskMap := make(map[string]kusciaapisv1alpha1.KusciaTaskTemplate)
	for _, t := range kusciaJob.Spec.Tasks {
		kusciaJobTaskMap[t.Alias] = t
	}

	originReadyTasks := make([]kusciaapisv1alpha1.KusciaTaskTemplate, len(readyTasks))
	for i, t := range readyTasks {
		originReadyTasks[i] = kusciaJobTaskMap[t.Alias]
	}
	return originReadyTasks
}

// generateTaskID 生成 task 的唯一 ID。
func generateTaskID(jobName string) string {
	uid := strings.Split(string(uuid.NewUUID()), "-")
	return jobName + "-" + uid[len(uid)-1]
}

// willStartTasksOf 根据 MaxParallelism 从 readyTasks 中筛选出即将启动的子任务。
//
// 统计当前 Running / Pending / 未设置 phase 的 task 数量，
// 只有当该数量小于 MaxParallelism 时才允许启动新的 task，
// 且最多启动 (MaxParallelism - 当前数量) 个。
func willStartTasksOf(kusciaJob *kusciaapisv1alpha1.KusciaJob, readyTasks []kusciaapisv1alpha1.KusciaTaskTemplate, status map[string]kusciaapisv1alpha1.KusciaTaskPhase) []kusciaapisv1alpha1.KusciaTaskTemplate {
	count := 0
	for _, phase := range status {
		if phase == kusciaapisv1alpha1.TaskRunning || phase == kusciaapisv1alpha1.TaskPending || phase == "" {
			count++
		}
	}

	if *kusciaJob.Spec.MaxParallelism <= count {
		return nil
	}

	willStartTasks := readyTasks
	if len(readyTasks) > (*kusciaJob.Spec.MaxParallelism - count) {
		willStartTasks = readyTasks[:*kusciaJob.Spec.MaxParallelism-count]
	}

	return willStartTasks
}

// buildWillStartKusciaTask 把 Job 中即将启动的子任务转换为 KusciaTask CR 对象。
//
// 生成的 KusciaTask 包含：
//   - Name：task.TaskID；
//   - OwnerReferences：指向所属 KusciaJob；
//   - Labels/Annotations：记录 JobID、TaskAlias、本方是否参与、互联协议等；
//   - Spec：由 createTaskSpec 构造。
func (h *RunningHandler) buildWillStartKusciaTask(kusciaJob *kusciaapisv1alpha1.KusciaJob, willStartTask []kusciaapisv1alpha1.KusciaTaskTemplate) ([]*kusciaapisv1alpha1.KusciaTask, error) {
	createdTasks := make([]*kusciaapisv1alpha1.KusciaTask, 0)
	isIcJob := isInterConnJob(kusciaJob)
	for i, t := range willStartTask {
		asParticipant, err := h.selfClusterAsParticipant(&willStartTask[i])
		if err != nil {
			return nil, err
		}
		var taskObject = &kusciaapisv1alpha1.KusciaTask{
			ObjectMeta: metav1.ObjectMeta{
				Name: t.TaskID,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(kusciaJob,
						kusciaapisv1alpha1.SchemeGroupVersion.WithKind(KusciaJobKind)),
				},
				Annotations: map[string]string{
					common.JobIDAnnotationKey:                    kusciaJob.Name,
					common.TaskAliasAnnotationKey:                t.Alias,
					common.SelfClusterAsParticipantAnnotationKey: strconv.FormatBool(asParticipant),
				},
				Labels: map[string]string{
					common.LabelController: LabelControllerValueKusciaJob,
					common.LabelJobUID:     string(kusciaJob.UID),
				},
			},
			Spec: h.createTaskSpec(kusciaJob.Spec.Initiator, t),
		}

		if isIcJob {
			// 互联互通任务：透传互联相关 annotations/labels。
			taskObject.Annotations[common.InterConnBFIAPartyAnnotationKey] = kusciaJob.Annotations[common.InterConnBFIAPartyAnnotationKey]
			taskObject.Annotations[common.InterConnKusciaPartyAnnotationKey] = kusciaJob.Annotations[common.InterConnKusciaPartyAnnotationKey]
			taskObject.Annotations[common.InterConnSelfPartyAnnotationKey] = kusciaJob.Annotations[common.InterConnSelfPartyAnnotationKey]
			taskObject.Annotations[common.InitiatorAnnotationKey] = kusciaJob.Annotations[common.InitiatorAnnotationKey]
			taskObject.Labels[common.LabelInterConnProtocolType] = kusciaJob.Labels[common.LabelInterConnProtocolType]
			if kusciaJob.Labels[common.LabelInterConnProtocolType] == string(kusciaapisv1alpha1.InterConnBFIA) {
				taskObject.Labels[common.LabelTaskUnschedulable] = common.True
			}
			if kusciaJob.Annotations[common.KusciaPartyMasterDomainAnnotationKey] != "" {
				taskObject.Annotations[common.KusciaPartyMasterDomainAnnotationKey] = kusciaJob.Annotations[common.KusciaPartyMasterDomainAnnotationKey]
			}
			if kusciaJob.Annotations[common.SelfClusterAsInitiatorAnnotationKey] != "" {
				taskObject.Annotations[common.SelfClusterAsInitiatorAnnotationKey] = kusciaJob.Annotations[common.SelfClusterAsInitiatorAnnotationKey]
			}
		}

		createdTasks = append(createdTasks, taskObject)
	}
	return createdTasks, nil
}

func (h *RunningHandler) selfClusterAsParticipant(task *kusciaapisv1alpha1.KusciaTaskTemplate) (bool, error) {
	for _, party := range task.Parties {
		isPartner, err := utilsres.IsPartnerDomain(h.namespaceLister, party.DomainID)
		if err != nil {
			return false, err
		}
		if !isPartner {
			return true, nil
		}
	}
	return false, nil
}

// createTaskSpec 根据 KusciaJob 中的 task 模板构造 KusciaTaskSpec。
func (h *RunningHandler) createTaskSpec(initiator string, t kusciaapisv1alpha1.KusciaTaskTemplate) kusciaapisv1alpha1.KusciaTaskSpec {
	result := kusciaapisv1alpha1.KusciaTaskSpec{
		Initiator:       initiator,
		TaskInputConfig: t.TaskInputConfig,
		Parties:         h.buildPartiesFromTaskInputConfig(t),
	}
	if t.ScheduleConfig != nil {
		result.ScheduleConfig = *t.ScheduleConfig
	}
	return result
}

// buildPartiesFromTaskInputConfig 根据 task 模板中的 parties 构造 KusciaTask 需要的 PartyInfo 列表。
func (h *RunningHandler) buildPartiesFromTaskInputConfig(template kusciaapisv1alpha1.KusciaTaskTemplate) []kusciaapisv1alpha1.PartyInfo {
	taskPartyInfos := make([]kusciaapisv1alpha1.PartyInfo, len(template.Parties))
	for i, p := range template.Parties {
		// build container resources of tasks
		tpl := h.buildPartyTemplate(p, template.AppImage)

		taskPartyInfos[i] = kusciaapisv1alpha1.PartyInfo{
			DomainID:       p.DomainID,
			AppImageRef:    template.AppImage,
			Role:           p.Role,
			Template:       tpl,
			BandwidthLimit: p.BandwidthLimit,
		}
	}
	return taskPartyInfos
}

// buildPartyTemplate 为单个 party 构造包含资源限制的 PartyTemplate。
//
// 流程：
//  1. 从 AppImage 中选择匹配 role 的 DeployTemplate；
//  2. 若用户指定了资源，则按容器实例数均分 limits/requests；
//  3. 将资源要求注入容器定义。
func (h *RunningHandler) buildPartyTemplate(p kusciaapisv1alpha1.Party, appImageName string) v1alpha1.PartyTemplate {
	var deployTemplate v1alpha1.DeployTemplate
	ptrDT, err := h.findMatchedDeployTemplate(p, appImageName)
	if err != nil {
		nlog.Warnf("Can not get suitable deployTemplate. err: %s", err.Error())
		return v1alpha1.PartyTemplate{}
	}

	deployTemplate = *ptrDT
	if deployTemplate.Replicas == nil {
		var replicas int32 = 1
		deployTemplate.Replicas = &replicas
	}
	totalInstances := len(deployTemplate.Spec.Containers) * int(*(deployTemplate.Replicas))

	var limitResource = corev1.ResourceList{}
	var requestResource = corev1.ResourceList{}

	// 若用户配置了资源，按总实例数均分到每个容器。
	if !utilsres.IsEmpty(p.Resources) {
		h.setResource(limitResource, p.Resources.Limits, corev1.ResourceCPU, totalInstances)
		h.setResource(limitResource, p.Resources.Limits, corev1.ResourceMemory, totalInstances)
		h.setResource(limitResource, p.Resources.Limits, common.ResourceBandwidth, totalInstances)

		h.setResource(requestResource, p.Resources.Requests, corev1.ResourceCPU, totalInstances)
		h.setResource(requestResource, p.Resources.Requests, corev1.ResourceMemory, totalInstances)
		h.setResource(requestResource, p.Resources.Requests, common.ResourceBandwidth, totalInstances)
	}

	containers := deployTemplate.Spec.Containers
	for ctrIdx := range containers {
		containers[ctrIdx].Resources = corev1.ResourceRequirements{
			Limits:   limitResource,
			Requests: requestResource,
		}
	}

	resources := v1alpha1.PartyTemplate{
		Spec: v1alpha1.PodSpec{
			Containers: containers,
		},
	}

	if utilsres.IsEmpty(p.Resources) {
		resources = v1alpha1.PartyTemplate{}
	}
	return resources
}

// setResource 将 sourceList 中的指定资源按 totalInstances 均分后写入 resourceMap。
func (h *RunningHandler) setResource(resourceMap, sourceList corev1.ResourceList, resourceName corev1.ResourceName, totalInstances int) {
	if !utilsres.IsEmpty(sourceList[resourceName]) {
		ptrValue := sourceList[resourceName]
		stringEveryResource, _ := utilsres.SplitRSC(ptrValue.String(), totalInstances)
		everyResource := k8sresource.MustParse(stringEveryResource)
		resourceMap[resourceName] = everyResource
	}
}

// findMatchedDeployTemplate 从 AppImage 中选择最匹配 party role 的 DeployTemplate。
func (h *RunningHandler) findMatchedDeployTemplate(p kusciaapisv1alpha1.Party, appImageName string) (*v1alpha1.DeployTemplate, error) {
	appImage, err := h.kusciaClient.KusciaV1alpha1().AppImages().Get(context.Background(), appImageName, metav1.GetOptions{})
	if err != nil {
		nlog.Warnf("Can not get appImage %s. error: %s", appImageName, err.Error())
		return nil, err
	}

	return utilsres.SelectDeployTemplate(appImage.Spec.DeployTemplates, p.Role)
}

// jobTaskSelector 构造用于筛选 KusciaJob 生成的 KusciaTask 的 labels.Selector。
//
// 选择条件：
//   - controller 为 kuscia-job；
//   - job-uid 与当前 Job 的 UID 一致。
func jobTaskSelector(jobUID string) (labels.Selector, error) {
	controllerEquals, err :=
		labels.NewRequirement(common.LabelController, selection.Equals, []string{LabelControllerValueKusciaJob})
	if err != nil {
		return nil, err
	}
	ownerEquals, err :=
		labels.NewRequirement(common.LabelJobUID, selection.Equals, []string{jobUID})
	if err != nil {
		return nil, err
	}

	return labels.NewSelector().Add(*controllerEquals, *ownerEquals), nil
}

// currentTask 在 KusciaTaskTemplate 基础上附加当前运行状态 Phase。
type currentTask struct {
	kusciaapisv1alpha1.KusciaTaskTemplate
	Phase *kusciaapisv1alpha1.KusciaTaskPhase
}

// currentTaskMap 以 task id 为 key 的当前任务视图，便于调度计算。
type currentTaskMap map[string]currentTask

// criticalTaskMap 返回所有非 tolerable 的关键任务。
func (c currentTaskMap) criticalTaskMap() currentTaskMap {
	criticalMap := currentTaskMap{}
	for k, t := range c {
		if t.Tolerable == nil || !*t.Tolerable {
			criticalMap[k] = t
		}
	}
	return criticalMap
}

// readyTaskMap 返回依赖已满足但尚未创建的任务（Phase 为 nil）。
func (c currentTaskMap) readyTaskMap() currentTaskMap {
	readyTaskMap := currentTaskMap{}
	for k, t := range c {
		if taskNotExists(t) && (len(t.Dependencies) == 0 || stringAllMatch(t.Dependencies, func(taskId string) bool {
			return c[taskId].Phase != nil && *c[taskId].Phase == kusciaapisv1alpha1.TaskSucceeded
		})) {
			readyTaskMap[k] = t
		}
	}
	return readyTaskMap
}

// ToShortString 输出 currentTaskMap 的简短调试字符串。
func (c currentTaskMap) ToShortString() string {
	var taskString = make([]string, 0)
	for _, t := range c {
		var phase = "nil"
		if t.Phase != nil {
			phase = string(*t.Phase)
		}
		tolerable := false
		if t.Tolerable != nil {
			tolerable = *t.Tolerable
		}
		taskString = append(taskString, fmt.Sprintf(
			"{taskId=%s, dependencies=%+v, tolerable=%+v, phase=%s}",
			t.TaskID, t.Dependencies, tolerable, phase))
	}
	return "{" + strings.Join(taskString, ",") + "}"
}

// AllMatch 判断是否所有任务都满足给定条件。
func (c currentTaskMap) AllMatch(p func(v currentTask) bool) bool {
	for _, v := range c {
		if !p(v) {
			return false
		}
	}
	return true
}

// AnyMatch 判断是否存在任务满足给定条件。
func (c currentTaskMap) AnyMatch(p func(v currentTask) bool) bool {
	for _, v := range c {
		if p(v) {
			return true
		}
	}
	return false
}

// taskFinished 判断任务是否已结束（Succeeded 或 Failed）。
func taskFinished(v currentTask) bool {
	return v.Phase != nil && (*v.Phase == kusciaapisv1alpha1.TaskSucceeded || *v.Phase == kusciaapisv1alpha1.TaskFailed)
}

// taskNotExists 判断任务是否尚未创建（Phase 为 nil）。
func taskNotExists(v currentTask) bool {
	return v.Phase == nil
}

// taskSucceeded 判断任务是否成功。
func taskSucceeded(v currentTask) bool {
	return v.Phase != nil && *v.Phase == kusciaapisv1alpha1.TaskSucceeded
}

// taskFailed 判断任务是否失败。
func taskFailed(v currentTask) bool {
	return v.Phase != nil && *v.Phase == kusciaapisv1alpha1.TaskFailed
}

// taskRunning 判断任务是否处于运行中（Running、Pending 或 phase 为空）。
func taskRunning(v currentTask) bool {
	return v.Phase != nil && (*v.Phase == kusciaapisv1alpha1.TaskRunning || *v.Phase == kusciaapisv1alpha1.TaskPending || *v.Phase == "")
}

// currentTaskMapFrom 根据 KusciaJob 与当前 task 状态构造 currentTaskMap。
func currentTaskMapFrom(kusciaJob *kusciaapisv1alpha1.KusciaJob, currentTaskStatus map[string]kusciaapisv1alpha1.KusciaTaskPhase) currentTaskMap {
	currentTasks := make(map[string]currentTask, 0)
	for _, t := range kusciaJob.Spec.Tasks {
		c := currentTask{
			KusciaTaskTemplate: t,
			Phase:              nil,
		}

		if phase, exist := currentTaskStatus[t.Alias]; exist {
			c.Phase = &phase
		}

		currentTasks[t.TaskID] = c
	}
	return currentTasks
}

// setKusciaJobStatus 设置 KusciaJobStatus 的 phase、reason、message 与时间戳。
func setKusciaJobStatus(now metav1.Time, status *kusciaapisv1alpha1.KusciaJobStatus, phase kusciaapisv1alpha1.KusciaJobPhase, reason, message string) {
	status.Phase = phase
	status.Reason = reason
	status.Message = message
	status.LastReconcileTime = &now
	if status.StartTime == nil {
		status.StartTime = &now
	}
}

// setRunningTaskStatusToFailed 将 job.Status.TaskStatus 中仍在运行/等待的 task 标记为 Failed。
func setRunningTaskStatusToFailed(status *kusciaapisv1alpha1.KusciaJobStatus) {
	for k, v := range status.TaskStatus {
		if v == kusciaapisv1alpha1.TaskPending || v == kusciaapisv1alpha1.TaskRunning {
			status.TaskStatus[k] = kusciaapisv1alpha1.TaskFailed
		}
	}
}

// setKusciaTaskStatus 设置 KusciaTaskStatus 的 phase、reason、message 与时间戳。
func setKusciaTaskStatus(now metav1.Time, status *kusciaapisv1alpha1.KusciaTaskStatus, phase kusciaapisv1alpha1.KusciaTaskPhase, reason, message string) {
	status.Phase = phase
	status.LastReconcileTime = &now
	status.Reason = reason
	status.Message = message
	if status.StartTime == nil {
		status.StartTime = &now
	}
}
