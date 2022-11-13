package cmd

import (
	"errors"
	"testing"

	tknv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
)

type ValidateTaskRefsTestCases struct {
	name          string
	cTasks        []extendedTask
	cClusterTasks []extendedClusterTask
	want          error
}

type ValidateParamsTestCases struct {
	name          string
	cTasks        []extendedTask
	cClusterTasks []extendedClusterTask
	want          error
}

type ValidateWorkspacesTestCases struct {
	name          string
	cTasks        []extendedTask
	cClusterTasks []extendedClusterTask
	want          error
}

type WarningsTestCases struct {
	name          string
	cTasks        []extendedTask
	cClusterTasks []extendedClusterTask
	want          error
}

var (
	yPipeline = `---
apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  name: test-pipeline
spec:
  workspaces:
    - name: ws1
    - name: ws-no-needed
    - name: ws2
      optional: true
  params:
    - name: param1
    - name: param2
      default: "param2-default"
      type: string
    - name: param3
      type: array
      default: ["param3-1", "param3-2"]
    - name: param4
    - name: param-finally
    - name: param-not-needed
  tasks:
    - name: task-a
      taskRef:
        name: task-a
      params:
        - name: param1
          value: $(params.param1)
        - name: param2
          value: $(params.param2)
        - name: param-extra
          value: $(params.param1)-$(params.param2)
        - name: param-extra-2
          value: some-string
      workspaces:
        - name: ws-a-1
          workspace: ws1
        - name: ws-a-2
          workspace: ws2
    - name: task-b
      taskRef:
        kind: ClusterTask
        name: task-b
      workspaces:
        - name: ws-b-1
          workspace: ws1
      params:
        - name: param3
          value: ["$(params.param3)"]
    - name: task-c
      runAfter:
        - task-a
        - task-b
      params:
        - name: param4
          value: $(params.param4)
      taskSpec:
        params:
          - name: param4
        steps:
          - image: ubuntu
            script: echo 'hello there'
  finally:
  - name: task-finally
    params:
    - name: param-finally
      value: $(params.param-finally)
    taskRef:
      name: task-finally
    workspaces:
    - name: ws-finally
      workspace: ws1
`
)

// tasksRef must refer to tasks or clusterTasks that exist in cluster.
// validate tasks in spec.tasks and spec.finally
func TestValidateTaskRefs(t *testing.T) {
	tPipeline := setupPipeline(yPipeline)

	validateTaskRefsTests := []ValidateTaskRefsTestCases{
		{
			name: "pipeline is valid",
			want: nil,
			cTasks: []extendedTask{
				{ObjectMeta: metav1.ObjectMeta{Name: "task-a"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "task-finally"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "task-z"}},
			},
			cClusterTasks: []extendedClusterTask{
				{ObjectMeta: metav1.ObjectMeta{Name: "task-b"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "task-zzz"}},
			},
		},
		{
			name: "pipeline is using undeclared task 1",
			want: errors.New("The following tasks/clusterTasks are used in test-pipeline pipeline but do not exist in the cluster: [task-a]"),
			cTasks: []extendedTask{
				{ObjectMeta: metav1.ObjectMeta{Name: "task-finally"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "task-z"}},
			},
			cClusterTasks: []extendedClusterTask{
				{ObjectMeta: metav1.ObjectMeta{Name: "task-b"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "task-zzz"}},
			},
		},
		{
			name: "pipeline is using undeclared cluster task 2",
			cTasks: []extendedTask{
				{ObjectMeta: metav1.ObjectMeta{Name: "task-a"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "task-b"}},
			},
			cClusterTasks: []extendedClusterTask{
				{ObjectMeta: metav1.ObjectMeta{Name: "task-finally"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "task-zzz"}},
			},
			want: errors.New("The following tasks/clusterTasks are used in test-pipeline pipeline but do not exist in the cluster: [task-b task-finally]"),
		},
		{
			name:   "pipeline is using undeclared cluster task 3",
			cTasks: []extendedTask{},
			cClusterTasks: []extendedClusterTask{
				{ObjectMeta: metav1.ObjectMeta{Name: "task-a"}},
			},
			want: errors.New("The following tasks/clusterTasks are used in test-pipeline pipeline but do not exist in the cluster: [task-a task-b task-finally]"),
		},
	}

	for _, tc := range validateTaskRefsTests {
		t.Run(tc.name, func(t *testing.T) {
			got := tPipeline.ValidateTaskRefs(tc.cTasks, tc.cClusterTasks)
			if got == nil && tc.want != nil {
				t.Errorf("\nwanted the following error: %s\nbut did not get any error", tc.want)
			}
			if got != nil {
				if tc.want == nil {
					t.Errorf("\ndid not expect error but got the following error: %s", got)
				}
				if tc.want.Error() != got.Error() {
					t.Errorf("\ngot error: %v\nbut wanted: %v", got.Error(), tc.want)
				}
			}
		})
	}
}

// spec.tasks[*].params and spec.finally[*].params must cover all the task
// params that do not have a default value
func TestValidateParams(t *testing.T) {
	tPipeline := setupPipeline(yPipeline)

	validateParamsTests := []ValidateParamsTestCases{
		{
			name: "pipeline has all the needed params",
			want: nil,
			cTasks: []extendedTask{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-a"},
					Spec: tknv1beta1.TaskSpec{
						Params: []tknv1beta1.ParamSpec{
							{Name: "param1"},
							{Name: "param2"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-finally"},
					Spec: tknv1beta1.TaskSpec{
						Params: []tknv1beta1.ParamSpec{
							{Name: "param-finally"},
						},
					},
				},
			},
			cClusterTasks: []extendedClusterTask{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-b"},
					Spec: tknv1beta1.TaskSpec{
						Params: []tknv1beta1.ParamSpec{
							{Name: "param3"},
						},
					},
				},
			},
		},
		{
			name: "pipeline has a missing task param",
			want: errors.New("test-pipeline is missing the following params:\nmap[task-a:[param-missed-1] task-b:[param-missed-3] task-finally:[param-missed-2 param-missed-3]]"),
			cTasks: []extendedTask{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-a"},
					Spec: tknv1beta1.TaskSpec{
						Params: []tknv1beta1.ParamSpec{
							{Name: "param1"},
							{Name: "param2"},
							{Name: "param-extra"},
							{Name: "param-extra-2"},
							{Name: "param-missed-1"},
							{Name: "param-with-default", Default: &tknv1beta1.ParamValue{StringVal: "default"}},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-finally"},
					Spec: tknv1beta1.TaskSpec{
						Params: []tknv1beta1.ParamSpec{
							{Name: "param-finally"},
							{Name: "param-missed-2"},
							{Name: "param-missed-3"},
							{Name: "param-with-default-f", Default: &tknv1beta1.ParamValue{StringVal: "default"}},
						},
					},
				},
			},
			cClusterTasks: []extendedClusterTask{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-b"},
					Spec: tknv1beta1.TaskSpec{
						Params: []tknv1beta1.ParamSpec{
							{Name: "param3"},
							{Name: "param-missed-3"},
							{Name: "param-with-default-3", Default: &tknv1beta1.ParamValue{StringVal: "default-3"}},
						},
					},
				},
			},
		},
	}

	for _, tc := range validateParamsTests {
		t.Run(tc.name, func(t *testing.T) {
			got := tPipeline.ValidateParams(tc.cTasks, tc.cClusterTasks)
			assertion(t, got, tc.want)
		})
	}
}

// spec.tasks[*].workspaces and spec.finally[*].workspaces must cover all the task
// workspaces that do not have a default value.
// In this case we are ignoring optional workspaces
// because they should generate warning and not error
func TestValidateWorkspaces(t *testing.T) {
	tPipeline := setupPipeline(yPipeline)

	validateWorkspaceTests := []ValidateWorkspacesTestCases{
		{
			name: "pipeline has all the needed workspaces",
			want: nil,
			cTasks: []extendedTask{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-a"},
					Spec: tknv1beta1.TaskSpec{
						Workspaces: []tknv1beta1.WorkspaceDeclaration{
							{Name: "ws-a-1"},
							{Name: "ws-a-2"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-finally"},
					Spec: tknv1beta1.TaskSpec{
						Workspaces: []tknv1beta1.WorkspaceDeclaration{
							{Name: "ws-finally"},
						},
					},
				},
			},
			cClusterTasks: []extendedClusterTask{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-b"},
					Spec: tknv1beta1.TaskSpec{
						Workspaces: []tknv1beta1.WorkspaceDeclaration{
							{Name: "ws-b-1"},
						},
					},
				},
			},
		},
		{
			name: "pipeline has a missing workspace",
			want: errors.New("test-pipeline is missing the following workspaces:\nmap[task-a:[ws-a-missing] task-b:[ws-b-missing ws-b-missing-2] task-finally:[ws-a-missing-finally]]"),
			cTasks: []extendedTask{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-a"},
					Spec: tknv1beta1.TaskSpec{
						Workspaces: []tknv1beta1.WorkspaceDeclaration{
							{Name: "ws-a-1"},
							{Name: "ws-a-2"},
							{Name: "ws-a-missing"},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-finally"},
					Spec: tknv1beta1.TaskSpec{
						Workspaces: []tknv1beta1.WorkspaceDeclaration{
							{Name: "ws-finally"},
							{Name: "ws-a-missing-finally"},
						},
					},
				},
			},
			cClusterTasks: []extendedClusterTask{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-b"},
					Spec: tknv1beta1.TaskSpec{
						Workspaces: []tknv1beta1.WorkspaceDeclaration{
							{Name: "ws-b-1"},
							{Name: "ws-b-missing"},
							{Name: "ws-b-missing-2"},
						},
					},
				},
			},
		},
	}

	for _, tc := range validateWorkspaceTests {
		t.Run(tc.name, func(t *testing.T) {
			got := tPipeline.ValidateWorkspaces(tc.cTasks, tc.cClusterTasks)
			assertion(t, got, tc.want)
		})
	}
}

func TestWarnings(t *testing.T) {
	tPipeline := setupPipeline(yPipeline)

	warningsTests := []WarningsTestCases{
		{
			name: "pipeline has no warnings",
			want: nil,
			cTasks: []extendedTask{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-a"},
					Spec: tknv1beta1.TaskSpec{
						Params: []tknv1beta1.ParamSpec{
							{Name: "param1"},
							{Name: "param2"},
							{Name: "param-not-needed"},
						},
						Workspaces: []tknv1beta1.WorkspaceDeclaration{
							{Name: "ws-a-1"},
							{Name: "ws-no-needed"},
							{Name: "ws-a-2", Optional: true},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-finally"},
					Spec: tknv1beta1.TaskSpec{
						Params: []tknv1beta1.ParamSpec{
							{Name: "param-finally"},
						},
						Workspaces: []tknv1beta1.WorkspaceDeclaration{
							{Name: "ws-finally"},
						},
					},
				},
			},
			cClusterTasks: []extendedClusterTask{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "task-b"},
					Spec: tknv1beta1.TaskSpec{
						Params: []tknv1beta1.ParamSpec{
							{Name: "param3"},
						},
						Workspaces: []tknv1beta1.WorkspaceDeclaration{
							{Name: "ws-b-1"},
						},
					},
				},
			},
		},
	}

	for _, tc := range warningsTests {
		t.Run(tc.name, func(t *testing.T) {
			got := tPipeline.Warnings(tc.cTasks, tc.cClusterTasks)
			assertion(t, got, tc.want)
		})
	}
}

func assertion(t *testing.T, got, want error) {
	if got == nil && want != nil {
		t.Errorf("\nwanted the following error: %s\nbut did not get any error", want)
	}
	if got != nil {
		if want == nil {
			t.Errorf("\ndid not expect error but got the following error: %s", got)
		}
		if want.Error() != got.Error() {
			t.Errorf("\ngot error: %v\nbut wanted: %v", got.Error(), want)
		}
	}
}

func setupPipeline(s string) (tPipeline extendedPipeline) {
	jPipeline, err := yaml.ToJSON([]byte(s))
	if err != nil {
		panic(err.Error())
	}

	object, err := runtime.Decode(unstructured.UnstructuredJSONScheme, jPipeline)
	if err != nil {
		panic(err.Error())
	}

	uPipeline, ok := object.(*unstructured.Unstructured)
	if !ok {
		panic("unstructured.Unstructured expected")
	}

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(uPipeline.Object, &tPipeline)
	if err != nil {
		panic(err.Error())
	}

	return tPipeline
}
