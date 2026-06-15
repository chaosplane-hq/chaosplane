package cmd

import (
	"context"
	"fmt"

	chaosv1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/cli"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var experimentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List chaos experiments",
	RunE: func(_ *cobra.Command, _ []string) error {
		k, err := cli.NewK8sClient(kubeconfig)
		if err != nil {
			return cli.FormatError(err, "experiment", "", namespace)
		}

		var opts []client.ListOption
		if !allNamespaces {
			opts = append(opts, client.InNamespace(namespace))
		}

		var list chaosv1alpha1.ChaosExperimentList
		if err := k.List(context.Background(), &list, opts...); err != nil {
			return cli.FormatError(err, "experiment", "", namespace)
		}

		headers := []string{"NAME", "NAMESPACE", "ACTION", "PHASE", "AGE"}
		var rows [][]string
		for _, e := range list.Items {
			phase := string(e.Status.Phase)
			if phase == "" {
				phase = "Pending"
			}
			rows = append(rows, []string{
				e.Name,
				e.Namespace,
				e.Spec.Action.Type,
				phase,
				age(e.CreationTimestamp.Time),
			})
		}

		if len(rows) == 0 {
			ns := namespace
			if allNamespaces {
				ns = "all namespaces"
			}
			fmt.Printf("No experiments found in %s\n", ns)
			return nil
		}

		return cli.PrintObject(outputFmt, list, headers, rows)
	},
}
