import Link from "next/link";
import { getCompanies, getTimeline, getSeasonality } from "@/lib/api";
import { Logo } from "@/app/components/Logo";
import { Seasonality, expectedWindow } from "@/app/components/Seasonality";

function fmtDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" });
}

export default async function CompanyPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const cid = Number(id);

  const [companies, timeline, season] = await Promise.all([getCompanies(), getTimeline(cid), getSeasonality(cid)]);
  const company = companies.find((c) => c.id === cid);
  if (!company) {
    return <p className="font-mono text-sm text-dim">// company not found</p>;
  }
  const window = expectedWindow(season);

  return (
    <div className="space-y-8">
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

      {/* Hero: the flagship answer — when does this company open roles? */}
      <section className="panel rounded-xl p-6">
        <SectionLabel>expected to open</SectionLabel>
        <p className="mt-3 font-mono text-3xl font-bold text-accent">{window ?? "—"}</p>
        <p className="mb-6 mt-1 font-mono text-[11px] text-dim">
          {window
            ? `based on ${company.name}'s posting history (each bar = a calendar month, all years combined)`
            : "needs historical data"}
        </p>
        <Seasonality season={season} />
      </section>

      {/* One clear, dated list of roles seen (replaces the old ambiguous split). */}
      <section className="panel rounded-xl p-5">
        <SectionLabel>recent roles · {timeline.length}</SectionLabel>
        <ul className="mt-4 space-y-2.5">
          {timeline.slice(0, 30).map((e, i) => (
            <li key={i} className="flex items-baseline justify-between gap-3 border-b border-line pb-2.5 text-sm last:border-0">
              <span className="min-w-0">
                {e.data?.url ? (
                  <a href={e.data.url} target="_blank" rel="noreferrer" className="text-ink transition-colors hover:text-accent">
                    {e.data.title}
                  </a>
                ) : (
                  <span className="text-ink">{e.data?.title ?? "—"}</span>
                )}
                {e.data?.locations && e.data.locations.length > 0 && (
                  <span className="font-mono text-xs text-dim"> · {e.data.locations.join(", ")}</span>
                )}
              </span>
              <span className="shrink-0 font-mono text-[11px] text-dim">
                {e.type === "jd_changed" && <span className="mr-1.5 text-accent/70">updated</span>}
                {fmtDate(e.event_time)}
              </span>
            </li>
          ))}
          {timeline.length === 0 && <li className="font-mono text-sm text-dim">// no roles recorded yet</li>}
        </ul>
      </section>
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
