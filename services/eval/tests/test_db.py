import pytest
import pytest_asyncio
from app.db import EvalDB

SIMPLE_ITEM = [{"query": "q", "expected_answer": "a", "expected_sources": []}]


@pytest_asyncio.fixture
async def db(tmp_path):
    """Create a test database in a temp directory."""
    db_path = str(tmp_path / "test.db")
    eval_db = EvalDB(db_path)
    await eval_db.init()
    yield eval_db
    await eval_db.close()


@pytest.mark.asyncio
async def test_create_and_get_dataset(db):
    ds_id = await db.create_dataset(
        name="test-dataset",
        items=[
            {
                "query": "What is chunking?",
                "expected_answer": "Splitting text into smaller pieces",
                "expected_sources": ["ingestion.pdf"],
            }
        ],
    )
    assert ds_id is not None

    dataset = await db.get_dataset(ds_id)
    assert dataset["name"] == "test-dataset"
    assert len(dataset["items"]) == 1
    assert dataset["items"][0]["query"] == "What is chunking?"


@pytest.mark.asyncio
async def test_create_dataset_duplicate_name(db):
    await db.create_dataset(name="dup", items=SIMPLE_ITEM)
    with pytest.raises(ValueError, match="already exists"):
        await db.create_dataset(
            name="dup",
            items=[{"query": "q2", "expected_answer": "a2", "expected_sources": []}],
        )


@pytest.mark.asyncio
async def test_list_datasets(db):
    await db.create_dataset(name="ds1", items=SIMPLE_ITEM)
    await db.create_dataset(
        name="ds2",
        items=[
            {"query": "q1", "expected_answer": "a1", "expected_sources": []},
            {"query": "q2", "expected_answer": "a2", "expected_sources": []},
        ],
    )

    datasets = await db.list_datasets()
    assert len(datasets) == 2
    by_name = {d["name"]: d for d in datasets}
    assert set(by_name) == {"ds1", "ds2"}
    assert by_name["ds1"]["item_count"] == 1
    assert by_name["ds2"]["item_count"] == 2


@pytest.mark.asyncio
async def test_create_and_get_evaluation(db):
    ds_id = await db.create_dataset(name="ds", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    evaluation = await db.get_evaluation(eval_id)
    assert evaluation["status"] == "running"
    assert evaluation["dataset_id"] == ds_id
    assert evaluation["collection"] == "documents"


@pytest.mark.asyncio
async def test_complete_evaluation(db):
    ds_id = await db.create_dataset(name="ds", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    aggregate = {"faithfulness": 0.87, "answer_relevancy": 0.92}
    results = [
        {
            "query": "q",
            "answer": "a",
            "contexts": [],
            "scores": {"faithfulness": 0.87},
        }
    ]

    await db.complete_evaluation(eval_id, aggregate_scores=aggregate, results=results)

    evaluation = await db.get_evaluation(eval_id)
    assert evaluation["status"] == "completed"
    assert evaluation["aggregate_scores"]["faithfulness"] == 0.87
    assert len(evaluation["results"]) == 1


@pytest.mark.asyncio
async def test_fail_evaluation(db):
    ds_id = await db.create_dataset(name="ds", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    await db.fail_evaluation(eval_id, error="LLM timeout")

    evaluation = await db.get_evaluation(eval_id)
    assert evaluation["status"] == "failed"
    assert evaluation["error"] == "LLM timeout"


@pytest.mark.asyncio
async def test_list_evaluations(db):
    ds_id = await db.create_dataset(name="ds", items=SIMPLE_ITEM)
    await db.create_evaluation(dataset_id=ds_id, collection="documents")
    await db.create_evaluation(dataset_id=ds_id, collection="documents")

    evaluations = await db.list_evaluations(limit=10, offset=0)
    assert len(evaluations) == 2


@pytest.mark.asyncio
async def test_get_dataset_not_found(db):
    result = await db.get_dataset("nonexistent")
    assert result is None


@pytest.mark.asyncio
async def test_get_evaluation_not_found(db):
    result = await db.get_evaluation("nonexistent")
    assert result is None


@pytest.mark.asyncio
async def test_create_run_with_notes_and_baseline(db):
    ds_id = await db.create_dataset(name="ds", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(
        dataset_id=ds_id,
        collection="documents",
        notes="bumped overlap to 300",
        baseline_eval_id=None,
    )

    evaluation = await db.get_evaluation(eval_id)
    assert evaluation["notes"] == "bumped overlap to 300"
    assert evaluation["baseline_eval_id"] is None
    assert evaluation["config"] is None


@pytest.mark.asyncio
async def test_set_config_persists_json(db):
    ds_id = await db.create_dataset(name="ds", items=SIMPLE_ITEM)
    eval_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    config = {"chat": {"llm_model": "qwen"}, "collection": {"chunk_size": 1000}}
    await db.set_evaluation_config(eval_id, config)

    evaluation = await db.get_evaluation(eval_id)
    assert evaluation["config"] == config


@pytest.mark.asyncio
async def test_init_is_idempotent_after_columns_exist(tmp_path):
    db_path = str(tmp_path / "idempotent.db")

    db1 = EvalDB(db_path)
    await db1.init()
    await db1.close()

    db2 = EvalDB(db_path)
    await db2.init()  # must not raise even though columns already exist
    await db2.close()


@pytest.mark.asyncio
async def test_create_get_and_list_experiment(db):
    ds_id = await db.create_dataset(name="ds-exp", items=SIMPLE_ITEM)

    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
        baseline_eval_id=None,
        status="running",
        notes="first pass",
    )

    detail = await db.get_experiment(exp_id)
    assert detail["id"] == exp_id
    assert detail["name"] == "precision tuning"
    assert detail["hypothesis"] == "Reranking improves context precision"
    assert detail["dataset_id"] == ds_id
    assert detail["collection"] == "documents"
    assert detail["status"] == "running"
    assert detail["decision"] is None
    assert detail["notes"] == "first pass"
    assert detail["runs"] == []

    experiments = await db.list_experiments(dataset_id=ds_id, collection="documents")
    assert [exp["id"] for exp in experiments] == [exp_id]


@pytest.mark.asyncio
async def test_update_experiment_changes_mutable_fields(db):
    ds_id = await db.create_dataset(name="ds-update", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="initial hypothesis",
        dataset_id=ds_id,
        collection="documents",
    )

    await db.update_experiment(
        exp_id,
        hypothesis="revised hypothesis",
        baseline_eval_id=None,
        status="completed",
        decision="keep",
        notes="rerank won",
    )

    detail = await db.get_experiment(exp_id)
    assert detail["hypothesis"] == "revised hypothesis"
    assert detail["status"] == "completed"
    assert detail["decision"] == "keep"
    assert detail["notes"] == "rerank won"
    assert detail["updated_at"] >= detail["created_at"]


@pytest.mark.asyncio
async def test_sync_experiment_baseline_run_updates_labels(db):
    ds_id = await db.create_dataset(name="ds-sync-baseline", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="baseline sync",
        hypothesis="baseline should be relabeled",
        dataset_id=ds_id,
        collection="documents",
    )
    first_run = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    second_run = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    await db.attach_experiment_run(exp_id, first_run, label="baseline")
    await db.attach_experiment_run(exp_id, second_run, label="candidate")

    await db.sync_experiment_baseline_run(exp_id, second_run)

    detail = await db.get_experiment(exp_id)
    labels = {run["evaluation_id"]: run["label"] for run in detail["runs"]}
    assert labels[first_run] != "baseline"
    assert labels[second_run] == "baseline"


@pytest.mark.asyncio
async def test_sync_experiment_baseline_run_clears_baseline_label(db):
    ds_id = await db.create_dataset(name="ds-clear-baseline", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="baseline clear",
        hypothesis="clear baseline",
        dataset_id=ds_id,
        collection="documents",
    )
    run_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    await db.attach_experiment_run(exp_id, run_id, label="baseline")

    await db.sync_experiment_baseline_run(exp_id, None)

    detail = await db.get_experiment(exp_id)
    labels = [run["label"] for run in detail["runs"]]
    assert "baseline" not in labels


@pytest.mark.asyncio
async def test_list_experiments_filters_by_status(db):
    ds_id = await db.create_dataset(name="ds-filter", items=SIMPLE_ITEM)
    running_id = await db.create_experiment(
        name="running exp",
        hypothesis="running hypothesis",
        dataset_id=ds_id,
        collection="documents",
        status="running",
    )
    await db.create_experiment(
        name="planned exp",
        hypothesis="planned hypothesis",
        dataset_id=ds_id,
        collection="documents",
        status="planned",
    )

    experiments = await db.list_experiments(status="running")

    assert [exp["id"] for exp in experiments] == [running_id]


@pytest.mark.asyncio
async def test_attach_running_completed_and_failed_runs_to_experiment(db):
    ds_id = await db.create_dataset(name="ds-runs", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
        status="running",
    )
    running_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    completed_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    failed_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    await db.complete_evaluation(
        completed_id,
        aggregate_scores={"context_precision": 0.4},
        results=[],
    )
    await db.fail_evaluation(failed_id, error="judge failed")

    await db.attach_experiment_run(
        exp_id, running_id, label="candidate_running", notes="still running"
    )
    await db.attach_experiment_run(
        exp_id, completed_id, label="candidate_completed", notes="finished"
    )
    await db.attach_experiment_run(
        exp_id, failed_id, label="candidate_failed", notes="failed"
    )

    detail = await db.get_experiment(exp_id)
    labels = [run["label"] for run in detail["runs"]]
    statuses = [run["evaluation"]["status"] for run in detail["runs"]]
    assert labels == ["candidate_running", "candidate_completed", "candidate_failed"]
    assert statuses == ["running", "completed", "failed"]


@pytest.mark.asyncio
async def test_attach_experiment_run_rejects_duplicate_label(db):
    ds_id = await db.create_dataset(name="ds-dupe-label", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
    )
    run_1 = await db.create_evaluation(dataset_id=ds_id, collection="documents")
    run_2 = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    await db.attach_experiment_run(exp_id, run_1, label="candidate")

    with pytest.raises(ValueError, match="duplicate experiment run label"):
        await db.attach_experiment_run(exp_id, run_2, label="candidate")


@pytest.mark.asyncio
async def test_attach_experiment_run_rejects_already_attached_evaluation(db):
    ds_id = await db.create_dataset(name="ds-dupe-run", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
    )
    run_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    await db.attach_experiment_run(exp_id, run_id, label="candidate")

    with pytest.raises(ValueError, match="evaluation already attached"):
        await db.attach_experiment_run(exp_id, run_id, label="challenger")


@pytest.mark.asyncio
async def test_attach_experiment_run_rejects_completed_experiment(db):
    ds_id = await db.create_dataset(name="ds-completed-exp", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
        status="completed",
    )
    run_id = await db.create_evaluation(dataset_id=ds_id, collection="documents")

    with pytest.raises(ValueError, match="completed experiments cannot accept runs"):
        await db.attach_experiment_run(exp_id, run_id, label="candidate")


@pytest.mark.asyncio
async def test_attach_experiment_run_returns_none_for_missing_experiment_or_run(db):
    ds_id = await db.create_dataset(name="ds-missing-run", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
    )

    assert (
        await db.attach_experiment_run("missing-exp", "missing-run", "candidate")
        is None
    )
    missing_run = await db.attach_experiment_run(exp_id, "missing-run", "candidate")
    assert missing_run is None


@pytest.mark.asyncio
async def test_attach_experiment_run_rejects_dataset_or_collection_mismatch(db):
    ds_id = await db.create_dataset(name="ds-match", items=SIMPLE_ITEM)
    other_ds_id = await db.create_dataset(name="ds-other", items=SIMPLE_ITEM)
    exp_id = await db.create_experiment(
        name="precision tuning",
        hypothesis="Reranking improves context precision",
        dataset_id=ds_id,
        collection="documents",
    )
    other_dataset_run = await db.create_evaluation(
        dataset_id=other_ds_id, collection="documents"
    )
    other_collection_run = await db.create_evaluation(
        dataset_id=ds_id, collection="release-notes"
    )

    with pytest.raises(ValueError, match="same dataset"):
        await db.attach_experiment_run(exp_id, other_dataset_run, label="other_ds")

    with pytest.raises(ValueError, match="same collection"):
        await db.attach_experiment_run(
            exp_id, other_collection_run, label="other_collection"
        )
