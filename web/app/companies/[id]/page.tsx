import Link from "next/link";
import { getCompanies, getTimeline, getPostings } from "@/lib/api";
import { Logo } from "@/app/components/Logo";
import { TimingBars } from "@/app/components/TimingBars";

export default async function CompanyPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const cid = Number(id);

  const [companies, timeline, postings] = await Promise.all([getCompanies(), getTimeline(cid), getPostings()]);
  const company = companies.find((c) => c.id === cid);
  if (!company) {
    return <p className="font-mono text-sm text-dim">// company not found</p>;
  }
  const myPostings = postings.filter((p) => p.company === company.name).slice(0, 25);

  return (
    <div className="space-y-9">
      <Link href="/" className="font-mono text-xs text-muted transition-colors hover:text-accent">
        ← watchlist
      </Link>

      <div className="flex items-center gap-4">
        <Logo domain={company.domain} name={company.name} size={52} />
        <div>
          <h1 className="font-mono text-2xl font-bold tracking-tight text-ink">{company.name}</h1>
          <p className="font-mono text-xs text-dim">{company.domain || "—"}</p>
        </div>
        {company.priority && (
          <span className="ml-auto rounded-md border border-line px-2.5 py-1 font-mono text-xs text-accent">
            tier {company.priority.toUpperCase()}
          </span>
        )}
      </div>

      <section className="panel rounded-xl p-5">
        <SectionLabel>posting activity</SectionLabel>
        <p className="mb-5 mt-2 font-mono text-[11px] text-dim">
          new postings detected per month — when {company.name} historically opens roles
        </p>
        <TimingBars timing={company.timing} />
      </section>

      <div className="grid gap-5 lg:grid-cols-2">
        <section className="panel rounded-xl p-5">
          <SectionLabel>open roles · {myPostings.length}</SectionLabel>
          <ul className="mt-4 space-y-2.5">
            {myPostings.map((p) => (
              <li key={p.id} className="border-b border-line pb-2.5 text-sm last:border-0">
                {p.url ? (
                  <a href={p.url} target="_blank" rel="noreferrer" className="text-ink transition-colors hover:text-accent">
                    {p.title}
                  </a>
                ) : (
                  <span className="text-ink">{p.title}</span>
                )}
                {p.locations?.length > 0 && <span className="font-mono text-xs text-dim"> · {p.locations.join(", ")}</span>}
              </li>
            ))}
            {myPostings.length === 0 && <li className="font-mono text-sm text-dim">// none tracked</li>}
          </ul>
        </section>

        <section className="panel rounded-xl p-5">
          <SectionLabel>recent updates</SectionLabel>
          <ul className="mt-4 space-y-2.5 font-mono text-xs">
            {timeline.slice(0, 20).map((e, i) => (
              <li key={i} className="flex items-baseline justify-between gap-3 border-b border-line pb-2.5 last:border-0">
                <span className="min-w-0">
                  <Badge type={e.type} /> <span className="text-muted">{e.data?.title ?? ""}</span>
                </span>
                <span className="shrink-0 text-dim">{new Date(e.event_time).toLocaleDateString()}</span>
              </li>
            ))}
            {timeline.length === 0 && <li className="text-dim">// no activity</li>}
          </ul>
        </section>
      </div>
    </div>
  );
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div>
      <h2 className="font-mono text-[11px] uppercase tracking-widest text-accent">{children}</h2>
      <span className="accent-bar mt-2 block" />
    </div>
  );
}

function Badge({ type }: { type: string }) {
  const styles: Record<string, string> = {
    posting_opened: "border-accent/40 text-accent",
    jd_changed: "border-line-strong text-muted",
  };
  const label = type === "posting_opened" ? "new" : type === "jd_changed" ? "edit" : type;
  return (
    <span className={`mr-1.5 rounded border px-1 py-px text-[10px] uppercase ${styles[type] ?? "border-line text-dim"}`}>
      {label}
    </span>
  );
}
