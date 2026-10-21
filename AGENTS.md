# AGENTS.md

## Project overview
- This repo contains one Go app: `proxy` (SOCKS5 proxy + admin interfaces).
- `docker-compose.yml` runs two services: `proxy` and `redis`.

## Service capabilities
### proxy (Go app)
- Starts a SOCKS5 proxy with optional authentication when `REQUIRE_AUTH=1`.
- Validates credentials against Redis hash `user_auth`, using bcrypt to compare passwords.
- Uses in-memory auth cache (`AUTH_CACHE_MAX_SIZE`, `AUTH_CACHE_TTL`) to reduce Redis/bcrypt load.
- Aggregates per-user data usage in memory and flushes it to Redis in batches (`REDIS_USAGE_FLUSH_INTERVAL`).
- Updates last-auth timestamps via async Redis update queues.
- Runs a monthly job (`0 0 1 * *`) to clear data-usage stats.
- Exposes optional Prometheus metrics (`METRICS_*`) with optional HTTP Basic Auth.
- Supports graceful shutdown for proxy listener, bot, scheduler, and metrics server.

### Telegram bot (embedded in the same Go process)
- Telegram bot for proxy administration; supports polling and webhook mode.
- Optional webhook TLS with self-signed cert/key via `TELEGRAM_WEBHOOK_TLS_*`.
- Access is restricted to admin usernames stored in Redis.
- Commands: `/start`, `/get_users`, `/users_stats`, `/create_user`, `/delete_user`, `/generate_pass [length]`.
- Multi-step create/delete flows are stateful (state stored in Redis).
- On user creation, bot sends host/port/credentials and Telegram SOCKS5 deeplink (`tg://socks?...`).

### CLI admin commands (same Go binary)
- Supported commands: `create-admin`, `delete-admin`, `create-user`, `delete-user`, `users-stats`.
- CLI mode is triggered when command name is passed as first process argument.

### Load testing utility
- `cmd/loadtest` can run local SOCKS5 load tests and generate reports (`json/csv/md`).

## Common commands
- Go proxy tests: `cd proxy && make test`
- Also call linters check after tests: `cd proxy && make lint`
- Build Go binary for Docker (expected at `proxy/dist/proxy`): `cd proxy && make build` (or your local build flow)
- Integration tests: `cd proxy && make regress`
- Run app locally: `cd proxy && go run ./cmd`
- Run CLI command locally: `cd proxy && go run ./cmd create-user` (replace command as needed)
- Run CLI command in container: `docker exec -it <proxy-container> /app/proxy create-user`
- Docker compose: `docker compose up --build`
- Always remove directiories .gocache and .golangci-lint-cache if you create theme after finishing task

## Conventions
- Prefer small, focused changes; keep Go code idiomatic and consistent with existing style.
- Do not add dependencies unless needed; update `go.mod`/`go.sum` accordingly.
- Keep configuration in env vars; see `.env` and `.env.example`.
- Use minimock for creating mocks (where it makes sense)
and put command for generating mocks outside the mock (in the test file).

## Notes for Docker
- `proxy/Dockerfile` expects a prebuilt binary at `proxy/dist/proxy`.
- Redis is required; services should point to `REDIS_HOST=redis`.
- Telegram bot is part of the `proxy` container (no separate `telegram_bot` service).
