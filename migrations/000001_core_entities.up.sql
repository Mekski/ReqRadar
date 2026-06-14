-- Core registry + user/watchlist + source config. Non-partitioned, low-volume,
-- forever-retained. See DESIGN.md §4, §6.

-- Person-aware from day one (kind) so v2 recruiter discovery is additive,
-- even though v1 only stores companies.
CREATE TYPE entity_kind AS ENUM ('company', 'person');

CREATE TABLE entities (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind           entity_kind NOT NULL,
    canonical_name TEXT        NOT NULL,
    domain         TEXT,                          -- strong entity-resolution key
    metadata       JSONB       NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kind, canonical_name)
);

CREATE TABLE entity_aliases (
    entity_id  BIGINT NOT NULL REFERENCES entities (id) ON DELETE CASCADE,
    alias      TEXT   NOT NULL,                   -- normalized: lower, stripped suffixes
    source     TEXT   NOT NULL,                   -- 'seed' | 'llm' | 'manual'
    confidence REAL   NOT NULL DEFAULT 1.0,
    PRIMARY KEY (entity_id, alias)
);
-- An alias resolves to exactly one entity (the resolution cascade depends on this).
CREATE UNIQUE INDEX entity_aliases_alias_key ON entity_aliases (alias);

-- Append-only audit log of every resolution outcome, including "not a watchlist
-- entity" (entity_id NULL). Each unique raw string costs at most one LLM call ever.
CREATE TABLE resolution_decisions (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    raw_text   TEXT        NOT NULL,
    entity_id  BIGINT      REFERENCES entities (id) ON DELETE SET NULL,
    method     TEXT        NOT NULL,              -- 'exact' | 'alias' | 'domain' | 'llm'
    confidence REAL        NOT NULL,
    model      TEXT,                              -- claude model id when method='llm'
    decided_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX resolution_decisions_raw_text_idx ON resolution_decisions (raw_text);

CREATE TABLE users (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name             TEXT NOT NULL,
    telegram_chat_id TEXT NOT NULL
);

CREATE TABLE watchlist (
    user_id      BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    entity_id    BIGINT NOT NULL REFERENCES entities (id) ON DELETE CASCADE,
    alert_config JSONB  NOT NULL DEFAULT '{}',    -- which event types alert, quiet hours
    PRIMARY KEY (user_id, entity_id)
);

CREATE TABLE sources (
    id      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name    TEXT    UNIQUE NOT NULL,              -- NATS subject suffix: signals.raw.<name>
    kind    TEXT    NOT NULL,
    config  JSONB   NOT NULL DEFAULT '{}',
    enabled BOOLEAN NOT NULL DEFAULT true         -- collector health flag
);
