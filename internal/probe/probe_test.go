package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

func TestPrometheusProbe_ConditionPass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := promQueryResponse{
			Status: "success",
			Data: promQueryData{
				ResultType: "vector",
				Result: []promQueryResult{
					{Value: []json.RawMessage{
						json.RawMessage(`1234567890`),
						json.RawMessage(`"0.95"`),
					}},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := &prometheusProbe{
		spec: v1alpha1.PrometheusProbe{
			URL:   srv.URL,
			Query: "up",
			Condition: v1alpha1.ProbeCondition{
				Operator:  ">",
				Threshold: 0.5,
			},
		},
	}

	ok, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected probe to pass")
	}
}

func TestPrometheusProbe_ConditionFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := promQueryResponse{
			Status: "success",
			Data: promQueryData{
				ResultType: "vector",
				Result: []promQueryResult{
					{Value: []json.RawMessage{
						json.RawMessage(`1234567890`),
						json.RawMessage(`"0.3"`),
					}},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := &prometheusProbe{
		spec: v1alpha1.PrometheusProbe{
			URL:   srv.URL,
			Query: "up",
			Condition: v1alpha1.ProbeCondition{
				Operator:  ">",
				Threshold: 0.5,
			},
		},
	}

	ok, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected probe to fail")
	}
}

func TestPrometheusProbe_EmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := promQueryResponse{
			Status: "success",
			Data: promQueryData{
				ResultType: "vector",
				Result:     []promQueryResult{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := &prometheusProbe{
		spec: v1alpha1.PrometheusProbe{
			URL:   srv.URL,
			Query: "nonexistent",
			Condition: v1alpha1.ProbeCondition{
				Operator:  ">",
				Threshold: 0,
			},
		},
	}

	ok, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected probe to fail on empty result")
	}
}

func TestEvaluateCondition(t *testing.T) {
	tests := []struct {
		val      float64
		operator string
		thresh   float64
		want     bool
	}{
		{10, ">", 5, true},
		{5, ">", 10, false},
		{10, ">=", 10, true},
		{5, "<", 10, true},
		{10, "<", 5, false},
		{10, "<=", 10, true},
		{10, "==", 10, true},
		{10, "==", 5, false},
		{10, "!=", 5, true},
		{10, "!=", 10, false},
		{10, "invalid", 5, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v_%s_%v", tt.val, tt.operator, tt.thresh), func(t *testing.T) {
			cond := v1alpha1.ProbeCondition{Operator: tt.operator, Threshold: tt.thresh}
			got := evaluateCondition(tt.val, cond)
			if got != tt.want {
				t.Errorf("evaluateCondition(%v, %s, %v) = %v, want %v", tt.val, tt.operator, tt.thresh, got, tt.want)
			}
		})
	}
}

func TestHTTPProbe_StatusMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"healthy"}`)
	}))
	defer srv.Close()

	p := &httpProbe{
		spec: v1alpha1.HTTPProbe{
			URL:            srv.URL,
			ExpectedStatus: 200,
		},
	}

	ok, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected probe to pass")
	}
}

func TestHTTPProbe_StatusMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p := &httpProbe{
		spec: v1alpha1.HTTPProbe{
			URL:            srv.URL,
			ExpectedStatus: 200,
		},
	}

	ok, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected probe to fail on status mismatch")
	}
}

func TestHTTPProbe_BodyMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"healthy","version":"1.2.3"}`)
	}))
	defer srv.Close()

	p := &httpProbe{
		spec: v1alpha1.HTTPProbe{
			URL:            srv.URL,
			ExpectedStatus: 200,
			ExpectedBody:   `"status"\s*:\s*"healthy"`,
		},
	}

	ok, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected probe to pass on body match")
	}
}

func TestHTTPProbe_BodyMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"degraded"}`)
	}))
	defer srv.Close()

	p := &httpProbe{
		spec: v1alpha1.HTTPProbe{
			URL:            srv.URL,
			ExpectedStatus: 200,
			ExpectedBody:   `"status"\s*:\s*"healthy"`,
		},
	}

	ok, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected probe to fail on body mismatch")
	}
}

func TestHTTPProbe_DefaultMethod(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := &httpProbe{
		spec: v1alpha1.HTTPProbe{
			URL: srv.URL,
		},
	}

	_, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("expected GET, got %s", gotMethod)
	}
}

func TestK8sProbe_MinReadyMet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-3", Namespace: "default", Labels: map[string]string{"app": "web"}},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
	}

	objs := make([]runtime.Object, len(pods))
	for i := range pods {
		objs[i] = &pods[i]
	}

	fc := fakeclient.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	p := &k8sProbe{
		spec: v1alpha1.K8sProbe{
			Resource:      "Pod",
			Namespace:     "default",
			LabelSelector: "app=web",
			Condition:     v1alpha1.K8sProbeCondition{MinReady: 2},
		},
		client: fc,
	}

	ok, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected probe to pass with 2 running pods")
	}
}

func TestK8sProbe_MinReadyNotMet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default", Labels: map[string]string{"app": "web"}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	fc := fakeclient.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(pod).Build()

	p := &k8sProbe{
		spec: v1alpha1.K8sProbe{
			Resource:      "Pod",
			Namespace:     "default",
			LabelSelector: "app=web",
			Condition:     v1alpha1.K8sProbeCondition{MinReady: 3},
		},
		client: fc,
	}

	ok, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected probe to fail with only 1 running pod")
	}
}

func TestK8sProbe_UnsupportedResource(t *testing.T) {
	scheme := runtime.NewScheme()
	fc := fakeclient.NewClientBuilder().WithScheme(scheme).Build()

	p := &k8sProbe{
		spec: v1alpha1.K8sProbe{
			Resource:  "Deployment",
			Namespace: "default",
			Condition: v1alpha1.K8sProbeCondition{MinReady: 1},
		},
		client: fc,
	}

	_, err := p.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for unsupported resource")
	}
}

func TestRunAll_AllPass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	probes := []v1alpha1.ProbeSpec{
		{
			Name: "http-check",
			Type: v1alpha1.ProbeTypeHTTP,
			HTTP: &v1alpha1.HTTPProbe{
				URL:            srv.URL,
				ExpectedStatus: 200,
			},
		},
	}

	ok, err := RunAll(context.Background(), probes, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected all probes to pass")
	}
}

func TestRunAll_OneFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	probes := []v1alpha1.ProbeSpec{
		{
			Name: "http-check",
			Type: v1alpha1.ProbeTypeHTTP,
			HTTP: &v1alpha1.HTTPProbe{
				URL:            srv.URL,
				ExpectedStatus: 200,
			},
		},
	}

	ok, err := RunAll(context.Background(), probes, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected probes to fail")
	}
}

func TestRunAll_InvalidProbeType(t *testing.T) {
	probes := []v1alpha1.ProbeSpec{
		{
			Name: "bad",
			Type: "unknown",
		},
	}

	_, err := RunAll(context.Background(), probes, nil)
	if err == nil {
		t.Fatal("expected error for unknown probe type")
	}
}

func TestNewProbe_MissingConfig(t *testing.T) {
	tests := []struct {
		name string
		spec v1alpha1.ProbeSpec
	}{
		{"prometheus_nil", v1alpha1.ProbeSpec{Name: "p", Type: v1alpha1.ProbeTypePrometheus}},
		{"http_nil", v1alpha1.ProbeSpec{Name: "h", Type: v1alpha1.ProbeTypeHTTP}},
		{"k8s_nil", v1alpha1.ProbeSpec{Name: "k", Type: v1alpha1.ProbeTypeK8s}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewProbe(tt.spec, nil)
			if err == nil {
				t.Fatal("expected error for missing config")
			}
		})
	}
}
