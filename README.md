# dsproxy

A local OpenAI-compatible proxy that repairs missing `reasoning_content` for DeepSeek V4 thinking-mode tool calls (used with Cursor and other OpenAI clients).

Set `HOST=0.0.0.0` in `.env` to listen on all interfaces.

## What it fixes

DeepSeek thinking-mode tool calls require prior assistant `reasoning_content` to be sent back. Cursor often omits that field, which causes:

```text
Error 400: The reasoning_content in the thinking mode must be passed back to the API.
```

This proxy caches reasoning from DeepSeek responses and injects it into later requests. It can also mirror thinking tokens into Cursor-visible `<details><summary>Thinking</summary>…</details>` blocks.

## Quick start

```bash
git clone https://github.com/mewisme/dsproxy.git
cd dsproxy
cp .env.example .env
go run ./cmd/dsproxy
```

## Configuration

Copy [`.env.example`](.env.example) to `.env` in the project directory (or place `.env` in `~/.dsproxy/`).

| Variable | Default | Description |
|----------|---------|-------------|
| `HOST` | `127.0.0.1` | Bind address |
| `PORT` | `9999` | Listen port |
| `BASE_URL` | `https://api.deepseek.com` | DeepSeek API base |
| `MODEL` | `deepseek-v4-pro` | Fallback model name |
| `THINKING` | `enabled` | `enabled` or `disabled` |
| `DISPLAY_REASONING` | `true` | Fold thinking into assistant `content` |
| `COLLAPSIBLE_REASONING` | `true` | Use `<details>` blocks for thinking |
| `VERBOSE` | `false` | Verbose logging |
| `MISSING_REASONING_STRATEGY` | `recover` | `recover` or `reject` |
| `REASONING_CONTENT_PATH` | `~/.dsproxy/reasoning_content.sqlite3` | SQLite cache path |
| `CLEAR_REASONING_CACHE` | `false` | Clear cache on startup and exit |

Expose on LAN:

```env
HOST=0.0.0.0
PORT=9999
```

## Cursor setup

1. Start the proxy (`http://127.0.0.1:9999` by default).
2. In Cursor, add a custom model:
   - **Model:** `deepseek-v4-pro` or `deepseek-v4-flash`
   - **API key:** your DeepSeek API key
   - **Base URL:** `http://127.0.0.1:9999/v1`

If Cursor rejects `localhost`, set `HOST=0.0.0.0` and use `http://<your-lan-ip>:9999/v1`.

## Docker

```bash
cp .env.example .env
docker compose up --build
```

Or without Compose:

```bash
docker build -t dsproxy:local .
docker run --rm -p 9999:9999 --env-file .env -v deepseek-cache:/home/nonroot/.dsproxy dsproxy:local
```

Published image:

```bash
docker pull ghcr.io/mewisme/dsproxy:latest
docker run --rm -p 9999:9999 -e HOST=0.0.0.0 --env-file .env ghcr.io/mewisme/dsproxy:latest
```

## Development

```bash
go test ./...
go build -o dsproxy ./cmd/dsproxy
```

## Releases and GHCR

```bash
git tag v0.1.0
git push origin v0.1.0
```

Image: `ghcr.io/mewisme/dsproxy:v0.1.0`

## License

MIT — Copyright (c) 2026 [mewisme](https://github.com/mewisme).
