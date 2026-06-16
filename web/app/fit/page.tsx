"use client";

import { useEffect, useRef, useState } from "react";
import {
  type FitJD,
  type FitResult,
  type Resume,
  deleteResume,
  getFitJDs,
  getFitStatus,
  getResumes,
  scoreFit,
  uploadResume,
} from "@/lib/api";

const TIER_ORDER = ["S", "A", "B", "C"];

export default function FitPage() {
  const [resumes, setResumes] = useState<Resume[]>([]);
  const [resumeId, setResumeId] = useState<number | null>(null);
  const [jds, setJDs] = useState<FitJD[]>([]);
  const [postingId, setPostingId] = useState<number | null>(null);
  const [pastedJD, setPastedJD] = useState("");
  const [configured, setConfigured] = useState(true);
  const [result, setResult] = useState<FitResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    getResumes().then((r) => {
      setResumes(r);
      if (r[0]) setResumeId(r[0].id);
    });
    getFitJDs().then(setJDs);
    getFitStatus().then((s) => setConfigured(s.configured)).catch(() => {});
  }, []);

  async function onUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setError("");
    setBusy(true);
    try {
      const r = await uploadResume(file);
      setResumes((prev) => [r, ...prev]);
      setResumeId(r.id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "upload failed");
    } finally {
      setBusy(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  async function onDeleteResume(id: number) {
    await deleteResume(id);
    setResumes((prev) => prev.filter((r) => r.id !== id));
    if (resumeId === id) setResumeId(null);
  }

  async function onScore() {
    if (!resumeId) {
      setError("pick or upload a resume first");
      return;
    }
    const usePosting = postingId != null;
    if (!usePosting && pastedJD.trim().length < 30) {
      setError("paste a job description, or pick a watchlist role");
      return;
    }
    setError("");
    setResult(null);
    setBusy(true);
    try {
      const res = await scoreFit({
        resume_id: resumeId,
        ...(usePosting ? { posting_id: postingId! } : { jd_text: pastedJD }),
      });
      setResult(res);
    } catch (err) {
      setError(err instanceof Error ? err.message : "scoring failed");
    } finally {
      setBusy(false);
    }
  }

  const tiered = [...jds].sort(
    (a, b) => TIER_ORDER.indexOf(a.tier || "C") - TIER_ORDER.indexOf(b.tier || "C") || a.company.localeCompare(b.company),
  );

  return (
    <div className="space-y-8">
      <div>
        <h1 className="font-mono text-xl font-bold tracking-tight text-ink">fit score</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted">
          Score how well a resume matches a job description — a rubric-based 0–100 with matched/missing
          skills and concrete resume tips. Pick a resume, then paste a JD or choose a watchlist role.
        </p>
      </div>

      {!configured && (
        <div className="panel rounded-xl border border-accent/30 p-4 font-mono text-xs text-muted">
          <span className="text-accent">⚠ not configured</span> — add{" "}
          <span className="text-ink">GEMINI_API_KEY</span> to <span className="text-ink">.env</span> and restart the
          api to enable scoring. You can still upload resumes and browse JDs.
        </div>
      )}

      {/* 1. Resume */}
      <section className="space-y-3">
        <SectionLabel>1 · resume</SectionLabel>
        <div className="flex flex-wrap gap-2">
          {resumes.map((r) => (
            <button
              key={r.id}
              onClick={() => setResumeId(r.id)}
              className={`group flex items-center gap-2 rounded-lg border px-3 py-2 font-mono text-xs transition-colors ${
                resumeId === r.id ? "border-accent bg-accent/10 text-accent" : "border-line text-muted hover:text-ink"
              }`}
            >
              {r.filename}
              <span
                onClick={(e) => {
                  e.stopPropagation();
                  onDeleteResume(r.id);
                }}
                className="text-dim opacity-0 transition group-hover:opacity-100 hover:text-accent"
              >
                ✕
              </span>
            </button>
          ))}
          <label className="cursor-pointer rounded-lg border border-dashed border-line px-3 py-2 font-mono text-xs text-dim transition-colors hover:text-accent">
            + upload PDF
            <input ref={fileRef} type="file" accept="application/pdf" onChange={onUpload} className="hidden" />
          </label>
        </div>
      </section>

      {/* 2. Job description: paste */}
      <section className="space-y-3">
        <SectionLabel>2 · job description</SectionLabel>
        <textarea
          value={pastedJD}
          onChange={(e) => {
            setPastedJD(e.target.value);
            if (e.target.value) setPostingId(null);
          }}
          rows={5}
          placeholder="Paste any job description here (works for every role, including firehose)…"
          className="w-full resize-y rounded-xl border border-line bg-surface px-3 py-2 text-sm text-ink placeholder:text-dim focus:border-accent focus:outline-none"
        />

        {/* …or pick a watchlist role (ATS postings with stored JD text, tiered) */}
        <div className="font-mono text-[11px] uppercase tracking-widest text-dim">or pick a watchlist role</div>
        {tiered.length === 0 ? (
          <p className="font-mono text-xs text-dim">
            // no watchlist roles with stored JD text yet (only Greenhouse/Ashby roles carry it). Paste a JD above.
          </p>
        ) : (
          <div className="grid gap-1.5 sm:grid-cols-2">
            {tiered.map((j) => (
              <button
                key={j.posting_id}
                onClick={() => {
                  setPostingId(j.posting_id);
                  setPastedJD("");
                }}
                className={`flex items-center gap-2 overflow-hidden rounded-lg border px-3 py-2 text-left text-sm transition-colors ${
                  postingId === j.posting_id ? "border-accent bg-accent/10" : "border-line hover:bg-overlay"
                }`}
              >
                <span className="shrink-0 rounded border border-line px-1.5 py-0.5 font-mono text-[10px] text-accent">
                  {(j.tier || "—").toUpperCase()}
                </span>
                <span className="min-w-0 flex-1 truncate">
                  <span className="text-ink">{j.company}</span>
                  <span className="text-muted"> · {j.title}</span>
                </span>
              </button>
            ))}
          </div>
        )}
      </section>

      <button
        onClick={onScore}
        disabled={busy || !configured}
        className="rounded-lg bg-accent px-5 py-2.5 font-mono text-sm font-medium text-bg transition hover:opacity-90 disabled:opacity-40"
      >
        {busy ? "scoring…" : "score fit"}
      </button>
      {error && <p className="font-mono text-xs text-accent">{error}</p>}

      {result && <FitReport result={result} />}
    </div>
  );
}

function FitReport({ result }: { result: FitResult }) {
  const c = result.component_scores;
  const components: [string, number, number][] = [
    ["technical skills", c.technical_skills, 40],
    ["relevant experience", c.relevant_experience, 25],
    ["impact & depth", c.impact_depth, 15],
    ["eligibility & level", c.eligibility_level, 10],
    ["ATS keywords", c.ats_keywords, 10],
  ];
  return (
    <section className="panel rise space-y-6 rounded-xl p-6">
      <div className="flex items-baseline gap-4">
        <div className="font-mono text-5xl font-bold text-accent">{result.overall_score}</div>
        <div>
          <div className="font-mono text-sm font-medium text-ink">{result.verdict}</div>
          <div className="font-mono text-[11px] text-dim">/ 100</div>
        </div>
      </div>

      <p className="max-w-2xl text-sm text-muted">{result.summary}</p>

      <div className="space-y-2">
        {components.map(([label, got, max]) => (
          <div key={label} className="flex items-center gap-3">
            <div className="w-36 shrink-0 font-mono text-[11px] text-dim">{label}</div>
            <div className="h-2 flex-1 overflow-hidden rounded-full bg-overlay">
              <div className="h-full rounded-full bg-accent" style={{ width: `${(got / max) * 100}%` }} />
            </div>
            <div className="w-12 shrink-0 text-right font-mono text-[11px] text-muted">
              {got}/{max}
            </div>
          </div>
        ))}
      </div>

      <div className="grid gap-6 sm:grid-cols-2">
        <div>
          <SectionLabel>matched</SectionLabel>
          <ul className="mt-2 space-y-1.5">
            {result.matched_skills.length === 0 && <li className="font-mono text-xs text-dim">// none</li>}
            {result.matched_skills.map((m, i) => (
              <li key={i} className="break-words text-sm">
                <span className="font-medium text-ink">{m.skill}</span>
                {m.evidence && <span className="text-dim"> — {snippet(m.evidence)}</span>}
              </li>
            ))}
          </ul>
        </div>
        <div>
          <SectionLabel>missing</SectionLabel>
          <ul className="mt-2 space-y-1.5">
            {result.missing_skills.length === 0 && <li className="font-mono text-xs text-dim">// none</li>}
            {result.missing_skills.map((m, i) => (
              <li key={i} className="break-words text-sm">
                <span className="font-medium text-ink">{m.skill}</span>
                <span className="text-dim"> ({m.importance})</span>
              </li>
            ))}
          </ul>
        </div>
      </div>

      {result.ats_keyword_gaps?.length > 0 && (
        <div>
          <SectionLabel>ATS keyword gaps</SectionLabel>
          <div className="mt-2 flex flex-wrap gap-1.5">
            {result.ats_keyword_gaps.map((k) => (
              <span key={k} className="rounded border border-line px-2 py-0.5 font-mono text-[11px] text-muted">
                {k}
              </span>
            ))}
          </div>
        </div>
      )}

      {result.suggestions?.length > 0 && (
        <div>
          <SectionLabel>suggestions</SectionLabel>
          <ul className="mt-2 space-y-1.5">
            {result.suggestions.map((sug, i) => (
              <li key={i} className="flex gap-2 text-sm text-muted">
                <span className="text-accent">→</span>
                {sug}
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

// snippet trims an evidence quote to the relevant chunk (≤6 words, ellipsized) so
// matched rows read cleanly instead of dumping the whole quoted line.
function snippet(s: string, max = 6): string {
  const words = s.trim().split(/\s+/);
  if (words.length <= max) return s.trim();
  return "…" + words.slice(0, max).join(" ") + "…";
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <h2 className="font-mono text-[11px] uppercase tracking-widest text-accent">{children}</h2>;
}
