package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"confcli/internal/client"
)

// NewLoginCmd creates the login command
func NewLoginCmd() *cobra.Command {
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Confluence using browser",
		Long:  `Open a browser to authenticate with Confluence and establish a session`,
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient, err := client.NewClient()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()
			
			fmt.Println("Starting browser-based authentication...")
			err = apiClient.AuthenticateViaBrowser(ctx)
			if err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}

			fmt.Println("Authentication successful! You can now use the CLI with your Confluence instance.")
			return nil
		},
	}

	return loginCmd
}