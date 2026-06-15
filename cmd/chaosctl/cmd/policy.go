package cmd

import (
	"context"
	"fmt"

	chaosv1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/cli"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var policyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Manage blast radius policies",
}

func init() {
	policyCmd.AddCommand(policyListCmd)
	policyCmd.AddCommand(policyGetCmd)
}

var policyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List blast radius policies",
	RunE: func(_ *cobra.Command, _ []string) error {
		k, err := cli.NewK8sClient(kubeconfig)
		if err != nil {
			return cli.FormatError(err, "BlastRadiusPolicy", "", "")
		}

		var list chaosv1alpha1.BlastRadiusPolicyList
		if err := k.List(context.Background(), &list); err != nil {
			return cli.FormatError(err, "BlastRadiusPolicy", "", "")
		}

		headers := []string{"NAME", "ENFORCEMENT", "MAX TARGETS", "AGE"}
		var rows [][]string
		for _, p := range list.Items {
			maxTargets := "<none>"
			if p.Spec.TargetLimits.MaxTargets != nil {
				maxTargets = fmt.Sprintf("%d", *p.Spec.TargetLimits.MaxTargets)
			}
			rows = append(rows, []string{
				p.Name,
				string(p.Spec.Enforcement),
				maxTargets,
				age(p.CreationTimestamp.Time),
			})
		}

		return cli.PrintObject(outputFmt, list, headers, rows)
	},
}

var policyGetCmd = &cobra.Command{
	Use:   "get [name]",
	Short: "Get blast radius policy details",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		k, err := cli.NewK8sClient(kubeconfig)
		if err != nil {
			return cli.FormatError(err, "BlastRadiusPolicy", args[0], "")
		}

		var policy chaosv1alpha1.BlastRadiusPolicy
		if err := k.Get(context.Background(), client.ObjectKey{Name: args[0]}, &policy); err != nil {
			return cli.FormatError(err, "BlastRadiusPolicy", args[0], "")
		}

		headers := []string{"NAME", "ENFORCEMENT", "MAX TARGETS", "MAX %", "NAMESPACES"}
		maxTargets := "<none>"
		if policy.Spec.TargetLimits.MaxTargets != nil {
			maxTargets = fmt.Sprintf("%d", *policy.Spec.TargetLimits.MaxTargets)
		}
		maxPct := "<none>"
		if policy.Spec.TargetLimits.MaxPercentage != nil {
			maxPct = fmt.Sprintf("%d%%", *policy.Spec.TargetLimits.MaxPercentage)
		}
		ns := "<all>"
		if len(policy.Spec.Scope.Namespaces) > 0 {
			ns = fmt.Sprintf("%v", policy.Spec.Scope.Namespaces)
		}
		rows := [][]string{{policy.Name, string(policy.Spec.Enforcement), maxTargets, maxPct, ns}}

		return cli.PrintObject(outputFmt, policy, headers, rows)
	},
}
