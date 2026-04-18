"use client";

import { useEffect, useRef, useState } from "react";
import {
  DatasetSummary,
  EvaluationDetail,
  getEvaluation,
  listDatasets,
  startEvaluation,
} from "@/lib/eval-api";

interface EvaluateTabProps {
  onComplete: (evaluation: EvaluationDetail) => void;
}

export default function EvaluateTab({ onComplete }: EvaluateTabProps) {
  const [datasets, setDatasets] = useState<DatasetSummary[]>([]);
  const [selectedDatasetId, setSelectedDatasetId] = useState<string>("");
  const [collection, setCollection] = useState<string>("documents");
  const [running, setRunning] = useState<boolean>(false);
  const [error, setError] = useState<string>("");
  const [statusMessage, setStatusMessage] = useState<string>("");
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    listDatasets()
      .then((data) => {
        setDatasets(data);
        if (data.length > 0) {
          setSelectedDatasetId(data[0].id);
        }
      })
      .catch(() => {
        // silently ignore load failure — user can retry via the run button
      });

    return () => {
      if (pollIntervalRef.current !== null) {
        clearInterval(pollIntervalRef.current);
      }
    };
  }, []);

  async function handleRun() {
    if (!selectedDatasetId || running) return;

    setError("");
    setRunning(true);
    setStatusMessage("Starting evaluation...");

    let evalId: string;
    try {
      const result = await startEvaluation(
        selectedDatasetId,
        collection.trim() || undefined,
      );
      evalId = result.id;
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to start evaluation");
      setRunning(false);
      setStatusMessage("");
      return;
    }

    setStatusMessage("Evaluating... this may take a few minutes.");

    pollIntervalRef.current = setInterval(async () => {
      try {
        const detail = await getEvaluation(evalId);
        if (detail.status === "completed") {
          if (pollIntervalRef.current !== null) {
            clearInterval(pollIntervalRef.current);
            pollIntervalRef.current = null;
          }
          setRunning(false);
          setStatusMessage("");
          onComplete(detail);
        } else if (detail.status === "failed") {
          if (pollIntervalRef.current !== null) {
            clearInterval(pollIntervalRef.current);
            pollIntervalRef.current = null;
          }
          setRunning(false);
          setStatusMessage("");
          setError(detail.error ?? "Evaluation failed");
        }
        // status === "running": keep polling
      } catch {
        // transient error — keep polling
      }
    }, 5000);
  }

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
      <h2 className="mb-2 text-xl font-semibold text-gray-900">
        Run Evaluation
      </h2>
      <p className="mb-6 text-sm text-gray-600">
        Select a golden dataset and run the RAG evaluation pipeline. The
        evaluator queries each item against the live retrieval stack and scores
        faithfulness, answer relevancy, context precision, and context recall
        using the Ollama judge model.
      </p>

      <div className="space-y-4">
        {/* Dataset selector */}
        <div>
          <label
            htmlFor="dataset-select"
            className="mb-1 block text-sm font-medium text-gray-700"
          >
            Dataset
          </label>
          <select
            id="dataset-select"
            value={selectedDatasetId}
            onChange={(e) => setSelectedDatasetId(e.target.value)}
            disabled={running}
            className="block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 disabled:cursor-not-allowed disabled:bg-gray-50 disabled:text-gray-400"
          >
            {datasets.length === 0 && (
              <option value="">No datasets available</option>
            )}
            {datasets.map((ds) => (
              <option key={ds.id} value={ds.id}>
                {ds.name} ({ds.item_count} items)
              </option>
            ))}
          </select>
        </div>

        {/* Collection input */}
        <div>
          <label
            htmlFor="collection-input"
            className="mb-1 block text-sm font-medium text-gray-700"
          >
            Collection{" "}
            <span className="font-normal text-gray-400">(optional)</span>
          </label>
          <input
            id="collection-input"
            type="text"
            value={collection}
            onChange={(e) => setCollection(e.target.value)}
            placeholder="documents"
            disabled={running}
            className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm placeholder:text-gray-400 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 disabled:cursor-not-allowed disabled:bg-gray-50 disabled:text-gray-400"
          />
        </div>

        {/* Error */}
        {error && (
          <p className="text-sm text-red-600">{error}</p>
        )}

        {/* Status */}
        {running && statusMessage && (
          <div className="flex items-center gap-2 text-sm text-gray-600">
            <svg
              className="h-4 w-4 animate-spin"
              viewBox="0 0 24 24"
              fill="none"
            >
              <circle
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                strokeWidth="4"
                className="opacity-25"
              />
              <path
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                className="opacity-75"
              />
            </svg>
            <span>{statusMessage}</span>
          </div>
        )}

        {/* Run button */}
        <button
          onClick={handleRun}
          disabled={running || !selectedDatasetId}
          className="inline-flex items-center rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white shadow-sm hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
        >
          Run Evaluation
        </button>
      </div>
    </div>
  );
}
