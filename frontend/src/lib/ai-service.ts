// frontend/src/lib/ai-service.ts
// SSE client for the go/ai-service /chat endpoint.
// Parses event: / data: lines and yields typed events.

export const AI_SERVICE_URL =
  process.env.NEXT_PUBLIC_AI_SERVICE_URL || "http://localhost:8093";

export type ChatMessage = {
  role: "user" | "assistant" | "system";
  content: string;
};

export type ToolCallEvent = { kind: "tool_call"; name: string; args: unknown };
export type ToolResultEvent = {
  kind: "tool_result";
  name: string;
  display?: unknown;
};
export type ToolErrorEvent = { kind: "tool_error"; name: string; error: string };
export type FinalEvent = { kind: "final"; text: string };
export type ErrorEvent = { kind: "error"; reason: string };

export type AiEvent =
  | ToolCallEvent
  | ToolResultEvent
  | ToolErrorEvent
  | FinalEvent
  | ErrorEvent;

export type SendChatArgs = {
  messages: ChatMessage[];
  jwt?: string | null;
  signal?: AbortSignal;
};

/**
 * sendChat POSTs to /chat and yields parsed SSE events in order.
 * Caller is responsible for accumulating state (message history, final text).
 */
export async function* sendChat(args: SendChatArgs): AsyncGenerator<AiEvent> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Accept: "text/event-stream",
  };
  if (args.jwt) {
    headers["Authorization"] = `Bearer ${args.jwt}`;
  }

  const res = await fetch(`${AI_SERVICE_URL}/chat`, {
    method: "POST",
    headers,
    body: JSON.stringify({ messages: args.messages }),
    signal: args.signal,
  });

  if (!res.ok || !res.body) {
    const text = await res.text().catch(() => "");
    throw new Error(`ai-service /chat: ${res.status} ${text}`);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    // SSE events are separated by a blank line (\n\n).
    let sep: number;
    while ((sep = buffer.indexOf("\n\n")) !== -1) {
      const chunk = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);

      const parsed = parseSseChunk(chunk);
      if (parsed) yield parsed;
    }
  }
}

function parseSseChunk(chunk: string): AiEvent | null {
  let eventName = "";
  const dataLines: string[] = [];
  for (const line of chunk.split("\n")) {
    if (line.startsWith("event:")) {
      eventName = line.slice("event:".length).trim();
    } else if (line.startsWith("data:")) {
      dataLines.push(line.slice("data:".length).trim());
    }
  }
  if (!eventName || dataLines.length === 0) return null;

  let data: unknown;
  try {
    data = JSON.parse(dataLines.join("\n"));
  } catch {
    return null;
  }

  switch (eventName) {
    case "tool_call":
      return {
        kind: "tool_call",
        name: (data as { name: string }).name,
        args: (data as { args: unknown }).args,
      };
    case "tool_result":
      return {
        kind: "tool_result",
        name: (data as { name: string }).name,
        display: (data as { display?: unknown }).display,
      };
    case "tool_error":
      return {
        kind: "tool_error",
        name: (data as { name: string }).name,
        error: (data as { error: string }).error,
      };
    case "final":
      return { kind: "final", text: (data as { text: string }).text };
    case "error":
      return { kind: "error", reason: (data as { reason: string }).reason };
    default:
      return null;
  }
}
