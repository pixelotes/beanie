package main

// Minimal MCP server (JSON-RPC 2.0) over stdio and stateless Streamable HTTP.
// Only what a tools-only server needs: initialize, ping, tools/list, tools/call.

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var protocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

type Tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`

	handler func(ctx context.Context, a Args) (string, error)
}

// Args reads tool arguments leniently: small models send numbers as strings.
type Args map[string]any

func (a Args) String(key string) string {
	switch v := a[key].(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func (a Args) Require(key string) (string, error) {
	if v := strings.TrimSpace(a.String(key)); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("falta el parámetro obligatorio %q", key)
}

func (a Args) Bool(key string, def bool) bool {
	switch v := a[key].(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "si", "sí":
			return true
		case "false", "0", "no":
			return false
		}
	}
	return def
}

func (a Args) Int(key string, def int) int {
	switch v := a[key].(type) {
	case float64:
		return int(v)
	case string:
		var n int
		if _, err := fmt.Sscan(strings.TrimSpace(v), &n); err == nil {
			return n
		}
	}
	return def
}

type prop struct {
	name, typ, desc string
	required        bool
}

func str(name, desc string) prop     { return prop{name, "string", desc, false} }
func boolean(name, desc string) prop { return prop{name, "boolean", desc, false} }
func num(name, desc string) prop     { return prop{name, "integer", desc, false} }
func required(p prop) prop           { p.required = true; return p }

func newTool(name, title, desc string, h func(context.Context, Args) (string, error), props ...prop) Tool {
	properties := map[string]any{}
	req := []string{}
	for _, p := range props {
		properties[p.name] = map[string]any{"type": p.typ, "description": p.desc}
		if p.required {
			req = append(req, p.name)
		}
	}
	return Tool{
		Name: name, Title: title, Description: desc,
		InputSchema: map[string]any{"type": "object", "properties": properties, "required": req},
		Annotations: map[string]any{"title": title, "readOnlyHint": true, "destructiveHint": false, "openWorldHint": true},
		handler:     h,
	}
}

type Server struct {
	name, version, instructions string
	tools                       []Tool
	byName                      map[string]*Tool

	// HTTP mode only, read from BEANIE_TOKEN and BEANIE_CORS_ORIGINS.
	token   string
	origins []string
}

func NewServer(name, version, instructions string, tools ...Tool) *Server {
	s := &Server{name: name, version: version, instructions: instructions, tools: tools, byName: map[string]*Tool{}}
	for i := range s.tools {
		s.byName[s.tools[i].Name] = &s.tools[i]
	}
	s.token = os.Getenv("BEANIE_TOKEN")
	for _, o := range strings.Split(os.Getenv("BEANIE_CORS_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			s.origins = append(s.origins, o)
		}
	}
	return s
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

const toolTimeout = 30 * time.Second

// handle processes one request; it returns nil for notifications.
func (s *Server) handle(ctx context.Context, req rpcRequest) *rpcResponse {
	if len(req.ID) == 0 { // notification (notifications/initialized, canceled...)
		return nil
	}
	resp := &rpcResponse{JSONRPC: "2.0", ID: req.ID}
	fail := func(code int, msg string) *rpcResponse {
		resp.Error = &rpcError{code, msg}
		return resp
	}

	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := protocolVersions[0]
		for _, v := range protocolVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		resp.Result = map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
			"instructions":    s.instructions,
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": s.tools}
	case "tools/call":
		var p struct {
			Name      string `json:"name"`
			Arguments Args   `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return fail(-32602, "parámetros no válidos: "+err.Error())
		}
		t, ok := s.byName[p.Name]
		if !ok {
			return fail(-32602, "herramienta desconocida: "+p.Name)
		}
		if p.Arguments == nil {
			p.Arguments = Args{}
		}
		ctx, cancel := context.WithTimeout(ctx, toolTimeout)
		defer cancel()
		out, err := t.handler(ctx, p.Arguments)
		isErr := err != nil
		if isErr {
			out = "Error: " + err.Error()
		}
		resp.Result = map[string]any{
			"content": []map[string]any{{"type": "text", "text": out}},
			"isError": isErr,
		}
	default:
		return fail(-32601, "método no soportado: "+req.Method)
	}
	return resp
}

// handleRaw accepts a single message or a batch and returns the encoded reply
// (nil when there is nothing to answer).
func (s *Server) handleRaw(ctx context.Context, raw []byte) []byte {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '[' {
		var reqs []rpcRequest
		if err := json.Unmarshal(raw, &reqs); err != nil {
			return mustJSON(rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "JSON no válido"}})
		}
		var out []*rpcResponse
		for _, r := range reqs {
			if resp := s.handle(ctx, r); resp != nil {
				out = append(out, resp)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return mustJSON(out)
	}
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return mustJSON(rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "JSON no válido"}})
	}
	if resp := s.handle(ctx, req); resp != nil {
		return mustJSON(resp)
	}
	return nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// ServeStdio reads newline-delimited JSON-RPC from stdin; requests run
// concurrently and replies are written as they finish.
func (s *Server) ServeStdio() error {
	in := bufio.NewReader(os.Stdin)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for {
		line, err := in.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			run := func(line []byte) {
				if out := s.handleRaw(context.Background(), line); out != nil {
					mu.Lock()
					_, _ = os.Stdout.Write(append(out, '\n'))
					mu.Unlock()
				}
			}
			// Only slow tool calls run concurrently; initialize & co. answer in order.
			if strings.Contains(string(line), `"tools/call"`) {
				wg.Add(1)
				go func(line []byte) { defer wg.Done(); run(line) }(line)
			} else {
				run(line)
			}
		}
		if err == io.EOF {
			wg.Wait()
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// ServeHTTP implements stateless Streamable HTTP: POST a JSON-RPC message,
// get a JSON reply. No sessions, no server-initiated streams.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Requests from a browser page carry Origin. The MCP spec requires
	// rejecting unknown origins (DNS rebinding); clients that are not browsers
	// send no Origin and go on to the token check.
	if origin := r.Header.Get("Origin"); origin != "" {
		allowed, wildcard := s.originAllowed(origin)
		if !allowed {
			http.Error(w, "Forbidden origin", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", map[bool]string{true: "*", false: origin}[wildcard])
		h.Set("Access-Control-Allow-Methods", "POST, GET, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization, X-Exa-Api-Key, Mcp-Session-Id, Mcp-Protocol-Version, Last-Event-ID")
		h.Set("Access-Control-Expose-Headers", "Mcp-Session-Id")
		h.Set("Vary", "Origin")
	}

	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
		return
	case http.MethodGet, http.MethodDelete: // no server-initiated stream, no sessions
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	case http.MethodPost:
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.token != "" {
		got := []byte(r.Header.Get("Authorization"))
		want := []byte("Bearer " + s.token)
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	out := s.handleRaw(withExaKey(r.Context(), r.Header.Get(ExaKeyHeader)), body)
	if out == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

// originAllowed reports whether a browser Origin may call the server, and
// whether it was let in by the "*" wildcard.
func (s *Server) originAllowed(origin string) (allowed, wildcard bool) {
	for _, o := range s.origins {
		if o == "*" {
			return true, true
		}
		if strings.EqualFold(o, origin) {
			return true, false
		}
	}
	return false, false
}

// isLoopback reports whether a listen address only accepts local connections.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) ListenHTTP(addr string) error {
	if s.token == "" && !isLoopback(addr) {
		if os.Getenv("BEANIE_INSECURE_NO_TOKEN") != "1" {
			return fmt.Errorf("%s no es una dirección local y BEANIE_TOKEN está vacío: define un token, "+
				"escucha en 127.0.0.1 o, si algo delante ya autentica, pon BEANIE_INSECURE_NO_TOKEN=1", addr)
		}
		log.Printf("aviso: escuchando en %s sin BEANIE_TOKEN (BEANIE_INSECURE_NO_TOKEN=1)", addr)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", s)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok\n") })
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("%s %s escuchando en http://%s/mcp", s.name, s.version, addr)
	return srv.ListenAndServe()
}
