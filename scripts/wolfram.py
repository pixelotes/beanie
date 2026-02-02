import sys
import os
import urllib.parse
import urllib.request

appid = os.environ.get('WOLFRAM_APPID')
if not appid:
    print("Error: Configura la variable de entorno WOLFRAM_APPID")
    sys.exit(1)

if len(sys.argv) < 2:
    print("Error: Falta la consulta")
    sys.exit(1)

query = " ".join(sys.argv[1:])
encoded = urllib.parse.quote(query)
# Usamos la API 'result' que devuelve texto plano directo
url = f"http://api.wolframalpha.com/v1/result?appid={appid}&i={encoded}"

try:
    with urllib.request.urlopen(url) as response:
        print(response.read().decode('utf-8'))
except Exception as e:
    print(f"Wolfram Error (puede que no tenga respuesta corta): {e}")