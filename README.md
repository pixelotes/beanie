# Beanie MCP Server

Servidor MCP mínimo para modelos locales: **un único binario en Go, sin dependencias** (solo la librería estándar), ~7 MB y ~15 MB de RAM.

## Herramientas

| Tool | Qué hace | Servicio |
|---|---|---|
| `web_fetch` | Lee una página y devuelve su texto principal en Markdown | directo |
| `web_search` | Búsqueda web | DuckDuckGo Lite (scraper, sin API key) |
| `search_wikipedia` | Busca artículos | API de Wikipedia |
| `fetch_wikipedia` | Lee la introducción, una sección o el artículo completo | API de Wikipedia |
| `get_weather` | Tiempo actual y previsión (1-7 días) | Open-Meteo (sin API key) |
| `get_datetime` | Fecha, hora y día de la semana | local |
| `calculate` | Expresiones y ecuaciones de una incógnita | local |

`calculate` acepta sintaxis variada: `2+2`, `2 + 2`, `2×3`, `2x3`, `2**10`, `2,5 * 2`, `2(3+1)`, `√16`, `3²`, `sin(30°)`, `20% de 150`, `150 + 10%`, `5!`, `log(8, 2)`, `mcd(12, 18)`. Ecuaciones: lineales y cuadráticas exactas (`x^2 - 5x + 6 = 0`), el resto numéricamente (`2^x = 1024`, `sin(x) = 0.5`).

Las respuestas largas se cortan (12 000 caracteres por defecto) e indican el `start_index` para seguir leyendo.

## Uso

```bash
go build -trimpath -ldflags="-s -w" -o beanie .
./beanie                                 # stdio (Claude Desktop, etc.)
./beanie -transport http -addr :8090     # HTTP en http://localhost:8090/mcp
```

Con Docker (imagen `scratch`, ~8 MB):

```bash
docker compose up -d --build
```

Para otra arquitectura (p. ej. Raspberry Pi): `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o beanie .`

### Claude Desktop

```json
{ "mcpServers": { "beanie": { "command": "/ruta/a/beanie" } } }
```

## Configuración (variables de entorno)

| Variable | Por defecto | |
|---|---|---|
| `MCP_TRANSPORT` | `stdio` | `stdio` o `http` |
| `MCP_ADDR` | `:8090` | dirección en modo http |
| `BEANIE_TIMEZONE` | `Europe/Madrid` | zona por defecto de `get_datetime` |
| `BEANIE_MAX_CHARS` | `12000` | tope de caracteres por respuesta |
| `BEANIE_CORS_ORIGINS` | vacío | orígenes permitidos, separados por comas (`*` para todos) |
| `BEANIE_TOKEN` | vacío | exige `Authorization: Bearer <token>` en modo http |
| `BEANIE_ALLOW_PRIVATE` | vacío | `1` permite que `web_fetch` acceda a IPs locales/privadas |

## Seguridad

- No ejecuta comandos de shell ni código: ninguna entrada del modelo llega a un proceso externo.
- `web_fetch` bloquea por defecto direcciones locales y privadas (comprobado tras resolver DNS y en cada redirección), para que una página no pueda hacer que el modelo explore tu red.
- Descargas limitadas a 2 MB y 15 s; cada llamada a una tool tiene un límite de 30 s.

## Tests

```bash
go test ./...
```
