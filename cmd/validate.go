/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	tknv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

type (
	extendedPipeline    tknv1beta1.Pipeline
	extendedTask        tknv1beta1.Task
	extendedClusterTask tknv1beta1.ClusterTask
	allTasks            interface {
		getName() string
		getParams() []tknv1beta1.ParamSpec
		getWorkspaces() []tknv1beta1.WorkspaceDeclaration
	}
)

func (et extendedTask) getName() string                                  { return et.GetName() }
func (et extendedTask) getParams() []tknv1beta1.ParamSpec                { return et.Spec.Params }
func (et extendedTask) getWorkspaces() []tknv1beta1.WorkspaceDeclaration { return et.Spec.Workspaces }
func (ect extendedClusterTask) getName() string                          { return ect.GetName() }
func (ect extendedClusterTask) getParams() []tknv1beta1.ParamSpec        { return ect.Spec.Params }
func (ect extendedClusterTask) getWorkspaces() []tknv1beta1.WorkspaceDeclaration {
	return ect.Spec.Workspaces
}

var (
	normal = "\033[0m"
	bold   = "\033[1m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validates the pipelines",
	Long: `Validates the pipelines that are installed on the
	cluster by ensuring the following:
	- tasksRefs that are used in pipeline, must exist in the cluster
	- task params that don't have default value must be present in pipelines
	- task workspaces that are not optional must be present in pipeline`,
	Run: func(cmd *cobra.Command, args []string) {
		var eClusterTasks []extendedClusterTask
		var eTasks []extendedTask
		client := GetDynamicClient(kubeConfig)

		pipelines, err := client.Resource(schema.GroupVersionResource{Group: "tekton.dev", Version: "v1beta1", Resource: "pipelines"}).List(context.TODO(), v1.ListOptions{})
		if err != nil {
			panic(err)
		}
		clusterTasks, err := client.Resource(schema.GroupVersionResource{Group: "tekton.dev", Version: "v1beta1", Resource: "clustertasks"}).List(context.TODO(), v1.ListOptions{})
		if err != nil {
			panic(err)
		}

		for _, t := range clusterTasks.Items {
			var eCT extendedClusterTask
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(t.Object, &eCT)
			if err != nil {
				panic(err.Error())
			}
			eClusterTasks = append(eClusterTasks, eCT)
		}

		for _, p := range pipelines.Items {
			errMap := make(map[string]error)
			ns := p.GetNamespace()
			tasks, err := client.Resource(schema.GroupVersionResource{Group: "tekton.dev", Version: "v1beta1", Resource: "tasks"}).Namespace(ns).List(context.TODO(), v1.ListOptions{})
			if err != nil {
				panic(err)
			}

			var eP extendedPipeline
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(p.Object, &eP)
			if err != nil {
				panic(err.Error())
			}

			for _, t := range tasks.Items {
				var eT extendedTask
				err = runtime.DefaultUnstructuredConverter.FromUnstructured(t.Object, &eT)
				if err != nil {
					panic(err.Error())
				}
				eTasks = append(eTasks, eT)
			}

			taskRefErr := eP.ValidateTaskRefs(eTasks, eClusterTasks)
			if taskRefErr != nil {
				errMap["taskRef validation"] = taskRefErr
			}
			paramsErr := eP.ValidateParams(eTasks, eClusterTasks)
			if paramsErr != nil {
				errMap["parameter validation"] = paramsErr
			}
			workspaceErr := eP.ValidateWorkspaces(eTasks, eClusterTasks)
			if workspaceErr != nil {
				errMap["workspace validation"] = workspaceErr
			}
			if len(errMap) == 0 {
				fmt.Println(string(green), fmt.Sprintf("%s verified!", p.GetName()), string(normal))
			} else {
				fmt.Println(string(yellow), fmt.Sprintf("%s has the following errors", p.GetName()), string(normal))
				for k, err := range errMap {
					fmt.Println(string(red), k, "error", string(normal))
					fmt.Println(err)
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(validateCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// validateCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// validateCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// Ensures that all the tasks and clusterTasks which are referred in a pipeline, exist in the cluster.
func (p *extendedPipeline) ValidateTaskRefs(cTasks []extendedTask, cClusterTasks []extendedClusterTask) error {
	var pTasksNames, pClusterTasksNames, cTasksNames, cClusterTasksNames []string

	for _, t := range allPipelineTasks(p) {
		if t.TaskRef != nil {
			if t.TaskRef.Kind == "ClusterTask" {
				pClusterTasksNames = append(pClusterTasksNames, t.TaskRef.Name)
			} else {
				pTasksNames = append(pTasksNames, t.TaskRef.Name)
			}
		}
	}

	for _, t := range cTasks {
		cTasksNames = append(cTasksNames, t.getName())
	}
	for _, t := range cClusterTasks {
		cClusterTasksNames = append(cClusterTasksNames, t.getName())
	}

	missingTasks := sliceOutliers(cTasksNames, pTasksNames)
	missingTasks = append(missingTasks, sliceOutliers(cClusterTasksNames, pClusterTasksNames)...)
	if len(missingTasks) > 0 {
		sort.Strings(missingTasks)
		return errors.New(fmt.Sprintf("The following tasks/clusterTasks are used in %s pipeline but do not exist in the cluster: %v", p.GetName(), missingTasks))
	}
	return nil
}

// Ensures that all the non-default params that pipelineTasks need are present in the spec.params of the pipeline
func (p *extendedPipeline) ValidateParams(cTasks []extendedTask, cClusterTasks []extendedClusterTask) error {
	var pParamNames []string
	pTasksWithMissingParams := make(map[string][]string)

	for _, pt := range allPipelineTasks(p) {
		for _, p := range pt.Params {
			pParamNames = append(pParamNames, p.Name)
		}
	}

	for _, pt := range allPipelineTasks(p) {
		var requiredParamsList []string
		for _, ct := range allclusterTasks(cTasks, cClusterTasks) {
			requiredParamsList = append(requiredParamsList, requiredParams(pt, ct)...)
		}
		missingParams := sliceOutliers(pParamNames, requiredParamsList)
		if len(missingParams) < 1 {
			continue
		}
		pTasksWithMissingParams[pt.Name] = missingParams
	}

	if len(pTasksWithMissingParams) != 0 {
		return errors.New(fmt.Sprintf("%s is missing the following params:\n%v", p.GetName(), pTasksWithMissingParams))
	}
	return nil
}

// Ensures that all the non-optional workspaces that pipelineTasks need are present in the spec.workspaces of the pipeline.
// It does consider the correct binding. For example if taskA needs ws-a however ws-a is bound to ws-1 in the pipelineTask,
// it expects the pipeline to have ws-a in it's spec.workspaces
func (p *extendedPipeline) ValidateWorkspaces(cTasks []extendedTask, cClusterTasks []extendedClusterTask) error {
	var pWorkspaceNames []string
	pTasksWithMissingWorkspaces := make(map[string][]string)

	for _, w := range p.Spec.Workspaces {
		pWorkspaceNames = append(pWorkspaceNames, w.Name)
	}

	for _, pt := range allPipelineTasks(p) {
		var requiredWorkspaceList []string
		for _, ct := range allclusterTasks(cTasks, cClusterTasks) {
			requiredWorkspaceList = append(requiredWorkspaceList, requiredWorkspaces(pt, ct)...)
		}
		missingWorkspaces := sliceOutliers(pWorkspaceNames, requiredWorkspaceList)
		if len(missingWorkspaces) < 1 {
			continue
		}
		pTasksWithMissingWorkspaces[pt.Name] = missingWorkspaces
	}

	if len(pTasksWithMissingWorkspaces) > 0 {
		return errors.New(fmt.Sprintf("%s is missing the following workspaces:\n%v", p.GetName(), pTasksWithMissingWorkspaces))
	}
	return nil
}

func (p *extendedPipeline) Warnings(cTasks []extendedTask, cClusterTasks []extendedClusterTask) error {
	return nil
}

// Returns all the parameters that are required by a given pipelineTask.
// It does not include parameters that have a default value. The reason is that
// spec.param of a pipeline doesn't need to have the task params that have default value.
func requiredParams(pTask tknv1beta1.PipelineTask, cTask allTasks) []string {
	var paramsThatPipelineMustHave []string
	if pTask.TaskRef != nil && cTask.getName() == pTask.TaskRef.Name {
		for _, cp := range cTask.getParams() {
			if cp.Default == nil {
				paramsThatPipelineMustHave = append(paramsThatPipelineMustHave, cp.Name)
			}
		}
	}
	return paramsThatPipelineMustHave
}

// Returns all the workspaces that are required by a given a pipelineTask.
// It does not include the task workspaces which are optional. If a pipelineTask
// does not declare the workspace, then the workspace which is declared in the
// spec.workspace of pipeline must have the same name as the task workspace.
func requiredWorkspaces(pTask tknv1beta1.PipelineTask, cTask allTasks) (workspacesThatPipelineMustHave []string) {
	if pTask.TaskRef != nil && cTask.getName() == pTask.TaskRef.Name {
		var cTaskWorkspaceNames, pTaskWorkspaceNames []string
		for _, ws := range cTask.getWorkspaces() {
			if !ws.Optional {
				cTaskWorkspaceNames = append(cTaskWorkspaceNames, ws.Name)
			}
		}

		for _, ws := range pTask.Workspaces {
			pTaskWorkspaceNames = append(pTaskWorkspaceNames, ws.Name)
		}

		// any workspace which is declared in task but not in pipelineTask must exist in spec.workspace of the pipeline
		workspacesThatPipelineMustHave = append(workspacesThatPipelineMustHave, sliceOutliers(pTaskWorkspaceNames, cTaskWorkspaceNames)...)

		// if pipelineTask is defining a workspace, check the binidng to make sure that pipeline actually has the correct ws name
		for _, w := range pTask.Workspaces {
			if w.Workspace == "" {
				workspacesThatPipelineMustHave = append(workspacesThatPipelineMustHave, w.Name)
			} else {
				workspacesThatPipelineMustHave = append(workspacesThatPipelineMustHave, w.Workspace)
			}
		}
	}
	return
}

// Returns a list of all tasks and clustertasks
func allclusterTasks(tList []extendedTask, ctList []extendedClusterTask) (tAll []allTasks) {
	for _, t := range ctList {
		tAll = append(tAll, t)
	}
	for _, t := range tList {
		tAll = append(tAll, t)
	}
	return
}

// Gets all the tasks in Spec and Finally
func allPipelineTasks(p *extendedPipeline) (pt []tknv1beta1.PipelineTask) {
	for _, t := range p.Spec.Tasks {
		pt = append(pt, t)
	}
	for _, t := range p.Spec.Finally {
		pt = append(pt, t)
	}
	return
}

// Returns items in smallSlice which do not exist in the bigSlice
func sliceOutliers(bigSlice, smallSlice []string) (outliers []string) {
	for _, s := range smallSlice {
		if !sliceIncludeString(bigSlice, s) {
			outliers = append(outliers, s)
		}
	}
	return outliers
}

// Returns true if the slice includes the given string
func sliceIncludeString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// Give the kubeconfig path, it returns a dynamic client
func GetDynamicClient(kubeconfig string) dynamic.Interface {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	return client
}
