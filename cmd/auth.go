package cmd

import (
	"context"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Credential status (Kalshi has no login flow — keys are created in account settings)",
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show environment, credential state (fingerprint only), and API reachability",
	Long: `Show the selected environment, whether credentials resolve, and —
when a key file is configured — the detected key type and a SHA-256
fingerprint of the PUBLIC key. Private key material is never printed.
Also probes the exchange status endpoint (unauthenticated) for the
selected environment to confirm reachability.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		env := "production"
		if demoSelected() {
			env = "demo"
		}
		fmt.Printf("environment: %s\n", env)
		fmt.Printf("rest base:   %s\n", baseURL())
		fmt.Printf("websocket:   %s\n", wsURL())

		configured, err := credentialsConfigured()
		if err != nil {
			return err
		}
		if !configured {
			fmt.Println("credentials: not configured (public market data still works; " +
				"portfolio/orders/stream need KALSHI_API_KEY_ID + KALSHI_PRIVATE_KEY_PATH)")
		} else {
			s, err := loadSigner()
			if err != nil {
				fmt.Printf("credentials: configured but key failed to load: %v\n", err)
			} else {
				fmt.Printf("credentials: configured\n")
				fmt.Printf("key id:      %s\n", maskKeyID(s.KeyID))
				fmt.Printf("key type:    %s\n", s.KeyType())
				fmt.Printf("key fingerprint: %s\n", s.Fingerprint())
			}
		}

		c := newPublicClient()
		resp, err := c.Get(context.Background(), "/exchange/status", nil, false)
		if err != nil {
			fmt.Printf("reachability: FAILED (%v)\n", err)
			return nil
		}
		if resp.StatusCode != http.StatusOK {
			fmt.Printf("reachability: %s answered HTTP %d\n", baseURL(), resp.StatusCode)
			return nil
		}
		fmt.Printf("reachability: %s OK (exchange status HTTP 200)\n", baseURL())
		return nil
	},
}

// maskKeyID shows only the last 4 characters of a key ID — key IDs are
// identifiers, not secrets, but there is no reason to print them whole.
func maskKeyID(id string) string {
	if len(id) <= 4 {
		return "****"
	}
	return "…" + id[len(id)-4:]
}

func init() {
	authCmd.AddCommand(authStatusCmd)
}
