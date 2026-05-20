from __future__ import annotations

from collections.abc import Iterable
from datetime import datetime, timezone
from typing import Protocol

import httpx

from app.models import (
    RAGReadinessResponse,
    ReadinessFinding,
    RetrievalConfig,
)

_UTC = timezone.utc  # noqa: UP017


class EvalDBLike(Protocol):
    async def get_dataset(self, ds_id: str) -> dict | None: ...
    async def get_evaluation(self, eval_id: str) -> dict | None: ...
    async def get_experiment(self, experiment_id: str) -> dict | None: ...


class RAGReadinessUpstream:
    def __init__(self, chat_url: str, ingestion_url: str):
        self.chat_url = chat_url.rstrip("/")
        self.ingestion_url = ingestion_url.rstrip("/")

    async def _get_json(self, url: str) -> dict:
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(url)
            resp.raise_for_status()
            return resp.json()

    async def list_collections(self) -> list[dict]:
        payload = await self._get_json(f"{self.ingestion_url}/collections")
        return payload.get("collections", [])

    async def get_collection_config(self, collection: str) -> dict:
        return await self._get_json(
            f"{self.ingestion_url}/collections/{collection}/config"
        )

    async def list_collection_sources(self, collection: str) -> list[dict]:
        payload = await self._get_json(
            f"{self.ingestion_url}/collections/{collection}/sources"
        )
        return payload.get("sources", [])

    async def get_chat_config(self) -> dict:
        return await self._get_json(f"{self.chat_url}/config")


class RAGReadinessChecker:
    def __init__(self, db: EvalDBLike, upstream):
        self.db = db
        self.upstream = upstream

    async def check(
        self,
        *,
        dataset_id: str,
        collection: str,
        rerank: bool,
        retrieval_config: RetrievalConfig | None,
        baseline_eval_id: str | None = None,
        experiment_id: str | None = None,
    ) -> RAGReadinessResponse:
        blocking: list[ReadinessFinding] = []
        warnings: list[ReadinessFinding] = []
        evidence: dict = {
            "checked_at": datetime.now(_UTC).isoformat(),
            "requested": {
                "rerank": rerank,
                "retrieval_config": (
                    retrieval_config.model_dump(exclude_none=True)
                    if retrieval_config
                    else {}
                ),
            },
        }

        dataset = await self.db.get_dataset(dataset_id)
        if dataset is None:
            blocking.append(
                _finding(
                    "dataset_not_found",
                    f"Dataset {dataset_id} was not found.",
                    "Choose an existing eval dataset before starting the run.",
                )
            )
            return _response(blocking, warnings, evidence)
        items = dataset.get("items", [])
        evidence["dataset"] = {
            "id": dataset["id"],
            "name": dataset.get("name"),
            "item_count": len(items),
            "expected_sources": sorted(_expected_sources(items)),
        }
        if not items:
            blocking.append(
                _finding(
                    "dataset_empty",
                    f"Dataset {dataset_id} has no items.",
                    "Create a dataset with at least one golden question.",
                )
            )

        await self._validate_baseline(
            baseline_eval_id, dataset_id, collection, blocking, evidence
        )
        await self._validate_experiment(
            experiment_id, dataset_id, collection, blocking, evidence
        )
        await self._collect_upstream(
            collection, rerank, retrieval_config, items, blocking, warnings, evidence
        )
        return _response(blocking, warnings, evidence)

    async def _validate_baseline(
        self, baseline_eval_id, dataset_id, collection, blocking, evidence
    ) -> None:
        if not baseline_eval_id:
            return
        baseline = await self.db.get_evaluation(baseline_eval_id)
        evidence["baseline"] = {"id": baseline_eval_id}
        if baseline is None:
            blocking.append(
                _finding(
                    "baseline_not_found",
                    f"Baseline evaluation {baseline_eval_id} was not found.",
                    "Choose an existing completed baseline for this dataset "
                    "and collection.",
                )
            )
            return
        evidence["baseline"].update(
            {
                "status": baseline.get("status"),
                "dataset_id": baseline.get("dataset_id"),
                "collection": baseline.get("collection"),
            }
        )
        if (
            baseline.get("dataset_id") != dataset_id
            or baseline.get("collection") != collection
        ):
            blocking.append(
                _finding(
                    "baseline_scope_mismatch",
                    "Baseline evaluation uses a different dataset or collection.",
                    "Use a baseline from the same dataset and collection.",
                )
            )

    async def _validate_experiment(
        self, experiment_id, dataset_id, collection, blocking, evidence
    ) -> None:
        if not experiment_id:
            return
        experiment = await self.db.get_experiment(experiment_id)
        evidence["experiment"] = {"id": experiment_id}
        if experiment is None:
            blocking.append(
                _finding(
                    "experiment_not_found",
                    f"Experiment {experiment_id} was not found.",
                    "Choose an existing experiment or create a new one.",
                )
            )
            return
        evidence["experiment"].update(
            {
                "status": experiment.get("status"),
                "dataset_id": experiment.get("dataset_id"),
                "collection": experiment.get("collection"),
            }
        )
        if (
            experiment.get("dataset_id") != dataset_id
            or experiment.get("collection") != collection
        ):
            blocking.append(
                _finding(
                    "experiment_scope_mismatch",
                    "Experiment uses a different dataset or collection.",
                    "Use an experiment with the same dataset and collection.",
                )
            )

    async def _collect_upstream(
        self, collection, rerank, retrieval_config, items, blocking, warnings, evidence
    ) -> None:
        try:
            collections = await self.upstream.list_collections()
        except Exception as exc:
            blocking.append(
                _finding(
                    "collections_unavailable",
                    f"Unable to list retrieval collections: {exc}",
                    "Restore ingestion collection discovery before starting evals.",
                )
            )
            return

        selected = next(
            (item for item in collections if item.get("name") == collection), None
        )
        if selected is None:
            blocking.append(
                _finding(
                    "collection_missing",
                    f"Collection {collection} does not exist.",
                    "Choose an existing retrieval collection.",
                )
            )
            return
        points_count = int(
            selected.get("points_count") or selected.get("point_count") or 0
        )
        evidence["collection"] = {"name": collection, "points_count": points_count}
        if points_count == 0:
            blocking.append(
                _finding(
                    "collection_empty",
                    f"Collection {collection} has 0 points.",
                    "Re-run ingestion or choose a populated collection before "
                    "starting the eval.",
                )
            )

        collection_config = await self._required_collection_config(collection, blocking)
        chat_config = await self._required_chat_config(blocking)
        sources = await self._required_sources(collection, blocking)
        if collection_config is not None:
            evidence["collection"]["config"] = collection_config
        if chat_config is not None:
            evidence["chat"] = chat_config
        if sources is not None:
            evidence["collection"]["sources"] = sources

        if collection_config and chat_config:
            self._check_vector_config(collection_config, chat_config, blocking)
            self._check_rerank_and_top_k(
                rerank, retrieval_config, chat_config, warnings
            )
        if sources is not None:
            self._check_source_coverage(items, sources, blocking, warnings)

    async def _required_collection_config(self, collection, blocking):
        try:
            return await self.upstream.get_collection_config(collection)
        except Exception as exc:
            blocking.append(
                _finding(
                    "collection_config_unavailable",
                    f"Collection config for {collection} is unavailable: {exc}",
                    "Re-ingest the collection so metadata is recorded.",
                )
            )
            return None

    async def _required_chat_config(self, blocking):
        try:
            return await self.upstream.get_chat_config()
        except Exception as exc:
            blocking.append(
                _finding(
                    "chat_config_unavailable",
                    f"Chat config is unavailable: {exc}",
                    "Restore chat /config before starting evals.",
                )
            )
            return None

    async def _required_sources(self, collection, blocking):
        try:
            return await self.upstream.list_collection_sources(collection)
        except Exception as exc:
            blocking.append(
                _finding(
                    "source_inventory_unavailable",
                    f"Source inventory for {collection} is unavailable: {exc}",
                    "Restore ingestion source inventory before starting "
                    "source-sensitive evals.",
                )
            )
            return None

    def _check_vector_config(self, collection_config, chat_config, blocking) -> None:
        if collection_config.get("dense_vector_name") != chat_config.get(
            "dense_vector_name"
        ):
            blocking.append(
                _finding(
                    "dense_vector_mismatch",
                    "Chat and collection dense vector names do not match.",
                    "Reconfigure chat or re-ingest the collection with matching "
                    "dense vector metadata.",
                )
            )
        if chat_config.get("retrieval_mode") == "hybrid":
            if collection_config.get("hybrid_enabled") is False:
                blocking.append(
                    _finding(
                        "hybrid_collection_disabled",
                        "Chat is configured for hybrid retrieval but collection "
                        "metadata has hybrid disabled.",
                        "Use a hybrid collection or switch chat retrieval mode.",
                    )
                )
            if collection_config.get("sparse_vector_name") != chat_config.get(
                "sparse_vector_name"
            ):
                blocking.append(
                    _finding(
                        "sparse_vector_mismatch",
                        "Chat and collection sparse vector names do not match.",
                        "Reconfigure chat or re-ingest the collection with "
                        "matching sparse vector metadata.",
                    )
                )

    def _check_rerank_and_top_k(
        self, rerank, retrieval_config, chat_config, warnings
    ) -> None:
        if rerank and not bool(chat_config.get("rerank_enabled")):
            warnings.append(
                _finding(
                    "rerank_requested_but_disabled",
                    "Rerank was requested but chat runtime rerank support is disabled.",
                    "Enable rerank in chat or run without rerank and record "
                    "the caveat.",
                )
            )
        if (
            retrieval_config
            and retrieval_config.top_k is not None
            and retrieval_config.top_k != chat_config.get("top_k")
        ):
            warnings.append(
                _finding(
                    "top_k_override",
                    "Requested top_k differs from the chat runtime default.",
                    "Keep this caveat with the run because it is an intentional "
                    "retrieval override.",
                )
            )

    def _check_source_coverage(self, items, sources, blocking, warnings) -> None:
        expected = _expected_sources(items)
        if not expected:
            return
        indexed = {
            source.get("filename") for source in sources if source.get("filename")
        }
        matched = sorted(expected & indexed)
        missing = sorted(expected - indexed)
        if not matched:
            blocking.append(
                _finding(
                    "expected_sources_missing",
                    "None of the dataset expected sources are present in the "
                    "selected collection.",
                    "Re-ingest the expected documents or choose the collection "
                    "that contains them.",
                )
            )
        elif missing:
            warnings.append(
                _finding(
                    "partial_expected_source_coverage",
                    f"{len(matched)} of {len(expected)} expected source names "
                    "were found in the collection.",
                    "Review missing sources before treating source-sensitive "
                    "regressions as retrieval failures.",
                )
            )


def _expected_sources(items: Iterable[dict]) -> set[str]:
    return {
        source
        for item in items
        for source in item.get("expected_sources", [])
        if source
    }


def _finding(code: str, message: str, remediation: str) -> ReadinessFinding:
    return ReadinessFinding(code=code, message=message, remediation=remediation)


def _response(blocking, warnings, evidence) -> RAGReadinessResponse:
    status = "blocked" if blocking else "warning" if warnings else "ready"
    if blocking:
        next_steps = [finding.remediation for finding in blocking]
    elif warnings:
        next_steps = [
            "Proceed only if the warning caveats are acceptable for this experiment."
        ]
    else:
        next_steps = ["Proceed with the eval run."]
    return RAGReadinessResponse(
        status=status,
        blocking_failures=blocking,
        warnings=warnings,
        evidence=evidence,
        next_steps=next_steps,
    )
