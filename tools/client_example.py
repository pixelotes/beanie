#!/usr/bin/env python3
"""
Ejemplo simple de cliente MCP para probar el servidor.
Requiere: pip install mcp

Uso:
  python3 client_example.py
"""

import sys
import json
import subprocess

def test_mcp_server():
    """Prueba básica de conexión al servidor MCP"""
    
    # Iniciar el servidor (asume que docker compose está corriendo)
    print("🔌 Iniciando servidor MCP...")
    process = subprocess.Popen(
        ["docker", "compose", "exec", "mcp-server", "/app/mcp-server"],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True
    )
    
    try:
        # Enviar solicitud de inicialización
        init_request = {
            "jsonrpc": "2.0",
            "id": 1,
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {
                    "name": "test-client",
                    "version": "1.0.0"
                }
            }
        }
        
        print("📤 Enviando:", json.dumps(init_request, indent=2))
        process.stdin.write(json.dumps(init_request) + "\n")
        process.stdin.flush()
        
        # Leer respuesta
        response = process.stdout.readline()
        if response:
            print("📥 Respuesta:", json.dumps(json.loads(response), indent=2))
        
        # Listar herramientas
        tools_request = {
            "jsonrpc": "2.0",
            "id": 2,
            "method": "tools/list",
            "params": {}
        }
        
        print("\n📤 Pidiendo lista de herramientas...")
        process.stdin.write(json.dumps(tools_request) + "\n")
        process.stdin.flush()
        
        response = process.stdout.readline()
        if response:
            data = json.loads(response)
            print("📥 Herramientas disponibles:")
            if "result" in data and "tools" in data["result"]:
                for tool in data["result"]["tools"]:
                    print(f"  - {tool['name']}: {tool['description']}")
        
    finally:
        process.terminate()
        process.wait()
        print("\n✅ Test completado")

if __name__ == "__main__":
    try:
        test_mcp_server()
    except Exception as e:
        print(f"❌ Error: {e}", file=sys.stderr)
        sys.exit(1)
