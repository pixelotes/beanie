package main

// Exa search through its hosted MCP endpoint. It works without an API key
// (rate-limited free tier); with a key it is sent as x-api-key on the same
// call. The key comes from the X-Exa-Api-Key header of the incoming HTTP
// request, or from EXA_API_KEY.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	exaEndpoint = "https://mcp.exa.ai/mcp"
	exaTimeout  = 15 * time.Second
	// ExaKeyHeader lets each client bring its own Exa key.
	ExaKeyHeader = "X-Exa-Api-Key"
)

type exaKeyCtx struct{}

func withExaKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, exaKeyCtx{}, strings.TrimSpace(key))
}

func exaKey(ctx context.Context) string {
	if k, _ := ctx.Value(exaKeyCtx{}).(string); k != "" {
		return k
	}
	return strings.TrimSpace(os.Getenv("EXA_API_KEY"))
}

var errExaEmpty = errors.New("sin resultados")

func exaSearch(ctx context.Context, query string, n int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, exaTimeout)
	defer cancel()

	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "web_search_exa",
			"arguments": map[string]any{"query": query, "numResults": n},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, exaEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	header := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json, text/event-stream",
	}
	if key := exaKey(ctx); key != "" {
		header["x-api-key"] = key
	}
	resp, err := do(req, beanieUA, header)
	if err != nil {
		return "", err
	}

	var msg struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(sseData(resp.body), &msg); err != nil {
		return "", fmt.Errorf("respuesta de Exa no válida")
	}
	if msg.Error != nil {
		return "", errors.New(msg.Error.Message)
	}
	var text strings.Builder
	for _, c := range msg.Result.Content {
		text.WriteString(c.Text)
	}
	out := strings.TrimSpace(text.String())
	if msg.Result.IsError {
		// "web_search_exa error (401): Invalid API key. ..." → keep the first line.
		first, _, _ := strings.Cut(out, "\n")
		return "", errors.New(first)
	}
	if out == "" || !strings.Contains(out, "URL:") {
		return "", errExaEmpty
	}
	return out, nil
}

// sseData returns the JSON payload of a Streamable HTTP reply, which may be
// plain JSON or a text/event-stream with "data:" lines.
func sseData(b []byte) []byte {
	s := strings.TrimSpace(string(b))
	if strings.HasPrefix(s, "{") {
		return []byte(s)
	}
	var data []string
	for _, line := range strings.Split(s, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "data:"); ok {
			data = append(data, strings.TrimSpace(rest))
		}
	}
	return []byte(strings.Join(data, "\n"))
}
