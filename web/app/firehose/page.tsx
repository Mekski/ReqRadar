import { getFirehose, API_BASE } from "@/lib/api";

export default async function FirehosePage() {
  let items;
  try {
    items = await getFirehose();
  } catch {
    return (
      <div className="panel rounded-xl p-4 font-mono text-sm text-accent">
        <span className="text-dim">// </span>no connection to API at <span className="text-ink">{API_BASE}</span>
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6">
        <h1 className="font-mono text-xl font-bold tracking-tight text-ink">firehose</h1>
        <p className="mt-1 font-mono text-xs text-dim">new SWE / AI-ML internships beyond your watchlist</p>
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
                <div>{new Date(f.first_seen).toLocaleDateString()}</div>
              </div>
            </li>
          ))}
          {items.length === 0 && <li className="px-4 py-3 font-mono text-sm text-dim">// no postings yet</li>}
        </ul>
      </div>
    </div>
  );
}
