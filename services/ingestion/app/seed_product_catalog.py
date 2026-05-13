from __future__ import annotations

import argparse
import asyncio
import os
import uuid
from io import BytesIO
from pathlib import Path

from llm.factory import get_embedding_provider
from rag.sparse import SparseVectorEncoder

from app.chunker import chunk_pages
from app.collection_meta import CollectionMetaDB
from app.config import settings
from app.embedder import embed_texts
from app.pdf_parser import extract_pages
from app.store import QdrantStore

DEFAULT_PDF_DIR = Path("/app/seed/product-catalog")


def _point_count(store: QdrantStore, collection_name: str) -> int:
    if not store.client.collection_exists(collection_name):
        return 0
    info = store.client.get_collection(collection_name)
    return int(info.points_count or 0)


async def _write_collection_metadata(collection_name: str) -> None:
    os.makedirs(os.path.dirname(settings.collection_meta_db_path) or ".", exist_ok=True)
    meta_db = CollectionMetaDB(settings.collection_meta_db_path)
    await meta_db.init()
    try:
        await meta_db.upsert(
            collection=collection_name,
            chunk_size=settings.chunk_size,
            chunk_overlap=settings.chunk_overlap,
            embedding_model=settings.embedding_model,
            sparse_model=settings.sparse_model,
            hybrid_enabled=settings.hybrid_enabled,
        )
    finally:
        await meta_db.close()


async def seed_product_catalog(
    pdf_dir: Path,
    collection_name: str,
) -> dict[str, int | str]:
    store = QdrantStore(
        host=settings.qdrant_host,
        port=settings.qdrant_port,
        collection_name=collection_name,
    )

    existing_points = _point_count(store, collection_name)
    if existing_points > 0:
        return {
            "collection": collection_name,
            "pdfs_seeded": 0,
            "points_count": existing_points,
        }

    pdf_paths = sorted(path for path in pdf_dir.glob("*.pdf") if path.is_file())
    if not pdf_paths:
        raise FileNotFoundError(f"no PDFs found in {pdf_dir}")

    embedding_provider = get_embedding_provider(
        provider=settings.embedding_provider,
        base_url=settings.get_embedding_base_url(),
        api_key=settings.embedding_api_key,
        model=settings.embedding_model,
    )
    sparse_encoder = SparseVectorEncoder(
        model_name=settings.sparse_model,
        batch_size=settings.sparse_batch_size,
    )

    seeded_count = 0
    for pdf_path in pdf_paths:
        pages = extract_pages(BytesIO(pdf_path.read_bytes()))
        chunks = chunk_pages(
            pages,
            chunk_size=settings.chunk_size,
            chunk_overlap=settings.chunk_overlap,
        )
        if not chunks:
            continue

        texts = [chunk["text"] for chunk in chunks]
        vectors = await embed_texts(
            texts=texts,
            provider=embedding_provider,
            model=settings.embedding_model,
        )
        sparse_vectors = sparse_encoder.embed(texts)
        store.upsert(
            chunks=chunks,
            vectors=vectors,
            sparse_vectors=sparse_vectors,
            document_id=str(uuid.uuid4()),
            filename=pdf_path.name,
        )
        seeded_count += 1

    await _write_collection_metadata(collection_name)

    final_points = _point_count(store, collection_name)
    if final_points <= 0:
        raise RuntimeError(f"collection {collection_name} is empty after seeding")

    return {
        "collection": collection_name,
        "pdfs_seeded": seeded_count,
        "points_count": final_points,
    }


async def async_main() -> None:
    parser = argparse.ArgumentParser(description="Seed product PDFs into Qdrant.")
    parser.add_argument(
        "--pdf-dir",
        type=Path,
        default=DEFAULT_PDF_DIR,
        help="Directory containing product PDF files.",
    )
    parser.add_argument(
        "--collection",
        default=settings.collection_name,
        help="Target Qdrant collection. Defaults to COLLECTION_NAME.",
    )
    args = parser.parse_args()

    result = await seed_product_catalog(
        pdf_dir=args.pdf_dir,
        collection_name=args.collection,
    )
    print(
        "seed_product_catalog complete: "
        f"collection={result['collection']} "
        f"pdfs_seeded={result['pdfs_seeded']} "
        f"points_count={result['points_count']}"
    )


def main() -> None:
    asyncio.run(async_main())


if __name__ == "__main__":
    main()
