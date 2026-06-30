/**
 * Wire contracts for the IR agent's SSE stream and replay transcripts.
 *
 * These mirror the pydantic models in services/ir-agent/app/models.py and the
 * summary shape emitted by app/usage.py. The same types describe both a live
 * `/investigate` stream and a captured transcript, so the demo renders either
 * source through one code path.
 */

export type Severity = "low" | "medium" | "high" | "critical";
export type Category =
  | "phishing"
  | "malware"
  | "lateral-movement"
  | "data-exfil"
  | "other";

export interface TriageResult {
  severity: Severity;
  category: Category;
  confidence: number;
  rationale: string;
}

export interface EvidenceItem {
  id: string;
  source_tool: string;
  query: string;
  content: string;
}

export interface Findings {
  summary: string;
  hypothesis: string;
  evidence_refs: string[];
  iocs: string[];
  affected_assets: string[];
}

export interface ValidationVerdict {
  grounded: boolean;
  unsupported_claims: string[];
  gaps: string[];
  needs_more_investigation: boolean;
}

export interface IRReport {
  executive_summary: string;
  timeline: string[];
  severity: Severity;
  iocs: string[];
  mitre_attack: string[];
  recommended_containment: string[];
  confidence: number;
}

export interface PerRoleUsage {
  model: string;
  input_tokens: number;
  output_tokens: number;
  calls: number;
  cost_usd: number;
  seconds: number;
}

export type Role = "triage" | "investigate" | "validate" | "report";

export interface RunSummary {
  per_role: Partial<Record<Role, PerRoleUsage>>;
  totals: {
    input_tokens: number;
    output_tokens: number;
    total_tokens: number;
    cost_usd: number;
    seconds: number;
  };
  comparison: {
    tiered_cost_usd: number;
    opus_everywhere_cost_usd: number;
    savings_factor: number;
  };
  tool_calls: number;
  investigate_attempts: number;
}

/**
 * One SSE frame. The `event` name selects the `data` shape. A discriminated
 * union keeps the timeline renderer type-safe per event.
 */
export type IRAgentEvent =
  | { event: "triage"; data: TriageResult }
  | { event: "evidence"; data: EvidenceItem[] }
  | { event: "findings"; data: Findings }
  | { event: "verdict"; data: ValidationVerdict }
  | { event: "report"; data: IRReport }
  | { event: "summary"; data: RunSummary }
  | { event: "done"; data: Record<string, never> }
  | { event: "error"; data: { detail: string } };

export type IRAgentEventName = IRAgentEvent["event"];

/** A captured, replayable run: metadata plus the ordered event sequence. */
export interface Transcript {
  incident_id: string;
  title: string;
  captured_at: string;
  events: IRAgentEvent[];
}

/** One entry in the transcript manifest (public/ir-agent/transcripts/index.json). */
export interface TranscriptManifestEntry {
  incident_id: string;
  title: string;
  source: string;
  file: string;
}
