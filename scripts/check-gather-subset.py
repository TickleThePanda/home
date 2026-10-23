#!/usr/bin/env python3
"""Fail if any Ansible play that gathers facts doesn't use the shared subset.

Every layer shares one fact cache under `gathering = smart`, so a play that
gathers less starves one that needs more. See ansible/ansible.cfg.

Usage: scripts/check-gather-subset.py [ansible-dir]
"""

import pathlib
import sys

import yaml

REQUIRED = {"!all", "min", "network"}


class Loader(yaml.SafeLoader):
    """SafeLoader that tolerates Ansible tags such as !vault."""


Loader.add_multi_constructor("!", lambda loader, suffix, node: None)


def plays(path):
    try:
        doc = yaml.load(path.read_text(), Loader=Loader)
    except yaml.YAMLError:
        return []
    if not isinstance(doc, list):
        return []
    return [p for p in doc if isinstance(p, dict) and "hosts" in p]


def main():
    root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else "ansible")
    bad = []
    for path in sorted(root.rglob("*.yml")):
        for play in plays(path):
            if play.get("gather_facts", True) in (False, "false", "no"):
                continue
            subset = play.get("gather_subset")
            if isinstance(subset, str):
                subset = [s.strip() for s in subset.split(",")]
            if set(subset or []) != REQUIRED:
                name = play.get("name") or play["hosts"]
                bad.append(f"{path}: play '{name}' has gather_subset {subset}")
    for line in bad:
        print(f"::error::{line}, expected {sorted(REQUIRED)} (see ansible/ansible.cfg)")
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
