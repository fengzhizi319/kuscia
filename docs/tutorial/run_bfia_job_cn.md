# 如何运行一个互联互通银联 BFIA 协议作业

若您使用第三方算法镜像提交互联互通作业，强烈建议您检查镜像安全性。

本教程以隐语 ECDH 算法互联互通算子为示例，介绍如何通过互联互通银联 BFIA（Beijing FinTech Industry Alliance 北京金融科技产业联盟）协议运行一个包含两方任务的作业。

在本教程中，通过两个 Kuscia Autonomy 节点来模拟不同框架底座的节点。在这两个节点之间，通过互联互通银联 BFIA 协议运行一个包含两方任务的作业。

**注意：** 银联 BFIA 协议请参考：https://github.com/secretflow/InterOp

## 准备环境

### 准备运行银联 BFIA 协议的节点

部署运行银联 BFIA 协议的节点，请参考 [快速入门点对点组网模式](../getting_started/quickstart_cn.md/#点对点组网模式)。

在执行启动集群命令时，需要新增一个命令行参数`-p bfia`，详细命令如下：

```shell
# Start the cluster, which will launch two docker containers, representing the Autonomy nodes alice and bob.
./kuscia.sh p2p -P bfia -a none
```

### 应用 AppImage 配置

将 AppImage 配置文件应用到 Kuscia 集群中，该配置定义了隐语 ECDH 算法互联互通的镜像和部署模板。配置文件在容器的 `/home/kuscia/scripts/user/bfia/ic-ecdh.yaml`。

1. 在 Alice 节点应用配置

    ```shell
    docker exec -it ${USER}-kuscia-autonomy-alice kubectl apply -f /home/kuscia/scripts/user/bfia/ic-ecdh.yaml
    ```

2. 在 Bob 节点应用配置

    ```shell
    docker exec -it ${USER}-kuscia-autonomy-bob kubectl apply -f /home/kuscia/scripts/user/bfia/ic-ecdh.yaml
    ```

### 准备数据

Kuscia 容器中已经包含了示例数据文件，需要将数据文件复制到数据存储目录。

#### 复制示例数据文件

将容器中的示例数据文件复制到数据存储目录：

1. 在 Alice 节点复制数据文件

    ```shell
    docker exec -it ${USER}-kuscia-autonomy-alice cp /home/kuscia/scripts/user/bfia/breast_hetero_guest.csv /home/kuscia/var/storage/data/
    docker exec -it ${USER}-kuscia-autonomy-alice cp /home/kuscia/scripts/user/bfia/breast_hetero_host.csv /home/kuscia/var/storage/data/
    ```

2. 在 Bob 节点复制数据文件

    ```shell
    docker exec -it ${USER}-kuscia-autonomy-bob cp /home/kuscia/scripts/user/bfia/breast_hetero_guest.csv /home/kuscia/var/storage/data/
    docker exec -it ${USER}-kuscia-autonomy-bob cp /home/kuscia/scripts/user/bfia/breast_hetero_host.csv /home/kuscia/var/storage/data/
    ```

### 查看 Kuscia 示例数据

#### 查看 Alice 节点示例数据

```shell
docker exec -it ${USER}-kuscia-autonomy-alice more /home/kuscia/var/storage/data/breast_hetero_guest.csv
docker exec -it ${USER}-kuscia-autonomy-alice more /home/kuscia/var/storage/data/breast_hetero_host.csv
```

#### 查看 Bob 节点示例数据

```shell
docker exec -it ${USER}-kuscia-autonomy-bob more /home/kuscia/var/storage/data/breast_hetero_guest.csv
docker exec -it ${USER}-kuscia-autonomy-bob more /home/kuscia/var/storage/data/breast_hetero_host.csv
```

### 准备您自己的数据

您也可以使用您自己的数据文件，首先您要将数据文件复制到节点容器中，以 Alice 节点为例：

```shell
docker cp {your_alice_data} ${USER}-kuscia-autonomy-alice:/home/kuscia/var/storage/data/
```

接下来您可以像[查看 Kuscia 示例数据](#kuscia) 一样查看您的数据文件，这里不再赘述。

## 提交一个银联 BFIA 协议的作业

目前在 Kuscia 中有两种方式提交银联 BFIA 协议的作业

- 通过配置 KusciaJob 提交作业
- 通过银联 BFIA 协议创建作业 API 接口提交作业

{#configure-bfia-kuscia-job}

### 通过配置 KusciaJob 提交作业

数据准备好之后，我们将 alice 作为任务发起方，进入 alice 节点容器中，然后配置和运行作业。

```shell
docker exec -it ${USER}-kuscia-autonomy-alice bash
```

#### 使用 Kuscia 示例数据配置 KusciaJob

下面的示例展示了一个 KusciaJob， 该作业包含 1 个任务

- 算子通过读取 alice 和 bob 的数据文件，完成 ECDH PSI（隐私集合求交）任务。

- KusciaJob 的名称为 job-ic-ecdh，在一个 Kuscia 集群中，这个名称必须是唯一的，由 `.metadata.name` 指定。

在 Alice 容器中，创建文件 job-ic-ecdh.yaml，内容如下：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  name: job-ic-ecdh
  namespace: cross-domain
spec:
  initiator: alice
  tasks:
  - alias: ic_psi_ecdh_1
    appImage: ic-ecdh
    parties:
    - domainID: alice
      role: guest
    - domainID: bob
      role: host
    taskInputConfig: '{"name":"ic_psi_ecdh_1","module_name":"ic-ecdh","output":[{"type":"dataset","key":"data"},{"type":"report","key":"summary"}],"role":{"host":["bob"],"guest":["alice"]},"initiator":{"role":"guest","node_id":"alice"},"task_params":{"host":{"0":{"rank":1,"field_names":"id","name":"breast_hetero_host.csv","namespace":"data"}},"guest":{"0":{"namespace":"data","name":"breast_hetero_guest.csv","rank":0,"field_names":"id"}},"common":{"result_to_rank":-1,"algo":"ecdh_psi","protocol_families":"ecc","curve_type":"curve25519","hash_type":"sha_256","hash2curve_strategy":"direct_hash_as_point_x","point_octet_format":"uncompressed","bit_length_after_truncated":-1}}}'
    tolerable: false
```

#### 算子参数描述

KusciaJob 中算子参数由 `taskInputConfig` 字段定义，对于不同的算子，算子的参数不同

- ECDH PSI 算子相关信息可参考 https://github.com/secretflow/InterOp
- 本教程 ECDH PSI 算子对应的 KusciaJob TaskInputConfig 结构可参考 [TaskInputConfig 结构示例](#ic-ecdh-task-input-config)

#### 提交 KusciaJob

现在已经配置好了一个 KusciaJob，接下来，让运行以下命令提交这个 KusciaJob。

```shell
kubectl apply -f job-ic-ecdh.yaml
```

### 通过银联 BFIA 协议 API 接口提交作业

数据准备好之后，将 Alice 作为任务发起方，进入 Alice 节点容器中。

```shell
docker exec -it ${USER}-kuscia-autonomy-alice bash
```

下面使用银联 BFIA 协议创建作业接口提交作业，该作业会提交给 Kuscia 互联互通 InterConn 控制器，该控制器将银联 BFIA 协议规定的创建作业请求参数转化为 Kuscia 中的 KusciaJob 作业定义。
最后，InterConn 控制器在 Kuscia 中创建 KusciaJob 资源。

```shell
curl -X POST 'http://127.0.0.1:8084/v1/interconn/schedule/job/create' \
--header 'Content-Type: application/json' \
-d '{
  "job_id": "job-ic-ecdh",
  "dag": {
    "version": "1.0.0",
    "components": [{
      "code": "ic-ecdh",
      "name": "ic_psi_ecdh_1",
      "module_name": "ic-ecdh",
      "componentName": "ic-ecdh",
      "provider": "morse",
      "version": "1.0.0",
      "input": [],
      "output": [
        {"type": "dataset", "key": "data"},
        {"type": "report", "key": "summary"}
      ]
    }]
  },
  "config": {
    "role": {
      "host": ["bob"],
      "guest": ["alice"]
    },
    "initiator": {
      "role": "guest",
      "node_id": "alice"
    },
    "job_params": {
      "common": {"sync_type": "poll"},
      "guest": {"0": {"resources": {"cpu": -1, "memory": -1, "disk": -1}}},
      "host": {"0": {"resources": {"cpu": -1, "memory": -1, "disk": -1}}},
      "arbiter": {}
    },
    "task_params": {
      "host": {
        "0": {
          "ic_psi_ecdh_1": {
            "rank": 1,
            "field_names": "id",
            "name": "breast_hetero_host.csv",
            "namespace": "data"
          }
        }
      },
      "arbiter": {},
      "guest": {
        "0": {
          "ic_psi_ecdh_1": {
            "namespace": "data",
            "name": "breast_hetero_guest.csv",
            "rank": 0,
            "field_names": "id"
          }
        }
      },
      "common": {
        "ic_psi_ecdh_1": {
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
    },
    "version": "1.0.0"
  }
}'
```

提交作业接口请求参数内容结构请参考 [提交 ECDH PSI 作业接口请求内容示例](#bfia-create-job-req-body)。

{#get-kuscia-job-phase}

## 查看 KusciaJob 运行状态

在提交完 KusciaJob 作业后，我们可以在 alice 容器中通过下面的命令查看 Alice 方的 KusciaJob 的运行情况。
同样，也可以登陆到 bob 容器中查看 Bob 方的 KusciaJob 的运行情况。下面以 Alice 节点容器为例。

### 查看所有的 KusciaJob

```shell
kubectl get kj -n cross-domain
```

您可以看到如下输出：

```shell
NAME            STARTTIME   COMPLETIONTIME   LASTRECONCILETIME   PHASE
job-ic-ecdh     3s                           3s                  Running
```

> job-ic-ecdh  就是刚刚创建出来的 KusciaJob。

### 查看运行中的 KusciaJob 的详细状态

通过指定 `-o yaml` 参数，能够以 Yaml 的形式看到 KusciaJob 的详细状态。job-ic-ecdh 是提交的作业名称。

```shell
kubectl get kj job-ic-ecdh -n cross-domain -o yaml
```

如果任务成功了，您可以看到如下输出：

```yaml
apiVersion: kuscia.secretflow/v1alpha1
kind: KusciaJob
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"kuscia.secretflow/v1alpha1","kind":"KusciaJob","metadata":{"annotations":{},"name":"job-ic-ecdh","namespace":"cross-domain"},"spec":{"initiator":"alice","tasks":[{"alias":"ic_psi_ecdh_1","appImage":"ic-ecdh","parties":[{"domainID":"alice","role":"guest"},{"domainID":"bob","role":"host"}],"taskInputConfig":"{\"name\":\"ic_psi_ecdh_1\",\"module_name\":\"ic-ecdh\",\"output\":[{\"type\":\"dataset\",\"key\":\"data\"},{\"type\":\"report\",\"key\":\"summary\"}],\"role\":{\"host\":[\"bob\"],\"guest\":[\"alice\"]},\"initiator\":{\"role\":\"guest\",\"node_id\":\"alice\"},\"task_params\":{\"host\":{\"0\":{\"rank\":1,\"field_names\":\"id\",\"name\":\"breast_hetero_host.csv\",\"namespace\":\"data\"}},\"guest\":{\"0\":{\"namespace\":\"data\",\"name\":\"breast_hetero_guest.csv\",\"rank\":0,\"field_names\":\"id\"}},\"common\":{\"result_to_rank\":-1,\"algo\":\"ecdh_psi\",\"protocol_families\":\"ecc\",\"curve_type\":\"curve25519\",\"hash_type\":\"sha_256\",\"hash2curve_strategy\":\"direct_hash_as_point_x\",\"point_octet_format\":\"uncompressed\",\"bit_length_after_truncated\":-1}}}","tolerable":false}]}}
    kuscia.secretflow/initiator: alice
    kuscia.secretflow/interconn-bfia-parties: bob
    kuscia.secretflow/interconn-self-parties: alice
    kuscia.secretflow/self-cluster-as-initiator: "true"
  creationTimestamp: "2026-02-26T02:26:32Z"
  generation: 2
  labels:
    kuscia.secretflow/interconn-protocol-type: bfia
    kuscia.secretflow/job-stage: Start
    kuscia.secretflow/job-stage-trigger: alice
    kuscia.secretflow/job-stage-version: "1"
  name: job-ic-ecdh
  namespace: cross-domain
  resourceVersion: "2998"
  uid: 084c4ada-4458-4745-9bbf-bf2b87edacaa
spec:
  initiator: alice
  maxParallelism: 1
  scheduleMode: Strict
  tasks:
  - alias: ic_psi_ecdh_1
    appImage: ic-ecdh
    parties:
    - domainID: alice
      role: guest
    - domainID: bob
      role: host
    taskID: job-ic-ecdh-e8f1df611d09
    taskInputConfig: '{"name":"ic_psi_ecdh_1","module_name":"ic-ecdh","output":[{"type":"dataset","key":"data"},{"type":"report","key":"summary"}],"role":{"host":["bob"],"guest":["alice"]},"initiator":{"role":"guest","node_id":"alice"},"task_params":{"host":{"0":{"rank":1,"field_names":"id","name":"breast_hetero_host.csv","namespace":"data"}},"guest":{"0":{"namespace":"data","name":"breast_hetero_guest.csv","rank":0,"field_names":"id"}},"common":{"result_to_rank":-1,"algo":"ecdh_psi","protocol_families":"ecc","curve_type":"curve25519","hash_type":"sha_256","hash2curve_strategy":"direct_hash_as_point_x","point_octet_format":"uncompressed","bit_length_after_truncated":-1}}}'
    tolerable: false
status:
  completionTime: "2026-02-26T02:32:52Z"
  conditions:
  - lastTransitionTime: "2026-02-26T02:26:32Z"
    status: "True"
    type: JobValidated
  lastReconcileTime: "2026-02-26T02:32:52Z"
  phase: Succeeded
  stageStatus:
    alice: JobStartStageSucceeded
    bob: JobStartStageSucceeded
  startTime: "2026-02-26T02:26:32Z"
  taskStatus:
    job-ic-ecdh-e8f1df611d09: Succeeded
```

- `status` 字段记录了 KusciaJob 的运行状态，`.status.phase` 字段描述了 KusciaJob 的整体状态，而 `.status.taskStatus` 则描述了包含的 KusciaTask 的状态。
详细信息请参考 [KusciaJob](../reference/concepts/kusciajob_cn.md)。

### 查看 KusciaJob 中 KusciaTask 的详细状态

KusciaJob 中的每一个 KusciaTask 都有一个 `taskID`，通过 `taskID` 我们可以查看 KusciaTask 的详细状态。

```shell
kubectl get kt job-ic-ecdh-{random-id} -n cross-domain -o yaml
```

KusciaTask 的介绍，请参考 [KusciaTask](../reference/concepts/kusciatask_cn.md)。

## 查看 ECDH PSI 算子运行结果

可以通过 [查看 KusciaJob 运行状态](#get-kuscia-job-phase) 查询作业的运行状态。 当作业状态 PHASE 变成 `Succeeded` 时，可以查看算子输出结果。

1. 进入节点 Alice 或 Bob 容器
    若已经在容器中，跳过该步骤

    ```shell
    # Enter the alice node container
    docker exec -it ${USER}-kuscia-autonomy-alice bash

    # Enter the bob node container
    docker exec -it ${USER}-kuscia-autonomy-bob bash
    ```

2. 查看 KusciaJob 作业状态

    ```shell
    kubectl get kj job-ic-ecdh -n cross-domain
    NAME            STARTTIME   COMPLETIONTIME   LASTRECONCILETIME   PHASE
    job-ic-ecdh     13s         2s               2s                  Succeeded
    ```

3. 查看 ECDH PSI 算子输出结果

    输出内容表示 ECDH PSI 算子的求交结果和统计报告

    ```shell
    # View the output result in the alice container
    more /home/kuscia/var/storage/job-ic-ecdh-guest-0/{kt-name}-data

    # View the output result in the bob container
    more /home/kuscia/var/storage/job-ic-ecdh-host-0/{kt-name}-data
    ```

## 删除 KusciaJob

当您想清理这个 KusciaJob 时，您可以通过下面的命令完成：

```shell
kubectl delete kj job-ic-ecdh -n cross-domain
```

当这个 KusciaJob 被清理时， 这个 KusciaJob 创建的 KusciaTask 也会一起被清理。

## 参考

{#ic-ecdh-task-input-config}

### ECDH PSI 算子对应的 TaskInputConfig 结构示例

```json
{
  "name": "ic_psi_ecdh_1",
  "module_name": "ic-ecdh",
  "input": [],
  "output": [
    {
      "type": "dataset",
      "key": "data"
    },
    {
      "type": "report",
      "key": "summary"
    }
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

字段说明

- `name` 描述了任务算子的名称。
- `module_name` 描述了任务算子所属模块名称。
- `input` 描述了任务算子的输入，若任务不依赖其他任务的输出，则可以将该项置为空。
- `output` 描述了任务算子的输出。
- `role` 描述了任务的角色。
- `initiator` 描述了任务发起方的信息。
- `task_params` 描述了任务算子依赖的参数。

{#bfia-create-job-req-body}

### 提交 ECDH PSI 作业接口请求内容示例

```json
{
  "job_id": "job-ic-ecdh",
  "dag": {
    "version": "1.0.0",
    "components": [{
      "code": "ic-ecdh",
      "name": "ic_psi_ecdh_1",
      "module_name": "ic-ecdh",
      "componentName": "ic-ecdh",
      "provider": "morse",
      "version": "1.0.0",
      "input": [],
      "output": [
        {
          "type": "dataset",
          "key": "data"
        },
        {
          "type": "report",
          "key": "summary"
        }
      ]
    }]
  },
  "config": {
    "role": {
      "host": ["bob"],
      "guest": ["alice"]
    },
    "initiator": {
      "role": "guest",
      "node_id": "alice"
    },
    "job_params": {
      "common": {
        "sync_type": "poll"
      },
      "guest": {
        "0": {
          "resources": {
            "cpu": -1,
            "memory": -1,
            "disk": -1
          }
        }
      },
      "host": {
        "0": {
          "resources": {
            "cpu": -1,
            "memory": -1,
            "disk": -1
          }
        }
      },
      "arbiter": {}
    },
    "task_params": {
      "host": {
        "0": {
          "ic_psi_ecdh_1": {
            "rank": 1,
            "field_names": "id",
            "name": "breast_hetero_host.csv",
            "namespace": "data"
          }
        }
      },
      "arbiter": {},
      "guest": {
        "0": {
          "ic_psi_ecdh_1": {
            "namespace": "data",
            "name": "breast_hetero_guest.csv",
            "rank": 0,
            "field_names": "id"
          }
        }
      },
      "common": {
        "ic_psi_ecdh_1": {
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
    },
    "version": "1.0.0"
  }
}
```

### 字段说明

- `job_id` 描述了作业的标识。
- `dag` 描述了作业的组件之间组合的配置。
- `config` 描述了作业运行时的参数配置。
