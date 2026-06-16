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

var _ executor.Executor = (*ConfigMapChaosExecutor)(nil)

// configMapSnapshot preserves what is needed to undo a fault: the original
// Data/BinaryData, and whether the object existed at all (delete mode needs to
// recreate it on rollback).
type configMapSnapshot struct {
	existed    bool
	data       map[string]string
	binaryData map[string][]byte
}

type ConfigMapChaosExecutor struct {
	Logger *slog.Logger
	Client client.Client

	mu    sync.Mutex
	state map[string]configMapSnapshot
}

func NewConfigMapChaosExecutor(logger *slog.Logger, c client.Client) *ConfigMapChaosExecutor {
	return &ConfigMapChaosExecutor{Logger: logger, Client: c, state: make(map[string]configMapSnapshot)}
}

// Execute corrupts a target ConfigMap. mode "corrupt" (default) overwrites the
// value of one key (or every key when key is empty); mode "delete" removes the
// whole object. The pre-fault state is snapshotted so Rollback restores it.
func (e *ConfigMapChaosExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	name, err := targetName(exp.Spec.Target, "configmap-chaos")
	if err != nil {
		return err
	}
	ns := exp.Spec.Target.Namespace
	params, err := ParseParameters(exp)
	if err != nil {
		return fmt.Errorf("configmap-chaos: %w", err)
	}

	var cm corev1.ConfigMap
	if err := e.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &cm); err != nil {
		return fmt.Errorf("configmap-chaos: get configmap %s/%s: %w", ns, name, err)
	}

	snap := configMapSnapshot{existed: true, data: copyStringMap(cm.Data), binaryData: copyBytesMap(cm.BinaryData)}

	mode := params["mode"]
	if mode == "" {
		mode = "corrupt"
	}
	switch mode {
	case "delete":
		if err := e.Client.Delete(ctx, &cm); err != nil {
			return fmt.Errorf("configmap-chaos: delete %s/%s: %w", ns, name, err)
		}
	case "corrupt":
		corruptValue := params["value"]
		if corruptValue == "" {
			corruptValue = "chaosplane-corrupted"
		}
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		if key := params["key"]; key != "" {
			cm.Data[key] = corruptValue
		} else {
			for k := range cm.Data {
				cm.Data[k] = corruptValue
			}
			if len(cm.Data) == 0 {
				cm.Data["chaosplane"] = corruptValue
			}
		}
		if err := e.Client.Update(ctx, &cm); err != nil {
			return fmt.Errorf("configmap-chaos: update %s/%s: %w", ns, name, err)
		}
	default:
		return fmt.Errorf("configmap-chaos: unknown mode %q (want corrupt|delete)", mode)
	}

	e.mu.Lock()
	e.state[string(exp.UID)] = snap
	e.mu.Unlock()

	e.Logger.InfoContext(ctx, "configmap-chaos: applied", "namespace", ns, "name", name, "mode", mode)
	return nil
}

func (e *ConfigMapChaosExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	e.mu.Lock()
	snap, ok := e.state[string(exp.UID)]
	delete(e.state, string(exp.UID))
	e.mu.Unlock()
	if !ok {
		return nil
	}

	name := exp.Spec.Target.Names[0]
	ns := exp.Spec.Target.Namespace

	var cm corev1.ConfigMap
	err := e.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &cm)
	switch {
	case apierrors.IsNotFound(err):
		restored := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
			Data:       snap.data,
			BinaryData: snap.binaryData,
		}
		if err := e.Client.Create(ctx, restored); err != nil {
			return fmt.Errorf("configmap-chaos: recreate %s/%s: %w", ns, name, err)
		}
	case err != nil:
		return fmt.Errorf("configmap-chaos: rollback get %s/%s: %w", ns, name, err)
	default:
		cm.Data = snap.data
		cm.BinaryData = snap.binaryData
		if err := e.Client.Update(ctx, &cm); err != nil {
			return fmt.Errorf("configmap-chaos: restore %s/%s: %w", ns, name, err)
		}
	}
	e.Logger.InfoContext(ctx, "configmap-chaos: restored", "namespace", ns, "name", name)
	return nil
}

func (e *ConfigMapChaosExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := targetName(exp.Spec.Target, "configmap-chaos")
	return err
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyBytesMap(in map[string][]byte) map[string][]byte {
	if in == nil {
		return nil
	}
	out := make(map[string][]byte, len(in))
	for k, v := range in {
		cp := make([]byte, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}
