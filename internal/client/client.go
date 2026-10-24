// Package client is the Kalshi Trade API v2 REST client: environment
// base URLs, optional per-request signing via internal/auth, a
// conservative client-side token-bucket limiter, and bounded retries on
// HTTP 429 honoring Retry-After.
package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rayli09/kalshi-cli/internal/auth"
)

const (
	ProdBaseURL    = "https://external-api.kalshi.com/trade-api/v2"
	DemoBaseURL    = "https://external-api.demo.kalshi.co/trade-api/v2"
	ProdWSURL      = "wss://external-api-ws.kalshi.com/trade-api/ws/v2"
	DemoWSURL      = "wss://external-api-ws.demo.kalshi.co/trade-api/ws/v2"
	APIPathPrefix  = "/trade-api/v2"
	DefaultTimeout = 30 * time.Second
	maxRetries     = 3
)

// Limiter is a minimal token bucket: tokens refill at Rate per second up
// to Burst; Wait blocks until a token is available. Rate <= 0 disables
// limiting.
type Limiter struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func NewLimiter(ratePerSec, burst float64) *Limiter {
	return &Limiter{rate: ratePerSec, burst: burst, tokens: burst, last: time.Now()}
}

func (l *Limiter) Wait(ctx context.Context) error {
	if l == nil || l.rate <= 0 {
		return nil
	}
	for {
		l.mu.Lock()
		now := time.Now()
		l.tokens += now.Sub(l.last).Seconds() * l.rate
		if l.tokens > l.burst {
			l.tokens = l.burst
		}
		l.last = now
		if l.tokens >= 1 {
			l.tokens--
			l.mu.Unlock()
			return nil
		}
		deficit := 1 - l.tokens
		l.mu.Unlock()
		wait := time.Duration(deficit / l.rate * float64(time.Second))
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// Client talks to one Kalshi environment. Signer may be nil for
// public-only use; signed calls then fail fast with a clear error.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	Signer  *auth.Signer
	Limiter *Limiter
}

// New builds a client. demo selects the demo environment.
func New(demo bool, signer *auth.Signer) *Client {
	base := ProdBaseURL
	if demo {
		base = DemoBaseURL
	}
	return &Client{
		BaseURL: base,
		HTTP:    &http.Client{Timeout: DefaultTimeout},
		Signer:  signer,
		Limiter: NewLimiter(10, 10),
	}
}

// Response is one API response.
type Response struct {
	StatusCode int
	Body       []byte
}

func (r *Response) IsError() bool { return r.StatusCode >= 400 }

// SignedPath returns the path Kalshi expects in the signature for a
// request to this client's base URL + endpoint (endpoint starts with "/",
// no query string). It derives the API-root path from the configured
// base URL so test servers and host overrides sign correctly.
func (c *Client) SignedPath(endpoint string) string {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Path == "" {
		return APIPathPrefix + endpoint
	}
	return strings.TrimSuffix(u.Path, "/") + endpoint
}

// Do performs one request. endpoint is like "/markets" or
// "/portfolio/balance" (relative to the base URL). signed=true attaches
// the KALSHI-ACCESS-* headers. Retries on 429 up to maxRetries times.
func (c *Client) Do(ctx context.Context, method, endpoint string, query url.Values, body []byte, signed bool) (*Response, error) {
	var authHeader http.Header
	if signed {
		if c.Signer == nil {
			return nil, fmt.Errorf("this command requires Kalshi API credentials: set KALSHI_API_KEY_ID and KALSHI_PRIVATE_KEY_PATH (or --api-key-id/--private-key-path, or a 0600 config file)")
		}
		h, err := c.Signer.Headers(method, c.SignedPath(endpoint))
		if err != nil {
			return nil, err
		}
		authHeader = h
	}

	full := c.BaseURL + endpoint
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := c.Limiter.Wait(ctx); err != nil {
			return nil, err
		}
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, full, rdr)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, vs := range authHeader {
			for _, v := range vs {
				req.Header.Set(k, v)
			}
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			wait := retryAfter(resp.Header, attempt)
			// Re-sign: a fresh timestamp on retry.
			if signed {
				h, herr := c.Signer.Headers(method, c.SignedPath(endpoint))
				if herr != nil {
					return nil, herr
				}
				authHeader = h
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			lastErr = fmt.Errorf("rate limited (429), retry %d", attempt+1)
			continue
		}
		return &Response{StatusCode: resp.StatusCode, Body: data}, nil
	}
	return nil, fmt.Errorf("request failed after %d retries: %w", maxRetries, lastErr)
}

func retryAfter(h http.Header, attempt int) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return time.Duration(1<<attempt) * 500 * time.Millisecond
}

// Get is a convenience wrapper for a GET with no body.
func (c *Client) Get(ctx context.Context, endpoint string, query url.Values, signed bool) (*Response, error) {
	return c.Do(ctx, http.MethodGet, endpoint, query, nil, signed)
}
