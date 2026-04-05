import os
import tempfile
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from app.tools import (
    TOOL_DEFINITIONS,
    execute_tool,
    tool_grep,
    tool_read_file,
    tool_run_tests,
)

# ---------------------------------------------------------------------------
# TOOL_DEFINITIONS structure tests
# ---------------------------------------------------------------------------


def test_tool_definitions_has_four_tools():
    assert len(TOOL_DEFINITIONS) == 4
    names = [t["function"]["name"] for t in TOOL_DEFINITIONS]
    assert "search_code" in names
    assert "read_file" in names
    assert "grep" in names
    assert "run_tests" in names


def test_tool_definitions_have_required_fields():
    for tool in TOOL_DEFINITIONS:
        assert tool["type"] == "function"
        fn = tool["function"]
        assert "name" in fn
        assert "description" in fn
        assert fn["description"]  # non-empty
        assert "parameters" in fn
        params = fn["parameters"]
        assert "type" in params
        assert "properties" in params
        assert "required" in params


# ---------------------------------------------------------------------------
# tool_read_file tests
# ---------------------------------------------------------------------------


def test_read_file_returns_content():
    with tempfile.TemporaryDirectory() as tmpdir:
        filepath = os.path.join(tmpdir, "hello.py")
        lines = [
            "line one\n",
            "line two\n",
            "line three\n",
            "line four\n",
            "line five\n",
        ]
        with open(filepath, "w") as f:
            f.writelines(lines)

        result = tool_read_file(tmpdir, "hello.py", start_line=2, end_line=4)
        assert "line two" in result
        assert "line three" in result
        assert "line four" in result
        assert "line one" not in result
        assert "line five" not in result


def test_read_file_full_file():
    with tempfile.TemporaryDirectory() as tmpdir:
        filepath = os.path.join(tmpdir, "full.py")
        with open(filepath, "w") as f:
            f.write("alpha\nbeta\ngamma\n")

        result = tool_read_file(tmpdir, "full.py")
        assert "alpha" in result
        assert "beta" in result
        assert "gamma" in result


def test_read_file_missing_file():
    with tempfile.TemporaryDirectory() as tmpdir:
        result = tool_read_file(tmpdir, "nonexistent.py")
        assert "not found" in result.lower() or "error" in result.lower()


def test_read_file_rejects_path_traversal():
    with tempfile.TemporaryDirectory() as tmpdir:
        result = tool_read_file(tmpdir, "../../../etc/passwd")
        lower = result.lower()
        assert "error" in lower or "not allowed" in lower or "traversal" in lower


# ---------------------------------------------------------------------------
# tool_grep tests
# ---------------------------------------------------------------------------


def test_grep_finds_matches():
    with tempfile.TemporaryDirectory() as tmpdir:
        filepath = os.path.join(tmpdir, "errors.py")
        with open(filepath, "w") as f:
            f.write("def foo():\n    raise ValueError('bad')\n    return 1\n")

        result = tool_grep(tmpdir, "raise", file_glob="*.py")
        assert "raise" in result
        assert "errors.py" in result


def test_grep_no_matches():
    with tempfile.TemporaryDirectory() as tmpdir:
        filepath = os.path.join(tmpdir, "clean.py")
        with open(filepath, "w") as f:
            f.write("def hello():\n    return 'world'\n")

        result = tool_grep(tmpdir, "xyzzy_nonexistent_pattern", file_glob="*.py")
        assert "No matches found" in result


# ---------------------------------------------------------------------------
# tool_run_tests tests
# ---------------------------------------------------------------------------


def test_run_tests_success():
    with tempfile.TemporaryDirectory() as tmpdir:
        test_file = os.path.join(tmpdir, "test_pass.py")
        with open(test_file, "w") as f:
            f.write("def test_always_passes():\n    assert 1 + 1 == 2\n")

        result = tool_run_tests(tmpdir, "test_pass.py")
        assert "passed" in result.lower() or "1 passed" in result.lower()


def test_run_tests_failure():
    with tempfile.TemporaryDirectory() as tmpdir:
        test_file = os.path.join(tmpdir, "test_fail.py")
        with open(test_file, "w") as f:
            f.write("def test_always_fails():\n    assert 1 == 2\n")

        result = tool_run_tests(tmpdir, "test_fail.py")
        lower = result.lower()
        assert "failed" in lower or "error" in lower or "assert" in lower


# ---------------------------------------------------------------------------
# tool_search_code tests (mocked)
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
@patch("app.tools.QdrantClient")
@patch("app.tools.embed_texts", new_callable=AsyncMock)
async def test_search_code_returns_results(mock_embed, mock_qdrant_cls):
    mock_embed.return_value = [[0.1] * 768]

    mock_hit = MagicMock()
    mock_hit.score = 0.95
    mock_hit.payload = {
        "file_path": "app/main.py",
        "start_line": 10,
        "end_line": 20,
        "text": "def hello(): pass",
    }

    mock_qdrant = MagicMock()
    mock_qdrant.search.return_value = [mock_hit]
    mock_qdrant_cls.return_value = mock_qdrant

    from app.tools import tool_search_code

    result = await tool_search_code(
        query="hello function",
        collection="debug-myproject",
        ollama_base_url="http://localhost:11434",
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
    )

    assert "app/main.py" in result
    assert "0.95" in result
    assert "hello" in result


# ---------------------------------------------------------------------------
# execute_tool dispatcher tests
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_execute_tool_dispatches_correctly():
    with tempfile.TemporaryDirectory() as tmpdir:
        filepath = os.path.join(tmpdir, "sample.py")
        with open(filepath, "w") as f:
            f.write("x = 42\n")

        result = await execute_tool(
            tool_name="read_file",
            arguments={"path": "sample.py"},
            project_path=tmpdir,
            collection="debug-test",
            ollama_base_url="http://localhost:11434",
            embedding_model="nomic-embed-text",
            qdrant_host="localhost",
            qdrant_port=6333,
        )

        assert "42" in result


@pytest.mark.asyncio
async def test_execute_tool_unknown_tool():
    result = await execute_tool(
        tool_name="unknown_tool",
        arguments={},
        project_path="/mock/project",
        collection="debug-test",
        ollama_base_url="http://localhost:11434",
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
    )

    assert "unknown" in result.lower() or "error" in result.lower()
