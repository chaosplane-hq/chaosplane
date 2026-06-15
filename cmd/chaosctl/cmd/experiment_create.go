package cmd

import (
	"context"
	"fmt"
	"os"

	chaosv1alpha1 "github.com/chaosplane-hq/chaosplane/api/v1alpha1"
	"github.com/chaosplane-hq/chaosplane/internal/cli"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var createFile string

var experimentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a chaos experiment from a YAML file",
	RunE: func(_ *cobra.Command, _ []string) error {
		if createFile == "" {
			return fmt.Errorf("flag -f / --file is required")
		}

		data, err := os.ReadFile(createFile)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", createFile, err)
		}

		var exp chaosv1alpha1.ChaosExperiment
		if err := yaml.UnmarshalStrict(data, &exp); err != nil {
			return fmt.Errorf("parsing YAML: %w", err)
		}

		if exp.Namespace == "" {
			exp.Namespace = namespace
		}

		k, err := cli.NewK8sClient(kubeconfig)
		if err != nil {
			return cli.FormatError(err, "experiment", exp.Name, exp.Namespace)
		}

		if err := k.Create(context.Background(), &exp); err != nil {
			return cli.FormatError(err, "experiment", exp.Name, exp.Namespace)
		}

		fmt.Fprintf(os.Stdout, "experiment/%s created\n", exp.Name)
		return nil
	},
}

func init() {
	experimentCreateCmd.Flags().StringVarP(&createFile, "file", "f", "", "path to experiment YAML file")
}
