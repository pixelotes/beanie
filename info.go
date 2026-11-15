package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // time zones without relying on the OS image
)

// ---------------------------------------------------------------------------
// Wikipedia
// ---------------------------------------------------------------------------

var langRe = regexp.MustCompile(`^[a-z]{2,3}(-[a-z]{2,8})?$`)

func wikiLang(lang string) (string, error) {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return "es", nil
	}
	if !langRe.MatchString(lang) {
		return "", fmt.Errorf("idioma no válido %q (usa un código como es, en, fr)", lang)
	}
	return lang, nil
}

var tagRe = regexp.MustCompile(`<[^>]+>`)

func searchWikipedia(ctx context.Context, query, lang string) (string, error) {
	lang, err := wikiLang(lang)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("la búsqueda está vacía")
	}
	u := fmt.Sprintf("https://%s.wikipedia.org/w/rest.php/v1/search/page?limit=6&q=%s", lang, url.QueryEscape(query))
	resp, err := get(ctx, u, beanieUA, nil)
	if err != nil {
		return "", err
	}
	var data struct {
		Pages []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Excerpt     string `json:"excerpt"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(resp.body, &data); err != nil {
		return "", fmt.Errorf("respuesta de Wikipedia no válida")
	}
	if len(data.Pages) == 0 {
		return fmt.Sprintf("Sin resultados en %s.wikipedia.org para %q.", lang, query), nil
	}
	var b strings.Builder
	for i, p := range data.Pages {
		fmt.Fprintf(&b, "%d. %s", i+1, p.Title)
		if p.Description != "" {
			fmt.Fprintf(&b, " — %s", p.Description)
		}
		if ex := strings.Join(strings.Fields(tagRe.ReplaceAllString(p.Excerpt, "")), " "); ex != "" {
			fmt.Fprintf(&b, "\n   %s…", ex)
		}
		b.WriteByte('\n')
	}
	b.WriteString("\nUsa fetch_wikipedia con el título exacto para leer el artículo.")
	return b.String(), nil
}

var headingRe = regexp.MustCompile(`(?m)^(={2,6})\s*(.+?)\s*={2,6}\s*$`)

func fetchWikipedia(ctx context.Context, title, lang, section string, start int) (string, error) {
	title = strings.TrimSpace(title)
	// Accept full article URLs too.
	if u, err := url.Parse(title); err == nil && strings.HasSuffix(u.Host, ".wikipedia.org") && strings.HasPrefix(u.Path, "/wiki/") {
		title = strings.ReplaceAll(strings.TrimPrefix(u.Path, "/wiki/"), "_", " ")
		lang = strings.TrimSuffix(strings.TrimSuffix(u.Host, ".wikipedia.org"), ".m")
	}
	lang, err := wikiLang(lang)
	if err != nil {
		return "", err
	}
	if title == "" {
		return "", fmt.Errorf("falta el título del artículo")
	}
	q := url.Values{
		"action": {"query"}, "prop": {"extracts"}, "explaintext": {"1"}, "redirects": {"1"},
		"format": {"json"}, "formatversion": {"2"}, "titles": {title},
	}
	resp, err := get(ctx, fmt.Sprintf("https://%s.wikipedia.org/w/api.php?%s", lang, q.Encode()), beanieUA, nil)
	if err != nil {
		return "", err
	}
	var data struct {
		Query struct {
			Pages []struct {
				Title   string `json:"title"`
				Missing bool   `json:"missing"`
				Extract string `json:"extract"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.Unmarshal(resp.body, &data); err != nil || len(data.Query.Pages) == 0 {
		return "", fmt.Errorf("respuesta de Wikipedia no válida")
	}
	page := data.Query.Pages[0]
	if page.Missing || page.Extract == "" {
		return "", fmt.Errorf("no existe el artículo %q en %s.wikipedia.org; búscalo primero con search_wikipedia", title, lang)
	}

	text := page.Extract
	link := fmt.Sprintf("https://%s.wikipedia.org/wiki/%s", lang, url.PathEscape(strings.ReplaceAll(page.Title, " ", "_")))
	head := fmt.Sprintf("# %s\n%s\n\n", page.Title, link)

	locs := headingRe.FindAllStringSubmatchIndex(text, -1)
	var names []string
	for _, l := range locs {
		if text[l[2]:l[3]] == "==" { // top-level sections only in the index
			names = append(names, text[l[4]:l[5]])
		}
	}

	section = strings.TrimSpace(section)
	if section == "" {
		intro := text
		if len(locs) > 0 {
			intro = strings.TrimSpace(text[:locs[0][0]])
		}
		out := head + window(intro, start)
		if len(names) > 0 {
			out += "\n\nSecciones: " + strings.Join(names, " · ") +
				"\n(Usa el parámetro section para leer una sección, o section=\"*\" para el artículo completo.)"
		}
		return out, nil
	}
	if section == "*" {
		return head + window(cleanHeadings(text), start), nil
	}

	want := strings.ToLower(section)
	for i, l := range locs {
		if !strings.Contains(strings.ToLower(text[l[4]:l[5]]), want) {
			continue
		}
		level := l[3] - l[2]
		end := len(text)
		for _, m := range locs[i+1:] {
			if m[3]-m[2] <= level {
				end = m[0]
				break
			}
		}
		return head + window(cleanHeadings(strings.TrimSpace(text[l[0]:end])), start), nil
	}
	return "", fmt.Errorf("no hay ninguna sección %q. Secciones: %s", section, strings.Join(names, " · "))
}

// cleanHeadings turns "== Historia ==" into "## Historia".
func cleanHeadings(s string) string {
	return headingRe.ReplaceAllStringFunc(s, func(m string) string {
		p := headingRe.FindStringSubmatch(m)
		return strings.Repeat("#", len(p[1])) + " " + p[2]
	})
}

// ---------------------------------------------------------------------------
// get_weather (Open-Meteo, no API key)
// ---------------------------------------------------------------------------

var wmo = map[int]string{
	0: "despejado", 1: "mayormente despejado", 2: "parcialmente nublado", 3: "nublado",
	45: "niebla", 48: "niebla con escarcha",
	51: "llovizna débil", 53: "llovizna", 55: "llovizna intensa", 56: "llovizna helada", 57: "llovizna helada intensa",
	61: "lluvia débil", 63: "lluvia", 65: "lluvia intensa", 66: "lluvia helada", 67: "lluvia helada intensa",
	71: "nieve débil", 73: "nieve", 75: "nieve intensa", 77: "granos de nieve",
	80: "chubascos débiles", 81: "chubascos", 82: "chubascos violentos",
	85: "chubascos de nieve", 86: "chubascos de nieve intensos",
	95: "tormenta", 96: "tormenta con granizo", 99: "tormenta con granizo fuerte",
}

func getWeather(ctx context.Context, location string, days int) (string, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", fmt.Errorf("falta la ubicación (por ejemplo \"Madrid\" o \"Bilbao, España\")")
	}
	days = max(1, min(days, 7))

	// The geocoder only matches place names: "Bilbao, España" → "Bilbao", then filter by country.
	name, country, _ := strings.Cut(location, ",")
	geo, err := get(ctx, "https://geocoding-api.open-meteo.com/v1/search?count=10&language=es&format=json&name="+
		url.QueryEscape(strings.TrimSpace(name)), beanieUA, nil)
	if err != nil {
		return "", err
	}
	var g struct {
		Results []struct {
			Name      string  `json:"name"`
			Admin1    string  `json:"admin1"`
			Country   string  `json:"country"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := json.Unmarshal(geo.body, &g); err != nil {
		return "", fmt.Errorf("respuesta de geocodificación no válida")
	}
	if len(g.Results) == 0 {
		return "", fmt.Errorf("no se encontró la ubicación %q", location)
	}
	place := g.Results[0]
	if country = strings.ToLower(strings.TrimSpace(country)); country != "" {
		for _, r := range g.Results {
			if strings.Contains(strings.ToLower(r.Country+" "+r.Admin1), country) {
				place = r
				break
			}
		}
	}

	q := url.Values{
		"latitude":      {fmt.Sprint(place.Latitude)},
		"longitude":     {fmt.Sprint(place.Longitude)},
		"current":       {"temperature_2m,apparent_temperature,relative_humidity_2m,precipitation,weather_code,wind_speed_10m"},
		"daily":         {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max,precipitation_sum"},
		"timezone":      {"auto"},
		"forecast_days": {fmt.Sprint(days)},
	}
	fc, err := get(ctx, "https://api.open-meteo.com/v1/forecast?"+q.Encode(), beanieUA, nil)
	if err != nil {
		return "", err
	}
	var f struct {
		Timezone string `json:"timezone"`
		Current  struct {
			Time     string  `json:"time"`
			Temp     float64 `json:"temperature_2m"`
			Feels    float64 `json:"apparent_temperature"`
			Humidity float64 `json:"relative_humidity_2m"`
			Precip   float64 `json:"precipitation"`
			Code     int     `json:"weather_code"`
			Wind     float64 `json:"wind_speed_10m"`
		} `json:"current"`
		Daily struct {
			Time   []string   `json:"time"`
			Code   []int      `json:"weather_code"`
			Max    []float64  `json:"temperature_2m_max"`
			Min    []float64  `json:"temperature_2m_min"`
			Prob   []*float64 `json:"precipitation_probability_max"`
			Precip []float64  `json:"precipitation_sum"`
		} `json:"daily"`
	}
	if err := json.Unmarshal(fc.body, &f); err != nil {
		return "", fmt.Errorf("respuesta meteorológica no válida")
	}

	where := place.Name
	for _, p := range []string{place.Admin1, place.Country} {
		if p != "" && p != place.Name {
			where += ", " + p
		}
	}
	c := f.Current
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\nAhora (%s): %s, %.0f °C (sensación %.0f °C), humedad %.0f%%, viento %.0f km/h",
		where, f.Timezone, strings.Replace(c.Time, "T", " ", 1), wmoText(c.Code), c.Temp, c.Feels, c.Humidity, c.Wind)
	if c.Precip > 0 {
		fmt.Fprintf(&b, ", precipitación %.1f mm", c.Precip)
	}
	b.WriteString("\n\nPrevisión:\n")
	d := f.Daily
	for i := range d.Time {
		if i >= len(d.Code) || i >= len(d.Max) || i >= len(d.Min) {
			break
		}
		day := d.Time[i]
		if t, err := time.Parse("2006-01-02", day); err == nil {
			day = fmt.Sprintf("%s %d %s", weekdays[t.Weekday()][:3], t.Day(), months[t.Month()-1][:3])
		}
		fmt.Fprintf(&b, "- %s: %s, %.0f–%.0f °C", day, wmoText(d.Code[i]), d.Min[i], d.Max[i])
		if i < len(d.Prob) && d.Prob[i] != nil {
			fmt.Fprintf(&b, ", lluvia %.0f%%", *d.Prob[i])
		}
		if i < len(d.Precip) && d.Precip[i] > 0 {
			fmt.Fprintf(&b, " (%.1f mm)", d.Precip[i])
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func wmoText(code int) string {
	if s, ok := wmo[code]; ok {
		return s
	}
	return fmt.Sprintf("código %d", code)
}

// ---------------------------------------------------------------------------
// get_datetime
// ---------------------------------------------------------------------------

var (
	weekdays = []string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}
	months   = []string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio",
		"agosto", "septiembre", "octubre", "noviembre", "diciembre"}
)

func defaultTZ() string {
	if tz := os.Getenv("BEANIE_TIMEZONE"); tz != "" {
		return tz
	}
	return "Europe/Madrid"
}

func getDatetime(tz string) (string, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		tz = defaultTZ()
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return "", fmt.Errorf("zona horaria desconocida %q (usa nombres IANA como Europe/Madrid o America/Mexico_City)", tz)
	}
	now := time.Now().In(loc)
	_, week := now.ISOWeek()
	return fmt.Sprintf("%s, %d de %s de %d, %s (%s, UTC%s)\nISO 8601: %s\nDía del año: %d · Semana ISO: %d",
		weekdays[now.Weekday()], now.Day(), months[now.Month()-1], now.Year(), now.Format("15:04:05"),
		tz, now.Format("-07:00"), now.Format(time.RFC3339), now.YearDay(), week), nil
}
