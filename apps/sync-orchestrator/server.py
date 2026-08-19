#!/usr/bin/env python3
"""Web UI + scheduler for the librofm/libation sync-runner sidecars.

Talks to the runners over plain HTTP (via the audiobookshelf Service) --
no Kubernetes API access needed. Each runner owns its own subprocess and
concurrency lock; this component only decides *when* to call /run and
renders the aggregate status.
"""
import html
import json
import os
import secrets
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from string import Template
from zoneinfo import ZoneInfo

RUNNERS = {
    "librofm": "http://audiobookshelf.audiobookshelf.svc.cluster.local:8090",
    "libation": "http://audiobookshelf.audiobookshelf.svc.cluster.local:8091",
}
NAMES = {"librofm": "Libro.fm", "libation": "Audible (Libation)"}
INTERVAL_SECONDS = {
    "librofm": int(os.environ.get("LIBROFM_INTERVAL_SECONDS", 21600)),
    "libation": int(os.environ.get("LIBATION_INTERVAL_SECONDS", 3600)),
}
DISPLAY_TZ = ZoneInfo("Europe/London")

CSRF_TOKEN = secrets.token_hex(32)

state_lock = threading.Lock()
last_scheduled_run = {name: 0.0 for name in RUNNERS}


def call_runner(name, path, method="GET", timeout=5):
    url = RUNNERS[name] + path
    req = urllib.request.Request(url, method=method, data=b"" if method == "POST" else None)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()
    except Exception as e:
        return None, str(e).encode()


def get_status(name):
    status, body = call_runner(name, "/status", timeout=2)
    if status is None:
        return {"error": body.decode(errors="replace")}
    try:
        return json.loads(body)
    except Exception:
        return {"error": "bad response from runner"}


def trigger(name):
    # Reset the schedule on every attempt (manual or scheduled), regardless
    # of outcome -- avoids hot-retrying every tick if a run is stuck.
    with state_lock:
        last_scheduled_run[name] = time.time()
    return call_runner(name, "/run", method="POST", timeout=5)


def scheduler_loop(name):
    while True:
        time.sleep(60)
        with state_lock:
            due = time.time() - last_scheduled_run[name] >= INTERVAL_SECONDS[name]
        if due:
            trigger(name)


def parse_timestamp(value):
    if not value:
        return None
    try:
        return datetime.fromisoformat(value)
    except ValueError:
        return None


def relative_time(dt):
    seconds = max((datetime.now(timezone.utc) - dt).total_seconds(), 0)
    if seconds < 60:
        return "just now"
    minutes = int(seconds // 60)
    if minutes < 60:
        return f"{minutes} minute{'s' if minutes != 1 else ''} ago"
    hours = int(seconds // 3600)
    if hours < 24:
        return f"{hours} hour{'s' if hours != 1 else ''} ago"
    days = int(seconds // 86400)
    if days == 1:
        return "yesterday"
    if days < 7:
        return f"{days} days ago"
    return dt.astimezone(DISPLAY_TZ).strftime("%-d %B %Y")


def precise_time(dt):
    return dt.astimezone(DISPLAY_TZ).strftime("%-d %B %Y at %H:%M:%S %Z")


PAGE_TEMPLATE = Template("""<!doctype html>
<html>
<head>
<title>Audiobook sync</title>
$refresh
<style>
  body { font-family: sans-serif; max-width: 640px; margin: 2rem auto; padding: 0 1rem; color: #222; }
  section { border: 1px solid #ddd; border-radius: 8px; padding: 1rem 1.25rem; margin-bottom: 1rem; }
  .card-header { display: flex; justify-content: space-between; align-items: baseline; }
  .card-header h2 { margin: 0; font-size: 1.1rem; }
  .error-note { color: #888; font-size: 0.85rem; margin: 0.5rem 0 0; }
  .badge {
    display: inline-block; padding: 0.1rem 0.6rem; border-radius: 999px;
    background: #eee; color: #444; font-size: 0.85rem; white-space: nowrap;
  }
  .actions { margin-top: 1rem; text-align: right; }
  .actions form { margin-bottom: 0.5rem; }
  button.primary {
    background: #2563eb; color: #fff; border: none; border-radius: 6px;
    padding: 0.5rem 1rem; font-size: 0.95rem; cursor: pointer;
  }
  button.primary:hover:not(:disabled) { background: #1d4ed8; }
  button.primary:disabled { background: #a9bdec; cursor: default; }
  summary { cursor: pointer; color: #666; font-size: 0.9rem; }
  details h3 { font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.03em; color: #888; margin: 0.75rem 0 0.25rem; text-align: left; }
  pre { background: #f4f4f4; padding: 0.5rem; overflow-x: auto; max-height: 16rem; white-space: pre-wrap; margin: 0; text-align: left; }
</style>
</head>
<body>
<h1>Audiobook sync</h1>
$sections
</body>
</html>""")

SECTION_TEMPLATE = Template("""
<section>
  <div class="card-header">
    <h2>$title</h2>
    <span class="badge" title="$badge_title">$badge</span>
  </div>
  $error_note

  <div class="actions">
    <form method="post" action="/trigger/$name">
      <input type="hidden" name="csrf" value="$csrf">
      <button type="submit" class="primary"$disabled>Sync now</button>
    </form>
    <details>
      <summary>View full output</summary>
      <h3>Output</h3>
      <pre>$tail</pre>
    </details>
  </div>
</section>
""")


def render_section(name):
    """Build the escaped substitution values for one sync's <section>, then
    hand them to SECTION_TEMPLATE -- string.Template only does substitution,
    so escaping every value up front here is what keeps the markup itself
    free of ad hoc html.escape() calls."""
    status = get_status(name)
    running = bool(status.get("running"))
    last = status.get("last")
    finished = parse_timestamp(last.get("finished")) if last else None
    error_note = (
        f'<p class="error-note">{html.escape(last["error"])}</p>'
        if last and last.get("error") else ""
    )

    # One badge, not two: while a job is actively running that's the only
    # thing worth showing. Otherwise the badge *is* the last job's status --
    # there's no separate "idle" state distinct from "what did it last do".
    if "error" in status:
        badge, badge_title = f"unknown ({status['error']})", ""
    elif running:
        badge, badge_title = "running", ""
    elif finished:
        rel = relative_time(finished)
        exit_code = last.get("exit_code")
        if exit_code == 0:
            badge = f"ok ({rel})"
        else:
            badge = f"failed (exit {exit_code}, {rel})"
        badge_title = f"Finished {precise_time(finished)}"
    else:
        badge, badge_title = "never", ""

    return running, SECTION_TEMPLATE.substitute(
        title=html.escape(NAMES[name]),
        badge=html.escape(badge),
        badge_title=html.escape(badge_title),
        error_note=error_note,
        name=name,
        csrf=CSRF_TOKEN,
        disabled=" disabled" if running else "",
        # Deliberately the same output_tail already returned by /status --
        # the most recent output, not the runner's full multi-run /logs
        # buffer. Command output is shown exactly as produced, unmodified.
        tail=html.escape(status.get("output_tail", "")),
    )


def render_page():
    running_flags, sections = zip(*(render_section(name) for name in RUNNERS))
    refresh = '<meta http-equiv="refresh" content="5">' if any(running_flags) else ""
    return PAGE_TEMPLATE.substitute(refresh=refresh, sections="".join(sections))


class Handler(BaseHTTPRequestHandler):
    def _send(self, status, content_type, body):
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/healthz":
            self.send_response(200)
            self.end_headers()
            return

        if self.path == "/":
            self._send(200, "text/html; charset=utf-8", render_page().encode())
            return

        self._send(404, "text/plain", b"not found")

    def do_POST(self):
        if self.path.startswith("/trigger/"):
            name = self.path[len("/trigger/"):]
            if name not in RUNNERS:
                self._send(404, "text/plain", b"unknown sync")
                return

            length = int(self.headers.get("Content-Length", 0) or 0)
            body = self.rfile.read(length) if length else b""
            form = urllib.parse.parse_qs(body.decode())
            token = form.get("csrf", [""])[0]
            if not secrets.compare_digest(token, CSRF_TOKEN):
                self._send(403, "text/plain", b"invalid csrf token")
                return

            trigger(name)
            self.send_response(303)
            self.send_header("Location", "/")
            self.end_headers()
            return

        self._send(404, "text/plain", b"not found")

    def log_message(self, fmt, *args):
        pass


def main():
    for name in RUNNERS:
        threading.Thread(target=scheduler_loop, args=(name,), daemon=True).start()
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()


if __name__ == "__main__":
    main()
