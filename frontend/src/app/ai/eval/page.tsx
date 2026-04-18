"use client";

import { useState } from "react";
import Link from "next/link";
import { HealthGate } from "@/components/HealthGate";
import { GoAuthProvider, useGoAuth } from "@/components/go/GoAuthProvider";
import { DatasetTab } from "@/components/eval/DatasetTab";
import EvaluateTab from "@/components/eval/EvaluateTab";
import ResultsTab from "@/components/eval/ResultsTab";
import { EvaluationDetail } from "@/lib/eval-api";

type TabId = "datasets" | "evaluate" | "results";

const TABS: { id: TabId; label: string }[] = [
  { id: "datasets", label: "Datasets" },
  { id: "evaluate", label: "Evaluate" },
  { id: "results", label: "Results" },
];

function EvalPageInner() {
  const { isLoggedIn } = useGoAuth();
  const [activeTab, setActiveTab] = useState<TabId>("datasets");
  const [completedEval, setCompletedEval] = useState<EvaluationDetail | null>(
    null,
  );

  if (!isLoggedIn) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="rounded-lg border bg-card p-8 shadow-sm text-center max-w-sm w-full">
          <p className="mb-6 text-muted-foreground">
            Log in to use the evaluation tool
          </p>
          <Link
            href="/go/login?next=/ai/eval"
            className="inline-block rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 transition-colors"
          >
            Log in
          </Link>
        </div>
      </div>
    );
  }

  function handleEvalComplete(evaluation: EvaluationDetail) {
    setCompletedEval(evaluation);
    setActiveTab("results");
  }

  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto max-w-5xl px-4 py-10">
        <h1 className="text-2xl font-bold mb-1">RAG Evaluation</h1>
        <p className="text-muted-foreground mb-6">
          Measure RAG pipeline quality with golden datasets and RAGAS metrics.
        </p>

        {/* Tab bar */}
        <div className="flex flex-row border-b mb-6">
          {TABS.map((tab) => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2 text-sm font-medium transition-colors ${
                activeTab === tab.id
                  ? "border-b-2 border-indigo-500 text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Tab content */}
        {activeTab === "datasets" && <DatasetTab />}
        {activeTab === "evaluate" && (
          <EvaluateTab onComplete={handleEvalComplete} />
        )}
        {activeTab === "results" && (
          <ResultsTab selectedEvaluation={completedEval} />
        )}
      </div>
    </div>
  );
}

export default function EvalPage() {
  const evalHealthUrl =
    process.env.NEXT_PUBLIC_EVAL_API_URL || "http://localhost:8000/eval";

  return (
    <GoAuthProvider>
      <HealthGate
        endpoint={`${evalHealthUrl}/health`}
        stack="Eval Service"
        docsHref="/ai"
      >
        <EvalPageInner />
      </HealthGate>
    </GoAuthProvider>
  );
}
