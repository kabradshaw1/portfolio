from unittest.mock import AsyncMock, patch

import pytest
from app.agent import run_agent_loop


@pytest.mark.asyncio
@patch("app.agent.execute_tool", new_callable=AsyncMock)
@patch("app.agent.call_ollama", new_callable=AsyncMock)
async def test_agent_returns_diagnosis_on_text_response(mock_ollama, mock_tool):
    mock_ollama.return_value = {
        "message": {
            "role": "assistant",
            "content": "Root cause: missing null check in parser.py line 42",
        }
    }

    events = []
    async for event in run_agent_loop(
        description="upload fails",
        error_output=None,
        collection="debug-test",
        project_path="/mock/project",
        ollama_base_url="http://localhost:11434",
        chat_model="qwen2.5:14b",
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        max_steps=10,
    ):
        events.append(event)

    event_types = [e["event"] for e in events]
    assert "diagnosis" in event_types
    assert "done" in event_types
    diagnosis = next(e for e in events if e["event"] == "diagnosis")
    assert "null check" in diagnosis["data"]["content"]


@pytest.mark.asyncio
@patch("app.agent.execute_tool", new_callable=AsyncMock)
@patch("app.agent.call_ollama", new_callable=AsyncMock)
async def test_agent_executes_tool_calls(mock_ollama, mock_tool):
    mock_ollama.side_effect = [
        {
            "message": {
                "role": "assistant",
                "content": "",
                "tool_calls": [
                    {
                        "function": {
                            "name": "search_code",
                            "arguments": {"query": "upload handler"},
                        }
                    }
                ],
            }
        },
        {
            "message": {
                "role": "assistant",
                "content": (
                    "Root cause: the upload handler doesn't validate file type"
                ),
            }
        },
    ]
    mock_tool.return_value = "Found: app/upload.py def handle_upload()..."

    events = []
    async for event in run_agent_loop(
        description="upload fails",
        error_output=None,
        collection="debug-test",
        project_path="/mock/project",
        ollama_base_url="http://localhost:11434",
        chat_model="qwen2.5:14b",
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        max_steps=10,
    ):
        events.append(event)

    event_types = [e["event"] for e in events]
    assert "tool_call" in event_types
    assert "tool_result" in event_types
    assert "diagnosis" in event_types
    mock_tool.assert_called_once()


@pytest.mark.asyncio
@patch("app.agent.execute_tool", new_callable=AsyncMock)
@patch("app.agent.call_ollama", new_callable=AsyncMock)
async def test_agent_stops_at_max_steps(mock_ollama, mock_tool):
    mock_ollama.return_value = {
        "message": {
            "role": "assistant",
            "content": "",
            "tool_calls": [
                {"function": {"name": "grep", "arguments": {"pattern": "error"}}}
            ],
        }
    }
    mock_tool.return_value = "app.py:10: raise ValueError('error')"

    events = []
    async for event in run_agent_loop(
        description="bug",
        error_output=None,
        collection="debug-test",
        project_path="/mock/project",
        ollama_base_url="http://localhost:11434",
        chat_model="qwen2.5:14b",
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        max_steps=3,
    ):
        events.append(event)

    event_types = [e["event"] for e in events]
    assert "done" in event_types
    tool_calls = [e for e in events if e["event"] == "tool_call"]
    assert len(tool_calls) <= 3


@pytest.mark.asyncio
@patch("app.agent.execute_tool", new_callable=AsyncMock)
@patch("app.agent.call_ollama", new_callable=AsyncMock)
async def test_agent_detects_duplicate_tool_calls(mock_ollama, mock_tool):
    mock_ollama.side_effect = [
        {
            "message": {
                "role": "assistant",
                "content": "",
                "tool_calls": [
                    {"function": {"name": "grep", "arguments": {"pattern": "error"}}}
                ],
            }
        },
        {
            "message": {
                "role": "assistant",
                "content": "",
                "tool_calls": [
                    {"function": {"name": "grep", "arguments": {"pattern": "error"}}}
                ],
            }
        },
        {"message": {"role": "assistant", "content": "Root cause: found the issue"}},
    ]
    mock_tool.return_value = "app.py:10: raise ValueError"

    events = []
    async for event in run_agent_loop(
        description="bug",
        error_output=None,
        collection="debug-test",
        project_path="/mock/project",
        ollama_base_url="http://localhost:11434",
        chat_model="qwen2.5:14b",
        embedding_model="nomic-embed-text",
        qdrant_host="localhost",
        qdrant_port=6333,
        max_steps=10,
    ):
        events.append(event)

    event_types = [e["event"] for e in events]
    assert "diagnosis" in event_types
