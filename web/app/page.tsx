import { getCompanies, type Company } from "@/lib/api";
import { CompanyCard } from "@/app/components/CompanyCard";
import { AddCompanyForm } from "@/app/components/AddCompanyForm";
import { RebuildHistoryButton } from "@/app/components/RebuildHistoryButton";
import { ApiDown } from "@/app/components/ui";

const TIER_ORDER = ["S", "A", "B", "C"];

export default async function Home() {
  let companies: Company[];
  try {
    companies = await getCompanies();
  } catch {
    return <ApiDown />;
  }

  // group by tier (S → A → B → C, unknowns last)
  const groups = new Map<string, Company[]>();
  for (const c of companies) {
    const t = (c.priority || "?").toUpperCase();
    (groups.get(t) ?? groups.set(t, []).get(t)!).push(c);
  }
  const tiers = [...groups.keys()].sort((a, b) => {
    const ia = TIER_ORDER.indexOf(a);
    const ib = TIER_ORDER.indexOf(b);
    return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib);
  });

  let idx = 0;
  return (
    <div className="space-y-10">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-mono text-xl font-bold tracking-tight text-ink">watchlist</h1>
          <p className="mt-1 font-mono text-xs text-dim">{companies.length} companies tracked</p>
        </div>
        <div className="flex flex-col items-end gap-2">
          <AddCompanyForm />
          <RebuildHistoryButton />
        </div>
      </div>

      {tiers.map((t) => {
        const list = groups.get(t)!;
        return (
          <section key={t}>
            <div className="mb-4 flex items-center gap-3">
              <span className="font-mono text-2xl font-bold leading-none text-accent">{t}</span>
              <span className="font-mono text-[11px] uppercase tracking-widest text-dim">
                {list.length} {list.length === 1 ? "company" : "companies"}
              </span>
              <span className="h-px flex-1 bg-gradient-to-r from-line-strong to-transparent" />
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {list.map((c) => (
                <CompanyCard key={c.id} company={c} index={idx++} />
              ))}
            </div>
          </section>
        );
      })}
    </div>
  );
}

