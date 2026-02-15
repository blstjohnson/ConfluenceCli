package commands

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"confcli/pkg/confluence"
)

// NewLoginCmd creates the login command
func NewLoginCmd() *cobra.Command {
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Open Confluence login page in browser",
		Long:  `Open the Confluence login page in your default browser for manual authentication`,
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient, err := confluence.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			// Get the base URL from the client
			httpClient, ok := apiClient.GetHTTPClient().(*confluence.HTTPClient)
			if !ok {
				return fmt.Errorf("failed to get HTTP client")
			}
			baseURL := httpClient.GetBaseURL()

			fmt.Printf("Opening Confluence login page in your default browser...\n")
			fmt.Printf("URL: %s\n", baseURL)
			fmt.Printf("Please complete the login process in your browser.\n")
			fmt.Printf("After logging in, you can use the CLI with your Confluence instance.\n")

			// Open the browser to the login page
			var errOpen error
			switch runtime.GOOS {
			case "linux":
				errOpen = confluence.OpenBrowser("xdg-open", baseURL)
			case "windows":
				errOpen = confluence.OpenBrowser("rundll32", "url.dll,FileProtocolHandler", baseURL)
			case "darwin": // macOS
				errOpen = confluence.OpenBrowser("open", baseURL)
			default:
				fmt.Printf("Unsupported platform. Please open the URL manually in your browser.\n")
				return nil
			}

			if errOpen != nil {
				return fmt.Errorf("failed to open browser: %w\nPlease open the URL manually in your browser", errOpen)
			}

			return nil
		},
	}

	return loginCmd
}