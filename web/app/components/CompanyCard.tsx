"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { type Company, removeCompany } from "@/lib/api";
import { Logo } from "./Logo";
import { TimingBars } from "./TimingBars";

const priorityStyle: Record<string, string> = {
  top: "bg-green-100 text-green-700",
  mid: "bg-blue-100 text-blue-700",
  low: "bg-gray-100 text-gray-500",
};

export function CompanyCard({ company }: { company: Company }) {
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
    <div className="relative rounded-lg border border-gray-200 bg-white p-4 shadow-sm transition hover:shadow">
      <button
        onClick={onRemove}
        disabled={removing}
        title="Remove from watchlist"
        className="absolute right-2 top-2 z-10 text-lg leading-none text-gray-300 hover:text-red-500"
      >
        ×
      </button>
      <Link href={`/companies/${company.id}`} className="block">
        <div className="flex items-center gap-3">
          <Logo domain={company.domain} name={company.name} />
          <div>
            <div className="font-semibold">{company.name}</div>
            <span
              className={`inline-block rounded px-1.5 py-0.5 text-[10px] uppercase ${
                priorityStyle[company.priority] ?? "bg-gray-100 text-gray-500"
              }`}
            >
              {company.priority || "—"}
            </span>
          </div>
        </div>
        <div className="mt-3 flex gap-4 text-sm text-gray-600">
          <span>
            <b className="text-gray-900">{company.open_postings}</b> tracked
          </span>
          <span>
            <b className="text-gray-900">{company.total_events}</b> events
          </span>
        </div>
        <div className="mt-3">
          <TimingBars timing={company.timing} compact />
        </div>
      </Link>
    </div>
  );
}
