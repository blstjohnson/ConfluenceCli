package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version info set via ldflags at build time
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// NewVersionCmd creates the version command
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of confcli",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "confcli %s (commit: %s, built: %s)\n", Version, Commit, Date)
		},
	}
}
