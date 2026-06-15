package cmd

import (
	"context"
	"fmt"
	"os"

	chaosv1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/cli"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var experimentRunCmd = &cobra.Command{
	Use:   "run [name]",
	Short: "Run (unpause) a chaos experiment",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		k, err := cli.NewK8sClient(kubeconfig)
		if err != nil {
			return cli.FormatError(err, "experiment", args[0], namespace)
		}

		var exp chaosv1alpha1.ChaosExperiment
		if err := k.Get(context.Background(), client.ObjectKey{Name: args[0], Namespace: namespace}, &exp); err != nil {
			return cli.FormatError(err, "experiment", args[0], namespace)
		}

		phase := string(exp.Status.Phase)
		if phase == "" {
			phase = "Pending"
		}

		annotations := exp.GetAnnotations()
		if annotations != nil {
			if _, ok := annotations["chaosplane.io/paused"]; ok {
				delete(annotations, "chaosplane.io/paused")
				exp.SetAnnotations(annotations)
				if err := k.Update(context.Background(), &exp); err != nil {
					return cli.FormatError(err, "experiment", args[0], namespace)
				}
				fmt.Fprintf(os.Stdout, "experiment/%s unpaused\n", args[0])
				return nil
			}
		}

		fmt.Fprintf(os.Stdout, "experiment/%s is %s (experiments auto-start on creation)\n", args[0], phase)
		return nil
	},
}
