package main

import (
	"os"

	"confcli/cmd/confcli/commands"
)

func main() {
	cmd := commands.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}