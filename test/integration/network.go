//go:build integration

package integration

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// NetworkStats holds aggregate results from a ping run against a target.
type NetworkStats struct {
	Transmitted int
	Received    int
	LossPercent float64
	AvgRTT      time.Duration
	MinRTT      time.Duration
	MaxRTT      time.Duration
}

var (
	// busybox ping summary, e.g. "10 packets transmitted, 7 packets received, 30% packet loss"
	pingSummaryRE = regexp.MustCompile(`(\d+) packets transmitted, (\d+) packets received, (\d+(?:\.\d+)?)% packet loss`)
	// busybox rtt line, e.g. "round-trip min/avg/max = 0.045/0.120/0.300 ms"
	pingRTTRE = regexp.MustCompile(`min/avg/max = ([\d.]+)/([\d.]+)/([\d.]+) ms`)
)

// MeasureNetwork runs `ping -c count` from probePod toward target (IP or host)
// and parses loss and RTT. count drives statistical confidence; callers
// comparing before/after a fault should reuse the same count. ping returning a
// non-zero exit (100% loss) is reported as stats, not an error.
func (h *Harness) MeasureNetwork(ctx context.Context, namespace, probePod, target string, count int) (NetworkStats, error) {
	res, err := h.Exec(ctx, namespace, probePod,
		"ping", "-c", strconv.Itoa(count), "-W", "2", target)
	if err != nil {
		return NetworkStats{}, fmt.Errorf("ping %s: %w", target, err)
	}
	return parsePing(res.Stdout)
}

func parsePing(out string) (NetworkStats, error) {
	var stats NetworkStats

	m := pingSummaryRE.FindStringSubmatch(out)
	if m == nil {
		return stats, fmt.Errorf("ping summary not found in output:\n%s", out)
	}
	stats.Transmitted, _ = strconv.Atoi(m[1])
	stats.Received, _ = strconv.Atoi(m[2])
	stats.LossPercent, _ = strconv.ParseFloat(m[3], 64)

	if r := pingRTTRE.FindStringSubmatch(out); r != nil {
		stats.MinRTT = msToDuration(r[1])
		stats.AvgRTT = msToDuration(r[2])
		stats.MaxRTT = msToDuration(r[3])
	}
	return stats, nil
}

func msToDuration(ms string) time.Duration {
	f, err := strconv.ParseFloat(ms, 64)
	if err != nil {
		return 0
	}
	return time.Duration(f * float64(time.Millisecond))
}

// DNSResult reports whether a name resolved from inside the probe pod.
type DNSResult struct {
	Name     string
	Resolved bool
	Output   string
}

// ResolveDNS runs nslookup for name from probePod. A failed lookup is returned
// as Resolved=false (not an error), letting DNS-fault tests assert that a
// specific name fails while a control name still resolves.
func (h *Harness) ResolveDNS(ctx context.Context, namespace, probePod, name string) (DNSResult, error) {
	res, err := h.Exec(ctx, namespace, probePod, "nslookup", name)
	if err != nil {
		return DNSResult{}, fmt.Errorf("nslookup %s: %w", name, err)
	}
	resolved := res.ExitCode == 0 && strings.Contains(res.Stdout, "Address")
	return DNSResult{Name: name, Resolved: resolved, Output: res.Stdout + res.Stderr}, nil
}

// HTTPResult captures the outcome of an HTTP probe from inside a pod.
type HTTPResult struct {
	Success    bool
	StatusCode int
	Latency    time.Duration
	Output     string
}

// ProbeHTTP issues an HTTP GET to url from probePod using wget and reports
// success plus wall-clock latency. wget is used because it ships in busybox;
// HTTP-fault tests assert on Success flipping or Latency rising.
func (h *Harness) ProbeHTTP(ctx context.Context, namespace, probePod, url string) (HTTPResult, error) {
	start := time.Now()
	res, err := h.Exec(ctx, namespace, probePod,
		"wget", "-q", "-T", "5", "-O", "/dev/null", url)
	latency := time.Since(start)
	if err != nil {
		return HTTPResult{}, fmt.Errorf("wget %s: %w", url, err)
	}
	return HTTPResult{
		Success: res.ExitCode == 0,
		Latency: latency,
		Output:  res.Stdout + res.Stderr,
	}, nil
}
