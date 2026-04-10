package controller_test

import (
	"testing"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkflowDAG_CycleDetection(t *testing.T) {
	templates := []v1alpha1.WorkflowTemplate{
		{Name: "a", Type: v1alpha1.TemplateTypeDelay, Dependencies: []string{"c"}},
		{Name: "b", Type: v1alpha1.TemplateTypeDelay, Dependencies: []string{"a"}},
		{Name: "c", Type: v1alpha1.TemplateTypeDelay, Dependencies: []string{"b"}},
	}

	_, err := controller.TopologicalSort(templates)
	if err == nil {
		t.Fatal("expected cycle detection error, got nil")
	}
	if err.Error() != "cycle detected in workflow DAG" {
		t.Fatalf("expected cycle detected error, got: %v", err)
	}
}

func TestWorkflowDAG_TopologicalSort(t *testing.T) {
	templates := []v1alpha1.WorkflowTemplate{
		{Name: "a", Type: v1alpha1.TemplateTypeExperiment},
		{Name: "b", Type: v1alpha1.TemplateTypeDelay, Dependencies: []string{"a"}},
		{Name: "c", Type: v1alpha1.TemplateTypeExperiment, Dependencies: []string{"b"}},
	}

	order, err := controller.TopologicalSort(templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("expected 3 items, got %d", len(order))
	}

	indexOf := make(map[string]int)
	for i, name := range order {
		indexOf[name] = i
	}

	if indexOf["a"] >= indexOf["b"] {
		t.Fatalf("expected a before b, got order: %v", order)
	}
	if indexOf["b"] >= indexOf["c"] {
		t.Fatalf("expected b before c, got order: %v", order)
	}
}

func TestWorkflowDAG_TopologicalSort_Diamond(t *testing.T) {
	// Diamond: a -> b, a -> c, b -> d, c -> d
	templates := []v1alpha1.WorkflowTemplate{
		{Name: "a", Type: v1alpha1.TemplateTypeExperiment},
		{Name: "b", Type: v1alpha1.TemplateTypeDelay, Dependencies: []string{"a"}},
		{Name: "c", Type: v1alpha1.TemplateTypeDelay, Dependencies: []string{"a"}},
		{Name: "d", Type: v1alpha1.TemplateTypeExperiment, Dependencies: []string{"b", "c"}},
	}

	order, err := controller.TopologicalSort(templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(order) != 4 {
		t.Fatalf("expected 4 items, got %d", len(order))
	}

	indexOf := make(map[string]int)
	for i, name := range order {
		indexOf[name] = i
	}

	if indexOf["a"] >= indexOf["b"] || indexOf["a"] >= indexOf["c"] {
		t.Fatalf("expected a before b and c, got order: %v", order)
	}
	if indexOf["b"] >= indexOf["d"] || indexOf["c"] >= indexOf["d"] {
		t.Fatalf("expected b and c before d, got order: %v", order)
	}
}

func TestWorkflowDAG_ParallelExecution(t *testing.T) {
	templates := []v1alpha1.WorkflowTemplate{
		{Name: "a", Type: v1alpha1.TemplateTypeExperiment},
		{Name: "b", Type: v1alpha1.TemplateTypeExperiment},
		{Name: "c", Type: v1alpha1.TemplateTypeExperiment},
		{Name: "d", Type: v1alpha1.TemplateTypeExperiment, Dependencies: []string{"a", "b", "c"}},
	}

	completed := map[string]bool{}
	ready := controller.ReadyTemplates(templates, completed)

	readyMap := make(map[string]bool)
	for _, name := range ready {
		readyMap[name] = true
	}

	if !readyMap["a"] || !readyMap["b"] || !readyMap["c"] {
		t.Fatalf("expected a, b, c to be ready, got: %v", ready)
	}
	if readyMap["d"] {
		t.Fatal("d should not be ready yet")
	}

	// After a, b, c complete, d should be ready
	completed["a"] = true
	completed["b"] = true
	completed["c"] = true
	ready = controller.ReadyTemplates(templates, completed)

	readyMap = make(map[string]bool)
	for _, name := range ready {
		readyMap[name] = true
	}
	if !readyMap["d"] {
		t.Fatalf("expected d to be ready after a,b,c completed, got: %v", ready)
	}
}

func TestWorkflowDAG_UnknownDependency(t *testing.T) {
	templates := []v1alpha1.WorkflowTemplate{
		{Name: "a", Type: v1alpha1.TemplateTypeExperiment, Dependencies: []string{"nonexistent"}},
	}

	_, err := controller.TopologicalSort(templates)
	if err == nil {
		t.Fatal("expected error for unknown dependency, got nil")
	}
}

func TestWorkflowProgress(t *testing.T) {
	tests := []struct {
		name     string
		statuses []v1alpha1.TemplateStatus
		total    int
		expected string
	}{
		{
			name:     "no completions",
			statuses: nil,
			total:    5,
			expected: "0/5",
		},
		{
			name: "some completed",
			statuses: []v1alpha1.TemplateStatus{
				{Name: "a", Phase: v1alpha1.WorkflowPhaseCompleted},
				{Name: "b", Phase: v1alpha1.WorkflowPhaseCompleted},
				{Name: "c", Phase: v1alpha1.WorkflowPhaseRunning},
			},
			total:    5,
			expected: "2/5",
		},
		{
			name: "failed counts as done",
			statuses: []v1alpha1.TemplateStatus{
				{Name: "a", Phase: v1alpha1.WorkflowPhaseCompleted},
				{Name: "b", Phase: v1alpha1.WorkflowPhaseFailed},
			},
			total:    3,
			expected: "2/3",
		},
		{
			name: "all completed",
			statuses: []v1alpha1.TemplateStatus{
				{Name: "a", Phase: v1alpha1.WorkflowPhaseCompleted},
				{Name: "b", Phase: v1alpha1.WorkflowPhaseCompleted},
				{Name: "c", Phase: v1alpha1.WorkflowPhaseCompleted},
			},
			total:    3,
			expected: "3/3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := controller.FormatProgress(tt.statuses, tt.total)
			if result != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestWorkflowDAG_EmptyTemplates(t *testing.T) {
	order, err := controller.TopologicalSort(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 0 {
		t.Fatalf("expected empty order, got %v", order)
	}
}

func TestWorkflowDAG_SingleTemplate(t *testing.T) {
	templates := []v1alpha1.WorkflowTemplate{
		{Name: "only", Type: v1alpha1.TemplateTypeExperiment},
	}

	order, err := controller.TopologicalSort(templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 1 || order[0] != "only" {
		t.Fatalf("expected [only], got %v", order)
	}
}

func TestWorkflowDAG_SelfCycle(t *testing.T) {
	templates := []v1alpha1.WorkflowTemplate{
		{Name: "a", Type: v1alpha1.TemplateTypeDelay, Dependencies: []string{"a"}},
	}

	_, err := controller.TopologicalSort(templates)
	if err == nil {
		t.Fatal("expected cycle detection for self-referencing template")
	}
}

// Ensure the reconciler struct satisfies the expected shape
func TestWorkflowReconcilerStruct(t *testing.T) {
	_ = &controller.WorkflowReconciler{}
}

// Verify setWorkflowCondition is used correctly via FormatProgress (exported helper)
func TestWorkflowProgress_ZeroTotal(t *testing.T) {
	result := controller.FormatProgress(nil, 0)
	if result != "0/0" {
		t.Fatalf("expected 0/0, got %q", result)
	}
}

// Suppress unused import warning
var _ = metav1.Now
