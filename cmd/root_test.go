package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runCmd executes the real command tree with args and captures
// everything printed to stdout/stderr (commands print with fmt.Println,
// which bypasses cobra's output writer).
func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w
	rootCmd.SetOut(w)
	rootCmd.SetErr(w)
	rootCmd.SetArgs(args)
	execErr := rootCmd.Execute()
	rootCmd.SetArgs(nil)
	w.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String(), execErr
}

func leafCommands(c *cobra.Command) []string {
	var leaves []string
	for _, sub := range c.Commands() {
		if len(sub.Commands()) == 0 {
			leaves = append(leaves, c.Name()+" "+sub.Name())
		} else {
			leaves = append(leaves, leafCommands(sub)...)
		}
	}
	return leaves
}

func TestLeafCommandCount(t *testing.T) {
	leaves := leafCommands(rootCmd)
	if len(leaves) != 24 {
		t.Fatalf("leaf commands = %d, want 24: %v", len(leaves), leaves)
	}
}

func TestDryRunCreateSendsNothing(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	defer srv.Close()
	old := flagBaseURL
	flagBaseURL = srv.URL + "/trade-api/v2"
	defer func() { flagBaseURL = old }()

	resetCreateFlags(t)
	out, err := runCmd(t, "orders", "create", "--ticker", "KXTEST-1", "--side", "bid", "--count", "10", "--price", "0.3500")
	if err != nil {
		t.Fatalf("dry-run create: %v\n%s", err, out)
	}
	if hits != 0 {
		t.Fatalf("dry run sent %d requests, want 0", hits)
	}
	for _, want := range []string{"DRY RUN", "POST", "/portfolio/events/orders", `"side": "bid"`, `"price": "0.3500"`, `"count": "10"`, "client_order_id"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestDryRunCancelSendsNothing(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer srv.Close()
	old := flagBaseURL
	flagBaseURL = srv.URL + "/trade-api/v2"
	defer func() { flagBaseURL = old }()

	cancelLive, cancelLegacy = false, false
	out, err := runCmd(t, "orders", "cancel", "order-123")
	if err != nil {
		t.Fatalf("dry-run cancel: %v", err)
	}
	if hits != 0 {
		t.Fatalf("dry-run cancel sent %d requests", hits)
	}
	if !strings.Contains(out, "DELETE") || !strings.Contains(out, "/portfolio/events/orders/order-123") {
		t.Fatalf("unexpected dry-run cancel output:\n%s", out)
	}
}

func TestDryRunBatchCreateValidatesBody(t *testing.T) {
	batchCreateLive = false
	out, err := runCmd(t, "orders", "batch-create", "--json", `{"orders":[{"ticker":"KXTEST-1"}]}`)
	if err != nil {
		t.Fatalf("batch-create dry-run: %v", err)
	}
	if !strings.Contains(out, "DRY RUN") || !strings.Contains(out, "/portfolio/orders/batched") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	if _, err := runCmd(t, "orders", "batch-create", "--json", `{"nope":1}`); err == nil {
		t.Fatal("batch-create accepted a body without an orders array")
	}
}

func TestCreateValidation(t *testing.T) {
	resetCreateFlags(t)
	if _, _, err := buildCreateOrderBody(); err == nil {
		t.Fatal("create with no ticker accepted")
	}
	createTicker = "KXTEST-1"
	createSide = "yes" // V2 wants bid/ask
	createCount = "10"
	createPrice = "0.3500"
	if _, _, err := buildCreateOrderBody(); err == nil {
		t.Fatal("V2 create accepted side=yes")
	}
	createSide = "bid"
	createPrice = "1.50"
	if _, _, err := buildCreateOrderBody(); err == nil {
		t.Fatal("V2 create accepted price >= 1")
	}
	createPrice = "0.3500"
	body, endpoint, err := buildCreateOrderBody()
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "/portfolio/events/orders" {
		t.Fatalf("endpoint = %q", endpoint)
	}
	if !strings.Contains(string(body), `"side":"bid"`) {
		t.Fatalf("body = %s", body)
	}
	// Legacy path.
	createLegacy = true
	createAction = "buy"
	createSide = "yes"
	createType = "limit"
	createYesPrice = 35
	body, endpoint, err = buildCreateOrderBody()
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "/portfolio/orders" || !strings.Contains(string(body), `"yes_price":35`) {
		t.Fatalf("legacy body/endpoint = %s %s", endpoint, body)
	}
	resetCreateFlags(t)
}

func resetCreateFlags(t *testing.T) {
	t.Helper()
	createTicker, createSide, createCount, createPrice = "", "bid", "", ""
	createTIF, createSTP, createClientOID = "", "", ""
	createPostOnly, createLive, createLegacy = false, false, false
	createAction, createType = "", "limit"
	createYesPrice, createNoPrice = 0, 0
	createExpiration = 0
}
