package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rayli09/kalshi-cli/internal/ws"
)

var streamCmd = &cobra.Command{
	Use:   "stream",
	Short: "WebSocket streaming (signed handshake)",
}

var (
	streamChannels    []string
	streamTickers     []string
	streamMaxMessages int
	streamDuration    time.Duration
)

var streamSubscribeCmd = &cobra.Command{
	Use:   "subscribe",
	Short: "Subscribe to channels and print messages (WS /trade-api/ws/v2)",
	Long: `Connect to the Kalshi WebSocket with signed handshake headers and
subscribe to channels. Implemented channels: ticker, orderbook_delta
(private; snapshot + delta messages print raw), trade. Other documented
channels (fill, market_positions, market_lifecycle_v2, …) are accepted
and also print raw. Prints until --max-messages or --duration is
reached, or Ctrl-C. Streaming requires credentials: Kalshi signs the
handshake even for public channels.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(streamChannels) == 0 {
			return fmt.Errorf("at least one --channel is required (ticker, orderbook_delta, trade)")
		}
		if len(streamTickers) == 0 {
			return fmt.Errorf("at least one --ticker is required")
		}
		signer, err := loadSigner()
		if err != nil {
			return err
		}
		ctx := context.Background()
		if streamDuration > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, streamDuration)
			defer cancel()
		}
		params := ws.SubscribeParams{
			Channels:      streamChannels,
			MarketTickers: streamTickers,
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "connecting to %s, channels [%s], tickers [%s]\n",
			wsURL(), strings.Join(streamChannels, ","), strings.Join(streamTickers, ","))
		return ws.Stream(ctx, wsURL(), wsSignPath(), signer, params, streamMaxMessages)
	},
}

func init() {
	streamSubscribeCmd.Flags().StringArrayVar(&streamChannels, "channel", nil, "channel to subscribe to (repeatable): ticker, orderbook_delta, trade, …")
	streamSubscribeCmd.Flags().StringArrayVar(&streamTickers, "ticker", nil, "market ticker to subscribe to (repeatable)")
	streamSubscribeCmd.Flags().IntVar(&streamMaxMessages, "max-messages", 0, "stop after N messages (0 = no limit)")
	streamSubscribeCmd.Flags().DurationVar(&streamDuration, "duration", 0, "stop after this duration (0 = no limit)")
	streamCmd.AddCommand(streamSubscribeCmd)
}
