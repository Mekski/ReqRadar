"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { type Company, removeCompany } from "@/lib/api";
import { Logo } from "./Logo";
import { TimingBars } from "./TimingBars";

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

function lastActive(timing: Company["timing"]): string {
  if (!timing || timing.length === 0) return "—";
  for (let i = timing.length - 1; i >= 0; i--) {
    if (timing[i].count > 0) {
      const [y, m] = timing[i].month.split("-");
      return `${MONTHS[Number(m) - 1]} '${y.slice(2)}`;
    }
  }
  return "—";
}

export function CompanyCard({ company, index = 0 }: { company: Company; index?: number }) {
  const router = useRouter();
  const [removing, setRemoving] = useState(false);

  async function onRemove(e: React.MouseEvent) {
    e.preventDefault();
    if (!confirm(`Remove ${company.name} from the watchlist?`)) return;
    setRemoving(true);
    try {
      await removeCompany(company.id);
      router.refresh();
    } catch {
      setRemoving(false);
    }
  }

  return (
    <div className="rise group relative" style={{ ["--i" as string]: index } as React.CSSProperties}>
      <button
        onClick={onRemove}
        disabled={removing}
        title="Remove from watchlist"
        className="absolute right-3 top-3 z-10 font-mono text-xs text-dim opacity-0 transition hover:text-accent group-hover:opacity-100"
      >
        ✕
      </button>
      <Link href={`/companies/${company.id}`} className="panel glow-hover block rounded-xl p-4">
        <div className="flex items-center gap-3">
          <Logo domain={company.domain} name={company.name} />
          <div className="min-w-0 truncate font-medium text-ink">{company.name}</div>
        </div>

        <div className="mt-4 flex items-baseline justify-between font-mono">
          <div>
            <span className="text-2xl font-medium text-ink">{company.open_postings}</span>{" "}
            <span className="text-xs text-muted">roles</span>
          </div>
          <div className="text-[11px] text-dim">last active {lastActive(company.timing)}</div>
        </div>

        <div className="mt-3">
          <TimingBars timing={company.timing} compact />
        </div>
      </Link>
    </div>
  );
}
