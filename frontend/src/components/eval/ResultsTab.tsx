"use client";

import { useEffect, useState } from "react";
import {
  EvaluationDetail,
  EvaluationSummary,
  QueryScore,
  getEvaluation,
  listEvaluations,
} from "@/lib/eval-api";
import { RadialGauge } from "@/components/eval/RadialGauge";

interface ResultsTabProps {
  selectedEvaluation: EvaluationDetail | null;
}

function averageScore(scores: QueryScore): number | null {
  const values = [
    scores.faithfulness,
    scores.answer_relevancy,
    scores.context_precision,
    scores.context_recall,
  ].filter((v): v is number => v !== null);
  if (values.length === 0) return null;
  return values.reduce((sum, v) => sum + v, 0) / values.length;
}

function scoreColorClass(value: number | null): string {
  if (value === null) return "text-gray-400";
  if (value >= 0.7) return "text-green-600";
  if (value >= 0.4) return "text-yellow-600";
  return "text-red-600";
}

export default function ResultsTab({ selectedEvaluation }: ResultsTabProps) {
  const [evaluations, setEvaluations] = useState<EvaluationSummary[]>([]);
  const [selectedId, setSelectedId] = useState<string>("");
  const [detail, setDetail] = useState<EvaluationDetail | null>(null);
  const [expandedQuery, setExpandedQuery] = useState<number | null>(null);

  // On mount: load list, and if a freshly-completed evaluation is passed in, use it
  useEffect(() => {
    listEvaluations()
      .then((list) => {
        setEvaluations(list);
        if (selectedEvaluation) {
          // Merge in case it's not in the list yet
          const inList = list.some((e) => e.id === selectedEvaluation.id);
          if (!inList) {
            setEvaluations([selectedEvaluation, ...list]);
          }
          setSelectedId(selectedEvaluation.id);
          setDetail(selectedEvaluation);
        } else if (list.length > 0) {
          setSelectedId(list[0].id);
        }
      })
      .catch(() => {
        // silently ignore load failure
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // When selectedEvaluation prop changes (new run completed), switch to it
  useEffect(() => {
    if (selectedEvaluation) {
      setEvaluations((prev) => {
        const inList = prev.some((e) => e.id === selectedEvaluation.id);
        return inList ? prev : [selectedEvaluation, ...prev];
      });
      setSelectedId(selectedEvaluation.id);
      setDetail(selectedEvaluation);
      setExpandedQuery(null);
    }
  }, [selectedEvaluation]);

  // When selectedId changes, load the full detail (unless we already have it from the prop)
  useEffect(() => {
    if (!selectedId) return;
    if (selectedEvaluation && selectedEvaluation.id === selectedId) {
      setDetail(selectedEvaluation);
      return;
    }
    getEvaluation(selectedId)
      .then((d) => {
        setDetail(d);
        setExpandedQuery(null);
      })
      .catch(() => {
        // silently ignore — stale detail stays visible
      });
  }, [selectedId]); // eslint-disable-line react-hooks/exhaustive-deps

  function handleSelectChange(e: React.ChangeEvent<HTMLSelectElement>) {
    setSelectedId(e.target.value);
    setExpandedQuery(null);
  }

  function toggleQuery(index: number) {
    setExpandedQuery((prev) => (prev === index ? null : index));
  }

  return (
    <div className="space-y-6">
      {/* Evaluation selector */}
      <div>
        <label
          htmlFor="eval-select"
          className="mb-1 block text-sm font-medium text-gray-700"
        >
          Evaluation
        </label>
        <select
          id="eval-select"
          value={selectedId}
          onChange={handleSelectChange}
          className="block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
        >
          {evaluations.length === 0 && (
            <option value="">No evaluations yet</option>
          )}
          {evaluations.map((ev) => (
            <option key={ev.id} value={ev.id}>
              {new Date(ev.created_at).toLocaleString()} — {ev.status}
            </option>
          ))}
        </select>
      </div>

      {/* Empty state */}
      {!detail && evaluations.length === 0 && (
        <p className="text-sm text-gray-500">
          No evaluation results yet. Go to the Evaluate tab to run one.
        </p>
      )}

      {/* Detail states */}
      {detail && (
        <div className="space-y-6">
          {/* Failed state */}
          {detail.status === "failed" && (
            <div className="rounded-lg border border-red-300 bg-red-50 p-4">
              <p className="text-sm font-medium text-red-700">
                Evaluation failed
              </p>
              {detail.error && (
                <p className="mt-1 text-sm text-red-600">{detail.error}</p>
              )}
            </div>
          )}

          {/* Running state */}
          {detail.status === "running" && (
            <p className="text-sm text-gray-600">
              This evaluation is still running.
            </p>
          )}

          {/* Aggregate scorecard */}
          {detail.status === "completed" && detail.aggregate_scores && (
            <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
              <h3 className="mb-4 text-lg font-semibold text-gray-900">
                Aggregate Scores
              </h3>
              <div className="flex flex-wrap justify-center gap-8">
                <RadialGauge
                  value={detail.aggregate_scores.faithfulness}
                  label="Faithfulness"
                />
                <RadialGauge
                  value={detail.aggregate_scores.answer_relevancy}
                  label="Relevancy"
                />
                <RadialGauge
                  value={detail.aggregate_scores.context_precision}
                  label="Precision"
                />
                <RadialGauge
                  value={detail.aggregate_scores.context_recall}
                  label="Recall"
                />
              </div>
            </div>
          )}

          {/* Per-query breakdown */}
          {detail.status === "completed" && detail.results && detail.results.length > 0 && (
            <div>
              <h3 className="mb-3 text-lg font-semibold text-gray-900">
                Per-Query Breakdown
              </h3>
              <div className="space-y-2">
                {detail.results.map((result, index) => {
                  const avg = averageScore(result.scores);
                  const isExpanded = expandedQuery === index;

                  return (
                    <div
                      key={index}
                      className="rounded-lg border border-gray-200 bg-white shadow-sm"
                    >
                      {/* Row header */}
                      <button
                        onClick={() => toggleQuery(index)}
                        className="flex w-full items-center justify-between px-4 py-3 text-left"
                      >
                        <span className="truncate pr-4 text-sm text-gray-800">
                          {result.query}
                        </span>
                        <span
                          className={`shrink-0 text-sm font-semibold ${scoreColorClass(avg)}`}
                        >
                          {avg !== null ? avg.toFixed(2) : "N/A"}
                        </span>
                      </button>

                      {/* Expanded view */}
                      {isExpanded && (
                        <div className="border-t border-gray-100 px-4 pb-4 pt-3 space-y-4">
                          {/* Answer */}
                          <div>
                            <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-gray-500">
                              Answer
                            </p>
                            <p className="text-sm text-gray-800">
                              {result.answer}
                            </p>
                          </div>

                          {/* Retrieved Contexts */}
                          {result.contexts.length > 0 && (
                            <div>
                              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">
                                Retrieved Contexts
                              </p>
                              <ul className="space-y-2">
                                {result.contexts.map((ctx, ci) => (
                                  <li
                                    key={ci}
                                    className="rounded-md bg-muted/30 px-3 py-2 text-sm text-gray-700"
                                  >
                                    {ctx}
                                  </li>
                                ))}
                              </ul>
                            </div>
                          )}

                          {/* Scores */}
                          <div>
                            <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500">
                              Scores
                            </p>
                            <div className="grid grid-cols-2 gap-2">
                              {(
                                [
                                  ["Faithfulness", result.scores.faithfulness],
                                  ["Relevancy", result.scores.answer_relevancy],
                                  ["Precision", result.scores.context_precision],
                                  ["Recall", result.scores.context_recall],
                                ] as [string, number | null][]
                              ).map(([label, val]) => (
                                <div
                                  key={label}
                                  className="flex items-center justify-between rounded-md border border-gray-100 px-3 py-2"
                                >
                                  <span className="text-xs text-gray-600">
                                    {label}
                                  </span>
                                  <span
                                    className={`text-sm font-semibold ${scoreColorClass(val)}`}
                                  >
                                    {val !== null ? val.toFixed(2) : "N/A"}
                                  </span>
                                </div>
                              ))}
                            </div>
                          </div>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
