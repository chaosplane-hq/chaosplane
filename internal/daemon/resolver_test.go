package daemon

import (
	"context"
	"fmt"
	"testing"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/daemon/netns"
)

type fakeResolver struct {
	res *netns.Resolution
	err error
}

func (f fakeResolver) Resolve(_ context.Context, _ netns.PodRef) (*netns.Resolution, error) {
	return f.res, f.err
}

func (f fakeResolver) ResolveCgroupV2(_ context.Context, _ netns.PodRef) (string, error) {
	if f.res == nil {
		return "", f.err
	}
	return f.res.CgroupV2Path, f.err
}

func TestExecNetworkChaosPodTargetRequiresResolver(t *testing.T) {
	srv := newServerWithRunner(fakeRunner{})

	resp, err := srv.ExecNetworkChaos(context.Background(), &daemonv1.NetworkChaosRequest{
		ExperimentId: "exp-pod",
		Action:       "delay",
		Namespace:    "default",
		PodName:      "web-0",
		ContainerId:  "abc123",
		Parameters:   map[string]string{"latency": "100ms"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetSuccess() {
		t.Fatal("expected Success=false when no resolver is configured for a pod target")
	}
	if resp.GetApplied() {
		t.Fatal("expected Applied=false")
	}
}

func TestExecNetworkChaosPodTargetResolutionFailure(t *testing.T) {
	srv := newServerWithRunner(fakeRunner{})
	srv.SetResolver(fakeResolver{err: fmt.Errorf("container not found")})

	resp, err := srv.ExecNetworkChaos(context.Background(), &daemonv1.NetworkChaosRequest{
		ExperimentId: "exp-pod",
		Action:       "delay",
		Namespace:    "default",
		PodName:      "web-0",
		ContainerId:  "abc123",
		Parameters:   map[string]string{"latency": "100ms", "mode": "ebpf"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetSuccess() {
		t.Fatal("expected Success=false when resolution fails")
	}
}

func TestExecNetworkChaosPodTargetResolvesHostVeth(t *testing.T) {
	srv := newServerWithRunner(fakeRunner{})
	srv.SetResolver(fakeResolver{res: &netns.Resolution{
		ContainerPID:    4242,
		HostVethIfindex: 42,
		HostVethName:    "veth1a2b3c",
		PodEth0Ifindex:  7,
		CgroupV2Path:    "/kubepods/pod1/cid",
	}})

	resp, err := srv.ExecNetworkChaos(context.Background(), &daemonv1.NetworkChaosRequest{
		ExperimentId: "exp-pod",
		Action:       "delay",
		Namespace:    "default",
		PodName:      "web-0",
		ContainerId:  "abc123",
		Parameters:   map[string]string{"latency": "50ms"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetSuccess() {
		t.Fatalf("expected success, got message: %q", resp.GetMessage())
	}
	if !resp.GetApplied() {
		t.Fatal("expected Applied=true")
	}
	if resp.GetExecutionId() == "" {
		t.Fatal("expected execution ID")
	}
}
