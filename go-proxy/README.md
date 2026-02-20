## go-proxy

Re-written proxy apps from Node.js to Go.

Motivation for rewriting in Go: when I deployed the proxy on a low-resource VPS that was also running other applications, I noticed that the previous implementation was consuming a disproportionate amount of disk space and RAM. This is especially critical on minimal VPS setups, so as an experiment I decided to try a minimal implementation in Go — and it resulted in a significant improvement in resource efficiency.

What's needed to be implemented:
- [x] socks5 proxy
- [x] proxy authentication
- [x] last login user date saving
- [x] data usage stats
- [x] CLI commands for creating and deleting users and getting users stats
- [x] Telegram bot CLI commands for creating and deleting admin user
- [x] Telegram bot with commands
- [x] Make optional proxy authentication
- [x] Telegram bot handling updates via webhook
- [x] Add clearing usage stats every month
- [x] Add support for self-signed SSL certificates for Telegram webhook

Additional features:
- [x] In-memory cache for user authentication
- [ ] Config for disabling Telegram bot in app
- [ ] Graceful shutdown
- [ ] Prometheus metrics export
- [ ] Publishing proxy app as a separate image to Docker Hub
- [ ] Add different levels tests
- [ ] Linting and testing via Github Actions
- [ ] Generating deeplink for setting socks5 proxy in Telegram: https://core.telegram.org/api/links#socks5-proxy-links

## Load testing SOCKS5 proxy locally

For repeatable local benchmarks (including on MacBook Pro), use the built-in load test command:

```bash
cd go-proxy
go run ./cmd/loadtest \
    --proxy-addr=127.0.0.1:54321 \
    --target-addr=host.docker.internal:18080 \
    --use-local-sink=true \
    --concurrency=300 \
    --total-connections=5000 \
    --payload-bytes=524288 \
    --username=test1 \
    --password=test1
```

### What the load test measures

- Number of simultaneous connections (`max_simultaneous_connections`)
- Proxy connection latency percentiles: p95, p98, p99, p99.9
- Throughput (overall MB/s and per-connection MB/s)
- Average connection/session time
- Proxy CPU and RAM usage samples (when `--proxy-pid` is set)

### Reports

Each run writes timestamped files to `go-proxy/reports/loadtest` (or `--report-dir`):

- `loadtest-<timestamp>.json` — full structured report
- `connections-<timestamp>.csv` — per-connection samples
- `resources-<timestamp>.csv` — CPU/RAM samples over time
- `summary-<timestamp>.md` — short human-readable summary

These files allow you to track performance dynamics over multiple runs.


### Telegram webhook TLS (self-signed certificates)

If the bot works in webhook mode (`TELEGRAM_USE_WEBHOOKS=1`), you can optionally enable local TLS using a self-signed certificate:

- `TELEGRAM_WEBHOOK_TLS_CERT_PATH` — path to certificate file.
- `TELEGRAM_WEBHOOK_TLS_KEY_PATH` — path to private key file.

TLS is enabled only when **both** variables are set. If one or both are empty, webhook server starts without local TLS.
