package cmd

import (
	"net/url"

	"github.com/spf13/cobra"
)

var seriesCmd = &cobra.Command{
	Use:   "series",
	Short: "Market series (public)",
}

var (
	seriesListCategory      string
	seriesListTags          string
	seriesListIncludeVolume bool
)

var seriesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List series (GET /series)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		setStr(q, "category", seriesListCategory)
		setStr(q, "tags", seriesListTags)
		if seriesListIncludeVolume {
			q.Set("include_volume", "true")
		}
		return runPublic("/series", q)
	},
}

var seriesGetCmd = &cobra.Command{
	Use:   "get <series_ticker>",
	Short: "Get one series (GET /series/{ticker})",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		if seriesGetIncludeVolume {
			q.Set("include_volume", "true")
		}
		return runPublic("/series/"+url.PathEscape(args[0]), q)
	},
}

var seriesGetIncludeVolume bool

func init() {
	seriesListCmd.Flags().StringVar(&seriesListCategory, "category", "", "filter by category")
	seriesListCmd.Flags().StringVar(&seriesListTags, "tags", "", "filter by tags (comma-separated)")
	seriesListCmd.Flags().BoolVar(&seriesListIncludeVolume, "include-volume", false, "include volume in the response")
	seriesGetCmd.Flags().BoolVar(&seriesGetIncludeVolume, "include-volume", false, "include volume in the response")
	seriesCmd.AddCommand(seriesListCmd)
	seriesCmd.AddCommand(seriesGetCmd)
}
