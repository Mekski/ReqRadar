import { getFirehose } from "@/lib/api";
import { ApiDown } from "@/app/components/ui";

export default async function FirehosePage() {
  let items;
  try {
    items = await getFirehose();
  } catch {
    return <ApiDown />;
  }

  return (
    <div>
      <div className="mb-6">
        <h1 className="font-mono text-xl font-bold tracking-tight text-ink">firehose</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted">
          An early-warning feed of newly-posted SWE / AI-ML internships at companies{" "}
          <span className="text-ink">not</span> on your watchlist — so a great role somewhere unexpected
          doesn&apos;t slip by. Watchlist companies get richer tracking + alerts on the{" "}
          <span className="text-accent">watchlist</span> tab; this is everything else, newest first.
        </p>
      </div>

      <div className="panel overflow-hidden rounded-xl">
        <ul className="divide-y divide-line">
          {items.map((f, i) => (
            <li
              key={i}
              className="rise flex items-start justify-between gap-4 px-4 py-3 transition-colors hover:bg-overlay"
              style={{ ["--i" as string]: Math.min(i, 18) } as React.CSSProperties}
            >
              <div className="min-w-0">
                <div className="font-medium text-ink">{f.company}</div>
                <div className="truncate text-sm text-muted">
                  {f.url ? (
                    <a href={f.url} target="_blank" rel="noreferrer" className="transition-colors hover:text-accent">
                      {f.title}
                    </a>
                  ) : (
                    f.title
                  )}
                </div>
              </div>
              <div className="shrink-0 text-right font-mono text-[11px] text-dim">
                <div className="text-accent/70">{f.category}</div>
                <div>{f.event_time ? `posted ${new Date(f.event_time).toLocaleDateString()}` : "—"}</div>
              </div>
            </li>
          ))}
          {items.length === 0 && <li className="px-4 py-3 font-mono text-sm text-dim">// no postings yet</li>}
        </ul>
      </div>
    </div>
  );
}
