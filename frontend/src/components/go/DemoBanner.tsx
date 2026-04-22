"use client";

export function DemoBanner() {
  return (
    <div className="mb-6 rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-4 py-3">
      <p className="text-sm font-medium text-yellow-700 dark:text-yellow-400">
        Portfolio Demo
      </p>
      <p className="mt-1 text-sm text-yellow-600/80 dark:text-yellow-400/70">
        This panel demonstrates DLQ operational tooling for the checkout saga. In
        a production environment, this would be implemented as a CLI tool with
        role-based access control.
      </p>
    </div>
  );
}
