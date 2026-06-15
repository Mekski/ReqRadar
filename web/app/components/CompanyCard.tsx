"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { type Company, removeCompany, updateTier } from "@/lib/api";
import { Logo } from "./Logo";

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

  async function onTier(e: React.ChangeEvent<HTMLSelectElement>) {
    await updateTier(company.id, e.target.value);
    router.refresh();
  }

  const tier = (company.priority || "A").toUpperCase();

  return (
    <div className="rise group relative" style={{ ["--i" as string]: index } as React.CSSProperties}>
      <div className="absolute right-2.5 top-2.5 z-10 flex items-center gap-1.5">
        <select
          value={tier}
          onChange={onTier}
          onClick={(e) => e.stopPropagation()}
          title="Change tier"
          className="cursor-pointer rounded border border-line bg-surface px-1 py-0.5 font-mono text-[10px] text-muted hover:text-accent focus:outline-none"
        >
          {["S", "A", "B", "C"].map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
        <button
          onClick={onRemove}
          disabled={removing}
          title="Remove from watchlist"
          className="font-mono text-xs text-dim opacity-0 transition hover:text-accent group-hover:opacity-100"
        >
          ✕
        </button>
      </div>

      <Link href={`/companies/${company.id}`} className="panel glow-hover block rounded-xl p-4">
        <div className="flex items-center gap-3 pr-14">
          <Logo domain={company.domain} name={company.name} />
          <div className="min-w-0 truncate font-medium text-ink">{company.name}</div>
        </div>

        <div className="mt-4 flex items-end justify-between">
          <div>
            <div className="font-mono text-[10px] uppercase tracking-widest text-dim">expected</div>
            <div className="font-mono text-lg font-medium text-accent">{company.expected_open || "—"}</div>
          </div>
          <div className="text-right">
            <div className="font-mono text-[10px] uppercase tracking-widest text-dim">pay</div>
            <div className="font-mono text-lg font-medium text-muted">???</div>
          </div>
        </div>
      </Link>
    </div>
  );
}
