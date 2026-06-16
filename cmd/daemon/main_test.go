package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/chaosplane-hq/chaosplane/internal/daemon"
	"github.com/chaosplane-hq/chaosplane/internal/daemon/netns"
)

type stubResolver struct{}

func (stubResolver) Resolve(context.Context, netns.PodRef) (*netns.Resolution, error) {
	return &netns.Resolution{}, nil
}

func (stubResolver) ResolveCgroupV2(context.Context, netns.PodRef) (string, error) {
	return "", nil
}

func TestConfigureResolver(t *testing.T) {
	tests := []struct {
		name        string
		endpoint    string
		factoryErr  error
		wantBuilt   bool
		wantLogPart string
	}{
		{
			name:        "no endpoint skips resolver",
			endpoint:    "",
			wantBuilt:   false,
			wantLogPart: "no CRI endpoint",
		},
		{
			name:        "missing socket degrades gracefully",
			endpoint:    "unix:///nonexistent/containerd.sock",
			factoryErr:  fmt.Errorf("dial CRI endpoint: connection refused"),
			wantBuilt:   false,
			wantLogPart: "init failed",
		},
		{
			name:      "valid endpoint wires resolver",
			endpoint:  "unix:///run/containerd/containerd.sock",
			wantBuilt: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := daemon.NewServer()
			built := false
			factory := func(string) (netns.Resolver, error) {
				if tt.factoryErr != nil {
					return nil, tt.factoryErr
				}
				built = true
				return stubResolver{}, nil
			}
			var logged strings.Builder
			logf := func(format string, args ...any) {
				fmt.Fprintf(&logged, format, args...)
			}

			configureResolver(srv, tt.endpoint, factory, logf)

			if built != tt.wantBuilt {
				t.Fatalf("resolver built = %v, want %v", built, tt.wantBuilt)
			}
			if tt.wantLogPart != "" && !strings.Contains(logged.String(), tt.wantLogPart) {
				t.Fatalf("log %q does not contain %q", logged.String(), tt.wantLogPart)
			}
		})
	}
}
