package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"syscall"
	"time"
)

const (
	browserUA  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"
	beanieUA   = "beanie-mcp/1.0 (+https://github.com/pixelotes/beanie)"
	maxBody    = 2 << 20 // 2 MiB per download
	reqTimeout = 15 * time.Second
)

// maxChars caps every tool response so one page cannot fill a small model's context.
var maxChars = envInt("BEANIE_MAX_CHARS", 12000)

var allowPrivate = os.Getenv("BEANIE_ALLOW_PRIVATE") == "1"

var errPrivate = errors.New("dirección de red privada o local bloqueada")

// nonPublic lists special-purpose ranges that Go's netip helpers do not flag
// (IANA special-purpose registries), plus IPv6 ranges that embed IPv4.
var nonPublic = func() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range []string{
		"0.0.0.0/8",       // "this network"
		"100.64.0.0/10",   // carrier-grade NAT (and Tailscale)
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // documentation
		"192.88.99.0/24",  // 6to4 relay anycast
		"198.18.0.0/15",   // benchmarking
		"198.51.100.0/24", // documentation
		"203.0.113.0/24",  // documentation
		"240.0.0.0/4",     // reserved, includes broadcast
		"64:ff9b::/96",    // NAT64, embeds an IPv4 address
		"64:ff9b:1::/48",  // local-use NAT64
		"100::/64",        // discard-only
		"2001::/32",       // Teredo, embeds an IPv4 address
		"2001:db8::/32",   // documentation
		"2002::/16",       // 6to4, embeds an IPv4 address
		"fec0::/10",       // deprecated site-local
	} {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}()

// isPublic reports whether ip is a global unicast address on the public
// internet: everything else (loopback, private, link-local, which includes
// cloud metadata at 169.254.169.254, CGNAT, NAT64...) is refused.
func isPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// httpClient only connects to public addresses (checked after DNS
// resolution, so redirects and rebinding are covered too): a page read by
// the model must not be able to make it probe the LAN or the host.
var httpClient = &http.Client{
	Timeout: reqTimeout,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
			Control: func(_, address string, _ syscall.RawConn) error {
				if allowPrivate {
					return nil
				}
				ap, err := netip.ParseAddrPort(address)
				if err != nil || !isPublic(ap.Addr()) {
					return errPrivate
				}
				return nil
			},
		}).DialContext,
		MaxIdleConns:          8,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("demasiadas redirecciones")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("redirección a un esquema no permitido")
		}
		return nil
	},
}

type response struct {
	status      int
	body        []byte
	contentType string
	finalURL    string
}

func get(ctx context.Context, rawURL, ua string, header map[string]string) (*response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return do(req, ua, header)
}

func do(req *http.Request, ua string, header map[string]string) (*response, error) {
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en;q=0.8")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		if errors.Is(err, errPrivate) {
			return nil, errPrivate
		}
		var ue *url.Error
		if errors.As(err, &ue) && ue.Timeout() {
			return nil, fmt.Errorf("tiempo de espera agotado al conectar con %s", req.URL.Host)
		}
		return nil, fmt.Errorf("no se pudo conectar con %s: %w", req.URL.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("el servidor respondió %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("error leyendo la respuesta: %w", err)
	}
	return &response{status: resp.StatusCode, body: body, contentType: resp.Header.Get("Content-Type"), finalURL: resp.Request.URL.String()}, nil
}

// window returns at most maxChars runes starting at start, plus a hint on how
// to continue when the text was cut.
func window(text string, start int) string {
	r := []rune(text)
	if start < 0 {
		start = 0
	}
	if start >= len(r) {
		if len(r) == 0 {
			return "(sin contenido)"
		}
		return fmt.Sprintf("(start_index %d está fuera del texto, que tiene %d caracteres)", start, len(r))
	}
	end := min(start+maxChars, len(r))
	out := string(r[start:end])
	if start > 0 || end < len(r) {
		out += fmt.Sprintf("\n\n[Mostrados los caracteres %d–%d de %d.", start, end, len(r))
		if end < len(r) {
			out += fmt.Sprintf(" Para seguir leyendo usa start_index=%d.", end)
		}
		out += "]"
	}
	return out
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}
