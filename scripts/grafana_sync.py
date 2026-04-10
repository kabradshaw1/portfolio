#!/usr/bin/env python3
"""Sync Grafana dashboard JSON files into the K8s ConfigMap.

The dashboard source of truth lives at
``monitoring/grafana/dashboards/*.json``. Each JSON file becomes a key
in ``k8s/monitoring/configmaps/grafana-dashboards.yml`` with 4-space
indentation.

CI fails if these drift. This script regenerates the ConfigMap from the
source JSON files, or with ``--check`` verifies they're in sync without
writing.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

DASHBOARD_DIR = Path("monitoring/grafana/dashboards")
CONFIGMAP = Path("k8s/monitoring/configmaps/grafana-dashboards.yml")

CM_HEADER = (
    "apiVersion: v1\n"
    "kind: ConfigMap\n"
    "metadata:\n"
    "  name: grafana-dashboards\n"
    "  namespace: monitoring\n"
    "data:\n"
)


def regenerate() -> None:
    jsons = sorted(DASHBOARD_DIR.glob("*.json"))
    if not jsons:
        print("no dashboard JSON files found", file=sys.stderr)
        sys.exit(1)

    parts = [CM_HEADER]
    for path in jsons:
        src = path.read_text()
        json.loads(src)  # validate
        indented = "\n".join(
            ("    " + line) if line else line for line in src.splitlines()
        )
        parts.append(f"  {path.name}: |\n{indented}\n")

    CONFIGMAP.write_text("".join(parts))
    print(f"regenerated ({len(jsons)} dashboards)")


def check() -> int:
    import yaml  # only needed for the check path

    jsons = sorted(DASHBOARD_DIR.glob("*.json"))
    if not jsons:
        print("no dashboard JSON files found", file=sys.stderr)
        return 1

    cm = yaml.safe_load(CONFIGMAP.read_text())
    data = cm.get("data", {})

    for path in jsons:
        src = json.loads(path.read_text())
        embedded_str = data.get(path.name)
        if embedded_str is None:
            print(
                f"missing {path.name} in ConfigMap — run: make grafana-sync",
                file=sys.stderr,
            )
            return 1
        embedded = json.loads(embedded_str)
        if src != embedded:
            print(
                f"{path.name} drifted — run: make grafana-sync",
                file=sys.stderr,
            )
            return 1

    # Check for extra keys in ConfigMap that don't have source files
    source_names = {p.name for p in jsons}
    for key in data:
        if key not in source_names:
            print(
                f"extra key {key} in ConfigMap — run: make grafana-sync",
                file=sys.stderr,
            )
            return 1

    print(f"grafana dashboards in sync ({len(jsons)} dashboards)")
    return 0


def main() -> int:
    if len(sys.argv) > 1 and sys.argv[1] == "--check":
        return check()
    regenerate()
    return 0


if __name__ == "__main__":
    sys.exit(main())
