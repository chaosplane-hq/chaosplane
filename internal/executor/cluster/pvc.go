package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
)

var _ executor.Executor = (*PVCChaosExecutor)(nil)

type PVCChaosExecutor struct {
	Logger *slog.Logger
	Client client.Client

	mu    sync.Mutex
	state map[string]*corev1.PersistentVolumeClaim
}

func NewPVCChaosExecutor(logger *slog.Logger, c client.Client) *PVCChaosExecutor {
	return &PVCChaosExecutor{Logger: logger, Client: c, state: make(map[string]*corev1.PersistentVolumeClaim)}
}

// Execute deletes the target PVC to simulate storage loss, after snapshotting
// its spec so Rollback can recreate an identical claim. A PVC bound to a running
// pod carries the pvc-protection finalizer, so the API marks it terminating but
// defers actual removal until the pod releases it, which is itself the realistic
// "volume vanishing under a workload" signal.
func (e *PVCChaosExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	name, err := targetName(exp.Spec.Target, "pvc-chaos")
	if err != nil {
		return err
	}
	ns := exp.Spec.Target.Namespace

	var pvc corev1.PersistentVolumeClaim
	if err := e.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &pvc); err != nil {
		return fmt.Errorf("pvc-chaos: get pvc %s/%s: %w", ns, name, err)
	}

	snap := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        name,
			Labels:      copyStringMap(pvc.Labels),
			Annotations: copyStringMap(pvc.Annotations),
		},
		Spec: *pvc.Spec.DeepCopy(),
	}

	if err := e.Client.Delete(ctx, &pvc); err != nil {
		return fmt.Errorf("pvc-chaos: delete %s/%s: %w", ns, name, err)
	}

	e.mu.Lock()
	e.state[string(exp.UID)] = snap
	e.mu.Unlock()

	e.Logger.InfoContext(ctx, "pvc-chaos: deleted PVC", "namespace", ns, "name", name)
	return nil
}

func (e *PVCChaosExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	e.mu.Lock()
	snap := e.state[string(exp.UID)]
	delete(e.state, string(exp.UID))
	e.mu.Unlock()
	if snap == nil {
		return nil
	}

	var existing corev1.PersistentVolumeClaim
	err := e.Client.Get(ctx, client.ObjectKey{Namespace: snap.Namespace, Name: snap.Name}, &existing)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("pvc-chaos: rollback get %s/%s: %w", snap.Namespace, snap.Name, err)
	}

	restored := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   snap.Namespace,
			Name:        snap.Name,
			Labels:      snap.Labels,
			Annotations: snap.Annotations,
		},
		Spec: snap.Spec,
	}
	if err := e.Client.Create(ctx, restored); err != nil {
		return fmt.Errorf("pvc-chaos: recreate %s/%s: %w", snap.Namespace, snap.Name, err)
	}
	e.Logger.InfoContext(ctx, "pvc-chaos: recreated PVC", "namespace", snap.Namespace, "name", snap.Name)
	return nil
}

func (e *PVCChaosExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := targetName(exp.Spec.Target, "pvc-chaos")
	return err
}
