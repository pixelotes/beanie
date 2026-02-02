# Beanie MCP Server

Servidor Model Context Protocol (MCP) completo con herramientas de investigación, cálculo y web scraping. Diseñado para Raspberry Pi y otros sistemas embebidos.

## Características

- **Servidor MCP** en Go con arquitectura de plugins
- **Herramientas integradas**:
  - Calculadora científica (Qalculate)
  - Wolfram Alpha (requiere API Key)
  - Wikipedia Offline (Kiwix)
  - Web scraping inteligente (Jina AI + navegador stealth)
  - Consultas meteorológicas
  - Conversión de formatos (Pandoc)
  
- **Docker Compose** para despliegue simplificado
- **Python 3.11** para scripts especializados
- **Optimizado para ARM64** (Raspberry Pi 3/4/5)

## Requisitos

- Docker & Docker Compose v2.0+
- 2GB RAM mínimo (4GB recomendado)
- 15-20GB espacio en disco (si descargas Wikipedia offline)
- Conexión a internet (para algunas herramientas)

### Instalar Docker

**Linux (Debian/Ubuntu):**
```bash
bash install-docker.sh
# Después: logout y login
```

**macOS/Windows:**
Descarga [Docker Desktop](https://www.docker.com/products/docker-desktop)

## Inicio rápido

### 1. Clonar y preparar

```bash
git clone <repo>
cd raspberry-mcp-server
cp .env.example .env
```

### 2. Configurar (Opcional pero recomendado)

```bash
# Editar .env y agregar WOLFRAM_APPID
nano .env
```

Obtén tu API Key gratuita en: https://www.wolframalpha.com/developers/

### 3. Iniciar servicios

```bash
# Opción A: Con script conveniente
./scripts.sh up

# Opción B: Con docker compose directo
docker compose up -d --build
```

### 4. Verificar

```bash
./scripts.sh ps
./scripts.sh logs-mcp
```

## Documentación

- **[DOCKER_COMPOSE.md](./DOCKER_COMPOSE.md)** - Setup detallado de Docker
- **[tools.yaml](./tools.yaml)** - Definición de todas las herramientas disponibles
- **[main.go](./main.go)** - Código fuente del servidor

## Uso común

### Ver estado
```bash
./scripts.sh ps
```

### Ver logs
```bash
./scripts.sh logs              # Todos los servicios
./scripts.sh logs-mcp          # Solo MCP server
./scripts.sh logs-kiwix        # Solo Kiwix
```

### Ejecutar comando en el contenedor
```bash
./scripts.sh exec ls -la /app
./scripts.sh exec python3 --version
```

### Abrir shell interactivo
```bash
./scripts.sh shell
```

### Detener servicios
```bash
./scripts.sh down
```

### Limpiar todo (incluida base de datos)
```bash
./scripts.sh clean
```

## Servicios

| Servicio | Puerto | Tipo | Descripción |
|----------|--------|------|-------------|
| **MCP Server** | stdio | IPC | Servidor MCP principal |
| **Kiwix** | 8080 | HTTP | Wikipedia offline (opcional) |

## Configuración

Variables en `.env`:

```env
# Wolfram Alpha API Key (obtén en https://www.wolframalpha.com/developers/)
WOLFRAM_APPID=your-api-key

# Nivel de logging (debug, info, warn, error)
LOG_LEVEL=info
```

## Integrando con Claude/Copilot

El servidor expone herramientas vía MCP que pueden ser consumidas por:
- Claude Desktop
- VS Code + GitHub Copilot
- Cualquier cliente MCP compatible

Ver documentación oficial: https://modelcontextprotocol.io

## Troubleshooting

### "connection refused" a Kiwix
```bash
# Kiwix tarda 30-60s en iniciar
sleep 60
./scripts.sh logs-kiwix
```

### Puerto en uso
Edita `docker-compose.yml` y cambia los puertos.

### Wolfram Alpha no funciona
- Verifica `WOLFRAM_APPID` en `.env`
- Comprueba conexión a internet
- Ver logs: `./scripts.sh logs-mcp`

### Espacio en disco bajo
```bash
# Limpiar imágenes y contenedores no usados
docker system prune -a
```

## Licencia

[LICENSE](./LICENSE)

## Contribuciones

Las contribuciones son bienvenidas. Por favor:
1. Fork el proyecto
2. Crea una rama (`git checkout -b feature/mejora`)
3. Commit (`git commit -am 'Agrega mejora'`)
4. Push (`git push origin feature/mejora`)
5. Abre un Pull Request

## Soporte

Para reportar bugs o solicitar features, abre un Issue en GitHub.

---

**Hecho para Raspberry Pi**
