import type { SeasonBucket } from "@/lib/api";

const MON = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

// Turn the month-of-year counts into 12 slots + the peak index.
function toCounts(season: SeasonBucket[]): { counts: number[]; peak: number; total: number } {
  const counts = Array(12).fill(0);
  let total = 0;
  for (const b of season) {
    if (b.month >= 1 && b.month <= 12) {
      counts[b.month - 1] = b.count;
      total += b.count;
    }
  }
  let peak = 0;
  for (let i = 1; i < 12; i++) if (counts[i] > counts[peak]) peak = i;
  return { counts, peak, total };
}

// expectedWindow returns a human label like "Aug–Oct · peak Sep", expanding a
// contiguous run around the peak while months stay ≥25% of the peak.
export function expectedWindow(season: SeasonBucket[]): string | null {
  const { counts, peak, total } = toCounts(season);
  if (total === 0) return null;
  // Active months = those carrying ≥30% of the peak month's volume; report the
  // span from the first to the last so bimodal patterns (e.g. Oct + Dec) read as
  // a range rather than a single month.
  const thresh = counts[peak] * 0.3;
  const active: number[] = [];
  for (let i = 0; i < 12; i++) if (counts[i] >= thresh) active.push(i);
  const lo = active[0];
  const hi = active[active.length - 1];
  return lo === hi ? MON[peak] : `${MON[lo]}–${MON[hi]} · peak ${MON[peak]}`;
}

// Seasonality chart: 12 calendar months, the peak month in full accent.
export function Seasonality({ season }: { season: SeasonBucket[] }) {
  const { counts, peak, total } = toCounts(season);

  if (total === 0) {
    return (
      <p className="font-mono text-xs text-dim">
        not enough history yet — run a backfill to reveal the seasonal pattern
      </p>
    );
  }
  const max = Math.max(...counts);

  return (
    <div>
      {/* bars are DIRECT children of a fixed-height row so % heights resolve */}
      <div className="flex h-44 items-end gap-1.5">
        {counts.map((c, i) => (
          <div
            key={i}
            className={`flex-1 rounded-t-[2px] transition-[height] duration-500 ${i === peak ? "bg-accent" : "bg-accent/30"}`}
            style={{ height: c === 0 ? "2px" : `${Math.max(8, (c / max) * 100)}%` }}
            title={`${MON[i]} · ${c} ${c === 1 ? "posting" : "postings"}`}
          />
        ))}
      </div>
      <div className="mt-2 flex gap-1.5">
        {MON.map((m, i) => (
          <span key={i} className={`flex-1 text-center font-mono text-[10px] ${i === peak ? "text-accent" : "text-dim"}`}>
            {m[0]}
          </span>
        ))}
      </div>
    </div>
  );
}
