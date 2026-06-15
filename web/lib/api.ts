// Typed client for the ReqRadar Go API. Types mirror the JSON in
// internal/store/api.go and internal/signal.
export const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export type TimingBucket = { month: string; count: number };
export type SeasonBucket = { month: number; count: number }; // month 1–12

export type Company = {
  id: number;
  name: string;
  domain: string;
  priority: string;
  open_postings: number;
  total_events: number;
  timing: TimingBucket[] | null; // Go marshals an empty slice as null
  expected_open: string; // data-derived SWE peak month, e.g. "Aug" ("" if too few samples)
  expected_estimate: string; // curated fallback month when data is sparse ("" if none)
};

export type TimelineEvent = {
  type: string;
  event_time: string;
  data: { company?: string; title?: string; url?: string; locations?: string[] };
};

export type OpenPosting = {
  id: number;
  company: string;
  title: string;
  url: string;
  locations: string[];
  first_seen: string;
};

export type FirehosePosting = {
  company: string;
  title: string;
  url: string;
  category: string;
  first_seen: string;
};

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, { cache: "no-store" });
  if (!res.ok) throw new Error(`${path} → ${res.status}`);
  return res.json() as Promise<T>;
}

// Go marshals empty slices as null, so coerce list responses to [].
const arr = <T>(p: Promise<T[] | null>) => p.then((x) => x ?? []);

export const getCompanies = () => arr(get<Company[] | null>("/api/companies"));
export const getTimeline = (id: number) => arr(get<TimelineEvent[] | null>(`/api/companies/${id}/timeline`));
export const getSeasonality = (id: number, category = "swe") =>
  arr(get<SeasonBucket[] | null>(`/api/companies/${id}/seasonality?category=${category}`));
export const getPostings = () => arr(get<OpenPosting[] | null>("/api/postings"));
export const getFirehose = () => arr(get<FirehosePosting[] | null>("/api/firehose"));

export async function addCompany(input: {
  name: string;
  domain: string;
  priority: string;
  aliases: string[];
}): Promise<void> {
  const res = await fetch(`${API_BASE}/api/companies`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) throw new Error(`add company → ${res.status}`);
}

export async function removeCompany(id: number): Promise<void> {
  const res = await fetch(`${API_BASE}/api/companies/${id}`, { method: "DELETE" });
  if (!res.ok) throw new Error(`remove company → ${res.status}`);
}

export async function updateTier(id: number, priority: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/companies/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ priority }),
  });
  if (!res.ok) throw new Error(`update tier → ${res.status}`);
}
