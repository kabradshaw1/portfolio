import { refreshGoAccessToken } from "@/lib/go-auth";

const EVAL_API_URL =
  process.env.NEXT_PUBLIC_EVAL_API_URL || "http://localhost:8000/eval";

async function evalFetch(
  path: string,
  options: RequestInit = {},
): Promise<Response> {
  const headers = new Headers(options.headers);
  if (!headers.has("Content-Type") && options.body) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(`${EVAL_API_URL}${path}`, {
    ...options,
    headers,
    credentials: "include",
  });

  // Retry once on 401/403 after refreshing the httpOnly cookie
  if (res.status === 401 || res.status === 403) {
    const success = await refreshGoAccessToken();
    if (success) {
      return fetch(`${EVAL_API_URL}${path}`, {
        ...options,
        headers,
        credentials: "include",
      });
    }
  }

  return res;
}

// --- Types ---

export type GoldenItem = {
  query: string;
  expected_answer: string;
  expected_sources: string[];
};

export type DatasetSummary = {
  id: string;
  name: string;
  item_count: number;
  created_at: string;
};

export type QueryScore = {
  faithfulness: number | null;
  answer_relevancy: number | null;
  context_precision: number | null;
  context_recall: number | null;
};

export type QueryResult = {
  query: string;
  answer: string;
  contexts: string[];
  scores: QueryScore;
};

export type EvaluationSummary = {
  id: string;
  dataset_id: string;
  status: "running" | "completed" | "failed";
  collection: string | null;
  aggregate_scores: QueryScore | null;
  created_at: string;
  completed_at: string | null;
};

export type EvaluationDetail = EvaluationSummary & {
  results: QueryResult[] | null;
  error: string | null;
};

// --- API functions ---

export async function getHealth(): Promise<boolean> {
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 3000);
    const res = await fetch(`${EVAL_API_URL}/health`, {
      signal: controller.signal,
    });
    clearTimeout(timeout);
    return res.ok;
  } catch {
    return false;
  }
}

export async function createDataset(
  name: string,
  items: GoldenItem[],
): Promise<{ id: string }> {
  const res = await evalFetch("/datasets", {
    method: "POST",
    body: JSON.stringify({ name, items }),
  });
  if (!res.ok) {
    throw new Error(`Failed to create dataset: ${res.status} ${res.statusText}`);
  }
  return res.json();
}

export async function listDatasets(): Promise<DatasetSummary[]> {
  const res = await evalFetch("/datasets");
  if (!res.ok) {
    throw new Error(`Failed to list datasets: ${res.status} ${res.statusText}`);
  }
  const data = await res.json();
  return data.datasets ?? data;
}

export async function startEvaluation(
  datasetId: string,
  collection?: string,
): Promise<{ id: string; status: string }> {
  const body: Record<string, string> = { dataset_id: datasetId };
  if (collection !== undefined) {
    body.collection = collection;
  }
  const res = await evalFetch("/evaluations", {
    method: "POST",
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(
      `Failed to start evaluation: ${res.status} ${res.statusText}`,
    );
  }
  return res.json();
}

export async function getEvaluation(id: string): Promise<EvaluationDetail> {
  const res = await evalFetch(`/evaluations/${id}`);
  if (!res.ok) {
    throw new Error(
      `Failed to get evaluation: ${res.status} ${res.statusText}`,
    );
  }
  return res.json();
}

export async function listEvaluations(): Promise<EvaluationSummary[]> {
  const res = await evalFetch("/evaluations");
  if (!res.ok) {
    throw new Error(
      `Failed to list evaluations: ${res.status} ${res.statusText}`,
    );
  }
  const data = await res.json();
  return data.evaluations ?? data;
}
