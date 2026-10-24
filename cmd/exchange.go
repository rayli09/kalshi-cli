package cmd

import (
	"github.com/spf13/cobra"
)

var exchangeCmd = &cobra.Command{
	Use:   "exchange",
	Short: "Exchange status and schedule (public)",
}

var exchangeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get exchange status (GET /exchange/status)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPublic("/exchange/status", nil)
	},
}

var exchangeScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Get the exchange schedule (GET /exchange/schedule)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPublic("/exchange/schedule", nil)
	},
}

func init() {
	exchangeCmd.AddCommand(exchangeStatusCmd)
	exchangeCmd.AddCommand(exchangeScheduleCmd)
}
