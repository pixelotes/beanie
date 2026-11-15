package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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

// httpClient refuses to connect to loopback/private/link-local addresses
// (checked after DNS resolution, so redirects and rebinding are covered too):
// a page read by the model must not be able to make it probe the LAN.
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
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				ip := net.ParseIP(host)
				if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
					ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
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
		return nil, fmt.Errorf("no se pudo conectar con %s: %v", req.URL.Host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("el servidor respondió %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("error leyendo la respuesta: %v", err)
	}
	return &response{body: body, contentType: resp.Header.Get("Content-Type"), finalURL: resp.Request.URL.String()}, nil
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
