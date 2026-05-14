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
