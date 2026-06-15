package cmd

import (
	"context"
	"fmt"
	"strings"

	chaosv1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/cli"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var experimentGetCmd = &cobra.Command{
	Use:   "get [name]",
	Short: "Get chaos experiment details",
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

		headers := []string{"NAME", "NAMESPACE", "TARGET", "ACTION", "DURATION", "PHASE", "MESSAGE"}
		rows := [][]string{{
			exp.Name,
			exp.Namespace,
			fmt.Sprintf("%s/%s", exp.Spec.Target.Kind, strings.Join(exp.Spec.Target.Names, ",")),
			exp.Spec.Action.Type,
			exp.Spec.Duration.Duration.String(),
			phase,
			exp.Status.Message,
		}}

		return cli.PrintObject(outputFmt, exp, headers, rows)
	},
}
