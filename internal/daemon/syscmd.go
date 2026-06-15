package daemon

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func execCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s failed: %w (stderr: %s)", name, strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func tcAddNetem(iface, action string, params map[string]string) error {
	args := []string{"qdisc", "add", "dev", iface, "root", "netem"}

	switch action {
	case "delay":
		latency := params["latency"]
		if latency == "" {
			latency = "100ms"
		}
		args = append(args, "delay", latency)
		if jitter := params["jitter"]; jitter != "" {
			args = append(args, jitter)
		}
		if corr := params["correlation"]; corr != "" {
			args = append(args, corr)
		}
	case "loss":
		percent := params["percent"]
		if percent == "" {
			percent = "10%"
		}
		if !strings.HasSuffix(percent, "%") {
			percent += "%"
		}
		args = append(args, "loss", percent)
	case "corrupt":
		percent := params["percent"]
		if percent == "" {
			percent = "10%"
		}
		if !strings.HasSuffix(percent, "%") {
			percent += "%"
		}
		args = append(args, "corrupt", percent)
	case "duplicate":
		percent := params["percent"]
		if percent == "" {
			percent = "10%"
		}
		if !strings.HasSuffix(percent, "%") {
			percent += "%"
		}
		args = append(args, "duplicate", percent)
	case "bandwidth":
		rate := params["rate"]
		if rate == "" {
			rate = "1mbit"
		}
		args = []string{"qdisc", "add", "dev", iface, "root", "tbf", "rate", rate, "burst", "32kbit", "latency", "400ms"}
	}

	_, err := execCmd(context.Background(), "tc", args...)
	return err
}

func tcDelete(iface string) error {
	_, _ = execCmd(context.Background(), "tc", "qdisc", "del", "dev", iface, "root")
	return nil
}

func iptablesBlock(iface, direction string) error {
	chain := "OUTPUT"
	if direction == "ingress" || direction == "both" {
		chain = "INPUT"
	}
	_, err := execCmd(context.Background(), "iptables", "-A", chain, "-i", iface, "-j", "DROP")
	if err != nil {
		return err
	}
	if direction == "both" {
		_, err = execCmd(context.Background(), "iptables", "-A", "OUTPUT", "-o", iface, "-j", "DROP")
	}
	return err
}

func iptablesUnblock(iface, direction string) error {
	chain := "OUTPUT"
	if direction == "ingress" || direction == "both" {
		chain = "INPUT"
	}
	_, _ = execCmd(context.Background(), "iptables", "-D", chain, "-i", iface, "-j", "DROP")
	if direction == "both" {
		_, _ = execCmd(context.Background(), "iptables", "-D", "OUTPUT", "-o", iface, "-j", "DROP")
	}
	return nil
}

func stressNGStart(stressorType string, params map[string]string, durationSec int) error {
	args := []string{}
	switch stressorType {
	case "cpu":
		workers := params["workers"]
		if workers == "" {
			workers = "1"
		}
		args = append(args, "--cpu", workers)
		if load := params["load"]; load != "" {
			args = append(args, "--cpu-load", load)
		}
	case "memory", "vm":
		workers := params["workers"]
		if workers == "" {
			workers = "1"
		}
		size := params["size"]
		if size == "" {
			size = "256M"
		}
		args = append(args, "--vm", workers, "--vm-bytes", size)
	case "io", "hdd":
		workers := params["workers"]
		if workers == "" {
			workers = "1"
		}
		args = append(args, "--hdd", workers)
	}

	if durationSec > 0 {
		args = append(args, "--timeout", fmt.Sprintf("%ds", durationSec))
	}

	cmd := exec.Command("stress-ng", args...)
	return cmd.Start()
}

func stressNGStop() {
	_, _ = execCmd(context.Background(), "pkill", "-f", "stress-ng")
}

func dnsIntercept(action string, params map[string]string) error {
	domains := params["domains"]
	if domains == "" {
		return fmt.Errorf("domains parameter required for dns chaos")
	}

	for _, domain := range strings.Split(domains, ",") {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		switch action {
		case "error":
			_, err := execCmd(context.Background(), "iptables", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-m", "string", "--string", domain, "--algo", "bm", "-j", "DROP")
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func dnsRestore(params map[string]string) {
	domains := params["domains"]
	for _, domain := range strings.Split(domains, ",") {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		_, _ = execCmd(context.Background(), "iptables", "-D", "OUTPUT", "-p", "udp", "--dport", "53", "-m", "string", "--string", domain, "--algo", "bm", "-j", "DROP")
	}
}
