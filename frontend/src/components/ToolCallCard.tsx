"use client";

import { useState } from "react";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
} from "@/components/ui/card";

interface ToolCallCardProps {
  step: number;
  tool: string;
  args: Record<string, unknown>;
  result?: unknown;
  truncated?: boolean;
}

function argsSummary(args: Record<string, unknown>): string {
  const entries = Object.entries(args);
  if (entries.length === 0) return "(no args)";
  return entries
    .slice(0, 2)
    .map(([k, v]) => {
      const val = typeof v === "string" ? v.slice(0, 40) : JSON.stringify(v).slice(0, 40);
      return `${k}: ${val}`;
    })
    .join(", ");
}

export function ToolCallCard({
  step,
  tool,
  args,
  result,
  truncated,
}: ToolCallCardProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <Card
      className="cursor-pointer border-foreground/10"
      onClick={() => setExpanded((prev) => !prev)}
    >
      <CardHeader className="border-b border-foreground/10 py-2">
        <div className="flex items-center justify-between gap-2">
          <CardTitle className="flex items-center gap-2 text-xs font-mono">
            <span className="text-muted-foreground">#{step}</span>
            <span className="text-primary">{tool}</span>
          </CardTitle>
          <span className="text-xs text-muted-foreground">
            {expanded ? "▲" : "▼"}
          </span>
        </div>
        {!expanded && (
          <p className="mt-1 truncate text-xs text-muted-foreground font-mono">
            {argsSummary(args)}
          </p>
        )}
      </CardHeader>

      {expanded && (
        <CardContent className="pt-3">
          <div className="flex flex-col gap-3">
            <div>
              <p className="mb-1 text-xs font-medium text-muted-foreground uppercase tracking-wide">
                Args
              </p>
              <pre className="overflow-x-auto rounded bg-muted p-2 text-xs font-mono whitespace-pre-wrap break-all">
                {JSON.stringify(args, null, 2)}
              </pre>
            </div>
            {result !== undefined && (
              <div>
                <p className="mb-1 text-xs font-medium text-muted-foreground uppercase tracking-wide">
                  Result{truncated ? " (truncated)" : ""}
                </p>
                <div className="max-h-48 overflow-y-auto rounded bg-muted p-2">
                  <pre className="text-xs font-mono whitespace-pre-wrap break-all">
                    {typeof result === "string"
                      ? result
                      : JSON.stringify(result, null, 2)}
                  </pre>
                </div>
              </div>
            )}
          </div>
        </CardContent>
      )}
    </Card>
  );
}
