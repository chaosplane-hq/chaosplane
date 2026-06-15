package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("chaosctl %s (commit: %s, built: %s)\n", version, commit, date)
		fmt.Printf("go: %s, platform: %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	},
}
