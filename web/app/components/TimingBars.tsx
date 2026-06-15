import type { TimingBucket } from "@/lib/api";

// Hand-rolled CSS bar chart of monthly posting-open counts — the flagship
// "when do they open apps" view. `compact` renders a small label-less sparkline
// for cards; otherwise it labels the months.
export function TimingBars({
  timing,
  compact = false,
}: {
  timing: TimingBucket[];
  compact?: boolean;
}) {
  if (!timing || timing.length === 0) {
    return <p className="text-xs text-gray-400">No timing data yet.</p>;
  }
  const max = Math.max(1, ...timing.map((t) => t.count));

  return (
    <div className={`flex items-end gap-1 ${compact ? "h-10" : "h-40"}`}>
      {timing.map((t) => (
        <div key={t.month} className="flex flex-1 flex-col items-center justify-end gap-1">
          <div
            className="w-full rounded-t bg-indigo-500"
            style={{ height: `${Math.max(4, (t.count / max) * 100)}%` }}
            title={`${t.month}: ${t.count}`}
          />
          {!compact && <span className="text-[10px] text-gray-400">{t.month.slice(5)}</span>}
        </div>
      ))}
    </div>
  );
}
