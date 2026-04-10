package daemon

import (
	"context"
	"fmt"
	"sync"
	"testing"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
)

func TestExecNetworkChaos(t *testing.T) {
	srv := NewServer()
	resp, err := srv.ExecNetworkChaos(context.Background(), &daemonv1.NetworkChaosRequest{
		ExperimentId: "exp-1",
		Action:       "delay",
		TargetIface:  "eth0",
		Parameters:   map[string]string{"latency": "100ms"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetSuccess() {
		t.Fatal("expected success")
	}
	if resp.GetExecutionId() == "" {
		t.Fatal("expected execution ID")
	}
}

func TestExecStressChaos(t *testing.T) {
	srv := NewServer()
	resp, err := srv.ExecStressChaos(context.Background(), &daemonv1.StressChaosRequest{
		ExperimentId: "exp-2",
		StressorType: "cpu",
		Parameters:   map[string]string{"workers": "4"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetSuccess() {
		t.Fatal("expected success")
	}
	if resp.GetExecutionId() == "" {
		t.Fatal("expected execution ID")
	}
}

func TestExecDNSChaos(t *testing.T) {
	srv := NewServer()
	resp, err := srv.ExecDNSChaos(context.Background(), &daemonv1.DNSChaosRequest{
		ExperimentId: "exp-3",
		Action:       "error",
		Parameters:   map[string]string{"domain": "example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetSuccess() {
		t.Fatal("expected success")
	}
	if resp.GetExecutionId() == "" {
		t.Fatal("expected execution ID")
	}
}

func TestExecHTTPChaos(t *testing.T) {
	srv := NewServer()
	resp, err := srv.ExecHTTPChaos(context.Background(), &daemonv1.HTTPChaosRequest{
		ExperimentId: "exp-4",
		Action:       "abort",
		Port:         8080,
		Parameters:   map[string]string{"code": "503"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetSuccess() {
		t.Fatal("expected success")
	}
	if resp.GetExecutionId() == "" {
		t.Fatal("expected execution ID")
	}
}

func TestExecNodeChaos(t *testing.T) {
	srv := NewServer()
	resp, err := srv.ExecNodeChaos(context.Background(), &daemonv1.NodeChaosRequest{
		ExperimentId: "exp-5",
		Action:       "shutdown",
		Parameters:   map[string]string{"grace_period": "30s"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetSuccess() {
		t.Fatal("expected success")
	}
	if resp.GetExecutionId() == "" {
		t.Fatal("expected execution ID")
	}
}

func TestCancelChaos(t *testing.T) {
	srv := NewServer()

	resp, err := srv.ExecNetworkChaos(context.Background(), &daemonv1.NetworkChaosRequest{
		ExperimentId: "exp-cancel",
		Action:       "delay",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	execID := resp.GetExecutionId()

	cancelResp, err := srv.CancelChaos(context.Background(), &daemonv1.CancelRequest{
		ExecutionId: execID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cancelResp.GetSuccess() {
		t.Fatal("expected cancel success")
	}

	cancelResp2, err := srv.CancelChaos(context.Background(), &daemonv1.CancelRequest{
		ExecutionId: "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cancelResp2.GetSuccess() {
		t.Fatal("expected cancel failure for nonexistent ID")
	}
}

func TestGetChaosStatus(t *testing.T) {
	srv := NewServer()
	ctx := context.Background()

	statusResp, err := srv.GetChaosStatus(ctx, &daemonv1.StatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statusResp.GetExecutions()) != 0 {
		t.Fatalf("expected 0 executions, got %d", len(statusResp.GetExecutions()))
	}

	srv.ExecNetworkChaos(ctx, &daemonv1.NetworkChaosRequest{ExperimentId: "exp-s1", Action: "delay"})
	srv.ExecStressChaos(ctx, &daemonv1.StressChaosRequest{ExperimentId: "exp-s2", StressorType: "cpu"})
	srv.ExecDNSChaos(ctx, &daemonv1.DNSChaosRequest{ExperimentId: "exp-s3", Action: "error"})

	statusResp, err = srv.GetChaosStatus(ctx, &daemonv1.StatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statusResp.GetExecutions()) != 3 {
		t.Fatalf("expected 3 executions, got %d", len(statusResp.GetExecutions()))
	}
}

func TestExecutionStore(t *testing.T) {
	store := NewExecutionStore()

	store.Add("id-1", ExecutionInfo{ID: "id-1", Type: "network", Status: "running"})
	store.Add("id-2", ExecutionInfo{ID: "id-2", Type: "stress", Status: "running"})

	info, ok := store.Get("id-1")
	if !ok {
		t.Fatal("expected to find id-1")
	}
	if info.Type != "network" {
		t.Fatalf("expected network, got %s", info.Type)
	}

	_, ok = store.Get("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}

	all := store.List()
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}

	store.Remove("id-1")
	_, ok = store.Get("id-1")
	if ok {
		t.Fatal("expected id-1 removed")
	}

	all = store.List()
	if len(all) != 1 {
		t.Fatalf("expected 1, got %d", len(all))
	}
}

func TestExecutionStoreConcurrent(t *testing.T) {
	store := NewExecutionStore()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("id-%d", n)
			store.Add(id, ExecutionInfo{ID: id, Type: "network", Status: "running"})
			store.Get(id)
			store.List()
			store.Remove(id)
		}(i)
	}
	wg.Wait()
}
