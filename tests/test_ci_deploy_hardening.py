from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = ROOT / ".github/workflows/ci.yml"
GO_JOB_DIR = ROOT / "go/k8s/jobs"


def load_yaml(path: Path) -> dict:
    return yaml.safe_load(path.read_text())


def test_deploy_jobs_share_runtime_concurrency_lock() -> None:
    workflow = load_yaml(WORKFLOW)

    for job_name in ("deploy-qa", "deploy-prod"):
        concurrency = workflow["jobs"][job_name]["concurrency"]

        assert concurrency["group"] == "shared-runtime-deploy"
        assert concurrency["cancel-in-progress"] is False


def test_go_migration_jobs_allow_image_pull_queue_time() -> None:
    for path in GO_JOB_DIR.glob("*-migrate.yml"):
        job = load_yaml(path)

        assert job["spec"]["activeDeadlineSeconds"] >= 600, path


def test_go_migration_waits_and_failures_use_extended_diagnostics() -> None:
    workflow = WORKFLOW.read_text()

    assert "--timeout=600s job/go-payment-migrate" in workflow
    assert "dump_migration_debug()" in workflow
    assert "kubectl describe job $job -n $namespace" in workflow
    assert "kubectl describe pod -n $namespace -l job-name=$job" in workflow
    assert "kubectl get events -n $namespace --sort-by=.lastTimestamp | tail -80" in workflow
