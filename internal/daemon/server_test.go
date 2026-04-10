package daemon

import (
	"context"
	"testing"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestServerReturnsUnimplemented(t *testing.T) {
	srv := NewServer()
	ctx := context.Background()

	t.Run("ExecNetworkChaos", func(t *testing.T) {
		_, err := srv.ExecNetworkChaos(ctx, &daemonv1.NetworkChaosRequest{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if s, ok := status.FromError(err); !ok || s.Code() != codes.Unimplemented {
			t.Fatalf("expected Unimplemented, got %v", err)
		}
	})

	t.Run("ExecStressChaos", func(t *testing.T) {
		_, err := srv.ExecStressChaos(ctx, &daemonv1.StressChaosRequest{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if s, ok := status.FromError(err); !ok || s.Code() != codes.Unimplemented {
			t.Fatalf("expected Unimplemented, got %v", err)
		}
	})

	t.Run("CancelChaos", func(t *testing.T) {
		_, err := srv.CancelChaos(ctx, &daemonv1.CancelRequest{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if s, ok := status.FromError(err); !ok || s.Code() != codes.Unimplemented {
			t.Fatalf("expected Unimplemented, got %v", err)
		}
	})
}
