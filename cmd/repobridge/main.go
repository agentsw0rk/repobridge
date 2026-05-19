package main

import (
	"fmt"
	"os"

	"repobridge/internal/cli"
)

var version = "dev"

func main() {
	cmd := cli.NewRootCommand(cli.Options{Version: version})
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
