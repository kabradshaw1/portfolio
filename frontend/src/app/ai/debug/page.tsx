"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import Link from "next/link";
import { DebugForm } from "@/components/DebugForm";
import { AgentTimeline, AgentEvent } from "@/components/AgentTimeline";

export default function DebugPage() {
  const [events, setEvents] = useState<AgentEvent[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const timelineRef = useRef<HTMLDivElement>(null);

  const apiUrl = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8000";
  const debugApiUrl = `${apiUrl}/debug`;

  // Auto-scroll timeline as events arrive
  useEffect(() => {
    if (timelineRef.current) {
      timelineRef.current.scrollTop = timelineRef.current.scrollHeight;
    }
  }, [events]);

  const handleSubmit = useCallback(
    async (data: {
      collection: string;
      description: string;
      errorOutput?: string;
    }) => {
      setEvents([]);
      setError(null);
      setIsLoading(true);

      try {
        const res = await fetch(`${debugApiUrl}/debug`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            collection: data.collection,
            description: data.description,
            error_output: data.errorOutput,
          }),
        });

        if (!res.ok) {
          const err = await res
            .json()
            .catch(() => ({ detail: "Failed to start debug session" }));
          throw new Error(err.detail || "Failed to start debug session");
        }

        const reader = res.body?.getReader();
        if (!reader) throw new Error("No response stream");

        const decoder = new TextDecoder();
        let buffer = "";
        let currentEventType: string | null = null;

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split("\n");
          buffer = lines.pop() ?? "";

          for (const line of lines) {
            if (line.startsWith("event: ")) {
              currentEventType = line.slice(7).trim();
            } else if (line.startsWith("data: ")) {
              const jsonStr = line.slice(6).trim();
              if (!jsonStr) continue;

              try {
                const parsed = JSON.parse(jsonStr);
                const eventType = (currentEventType ??
                  parsed.event ??
                  "thinking") as AgentEvent["event"];

                setEvents((prev) => [
                  ...prev,
                  { event: eventType, data: parsed },
                ]);

                if (eventType === "done") {
                  setIsLoading(false);
                }
              } catch {
                // skip malformed SSE data lines
              }

              currentEventType = null;
            }
          }
        }
      } catch (err) {
        setError(
          err instanceof Error
            ? err.message
            : "Could not connect to the debug service. Make sure it is running."
        );
      } finally {
        setIsLoading(false);
      }
    },
    [debugApiUrl]
  );

  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto max-w-6xl px-6 py-12">
        {/* Navigation */}
        <Link
          href="/ai"
          className="text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          &larr; Back
        </Link>

        {/* Header */}
        <h1 className="mt-8 text-3xl font-bold">Debug Assistant</h1>
        <p className="mt-2 text-muted-foreground leading-relaxed">
          Index a Python project, describe a bug, and let the agent search
          through your code to diagnose the issue.
        </p>

        {/* Error Banner */}
        {error && (
          <div className="mt-6 rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {error}
          </div>
        )}

        {/* 2-column grid */}
        <div className="mt-8 grid grid-cols-1 gap-8 lg:grid-cols-2">
          {/* Left: Form */}
          <div>
            <h2 className="mb-4 text-lg font-semibold">Configure Debug Session</h2>
            <DebugForm onSubmit={handleSubmit} isLoading={isLoading} />
          </div>

          {/* Right: Timeline */}
          <div className="flex flex-col">
            <h2 className="mb-4 text-lg font-semibold">Agent Timeline</h2>
            <div
              ref={timelineRef}
              className="flex-1 overflow-y-auto rounded-xl border border-foreground/10 bg-card p-4 min-h-96 max-h-[60vh]"
            >
              <AgentTimeline events={events} />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
