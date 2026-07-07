# 1 典型任务调度流程

## 1.1 整体流程图

```text
SecretPad / 业务系统
        │
        │ 调用 KusciaAPI CreateJob
        ▼
┌──────────────────┐
│ KusciaAPI Server │  pkg/kusciaapi/handler/httphandler/
└────────┬─────────┘
         │ 创建 KusciaJob CR
         ▼
┌────────────────────┐
│ KusciaJob Controller│ pkg/controllers/kusciajob/
└────────┬───────────┘
         │ 按 DAG 创建 KusciaTask CR
         ▼
┌────────────────────┐
│ KusciaTask Controller│ pkg/controllers/kusciatask/
└────────┬───────────┘
         │ 创建 TaskResourceGroup / Pod / Service / ConfigMap
         ▼
┌─────────────────────────┐
│ TaskResourceGroup Controller│ pkg/controllers/taskresourcegroup/
└────────┬────────────────┘
         │ 资源预留协调
         ▼
┌────────────────────┐
│ Kuscia Scheduler   │ pkg/scheduler/kusciascheduling/
└────────┬───────────┘
         │ 绑定 Pod 到 Node
         ▼
┌────────────────────┐
│ Agent / Pods Controller│ pkg/agent/framework/pods_controller.go
└────────┬───────────┘
         │ 拉取镜像、启动容器
         │
         │
         │
         ▼
┌────────────────────┐
│ 引擎（Ray + SecretFlow）│
└────────┬───────────┘
         │ 通过 DataMesh 读取/写入数据
         ▼
┌────────────────────┐
│      DataMesh      │
└────────────────────┘
```

## 1.2 后端提交给 Kuscia 的计算任务格式

### 1.2.1 KusciaAPI CreateJob 请求格式

当 SecretPad 或业务系统调用 KusciaAPI 的 `CreateJob` 接口时，需要提交以下格式的 JSON 数据：

```json
{
  "header": {
    "id": "request-id-xxx",
    "version": "v1alpha1"
  },
  "job_id": "psi-demo-job-001",
  "initiator": "alice",
  "max_parallelism": 2,
  "tasks": [
    {
      "alias": "preprocess",
      "task_id": "preprocess-task",
      "app_image": "secretflow-image:latest",
      "dependencies": [],
      "priority": 100,
      "tolerable": false,
      "task_input_config": "{...SecretFlow计算配置JSON字符串...}",
      "parties": [
        {
          "domain_id": "alice",
          "role": "server",
          "resources": {
            "cpu": "4",
            "memory": "8Gi",
            "bandwidth": "100"
          }
        }
      ],
      "schedule_config": {
        "task_timeout_seconds": 3600,
        "resource_reserved_seconds": 300,
        "resource_reallocation_interval_seconds": 10
      }
    },
    {
      "alias": "psi",
      "task_id": "psi-task",
      "app_image": "secretflow-image:latest",
      "dependencies": ["preprocess"],
      "priority": 90,
      "tolerable": false,
      "task_input_config": "{...PSI计算配置JSON字符串...}",
      "parties": [
        {
          "domain_id": "alice",
          "role": "server",
          "resources": {
            "cpu": "8",
            "memory": "16Gi",
            "bandwidth": "100"
          },
          "bandwidth_limits": [
            {
              "destination_id": "bob",
              "limit_kbps": 102400
            }
          ]
        },
        {
          "domain_id": "bob",
          "role": "client",
          "resources": {
            "cpu": "8",
            "memory": "16Gi",
            "bandwidth": "100"
          },
          "bandwidth_limits": [
            {
              "destination_id": "alice",
              "limit_kbps": 102400
            }
          ]
        }
      ]
    }
  ],
  "custom_fields": {
    "business_scene": "marketing_analysis",
    "owner": "data_team"
  }
}
```

**关键字段说明：**

| 字段 | 类型 | 必填 | 说明 |
| ------ | ------ | ------ | ------ |
| `job_id` | string | 是 | 作业唯一标识，需符合 DNS_LABEL 规范（小写字母、数字、连字符） |
| `initiator` | string | 是 | 任务发起方域名（DomainID） |
| `max_parallelism` | int32 | 否 | 最大并发任务数，默认 1，范围 1-128 |
| `tasks` | array | 是 | 任务列表，构成 DAG 图，最多 128 个任务 |
| `tasks[].alias` | string | 是 | 任务别名，在 Job 内唯一，用于依赖引用 |
| `tasks[].task_id` | string | 否 | 任务 ID，若不填则由 Controller 自动生成（格式：`jobName-uuid`） |
| `tasks[].app_image` | string | 是 | 应用镜像名称，需在 Kuscia 中预先注册 |
| `tasks[].dependencies` | array | 否 | 前置任务的 alias 列表，必须全部 Succeeded 后本任务才就绪 |
| `tasks[].priority` | int32 | 否 | 优先级，值越大越先调度，默认 0 |
| `tasks[].tolerable` | bool | 否 | 是否可容忍失败，默认 false。true 表示失败不影响 Job 最终成败 |
| `tasks[].task_input_config` | string | 是 | **SecretFlow 计算配置（JSON 字符串）**，详见下文 |
| `tasks[].parties` | array | 是 | 参与方列表 |
| `tasks[].parties[].domain_id` | string | 是 | 参与方域名 ID |
| `tasks[].parties[].role` | string | 否 | 角色，如 server/client、guest/host 等 |
| `tasks[].parties[].resources` | object | 否 | 资源配置 |
| `tasks[].parties[].resources.cpu` | string | 否 | CPU 核心数，如 "4"、"500m" |
| `tasks[].parties[].resources.memory` | string | 否 | 内存大小，如 "8Gi"、"512Mi" |
| `tasks[].parties[].resources.bandwidth` | string | 否 | 带宽限制（Mbps），不带单位 |
| `tasks[].parties[].bandwidth_limits` | array | 否 | 针对特定目标域的带宽限制 |
| `tasks[].schedule_config` | object | 否 | 调度配置 |
| `tasks[].schedule_config.task_timeout_seconds` | int32 | 否 | 任务超时时间（秒） |
| `tasks[].schedule_config.resource_reserved_seconds` | int32 | 否 | 资源预留时长（秒） |

### 1.2.2 taskInputConfig 格式详解

`taskInputConfig` 是一个 **JSON 字符串**，包含传递给 SecretFlow 引擎的具体计算配置。不同算法的配置结构不同。

#### 示例 1：ECDH PSI 算法配置

```json
{
  "name": "ic_psi_ecdh_1",
  "module_name": "ic-ecdh",
  "output": [
    {"type": "dataset", "key": "data"},
    {"type": "report", "key": "summary"}
  ],
  "role": {
    "host": ["bob"],
    "guest": ["alice"]
  },
  "initiator": {
    "role": "guest",
    "node_id": "alice"
  },
  "task_params": {
    "host": {
      "0": {
        "rank": 1,
        "field_names": "id",
        "name": "breast_hetero_host.csv",
        "namespace": "data"
      }
    },
    "guest": {
      "0": {
        "namespace": "data",
        "name": "breast_hetero_guest.csv",
        "rank": 0,
        "field_names": "id"
      }
    },
    "common": {
      "result_to_rank": -1,
      "algo": "ecdh_psi",
      "protocol_families": "ecc",
      "curve_type": "curve25519",
      "hash_type": "sha_256",
      "hash2curve_strategy": "direct_hash_as_point_x",
      "point_octet_format": "uncompressed",
      "bit_length_after_truncated": -1
    }
  }
}
```

**字段说明：**

- `name`: 任务名称
- `module_name`: 模块名称（对应 AppImage 中的配置模板）
- `output`: 输出定义，包括数据集和报告
- `role`: 各参与方的角色分配
- `initiator`: 发起方信息
- `task_params`: 任务参数
  - `host/guest`: 各角色的具体参数（rank、数据文件名、命名空间、字段名等）
  - `common`: 通用算法参数（算法类型、加密曲线、哈希算法等）

#### 示例 2：SecretFlow 原生 PSI 配置

对于使用 SecretFlow 原生框架的任务，`taskInputConfig` 可能采用不同的结构：

```json
{
  "party": "alice",
  "peer_party": "bob",
  "input_data": [
    {
      "party": "alice",
      "domain_data_id": "alice_user_ids",
      "features": ["id"]
    },
    {
      "party": "bob",
      "domain_data_id": "bob_user_ids",
      "features": ["id"]
    }
  ],
  "output_data": [
    {
      "party": "alice",
      "domain_data_id": "psi_result_alice"
    },
    {
      "party": "bob",
      "domain_data_id": "psi_result_bob"
    }
  ],
  "algorithm": "semi_ecc_psi",
  "curve": "CURVE_25519",
  "protection_kind": "DT_HALF"
}
```

#### 示例 3：联邦学习配置

```json
{
  "module_name": "hetero_lr",
  "train_data": {
    "alice": {
      "domain_data_id": "alice_train",
      "label_col": "y",
      "feature_cols": ["x1", "x2", "x3"]
    },
    "bob": {
      "domain_data_id": "bob_train",
      "feature_cols": ["x4", "x5"]
    }
  },
  "eval_data": {
    "alice": {
      "domain_data_id": "alice_eval"
    },
    "bob": {
      "domain_data_id": "bob_eval"
    }
  },
  "hyper_params": {
    "epochs": 10,
    "batch_size": 64,
    "learning_rate": 0.01,
    "penalty": "l2"
  }
}
```

### 1.2.3 通过 kubectl 直接提交 YAML 格式

除了 API 调用，也可以直接使用 kubectl 提交 YAML 格式的 KusciaJob：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-demo-job
  namespace: cross-domain
spec:
  initiator: alice
  scheduleMode: Strict
  maxParallelism: 2
  tasks:
    - alias: preprocess
      taskID: preprocess-task
      appImage: secretflow-image:latest
      priority: 100
      tolerable: false
      taskInputConfig: '{"local_processing": true, "input_files": ["alice.id", "alice.label"], "output_file": "alice_features.csv"}'
      parties:
        - domainID: alice
          role: local
          resources:
            cpu: "4"
            memory: "8Gi"
    - alias: psi
      taskID: psi-task
      dependencies: ['preprocess']
      appImage: secretflow-image:latest
      priority: 90
      tolerable: false
      taskInputConfig: '{"name":"psi_task","module_name":"psi","role":{"server":["alice"],"client":["bob"]},"task_params":{"server":{"0":{"namespace":"data","name":"alice_features.csv","field_names":"id"}},"client":{"0":{"namespace":"data","name":"bob.id","field_names":"id"}},"common":{"algo":"ecdh_psi","curve_type":"curve25519"}}}'
      parties:
        - domainID: alice
          role: server
          resources:
            cpu: "8"
            memory: "16Gi"
          bandwidthLimits:
            - destinationID: bob
              limitKBps: 102400
        - domainID: bob
          role: client
          resources:
            cpu: "8"
            memory: "16Gi"
          bandwidthLimits:
            - destinationID: alice
              limitKBps: 102400
```

## 1.3 状态机

**KusciaJob 状态机：**

```text
Initialized → PendingApproval → Running → Succeeded / Failed
                  ↓
              Rejected
```

**KusciaTask 状态机：**

```text
Pending → Running → Succeeded / Failed
```

**对应的函数接口：**

- `kusciaJobDefault()` - 设置KusciaJob默认字段
- `failKusciaJob()` - 处理KusciaJob失败状态
- `syncHandler()` - 同步处理KusciaJob状态
- `handlerFactory.KusciaJobPhaseHandlerFor(phase).HandlePhase()` - 根据阶段处理KusciaJob
- `NewController()` - 创建KusciaJob控制器实例
- `Run()` - 启动控制器运行
- `enqueueKusciaJob()` - 将KusciaJob加入工作队列
- `handleTaskObject()` - 处理任务对象事件

## 1.4 P2P 模式下的资源同步流程

在 P2P 组网中，调度方与参与方各自拥有独立的 K3s 控制平面：

1. Task Controller 在各参与方 Namespace 下创建 TaskResource 和 PodGroup。
2. InterConn Controller 将本方的 TaskResource / PodGroup 同步到参与方集群。
3. Kuscia Scheduler 为 PodGroup 预留资源，满足 `MinReservedPods` 阈值后更新 TaskResource 为 Reserved。
4. Task Controller 监听到满足 `MinReservedMembers` 阈值后，将 TaskResource 更新为 Schedulable。
5. Kuscia Scheduler 绑定 Pod 到已分配节点。

### 1.4.1 TaskResourceGroup 和 TaskResource 数据结构

#### TaskResourceGroup CR 示例

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: TaskResourceGroup
metadata:
  name: trg-psi-task
  namespace: cross-domain
spec:
  jobID: psi-demo-job
  taskID: psi-task
  minReservedMembers: 2  # 至少需要 2 个参与方完成资源预留
  resources:
    - domainID: alice
      role: server
      minReservedPods: 1
      template:
        replicas: 1
        spec:
          containers:
            - name: psi-container
              image: secretflow-image:latest
              resources:
                requests:
                  cpu: "8"
                  memory: "16Gi"
                limits:
                  cpu: "8"
                  memory: "16Gi"
    - domainID: bob
      role: client
      minReservedPods: 1
      template:
        replicas: 1
        spec:
          containers:
            - name: psi-container
              image: secretflow-image:latest
              resources:
                requests:
                  cpu: "8"
                  memory: "16Gi"
                limits:
                  cpu: "8"
                  memory: "16Gi"
status:
  phase: Reserved  # Pending -> Reserved -> Schedulable -> Failed
  resourceStatus:
    alice:
      - phase: Reserved
        hostTaskResourceName: tr-alice-xxx
        memberTaskResourceName: tr-bob-yyy
    bob:
      - phase: Reserved
        hostTaskResourceName: tr-bob-yyy
        memberTaskResourceName: tr-alice-xxx
```

#### TaskResource CR 示例

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: TaskResource
metadata:
  name: tr-alice-xxx
  namespace: cross-domain
  labels:
    kuscia.secretflow/task-resource-group: trg-psi-task
    kuscia.secretflow/domain-id: alice
spec:
  taskID: psi-task
  jobID: psi-demo-job
  domainID: alice
  role: server
  minReservedPods: 1
  template:
    replicas: 1
    spec:
      containers:
        - name: psi-container
          image: secretflow-image:latest
          resources:
            requests:
              cpu: "8"
              memory: "16Gi"
status:
  phase: Reserved  # Pending -> Reserved -> Schedulable -> Scheduled -> Running -> Succeeded/Failed
  nodeName: node-alice-01
  podName: psi-task-alice-0
```

**对应的函数接口：**

- `handleAddedTaskResourceGroup()` - 处理新增的TaskResourceGroup
- `handleUpdatedTaskResourceGroup()` - 处理更新的TaskResourceGroup
- `handleAddedOrDeletedTaskResource()` - 处理新增或删除的TaskResource
- `handleUpdatedTaskResource()` - 处理更新的TaskResource
- `matchLabels()` - 匹配标签筛选资源
- `resourceFilter()` - 资源过滤函数
- `PreFilter()` - 预过滤插件函数
- `PostFilter()` - 后过滤插件函数
- `Permit()` - 许可插件函数
- `Reserve()` - 预留插件函数
- `PreBind()` - 预绑定插件函数
- `PostBind()` - 后绑定插件函数

# 2 按 DAG 创建 KusciaTask CR

KusciaJob 是用户视角的“工作流”，`spec.tasks` 与其 `dependencies` 共同构成一张**有向无环图（DAG）**；而 KusciaTask 则是真正被 KusciaTask Controller 调度的“算子实例”。
KusciaJob Controller 的核心职责之一，就是在 Job 进入 `Running` 阶段后，持续解析这张 DAG，按依赖顺序、并发度和调度策略，将满足条件的 `KusciaTaskTemplate` 实例化为 `KusciaTask` CR。

## 2.1 数据模型与关键字段

在理解算法前，先明确三类对象的关系：

| 对象 | 作用 | 关键字段 |
| ------ | ------ | ---------- |
| **KusciaJob** | 工作流定义 | `spec.tasks[]`、`spec.scheduleMode`、`spec.maxParallelism`、`status.taskStatus` |
| **KusciaTaskTemplate** | Job 内的子任务模板 | `alias`、`taskID`、`dependencies`、`tolerable`、`priority`、`appImage`、`parties`、`taskInputConfig` |
| **KusciaTask** | 实际被调度的任务 CR | `metadata.name = taskID`、OwnerReference 指向 KusciaJob、`spec.parties[]` |

其中：

- `alias`：任务的**展示名称**，在 Job 内必须唯一，用于日志、状态展示。
- `taskID`：任务的**调度标识**，KusciaTask CR 的 `metadata.name` 直接使用该值；若用户未填写，由 Controller 在首次进入 Running 阶段时生成（`jobName-<uuid后缀>`）。
- `dependencies`：元素为前置任务的 `alias`。**只有当前置任务全部 `Succeeded` 后，本任务才“就绪”**。
- `tolerable`：是否可容忍失败。`false`（默认）表示关键任务；`true` 表示失败不影响 Job 最终成败。
- `priority`：就绪任务之间的优先级，值越大越先被创建。
- `scheduleMode`：`Strict` 或 `BestEffort`，决定关键任务失败时是否立即停止调度。
- `maxParallelism`：同一时刻最多有多少个任务处于 `Pending/Running` 状态，默认 `1`。

## 2.2 整体控制流程

当 KusciaJob 进入 `Running` 阶段后，`RunningHandler.handleRunning()` 会按如下循环执行调度：

```text
1. 列出该 Job 已创建的所有 KusciaTask（通过 label selector: kuscia.secretflow/controller=kusciajob, kuscia.secretflow/job-uid=<uid>）
2. 根据 KusciaTask 当前 Phase，重建 Job 的 taskStatus 视图
3. 计算当前 Job 应处于的 Phase（Running/Succeeded/Failed）
4. 计算“就绪任务” readyTasks：依赖已全部 Succeeded 且尚未创建的任务
5. 按 priority 排序，并结合 maxParallelism，得到本轮将要创建的 willStartTasks
6. 将 willStartTasks 转换为 KusciaTask CR 并调用 K8s API 创建
7. 更新 Job 的 status.taskStatus 与 status.phase
```

KusciaTask 的状态变化会触发 `handleTaskObject()`，进而将对应 Job 重新加入队列，形成事件驱动的闭环。

## 2.3 核心算法详解

### 2.3.1 Job 提交时的 DAG 校验

在 `Initialized` 阶段，`kusciaJobValidate()` 会对 DAG 做两项基础校验：

- **依赖存在性检查**（`kusciaJobDependenciesExits`）：每个 `dependencies` 中的 `alias` 必须在 `spec.tasks[]` 中存在，否则 Job 进入 `Failed`。
- **环检测**（`kusciaJobHasTaskCycle`）：采用**拓扑剥离法**。循环执行：
    1. 移除所有入度为 0 的任务；
    2. 在剩余任务中删除已被移除任务的依赖；
    3. 若某轮无法移除任何任务，且仍有任务剩余，则说明存在环。

```text
示例：
  tasks: [a->b, b->c, c->a]  # 环
  第 1 轮：无入度为 0 的任务 → 发现环，校验失败

  tasks: [a, a->b, a->c, c->d]
  第 1 轮：移除 a
  第 2 轮：移除 b, c
  第 3 轮：移除 d
  全部移除 → 无环
```

### 2.3.2 TaskID 生成

若用户未显式填写 `taskID`，Controller 会在首次进入 Running 阶段时由 `setJobTaskID()` 统一生成：

```go
taskID = jobName + "-" + uuid.LastSegment()
```

生成后会立即回写 `KusciaJob.spec.tasks[].taskID`，确保后续所有参与方看到的任务标识一致。

### 2.3.3 就绪任务计算

`readyTasksOf(job, currentTasks)` 的算法如下：

```text
输入：KusciaJob、当前各 alias 的 Phase（currentTasks）
输出：可以立即创建的任务模板列表

1. 深拷贝 Job，遍历每个 task
2. 从 task.dependencies 中过滤掉已经 Succeeded 的依赖
3. 保留过滤后 dependencies 为空且 taskID 非空的任务
4. 再过滤掉 currentTasks 中已存在的任务（已创建）
5. 按 priority 降序排序
6. 返回原始 task 模板的副本
```

关键点：只有依赖**全部 Succeeded** 才就绪；**Failed 的前置任务不会使后置任务就绪**。这意味着失败会自然阻断下游任务的创建，除非失败任务是 `tolerable`（但仍需其状态为 `Succeeded` 才能解除阻塞，因此 tolerable 任务失败后其下游不会继续执行）。

### 2.3.4 最大并发度裁剪

`willStartTasksOf(job, readyTasks, status)` 负责根据 `maxParallelism` 决定本轮实际创建多少个任务：

```text
runningCount = status 中 Phase 为 Pending/Running/空 的任务数量
可创建数     = maxParallelism - runningCount
willStart    = readyTasks 的前 min(可创建数, len(readyTasks)) 个
```

例如 `maxParallelism=2`：

- 初始时 runningCount=0，可创建 2 个，但只有 1 个就绪（根任务 a），则只创建 a。
- a Succeeded 后，b、c 同时就绪，runningCount=0，可同时创建 b、c。
- b 进入 Running、c 进入 Running 后，d 即使就绪也必须等待 b 或 c 完成。

### 2.3.5 Job 状态计算

`jobStatusPhaseFrom(job, currentSubTasksStatus)` 综合所有任务状态，得出 Job 当前 Phase：

| 条件 | Job Phase |
| ------ | ----------- |
| 所有任务 Finished，且所有关键任务 Succeeded | `Succeeded` |
| `Strict` 模式下，任一关键任务 Failed | `Failed` |
| `BestEffort` 模式下，无就绪/运行中任务，且任一关键任务 Failed | `Failed` |
| 互联互通（InterConn）任务任一任务 Failed | `Failed` |
| 其他情况 | `Running` |

其中 `Finished = Succeeded || Failed`，关键任务 = `tolerable != true`。

### 2.3.6 KusciaTask CR 构建

`buildWillStartKusciaTask()` 将每个就绪任务模板转换为 KusciaTask CR：

```go
KusciaTask{
    ObjectMeta: {
        Name: task.TaskID,
        OwnerReferences: [ControllerRef(KusciaJob)],
        Annotations: {
            "kuscia.secretflow/job-id":              job.Name,
            "kuscia.secretflow/task-alias":          task.Alias,
            "kuscia.secretflow/self-cluster-as-participant": "true/false",
        },
        Labels: {
            "kuscia.secretflow/controller": "kusciajob",
            "kuscia.secretflow/job-uid":    string(job.UID),
        },
    },
    Spec: {
        Initiator:       job.Spec.Initiator,
        TaskInputConfig: task.TaskInputConfig,
        Parties:         buildPartiesFromTaskInputConfig(task),
        ScheduleConfig:  task.ScheduleConfig,
    },
}
```

对于互联互通（BFIA/Kuscia-InterOp）Job，还会额外携带协议相关 annotation/label（如 `LabelInterConnProtocolType`、`LabelTaskUnschedulable` 等）。

`buildPartiesFromTaskInputConfig()` 根据 `task.appImage` 和参与方 `role` 去匹配 `AppImage` 中对应的 `DeployTemplate`，再结合 `parties[].resources` 计算每个容器最终的 `limits/requests`，最终填充到 `spec.parties[].template`。

## 2.4 调度模式对 DAG 执行的影响

KusciaJob 支持两种调度模式，差异体现在**关键任务失败后的行为**：

- **Strict（严格模式）**：任一关键任务失败后，Job 立即置为 `Failed`，不再创建任何后续 KusciaTask。
- **BestEffort（尽力模式）**：关键任务失败后，仅阻塞其下游任务；不依赖失败任务的其他分支继续调度，待所有可达任务都执行完毕后，Job 最终置为 `Failed`。

`Tolerable` 任务失败不会直接导致 Job 失败，但其下游任务仍会被阻塞（因为依赖未满足）。

## 2.5 示例

### 2.5.1 示例 1：线性 DAG（PSI → 数据分割）

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-then-split
  namespace: cross-domain
spec:
  initiator: alice
  scheduleMode: Strict
  maxParallelism: 1
  tasks:
    - alias: psi
      taskID: psi
      appImage: secretflow-image
      taskInputConfig: '{...psi config...}'
      parties:
        - domainID: alice
        - domainID: bob
    - alias: split
      taskID: split
      dependencies: ['psi']
      appImage: secretflow-image
      taskInputConfig: '{...split config...}'
      parties:
        - domainID: alice
        - domainID: bob
```

执行过程：

| 轮次 | 就绪任务 | 当前 Running | 创建动作 |
|------|----------|--------------|----------|
| 1    | psi      | 0            | 创建 KusciaTask `psi` |
| 2    | split    | 0（psi 已 Succeeded） | 创建 KusciaTask `split` |
| 3    | 无       | 0            | Job 转为 Succeeded/Failed |

### 2.5.2 示例 2：树形 DAG（并行分支）

```yaml
spec:
  scheduleMode: BestEffort
  maxParallelism: 2
  tasks:
    - alias: a
      taskID: a
    - alias: b
      taskID: b
      dependencies: ['a']
    - alias: c
      taskID: c
      dependencies: ['a']
    - alias: d
      taskID: d
      dependencies: ['c']
```

执行过程：

| 阶段 | 任务状态 | 就绪任务 | 创建动作 |
| ------ | ---------- | ---------- | ---------- |
| 初始 | 全部未创建 | a | 创建 a |
| a Succeeded | a=Succeeded | b, c | 同时创建 b、c（maxParallelism=2） |
| b Succeeded, c Running | a,b=Succeeded; c=Running | d（依赖 c 未满足） | 无 |
| c Succeeded | a,b,c=Succeeded | d | 创建 d |

### 2.5.3 示例 3：Strict vs BestEffort 在失败场景的差异

沿用示例 2 的 DAG，假设 `b` 是关键任务且执行失败：

- **Strict 模式**：
  - b Failed → 立即将 Job 置为 `Failed`。
  - 即使 c 正在运行，d 也不会再被创建（`currentJobPhase == Failed` 时停止创建新 Task）。

- **BestEffort 模式**：
  - b Failed 仅阻塞 b 的下游（本例中 b 无下游）。
  - c 继续运行；c Succeeded 后 d 仍可被创建。
  - 待 a、c、d 均 Finished 后，Job 因 b 失败而最终变为 `Failed`。

### 2.5.4 示例 4：创建出的 KusciaTask CR

对于示例 1 中的 `psi` 任务，Controller 会生成如下 KusciaTask：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaTask
metadata:
  name: psi
  namespace: cross-domain
  ownerReferences:
    - apiVersion: kuscia.secretflow/v1alpha1
      kind: KusciaJob
      name: psi-then-split
      uid: <job-uid>
      controller: true
  labels:
    kuscia.secretflow/controller: kusciajob
    kuscia.secretflow/job-uid: <job-uid>
  annotations:
    kuscia.secretflow/job-id: psi-then-split
    kuscia.secretflow/task-alias: psi
    kuscia.secretflow/self-cluster-as-participant: "true"
spec:
  initiator: alice
  taskInputConfig: '{...psi config...}'
  parties:
    - domainID: alice
      appImageRef: secretflow-image
    - domainID: bob
      appImageRef: secretflow-image
```

### 2.5.5 示例 5：P2P 模式下多方 PSI 的完整调度与数据流

前面四个示例侧重于 **KusciaJob 内部的 DAG 调度规则**。本示例换一个视角，展示一次真实的 **端到端隐私集合求交（PSI）任务** 在 P2P 组网（Alice、Bob 均为 Autonomy 节点）中，从提交到执行完毕的完整数据流与资源协调过程。

## 场景设定

- Alice 和 Bob 各自部署了一个 **Autonomy** 节点，均包含独立的 K3s 控制平面。
- Alice 是本次任务的 **Initiator**，她拥有两张表：`alice.id` 和 `alice.label`。
- Bob 是参与方，拥有表 `bob.id`。
- 任务目标：先由 Alice 本地做特征拼接，再与 Bob 做 PSI 求交，最后由 Alice 输出结果。

对应的 KusciaJob 如下：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: psi-demo
  namespace: cross-domain
spec:
  initiator: alice
  scheduleMode: Strict
  maxParallelism: 2
  tasks:
    - alias: preprocess
      taskID: preprocess
      appImage: secretflow-image
      taskInputConfig: '{...本地特征拼接配置...}'
      parties:
        - domainID: alice
    - alias: psi
      taskID: psi
      dependencies: ['preprocess']
      appImage: secretflow-image
      taskInputConfig: '{...两方 PSI 配置...}'
      parties:
        - domainID: alice
        - domainID: bob
    - alias: postprocess
      taskID: postprocess
      dependencies: ['psi']
      appImage: secretflow-image
      taskInputConfig: '{...结果落盘配置...}'
      parties:
        - domainID: alice
```

## 完整执行流程

```text
Alice (Autonomy)                          Bob (Autonomy)
     │                                         │
     │ 1. 创建 KusciaJob psi-demo              │
     ▼                                         │
┌─────────────┐                                │
│KusciaJob    │                                │
│Controller   │                                │
└──────┬──────┘                                │
       │ 2. DAG 校验通过，按依赖创建 KusciaTask │
       ▼                                         │
┌─────────────┐                                │
│KusciaTask   │ 3. 针对 psi 任务创建            │
│Controller   │    TaskResourceGroup trg-psi   │
└──────┬──────┘                                 │
       │ 4. InterConn Controller 同步           │
       │    TaskResource/PodGroup 到 Bob        │
       └──────────────►┌─────────────────┐      │
                       │ TaskResource    │      │
                       │ PodGroup        │      │
                       └────────┬────────┘      │
                                │ 5. 双方 Scheduler
                                │    预留并绑定 Pod
                                ▼                ▼
                       ┌─────────────────┐  ┌─────────────────┐
                       │ Alice Agent     │  │ Bob Agent       │
                       │ 启动 PSI 容器   │  │ 启动 PSI 容器   │
                       └────────┬────────┘  └────────┬────────┘
                                │ 6. 通过 DataMesh   │
                                │    读取/写入数据   │
                                ▼                  ▼
                       ┌─────────────────────────────────────┐
                       │ 引擎完成 PSI，任务状态变为 Succeeded │
                       └─────────────────────────────────────┘
                                │ 7. KusciaTask 成功触发
                                │    postprocess 就绪
                                ▼
                       ┌─────────────────┐
                       │ Alice 本地执行  │
                       │ postprocess     │
                       └─────────────────┘
                                │
                                ▼
                       ┌─────────────────┐
                       │ Job 转为        │
                       │ Succeeded       │
                       └─────────────────┘
```

按阶段拆解：

| 阶段 | 发生位置 | 关键动作 | 产生的 CR / 事件 |
| ------ | ---------- | ---------- | ------------------ |
| 1 | Alice KusciaAPI | 校验 DAG、写入 `KusciaJob` | `KusciaJob/psi-demo` |
| 2 | Alice KusciaJob Controller | `preprocess` 无依赖，立即创建 `KusciaTask` | `KusciaTask/preprocess` |
| 3 | Alice KusciaTask Controller | 为 `preprocess` 创建 Pod；为 `psi` 创建 `TaskResourceGroup` | `TaskResourceGroup/trg-psi` |
| 4 | Alice/Bob InterConn Controller | 将本方 `TaskResource`/`PodGroup` 同步到对端 | 对端 Namespace 下出现同名资源 |
| 5 | Alice/Bob Scheduler | 双方 `TaskResource` 均 Reserved 后，Pod 进入 Schedulable；Scheduler 绑定到节点 | Pod 状态 `Scheduled` → `Running` |
| 6 | Alice/Bob Agent | 拉取镜像、启动容器、挂载 ConfigMap/Secret | Pod 状态 `Running` |
| 7 | Alice/Bob 引擎 | 通过本地 DataMesh 读取 `alice.id`、`bob.id`，写入交集结果 | DataMesh 产生访问日志 |
| 8 | Alice KusciaJob Controller | `psi` Succeeded 后，`postprocess` 就绪并被创建；全部 Succeeded 后 Job 成功 | `KusciaJob` Phase = `Succeeded` |

## 数据流要点

- **本地任务（preprocess）**：只在 Alice 侧创建 Pod，不触发跨域资源协调。
- **多方任务（psi）**：必须等待 Alice、Bob 双方都完成资源预留后才会真正调度 Pod，体现 All-or-Nothing 调度语义。
- **结果任务（postprocess）**：依赖 `psi` 成功，且只在 Alice 侧运行，因此失败场景下如果 `psi` 失败则不会执行。
- **DataMesh 访问**：引擎通过 `dm` 域名（如 `datamesh.alice.svc`）访问本地 DataMesh，DataMesh 根据 `domaindata` 定义决定实际数据源是 localfs、OSS 还是数据库。

## 2.6 相关单元测试介绍

Kuscia 针对任务调度与数据流的核心路径编写了大量单元测试，主要使用 **Kubernetes fake clientset** 与 **Kuscia fake clientset** 构造虚拟集群状态，无需真实 K3s 即可验证 Controller 与 Scheduler 的行为。

## 主要 UT 文件与测试重点

| UT 文件 | 测试对象 | 核心覆盖点 |
| --------- | ---------- | ------------ |
| `pkg/controllers/kusciajob/handler/scheduler_test.go` | `JobScheduler` | DAG 合法性校验、`kusciaJobHasTaskCycle` 环检测、就绪任务计算、并发度裁剪、Job 状态推导 |
| `pkg/controllers/kusciajob/handler/running_test.go` | `RunningHandler` | Running 阶段状态机、按 DAG 创建 KusciaTask、`Strict`/`BestEffort` 失败行为、任务重入队列 |
| `pkg/controllers/kusciajob/handler/initialized_test.go` | `InitializedHandler` | 首次处理 Job 时的默认字段填充、TaskID 生成准备 |
| `pkg/controllers/kusciatask/handler/pending_handler_test.go` | `PendingHandler` | KusciaTask 创建 TaskResourceGroup/Pod/Service/ConfigMap 的转换逻辑 |
| `pkg/controllers/taskresourcegroup/handler/reserving_handler_test.go` | `ReservingHandler` | 多方资源预留协调、`MinReservedMembers` 阈值判定、失败回滚 |
| `pkg/scheduler/kusciascheduling/kusciascheduling_test.go` | `KusciaScheduling` 插件 | `PreFilter`/`Reserve`/`Permit`/`PreBind`/`PostBind` 全链路调度插件行为 |
| `pkg/scheduler/kusciascheduling/core/core_test.go` | TaskResource 管理器 | TaskResource 生命周期、预留超时、状态同步 |

## 典型测试模式

以 `pkg/controllers/kusciajob/handler/scheduler_test.go` 中的 `Test_kusciaJobValidate` 为例：

```go
func Test_kusciaJobValidate(t *testing.T) {
    // ...
    tests := []struct {
        name    string
        args    args
        wantErr assert.ErrorAssertionFunc
    }{
        {
            name: "BestEffort mode task{a,b,c,d} should return want{false}",
            args: args{
                kusciaJob: makeKusciaJob(KusciaJobForShapeIndependent,
                    kusciaapisv1alpha1.KusciaJobScheduleModeBestEffort, 2, nil),
            },
            wantErr: assert.NoError,
        },
        {
            name: "BestEffort mode task{a,b,c,d} cycled should return true",
            args: args{
                kusciaJob: makeKusciaJob(KusciaJobForShapeCycled,
                    kusciaapisv1alpha1.KusciaJobScheduleModeBestEffort, 2, nil),
            },
            wantErr: assert.Error,
        },
    }
    // ...
}
```

测试用例通过 `makeKusciaJob` 构造不同 DAG 形状（独立、树形、环形）的 Job，然后调用 `kusciaJobValidate` 验证是否返回预期错误。环形 DAG 必须被检测出来并返回错误。

再以 `pkg/controllers/kusciajob/handler/running_test.go` 为例，它使用 `kusciafake.NewSimpleClientset()` 预先注入若干已存在的 `KusciaTask`（模拟上一轮已创建的任务），然后调用 `RunningHandler.HandlePhase`，断言：

- 是否需要更新 Job 状态（`wantNeedUpdate`）。
- Job 最终 Phase（`wantJobPhase`）。
- 哪些 KusciaTask 应该被创建或处于 Succeeded（`wantFinalTasks`）。

## 运行单元测试

在 Kuscia 仓库根目录执行：

```bash
# 运行第 6 章涉及的全部单元测试
go test ./pkg/controllers/kusciajob/... \
         ./pkg/controllers/kusciatask/... \
         ./pkg/controllers/taskresourcegroup/... \
         ./pkg/scheduler/kusciascheduling/...

# 仅运行 DAG 调度相关测试
go test ./pkg/controllers/kusciajob/handler -run Test_kusciaJobValidate -v

# 仅运行 Running 阶段状态机测试
go test ./pkg/controllers/kusciajob/handler -run TestRunningHandler_HandlePhase -v

# 仅运行资源预留协调测试
go test ./pkg/controllers/taskresourcegroup/handler -run TestReservingHandlerHandle -v

# 仅运行 Kuscia Scheduler 插件测试
go test ./pkg/scheduler/kusciascheduling -run TestPermit -v
```

> 提示：首次运行前请确保已执行 `go mod download` 下载依赖；部分测试依赖 `pkg/crd/clientset/versioned/fake` 生成的 fake clientset，若该文件缺失请先执行 CRD 代码生成（详见第 12 章）。

## 2.7 关键代码路径

| 函数/文件 | 作用 |
| ----------- | ------ |
| `pkg/controllers/kusciajob/handler/scheduler.go` | DAG 调度核心：校验、就绪任务计算、状态机、Task CR 构建 |
| `pkg/controllers/kusciajob/handler/running.go` | `RunningHandler.handleRunning()`：Running 阶段的入口，串联调度与创建 |
| `pkg/controllers/kusciajob/handler/initialized.go` | 首次处理：DAG 校验、TaskID 生成准备、状态初始化 |
| `pkg/controllers/kusciajob/handler/factory.go` | 状态机工厂，按 Job Phase 分发 Handler |
| `pkg/controllers/kusciajob/controller.go` | Controller 主循环、Informer 监听、事件入队 |
| `pkg/crd/apis/kuscia/v1alpha1/kusciajob_types.go` | KusciaJob / KusciaTaskTemplate 类型定义 |
| `pkg/crd/apis/kuscia/v1alpha1/kusciatask_types.go` | KusciaTask 类型定义 |

**对应函数接口：**

- `NewController()` - 创建 KusciaJob 控制器实例
- `Run()` - 启动控制器运行
- `syncHandler()` - 同步处理 KusciaJob 状态
- `enqueueKusciaJob()` - 将 KusciaJob 加入工作队列
- `handleTaskObject()` - 监听 KusciaTask 变化，反查所属 Job 并入队
- `kusciaJobDefault()` - 设置 KusciaJob 默认字段（初始 phase、maxParallelism 等）
- `failKusciaJob()` - 处理 KusciaJob 失败状态
- `handlerFactory.KusciaJobPhaseHandlerFor(phase).HandlePhase()` - 根据阶段处理 KusciaJob
- `kusciaJobValidate()` - DAG 合法性校验
- `kusciaJobHasTaskCycle()` - DAG 环检测
- `readyTasksOf()` - 计算就绪任务
- `willStartTasksOf()` - 根据并发度裁剪待创建任务
- `jobStatusPhaseFrom()` - 由任务状态推导 Job 状态
- `buildWillStartKusciaTask()` - 将任务模板构建为 KusciaTask CR

# 3 创建 TaskResourceGroup / Pod / Service / ConfigMap CR

## 3.1 KusciaTask 转换为 Kubernetes 资源的详细流程

KusciaTask Controller 负责将 KusciaTask 转换为实际的 K8s 资源，具体流程如下：

### 3.1.1 TaskResourceGroup 创建

为多方协同任务创建 TaskResourceGroup CR，用于跨域资源协调：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: TaskResourceGroup
metadata:
  name: trg-{taskID}
  namespace: {namespace}
spec:
  jobID: {jobName}
  taskID: {taskID}
  minReservedMembers: {参与方数量}
  resources:
    - domainID: alice
      role: server
      minReservedPods: 1
      template:
        replicas: 1
        spec:
          containers:
            - name: {containerName}
              image: {imageFromAppImage}
              command: {commandFromAppImage}
              args: {argsFromAppImage}
              resources:
                requests:
                  cpu: "{cpu}"
                  memory: "{memory}"
                limits:
                  cpu: "{cpu}"
                  memory: "{memory}"
```

### 3.1.2 Pod 创建

根据 Task 规约创建对应的 Pod CR，包含完整的容器配置：

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: {taskID}-{domainID}-{index}
  namespace: {namespace}
  labels:
    kuscia.secretflow/task-id: {taskID}
    kuscia.secretflow/job-id: {jobName}
    kuscia.secretflow/domain-id: {domainID}
  ownerReferences:
    - apiVersion: kuscia.secretflow/v1alpha1
      kind: KusciaTask
      name: {taskID}
      uid: {taskUID}
spec:
  restartPolicy: Never
  containers:
    - name: {containerName}
      image: {imageFromAppImage}
      command: {commandFromAppImage}
      args: {argsFromAppImage}
      workingDir: {workingDir}
      env:
        - name: KUSCIA_TASK_ID
          value: {taskID}
        - name: KUSCIA_JOB_ID
          value: {jobName}
        - name: KUSCIA_DOMAIN_ID
          value: {domainID}
        - name: KUSCIA_ROLE
          value: {role}
        - name: SF_CLUSTER_CONFIG
          value: |
            {
              "party": "{domainID}",
              "self_party": "{domainID}",
              "peers": [
                {
                  "party": "{peerDomainID}",
                  "address": "{peerServiceAddress}:{port}"
                }
              ]
            }
      envFrom:
        - configMapRef:
            name: {configMapName}
      volumeMounts:
        - name: config-volume
          mountPath: /etc/kuscia/config
          subPath: config.yaml
        - name: data-volume
          mountPath: /data
      ports:
        - name: grpc
          containerPort: 8080
          protocol: TCP
      resources:
        requests:
          cpu: "{cpu}"
          memory: "{memory}"
        limits:
          cpu: "{cpu}"
          memory: "{memory}"
      livenessProbe:
        httpGet:
          path: /healthz
          port: 8080
        initialDelaySeconds: 10
        periodSeconds: 5
      readinessProbe:
        httpGet:
          path: /ready
          port: 8080
        initialDelaySeconds: 5
        periodSeconds: 3
  volumes:
    - name: config-volume
      configMap:
        name: {configMapName}
    - name: data-volume
      persistentVolumeClaim:
        claimName: {pvcName}
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: kuscia.secretflow/domain-id
                operator: In
                values:
                  - {domainID}
```

**关键环境变量说明：**

| 环境变量 | 说明 |
| --------- | ------ |
| `KUSCIA_TASK_ID` | 当前任务 ID |
| `KUSCIA_JOB_ID` | 所属作业 ID |
| `KUSCIA_DOMAIN_ID` | 当前参与方域名 ID |
| `KUSCIA_ROLE` | 当前参与方角色（server/client、guest/host 等） |
| `SF_CLUSTER_CONFIG` | **SecretFlow 集群配置（JSON 格式）**，详见下文 |

### 3.1.3 SecretFlow 集群配置（SF_CLUSTER_CONFIG）格式

`SF_CLUSTER_CONFIG` 是 SecretFlow 引擎启动时必需的集群配置，定义了多方通信的网络拓扑：

```json
{
  "party": "alice",
  "self_party": "alice",
  "peers": [
    {
      "party": "bob",
      "address": "psi-task-bob-svc.cross-domain.svc.cluster.local:8080"
    }
  ],
  "options": {
    "recv_proxy_max_msg_size": 2147483647,
    "send_proxy_max_msg_size": 2147483647
  }
}
```

**字段说明：**

- `party`: 当前参与方的域名 ID
- `self_party`: 同上，表示自身
- `peers`: 对端参与方列表
  - `party`: 对端域名 ID
  - `address`: 对端服务的完整地址（Kubernetes Service DNS 名称 + 端口）
- `options`: 可选配置
  - `recv_proxy_max_msg_size`: 接收消息最大大小（字节）
  - `send_proxy_max_msg_size`: 发送消息最大大小（字节）

对于三方或多方任务，`peers` 数组会包含所有其他参与方：

```json
{
  "party": "alice",
  "self_party": "alice",
  "peers": [
    {
      "party": "bob",
      "address": "psi-task-bob-svc.cross-domain.svc.cluster.local:8080"
    },
    {
      "party": "charlie",
      "address": "psi-task-charlie-svc.cross-domain.svc.cluster.local:8080"
    }
  ]
}
```

> **实现说明**：上文将 `SF_CLUSTER_CONFIG` 描述为 SecretFlow 引擎可见的集群配置概念。在 Kuscia 当前实现中，Controller 并不会直接把该 JSON 作为环境变量注入 Pod，而是将等价的多方拓扑序列化为 protobuf `ClusterDefine`，写入 kuscia-gen ConfigMap 的 `TASK_CLUSTER_DEFINE` 键；Agent 的 `config-render` 插件再将其渲染到 `task-config.conf` 中，由 `secretflow.kuscia.entry` 解析并构建 SecretFlow 内部集群配置。详见 7.4.4 节。

### 3.1.4 Service 创建

为需要网络通信的任务创建 Service CR，实现服务发现和负载均衡：

```yaml
apiVersion: v1
kind: Service
metadata:
  name: {taskID}-{domainID}-svc
  namespace: {namespace}
  labels:
    kuscia.secretflow/task-id: {taskID}
    kuscia.secretflow/domain-id: {domainID}
spec:
  type: ClusterIP
  selector:
    kuscia.secretflow/task-id: {taskID}
    kuscia.secretflow/domain-id: {domainID}
  ports:
    - name: grpc
      port: 8080
      targetPort: 8080
      protocol: TCP
```

Service 的 DNS 名称格式为：`{service-name}.{namespace}.svc.cluster.local`，例如：`psi-task-alice-svc.cross-domain.svc.cluster.local`

### 3.1.5 ConfigMap 创建

为任务创建配置文件和参数传递的 ConfigMap CR：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {taskID}-{domainID}-config
  namespace: {namespace}
data:
  config.yaml: |
    task_id: {taskID}
    job_id: {jobName}
    domain_id: {domainID}
    role: {role}
    
  task_input_config.json: |
    {taskInputConfigJSON}
    
  app_config.yaml: |
    # 从 AppImage 的 ConfigTemplates 中提取的配置
    log_level: INFO
    data_dir: /data
```

### 3.1.6 资源关联

建立 Task 与创建的各类资源之间的关联关系：

- **OwnerReference**: Pod、Service、ConfigMap 都设置 OwnerReference 指向 KusciaTask
- **Labels**: 所有资源都携带统一的 Labels（task-id、job-id、domain-id）
- **Annotations**: 记录额外元数据（task-alias、initiator 等）

便于状态跟踪、级联删除和资源清理。

## 3.2 AppImage 与 DeployTemplate

### 3.2.1 AppImage CR 结构

AppImage 是 Kuscia 中定义的应用镜像抽象，封装了容器镜像及其部署模板：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: AppImage
metadata:
  name: secretflow-image
spec:
  image:
    name: secretflow/psi
    tag: latest
    id: "sha256:f1c20d8cb5c4c69d3997527e4912e794ba3cd7fa26bfaf6afa1383697c80ea9a"
  configTemplates:
    default: |
      log_level: INFO
      data_dir: /data
      algorithm: ecdh_psi
  deployTemplates:
    - name: server
      role: server
      replicas: 1
      spec:
        restartPolicy: Never
        containers:
          - name: psi-container
            command:
              - python3
              - /root/main.py
            args:
              - --mode
              - server
            workingDir: /root
            ports:
              - name: grpc
                port: 8080
                protocol: GRPC
                scope: Cluster
            resources:
              requests:
                cpu: "4"
                memory: "8Gi"
              limits:
                cpu: "8"
                memory: "16Gi"
            env:
              - name: LOG_LEVEL
                value: INFO
            livenessProbe:
              httpGet:
                path: /healthz
                port: 8080
              initialDelaySeconds: 10
              periodSeconds: 5
            readinessProbe:
              httpGet:
                path: /ready
                port: 8080
              initialDelaySeconds: 5
              periodSeconds: 3
    - name: client
      role: client
      replicas: 1
      spec:
        restartPolicy: Never
        containers:
          - name: psi-container
            command:
              - python3
              - /root/main.py
            args:
              - --mode
              - client
            workingDir: /root
            ports:
              - name: grpc
                port: 8080
                protocol: GRPC
                scope: Cluster
            resources:
              requests:
                cpu: "4"
                memory: "8Gi"
              limits:
                cpu: "8"
                memory: "16Gi"
```

**关键字段说明：**

| 字段 | 类型 | 说明 |
| ------ | ------ | ------ |
| `spec.image.name` | string | Docker 镜像名称 |
| `spec.image.tag` | string | 镜像标签 |
| `spec.image.id` | string | 镜像 SHA256 ID（可选，用于完整性校验） |
| `spec.configTemplates` | map | 配置模板，key 为模板名称，value 为配置内容 |
| `spec.deployTemplates` | array | 部署模板列表，每个模板对应一种角色 |
| `spec.deployTemplates[].name` | string | 模板名称 |
| `spec.deployTemplates[].role` | string | 角色名称（server/client、guest/host 等） |
| `spec.deployTemplates[].replicas` | int32 | 副本数 |
| `spec.deployTemplates[].spec.containers` | array | 容器列表 |
| `spec.deployTemplates[].spec.containers[].command` | array | 启动命令 |
| `spec.deployTemplates[].spec.containers[].args` | array | 启动参数 |
| `spec.deployTemplates[].spec.containers[].ports` | array | 端口列表 |
| `spec.deployTemplates[].spec.containers[].resources` | object | 资源请求和限制 |

### 3.2.2 DeployTemplate 到 PodSpec 的转换

当 KusciaTask Controller 创建 Pod 时，会根据任务的 `parties[].role` 匹配 AppImage 中对应的 DeployTemplate，然后将其转换为 Kubernetes PodSpec：

```go
// 伪代码示例
func buildPodSpec(task *KusciaTask, party *PartyInfo, appImage *AppImage) corev1.PodSpec {
    // 1. 根据 party.Role 查找匹配的 DeployTemplate
    deployTemplate := findDeployTemplate(appImage.Spec.DeployTemplates, party.Role)
    
    // 2. 提取容器配置
    container := deployTemplate.Spec.Containers[0]
    
    // 3. 合并资源配置（Party.Resources 覆盖 DeployTemplate 中的默认值）
    if party.Template.Spec.Containers[0].Resources != nil {
        container.Resources = mergeResources(container.Resources, party.Template.Spec.Containers[0].Resources)
    }
    
    // 4. 注入环境变量
    container.Env = append(container.Env, corev1.EnvVar{
        Name:  "SF_CLUSTER_CONFIG",
        Value: buildSFClusterConfig(task, party),
    })
    
    // 5. 构建完整 PodSpec
    return corev1.PodSpec{
        RestartPolicy: deployTemplate.Spec.RestartPolicy,
        Containers:    []corev1.Container{container},
        Affinity:      deployTemplate.Spec.Affinity,
        // ... 其他字段
    }
}
```

### 3.2.3 DomainAppImage 绑定

在跨域场景中，各参与方需要在其本地域中注册 DomainAppImage，将全局的 AppImage 绑定到本域：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainAppImage
metadata:
  name: secretflow-image
  namespace: alice  # 各域独立注册
spec:
  appImageRef: secretflow-image  # 引用全局 AppImage
  domainID: alice
```

这样，当 Task 在 alice 域执行时，会使用 alice 域中注册的 DomainAppImage，进而引用全局的 AppImage 获取 DeployTemplate。

**对应的函数接口：**

- `NewController()` - 创建KusciaTask控制器实例
- `Run()` - 启动控制器运行
- `syncHandler()` - 同步处理KusciaTask状态
- `enqueueKusciaTask()` - 将KusciaTask加入工作队列
- `handleTaskResourceGroupObject()` - 处理TaskResourceGroup对象
- `handlePodObject()` - 处理Pod对象
- `handleServiceObject()` - 处理Service对象
- `handleDeletedKusciaTask()` - 处理删除的KusciaTask
- `updateTaskStatus()` - 更新任务状态
- `failKusciaTask()` - 处理KusciaTask失败状态
- `handlerFactory.GetKusciaTaskPhaseHandler(phase).Handle()` - 根据阶段处理KusciaTask

# 4 资源预留协调

TaskResourceGroup Controller 负责协调多方任务的资源预留，确保所有参与方的资源同时可用：

1. **资源评估**：
    - 评估各方节点的可用资源（CPU、内存、GPU等）
    - 检查资源配额和限制
    - 验证节点标签和污点容忍度

2. **预留请求**：
    - 向各参与方发送资源预留请求
    - 暂时锁定资源以防其他任务占用
    - 记录预留状态和时间戳

3. **协调机制**：
    - 等待所有参与方确认资源预留
    - 如果任一参与方无法预留资源，则取消所有预留
    - 实现超时机制防止无限等待

4. **状态同步**：
    - 同步各方预留状态到 TaskResourceGroup CR
    - 更新协调进度和异常信息
    - 提供调试和监控信息

5. **回滚机制**：
    - 当资源预留失败时，自动释放已预留的资源
    - 更新任务状态为失败并记录原因
    - 支持重试机制

**对应的函数接口：**

- `NewController()` - 创建TaskResourceGroup控制器实例
- `Run()` - 启动控制器运行
- `syncHandler()` - 同步处理TaskResourceGroup状态
- `handleAddedTaskResourceGroup()` - 处理新增的TaskResourceGroup
- `handleUpdatedTaskResourceGroup()` - 处理更新的TaskResourceGroup
- `handleAddedOrDeletedTaskResource()` - 处理新增或删除的TaskResource
- `handleUpdatedTaskResource()` - 处理更新的TaskResource
- `handleAddedPod()` - 处理新增的Pod
- `resourceFilter()` - 资源过滤函数
- `matchLabels()` - 匹配标签筛选资源
- `updateTaskResourceGroupStatus()` - 更新TaskResourceGroup状态
- `needHandleExpiredTrg()` - 检查TaskResourceGroup是否过期
- `needHandleReserveFailedTrg()` - 检查预留失败的TaskResourceGroup

# 5 绑定 Pod 到 Node

## 5.1 Kuscia Scheduler 扩展调度策略

Kuscia Scheduler 是一个定制的调度器，负责将 Pod 绑定到合适的节点，特别针对多方协同任务进行了优化：

### 5.1.1 All-or-Nothing 调度策略

实现 All-or-Nothing 调度策略，确保多方协同任务要么全部调度成功，要么全部失败：

1. **资源预留阶段（Reserve）**：
   - 当 Pod 被调度到节点时，Scheduler 会暂时锁定该节点的资源
   - 如果多方任务中任一参与方无法预留资源，则取消所有已预留的资源
   - 通过 `Permit` 插件等待所有参与方都完成资源预留

2. **等待机制（Permit）**：
   - 对于多方任务，第一个到达的 Pod 会被设置为 "Waiting" 状态
   - 后续 Pod 到达后，检查是否满足 `MinReservedMembers` 阈值
   - 满足条件后，同时批准所有 Waiting 的 Pod 进入绑定阶段

3. **超时处理**：
   - 设置资源预留超时时间（默认 300 秒）
   - 如果超时后仍有参与方未完成预留，则取消所有预留并标记任务为 Failed

### 5.1.2 调度插件执行流程

Kuscia Scheduler 实现了以下调度插件：

```
PreFilter → Filter → PostFilter → Reserve → Permit → PreBind → Bind → PostBind
```

| 插件 | 作用 |
| ------ | ------ |
| `PreFilter` | 预过滤：检查 TaskResource 是否存在且状态合法 |
| `Filter` | 过滤：验证节点资源、标签、污点等 |
| `PostFilter` | 后过滤：如果无可用节点，尝试抢占或报告失败 |
| `Reserve` | 预留：锁定节点资源，更新 TaskResource 状态 |
| `Permit` | 许可：等待多方任务的所有参与方都完成预留 |
| `Unreserve` | 取消预留：如果绑定失败，释放已预留的资源 |
| `PreBind` | 预绑定：执行额外的绑定前操作 |
| `Bind` | 绑定：创建 Binding 对象，将 Pod 绑定到节点 |
| `PostBind` | 后绑定：更新 TaskResource 状态为 Scheduled |

### 5.1.3 跨集群调度协调

在 P2P 模式下协调多个集群的调度决策：

1. **TaskResource 同步**：
   - InterConn Controller 将本方的 TaskResource 同步到对端集群
   - 对端集群的 Scheduler 可以看到双方的资源需求

2. **独立调度决策**：
   - 各集群的 Scheduler 独立进行调度决策
   - 通过 TaskResource 的状态同步实现协调

3. **资源冲突处理**：
   - 如果某节点资源不足，Scheduler 会尝试其他节点
   - 如果所有节点都无法满足，任务进入 Pending 状态并等待重试

**对应的函数接口：**

- `New()` - 创建KusciaScheduling调度插件实例
- `Name()` - 返回调度插件名称
- `PreFilter()` - 预过滤插件函数
- `PostFilter()` - 后过滤插件函数
- `Reserve()` - 预留插件函数
- `Unreserve()` - 取消预留插件函数
- `Permit()` - 许可插件函数
- `PreBind()` - 预绑定插件函数
- `PostBind()` - 后绑定插件函数
- `EventsToRegister()` - 注册事件函数
- `trMgr.PreFilter()` - TaskResource管理器预过滤函数
- `trMgr.Permit()` - TaskResource管理器许可函数
- `trMgr.PreBind()` - TaskResource管理器预绑定函数
- `trMgr.PostBind()` - TaskResource管理器后绑定函数

# 6 完整配置传递流程总结

本节总结了从 SecretPad/业务系统提交任务到 SecretFlow 引擎执行的完整配置传递流程。

## 6.1 配置传递链路

```
SecretPad / 业务系统
    │
    │ 1. CreateJob API 请求
    ▼
{
  "job_id": "psi-demo",
  "tasks": [{
    "task_input_config": "{...SecretFlow计算配置JSON...}",
    "parties": [{
      "domain_id": "alice",
      "resources": {"cpu": "8", "memory": "16Gi"}
    }]
  }]
}
    │
    │ 2. KusciaAPI 转换为 KusciaJob CR
    ▼
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
spec:
  tasks:
    - taskInputConfig: '{...SecretFlow计算配置JSON...}'  # JSON字符串
      parties:
        - domainID: alice
          resources:
            cpu: "8"
            memory: "16Gi"
    │
    │ 3. KusciaJob Controller 创建 KusciaTask CR
    ▼
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaTask
spec:
  taskInputConfig: '{...SecretFlow计算配置JSON...}'  # 保持不变
  parties:
    - domainID: alice
      appImageRef: secretflow-image
      template:
        spec:
          containers:
            - resources:
                requests:
                  cpu: "8"
                  memory: "16Gi"
    │
    │ 4. KusciaTask Controller 查询 AppImage
    ▼
apiVersion: kuscia.secretflow/v1alpha1
kind: AppImage
spec:
  deployTemplates:
    - role: server
      spec:
        containers:
          - command: ["python3", "/root/main.py"]
            args: ["--mode", "server"]
            ports:
              - name: grpc
                port: 8080
    │
    │ 5. KusciaTask Controller 构建 Pod Spec
    ▼
apiVersion: v1
kind: Pod
spec:
  containers:
    - name: psi-container
      image: secretflow/psi:latest
      command: ["python3", "/root/main.py"]
      args: ["--mode", "server"]
      env:
        - name: KUSCIA_TASK_ID
          value: "psi-task"
        - name: SF_CLUSTER_CONFIG  # SecretFlow集群配置
          value: |
            {
              "party": "alice",
              "self_party": "alice",
              "peers": [{
                "party": "bob",
                "address": "psi-task-bob-svc.cross-domain.svc.cluster.local:8080"
              }]
            }
      resources:
        requests:
          cpu: "8"
          memory: "16Gi"
      volumeMounts:
        - name: config-volume
          mountPath: /etc/kuscia/config
  volumes:
    - name: config-volume
      configMap:
        name: psi-task-alice-config
    │
    │ 6. KusciaTask Controller 创建 ConfigMap
    ▼
apiVersion: v1
kind: ConfigMap
data:
  task_input_config.json: |
    {...SecretFlow计算配置JSON...}  # 原始 taskInputConfig
    │
    │ 7. Agent 启动容器，挂载 ConfigMap
    ▼
容器内文件系统：
/etc/kuscia/config/task_input_config.json  # 包含 SecretFlow 计算配置
    │
    │ 8. SecretFlow 引擎读取配置
    ▼
Python 代码：
import json
import os

# 读取环境变量中的集群配置
sf_cluster_config = json.loads(os.environ['SF_CLUSTER_CONFIG'])

# 初始化 SecretFlow 运行时
import secretflow as sf
sf.init(
    party='alice',
    addresses={
        'alice': '0.0.0.0:8080',
        'bob': 'psi-task-bob-svc.cross-domain.svc.cluster.local:8080'
    }
)

# 读取任务输入配置
with open('/etc/kuscia/config/task_input_config.json') as f:
    task_config = json.load(f)

# 执行隐私计算算法
# ... 根据 task_config 执行 PSI、联邦学习等算法 ...
```

## 6.2 关键配置映射关系

| 配置层级 | 配置项 | 传递方式 | 最终位置 |
| --------- | -------- | --------- | ---------- |
| **API 层** | `CreateJob.tasks[].task_input_config` | JSON 字符串 | KusciaJob CR |
| **CR 层** | `KusciaTask.spec.taskInputConfig` | JSON 字符串 | KusciaTask CR |
| **Pod 层** | ConfigMap data | Volume 挂载 | `/etc/kuscia/config/task_input_config.json` |
| **应用层** | SecretFlow 读取文件 | Python `json.load()` | 内存中的 dict |
| | | | |
| **API 层** | `CreateJob.tasks[].parties[].resources` | ResourceRequirements | KusciaJob CR |
| **CR 层** | `KusciaTask.spec.parties[].template.spec.containers[].resources` | ResourceRequirements | KusciaTask CR |
| **Pod 层** | `Pod.spec.containers[].resources` | ResourceRequirements | Pod CR |
| **调度层** | Scheduler 资源检查 | 节点资源对比 | 调度决策 |
| | | | |
| **AppImage 层** | `AppImage.spec.deployTemplates[].spec.containers[].command` | 容器命令 | AppImage CR |
| **Pod 层** | `Pod.spec.containers[].command` | 容器命令 | Pod CR |
| **运行时** | 容器启动命令 | exec | 进程启动 |
| | | | |
| **动态生成** | SecretFlow 集群拓扑 | 环境变量 `SF_CLUSTER_CONFIG` | Pod 环境变量 |
| **应用层** | SecretFlow 读取环境变量 | `os.environ[]` | 内存中的 dict |

## 6.3 数据流与配置流的对应关系

### 6.3.1 控制平面数据流

```
1. 配置提交
   SecretPad → KusciaAPI → KusciaJob CR
   
2. 任务分解
   KusciaJob CR → KusciaTask CR (按 DAG)
   
3. 资源协调
   KusciaTask CR → TaskResourceGroup CR → TaskResource CR
   
4. 资源调度
   TaskResource CR → Scheduler → Pod Binding
   
5. 任务执行
   Pod CR → Agent → Container → SecretFlow Engine
```

### 6.3.2 数据平面数据流

```
1. 数据准备
   DomainData CR → DataMesh → 实际数据源（LocalFS/OSS/DB）
   
2. 数据访问
   SecretFlow Engine → DataMesh API → DomainData → 实际数据
   
3. 计算执行
   SecretFlow Engine ↔ Peer SecretFlow Engine (通过 gRPC)
   
4. 结果输出
   SecretFlow Engine → DataMesh → DomainData → 实际数据源
```

### 6.3.3 配置与数据的关联

```yaml
# DomainData 定义数据源
apiVersion: kuscia.secretflow/v1alpha1
kind: DomainData
metadata:
  name: alice_user_ids
  namespace: alice
spec:
  domainID: alice
  type: localfs
  localFile:
    path: /data/alice/ids.csv

# taskInputConfig 引用 DomainData
taskInputConfig: |
  {
    "input_data": {
      "domain_data_id": "alice_user_ids"  # 引用上面的 DomainData
    }
  }

# SecretFlow 引擎通过 DataMesh 访问数据
import secretflow.data.ndarray as sf_ndarray

# DataMesh 根据 domain_data_id 查找 DomainData CR
# 然后读取实际文件 /data/alice/ids.csv
data = sf_ndarray.read_csv('alice_user_ids')
```

## 6.4 常见问题排查

### 6.4.1 配置传递问题

**问题 1：SecretFlow 引擎无法解析 taskInputConfig**

- **症状**：容器日志显示 JSON 解析错误
- **原因**：taskInputConfig 格式不正确或转义错误
- **排查**：

  ```bash
  # 查看 ConfigMap 内容
  kubectl get configmap psi-task-alice-config -n cross-domain -o yaml
  
  # 进入容器查看文件
  kubectl exec -it psi-task-alice-0 -n cross-domain -- cat /etc/kuscia/config/task_input_config.json
  
  # 验证 JSON 格式
  python3 -m json.tool /etc/kuscia/config/task_input_config.json
  ```

**问题 2：SF_CLUSTER_CONFIG 环境变量缺失**

- **症状**：SecretFlow 引擎启动失败，提示无法连接对端
- **原因**：Pod 未正确注入环境变量
- **排查**：

  ```bash
  # 查看 Pod 环境变量
  kubectl get pod psi-task-alice-0 -n cross-domain -o yaml | grep -A 5 SF_CLUSTER_CONFIG
  
  # 进入容器检查环境变量
  kubectl exec -it psi-task-alice-0 -n cross-domain -- env | grep SF_CLUSTER_CONFIG
  ```

**问题 3：资源配置不匹配**

- **症状**：Pod Pending，提示资源不足
- **原因**：Party.Resources 与节点可用资源不匹配
- **排查**：

  ```bash
  # 查看 Pod 事件
  kubectl describe pod psi-task-alice-0 -n cross-domain
  
  # 查看节点资源
  kubectl top nodes
  
  # 查看 TaskResource
  kubectl get taskresource tr-alice-xxx -n cross-domain -o yaml
  ```

### 6.4.2 网络通信问题

**问题 4：SecretFlow 引擎无法连接对端**

- **症状**：gRPC 连接超时
- **原因**：Service 未正确创建或 DNS 解析失败
- **排查**：

  ```bash
  # 查看 Service
  kubectl get svc psi-task-bob-svc -n cross-domain
  
  # 测试 DNS 解析
  kubectl exec -it psi-task-alice-0 -n cross-domain -- nslookup psi-task-bob-svc.cross-domain.svc.cluster.local
  
  # 测试端口连通性
  kubectl exec -it psi-task-alice-0 -n cross-domain -- nc -zv psi-task-bob-svc.cross-domain.svc.cluster.local 8080
  ```

### 6.4.3 数据访问问题

**问题 5：DomainData 读取失败**

- **症状**：SecretFlow 引擎报告数据文件不存在
- **原因**：DomainData CR 配置错误或文件路径不正确
- **排查**：

  ```bash
  # 查看 DomainData CR
  kubectl get domaindata alice_user_ids -n alice -o yaml
  
  # 进入容器检查文件
  kubectl exec -it psi-task-alice-0 -n cross-domain -- ls -l /data/alice/ids.csv
  
  # 查看 DataMesh 日志
  kubectl logs datamesh-alice-0 -n alice
  ```

## 6.5 最佳实践

### 6.5.1 配置管理

1. **使用 ConfigTemplates**：将通用配置提取到 AppImage 的 ConfigTemplates 中，避免重复
2. **参数化配置**：使用模板变量（如 `{{.TASK_INPUT_CONFIG}}`）实现配置复用
3. **版本控制**：为 AppImage 设置明确的 tag，避免使用 `latest`

### 6.5.2 资源管理

1. **合理设置资源请求**：根据算法复杂度设置 CPU 和 Memory
2. **带宽限制**：为跨域通信设置 bandwidthLimits，避免网络拥塞
3. **超时配置**：设置合理的 task_timeout_seconds，防止任务无限运行

### 6.5.3 安全实践

1. **镜像完整性校验**：设置 AppImage 的 `spec.image.id`，防止镜像被篡改
2. **网络隔离**：使用 NetworkPolicy 限制 Pod 间通信
3. **数据加密**：敏感数据使用 Secret 存储，避免明文传输

### 6.5.4 监控与诊断

1. **启用健康检查**：配置 livenessProbe 和 readinessProbe
2. **日志收集**：集中收集容器日志，便于问题排查
3. **指标监控**：通过 MetricProbe 暴露性能指标

# 7. 端到端函数级处理流程详解

前面章节已经说明了任务调度涉及哪些资源、状态机和配置传递。本章从**函数调用链**视角，详细说明一次联邦学习任务从前端发起、到后端提交、到 Kuscia 控制器、再到 Agent 拉起容器、SecretFlow 执行、DataMesh 读写数据的完整处理过程。

---

## 7.1 SecretPad 前端 → 后端 → KusciaAPI 提交流程

### 7.1.1 前端调用链

| 文件 | 关键函数/组件 | 作用 |
| ------ | --------------- | ------ |
| `secretpad/frontend-src/apps/platform/src/modules/main-dag/graph.tsx` | `GraphView` | 渲染 DAG 画布 |
| `secretpad/frontend-src/apps/platform/src/modules/component-config/config-modal.tsx` / `config-form-view.tsx` | 配置表单 | 收集算法超参、特征选择、参与方配置，结果保存为节点 `nodeDef` |
| `secretpad/frontend-src/apps/platform/src/modules/main-dag/toolbar.tsx` | `ToolbarView.exec` / `run` | 用户点击“全部执行 / 执行到此 / 执行单节点” |
| `secretpad/frontend-src/apps/platform/src/modules/main-dag/graph-request-service.tsx` | `startRun(dagId, componentIds)` | 构造 `{ projectId, graphId, nodes }` 并调用 REST API |
| `secretpad/frontend-src/apps/platform/src/services/secretpad/GraphController.ts` | `startGraph(body)` | OneAPI 生成的请求函数，发送 `POST /api/v1alpha1/graph/start` |

完整调用链：

```text
User 配置节点 (config-modal.tsx / config-form-view.tsx)
        ↓
Toolbar 点击运行 (toolbar.tsx ToolbarView.exec/run)
        ↓
DAG action runAll/runDown/runSingle/runUp
        ↓
GraphRequestService.startRun()
        ↓
POST /api/v1alpha1/graph/start
```

### 7.1.2 后端控制器接收

**文件**：`secretpad/secretpad-web/src/main/java/org/secretflow/secretpad/web/controller/GraphController.java`

```java
@PostMapping("/graph/start")
public SecretPadResponse<StartGraphVO> startGraph(
        @Valid @RequestBody StartGraphRequest request) {
    return SecretPadResponse.success(graphService.startGraph(request));
}
```

处理逻辑：

1. `@Valid` 触发 `StartGraphRequest` 校验：`projectId`、`graphId`、`nodes` 非空。
2. `@DataResource(field = "projectId", resourceType = DataResourceTypeEnum.PROJECT_ID)` 进行项目级权限校验。
3. Controller 本身不做业务解析，直接委托给 `GraphService.startGraph(request)`。

### 7.1.3 GraphServiceImpl 处理

**文件**：`secretpad/secretpad-service/src/main/java/org/secretflow/secretpad/service/impl/GraphServiceImpl.java`

`startGraph(StartGraphRequest request)` 的处理步骤：

1. **存在性与归属检查**
   - `ownerCheck(projectId, graphId)`：确认图存在且当前用户有权限。
   - 校验选中的节点 ID 是否真实存在。
2. **参与方解析**
   - `findTopNodes(edges, selectedNodes)`：计算每个选中节点的上游闭包。
   - `findParties(nodes, topNodes, projectId, partyList)`：从上游数据节点关联的 `ProjectDatatableDO` 推导参与方，并构建列元数据 `TaskConfig.TableAttr`。
   - TEE 模式下，参与方被替换为 TEE 域。
   - `verifyNodeAndRouteHealthy(...)`：检查每个参与方状态及两两路由是否健康。
3. **构建项目作业模型**
   - `ProjectJob.genProjectJob(graphDO, selectedNodes, parties)`：
     - 生成 `jobId = UUIDUtils.random(4)`
     - 每个选中节点生成 `JobTask`，`taskId = JobUtils.genTaskId(jobId, graphNodeId)`
4. **执行 Handler 链**
   - `jobChain.proceed(projectJob)`

Handler 链定义在 `secretpad-service/.../service/configuration/ServiceConfiguration.java`，按 Spring `Ordered` 排序：

| 顺序 | Handler | 文件 | 作用 |
| ------ | --------- | ------ | ------ |
| 1 | `JobPersistentHandler` | `service/graph/chain/JobPersistentHandler.java` | 设置任务状态，持久化 `ProjectJobDO` / `ProjectTaskDO`，写入开始/成功日志 |
| 2 | `JobRenderHandler` | `service/graph/chain/JobRenderHandler.java` | 渲染任务 `inputs`/`outputs`，解析上游依赖，裁剪 SecretPad 内部组件，调用 `NodeDefAdapterFactory` 处理自定义 DSL |
| 3 | `JobSubmittedHandler` | `service/graph/chain/JobSubmittedHandler.java` | 将 `ProjectJob` 转换为 Kuscia `CreateJobRequest`，调用 `jobManager.createJob` |

### 7.1.4 KusciaJobConverter 构建 CreateJobRequest

**文件**：`secretpad/secretpad-service/src/main/java/org/secretflow/secretpad/service/graph/converter/KusciaJobConverter.java`

`JobSubmittedHandler.doHandler` 中：

```java
if (GraphContext.isTee()) {
    request = trustedFlowJobConverter.converter(job);
} else {
    request = jobConverter.converter(job);
}
jobManager.createJob(request);
```

`KusciaJobConverter.converter(ProjectJob job)` 的关键字段映射：

| Kuscia `CreateJobRequest` 字段 | 来源 |
| -------------------------------- | ------ |
| `job_id` | `job.getJobId()` |
| `initiator` | 首个参与方；`AUTONOMY` 模式下由 `envService.findLocalNodeId(task)` 覆盖为本地节点 |
| `max_parallelism` | `job.getMaxParallelism()` |
| `tasks` | 每个 `ProjectJob.JobTask` 对应一个 `Job.Task` |
| `Task.task_id` / `alias` | `task.getTaskId()` |
| `Task.app_image` | `JobConstants.APP_IMAGE`（SCQL 任务使用 `SCQL_IMAGE`） |
| `Task.parties` | `task.getParties()` → `Job.Party{domain_id}` |
| `Task.dependencies` | `task.getDependencies()` |
| `Task.task_input_config` | `TaskConfig.TaskInputConfig` 的 JSON 序列化结果 |

`renderTaskInputConfig(...)` 构建 `TaskConfig.TaskInputConfig` 的过程：

1. 从任务节点获取 `nodeDef`。
2. `ComponentTools.getNodeDef(nodeDef)` 将前端 `nodeDef` JSON 转换为 `Pipeline.NodeDef`。
3. 处理可恢复训练组件的 checkpoint URI。
4. `buildDatasourceConfig(parties, projectId, graphId)` 构建每个参与方的 datasource 配置。
5. 构建 `Cluster.SFClusterDesc`，包含参与方、设备配置、`ray_fed_config.cross_silo_comm_backend`。
6. 组装 `TaskConfig.TaskInputConfig`：

```java
TaskConfig.TaskInputConfig taskInputConfig = TaskConfig.TaskInputConfig.newBuilder()
        .putAllSfDatasourceConfig(stringDatasourceConfigMap)
        .addAllSfInputIds(task.getNode().getInputs())
        .addAllTableAttrs(...)
        .addAllSfInputPartitionsSpec(buildSfInputPartitions(...))
        .addAllSfOutputIds(task.getNode().getOutputs())
        .addAllSfOutputUris(outputUris)
        .setSfClusterDesc(sfClusterDesc)
        .setSfNodeEvalParam(pipelineNodeDef)
        .build();
```

7. 使用 `ProtoUtils.toJsonString(taskInputConfig, typeRegistry)` 序列化为 JSON 字符串，作为 `task_input_config`。

### 7.1.5 JobManager 通过 gRPC 调用 KusciaAPI

**文件**：`secretpad-manager/.../manager/integration/job/JobManager.java`

```java
public void createJob(Job.CreateJobRequest request) {
    Job.CreateJobResponse response;
    if (PlatformTypeEnum.AUTONOMY.equals(getPlaformType())) {
        response = kusciaGrpcClientAdapter.createJob(request, request.getInitiator());
    } else {
        response = kusciaGrpcClientAdapter.createJob(request);
    }
    if (status.getCode() != 0) {
        throw SecretpadException.of(PROJECT_JOB_CREATE_ERROR, status.getMessage());
    }
}
```

gRPC 调用链：

```text
JobManager.createJob
  → KusciaGrpcClientAdapter.createJob(request)
  → DynamicKusciaChannelProvider.currentStub / createStub
  → JobServiceGrpc.JobServiceBlockingStub.createJob(...)
  → KusciaAPI CreateJob gRPC / HTTP
```

- `DynamicKusciaChannelProvider`：`secretpad-api/client-java-kusciaapi/.../kuscia/v1alpha1/DynamicKusciaChannelProvider.java`，维护 `ConcurrentHashMap<String, KusciaApiChannelFactory>`，按 `secretpad.node-id` 选择当前 stub，或为指定 domainId 创建 stub。
- `KusciaGrpcClientAdapter`：`secretpad-api/client-java-kusciaapi/.../kuscia/v1alpha1/service/impl/KusciaGrpcClientAdapter.java`，实现 `KusciaJobService` 接口。

---

## 7.2 KusciaAPI CreateJob 的函数处理流程

### 7.2.1 Handler 入口

KusciaAPI 同时暴露 HTTP 和 gRPC 两种入口，最终都调用同一 Service。

**HTTP Handler**：`kuscia/pkg/kusciaapi/handler/httphandler/job/create.go`

```go
func (h *createJobHandler) Handle(ctx *api.BizContext, request api.ProtoRequest) api.ProtoResponse {
    req := request.(*kusciaapi.CreateJobRequest)
    return h.jobService.CreateJob(ctx, req)
}
```

路由注册在 `pkg/kusciaapi/bean/http_server_bean.go:188`，路径为 `POST /api/v1/job/create`。

**gRPC Handler**：`kuscia/pkg/kusciaapi/handler/grpchandler/job_handler.go`

```go
func (h *jobHandler) CreateJob(ctx context.Context, request *kusciaapi.CreateJobRequest) (*kusciaapi.CreateJobResponse, error) {
    return h.jobService.CreateJob(ctx, request), nil
}
```

注册在 `pkg/kusciaapi/bean/grpc_server_bean.go:106`。

### 7.2.2 JobService.CreateJob 校验与转换

**文件**：`kuscia/pkg/kusciaapi/service/job_service.go`

```go
func (h *jobService) CreateJob(ctx context.Context, request *kusciaapi.CreateJobRequest) *kusciaapi.CreateJobResponse
```

处理步骤：

1. **请求校验**（`validateCreateJobRequest(request, h.Initiator)`）
   - `job_id` 非空且符合 K8s 命名规范（`resources.ValidateK8sName`）。
   - `tasks` 非空。
   - `initiator` 非空；P2P 模式下必须等于本地 domain ID。
   - `max_parallelism` 缺省或 `<=0` 时设为 `1`。
   - 每个任务必须有非空 `alias`。
   - 每个任务至少有一个参与方，且 `domain_id` 非空。
2. **权限检查**（`h.authHandlerJobCreate(ctx, request)`）
   - 允许发起方 domain。
   - 允许任务中所有参与方 domain。
   - 其他情况拒绝。
3. **资源解析**
   - 对每个参与方解析 `cpu`、`memory`、`bandwidth` 为 `corev1.ResourceQuantity`。
   - 校验 `bandwidth_limits`：`limit_kbps > 0`，`destination_id` 非空。
4. **构建并创建 KusciaJob CR**
   - 调用 `h.kusciaClient.KusciaV1alpha1().KusciaJobs(common.KusciaCrossDomain).Create(...)`。

### 7.2.3 Proto → CRD 字段映射

转换逻辑在 `job_service.go` 的 `CreateJob` 方法中完成，主要映射关系：

| Proto 字段 | CRD 字段 |
| ------------ | ---------- |
| `job_id` | `metadata.name` |
| `custom_fields` | 标签 `kuscia.secretflow/job-custom-fields.<key>` |
| `initiator` | `spec.initiator` |
| `max_parallelism` | `spec.maxParallelism`（缺省 1） |
| `schedule_mode` | 当前硬编码为 `KusciaJobScheduleModeBestEffort` |
| `tasks` | `spec.tasks`（`[]KusciaTaskTemplate`） |
| `task.task_id` | `KusciaTaskTemplate.TaskID` |
| `task.alias` | `KusciaTaskTemplate.Alias` |
| `task.dependencies` | `KusciaTaskTemplate.Dependencies` |
| `task.app_image` | `KusciaTaskTemplate.AppImage` |
| `task.task_input_config` | `KusciaTaskTemplate.TaskInputConfig` |
| `task.priority` | `KusciaTaskTemplate.Priority` |
| `task.tolerable` | `KusciaTaskTemplate.Tolerable` |
| `task.schedule_config` | 经 `buildScheduleConfigForKusciaTask` 转换 |
| `party.domain_id` | `Party.DomainID` |
| `party.role` | `Party.Role` |
| `party.resources` | `Party.Resources` |
| `party.bandwidth_limits` | `Party.BandwidthLimit` |

`ScheduleConfig` 转换：

| Proto 字段 | CRD 字段 | 缺省值 |
| ------------ | ---------- | -------- |
| `task_timeout_seconds` | `LifecycleSeconds` | 300 |
| `resource_reserved_seconds` | `ResourceReservedSeconds` | 30 |
| `resource_reallocation_interval_seconds` | `RetryIntervalSeconds` | 30 |

---

## 7.3 KusciaJob Controller 分阶段函数处理

### 7.3.1 Controller 主循环

**文件**：`kuscia/pkg/controllers/kusciajob/controller.go`

`NewController` 中注册事件处理器：

```go
_, _ = kusciaJobInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc:    controller.enqueueKusciaJob,
    UpdateFunc: controller.enqueueKusciaJob,
    DeleteFunc: controller.enqueueKusciaJob,
})
```

`enqueueKusciaJob` 使用 `cache.DeletionHandlingMetaNamespaceKeyFunc` 生成 `namespace/name` key，加入 `c.workqueue`。

`syncHandler` 主流程：

```go
func (c *Controller) syncHandler(ctx context.Context, key string) (retErr error) {
    // 1. 解析 key 得到 job name
    // 2. 从 lister 获取 KusciaJob
    // 3. DeepCopy：curJob := preJob.DeepCopy()
    // 4. 填充默认值：kusciaJobDefault(curJob)
    //    - status.phase 为空时设为 KusciaJobInitialized
    //    - spec.maxParallelism 为空时设为 1
    // 5. 若 handler.ShouldReconcile(curJob) 返回 false 则跳过（已完成且设置 completionTime）
    // 6. 按 phase 分发：needUpdate, err := c.handlerFactory.KusciaJobPhaseHandlerFor(phase).HandlePhase(curJob)
    // 7. 最大重试次数超过阈值则 failKusciaJob
    // 8. 若 needUpdate 为 true：utilsres.UpdateKusciaJobStatus(c.kusciaClient, preJob, curJob)
}
```

Handler 工厂（`pkg/controllers/kusciajob/handler/factory.go`）：

```go
KusciaJobStateHandlerMap := map[kusciaapisv1alpha1.KusciaJobPhase]KusciaJobPhaseHandler{
    KusciaJobInitialized:      NewInitializedHandler(deps),
    KusciaJobAwaitingApproval: NewAwaitingApprovalHandler(deps),
    KusciaJobPending:          NewPendingHandler(deps),
    KusciaJobRunning:          NewRunningHandler(deps),
    ...
}
```

### 7.3.2 InitializedHandler

**文件**：`kuscia/pkg/controllers/kusciajob/handler/initialized.go`

```go
func (h *InitializedHandler) HandlePhase(kusciaJob *v1alpha1.KusciaJob) (bool, error)
```

内部委托给 `*JobScheduler.handleInitialized`：

1. `updateJobTime(now, job)`：记录 reconcile 时间。
2. **校验**（`h.validateJob(now, job)`）：
   - 若 `JobValidated` 条件已为 true，跳过。
   - 调用 `kusciaJobValidate`：
     - 发起方 namespace 必须存在。
     - 至少有一个任务。
     - 依赖存在性检查 `kusciaJobDependenciesExits`。
     - 环检测 `kusciaJobHasTaskCycle`（拓扑剥离法）。
   - 校验失败：设置 `JobValidated=False`，phase → `KusciaJobFailed`。
3. **注解处理**（`h.annotateKusciaJob`）：
   - 判断 `SelfClusterAsInitiator`。
   - 标注互联互通类型（`bfia` / `kuscia`）。
   - 更新 CR 后返回 `(false, nil)`，等待下次 reconcile。
4. **初始化状态映射**：
   - `Status.StageStatus`
   - `Status.ApproveStatus`
5. **设置本方参与方状态**：
   - 若本集群是发起方：本方参与方 `JobCreateStageSucceeded` + `JobAccepted`。
   - 若本集群是参与方：本方参与方仅 `JobCreateStageSucceeded`。
6. **阶段转换**：
   - 互联互通任务且非 BFIA：phase → `KusciaJobAwaitingApproval`。
   - 其他：phase → `KusciaJobPending`。
7. 返回 `needUpdate=true`。

### 7.3.3 RunningHandler

**文件**：`kuscia/pkg/controllers/kusciajob/handler/running.go`

```go
func (h *RunningHandler) HandlePhase(job *v1alpha1.KusciaJob) (bool, error)
```

内部委托给 `*JobScheduler.handleRunning`：

1. `updateJobTime(now, job)`。
2. **阶段命令处理**（`h.handleStageCommand(now, job)`）：
   - 检查标签 `kuscia.secretflow/job-stage`、`job-stage-trigger`。
   - 处理 `Start`、`Stop`、`Cancel`、`Restart`、`Suspend` 命令。
   - 若处理过命令，提前返回。
3. **设置 TaskID**（`h.setJobTaskID(job)`，仅发起方）：
   - 对空 `TaskID` 生成 `jobName-<uuid后缀>`。
   - 回写 `KusciaJob.spec.tasks[].taskID` 后返回 `(false, err)`，重新入队。
4. **列出子任务**：
   - 选择器：`kuscia.secretflow/controller=kuscia-job` + `kuscia.secretflow/job-uid=<job.UID>`。
   - `subTasks, err := h.kusciaTaskLister.List(selector)`。
5. **重建任务状态视图**：
   - `buildJobSubTaskStatus(subTasks, job)` 返回两个 map：
     - `currentSubTasksStatusWithAlias`：key 为 alias。
     - `currentSubTasksStatusWithID`：key 为 taskID。
   - `updateJobSubTaskStatus` 更新 `job.Status.TaskStatus`。
6. **计算 Job 阶段**：
   - `currentJobPhase := jobStatusPhaseFrom(job, currentSubTasksStatusWithAlias)`。
   - 根据任务状态与 `ScheduleMode`（`Strict`/`BestEffort`）推导 Running/Succeeded/Failed。
7. **计算就绪任务**：
   - `readyTasks := readyTasksOf(job, currentSubTasksStatusWithAlias)`。
   - 过滤依赖全部 `Succeeded` 且尚未创建的任务，按 `priority` 降序。
8. **并发度裁剪**：
   - `willStartTask := willStartTasksOf(job, readyTask, currentSubTasksStatusWithAlias)`。
   - `runningCount = Pending/Running/空 状态任务数`。
   - 可创建数 = `maxParallelism - runningCount`。
9. **创建 KusciaTask**：
   - `willStartKusciaTasks := h.buildWillStartKusciaTask(job, willStartTask)`。
   - 每个任务 CR：
     - `metadata.name = task.TaskID`
     - `OwnerReference` 指向 KusciaJob
     - 注解：`job-id`、`task-alias`、`self-cluster-as-participant`
     - 标签：`controller=kuscia-job`、`job-uid=<job.UID>`
   - 调用 `h.kusciaClient.KusciaV1alpha1().KusciaTasks(common.KusciaCrossDomain).Create(...)`。
   - 若已存在同名任务，校验 `LabelJobUID` 是否一致，否则失败 Job。
10. **更新 Job 状态**：
    - `buildJobStatus(now, &job.Status, currentJobPhase)` 设置 phase 与完成时间。
11. 返回 `needUpdateStatus`。

**子任务变化反推父 Job**：

```go
_, _ = kusciaTaskInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
    AddFunc:    controller.handleTaskObject,
    UpdateFunc: controller.handleTaskObject,
    DeleteFunc: controller.handleTaskObject,
})
```

`handleTaskObject`：

1. 提取对象，处理 tombstone。
2. `ownerRef := metav1.GetControllerOf(object)`。
3. 若 `ownerRef.Kind != KusciaJobKind` 则跳过。
4. 通过 lister 找到父 Job，调用 `enqueueKusciaJob(kusciaJob)`。

这样每次 KusciaTask 状态变化都会触发父 Job 重新 reconcile。

---

## 7.4 KusciaTask Controller 创建资源的函数处理流程

### 7.4.1 syncHandler 与 PendingHandler

**文件**：`kuscia/pkg/controllers/kusciatask/controller.go`

```go
func (c *Controller) syncHandler(key string) (retErr error)
```

处理流程：

1. `processNextWorkItem` 弹出 `namespace/name` key，调用 `syncHandler`。
2. 从 lister 获取 `KusciaTask`（`common.KusciaCrossDomain`）。
3. DeepCopy 后，根据 `status.phase` 分发；为空时默认 `TaskPending`。
4. `needUpdate, err := c.handlerFactory.GetKusciaTaskPhaseHandler(phase).Handle(kusciaTask)`。

Phase Handler 工厂（`pkg/controllers/kusciatask/handler/factory.go`）：

```go
kusciaTaskStateHandlerMap := map[kusciaapisv1alpha1.KusciaTaskPhase]KusciaTaskPhaseHandler{
    kusciaapisv1alpha1.TaskPending:   pendingHandler,
    kusciaapisv1alpha1.TaskRunning:   runningHandler,
    kusciaapisv1alpha1.TaskSucceeded: succeededHandler,
    kusciaapisv1alpha1.TaskFailed:    failedHandler,
}
```

**PendingHandler**：`pkg/controllers/kusciatask/handler/pending_handler.go`

```go
func (h *PendingHandler) Handle(kusciaTask *kusciaapisv1alpha1.KusciaTask) (needUpdate bool, err error)
```

执行顺序：

1. `prepareTaskResources(now, kusciaTask)`
   - `StartTime == nil` → 设置 `StartTime`
   - `Phase == ""` → 设置 `Phase = TaskPending`
   - `PortsAllocated` 未设置 → `allocatePorts(kusciaTask)`
   - `ResourceCreated` 未设置 → `createTaskResources(kusciaTask)`
2. `initPartyTaskStatus(...)` + `refreshKtResourcesStatus(...)`
3. 检查 `taskFailed`、`taskRunning`、`taskExpired`

`createTaskResources` 核心流程：

```text
buildPartyKitInfos(kusciaTask)
buildPodAllocatePorts(kusciaTask, selfPartyKitInfos)
generateParties(partyKitInfos)
for each partyKitInfo: fillPartyClusterDefine(partyKitInfo, parties)

for each selfPartyKitInfo:
    pluginManager.Permit(...)
    createResourceForParty(partyKit)   // ConfigMap → Pod → Services

createTaskResourceGroup(kusciaTask, partyKitInfos)
```

`createResourceForParty` 每参与方创建顺序：

1. `generateConfigMap(partyKit)` + `submitConfigMap(...)`（模板 ConfigMap）
2. `generatePod(partyKit, podKit)` + `submitPod(...)`（Pod，内部会生成 kuscia-gen ConfigMap）
3. `generateServices(...)` + `submitService(...)`（每个服务端口一个 Service）

全局顺序：

```text
for each self party:
    ConfigMap(template)
    Pod
    Service(s)
    Kuscia-gen ConfigMap (created during Pod generation)
TaskResourceGroup
```

### 7.4.2 AppImage 与 DeployTemplate 选择

**文件**：`kuscia/pkg/controllers/kusciatask/handler/pending_handler.go`

`buildPartyKitInfo` 中：

```go
appImage, err := h.appImagesLister.Get(party.AppImageRef)
baseDeployTemplate, err := utilsres.SelectDeployTemplate(appImage.Spec.DeployTemplates, party.Role)
deployTemplate := mergeDeployTemplate(baseDeployTemplate, &party.Template)
```

`SelectDeployTemplate`（`pkg/utils/resources/appimage.go`）匹配逻辑：

1. 遍历所有 deployTemplate，将 `template.Role` 按 `,` 拆分。
2. 若任一 role 等于 `party.Role`，命中。
3. 未命中且存在 `Role == ""` 的模板，作为默认模板。
4. 仍无匹配且 `party.Role == ""`，使用 `templates[0]`。
5. 否则报错。

`mergeDeployTemplate` 合并逻辑：

1. `template = baseTemplate.DeepCopy()`。
2. 若 `partyTemplate.Replicas != nil`，覆盖 `Replicas`。
3. 若 `partyTemplate.Spec.RestartPolicy` 非空，覆盖。
4. 按容器索引对齐：
   - 覆盖 `Name`
   - 覆盖 `Command`/`Args`
   - 追加 `Env`
   - 覆盖 `Resource Requests/Limits`

### 7.4.3 资源创建顺序与命名

**Pod 名称**：`generatePodName(taskName, role, index)`

```go
func generatePodName(taskName string, role string, index int) string {
    if role == "" {
        return fmt.Sprintf("%s-%d", taskName, index)
    }
    return fmt.Sprintf("%s-%s-%d", taskName, role, index)
}
```

示例：`task=psi-abc, role=server, index=0` → `psi-abc-server-0`。

**Service 名称**：`GenerateServiceName(prefix, portName)`（`pkg/utils/resources/service.go`）

以 Pod 名为 prefix，例如 `psi-abc-server-0` + `grpc` → `psi-abc-server-0-grpc`。

**ConfigMap 名称**：

- 模板 ConfigMap：`<task>-configtemplate`
- Kuscia 生成值 ConfigMap：`<task>-kuscia-gen-conf`（格式 `common.KusciaGenerateConfigMapFormat = "%s-kuscia-gen-conf"`）

**TaskResourceGroup 名称**：

与 KusciaTask 同名：`kusciaTask.Name`。

### 7.4.4 ClusterDefine 与 TASK_CLUSTER_DEFINE 生成

Controller 并不会直接注入名为 `SF_CLUSTER_CONFIG` 的环境变量。实际流程是：

1. Controller 将多方通信拓扑序列化为 protobuf `ClusterDefine`。
2. 写入 kuscia-gen ConfigMap 的 `TASK_CLUSTER_DEFINE` 键。
3. Agent 的 `config-render` 插件读取模板 ConfigMap 与 kuscia-gen ConfigMap，渲染出 `task-config.conf`。
4. SecretFlow 容器启动后读取 `task-config.conf`，解析 `task_cluster_def`，再构建内部 SecretFlow 集群配置。

`ClusterDefine` 构建流程：

1. `generateParty(kitInfo)` 为每个参与方构建 `proto.Party`。
2. Endpoint DNS 构造：

```go
if pod.Ports[portName].Scope == kusciaapisv1alpha1.ScopeDomain {
    endpointAddress = fmt.Sprintf("%s.%s.svc:%d", pod.PortService[portName], kitInfo.DomainID, pod.Ports[portName].Port)
} else {
    endpointAddress = fmt.Sprintf("%s.%s.svc", pod.PortService[portName], kitInfo.DomainID)
}
```

3. `generateParties(partyKitInfos)` 收集所有参与方。
4. `fillPartyClusterDefine(kitInfo, parties)` / `fillPodClusterDefine` 将完整 `ClusterDefine` 附加到每个 Pod：

```go
pod.ClusterDef = &proto.ClusterDefine{
    Parties:         parties,
    SelfPartyIdx:    int32(partyIndex),
    SelfEndpointIdx: int32(endpointIndex),
}
```

5. `generateKusciaConfigMap` 序列化：

```go
clusterDefine, _ := protoJSONOptions.Marshal(podKit.ClusterDef)
confMap[common.EnvTaskClusterDefine] = string(clusterDefine)

allocatedPorts, _ := protoJSONOptions.Marshal(podKit.AllocatedPorts)
confMap[common.EnvAllocatedPorts] = string(allocatedPorts)

confMap[common.EnvDomainID] = partyKit.DomainID
confMap[common.EnvTaskID] = partyKit.KusciaTask.Name
confMapBinaryData[common.EnvTaskInputConfig] = gzip(taskInputConfig)
```

**ConfigMap 内容**：

- **模板 ConfigMap**（`<task>-configtemplate`）：来自 `AppImage.Spec.ConfigTemplates`，例如 `task-config.conf`，内含 Go template 占位符 `{{.TASK_ID}}`、`{{.TASK_INPUT_CONFIG}}`、`{{.TASK_CLUSTER_DEFINE}}`、`{{.ALLOCATED_PORTS}}`。
- **Kuscia 生成值 ConfigMap**（`<task>-kuscia-gen-conf`）：包含实际值，`TASK_INPUT_CONFIG` 使用 gzip 压缩后放在 `BinaryData`。

**配置渲染**：`pkg/agent/middleware/plugins/hook/configrender/config_render.go`

`config-render` 插件合并：

- Pod 环境变量
- kuscia-gen ConfigMap 值
- 本地 Kuscia 配置（`KUSCIA_API_PROTOCOL`、`KUSCIA_API_TOKEN`、`KUSCIA_DOMAIN_KEY_DATA`）
- 可选 ConfManager 条目

最终渲染出容器内的真实文件，如 `/work/kuscia/task-config.conf`。

### 7.4.5 单参与方与多方任务差异

**是否本方参与**（`handler/common.go` 与 `pending_handler.go`）：

```go
if kusciaTask.Annotations[common.SelfClusterAsParticipantAnnotationKey] == "true" → true
else if any party.DomainID is NOT a partner domain → true
else → false
```

Partner domain 由 namespace 标签 `kuscia.secretflow/role: Partner` 标识。

**本地单参与方任务**：

- `partyKitInfos == selfPartyKitInfos`，无 partner。
- 为每个参与方创建 Pod/Service/ConfigMap。
- TRG 的 `Spec.OutOfControlledParties` 为空。
- 不生成 `PortAccessDomains`。

**多方任务**：

- `buildPartyKitInfos` 拆分为：
  - `partyKitInfos`：所有参与方
  - `selfPartyKitInfos`：仅非 partner（本地可控）参与方
- 仅为 self parties 创建 Pod/Service/ConfigMap；partner parties 保留占位信息用于构建 `ClusterDefine`。
- TRG 区分：
  - `Spec.Parties`：本地可控参与方，含真实 pod 列表
  - `Spec.OutOfControlledParties`：partner 参与方，`MinReservedPods: 1`，pod 列表为空
- `generateTaskResourceGroup` 会调整 `MinReservedMembers`，减去不可控参与方数量。
- TaskResourceGroup Controller 为 partner domains 创建 `TaskResource` 并同步到对端集群。
- `generatePortAccessDomains` 仅在参与方数量大于 1 时启用，限制跨域访问 Cluster-scope 端口。

---

## 7.5 Agent 启动 Pod 与容器的函数处理流程

### 7.5.1 PodsController 事件分发

**文件**：`kuscia/pkg/agent/framework/pods_controller.go`

主循环：

```go
func (pc *PodsController) Run(ctx context.Context) error {
    return pc.syncLoop(ctx, pc, pc.chUpdates)
}
```

`syncLoop` → `syncLoopIteration`，按操作类型分发：

| 操作 | 处理函数 | 行为 |
| ------ | ---------- | ------ |
| `ADD` | `HandlePodAdditions(pods)` | `podManager.AddPod` → hook 插件 → `dispatchWork(pod, SyncPodCreate, ...)` |
| `UPDATE` / `DELETE` | `HandlePodUpdates(pods)` | `podManager.UpdatePod` → `dispatchWork(pod, SyncPodUpdate, ...)` |
| `REMOVE` | `HandlePodRemoves(pods)` | `podManager.DeletePod` → `deletePod` → `dispatchWork(pod, SyncPodKill, ...)` |
| `RECONCILE` | `HandlePodReconcile(pods)` | `dispatchWork(pod, SyncPodSync, ...)` |

`dispatchWork` 将 Pod 加入 `podWorkers`（`pkg/agent/framework/pod_workers.go`）：

```go
pc.podWorkers.UpdatePod(UpdatePodOptions{Pod: pod, UpdateType: syncType, ...})
```

每个 UID 对应一个 goroutine 在 `managePodLoop` 中循环调用：

- `syncPodFn` → `PodsController.syncPod`
- `syncTerminatingPodFn` → `syncTerminatingPod`
- `syncTerminatedPodFn` → `syncTerminatedPod`

`PodsController.syncPod` 最终调用 runtime provider：

```go
if err = pc.provider.SyncPod(ctx, podCopy, podStatus, pc.reasonCache); err != nil {
    ...
}
```

### 7.5.2 CRIProvider 容器启动

对于 container-runtime 路径，provider 为 `CRIProvider`（`kuscia/pkg/agent/provider/pod/cri_provider.go`）：

```go
func (cp *CRIProvider) SyncPod(ctx, pod, podStatus, reasonCache) error {
    cp.volumeManager.MountVolumesForPod(pod)
    cp.containerRuntime.SyncPod(ctx, pod, podStatus, auth, cp.backOff)
}
```

`cp.containerRuntime` 是 `kubeGenericRuntimeManager`（`pkg/agent/kuberuntime/kuberuntime_manager.go`）：

```go
func (m *kubeGenericRuntimeManager) SyncPod(ctx, pod, podStatus, auth, backOff) PodSyncResult {
    podContainerChanges := m.computePodActions(pod, podStatus)
    if podContainerChanges.KillPod {
        m.killPodWithSyncResult(...)
    }
    if podContainerChanges.CreateSandbox {
        podSandboxID, _, err = m.createPodSandbox(ctx, pod, attempt)
    }
    for idx := range podContainerChanges.ContainersToStart {
        _ = start(ctx, "container", metrics.Container,
                   containerStartSpec(&pod.Spec.Containers[idx]))
    }
}
```

`start` 调用 `startContainer`（`pkg/agent/kuberuntime/kuberuntime_container.go`）：

```go
func (m *kubeGenericRuntimeManager) startContainer(ctx, podSandboxID, podSandboxConfig,
                                                    spec, pod, podStatus, auth, podIP, podIPs) {
    imageRef, msg, err := m.imagePuller.EnsureImageExists(ctx, pod, container, auth, podSandboxConfig)
    containerConfig, cleanupAction, err := m.generateContainerConfig(container, pod, restartCount,
                                                                      podIP, imageRef, podIPs)
    containerID, err := m.runtimeService.CreateContainer(ctx, podSandboxID, containerConfig, podSandboxConfig)
    err = m.runtimeService.StartContainer(ctx, containerID)
}
```

容器启动顺序：

1. **拉取镜像**：`imageManager.EnsureImageExists` → CRI `PullImage`
2. **创建 Pod Sandbox**：`createPodSandbox` → CRI `RunPodSandbox`
3. **生成容器配置**（含挂载）
4. **创建容器**：CRI `CreateContainer`
5. **启动容器**：CRI `StartContainer`

CRI 客户端在 `CRIProvider` 构造时创建：

```go
remoteRuntimeService, err = remote.NewRemoteRuntimeService(dep.CRIProviderCfg.RemoteRuntimeEndpoint, ...)
remoteImageService, err   = remote.NewRemoteImageService(dep.CRIProviderCfg.RemoteImageEndpoint, ...)
```

containerd 模式下 endpoint 通常为 `/run/containerd/containerd.sock`。

### 7.5.3 ConfigMap 挂载

在 CRI/containerd 模式下，Agent 负责将 ConfigMap 内容落到宿主机，再告诉 containerd bind-mount。

1. `CRIProvider.SyncPod` 调用 `cp.volumeManager.MountVolumesForPod(pod)`：

   **文件**：`kuscia/pkg/agent/resource/volume_manager.go`

   ```go
   func (vm *VolumeManager) MountVolumesForPod(pod *v1.Pod) error {
       for each volume:
           case volume.ConfigMap != nil:
               volumeInfo, err = vm.mountConfigMap(pod, volume.ConfigMap)
   }
   ```

2. `mountConfigMap` 从 `KubeResourceManager` 获取 ConfigMap，将 `Data` + `BinaryData` 每个 key 写入宿主机文件：

   ```go
   hostPath := vm.getPath(pod.UID, configMapPluginName, volume.Name)
   vm.mountLiteralVolume(hostPath, volume.DefaultMode, volume.Items, volume.Optional, dataMap)
   ```

   宿主机路径形如：`<podDir>/volumes/kubernetes.io~configmap/<volName>/<key>`。

3. 生成容器配置时，`CRIProvider.GenerateRunContainerOptions` 读取已挂载卷，将容器 `VolumeMount` 映射为 `pkgcontainer.Mount`：

   **文件**：`kuscia/pkg/agent/provider/pod/cri_provider.go`

   ```go
   func (cp *CRIProvider) GenerateRunContainerOptions(...) {
       volumes := cp.volumeManager.GetMountedVolumesForPod(pod.UID)
       mounts, err := cp.makeMounts(pod, container, volumes, opts.Envs)
   }
   ```

4. `kubeGenericRuntimeManager.generateContainerConfig` 将 mounts 传给 CRI runtime（`pkg/agent/kuberuntime/kuberuntime_container.go`）。

5. `makeMounts` 转换为 `runtimeapi.Mount`，containerd 完成最终的 bind mount。

> 注：`K8sProvider` 模式（`pkg/agent/provider/pod/k8s_provider.go`）下，Agent 只在后端 Kubernetes 中重建 ConfigMap/Secret，由后端 kubelet 负责挂载。

---

## 7.6 SecretFlow 容器启动与运行时初始化

### 7.6.1 容器入口与 task-config.conf 解析

容器入口由 **AppImage** 的 deployTemplate 定义，典型配置：

```yaml
spec:
  configTemplates:
    task-config.conf: |
      {
        "task_id": "{{.TASK_ID}}",
        "task_input_config": "{{.TASK_INPUT_CONFIG}}",
        "task_cluster_def": "{{.TASK_CLUSTER_DEFINE}}",
        "task_progress_url": "http://reporter.master.svc/report/progress?task_id={{.TASK_ID}}",
        "allocated_ports": "{{.ALLOCATED_PORTS}}"
      }
  deployTemplates:
    - name: secretflow
      spec:
        containers:
          - command:
              - sh
            args:
              - -c
              - "python -m secretflow.kuscia.entry ./kuscia/task-config.conf"
            configVolumeMounts:
              - mountPath: /work/kuscia/task-config.conf
                subPath: task-config.conf
```

容器实际启动命令：

```bash
sh -c "python -m secretflow.kuscia.entry ./kuscia/task-config.conf"
```

`task-config.conf` 经 config-render 渲染后挂载到 `/work/kuscia/task-config.conf`。

**Python 入口**：`secretflow/secretflow/kuscia/entry.py`

```python
@click.command()
@click.argument("task_config_path", type=click.Path(exists=True))
def main(task_config_path, datamesh_addr, enable_plugins: bool):
    if enable_plugins:
        load_plugins()
    os.environ["DATAMESH_ADDRESS"] = datamesh_addr
    task_conf = KusciaTaskConfig.from_file(task_config_path)
    sf_cluster_config = get_sf_cluster_config(task_conf)
    res = comp_eval(sf_node_eval_param, storage_config, sf_cluster_config)
```

**配置解析**：`secretflow/secretflow/kuscia/task_config.py`

```python
@classmethod
def from_file(cls, task_config_path: str):
    with open(task_config_path) as f:
        configs = json.load(f)
        configs["task_input_config"] = json.loads(configs["task_input_config"])
        return cls.from_json(configs)

@classmethod
def from_json(cls, req: Dict):
    task_id = req["task_id"]
    task_cluster_def = ClusterDefine()
    json_format.Parse(req["task_cluster_def"], task_cluster_def)
    task_allocated_ports = AllocatedPorts()
    json_format.Parse(req["allocated_ports"], task_allocated_ports)

    sf_node_eval_param = NodeEvalParam()
    json_format.ParseDict(req["task_input_config"]["sf_node_eval_param"], sf_node_eval_param)

    sf_cluster_desc = SFClusterDesc()
    json_format.ParseDict(req["task_input_config"]["sf_cluster_desc"], sf_cluster_desc)
    ...
```

`task_input_config` 不是直接读取 `/etc/kuscia/config/task_input_config.json`，而是从 `task-config.conf` 的 `task_input_config` 字段解析得到。

### 7.6.2 SFClusterConfig 构建

**文件**：`secretflow/secretflow/kuscia/sf_config.py`

```python
def get_sf_cluster_config(kuscia_config: KusciaTaskConfig) -> SFClusterConfig:
    sf_cluster_desc = kuscia_config.sf_cluster_desc
    kuscia_task_cluster_def = kuscia_config.task_cluster_def
    kuscia_task_allocated_ports = kuscia_config.task_allocated_ports
    ray_config = RayConfig.from_kuscia_task_config(kuscia_config)

    party_id = kuscia_task_cluster_def.self_party_idx
    party_name = kuscia_task_cluster_def.parties[party_id].name
    ...
    return SFClusterConfig(
        desc=sf_cluster_desc,
        private_config=SFClusterConfig.PrivateConfig(
            self_party=party_name,
            ray_head_addr=f"{ray_config.ray_node_ip_address}:{ray_config.ray_gcs_port}",
        ),
        ...
    )
```

Ray 配置来源：`secretflow/secretflow/kuscia/ray_config.py`

```python
@classmethod
def from_kuscia_task_config(cls, config: KusciaTaskConfig):
    for port in allocated_port.ports:
        if port.name.startswith("ray-worker"):
            ray_worker_ports.append(port.port)
        elif port.name == "node-manager": ...
    for party in cluster_define.parties:
        if party.name == party_name:
            for service in party.services:
                if service.port_name == "global":
                    segs = service.endpoints[0].split(":")
                    ray_node_ip_address = segs[0]
                    ray_gcs_port = int(segs[1]) if len(segs) == 2 else 80
```

### 7.6.3 sf.init 与多方通信建立

调用链：

```text
main()
  → comp_eval(sf_node_eval_param, storage_config, sf_cluster_config)
```

**文件**：`secretflow/secretflow/component/core/entry.py`

```python
def comp_eval(param, storage_config, cluster_config, ...):
    if cluster_config is not None:
        setup_sf_cluster(cluster_config)
    ...
    comp.evaluate(ctx)
    ...
    shutdown(...)
```

`setup_sf_cluster`：

```python
def setup_sf_cluster(config: SFClusterConfig):
    cluster_config = {
        "parties": {},
        "self_party": config.private_config.self_party,
    }
    for party, addr in zip(
        list(config.public_config.ray_fed_config.parties),
        list(config.public_config.ray_fed_config.addresses),
    ):
        if cross_silo_comm_backend == "brpc_link":
            addr += ":80" if len(addr.split(":")) < 2 else ""
            cluster_config["parties"][party] = {
                "address": f"http://{addr}",
                "listen_addr": f"0.0.0.0:{addr.split(':')[1]}",
            }
        else:
            cluster_config["parties"][party] = {"address": addr}

    init(
        address=config.private_config.ray_head_addr,
        num_cpus=32,
        cluster_config=cluster_config,
        cross_silo_comm_backend=cross_silo_comm_backend,
        cross_silo_comm_options=cross_silo_comm_options,
        enable_waiting_for_other_parties_ready=True,
        ray_mode=False,
        ...
    )
```

`sf.init`：`secretflow/secretflow/device/driver.py`

```python
def init(parties=None, ray_mode=True, address=None, cluster_config=None, ...):
    _init_global_state(...)
    if ray_mode:
        sfd.init(DISTRIBUTION_MODE.RAY_PRODUCTION, address=address,
                 cluster_config=cluster_config, ...)
    else:
        sfd.init(DISTRIBUTION_MODE.PRODUCTION, cluster_config=cluster_config, ...)
```

**SPU 初始化**：`secretflow/secretflow/device/device/spu.py`

```python
class SPU(Device):
    def __init__(self, cluster_def: Dict, link_desc: Dict = None, ...):
        self.cluster_def = cluster_def
        self.link_desc = link_desc
        self.conf = spu.RuntimeConfig()
        self.conf.ParseFromJsonString(json.dumps(cluster_def["runtime_config"]))
        self.init()

    def init(self):
        for rank, node in enumerate(self.cluster_def["nodes"]):
            self.actors[node["party"]] = (
                sfd.remote(SPURuntime).party(node["party"])
                .remote(rank, self.cluster_def, self.link_desc, ...)
            )
```

每个 `SPURuntime` 使用节点地址创建 BRPC link：

```python
class SPURuntime:
    def __init__(self, rank, cluster_def, link_desc, ...):
        desc = spu.link.Desc()
        for i, node in enumerate(cluster_def["nodes"]):
            address = node["address"]
            if i == rank and node.get("listen_address", ""):
                address = node["listen_address"]
            desc.add_party(node["party"], address)
        _fill_link_desc_attrs(link_desc=link_desc, desc=desc)
        self.link = spu.link.create_brpc(desc, rank)
        self.runtime = spu.Runtime(self.link, self.conf)
```

### 7.6.4 组件执行

`comp_eval` 初始化集群后：

1. 按 `comp_id` 查找组件。
2. 解析参数。
3. 创建 `Context`。
4. 调用 `comp.evaluate(ctx)`。

**PSI 示例**：`secretflow/secretflow/component/preprocessing/data_prep/psi.py`

```python
@register(domain="data_prep", version="1.0.0", name="psi")
class PSI(Component):
    def evaluate(self, ctx: Context):
        tbl1 = VTable.from_distdata(self.input_ds1).get_party(0)
        tbl2 = VTable.from_distdata(self.input_ds2).get_party(0)
        ...
        spu = ctx.make_spu()
        psi_res = spu.psi(
            keys=keys,
            input_path=input_paths,
            output_path=output_paths,
            receiver=receiver_party,
            broadcast_result=broadcast_result,
            protocol=protocol,
            ecdh_curve=ecdh_curve,
            ...
        )
        self.output_ds.data = output.to_distdata()
```

**联邦学习示例**：`secretflow/secretflow/component/ml/boost/sgb_train.py`

```python
@register(domain="ml.train", version="1.1.0", name="sgb_train")
class SGBTrain(Component):
    def evaluate(self, ctx: Context):
        y = ctx.load_table(tbl_y).to_pandas()
        x = ctx.load_table(tbl_x).to_pandas()
        label_party = next(iter(y.partitions.keys())).party
        heu_evaluators = [p.party for p in x.partitions if p.party != label_party]
        heu = ctx.make_heu(label_party, heu_evaluators)
        pyus = {p: PYU(p) for p in ctx.parties}
        sgb = Sgb(heu)
        model = sgb.train(params={...}, dtrain=x, label=y, ...)
        ctx.dump_to(model_db, self.output_model)
```

高层流程：

```text
main() → comp_eval() → setup_sf_cluster() → sf.init
  → <Component>.evaluate(ctx)
      → ctx.load_table(...) / ctx.make_spu() / ctx.make_heu()
      → 算法-specific train / predict / psi
      → ctx.dump_to(...)
  → shutdown
```

---

## 7.7 DataMesh 在任务执行中的数据读写

### 7.7.1 SecretFlow 读取 DomainData

入口有两条路径：

| 路径 | 文件 | 函数 |
| ------ | ------ | ------ |
| Kuscia 任务入口 | `secretflow/secretflow/kuscia/entry.py` | `domaindata_id_to_dist_data()` → `get_file_from_dp()` |
| 组件连接器 | `secretflow/secretflow/component/io/data_source.py` | `DataSource.evaluate()` → `new_connector("datamesh")` → `conn.download_table()` |
| 连接器实现 | `secretflow/secretflow/component/core/connector/datamesh.py` | `DataMesh.download_table()` |

实际 Flight 下载：`secretflow/secretflow/kuscia/datamesh.py`

```python
def get_file_from_dp(dm_flight_client, domain_data_id, output_file_path,
                     file_format, partition_spec=""):
    download_info = DownloadInfo(domaindata_id=domain_data_id,
                                 partition_spec=partition_spec)
    dm_flight_client.download_file(download_info, output_file_path, file_format)
```

元数据查询：

```python
def get_domain_data(stub: DomainDataServiceStub, id: str) -> DomainData:
    ret = stub.QueryDomainData(QueryDomainDataRequest(domaindata_id=id))
```

### 7.7.2 Flight 命令构造

SecretFlow 使用 C++ `DataProxyFileAdapter` 打包命令。Python 参考实现（`kuscia/python/kuscia/datamesh/dataproxy.py`）：

**读取**：

```python
if file_format == FileFormat.CSV:
    domain_data_query = CommandDomainDataQuery(
        domaindata_id=domain_data_id,
        content_type=ContentType.Table,
    )
elif file_format == FileFormat.BINARY:
    domain_data_query = CommandDomainDataQuery(
        domaindata_id=domain_data_id,
        content_type=ContentType.RAW,
    )
any_msg = Any()
any_msg.Pack(domain_data_query)
descriptor = flight.FlightDescriptor.for_command(any_msg.SerializeToString())
flight_info = dm_flight_client.get_flight_info(descriptor=descriptor, ...)
ticket = flight_info.endpoints[0].ticket
reader = dm_flight_client.do_get(ticket=ticket).to_reader()
```

**写入**：

```python
command_domain_data_update = CommandDomainDataUpdate(
    domaindata_id=domaindata_id,
    file_write_options=FileWriteOptions(
        csv_options=CSVWriteOptions(field_delimiter=",")
    ),
)
any_msg.Pack(command_domain_data_update)
descriptor = flight.FlightDescriptor.for_command(any_msg.SerializeToString())
flight_info = dm_flight_client.get_flight_info(descriptor=descriptor, ...)
descriptor = flight.FlightDescriptor.for_command(ticket.ticket)
flight_writer, _ = dm_flight_client.do_put(descriptor=descriptor, schema=schema)
```

协议流程：

1. `GetFlightInfo` 携带 `CommandDomainDataQuery` / `CommandDomainDataUpdate`（包装在 `google.protobuf.Any` 中）。
2. Server 返回 `FlightInfo`，其中 `ticket` 是一个 UUID。
3. Client 使用 `DoGet(ticket)` 读数据，或使用 `DoPut(ticket-as-descriptor)` 写数据。

### 7.7.3 DataMesh Flight 处理

**Flight Server 注册**：`kuscia/pkg/datamesh/bean/grpc_server_bean.go`

```go
flight.RegisterFlightServiceServer(server,
    handler.NewDataMeshFlightHandler(domainDataService, datasourceService, s.config.DataProxyList))
```

**Handler 入口**：`kuscia/pkg/datamesh/dataserver/handler/handler.go`

```go
func (f *datameshFlightHandler) GetFlightInfo(...)
func (f *datameshFlightHandler) DoAction(...)
func (f *datameshFlightHandler) DoGet(tkt, fs)
func (f *datameshFlightHandler) DoPut(stream)
```

`DoGet`/`DoPut` 直接委托给 `FlightIO` service：

```go
func (f *datameshFlightHandler) DoGet(tkt *flight.Ticket, fs flight.FlightService_DoGetServer) error {
    return f.flightService.DoGet(tkt, fs)
}
func (f *datameshFlightHandler) DoPut(stream flight.FlightService_DoPutServer) error {
    return f.flightService.DoPut(stream)
}
```

`DoAction` 按 action type 分发给自定义处理器：

```go
if cah, ok := f.customHandles[action.Type]; ok {
    result, err := cah(context.Background(), action.GetBody())
    return stream.Send(result)
}
```

自定义 action 在 `NewDataMeshFlightHandler` 中注册：

- `ActionCreateDomainDataRequest`
- `ActionQueryDomainDataRequest`
- `ActionUpdateDomainDataRequest`
- `ActionDeleteDomainDataRequest`
- `ActionQueryDomainDataSourceRequest`

实现位于 `kuscia/pkg/datamesh/dataserver/service/action.go`。

### 7.7.4 存储后端 IO

`FlightIO.GetFlightInfo`（`pkg/datamesh/dataserver/service/flight_io.go`）：

```go
func (dp *FlightIO) GetFlightInfo(ctx context.Context, msg proto.Message) (*flight.FlightInfo, error) {
    reqCtx, err := utils.NewDataMeshRequestContext(dp.dd, dp.ds, msg)
    ...
    if dpX, ok := dp.ioMap[reqCtx.DataSourceType]; ok {
        return dpX.GetFlightInfo(ctx, reqCtx)
    }
}
```

`NewDataMeshRequestContext` 解析命令并查询 CR：

```go
func NewDataMeshRequestContext(dd, ds, msg, dsType...) (*DataMeshRequestContext, error) {
    ...
    dds, err := info.GetDomainDataSource(context.Background())
    info.DataSourceType = dds.GetType()
}

func (rc *DataMeshRequestContext) GetDomainData(ctx context.Context) (*datamesh.DomainData, error) {
    domainDataReq := &datamesh.QueryDomainDataRequest{DomaindataId: rc.getDomainDataID()}
    domainDataResp := rc.domainDataService.QueryDomainData(ctx, domainDataReq)
    return domainDataResp.Data, nil
}

func (rc *DataMeshRequestContext) GetDomainDataAndSource(ctx context.Context) (*datamesh.DomainData, *datamesh.DomainDataSource, error) {
    data, _ := rc.GetDomainData(ctx)
    datasourceReq := &datamesh.QueryDomainDataSourceRequest{DatasourceId: data.DatasourceId}
    datasourceResp := rc.domainDataSourceService.QueryDomainDataSource(ctx, datasourceReq)
    return data, datasourceResp.GetData(), nil
}
```

K8s CR 查询：

- `pkg/datamesh/metaserver/service/domaindata.go`：通过 `KusciaClient.KusciaV1alpha1().DomainDatas(ns).Get(...)` 查询 `DomainData`。
- `pkg/datamesh/metaserver/service/domaindatasource.go`：查询 `DomainDataSource`，并用 domain 私钥解密加密的 `DataSourceInfo`。

**IO 工厂分发**：`pkg/datamesh/dataserver/service/flight_io.go`

```go
func NewFlightIO(dd, ds, configs []config.DataProxyConfig) *FlightIO {
    inIO := io.NewBuiltinIO()
    fs := FlightIO{
        dd: dd, ds: ds,
        ioMap: map[string]io.Server{
            common.DomainDataSourceTypeLocalFS:    inIO,
            common.DomainDataSourceTypeOSS:        inIO,
            common.DomainDataSourceTypeMysql:      inIO,
            common.DomainDataSourceTypePostgreSQL: inIO,
        },
        inIO: inIO,
    }
    for _, conf := range configs {
        exDp := io.NewExternalIO(&conf)
        for _, typ := range conf.DataSourceTypes {
            fs.ioMap[typ] = exDp
        }
    }
    return &fs
}
```

内置 IO 工厂：`pkg/datamesh/dataserver/io/builtin/builtin.go`

```go
func NewIOServer() *IOServer {
    return &IOServer{
        cmds: gocache.New(10*time.Minute, time.Minute),
        ioChannels: map[string]DataMeshDataIOInterface{
            common.DomainDataSourceTypeLocalFS:    NewBuiltinLocalFileIOChannel(),
            common.DomainDataSourceTypeOSS:        NewBuiltinOssIOChannel(),
            common.DomainDataSourceTypeMysql:      NewBuiltinMySQLIOChannel(),
            common.DomainDataSourceTypePostgreSQL: NewBuiltinPostgresqlIOChannel(),
        },
    }
}
```

`DoGet`/`DoPut` 按 `reqCtx.DataSourceType` 分发：

```go
if ios, ok := d.ioChannels[reqCtx.DataSourceType]; ok {
    return ios.Read(fs.Context(), reqCtx, w)
}
if ios, ok := d.ioChannels[reqCtx.DataSourceType]; ok {
    return ios.Write(stream.Context(), reqCtx, reader)
}
```

**LocalFS CSV 读取**：`pkg/datamesh/dataserver/io/builtin/dataio_localfile.go`

```go
func (fio *BuiltinLocalFileIO) Read(ctx context.Context, rc *utils.DataMeshRequestContext, w utils.RecordWriter) error {
    data, ds, err := rc.GetDomainDataAndSource(ctx)
    if !isValidRelativePath(data.RelativeUri) {
        return errors.Errorf("invalid relative path: %s", data.RelativeUri)
    }
    filePath := path.Join(ds.Info.Localfs.Path, data.RelativeUri)
    file, err := os.Open(filePath)
    ...
    switch rc.GetTransferContentType() {
    case datamesh.ContentType_RAW:
        return DataProxyContentToFlightStreamBinary(data, file, w, fio.batchReadSize)
    case datamesh.ContentType_CSV, datamesh.ContentType_Table:
        return DataProxyContentToFlightStreamCSV(data, file, w)
    }
}
```

CSV → Arrow Flight 转换：`pkg/datamesh/dataserver/io/builtin/dataio.go`

```go
func DataProxyContentToFlightStreamCSV(data *datamesh.DomainData, r io.Reader, w utils.RecordWriter) error {
    colTypes, _ := utils.GenerateArrowColumnType(data)
    colNames := utils.GenerateArrowColumnNames(data)
    schema, _ := utils.GenerateArrowSchema(data)

    csvReader := csv.NewInferringReader(r,
        csv.WithColumnTypes(colTypes),
        csv.WithHeader(true),
        csv.WithNullReader(true, CSVDefaultNullValues...),
        csv.WithChunk(1024),
        csv.WithIncludeColumns(colNames),
    )
    for csvReader.Next() {
        record := csvReader.Record()
        if err := w.Write(record); err != nil { ... }
    }
    return csvReader.Err()
}
```

`w` 是配置好 Arrow schema 的 `flight.NewRecordWriter`。

### 7.7.5 输出写入与 DomainData 创建

**SecretFlow 侧输出处理**：`secretflow/secretflow/kuscia/entry.py`

```python
def postprocess_sf_node_eval_result(...):
    for domain_data_id, dist_data, output_uri, output_partition in zip(...):
        domain_data = convert_dist_data_to_domain_data(
            domain_data_id, datasource.datasource_id, dist_data,
            output_uri, party, output_partition
        )
        create_domain_data_in_dm(domaindata_stub, domain_data)
        if not datasource.access_directly:
            upload_dist_data_to_dp(..., domain_data, dist_data)
```

创建 DomainData CR：`secretflow/secretflow/kuscia/datamesh.py`

```python
def create_domain_data_in_dm(stub, data: DomainData):
    ret = stub.CreateDomainData(CreateDomainDataRequest(...))
```

上传字节：`secretflow/secretflow/kuscia/datamesh.py`

```python
def put_file_to_dp(dm_flight_client, domaindata_id, file_local_path,
                   file_format, data: DomainData):
    upload_info = UploadInfo(...)
    dm_flight_client.upload_file(upload_info, file_local_path, file_format)
```

**DataMesh 侧创建 DomainData**：`kuscia/pkg/datamesh/metaserver/service/domaindata.go`

```go
func (s domainDataService) CreateDomainData(ctx, request) *datamesh.CreateDomainDataResponse {
    kusciaDomainData := &v1alpha1.DomainData{
        ObjectMeta: metav1.ObjectMeta{Name: request.DomaindataId, ...},
        Spec: v1alpha1.DomainDataSpec{
            RelativeURI: request.RelativeUri,
            DataSource:  request.DatasourceId,
            ...
        },
    }
    _, err = s.conf.KusciaClient.KusciaV1alpha1().
        DomainDatas(s.conf.KubeNamespace).Create(ctx, kusciaDomainData, metav1.CreateOptions{})
}
```

**DataMesh 侧写入字节**：`pkg/datamesh/dataserver/io/builtin/builtin.go`

```go
func (d *IOServer) DoPut(stream flight.FlightService_DoPutServer) error {
    reader, err := flight.NewRecordReader(stream)
    desc := reader.LatestFlightDescriptor()
    ticketID := string(desc.Cmd)
    reqContext, _ := d.cmds.Get(ticketID)
    reqCtx := reqContext.(*utils.DataMeshRequestContext)
    return ios.Write(stream.Context(), reqCtx, reader)
}
```

LocalFS 写：`pkg/datamesh/dataserver/io/builtin/dataio_localfile.go`

```go
func (fio *BuiltinLocalFileIO) Write(ctx context.Context, rc *utils.DataMeshRequestContext, reader *flight.Reader) error {
    data, ds, err := rc.GetDomainDataAndSource(ctx)
    filePath := path.Join(ds.Info.Localfs.Path, data.RelativeUri)
    paths.EnsurePath(path.Dir(filePath), true)
    file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
    switch rc.GetTransferContentType() {
    case datamesh.ContentType_RAW:
        return FlightStreamToDataProxyContentBinary(data, file, reader)
    case datamesh.ContentType_CSV, datamesh.ContentType_Table:
        return FlightStreamToDataProxyContentCSV(data, file, reader)
    }
}
```

CSV 写出：`pkg/datamesh/dataserver/io/builtin/dataio.go`

```go
func FlightStreamToDataProxyContentCSV(data *datamesh.DomainData, w io.Writer, reader *flight.Reader) error {
    schema, _ := utils.GenerateArrowSchema(data)
    csvWriter := csv.NewWriter(w, reader.Schema(),
        csv.WithHeader(true), csv.WithNullWriter(CSVDefaultNullValue))
    for reader.Next() {
        record := reader.Record()
        if err := csvWriter.Write(record); err != nil { ... }
    }
    return nil
}
```

### 7.7.6 跨域授权 DomainDataGrant

DataMesh dataserver 本身不执行单次读写的授权检查；跨域授权由 Kuscia 控制平面通过 `DomainDataGrant` CR 及其控制器完成。

**Grant CRD**：`kuscia/pkg/crd/apis/kuscia/v1alpha1/domaindatagrant_types.go`

```go
type DomainDataGrantSpec struct {
    Author       string
    DomainDataID string
    GrantDomain  string
    Limit        *GrantLimit
    Signature    string
}
```

**Grant 控制器**：`kuscia/pkg/controllers/domaindata/controller.go`

`syncDomainDataGrantHandler`：

1. 获取 Grant CR。
2. `doValidate(dg)` 校验。
3. 若 `dg.Spec.Author == namespace`：
   - 根据 domain 角色决定目标 namespace（partner domain 的 master domain）。
   - `ensureDomainData(dg)` 将原始 `DomainData` 复制到被授权方 namespace。
   - 在目标 namespace 创建/更新 `DomainDataGrant`。
4. `verify(dg)`：
   - 校验签名（使用 author domain 证书公钥）。
   - 检查过期时间。
   - 检查使用次数。

`ensureDomainData`：

```go
func (c *Controller) ensureDomainData(dg *v1alpha1.DomainDataGrant) error {
    ddCopy := resources.ExtractDomainDataSpec(dd)
    ddCopy.Namespace = destDomain
    ddCopy.Labels[common.LabelOwnerReferences] = dd.Name
    ddCopy.Labels[common.LabelDomainDataVendor] = common.DomainDataVendorGrant
    ddCopy.Spec.Vendor = common.DomainDataVendorGrant
    _, createErr := c.kusciaClient.KusciaV1alpha1().
        DomainDatas(destDomain).Create(c.ctx, ddCopy, metav1.CreateOptions{})
    return createErr
}
```

对于 host/member 多集群拓扑，`pkg/interconn/kuscia/hostresources/domaindatagrant.go` 会将 Grant CR 同步到 member 集群。

**效果**：一旦被授权，远端 domain 会在本地看到 vendor=`grant` 的 `DomainData` CR；远端 DataMesh 像解析本地 DomainData 一样解析它并返回字节。无效/过期/次数耗尽的 Grant 会被标记为 `GrantUnavailable`，镜像 DomainData 不会被创建/更新，从而阻止访问。

---

## 7.8 函数级完整调用链总结

将上述流程串起来，一次联邦学习任务（例如两方 PSI）的函数级调用链如下：

```text
SecretPad 前端
  modules/main-dag/toolbar.tsx (ToolbarView.exec/run)
    → modules/main-dag/graph-request-service.tsx (startRun)
    → services/secretpad/GraphController.ts (startGraph)

SecretPad 后端
  GraphController.startGraph
    → GraphServiceImpl.startGraph
      → ProjectJob.genProjectJob
      → JobChain.proceed
        → JobPersistentHandler
        → JobRenderHandler
        → JobSubmittedHandler
          → KusciaJobConverter.converter
            → renderTaskInputConfig (ComponentTools, SFClusterDesc, TaskInputConfig)
          → JobManager.createJob
            → KusciaGrpcClientAdapter.createJob
              → DynamicKusciaChannelProvider.currentStub
              → JobServiceGrpc.JobServiceBlockingStub.createJob

KusciaAPI
  grpchandler/job_handler.go CreateJob
    → service/job_service.go CreateJob
      → validateCreateJobRequest
      → authHandlerJobCreate
      → build KusciaJob CR
      → KusciaClient.KusciaV1alpha1().KusciaJobs(common.KusciaCrossDomain).Create

KusciaJob Controller
  controller.go syncHandler
    → kusciaJobDefault
    → handlerFactory.KusciaJobPhaseHandlerFor(phase).HandlePhase
      → InitializedHandler.handleInitialized
        → validateJob (kusciaJobValidate, cycle detection)
        → annotateKusciaJob
        → phase → Pending/AwaitingApproval
      → RunningHandler.handleRunning
        → setJobTaskID
        → list subTasks
        → buildJobSubTaskStatus
        → jobStatusPhaseFrom
        → readyTasksOf
        → willStartTasksOf
        → buildWillStartKusciaTask / createTaskSpec
        → create KusciaTask CRs
    → handleTaskObject (watch task → enqueue parent job)

KusciaTask Controller
  controller.go syncHandler
    → handlerFactory.GetKusciaTaskPhaseHandler(TaskPending).Handle
      → PendingHandler.Handle
        → prepareTaskResources
          → allocatePorts
          → createTaskResources
            → buildPartyKitInfos
            → generateParties / fillPartyClusterDefine
            → for each self party:
                 createResourceForParty
                   → generateConfigMap (template)
                   → generatePod (also generates kuscia-gen ConfigMap)
                   → generateServices
            → createTaskResourceGroup

TaskResourceGroup Controller + Scheduler
  → ReservingHandler (resource reservation)
  → KusciaScheduling plugins (PreFilter/Reserve/Permit/PreBind/Bind/PostBind)
  → Pod bound to node

Kuscia Agent
  PodsController.Run → syncLoop → syncLoopIteration
    → HandlePodAdditions/Updates → dispatchWork
      → podWorkers.UpdatePod → managePodLoop
        → PodsController.syncPod
          → CRIProvider.SyncPod
            → VolumeManager.MountVolumesForPod (ConfigMap → host files)
            → kubeGenericRuntimeManager.SyncPod
              → createPodSandbox
              → startContainer
                → EnsureImageExists
                → runtimeService.CreateContainer
                → runtimeService.StartContainer

SecretFlow 容器
  sh -c "python -m secretflow.kuscia.entry ./kuscia/task-config.conf"
    → secretflow.kuscia.entry.main
      → KusciaTaskConfig.from_file
      → get_sf_cluster_config
      → comp_eval
        → setup_sf_cluster → sf.init
        → Component.evaluate(ctx)
          → ctx.load_table / ctx.make_spu / spu.psi / Sgb.train

DataMesh 读写
  → get_file_from_dp / put_file_to_dp
    → dm_flight_client.get_flight_info (CommandDomainDataQuery/Update)
    → DataMesh Flight handler.GetFlightInfo / DoGet / DoPut
      → NewDataMeshRequestContext
        → QueryDomainData + QueryDomainDataSource
      → BuiltinLocalFileIO.Read / Write
        → CSV ↔ Arrow Flight
```

---

**文档版本**: v1.1  
**最后更新**: 2026-07-06  
**维护者**: Kuscia Team
