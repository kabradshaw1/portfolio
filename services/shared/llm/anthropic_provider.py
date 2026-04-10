"""Anthropic provider — Claude via the Anthropic SDK."""

from __future__ import annotations

import json
from collections.abc import AsyncIterator

import anthropic


class AnthropicLLMProvider:
    def __init__(self, api_key: str, model: str) -> None:
        self.client = anthropic.AsyncAnthropic(api_key=api_key)
        self.model = model

    async def generate(
        self,
        prompt: str,
        system: str,
        *,
        stream: bool = True,
    ) -> AsyncIterator[dict]:
        prompt_tokens = 0
        completion_tokens = 0
        async with self.client.messages.stream(
            model=self.model,
            system=system,
            messages=[{"role": "user", "content": prompt}],
            max_tokens=4096,
        ) as stream_resp:
            async for text in stream_resp.text_stream:
                yield {"token": text}
            message = await stream_resp.get_final_message()
            prompt_tokens = message.usage.input_tokens
            completion_tokens = message.usage.output_tokens
        yield {
            "done": True,
            "prompt_eval_count": prompt_tokens,
            "eval_count": completion_tokens,
            "eval_duration": 0,
        }

    async def chat(
        self,
        messages: list[dict],
        tools: list[dict] | None = None,
    ) -> dict:
        # Separate system message from conversation
        system_content = ""
        chat_messages = []
        for msg in messages:
            if msg.get("role") == "system":
                system_content = msg.get("content", "")
            elif msg.get("role") == "tool":
                # Anthropic uses tool_result blocks
                chat_messages.append(
                    {
                        "role": "user",
                        "content": [
                            {
                                "type": "tool_result",
                                "tool_use_id": msg.get("tool_call_id", ""),
                                "content": msg.get("content", ""),
                            }
                        ],
                    }
                )
            elif msg.get("role") == "assistant" and msg.get("tool_calls"):
                # Convert tool_calls to Anthropic's tool_use blocks
                content_blocks = []
                if msg.get("content"):
                    content_blocks.append({"type": "text", "text": msg["content"]})
                for tc in msg["tool_calls"]:
                    fn = tc.get("function", {})
                    args = fn.get("arguments", {})
                    if isinstance(args, str):
                        args = json.loads(args)
                    content_blocks.append(
                        {
                            "type": "tool_use",
                            "id": tc.get("id", ""),
                            "name": fn.get("name", ""),
                            "input": args,
                        }
                    )
                chat_messages.append(
                    {
                        "role": "assistant",
                        "content": content_blocks,
                    }
                )
            else:
                chat_messages.append(
                    {
                        "role": msg.get("role", "user"),
                        "content": msg.get("content", ""),
                    }
                )

        kwargs: dict = {
            "model": self.model,
            "messages": chat_messages,
            "max_tokens": 4096,
        }
        if system_content:
            kwargs["system"] = system_content
        if tools:
            kwargs["tools"] = [
                {
                    "name": t["function"]["name"],
                    "description": t["function"]["description"],
                    "input_schema": t["function"]["parameters"],
                }
                for t in tools
            ]

        response = await self.client.messages.create(**kwargs)

        # Normalize to Ollama-compatible response shape
        message: dict = {"role": "assistant", "content": ""}
        tool_calls = []
        for block in response.content:
            if block.type == "text":
                message["content"] += block.text
            elif block.type == "tool_use":
                tool_calls.append(
                    {
                        "function": {
                            "name": block.name,
                            "arguments": block.input,
                        }
                    }
                )
        if tool_calls:
            message["tool_calls"] = tool_calls

        return {
            "message": message,
            "prompt_eval_count": response.usage.input_tokens,
            "eval_count": response.usage.output_tokens,
            "eval_duration": 0,
        }

    async def check_health(self) -> bool:
        try:
            # Anthropic doesn't have a list-models endpoint;
            # a minimal message is the lightest check
            await self.client.messages.create(
                model=self.model,
                messages=[{"role": "user", "content": "ping"}],
                max_tokens=1,
            )
            return True
        except Exception:
            return False
