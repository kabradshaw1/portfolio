"use client";

import { useCallback, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { ScrollArea } from "@/components/ui/scroll-area";
import { sendChat, type AiEvent, type ChatMessage } from "@/lib/ai-service";

import { ToolResultDisplay } from "./tool-results/ToolResultDisplay";

type ToolCallView = {
  id: string;
  name: string;
  args: unknown;
  status: "running" | "success" | "error";
  display?: unknown;
  error?: string;
};

type DisplayItem =
  | { kind: "user"; text: string }
  | { kind: "assistant"; text: string }
  | { kind: "tool"; call: ToolCallView };

export function AiAssistantDrawer() {
  const [open, setOpen] = useState(false);
  const [input, setInput] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [items, setItems] = useState<DisplayItem[]>([]);
  const [busy, setBusy] = useState(false);
  const [errorText, setErrorText] = useState<string | null>(null);
  const [showPanel, setShowPanel] = useState(true);
  const abortRef = useRef<AbortController | null>(null);

  const appendItem = useCallback((item: DisplayItem) => {
    setItems((prev) => [...prev, item]);
  }, []);

  const updateLastTool = useCallback(
    (fn: (call: ToolCallView) => ToolCallView) => {
      setItems((prev) => {
        for (let i = prev.length - 1; i >= 0; i--) {
          const it = prev[i];
          if (it.kind === "tool" && it.call.status === "running") {
            const next = [...prev];
            next[i] = { kind: "tool", call: fn(it.call) };
            return next;
          }
        }
        return prev;
      });
    },
    [],
  );

  const handleSend = useCallback(async () => {
    const trimmed = input.trim();
    if (!trimmed || busy) return;

    const userMsg: ChatMessage = { role: "user", content: trimmed };
    const nextMessages = [...messages, userMsg];
    setMessages(nextMessages);
    appendItem({ kind: "user", text: trimmed });
    setInput("");
    setBusy(true);
    setShowPanel(false);
    setErrorText(null);

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      let idCounter = 0;
      let assistantText = "";

      for await (const ev of sendChat({
        messages: nextMessages,
        signal: controller.signal,
      })) {
        handleEvent(
          ev,
          () => `tc-${idCounter++}`,
          appendItem,
          updateLastTool,
          (text: string) => {
            assistantText = text;
            setItems((prev) => {
              if (prev.length > 0 && prev[prev.length - 1].kind === "assistant") {
                const next = [...prev];
                next[next.length - 1] = { kind: "assistant", text };
                return next;
              }
              return [...prev, { kind: "assistant", text }];
            });
          },
          (reason: string) => setErrorText(reason),
        );
      }

      if (assistantText) {
        setMessages((prev) => [
          ...prev,
          { role: "assistant", content: assistantText },
        ]);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : "unknown error";
      setErrorText(msg);
    } finally {
      setBusy(false);
      abortRef.current = null;
    }
  }, [appendItem, busy, input, messages, updateLastTool]);

  const handleCancel = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  return (
    <>
      {!open && (
        <button
          type="button"
          onClick={() => setOpen(true)}
          data-testid="ai-assistant-open"
          className="fixed bottom-6 right-6 z-40 rounded-full bg-primary px-5 py-3 text-primary-foreground shadow-lg hover:opacity-90"
        >
          Ask AI
        </button>
      )}
      {open && (
        <aside
          role="dialog"
          aria-label="AI assistant"
          data-testid="ai-assistant-drawer"
          className="fixed bottom-0 right-0 top-0 z-50 flex w-full max-w-md flex-col border-l bg-background shadow-xl"
        >
          <header className="flex items-center justify-between border-b px-4 py-3">
            <h2 className="text-base font-semibold">Shopping Assistant</h2>
            <button
              type="button"
              onClick={() => setOpen(false)}
              data-testid="ai-assistant-close"
              className="text-sm text-muted-foreground hover:underline"
            >
              Close
            </button>
          </header>
          {showPanel && (
            <div className="border-b px-4 py-3">
              <button
                type="button"
                onClick={() => setShowPanel(false)}
                className="mb-2 flex w-full items-center justify-between text-xs font-semibold text-muted-foreground"
              >
                <span>What can I help with?</span>
                <span className="text-[10px]">✕</span>
              </button>
              <div className="grid grid-cols-2 gap-2">
                <div className="rounded-md bg-muted p-2">
                  <div className="text-[10px] font-semibold text-blue-500">
                    📦 PRODUCT CATALOG
                  </div>
                  <p className="mt-1 text-[10px] text-muted-foreground">
                    Search products, check prices &amp; stock, manage your cart, view orders
                  </p>
                </div>
                <div className="rounded-md bg-muted p-2">
                  <div className="text-[10px] font-semibold text-green-500">
                    📄 PRODUCT KNOWLEDGE
                  </div>
                  <p className="mt-1 text-[10px] text-muted-foreground">
                    Spec sheets, buying guides, compatibility info
                  </p>
                  <a
                    href="/ai/rag"
                    className="mt-1 block text-[10px] text-blue-400 hover:underline"
                  >
                    Add docs at AI / Document Q&amp;A →
                  </a>
                </div>
              </div>
              <div className="mt-2">
                <div className="text-[10px] text-muted-foreground">TRY ASKING:</div>
                <div className="mt-1 flex flex-wrap gap-1">
                  {[
                    "Compare laptops under $1000",
                    "What's the battery life of the Laptop Pro 15?",
                    "Which cookware is oven-safe?",
                  ].map((q) => (
                    <button
                      key={q}
                      type="button"
                      className="rounded-full bg-muted px-2.5 py-1 text-[10px] text-muted-foreground hover:bg-muted/80"
                      onClick={() => {
                        setInput(q);
                        setShowPanel(false);
                      }}
                    >
                      &quot;{q}&quot;
                    </button>
                  ))}
                </div>
              </div>
            </div>
          )}
          <ScrollArea className="flex-1 px-4 py-3">
            {items.map((it, i) => (
              <div key={i} className="mb-3">
                {it.kind === "user" && (
                  <div className="ml-auto w-fit max-w-[85%] rounded-lg bg-primary px-3 py-2 text-sm text-primary-foreground">
                    {it.text}
                  </div>
                )}
                {it.kind === "assistant" && (
                  <div
                    data-testid="ai-assistant-final"
                    className="mr-auto w-fit max-w-[90%] rounded-lg bg-muted px-3 py-2 text-sm"
                  >
                    {it.text}
                  </div>
                )}
                {it.kind === "tool" && (
                  <>
                    {it.call.status === "running" && (
                      <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                        <span className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-yellow-500" />
                        {it.call.name}
                      </div>
                    )}
                    {it.call.status === "error" && (
                      <div className="rounded border border-red-500/30 p-2 text-xs text-red-500">
                        <span className="font-semibold">{it.call.name}</span>: {it.call.error}
                      </div>
                    )}
                    {it.call.status === "success" && it.call.display !== undefined && (
                      <ToolResultDisplay toolName={it.call.name} display={it.call.display} />
                    )}
                  </>
                )}
              </div>
            ))}
            {errorText && (
              <div
                data-testid="ai-assistant-error"
                className="mt-2 rounded border border-red-500 p-2 text-sm text-red-600"
              >
                {errorText}
              </div>
            )}
          </ScrollArea>
          <div className="border-t p-3">
            <Textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="Ask the shopping assistant…"
              rows={2}
              data-testid="ai-assistant-input"
              disabled={busy}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  void handleSend();
                }
              }}
            />
            <div className="mt-2 flex gap-2">
              <Button
                onClick={handleSend}
                disabled={busy || input.trim().length === 0}
                data-testid="ai-assistant-send"
              >
                {busy ? "Thinking…" : "Send"}
              </Button>
              {busy && (
                <Button variant="outline" onClick={handleCancel}>
                  Cancel
                </Button>
              )}
            </div>
          </div>
        </aside>
      )}
    </>
  );
}

function handleEvent(
  ev: AiEvent,
  nextId: () => string,
  appendItem: (item: DisplayItem) => void,
  updateLastTool: (fn: (call: ToolCallView) => ToolCallView) => void,
  setAssistant: (text: string) => void,
  setError: (reason: string) => void,
) {
  switch (ev.kind) {
    case "tool_call": {
      const id = nextId();
      appendItem({
        kind: "tool",
        call: { id, name: ev.name, args: ev.args, status: "running" },
      });
      break;
    }
    case "tool_result": {
      updateLastTool((call) =>
        call.name === ev.name
          ? { ...call, status: "success", display: ev.display }
          : call,
      );
      break;
    }
    case "tool_error": {
      updateLastTool((call) =>
        call.name === ev.name
          ? { ...call, status: "error", error: ev.error }
          : call,
      );
      break;
    }
    case "final": {
      setAssistant(ev.text);
      break;
    }
    case "error": {
      setError(ev.reason);
      break;
    }
  }
}
