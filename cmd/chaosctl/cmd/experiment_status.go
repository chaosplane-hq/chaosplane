package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	chaosv1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/cli"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var statusWatch bool

var experimentStatusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show experiment status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		k, err := cli.NewK8sClient(kubeconfig)
		if err != nil {
			return cli.FormatError(err, "experiment", args[0], namespace)
		}

		printStatus := func() error {
			var exp chaosv1alpha1.ChaosExperiment
			if err := k.Get(context.Background(), client.ObjectKey{Name: args[0], Namespace: namespace}, &exp); err != nil {
				return cli.FormatError(err, "experiment", args[0], namespace)
			}

			phase := string(exp.Status.Phase)
			if phase == "" {
				phase = "Pending"
			}

			fmt.Fprintf(os.Stdout, "Name:      %s\n", exp.Name)
			fmt.Fprintf(os.Stdout, "Namespace: %s\n", exp.Namespace)
			fmt.Fprintf(os.Stdout, "Phase:     %s\n", phase)
			if exp.Status.Message != "" {
				fmt.Fprintf(os.Stdout, "Message:   %s\n", exp.Status.Message)
			}
			if exp.Status.StartTime != nil {
				fmt.Fprintf(os.Stdout, "Started:   %s\n", exp.Status.StartTime.Format(time.RFC3339))
			}
			if exp.Status.EndTime != nil {
				fmt.Fprintf(os.Stdout, "Ended:     %s\n", exp.Status.EndTime.Format(time.RFC3339))
			}
			if len(exp.Status.AffectedResources) > 0 {
				fmt.Fprintf(os.Stdout, "Affected:  %v\n", exp.Status.AffectedResources)
			}

			for _, c := range exp.Status.Conditions {
				fmt.Fprintf(os.Stdout, "Condition: %s=%s (%s)\n", c.Type, c.Status, c.Message)
			}
			return nil
		}

		if err := printStatus(); err != nil {
			return err
		}

		if !statusWatch {
			return nil
		}

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			fmt.Fprintln(os.Stdout, "---")
			if err := printStatus(); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	experimentStatusCmd.Flags().BoolVarP(&statusWatch, "watch", "w", false, "poll status every 2 seconds")
}
