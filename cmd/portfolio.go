package cmd

import (
	"net/url"

	"github.com/spf13/cobra"
)

var portfolioCmd = &cobra.Command{
	Use:   "portfolio",
	Short: "Account balance, positions, fills, settlements (signed)",
}

var portfolioBalanceCmd = &cobra.Command{
	Use:   "balance",
	Short: "Get account balance (GET /portfolio/balance)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSigned("/portfolio/balance", nil)
	},
}

var (
	positionsLimit            int
	positionsCursor           string
	positionsTicker           string
	positionsEventTicker      string
	positionsCountFilter      string
	positionsSettlementStatus string
)

var portfolioPositionsCmd = &cobra.Command{
	Use:   "positions",
	Short: "List positions (GET /portfolio/positions)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		setInt(q, "limit", positionsLimit)
		setStr(q, "cursor", positionsCursor)
		setStr(q, "ticker", positionsTicker)
		setStr(q, "event_ticker", positionsEventTicker)
		setStr(q, "count_filter", positionsCountFilter)
		setStr(q, "settlement_status", positionsSettlementStatus)
		return runSigned("/portfolio/positions", q)
	},
}

var (
	fillsLimit   int
	fillsCursor  string
	fillsTicker  string
	fillsOrderID string
	fillsMinTS   int64
	fillsMaxTS   int64
)

var portfolioFillsCmd = &cobra.Command{
	Use:   "fills",
	Short: "List fills (GET /portfolio/fills)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		setInt(q, "limit", fillsLimit)
		setStr(q, "cursor", fillsCursor)
		setStr(q, "ticker", fillsTicker)
		setStr(q, "order_id", fillsOrderID)
		setInt64(q, "min_ts", fillsMinTS)
		setInt64(q, "max_ts", fillsMaxTS)
		return runSigned("/portfolio/fills", q)
	},
}

var (
	settlementsLimit       int
	settlementsCursor      string
	settlementsTicker      string
	settlementsEventTicker string
	settlementsMinTS       int64
	settlementsMaxTS       int64
)

var portfolioSettlementsCmd = &cobra.Command{
	Use:   "settlements",
	Short: "List settlements (GET /portfolio/settlements)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		setInt(q, "limit", settlementsLimit)
		setStr(q, "cursor", settlementsCursor)
		setStr(q, "ticker", settlementsTicker)
		setStr(q, "event_ticker", settlementsEventTicker)
		setInt64(q, "min_ts", settlementsMinTS)
		setInt64(q, "max_ts", settlementsMaxTS)
		return runSigned("/portfolio/settlements", q)
	},
}

func init() {
	portfolioPositionsCmd.Flags().IntVar(&positionsLimit, "limit", 0, "max results")
	portfolioPositionsCmd.Flags().StringVar(&positionsCursor, "cursor", "", "pagination cursor")
	portfolioPositionsCmd.Flags().StringVar(&positionsTicker, "ticker", "", "filter by market ticker")
	portfolioPositionsCmd.Flags().StringVar(&positionsEventTicker, "event-ticker", "", "filter by event ticker")
	portfolioPositionsCmd.Flags().StringVar(&positionsCountFilter, "count-filter", "", "count filter: position, total_traded (comma-separated)")
	portfolioPositionsCmd.Flags().StringVar(&positionsSettlementStatus, "settlement-status", "", "filter: all, unsettled, settled")
	portfolioFillsCmd.Flags().IntVar(&fillsLimit, "limit", 0, "max results")
	portfolioFillsCmd.Flags().StringVar(&fillsCursor, "cursor", "", "pagination cursor")
	portfolioFillsCmd.Flags().StringVar(&fillsTicker, "ticker", "", "filter by market ticker")
	portfolioFillsCmd.Flags().StringVar(&fillsOrderID, "order-id", "", "filter by order ID")
	portfolioFillsCmd.Flags().Int64Var(&fillsMinTS, "min-ts", 0, "minimum fill timestamp (Unix seconds)")
	portfolioFillsCmd.Flags().Int64Var(&fillsMaxTS, "max-ts", 0, "maximum fill timestamp (Unix seconds)")
	portfolioSettlementsCmd.Flags().IntVar(&settlementsLimit, "limit", 0, "max results")
	portfolioSettlementsCmd.Flags().StringVar(&settlementsCursor, "cursor", "", "pagination cursor")
	portfolioSettlementsCmd.Flags().StringVar(&settlementsTicker, "ticker", "", "filter by market ticker")
	portfolioSettlementsCmd.Flags().StringVar(&settlementsEventTicker, "event-ticker", "", "filter by event ticker")
	portfolioSettlementsCmd.Flags().Int64Var(&settlementsMinTS, "min-ts", 0, "minimum settlement timestamp (Unix seconds)")
	portfolioSettlementsCmd.Flags().Int64Var(&settlementsMaxTS, "max-ts", 0, "maximum settlement timestamp (Unix seconds)")
	portfolioCmd.AddCommand(portfolioBalanceCmd)
	portfolioCmd.AddCommand(portfolioPositionsCmd)
	portfolioCmd.AddCommand(portfolioFillsCmd)
	portfolioCmd.AddCommand(portfolioSettlementsCmd)
}
