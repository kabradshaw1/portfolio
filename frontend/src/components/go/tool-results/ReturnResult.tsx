export function ReturnResult({
  ret,
}: {
  ret: { id: string; order_id: string; status: string; reason: string };
}) {
  return (
    <div>
      <div className="flex items-center gap-2">
        <span className="text-sm" aria-hidden>
          ↩️
        </span>
        <span className="text-xs font-semibold">Return Initiated</span>
      </div>
      <div className="mt-1 space-y-1 text-[10px] text-muted-foreground">
        <div>Return ID: {ret.id.slice(0, 8)}</div>
        <div>Order: #{ret.order_id.slice(0, 8)}</div>
        <div>
          Status: <span className="font-semibold text-foreground">{ret.status}</span>
        </div>
        <div>Reason: {ret.reason}</div>
      </div>
    </div>
  );
}
