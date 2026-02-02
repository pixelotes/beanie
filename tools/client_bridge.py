import os
import json
import subprocess
from openai import OpenAI

# --- CONFIGURACIÓN ---
LM_STUDIO_URL = "http://localhost:1234/v1"
DOCKER_IMAGE = "mcp-local-test"
MODEL_NAME = "model-identifier" # LM Studio suele ignorar esto, pero pon algo genérico

# Definimos las herramientas manualmente para que LM Studio sepa qué puede hacer
# (Esto es lo que normalmente hace el protocolo MCP automático, aquí lo hacemos manual para testear)
TOOLS_SCHEMA = [
    {
        "type": "function",
        "function": {
            "name": "calculator",
            "description": "Calculadora útil para matemáticas y conversiones de unidades.",
            "parameters": {
                "type": "object",
                "properties": {
                    "expression": {"type": "string", "description": "La expresión a calcular (ej: '2+2', '5 miles to km')"}
                },
                "required": ["expression"]
            }
        }
    },
    {
        "type": "function",
        "function": {
            "name": "fortune_cookie",
            "description": "Devuelve un refrán o cita aleatoria.",
            "parameters": {"type": "object", "properties": {}}
        }
    }
]

client = OpenAI(base_url=LM_STUDIO_URL, api_key="lm-studio")

def run_docker_tool(tool_name, tool_args):
    """Llama al contenedor Docker usando el protocolo JSON-RPC de MCP"""
    
    # Construir petición JSON-RPC 2.0
    request = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/call",
        "params": {
            "name": tool_name,
            "arguments": tool_args
        }
    }
    
    request_str = json.dumps(request)
    
    # Ejecutar Docker pasándole el JSON por stdin
    # IMPORTANTE: --rm para borrarlo al acabar, -i para interactivo
    cmd = ["docker", "run", "-i", "--rm", DOCKER_IMAGE]
    
    try:
        process = subprocess.run(
            cmd,
            input=request_str.encode('utf-8'),
            capture_output=True
        )
        
        if process.returncode != 0:
            return f"Error Docker: {process.stderr.decode('utf-8')}"
            
        # Parsear respuesta del MCP
        response_json = json.loads(process.stdout.decode('utf-8'))
        
        if "error" in response_json:
            return f"Error MCP: {response_json['error']['message']}"
            
        # Extraer el texto del resultado
        content = response_json.get("result", {}).get("content", [])
        if content and len(content) > 0:
            return content[0].get("text", "Sin resultado")
        return "Respuesta vacía"

    except Exception as e:
        return f"Excepción ejecutando herramienta: {str(e)}"

def chat_loop():
    history = [
        {"role": "system", "content": "Eres un asistente útil que tiene acceso a una calculadora y galletas de la fortuna. Usa las herramientas cuando sea necesario."}
    ]

    print("\n--- Chat con LM Studio + Docker MCP (Escribe 'salir' para terminar) ---")
    
    while True:
        user_input = input("\nTú: ")
        if user_input.lower() in ["salir", "exit"]:
            break
            
        history.append({"role": "user", "content": user_input})

        # 1. Preguntar a LM Studio
        response = client.chat.completions.create(
            model=MODEL_NAME,
            messages=history,
            tools=TOOLS_SCHEMA,
            tool_choice="auto"
        )

        msg = response.choices[0].message
        
        # 2. ¿Quiere usar herramientas?
        if msg.tool_calls:
            print(f"(Pensamiento: El modelo quiere usar herramientas...)")
            history.append(msg) # Añadimos la intención al historial

            for tool_call in msg.tool_calls:
                fn_name = tool_call.function.name
                fn_args = json.loads(tool_call.function.arguments)
                
                print(f" > Ejecutando Docker: {fn_name} con {fn_args}...")
                
                # LLAMADA REAL AL DOCKER
                tool_result = run_docker_tool(fn_name, fn_args)
                
                print(f" < Resultado Docker: {tool_result.strip()}")

                # Añadimos el resultado al historial
                history.append({
                    "role": "tool",
                    "tool_call_id": tool_call.id,
                    "name": fn_name,
                    "content": tool_result
                })

            # 3. Segunda llamada a LM Studio para que interprete el resultado
            final_response = client.chat.completions.create(
                model=MODEL_NAME,
                messages=history
            )
            print(f"Asistente: {final_response.choices[0].message.content}")
            history.append(final_response.choices[0].message)
        
        else:
            # Respuesta normal sin herramientas
            print(f"Asistente: {msg.content}")
            history.append(msg)

if __name__ == "__main__":
    chat_loop()