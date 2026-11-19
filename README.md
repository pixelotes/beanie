# Beanie MCP Server

[![CI](https://github.com/pixelotes/beanie/actions/workflows/ci.yml/badge.svg)](https://github.com/pixelotes/beanie/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A minimal [Model Context Protocol](https://modelcontextprotocol.io) server for local models: **a single Go binary with no dependencies** (standard library only), ~7 MB on disk and ~15 MB of RAM.

Tool descriptions and output are in Spanish, since they are aimed at Spanish-speaking chats.

## Tools

| Tool | What it does | Backend |
|---|---|---|
| `web_fetch` | Downloads a page and returns its main text as Markdown | direct |
| `web_search` | Web search that returns page content, not just links | Exa (free tier, optional key), falling back to DuckDuckGo Lite |
| `news` | Current headlines with source and time, or news on a topic | Google News RSS (no API key) |
| `search_wikipedia` | Searches for articles | Wikipedia API |
| `fetch_wikipedia` | Reads an article's intro, one section or the whole text | Wikipedia API |
| `get_weather` | Current weather and a 1–7 day forecast | Open-Meteo (no API key) |
| `get_datetime` | Current date, time and weekday | local |
| `calculate` | Expressions and one-unknown equations | local |

`calculate` accepts loose syntax: `2+2`, `2 + 2`, `2×3`, `2x3`, `2**10`, `2,5 * 2` (decimal comma), `2(3+1)`, `√16`, `3²`, `sin(30°)`, `20% de 150`, `150 + 10%`, `5!`, `log(8, 2)`, `mcd(12, 18)`. Linear and quadratic equations are solved exactly (`x^2 - 5x + 6 = 0`); anything else numerically (`2^x = 1024`, `sin(x) = 0.5`).

Long responses are cut (12,000 characters by default) and tell the model which `start_index` to use to keep reading.

## Usage

Prebuilt binaries for Linux, macOS and Windows, with `SHA256SUMS`, are on the [releases page](https://github.com/pixelotes/beanie/releases). Or build it:

```bash
make build                               # or: go build -trimpath -ldflags="-s -w" -o beanie .
./beanie                                 # stdio (Claude Desktop, etc.)
./beanie -transport http                # HTTP at http://127.0.0.1:8090/mcp
```

With Docker (`scratch` image, ~8 MB; the port is published on 127.0.0.1 only):

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

Run in HTTP mode and allow the UI's origin (requests from any other browser origin get 403):

```bash
BEANIE_CORS_ORIGINS=https://chat.example.com ./beanie -transport http
```

## Configuration

| Variable | Default | |
|---|---|---|
| `MCP_TRANSPORT` | `stdio` | `stdio` or `http` |
| `MCP_ADDR` | `127.0.0.1:8090` | listen address in HTTP mode |
| `BEANIE_TIMEZONE` | `Europe/Madrid` | default zone for `get_datetime` |
| `BEANIE_MAX_CHARS` | `12000` | character cap per response |
| `BEANIE_CORS_ORIGINS` | empty | comma-separated browser origins allowed to call it (`*` for any); others get 403 |
| `BEANIE_TOKEN` | empty | require `Authorization: Bearer <token>` in HTTP mode |
| `BEANIE_INSECURE_NO_TOKEN` | empty | `1` allows listening on a non-loopback address without a token |
| `BEANIE_ALLOW_PRIVATE` | empty | `1` lets `web_fetch` reach local/private IPs |
| `EXA_API_KEY` | empty | Exa API key for `web_search`; without it Exa's keyless free tier is used |
| `BEANIE_EXA` | empty | `0` disables Exa and searches DuckDuckGo only |

## Web search

`web_search` tries [Exa](https://exa.ai) first through its hosted MCP endpoint, which works without a key on a rate-limited free tier and returns highlights from each page. If Exa fails, is rate-limited or finds nothing, it falls back to DuckDuckGo Lite and tells the model so. With DuckDuckGo, Beanie fetches the top 3 results itself and attaches the paragraphs that best match the query.

To use your own Exa key, set `EXA_API_KEY`, or send it per request in HTTP mode with the `X-Exa-Api-Key` header. The header wins over the variable, so each client can bring its own key.

## Security

- No shell commands or code execution: model input never reaches an external process.
- Without `BEANIE_TOKEN`, HTTP mode refuses to listen on anything but loopback (unless `BEANIE_INSECURE_NO_TOKEN=1`). The token is compared in constant time.
- Browser requests whose `Origin` is not in `BEANIE_CORS_ORIGINS` are rejected (DNS rebinding protection, as the MCP spec requires). Clients that are not browsers send no `Origin`.
- Outgoing requests only reach public addresses by default (checked after DNS resolution and on every redirect): loopback, private, link-local (including cloud metadata), CGNAT, NAT64/6to4/Teredo and other special-purpose ranges are blocked, so a page cannot make the model probe your network.
- When DuckDuckGo answers with its anti-bot page, `web_search` reports it as an error instead of "no results". Datacenter IPs are more likely to get it.
- Downloads are capped at 2 MB and 15 s; each tool call times out after 30 s.

## Development

```bash
make test    # go test -race ./...
make lint    # golangci-lint run
make fmt     # gofmt -w .
```

To release, push a tag: `git tag v4.1.0 && git push origin v4.1.0`. The Release workflow tests, cross-compiles and publishes the binaries.

The HTTP transport is stateless Streamable HTTP: `POST /mcp` with a JSON-RPC message returns a JSON reply. There are no sessions and no server-initiated streams. `GET /healthz` returns `ok`.

## License

[MIT](LICENSE)
