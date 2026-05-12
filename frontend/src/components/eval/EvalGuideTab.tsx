"use client";

export type EvalGuideTarget = "datasets" | "evaluate" | "results" | "dashboard";

type GuideStep = {
  title: string;
  body: string;
  actionLabel: string;
  target: EvalGuideTarget;
};

const GUIDE_STEPS: GuideStep[] = [
  {
    title: "Create or select a golden dataset",
    body:
      "Start with 8-15 stable, high-signal questions. Each item should include a realistic query, expected answer, and expected sources.",
    actionLabel: "Create dataset",
    target: "datasets",
  },
  {
    title: "Run a baseline",
    body:
      "Use the same collection you plan to improve, usually documents. Leave Baseline run empty and use notes such as Baseline before rerank comparison.",
    actionLabel: "Run baseline",
    target: "evaluate",
  },
  {
    title: "Inspect low-scoring queries",
    body:
      "Review aggregate scores first, then expand weak queries and classify failures as retrieval misses, weak answers, unsupported claims, or dataset issues.",
    actionLabel: "Review results",
    target: "results",
  },
  {
    title: "Run one candidate change",
    body:
      "Change one RAG variable at a time, then run the same dataset and collection. Select the completed baseline and write notes that name the exact change.",
    actionLabel: "Run candidate",
    target: "evaluate",
  },
  {
    title: "Compare and decide",
    body:
      "Use dashboard deltas as directional evidence, then inspect per-query results before deciding whether to keep, adjust, or revert the change.",
    actionLabel: "Compare runs",
    target: "dashboard",
  },
];

interface EvalGuideTabProps {
  onSelectTab: (tab: EvalGuideTarget) => void;
}

export function EvalGuideTab({ onSelectTab }: EvalGuideTabProps) {
  return (
    <div className="space-y-6">
      <section className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h2 className="text-xl font-semibold text-gray-900">
          RAG Evaluation Workflow
        </h2>
        <p className="mt-2 max-w-3xl text-sm text-gray-600">
          Use this page to build a measurement history: stable golden dataset,
          baseline run, candidate run, score comparison, per-query review, and
          a written decision. Treat aggregate scores as signals, not proof, and
          always inspect the queries that moved.
        </p>
      </section>

      <section
        aria-label="Evaluation workflow steps"
        className="grid gap-4 md:grid-cols-2"
      >
        {GUIDE_STEPS.map((step, index) => (
          <article
            key={step.title}
            className="rounded-lg border border-gray-200 bg-white p-5 shadow-sm"
          >
            <div className="flex items-start gap-3">
              <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-indigo-50 text-sm font-semibold text-indigo-700">
                {index + 1}
              </span>
              <div className="min-w-0">
                <h3 className="text-base font-semibold text-gray-900">
                  {step.title}
                </h3>
                <p className="mt-2 text-sm leading-6 text-gray-600">
                  {step.body}
                </p>
                <button
                  type="button"
                  onClick={() => onSelectTab(step.target)}
                  className="mt-4 rounded-md border border-indigo-200 px-3 py-2 text-sm font-medium text-indigo-700 transition-colors hover:border-indigo-300 hover:bg-indigo-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
                >
                  {step.actionLabel}
                </button>
              </div>
            </div>
          </article>
        ))}
      </section>

      <section className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="text-lg font-semibold text-gray-900">
          Decision checklist
        </h3>
        <div className="mt-4 grid gap-3 text-sm text-gray-700 md:grid-cols-3">
          <p className="rounded-md border border-gray-100 p-3">
            Keep changes when targeted metrics improve and important queries do
            not regress.
          </p>
          <p className="rounded-md border border-gray-100 p-3">
            Adjust changes when aggregate gains are narrow, noisy, or conflict
            with per-query evidence.
          </p>
          <p className="rounded-md border border-gray-100 p-3">
            Fix dataset issues before treating a bad score as a RAG pipeline
            failure.
          </p>
        </div>
      </section>
    </div>
  );
}
