#!/usr/bin/env python3
"""Render router/templates/ into router/build/files/ with Jinja2.

Every file under templates/ is a Jinja template mirroring its path in the
image rootfs; a file with no `{{ }}` passes through unchanged. The context
is the ROUTER_* secrets plus ROOT_HASH, taken from the environment
(build.sh exports them). `shquote` wraps a value as a safe POSIX
single-quoted shell literal, so any value embeds correctly.
"""
import os
import pathlib
import sys

try:
    from jinja2 import Environment, FileSystemLoader, StrictUndefined
except ImportError:
    sys.exit("jinja2 is required (it ships with ansible, or: pip install jinja2)")

VARS = (
    "ROUTER_PPPOE_USERNAME",
    "ROUTER_PPPOE_PASSWORD",
    "ROUTER_WIFI_KEY",
    "ROUTER_TS_AUTHKEY",
    "ROOT_HASH",
)


def shquote(value):
    return "'" + str(value).replace("'", "'\\''") + "'"


root = pathlib.Path(__file__).resolve().parent
src = root / "templates"
dst = root / "build" / "files"

env = Environment(
    loader=FileSystemLoader(str(src)),
    undefined=StrictUndefined,
    keep_trailing_newline=True,
    autoescape=False,
)
env.filters["shquote"] = shquote

try:
    ctx = {name: os.environ[name] for name in VARS}
except KeyError as missing:
    sys.exit(f"missing environment variable: {missing}")

for path in sorted(src.rglob("*")):
    if not path.is_file():
        continue
    rel = path.relative_to(src)
    out = dst / rel
    out.parent.mkdir(parents=True, exist_ok=True)
    text = env.get_template(str(rel)).render(**ctx)
    out.write_text(text)
    out.chmod(0o755 if text.startswith("#!") else 0o644)
