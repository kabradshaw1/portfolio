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
    await db.create_dataset(name="ds2", items=SIMPLE_ITEM)

    datasets = await db.list_datasets()
    assert len(datasets) == 2
    names = {d["name"] for d in datasets}
    assert names == {"ds1", "ds2"}


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
