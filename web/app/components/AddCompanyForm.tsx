"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { addCompany } from "@/lib/api";

export function AddCompanyForm() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [domain, setDomain] = useState("");
  const [aliases, setAliases] = useState("");
  const [priority, setPriority] = useState("mid");
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
        priority,
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
        className="rounded bg-indigo-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-indigo-700"
      >
        + Add company
      </button>
    );
  }

  return (
    <form onSubmit={submit} className="flex flex-wrap items-center gap-2 rounded-lg border bg-white p-3">
      <input
        required
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="Name (e.g. Stripe)"
        className="rounded border px-2 py-1 text-sm"
      />
      <input
        value={domain}
        onChange={(e) => setDomain(e.target.value)}
        placeholder="Domain (stripe.com)"
        className="rounded border px-2 py-1 text-sm"
      />
      <input
        value={aliases}
        onChange={(e) => setAliases(e.target.value)}
        placeholder="aliases, comma-separated"
        className="rounded border px-2 py-1 text-sm"
      />
      <select value={priority} onChange={(e) => setPriority(e.target.value)} className="rounded border px-2 py-1 text-sm">
        <option value="top">top</option>
        <option value="mid">mid</option>
        <option value="low">low</option>
      </select>
      <button disabled={busy} className="rounded bg-indigo-600 px-3 py-1 text-sm text-white hover:bg-indigo-700">
        {busy ? "Adding…" : "Add"}
      </button>
      <button type="button" onClick={() => setOpen(false)} className="px-2 py-1 text-sm text-gray-500">
        Cancel
      </button>
      {err && <span className="text-sm text-red-600">{err}</span>}
    </form>
  );
}
