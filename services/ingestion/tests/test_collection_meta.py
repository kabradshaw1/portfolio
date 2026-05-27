import pytest
from app.collection_meta import CollectionMetaDB


@pytest.mark.asyncio
async def test_round_trip(tmp_path):
    db = CollectionMetaDB(str(tmp_path / "meta.db"))
    await db.init()
    await db.upsert(
        collection="documents",
        chunk_size=1000,
        chunk_overlap=200,
        embedding_model="nomic-embed-text",
    )
    cfg = await db.get("documents")
    assert cfg == {
        "chunk_size": 1000,
        "chunk_overlap": 200,
        "embedding_model": "nomic-embed-text",
    }
    await db.close()


@pytest.mark.asyncio
async def test_get_missing_returns_none(tmp_path):
    db = CollectionMetaDB(str(tmp_path / "meta.db"))
    await db.init()
    assert await db.get("nope") is None
    await db.close()


@pytest.mark.asyncio
async def test_upsert_overwrites_existing(tmp_path):
    db = CollectionMetaDB(str(tmp_path / "meta.db"))
    await db.init()
    await db.upsert(
        collection="documents",
        chunk_size=1000,
        chunk_overlap=200,
        embedding_model="nomic-embed-text",
    )
    await db.upsert(
        collection="documents",
        chunk_size=1500,
        chunk_overlap=300,
        embedding_model="nomic-embed-text",
    )
    cfg = await db.get("documents")
    assert cfg["chunk_size"] == 1500
    assert cfg["chunk_overlap"] == 300
    await db.close()


@pytest.mark.asyncio
async def test_init_idempotent(tmp_path):
    path = str(tmp_path / "meta.db")
    db1 = CollectionMetaDB(path)
    await db1.init()
    await db1.close()
    db2 = CollectionMetaDB(path)
    await db2.init()
    await db2.close()


@pytest.mark.asyncio
async def test_manifest_round_trip(tmp_path):
    db = CollectionMetaDB(str(tmp_path / "meta.db"))
    await db.init()
    manifest = {
        "collection": "eval_product_catalog_v1_a1b2c3d4",
        "fixture_id": "product_catalog_v1",
        "fixture_hash": "a1b2c3d4",
        "documents": [
            {
                "path": "docs/product-catalog/laptop-pro-15-specs.pdf",
                "sha256": "abc123",
            }
        ],
        "provisioned_at": "2026-05-20T00:00:00Z",
        "provisioned_by": "eval-mcp-service",
    }

    await db.upsert_manifest("eval_product_catalog_v1_a1b2c3d4", manifest)

    assert await db.get_manifest("eval_product_catalog_v1_a1b2c3d4") == manifest
    await db.close()


@pytest.mark.asyncio
async def test_get_missing_manifest_returns_none(tmp_path):
    db = CollectionMetaDB(str(tmp_path / "meta.db"))
    await db.init()
    assert await db.get_manifest("missing") is None
    await db.close()
