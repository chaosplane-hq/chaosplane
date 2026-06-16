package daemon

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// commandRunner is the seam between the daemon's fault logic and the host's
// command-line tools (tc, iptables, stress-ng). Injecting a fake runner in
// tests lets us assert failure propagation without touching the real host.
type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
	Start(name string, args ...string) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
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

func (execRunner) Start(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}

type sysOps struct {
	runner commandRunner
}

func newSysOps(r commandRunner) *sysOps {
	return &sysOps{runner: r}
}

func (s *sysOps) tcAddNetem(iface, action string, params map[string]string) error {
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

	_, err := s.runner.Run(context.Background(), "tc", args...)
	return err
}

func (s *sysOps) tcDelete(iface string) error {
	_, _ = s.runner.Run(context.Background(), "tc", "qdisc", "del", "dev", iface, "root")
	return nil
}

// httpAbortRuleBody returns the iptables rule body (after -A/-D) that REJECTs
// TCP traffic to an HTTP port. Scoped to a pod it lives in FORWARD matched on
// the pod-bound direction of its host-side veth (-o iface), so only requests
// reaching that pod's port are aborted; unscoped it falls back to the
// daemon-local INPUT chain.
func httpAbortRuleBody(iface, portStr string, scoped bool) []string {
	if scoped {
		return []string{"FORWARD", "-o", iface, "-p", "tcp", "--dport", portStr, "-j", "REJECT"}
	}
	return []string{"INPUT", "-p", "tcp", "--dport", portStr, "-j", "REJECT"}
}

func (s *sysOps) httpAbort(target podTarget, port int) error {
	body := httpAbortRuleBody(target.hostVeth, strconv.Itoa(port), target.scoped)
	_, err := s.runner.Run(context.Background(), "iptables", append([]string{"-A"}, body...)...)
	return err
}

func (s *sysOps) httpDelay(target podTarget, delayMS string) error {
	iface := "lo"
	if target.scoped {
		iface = target.hostVeth
	}
	if delayMS == "" {
		delayMS = "100"
	}
	_, err := s.runner.Run(context.Background(), "tc", "qdisc", "add", "dev", iface, "root", "netem", "delay", delayMS+"ms")
	return err
}

func (s *sysOps) httpRestore(params map[string]string) {
	scoped := params["iface"] != ""
	iface := params["iface"]
	switch params["action"] {
	case "abort":
		if port := params["port"]; port != "" {
			body := httpAbortRuleBody(iface, port, scoped)
			_, _ = s.runner.Run(context.Background(), "iptables", append([]string{"-D"}, body...)...)
		}
	case "delay":
		if scoped {
			_ = s.tcDelete(iface)
		} else {
			_ = s.tcDelete("lo")
		}
	}
}

func (s *sysOps) iptablesBlock(iface, direction string) error {
	chain := "OUTPUT"
	if direction == "ingress" || direction == "both" {
		chain = "INPUT"
	}
	_, err := s.runner.Run(context.Background(), "iptables", "-A", chain, "-i", iface, "-j", "DROP")
	if err != nil {
		return err
	}
	if direction == "both" {
		_, err = s.runner.Run(context.Background(), "iptables", "-A", "OUTPUT", "-o", iface, "-j", "DROP")
	}
	return err
}

func (s *sysOps) iptablesUnblock(iface, direction string) error {
	chain := "OUTPUT"
	if direction == "ingress" || direction == "both" {
		chain = "INPUT"
	}
	_, _ = s.runner.Run(context.Background(), "iptables", "-D", chain, "-i", iface, "-j", "DROP")
	if direction == "both" {
		_, _ = s.runner.Run(context.Background(), "iptables", "-D", "OUTPUT", "-o", iface, "-j", "DROP")
	}
	return nil
}

// partitionRules builds the iptables FORWARD rules that partition a pod from a
// CIDR on its host-side veth. Pod traffic is forwarded through the host netns,
// so rules live in FORWARD keyed on the veth:
//
//	egress  (pod -> cidr): -i veth -d cidr  (packets entering the host from the pod)
//	ingress (cidr -> pod): -o veth -s cidr  (packets leaving the host toward the pod)
//
// Each returned slice is the rule body that follows the -A/-D verb.
func partitionRules(iface, direction, cidr string) [][]string {
	egress := []string{"FORWARD", "-i", iface, "-d", cidr, "-j", "DROP"}
	ingress := []string{"FORWARD", "-o", iface, "-s", cidr, "-j", "DROP"}
	switch direction {
	case "egress":
		return [][]string{egress}
	case "ingress":
		return [][]string{ingress}
	default:
		return [][]string{egress, ingress}
	}
}

func (s *sysOps) partition(iface, direction, cidr string) error {
	if cidr == "" {
		return fmt.Errorf("partition requires target_cidr")
	}
	if direction == "" {
		direction = "both"
	}
	for _, rule := range partitionRules(iface, direction, cidr) {
		args := append([]string{"-A"}, rule...)
		if _, err := s.runner.Run(context.Background(), "iptables", args...); err != nil {
			return err
		}
	}
	return nil
}

func (s *sysOps) partitionRestore(iface, direction, cidr string) {
	if cidr == "" {
		return
	}
	if direction == "" {
		direction = "both"
	}
	for _, rule := range partitionRules(iface, direction, cidr) {
		args := append([]string{"-D"}, rule...)
		_, _ = s.runner.Run(context.Background(), "iptables", args...)
	}
}

func (s *sysOps) stressNGStart(stressorType string, params map[string]string, durationSec int, cgroupPath string) error {
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

	if cgroupPath != "" {
		return s.startInCgroup(cgroupPath, "stress-ng", args)
	}
	return s.runner.Start("stress-ng", args...)
}

// startInCgroup launches a process inside an existing cgroup v2 by moving the
// shell into the cgroup before exec. Writing the shell's own PID to
// cgroup.procs migrates it (and the exec'd child, since it keeps the PID) into
// the pod's cgroup, so stress-ng runs under the pod's cpu/memory accounting and
// limits. cgroupPath is the pod's path from the netns resolver. This needs no
// privileged container, only write access to the host cgroup2 hierarchy.
func (s *sysOps) startInCgroup(cgroupPath, name string, args []string) error {
	procs := cgroupProcsFile(cgroupPath)
	quoted := append([]string{name}, args...)
	cmd := fmt.Sprintf("echo $$ > %s && exec %s", procs, strings.Join(quoted, " "))
	return s.runner.Start("sh", "-c", cmd)
}

const cgroupV2Mount = "/sys/fs/cgroup"

// cgroupProcsFile joins the cgroup2 mount with the pod's relative cgroup path
// and the cgroup.procs control file processes are written to for migration.
func cgroupProcsFile(cgroupPath string) string {
	trimmed := strings.TrimPrefix(cgroupPath, "/")
	return cgroupV2Mount + "/" + trimmed + "/cgroup.procs"
}

func (s *sysOps) stressNGStop() {
	_, _ = s.runner.Run(context.Background(), "pkill", "-f", "stress-ng")
}

// dnsRuleBodies returns the iptables rule bodies (after the -A/-D verb) that
// intercept DNS for a single domain. When scoped to a pod, the rule lives in
// FORWARD matched on the pod's host-side veth (-i iface) so only that pod's
// queries are affected; the daemon's own resolution is untouched. Unscoped, it
// falls back to the daemon-local OUTPUT chain. Both UDP and TCP port 53 are
// matched because resolvers retry over TCP for large or truncated answers.
func dnsRuleBodies(iface, domain string, scoped bool) [][]string {
	chain, scopeMatch := "OUTPUT", []string{}
	if scoped {
		chain, scopeMatch = "FORWARD", []string{"-i", iface}
	}
	bodies := make([][]string, 0, 2)
	for _, proto := range []string{"udp", "tcp"} {
		body := append([]string{chain, "-p", proto}, scopeMatch...)
		body = append(body, "--dport", "53", "-m", "string", "--string", domain, "--algo", "bm", "-j", "DROP")
		bodies = append(bodies, body)
	}
	return bodies
}

func (s *sysOps) dnsIntercept(action string, target podTarget, params map[string]string) error {
	domains := params["domains"]
	if domains == "" {
		return fmt.Errorf("domains parameter required for dns chaos")
	}
	if action != "error" {
		return fmt.Errorf("unsupported dns chaos action: %s", action)
	}

	for _, domain := range strings.Split(domains, ",") {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		for _, body := range dnsRuleBodies(target.hostVeth, domain, target.scoped) {
			args := append([]string{"-A"}, body...)
			if _, err := s.runner.Run(context.Background(), "iptables", args...); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *sysOps) dnsRestore(params map[string]string) {
	domains := params["domains"]
	scoped := params["iface"] != ""
	for _, domain := range strings.Split(domains, ",") {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		for _, body := range dnsRuleBodies(params["iface"], domain, scoped) {
			args := append([]string{"-D"}, body...)
			_, _ = s.runner.Run(context.Background(), "iptables", args...)
		}
	}
}
