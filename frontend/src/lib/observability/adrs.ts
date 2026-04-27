const REPO_BASE_URL =
  "https://github.com/kabradshaw1/portfolio/blob/main";

export const ADR_DIRECTORY_URL =
  "https://github.com/kabradshaw1/portfolio/tree/main/docs/adr/observability";

export type AdrId = "07" | "08" | "09" | "10" | "pg-query";

const ADR_PATHS: Record<AdrId, string> = {
  "07": "docs/adr/observability/07-debuggability-and-instrumentation-gaps.md",
  "08": "docs/adr/observability/08-webhook-incident-and-environment-isolation.md",
  "09": "docs/adr/observability/09-ai-service-observability.md",
  "10": "docs/adr/observability/10-observability-gaps.md",
  "pg-query": "docs/adr/observability/2026-04-27-pg-query-observability.md",
};

const ADR_LABELS: Record<AdrId, string> = {
  "07": "ADR 07",
  "08": "ADR 08",
  "09": "ADR 09",
  "10": "ADR 10",
  "pg-query": "Query Observability ADR",
};

export function adrUrl(id: AdrId): string {
  return `${REPO_BASE_URL}/${ADR_PATHS[id]}`;
}

export function adrLabel(id: AdrId): string {
  return ADR_LABELS[id];
}
