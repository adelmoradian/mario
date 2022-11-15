# Mario

Status: pre-alpha

Command line utility for vaidating tekton pipelines.

Take the following tasks and pipeline for example… I can do `kubectl apply -f`
however my pipeline has the following issues:

* `task-c`  is used in the pipeline but does not exist in my cluster
* `task-b`  is a Task but in pipeline, the kind is ClusterTask
* `task-a` and `task-b`  both need params that are not specified in the
pipeline spec (same thing can happen with workspaces)…
* `param-z` is not used anywhere (same thing can happen with workspaces)…

The goal of this project is to help capture these issues make
refactoring/troubleshooting slightly easier.

There are tekton plugins for vscode that do similar things but it is
also nice to have this in cli format.

```yaml
kind: Pipeline
metadata:
  name: test-pipeline
spec:
  params:
    - name: param-a
    - name: param-b
    - name: param-c
    - name: param-z
  tasks:
    - name: task-a
      taskRef:
        name: task-a
      params:
        - name: param-a
          value: $(params.param-a)
    - name: task-b
      taskRef:
        kind: ClusterTask
        name: task-b
      params:
        - name: param-b
          value: $(params.param-b)
    - name: task-c
      taskRef:
        name: task-c
      params:
        - name: param-c
          value: $(params.param-c)

---

apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: task-a
spec:
  params:
    - name: param-a
    - name: param-a-missed
  steps:
    - image: ubuntu
      script: echo 'hello there'

---

apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: task-b
spec:
  params:
    - name: param-b
    - name: param-b-missed
  steps:
    - image: ubuntu
      script: echo 'hello there'
```

