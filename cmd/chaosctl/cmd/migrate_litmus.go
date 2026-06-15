package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

var litmusFile string
var litmusOutput string

var migrateLitmusCmd = &cobra.Command{
	Use:   "migrate-litmus",
	Short: "Convert a LitmusChaos experiment YAML to a ChaosPlane experiment YAML",
	RunE: func(_ *cobra.Command, _ []string) error {
		if litmusFile == "" {
			return fmt.Errorf("flag -f / --file is required")
		}

		data, err := os.ReadFile(litmusFile)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", litmusFile, err)
		}

		var raw map[string]any
		if err := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), len(data)).Decode(&raw); err != nil {
			return fmt.Errorf("parsing LitmusChaos YAML: %w", err)
		}

		exp, err := convertLitmusExperiment(raw)
		if err != nil {
			return fmt.Errorf("converting LitmusChaos experiment: %w", err)
		}

		out, err := yaml.Marshal(exp)
		if err != nil {
			return fmt.Errorf("marshalling YAML: %w", err)
		}

		if litmusOutput != "" {
			if err := os.WriteFile(litmusOutput, out, 0o644); err != nil {
				return fmt.Errorf("writing output file: %w", err)
			}
			fmt.Fprintf(os.Stdout, "wrote ChaosPlane experiment to %s\n", litmusOutput)
			return nil
		}

		fmt.Fprint(os.Stdout, string(out))
		return nil
	},
}

func init() {
	migrateLitmusCmd.Flags().StringVarP(&litmusFile, "file", "f", "", "path to LitmusChaos experiment YAML file")
	migrateLitmusCmd.Flags().StringVarP(&litmusOutput, "out", "o", "", "output file path (default: stdout)")
	rootCmd.AddCommand(migrateLitmusCmd)
}

var litmusActionMap = map[string]string{
	"pod-delete":             "pod-kill",
	"pod-kill":               "pod-kill",
	"container-kill":         "container-kill",
	"pod-cpu-hog":            "cpu-stress",
	"pod-memory-hog":         "memory-stress",
	"pod-network-latency":    "network-latency",
	"pod-network-loss":       "network-loss",
	"pod-network-corruption": "network-corruption",
	"node-drain":             "node-drain",
	"node-cpu-hog":           "cpu-stress",
	"node-memory-hog":        "memory-stress",
	"disk-fill":              "disk-fill",
	"pod-io-stress":          "io-stress",
	"pod-dns-error":          "dns-chaos",
}

func convertLitmusExperiment(raw map[string]any) (map[string]any, error) {
	kind := getNestedString(raw, "kind")
	if kind != "ChaosEngine" && kind != "ChaosExperiment" {
		return nil, fmt.Errorf("unsupported LitmusChaos kind %q, expected ChaosEngine or ChaosExperiment", kind)
	}

	metadata := getNestedMap(raw, "metadata")
	spec := getNestedMap(raw, "spec")

	name := getNestedString(metadata, "name")
	ns := getNestedString(metadata, "namespace")

	if kind == "ChaosEngine" {
		return convertChaosEngine(name, ns, spec)
	}
	return convertChaosExperimentDef(name, ns, spec)
}

func convertChaosEngine(name, ns string, spec map[string]any) (map[string]any, error) {
	appInfo := getNestedMap(spec, "appinfo")
	appLabel := getNestedString(appInfo, "applabel")
	appKind := getNestedString(appInfo, "appkind")

	targetKind := "Pod"
	if strings.EqualFold(appKind, "node") {
		targetKind = "Node"
	}

	experiments := getNestedSlice(spec, "experiments")
	expName := ""
	var envVars map[string]any
	if len(experiments) > 0 {
		first := toMap(experiments[0])
		expName = getNestedString(first, "name")
		specMap := getNestedMap(first, "spec")
		components := getNestedMap(specMap, "components")
		envVars = extractEnvMap(getNestedSlice(components, "env"))
	}

	actionType := mapLitmusAction(expName)
	duration := getEnvOrDefault(envVars, "TOTAL_CHAOS_DURATION", "60")
	params := buildLitmusParams(envVars)

	target := map[string]any{
		"kind": targetKind,
	}
	if appLabel != "" {
		parts := strings.SplitN(appLabel, "=", 2)
		if len(parts) == 2 {
			target["labelSelector"] = map[string]any{
				"matchLabels": map[string]string{parts[0]: parts[1]},
			}
		}
	}
	if ns != "" {
		target["namespace"] = ns
	}

	migName := name
	if migName == "" {
		migName = fmt.Sprintf("migrated-litmus-%s", actionType)
	}

	return map[string]any{
		"apiVersion": "chaos.chaosplane.io/v1alpha1",
		"kind":       "ChaosExperiment",
		"metadata": map[string]any{
			"name":      sanitizeName(migName),
			"namespace": ns,
			"labels":    litmusMigrationLabels(),
		},
		"spec": map[string]any{
			"target": target,
			"action": map[string]any{
				"type":       actionType,
				"parameters": params,
			},
			"duration": fmt.Sprintf("%ss", duration),
			"rollback": map[string]any{
				"enabled": true,
			},
		},
	}, nil
}

func convertChaosExperimentDef(name, ns string, spec map[string]any) (map[string]any, error) {
	defn := getNestedMap(spec, "definition")
	scope := getNestedString(defn, "scope")

	targetKind := "Pod"
	if strings.EqualFold(scope, "node") || strings.EqualFold(scope, "cluster") {
		targetKind = "Node"
	}

	envVars := extractEnvMap(getNestedSlice(defn, "env"))
	expName := getNestedString(defn, "name")
	if expName == "" {
		expName = name
	}

	actionType := mapLitmusAction(expName)
	duration := getEnvOrDefault(envVars, "TOTAL_CHAOS_DURATION", "60")
	params := buildLitmusParams(envVars)

	migName := name
	if migName == "" {
		migName = fmt.Sprintf("migrated-litmus-%s", actionType)
	}

	return map[string]any{
		"apiVersion": "chaos.chaosplane.io/v1alpha1",
		"kind":       "ChaosExperiment",
		"metadata": map[string]any{
			"name":      sanitizeName(migName),
			"namespace": ns,
			"labels":    litmusMigrationLabels(),
		},
		"spec": map[string]any{
			"target": map[string]any{
				"kind":      targetKind,
				"namespace": ns,
			},
			"action": map[string]any{
				"type":       actionType,
				"parameters": params,
			},
			"duration": fmt.Sprintf("%ss", duration),
			"rollback": map[string]any{
				"enabled": true,
			},
		},
	}, nil
}

func mapLitmusAction(litmusName string) string {
	actionType, ok := litmusActionMap[strings.ToLower(litmusName)]
	if !ok {
		return strings.ToLower(litmusName)
	}
	return actionType
}

func buildLitmusParams(envVars map[string]any) map[string]any {
	params := make(map[string]any)
	skipKeys := map[string]bool{
		"TOTAL_CHAOS_DURATION": true,
		"CHAOS_INTERVAL":       true,
		"LIB":                  true,
		"RAMP_TIME":            true,
		"SEQUENCE":             true,
	}
	for k, v := range envVars {
		if skipKeys[k] {
			continue
		}
		params[strings.ToLower(k)] = v
	}
	return params
}

func extractEnvMap(envSlice []any) map[string]any {
	result := make(map[string]any)
	for _, item := range envSlice {
		m := toMap(item)
		name := getNestedString(m, "name")
		value := getNestedString(m, "value")
		if name != "" {
			result[name] = value
		}
	}
	return result
}

func getEnvOrDefault(envVars map[string]any, key, fallback string) string {
	if v, ok := envVars[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return fallback
}

func litmusMigrationLabels() map[string]string {
	return map[string]string{
		"chaosplane.io/migrated-from": "litmuschaos",
		"chaosplane.io/migrated-at":   time.Now().UTC().Format(time.RFC3339),
	}
}

func getNestedString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func getNestedMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok {
		return nil
	}
	result, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return result
}

func getNestedSlice(m map[string]any, key string) []any {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok {
		return nil
	}
	result, ok := v.([]any)
	if !ok {
		return nil
	}
	return result
}

func toMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return m
}
