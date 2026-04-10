import os
import tempfile
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from app.indexer import chunk_code_files, collect_python_files, index_project


def test_collect_python_files_finds_py_files():
    with tempfile.TemporaryDirectory() as tmpdir:
        open(os.path.join(tmpdir, "main.py"), "w").write("def hello(): pass")
        open(os.path.join(tmpdir, "util.py"), "w").write("x = 1")
        open(os.path.join(tmpdir, "readme.md"), "w").write("# readme")
        open(os.path.join(tmpdir, "data.json"), "w").write("{}")

        files = collect_python_files(tmpdir)
        assert len(files) == 2
        assert all(f.endswith(".py") for f in files)


def test_collect_python_files_skips_hidden_and_venv():
    with tempfile.TemporaryDirectory() as tmpdir:
        os.makedirs(os.path.join(tmpdir, ".git"))
        open(os.path.join(tmpdir, ".git", "config.py"), "w").write("")
        os.makedirs(os.path.join(tmpdir, "__pycache__"))
        open(os.path.join(tmpdir, "__pycache__", "mod.py"), "w").write("")
        os.makedirs(os.path.join(tmpdir, "venv", "lib"), exist_ok=True)
        open(os.path.join(tmpdir, "venv", "lib", "site.py"), "w").write("")
        open(os.path.join(tmpdir, "app.py"), "w").write("x = 1")

        files = collect_python_files(tmpdir)
        assert len(files) == 1
        assert files[0].endswith("app.py")


def test_chunk_code_files_splits_on_functions():
    with tempfile.TemporaryDirectory() as tmpdir:
        code = '''def foo():
    """Do foo."""
    return 1


def bar():
    """Do bar."""
    return 2


class Baz:
    """A class."""

    def method(self):
        return 3
'''
        filepath = os.path.join(tmpdir, "module.py")
        open(filepath, "w").write(code)

        chunks = chunk_code_files([filepath], project_root=tmpdir)
        assert len(chunks) >= 1
        for chunk in chunks:
            assert "text" in chunk
            assert "file_path" in chunk
            assert "start_line" in chunk
            assert "end_line" in chunk
            assert chunk["file_path"] == "module.py"


def test_chunk_code_files_skips_empty_files():
    with tempfile.TemporaryDirectory() as tmpdir:
        filepath = os.path.join(tmpdir, "empty.py")
        open(filepath, "w").write("")

        chunks = chunk_code_files([filepath], project_root=tmpdir)
        assert len(chunks) == 0


@pytest.mark.asyncio
@patch("app.indexer.QdrantClient")
async def test_index_project_returns_stats(mock_qdrant_cls):
    mock_embedding_provider = AsyncMock()
    mock_embedding_provider.embed.return_value = [[0.1] * 768]
    mock_qdrant = MagicMock()
    mock_qdrant_cls.return_value = mock_qdrant

    with tempfile.TemporaryDirectory() as tmpdir:
        open(os.path.join(tmpdir, "app.py"), "w").write("def hello(): pass")

        result = await index_project(
            project_path=tmpdir,
            embedding_provider=mock_embedding_provider,
            qdrant_host="localhost",
            qdrant_port=6333,
        )

    assert result["files_indexed"] == 1
    assert result["chunks"] >= 1
    assert "collection" in result
    mock_qdrant.delete_collection.assert_called_once()
    mock_qdrant.create_collection.assert_called_once()
