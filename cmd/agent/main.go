package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

type AgentConfig struct {
	PlatformURL       string
	Token             string
	HeartbeatInterval time.Duration
	TopologyInterval  time.Duration
}

type registerRequest struct {
	Token        string `json:"token"`
	AgentVersion string `json:"agentVersion"`
	ClusterInfo  string `json:"clusterInfo,omitempty"`
}

type heartbeatRequest struct {
	Token        string `json:"token"`
	AgentVersion string `json:"agentVersion,omitempty"`
	Status       string `json:"status,omitempty"`
}

type topologySnapshot struct {
	EnvironmentID string          `json:"environmentId"`
	Nodes         json.RawMessage `json:"nodes"`
	Namespaces    json.RawMessage `json:"namespaces"`
	Services      json.RawMessage `json:"services"`
	Deployments   json.RawMessage `json:"deployments"`
	Pods          json.RawMessage `json:"pods"`
}

func main() {
	platformURL := flag.String("platform-url", os.Getenv("CHAOSPLANE_PLATFORM_URL"), "Platform API URL")
	token := flag.String("token", os.Getenv("CHAOSPLANE_AGENT_TOKEN"), "Agent authentication token")
	heartbeatSec := flag.Int("heartbeat-interval", 30, "Heartbeat interval in seconds")
	topologySec := flag.Int("topology-interval", 300, "Topology collection interval in seconds")
	version := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *version {
		fmt.Println("chaosplane-agent v1.0.0")
		return
	}

	if *platformURL == "" || *token == "" {
		slog.Error("--platform-url and --token are required (or set CHAOSPLANE_PLATFORM_URL and CHAOSPLANE_AGENT_TOKEN)")
		os.Exit(1)
	}

	cfg := AgentConfig{
		PlatformURL:       *platformURL,
		Token:             *token,
		HeartbeatInterval: time.Duration(*heartbeatSec) * time.Second,
		TopologyInterval:  time.Duration(*topologySec) * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.Info("chaosplane-agent starting", "platform", cfg.PlatformURL, "heartbeat", cfg.HeartbeatInterval, "topology", cfg.TopologyInterval)

	if err := register(ctx, cfg); err != nil {
		slog.Error("failed to register with platform", "error", err)
		os.Exit(1)
	}
	slog.Info("registered with platform")

	go heartbeatLoop(ctx, cfg)
	go topologyLoop(ctx, cfg)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("chaosplane-agent shutting down")
	cancel()
}

func register(ctx context.Context, cfg AgentConfig) error {
	body := registerRequest{
		Token:        cfg.Token,
		AgentVersion: "1.0.0",
	}
	_, err := postJSON(ctx, cfg.PlatformURL+"/agent/register", body)
	return err
}

func heartbeatLoop(ctx context.Context, cfg AgentConfig) {
	ticker := time.NewTicker(cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			body := heartbeatRequest{
				Token:  cfg.Token,
				Status: "connected",
			}
			if _, err := postJSON(ctx, cfg.PlatformURL+"/agent/heartbeat", body); err != nil {
				slog.Warn("heartbeat failed", "error", err)
			}
		}
	}
}

func topologyLoop(ctx context.Context, cfg AgentConfig) {
	ticker := time.NewTicker(cfg.TopologyInterval)
	defer ticker.Stop()

	collectAndSubmit(ctx, cfg)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectAndSubmit(ctx, cfg)
		}
	}
}

func collectAndSubmit(ctx context.Context, cfg AgentConfig) {
	snapshot := collectTopology()
	if _, err := postJSON(ctx, cfg.PlatformURL+"/agent/topology", snapshot); err != nil {
		slog.Warn("topology submission failed", "error", err)
	} else {
		slog.Info("topology submitted")
	}
}

func collectTopology() topologySnapshot {
	nodes := collectKubeResource("nodes")
	namespaces := collectKubeResource("namespaces")
	services := collectKubeResource("services")
	deployments := collectKubeResource("deployments")
	pods := collectKubeResource("pods")

	return topologySnapshot{
		Nodes:       nodes,
		Namespaces:  namespaces,
		Services:    services,
		Deployments: deployments,
		Pods:        pods,
	}
}

func collectKubeResource(resource string) json.RawMessage {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := []string{"get", resource, "-A", "-o", "json"}
	out, err := execKubectl(ctx, args...)
	if err != nil {
		slog.Warn("failed to collect resource", "resource", resource, "error", err)
		return json.RawMessage("[]")
	}

	var result struct {
		Items json.RawMessage `json:"items"`
	}
	if json.Unmarshal([]byte(out), &result) != nil {
		return json.RawMessage("[]")
	}
	return result.Items
}

func execKubectl(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("kubectl %v: %w (stderr: %s)", args, err, stderr.String())
	}
	return stdout.String(), nil
}

func postJSON(ctx context.Context, url string, body interface{}) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
