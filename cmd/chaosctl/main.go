package main

import (
	"fmt"
	"os"

	"github.com/chaosplane-hq/chaosplane/cmd/chaosctl/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
