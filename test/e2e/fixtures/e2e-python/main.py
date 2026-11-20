# Proves the python driver: virtualenv creation, privilege drop and routing.
import http.server, os, sys

class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = f"E2E-PYTHON-OK venv={os.environ.get('VIRTUAL_ENV')} uid={os.getuid()}\n".encode()
        self.send_response(200)
        self.end_headers()
        self.wfile.write(body)

http.server.HTTPServer(("0.0.0.0", int(os.environ["PORT"])), Handler).serve_forever()
