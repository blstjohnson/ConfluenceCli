package main

import (
	"os"

	"confcli/cmd/confcli/commands"
)

func main() {
	// Clean up .old binary from a previous Windows update
	commands.CleanupOldBinary()

	cmd := commands.NewRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}