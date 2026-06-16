-- On-demand, grounded-search sentiment report per company (LLM, free-tier Gemini
-- "Grounding with Google Search"). One row per company: regenerating REPLACES the
-- old report (UNIQUE entity_id + UPSERT), so we always store exactly the latest.
-- NOT auto-refreshed — only generated when the user clicks the button on the
-- company view, so we never spend a grounded call on a company Mark doesn't care about.
CREATE TABLE company_sentiment (
  entity_id    BIGINT PRIMARY KEY REFERENCES entities(id),
  report       TEXT NOT NULL,          -- structured markdown the model returned
  sources      JSONB NOT NULL,         -- real citation URIs from groundingMetadata (not model-authored)
  model        TEXT NOT NULL,
  generated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
