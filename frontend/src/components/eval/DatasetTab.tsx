"use client";

import { useEffect, useState } from "react";
import {
  createDataset,
  listDatasets,
  DatasetSummary,
  GoldenItem,
} from "@/lib/eval-api";

const EXAMPLE_ITEMS = JSON.stringify(
  [
    {
      query: "What is chunking in document processing?",
      expected_answer:
        "Chunking is the process of splitting documents into smaller pieces for embedding and retrieval.",
      expected_sources: ["ingestion.pdf"],
    },
  ],
  null,
  2,
);

export function DatasetTab() {
  const [datasets, setDatasets] = useState<DatasetSummary[]>([]);
  const [name, setName] = useState("");
  const [itemsJson, setItemsJson] = useState(EXAMPLE_ITEMS);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  useEffect(() => {
    listDatasets()
      .then(setDatasets)
      .catch(() => {
        // silently ignore load errors — user can still create datasets
      });
  }, []);

  async function handleCreate() {
    setError(null);

    let items: GoldenItem[];
    try {
      items = JSON.parse(itemsJson);
      if (!Array.isArray(items)) throw new Error("Expected a JSON array");
    } catch (e) {
      setError(
        e instanceof Error ? `Invalid JSON: ${e.message}` : "Invalid JSON",
      );
      return;
    }

    if (!name.trim()) {
      setError("Dataset name is required");
      return;
    }

    setCreating(true);
    try {
      await createDataset(name.trim(), items);
      setName("");
      setItemsJson(EXAMPLE_ITEMS);
      const updated = await listDatasets();
      setDatasets(updated);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to create dataset");
    } finally {
      setCreating(false);
    }
  }

  function toggleExpand(id: string) {
    setExpandedId((prev) => (prev === id ? null : id));
  }

  return (
    <div className="space-y-8">
      {/* Create form */}
      <div className="rounded-lg border border-border p-4 space-y-4">
        <div>
          <h2 className="text-lg font-semibold">Create a Golden Dataset</h2>
          <p className="text-sm text-muted-foreground mt-1">
            A golden dataset is a curated set of queries with known expected
            answers and sources. It serves as the ground truth for evaluating
            RAG pipeline quality — run evaluations against it to measure
            faithfulness, answer relevancy, and retrieval precision.
          </p>
        </div>

        <div className="space-y-3">
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Dataset name (e.g., rag-baseline-v1)"
            className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-indigo-500"
          />

          <textarea
            value={itemsJson}
            onChange={(e) => setItemsJson(e.target.value)}
            rows={8}
            className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm font-mono placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-indigo-500 resize-y"
          />

          {error && <p className="text-sm text-red-400">{error}</p>}

          <button
            onClick={handleCreate}
            disabled={creating}
            className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {creating ? "Creating…" : "Create Dataset"}
          </button>
        </div>
      </div>

      {/* Dataset list */}
      <div className="space-y-3">
        <h2 className="text-lg font-semibold">Existing Datasets</h2>

        {datasets.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No datasets yet. Create one above to get started.
          </p>
        ) : (
          <div className="space-y-2">
            {datasets.map((ds) => (
              <div
                key={ds.id}
                className="rounded-lg border border-border p-4 cursor-pointer hover:border-indigo-500 transition-colors"
                onClick={() => toggleExpand(ds.id)}
              >
                <div className="flex items-center justify-between">
                  <div>
                    <span className="font-medium">{ds.name}</span>
                    <span className="ml-2 text-xs text-muted-foreground">
                      {ds.item_count} item{ds.item_count !== 1 ? "s" : ""}
                    </span>
                  </div>
                  <span className="text-xs text-muted-foreground">
                    {new Date(ds.created_at).toLocaleDateString()}
                  </span>
                </div>

                {expandedId === ds.id && (
                  <div className="mt-3 pt-3 border-t border-border text-sm text-muted-foreground">
                    <span className="font-mono text-xs">ID: {ds.id}</span>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
