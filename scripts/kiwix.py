import sys
import urllib.parse
import urllib.request
from bs4 import BeautifulSoup

# Ajusta el puerto si es necesario
KIWIX_HOST = "http://localhost:9090"
# Nombre de tu archivo ZIM (Ajusta si cambia la versión)
ZIM_FILE = "wikipedia_es_top_mini_2026-01"

def clean_html(html_content):
    """Limpia la 'zurraspa' del HTML para dejar solo texto útil."""
    soup = BeautifulSoup(html_content, 'html.parser')

    # 1. Eliminar elementos basura estándar
    for tag in soup(["script", "style", "meta", "link", "noscript", "iframe", "svg"]):
        tag.decompose()

    # 2. Eliminar elementos específicos de la interfaz de Kiwix
    # (Header superior, barra de búsqueda, pie de página, navegación)
    # Estas clases/ids dependen de la skin de Kiwix, estos son los comunes:
    for tag in soup.find_all(attrs={"id": ["header", "footer", "mw-navigation", "siteNotice"]}):
        tag.decompose()
    for tag in soup.find_all(attrs={"class": ["mw-editsection", "noprint", "mw-jump-link"]}):
        tag.decompose()

    # 3. Obtener texto y limpiar líneas vacías
    text = soup.get_text(separator='\n')
    lines = (line.strip() for line in text.splitlines())
    return '\n'.join(line for line in lines if line)

def search(query):
    """Busca y devuelve una lista limpia de Título -> Enlace."""
    params = urllib.parse.urlencode({'pattern': query, 'content': ZIM_FILE})
    url = f"{KIWIX_HOST}/search?{params}"
    
    try:
        with urllib.request.urlopen(url) as response:
            html = response.read()
            
        soup = BeautifulSoup(html, 'html.parser')
        results = []
        
        # Kiwix suele poner los resultados en una lista <ul> o <table id="mw-search-results">
        # Buscamos enlaces que apunten al contenido interno
        # La estructura típica de resultados de kiwix-serve es enlaces directos
        for a in soup.find_all('a', href=True):
            href = a['href']
            # Filtramos enlaces de sistema (admin, skins, etc)
            if "/content/" in href and "skin=" not in href:
                title = a.get_text(strip=True)
                results.append(f"- {title}: {href}")
        
        if not results:
            print("No se encontraron resultados. Intenta con otro término.")
        else:
            print(f"Resultados para '{query}':\n")
            # Limitamos a los primeros 15 para no saturar
            print("\n".join(results[:15]))
            print("\n\n(Usa la herramienta 'read_kiwix' copiando la ruta que empieza por /content/...)")

    except Exception as e:
        print(f"Error buscando: {e}")

def read(path):
    """Lee una URL relativa de Kiwix y devuelve el texto limpio."""
    # Aseguramos que la ruta empieza bien
    if not path.startswith("/"):
        path = "/" + path
        
    url = f"{KIWIX_HOST}{path}"
    
    try:
        with urllib.request.urlopen(url) as response:
            html = response.read()
        
        print(clean_html(html)[:40000]) # Limitamos caracteres
    except Exception as e:
        print(f"Error leyendo artículo: {e}")

if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Uso: python3 kiwix.py [search|read] [termino|url]")
        sys.exit(1)

    mode = sys.argv[1]
    arg = " ".join(sys.argv[2:])

    if mode == "search":
        search(arg)
    elif mode == "read":
        read(arg)
    else:
        print("Modo desconocido.")