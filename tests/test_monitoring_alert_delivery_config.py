from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[1]


def load_yaml(path: str) -> dict:
    return yaml.safe_load((ROOT / path).read_text())


def test_prometheus_datasource_declares_prometheus_type() -> None:
    configmap = load_yaml("k8s/monitoring/configmaps/grafana-datasource.yml")
    datasources = yaml.safe_load(configmap["data"]["datasources.yml"])["datasources"]

    prometheus = next(ds for ds in datasources if ds["uid"] == "PBFA97CFB590B2093")

    assert prometheus["jsonData"]["prometheusType"] == "Prometheus"


def test_prometheus_rolls_when_config_changes() -> None:
    deployment = load_yaml("k8s/monitoring/deployments/prometheus.yml")

    annotations = deployment["spec"]["template"]["metadata"]["annotations"]

    assert "monitoring.kylebradshaw.dev/prometheus-config-revision" in annotations


def test_alert_delivery_checks_use_current_grafana_metrics() -> None:
    script = (ROOT / "scripts/ops/check-grafana-alert-delivery.sh").read_text()
    dashboard = (ROOT / "monitoring/grafana/dashboards/system-overview.json").read_text()

    assert "grafana_alerting_remote_writer_writes_total" in script
    assert "grafana_alerting_remote_writer_writes_total" in dashboard
    assert "grafana_alerting_notifications_failed_total" not in script
    assert "grafana_alerting_notifications_failed_total" not in dashboard
