package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	daemonv1 "github.com/chaosplane-hq/chaosplane/gen/daemon/v1"
	"github.com/chaosplane-hq/chaosplane/internal/daemon"
	"github.com/chaosplane-hq/chaosplane/internal/daemon/netns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// resolverFactory mirrors netns.New so configureResolver can be tested with a
// fake that simulates a missing CRI socket.
type resolverFactory func(criEndpoint string) (netns.Resolver, error)

// configureResolver wires the CRI-backed resolver onto the server.
//
// A missing or unreachable CRI socket must NOT crash the daemon: without a
// resolver the server still starts and honestly reports Success=false for
// pod-targeted faults, which is strictly better than taking the whole node's
// chaos daemon offline. We therefore log and continue rather than exiting.
func configureResolver(srv *daemon.Server, criEndpoint string, newResolver resolverFactory, logf func(string, ...any)) {
	if criEndpoint == "" {
		logf("no CRI endpoint configured; pod-targeted faults will report failure until -cri-endpoint is set")
		return
	}
	resolver, err := newResolver(criEndpoint)
	if err != nil {
		logf("netns resolver init failed for %s: %v; daemon will start and report failure for pod-targeted faults", criEndpoint, err)
		return
	}
	srv.SetResolver(resolver)
	logf("pod netns resolution enabled via %s", criEndpoint)
}

func main() {
	addr := flag.String("addr", ":50051", "gRPC listen address")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file")
	tlsKey := flag.String("tls-key", "", "TLS private key file")
	tlsCA := flag.String("tls-ca", "", "TLS CA certificate file for mTLS")
	criEndpoint := flag.String("cri-endpoint", "", "CRI runtime socket (e.g. unix:///run/containerd/containerd.sock) for pod netns/host-veth resolution")
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
	chaosSrv := daemon.NewServer()
	configureResolver(chaosSrv, *criEndpoint, netns.New, log.Printf)
	daemonv1.RegisterChaosDaemonServer(srv, chaosSrv)

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
