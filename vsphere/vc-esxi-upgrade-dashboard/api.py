import json
from flask import Flask, jsonify, send_from_directory

app = Flask(__name__)


@app.get("/api/v1/vcenters")
def vcenters():
    """Serve the collector JSON as an API endpoint."""
    with open("public/vcenters.json", "r") as f:
        return jsonify(json.load(f))


@app.get("/")
def index():
    """Serve the dashboard HTML."""
    return send_from_directory("public", "index.html")


@app.get("/<path:path>")
def static_files(path):
    """
    Serve static assets (CSS, JS, etc.) securely using send_from_directory [web:41].
    This prevents directory traversal attacks and other path-based vulnerabilities.
    """
    return send_from_directory("public", path)


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080, debug=False)