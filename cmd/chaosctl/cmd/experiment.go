package cmd

import (
	"github.com/spf13/cobra"
)

var allNamespaces bool

var experimentCmd = &cobra.Command{
	Use:     "experiment",
	Aliases: []string{"exp"},
	Short:   "Manage chaos experiments",
}

func init() {
	experimentCmd.PersistentFlags().BoolVarP(&allNamespaces, "all-namespaces", "A", false, "list across all namespaces")

	experimentCmd.AddCommand(experimentCreateCmd)
	experimentCmd.AddCommand(experimentListCmd)
	experimentCmd.AddCommand(experimentGetCmd)
	experimentCmd.AddCommand(experimentDeleteCmd)
	experimentCmd.AddCommand(experimentRunCmd)
	experimentCmd.AddCommand(experimentStopCmd)
	experimentCmd.AddCommand(experimentStatusCmd)
}
