package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/daemon"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	addr := flag.String("addr", ":50051", "gRPC listen address")
	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to listen: %v\n", err)
		os.Exit(1)
	}

	srv := grpc.NewServer()
	daemonv1.RegisterChaosDaemonServer(srv, daemon.NewServer())

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(srv, healthSrv)
	healthSrv.SetServingStatus(daemonv1.ChaosDaemon_ServiceName, healthpb.HealthCheckResponse_SERVING)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigCh
		fmt.Println("shutting down gRPC server...")
		srv.GracefulStop()
	}()

	fmt.Printf("chaos-daemon listening on %s\n", *addr)
	if err := srv.Serve(lis); err != nil {
		fmt.Fprintf(os.Stderr, "gRPC serve error: %v\n", err)
		os.Exit(1)
	}
}
