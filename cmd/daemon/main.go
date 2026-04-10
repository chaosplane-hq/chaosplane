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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	addr := flag.String("addr", ":50051", "gRPC listen address")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file")
	tlsKey := flag.String("tls-key", "", "TLS private key file")
	tlsCA := flag.String("tls-ca", "", "TLS CA certificate file for mTLS")
	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to listen: %v\n", err)
		os.Exit(1)
	}

	var opts []grpc.ServerOption
	if *tlsCert != "" && *tlsKey != "" && *tlsCA != "" {
		tlsConfig, err := daemon.LoadTLSConfig(*tlsCert, *tlsKey, *tlsCA)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load TLS config: %v\n", err)
			os.Exit(1)
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
		fmt.Println("mTLS enabled")
	}

	srv := grpc.NewServer(opts...)
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
