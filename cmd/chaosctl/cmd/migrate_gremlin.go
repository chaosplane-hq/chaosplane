package cmd

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var gremlinFile string
var gremlinOutput string

var migrateGremlinCmd = &cobra.Command{
	Use:   "migrate-gremlin",
	Short: "Convert a Gremlin attack JSON file to a ChaosPlane experiment YAML",
	RunE: func(_ *cobra.Command, _ []string) error {
		if gremlinFile == "" {
			return fmt.Errorf("flag -f / --file is required")
		}

		data, err := os.ReadFile(gremlinFile)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", gremlinFile, err)
		}

		var attack gremlinAttack
		if err := json.Unmarshal(data, &attack); err != nil {
			return fmt.Errorf("parsing Gremlin JSON: %w", err)
		}

		exp, err := convertGremlinAttack(attack)
		if err != nil {
			return fmt.Errorf("converting Gremlin attack: %w", err)
		}

		out, err := yaml.Marshal(exp)
		if err != nil {
			return fmt.Errorf("marshalling YAML: %w", err)
		}

		if gremlinOutput != "" {
			if err := os.WriteFile(gremlinOutput, out, 0o644); err != nil {
				return fmt.Errorf("writing output file: %w", err)
			}
			fmt.Fprintf(os.Stdout, "wrote ChaosPlane experiment to %s\n", gremlinOutput)
			return nil
		}

		fmt.Fprint(os.Stdout, string(out))
		return nil
	},
}

func init() {
	migrateGremlinCmd.Flags().StringVarP(&gremlinFile, "file", "f", "", "path to Gremlin attack JSON file")
	migrateGremlinCmd.Flags().StringVarP(&gremlinOutput, "out", "o", "", "output file path (default: stdout)")
	rootCmd.AddCommand(migrateGremlinCmd)
}

type gremlinAttack struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Command    gremlinCommand    `json:"command"`
	Target     gremlinTarget     `json:"target"`
	ScheduleAt string            `json:"scheduleAt,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type gremlinCommand struct {
	Type       string   `json:"type"`
	Args       []string `json:"args,omitempty"`
	Length     int      `json:"length,omitempty"`
	Percentage int      `json:"percentage,omitempty"`
	Port       int      `json:"port,omitempty"`
	Protocol   string   `json:"protocol,omitempty"`
	Delay      int      `json:"delay,omitempty"`
}

type gremlinTarget struct {
	Type  string            `json:"type"`
	Exact int               `json:"exact,omitempty"`
	Tags  map[string]string `json:"tags,omitempty"`
	Hosts []string          `json:"hosts,omitempty"`
}

var gremlinActionMap = map[string]string{
	"cpu":         "cpu-stress",
	"memory":      "memory-stress",
	"disk":        "disk-fill",
	"io":          "io-stress",
	"shutdown":    "pod-kill",
	"process":     "process-kill",
	"latency":     "network-latency",
	"packet_loss": "network-loss",
	"dns":         "dns-chaos",
	"blackhole":   "network-partition",
}

func convertGremlinAttack(attack gremlinAttack) (map[string]any, error) {
	actionType, ok := gremlinActionMap[strings.ToLower(attack.Command.Type)]
	if !ok {
		actionType = strings.ToLower(attack.Command.Type)
	}

	params := buildGremlinParams(attack.Command)

	duration := attack.Command.Length
	if duration == 0 {
		duration = 60
	}

	name := attack.Name
	if name == "" {
		name = fmt.Sprintf("migrated-gremlin-%s", actionType)
	}

	targetKind := "Pod"
	if attack.Target.Type == "host" {
		targetKind = "Node"
	}

	exp := map[string]any{
		"apiVersion": "chaos.chaosplane.io/v1alpha1",
		"kind":       "ChaosExperiment",
		"metadata": map[string]any{
			"name":   sanitizeName(name),
			"labels": mergeMigrationLabels(attack.Labels),
		},
		"spec": map[string]any{
			"target": buildGremlinTarget(targetKind, attack.Target),
			"action": map[string]any{
				"type":       actionType,
				"parameters": params,
			},
			"duration": fmt.Sprintf("%ds", duration),
			"rollback": map[string]any{
				"enabled": true,
			},
		},
	}

	return exp, nil
}

func buildGremlinParams(cmd gremlinCommand) map[string]any {
	params := make(map[string]any)
	if cmd.Percentage > 0 {
		params["percentage"] = cmd.Percentage
	}
	if cmd.Port > 0 {
		params["port"] = cmd.Port
	}
	if cmd.Protocol != "" {
		params["protocol"] = cmd.Protocol
	}
	if cmd.Delay > 0 {
		params["latencyMs"] = cmd.Delay
	}
	for _, arg := range cmd.Args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			params[parts[0]] = parts[1]
		}
	}
	return params
}

func buildGremlinTarget(kind string, t gremlinTarget) map[string]any {
	target := map[string]any{
		"kind": kind,
	}
	if len(t.Tags) > 0 {
		target["labelSelector"] = map[string]any{
			"matchLabels": t.Tags,
		}
	}
	if len(t.Hosts) > 0 {
		target["names"] = t.Hosts
	}
	return target
}

func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, "-")
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func mergeMigrationLabels(extra map[string]string) map[string]string {
	labels := map[string]string{
		"chaosplane.io/migrated-from": "gremlin",
		"chaosplane.io/migrated-at":   time.Now().UTC().Format(time.RFC3339),
	}
	maps.Copy(labels, extra)
	return labels
}
