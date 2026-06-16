package netns

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// parsePIDFromInfo extracts the container PID from a CRI ContainerStatus
// verbose info map. containerd places a JSON blob under the "info" key whose
// "pid" field is the container's init PID in the host PID namespace.
func parsePIDFromInfo(info map[string]string) (int, error) {
	raw, ok := info["info"]
	if !ok {
		if p := info["pid"]; p != "" {
			return strconv.Atoi(p)
		}
		return 0, fmt.Errorf("CRI verbose info missing %q key", "info")
	}

	var parsed struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return 0, fmt.Errorf("parse CRI info JSON: %w", err)
	}
	if parsed.PID <= 0 {
		return 0, fmt.Errorf("CRI info reported non-positive pid %d", parsed.PID)
	}
	return parsed.PID, nil
}
