-- Fit-score feature (LLM, free-tier Gemini): match a resume against a JD.
-- See CLAUDE.md "LLM roadmap" + the fit-score design notes.

-- Uploaded resumes (PDF -> extracted text). Multiple per user; pick one to score.
-- content_hash keys the fit-score cache so editing/re-uploading invalidates it.
CREATE TABLE resumes (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id      BIGINT NOT NULL REFERENCES users(id),
  filename     TEXT NOT NULL,
  content_text TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX resumes_user_idx ON resumes (user_id, created_at DESC);

-- Fit-score cache: one Gemini call per unique (jd, resume) pair, EVER (mirrors the
-- resolution-cache "cache forever" invariant — the LLM is never re-hit for a combo
-- we've already scored). posting_id is set when the JD came from a watchlist
-- posting; NULL for a pasted JD. result is the validated JSON the model returned.
CREATE TABLE fit_scores (
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  jd_hash     TEXT NOT NULL,
  resume_hash TEXT NOT NULL,
  posting_id  BIGINT REFERENCES postings(id),
  model       TEXT NOT NULL,
  result      JSONB NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (jd_hash, resume_hash)
);

-- Plain-text JD extracted from an ATS posting's content/description at process
-- time, so the fit-score JD picker can offer watchlist roles without re-fetching.
-- Only ATS sources (Greenhouse/Ashby) carry JD text; SimplifyJobs rows stay NULL.
ALTER TABLE postings ADD COLUMN jd_text TEXT;
