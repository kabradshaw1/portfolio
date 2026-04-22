"use client";

import { useEffect, useState, useCallback } from "react";
import { DemoBanner } from "@/components/go/DemoBanner";
import { DLQMessageRow } from "@/components/go/DLQMessageRow";
import { fetchDLQMessages } from "@/lib/go-admin-api";
import type { DLQMessage } from "@/lib/go-admin-api";

export default function AdminPage() {
  const [messages, setMessages] = useState<DLQMessage[]>([]);
  const [count, setCount] = useState(0);
  const [connected, setConnected] = useState(true);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    const result = await fetchDLQMessages();
    setLoading(false);
    setConnected(result.connected);

    if (result.data) {
      setMessages(result.data.messages);
      setCount(result.data.count);
    } else if (result.error) {
      setError(result.error);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      const result = await fetchDLQMessages();
      if (cancelled) return;
      setLoading(false);
      setConnected(result.connected);

      if (result.data) {
        setMessages(result.data.messages);
        setCount(result.data.count);
      } else if (result.error) {
        setError(result.error);
      }
    }

    load();
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <h1 className="mb-2 text-2xl font-bold">DLQ Admin</h1>
      <p className="mb-6 text-sm text-muted-foreground">
        Inspect and replay dead-lettered messages from the checkout saga.
      </p>

      <DemoBanner />

      {!connected && (
        <div className="mb-6 rounded-lg border border-muted-foreground/20 bg-muted px-4 py-4">
          <p className="font-medium">Admin endpoints are not publicly exposed</p>
          <p className="mt-1 text-sm text-muted-foreground">
            These endpoints are only reachable from within the Kubernetes
            cluster. To use this panel locally:
          </p>
          <pre className="mt-2 overflow-x-auto rounded bg-background px-3 py-2 text-xs">
            ssh -f -N -L 8092:localhost:8092 debian
          </pre>
        </div>
      )}

      {connected && (
        <>
          <div className="mb-6 flex items-center gap-4">
            <div className="rounded border bg-card px-4 py-3">
              <p className="text-xs text-muted-foreground">DLQ Messages</p>
              <p className="mt-1 text-2xl font-bold">{count}</p>
            </div>
            <button
              onClick={refresh}
              disabled={loading}
              className="rounded bg-muted px-4 py-2 text-sm font-medium transition-colors hover:bg-muted/80 disabled:opacity-50"
            >
              {loading ? "Loading..." : "↻ Refresh"}
            </button>
          </div>

          {error && (
            <div className="mb-4 rounded border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-600 dark:text-red-400">
              {error}
            </div>
          )}

          <div className="rounded border bg-card">
            {messages.length > 0 ? (
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="px-4 py-2">Index</th>
                    <th className="px-4 py-2">Routing Key</th>
                    <th className="px-4 py-2">Timestamp</th>
                    <th className="px-4 py-2 text-center">Retries</th>
                    <th className="px-4 py-2 text-right">Action</th>
                  </tr>
                </thead>
                <tbody>
                  {messages.map((msg) => (
                    <DLQMessageRow
                      key={msg.index}
                      message={msg}
                      onReplayed={refresh}
                    />
                  ))}
                </tbody>
              </table>
            ) : (
              <p className="px-4 py-8 text-center text-muted-foreground">
                {loading ? "Loading..." : "No messages in the dead-letter queue"}
              </p>
            )}
          </div>
        </>
      )}
    </div>
  );
}
