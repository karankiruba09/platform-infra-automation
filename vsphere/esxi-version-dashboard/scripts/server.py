#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
import subprocess
import threading
import time
from collections import deque
from datetime import datetime
from html import escape
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, urlparse

ROOT_DIR = Path(__file__).resolve().parents[1]
PUBLIC_DIR = ROOT_DIR / "public"
COLLECTOR = ROOT_DIR / "scripts" / "collect.sh"

REPORT_PATH = PUBLIC_DIR / "esxi_versions.json"
HOSTS_CSV_PATH = PUBLIC_DIR / "esxi_hosts.csv"
STYLE_PATH = PUBLIC_DIR / "style.css"

_refresh_lock = threading.Lock()
_activity = deque(maxlen=10)


def add_activity(msg: str) -> None:
    ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    _activity.appendleft((msg, ts))


def clamp_pct(p: float) -> float:
    try:
        x = float(p)
    except Exception:
        return 0.0
    if x < 0:
        return 0.0
    if x > 100:
        return 100.0
    return x


def fmt_pct(p: float) -> str:
    return f"{clamp_pct(p):.1f}%"


def fmt_duration(ms: Any) -> str:
    try:
        x = int(ms or 0)
    except Exception:
        x = 0
    if x >= 60_000:
        return f"{x / 60_000:.1f}m"
    if x >= 1_000:
        return f"{x / 1_000:.1f}s"
    return f"{x}ms"


def first_line(s: Any) -> str:
    if not s:
        return ""
    return str(s).splitlines()[0]


def health_from_coverage_pct(p: float) -> dict[str, str]:
    if p >= 99.9:
        return {"label": "Healthy", "cls": "good"}
    if p >= 85:
        return {"label": "In progress", "cls": "warn"}
    return {"label": "At risk", "cls": "bad"}


def summarize_builds_8x(builds: Any) -> str:
    if not isinstance(builds, list) or not builds:
        return ""
    top = []
    for b in builds[:3]:
        version = str((b or {}).get("version") or "8.x")
        build = str((b or {}).get("build") or "unknown")
        top.append(f"{version} build ?" if build == "unknown" else f"{version} build {build}")
    suffix = ", ..." if len(builds) > 3 else ""
    return f"8.x builds: {', '.join(top)}{suffix}"


def load_report() -> dict[str, Any] | None:
    try:
        with REPORT_PATH.open("r", encoding="utf-8") as f:
            return json.load(f)
    except FileNotFoundError:
        return None


def render_dashboard(
    data: dict[str, Any] | None,
    *,
    filter_q: str,
    errors_only: bool,
    url_prefix: str,
    include_refresh_controls: bool = True,
    include_filter_controls: bool = True,
    flash: str | None = None,
    error: str | None = None,
) -> str:
    base = url_prefix or ""
    if base and not base.endswith("/"):
        base = base + "/"

    def href(p: str) -> str:
        return f"{base}{p.lstrip('/')}"

    generated_at = "-" if not data else (data.get("generated_at") or "-")

    totals = {} if not data else (data.get("totals") or {})
    v_ok = int(totals.get("vcenters_ok") or 0)
    v_total = int(totals.get("vcenters_total") or 0)
    v_err = int(totals.get("vcenters_error") or 0)

    partial = "All sources healthy" if v_err <= 0 else f"Partial data: {v_err} failing"

    total_hosts = int(totals.get("hosts_total") or 0)
    major = totals.get("major_counts") or {}
    v8 = int(major.get("8.x") or 0)
    v7 = int(major.get("7.x") or 0)
    v6 = int(major.get("6.x") or 0)
    other = int(major.get("other") or 0)
    unknown = int(major.get("unknown") or 0)
    unknown_combined = unknown + other

    v8_pct = (v8 / total_hosts * 100.0) if total_hosts else 0.0
    v7_pct = (v7 / total_hosts * 100.0) if total_hosts else 0.0
    v6_pct = (v6 / total_hosts * 100.0) if total_hosts else 0.0
    unk_pct = (unknown_combined / total_hosts * 100.0) if total_hosts else 0.0

    health = health_from_coverage_pct(v8_pct) if total_hosts else {"label": "No data", "cls": "warn"}
    if not total_hosts:
        health_pill = (
            "No data yet. Click Refresh to collect."
            if include_refresh_controls
            else "No data yet. Run ./scripts/collect.sh to collect."
        )
    else:
        health_pill = f"{health['label']}: 8.x coverage {fmt_pct(v8_pct)}"

    builds = [] if not data else (data.get("build_breakdown_8x") or [])
    if not isinstance(builds, list):
        builds = []
    builds = builds[:10]
    max_build_hosts = 0
    for b in builds:
        try:
            max_build_hosts = max(max_build_hosts, int((b or {}).get("hosts") or 0))
        except Exception:
            pass

    rows = [] if not data else (data.get("rows") or [])
    if not isinstance(rows, list):
        rows = []

    fq = (filter_q or "").strip().lower()
    shown_rows = []
    for r in rows:
        if not isinstance(r, dict):
            continue
        vc = str(r.get("vcenter") or "-")
        status = str(r.get("status") or "unknown")
        if fq and fq not in vc.lower():
            continue
        if errors_only and status == "ok":
            continue
        shown_rows.append(r)

    activity_html = []
    for msg, ts in list(_activity):
        activity_html.append(
            f'<div class="log-item"><div class="log-title">{escape(msg)}</div>'
            f'<div class="log-meta">{escape(ts)}</div></div>'
        )
    if not activity_html:
        activity_html.append('<div class="hint">No activity yet.</div>')

    # Flash/error banner
    banner_html = ""
    if flash:
        banner_html = f'<div class="card span-12"><div class="hint">{escape(flash)}</div></div>'
    if error:
        banner_html = (
            f'<div class="card span-12"><div class="hint" style="color: rgba(239,68,68,.92)">'
            f"{escape(error)}</div></div>"
        )

    # Build rows HTML
    builds_html = []
    if not builds:
        builds_html.append('<div class="hint">No 8.x hosts observed yet.</div>')
    else:
        for b in builds:
            version = str((b or {}).get("version") or "8.x")
            build = str((b or {}).get("build") or "unknown")
            label = f"{version} build unknown" if build == "unknown" else f"{version} build {build}"
            try:
                hosts = int((b or {}).get("hosts") or 0)
            except Exception:
                hosts = 0
            pct = (hosts / max_build_hosts * 100.0) if max_build_hosts else 0.0
            builds_html.append(
                '<div class="ver-row">'
                f'<div class="ver-label" title="{escape(label, quote=True)}">{escape(label)}</div>'
                f'<div class="ver-count">{hosts}</div>'
                '<div class="ver-bar-wrap"><div class="ver-bar" '
                f'style="width:{clamp_pct(pct):.1f}%"></div></div>'
                "</div>"
            )

    # Table body HTML
    tbody_html = []
    if not shown_rows:
        tbody_html.append(
            '<tr><td colspan="7"><div class="hint">No rows to show.</div></td></tr>'
        )
    else:
        for r in shown_rows:
            vc = str(r.get("vcenter") or "-")
            status = str(r.get("status") or "unknown")
            err = r.get("error")
            duration_ms = r.get("duration_ms") or 0

            total = int(r.get("total_hosts") or 0)
            connected = int(r.get("connected_hosts") or 0)
            maintenance = int(r.get("maintenance_hosts") or 0)
            m = r.get("major_counts") or {}
            v8r = int((m.get("8.x") if isinstance(m, dict) else 0) or 0)
            v7r = int((m.get("7.x") if isinstance(m, dict) else 0) or 0)
            v6r = int((m.get("6.x") if isinstance(m, dict) else 0) or 0)
            othr = int((m.get("other") if isinstance(m, dict) else 0) or 0)
            unkr = int((m.get("unknown") if isinstance(m, dict) else 0) or 0)
            unk_comb = othr + unkr
            v8p = (v8r / total * 100.0) if total else 0.0

            is_ok = status == "ok"
            if is_ok:
                tag = health_from_coverage_pct(v8p)
                builds_summary = ""
                if v8r > 0:
                    builds_summary = summarize_builds_8x(r.get("build_breakdown_8x") or [])
                meta = (
                    f"connected {connected}/{total} | maint {maintenance} | {fmt_duration(duration_ms)}"
                    + (f" | {builds_summary}" if builds_summary else "")
                )
                meta_cls = "vc-sub"
                title = ""
            else:
                tag = {"label": "Error", "cls": "bad"}
                meta = f"ERROR | {fmt_duration(duration_ms)} | {first_line(err)}"
                meta_cls = "vc-sub err"
                title = str(err or "")

            tbody_html.append(
                "<tr>"
                f'<td title="{escape(title, quote=True)}">'
                '<div class="vc-name">'
                f'<span class="tag"><span class="bullet {escape(tag["cls"])}"></span>{escape(vc)}</span>'
                "</div>"
                f'<div class="{meta_cls}">{escape(meta)}</div>'
                "</td>"
                f'<td class="num">{total if is_ok else "&#8212;"}</td>'
                f'<td class="num">{v8r if is_ok else "&#8212;"}</td>'
                f'<td class="num">{v7r if is_ok else "&#8212;"}</td>'
                f'<td class="num">{v6r if is_ok else "&#8212;"}</td>'
                f'<td class="num">{unk_comb if is_ok else "&#8212;"}</td>'
                "<td>"
                '<div class="progress-cell">'
                '<div class="progress-wrap"><div class="progress-bar" '
                f'style="width:{clamp_pct(v8p):.1f}%"></div></div>'
                f'<div class="progress-label">{fmt_pct(v8p) if is_ok else "&#8212;"}</div>'
                "</div>"
                "</td>"
                "</tr>"
            )

    # Table filter form
    filter_val = filter_q or ""
    checked = "checked" if errors_only else ""

    refresh_controls_html = ""
    if include_refresh_controls:
        refresh_controls_html = f"""
      <form method="post" action="{escape(href("refresh"), quote=True)}" style="margin:0">
        <button class="btn primary" type="submit">Refresh</button>
      </form>
"""

    if include_filter_controls:
        table_actions_html = f"""
      <form class="table-actions" method="get" action="{escape(href(""), quote=True)}" style="margin:0">
        <input class="filter" type="text" name="filter" value="{escape(filter_val, quote=True)}"
               placeholder="Filter vCenters (name contains)..." />
        <label class="check">
          <input type="checkbox" name="errors_only" value="1" {checked} />
          <span>Errors only</span>
        </label>
        <button class="btn" type="submit">Apply</button>
        <a class="btn" href="{escape(href(""), quote=True)}">Clear</a>
      </form>
"""
    else:
        table_actions_html = '<div class="hint">Tip: use your browser Find (Ctrl+F) to search vCenter names.</div>'

    html = f"""<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <title>ESXi Versions Dashboard</title>
  <link rel="stylesheet" href="{escape(href("style.css"), quote=True)}" />
</head>
<body>
  <div class="bg"></div>
  <div class="grain"></div>

  <header class="header">
    <div class="brand">
      <div class="title">ESXi Fleet</div>
      <div class="subtitle">Version distribution across vCenters</div>
      <div class="meta">
        <span class="meta-item">Updated <span>{escape(str(generated_at))}</span></span>
        <span class="sep">|</span>
        <span class="meta-item">vCenters <span>{v_ok}</span>/<span>{v_total}</span> OK</span>
        <span class="sep">|</span>
        <span class="meta-item">{escape(partial)}</span>
      </div>
    </div>

    <div class="header-actions">
      {refresh_controls_html}
      <a class="btn" href="{escape(href("esxi_hosts.csv"), quote=True)}" target="_blank" rel="noopener">Download CSV</a>
      <a class="btn" href="{escape(href("esxi_versions.json"), quote=True)}" target="_blank" rel="noopener">Download JSON</a>
      <div class="pill" id="healthPill">{escape(health_pill)}</div>
    </div>
  </header>

  <main class="grid">
    {banner_html}

    <section class="card kpi">
      <div class="kpi-label">vCenters OK</div>
      <div class="kpi-value">{v_ok}/{v_total}</div>
      <div class="kpi-foot">Healthy sources</div>
    </section>

    <section class="card kpi">
      <div class="kpi-label">Total hosts</div>
      <div class="kpi-value">{total_hosts}</div>
      <div class="kpi-foot">Observed hosts</div>
    </section>

    <section class="card kpi">
      <div class="kpi-label">ESXi 8.x</div>
      <div class="kpi-value">{v8}</div>
      <div class="kpi-foot">{escape(fmt_pct(v8_pct))} of observed hosts</div>
    </section>

    <section class="card kpi">
      <div class="kpi-label">ESXi 7.x</div>
      <div class="kpi-value">{v7}</div>
      <div class="kpi-foot">{escape(fmt_pct(v7_pct))} of observed hosts</div>
    </section>

    <section class="card kpi">
      <div class="kpi-label">ESXi 6.x</div>
      <div class="kpi-value">{v6}</div>
      <div class="kpi-foot">{escape(fmt_pct(v6_pct))} of observed hosts</div>
    </section>

    <section class="card kpi">
      <div class="kpi-label">Unknown / Other</div>
      <div class="kpi-value">{unknown_combined}</div>
      <div class="kpi-foot">{escape(fmt_pct(unk_pct))} of observed hosts</div>
    </section>

    <section class="card span-6">
      <div class="card-title">Fleet distribution</div>
      <div class="stacked" aria-label="Fleet distribution stacked bar">
        <div class="seg v8" style="width:{clamp_pct(v8_pct):.1f}%"></div>
        <div class="seg v7" style="width:{clamp_pct(v7_pct):.1f}%"></div>
        <div class="seg v6" style="width:{clamp_pct(v6_pct):.1f}%"></div>
        <div class="seg unknown" style="width:{clamp_pct(unk_pct):.1f}%"></div>
      </div>
      <div class="legend">
        <div class="legend-item"><span class="swatch v8"></span>8.x</div>
        <div class="legend-item"><span class="swatch v7"></span>7.x</div>
        <div class="legend-item"><span class="swatch v6"></span>6.x</div>
        <div class="legend-item"><span class="swatch unknown"></span>Unknown/Other</div>
      </div>
      <div class="hint">This view is based on observed hosts (vCenters with collection errors are excluded).</div>
    </section>

    <section class="card span-6">
      <div class="card-title">8.x build numbers</div>
      <div class="versions">
        {''.join(builds_html)}
      </div>
      <div class="hint">Top 8.x version/build combinations across the fleet (top 10).</div>
    </section>

    <section class="card table-card">
      <div class="card-title">vCenter breakdown</div>

      {table_actions_html}

      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>vCenter</th>
              <th class="num">Total</th>
              <th class="num">8.x</th>
              <th class="num">7.x</th>
              <th class="num">6.x</th>
              <th class="num">Unknown/Other</th>
              <th>8.x coverage</th>
            </tr>
          </thead>
          <tbody>
            {''.join(tbody_html)}
          </tbody>
        </table>
      </div>

      <div class="hint">Click Refresh to re-collect live. Errors show the first part of the failure message.</div>
    </section>

    <section class="card log-card">
      <div class="card-title">Activity</div>
      <div class="log">
        {''.join(activity_html)}
      </div>
    </section>
  </main>
</body>
</html>
"""
    return html


class Handler(BaseHTTPRequestHandler):
    server_version = "esxi-version-dashboard/py"

    def _send(self, status: int, body: bytes, *, content_type: str) -> None:
        self.send_response(status)
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _send_text(self, status: int, text: str, *, content_type: str = "text/plain; charset=utf-8") -> None:
        self._send(status, text.encode("utf-8"), content_type=content_type)

    def _send_html(self, status: int, html: str) -> None:
        self._send(status, html.encode("utf-8"), content_type="text/html; charset=utf-8")

    def _send_file(self, p: Path, *, content_type: str) -> None:
        try:
            body = p.read_bytes()
        except FileNotFoundError:
            self._send_text(HTTPStatus.NOT_FOUND, "Not found")
            return
        self._send(HTTPStatus.OK, body, content_type=content_type)

    def _redirect(self, to: str) -> None:
        self.send_response(HTTPStatus.SEE_OTHER)
        self.send_header("Cache-Control", "no-store")
        self.send_header("Location", to)
        self.end_headers()

    def do_GET(self) -> None:  # noqa: N802
        url = urlparse(self.path)
        path = url.path

        if path in ("/", "/index.html"):
            qs = parse_qs(url.query or "")
            filter_q = (qs.get("filter", [""]) or [""])[0]
            errors_only = (qs.get("errors_only", ["0"]) or ["0"])[0] in ("1", "true", "yes", "on")
            data = load_report()
            if data is None:
                add_activity("No report found yet. Click Refresh to collect.")
            html = render_dashboard(data, filter_q=filter_q, errors_only=errors_only, url_prefix="/")
            self._send_html(HTTPStatus.OK, html)
            return

        if path == "/refresh":
            self._handle_refresh()
            return

        if path == "/style.css":
            self._send_file(STYLE_PATH, content_type="text/css; charset=utf-8")
            return

        if path == "/esxi_hosts.csv":
            self._send_file(HOSTS_CSV_PATH, content_type="text/csv; charset=utf-8")
            return

        if path == "/esxi_versions.json":
            self._send_file(REPORT_PATH, content_type="application/json; charset=utf-8")
            return

        self._send_text(HTTPStatus.NOT_FOUND, "Not found")

    def do_POST(self) -> None:  # noqa: N802
        url = urlparse(self.path)
        if url.path == "/refresh":
            self._handle_refresh()
            return
        self._send_text(HTTPStatus.NOT_FOUND, "Not found")

    def _handle_refresh(self) -> None:
        if not _refresh_lock.acquire(blocking=False):
            data = load_report()
            html = render_dashboard(
                data,
                filter_q="",
                errors_only=False,
                url_prefix="/",
                error="Refresh already in progress. Reload this page in a few seconds.",
            )
            self._send_html(HTTPStatus.CONFLICT, html)
            return

        try:
            add_activity("Refresh started")
            started = time.time()
            proc = subprocess.run(
                ["bash", str(COLLECTOR)],
                cwd=str(ROOT_DIR),
                env=os.environ.copy(),
                capture_output=True,
                text=True,
            )
            dur_ms = int((time.time() - started) * 1000)
            if proc.returncode != 0:
                msg = (proc.stderr or proc.stdout or "").strip() or f"collector exited with code {proc.returncode}"
                add_activity(f"Refresh failed: {first_line(msg)}")
                data = load_report()
                html = render_dashboard(
                    data,
                    filter_q="",
                    errors_only=False,
                    url_prefix="/",
                    error=f"Refresh failed ({fmt_duration(dur_ms)}): {first_line(msg)}",
                )
                self._send_html(HTTPStatus.INTERNAL_SERVER_ERROR, html)
                return

            add_activity(f"Refresh OK ({fmt_duration(dur_ms)})")
            self._redirect("/")
        finally:
            _refresh_lock.release()


def main() -> int:
    parser = argparse.ArgumentParser(description="ESXi Version Dashboard (server-rendered, no JavaScript)")
    parser.add_argument("--host", default=os.environ.get("HOST", "127.0.0.1"))
    parser.add_argument("--port", type=int, default=int(os.environ.get("PORT", "8081")))
    parser.add_argument("--render-html", metavar="PATH", help="Render a static HTML report to PATH and exit.")
    args = parser.parse_args()

    if args.render_html:
        data = load_report()
        html = render_dashboard(
            data,
            filter_q="",
            errors_only=False,
            url_prefix="",
            include_refresh_controls=False,
            include_filter_controls=False,
            flash="To refresh this report, run ./scripts/collect.sh",
        )
        out = Path(args.render_html)
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(html, encoding="utf-8")
        return 0

    host = str(args.host)
    port = int(args.port)
    add_activity(f"Server started on http://{host}:{port}")
    httpd = ThreadingHTTPServer((host, port), Handler)
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        return 0
    finally:
        httpd.server_close()


if __name__ == "__main__":
    raise SystemExit(main())
