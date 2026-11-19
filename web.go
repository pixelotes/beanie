package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
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

type page struct{ title, text, url string }

// fetchPage downloads a URL and extracts its readable text.
func fetchPage(ctx context.Context, rawURL string) (*page, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("URL no válida %q: debe empezar por http:// o https://", rawURL)
	}
	resp, err := get(ctx, u.String(), browserUA, map[string]string{
		"Accept": "text/html,application/xhtml+xml,text/plain,application/json;q=0.9,*/*;q=0.5",
	})
	if err != nil {
		return nil, err
	}

	ct := strings.ToLower(resp.contentType)
	body := decodeBody(resp.body, resp.contentType)
	p := &page{url: resp.finalURL}
	switch {
	case ct == "" || strings.Contains(ct, "html") || strings.Contains(ct, "xml"):
		p.title, p.text = htmlToText(parseHTML(body))
	case strings.HasPrefix(ct, "text/") || strings.Contains(ct, "json") || strings.Contains(ct, "markdown"):
		p.text = body
	default:
		return nil, fmt.Errorf("tipo de contenido no soportado: %s (solo HTML y texto)", resp.contentType)
	}
	return p, nil
}

func webFetch(ctx context.Context, rawURL string, start int) (string, error) {
	p, err := fetchPage(ctx, rawURL)
	if err != nil {
		return "", err
	}
	head := "URL: " + p.url + "\n\n"
	if p.title != "" {
		head = "# " + p.title + "\n" + head
	}
	if strings.TrimSpace(p.text) == "" {
		return head + "(La página no tiene texto legible; puede que necesite JavaScript.)", nil
	}
	return head + window(p.text, start), nil
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
// web_search backends: DuckDuckGo Lite scraper here, the rest in search.go.
// ---------------------------------------------------------------------------

type searchResult struct {
	title, url, snippet string
	ad                  bool
}

// Pages read per search so the model gets content, not just links.
const (
	searchFetchN     = 3
	searchExcerpt    = 1500
	searchFetchLimit = 6 * time.Second
	snippetChars     = 600
)

// ddgSearch is the DuckDuckGo Lite backend of web_search (no API key).
func ddgSearch(ctx context.Context, query string, n int, withContent bool) (string, error) {
	results, err := searchDDG(ctx, query)
	if err != nil {
		return "", err
	}
	return formatResults(ctx, query, results, n, withContent)
}

// formatResults lists up to n results; with withContent it also reads the
// first pages so the model gets content, not just links.
func formatResults(ctx context.Context, query string, results []searchResult, n int, withContent bool) (string, error) {
	if len(results) == 0 {
		return "", errNoResults
	}
	if len(results) > n {
		results = results[:n]
	}

	excerpts := make([]string, len(results))
	if withContent {
		fctx, cancel := context.WithTimeout(ctx, searchFetchLimit)
		var wg sync.WaitGroup
		for i := range results[:min(searchFetchN, len(results))] {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if p, err := fetchPage(fctx, results[i].url); err == nil {
					excerpts[i] = excerpt(p.text, query, searchExcerpt)
				}
			}(i)
		}
		wg.Wait()
		cancel()
	}

	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, r.title, r.url)
		if excerpts[i] != "" {
			fmt.Fprintf(&b, "   Contenido:\n%s\n\n", indent(excerpts[i], "   > "))
		} else if r.snippet != "" {
			fmt.Fprintf(&b, "   %s\n", truncate(r.snippet, snippetChars))
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// excerpt keeps the paragraphs that best match the query, in page order,
// up to limit characters; the first paragraph wins ties as it usually
// carries the lede.
func excerpt(text, query string, limit int) string {
	paras := strings.Split(text, "\n\n")
	terms := strings.Fields(strings.ToLower(query))
	type scored struct{ i, score int }
	var cands []scored
	for i, p := range paras {
		p = strings.TrimSpace(p)
		if len([]rune(p)) < 40 || strings.HasPrefix(p, "#") {
			continue
		}
		lp := strings.ToLower(p)
		score := 0
		for _, t := range terms {
			if len(t) > 2 {
				score += strings.Count(lp, t)
			}
		}
		if len(cands) == 0 {
			score++
		}
		cands = append(cands, scored{i, score})
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].score > cands[b].score })
	keep := map[int]bool{}
	total := 0
	for _, c := range cands {
		l := len([]rune(paras[c.i]))
		if total+l > limit && total > 0 {
			continue
		}
		keep[c.i] = true
		total += l
		if total >= limit {
			break
		}
	}
	var out []string
	for i, p := range paras {
		if keep[i] {
			out = append(out, truncate(strings.TrimSpace(p), limit))
		}
	}
	return strings.Join(out, "\n")
}

func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}

func searchDDG(ctx context.Context, query string) ([]searchResult, error) {
	form := url.Values{"q": {query}, "kl": {"es-es"}}.Encode()
	var resp *response
	// DuckDuckGo sometimes resets automated connections; one retry after a
	// short pause usually gets through.
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://lite.duckduckgo.com/lite/", strings.NewReader(form))
		if err != nil {
			return nil, err
		}
		resp, err = do(req, browserUA, map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
		if err == nil {
			break
		}
		if attempt == 1 || errors.Is(err, errPrivate) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(time.Second):
		}
	}
	return parseDDG(resp.status, decodeBody(resp.body, resp.contentType))
}

// Markers of DuckDuckGo's anti-bot page, which it serves (often with status
// 202) to rate-limited clients and to many datacenter IPs.
var ddgBlockMarkers = []string{"anomaly-modal", "challenge-form", "bots use duckduckgo", "/anomaly.js"}

var errDDGBlocked = errors.New("DuckDuckGo ha bloqueado la búsqueda (límite de peticiones o comprobación anti-bots). " +
	"Vuelve a intentarlo más tarde; mientras, usa news o search_wikipedia, o web_fetch con una URL concreta")

// parseDDG extracts results from a DuckDuckGo Lite page. An empty page that
// looks like the anti-bot challenge is an error, not "no results", so the
// model does not conclude that nothing exists.
func parseDDG(status int, body string) ([]searchResult, error) {
	doc := parseHTML(body)

	var out []searchResult
	var walk func(*node)
	walk = func(n *node) {
		if n.typ == elemNode {
			class := " " + n.attr("class") + " "
			switch {
			case n.tag == "a" && strings.Contains(class, " result-link "):
				target := ddgURL(n.attr("href"))
				out = append(out, searchResult{title: nodeText(n), url: target, ad: isDDGAd(target)})
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
	if len(out) > 0 {
		results := out[:0]
		for _, r := range out {
			if !r.ad {
				results = append(results, r)
			}
		}
		return results, nil
	}
	if status == http.StatusAccepted {
		return nil, errDDGBlocked
	}
	lower := strings.ToLower(body)
	for _, m := range ddgBlockMarkers {
		if strings.Contains(lower, m) {
			return nil, errDDGBlocked
		}
	}
	return nil, nil
}

// isDDGAd spots sponsored entries: organic results are unwrapped to the
// target site, so anything still pointing at duckduckgo.com is an ad
// (y.js click-through) or its "more info" link.
func isDDGAd(target string) bool {
	u, err := url.Parse(target)
	return err == nil && (u.Host == "duckduckgo.com" || strings.HasSuffix(u.Host, ".duckduckgo.com"))
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
