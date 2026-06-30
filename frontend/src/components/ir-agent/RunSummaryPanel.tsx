import { Coins, Gauge, Repeat, Wrench } from "lucide-react";
import type { ReactNode } from "react";
import type { Role, RunSummary } from "@/lib/ir-agent/types";

const ROLE_ORDER: Role[] = ["triage", "investigate", "validate", "report"];

function Stat({
  icon,
  label,
  value,
  sub,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  sub?: string;
}) {
  return (
    <div className="rounded-xl border border-foreground/10 bg-card p-4">
      <div className="flex items-center gap-2 text-muted-foreground">
        {icon}
        <span className="text-xs font-medium uppercase tracking-wide">{label}</span>
      </div>
      <div className="mt-2 text-2xl font-bold text-foreground">{value}</div>
      {sub && <div className="mt-1 text-xs text-muted-foreground">{sub}</div>}
    </div>
  );
}

/** Renders the trailing `summary` event as a measured-outcomes panel. */
export function RunSummaryPanel({ summary }: { summary: RunSummary }) {
  const { totals, comparison } = summary;
  return (
    <div>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat
          icon={<Coins size={15} />}
          label="Run cost"
          value={`$${totals.cost_usd.toFixed(4)}`}
          sub={`vs $${comparison.opus_everywhere_cost_usd.toFixed(4)} all-Opus`}
        />
        <Stat
          icon={<Gauge size={15} />}
          label="Savings"
          value={`${comparison.savings_factor.toFixed(2)}×`}
          sub="cheaper than Opus-everywhere"
        />
        <Stat
          icon={<Repeat size={15} />}
          label="Latency"
          value={`${totals.seconds.toFixed(1)}s`}
          sub={`${totals.total_tokens.toLocaleString()} tokens`}
        />
        <Stat
          icon={<Wrench size={15} />}
          label="Tool calls"
          value={`${summary.tool_calls}`}
          sub={`${summary.investigate_attempts} investigate pass${
            summary.investigate_attempts === 1 ? "" : "es"
          }`}
        />
      </div>

      <div className="mt-4 overflow-x-auto rounded-xl border border-foreground/10">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
            <tr>
              <th className="px-4 py-2 font-medium">Role</th>
              <th className="px-4 py-2 font-medium">Model</th>
              <th className="px-4 py-2 text-right font-medium">In</th>
              <th className="px-4 py-2 text-right font-medium">Out</th>
              <th className="px-4 py-2 text-right font-medium">Cost</th>
              <th className="px-4 py-2 text-right font-medium">Time</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-foreground/10">
            {ROLE_ORDER.filter((role) => summary.per_role[role]).map((role) => {
              const r = summary.per_role[role]!;
              return (
                <tr key={role}>
                  <td className="px-4 py-2 font-medium capitalize text-foreground">{role}</td>
                  <td className="px-4 py-2">
                    <code className="font-mono text-xs text-muted-foreground">{r.model}</code>
                  </td>
                  <td className="px-4 py-2 text-right tabular-nums text-muted-foreground">
                    {r.input_tokens.toLocaleString()}
                  </td>
                  <td className="px-4 py-2 text-right tabular-nums text-muted-foreground">
                    {r.output_tokens.toLocaleString()}
                  </td>
                  <td className="px-4 py-2 text-right tabular-nums text-foreground">
                    ${r.cost_usd.toFixed(4)}
                  </td>
                  <td className="px-4 py-2 text-right tabular-nums text-muted-foreground">
                    {r.seconds.toFixed(1)}s
                  </td>
                </tr>
              );
            })}
          </tbody>
          <tfoot className="border-t border-foreground/15 bg-muted/30 font-medium text-foreground">
            <tr>
              <td className="px-4 py-2" colSpan={2}>
                Total
              </td>
              <td className="px-4 py-2 text-right tabular-nums">
                {totals.input_tokens.toLocaleString()}
              </td>
              <td className="px-4 py-2 text-right tabular-nums">
                {totals.output_tokens.toLocaleString()}
              </td>
              <td className="px-4 py-2 text-right tabular-nums">${totals.cost_usd.toFixed(4)}</td>
              <td className="px-4 py-2 text-right tabular-nums">{totals.seconds.toFixed(1)}s</td>
            </tr>
          </tfoot>
        </table>
      </div>
    </div>
  );
}
