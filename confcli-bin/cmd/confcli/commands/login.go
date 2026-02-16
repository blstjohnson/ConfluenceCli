package commands

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

// NewLoginCmd creates the login command
func NewLoginCmd() *cobra.Command {
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Open Confluence login page in browser",
		Long:  `Open the Confluence login page in your default browser for manual authentication`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get the base URL from configuration
			baseURL := "https://your-confluence-instance.atlassian.net/wiki"
			// TODO: Get the actual base URL from configuration

			fmt.Printf("Opening Confluence login page in your default browser...\n")
			fmt.Printf("URL: %s\n", baseURL)
			fmt.Printf("Please complete the login process in your browser.\n")
			fmt.Printf("After logging in, you can use the CLI with your Confluence instance.\n")

			// Open the browser to the login page
			var errOpen error
			switch runtime.GOOS {
			case "linux":
				errOpen = exec.Command("xdg-open", baseURL).Start()
			case "windows":
				errOpen = exec.Command("rundll32", "url.dll,FileProtocolHandler", baseURL).Start()
			case "darwin": // macOS
				errOpen = exec.Command("open", baseURL).Start()
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
