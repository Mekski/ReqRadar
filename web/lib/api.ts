// Typed client for the ReqRadar Go API. Types mirror the JSON in
// internal/store/api.go and internal/signal.
export const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export type TimingBucket = { month: string; count: number };

export type Company = {
  id: number;
  name: string;
  domain: string;
  priority: string;
  open_postings: number;
  total_events: number;
  timing: TimingBucket[] | null; // Go marshals an empty slice as null
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

export const getCompanies = () => get<Company[]>("/api/companies");
export const getTimeline = (id: number) => get<TimelineEvent[]>(`/api/companies/${id}/timeline`);
export const getPostings = () => get<OpenPosting[]>("/api/postings");
export const getFirehose = () => get<FirehosePosting[]>("/api/firehose");

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
