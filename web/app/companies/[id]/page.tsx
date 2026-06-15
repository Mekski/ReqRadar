import Link from "next/link";
import { getCompanies, getTimeline, getPostings } from "@/lib/api";
import { Logo } from "@/app/components/Logo";
import { TimingBars } from "@/app/components/TimingBars";

export default async function CompanyPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const cid = Number(id);

  const [companies, timeline, postings] = await Promise.all([
    getCompanies(),
    getTimeline(cid),
    getPostings(),
  ]);
  const company = companies.find((c) => c.id === cid);
  if (!company) {
    return <p className="text-gray-500">Company not found.</p>;
  }
  const myPostings = postings.filter((p) => p.company === company.name).slice(0, 25);

  return (
    <div className="space-y-6">
      <Link href="/" className="text-sm text-indigo-600 hover:underline">
        ← Watchlist
      </Link>

      <div className="flex items-center gap-4">
        <Logo domain={company.domain} name={company.name} size={56} />
        <div>
          <h1 className="text-2xl font-bold">{company.name}</h1>
          <p className="text-sm text-gray-500">{company.domain}</p>
        </div>
      </div>

      <section className="rounded-lg border border-gray-200 bg-white p-5">
        <h2 className="mb-1 font-semibold">When they post</h2>
        <p className="mb-4 text-xs text-gray-400">
          Monthly count of new postings detected (last 12 months) — the historical timing pattern.
        </p>
        <TimingBars timing={company.timing} />
      </section>

      <div className="grid gap-6 lg:grid-cols-2">
        <section className="rounded-lg border border-gray-200 bg-white p-5">
          <h2 className="mb-3 font-semibold">Tracked postings ({myPostings.length})</h2>
          <ul className="space-y-2 text-sm">
            {myPostings.map((p) => (
              <li key={p.id} className="border-b border-gray-100 pb-2 last:border-0">
                {p.url ? (
                  <a href={p.url} target="_blank" rel="noreferrer" className="font-medium text-indigo-600 hover:underline">
                    {p.title}
                  </a>
                ) : (
                  <span className="font-medium">{p.title}</span>
                )}
                {p.locations?.length > 0 && (
                  <span className="text-gray-400"> · {p.locations.join(", ")}</span>
                )}
              </li>
            ))}
            {myPostings.length === 0 && <li className="text-gray-400">No tracked postings.</li>}
          </ul>
        </section>

        <section className="rounded-lg border border-gray-200 bg-white p-5">
          <h2 className="mb-3 font-semibold">Recent activity</h2>
          <ul className="space-y-2 text-sm">
            {timeline.slice(0, 20).map((e, i) => (
              <li key={i} className="flex items-baseline justify-between gap-3 border-b border-gray-100 pb-2 last:border-0">
                <span>
                  <Badge type={e.type} /> {e.data?.title ?? ""}
                </span>
                <span className="shrink-0 text-xs text-gray-400">
                  {new Date(e.event_time).toLocaleDateString()}
                </span>
              </li>
            ))}
            {timeline.length === 0 && <li className="text-gray-400">No activity yet.</li>}
          </ul>
        </section>
      </div>
    </div>
  );
}

function Badge({ type }: { type: string }) {
  const styles: Record<string, string> = {
    posting_opened: "bg-green-100 text-green-700",
    jd_changed: "bg-amber-100 text-amber-700",
  };
  const label = type === "posting_opened" ? "opened" : type === "jd_changed" ? "changed" : type;
  return (
    <span className={`mr-1 rounded px-1.5 py-0.5 text-[10px] ${styles[type] ?? "bg-gray-100 text-gray-600"}`}>
      {label}
    </span>
  );
}
