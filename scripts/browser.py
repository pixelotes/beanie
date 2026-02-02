import sys
import re
from curl_cffi import requests
from bs4 import BeautifulSoup

if len(sys.argv) < 2:
    print("Error: Falta URL")
    sys.exit(1)

url = sys.argv[1]

try:
    # 1. NAVEGACIÓN STEALTH
    # Usamos impersonate para parecer un Chrome real
    response = requests.get(
        url,
        impersonate="chrome110",
        headers={
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/110.0.0.0 Safari/537.36",
            "Accept-Language": "es-ES,es;q=0.9,en;q=0.8",
        },
        timeout=15
    )
    
    # Si falla la carga, avisamos
    if response.status_code != 200:
        print(f"Error {response.status_code} al cargar la web.")
        sys.exit(0)

    # 2. LIMPIEZA DE "ZURRASPA" (Parsing)
    soup = BeautifulSoup(response.text, 'html.parser')

    # Lista negra de etiquetas que no aportan información al LLM
    tags_to_remove = [
        "script", "style", "noscript", "iframe", "svg",  # Código y objetos
        "header", "footer", "nav", "aside",              # Estructura superflua
        "form", "button", "input",                       # Elementos interactivos
        "meta", "link",                                  # Metadatos
        "ad", "ins"                                      # Contenedores de anuncios comunes
    ]

    # Cirugía: Eliminar etiquetas basura
    for tag in soup(tags_to_remove):
        tag.decompose()

    # Truco extra: Eliminar divs que parezcan anuncios por su clase/id
    # (Búsqueda heurística simple)
    for tag in soup.find_all(attrs={"class": re.compile(r"(cookie|popup|ad-|ads|banner|promo|social)", re.I)}):
        tag.decompose()

    # 3. EXTRACCIÓN Y CONDENSACIÓN
    # Obtenemos el texto separando bloques con saltos de línea
    text = soup.get_text(separator='\n')

    # Limpieza final de espacios en blanco
    lines = (line.strip() for line in text.splitlines())
    
    # Reconstruimos el texto eliminando líneas vacías consecutivas
    clean_text = '\n'.join(line for line in lines if line)

    # Límite de seguridad para no saturar la ventana de contexto
    print(clean_text[:40000])

except Exception as e:
    # En caso de error crítico, devolvemos el error para que el LLM lo sepa
    print(f"Error procesando la web: {str(e)}")