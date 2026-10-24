// kalshi-cli is a Go command-line client for the Kalshi Trade API v2,
// modeled on schwab-cli / etrade-cli / ibkr-cli. See docs/SPEC.md for the
// endpoint map, auth scheme, and known gaps, and README.md for usage.
package main

import "github.com/rayli09/kalshi-cli/cmd"

func main() {
	cmd.Execute()
}
