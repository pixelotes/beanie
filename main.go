// Beanie: a small MCP server for local models, standard library only.
// Tools: web_fetch, web_search, news, search_wikipedia, fetch_wikipedia,
// get_weather, get_datetime, calculate.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/pixelotes/beanie/calc"
)

const version = "4.0.0"

const startDesc = "Carácter desde el que seguir leyendo si la respuesta anterior se cortó. Por defecto 0."

func tools() []Tool {
	return []Tool{
		newTool("web_fetch", "Leer página web",
			"Descarga una página web y devuelve su texto principal en Markdown. Úsala cuando el usuario dé una URL o para leer un resultado de web_search.",
			func(ctx context.Context, a Args) (string, error) {
				u, err := a.Require("url")
				if err != nil {
					return "", err
				}
				return webFetch(ctx, u, a.Int("start_index", 0))
			},
			required(str("url", "URL completa, con http:// o https://")),
			num("start_index", startDesc)),

		newTool("web_search", "Buscar en la web",
			"Busca en internet (DuckDuckGo) y devuelve títulos, URLs y un extracto del contenido de los primeros resultados. Úsala para noticias, datos actuales o cualquier cosa que no sepas.",
			func(ctx context.Context, a Args) (string, error) {
				q, err := a.Require("query")
				if err != nil {
					return "", err
				}
				return webSearch(ctx, q, max(1, min(a.Int("max_results", 6), 10)), a.Bool("include_content", true))
			},
			required(str("query", "Términos de búsqueda")),
			num("max_results", "Número de resultados (1-10). Por defecto 6."),
			boolean("include_content", "Adjuntar un extracto del contenido de los 3 primeros resultados. Por defecto true; usa false para una búsqueda rápida solo con enlaces.")),

		newTool("news", "Noticias",
			"Titulares de actualidad con medio y fecha (Google News). Sin query devuelve las noticias destacadas del momento; con query, las noticias sobre ese tema. Úsala para cualquier pregunta sobre noticias o qué está pasando.",
			func(ctx context.Context, a Args) (string, error) {
				return news(ctx, a.String("query"), a.String("lang"), a.String("country"), max(1, min(a.Int("max_results", 10), 20)))
			},
			str("query", "Tema a buscar. Vacío para los titulares destacados."),
			str("lang", "Idioma (es, en, fr...). Por defecto es."),
			str("country", "País en dos letras (ES, MX, AR, US...). Por defecto el del idioma."),
			num("max_results", "Número de titulares (1-20). Por defecto 10.")),

		newTool("search_wikipedia", "Buscar en Wikipedia",
			"Busca artículos de Wikipedia y devuelve títulos con una descripción breve. Úsala antes de fetch_wikipedia si no conoces el título exacto.",
			func(ctx context.Context, a Args) (string, error) {
				q, err := a.Require("query")
				if err != nil {
					return "", err
				}
				return searchWikipedia(ctx, q, a.String("lang"))
			},
			required(str("query", "Qué buscar")),
			str("lang", "Código de idioma de Wikipedia (es, en, fr...). Por defecto es.")),

		newTool("fetch_wikipedia", "Leer artículo de Wikipedia",
			"Lee un artículo de Wikipedia. Sin section devuelve la introducción y la lista de secciones; con section, solo esa sección.",
			func(ctx context.Context, a Args) (string, error) {
				t, err := a.Require("title")
				if err != nil {
					return "", err
				}
				return fetchWikipedia(ctx, t, a.String("lang"), a.String("section"), a.Int("start_index", 0))
			},
			required(str("title", "Título exacto del artículo o su URL")),
			str("lang", "Código de idioma de Wikipedia (es, en, fr...). Por defecto es."),
			str("section", "Nombre (o parte) de la sección a leer; \"*\" para el artículo completo"),
			num("start_index", startDesc)),

		newTool("get_weather", "Consultar el tiempo",
			"Tiempo actual y previsión diaria de una ciudad (Open-Meteo).",
			func(ctx context.Context, a Args) (string, error) {
				loc, err := a.Require("location")
				if err != nil {
					return "", err
				}
				return getWeather(ctx, loc, a.Int("days", 3))
			},
			required(str("location", "Ciudad, opcionalmente con país: \"Bilbao\" o \"Valencia, Venezuela\"")),
			num("days", "Días de previsión (1-7). Por defecto 3.")),

		newTool("get_datetime", "Fecha y hora",
			"Fecha, hora y día de la semana actuales. Úsala siempre que la respuesta dependa de qué día es hoy (\"mañana\", \"este año\", edades, plazos).",
			func(_ context.Context, a Args) (string, error) {
				return getDatetime(a.String("timezone"))
			},
			str("timezone", fmt.Sprintf("Zona horaria IANA (ej. America/Mexico_City). Por defecto %s.", defaultTZ()))),

		newTool("calculate", "Calcular",
			"Calcula expresiones y resuelve ecuaciones de una incógnita. Ejemplos: \"2+2\", \"(3,5 × 4)^2\", \"sqrt(2)*17^3\", \"20% de 150\", \"150 + 10%\", \"sin(30°)\", \"log(8, 2)\", \"5!\", \"2x + 3 = 11\", \"x^2 - 5x + 6 = 0\", \"2^x = 1024\". Funciones: sqrt, abs, round(x, n), floor, ceil, sin/cos/tan (radianes, ° para grados), asin, ln, log, exp, min, max, mcd, mcm.",
			func(_ context.Context, a Args) (string, error) {
				e, err := a.Require("expression")
				if err != nil {
					return "", err
				}
				res, err := calc.Evaluate(e)
				if err != nil {
					return "", err
				}
				if strings.Contains(e, "=") {
					return res, nil
				}
				return e + " = " + res, nil
			},
			required(str("expression", "Expresión o ecuación"))),
	}
}

func main() {
	transport := flag.String("transport", envOr("MCP_TRANSPORT", "stdio"), "stdio | http")
	addr := flag.String("addr", envOr("MCP_ADDR", ":8090"), "dirección de escucha en modo http")
	flag.Parse()

	s := NewServer("beanie", version,
		"Herramientas de consulta: web, noticias, Wikipedia, tiempo, fecha y cálculo. Usa calculate para cualquier operación numérica en lugar de calcular de cabeza.",
		tools()...)

	var err error
	switch *transport {
	case "http":
		err = s.ListenHTTP(*addr)
	case "stdio":
		err = s.ServeStdio()
	default:
		err = fmt.Errorf("transporte desconocido %q (usa stdio o http)", *transport)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
