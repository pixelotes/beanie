package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// web_fetch
// ---------------------------------------------------------------------------

var metaCharset = regexp.MustCompile(`(?i)<meta[^>]+charset=["']?\s*([a-z0-9_-]+)`)

func decodeBody(body []byte, contentType string) string {
	cs := ""
	if i := strings.Index(strings.ToLower(contentType), "charset="); i >= 0 {
		cs = contentType[i+8:]
	} else if m := metaCharset.FindSubmatch(body[:min(len(body), 4096)]); m != nil {
		cs = string(m[1])
	}
	cs = strings.ToLower(strings.Trim(cs, `"' ;`))
	if strings.Contains(cs, "8859-1") || strings.Contains(cs, "1252") || cs == "latin1" ||
		(cs == "" && !utf8.Valid(body)) {
		return latin1ToUTF8(body)
	}
	return strings.ToValidUTF8(string(body), "�")
}

func webFetch(ctx context.Context, rawURL string, start int) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("URL no válida %q: debe empezar por http:// o https://", rawURL)
	}
	resp, err := get(ctx, u.String(), browserUA, map[string]string{
		"Accept": "text/html,application/xhtml+xml,text/plain,application/json;q=0.9,*/*;q=0.5",
	})
	if err != nil {
		return "", err
	}

	ct := strings.ToLower(resp.contentType)
	body := decodeBody(resp.body, resp.contentType)
	var title, text string
	switch {
	case ct == "" || strings.Contains(ct, "html") || strings.Contains(ct, "xml"):
		title, text = htmlToText(parseHTML(body))
	case strings.HasPrefix(ct, "text/") || strings.Contains(ct, "json") || strings.Contains(ct, "markdown"):
		text = body
	default:
		return "", fmt.Errorf("tipo de contenido no soportado: %s (solo HTML y texto)", resp.contentType)
	}

	head := "URL: " + resp.finalURL + "\n\n"
	if title != "" {
		head = "# " + title + "\n" + head
	}
	if strings.TrimSpace(text) == "" {
		return head + "(La página no tiene texto legible; puede que necesite JavaScript.)", nil
	}
	return head + window(text, start), nil
}

var skipTags = set("script", "style", "noscript", "iframe", "svg", "nav", "header", "footer",
	"aside", "form", "button", "template", "select", "dialog", "canvas", "object", "head", "textarea")

var blockTags = set("p", "div", "section", "article", "main", "ul", "ol", "table", "blockquote",
	"figure", "figcaption", "dl", "dt", "dd", "details", "summary", "hr", "address")

var junkClass = regexp.MustCompile(`(?i)\b(cookie|consent|banner|share|social|related|newsletter|subscribe|advert|promo|sidebar|breadcrumb|comments?)\b`)

func findTag(n *node, tag string) *node {
	if n.typ == elemNode && n.tag == tag {
		return n
	}
	for _, c := range n.children {
		if f := findTag(c, tag); f != nil {
			return f
		}
	}
	return nil
}

func nodeText(n *node) string {
	var b strings.Builder
	var walk func(*node)
	walk = func(n *node) {
		if n.typ == textNode {
			b.WriteString(n.text)
			b.WriteByte(' ')
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// htmlToText extracts the main content as light Markdown (headings, lists,
// tables, code), dropping navigation, scripts and boilerplate.
func htmlToText(doc *node) (title, text string) {
	if t := findTag(doc, "title"); t != nil {
		title = nodeText(t)
	}
	root := findTag(doc, "article")
	if root == nil {
		root = findTag(doc, "main")
	}
	if root == nil {
		root = findTag(doc, "body")
	}
	if root == nil {
		root = doc
	}

	var b strings.Builder
	endsWith := func(chars string) bool {
		s := b.String()
		return len(s) == 0 || strings.ContainsRune(chars, rune(s[len(s)-1]))
	}
	newline := func(n int) {
		if b.Len() == 0 {
			return
		}
		s := b.String()
		for have := len(s) - len(strings.TrimRight(s, "\n")); have < n; have++ {
			b.WriteByte('\n')
		}
	}
	isSpace := func(c byte) bool { return c == ' ' || c == '\n' || c == '\t' || c == '\r' }

	var walk func(n *node, pre bool)
	walk = func(n *node, pre bool) {
		if n.typ == textNode {
			if pre {
				b.WriteString(n.text)
				return
			}
			t := strings.Join(strings.Fields(n.text), " ")
			if t == "" {
				if n.text != "" && !endsWith(" \n") {
					b.WriteByte(' ')
				}
				return
			}
			if isSpace(n.text[0]) && !endsWith(" \n") {
				b.WriteByte(' ')
			}
			b.WriteString(t)
			if isSpace(n.text[len(n.text)-1]) {
				b.WriteByte(' ')
			}
			return
		}
		if n.typ == elemNode {
			if skipTags[n.tag] || n.attrs["hidden"] != "" || n.attr("aria-hidden") == "true" ||
				n.attr("role") == "navigation" || junkClass.MatchString(n.attr("class")+" "+n.attr("id")) {
				return
			}
			if _, ok := n.attrs["hidden"]; ok {
				return
			}
		}
		children := func(pre bool) {
			for _, c := range n.children {
				walk(c, pre)
			}
		}

		switch n.tag {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			newline(2)
			b.WriteString(strings.Repeat("#", int(n.tag[1]-'0')) + " ")
			children(false)
			newline(2)
		case "li":
			newline(1)
			b.WriteString("- ")
			children(false)
			newline(1)
		case "br":
			newline(1)
		case "pre":
			newline(2)
			b.WriteString("```\n")
			children(true)
			newline(1)
			b.WriteString("```")
			newline(2)
		case "tr":
			newline(1)
			b.WriteString("|")
			children(false)
			newline(1)
		case "td", "th":
			b.WriteByte(' ')
			children(false)
			b.WriteString(" |")
		case "img":
			if alt := strings.TrimSpace(n.attr("alt")); alt != "" && !pre {
				b.WriteString("[imagen: " + alt + "]")
			}
		default:
			if blockTags[n.tag] {
				newline(2)
				children(pre)
				newline(2)
			} else {
				children(pre)
			}
		}
	}
	walk(root, false)

	lines := strings.Split(b.String(), "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, l := range lines {
		l = strings.TrimRight(l, " ")
		if t := strings.TrimSpace(l); t == "" || t == "-" {
			if blank++; blank > 1 {
				continue
			}
			l = ""
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return title, strings.TrimSpace(strings.Join(out, "\n"))
}

// ---------------------------------------------------------------------------
// web_search: plain DuckDuckGo Lite scraper, no API key.
// ---------------------------------------------------------------------------

type searchResult struct{ title, url, snippet string }

func webSearch(ctx context.Context, query string, n int) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("la búsqueda está vacía")
	}
	results, err := searchDDG(ctx, query)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "Sin resultados. Prueba con otras palabras (si se repite, DuckDuckGo puede estar limitando las peticiones).", nil
	}
	if len(results) > n {
		results = results[:n]
	}
	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, r.title, r.url)
		if r.snippet != "" {
			fmt.Fprintf(&b, "   %s\n", truncate(r.snippet, 300))
		}
	}
	b.WriteString("\nUsa web_fetch con una URL para leer la página completa.")
	return b.String(), nil
}

func searchDDG(ctx context.Context, query string) ([]searchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://lite.duckduckgo.com/lite/",
		strings.NewReader(url.Values{"q": {query}, "kl": {"es-es"}}.Encode()))
	if err != nil {
		return nil, err
	}
	resp, err := do(req, browserUA, map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if err != nil {
		return nil, err
	}
	doc := parseHTML(decodeBody(resp.body, resp.contentType))

	var out []searchResult
	var walk func(*node)
	walk = func(n *node) {
		if n.typ == elemNode {
			class := " " + n.attr("class") + " "
			switch {
			case n.tag == "a" && strings.Contains(class, " result-link "):
				out = append(out, searchResult{title: nodeText(n), url: ddgURL(n.attr("href"))})
				return
			case n.tag == "td" && strings.Contains(class, " result-snippet ") && len(out) > 0:
				out[len(out)-1].snippet = nodeText(n)
				return
			}
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(doc)
	return out, nil
}

// ddgURL unwraps DuckDuckGo redirect links (//duckduckgo.com/l/?uddg=...).
func ddgURL(href string) string {
	if u, err := url.Parse(href); err == nil {
		if target := u.Query().Get("uddg"); target != "" {
			return target
		}
	}
	return href
}
