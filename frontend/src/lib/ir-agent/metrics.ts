/**
 * Measured outcomes for the IR agent doc page.
 *
 * These are REAL numbers transcribed from a capture run (see
 * services/ir-agent/scripts/capture_transcripts.py). They are kept as a small
 * curated constant — not invented, not estimated — so the doc page can cite
 * specific figures. Update them whenever transcripts are re-captured.
 *
 * Until the first capture, MEASURED is false and the page shows a placeholder
 * instead of fabricated numbers.
 */

export interface IncidentMetric {
  incidentId: string;
  title: string;
  category: string;
  /** Total run cost under the tiered model assignment. */
  costUsd: number;
  /** Hypothetical cost if every node ran on Opus. */
  opusCostUsd: number;
  totalTokens: number;
  seconds: number;
  /** opusCostUsd / costUsd. */
  savingsFactor: number;
  /** Number of investigate↔validate passes (validator loop). */
  investigateAttempts: number;
  toolCalls: number;
}

/** Flip to true once IR_AGENT_METRICS holds real captured numbers. */
export const MEASURED = true;

/** Date the figures below were captured (ISO date). */
export const CAPTURED_ON = "2026-07-01";

export const IR_AGENT_METRICS: IncidentMetric[] = [
  {
    incidentId: "INC-PHISH-001",
    title: "Credential phishing email followed by anomalous login",
    category: "phishing",
    costUsd: 0.5502,
    opusCostUsd: 0.5835,
    totalTokens: 62903,
    seconds: 234.4,
    savingsFactor: 1.06,
    investigateAttempts: 2,
    toolCalls: 38,
  },
  {
    incidentId: "INC-MAL-002",
    title: "Endpoint beaconing to suspected C2",
    category: "malware",
    costUsd: 0.4055,
    opusCostUsd: 0.431,
    totalTokens: 46266,
    seconds: 138.2,
    savingsFactor: 1.06,
    investigateAttempts: 2,
    toolCalls: 25,
  },
  {
    incidentId: "INC-EXFIL-003",
    title: "Large outbound transfer to personal cloud storage",
    category: "data-exfil",
    costUsd: 0.4454,
    opusCostUsd: 0.4717,
    totalTokens: 51615,
    seconds: 161.5,
    savingsFactor: 1.06,
    investigateAttempts: 2,
    toolCalls: 31,
  },
];
