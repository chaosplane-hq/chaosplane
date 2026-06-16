package cluster

import (
	"encoding/json"
	"fmt"

	v1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
)

// ParseParameters unmarshals action parameters from RawExtension into a string map.
func ParseParameters(exp *v1alpha1.ChaosExperiment) (map[string]string, error) {
	params := make(map[string]string)
	if exp.Spec.Action.Parameters.Raw == nil {
		return params, nil
	}
	if err := json.Unmarshal(exp.Spec.Action.Parameters.Raw, &params); err != nil {
		return nil, fmt.Errorf("failed to parse action parameters: %w", err)
	}
	return params, nil
}

// targetName returns the single resource name from the target spec, requiring
// exactly one name since ConfigMap/PVC faults operate on a named object.
func targetName(target v1alpha1.TargetSpec, actionType string) (string, error) {
	if len(target.Names) != 1 {
		return "", fmt.Errorf("%s: target must specify exactly one name", actionType)
	}
	if target.Namespace == "" {
		return "", fmt.Errorf("%s: target namespace is required", actionType)
	}
	return target.Names[0], nil
}
