# dsproxy

OpenAI-compatible local proxy for [DeepSeek V4](https://www.deepseek.com/) thinking mode with tool calls. It repairs missing `reasoning_content` in chat history so clients like [Cursor](https://cursor.com/) can keep multi-turn agent sessions working.

Your API key stays in the client: dsproxy forwards the `Authorization` header to DeepSeek and does not store credentials.

## The problem

DeepSeek thinking mode requires prior assistant `reasoning_content` to be sent back on tool-call turns. Many OpenAI clients omit that field, which produces:

```text
Error 400: The reasoning_content in the thinking mode must be passed back to the API.
```

## What dsproxy does

1. **Caches** `reasoning_content` from DeepSeek responses in a local SQLite database.
2. **Injects** cached reasoning into later requests when the client omits it.
3. **Recovers** broken histories when possible (`MISSING_REASONING_STRATEGY=recover`), or fails fast in strict mode (`reject`).
4. **Optionally mirrors** thinking tokens into assistant `content` so UIs can show them (plain or collapsible `<details>` blocks).
5. **Normalizes** requests for DeepSeek (thinking/reasoning effort, tools, streaming usage, legacy `functions` → `tools`).

Supported endpoints:

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/health`, `/healthz`, `/v1/health`, `/v1/healthz` | Health check |
| `GET` | `/models`, `/v1/models` | Model list for client discovery |
| `POST` | `/chat/completions`, `/v1/chat/completions` | Proxied chat (only supported API) |

## Quick start

### From source

Requires Go 1.26+.

```bash
git clone https://github.com/mewisme/dsproxy.git
cd dsproxy
cp .env.example .env
go run ./cmd/dsproxy
```

Default base URL: `http://127.0.0.1:9999/v1`

### Docker Compose

```bash
cp .env.example .env
# For containers, bind on all interfaces (see Configuration)
echo HOST=0.0.0.0 >> .env

docker compose pull
docker compose up -d
```

Health check: `curl http://127.0.0.1:9999/health`

### Published image (no Compose)

```bash
docker pull ghcr.io/mewisme/dsproxy:latest
docker run --rm -p 9999:9999 \
  -e HOST=0.0.0.0 \
  --env-file .env \
  -v dsproxy-cache:/home/nonroot/.dsproxy \
  ghcr.io/mewisme/dsproxy:latest
```

## Cursor setup

1. Start dsproxy (default `http://127.0.0.1:9999`).
2. In Cursor → **Models** → add a custom model:
   - **Model:** `deepseek-v4-pro` or `deepseek-v4-flash`
   - **API key:** your DeepSeek API key
   - **Base URL:** `http://127.0.0.1:9999/v1`

If Cursor rejects `localhost`, set `HOST=0.0.0.0` in `.env` and use `http://<your-lan-ip>:9999/v1` (startup logs list discovered LAN URLs).

## Configuration

Copy [`.env.example`](.env.example) to `.env` in the project directory. If no project `.env` exists, `~/.dsproxy/.env` is also loaded.

| Variable | Default | Description |
|----------|---------|-------------|
| `HOST` | `127.0.0.1` | Bind address. Use `0.0.0.0` for LAN or Docker. |
| `PORT` | `9999` | Listen port |
| `BASE_URL` | `https://api.deepseek.com` | Upstream DeepSeek API base URL |
| `MODEL` | `deepseek-v4-pro` | Default model when the client omits `model` |
| `THINKING` | `enabled` | `enabled` or `disabled` — forwarded as DeepSeek `thinking.type` |
| `REASONING_EFFORT` | `max` | `low` / `medium` / `high` / `max` (aliases normalized upstream) |
| `DISPLAY_REASONING` | `true` | Mirror thinking into streamed assistant `content` |
| `COLLAPSIBLE_REASONING` | `true` | Use `<details><summary>Thinking</summary>…</details>` when displaying |
| `VERBOSE` | `false` | Debug logging (request/response summaries) |
| `REQUEST_TIMEOUT` | `300` | Upstream and server read/write timeout (seconds) |
| `MAX_REQUEST_BODY_BYTES` | `20971520` | Max client request body size (20 MiB) |
| `CORS` | `false` | Send CORS headers on responses |
| `MISSING_REASONING_STRATEGY` | `recover` | `recover` (cache + trim history) or `reject` (HTTP 409) |
| `REASONING_CONTENT_PATH` | `~/.dsproxy/reasoning_content.sqlite3` | SQLite cache file (empty = default path) |
| `REASONING_CACHE_MAX_AGE_SECONDS` | `2592000` | Evict cache rows older than this (30 days) |
| `REASONING_CACHE_MAX_ROWS` | `100000` | Max rows before LRU-style pruning |
| `CLEAR_REASONING_CACHE` | `false` | If `true`, clear cache on startup and exit (one-shot maintenance) |
| `TRACE_DIR` | _(empty)_ | If set, write JSON trace files for debugging |

Cache keys are scoped per API key hash, upstream URL, model family, and thinking settings — different keys do not share reasoning.

### Missing reasoning strategies

- **`recover`** (default): Restore `reasoning_content` from SQLite; if some turns are still missing, drop unrecoverable tail messages and continue (may add a short system notice in the repaired context).
- **`reject`**: Return HTTP 409 with `missing_reasoning_content` when any assistant tool-turn is missing reasoning.

### Displaying thinking in the UI

When `DISPLAY_REASONING=true`, streaming deltas include thinking text in `content`:

- `COLLAPSIBLE_REASONING=true` → HTML `<details>` blocks (Cursor-friendly)
- `COLLAPSIBLE_REASONING=false` → `<think>…</think>` wrappers

Upstream payloads still carry real `reasoning_content` for API correctness.

## Docker notes

- The image runs as user `nonroot` (uid 65532). Cache data lives in `/home/nonroot/.dsproxy` (Compose volume `dsproxy-cache`).
- Set `HOST=0.0.0.0` in `.env` so port publishing works from outside the container.
- `REASONING_CONTENT_PATH` defaults to `/home/nonroot/.dsproxy/reasoning_content.sqlite3` in Compose.

Reset a corrupted cache volume:

```bash
docker compose down
docker volume rm deepseek-v4-proxy_dsproxy-cache   # project name may vary
docker compose up -d
```

Build locally:

```bash
docker build -t dsproxy:local .
docker run --rm -p 9999:9999 -e HOST=0.0.0.0 --env-file .env -v dsproxy-cache:/home/nonroot/.dsproxy dsproxy:local
```

## Development

```bash
go test ./...
go build -o dsproxy ./cmd/dsproxy
```

Project layout:

- `cmd/dsproxy` — entrypoint
- `internal/proxy` — HTTP server and upstream forwarding
- `internal/transform` — request/response normalization and reasoning repair
- `internal/reasoning` — SQLite cache
- `internal/stream` — SSE display adapter

## Releases

Tag a version to publish multi-arch images to GHCR:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Image: [`ghcr.io/mewisme/dsproxy:latest`](https://github.com/mewisme/dsproxy/pkgs/container/dsproxy)

## License

MIT — Copyright (c) 2026 [mewisme](https://github.com/mewisme).
