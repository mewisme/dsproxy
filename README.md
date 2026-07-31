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
6. **Maps** OpenAI-compatible `user` to opaque DeepSeek `user_id` for privacy-safe rate-limit isolation.

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
# Docker: the container always binds 0.0.0.0:9999 (set in docker-compose.yml).
# Use HOST_PORT to change the published port on the host.
# HOST_PORT=8080   # optional — host side only (maps to container :9999)

docker compose pull
docker compose up -d
```

Health check: `curl http://127.0.0.1:9999/health` (use your `HOST_PORT` value if you changed the host mapping)

### Published image (no Compose)

```bash
docker pull ghcr.io/mewisme/dsproxy:latest
docker run --rm -p "${HOST_PORT:-9999}:9999" \
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

## Embedded ngrok

Cursor requires a public HTTPS URL in some environments. dsproxy can create one directly inside the application with the embedded ngrok Go SDK; no ngrok CLI, sidecar, second container, or extra inbound port is used. It is disabled by default.

### Direct run

```bash
NGROK_ENABLED=true \
NGROK_AUTHTOKEN='<token>' \
go run ./cmd/dsproxy
```

Or set it in `.env`:

```env
NGROK_ENABLED=true
NGROK_AUTHTOKEN=<token>
NGROK_URL=
```

Leave `NGROK_URL` empty for a random public URL. dsproxy logs the Cursor-compatible URL after the endpoint is live, for example `https://actual-domain.ngrok.app/v1`. Random URLs can change after restart; local access remains available.

### Reserved URL

```env
NGROK_ENABLED=true
NGROK_AUTHTOKEN=<token>
NGROK_URL=https://my-dsproxy.ngrok.app
```

The reserved domain must be available to your ngrok account. Do not include `/v1` in `NGROK_URL`; use the logged `https://my-dsproxy.ngrok.app/v1` as Cursor's base URL.

### Docker

Compose passes `.env` into the normal application container, so no second container is needed:

```bash
docker compose up -d
docker compose logs -f dsproxy
```

The public endpoint remains protected by dsproxy's existing bearer-token requirement. Traffic through it passes through ngrok infrastructure and can include the DeepSeek API key in the Authorization header, prompts, source code, system instructions, tool schemas/results, and reasoning history. Enable it only when that exposure is acceptable.

## Configuration

Copy [`.env.example`](.env.example) to `.env` in the project directory. If no project `.env` exists, `~/.dsproxy/.env` is also loaded.

**Precedence:** process environment > project `.env` > `~/.dsproxy/.env` > built-in defaults.

| Variable | Default | Description |
|----------|---------|-------------|
| `HOST` | `127.0.0.1` | Bind address. Use `0.0.0.0` for LAN or Docker. |
| `PORT` | `9999` | Listen port (keep `9999` in Docker; app port inside the container) |
| `HOST_PORT` | `9999` | Docker Compose / `docker run -p` only — published port on the host (`HOST_PORT:9999`) |
| `BASE_URL` | `https://api.deepseek.com` | Upstream DeepSeek API base URL |
| `MODEL` | `deepseek-v4-pro` | Default model when the client omits `model` |
| `THINKING` | `enabled` | `enabled` or `disabled` — forwarded as DeepSeek `thinking.type` |
| `REASONING_EFFORT` | `max` | `low` / `medium` / `high` / `max` |
| `DISPLAY_REASONING` | `true` | Mirror thinking into streamed assistant `content` |
| `COLLAPSIBLE_REASONING` | `true` | Use `<details><summary>Thinking</summary>…</details>` when displaying. Only takes effect when `DISPLAY_REASONING=true`. |
| `VERBOSE` | `false` | Debug logging (request/response summaries). **WARNING:** may expose sensitive data in logs, including request bodies and headers. |
| `REQUEST_TIMEOUT` | `300` | Upstream HTTP client timeout and server read timeout (seconds) |
| `MAX_REQUEST_BODY_BYTES` | `20971520` | Max client request body size (20 MiB) |
| `CORS` | `false` | Send CORS headers on all responses (`Access-Control-Allow-Origin: *`). No `Access-Control-Allow-Credentials` is sent. |
| `MISSING_REASONING_STRATEGY` | `recover` | `recover` (cache + trim history) or `reject` (HTTP 409) |
| `REASONING_CONTENT_PATH` | `~/.dsproxy/reasoning_content.sqlite3` | SQLite cache file. Relative paths are resolved against the project directory. Set to `:memory:` for an in-memory store (not persisted). |
| `REASONING_CACHE_MAX_AGE_SECONDS` | `2592000` | Evict cache rows older than this (30 days). Set to `0` to disable age-based eviction. |
| `REASONING_CACHE_MAX_ROWS` | `100000` | Max rows before LRU-style pruning. Set to `0` to disable row-count eviction. |
| `CLEAR_REASONING_CACHE` | `false` | If `true`, clear entire cache on startup and exit immediately (one-shot maintenance). |
| `TRACE_DIR` | _(empty)_ | If set, write one JSON trace file per POST `/v1/chat/completions` request. See [Tracing](#tracing). |
| `NGROK_ENABLED` | `false` | Enable the optional embedded public HTTPS endpoint. |
| `NGROK_AUTHTOKEN` | _(empty)_ | Required only when ngrok is enabled. Never logged. |
| `NGROK_URL` | _(empty)_ | Optional reserved HTTPS endpoint. Leave empty for random URL; omit `/v1`. |

### Cache namespace (v3)

Cache keys are scoped by a namespace derived from:

- API key hash (SHA-256 prefix)
- Upstream base URL
- Model family
- Thinking mode (enabled/disabled)
- Reasoning effort
- Normalized `user_id` (see [User identity mapping](#user-identity-mapping))
- Runtime context hash (system messages, tools, tool_choice)

Different keys, URLs, models, tools, system prompts, or user identities do not share cached reasoning. This prevents cross-tenant reasoning leaks.

The namespace version was bumped from `v2` to `v3` to include normalized `user_id`. Old cached data is not migrated; existing rows may remain in SQLite and age out under normal pruning. Deploying this change may cause a one-time cache miss for existing conversations.

### User identity mapping

dsproxy normalizes OpenAI-compatible `user` and DeepSeek-native `user_id` fields before forwarding to the upstream API.

**Precedence rules:**

1. **Explicit valid `user_id`** is preserved unchanged (must match `^[A-Za-z0-9_-]{1,512}$`)
2. **OpenAI-compatible `user`** is mapped to an opaque, deterministic DeepSeek `user_id` (`u_` prefix + 32 lowercase hex chars)
3. **Both present**: explicit `user_id` wins; `user` is removed
4. **Absent, null, empty, or whitespace**: no identity is sent upstream

The opaque mapping uses HMAC-SHA256 keyed by the request's bearer token with domain separation `dsproxy-user-id-v1`. Properties:

- Same API key + same user → same `user_id`
- Same API key + different user → different `user_id`
- Different API key + same user → different `user_id`
- API key rotation changes generated `user_id`

The upstream DeepSeek payload never contains the raw OpenAI `user` value. Invalid `user_id` values (wrong characters, too long, non-string type) return HTTP 400 with `type: invalid_request_error` before making any upstream request.

### Missing reasoning strategies

- **`recover`** (default): Restore `reasoning_content` from SQLite; if some turns are still missing, drop unrecoverable tail messages and continue (may add a short system notice in the repaired context).
- **`reject`**: Return HTTP 409 with `missing_reasoning_content` when any assistant tool-turn is missing reasoning.

### Displaying thinking in the UI

When `DISPLAY_REASONING=true`, streaming deltas include thinking text in `content`:

- `COLLAPSIBLE_REASONING=true` → HTML `<details>` blocks (Cursor-friendly)
- `COLLAPSIBLE_REASONING=false` → `<think>…</think>` wrappers

Upstream payloads still carry real `reasoning_content` for API correctness.

## Tracing

Set `TRACE_DIR` to a directory path to enable request tracing. Each POST `/v1/chat/completions` request writes one JSON file containing:

- **Timestamp** — when the trace was written
- **Client request** — method, path, headers (with auth redacted)
- **Upstream request** — URL, headers (with auth redacted), body
- **Response status** — upstream HTTP status code

Sensitive headers (`Authorization`, `Proxy-Authorization`, `X-Api-Key`, `Api-Key`) are replaced with `[redacted]` before writing. Trace files are written atomically (temp file + rename) with permissions `0600` in a directory created with `0700`.

Trace failures are logged but never propagated — tracing cannot break request handling.

**Docker:** mount a host directory for trace output:

```yaml
volumes:
  - ./traces:/home/nonroot/traces
environment:
  TRACE_DIR: /home/nonroot/traces
```

## Docker notes

- The image runs as user `nonroot` (uid 65532). Cache data lives in `/home/nonroot/.dsproxy` (Compose volume `dsproxy-cache`).
- Compose reads config from `.env` only (`env_file`). `HOST` and `PORT` are overridden to `0.0.0.0` and `9999` in `docker-compose.yml`. Use `HOST_PORT` to change the published port on the host (`${HOST_PORT}:9999`); the app always listens on `PORT` (**9999**) inside the container.
- `REASONING_CONTENT_PATH` defaults to `/home/nonroot/.dsproxy/reasoning_content.sqlite3` in Compose. Comment out or change this line in `.env` when running directly from source.

Reset a corrupted cache volume:

```bash
docker compose down
docker volume rm deepseek-v4-proxy_dsproxy-cache   # project name may vary
docker compose up -d
```

Build locally:

```bash
docker build -t dsproxy:local .
docker run --rm -p "${HOST_PORT:-9999}:9999" --env-file .env -v dsproxy-cache:/home/nonroot/.dsproxy dsproxy:local
```

## Development

```bash
go test ./...
go test -race ./...
go build -o dsproxy ./cmd/dsproxy
```

Project layout:

- `cmd/dsproxy` — entrypoint
- `internal/config` — configuration loading, validation, and env precedence
- `internal/proxy` — HTTP server, upstream forwarding, CORS, tracing
- `internal/transform` — request/response normalization and reasoning repair
- `internal/reasoning` — SQLite cache with scoped key generation
- `internal/stream` — SSE display adapter
- `internal/jsoncanon` — deterministic JSON marshaling

## Releases

Image: [`ghcr.io/mewisme/dsproxy:latest`](https://github.com/mewisme/dsproxy/pkgs/container/dsproxy)

## License

MIT — Copyright (c) 2026 [mewisme](https://github.com/mewisme).
