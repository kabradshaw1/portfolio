"use client";

import { Loader2, Play, Radio } from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { IRAgentTimeline } from "@/components/ir-agent/IRAgentTimeline";
import { RunSummaryPanel } from "@/components/ir-agent/RunSummaryPanel";
import {
  fetchTranscript,
  fetchTranscriptManifest,
  irAgentHealthUrl,
  replayTranscript,
  streamInvestigation,
} from "@/lib/ir-agent-api";
import type {
  IRAgentEvent,
  RunSummary,
  TranscriptManifestEntry,
} from "@/lib/ir-agent/types";

type Mode = "replay" | "live";

export default function IRAgentDemo() {
  const [manifest, setManifest] = useState<TranscriptManifestEntry[]>([]);
  const [selected, setSelected] = useState<string>("");
  const [mode, setMode] = useState<Mode>("replay");
  const [events, setEvents] = useState<IRAgentEvent[]>([]);
  const [summary, setSummary] = useState<RunSummary | null>(null);
  const [isRunning, setIsRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [liveHealthy, setLiveHealthy] = useState<boolean | null>(null);

  const abortRef = useRef<AbortController | null>(null);
  const timelineRef = useRef<HTMLDivElement>(null);

  // Load the transcript manifest for the incident selector.
  useEffect(() => {
    fetchTranscriptManifest()
      .then((entries) => {
        setManifest(entries);
        if (entries.length > 0) setSelected(entries[0].incident_id);
      })
      .catch(() => setManifest([]));
  }, []);

  // Probe live-backend health when the user switches to live mode.
  useEffect(() => {
    if (mode !== "live") return;
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 3000);
    fetch(irAgentHealthUrl, { signal: controller.signal })
      .then((res) => setLiveHealthy(res.ok))
      .catch(() => setLiveHealthy(false))
      .finally(() => clearTimeout(timer));
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [mode]);

  useEffect(() => {
    if (timelineRef.current) {
      timelineRef.current.scrollTop = timelineRef.current.scrollHeight;
    }
  }, [events]);

  const handleEvent = useCallback((event: IRAgentEvent) => {
    if (event.event === "summary") {
      setSummary(event.data);
      return;
    }
    if (event.event === "done") {
      setIsRunning(false);
      return;
    }
    if (event.event === "error") {
      setError(event.data.detail);
      return;
    }
    setEvents((prev) => [...prev, event]);
  }, []);

  const run = useCallback(async () => {
    if (!selected) return;
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    setEvents([]);
    setSummary(null);
    setError(null);
    setIsRunning(true);

    try {
      if (mode === "replay") {
        const entry = manifest.find((m) => m.incident_id === selected);
        if (!entry) throw new Error("Transcript not found");
        const transcript = await fetchTranscript(entry.file);
        await replayTranscript(transcript, handleEvent, {
          signal: controller.signal,
        });
      } else {
        await streamInvestigation(selected, handleEvent, {
          signal: controller.signal,
        });
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        setError(
          err instanceof Error
            ? err.message
            : "The investigation could not be started.",
        );
      }
    } finally {
      if (!controller.signal.aborted) setIsRunning(false);
    }
  }, [selected, mode, manifest, handleEvent]);

  useEffect(() => () => abortRef.current?.abort(), []);

  // Auto-play the replay once on first load so the demo is alive immediately.
  const didAutoplay = useRef(false);
  useEffect(() => {
    if (didAutoplay.current) return;
    if (mode !== "replay" || !selected || manifest.length === 0) return;
    didAutoplay.current = true;
    void run();
  }, [selected, manifest, mode, run]);

  const hasTranscripts = manifest.length > 0;
  const liveBlocked = mode === "live" && liveHealthy === false;

  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto max-w-6xl px-6 py-12">
        <Link
          href="/ai/ir-agent"
          className="text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          &larr; Back to overview
        </Link>

        <h1 className="mt-8 text-3xl font-bold">IR Agent — Live Demo</h1>
        <p className="mt-2 max-w-2xl leading-relaxed text-muted-foreground">
          Pick an incident and watch the four-agent pipeline triage, investigate,
          validate, and report — with real token, cost, and latency numbers. Replay
          uses pre-recorded runs (free, always on); &ldquo;Run live&rdquo; streams a
          fresh investigation from the service.
        </p>

        {/* Controls */}
        <div className="mt-8 flex flex-wrap items-end gap-4 rounded-xl border border-foreground/10 bg-card p-4">
          <div className="flex flex-col gap-1">
            <label
              htmlFor="incident"
              className="text-xs font-medium uppercase tracking-wide text-muted-foreground"
            >
              Incident
            </label>
            <select
              id="incident"
              value={selected}
              disabled={!hasTranscripts || isRunning}
              onChange={(e) => setSelected(e.target.value)}
              className="rounded-lg border border-foreground/15 bg-background px-3 py-2 text-sm disabled:opacity-50"
            >
              {!hasTranscripts && <option>No transcripts captured yet</option>}
              {manifest.map((m) => (
                <option key={m.incident_id} value={m.incident_id}>
                  {m.incident_id} — {m.title}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Mode
            </span>
            <div className="inline-flex rounded-lg border border-foreground/15 p-0.5">
              {(["replay", "live"] as Mode[]).map((m) => (
                <button
                  key={m}
                  type="button"
                  disabled={isRunning}
                  onClick={() => setMode(m)}
                  className={`rounded-md px-3 py-1.5 text-sm font-medium capitalize transition-colors disabled:opacity-50 ${
                    mode === m
                      ? "bg-primary text-primary-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {m === "live" ? "Run live" : "Replay"}
                </button>
              ))}
            </div>
          </div>

          <button
            type="button"
            onClick={run}
            disabled={isRunning || (mode === "replay" && !hasTranscripts) || liveBlocked}
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-5 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
          >
            {isRunning ? (
              <Loader2 size={16} className="animate-spin" />
            ) : mode === "live" ? (
              <Radio size={16} />
            ) : (
              <Play size={16} />
            )}
            {isRunning ? "Investigating…" : mode === "live" ? "Run live" : "Replay run"}
          </button>
        </div>

        {liveBlocked && (
          <div className="mt-4 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-700 dark:text-amber-400">
            The live ir-agent service is offline. Replay is always available and
            shows the same captured run.
          </div>
        )}
        {error && (
          <div className="mt-4 rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {error}
          </div>
        )}

        {/* Results */}
        <div className="mt-8 grid grid-cols-1 gap-8 lg:grid-cols-2">
          <div className="flex flex-col">
            <h2 className="mb-4 text-lg font-semibold">Investigation timeline</h2>
            <div
              ref={timelineRef}
              className="max-h-[70vh] min-h-96 flex-1 overflow-y-auto rounded-xl border border-foreground/10 bg-background/40 p-4"
            >
              <IRAgentTimeline events={events} />
            </div>
          </div>

          <div className="flex flex-col">
            <h2 className="mb-4 text-lg font-semibold">Measured outcomes</h2>
            <div className="flex-1 rounded-xl border border-foreground/10 bg-background/40 p-4">
              {summary ? (
                <RunSummaryPanel summary={summary} />
              ) : (
                <p className="text-sm text-muted-foreground">
                  Per-role token, cost, and latency numbers appear here once the run
                  completes — including how much the role-tiering saves versus running
                  Opus on every node.
                </p>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
