//go:build !linux

package netns

import (
	"fmt"
	"runtime"
)

// New returns an error on non-Linux platforms: namespace, sysfs, and netlink
// resolution are Linux-only. The daemon only runs on Linux nodes; this stub
// exists so the rest of the module builds and tests on other platforms.
func New(_ string) (Resolver, error) {
	return nil, fmt.Errorf("netns resolution is only supported on Linux, not %s", runtime.GOOS)
}
