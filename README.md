# kalshi-cli

A Go command-line client for the **Kalshi Trade API v2** — modeled 1:1
on [schwab-cli](https://github.com/rayli09/schwab-cli) /
[etrade-cli](https://github.com/rayli09/etrade-cli) /
[ibkr-cli](https://github.com/rayli09/ibkr-cli) (same layout,
conventions, and safety rails).

Kalshi publishes a REST + WebSocket API and official Python/Rust SDKs;
this project implements the core documented surface as a CLI — exchange,
series, events, markets, portfolio, orders, and streaming — **24 leaf
commands**. Request signing is hand-written on the Go standard library:
Ed25519 direct signing, or RSA-PSS/SHA-256, auto-detected from your PEM
key. See [docs/SPEC.md](docs/SPEC.md) for the full spec, endpoint map,
and known gaps.

## Install

```sh
# With Go 1.27+
go install github.com/rayli09/kalshi-cli@latest

# Or build from source
git clone https://github.com/rayli09/kalshi-cli.git
cd kalshi-cli
go build -o kalshi-cli .
```

## Getting a Kalshi API key

You need a Kalshi account. Public market data needs **no key at all**;
keys are only for portfolio, orders, and streaming.

1. Log in at kalshi.com → **Settings → API Keys** (account settings →
   API).
2. **Create Key.** Keys created since 2026-10-01 are **Ed25519 by
   default** (RSA is still offered). Kalshi shows you the **API key ID**
   and downloads the **private key file** (`.key`, a PEM) **once** —
   store it somewhere safe with `chmod 600`. Kalshi never sees your
   private key and cannot recover it.
3. Production and demo (demo.kalshi.co) accounts/keys are **separate**:
   a demo key only works against demo endpoints, and vice versa.

## Auth setup

```sh
export KALSHI_API_KEY_ID=<your key id>
export KALSHI_PRIVATE_KEY_PATH=~/keys/kalshi.key   # chmod 600

kalshi-cli auth status      # fingerprint + key type only, never key material
```

Credentials can also come from `--api-key-id` / `--private-key-path`
flags, demo-specific `KALSHI_DEMO_API_KEY_ID` /
`KALSHI_DEMO_PRIVATE_KEY_PATH` (with `--demo`), or a JSON config file
(`--config`, default `~/.kalshi-cli.json` when present — keep it 0600;
it stores the key *path*, never key material):

```json
{
  "api_key_id": "...",
  "private_key_path": "/home/you/keys/kalshi.key",
  "demo_api_key_id": "...",
  "demo_private_key_path": "/home/you/keys/kalshi-demo.key"
}
```

Every private request is signed fresh: `KALSHI-ACCESS-*` headers over
`timestamp + METHOD + path` (path from the API root, query string
stripped). There is no login flow and no token to expire — if `auth
status` shows your fingerprint and reachability OK, signed commands
will work.

## Usage

Production is the default; pass `--demo` (or `KALSHI_ENV=demo`) for the
demo environment. Every read command prints the API response as
pretty-printed JSON.

```sh
kalshi-cli exchange status
kalshi-cli exchange schedule

kalshi-cli series list --category Crypto
kalshi-cli series get KXBTC15M
kalshi-cli events list --status open --limit 10
kalshi-cli events get KXFED-26OCT --with-nested-markets

kalshi-cli markets list --status open --series-ticker KXBTC15M --limit 10
kalshi-cli markets get KXBTC15M-26OCT241500-00
kalshi-cli markets orderbook KXBTC15M-26OCT241500-00
kalshi-cli markets trades --ticker KXBTC15M-26OCT241500-00 --limit 20
kalshi-cli markets candlesticks KXBTC15M KXBTC15M-26OCT241500-00 \
  --start-ts 1792864800 --end-ts 1792868400 --period-interval 1

kalshi-cli portfolio balance
kalshi-cli portfolio positions
kalshi-cli portfolio fills --limit 50
kalshi-cli portfolio settlements

kalshi-cli orders list --status resting
kalshi-cli orders get <order-id>

kalshi-cli stream subscribe --channel ticker --ticker KXBTC15M-26OCT241500-00 --max-messages 5
```

Global flags: `--api-key-id`, `--private-key-path`, `--config`,
`--demo`, `--timeout`, `--base-url` (testing override), `--ws-url`,
`--rate-limit` / `--rate-burst` (client-side token bucket, default
10 req/s — deliberately conservative; 429s are retried up to 3 times
honoring `Retry-After`). Every command has `--help`.

> **Response-shape note:** live responses are fixed-point — prices are
> dollar strings (`yes_price_dollars`), counts are `*_fp` strings, and
> the book is `orderbook_fp.yes_dollars` / `no_dollars`
> `[price, quantity]` pairs. Older docs showing integer cents describe
> the legacy surface. Details in docs/SPEC.md.

### Order safety: dry-run by default

`orders create`, `orders amend`, `orders cancel`, `orders cancel-batch`,
and `orders batch-create` **send nothing** unless you pass `--live`.
Without it they print the exact request (method, URL, JSON body) they
would send — no credentials are needed to produce that preview.

```sh
# Preview a V2 event order: bid = buy YES at $0.50, 10 contracts
kalshi-cli orders create --ticker KXBTC15M-26OCT241500-00 \
  --side bid --count 10 --price 0.5000

# Submit it for real
kalshi-cli orders create --ticker KXBTC15M-26OCT241500-00 \
  --side bid --count 10 --price 0.5000 --live

kalshi-cli orders cancel <order-id>            # dry-run
kalshi-cli orders cancel <order-id> --live
```

Create defaults to Kalshi's current **V2 event orders**
(`POST /portfolio/events/orders`, unified book: `bid` buys YES, `ask`
sells YES, fixed-point dollar prices). `--legacy` builds the deprecated
V1 shape (`POST /portfolio/orders`, `action`/`side: yes|no`, prices in
integer cents). Amend and batch operations exist only in the legacy
namespace today — see docs/SPEC.md.

## Parity checklist (Kalshi Trade API v2 → kalshi-cli)

| Area | Commands | Endpoints |
|---|---|---|
| Auth | `auth status` (local + probe) | — / `GET /exchange/status` |
| Exchange | `exchange status`, `exchange schedule` | `GET /exchange/status`, `GET /exchange/schedule` |
| Series | `series list`, `series get` | `GET /series`, `GET /series/{ticker}` |
| Events | `events list`, `events get` | `GET /events`, `GET /events/{event_ticker}` |
| Markets | `markets list`, `get`, `orderbook`, `trades`, `candlesticks` | `GET /markets`, `GET /markets/{ticker}`, `GET /markets/{ticker}/orderbook`, `GET /markets/trades`, `GET /series/{s}/markets/{t}/candlesticks` |
| Portfolio | `portfolio balance`, `positions`, `fills`, `settlements` | `GET /portfolio/balance|positions|fills|settlements` |
| Orders | `orders list`, `get`, `create`, `amend`, `cancel`, `cancel-batch`, `batch-create` | `GET /portfolio/orders[/{id}]`, `POST /portfolio/events/orders`, `POST /portfolio/orders/{id}/amend`, `DELETE /portfolio/events/orders/{id}`, `DELETE|POST /portfolio/orders/batched` |
| Streaming | `stream subscribe` | WS `/trade-api/ws/v2` (`ticker`, `orderbook_delta`, `trade`) |

### Not implemented, honestly

- Order groups, decrease-order, RFQs, multivariate event collections,
  milestones, and the `/historical/*` archive tier (see docs/SPEC.md).
- The institutional external-feeds WebSocket.
- Local order-book reconstruction — stream messages print raw.
- No live order has ever been placed with this tool by its author; the
  signed write paths are covered by stub-server tests and dry-run
  previews only, pending a first demo-account key.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

Tests never touch the real API: signing is verified by round-tripping
Ed25519 and RSA-PSS signatures generated in-test, REST behavior
(including 429 retries and the signature on the wire) is exercised
against a local `httptest` stub, and the dry-run rail is tested by
running the real command tree with no `--live` and asserting the stub
saw zero requests.

CI (GitHub Actions) runs gofmt, vet, build, and tests on every push.

## License & attribution

MIT. The endpoint surface and auth scheme follow the official
[Kalshi API documentation](https://docs.kalshi.com), cross-checked
against the official kalshi-python SDK's endpoint list. Not affiliated
with or endorsed by Kalshi Inc. or KalshiEX LLC. Kalshi event contracts
involve risk; this tool's dry-run default exists for a reason — check
every request before you add `--live`.
