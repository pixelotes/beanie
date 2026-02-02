# --- ETAPA 1: Builder de Go ---
FROM golang:1.23-bookworm AS builder

WORKDIR /app

# Copiamos definición de módulos y descargamos
COPY go.mod go.sum ./
# (Si tuvieras go.sum, lo copiarías aquí también)
RUN go mod download || echo "Creando modulo al vuelo..."

COPY main.go .

# Compilación estática para Linux ARM64 (Raspberry Pi 3/4/5)
ENV CGO_ENABLED=0 GOOS=linux GOARCH=arm64
RUN go build -ldflags="-s -w" -o mcp-server main.go

# --- ETAPA 2: Runtime Final ---
FROM python:3.11-slim-bookworm

LABEL description="Pixelotes MCP Server: Go + Python + Tools"

# Instalamos todas las herramientas ("Swiss Army Knife")
# - libqalculate-utils: Matemáticas (qalc)
# - fortune-mod, cowsay, figlet: Diversión y formato
# - pandoc: Conversión de textos
# - bsdmainutils: Calendarios (ncal)
# - curl, wget: Descarga de archivos
# - jq: Procesamiento JSON
# - ffmpeg, imagemagick: Procesamiento multimedia
# - zip, unzip: Compresión
# - iputils-ping, dnsutils: Red
RUN apt-get update && apt-get install -y \
    qalc\
    curl wget jq \
    fortune-mod cowsay figlet \
    bsdmainutils \
    pandoc \
    ffmpeg imagemagick \
    zip unzip \
    iputils-ping dnsutils \
    && rm -rf /var/lib/apt/lists/*

# Añadimos /usr/games al PATH para fortune/cowsay
ENV PATH="$PATH:/usr/games"

COPY requirements.txt .

# Instalamos librería Python para navegación Stealth
RUN pip3 install -r requirements.txt

WORKDIR /app

# Copiamos el binario de Go
COPY --from=builder /app/mcp-server .

# Copiamos configuración y scripts
COPY tools.yaml .
COPY scripts/ ./scripts/

# Ejecutar
ENTRYPOINT ["./mcp-server"]