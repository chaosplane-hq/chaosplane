package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

type workItem struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	ExperimentType  string          `json:"experimentType"`
	Target          json.RawMessage `json:"target"`
	Action          json.RawMessage `json:"action"`
	DurationSeconds int             `json:"durationSeconds"`
	DesiredState    string          `json:"desiredState"`
	Generation      int64           `json:"generation"`
}

type workResponse struct {
	Experiment *workItem `json:"experiment"`
}

type statusReport struct {
	Status             string          `json:"status"`
	ObservedGeneration *int64          `json:"observedGeneration,omitempty"`
	Phase              string          `json:"phase,omitempty"`
	Result             json.RawMessage `json:"result,omitempty"`
}

type statusAck struct {
	Acknowledged bool   `json:"acknowledged"`
	DesiredState string `json:"desiredState"`
	Generation   int64  `json:"generation"`
}

type targetPayload struct {
	Namespace     string            `json:"namespace"`
	LabelSelector map[string]string `json:"labelSelector"`
	Mode          string            `json:"mode"`
}

type actionPayload struct {
	Type       string          `json:"type"`
	Parameters json.RawMessage `json:"parameters"`
}

const agentManagedNamespace = "chaosplane-experiments"

func workLoop(ctx context.Context, cfg AgentConfig, agentInstance string) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	restCfg, err := ctrl.GetConfig()
	if err != nil {
		slog.Error("work loop disabled: no kube config", "error", err)
		return
	}
	k8s, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		slog.Error("work loop disabled: cannot build client", "error", err)
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w, err := claimWork(ctx, cfg, agentInstance)
			if err != nil {
				slog.Warn("claim work failed", "error", err)
				continue
			}
			if w == nil {
				continue
			}
			slog.Info("claimed experiment", "id", w.ID, "name", w.Name)
			go runExperiment(ctx, cfg, k8s, agentInstance, w)
		}
	}
}

func claimWork(ctx context.Context, cfg AgentConfig, agentInstance string) (*workItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.PlatformURL+"/agent/v1/work", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("X-Agent-Instance", agentInstance)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("claim status %d", resp.StatusCode)
	}
	var wr workResponse
	if err := json.NewDecoder(resp.Body).Decode(&wr); err != nil {
		return nil, err
	}
	return wr.Experiment, nil
}

func runExperiment(ctx context.Context, cfg AgentConfig, k8s client.Client, agentInstance string, w *workItem) {
	cr, err := toChaosExperiment(w)
	if err != nil {
		slog.Error("translate experiment failed", "id", w.ID, "error", err)
		reportStatus(ctx, cfg, agentInstance, w.ID, statusReport{Status: "failed", Result: resultJSON(false, err.Error())})
		return
	}

	if err := ensureNamespace(ctx, k8s); err != nil {
		slog.Error("ensure namespace failed", "error", err)
	}

	if err := k8s.Patch(ctx, cr, client.Apply, client.FieldOwner("chaosplane-agent"), client.ForceOwnership); err != nil {
		slog.Error("apply CRD failed", "id", w.ID, "error", err)
		reportStatus(ctx, cfg, agentInstance, w.ID, statusReport{Status: "failed", Result: resultJSON(false, err.Error())})
		return
	}

	reportStatus(ctx, cfg, agentInstance, w.ID, statusReport{Status: "running", Phase: "Running"})
	watchExperiment(ctx, cfg, k8s, agentInstance, w)
}

func watchExperiment(ctx context.Context, cfg AgentConfig, k8s client.Client, agentInstance string, w *workItem) {
	deadline := time.Now().Add(time.Duration(w.DurationSeconds+120) * time.Second)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	key := types.NamespacedName{Namespace: agentManagedNamespace, Name: crName(w.ID)}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var cr v1alpha1.ChaosExperiment
			if err := k8s.Get(ctx, key, &cr); err != nil {
				slog.Warn("get CRD failed", "id", w.ID, "error", err)
				if time.Now().After(deadline) {
					return
				}
				continue
			}

			ack := reportStatus(ctx, cfg, agentInstance, w.ID, mapPhase(cr.Status.Phase, cr.Status.Message))
			if ack != nil && ack.DesiredState == "abort" {
				annotateAbort(ctx, k8s, &cr)
			}

			switch cr.Status.Phase {
			case v1alpha1.PhaseCompleted, v1alpha1.PhaseFailed, v1alpha1.PhaseAborted:
				return
			}
			if time.Now().After(deadline) {
				slog.Warn("experiment watch timed out", "id", w.ID)
				reportStatus(ctx, cfg, agentInstance, w.ID, statusReport{Status: "failed", Result: resultJSON(false, "agent watch deadline exceeded")})
				return
			}
		}
	}
}

func mapPhase(phase v1alpha1.ExperimentPhase, message string) statusReport {
	switch phase {
	case v1alpha1.PhaseCompleted:
		return statusReport{Status: "completed", Phase: string(phase), Result: resultJSON(true, "")}
	case v1alpha1.PhaseFailed:
		return statusReport{Status: "failed", Phase: string(phase), Result: resultJSON(false, message)}
	case v1alpha1.PhaseAborted:
		return statusReport{Status: "aborted", Phase: string(phase), Result: resultJSON(false, message)}
	default:
		return statusReport{Status: "running", Phase: string(phase)}
	}
}

func annotateAbort(ctx context.Context, k8s client.Client, cr *v1alpha1.ChaosExperiment) {
	patch := client.MergeFrom(cr.DeepCopy())
	if cr.Annotations == nil {
		cr.Annotations = map[string]string{}
	}
	cr.Annotations["chaosplane.dev/abort"] = "true"
	if err := k8s.Patch(ctx, cr, patch); err != nil {
		slog.Warn("annotate abort failed", "error", err)
	}
}

func reportStatus(ctx context.Context, cfg AgentConfig, agentInstance, experimentID string, report statusReport) *statusAck {
	data, _ := json.Marshal(report)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.PlatformURL+"/agent/v1/experiments/"+experimentID+"/status", newReader(data))
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("X-Agent-Instance", agentInstance)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		slog.Warn("report status failed", "id", experimentID, "error", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}
	var ack statusAck
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		return nil
	}
	return &ack
}

func toChaosExperiment(w *workItem) (*v1alpha1.ChaosExperiment, error) {
	var tp targetPayload
	if err := json.Unmarshal(w.Target, &tp); err != nil {
		return nil, fmt.Errorf("decode target: %w", err)
	}
	var ap actionPayload
	if err := json.Unmarshal(w.Action, &ap); err != nil {
		return nil, fmt.Errorf("decode action: %w", err)
	}

	ns := tp.Namespace
	if ns == "" {
		ns = "default"
	}

	target := v1alpha1.TargetSpec{Kind: "Pod", Namespace: ns}
	if len(tp.LabelSelector) > 0 {
		target.LabelSelector = &metav1.LabelSelector{MatchLabels: tp.LabelSelector}
	}

	action := v1alpha1.ActionSpec{Type: ap.Type}
	if len(ap.Parameters) > 0 {
		action.Parameters = runtime.RawExtension{Raw: ap.Parameters}
	}

	cr := &v1alpha1.ChaosExperiment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "chaos.chaosplane.dev/v1alpha1",
			Kind:       "ChaosExperiment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName(w.ID),
			Namespace: agentManagedNamespace,
			Labels:    map[string]string{"chaosplane.dev/experiment-id": w.ID},
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:   target,
			Action:   action,
			Duration: metav1.Duration{Duration: time.Duration(w.DurationSeconds) * time.Second},
		},
	}
	return cr, nil
}

func crName(experimentID string) string {
	return "exp-" + experimentID
}

func resultJSON(steadyStateMet bool, errMsg string) json.RawMessage {
	m := map[string]interface{}{"steadyStateMet": steadyStateMet}
	if errMsg != "" {
		m["errorMessage"] = errMsg
	}
	b, _ := json.Marshal(m)
	return b
}

func newReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}

func ensureNamespace(ctx context.Context, k8s client.Client) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: agentManagedNamespace}}
	err := k8s.Create(ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}
