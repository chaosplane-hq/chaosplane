package timechaos

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

const (
	faketimeVolumeName  = "chaosplane-faketime"
	faketimeMountPath   = "/usr/local/lib/faketime"
	faketimeLibPath     = "/usr/local/lib/faketime/libfaketime.so.1"
	faketimeInitName    = "chaosplane-faketime-init"
	defaultFaketimeImg  = "trpsubstitution/libfaketime:latest"
	defaultInitCopyDest = "/data/libfaketime.so.1"
)

var _ executor.Executor = (*TimeChaosExecutor)(nil)

// TimeChaosExecutor skews the wall clock a target Deployment's containers
// observe by injecting libfaketime through LD_PRELOAD. libfaketime intercepts
// time()/clock_gettime()/gettimeofday() in userspace, so the application sees a
// shifted clock without changing the node's real clock (which is shared by all
// containers and must never be touched).
//
// Real-skew mechanism: an init container copies libfaketime.so.1 into a shared
// emptyDir, and every app container gets LD_PRELOAD pointing at it plus a
// FAKETIME offset (e.g. "+1h", "-30m"). Pods are recreated by the Deployment
// controller and come up with the skew applied.
//
// Constraints (honest): the target must be a Deployment so the controller
// recreates pods with the injection; already-running processes cannot be
// preloaded. libfaketime only shifts CLOCK_REALTIME, not CLOCK_MONOTONIC, and a
// statically linked binary that bypasses libc time calls will not be affected.
// Rollback strips the injection, returning the Deployment to its snapshot.
type TimeChaosExecutor struct {
	Logger *slog.Logger
	Client client.Client

	mu    sync.Mutex
	state map[string]*deploymentSnapshot
}

type deploymentSnapshot struct {
	initContainers []corev1.Container
	containers     []corev1.Container
	volumes        []corev1.Volume
}

func NewTimeChaosExecutor(logger *slog.Logger, c client.Client) *TimeChaosExecutor {
	return &TimeChaosExecutor{Logger: logger, Client: c, state: make(map[string]*deploymentSnapshot)}
}

func parseParameters(exp *v1alpha1.ChaosExperiment) (map[string]string, error) {
	params := map[string]string{}
	if exp.Spec.Action.Parameters.Raw == nil {
		return params, nil
	}
	if err := json.Unmarshal(exp.Spec.Action.Parameters.Raw, &params); err != nil {
		return nil, fmt.Errorf("parse parameters: %w", err)
	}
	return params, nil
}

func (e *TimeChaosExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	if len(exp.Spec.Target.Names) != 1 || exp.Spec.Target.Namespace == "" {
		return fmt.Errorf("time-chaos: target must specify exactly one Deployment name and a namespace")
	}
	params, err := parseParameters(exp)
	if err != nil {
		return fmt.Errorf("time-chaos: %w", err)
	}
	offset := params["offset"]
	if offset == "" {
		return fmt.Errorf("time-chaos: offset parameter is required (e.g. +1h, -30m)")
	}
	faketimeImg := params["faketimeImage"]
	if faketimeImg == "" {
		faketimeImg = defaultFaketimeImg
	}

	name := exp.Spec.Target.Names[0]
	ns := exp.Spec.Target.Namespace

	var dep appsv1.Deployment
	if err := e.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &dep); err != nil {
		return fmt.Errorf("time-chaos: get deployment %s/%s: %w", ns, name, err)
	}

	tmpl := &dep.Spec.Template.Spec
	snap := &deploymentSnapshot{
		initContainers: deepCopyContainers(tmpl.InitContainers),
		containers:     deepCopyContainers(tmpl.Containers),
		volumes:        deepCopyVolumes(tmpl.Volumes),
	}

	if hasFaketimeInjection(tmpl) {
		return fmt.Errorf("time-chaos: deployment %s/%s already has time chaos injected", ns, name)
	}

	tmpl.Volumes = append(tmpl.Volumes, corev1.Volume{
		Name:         faketimeVolumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	})

	tmpl.InitContainers = append(tmpl.InitContainers, corev1.Container{
		Name:    faketimeInitName,
		Image:   faketimeImg,
		Command: []string{"sh", "-c", fmt.Sprintf("cp /usr/lib/faketime/libfaketime.so.1 %s || cp %s %s", faketimeMountPath+"/libfaketime.so.1", defaultInitCopyDest, faketimeMountPath+"/libfaketime.so.1")},
		VolumeMounts: []corev1.VolumeMount{
			{Name: faketimeVolumeName, MountPath: faketimeMountPath},
		},
	})

	for i := range tmpl.Containers {
		injectFaketime(&tmpl.Containers[i], offset)
	}

	if err := e.Client.Update(ctx, &dep); err != nil {
		return fmt.Errorf("time-chaos: update deployment %s/%s: %w", ns, name, err)
	}

	e.mu.Lock()
	e.state[string(exp.UID)] = snap
	e.mu.Unlock()

	e.Logger.InfoContext(ctx, "time-chaos: injected libfaketime", "namespace", ns, "name", name, "offset", offset)
	return nil
}

func (e *TimeChaosExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	e.mu.Lock()
	snap := e.state[string(exp.UID)]
	delete(e.state, string(exp.UID))
	e.mu.Unlock()
	if snap == nil {
		return nil
	}

	name := exp.Spec.Target.Names[0]
	ns := exp.Spec.Target.Namespace

	var dep appsv1.Deployment
	if err := e.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &dep); err != nil {
		return fmt.Errorf("time-chaos: rollback get deployment %s/%s: %w", ns, name, err)
	}

	dep.Spec.Template.Spec.InitContainers = snap.initContainers
	dep.Spec.Template.Spec.Containers = snap.containers
	dep.Spec.Template.Spec.Volumes = snap.volumes

	if err := e.Client.Update(ctx, &dep); err != nil {
		return fmt.Errorf("time-chaos: rollback update %s/%s: %w", ns, name, err)
	}
	e.Logger.InfoContext(ctx, "time-chaos: removed libfaketime injection", "namespace", ns, "name", name)
	return nil
}

func (e *TimeChaosExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	if len(exp.Spec.Target.Names) != 1 || exp.Spec.Target.Namespace == "" {
		return fmt.Errorf("time-chaos: target must specify exactly one Deployment name and a namespace")
	}
	params, err := parseParameters(exp)
	if err != nil {
		return fmt.Errorf("time-chaos: %w", err)
	}
	if params["offset"] == "" {
		return fmt.Errorf("time-chaos: offset parameter is required (e.g. +1h, -30m)")
	}
	return nil
}

// injectFaketime adds the LD_PRELOAD and FAKETIME env vars plus the shared
// libfaketime mount to a container. Existing LD_PRELOAD is replaced; the
// snapshot holds the original for rollback.
func injectFaketime(c *corev1.Container, offset string) {
	env := make([]corev1.EnvVar, 0, len(c.Env)+2)
	for _, e := range c.Env {
		if e.Name == "LD_PRELOAD" || e.Name == "FAKETIME" {
			continue
		}
		env = append(env, e)
	}
	env = append(env,
		corev1.EnvVar{Name: "LD_PRELOAD", Value: faketimeLibPath},
		corev1.EnvVar{Name: "FAKETIME", Value: offset},
	)
	c.Env = env

	for _, m := range c.VolumeMounts {
		if m.Name == faketimeVolumeName {
			return
		}
	}
	c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
		Name:      faketimeVolumeName,
		MountPath: faketimeMountPath,
		ReadOnly:  true,
	})
}

func hasFaketimeInjection(spec *corev1.PodSpec) bool {
	for _, ic := range spec.InitContainers {
		if ic.Name == faketimeInitName {
			return true
		}
	}
	return false
}

func deepCopyContainers(in []corev1.Container) []corev1.Container {
	if in == nil {
		return nil
	}
	out := make([]corev1.Container, len(in))
	for i := range in {
		out[i] = *in[i].DeepCopy()
	}
	return out
}

func deepCopyVolumes(in []corev1.Volume) []corev1.Volume {
	if in == nil {
		return nil
	}
	out := make([]corev1.Volume, len(in))
	for i := range in {
		out[i] = *in[i].DeepCopy()
	}
	return out
}
