import os
import uuid

from langchain_text_splitters import Language, RecursiveCharacterTextSplitter
from llm.base import EmbeddingProvider
from qdrant_client import QdrantClient
from qdrant_client.models import Distance, PointStruct, VectorParams

_SKIP_DIRS = {
    ".git",
    "__pycache__",
    ".mypy_cache",
    ".pytest_cache",
    ".ruff_cache",
    "venv",
    ".venv",
    "env",
    ".env",
    "node_modules",
    ".tox",
    "dist",
    "build",
    "egg-info",
}


def collect_python_files(project_path: str) -> list[str]:
    """Walk a project directory and return sorted absolute paths to .py files.

    Skips hidden directories (starting with '.') and known non-source dirs.
    """
    results: list[str] = []

    for root, dirs, files in os.walk(project_path):
        # Prune dirs in-place so os.walk skips them
        dirs[:] = [d for d in dirs if d not in _SKIP_DIRS and not d.startswith(".")]

        for filename in files:
            if filename.endswith(".py"):
                results.append(os.path.abspath(os.path.join(root, filename)))

    return sorted(results)


def chunk_code_files(
    file_paths: list[str],
    project_root: str,
    chunk_size: int = 1500,
    chunk_overlap: int = 200,
) -> list[dict]:
    """Chunk Python source files on logical boundaries (functions, classes).

    Returns a list of chunk dicts with keys: text, file_path, start_line, end_line.
    file_path is relative to project_root.
    start_line and end_line are 1-indexed.
    """
    splitter = RecursiveCharacterTextSplitter.from_language(
        language=Language.PYTHON,
        chunk_size=chunk_size,
        chunk_overlap=chunk_overlap,
    )

    chunks: list[dict] = []

    for abs_path in file_paths:
        try:
            with open(abs_path, encoding="utf-8") as f:
                source = f.read()
        except OSError:
            continue

        if not source.strip():
            continue

        rel_path = os.path.relpath(abs_path, project_root)
        source_lines = source.splitlines()

        splits = splitter.split_text(source)

        for split_text in splits:
            start_line = _find_start_line(split_text, source_lines)
            split_line_count = len(split_text.splitlines())
            end_line = start_line + split_line_count - 1

            chunks.append(
                {
                    "text": split_text,
                    "file_path": rel_path,
                    "start_line": start_line,
                    "end_line": end_line,
                }
            )

    return chunks


def _find_start_line(split_text: str, source_lines: list[str]) -> int:
    """Return the 1-indexed line number where split_text first appears in source_lines.

    Matches on the first non-empty line of the split. Falls back to 1 if not found.
    """
    first_line = next(
        (line for line in split_text.splitlines() if line.strip()),
        None,
    )
    if first_line is None:
        return 1

    for i, src_line in enumerate(source_lines):
        if src_line == first_line:
            return i + 1  # 1-indexed

    return 1


async def index_project(
    project_path: str,
    embedding_provider: EmbeddingProvider,
    qdrant_host: str,
    qdrant_port: int,
) -> dict:
    """Index a Python project into Qdrant for later retrieval.

    Drops and recreates the collection on each run to keep it fresh.
    Returns stats: collection name, files indexed, and chunk count.
    """
    project_name = os.path.basename(os.path.abspath(project_path))
    collection_name = f"debug-{project_name}"

    file_paths = collect_python_files(project_path)
    chunks = chunk_code_files(file_paths, project_root=project_path)

    texts = [chunk["text"] for chunk in chunks]
    vectors = await embedding_provider.embed(texts)

    client = QdrantClient(host=qdrant_host, port=qdrant_port)

    # Drop and recreate so re-indexing is idempotent
    client.delete_collection(collection_name)
    client.create_collection(
        collection_name=collection_name,
        vectors_config=VectorParams(size=768, distance=Distance.COSINE),
    )

    points = [
        PointStruct(
            id=str(uuid.uuid4()),
            vector=vector,
            payload={
                "text": chunk["text"],
                "file_path": chunk["file_path"],
                "start_line": chunk["start_line"],
                "end_line": chunk["end_line"],
                "project_path": project_path,
            },
        )
        for chunk, vector in zip(chunks, vectors)
    ]

    if points:
        client.upsert(collection_name=collection_name, points=points)

    return {
        "collection": collection_name,
        "files_indexed": len(file_paths),
        "chunks": len(chunks),
    }
