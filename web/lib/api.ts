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
  pay_min: number; // posted pay of the latest SWE-category intern (0 when unknown)
  pay_max: number;
  pay_period: string; // "hourly" | "annual" | "monthly"; "" when no SWE-intern pay known
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
  event_time: string | null; // the job's posting date (null for legacy rows)
  first_seen: string;
};

// ---- Fit score ----

export type Resume = { id: number; filename: string; created_at: string };
export type FitJD = { posting_id: number; company: string; title: string; tier: string; source: string };

export type FitResult = {
  overall_score: number;
  verdict: string;
  component_scores: {
    technical_skills: number;
    relevant_experience: number;
    impact_depth: number;
    eligibility_level: number;
    ats_keywords: number;
  };
  summary: string;
  matched_skills: { skill: string; evidence: string }[];
  missing_skills: { skill: string; importance: string }[];
  ats_keyword_gaps: string[];
  suggestions: string[];
};

// ---- Sentiment (grounded LLM report) ----

export type SentimentSource = { title: string; uri: string };
export type SentimentReport = {
  report: string; // markdown
  sources: SentimentSource[];
  model: string;
  generated_at: string;
};

export const getSentiment = (id: number) =>
  get<{ configured: boolean; report: SentimentReport | null }>(`/api/companies/${id}/sentiment`);

export async function generateSentiment(id: number): Promise<SentimentReport> {
  const res = await fetch(`${API_BASE}/api/companies/${id}/sentiment`, { method: "POST" });
  if (!res.ok) throw new Error((await res.text()) || `generate → ${res.status}`);
  return res.json() as Promise<SentimentReport>;
}

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

export const getResumes = () => arr(get<Resume[] | null>("/api/resumes"));
export const getFitJDs = () => arr(get<FitJD[] | null>("/api/fit/jds"));
export const getFitStatus = () => get<{ configured: boolean }>("/api/fit/status");

export async function uploadResume(file: File): Promise<Resume> {
  // Extract text in the browser (pdf.js handles LaTeX spacing); send text, not the PDF.
  const { extractPdfText } = await import("./pdf");
  const text = await extractPdfText(file);
  if (text.trim().length < 50) {
    throw new Error("couldn't read text from this PDF — is it a scan/image? upload a text-based PDF");
  }
  const res = await fetch(`${API_BASE}/api/resumes`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ filename: file.name, text }),
  });
  if (!res.ok) throw new Error((await res.text()) || `upload → ${res.status}`);
  return res.json() as Promise<Resume>;
}

export async function deleteResume(id: number): Promise<void> {
  const res = await fetch(`${API_BASE}/api/resumes/${id}`, { method: "DELETE" });
  if (!res.ok) throw new Error(`delete resume → ${res.status}`);
}

export async function scoreFit(input: {
  resume_id: number;
  posting_id?: number;
  jd_text?: string;
}): Promise<FitResult> {
  const res = await fetch(`${API_BASE}/api/fit`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  if (!res.ok) throw new Error((await res.text()) || `score → ${res.status}`);
  return res.json() as Promise<FitResult>;
}

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
