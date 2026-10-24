package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var ordersCmd = &cobra.Command{
	Use:   "orders",
	Short: "Orders (signed; mutating commands are dry-run unless --live)",
}

// ---------- list / get ----------

var (
	ordersListLimit       int
	ordersListCursor      string
	ordersListStatus      string
	ordersListTicker      string
	ordersListEventTicker string
	ordersListMinTS       int64
	ordersListMaxTS       int64
)

var ordersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List orders (GET /portfolio/orders)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		setInt(q, "limit", ordersListLimit)
		setStr(q, "cursor", ordersListCursor)
		setStr(q, "status", ordersListStatus)
		setStr(q, "ticker", ordersListTicker)
		setStr(q, "event_ticker", ordersListEventTicker)
		setInt64(q, "min_ts", ordersListMinTS)
		setInt64(q, "max_ts", ordersListMaxTS)
		return runSigned("/portfolio/orders", q)
	},
}

var ordersGetCmd = &cobra.Command{
	Use:   "get <order_id>",
	Short: "Get one order (GET /portfolio/orders/{order_id})",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSigned("/portfolio/orders/"+url.PathEscape(args[0]), nil)
	},
}

// ---------- create ----------

var (
	createTicker     string
	createSide       string
	createCount      string
	createPrice      string
	createTIF        string
	createPostOnly   bool
	createSTP        string
	createClientOID  string
	createExpiration int64
	createLive       bool
	createLegacy     bool
	// legacy-only fields
	createAction   string
	createType     string
	createYesPrice int
	createNoPrice  int
)

var ordersCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an order (POST /portfolio/events/orders) — dry-run unless --live",
	Long: `Create an order. DRY-RUN BY DEFAULT: without --live it prints the
exact request and sends nothing (no credentials needed for the preview).

Default is Kalshi's current V2 event-order shape on the unified book:
--side bid buys YES at --price, --side ask sells YES at --price. Prices
are fixed-point dollar strings ("0.3500"); --count is a fixed-point
contract count string ("10"). --client-order-id defaults to a random
UUID; pass your own for idempotent retries.

--legacy builds the deprecated V1 shape instead (POST /portfolio/orders
with --action buy|sell, --side yes|no, --type limit|market, and
--yes-price/--no-price in integer cents).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		body, endpoint, err := buildCreateOrderBody()
		if err != nil {
			return err
		}
		if !createLive {
			return dryRunPreview("POST", endpoint, body)
		}
		return runMutation("POST", endpoint, body)
	},
}

func buildCreateOrderBody() ([]byte, string, error) {
	if createTicker == "" {
		return nil, "", fmt.Errorf("--ticker is required")
	}
	clientOID := createClientOID
	if clientOID == "" {
		clientOID = newUUID()
	}
	if createLegacy {
		if createAction != "buy" && createAction != "sell" {
			return nil, "", fmt.Errorf("--action must be buy or sell (legacy mode)")
		}
		if createSide != "yes" && createSide != "no" {
			return nil, "", fmt.Errorf("--side must be yes or no in --legacy mode (bid/ask are V2)")
		}
		if createType != "limit" && createType != "market" {
			return nil, "", fmt.Errorf("--type must be limit or market (legacy mode)")
		}
		m := map[string]any{
			"ticker":          createTicker,
			"client_order_id": clientOID,
			"side":            createSide,
			"action":          createAction,
			"type":            createType,
			"count":           mustParseCount(createCount),
		}
		if createType == "limit" {
			switch createSide {
			case "yes":
				if createYesPrice < 1 || createYesPrice > 99 {
					return nil, "", fmt.Errorf("--yes-price must be 1..99 cents for a limit order on the yes side")
				}
				m["yes_price"] = createYesPrice
			case "no":
				if createNoPrice < 1 || createNoPrice > 99 {
					return nil, "", fmt.Errorf("--no-price must be 1..99 cents for a limit order on the no side")
				}
				m["no_price"] = createNoPrice
			}
		}
		if createExpiration != 0 {
			m["expiration_ts"] = createExpiration
		}
		b, err := json.Marshal(m)
		return b, "/portfolio/orders", err
	}
	// V2 event order.
	if createSide != "bid" && createSide != "ask" {
		return nil, "", fmt.Errorf("--side must be bid or ask (V2 unified book; bid buys YES, ask sells YES)")
	}
	if createCount == "" {
		return nil, "", fmt.Errorf("--count is required (fixed-point string, e.g. \"10\")")
	}
	if createPrice == "" {
		return nil, "", fmt.Errorf("--price is required (fixed-point dollars, e.g. \"0.3500\")")
	}
	if _, err := parseDollars(createPrice); err != nil {
		return nil, "", err
	}
	m := map[string]any{
		"ticker":          createTicker,
		"side":            createSide,
		"count":           createCount,
		"price":           createPrice,
		"client_order_id": clientOID,
	}
	if createTIF != "" {
		m["time_in_force"] = createTIF
	}
	if createPostOnly {
		m["post_only"] = true
	}
	if createSTP != "" {
		m["self_trade_prevention_type"] = createSTP
	}
	if createExpiration != 0 {
		m["expiration_time"] = createExpiration
	}
	b, err := json.Marshal(m)
	return b, "/portfolio/events/orders", err
}

// parseDollars validates a fixed-point dollar price string in (0, 1).
func parseDollars(s string) (float64, error) {
	var v float64
	if _, err := fmt.Sscanf(s, "%g", &v); err != nil {
		return 0, fmt.Errorf("invalid --price %q: want a fixed-point dollar string like \"0.3500\"", s)
	}
	if v <= 0 || v >= 1 {
		return 0, fmt.Errorf("invalid --price %q: Kalshi prices are between $0.01 and $0.99", s)
	}
	return v, nil
}

func mustParseCount(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// newUUID returns a random RFC 4122 v4 UUID (no external dependency).
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// ---------- cancel ----------

var (
	cancelLive   bool
	cancelLegacy bool
)

var ordersCancelCmd = &cobra.Command{
	Use:   "cancel <order_id>",
	Short: "Cancel an order (DELETE /portfolio/events/orders/{order_id}) — dry-run unless --live",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint := "/portfolio/events/orders/" + url.PathEscape(args[0])
		if cancelLegacy {
			endpoint = "/portfolio/orders/" + url.PathEscape(args[0])
		}
		if !cancelLive {
			return dryRunPreview("DELETE", endpoint, nil)
		}
		return runMutation("DELETE", endpoint, nil)
	},
}

// ---------- cancel-batch ----------

var cancelBatchLive bool

var ordersCancelBatchCmd = &cobra.Command{
	Use:   "cancel-batch <order_id> [order_id...]",
	Short: "Cancel several orders at once (DELETE /portfolio/orders/batched) — dry-run unless --live",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		list := make([]map[string]string, 0, len(args))
		for _, id := range args {
			list = append(list, map[string]string{"order_id": id})
		}
		body, err := json.Marshal(map[string]any{"orders": list})
		if err != nil {
			return err
		}
		if !cancelBatchLive {
			return dryRunPreview("DELETE", "/portfolio/orders/batched", body)
		}
		return runMutation("DELETE", "/portfolio/orders/batched", body)
	},
}

// ---------- amend ----------

var (
	amendTicker    string
	amendSide      string
	amendAction    string
	amendCount     int
	amendYesPrice  int
	amendNoPrice   int
	amendClientOID string
	amendLive      bool
)

var ordersAmendCmd = &cobra.Command{
	Use:   "amend <order_id>",
	Short: "Amend an order (POST /portfolio/orders/{order_id}/amend) — dry-run unless --live",
	Long: `Amend an order's price/count. Amend exists only in the legacy
order namespace as of this writing. Requires --ticker, --side yes|no,
--action buy|sell, --count, and the matching --yes-price or --no-price
(cents). --client-order-id defaults to a random UUID; Kalshi rotates
the order's client_order_id on amend (sent as updated_client_order_id).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if amendTicker == "" {
			return fmt.Errorf("--ticker is required")
		}
		if amendSide != "yes" && amendSide != "no" {
			return fmt.Errorf("--side must be yes or no")
		}
		if amendAction != "buy" && amendAction != "sell" {
			return fmt.Errorf("--action must be buy or sell")
		}
		if amendCount <= 0 {
			return fmt.Errorf("--count must be positive")
		}
		m := map[string]any{
			"ticker": amendTicker,
			"side":   amendSide,
			"action": amendAction,
			"count":  amendCount,
		}
		switch amendSide {
		case "yes":
			if amendYesPrice < 1 || amendYesPrice > 99 {
				return fmt.Errorf("--yes-price must be 1..99 cents")
			}
			m["yes_price"] = amendYesPrice
		case "no":
			if amendNoPrice < 1 || amendNoPrice > 99 {
				return fmt.Errorf("--no-price must be 1..99 cents")
			}
			m["no_price"] = amendNoPrice
		}
		oid := amendClientOID
		if oid == "" {
			oid = newUUID()
		}
		m["updated_client_order_id"] = oid
		body, err := json.Marshal(m)
		if err != nil {
			return err
		}
		endpoint := "/portfolio/orders/" + url.PathEscape(args[0]) + "/amend"
		if !amendLive {
			return dryRunPreview("POST", endpoint, body)
		}
		return runMutation("POST", endpoint, body)
	},
}

// ---------- batch-create ----------

var (
	batchCreateFile string
	batchCreateJSON string
	batchCreateLive bool
)

var ordersBatchCreateCmd = &cobra.Command{
	Use:   "batch-create",
	Short: "Create several orders at once (POST /portfolio/orders/batched) — dry-run unless --live",
	Long: `Batch-create legacy-shape orders. The request body is a JSON
object {"orders": [ …legacy order objects… ]} supplied with --file,
--json, or stdin. The body is sent verbatim (after a JSON validity
check); build individual orders with 'orders create --legacy' first if
you are unsure of the shape.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := readBody(batchCreateFile, batchCreateJSON)
		if err != nil {
			return err
		}
		var check map[string]json.RawMessage
		if err := json.Unmarshal(body, &check); err != nil {
			return fmt.Errorf("request body is not valid JSON: %w", err)
		}
		if _, ok := check["orders"]; !ok {
			return fmt.Errorf("request body must be a JSON object with an \"orders\" array")
		}
		if !batchCreateLive {
			return dryRunPreview("POST", "/portfolio/orders/batched", body)
		}
		return runMutation("POST", "/portfolio/orders/batched", body)
	},
}

// readBody loads a request body from --file, --json, or stdin.
func readBody(file, inline string) ([]byte, error) {
	switch {
	case file != "":
		return os.ReadFile(file)
	case inline != "":
		return []byte(inline), nil
	default:
		fi, err := os.Stdin.Stat()
		if err != nil {
			return nil, err
		}
		if fi.Mode()&os.ModeCharDevice != 0 {
			return nil, fmt.Errorf("no request body: pass --file, --json, or pipe JSON on stdin")
		}
		return io.ReadAll(os.Stdin)
	}
}

var _ = strings.TrimSpace // anchor for future flag parsing helpers

func init() {
	ordersListCmd.Flags().IntVar(&ordersListLimit, "limit", 0, "max results")
	ordersListCmd.Flags().StringVar(&ordersListCursor, "cursor", "", "pagination cursor")
	ordersListCmd.Flags().StringVar(&ordersListStatus, "status", "", "filter: resting, canceled, pending, executed")
	ordersListCmd.Flags().StringVar(&ordersListTicker, "ticker", "", "filter by market ticker")
	ordersListCmd.Flags().StringVar(&ordersListEventTicker, "event-ticker", "", "filter by event ticker")
	ordersListCmd.Flags().Int64Var(&ordersListMinTS, "min-ts", 0, "minimum created timestamp (Unix seconds)")
	ordersListCmd.Flags().Int64Var(&ordersListMaxTS, "max-ts", 0, "maximum created timestamp (Unix seconds)")

	ordersCreateCmd.Flags().StringVar(&createTicker, "ticker", "", "market ticker (required)")
	ordersCreateCmd.Flags().StringVar(&createSide, "side", "bid", "V2: bid (buy YES) or ask (sell YES); legacy: yes or no")
	ordersCreateCmd.Flags().StringVar(&createCount, "count", "", "contract count, fixed-point string (required)")
	ordersCreateCmd.Flags().StringVar(&createPrice, "price", "", "limit price, fixed-point dollars e.g. \"0.3500\" (V2, required)")
	ordersCreateCmd.Flags().StringVar(&createTIF, "time-in-force", "", "V2: fill_or_kill, good_till_canceled, immediate_or_cancel")
	ordersCreateCmd.Flags().BoolVar(&createPostOnly, "post-only", false, "V2: post only (maker)")
	ordersCreateCmd.Flags().StringVar(&createSTP, "self-trade-prevention", "", "V2: taker_at_cross or maker")
	ordersCreateCmd.Flags().StringVar(&createClientOID, "client-order-id", "", "client order ID (default: random UUID)")
	ordersCreateCmd.Flags().Int64Var(&createExpiration, "expiration-ts", 0, "order expiration (Unix seconds)")
	ordersCreateCmd.Flags().BoolVar(&createLegacy, "legacy", false, "build the deprecated V1 order shape instead of V2")
	ordersCreateCmd.Flags().StringVar(&createAction, "action", "", "legacy: buy or sell")
	ordersCreateCmd.Flags().StringVar(&createType, "type", "limit", "legacy: limit or market")
	ordersCreateCmd.Flags().IntVar(&createYesPrice, "yes-price", 0, "legacy: yes limit price in cents (1..99)")
	ordersCreateCmd.Flags().IntVar(&createNoPrice, "no-price", 0, "legacy: no limit price in cents (1..99)")
	ordersCreateCmd.Flags().BoolVar(&createLive, "live", false, "actually submit the order (default is dry-run)")

	ordersCancelCmd.Flags().BoolVar(&cancelLegacy, "legacy", false, "cancel via the deprecated V1 path (/portfolio/orders/{id})")
	ordersCancelCmd.Flags().BoolVar(&cancelLive, "live", false, "actually cancel the order (default is dry-run)")

	ordersCancelBatchCmd.Flags().BoolVar(&cancelBatchLive, "live", false, "actually cancel the orders (default is dry-run)")

	ordersAmendCmd.Flags().StringVar(&amendTicker, "ticker", "", "market ticker (required)")
	ordersAmendCmd.Flags().StringVar(&amendSide, "side", "", "yes or no (required)")
	ordersAmendCmd.Flags().StringVar(&amendAction, "action", "", "buy or sell (required)")
	ordersAmendCmd.Flags().IntVar(&amendCount, "count", 0, "new contract count (required)")
	ordersAmendCmd.Flags().IntVar(&amendYesPrice, "yes-price", 0, "new yes price in cents (1..99)")
	ordersAmendCmd.Flags().IntVar(&amendNoPrice, "no-price", 0, "new no price in cents (1..99)")
	ordersAmendCmd.Flags().StringVar(&amendClientOID, "client-order-id", "", "new client order ID (default: random UUID)")
	ordersAmendCmd.Flags().BoolVar(&amendLive, "live", false, "actually amend the order (default is dry-run)")

	ordersBatchCreateCmd.Flags().StringVar(&batchCreateFile, "file", "", "read the request body from a file")
	ordersBatchCreateCmd.Flags().StringVar(&batchCreateJSON, "json", "", "request body as an inline JSON string")
	ordersBatchCreateCmd.Flags().BoolVar(&batchCreateLive, "live", false, "actually submit the orders (default is dry-run)")

	ordersCmd.AddCommand(ordersListCmd)
	ordersCmd.AddCommand(ordersGetCmd)
	ordersCmd.AddCommand(ordersCreateCmd)
	ordersCmd.AddCommand(ordersAmendCmd)
	ordersCmd.AddCommand(ordersCancelCmd)
	ordersCmd.AddCommand(ordersCancelBatchCmd)
	ordersCmd.AddCommand(ordersBatchCreateCmd)
}
