package netns

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type criRuntimeClient struct {
	conn    *grpc.ClientConn
	runtime runtimeapi.RuntimeServiceClient
}

func newCRIRuntimeClient(endpoint string) (*criRuntimeClient, error) {
	target := strings.TrimPrefix(endpoint, "unix://")
	conn, err := grpc.NewClient(
		"unix://"+target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial CRI endpoint %s: %w", endpoint, err)
	}
	return &criRuntimeClient{conn: conn, runtime: runtimeapi.NewRuntimeServiceClient(conn)}, nil
}

func (c *criRuntimeClient) close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// containerInfo asks the CRI runtime for the container's verbose status, whose
// info["pid"] and cgroup path expose the host-PID-namespace PID and cgroup v2
// path without entering the container.
func (c *criRuntimeClient) containerInfo(ctx context.Context, containerID string) (containerInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := c.runtime.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
	if err != nil {
		return containerInfo{}, fmt.Errorf("CRI ContainerStatus %s: %w", containerID, err)
	}

	pid, err := parsePIDFromInfo(resp.GetInfo())
	if err != nil {
		return containerInfo{}, fmt.Errorf("container %s: %w", containerID, err)
	}
	return containerInfo{pid: pid}, nil
}
