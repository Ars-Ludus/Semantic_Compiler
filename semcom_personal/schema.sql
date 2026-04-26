CREATE TABLE IF NOT EXISTS personal_tokens (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    word  TEXT NOT NULL UNIQUE,
    type  TEXT NOT NULL -- e.g., "PERSON", "PLACE", "EVENT"
);

CREATE TABLE IF NOT EXISTS personal_ignore (
    word  TEXT PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS personal_semkeys (
    personal_id INTEGER NOT NULL REFERENCES personal_tokens(id) ON DELETE CASCADE,
    memory_id   INTEGER NOT NULL,
    PRIMARY KEY (personal_id, memory_id)
);
CREATE INDEX IF NOT EXISTS idx_personal_semkeys_val ON personal_semkeys(personal_id);
