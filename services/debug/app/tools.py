"""Tool definitions and implementations for the debug agent.

Each tool takes string arguments and returns a string result. The LLM sees
TOOL_DEFINITIONS to decide which tool to call; execute_tool routes the call
to the correct implementation.
"""

import fnmatch
import os
import re
import subprocess

import httpx
from qdrant_client import QdrantClient
from qdrant_client.models import ScoredPoint

# ---------------------------------------------------------------------------
# Tool definitions (Ollama tool-calling format)
# ---------------------------------------------------------------------------

TOOL_DEFINITIONS = [
    {
        "type": "function",
        "function": {
            "name": "search_code",
            "description": (
                "Semantically search the indexed codebase for code related to a query."
                " Use this when you need to find functions, classes, or patterns by"
                " meaning rather than exact text. Returns ranked code snippets with"
                " file locations."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": (
                            "Natural-language description of the code to find."
                        ),
                    },
                    "top_k": {
                        "type": "integer",
                        "description": "Number of results to return (default 5).",
                    },
                },
                "required": ["query"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "read_file",
            "description": (
                "Read the contents of a file in the project, optionally limited to a"
                " line range. Use this to inspect source code, configuration, or test"
                " files in detail. Paths must be relative to the project root."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {
                        "type": "string",
                        "description": (
                            "Relative path from project root to the file to read."
                        ),
                    },
                    "start_line": {
                        "type": "integer",
                        "description": (
                            "First line to read (1-indexed, inclusive)."
                            " Omit to start from line 1."
                        ),
                    },
                    "end_line": {
                        "type": "integer",
                        "description": (
                            "Last line to read (1-indexed, inclusive)."
                            " Omit to read to end of file."
                        ),
                    },
                },
                "required": ["path"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "grep",
            "description": (
                "Search for a regex pattern across project files matching a glob."
                " Use this for exact-text or pattern searches, such as finding all"
                " usages of a variable name, import, or error message."
                " Skips virtual envs and caches."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "pattern": {
                        "type": "string",
                        "description": "Regular expression pattern to search for.",
                    },
                    "file_glob": {
                        "type": "string",
                        "description": (
                            "Glob pattern to filter files"
                            " (e.g. '*.py', '*.ts'). Default: '*.py'."
                        ),
                    },
                    "max_matches": {
                        "type": "integer",
                        "description": (
                            "Maximum number of matches to return (default 20)."
                        ),
                    },
                },
                "required": ["pattern"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "run_tests",
            "description": (
                "Run pytest on a specific test file or test within the project."
                " Use this to verify whether a fix works or to understand why a test"
                " is failing. Returns truncated output (up to 50 lines)."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "target": {
                        "type": "string",
                        "description": (
                            "Relative path to the test file to run"
                            " (e.g. 'tests/test_main.py')."
                        ),
                    },
                    "test_name": {
                        "type": "string",
                        "description": (
                            "Optional specific test function name within the file."
                        ),
                    },
                },
                "required": ["target"],
            },
        },
    },
]

# ---------------------------------------------------------------------------
# Dirs to skip when walking the project tree
# ---------------------------------------------------------------------------

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
}

# ---------------------------------------------------------------------------
# embed_texts — duplicated from indexer.py (each service owns its own copy)
# ---------------------------------------------------------------------------


async def embed_texts(
    texts: list[str],
    ollama_base_url: str,
    model: str,
) -> list[list[float]]:
    """Embed a list of texts using Ollama's /api/embed endpoint."""
    if not texts:
        return []

    async with httpx.AsyncClient(timeout=120.0) as client:
        response = await client.post(
            f"{ollama_base_url}/api/embed",
            json={"model": model, "input": texts},
        )
        response.raise_for_status()
        data = response.json()

    return data["embeddings"]


# ---------------------------------------------------------------------------
# Tool implementations
# ---------------------------------------------------------------------------


async def tool_search_code(
    query: str,
    collection: str,
    ollama_base_url: str,
    embedding_model: str,
    qdrant_host: str,
    qdrant_port: int,
    top_k: int = 5,
) -> str:
    """Embed query, search Qdrant, return formatted results."""
    vectors = await embed_texts([query], ollama_base_url, embedding_model)
    if not vectors:
        return "No matching code found."

    client = QdrantClient(host=qdrant_host, port=qdrant_port)
    hits: list[ScoredPoint] = client.search(
        collection_name=collection,
        query_vector=vectors[0],
        limit=top_k,
    )

    if not hits:
        return "No matching code found."

    parts: list[str] = []
    for hit in hits:
        payload = hit.payload or {}
        file_path = payload.get("file_path", "unknown")
        start_line = payload.get("start_line", "?")
        end_line = payload.get("end_line", "?")
        text = payload.get("text", "")
        score = round(hit.score, 2)
        header = f"--- {file_path} (lines {start_line}-{end_line}, score: {score}) ---"
        parts.append(f"{header}\n{text}")

    return "\n\n".join(parts)


def tool_read_file(
    project_path: str,
    path: str,
    start_line: int | None = None,
    end_line: int | None = None,
    max_lines: int = 100,
) -> str:
    """Read a file within project_path, optionally limited to a line range.

    Prevents path traversal by ensuring the resolved path stays inside
    project_path. Returns line-numbered output in "NNNN | content" format.
    """
    # Resolve and validate path
    abs_project = os.path.realpath(project_path)
    # Join raw path segments to project root (strip leading separators)
    candidate = os.path.realpath(os.path.join(abs_project, path.lstrip("/")))

    inside = candidate.startswith(abs_project + os.sep) or candidate == abs_project
    if not inside:
        return (
            f"Error: path traversal not allowed"
            f" — '{path}' resolves outside project root."
        )

    if not os.path.isfile(candidate):
        return f"Error: file not found — '{path}'."

    try:
        with open(candidate, encoding="utf-8") as f:
            all_lines = f.readlines()
    except OSError as exc:
        return f"Error reading file: {exc}"

    # Apply line range (1-indexed, inclusive)
    start = (start_line - 1) if start_line and start_line > 0 else 0
    end = end_line if end_line and end_line > 0 else len(all_lines)
    selected = all_lines[start:end]

    truncated = False
    if len(selected) > max_lines:
        selected = selected[:max_lines]
        truncated = True

    lines_out: list[str] = []
    for i, line in enumerate(selected, start=start + 1):
        lines_out.append(f"{i:4d} | {line.rstrip()}")

    output = "\n".join(lines_out)
    if truncated:
        output += f"\n[output truncated at {max_lines} lines]"

    return output


def tool_grep(
    project_path: str,
    pattern: str,
    file_glob: str = "*.py",
    max_matches: int = 20,
) -> str:
    """Search for a regex pattern across project files matching file_glob."""
    try:
        regex = re.compile(pattern)
    except re.error as exc:
        return f"Error: invalid regex pattern — {exc}"

    abs_project = os.path.realpath(project_path)
    matches: list[str] = []

    for root, dirs, files in os.walk(abs_project):
        # Prune dirs in-place
        dirs[:] = [d for d in dirs if d not in _SKIP_DIRS and not d.startswith(".")]

        for filename in files:
            if not fnmatch.fnmatch(filename, file_glob):
                continue

            abs_file = os.path.join(root, filename)
            rel_file = os.path.relpath(abs_file, abs_project)

            try:
                with open(abs_file, encoding="utf-8", errors="replace") as f:
                    for line_num, line in enumerate(f, start=1):
                        if regex.search(line):
                            matches.append(f"{rel_file}:{line_num}: {line.rstrip()}")
                            if len(matches) >= max_matches:
                                break
            except OSError:
                continue

        if len(matches) >= max_matches:
            break

    if not matches:
        return f"No matches found for pattern: {pattern}"

    return "\n".join(matches)


def tool_run_tests(
    project_path: str,
    target: str,
    test_name: str | None = None,
    timeout: int = 30,
) -> str:
    """Run pytest on a target file (optionally a specific test) within project_path."""
    abs_project = os.path.realpath(project_path)

    pytest_target = target
    if test_name:
        pytest_target = f"{target}::{test_name}"

    cmd = ["python", "-m", "pytest", pytest_target, "-v", "--tb=short", "--no-header"]

    try:
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=timeout,
            cwd=abs_project,
        )
        output = result.stdout + result.stderr
    except subprocess.TimeoutExpired:
        return f"Error: test run timed out after {timeout} seconds."
    except FileNotFoundError:
        return "Error: pytest not found — ensure it is installed in the environment."

    lines = output.splitlines()
    max_output_lines = 50
    if len(lines) > max_output_lines:
        lines = lines[:max_output_lines]
        lines.append(f"[output truncated at {max_output_lines} lines]")

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Dispatcher
# ---------------------------------------------------------------------------


async def execute_tool(
    tool_name: str,
    arguments: dict,
    project_path: str,
    collection: str,
    ollama_base_url: str,
    embedding_model: str,
    qdrant_host: str,
    qdrant_port: int,
) -> str:
    """Route a tool call to the correct implementation.

    All tools return a string result. Unknown tool names return an error string.
    """
    if tool_name == "search_code":
        return await tool_search_code(
            query=arguments.get("query", ""),
            collection=collection,
            ollama_base_url=ollama_base_url,
            embedding_model=embedding_model,
            qdrant_host=qdrant_host,
            qdrant_port=qdrant_port,
            top_k=arguments.get("top_k", 5),
        )

    if tool_name == "read_file":
        return tool_read_file(
            project_path=project_path,
            path=arguments.get("path", ""),
            start_line=arguments.get("start_line"),
            end_line=arguments.get("end_line"),
        )

    if tool_name == "grep":
        return tool_grep(
            project_path=project_path,
            pattern=arguments.get("pattern", ""),
            file_glob=arguments.get("file_glob", "*.py"),
            max_matches=arguments.get("max_matches", 20),
        )

    if tool_name == "run_tests":
        return tool_run_tests(
            project_path=project_path,
            target=arguments.get("target", ""),
            test_name=arguments.get("test_name"),
        )

    return f"Error: unknown tool '{tool_name}'."
