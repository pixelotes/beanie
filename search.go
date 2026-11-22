package main

// web_search runs a chain of search backends, set with BEANIE_SEARCH
// (e.g. "exa,tavily,jina,duckduckgo"): the first one that answers wins, the
// next is tried when one fails, has no results or has no API key. Without
// BEANIE_SEARCH only DuckDuckGo is used.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var (
	errNoResults = errors.New("sin resultados")
	errNoKey     = errors.New("sin clave de API")
)

type searchBackend struct {
	name, label string
	run         func(ctx context.Context, query string, n int, withContent bool) (string, error)
}

var searchBackends = map[string]searchBackend{
	"exa":        {"exa", "Exa", exaBackend},
	"tavily":     {"tavily", "Tavily", tavilySearch},
	"jina":       {"jina", "Jina", jinaSearch},
	"duckduckgo": {"duckduckgo", "DuckDuckGo", ddgSearch},
}

var searchAliases = map[string]string{"ddg": "duckduckgo"}

// searchChain is the backend order for web_search, read once from BEANIE_SEARCH.
var searchChain = parseSearchChain(os.Getenv("BEANIE_SEARCH"), log.Printf)

// parseSearchChain reads a comma-separated list of backends. Unknown names and
// repeats are skipped with a warning; an empty or useless list means DuckDuckGo.
func parseSearchChain(spec string, warn func(string, ...any)) []searchBackend {
	var chain []searchBackend
	seen := map[string]bool{}
	for _, name := range strings.Split(spec, ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if alias, ok := searchAliases[name]; ok {
			name = alias
		}
		b, ok := searchBackends[name]
		if !ok {
			warn("BEANIE_SEARCH: buscador desconocido %q (se ignora; disponibles: exa, tavily, jina, duckduckgo)", name)
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		if key := backendKeyVar(name); key != "" && os.Getenv(key) == "" {
			warn("BEANIE_SEARCH: %s no tiene %s; se saltará", b.label, key)
		}
		chain = append(chain, b)
	}
	if len(chain) == 0 {
		chain = []searchBackend{searchBackends["duckduckgo"]}
	}
	return chain
}

// backendKeyVar names the API key a backend cannot work without ("" if none).
func backendKeyVar(name string) string {
	switch name {
	case "tavily":
		return "TAVILY_API_KEY"
	case "jina":
		return "JINA_API_KEY"
	}
	return ""
}

func webSearch(ctx context.Context, query string, n int, withContent bool) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("la búsqueda está vacía")
	}
	return runSearchChain(ctx, searchChain, query, n, withContent)
}

func runSearchChain(ctx context.Context, chain []searchBackend, query string, n int, withContent bool) (string, error) {
	var failures []string
	var errs []error
	allEmpty := true
	for _, b := range chain {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("búsqueda interrumpida: %w", err)
		}
		out, err := b.run(ctx, query, n, withContent)
		if err == nil {
			head := ""
			if len(failures) > 0 {
				head = fmt.Sprintf("(%s. Resultados de %s.)\n\n", strings.Join(failures, "; "), b.label)
			}
			return head + window(out, 0) +
				fmt.Sprintf("\n\n(Resultados de %s. Usa web_fetch con una URL para leer la página completa.)", b.label), nil
		}
		if errors.Is(err, errNoKey) {
			continue // configured without its key: skip quietly, it was logged at start-up
		}
		if !errors.Is(err, errNoResults) {
			allEmpty = false
		}
		failures = append(failures, fmt.Sprintf("%s: %v", b.label, err))
		errs = append(errs, fmt.Errorf("%s: %w", b.label, err))
	}
	if len(errs) == 0 || allEmpty {
		return "Sin resultados. Prueba con otras palabras.", nil
	}
	return "", fmt.Errorf("ningún buscador respondió (%s): %w", strings.Join(failures, "; "), errors.Join(errs...))
}

// exaBackend adapts exaSearch (exa.go), which works without a key on Exa's free tier.
func exaBackend(ctx context.Context, query string, n int, _ bool) (string, error) {
	out, err := exaSearch(ctx, query, n)
	if errors.Is(err, errExaEmpty) {
		return "", errNoResults
	}
	return out, err
}

// tavilySearch uses Tavily's search API (TAVILY_API_KEY). Its results already
// carry a relevant extract of each page, so no page is fetched here.
func tavilySearch(ctx context.Context, query string, n int, _ bool) (string, error) {
	key := strings.TrimSpace(os.Getenv("TAVILY_API_KEY"))
	if key == "" {
		return "", errNoKey
	}
	body, _ := json.Marshal(map[string]any{
		"query": query, "max_results": n, "search_depth": "basic", "include_answer": false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	resp, err := do(req, beanieUA, map[string]string{
		"Content-Type": "application/json", "Authorization": "Bearer " + key,
	})
	if err != nil {
		return "", err
	}
	results, err := parseTavily(resp.body)
	if err != nil {
		return "", err
	}
	return formatResults(ctx, query, results, n, false)
}

func parseTavily(body []byte) ([]searchResult, error) {
	var data struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("respuesta de Tavily no válida")
	}
	var out []searchResult
	for _, r := range data.Results {
		if r.URL != "" {
			out = append(out, searchResult{title: strings.TrimSpace(r.Title), url: r.URL, snippet: strings.TrimSpace(r.Content)})
		}
	}
	return out, nil
}

// jinaSearch uses Jina's s.jina.ai (JINA_API_KEY) in "no-content" mode, which
// answers fast with title, URL and description; the first pages are then read
// here like DuckDuckGo's, so slow pages cannot stall the whole search.
func jinaSearch(ctx context.Context, query string, n int, withContent bool) (string, error) {
	key := strings.TrimSpace(os.Getenv("JINA_API_KEY"))
	if key == "" {
		return "", errNoKey
	}
	u := "https://s.jina.ai/?" + url.Values{"q": {query}, "count": {fmt.Sprint(n)}}.Encode()
	resp, err := get(ctx, u, beanieUA, map[string]string{
		"Authorization":   "Bearer " + key,
		"Accept":          "application/json",
		"X-Respond-With":  "no-content",
		"X-Retain-Images": "none",
	})
	if err != nil {
		return "", err
	}
	results, err := parseJina(resp.body)
	if err != nil {
		return "", err
	}
	return formatResults(ctx, query, results, n, withContent)
}

func parseJina(body []byte) ([]searchResult, error) {
	var data struct {
		Data []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("respuesta de Jina no válida")
	}
	var out []searchResult
	for _, r := range data.Data {
		if r.URL != "" {
			out = append(out, searchResult{title: strings.TrimSpace(r.Title), url: r.URL, snippet: strings.TrimSpace(r.Description)})
		}
	}
	return out, nil
}
