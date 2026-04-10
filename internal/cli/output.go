package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"sigs.k8s.io/yaml"
)

var Output io.Writer = os.Stdout

func PrintTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(Output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

func PrintJSON(obj interface{}) error {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling JSON: %w", err)
	}
	fmt.Fprintln(Output, string(data))
	return nil
}

func PrintYAML(obj interface{}) error {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("marshalling YAML: %w", err)
	}
	fmt.Fprint(Output, string(data))
	return nil
}

func PrintObject(format string, obj interface{}, tableHeaders []string, tableRows [][]string) error {
	switch format {
	case "json":
		return PrintJSON(obj)
	case "yaml":
		return PrintYAML(obj)
	default:
		PrintTable(tableHeaders, tableRows)
		return nil
	}
}
