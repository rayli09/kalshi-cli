package cmd

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

var marketsCmd = &cobra.Command{
	Use:   "markets",
	Short: "Markets, orderbook, trades, candlesticks (public)",
}

var (
	marketsListLimit        int
	marketsListCursor       string
	marketsListStatus       string
	marketsListSeriesTicker string
	marketsListEventTicker  string
	marketsListTickers      string

	marketsOrderbookDepth int

	marketsTradesTicker string
	marketsTradesLimit  int
	marketsTradesCursor string
	marketsTradesMinTS  int64
	marketsTradesMaxTS  int64

	marketsCandlesStart  int64
	marketsCandlesEnd    int64
	marketsCandlesPeriod int
)

var marketsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List markets (GET /markets)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		setInt(q, "limit", marketsListLimit)
		setStr(q, "cursor", marketsListCursor)
		setStr(q, "status", marketsListStatus)
		setStr(q, "series_ticker", marketsListSeriesTicker)
		setStr(q, "event_ticker", marketsListEventTicker)
		setStr(q, "tickers", marketsListTickers)
		return runPublic("/markets", q)
	},
}

var marketsGetCmd = &cobra.Command{
	Use:   "get <ticker>",
	Short: "Get one market (GET /markets/{ticker})",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPublic("/markets/"+url.PathEscape(args[0]), nil)
	},
}

var marketsOrderbookCmd = &cobra.Command{
	Use:   "orderbook <ticker>",
	Short: "Get a market's order book (GET /markets/{ticker}/orderbook)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		setInt(q, "depth", marketsOrderbookDepth)
		return runPublic("/markets/"+url.PathEscape(args[0])+"/orderbook", q)
	},
}

var marketsTradesCmd = &cobra.Command{
	Use:   "trades",
	Short: "List public trades (GET /markets/trades)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		setStr(q, "ticker", marketsTradesTicker)
		setInt(q, "limit", marketsTradesLimit)
		setStr(q, "cursor", marketsTradesCursor)
		setInt64(q, "min_ts", marketsTradesMinTS)
		setInt64(q, "max_ts", marketsTradesMaxTS)
		return runPublic("/markets/trades", q)
	},
}

var marketsCandlesticksCmd = &cobra.Command{
	Use:   "candlesticks <series_ticker> <market_ticker>",
	Short: "Market candlesticks (GET /series/{series}/markets/{ticker}/candlesticks)",
	Long: `Fetch candlesticks for one market. --start-ts and --end-ts are Unix
seconds; --period-interval is minutes and must be one of 1, 60, 1440.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if marketsCandlesStart == 0 || marketsCandlesEnd == 0 {
			return fmt.Errorf("--start-ts and --end-ts are required (Unix seconds)")
		}
		switch marketsCandlesPeriod {
		case 1, 60, 1440:
		default:
			return fmt.Errorf("invalid --period-interval %d: must be 1, 60, or 1440", marketsCandlesPeriod)
		}
		q := url.Values{}
		setInt64(q, "start_ts", marketsCandlesStart)
		setInt64(q, "end_ts", marketsCandlesEnd)
		setInt(q, "period_interval", marketsCandlesPeriod)
		endpoint := "/series/" + url.PathEscape(args[0]) + "/markets/" + url.PathEscape(args[1]) + "/candlesticks"
		return runPublic(endpoint, q)
	},
}

func init() {
	marketsListCmd.Flags().IntVar(&marketsListLimit, "limit", 0, "max results")
	marketsListCmd.Flags().StringVar(&marketsListCursor, "cursor", "", "pagination cursor")
	marketsListCmd.Flags().StringVar(&marketsListStatus, "status", "", "filter: unopened, open, closed, settled")
	marketsListCmd.Flags().StringVar(&marketsListSeriesTicker, "series-ticker", "", "filter by series ticker")
	marketsListCmd.Flags().StringVar(&marketsListEventTicker, "event-ticker", "", "filter by event ticker")
	marketsListCmd.Flags().StringVar(&marketsListTickers, "tickers", "", "filter by tickers (comma-separated)")
	marketsOrderbookCmd.Flags().IntVar(&marketsOrderbookDepth, "depth", 0, "book depth per side (0 = full book)")
	marketsTradesCmd.Flags().StringVar(&marketsTradesTicker, "ticker", "", "filter by market ticker")
	marketsTradesCmd.Flags().IntVar(&marketsTradesLimit, "limit", 0, "max results")
	marketsTradesCmd.Flags().StringVar(&marketsTradesCursor, "cursor", "", "pagination cursor")
	marketsTradesCmd.Flags().Int64Var(&marketsTradesMinTS, "min-ts", 0, "minimum trade timestamp (Unix seconds)")
	marketsTradesCmd.Flags().Int64Var(&marketsTradesMaxTS, "max-ts", 0, "maximum trade timestamp (Unix seconds)")
	marketsCandlesticksCmd.Flags().Int64Var(&marketsCandlesStart, "start-ts", 0, "start of range (Unix seconds, required)")
	marketsCandlesticksCmd.Flags().Int64Var(&marketsCandlesEnd, "end-ts", 0, "end of range (Unix seconds, required)")
	marketsCandlesticksCmd.Flags().IntVar(&marketsCandlesPeriod, "period-interval", 60, "candle size in minutes: 1, 60, or 1440")
	marketsCmd.AddCommand(marketsListCmd)
	marketsCmd.AddCommand(marketsGetCmd)
	marketsCmd.AddCommand(marketsOrderbookCmd)
	marketsCmd.AddCommand(marketsTradesCmd)
	marketsCmd.AddCommand(marketsCandlesticksCmd)
}
