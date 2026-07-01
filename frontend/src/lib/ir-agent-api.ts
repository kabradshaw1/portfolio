/**
 * IR agent client: live SSE investigation, plus replay of captured transcripts.
 *
 * Replay (the default demo mode) fetches pre-recorded transcripts from static
 * assets — no backend, no key, no cost. Live mode streams a real investigation
 * from the ir-agent service.
 *
 * Auth (live mode only): /investigate is behind the shared auth dependency. In
 * production IR_JWT_SECRET must equal the Go auth service's signing secret, so
 * the same httpOnly `access_token` cookie authenticates here too — exactly the
 * pattern eval-api.ts uses (`credentials: "include"` + a refresh-and-retry on
 * 401/403). In local dev ir-agent runs with an empty secret (auth disabled), so
 * live mode works with no token at all.
 */

import { refreshGoAccessToken } from "@/lib/go-auth";
import type {
  IRAgentEvent,
  IRAgentEventName,
  Transcript,
  TranscriptManifestEntry,
} from "@/lib/ir-agent/types";

const IR_AGENT_API_URL =
  process.env.NEXT_PUBLIC_IR_AGENT_API_URL ||
  process.env.NEXT_PUBLIC_API_URL ||
  "http://localhost:8000";

export const irAgentHealthUrl = `${IR_AGENT_API_URL}/health`;

/** Where the capture script writes static transcripts (served from /public). */
const TRANSCRIPTS_BASE = "/ir-agent/transcripts";

export type EventHandler = (event: IRAgentEvent) => void;

const STREAM_EVENT_NAMES: ReadonlySet<string> = new Set<IRAgentEventName>([
  "triage",
  "evidence",
  "findings",
  "verdict",
  "report",
  "summary",
  "done",
  "error",
]);

function asEvent(name: string, data: unknown): IRAgentEvent | null {
  if (!STREAM_EVENT_NAMES.has(name)) return null;
  // The data shape is guaranteed by the backend per event name; the runtime
  // union is validated by the wire contract, not re-parsed here.
  return { event: name, data } as IRAgentEvent;
}

// --- Replay -----------------------------------------------------------------

export async function fetchTranscriptManifest(): Promise<
  TranscriptManifestEntry[]
> {
  const res = await fetch(`${TRANSCRIPTS_BASE}/index.json`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("No transcripts available");
  return res.json();
}

export async function fetchTranscript(file: string): Promise<Transcript> {
  const res = await fetch(`${TRANSCRIPTS_BASE}/${file}`, { cache: "no-store" });
  if (!res.ok) throw new Error(`Transcript not found: ${file}`);
  return res.json();
}

/**
 * Replay a captured transcript through `onEvent`, pacing events so the run
 * unfolds like a live stream rather than appearing all at once. Honors an
 * AbortSignal so the UI can cancel an in-flight replay.
 */
export async function replayTranscript(
  transcript: Transcript,
  onEvent: EventHandler,
  options: { signal?: AbortSignal; stepDelayMs?: number } = {},
): Promise<void> {
  const { signal, stepDelayMs = 600 } = options;
  for (const event of transcript.events) {
    if (signal?.aborted) return;
    onEvent(event);
    if (event.event === "done") return;
    await delay(stepDelayMs, signal);
  }
}

function delay(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal?.aborted) return resolve();
    const timer = setTimeout(resolve, ms);
    signal?.addEventListener(
      "abort",
      () => {
        clearTimeout(timer);
        resolve();
      },
      { once: true },
    );
  });
}

// --- Live -------------------------------------------------------------------

/**
 * Stream a live investigation from the ir-agent service, invoking `onEvent` per
 * SSE frame. Retries once after refreshing the auth cookie on a 401/403.
 */
export async function streamInvestigation(
  incidentId: string,
  onEvent: EventHandler,
  options: { signal?: AbortSignal } = {},
): Promise<void> {
  const res = await postInvestigate(incidentId, options.signal);
  if (!res.ok) {
    const detail = await res
      .json()
      .then((b) => b.detail)
      .catch(() => null);
    throw new Error(detail || `Investigation failed (${res.status})`);
  }
  await consumeSSE(res, onEvent, options.signal);
}

async function postInvestigate(
  incidentId: string,
  signal?: AbortSignal,
): Promise<Response> {
  const init: RequestInit = {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ incident_id: incidentId }),
    credentials: "include",
    signal,
  };
  const res = await fetch(`${IR_AGENT_API_URL}/investigate`, init);
  if (res.status === 401 || res.status === 403) {
    const refreshed = await refreshGoAccessToken();
    if (refreshed) {
      return fetch(`${IR_AGENT_API_URL}/investigate`, init);
    }
  }
  return res;
}

async function consumeSSE(
  res: Response,
  onEvent: EventHandler,
  signal?: AbortSignal,
): Promise<void> {
  const reader = res.body?.getReader();
  if (!reader) throw new Error("No response stream");

  const decoder = new TextDecoder();
  let buffer = "";
  let currentEvent: string | null = null;

  while (true) {
    if (signal?.aborted) {
      await reader.cancel();
      return;
    }
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      if (line.startsWith("event: ")) {
        currentEvent = line.slice(7).trim();
      } else if (line.startsWith("data: ")) {
        const jsonStr = line.slice(6).trim();
        if (!jsonStr || !currentEvent) continue;
        try {
          const event = asEvent(currentEvent, JSON.parse(jsonStr));
          if (event) onEvent(event);
        } catch {
          // skip malformed frames
        }
        currentEvent = null;
      }
    }
  }
}
