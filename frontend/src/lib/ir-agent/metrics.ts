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
export const MEASURED = false;

/** Date the figures below were captured (ISO date). */
export const CAPTURED_ON = "";

export const IR_AGENT_METRICS: IncidentMetric[] = [
  // Filled from the capture run, e.g.:
  // {
  //   incidentId: "INC-PHISH-001",
  //   title: "Phishing credential theft",
  //   category: "phishing",
  //   costUsd: 0,
  //   opusCostUsd: 0,
  //   totalTokens: 0,
  //   seconds: 0,
  //   savingsFactor: 0,
  //   investigateAttempts: 0,
  //   toolCalls: 0,
  // },
];
