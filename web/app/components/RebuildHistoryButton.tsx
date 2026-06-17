"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { runBackfill, getBackfillStatus, type BackfillStatus } from "@/lib/api";

function ago(iso: string | null): string {
  if (!iso) return "";
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

// RebuildHistoryButton triggers an on-demand history backfill (replay 3yr of
// SimplifyJobs git snapshots) so a company you just added gets its timing filled.
// It kicks off async and polls status until done, then refreshes the page.
export function RebuildHistoryButton() {
  const router = useRouter();
  const [status, setStatus] = useState<BackfillStatus | null>(null);
  const [err, setErr] = useState("");
  const poll = useRef<ReturnType<typeof setInterval> | null>(null);

  function startPolling() {
    if (poll.current) return;
    poll.current = setInterval(async () => {
      try {
        const s = await getBackfillStatus();
        setStatus(s);
        if (!s.running) {
          clearInterval(poll.current!);
          poll.current = null;
          router.refresh(); // pick up newly-filled history
        }
      } catch {
        /* transient — keep polling */
      }
    }, 3000);
  }

  useEffect(() => {
    getBackfillStatus()
      .then((s) => {
        setStatus(s);
        if (s.running) startPolling();
      })
      .catch(() => {});
    return () => {
      if (poll.current) clearInterval(poll.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function onClick() {
    setErr("");
    try {
      await runBackfill();
      setStatus((s) => ({ emitted: s?.emitted ?? 0, last_run_at: s?.last_run_at ?? null, running: true }));
      startPolling();
    } catch (e) {
      setErr(String(e));
    }
  }

  const running = status?.running ?? false;
  return (
    <div className="flex items-center gap-2">
      <button
        onClick={onClick}
        disabled={running}
        title="Replay ~3 years of SimplifyJobs history to fill timing for newly-added companies (~30s)"
        className="glow-hover rounded-md border border-line px-3 py-1.5 font-mono text-sm text-muted hover:text-accent disabled:opacity-50"
      >
        {running ? "rebuilding history…" : "⟳ rebuild history"}
      </button>
      {!running && status?.last_run_at && (
        <span className="font-mono text-[11px] text-dim">history updated {ago(status.last_run_at)}</span>
      )}
      {err && <span className="font-mono text-[11px] text-red-400">{err}</span>}
    </div>
  );
}
