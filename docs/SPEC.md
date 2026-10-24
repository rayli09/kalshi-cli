# kalshi-cli — SPEC

A Go (cobra) command-line client for the **Kalshi Trade API v2**, modeled
1:1 on [schwab-cli](https://github.com/rayli09/schwab-cli) /
[etrade-cli](https://github.com/rayli09/etrade-cli) /
[ibkr-cli](https://github.com/rayli09/ibkr-cli): same layout, conventions,
and safety rails (dry-run-by-default order commands, credentials never
printed, tests against a local stub, never the real account).

Written 2026-10-24 against the official documentation at
<https://docs.kalshi.com> (environments page updated 2026-10-19),
cross-checked against the endpoint list of the official `kalshi-python`
SDK and live public-endpoint responses the same day.

## Environments

Credentials do **not** cross environments.

| Env | REST base | WebSocket |
|---|---|---|
| Production (default) | `https://external-api.kalshi.com/trade-api/v2` | `wss://external-api-ws.kalshi.com/trade-api/ws/v2` |
| Demo (`--demo` / `KALSHI_ENV=demo`) | `https://external-api.demo.kalshi.co/trade-api/v2` | `wss://external-api-ws.demo.kalshi.co/trade-api/ws/v2` |

The older shared hosts (`api.elections.kalshi.com`, `demo-api.kalshi.co`)
remain supported by Kalshi but are not used here. Signing is identical on
every host: the signed path excludes the host.

## Authentication

- **Public market-data reads need no auth** (series, events, markets,
  orderbook, trades, candlesticks, exchange status/schedule).
- **Private requests** (everything under `/portfolio`, plus the
  WebSocket handshake — even for public channels) are signed per request:

  ```
  message   = timestamp_ms + METHOD + path        # path from API root, NO query string
              e.g. "1730000000000GET/trade-api/v2/portfolio/balance"
  signature = sign(message)
  headers   = KALSHI-ACCESS-KEY:       <api key id>
              KALSHI-ACCESS-TIMESTAMP: <timestamp_ms>
              KALSHI-ACCESS-SIGNATURE: base64(signature)
  ```

- **Two key types, auto-detected from the PEM contents:**
  - **Ed25519** — the default for keys created in the web app since
    2026-10-01. Sign the message directly.
  - **RSA** — sign with **RSA-PSS**, SHA-256, MGF1-SHA256, salt length =
    digest length (32).
- Key material is loaded from a PEM file (`KALSHI_PRIVATE_KEY_PATH` /
  `--private-key-path`, or `private_key_path` in the config file). It is
  never printed or logged; `auth status` shows only a SHA-256 fingerprint
  of the *public* key and the detected key type.
- Credential precedence: flags → env (`KALSHI_API_KEY_ID`,
  `KALSHI_PRIVATE_KEY_PATH`; demo-specific `KALSHI_DEMO_API_KEY_ID`,
  `KALSHI_DEMO_PRIVATE_KEY_PATH` with `--demo`) → 0600 JSON config
  (`--config`, default `~/.kalshi-cli.json` when present).

## Rate limits & fees (context, not code promises)

Kalshi rate-limits with a token bucket that scales by account tier
(official docs describe read/write budgets; third-party measurements of
the Basic tier start around 200 read / 100 write tokens/sec). This CLI is
deliberately more conservative: a client-side limiter defaults to
**10 requests/sec, burst 10** (`--rate-limit`, `--rate-burst`), and 429
responses are retried at most 3 times, honoring `Retry-After` and backing
off exponentially.

Taker fees follow Kalshi's published formula
`round_up(0.07 × contracts × P × (1−P))` (≤ $0.02/contract); makers may be
charged per-series fees. The CLI does not compute fees — check the
market's fee schedule before trading.

## Command tree (24 leaf commands)

```
auth status                                   env, key fingerprint/type, reachability
exchange status                               GET  /exchange/status
exchange schedule                             GET  /exchange/schedule
series list                                   GET  /series
series get <ticker>                           GET  /series/{ticker}
events list                                   GET  /events
events get <event_ticker>                     GET  /events/{event_ticker}
markets list                                  GET  /markets
markets get <ticker>                          GET  /markets/{ticker}
markets orderbook <ticker>                    GET  /markets/{ticker}/orderbook
markets trades                                GET  /markets/trades
markets candlesticks <series> <ticker>        GET  /series/{series}/markets/{ticker}/candlesticks
portfolio balance                             GET  /portfolio/balance            (auth)
portfolio positions                           GET  /portfolio/positions          (auth)
portfolio fills                               GET  /portfolio/fills              (auth)
portfolio settlements                         GET  /portfolio/settlements        (auth)
orders list                                   GET  /portfolio/orders             (auth)
orders get <order_id>                         GET  /portfolio/orders/{order_id}  (auth)
orders create                                 POST /portfolio/events/orders      (auth, dry-run default)
orders amend <order_id>                       POST /portfolio/orders/{order_id}/amend (auth, dry-run default)
orders cancel <order_id>                      DELETE /portfolio/events/orders/{order_id} (auth, dry-run default)
orders cancel-batch                           DELETE /portfolio/orders/batched   (auth, dry-run default)
orders batch-create                           POST /portfolio/orders/batched     (auth, dry-run default)
stream subscribe                              WS   /trade-api/ws/v2              (auth handshake)
```

### Orders: V2 event orders vs legacy

Kalshi's current order API is **Create Order V2**:
`POST /portfolio/events/orders` on a single unified book —

```json
{"ticker": "…", "side": "bid", "count": "10", "price": "0.3500",
 "time_in_force": "good_till_canceled",
 "self_trade_prevention_type": "taker_at_cross",
 "client_order_id": "<uuid>"}
```

`side=bid` buys YES at `price`; `side=ask` sells YES at `price` (prices
are fixed-point dollar strings; counts are fixed-point strings). Cancel
V2 is `DELETE /portfolio/events/orders/{order_id}`.

The legacy shape (`POST /portfolio/orders` with `action`/`side:
yes|no`/`yes_price` in integer cents) was announced as deprecated no
earlier than 2026-05-06. `orders create --legacy` and
`orders cancel --legacy` still build the legacy requests for parity with
older SDKs, but V2 is the default and the recommended path. Amend and
batch operations exist only in the legacy namespace
(`/portfolio/orders/{id}/amend`, `/portfolio/orders/batched`) as of this
writing — a documented gap of the V2 surface, not of this CLI.

### Order safety

Every mutating command is **dry-run by default**: without `--live` it
prints the exact request it would send (method, URL, JSON body) and
sends nothing — no credentials are even required to produce the preview.
`--live` is required to submit. (Sibling CLIs used `--execute`; Kalshi
brief standardized on `--live`, matching the Alpaca CLI convention.)

### Streaming

`stream subscribe --channel ticker|orderbook_delta|trade --ticker …`
opens the WebSocket with signed handshake headers
(sign `GET /trade-api/ws/v2`), sends
`{"id":1,"cmd":"subscribe","params":{"channels":[…],"market_tickers":[…]}}`,
and prints each message as JSON until `--max-messages` or `--duration`
is reached (or Ctrl-C). Implemented channels: `ticker`,
`orderbook_delta` (snapshot + delta messages are printed raw), `trade`.
Private channels (`fill`, `market_positions`, …) are accepted by the
`--channel` flag but are not specially decoded — they print raw like
everything else.

## Live response shapes (verified 2026-10-24 against prod)

Current live responses are **fixed-point**: prices are dollar strings
(`yes_price_dollars: "0.7200"`), counts are `*_fp` strings
(`count_fp: "22.10"`, `open_interest_fp`), and the order book comes back
as `orderbook_fp` with `yes_dollars` / `no_dollars` arrays of
`[price, quantity]` string pairs. Older documentation and SDKs describe
integer-cent fields (`yes_bid`, `yes_price`) and an `orderbook` object;
those shapes belong to the legacy API surface. This CLI prints responses
verbatim, so both shapes pass through untouched — parse the `_fp` /
`_dollars` fields in new code. Market objects report `"status":
"active"` in the payload while list filters accept `open`.

## Known gaps / not implemented

- **Order groups** (`/portfolio/order_groups/*`), **decrease order**
  (`POST /portfolio/orders/{id}/decrease`), **RFQ** endpoints,
  **multivariate event collections**, **milestones**, **live data /
  series settlement sources**, and the **historical tier**
  (`/historical/markets`, `/historical/trades`,
  `/historical/markets/{ticker}/candlesticks`) are documented Kalshi
  endpoints not covered by this CLI's command tree.
- **External feeds WebSocket** (`wss://external-feeds.kalshi.com/…`) —
  institutional feed product; not implemented.
- No local order-book reconstruction from deltas (messages print raw).
- No credential storage of its own: the CLI reads a PEM file path; it
  never writes key material anywhere.
