#!/usr/bin/env python3
"""Focused checks for Grafana alert rules with operational failure history."""

from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
ALERTING = ROOT / "k8s/monitoring/configmaps/grafana-alerting.yml"
JAVA_KUSTOMIZATION = ROOT / "java/k8s/kustomization.yaml"
CANARY_CRONJOB = ROOT / "java/k8s/cronjobs/postgres-auto-explain-canary.yml"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def main() -> None:
    alerting = ALERTING.read_text()

    require(
        '[1h]))' in alerting,
        "pg-auto-explain-stalled must use a bounded 1h Loki range",
    )
    require(
        '|= "duration:" |= "plan:" [12h]' not in alerting,
        "pg-auto-explain-stalled must not scan 12h of raw Postgres logs",
    )

    kustomization = JAVA_KUSTOMIZATION.read_text()
    require(
        "cronjobs/postgres-auto-explain-canary.yml" in kustomization,
        "java/k8s/kustomization.yaml must deploy the auto_explain canary",
    )

    canary = CANARY_CRONJOB.read_text()
    require("SELECT pg_sleep(0.6);" in canary, "canary must emit a slow query plan")
    require("*/30 * * * *" in canary, "canary must run often enough for the 1h alert")


if __name__ == "__main__":
    main()
