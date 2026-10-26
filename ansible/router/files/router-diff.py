"""Render templates/apply.sh.j2's output as a unified diff.

Called by router.yml with that output, base64-encoded, as its only
argument. The router masks secrets before printing, so none reach here.
"""

import base64
import difflib
import sys

old, new, notes = {}, {}, []
current = None
for line in base64.b64decode(sys.argv[1]).decode().splitlines():
    if line.startswith("@@old "):
        current = old.setdefault(line[6:], [])
    elif line.startswith("@@new "):
        current = new.setdefault(line[6:], [])
    elif line.startswith("@@note "):
        notes.append(line[7:])
        current = None
    elif line.startswith("@@changed"):
        current = None
    elif current is not None:
        current.append(line.replace("\t", "    "))

for name in new:
    diff = list(
        difflib.unified_diff(old.get(name, []), new[name], f"{name} (router)", f"{name} (repo)", lineterm="")
    )
    print("\n".join(diff) if diff else f"{name}: only masked values differ")
for note in notes:
    print(note)
