package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// news reads the Google News RSS feed: headlines with source and date,
// either the current top stories or the results for a query.

var newsLangRe = regexp.MustCompile(`^[a-z]{2}$`)

type rssFeed struct {
	Items []struct {
		Title   string `xml:"title"`
		Link    string `xml:"link"`
		PubDate string `xml:"pubDate"`
		Source  struct {
			Name string `xml:",chardata"`
			URL  string `xml:"url,attr"`
		} `xml:"source"`
	} `xml:"channel>item"`
}

func news(ctx context.Context, query, lang, country string, n int) (string, error) {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		lang = "es"
	}
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		country = map[string]string{"es": "ES", "en": "US", "pt": "BR", "fr": "FR", "de": "DE", "it": "IT"}[lang]
	}
	if !newsLangRe.MatchString(lang) || !newsLangRe.MatchString(strings.ToLower(country)) {
		return "", fmt.Errorf("lang y country deben ser códigos de dos letras (es, ES)")
	}

	params := url.Values{"hl": {lang}, "gl": {country}, "ceid": {country + ":" + lang}}
	feedURL := "https://news.google.com/rss?" + params.Encode()
	if query = strings.TrimSpace(query); query != "" {
		params.Set("q", query)
		feedURL = "https://news.google.com/rss/search?" + params.Encode()
	}
	resp, err := get(ctx, feedURL, browserUA, nil)
	if err != nil {
		return "", err
	}
	var feed rssFeed
	if err := xml.Unmarshal(resp.body, &feed); err != nil {
		return "", fmt.Errorf("respuesta de Google News no válida")
	}
	if len(feed.Items) == 0 {
		return "No hay noticias para esa búsqueda. Prueba con menos palabras.", nil
	}
	if len(feed.Items) > n {
		feed.Items = feed.Items[:n]
	}

	loc, err := time.LoadLocation(defaultTZ())
	if err != nil {
		loc = time.UTC
	}
	var b strings.Builder
	if query == "" {
		fmt.Fprintf(&b, "Titulares destacados (%s):\n", country)
	} else {
		fmt.Fprintf(&b, "Noticias sobre %q:\n", query)
	}
	for i, it := range feed.Items {
		title := strings.TrimSpace(it.Title)
		// Google appends " - Source" to every title.
		if src := it.Source.Name; src != "" {
			title = strings.TrimSuffix(title, " - "+src)
		}
		fmt.Fprintf(&b, "%d. %s", i+1, title)
		var meta []string
		if it.Source.Name != "" {
			meta = append(meta, it.Source.Name)
		}
		if t, err := time.Parse(time.RFC1123, it.PubDate); err == nil {
			t = t.In(loc)
			meta = append(meta, fmt.Sprintf("%d %s %s", t.Day(), months[t.Month()-1][:3], t.Format("15:04")))
		}
		if len(meta) > 0 {
			fmt.Fprintf(&b, " — %s", strings.Join(meta, ", "))
		}
		b.WriteByte('\n')
	}
	b.WriteString("\nPara ampliar una noticia, búscala con web_search (título + medio).")
	return b.String(), nil
}
