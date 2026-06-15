import type { TimingBucket } from "@/lib/api";

// Monthly posting-activity chart. `compact` = the label-less card sparkline.
export function TimingBars({ timing, compact = false }: { timing: TimingBucket[] | null; compact?: boolean }) {
  if (!timing || timing.length === 0) {
    return <p className="font-mono text-xs text-dim">no activity</p>;
  }
  const max = Math.max(1, ...timing.map((t) => t.count));

  return (
    <div className={`flex items-end gap-[3px] ${compact ? "h-8" : "h-40"}`}>
      {timing.map((t) => (
        <div key={t.month} className="flex flex-1 flex-col items-center justify-end gap-1.5">
          <div
            className="w-full rounded-[2px] bg-accent/80 transition-[height] duration-500"
            style={{ height: `${Math.max(6, (t.count / max) * 100)}%` }}
            title={`${t.month} · ${t.count} ${t.count === 1 ? "posting" : "postings"}`}
          />
          {!compact && <span className="font-mono text-[10px] text-dim">{t.month.slice(5)}</span>}
        </div>
      ))}
    </div>
  );
}
