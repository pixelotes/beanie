# Build: static binary, standard library only.
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /beanie .

# Runtime: just the binary and CA certificates (~8 MB image).
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /beanie /beanie
USER 65534:65534
EXPOSE 8090
ENV MCP_TRANSPORT=http MCP_ADDR=:8090
ENTRYPOINT ["/beanie"]
