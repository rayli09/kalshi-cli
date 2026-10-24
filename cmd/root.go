// Package cmd defines the kalshi-cli command tree: one command group per
// Kalshi Trade API v2 area (auth, exchange, series, events, markets,
// portfolio, orders, stream). Every leaf maps to exactly one API
// endpoint — see docs/SPEC.md and README.md for the parity checklist.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rayli09/kalshi-cli/internal/auth"
	"github.com/rayli09/kalshi-cli/internal/client"
)

var (
	flagAPIKeyID       string
	flagPrivateKeyPath string
	flagConfigPath     string
	flagDemo           bool
	flagTimeout        time.Duration
	flagBaseURL        string
	flagWSURL          string
	flagRateLimit      float64
	flagRateBurst      float64
)

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "kalshi-cli",
	Short: "Command-line client for the Kalshi Trade API v2",
	Long: `kalshi-cli is a Go command-line client for the Kalshi Trade API
v2, modeled on schwab-cli / etrade-cli / ibkr-cli.

Public market data (series, events, markets, orderbook, trades,
candlesticks, exchange status) needs no credentials. Portfolio, orders,
and streaming are signed per request with your API key: an Ed25519 key
(the default for keys created since 2026-10-01) or an RSA key
(RSA-PSS/SHA-256), auto-detected from the PEM file. Key material is
never printed; 'auth status' shows only a public-key fingerprint.

Order-mutating commands (orders create, amend, cancel, cancel-batch,
batch-create) are DRY-RUN BY DEFAULT: they print the exact request and
send nothing. Pass --live to actually submit.

Production is the default environment; --demo (or KALSHI_ENV=demo)
targets the demo environment. Keys do not cross environments.`,
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagAPIKeyID, "api-key-id", "",
		"API key ID (env: KALSHI_API_KEY_ID / KALSHI_DEMO_API_KEY_ID, or config file)")
	rootCmd.PersistentFlags().StringVar(&flagPrivateKeyPath, "private-key-path", "",
		"path to the PEM private key file (env: KALSHI_PRIVATE_KEY_PATH / KALSHI_DEMO_PRIVATE_KEY_PATH, or config file)")
	rootCmd.PersistentFlags().StringVar(&flagConfigPath, "config", "",
		"path to a 0600 JSON config file with api_key_id/private_key_path "+
			"(default ~/.kalshi-cli.json when present)")
	rootCmd.PersistentFlags().BoolVar(&flagDemo, "demo", false,
		"use the demo environment (https://external-api.demo.kalshi.co) and its keys")
	rootCmd.PersistentFlags().DurationVar(&flagTimeout, "timeout",
		client.DefaultTimeout, "HTTP timeout for API calls")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url", "",
		"override the REST base URL (env: KALSHI_BASE_URL; for testing)")
	rootCmd.PersistentFlags().StringVar(&flagWSURL, "ws-url", "",
		"override the WebSocket URL (for testing)")
	rootCmd.PersistentFlags().Float64Var(&flagRateLimit, "rate-limit", 10,
		"client-side request rate limit, requests/second (0 disables)")
	rootCmd.PersistentFlags().Float64Var(&flagRateBurst, "rate-burst", 10,
		"client-side rate limiter burst size")

	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(exchangeCmd)
	rootCmd.AddCommand(seriesCmd)
	rootCmd.AddCommand(eventsCmd)
	rootCmd.AddCommand(marketsCmd)
	rootCmd.AddCommand(portfolioCmd)
	rootCmd.AddCommand(ordersCmd)
	rootCmd.AddCommand(streamCmd)
}

// demoSelected reports whether the demo environment is in effect
// (--demo flag or KALSHI_ENV=demo).
func demoSelected() bool {
	return flagDemo || strings.EqualFold(os.Getenv("KALSHI_ENV"), "demo")
}

// configFile mirrors the optional JSON config file. It holds the key ID
// and the *path* to the private key — never key material itself.
type configFile struct {
	APIKeyID       string `json:"api_key_id"`
	PrivateKeyPath string `json:"private_key_path"`
	DemoAPIKeyID   string `json:"demo_api_key_id"`
	DemoKeyPath    string `json:"demo_private_key_path"`
}

func loadConfigFile() (*configFile, error) {
	path := flagConfigPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return &configFile{}, nil
		}
		path = filepath.Join(home, ".kalshi-cli.json")
		if _, err := os.Stat(path); err != nil {
			return &configFile{}, nil // no config file is fine
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "warning: config file %s is readable by "+
			"others (mode %o); it should be 0600\n", path, info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}
	return &cfg, nil
}

// resolveCredentials applies the credential precedence: explicit flags,
// then demo-specific env, generic env, demo config, generic config. It
// never prints the values.
func resolveCredentials() (keyID, keyPath string, err error) {
	cfg, err := loadConfigFile()
	if err != nil {
		return "", "", err
	}
	keyID = flagAPIKeyID
	keyPath = flagPrivateKeyPath
	demo := demoSelected()
	if demo {
		if keyID == "" {
			keyID = os.Getenv("KALSHI_DEMO_API_KEY_ID")
		}
		if keyPath == "" {
			keyPath = os.Getenv("KALSHI_DEMO_PRIVATE_KEY_PATH")
		}
	}
	if keyID == "" {
		keyID = os.Getenv("KALSHI_API_KEY_ID")
	}
	if keyPath == "" {
		keyPath = os.Getenv("KALSHI_PRIVATE_KEY_PATH")
	}
	if demo {
		if keyID == "" {
			keyID = cfg.DemoAPIKeyID
		}
		if keyPath == "" {
			keyPath = cfg.DemoKeyPath
		}
	}
	if keyID == "" {
		keyID = cfg.APIKeyID
	}
	if keyPath == "" {
		keyPath = cfg.PrivateKeyPath
	}
	return keyID, keyPath, nil
}

// credentialsConfigured reports whether both halves of the credential
// pair resolved, without loading the key.
func credentialsConfigured() (bool, error) {
	keyID, keyPath, err := resolveCredentials()
	if err != nil {
		return false, err
	}
	return keyID != "" && keyPath != "", nil
}

// loadSigner resolves credentials and loads the private key. Errors are
// explicit about which half is missing.
func loadSigner() (*auth.Signer, error) {
	keyID, keyPath, err := resolveCredentials()
	if err != nil {
		return nil, err
	}
	if keyID == "" || keyPath == "" {
		return nil, fmt.Errorf("Kalshi API credentials are required: pass " +
			"--api-key-id/--private-key-path, set KALSHI_API_KEY_ID and " +
			"KALSHI_PRIVATE_KEY_PATH (KALSHI_DEMO_* with --demo), or " +
			"create a 0600 config file (see --config)")
	}
	return auth.LoadSigner(keyID, keyPath)
}

// baseURL resolves the REST base URL: explicit override, env, demo/prod.
func baseURL() string {
	if flagBaseURL != "" {
		return strings.TrimSuffix(flagBaseURL, "/")
	}
	if b := os.Getenv("KALSHI_BASE_URL"); b != "" {
		return strings.TrimSuffix(b, "/")
	}
	if demoSelected() {
		return client.DemoBaseURL
	}
	return client.ProdBaseURL
}

// wsURL resolves the WebSocket URL for the selected environment.
func wsURL() string {
	if flagWSURL != "" {
		return flagWSURL
	}
	if demoSelected() {
		return client.DemoWSURL
	}
	return client.ProdWSURL
}

// wsSignPath is the API-root path signed for the WebSocket handshake,
// derived from the WS URL so overrides sign consistently.
func wsSignPath() string {
	u, err := url.Parse(wsURL())
	if err != nil || u.Path == "" {
		return client.APIPathPrefix + "/ws/v2"
	}
	return u.Path
}

// newClient builds a REST client. signer may be nil (public commands);
// signed calls then fail fast inside the client with a clear message.
func newClient(signer *auth.Signer) *client.Client {
	c := client.New(demoSelected(), signer)
	c.BaseURL = baseURL()
	c.HTTP.Timeout = flagTimeout
	c.Limiter = client.NewLimiter(flagRateLimit, flagRateBurst)
	return c
}

// newPublicClient builds a client for unauthenticated reads.
func newPublicClient() *client.Client { return newClient(nil) }

// newSignedClient builds a client with credentials loaded.
func newSignedClient() (*client.Client, error) {
	s, err := loadSigner()
	if err != nil {
		return nil, err
	}
	return newClient(s), nil
}

// printResponse pretty-prints a JSON response body (verbatim otherwise)
// and converts error statuses into errors.
func printResponse(resp *client.Response) error {
	if len(resp.Body) > 0 {
		if json.Valid(resp.Body) {
			var buf json.RawMessage = resp.Body
			out, err := json.MarshalIndent(buf, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(out))
		} else {
			fmt.Println(string(resp.Body))
		}
	}
	if resp.IsError() {
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}
	return nil
}

// runPublic executes one unauthenticated GET and prints the response.
func runPublic(endpoint string, query url.Values) error {
	resp, err := newPublicClient().Get(context.Background(), endpoint, query, false)
	if err != nil {
		return err
	}
	return printResponse(resp)
}

// runSigned executes one authenticated GET and prints the response.
func runSigned(endpoint string, query url.Values) error {
	c, err := newSignedClient()
	if err != nil {
		return err
	}
	resp, err := c.Get(context.Background(), endpoint, query, true)
	if err != nil {
		return err
	}
	return printResponse(resp)
}

// setStr adds a query parameter only when non-empty.
func setStr(q url.Values, name, v string) {
	if v != "" {
		q.Set(name, v)
	}
}

// setInt adds a query parameter only when non-zero.
func setInt(q url.Values, name string, v int) {
	if v != 0 {
		q.Set(name, fmt.Sprint(v))
	}
}

// setInt64 adds a query parameter only when non-zero.
func setInt64(q url.Values, name string, v int64) {
	if v != 0 {
		q.Set(name, fmt.Sprint(v))
	}
}

// dryRunPreview prints the request a mutating command would send. This
// is the entirety of a dry run: nothing is transmitted.
func dryRunPreview(method, endpoint string, body []byte) error {
	fmt.Println("DRY RUN — nothing was sent. Pass --live to submit this request:")
	fmt.Printf("%s %s%s\n", method, baseURL(), endpoint)
	if body != nil {
		var buf json.RawMessage = body
		out, err := json.MarshalIndent(buf, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
	}
	return nil
}

// runMutation executes one signed mutating request and prints the
// response. Callers gate this behind --live.
func runMutation(method, endpoint string, body []byte) error {
	c, err := newSignedClient()
	if err != nil {
		return err
	}
	resp, err := c.Do(context.Background(), method, endpoint, nil, body, true)
	if err != nil {
		return err
	}
	return printResponse(resp)
}
