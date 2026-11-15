# Beanie MCP Server

[![CI](https://github.com/pixelotes/beanie/actions/workflows/ci.yml/badge.svg)](https://github.com/pixelotes/beanie/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A minimal [Model Context Protocol](https://modelcontextprotocol.io) server for local models: **a single Go binary with no dependencies** (standard library only), ~7 MB on disk and ~15 MB of RAM.

Tool descriptions and output are in Spanish, since they are aimed at Spanish-speaking chats.

## Tools

| Tool | What it does | Backend |
|---|---|---|
| `web_fetch` | Downloads a page and returns its main text as Markdown | direct |
| `web_search` | Web search | DuckDuckGo Lite (scraper, no API key) |
| `search_wikipedia` | Searches for articles | Wikipedia API |
| `fetch_wikipedia` | Reads an article's intro, one section or the whole text | Wikipedia API |
| `get_weather` | Current weather and a 1–7 day forecast | Open-Meteo (no API key) |
| `get_datetime` | Current date, time and weekday | local |
| `calculate` | Expressions and one-unknown equations | local |

`calculate` accepts loose syntax: `2+2`, `2 + 2`, `2×3`, `2x3`, `2**10`, `2,5 * 2` (decimal comma), `2(3+1)`, `√16`, `3²`, `sin(30°)`, `20% de 150`, `150 + 10%`, `5!`, `log(8, 2)`, `mcd(12, 18)`. Linear and quadratic equations are solved exactly (`x^2 - 5x + 6 = 0`); anything else numerically (`2^x = 1024`, `sin(x) = 0.5`).

Long responses are cut (12,000 characters by default) and tell the model which `start_index` to use to keep reading.

## Usage

```bash
make build                               # or: go build -trimpath -ldflags="-s -w" -o beanie .
./beanie                                 # stdio (Claude Desktop, etc.)
./beanie -transport http -addr :8090     # HTTP at http://localhost:8090/mcp
```

With Docker (`scratch` image, ~8 MB):

```bash
docker compose up -d --build
```

Cross-compiling, e.g. for a Raspberry Pi:

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o beanie .
```

### Claude Desktop

```json
{ "mcpServers": { "beanie": { "command": "/path/to/beanie" } } }
```

### Browser clients (llampart, llama.cpp web UI…)

Run in HTTP mode and allow the UI's origin:

```bash
BEANIE_CORS_ORIGINS=https://chat.example.com ./beanie -transport http
```

## Configuration

| Variable | Default | |
|---|---|---|
| `MCP_TRANSPORT` | `stdio` | `stdio` or `http` |
| `MCP_ADDR` | `:8090` | listen address in HTTP mode |
| `BEANIE_TIMEZONE` | `Europe/Madrid` | default zone for `get_datetime` |
| `BEANIE_MAX_CHARS` | `12000` | character cap per response |
| `BEANIE_CORS_ORIGINS` | empty | comma-separated allowed origins (`*` for any) |
| `BEANIE_TOKEN` | empty | require `Authorization: Bearer <token>` in HTTP mode |
| `BEANIE_ALLOW_PRIVATE` | empty | `1` lets `web_fetch` reach local/private IPs |

## Security

- No shell commands or code execution: model input never reaches an external process.
- `web_fetch` blocks loopback and private addresses by default (checked after DNS resolution and on every redirect), so a page cannot make the model probe your network.
- Downloads are capped at 2 MB and 15 s; each tool call times out after 30 s.

## Development

```bash
make test    # go test -race ./...
make lint    # golangci-lint run
make fmt     # gofmt -w .
```

The HTTP transport is stateless Streamable HTTP: `POST /mcp` with a JSON-RPC message returns a JSON reply. There are no sessions and no server-initiated streams. `GET /healthz` returns `ok`.

## License

[MIT](LICENSE)
