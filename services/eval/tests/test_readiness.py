import pytest
from app.models import RetrievalConfig
from app.readiness import RAGReadinessChecker


class FakeDB:
    def __init__(self):
        self.dataset = {
            "id": "ds-1",
            "name": "product-docs-rag-v1",
            "items": [
                {
                    "query": "q1",
                    "expected_answer": "a1",
                    "expected_sources": ["laptop.pdf"],
                },
                {
                    "query": "q2",
                    "expected_answer": "a2",
                    "expected_sources": ["monitor.pdf"],
                },
            ],
            "created_at": "2026-05-20T00:00:00+00:00",
        }
        self.evaluations = {}
        self.experiments = {}

    async def get_dataset(self, dataset_id):
        return self.dataset if dataset_id == self.dataset["id"] else None

    async def get_evaluation(self, eval_id):
        return self.evaluations.get(eval_id)

    async def get_experiment(self, experiment_id):
        return self.experiments.get(experiment_id)


class FakeUpstream:
    def __init__(self):
        self.collections = [{"name": "documents", "points_count": 10}]
        self.collection_config = {
            "chunk_size": 1000,
            "chunk_overlap": 200,
            "embedding_model": "nomic-embed-text",
            "hybrid_enabled": True,
            "dense_vector_name": "dense",
            "sparse_vector_name": "sparse",
            "sparse_model": "Qdrant/bm25",
        }
        self.sources = [
            {"filename": "laptop.pdf", "chunks": 4},
            {"filename": "monitor.pdf", "chunks": 3},
        ]
        self.chat_config = {
            "retrieval_mode": "hybrid",
            "dense_vector_name": "dense",
            "sparse_vector_name": "sparse",
            "rerank_enabled": True,
            "top_k": 5,
        }

    async def list_collections(self):
        return self.collections

    async def get_collection_config(self, collection):
        return self.collection_config

    async def list_collection_sources(self, collection):
        return self.sources

    async def get_chat_config(self):
        return self.chat_config


@pytest.mark.asyncio
async def test_readiness_ready_when_dataset_collection_and_configs_match():
    result = await RAGReadinessChecker(FakeDB(), FakeUpstream()).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=None,
    )

    assert result.status == "ready"
    assert result.blocking_failures == []
    assert result.warnings == []
    assert result.evidence["dataset"]["item_count"] == 2
    assert result.evidence["collection"]["points_count"] == 10


@pytest.mark.asyncio
async def test_readiness_blocks_empty_collection():
    upstream = FakeUpstream()
    upstream.collections = [{"name": "documents", "points_count": 0}]

    result = await RAGReadinessChecker(FakeDB(), upstream).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=None,
    )

    assert result.status == "blocked"
    assert [finding.code for finding in result.blocking_failures] == [
        "collection_empty"
    ]


@pytest.mark.asyncio
async def test_readiness_blocks_missing_collection_config():
    class MissingConfigUpstream(FakeUpstream):
        async def get_collection_config(self, collection):
            raise RuntimeError("status 404")

    result = await RAGReadinessChecker(FakeDB(), MissingConfigUpstream()).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=None,
    )

    assert result.status == "blocked"
    assert [finding.code for finding in result.blocking_failures] == [
        "collection_config_unavailable"
    ]


@pytest.mark.asyncio
async def test_readiness_blocks_zero_expected_source_matches():
    upstream = FakeUpstream()
    upstream.sources = [{"filename": "other.pdf", "chunks": 2}]

    result = await RAGReadinessChecker(FakeDB(), upstream).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=None,
    )

    assert result.status == "blocked"
    assert [finding.code for finding in result.blocking_failures] == [
        "expected_sources_missing"
    ]


@pytest.mark.asyncio
async def test_readiness_warns_partial_expected_source_coverage():
    upstream = FakeUpstream()
    upstream.sources = [{"filename": "laptop.pdf", "chunks": 4}]

    result = await RAGReadinessChecker(FakeDB(), upstream).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=None,
    )

    assert result.status == "warning"
    assert result.blocking_failures == []
    assert [finding.code for finding in result.warnings] == [
        "partial_expected_source_coverage"
    ]


@pytest.mark.asyncio
async def test_readiness_blocks_chat_collection_vector_mismatch():
    upstream = FakeUpstream()
    upstream.collection_config["dense_vector_name"] = "content"

    result = await RAGReadinessChecker(FakeDB(), upstream).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=None,
    )

    assert result.status == "blocked"
    assert [finding.code for finding in result.blocking_failures] == [
        "dense_vector_mismatch"
    ]


@pytest.mark.asyncio
async def test_readiness_warns_rerank_requested_when_runtime_disabled():
    upstream = FakeUpstream()
    upstream.chat_config["rerank_enabled"] = False

    result = await RAGReadinessChecker(FakeDB(), upstream).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=True,
        retrieval_config=None,
    )

    assert result.status == "warning"
    assert [finding.code for finding in result.warnings] == [
        "rerank_requested_but_disabled"
    ]


@pytest.mark.asyncio
async def test_readiness_warns_top_k_override_differs_from_chat_default():
    result = await RAGReadinessChecker(FakeDB(), FakeUpstream()).check(
        dataset_id="ds-1",
        collection="documents",
        rerank=False,
        retrieval_config=RetrievalConfig(top_k=3),
    )

    assert result.status == "warning"
    assert [finding.code for finding in result.warnings] == ["top_k_override"]
