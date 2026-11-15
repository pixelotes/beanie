package main

import (
	"context"
	"encoding/json"
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
