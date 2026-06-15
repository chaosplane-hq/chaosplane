package cmd

import (
	"github.com/spf13/cobra"
)

var (
	kubeconfig string
	namespace  string
	outputFmt  string
)

var rootCmd = &cobra.Command{
	Use:   "chaosctl",
	Short: "ChaosPlane CLI — manage chaos experiments and policies",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file (default: $KUBECONFIG or ~/.kube/config)")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "default", "target namespace")
	rootCmd.PersistentFlags().StringVarP(&outputFmt, "output", "o", "table", "output format: table, json, yaml")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(experimentCmd)
	rootCmd.AddCommand(policyCmd)
}
