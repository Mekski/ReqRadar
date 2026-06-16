"use client";

import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { type SentimentReport, generateSentiment, getSentiment } from "@/lib/api";

export function SentimentCard({ companyId }: { companyId: number }) {
  const [report, setReport] = useState<SentimentReport | null>(null);
  const [configured, setConfigured] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    getSentiment(companyId)
      .then((d) => {
        setConfigured(d.configured);
        setReport(d.report);
      })
      .catch(() => {});
  }, [companyId]);

  async function onGenerate() {
    setBusy(true);
    setError("");
    try {
      setReport(await generateSentiment(companyId));
    } catch (e) {
      setError(e instanceof Error ? e.message : "generation failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel rounded-xl p-6">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="font-mono text-[11px] uppercase tracking-widest text-accent">community sentiment</h2>
          <span className="accent-bar mt-2 block" />
        </div>
        <button
          onClick={onGenerate}
          disabled={busy || !configured}
          className="shrink-0 rounded-lg border border-line px-3 py-1.5 font-mono text-xs text-muted transition hover:border-accent hover:text-accent disabled:opacity-40"
        >
          {busy ? "researching…" : report ? "regenerate" : "generate"}
        </button>
      </div>

      {!configured && (
        <p className="mt-4 font-mono text-xs text-dim">
          // add <span className="text-ink">GEMINI_API_KEY</span> to .env and restart the api to enable this
        </p>
      )}

      {error && <p className="mt-4 font-mono text-xs text-accent">{error}</p>}

      {busy && (
        <p className="mt-4 font-mono text-xs text-dim">
          // searching the web (Reddit, HN, blogs) and synthesizing — this takes ~10–20s…
        </p>
      )}

      {!report && !busy && configured && (
        <p className="mt-4 text-sm text-muted">
          What does the developer community say about interning here — prestige, culture, the interview
          process (OA, rounds), intern pay &amp; housing? Click <span className="text-accent">generate</span> to
          research it live. Grounded in public sources; says so honestly when something can&apos;t be found.
        </p>
      )}

      {report && (
        <div className="mt-5 space-y-4">
          <div className="markdown space-y-3 text-sm text-muted">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{report.report}</ReactMarkdown>
          </div>

          {report.sources?.length > 0 && (
            <details className="border-t border-line pt-3">
              <summary className="cursor-pointer font-mono text-[11px] uppercase tracking-widest text-dim hover:text-accent">
                sources ({report.sources.length})
              </summary>
              <ul className="mt-2 space-y-1">
                {report.sources.map((s, i) => (
                  <li key={i} className="truncate text-xs">
                    <a href={s.uri} target="_blank" rel="noreferrer" className="text-dim hover:text-accent">
                      {s.title || s.uri}
                    </a>
                  </li>
                ))}
              </ul>
            </details>
          )}

          <p className="font-mono text-[10px] text-dim">
            generated {new Date(report.generated_at).toLocaleString()} · {report.model} · grounded web search
          </p>
        </div>
      )}
    </section>
  );
}
