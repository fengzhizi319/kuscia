# Kuscia隐私计算调度架构与数据流详解

## 概述

本文档详细介绍Kuscia系统中接收SecretPad调度请求、并通过Ray实现SecretFlow隐私计算功能的完整架构和数据流。

## 核心模块架构

### 1. 整体架构图

```
┌──────────────┐
│   SecretPad  │ (调度发起方)
└───────┬──────┘
        │ kuscia_request_json (HTTP/gRPC API)
        ▼
┌──────────────────────────────────────┐
│       KusciaAPI Server               │ (API接入层)
│  pkg/kusciaapi/handler/httphandler   │
└───────┬──────────────────────────────┘
        │
        ▼
┌──────────────────────────────────────┐
│       Job Service                    │ (作业服务层)
│  pkg/kusciaapi/service/job_service.go│
│  - CreateJob                         │
│  - QueryJob                          │
│  - StopJob                           │
└───────┬──────────────────────────────┘
        │ 创建 KusciaJob CRD
        ▼
┌──────────────────────────────────────┐
│    KusciaJob Controller              │ (作业控制器)
│ pkg/controllers/kusciajob/           │
│  - 监听KusciaJob变化                 │
│  - 状态机管理                        │
└───────┬──────────────────────────────┘
        │ 创建 KusciaTask CRD
        ▼
┌──────────────────────────────────────┐
│    KusciaTask Controller             │ (任务控制器)
│ pkg/controllers/kusciatask/          │
│  - Pending Handler                   │
│  - Running Handler                   │
│  - Failed/Succeeded Handler          │
└───────┬──────────────────────────────┘
        │ 创建 TaskResourceGroup + Pod
        ▼
┌──────────────────────────────────────┐
│  TaskResourceGroup Controller        │ (资源组控制器)
│ pkg/controllers/taskresourcegroup/   │
│  - 资源预留                          │
│  - 生命周期管理                      │
└───────┬──────────────────────────────┘
        │ 触发Pod调度
        ▼
┌──────────────────────────────────────┐
│     Pods Controller (Agent)          │ (Pod控制器)
│ pkg/agent/framework/                 │
│  - SyncPod                           │
│  - 容器运行时管理                     │
└───────┬──────────────────────────────┘
        │ 创建容器
        ▼
┌──────────────────────────────────────┐
│    Container Runtime                 │ (容器运行时)
│ pkg/agent/kuberuntime/               │
│  - 拉取镜像(Ray/SecretFlow)          │
│  - 启动容器                          │
└───────┬──────────────────────────────┘
        │
        ▼
┌──────────────────────────────────────┐
│   Ray Cluster + SecretFlow           │ (计算引擎)
│  - Ray Head/Worker节点               │
│  - SecretFlow隐私计算任务            │
└──────────────────────────────────────┘
```

## 详细数据流

### 阶段一：API请求接收 (KusciaAPI)

#### 1.1 HTTP Handler入口

**文件位置**: `pkg/kusciaapi/handler/httphandler/job/create.go`

```go
// SecretPad发送POST请求到 /api/v1/kusciaapi/job/create
// 请求体包含CreateJobRequest结构
type CreateJobRequest struct {
    JobId          string         // 作业ID
    Initiator      string         // 发起方Domain ID
    MaxParallelism int32          // 最大并行度
    Tasks          []*TaskConfig  // 任务配置列表
}
```

**关键代码路径**:

- Handler: `pkg/kusciaapi/handler/httphandler/job/create.go`
- Service: `pkg/kusciaapi/service/job_service.go`

#### 1.2 Job Service处理

**文件**: `pkg/kusciaapi/service/job_service.go`

**核心函数**: `CreateJob()`

```go
func (h *jobService) CreateJob(ctx context.Context, request *kusciaapi.CreateJobRequest) *kusciaapi.CreateJobResponse {
    // 1. 参数验证
    if err := validateCreateJobRequest(request, h.Initiator); err != nil {
        return error response
    }
    
    // 2. 权限认证
    if err := h.authHandlerJobCreate(ctx, request); err != nil {
        return error response
    }
    
    // 3. 转换请求为KusciaJob CRD
    kusciaJob := &v1alpha1.KusciaJob{
        ObjectMeta: metav1.ObjectMeta{
            Name: request.JobId,
        },
        Spec: v1alpha1.KusciaJobSpec{
            Initiator:      request.Initiator,
            MaxParallelism: utils.IntValue(request.MaxParallelism),
            ScheduleMode:   v1alpha1.KusciaJobScheduleModeBestEffort,
            Tasks:          kusciaTasks, // 转换后的任务模板
        },
    }
    
    // 4. 创建KusciaJob到Kubernetes
    _, err := h.kusciaClient.KusciaV1alpha1().KusciaJobs(common.KusciaCrossDomain).Create(
        ctx, kusciaJob, metav1.CreateOptions{})
    
    return success response
}
```

**TaskConfig结构包含**:

```go
type TaskConfig struct {
    TaskId          string           // 任务ID
    Alias           string           // 任务别名
    Dependencies    []string         // 依赖任务
    AppImage        string           // 应用镜像(Ray/SecretFlow镜像)
    TaskInputConfig string           // 任务输入配置(JSON格式)
    Parties         []*Party         // 参与方列表
    Priority        int32            // 优先级
    ScheduleConfig  *ScheduleConfig  // 调度配置
}

type Party struct {
    DomainId   string              // 域ID
    Role       string              // 角色(如: alice, bob)
    Resources  *ResourceRequirements // CPU/Memory/Bandwidth
}
```

### 阶段二：KusciaJob控制器处理

#### 2.1 Job Controller监听

**文件**: `pkg/controllers/kusciajob/controller.go`

**工作流程**:

1. **Informer监听**: 通过Kubernetes Informer监听KusciaJob的增删改事件
2. **入队处理**: 将变化的Job加入工作队列
3. **Sync处理**: 从队列取出Job进行同步处理

```go
func (c *Controller) syncHandler(ctx context.Context, key string) (retErr error) {
    // 1. 获取KusciaJob
    curJob, err := c.kusciaJobLister.KusciaJobs(common.KusciaCrossDomain).Get(name)
    
    // 2. 设置默认值
    kusciaJobDefault(curJob)
    
    // 3. 检查是否需要协调
    if !handler.ShouldReconcile(curJob) {
        return nil
    }
    
    // 4. 状态机处理
    phase := curJob.Status.Phase
    needUpdate, err := c.handlerFactory.KusciaJobPhaseHandlerFor(phase).HandlePhase(curJob)
    
    // 5. 更新状态
    if needUpdate {
        utilsres.UpdateKusciaJobStatus(c.kusciaClient, preJob, curJob)
    }
}
```

#### 2.2 Job状态机

**状态流转**:

```
Initialized → PendingApproval → Running → Succeeded/Failed
                  ↓
            Rejected
```

**Handler工厂**: `pkg/controllers/kusciajob/handler/factory.go`

根据不同状态调用不同的Handler:

- `initialized_handler.go`: 初始化处理，创建KusciaTask
- `pending_approval_handler.go`: 等待审批
- `running_handler.go`: 运行中监控
- `succeeded_handler.go`: 成功清理
- `failed_handler.go`: 失败处理

**创建KusciaTask**:

```go
// 在initialized_handler中
for _, taskTemplate := range job.Spec.Tasks {
    kusciaTask := &v1alpha1.KusciaTask{
        ObjectMeta: metav1.ObjectMeta{
            Name: taskTemplate.TaskID,
            Annotations: map[string]string{
                common.JobIDAnnotationKey: job.Name,
            },
        },
        Spec: v1alpha1.KusciaTaskSpec{
            Initiator:       job.Spec.Initiator,
            TaskID:          taskTemplate.TaskID,
            Parties:         taskTemplate.Parties,
            AppImage:        taskTemplate.AppImage,
            TaskInputConfig: taskTemplate.TaskInputConfig,
        },
    }
    // 创建Task到K8s
    c.kusciaClient.KusciaV1alpha1().KusciaTasks(...).Create(...)
}
```

### 阶段三：KusciaTask控制器处理

#### 3.1 Task Controller

**文件**: `pkg/controllers/kusciatask/controller.go`

**核心职责**:

- 监听KusciaTask状态变化
- 管理Task的生命周期
- 创建底层资源(Pod、Service、ConfigMap)

```go
func (c *Controller) syncHandler(key string) (retErr error) {
    // 1. 获取Task
    kusciaTask, err := c.kusciaTaskLister.KusciaTasks(common.KusciaCrossDomain).Get(name)
    
    // 2. 获取当前阶段
    phase := kusciaTask.Status.Phase
    if phase == "" {
        phase = kusciaapisv1alpha1.TaskPending
    }
    
    // 3. 状态机处理
    needUpdate, err := c.handlerFactory.GetKusciaTaskPhaseHandler(phase).Handle(kusciaTask)
    
    // 4. 更新状态
    if needUpdate {
        c.updateTaskStatus(sharedTask, kusciaTask)
    }
}
```

#### 3.2 Pending Handler - 资源准备

**文件**: `pkg/controllers/kusciatask/handler/pending_handler.go`

这是**最关键的阶段**，负责创建所有运行时资源。

**Handle流程**:

```go
func (h *PendingHandler) Handle(kusciaTask *kusciaapisv1alpha1.KusciaTask) (needUpdate bool, err error) {
    // 1. 准备Task资源
    if needUpdate, err = h.prepareTaskResources(now, kusciaTask); needUpdate || err != nil {
        return needUpdate, err
    }
    
    // 2. 初始化Party状态
    h.initPartyTaskStatus(kusciaTask, curKtStatus)
    
    // 3. 刷新资源状态
    refreshKtResourcesStatus(h.kubeClient, h.podsLister, h.servicesLister, curKtStatus)
    
    // 4. 检查是否失败
    if updated := h.taskFailed(now, kusciaTask); updated {
        return updated, nil
    }
    
    // 5. 检查是否可以运行
    if updated, err := h.taskRunning(now, kusciaTask); updated || err != nil {
        return updated, err
    }
    
    return needUpdate, nil
}
```

**prepareTaskResources详细步骤**:

```go
func (h *PendingHandler) prepareTaskResources(now metav1.Time, kusciaTask *kusciaapisv1alpha1.KusciaTask) (needUpdate bool, err error) {
    // Step 1: 分配端口
    cond, found := utilsres.GetKusciaTaskCondition(&kusciaTask.Status, 
        kusciaapisv1alpha1.KusciaTaskCondPortsAllocated, true)
    if !found {
        needUpdate, err = h.allocatePorts(kusciaTask)
        if needUpdate {
            utilsres.SetKusciaTaskCondition(now, cond, v1.ConditionTrue, "", "")
            return true, nil
        }
    }
    
    // Step 2: 创建Task资源(Pod、Service、ConfigMap)
    cond, found = utilsres.GetKusciaTaskCondition(&kusciaTask.Status, 
        kusciaapisv1alpha1.KusciaTaskCondResourceCreated, true)
    if !found {
        if err = h.createTaskResources(kusciaTask); err != nil {
            return false, err
        }
        utilsres.SetKusciaTaskCondition(now, cond, v1.ConditionTrue, "", "")
        return true, nil
    }
    
    return false, nil
}
```

#### 3.3 创建Task资源

**createTaskResources函数**是核心中的核心:

```go
func (h *PendingHandler) createTaskResources(kusciaTask *kusciaapisv1alpha1.KusciaTask) error {
    // 1. 构建每个Party的Kit信息(包含Pod配置、端口、镜像等)
    partyKitInfos, selfPartyKitInfos, err := h.buildPartyKitInfos(kusciaTask)
    
    // 2. 填充分配的端口
    buildPodAllocatePorts(kusciaTask, selfPartyKitInfos)
    
    // 3. 生成分布式集群配置
    parties := generateParties(partyKitInfos)
    for _, partyKitInfo := range partyKitInfos {
        fillPartyClusterDefine(partyKitInfo, parties)
    }
    
    // 4. 为本域创建Pod和Service
    podStatuses := make(map[string]*kusciaapisv1alpha1.PodStatus)
    serviceStatuses := make(map[string]*kusciaapisv1alpha1.ServiceStatus)
    
    for _, partyKitInfo := range selfPartyKitInfos {
        // 插件准入检查
        permit, errors := h.pluginManager.Permit(context.Background(), *partyKitInfo)
        
        // 创建资源
        ps, ss, err := h.createResourceForParty(partyKitInfo)
        
        // 收集状态
        for key, v := range ps {
            podStatuses[key] = v
        }
        for key, v := range ss {
            serviceStatuses[key] = v
        }
    }
    
    kusciaTask.Status.PodStatuses = podStatuses
    kusciaTask.Status.ServiceStatuses = serviceStatuses
    
    // 5. 创建TaskResourceGroup
    if err := h.createTaskResourceGroup(kusciaTask, partyKitInfos); err != nil {
        return err
    }
    
    return nil
}
```

#### 3.4 构建Pod Kit信息

**buildPartyKitInfo函数** - 解析AppImage并构建Pod模板:

```go
func (h *PendingHandler) buildPartyKitInfo(kusciaTask *kusciaapisv1alpha1.KusciaTask, 
    party *kusciaapisv1alpha1.PartyInfo) (*PartyKitInfo, error) {
    
    // 1. 获取AppImage CRD
    appImage, err := h.appImagesLister.AppImages(common.KusciaCrossDomain).Get(party.AppImage)
    
    // 2. 提取部署模板
    deployTemplate := appImage.Spec.DeployTemplates[0]
    
    // 3. 合并Party特定配置
    if party.Template != nil {
        deployTemplate = mergeDeployTemplate(deployTemplate, party.Template)
    }
    
    // 4. 提取端口配置
    ports, err := mergeContainersPorts(deployTemplate.Spec.Containers)
    servicedPorts := generateServicedPorts(ports)
    
    // 5. 生成Pod列表(支持多副本)
    replicas := int(*deployTemplate.Replicas)
    pods := make([]*PodKitInfo, replicas)
    for i := 0; i < replicas; i++ {
        podName := generatePodName(kusciaTask.Name, party.Role, i)
        podIdentity := generatePodIdentity(string(kusciaTask.UID), party.Role, i)
        
        pods[i] = &PodKitInfo{
            PodName:     podName,
            PodIdentity: podIdentity,
            Ports:       ports,
            PortService: generatePortServices(podName, servicedPorts),
        }
    }
    
    // 6. 构建PartyKitInfo
    kit := &PartyKitInfo{
        DomainID:        party.DomainID,
        Role:            party.Role,
        Image:           appImage.Spec.Image,
        ImageID:         appImage.Spec.ImageID,
        DeployTemplate:  deployTemplate,
        ConfigTemplates: appImage.Spec.ConfigTemplates,
        ServicedPorts:   servicedPorts,
        Pods:            pods,
    }
    
    return kit, nil
}
```

#### 3.5 创建Pod

**createResourceForParty函数** - 实际创建Kubernetes资源:

```go
func (h *PendingHandler) createResourceForParty(partyKit *PartyKitInfo) (
    map[string]*kusciaapisv1alpha1.PodStatus,
    map[string]*kusciaapisv1alpha1.ServiceStatus, error) {
    
    podStatuses := make(map[string]*kusciaapisv1alpha1.PodStatus)
    serviceStatuses := make(map[string]*kusciaapisv1alpha1.ServiceStatus)
    
    for _, podKit := range partyKit.Pods {
        // 1. 生成Pod对象
        pod, err := h.generatePod(partyKit, podKit)
        
        // 2. 提交Pod到K8s
        createdPod, err := h.submitPod(pod)
        
        // 3. 记录Pod状态
        podStatuses[pod.Namespace+"/"+pod.Name] = &kusciaapisv1alpha1.PodStatus{
            Namespace: pod.Namespace,
            PodName:   pod.Name,
        }
        
        // 4. 为需要暴露的端口创建Service
        for portName, serviceName := range podKit.PortService {
            port := podKit.Ports[portName]
            service, err := generateServices(partyKit, pod, serviceName, port)
            
            createdSvc, err := h.submitService(service)
            
            serviceStatuses[service.Namespace+"/"+service.Name] = &kusciaapisv1alpha1.ServiceStatus{
                Namespace:   service.Namespace,
                ServiceName: service.Name,
                PortName:    portName,
                PortNumber:  port.Port,
                Scope:       port.Scope,
            }
        }
    }
    
    return podStatuses, serviceStatuses, nil
}
```

**generatePod函数** - 生成完整的Pod Spec:

```go
func (h *PendingHandler) generatePod(partyKit *PartyKitInfo, podKit *PodKitInfo) (*v1.Pod, error) {
    // 1. 设置Labels和Annotations
    labels := map[string]string{
        common.LabelController:            common.ControllerKusciaTask,
        common.LabelTaskUID:               string(partyKit.KusciaTask.UID),
        labelKusciaTaskPodIdentity:        podKit.PodIdentity,
    }
    
    annotations := map[string]string{
        common.InitiatorAnnotationKey: partyKit.KusciaTask.Spec.Initiator,
        common.TaskIDAnnotationKey:    partyKit.KusciaTask.Name,
        common.ImageIDAnnotationKey:   partyKit.ImageID,
    }
    
    // 2. 确定调度器
    schedulerName := common.KusciaSchedulerName
    if ns.Labels[common.LabelDomainRole] == string(kusciaapisv1alpha1.Partner) {
        schedulerName = fmt.Sprintf("%v-%v", partyKit.DomainID, schedulerName)
    }
    
    // 3. 构建Pod Spec
    pod := &v1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name:        podKit.PodName,
            Namespace:   partyKit.DomainID,
            Labels:      labels,
            Annotations: annotations,
        },
        Spec: v1.PodSpec{
            RestartPolicy: restartPolicy,
            Tolerations: []v1.Toleration{
                {
                    Key:      common.KusciaTaintTolerationKey,
                    Operator: v1.TolerationOpExists,
                    Effect:   v1.TaintEffectNoSchedule,
                },
            },
            NodeSelector: map[string]string{
                common.LabelNodeNamespace: partyKit.DomainID,
            },
            SchedulerName:                schedulerName,
            AutomountServiceAccountToken: &automountServiceAccountToken,
        },
    }
    
    // 4. 添加容器
    for _, ctr := range partyKit.DeployTemplate.Spec.Containers {
        resCtr := v1.Container{
            Name:            ctr.Name,
            Image:           partyKit.Image,  // Ray/SecretFlow镜像
            Command:         ctr.Command,
            Args:            ctr.Args,
            Env:             ctr.Env,
            Ports:           []v1.ContainerPort{},
            Resources:       ctr.Resources,
            ImagePullPolicy: ctr.ImagePullPolicy,
        }
        
        // 5. 配置端口
        for _, port := range ctr.Ports {
            namedPort, ok := podKit.Ports[port.Name]
            resPort := v1.ContainerPort{
                Name:          port.Name,
                ContainerPort: namedPort.Port,
                Protocol:      v1.ProtocolTCP,
            }
            resCtr.Ports = append(resCtr.Ports, resPort)
        }
        
        // 6. 挂载配置卷
        if len(ctr.ConfigVolumeMounts) > 0 {
            // 创建ConfigMap包含集群配置
            confMap := generateKusciaConfigMap(partyKit, podKit)
            h.submitConfigMap(confMap)
            
            // 挂载ConfigMap
            pod.Spec.Volumes = append(pod.Spec.Volumes, v1.Volume{
                Name: configTemplateVolumeName,
                VolumeSource: v1.VolumeSource{
                    ConfigMap: &v1.ConfigMapVolumeSource{
                        LocalObjectReference: v1.LocalObjectReference{
                            Name: partyKit.ConfigTemplatesCMName,
                        },
                    },
                },
            })
        }
        
        pod.Spec.Containers = append(pod.Spec.Containers, resCtr)
    }
    
    return pod, nil
}
```

**ConfigMap内容** - 包含分布式计算所需的关键配置:

```go
func generateKusciaConfigMap(partyKit *PartyKitInfo, podKit *PodKitInfo) *v1.ConfigMap {
    protoJSONOptions := protojson.MarshalOptions{EmitUnpopulated: true}
    
    // 序列化集群定义
    clusterDefine, _ := protoJSONOptions.Marshal(podKit.ClusterDef)
    // 序列化分配的端口
    allocatedPorts, _ := protoJSONOptions.Marshal(podKit.AllocatedPorts)
    
    confMap := make(map[string]string)
    confMap[common.EnvDomainID] = partyKit.DomainID
    confMap[common.EnvTaskID] = partyKit.KusciaTask.Name
    confMap[common.EnvTaskClusterDefine] = string(clusterDefine)  // 集群拓扑
    confMap[common.EnvAllocatedPorts] = string(allocatedPorts)    // 端口映射
    confMap[common.EnvTaskInputConfig] = partyKit.KusciaTask.Spec.TaskInputConfig
    
    return &v1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf(common.KusciaGenerateConfigMapFormat, partyKit.KusciaTask.Name),
            Namespace: partyKit.DomainID,
        },
        Data: confMap,
    }
}
```

**ClusterDef结构示例** (Ray集群配置):

```protobuf
message ClusterDef {
    repeated Party parties = 1;
}

message Party {
    string domain_id = 1;
    string role = 2;
    repeated Service services = 3;
}

message Service {
    string port_name = 1;
    repeated string endpoints = 2;  // 各节点的endpoint地址
}
```

### 阶段四：TaskResourceGroup控制器

#### 4.1 TRG Controller职责

**文件**: `pkg/controllers/taskresourcegroup/controller.go`

**核心功能**:

1. **资源预留管理**: 确保所有参与方的资源都就绪
2. **生命周期控制**: 管理Task资源的超时和重试
3. **跨域协调**: 协调不同域的TaskResource状态

```go
func (c *Controller) syncHandler(ctx context.Context, key string) (err error) {
    // 1. 获取TaskResourceGroup
    trg, err := c.trgLister.Get(key)
    
    // 2. 检查过期
    if c.needHandleExpiredTrg(trg) {
        return nil
    }
    
    // 3. 状态机处理
    phase := trg.Status.Phase
    if phase == "" {
        phase = kusciaapisv1alpha1.TaskResourceGroupPhasePending
    }
    
    needUpdate, err := c.handlerFactory.GetTaskResourceGroupPhaseHandler(phase).Handle(trg)
    
    // 4. 更新状态
    if needUpdate {
        c.updateTaskResourceGroupStatus(ctx, rawTrg, trg)
    }
    
    return err
}
```

**TRG状态流转**:

```
Pending → Reserving → Reserved → Released
                    ↓
                ReserveFailed (可重试)
                    ↓
                Failed
```

### 阶段五：Agent Pod控制器

#### 5.1 Pods Controller

**文件**: `pkg/agent/framework/pods_controller.go`

当Pod被调度到节点后，Kuscia Agent接管Pod的生命周期管理。

**SyncLoop主循环**:

```go
func (pc *PodsController) syncLoop(ctx context.Context, handler SyncHandler, updates <-chan kubetypes.PodUpdate) {
    for {
        pc.syncLoopIteration(ctx, handler, updates, housekeepingTicker.C)
    }
}

func (pc *PodsController) syncLoopIteration(...) bool {
    select {
    case u, open := <-configCh:
        switch u.Op {
        case kubetypes.ADD:
            handler.HandlePodAdditions(u.Pods)
        case kubetypes.UPDATE:
            handler.HandlePodUpdates(u.Pods)
        case kubetypes.REMOVE:
            handler.HandlePodRemoves(u.Pods)
        }
    }
}
```

**HandlePodAdditions**:

```go
func (pc *PodsController) HandlePodAdditions(pods []*corev1.Pod) {
    for _, pod := range pods {
        // 1. 添加到Pod管理器
        pc.podManager.AddPod(pod)
        
        // 2. 执行Hook插件
        hook.Execute(&hook.PodAdditionContext{
            Pod:         pod,
            PodProvider: pc.provider,
        })
        
        // 3. 分发到Pod Worker进行异步同步
        pc.dispatchWork(pod, kubetypes.SyncPodCreate, mirrorPod, start)
    }
}
```

#### 5.2 SyncPod - 容器运行时同步

**syncPod函数**是Agent的核心:

```go
func (pc *PodsController) syncPod(ctx context.Context, updateType kubetypes.SyncPodType, 
    pod, mirrorPod *corev1.Pod, podStatus *pkgcontainer.PodStatus) (isTerminal bool, err error) {
    
    // 1. 生成API Pod状态
    apiPodStatus := pc.generateAPIPodStatus(pod, podStatus)
    
    // 2. 更新状态管理器
    pc.statusManager.SetPodStatus(pod, apiPodStatus)
    
    // 3. 创建Mirror Pod(如果是Static Pod)
    if kubetypes.IsStaticPod(pod) {
        pc.podManager.CreateMirrorPod(pod)
    }
    
    // 4. 构造镜像地址(如果需要)
    podCopy := pod.DeepCopy()
    pc.constructPodImage(podCopy)
    
    // 5. 调用容器运行时SyncPod
    if err = pc.provider.SyncPod(ctx, podCopy, podStatus, pc.reasonCache); err != nil {
        return false, err
    }
    
    return false, nil
}
```

#### 5.3 KubeRuntime - 容器运行时实现

**文件**: `pkg/agent/kuberuntime/kuberuntime_manager.go`

**SyncPod实现**:

```go
func (m *kubeRuntimeManager) SyncPod(ctx context.Context, pod *v1.Pod, 
    podStatus *pkgcontainer.PodStatus, reasonCache *ReasonCache) error {
    
    // 1. 创建Pod Sandbox
    podSandboxID, msg, err := m.createPodSandbox(ctx, pod, attempt)
    
    // 2. 启动Init Containers
    for _, container := range pod.Spec.InitContainers {
        m.startContainer(ctx, pod, podSandboxID, &container, ...)
    }
    
    // 3. 启动主Containers (Ray/SecretFlow容器)
    for _, container := range pod.Spec.Containers {
        m.startContainer(ctx, pod, podSandboxID, &container, ...)
    }
    
    return nil
}
```

**startContainer流程**:

```go
func (m *kubeRuntimeManager) startContainer(ctx context.Context, pod *v1.Pod, 
    podSandboxID string, container *v1.Container, ...) error {
    
    // 1. 拉取镜像
    imageRef, msg, err := m.imagePuller.PullImage(ctx, pod, container, auth)
    
    // 2. 创建容器
    containerID, err := m.runtimeService.CreateContainer(ctx, &runtimeapi.CreateContainerRequest{
        PodSandboxId:  podSandboxID,
        Config:        containerConfig,
        SandboxConfig: podSandboxConfig,
    })
    
    // 3. 启动容器
    err = m.runtimeService.StartContainer(ctx, containerID.ID)
    
    return nil
}
```

### 阶段六：Ray集群与SecretFlow执行

#### 6.1 容器启动后的行为

当Pod中的容器启动后，根据AppImage中定义的Command和Args，执行以下操作:

**典型Ray Head节点启动命令**:

```bash
ray start --head \
  --port=6379 \
  --dashboard-port=8265 \
  --node-ip-address=$POD_IP \
  --resources='{"CPU": 4, "memory": 8589934592}'
```

**Ray Worker节点启动命令**:

```bash
ray start \
  --address=$RAY_HEAD_SERVICE.alice.svc:6379 \
  --node-ip-address=$POD_IP \
  --resources='{"CPU": 4, "memory": 8589934592}'
```

**SecretFlow任务启动**:

```python
import secretflow as sf
import ray

# 连接到Ray集群
ray.init(address='auto')

# 初始化SecretFlow
sf.init(parties=['alice', 'bob'], address='auto')

# 加载任务配置
task_config = json.loads(os.environ['TASK_INPUT_CONFIG'])

# 执行隐私计算任务
if __name__ == '__main__':
    # 根据TaskInputConfig执行具体的MPC/FL任务
    result = execute_privacy_computing_task(task_config)
```

#### 6.2 任务输入配置 (TaskInputConfig)

**JSON格式示例**:

```json
{
  "task_type": "psi",  // 隐私集合求交
  "parties": ["alice", "bob"],
  "input_data": {
    "alice": {
      "data_source": "alice_db",
      "table": "user_list_a"
    },
    "bob": {
      "data_source": "bob_db",
      "table": "user_list_b"
    }
  },
  "protocol": "ECDH",
  "output": {
    "result_path": "/tmp/psi_result"
  }
}
```

## 关键数据结构

### 1. KusciaJob CRD

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: job-psi-001
spec:
  initiator: alice
  maxParallelism: 1
  scheduleMode: BestEffort
  tasks:
    - taskId: task-psi-001
      alias: psi-task
      appImage: ray-secretflow:latest
      taskInputConfig: '{"task_type": "psi", ...}'
      parties:
        - domainId: alice
          role: alice
          resources:
            cpu: "4"
            memory: 8Gi
        - domainId: bob
          role: bob
          resources:
            cpu: "4"
            memory: 8Gi
```

### 2. KusciaTask CRD

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaTask
metadata:
  name: task-psi-001
  annotations:
    job-id: job-psi-001
spec:
  initiator: alice
  taskId: task-psi-001
  parties:
    - domainId: alice
      role: alice
    - domainId: bob
      role: bob
  appImage: ray-secretflow:latest
  taskInputConfig: '{"task_type": "psi", ...}'
status:
  phase: Running
  podStatuses:
    alice/task-psi-001-alice-0:
      namespace: alice
      podName: task-psi-001-alice-0
      phase: Running
  serviceStatuses:
    alice/task-psi-001-alice-0-ray-head:
      namespace: alice
      serviceName: task-psi-001-alice-0-ray-head
      portNumber: 6379
```

### 3. TaskResourceGroup CRD

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: TaskResourceGroup
metadata:
  name: task-psi-001
spec:
  minReservedMembers: 2
  resourceReservedSeconds: 30
  lifecycleSeconds: 300
  retryIntervalSeconds: 30
  initiator: alice
  parties:
    - domainId: alice
      role: alice
      minReservedPods: 1
      pods:
        - name: task-psi-001-alice-0
status:
  phase: Reserved
```

## 完整数据流时序图

```
SecretPad                    KusciaAPI              Job Controller        Task Controller       Agent           Ray/SecretFlow
    |                           |                        |                      |                  |                  |
    |-- CreateJob Request ---->|                        |                      |                  |                  |
    |                           |-- Validate & Auth -->|                      |                  |                  |
    |                           |-- Create KusciaJob -->|                      |                  |                  |
    |                           |<-- Job Created ------|                      |                  |                  |
    |                           |                        |                      |                  |                  |
    |                           |                        |-- Watch Job Change ->|                  |                  |
    |                           |                        |-- Create KusciaTask->|                  |                  |
    |                           |                        |                      |                  |                  |
    |                           |                        |                      |-- Task Pending ->|                  |
    |                           |                        |                      |-- Allocate Ports->|                  |
    |                           |                        |                      |-- Build Pod Spec->|                  |
    |                           |                        |                      |-- Create Pod ---->|                  |
    |                           |                        |                      |-- Create Service->|                  |
    |                           |                        |                      |-- Create ConfigMap>|                  |
    |                           |                        |                      |-- Create TRG ---->|                  |
    |                           |                        |                      |                  |                  |
    |                           |                        |                      |                  |-- Schedule Pod ->|
    |                           |                        |                      |                  |-- Pull Image --->|
    |                           |                        |                      |                  |-- Start Container>|
    |                           |                        |                      |                  |                  |-- Ray Start ---->
    |                           |                        |                      |                  |                  |-- SF Init ------>
    |                           |                        |                      |                  |                  |-- Execute Task ->
    |                           |                        |                      |                  |                  |
    |                           |                        |<-- Task Running -----|                  |                  |
    |<-- Job Running -----------|                        |                      |                  |                  |
    |                           |                        |                      |                  |                  |
    |                           |                        |                      |                  |<-- Task Complete-|
    |                           |                        |<-- Task Succeeded ---|                  |                  |
    |<-- Job Succeeded ---------|                        |                      |                  |                  |
```

## 关键配置项

### 1. AppImage注册

在使用前需要先注册AppImage:

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: AppImage
metadata:
  name: ray-secretflow-v1.0
spec:
  image: secretflow/ray:latest
  configTemplates:
    ray.conf: |
      [ray]
      head-port=6379
      dashboard-port=8265
  deployTemplates:
    - replicas: 1
      spec:
        containers:
          - name: ray-head
            command: ["/bin/bash", "-c"]
            args: ["ray start --head --port=6379 && python /app/task.py"]
            ports:
              - name: ray-head
                port: 6379
                scope: Domain
              - name: dashboard
                port: 8265
                scope: Cluster
            resources:
              requests:
                cpu: "4"
                memory: 8Gi
              limits:
                cpu: "4"
                memory: 8Gi
```

### 2. 调度配置

```go
type ScheduleConfig struct {
    TaskTimeoutSeconds                  int32  // 任务超时时间(默认300s)
    ResourceReservedSeconds             int32  // 资源预留时间(默认30s)
    ResourceReallocationIntervalSeconds int32  // 资源重新分配间隔(默认30s)
    MinReservedMembers                  int32  // 最小参与成员数
}
```

## 总结

Kuscia通过多层控制器架构实现了从SecretPad调度请求到Ray/SecretFlow隐私计算执行的完整流程:

1. **KusciaAPI层**: 接收HTTP/gRPC请求，验证并转换为Kubernetes CRD
2. **Job Controller层**: 管理作业生命周期，创建Task
3. **Task Controller层**: 核心调度逻辑，分配资源，创建Pod/Service/ConfigMap
4. **TaskResourceGroup Controller层**: 跨域资源协调和生命周期管理
5. **Agent层**: 节点级Pod管理，容器运行时集成
6. **Ray/SecretFlow层**: 实际的隐私计算执行引擎

整个系统基于Kubernetes的声明式API和控制器模式，实现了高可用、可扩展的分布式隐私计算调度平台。
