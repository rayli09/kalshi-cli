package cmd

import (
	"net/url"

	"github.com/spf13/cobra"
)

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Events (public)",
}

var (
	eventsListLimit        int
	eventsListCursor       string
	eventsListStatus       string
	eventsListSeriesTicker string
	eventsListNested       bool
	eventsGetNested        bool
)

var eventsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List events (GET /events)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		setInt(q, "limit", eventsListLimit)
		setStr(q, "cursor", eventsListCursor)
		setStr(q, "status", eventsListStatus)
		setStr(q, "series_ticker", eventsListSeriesTicker)
		if eventsListNested {
			q.Set("with_nested_markets", "true")
		}
		return runPublic("/events", q)
	},
}

var eventsGetCmd = &cobra.Command{
	Use:   "get <event_ticker>",
	Short: "Get one event (GET /events/{event_ticker})",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		if eventsGetNested {
			q.Set("with_nested_markets", "true")
		}
		return runPublic("/events/"+url.PathEscape(args[0]), q)
	},
}

func init() {
	eventsListCmd.Flags().IntVar(&eventsListLimit, "limit", 0, "max results (API default applies when 0)")
	eventsListCmd.Flags().StringVar(&eventsListCursor, "cursor", "", "pagination cursor")
	eventsListCmd.Flags().StringVar(&eventsListStatus, "status", "", "filter: unopened, open, closed, settled")
	eventsListCmd.Flags().StringVar(&eventsListSeriesTicker, "series-ticker", "", "filter by series ticker")
	eventsListCmd.Flags().BoolVar(&eventsListNested, "with-nested-markets", false, "embed each event's markets")
	eventsGetCmd.Flags().BoolVar(&eventsGetNested, "with-nested-markets", false, "embed the event's markets")
	eventsCmd.AddCommand(eventsListCmd)
	eventsCmd.AddCommand(eventsGetCmd)
}
