"""Agent loop for the debug service.

Calls Ollama repeatedly, dispatching tool calls and collecting results until
the model produces a final text diagnosis.
"""

import json
import time
from collections.abc import AsyncGenerator

import httpx

from app.metrics import (
    AGENT_LOOP_ITERATIONS,
    AGENT_TOOL_CALLS,
    AGENT_TOOL_DURATION,
    OLLAMA_EVAL_DURATION,
    OLLAMA_REQUEST_DURATION,
    OLLAMA_TOKENS,
)
from app.prompts import SYSTEM_PROMPT, build_duplicate_nudge, build_user_prompt
from app.tools import TOOL_DEFINITIONS, execute_tool

# ---------------------------------------------------------------------------
# Ollama HTTP client
# ---------------------------------------------------------------------------


async def call_ollama(
    messages: list[dict],
    model: str,
    base_url: str,
    tools: list[dict] | None = None,
) -> dict:
    """POST to Ollama /api/chat and return the parsed JSON response."""
    payload: dict = {
        "model": model,
        "messages": messages,
        "stream": False,
    }
    if tools is not None:
        payload["tools"] = tools

    start = time.perf_counter()
    async with httpx.AsyncClient(timeout=300.0) as client:
        response = await client.post(f"{base_url}/api/chat", json=payload)
        response.raise_for_status()
        data = response.json()
    duration = time.perf_counter() - start

    operation = "chat" if tools else "chat_final"
    OLLAMA_REQUEST_DURATION.labels(
        service="debug", model=model, operation=operation
    ).observe(duration)

    # Record token metrics from response
    prompt_tokens = data.get("prompt_eval_count", 0)
    completion_tokens = data.get("eval_count", 0)
    if prompt_tokens:
        OLLAMA_TOKENS.labels(service="debug", model=model, kind="prompt").inc(
            prompt_tokens
        )
    if completion_tokens:
        OLLAMA_TOKENS.labels(service="debug", model=model, kind="completion").inc(
            completion_tokens
        )
    eval_ns = data.get("eval_duration", 0)
    if eval_ns:
        OLLAMA_EVAL_DURATION.labels(service="debug", model=model).observe(eval_ns / 1e9)

    return data


# ---------------------------------------------------------------------------
# Agent loop
# ---------------------------------------------------------------------------


async def run_agent_loop(
    description: str,
    error_output: str | None,
    collection: str,
    project_path: str,
    ollama_base_url: str,
    chat_model: str,
    embedding_model: str,
    qdrant_host: str,
    qdrant_port: int,
    max_steps: int = 10,
) -> AsyncGenerator[dict, None]:
    """Run the debug agent loop, yielding SSE-style event dicts.

    The loop calls Ollama with tool definitions, dispatches any tool calls,
    appends results to the message history, and repeats until the model
    responds with plain text (a diagnosis) or max_steps is exhausted.
    """
    messages: list[dict] = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": build_user_prompt(description, error_output)},
    ]

    seen_calls: set[str] = set()

    for step in range(1, max_steps + 1):
        # --- call Ollama ---
        try:
            response = await call_ollama(
                messages=messages,
                model=chat_model,
                base_url=ollama_base_url,
                tools=TOOL_DEFINITIONS,
            )
        except Exception as exc:  # noqa: BLE001
            AGENT_LOOP_ITERATIONS.labels(service="debug").observe(step)
            yield {
                "event": "diagnosis",
                "data": {"step": step, "content": f"Agent error: {exc}"},
            }
            yield {"event": "done", "data": {}}
            return

        message = response.get("message", {})
        tool_calls = message.get("tool_calls")

        # --- no tool calls → final diagnosis ---
        if not tool_calls:
            content = message.get("content", "")
            AGENT_LOOP_ITERATIONS.labels(service="debug").observe(step)
            yield {"event": "diagnosis", "data": {"step": step, "content": content}}
            yield {"event": "done", "data": {}}
            return

        # --- process first tool call ---
        first_call = tool_calls[0]
        fn = first_call.get("function", {})
        tool_name: str = fn.get("name", "")
        arguments = fn.get("arguments", {})

        # Arguments may arrive as a JSON string
        if isinstance(arguments, str):
            arguments = json.loads(arguments)

        # Emit thinking event if the assistant produced content alongside the call
        assistant_content = message.get("content", "")
        if assistant_content:
            yield {
                "event": "thinking",
                "data": {"step": step, "content": assistant_content},
            }

        # Duplicate-call detection
        call_key = json.dumps({"tool": tool_name, "args": arguments}, sort_keys=True)
        if call_key in seen_calls:
            nudge = build_duplicate_nudge(tool_name, json.dumps(arguments))
            messages.append({"role": "user", "content": nudge})
            continue

        seen_calls.add(call_key)

        # Emit tool_call event
        yield {
            "event": "tool_call",
            "data": {"step": step, "tool": tool_name, "args": arguments},
        }

        # Execute the tool
        tool_start = time.perf_counter()
        result: str = await execute_tool(
            tool_name=tool_name,
            arguments=arguments,
            project_path=project_path,
            collection=collection,
            ollama_base_url=ollama_base_url,
            embedding_model=embedding_model,
            qdrant_host=qdrant_host,
            qdrant_port=qdrant_port,
        )
        AGENT_TOOL_DURATION.labels(tool=tool_name).observe(
            time.perf_counter() - tool_start
        )
        AGENT_TOOL_CALLS.labels(tool=tool_name, result="success").inc()

        # Emit tool_result event (truncate for display)
        truncated = len(result) > 2000
        display_result = result[:2000] if truncated else result
        yield {
            "event": "tool_result",
            "data": {
                "step": step,
                "tool": tool_name,
                "result": display_result,
                "truncated": truncated,
            },
        }

        # Append assistant message + tool result to history
        messages.append(
            {
                "role": "assistant",
                "content": assistant_content,
                "tool_calls": tool_calls,
            }
        )
        messages.append(
            {
                "role": "tool",
                "content": result,
            }
        )

    # --- max_steps exhausted: force a final diagnosis ---
    messages.append(
        {
            "role": "user",
            "content": (
                "You have used the maximum number of tool calls. "
                "Based on the evidence gathered so far, provide your diagnosis now."
            ),
        }
    )

    try:
        response = await call_ollama(
            messages=messages,
            model=chat_model,
            base_url=ollama_base_url,
            tools=None,
        )
    except Exception as exc:  # noqa: BLE001
        AGENT_LOOP_ITERATIONS.labels(service="debug").observe(max_steps)
        yield {
            "event": "diagnosis",
            "data": {"step": max_steps, "content": f"Agent error: {exc}"},
        }
        yield {"event": "done", "data": {}}
        return

    content = response.get("message", {}).get("content", "")
    AGENT_LOOP_ITERATIONS.labels(service="debug").observe(max_steps)
    yield {"event": "diagnosis", "data": {"step": max_steps, "content": content}}
    yield {"event": "done", "data": {}}
