package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestProtocol(t *testing.T) {
	s := NewServer("beanie", version, "", tools()...)
	ctx := context.Background()

	var init struct {
		Result struct{ ProtocolVersion string } `json:"result"`
	}
	_ = json.Unmarshal(s.handleRaw(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)), &init)
	if init.Result.ProtocolVersion != "2024-11-05" {
		t.Errorf("version negotiation: %+v", init)
	}
	if out := s.handleRaw(ctx, []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)); out != nil {
		t.Errorf("notification got a reply: %s", out)
	}
	out := string(s.handleRaw(ctx, []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"calculate","arguments":{"expression":"2 + 2"}}}`)))
	if !strings.Contains(out, `"2 + 2 = 4"`) || !strings.Contains(out, `"isError":false`) {
		t.Errorf("calculate: %s", out)
	}
	out = string(s.handleRaw(ctx, []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"calculate","arguments":{}}}`)))
	if !strings.Contains(out, `"isError":true`) || !strings.Contains(out, "expression") {
		t.Errorf("missing arg: %s", out)
	}
	out = string(s.handleRaw(ctx, []byte(`{"jsonrpc":"2.0","id":4,"method":"nope"}`)))
	if !strings.Contains(out, "-32601") {
		t.Errorf("unknown method: %s", out)
	}
	out = string(s.handleRaw(ctx, []byte(`[{"jsonrpc":"2.0","id":5,"method":"ping"},{"jsonrpc":"2.0","method":"x"}]`)))
	if !strings.HasPrefix(out, `[{"jsonrpc":"2.0","id":5`) {
		t.Errorf("batch: %s", out)
	}
}

func TestHTMLToText(t *testing.T) {
	page := `<!DOCTYPE html><html><head><title>Hola &amp; adiós</title><style>p{color:red}</style>
<script>if (a < b) { document.write("<p>no</p>") }</script></head>
<body><nav><a href="/">Inicio</a></nav><div class="cookie-banner">Acepta cookies</div>
<article><h1>Título</h1><p>Primer <b>párrafo</b> con&nbsp;texto.<p>Segundo párrafo
<ul><li>uno<li>dos</ul><table><tr><th>A<th>B<tr><td>1<td>2</table>
<pre>  code  here</pre><img src=x alt="un gato"></article><footer>pie</footer></body></html>`
	title, text := htmlToText(parseHTML(page))
	if title != "Hola & adiós" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{"# Título", "Primer párrafo con texto.", "Segundo párrafo", "- uno", "- dos", "| A | B |", "| 1 | 2 |", "```\n  code  here\n```", "[imagen: un gato]"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	for _, bad := range []string{"Inicio", "cookies", "pie", "color", "document"} {
		if strings.Contains(text, bad) {
			t.Errorf("should not contain %q:\n%s", bad, text)
		}
	}
}

func TestWindow(t *testing.T) {
	old := maxChars
	maxChars = 5
	defer func() { maxChars = old }()
	if got := window("abcdefgh", 0); !strings.HasPrefix(got, "abcde") || !strings.Contains(got, "start_index=5") {
		t.Errorf("window: %q", got)
	}
	if got := window("abcdefgh", 5); !strings.HasPrefix(got, "fgh") || strings.Contains(got, "start_index=") {
		t.Errorf("window tail: %q", got)
	}
}

func TestExcerpt(t *testing.T) {
	text := "Lede paragraph that introduces the story in general terms.\n\n" +
		"Unrelated paragraph about the weather and nothing else at all.\n\n" +
		"The election results show the ruling party lost the election narrowly."
	got := excerpt(text, "election results", 140)
	if !strings.Contains(got, "election results show") || strings.Contains(got, "weather") {
		t.Errorf("excerpt picked the wrong paragraphs: %q", got)
	}
}

func TestIsPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"1.1.1.1": true, "2606:4700:4700::1111": true, "8.8.8.8": true,
		"127.0.0.1": false, "10.0.0.1": false, "192.168.1.1": false, "172.16.0.1": false,
		"169.254.169.254": false, "100.64.0.1": false, "100.127.255.254": false,
		"0.0.0.0": false, "0.1.2.3": false, "198.18.0.1": false, "255.255.255.255": false,
		"::1": false, "::": false, "fe80::1": false, "fd00::1": false,
		"::ffff:127.0.0.1": false, "::ffff:1.1.1.1": true,
		"64:ff9b::7f00:1": false, "2002:7f00:1::": false, "2001:0:7f00:1::": false,
	} {
		if got := isPublic(netip.MustParseAddr(addr)); got != want {
			t.Errorf("isPublic(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestHTTPAuthAndOrigin(t *testing.T) {
	s := NewServer("beanie", version, "", tools()...)
	s.token = "secret"
	s.origins = []string{"https://chat.example.com"}
	ping := `{"jsonrpc":"2.0","id":1,"method":"ping"}`

	for _, c := range []struct {
		name, origin, auth string
		want               int
	}{
		{"no origin, good token", "", "Bearer secret", http.StatusOK},
		{"allowed origin", "https://chat.example.com", "Bearer secret", http.StatusOK},
		{"unknown origin", "https://evil.example", "Bearer secret", http.StatusForbidden},
		{"missing token", "", "", http.StatusUnauthorized},
		{"wrong token", "", "Bearer nope", http.StatusUnauthorized},
	} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(ping))
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		if c.auth != "" {
			req.Header.Set("Authorization", c.auth)
		}
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, rec.Code, c.want)
		}
	}
}

func TestListenNeedsTokenOffLoopback(t *testing.T) {
	t.Setenv("BEANIE_INSECURE_NO_TOKEN", "")
	s := NewServer("beanie", version, "", tools()...)
	s.token = ""
	if err := s.ListenHTTP(":0"); err == nil || !strings.Contains(err.Error(), "BEANIE_TOKEN") {
		t.Errorf("ListenHTTP on all interfaces without a token: %v", err)
	}
	for addr, want := range map[string]bool{"127.0.0.1:8090": true, "localhost:1": true, "[::1]:1": true, ":8090": false, "0.0.0.0:1": false} {
		if got := isLoopback(addr); got != want {
			t.Errorf("isLoopback(%q) = %v", addr, got)
		}
	}
}

func TestParseDDG(t *testing.T) {
	ok := `<table><tr><td><a class="result-link" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2F">Example</a></td></tr>
<tr><td class="result-snippet">An example page.</td></tr></table>`
	res, err := parseDDG(http.StatusOK, ok)
	if err != nil || len(res) != 1 || res[0].url != "https://example.com/" || res[0].snippet != "An example page." {
		t.Errorf("results: %+v, %v", res, err)
	}
	if _, err := parseDDG(http.StatusAccepted, "<html><body></body></html>"); !errors.Is(err, errDDGBlocked) {
		t.Errorf("status 202: %v", err)
	}
	if _, err := parseDDG(http.StatusOK, `<div class="anomaly-modal__title">Unfortunately, bots use DuckDuckGo too.</div>`); !errors.Is(err, errDDGBlocked) {
		t.Errorf("challenge page: %v", err)
	}
	if res, err := parseDDG(http.StatusOK, "<p>No results.</p>"); err != nil || len(res) != 0 {
		t.Errorf("no results: %+v, %v", res, err)
	}
}

func TestDDGAds(t *testing.T) {
	if !isDDGAd("https://duckduckgo.com/y.js?ad_domain=cleverfiles.com&ad_provider=bingv7aa") {
		t.Error("ad link not detected")
	}
	if !isDDGAd(ddgURL("https://duckduckgo.com/duckduckgo-help-pages/company/ads-by-microsoft-on-duckduckgo-private-search/")) {
		t.Error("ad info link not detected")
	}
	if isDDGAd(ddgURL("//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com")) || isDDGAd("https://example.com/y.js") {
		t.Error("organic link flagged as ad")
	}
}

func TestSSEData(t *testing.T) {
	sse := "event: message\r\ndata: {\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"Title: A\\nURL: https://a.example\"}]}}\r\n\r\n"
	if got := string(sseData([]byte(sse))); !strings.HasPrefix(got, `{"result"`) {
		t.Errorf("sseData = %q", got)
	}
	if got := string(sseData([]byte(` {"a":1} `))); got != `{"a":1}` {
		t.Errorf("plain JSON = %q", got)
	}
}

func TestExaKeySource(t *testing.T) {
	t.Setenv("EXA_API_KEY", "from-env")
	if k := exaKey(context.Background()); k != "from-env" {
		t.Errorf("env key = %q", k)
	}
	if k := exaKey(withExaKey(context.Background(), " from-header ")); k != "from-header" {
		t.Errorf("header key = %q", k)
	}
	if k := exaKey(withExaKey(context.Background(), "")); k != "from-env" {
		t.Errorf("empty header should fall back to env, got %q", k)
	}
}
