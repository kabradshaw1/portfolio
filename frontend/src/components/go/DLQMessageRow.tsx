"use client";

import { useState } from "react";
import type { DLQMessage } from "@/lib/go-admin-api";
import { replayDLQMessage } from "@/lib/go-admin-api";

function timeAgo(timestamp: string): string {
  const seconds = Math.floor(
    (Date.now() - new Date(timestamp).getTime()) / 1000,
  );
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

interface DLQMessageRowProps {
  message: DLQMessage;
  onReplayed: () => void;
}

export function DLQMessageRow({ message, onReplayed }: DLQMessageRowProps) {
  const [expanded, setExpanded] = useState(false);
  const [replaying, setReplaying] = useState(false);
  const [replayResult, setReplayResult] = useState<
    "success" | "error" | null
  >(null);
  const [replayError, setReplayError] = useState<string | null>(null);

  async function handleReplay(e: React.MouseEvent) {
    e.stopPropagation();
    setReplaying(true);
    setReplayResult(null);
    setReplayError(null);

    const result = await replayDLQMessage(message.index);

    setReplaying(false);
    if (result.data) {
      setReplayResult("success");
      setTimeout(() => onReplayed(), 1000);
    } else {
      setReplayResult("error");
      setReplayError(result.error || "Replay failed");
    }
  }

  return (
    <>
      <tr
        className="cursor-pointer border-b transition-colors hover:bg-muted/50"
        onClick={() => setExpanded(!expanded)}
      >
        <td className="px-4 py-2 text-muted-foreground">#{message.index}</td>
        <td className="px-4 py-2 font-mono text-sm">{message.routing_key}</td>
        <td className="px-4 py-2 text-muted-foreground">
          {message.timestamp ? timeAgo(message.timestamp) : "—"}
        </td>
        <td className="px-4 py-2 text-center">{message.retry_count}</td>
        <td className="px-4 py-2 text-right">
          {replayResult === "success" ? (
            <span className="text-sm text-green-600 dark:text-green-400">
              Replayed
            </span>
          ) : (
            <button
              onClick={handleReplay}
              disabled={replaying}
              className="rounded bg-primary px-3 py-1 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/80 disabled:opacity-50"
            >
              {replaying ? "..." : "Replay"}
            </button>
          )}
        </td>
      </tr>
      {expanded && (
        <tr className="border-b bg-muted/30">
          <td colSpan={5} className="px-4 py-3">
            <div className="space-y-3 text-sm">
              <div>
                <span className="text-xs font-medium uppercase text-muted-foreground">
                  Exchange
                </span>
                <p className="font-mono">{message.exchange}</p>
              </div>
              <div>
                <span className="text-xs font-medium uppercase text-muted-foreground">
                  Headers
                </span>
                <pre className="mt-1 overflow-x-auto rounded bg-background p-2 text-xs">
                  {JSON.stringify(message.headers, null, 2)}
                </pre>
              </div>
              <div>
                <span className="text-xs font-medium uppercase text-muted-foreground">
                  Body
                </span>
                <pre className="mt-1 overflow-x-auto rounded bg-background p-2 text-xs">
                  {JSON.stringify(message.body, null, 2)}
                </pre>
              </div>
              {replayResult === "error" && replayError && (
                <p className="text-sm text-red-600 dark:text-red-400">
                  {replayError}
                </p>
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  );
}
