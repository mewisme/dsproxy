<div align="center">

# dsproxy

**An OpenAI-compatible proxy that makes DeepSeek V4 thinking mode work reliably in Cursor and other coding agents.**

[![Release](https://github.com/mewisme/dsproxy/actions/workflows/release.yml/badge.svg)](https://github.com/mewisme/dsproxy/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/mewisme/dsproxy?display_name=tag&sort=semver)](https://github.com/mewisme/dsproxy/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/mewisme/dsproxy)](https://github.com/mewisme/dsproxy/blob/main/go.mod)
[![License](https://img.shields.io/github/license/mewisme/dsproxy)](https://github.com/mewisme/dsproxy/blob/main/LICENSE)
[![Container](https://img.shields.io/badge/container-ghcr.io%2Fmewisme%2Fdsproxy-2496ED?logo=docker&logoColor=white)](https://github.com/mewisme/dsproxy/pkgs/container/dsproxy)

[Quick start](#quick-start) · [Cursor setup](#cursor-setup) · [Embedded ngrok](#embedded-ngrok) · [Configuration](#configuration) · [Security](#security)

</div>

> [!IMPORTANT]
> `dsproxy` is an independent compatibility proxy. It is not affiliated with or endorsed by DeepSeek, Cursor, OpenAI, or ngrok.

## Why dsproxy?

DeepSeek thinking-mode tool calls require every assistant tool-call message to be replayed with its original `reasoning_content`. Some OpenAI-compatible clients omit that field when they send conversation history back to the API, causing requests to fail with errors such as:

```text
Error 400: The reasoning_content in the thinking mode must be passed back to the API.
```

`dsproxy` sits between the client and DeepSeek and keeps that conversation state usable:

- stores returned `reasoning_content` in a local SQLite cache;
- restores missing reasoning for later tool-call turns;
- recovers from unrecoverable history when configured to do so;
- preserves streaming, tool calls, usage chunks, and OpenAI-compatible response shapes;
- optionally mirrors thinking into assistant `content` for clients that cannot render `reasoning_content` directly;
- maps OpenAI-compatible `user` values to opaque DeepSeek `user_id` values;
- can expose the same proxy handler through an embedded public ngrok HTTPS endpoint.

## Features

- **OpenAI-compatible endpoints** for models and chat completions
- **DeepSeek V4 thinking mode** with `reasoning_content` replay
- **Streaming SSE support** with optional reasoning display
- **Tool-call history repair** backed by SQLite
- **Strict or recovery mode** for missing reasoning
- **Privacy-safe user identity mapping** and cache isolation
- **Embedded ngrok** without a CLI, subprocess, or sidecar container
- **Docker and direct-run support**
- **Strict configuration validation** with deterministic precedence
- **Optional request tracing** with sensitive header redaction
- **Graceful shutdown** for local and public listeners

## Supported endpoints

| Method | Endpoint | Purpose |
|---|---|---|
| `GET` | `/v1/models` | List supported DeepSeek models |
| `GET` | `/models` | Compatibility alias for `/v1/models` |
| `POST` | `/v1/chat/completions` | OpenAI-compatible chat completions |
| `POST` | `/chat/completions` | Compatibility alias for `/v1/chat/completions` |

Chat-completion requests require an `Authorization: Bearer ...` header. The bearer token is forwarded to the configured upstream DeepSeek-compatible API. The model-list endpoints are read-only discovery routes.

## How it works

```text
Cursor / OpenAI-compatible client
                │
                │  /v1/chat/completions
                ▼
          ┌───────────┐
          │  dsproxy  │
          ├───────────┤
          │ normalize │
          │ repair    │◄──── SQLite reasoning cache
          │ stream    │
          └─────┬─────┘
                │
                │  DeepSeek Chat Completions API
                ▼
         api.deepseek.com
```

When embedded ngrok is enabled, the local TCP listener and the ngrok listener serve the same HTTP handler:

```text
                         ┌── http://127.0.0.1:9999/v1
same dsproxy handler ────┤
                         └── https://<domain>.ngrok.app/v1
```

## Quick start

### Docker Compose

Docker Compose is the simplest persistent setup.

```bash
cp .env.example .env
docker compose up -d
docker compose logs -f dsproxy
```

The default local base URL is:

```text
http://127.0.0.1:9999/v1
```

Update the image later with:

```bash
docker compose pull
docker compose up -d
```

Stop the service with:

```bash
docker compose down
```

The reasoning cache is stored in the named `dsproxy-cache` volume and survives container recreation.

### Docker run

```bash
docker run --rm \
  -p 9999:9999 \
  --env-file .env \
  -e HOST=0.0.0.0 \
  -e PORT=9999 \
  -v dsproxy-cache:/home/nonroot/.dsproxy \
  ghcr.io/mewisme/dsproxy:latest
```

### Run from source

Requirements:

- Go version declared in [`go.mod`](go.mod)
- a DeepSeek-compatible API key supplied by the client

```bash
git clone https://github.com/mewisme/dsproxy.git
cd dsproxy
go run ./cmd/dsproxy
```

Or build a local binary:

```bash
go build -o dsproxy ./cmd/dsproxy
./dsproxy
```

Direct runs bind to `127.0.0.1:9999` by default and work without a `.env` file. When reusing `.env.example` outside Docker, comment out its container-specific `REASONING_CONTENT_PATH` value or replace it with a host path.

## Cursor setup

Configure Cursor to use an OpenAI-compatible provider with:

| Setting | Value |
|---|---|
| Base URL | `http://127.0.0.1:9999/v1` or the logged ngrok URL |
| API key | Your DeepSeek API key |
| Model | `deepseek-v4-pro` or `deepseek-v4-flash` |

Use the base URL exactly as logged by `dsproxy`, including `/v1`.

Some Cursor environments reject private or loopback API URLs. In that case, enable the embedded ngrok endpoint described below instead of exposing the local port manually.

## Embedded ngrok

`dsproxy` embeds the official ngrok Go SDK. No ngrok CLI, child process, second container, agent configuration file, or additional inbound port is required.

Ngrok is disabled by default.

### Random public URL

Set the following values in `.env`:

```env
NGROK_ENABLED=true
NGROK_AUTHTOKEN=<your-ngrok-authtoken>
NGROK_URL=
```

Then start `dsproxy` normally:

```bash
go run ./cmd/dsproxy
```

or:

```bash
docker compose up -d
docker compose logs -f dsproxy
```

After the endpoint is ready, the application logs a Cursor-compatible URL:

```text
public_base_url=https://example.ngrok.app/v1
```

A randomly assigned URL may change after restart.

### Reserved URL

To use a domain available to your ngrok account:

```env
NGROK_ENABLED=true
NGROK_AUTHTOKEN=<your-ngrok-authtoken>
NGROK_URL=https://my-dsproxy.ngrok.app
```

Do not append `/v1` to `NGROK_URL`; `dsproxy` appends it when displaying the public base URL.

### Failure behavior

When ngrok is enabled:

- a missing token or invalid URL fails configuration validation;
- an authentication or endpoint startup failure stops application startup;
- `dsproxy` does not silently fall back to local-only mode;
- local and ngrok serving are shut down together on a terminal listener failure or operating-system signal.

> [!WARNING]
> An ngrok endpoint is public. Requests can contain the DeepSeek API key, prompts, source code, system instructions, tool definitions, tool results, and reasoning history. Chat-completion requests still require a bearer token, but traffic passes through ngrok infrastructure. Enable the tunnel only when that exposure is acceptable.

## API examples

### List models

```bash
curl http://127.0.0.1:9999/v1/models
```

### Streaming chat completion

```bash
curl -N http://127.0.0.1:9999/v1/chat/completions \
  -H 'Authorization: Bearer sk-your-deepseek-key' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "deepseek-v4-pro",
    "messages": [
      {"role": "user", "content": "Explain this repository."}
    ],
    "stream": true
  }'
```

## Configuration

Copy [`.env.example`](.env.example) to `.env` and adjust it as needed.

Configuration precedence is:

```text
process environment
  > project .env
  > ~/.dsproxy/.env
  > built-in defaults
```

Unknown variables are ignored by the application. Recognized variables are validated before listeners are opened.

| Variable | Default | Description |
|---|---:|---|
| `HOST` | `127.0.0.1` | Local bind address. Docker Compose overrides this to `0.0.0.0`. |
| `PORT` | `9999` | Local listen port. Docker Compose keeps the container port at `9999`. |
| `HOST_PORT` | `9999` | Docker Compose host-side published port. Not used by the Go server directly. |
| `BASE_URL` | `https://api.deepseek.com` | Upstream API base URL, without `/chat/completions`. |
| `MODEL` | `deepseek-v4-pro` | Default upstream model when the client omits `model`. |
| `THINKING` | `enabled` | DeepSeek thinking mode: `enabled` or `disabled`. |
| `REASONING_EFFORT` | `max` | Reasoning effort: `low`, `medium`, `high`, or `max`. |
| `DISPLAY_REASONING` | `true` | Mirror streamed reasoning into assistant `content`. |
| `COLLAPSIBLE_REASONING` | `true` | Wrap displayed reasoning in an HTML `<details>` block. Effective only when display is enabled. |
| `VERBOSE` | `false` | Enable debug request and response summaries. May expose sensitive body content in logs. |
| `REQUEST_TIMEOUT` | `300` | Upstream HTTP client and server read timeout in seconds. |
| `MAX_REQUEST_BODY_BYTES` | `20971520` | Maximum client request body size, in bytes. |
| `CORS` | `false` | Add permissive CORS response headers. Credentials are not enabled. |
| `NGROK_ENABLED` | `false` | Enable the embedded public HTTPS endpoint. |
| `NGROK_AUTHTOKEN` | _(empty)_ | Required only when ngrok is enabled. Never included in the startup summary. |
| `NGROK_URL` | _(empty)_ | Optional reserved HTTPS endpoint. Leave empty for a random URL and omit `/v1`. |
| `REASONING_CONTENT_PATH` | `~/.dsproxy/reasoning_content.sqlite3` | SQLite reasoning-cache path. Relative paths are resolved against the project directory. Use `:memory:` for an in-memory cache. |
| `MISSING_REASONING_STRATEGY` | `recover` | `recover` trims unrecoverable history and continues; `reject` returns HTTP 409. |
| `REASONING_CACHE_MAX_AGE_SECONDS` | `2592000` | Maximum cache age, 30 days by default. Set `0` to disable age-based eviction. |
| `REASONING_CACHE_MAX_ROWS` | `100000` | Maximum row count before pruning. Set `0` to disable row-count eviction. |
| `CLEAR_REASONING_CACHE` | `false` | Clear the entire cache at startup and exit. Intended for one-shot maintenance. |
| `TRACE_DIR` | _(empty)_ | Directory for one JSON trace per chat-completion request. Disabled when empty. |

### Docker-specific values

The Compose file intentionally overrides the in-container listener:

```yaml
environment:
  HOST: "0.0.0.0"
  PORT: "9999"
```

Use `HOST_PORT` to change only the host-side port:

```env
HOST_PORT=8080
```

The resulting local base URL is `http://127.0.0.1:8080/v1`.

## Reasoning repair

### Why reasoning must be replayed

In thinking mode, DeepSeek associates tool calls with the assistant's hidden reasoning. On subsequent requests, tool-call assistant messages must include the original `reasoning_content`. `dsproxy` records that content and restores it when a compatible client omits it.

### Recovery mode

```env
MISSING_REASONING_STRATEGY=recover
```

When a required reasoning entry cannot be restored, `dsproxy` keeps the usable leading system context and current conversation tail, inserts a recovery instruction, and omits older unrecoverable tool-call history.

The client may receive a short recovery notice. A recovery is intentionally visible because conversation history was changed.

### Strict mode

```env
MISSING_REASONING_STRATEGY=reject
```

The request is rejected with HTTP `409 Conflict` when required reasoning cannot be restored.

### Cache namespace v3

Reasoning cache entries are isolated by a namespace derived from:

- upstream base URL;
- model;
- thinking mode;
- reasoning effort;
- a hash of the upstream authorization value;
- normalized `user_id`;
- runtime context, including system messages, tools, and effective `tool_choice`.

Different API keys, users, models, tools, or system prompts do not share portable reasoning entries.

The namespace version was bumped from `v2` to `v3` when user identity isolation was added. Existing `v2` rows are not migrated and age out through the normal pruning process. The first request after an upgrade may therefore miss older cache entries.

## User identity mapping

DeepSeek uses top-level `user_id`, while OpenAI-compatible clients may send `user`.

`dsproxy` applies these rules before calling the upstream API:

1. A valid explicit `user_id` is preserved unchanged.
2. Otherwise, a string `user` is converted to an opaque deterministic identifier.
3. When both are present, the explicit valid `user_id` wins.
4. Null, empty, and whitespace-only values are omitted.
5. Invalid explicit values are rejected with HTTP `400` before any upstream request.

Valid explicit IDs match:

```text
^[A-Za-z0-9_-]{1,512}$
```

Generated IDs have this shape:

```text
u_<32 lowercase hexadecimal characters>
```

The mapping uses HMAC-SHA256 keyed by the request bearer token with the domain separator `dsproxy-user-id-v1`. The raw OpenAI `user` value is never forwarded to DeepSeek. Rotating the API key changes generated user IDs.

## Reasoning display

DeepSeek emits thinking text through `reasoning_content`. When `DISPLAY_REASONING=true`, `dsproxy` also mirrors streamed reasoning into normal assistant `content`.

With `COLLAPSIBLE_REASONING=true`, the mirrored content is rendered as:

```html
<details>
<summary>Thinking</summary>

...

</details>
```

The upstream payload still uses real `reasoning_content`; display adaptation affects only the client-facing stream.

## Tracing

Set `TRACE_DIR` to enable JSON request traces:

```env
TRACE_DIR=./traces
```

Each successful upstream request produces a file containing:

- UTC timestamp;
- client method, path, and headers;
- prepared upstream method, URL, headers, and body;
- upstream response status.

Sensitive headers are replaced with `[redacted]`, including:

- `Authorization`;
- `Proxy-Authorization`;
- `X-Api-Key`;
- `Api-Key`.

Trace files are written atomically with restrictive permissions where supported. Trace failures are logged but never break request forwarding.

> [!CAUTION]
> Redacting authentication headers does not make a trace safe to publish. Upstream bodies can still contain prompts, source code, system instructions, tool definitions, local paths, user identifiers, and complete conversation history.

### Tracing with Docker

The named cache volume is not convenient for retrieving traces. Mount a host directory explicitly:

```yaml
services:
  dsproxy:
    volumes:
      - dsproxy-cache:/home/nonroot/.dsproxy
      - ./traces:/home/nonroot/traces
    environment:
      TRACE_DIR: /home/nonroot/traces
```

## Security

- Chat-completion endpoints require a bearer token; model discovery remains read-only.
- The bearer token is sent to the configured upstream API.
- Use an HTTPS `BASE_URL` unless the upstream is a trusted local service.
- `NGROK_ENABLED` is off by default because it creates a public endpoint.
- `NGROK_AUTHTOKEN` is not included in the startup summary.
- `VERBOSE=true` can expose request and response content in logs.
- `TRACE_DIR` can persist sensitive prompts and workspace context.
- CORS is disabled by default.
- Docker runs the application as a non-root user.

Treat the SQLite cache, traces, application logs, and mounted volumes as sensitive data.

## Cache maintenance

Clear the configured cache and exit:

```bash
CLEAR_REASONING_CACHE=true ./dsproxy
```

With Docker Compose:

```bash
docker compose run --rm -e CLEAR_REASONING_CACHE=true dsproxy
```

To remove the entire Compose cache volume:

```bash
docker compose down -v
```

## Development

```bash
go mod download
go test ./...
go test -race ./...
go vet ./...
go build -o dsproxy ./cmd/dsproxy
```

Validate Docker configuration and build the image:

```bash
docker compose config
docker build -t dsproxy:dev .
```

### Project layout

```text
cmd/dsproxy          application entrypoint and lifecycle
internal/config      environment loading and strict validation
internal/jsoncanon   deterministic JSON canonicalization
internal/log         structured logging helpers
internal/proxy       HTTP handler, local serving, tracing, and upstream forwarding
internal/reasoning   SQLite reasoning cache and scoped lookup logic
internal/stream      SSE accumulation and client display adaptation
internal/transform   request normalization, reasoning repair, and response rewriting
internal/tunnel      embedded ngrok provider abstraction and implementation
```

## Releases

Tagged releases build and publish multi-architecture container images through the repository release workflow.

Primary image:

```text
ghcr.io/mewisme/dsproxy:latest
```

Use a version tag in production when reproducibility matters:

```bash
docker pull ghcr.io/mewisme/dsproxy:<version>
```

See the [GitHub Releases](https://github.com/mewisme/dsproxy/releases) page for available versions.

## License

Distributed under the terms in [LICENSE](LICENSE).
