import { getFirehose, API_BASE } from "@/lib/api";

export default async function FirehosePage() {
  let items;
  try {
    items = await getFirehose();
  } catch {
    return (
      <div className="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800">
        Couldn&apos;t reach the API at <code>{API_BASE}</code>. Start it with <code>make run-api</code>.
      </div>
    );
  }

  return (
    <div>
      <h1 className="text-xl font-bold">Firehose</h1>
      <p className="mb-4 text-sm text-gray-500">
        New SWE / AI-ML internships at companies outside your watchlist — the broad feed (also sent to Telegram).
      </p>
      <ul className="divide-y divide-gray-100 rounded-lg border border-gray-200 bg-white">
        {items.map((f, i) => (
          <li key={i} className="flex items-start justify-between gap-4 p-3">
            <div>
              <div className="font-medium">{f.company}</div>
              <div className="text-sm text-gray-600">
                {f.url ? (
                  <a href={f.url} target="_blank" rel="noreferrer" className="hover:underline">
                    {f.title}
                  </a>
                ) : (
                  f.title
                )}
              </div>
            </div>
            <div className="shrink-0 text-right text-xs text-gray-400">
              <div>{f.category}</div>
              <div>{new Date(f.first_seen).toLocaleDateString()}</div>
            </div>
          </li>
        ))}
        {items.length === 0 && <li className="p-3 text-sm text-gray-400">No firehose postings yet.</li>}
      </ul>
    </div>
  );
}
