"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { addCompany } from "@/lib/api";

const inputClass =
  "rounded-md border border-line bg-raised px-2.5 py-1.5 font-mono text-sm text-ink placeholder:text-dim focus:border-accent focus:outline-none";

export function AddCompanyForm() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [domain, setDomain] = useState("");
  const [aliases, setAliases] = useState("");
  const [tier, setTier] = useState("A");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      await addCompany({
        name: name.trim(),
        domain: domain.trim(),
        priority: tier,
        aliases: aliases.split(",").map((s) => s.trim()).filter(Boolean),
      });
      setName("");
      setDomain("");
      setAliases("");
      setOpen(false);
      router.refresh();
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  }

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="glow-hover rounded-md border border-line px-3 py-1.5 font-mono text-sm text-muted hover:text-accent"
      >
        + track company
      </button>
    );
  }

  return (
    <form onSubmit={submit} className="panel flex flex-wrap items-center gap-2 rounded-xl p-3">
      <input required value={name} onChange={(e) => setName(e.target.value)} placeholder="name" className={inputClass} />
      <input value={domain} onChange={(e) => setDomain(e.target.value)} placeholder="domain.com" className={inputClass} />
      <input value={aliases} onChange={(e) => setAliases(e.target.value)} placeholder="aliases, comma-sep" className={inputClass} />
      <select value={tier} onChange={(e) => setTier(e.target.value)} className={inputClass} title="tier">
        <option value="S">S tier</option>
        <option value="A">A tier</option>
        <option value="B">B tier</option>
        <option value="C">C tier</option>
      </select>
      <button
        disabled={busy}
        className="rounded-md bg-accent px-3 py-1.5 font-mono text-sm font-medium text-bg transition-opacity hover:opacity-90 disabled:opacity-50"
      >
        {busy ? "…" : "track"}
      </button>
      <button type="button" onClick={() => setOpen(false)} className="px-2 py-1.5 font-mono text-sm text-dim hover:text-muted">
        esc
      </button>
      {err && <span className="font-mono text-sm text-red-400">{err}</span>}
    </form>
  );
}
