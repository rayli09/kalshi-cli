// Package ws is a minimal Kalshi WebSocket client: it dials the Trade
// API WebSocket endpoint with signed handshake headers (sign
// GET /trade-api/ws/v2 — the full path from the API root), sends one
// subscribe command, and streams raw messages to a handler. Kalshi
// requires authentication to connect at all, even for public channels
// (ticker, trade, market_lifecycle_v2); orderbook_delta is a private
// channel. See docs.kalshi.com "Quick Start: WebSockets".
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/rayli09/kalshi-cli/internal/auth"
)

// SubscribeParams is the body of a subscribe command.
type SubscribeParams struct {
	Channels      []string `json:"channels"`
	MarketTickers []string `json:"market_tickers,omitempty"`
	MarketTicker  string   `json:"market_ticker,omitempty"`
}

type command struct {
	ID     int             `json:"id"`
	Cmd    string          `json:"cmd"`
	Params SubscribeParams `json:"params"`
}

// Handler is called with each raw message received after subscribing.
// Returning false stops the stream.
type Handler func(msg []byte) bool

// Stream connects, subscribes, and reads until the handler stops it, the
// context ends, maxMessages (>0) is reached, or the connection fails.
func Stream(ctx context.Context, wsURL, signPath string, signer *auth.Signer, params SubscribeParams, maxMessages int) error {
	if signer == nil {
		return fmt.Errorf("streaming requires Kalshi API credentials (the WebSocket handshake is signed): set KALSHI_API_KEY_ID and KALSHI_PRIVATE_KEY_PATH")
	}
	headers, err := signer.Headers(http.MethodGet, signPath)
	if err != nil {
		return err
	}
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		return fmt.Errorf("websocket dial %s: %w", wsURL, err)
	}
	defer conn.Close()

	cmd := command{ID: 1, Cmd: "subscribe", Params: params}
	raw, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	if err := conn.WriteMessage(websocket.TextMessage, raw); err != nil {
		return fmt.Errorf("sending subscribe: %w", err)
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	count := 0
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("reading websocket: %w", err)
		}
		count++
		fmt.Println(string(msg))
		if maxMessages > 0 && count >= maxMessages {
			return nil
		}
	}
}
