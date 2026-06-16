// Small presentational bits shared across pages (previously copy-pasted).
import type { ReactNode } from "react";
import { API_BASE } from "@/lib/api";

// SectionLabel is the uppercase mono section heading. `bar` adds the accent
// underline used on the company-detail page (the home/fit pages omit it).
export function SectionLabel({ children, bar = false }: { children: ReactNode; bar?: boolean }) {
  const h = <h2 className="font-mono text-[11px] uppercase tracking-widest text-accent">{children}</h2>;
  if (!bar) return h;
  return (
    <div>
      {h}
      <span className="accent-bar mt-2 block" />
    </div>
  );
}

// ApiDown is the "couldn't reach the API" panel shown when a fetch fails.
export function ApiDown() {
  return (
    <div className="panel rounded-xl p-4 font-mono text-sm text-accent">
      <span className="text-dim">// </span>no connection to API at <span className="text-ink">{API_BASE}</span> — run{" "}
      <span className="text-ink">make run-api</span>
    </div>
  );
}
