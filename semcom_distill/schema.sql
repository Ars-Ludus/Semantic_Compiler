CREATE TABLE IF NOT EXISTS distillations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    topic           TEXT NOT NULL,
    snippet         TEXT NOT NULL,
    personal_tokens TEXT, -- JSON array of related personal token IDs
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS distillation_semkeys (
    distillation_id INTEGER NOT NULL REFERENCES distillations(id) ON DELETE CASCADE,
    semkey_value    INTEGER NOT NULL,
    PRIMARY KEY (distillation_id, semkey_value)
);
CREATE INDEX IF NOT EXISTS idx_distill_semkeys_val ON distillation_semkeys(semkey_value);

CREATE TABLE IF NOT EXISTS distill_metadata (
    key   TEXT PRIMARY KEY,
    value TEXT
);
