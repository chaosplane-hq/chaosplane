package cmd

import (
	"context"
	"fmt"
	"os"

	chaosv1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/cli"
	"github.com/spf13/cobra"
)

var experimentDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a chaos experiment",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		k, err := cli.NewK8sClient(kubeconfig)
		if err != nil {
			return cli.FormatError(err, "experiment", args[0], namespace)
		}

		exp := &chaosv1alpha1.ChaosExperiment{}
		exp.Name = args[0]
		exp.Namespace = namespace

		if err := k.Delete(context.Background(), exp); err != nil {
			return cli.FormatError(err, "experiment", args[0], namespace)
		}

		fmt.Fprintf(os.Stdout, "experiment/%s deleted\n", args[0])
		return nil
	},
}
