package vm

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/executor"
	"github.com/chaosplane-hq/chaosplane/internal/executor/pod"
)

type SSHConfig struct {
	Host    string
	Port    string
	User    string
	KeyPath string
	Timeout time.Duration
}

func configFromParams(params map[string]string) SSHConfig {
	cfg := SSHConfig{
		Host:    params["sshHost"],
		Port:    params["sshPort"],
		User:    params["sshUser"],
		KeyPath: params["sshKeyPath"],
		Timeout: 30 * time.Second,
	}
	if cfg.Port == "" {
		cfg.Port = "22"
	}
	if cfg.User == "" {
		cfg.User = "root"
	}
	return cfg
}

func sshExec(ctx context.Context, cfg SSHConfig, command string) (string, error) {
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=10",
		"-p", cfg.Port,
	}
	if cfg.KeyPath != "" {
		args = append(args, "-i", cfg.KeyPath)
	}
	args = append(args, fmt.Sprintf("%s@%s", cfg.User, cfg.Host), command)

	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ssh exec failed: %w (stderr: %s)", err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func validateSSHParams(exp *v1alpha1.ChaosExperiment, actionType string, requiredParams ...string) (map[string]string, error) {
	params, err := pod.ParseParameters(exp)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", actionType, err)
	}
	if params["sshHost"] == "" {
		return nil, fmt.Errorf("%s: sshHost parameter is required", actionType)
	}
	for _, p := range requiredParams {
		if params[p] == "" {
			return nil, fmt.Errorf("%s: %s parameter is required", actionType, p)
		}
	}
	return params, nil
}

var _ executor.Executor = (*CPUStressExecutor)(nil)

type CPUStressExecutor struct {
	Logger *slog.Logger
}

func NewCPUStressExecutor(logger *slog.Logger) *CPUStressExecutor {
	return &CPUStressExecutor{Logger: logger}
}

func (e *CPUStressExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateSSHParams(exp, "vm-cpu-stress")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	workers := params["workers"]
	if workers == "" {
		workers = "1"
	}
	cmd := fmt.Sprintf("nohup stress-ng --cpu %s --timeout %ds > /dev/null 2>&1 &", workers, int(exp.Spec.Duration.Seconds()))
	_, err = sshExec(ctx, cfg, cmd)
	if err != nil {
		return fmt.Errorf("vm-cpu-stress: %w", err)
	}
	e.Logger.Info("vm-cpu-stress: started", "host", cfg.Host, "workers", workers)
	return nil
}

func (e *CPUStressExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, _ := pod.ParseParameters(exp)
	cfg := configFromParams(params)
	_, _ = sshExec(ctx, cfg, "pkill -f stress-ng || true")
	return nil
}

func (e *CPUStressExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateSSHParams(exp, "vm-cpu-stress")
	return err
}

var _ executor.Executor = (*MemoryStressExecutor)(nil)

type MemoryStressExecutor struct {
	Logger *slog.Logger
}

func NewMemoryStressExecutor(logger *slog.Logger) *MemoryStressExecutor {
	return &MemoryStressExecutor{Logger: logger}
}

func (e *MemoryStressExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateSSHParams(exp, "vm-memory-stress")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	workers := params["workers"]
	if workers == "" {
		workers = "1"
	}
	size := params["size"]
	if size == "" {
		size = "256M"
	}
	cmd := fmt.Sprintf("nohup stress-ng --vm %s --vm-bytes %s --timeout %ds > /dev/null 2>&1 &", workers, size, int(exp.Spec.Duration.Seconds()))
	_, err = sshExec(ctx, cfg, cmd)
	if err != nil {
		return fmt.Errorf("vm-memory-stress: %w", err)
	}
	e.Logger.Info("vm-memory-stress: started", "host", cfg.Host, "size", size)
	return nil
}

func (e *MemoryStressExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, _ := pod.ParseParameters(exp)
	cfg := configFromParams(params)
	_, _ = sshExec(ctx, cfg, "pkill -f stress-ng || true")
	return nil
}

func (e *MemoryStressExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateSSHParams(exp, "vm-memory-stress")
	return err
}

var _ executor.Executor = (*DiskStressExecutor)(nil)

type DiskStressExecutor struct {
	Logger *slog.Logger
}

func NewDiskStressExecutor(logger *slog.Logger) *DiskStressExecutor {
	return &DiskStressExecutor{Logger: logger}
}

func (e *DiskStressExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateSSHParams(exp, "vm-disk-stress")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	workers := params["workers"]
	if workers == "" {
		workers = "1"
	}
	cmd := fmt.Sprintf("nohup stress-ng --hdd %s --timeout %ds > /dev/null 2>&1 &", workers, int(exp.Spec.Duration.Seconds()))
	_, err = sshExec(ctx, cfg, cmd)
	if err != nil {
		return fmt.Errorf("vm-disk-stress: %w", err)
	}
	e.Logger.Info("vm-disk-stress: started", "host", cfg.Host)
	return nil
}

func (e *DiskStressExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, _ := pod.ParseParameters(exp)
	cfg := configFromParams(params)
	_, _ = sshExec(ctx, cfg, "pkill -f stress-ng || true")
	return nil
}

func (e *DiskStressExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateSSHParams(exp, "vm-disk-stress")
	return err
}

var _ executor.Executor = (*NetworkDelayExecutor)(nil)

type NetworkDelayExecutor struct {
	Logger *slog.Logger
}

func NewNetworkDelayExecutor(logger *slog.Logger) *NetworkDelayExecutor {
	return &NetworkDelayExecutor{Logger: logger}
}

func (e *NetworkDelayExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateSSHParams(exp, "vm-network-delay", "latency", "iface")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	cmd := fmt.Sprintf("tc qdisc add dev %s root netem delay %s", params["iface"], params["latency"])
	_, err = sshExec(ctx, cfg, cmd)
	if err != nil {
		return fmt.Errorf("vm-network-delay: %w", err)
	}
	e.Logger.Info("vm-network-delay: applied", "host", cfg.Host, "latency", params["latency"])
	return nil
}

func (e *NetworkDelayExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, _ := pod.ParseParameters(exp)
	cfg := configFromParams(params)
	iface := params["iface"]
	if iface == "" {
		iface = "eth0"
	}
	_, _ = sshExec(ctx, cfg, fmt.Sprintf("tc qdisc del dev %s root || true", iface))
	return nil
}

func (e *NetworkDelayExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateSSHParams(exp, "vm-network-delay", "latency", "iface")
	return err
}

var _ executor.Executor = (*ProcessKillExecutor)(nil)

type ProcessKillExecutor struct {
	Logger *slog.Logger
}

func NewProcessKillExecutor(logger *slog.Logger) *ProcessKillExecutor {
	return &ProcessKillExecutor{Logger: logger}
}

func (e *ProcessKillExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateSSHParams(exp, "vm-process-kill", "processName")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	signal := params["signal"]
	if signal == "" {
		signal = "SIGKILL"
	}
	cmd := fmt.Sprintf("pkill -%s -f '%s'", signal, params["processName"])
	_, err = sshExec(ctx, cfg, cmd)
	if err != nil {
		return fmt.Errorf("vm-process-kill: %w", err)
	}
	e.Logger.Info("vm-process-kill: executed", "host", cfg.Host, "process", params["processName"])
	return nil
}

func (e *ProcessKillExecutor) Rollback(_ context.Context, _ *v1alpha1.ChaosExperiment) error {
	return nil
}

func (e *ProcessKillExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateSSHParams(exp, "vm-process-kill", "processName")
	return err
}

var _ executor.Executor = (*ProcessSuspendExecutor)(nil)

type ProcessSuspendExecutor struct {
	Logger *slog.Logger
}

func NewProcessSuspendExecutor(logger *slog.Logger) *ProcessSuspendExecutor {
	return &ProcessSuspendExecutor{Logger: logger}
}

func (e *ProcessSuspendExecutor) Execute(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, err := validateSSHParams(exp, "vm-process-suspend", "processName")
	if err != nil {
		return err
	}
	cfg := configFromParams(params)
	cmd := fmt.Sprintf("pkill -SIGSTOP -f '%s'", params["processName"])
	_, err = sshExec(ctx, cfg, cmd)
	if err != nil {
		return fmt.Errorf("vm-process-suspend: %w", err)
	}
	e.Logger.Info("vm-process-suspend: executed", "host", cfg.Host, "process", params["processName"])
	return nil
}

func (e *ProcessSuspendExecutor) Rollback(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	params, _ := pod.ParseParameters(exp)
	cfg := configFromParams(params)
	cmd := fmt.Sprintf("pkill -SIGCONT -f '%s' || true", params["processName"])
	_, _ = sshExec(ctx, cfg, cmd)
	return nil
}

func (e *ProcessSuspendExecutor) Validate(exp *v1alpha1.ChaosExperiment) error {
	_, err := validateSSHParams(exp, "vm-process-suspend", "processName")
	return err
}
