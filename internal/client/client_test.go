package client

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rayli09/kalshi-cli/internal/auth"
)

func testSigner(t *testing.T) *auth.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	s, err := auth.ParseSigner("test-key-id", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func stubClient(t *testing.T, handler http.Handler, signer *auth.Signer) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New(false, signer)
	c.BaseURL = srv.URL + APIPathPrefix
	c.Limiter = NewLimiter(0, 0)
	return c
}

func TestPublicGetNoAuthHeaders(t *testing.T) {
	var sawKey, sawPath, sawQuery string
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawKey = r.Header.Get("KALSHI-ACCESS-KEY")
		sawPath = r.URL.Path
		sawQuery = r.URL.RawQuery
		fmt.Fprint(w, `{"markets":[]}`)
	}), nil)
	resp, err := c.Get(context.Background(), "/markets", url.Values{"limit": {"5"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || !strings.Contains(string(resp.Body), "markets") {
		t.Fatalf("resp = %d %s", resp.StatusCode, resp.Body)
	}
	if sawKey != "" {
		t.Fatalf("public request carried an auth header: %q", sawKey)
	}
	if sawPath != "/trade-api/v2/markets" || sawQuery != "limit=5" {
		t.Fatalf("path/query = %s ? %s", sawPath, sawQuery)
	}
}

func TestSignedGetHeadersVerify(t *testing.T) {
	signer := testSigner(t)
	var ok bool
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts := r.Header.Get("KALSHI-ACCESS-TIMESTAMP")
		if r.Header.Get("KALSHI-ACCESS-KEY") != "test-key-id" || ts == "" {
			t.Errorf("missing auth headers")
		}
		var tsMs int64
		fmt.Sscanf(ts, "%d", &tsMs)
		sig := r.Header.Get("KALSHI-ACCESS-SIGNATURE")
		ok = signer.VerifyForTest(tsMs, "GET", "/trade-api/v2/portfolio/balance", sig)
		fmt.Fprint(w, `{"balance": 100}`)
	}), signer)
	resp, err := c.Get(context.Background(), "/portfolio/balance", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !ok {
		t.Fatal("signature header did not verify against the signed path (query-stripped, API-root)")
	}
}

func TestSignedWithoutSignerFails(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil)
	_, err := c.Get(context.Background(), "/portfolio/balance", nil, true)
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("err = %v, want credentials error", err)
	}
}

func Test429RetriedThenSucceeds(t *testing.T) {
	var calls int64
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&calls, 1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}), nil)
	resp, err := c.Get(context.Background(), "/exchange/status", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (2x 429 + success)", calls)
	}
}

func TestErrorStatusReturned(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":"forbidden"}}`)
	}), nil)
	resp, err := c.Get(context.Background(), "/markets", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError() || resp.StatusCode != 403 {
		t.Fatalf("resp = %d", resp.StatusCode)
	}
}

func TestSignedPathDerivation(t *testing.T) {
	c := New(false, nil)
	if got := c.SignedPath("/portfolio/balance"); got != "/trade-api/v2/portfolio/balance" {
		t.Fatalf("SignedPath = %q", got)
	}
	c.BaseURL = "http://127.0.0.1:9999/trade-api/v2"
	if got := c.SignedPath("/markets"); got != "/trade-api/v2/markets" {
		t.Fatalf("SignedPath = %q", got)
	}
}

func TestLimiterRespectsRate(t *testing.T) {
	l := NewLimiter(100, 1) // 100/s, burst 1: second wait ~10ms
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := l.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Fatalf("limiter did not throttle: 3 waits in %v", elapsed)
	}
}
