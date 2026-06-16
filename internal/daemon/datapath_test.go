package daemon

import (
	"context"
	"strings"
	"testing"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/daemon/netns"
)

// recordingRunner captures every command issued so tests can assert the exact
// tc/iptables/stress-ng invocation the daemon made.
type recordingRunner struct {
	calls    [][]string
	runErr   error
	startErr error
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	return "", r.runErr
}

func (r *recordingRunner) Start(name string, args ...string) error {
	r.calls = append(r.calls, append([]string{name}, args...))
	return r.startErr
}

func (r *recordingRunner) findCall(name string, mustContain ...string) []string {
	for _, c := range r.calls {
		if c[0] != name {
			continue
		}
		joined := strings.Join(c, " ")
		ok := true
		for _, sub := range mustContain {
			if !strings.Contains(joined, sub) {
				ok = false
				break
			}
		}
		if ok {
			return c
		}
	}
	return nil
}

func resolverForVeth(name string, idx int) fakeResolver {
	return fakeResolver{res: &netns.Resolution{
		ContainerPID:    1234,
		HostVethName:    name,
		HostVethIfindex: idx,
		PodEth0Ifindex:  7,
		CgroupV2Path:    "/kubepods/podabc/cid",
	}}
}

// TestNetworkDelayUsesNetemOnResolvedVeth verifies T6: a delay fault issues a
// tc netem delay against the resolved host-side veth, even when mode=ebpf is
// requested (delay has no eBPF datapath, so it must route to netem).
func TestNetworkDelayUsesNetemOnResolvedVeth(t *testing.T) {
	rr := &recordingRunner{}
	srv := newServerWithRunner(rr)
	srv.SetResolver(resolverForVeth("vethAAA", 11))

	resp, _ := srv.ExecNetworkChaos(context.Background(), &daemonv1.NetworkChaosRequest{
		ExperimentId: "exp-delay", Action: "delay",
		Namespace: "default", PodName: "web-0", ContainerId: "cid",
		Parameters: map[string]string{"latency": "120ms", "mode": "ebpf"},
	})
	if !resp.GetSuccess() || !resp.GetApplied() {
		t.Fatalf("expected success+applied, got %+v", resp)
	}
	call := rr.findCall("tc", "vethAAA")
	if call == nil {
		t.Fatalf("expected tc command on vethAAA, calls: %v", rr.calls)
	}
	if rr.findCall("tc", "netem", "delay", "120ms") == nil {
		t.Fatalf("expected netem delay 120ms, calls: %v", rr.calls)
	}
}

// TestNetworkLossEBPFFallbackToNetem verifies T7: when the real eBPF attach
// fails (no kernel TCX/CAP_BPF in the test env), the daemon falls back to netem
// loss on the resolved veth and still reports the fault applied.
func TestNetworkLossEBPFFallbackToNetem(t *testing.T) {
	rr := &recordingRunner{}
	srv := newServerWithRunner(rr)
	srv.SetResolver(resolverForVeth("vethBBB", 12))

	resp, _ := srv.ExecNetworkChaos(context.Background(), &daemonv1.NetworkChaosRequest{
		ExperimentId: "exp-loss", Action: "loss",
		Namespace: "default", PodName: "web-0", ContainerId: "cid",
		Parameters: map[string]string{"percent": "40", "mode": "ebpf"},
	})
	if !resp.GetSuccess() || !resp.GetApplied() {
		t.Fatalf("expected success+applied via fallback, got %+v", resp)
	}
	if rr.findCall("tc", "vethBBB", "loss") == nil {
		t.Fatalf("expected netem loss fallback on vethBBB, calls: %v", rr.calls)
	}
}

// TestNetworkLossNetemMode verifies the plain (non-ebpf) loss path issues netem
// loss with a percent suffix on the resolved veth.
func TestNetworkLossNetemMode(t *testing.T) {
	rr := &recordingRunner{}
	srv := newServerWithRunner(rr)
	srv.SetResolver(resolverForVeth("vethCCC", 13))

	resp, _ := srv.ExecNetworkChaos(context.Background(), &daemonv1.NetworkChaosRequest{
		ExperimentId: "exp-loss2", Action: "loss",
		Namespace: "default", PodName: "web-0", ContainerId: "cid",
		Parameters: map[string]string{"percent": "25"},
	})
	if !resp.GetSuccess() {
		t.Fatalf("expected success, got %+v", resp)
	}
	if rr.findCall("tc", "vethCCC", "loss", "25%") == nil {
		t.Fatalf("expected netem loss 25%% on vethCCC, calls: %v", rr.calls)
	}
}
