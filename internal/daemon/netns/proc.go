package netns

import (
	"fmt"
	"strconv"
	"strings"
)

// parseCgroupV2Path extracts the cgroup v2 path from the contents of
// /proc/<pid>/cgroup. Under the unified (v2) hierarchy the relevant line has
// the form "0::<path>"; we return <path> rooted at the cgroup2 mount.
func parseCgroupV2Path(procCgroupContents string) (string, error) {
	for _, line := range strings.Split(strings.TrimSpace(procCgroupContents), "\n") {
		fields := strings.SplitN(line, ":", 3)
		if len(fields) != 3 {
			continue
		}
		if fields[0] == "0" && fields[1] == "" {
			return fields[2], nil
		}
	}
	return "", fmt.Errorf("no cgroup v2 (unified) entry found; cgroup v2 is required")
}

func netnsPathForPID(pid int) string {
	return fmt.Sprintf("/proc/%d/ns/net", pid)
}

func procCgroupPathForPID(pid int) string {
	return fmt.Sprintf("/proc/%d/cgroup", pid)
}

// podSysClassNet returns the pod's sysfs net directory as seen from the host
// via the container's proc root, so eth0's ifindex/iflink can be read WITHOUT
// entering the pod netns.
func podSysClassNet(pid int) string {
	return fmt.Sprintf("/proc/%d/root/sys/class/net", pid)
}

func parseIfindexFile(contents string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(contents))
}
