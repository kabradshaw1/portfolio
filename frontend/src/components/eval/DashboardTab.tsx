"use client";

import { useEffect, useMemo, useState } from "react";
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import {
  DatasetSummary,
  EvaluationDetail,
  QueryScore,
  compareRuns,
  getHistory,
  listDatasets,
} from "@/lib/eval-api";

const METRICS = [
  ["faithfulness", "Faithfulness"],
  ["answer_relevancy", "Relevancy"],
  ["context_precision", "Precision"],
  ["context_recall", "Recall"],
] as const;

type MetricKey = (typeof METRICS)[number][0];

const chartConfig = {
  faithfulness: { label: "Faithfulness", color: "#2563eb" },
  answer_relevancy: { label: "Relevancy", color: "#16a34a" },
  context_precision: { label: "Precision", color: "#ca8a04" },
  context_recall: { label: "Recall", color: "#dc2626" },
} satisfies ChartConfig;

interface DashboardTabProps {
  onSelectEvaluation: (evaluation: EvaluationDetail) => void;
}

function shortId(id: string): string {
  return id.slice(0, 12);
}

function formatScore(value: number | null | undefined): string {
  return typeof value === "number" ? value.toFixed(2) : "N/A";
}

function formatDelta(value: number | null | undefined): string {
  if (typeof value !== "number") return "0.00";
  const rounded = value.toFixed(2);
  return value > 0 ? `+${rounded}` : rounded;
}

function deltaClass(value: number | null | undefined): string {
  if (typeof value !== "number" || Math.abs(value) < 0.005) {
    return "text-gray-500";
  }
  return value > 0 ? "text-green-600" : "text-red-600";
}

function averageScore(scores: QueryScore | null): number | null {
  if (!scores) return null;
  const values = METRICS.map(([key]) => scores[key]).filter(
    (value): value is number => typeof value === "number",
  );
  if (values.length === 0) return null;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

export function DashboardTab({ onSelectEvaluation }: DashboardTabProps) {
  const [datasets, setDatasets] = useState<DatasetSummary[]>([]);
  const [selectedDatasetId, setSelectedDatasetId] = useState("");
  const [collection, setCollection] = useState("documents");
  const [runs, setRuns] = useState<EvaluationDetail[]>([]);
  const [baselineId, setBaselineId] = useState("");
  const [candidateId, setCandidateId] = useState("");
  const [deltas, setDeltas] = useState<Record<MetricKey, number[]> | null>(
    null,
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [compareError, setCompareError] = useState("");

  useEffect(() => {
    let cancelled = false;
    void listDatasets()
      .then((data) => {
        if (cancelled) return;
        setDatasets(data);
        if (data.length > 0) setSelectedDatasetId(data[0].id);
      })
      .catch(() => {
        if (!cancelled) setError("Failed to load datasets.");
      });

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const trimmedCollection = collection.trim();
    if (!selectedDatasetId || !trimmedCollection) {
      return;
    }

    let cancelled = false;
    void Promise.resolve().then(async () => {
      setLoading(true);
      setError("");
      setCompareError("");
      try {
        const history = await getHistory(selectedDatasetId, trimmedCollection);
        if (cancelled) return;
        setRuns(history.runs);
        setBaselineId(history.runs[0]?.id ?? "");
        setCandidateId(history.runs[history.runs.length - 1]?.id ?? "");
        setDeltas(null);
      } catch {
        if (cancelled) return;
        setRuns([]);
        setError("Failed to load evaluation history.");
      } finally {
        if (!cancelled) setLoading(false);
      }
    });

    return () => {
      cancelled = true;
    };
  }, [selectedDatasetId, collection]);

  useEffect(() => {
    if (!baselineId || !candidateId || baselineId === candidateId) {
      return;
    }

    let cancelled = false;
    void Promise.resolve().then(async () => {
      setCompareError("");
      try {
        const comparison = await compareRuns([baselineId, candidateId]);
        if (cancelled) return;
        setDeltas(comparison.deltas as Record<MetricKey, number[]>);
      } catch {
        if (cancelled) return;
        setDeltas(null);
        setCompareError("Failed to compare selected runs.");
      }
    });

    return () => {
      cancelled = true;
    };
  }, [baselineId, candidateId]);

  const chartData = useMemo(
    () =>
      runs.map((run) => ({
        id: run.id,
        createdAt: new Date(run.created_at).toLocaleDateString(),
        faithfulness: run.aggregate_scores?.faithfulness ?? null,
        answer_relevancy: run.aggregate_scores?.answer_relevancy ?? null,
        context_precision: run.aggregate_scores?.context_precision ?? null,
        context_recall: run.aggregate_scores?.context_recall ?? null,
      })),
    [runs],
  );

  const baselineRun = runs.find((run) => run.id === baselineId) ?? null;
  const candidateRun = runs.find((run) => run.id === candidateId) ?? null;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold text-gray-900">
          RAG Improvement Dashboard
        </h2>
        <p className="mt-1 text-sm text-gray-600">
          Track RAG quality scores over time, compare runs, and connect changes
          to measured quality impact.
        </p>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <div>
          <label
            htmlFor="dashboard-dataset"
            className="mb-1 block text-sm font-medium text-gray-700"
          >
            Dataset
          </label>
          <select
            id="dashboard-dataset"
            value={selectedDatasetId}
            onChange={(event) => setSelectedDatasetId(event.target.value)}
            className="block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
          >
            {datasets.length === 0 && (
              <option value="">No datasets available</option>
            )}
            {datasets.map((dataset) => (
              <option key={dataset.id} value={dataset.id}>
                {dataset.name}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label
            htmlFor="dashboard-collection"
            className="mb-1 block text-sm font-medium text-gray-700"
          >
            Collection
          </label>
          <input
            id="dashboard-collection"
            value={collection}
            onChange={(event) => setCollection(event.target.value)}
            className="block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
          />
        </div>
      </div>

      {error && <p className="text-sm text-red-600">{error}</p>}
      {loading && (
        <p className="text-sm text-gray-600">Loading evaluation history...</p>
      )}
      {!loading && !error && selectedDatasetId && !collection.trim() && (
        <p className="text-sm text-gray-600">
          Enter a collection to load history.
        </p>
      )}
      {!loading && !error && collection.trim() && runs.length === 0 && (
        <p className="text-sm text-gray-600">
          No completed runs exist for this dataset and collection.
        </p>
      )}

      {runs.length > 0 && (
        <>
          <section
            className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm"
            data-testid="rag-score-trend-chart"
          >
            <h3 className="mb-4 text-lg font-semibold text-gray-900">
              Score Trends
            </h3>
            <ChartContainer
              config={chartConfig}
              className="min-h-[280px] w-full"
            >
              <LineChart data={chartData} margin={{ left: 12, right: 12 }}>
                <CartesianGrid vertical={false} />
                <XAxis dataKey="createdAt" tickLine={false} axisLine={false} />
                <YAxis domain={[0, 1]} tickLine={false} axisLine={false} />
                <ChartTooltip content={<ChartTooltipContent />} />
                {METRICS.map(([key]) => (
                  <Line
                    key={key}
                    type="monotone"
                    dataKey={key}
                    stroke={`var(--color-${key})`}
                    strokeWidth={2}
                    dot
                    connectNulls={false}
                  />
                ))}
              </LineChart>
            </ChartContainer>
          </section>

          <div className="grid gap-6 lg:grid-cols-2">
            <section className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
              <h3 className="mb-4 text-lg font-semibold text-gray-900">
                Run Comparison
              </h3>
              {runs.length < 2 ? (
                <p className="text-sm text-gray-600">
                  At least two completed runs are needed for comparison.
                </p>
              ) : (
                <div className="space-y-4">
                  <div className="grid gap-3 sm:grid-cols-2">
                    <select
                      aria-label="Baseline comparison run"
                      value={baselineId}
                      onChange={(event) => setBaselineId(event.target.value)}
                      className="rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900"
                    >
                      {runs.map((run) => (
                        <option key={run.id} value={run.id}>
                          {shortId(run.id)}
                        </option>
                      ))}
                    </select>
                    <select
                      aria-label="Candidate comparison run"
                      value={candidateId}
                      onChange={(event) => setCandidateId(event.target.value)}
                      className="rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900"
                    >
                      {runs.map((run) => (
                        <option key={run.id} value={run.id}>
                          {shortId(run.id)}
                        </option>
                      ))}
                    </select>
                  </div>
                  {compareError && (
                    <p className="text-sm text-red-600">{compareError}</p>
                  )}
                  <div className="space-y-2">
                    {METRICS.map(([key, label]) => {
                      const base = baselineRun?.aggregate_scores?.[key] ?? null;
                      const candidate =
                        candidateRun?.aggregate_scores?.[key] ?? null;
                      const delta =
                        deltas?.[key]?.[1] ??
                        (typeof base === "number" &&
                        typeof candidate === "number"
                          ? candidate - base
                          : 0);
                      return (
                        <div
                          key={key}
                          className="grid grid-cols-4 items-center gap-2 rounded-md border border-gray-100 px-3 py-2 text-sm"
                        >
                          <span className="font-medium text-gray-700">
                            {label}
                          </span>
                          <span>{formatScore(base)}</span>
                          <span>{formatScore(candidate)}</span>
                          <span className={`font-semibold ${deltaClass(delta)}`}>
                            {formatDelta(delta)}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                </div>
              )}
            </section>

            <section className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
              <h3 className="mb-4 text-lg font-semibold text-gray-900">
                Annotated Change Log
              </h3>
              <div className="space-y-3">
                {runs.map((run) => {
                  const avg = averageScore(run.aggregate_scores);
                  return (
                    <article
                      key={run.id}
                      className="rounded-md border border-gray-100 p-3"
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <p className="text-sm font-medium text-gray-900">
                            {shortId(run.id)}
                          </p>
                          <p className="text-xs text-gray-500">
                            {new Date(run.created_at).toLocaleString()} -{" "}
                            {run.collection}
                          </p>
                        </div>
                        <span className="text-sm font-semibold text-gray-700">
                          {formatScore(avg)}
                        </span>
                      </div>
                      {run.notes && (
                        <p className="mt-2 text-sm text-gray-700">
                          {run.notes}
                        </p>
                      )}
                      {run.config && (
                        <p className="mt-2 text-xs text-gray-500">
                          Config snapshot captured
                        </p>
                      )}
                      <button
                        type="button"
                        onClick={() => onSelectEvaluation(run)}
                        className="mt-3 text-sm font-medium text-indigo-600 hover:text-indigo-700"
                      >
                        View {run.id} results
                      </button>
                    </article>
                  );
                })}
              </div>
            </section>
          </div>
        </>
      )}
    </div>
  );
}
