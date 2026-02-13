import json
import os
from collections import deque
from datetime import datetime
from pathlib import Path
from typing import Any

from flask import Flask, jsonify, render_template, send_from_directory

app = Flask(__name__)

PUBLIC_DIR = Path(__file__).resolve().parent / "public"
REPORT_PATH = PUBLIC_DIR / "vcenters.json"

_activity = deque(maxlen=8)
_last_generated_at: str | None = None


def add_activity(msg: str) -> None:
    ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    _activity.appendleft({"msg": msg, "ts": ts})


def clamp_pct(p: Any) -> float:
    try:
        x = float(p)
    except Exception:
        return 0.0
    if x < 0.0:
        return 0.0
    if x > 100.0:
        return 100.0
    return x


def fmt_pct(p: Any) -> str:
    return f"{clamp_pct(p):.1f}%"


def health_from_pct(p: float) -> dict[str, str]:
    if p >= 99.9:
        return {"label": "Healthy", "cls": "good"}
    if p >= 85.0:
        return {"label": "In progress", "cls": "warn"}
    return {"label": "At risk", "cls": "bad"}


def load_report() -> dict[str, Any] | None:
    try:
        with REPORT_PATH.open("r", encoding="utf-8") as f:
            return json.load(f)
    except FileNotFoundError:
        return None


@app.get("/api/v1/vcenters")
def vcenters():
    """Serve the collector JSON as an API endpoint."""
    data = load_report()
    if data is None:
        return jsonify({"error": "Report not found. Run python3 collector.py first."}), 404
    return jsonify(data)


@app.get("/")
def index():
    """Server-rendered dashboard (no JavaScript)."""
    global _last_generated_at

    refresh_seconds = int(os.environ.get("REFRESH_SECONDS", "60"))
    banner = None

    data = load_report()
    if data is None:
        banner = "No report found yet. Run python3 collector.py to generate public/vcenters.json."
        rows: list[dict[str, Any]] = []
        target = "-"
        generated_at = "-"
    else:
        rows = data.get("rows") or []
        if not isinstance(rows, list):
            rows = []
        target = str(data.get("target_esxi_version") or "-")
        generated_at = str(data.get("generated_at") or "-")

    total_vcs = len(rows)
    total_hosts = sum(int((r or {}).get("total_hosts") or 0) for r in rows if isinstance(r, dict))
    total_upgraded = sum(int((r or {}).get("upgrade_completed_total") or 0) for r in rows if isinstance(r, dict))
    weighted_pct = (total_upgraded / total_hosts * 100.0) if total_hosts else 0.0

    overall_health = health_from_pct(weighted_pct) if total_hosts else {"label": "No data", "cls": "warn"}
    health_pill = (
        "No data"
        if not total_hosts
        else f"{overall_health['label']}: {fmt_pct(weighted_pct)} complete"
    )

    rows_view = []
    for r in rows:
        if not isinstance(r, dict):
            continue
        vc = str(r.get("vcenter") or "-")
        total = int(r.get("total_hosts") or 0)
        upgraded = int(r.get("upgrade_completed_total") or 0)
        if "completion_percentage" in r:
            pct = clamp_pct(r.get("completion_percentage"))
        else:
            pct = (upgraded / total * 100.0) if total else 0.0
        tag = health_from_pct(pct) if total else {"label": "No data", "cls": "warn"}
        rows_view.append(
            {
                "vcenter": vc,
                "total_hosts": total,
                "upgrade_completed_total": upgraded,
                "pct_width": f"{clamp_pct(pct):.1f}",
                "pct_str": fmt_pct(pct),
                "health_cls": tag["cls"],
            }
        )

    # Only log when the report changes to avoid noise from meta refresh.
    if data is None:
        if _last_generated_at is not None:
            add_activity("Report missing")
            _last_generated_at = None
    else:
        if generated_at != _last_generated_at:
            add_activity(f"Loaded report: {total_vcs} vCenters, overall {fmt_pct(weighted_pct)}")
            _last_generated_at = generated_at

    return render_template(
        "index.html",
        refresh_seconds=refresh_seconds,
        banner=banner,
        target_esxi_version=target,
        generated_at=generated_at,
        kpi_vcenters=total_vcs,
        kpi_hosts=total_hosts,
        kpi_upgraded=total_upgraded,
        kpi_completion=fmt_pct(weighted_pct),
        health_pill=health_pill,
        rows=rows_view,
        activity=list(_activity),
    )


@app.get("/<path:path>")
def static_files(path):
    """
    Serve static assets (CSS, JSON, etc.) securely using send_from_directory.
    """
    return send_from_directory(str(PUBLIC_DIR), path)


if __name__ == "__main__":
    host = os.environ.get("HOST", "0.0.0.0")
    port = int(os.environ.get("PORT", "8080"))
    app.run(host=host, port=port, debug=False)
