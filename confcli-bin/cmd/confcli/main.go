package main

import (
	"os"

	"confcli/internal/commands"
)

func main() {
	cmd := commands.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}