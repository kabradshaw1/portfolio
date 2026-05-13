from pathlib import Path
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from qdrant_client.models import SparseVector


@pytest.mark.asyncio
async def test_seed_skips_when_collection_already_has_points(tmp_path: Path):
    from app.seed_product_catalog import seed_product_catalog

    pdf = tmp_path / "catalog.pdf"
    pdf.write_bytes(b"%PDF-1.4 demo")

    with (
        patch("app.seed_product_catalog.QdrantStore") as store_cls,
        patch("app.seed_product_catalog.get_embedding_provider") as provider_factory,
        patch("app.seed_product_catalog.SparseVectorEncoder") as sparse_cls,
    ):
        store = MagicMock()
        store.client.collection_exists.return_value = True
        store.client.get_collection.return_value = SimpleNamespace(points_count=3)
        store_cls.return_value = store

        result = await seed_product_catalog(
            pdf_dir=tmp_path,
            collection_name="documents",
        )

    assert result == {"collection": "documents", "pdfs_seeded": 0, "points_count": 3}
    provider_factory.assert_not_called()
    sparse_cls.assert_not_called()
    store.upsert.assert_not_called()


@pytest.mark.asyncio
async def test_seed_processes_all_pdfs_and_writes_collection_metadata(tmp_path: Path):
    from app.seed_product_catalog import seed_product_catalog

    (tmp_path / "a.pdf").write_bytes(b"%PDF-1.4 a")
    (tmp_path / "b.pdf").write_bytes(b"%PDF-1.4 b")
    (tmp_path / "ignore.md").write_text("not a pdf")

    with (
        patch("app.seed_product_catalog.QdrantStore") as store_cls,
        patch("app.seed_product_catalog.get_embedding_provider") as provider_factory,
        patch("app.seed_product_catalog.extract_pages") as extract_pages,
        patch("app.seed_product_catalog.chunk_pages") as chunk_pages,
        patch("app.seed_product_catalog.embed_texts", new_callable=AsyncMock) as embed,
        patch("app.seed_product_catalog.SparseVectorEncoder") as sparse_cls,
        patch("app.seed_product_catalog.CollectionMetaDB") as meta_db_cls,
    ):
        store = MagicMock()
        store.client.collection_exists.return_value = True
        store.client.get_collection.side_effect = [
            SimpleNamespace(points_count=0),
            SimpleNamespace(points_count=4),
        ]
        store_cls.return_value = store

        provider = MagicMock()
        provider_factory.return_value = provider
        extract_pages.return_value = [{"page_number": 1, "text": "demo text"}]
        chunk_pages.return_value = [
            {"text": "chunk one", "page_number": 1, "chunk_index": 0},
            {"text": "chunk two", "page_number": 1, "chunk_index": 1},
        ]
        embed.return_value = [[0.1] * 768, [0.2] * 768]

        sparse_encoder = MagicMock()
        sparse_vectors = [
            SparseVector(indices=[1], values=[0.5]),
            SparseVector(indices=[2], values=[0.4]),
        ]
        sparse_encoder.embed.return_value = sparse_vectors
        sparse_cls.return_value = sparse_encoder

        meta_db = AsyncMock()
        meta_db_cls.return_value = meta_db

        result = await seed_product_catalog(
            pdf_dir=tmp_path,
            collection_name="documents",
        )

    assert result == {"collection": "documents", "pdfs_seeded": 2, "points_count": 4}
    assert extract_pages.call_count == 2
    assert chunk_pages.call_count == 2
    assert embed.await_count == 2
    assert sparse_encoder.embed.call_count == 2
    assert store.upsert.call_count == 2
    meta_db.upsert.assert_awaited_once()
    assert meta_db.upsert.await_args.kwargs["collection"] == "documents"


@pytest.mark.asyncio
async def test_seed_fails_when_collection_remains_empty(tmp_path: Path):
    from app.seed_product_catalog import seed_product_catalog

    (tmp_path / "catalog.pdf").write_bytes(b"%PDF-1.4 demo")

    with (
        patch("app.seed_product_catalog.QdrantStore") as store_cls,
        patch("app.seed_product_catalog.get_embedding_provider"),
        patch("app.seed_product_catalog.extract_pages") as extract_pages,
        patch("app.seed_product_catalog.chunk_pages") as chunk_pages,
        patch("app.seed_product_catalog.embed_texts", new_callable=AsyncMock) as embed,
        patch("app.seed_product_catalog.SparseVectorEncoder") as sparse_cls,
    ):
        store = MagicMock()
        store.client.collection_exists.return_value = True
        store.client.get_collection.return_value = SimpleNamespace(points_count=0)
        store_cls.return_value = store

        extract_pages.return_value = [{"page_number": 1, "text": "demo text"}]
        chunk_pages.return_value = [
            {"text": "chunk", "page_number": 1, "chunk_index": 0},
        ]
        embed.return_value = [[0.1] * 768]
        sparse_encoder = MagicMock()
        sparse_encoder.embed.return_value = [SparseVector(indices=[1], values=[0.5])]
        sparse_cls.return_value = sparse_encoder

        with pytest.raises(RuntimeError, match="collection documents is empty"):
            await seed_product_catalog(
                pdf_dir=tmp_path,
                collection_name="documents",
            )
