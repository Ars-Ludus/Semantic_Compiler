CREATE TABLE IF NOT EXISTS personal_tokens (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    word  TEXT NOT NULL UNIQUE,
    type  TEXT NOT NULL -- e.g., "PERSON", "PLACE", "EVENT"
);

CREATE TABLE IF NOT EXISTS personal_semkeys (
    personal_id INTEGER NOT NULL REFERENCES personal_tokens(id) ON DELETE CASCADE,
    memory_id   INTEGER NOT NULL,
    PRIMARY KEY (personal_id, memory_id)
);
CREATE INDEX IF NOT EXISTS idx_personal_semkeys_val ON personal_semkeys(personal_id);

CREATE TABLE IF NOT EXISTS distillations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    topic           TEXT NOT NULL,
    snippet         TEXT NOT NULL,
    personal_tokens TEXT, -- JSON array of related personal IDs
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS distillation_semkeys (
    distillation_id INTEGER NOT NULL REFERENCES distillations(id) ON DELETE CASCADE,
    semkey_value    INTEGER NOT NULL,
    PRIMARY KEY (distillation_id, semkey_value)
);
CREATE INDEX IF NOT EXISTS idx_distill_semkeys_val ON distillation_semkeys(semkey_value);

CREATE TABLE IF NOT EXISTS metadata (
    key   TEXT PRIMARY KEY,
    value TEXT
);
