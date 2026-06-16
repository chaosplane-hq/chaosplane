package timechaos_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor/timechaos"
)

var testScheme = runtime.NewScheme()

func init() {
	_ = appsv1.AddToScheme(testScheme)
	_ = corev1.AddToScheme(testScheme)
	_ = v1alpha1.AddToScheme(testScheme)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func makeDeployment(name, ns string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "myapp:1", Env: []corev1.EnvVar{{Name: "FOO", Value: "bar"}}},
					},
				},
			},
		},
	}
}

func newExp(offset, name, ns string) *v1alpha1.ChaosExperiment {
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "default", UID: types.UID("time-uid-1")},
		Spec: v1alpha1.ChaosExperimentSpec{
			Action: v1alpha1.ActionSpec{Type: "time-chaos"},
			Target: v1alpha1.TargetSpec{Kind: "Deployment", Namespace: ns, Names: []string{name}},
		},
	}
	params := map[string]string{}
	if offset != "" {
		params["offset"] = offset
	}
	raw, _ := json.Marshal(params)
	exp.Spec.Action.Parameters = runtime.RawExtension{Raw: raw}
	return exp
}

func getDep(t *testing.T, c client.Client, ns, name string) *appsv1.Deployment {
	t.Helper()
	var dep appsv1.Deployment
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: name}, &dep); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	return &dep
}

func TestTimeChaos_InjectsAndRollsBack(t *testing.T) {
	dep := makeDeployment("web", "default")
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(dep).Build()
	e := timechaos.NewTimeChaosExecutor(testLogger(), c)
	exp := newExp("+1h", "web", "default")

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got := getDep(t, c, "default", "web")
	spec := got.Spec.Template.Spec
	if len(spec.InitContainers) != 1 {
		t.Fatalf("expected faketime init container, got %d", len(spec.InitContainers))
	}
	foundVol := false
	for _, v := range spec.Volumes {
		if v.Name == "chaosplane-faketime" {
			foundVol = true
		}
	}
	if !foundVol {
		t.Fatal("expected faketime emptyDir volume")
	}

	var ldPreload, faketime string
	for _, env := range spec.Containers[0].Env {
		switch env.Name {
		case "LD_PRELOAD":
			ldPreload = env.Value
		case "FAKETIME":
			faketime = env.Value
		}
	}
	if ldPreload == "" {
		t.Fatal("expected LD_PRELOAD injected")
	}
	if faketime != "+1h" {
		t.Fatalf("expected FAKETIME=+1h, got %q", faketime)
	}

	if err := e.Rollback(context.Background(), exp); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	restored := getDep(t, c, "default", "web")
	rspec := restored.Spec.Template.Spec
	if len(rspec.InitContainers) != 0 {
		t.Fatalf("expected init containers removed, got %d", len(rspec.InitContainers))
	}
	if len(rspec.Volumes) != 0 {
		t.Fatalf("expected faketime volume removed, got %d", len(rspec.Volumes))
	}
	if len(rspec.Containers[0].Env) != 1 || rspec.Containers[0].Env[0].Name != "FOO" {
		t.Fatalf("expected original env restored, got %v", rspec.Containers[0].Env)
	}
}

func TestTimeChaos_DoubleInjectFails(t *testing.T) {
	dep := makeDeployment("web", "default")
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(dep).Build()
	e := timechaos.NewTimeChaosExecutor(testLogger(), c)
	exp := newExp("+1h", "web", "default")

	if err := e.Execute(context.Background(), exp); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	exp2 := newExp("+2h", "web", "default")
	exp2.UID = types.UID("time-uid-2")
	if err := e.Execute(context.Background(), exp2); err == nil {
		t.Fatal("expected error injecting time chaos twice")
	}
}

func TestTimeChaos_MissingDeploymentFails(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme).Build()
	e := timechaos.NewTimeChaosExecutor(testLogger(), c)
	exp := newExp("+1h", "nope", "default")
	if err := e.Execute(context.Background(), exp); err == nil {
		t.Fatal("expected error for missing deployment")
	}
}

func TestTimeChaos_Validate(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme).Build()
	e := timechaos.NewTimeChaosExecutor(testLogger(), c)

	if err := e.Validate(newExp("+1h", "web", "default")); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if err := e.Validate(newExp("", "web", "default")); err == nil {
		t.Fatal("expected error for missing offset")
	}
	noName := newExp("+1h", "web", "default")
	noName.Spec.Target.Names = nil
	if err := e.Validate(noName); err == nil {
		t.Fatal("expected error for missing target name")
	}
}
