export type FailureClass =
  | "expected"
  | "permanent"
  | "transient"
  | "unexpected";

export interface FailureRow {
  failure: string;
  classification: FailureClass;
  action: string;
}

export interface FailurePolicyTableProps {
  rows: readonly FailureRow[];
}

const CLASS_STYLE: Record<FailureClass, string> = {
  expected: "border-foreground/20 bg-foreground/5 text-muted-foreground",
  permanent: "border-red-500/40 bg-red-500/10 text-red-300",
  transient: "border-amber-500/40 bg-amber-500/10 text-amber-300",
  unexpected: "border-purple-500/40 bg-purple-500/10 text-purple-300",
};

export function FailurePolicyTable({ rows }: FailurePolicyTableProps) {
  return (
    <div className="overflow-x-auto rounded-lg border border-foreground/10">
      <table className="w-full text-left text-sm" data-testid="failure-policy-table">
        <thead className="bg-muted/30">
          <tr className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            <th scope="col" className="px-4 py-3">
              Failure
            </th>
            <th scope="col" className="px-4 py-3">
              Class
            </th>
            <th scope="col" className="px-4 py-3">
              Action
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-foreground/10">
          {rows.map((row) => (
            <tr key={row.failure} className="align-top">
              <td className="px-4 py-3 text-muted-foreground">{row.failure}</td>
              <td className="px-4 py-3">
                <span
                  className={`inline-block rounded-full border px-2 py-0.5 text-xs font-medium ${CLASS_STYLE[row.classification]}`}
                >
                  {row.classification}
                </span>
              </td>
              <td className="px-4 py-3 text-muted-foreground">{row.action}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
