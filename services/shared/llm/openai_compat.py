"""OpenAI-compatible provider — works with OpenAI, Groq, Together AI, OpenRouter."""

from __future__ import annotations

import json
from collections.abc import AsyncIterator

import openai


class OpenAIEmbeddingProvider:
    def __init__(self, base_url: str, api_key: str, model: str) -> None:
        self.client = openai.AsyncOpenAI(base_url=base_url, api_key=api_key)
        self.model = model

    async def embed(self, texts: list[str]) -> list[list[float]]:
        if not texts:
            return []
        response = await self.client.embeddings.create(model=self.model, input=texts)
        return [item.embedding for item in response.data]

    async def check_health(self) -> bool:
        try:
            await self.client.models.list()
            return True
        except Exception:
            return False


class OpenAILLMProvider:
    def __init__(self, base_url: str, api_key: str, model: str) -> None:
        self.client = openai.AsyncOpenAI(base_url=base_url, api_key=api_key)
        self.model = model

    async def generate(
        self,
        prompt: str,
        system: str,
        *,
        stream: bool = True,
    ) -> AsyncIterator[dict]:
        messages = [
            {"role": "system", "content": system},
            {"role": "user", "content": prompt},
        ]
        response = await self.client.chat.completions.create(
            model=self.model,
            messages=messages,
            stream=True,
        )
        prompt_tokens = 0
        completion_tokens = 0
        async for chunk in response:
            if chunk.choices and chunk.choices[0].delta.content:
                yield {"token": chunk.choices[0].delta.content}
            if chunk.usage:
                prompt_tokens = chunk.usage.prompt_tokens
                completion_tokens = chunk.usage.completion_tokens
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
        kwargs: dict = {
            "model": self.model,
            "messages": messages,
        }
        if tools:
            kwargs["tools"] = tools

        response = await self.client.chat.completions.create(**kwargs)
        choice = response.choices[0]

        # Normalize to Ollama-compatible response shape
        message: dict = {
            "role": "assistant",
            "content": choice.message.content or "",
        }
        if choice.message.tool_calls:
            message["tool_calls"] = [
                {
                    "function": {
                        "name": tc.function.name,
                        "arguments": json.loads(tc.function.arguments),
                    }
                }
                for tc in choice.message.tool_calls
            ]

        return {
            "message": message,
            "prompt_eval_count": response.usage.prompt_tokens if response.usage else 0,
            "eval_count": response.usage.completion_tokens if response.usage else 0,
            "eval_duration": 0,
        }

    async def check_health(self) -> bool:
        try:
            await self.client.models.list()
            return True
        except Exception:
            return False
