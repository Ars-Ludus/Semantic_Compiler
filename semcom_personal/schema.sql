CREATE TABLE IF NOT EXISTS personal_tokens (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    word  TEXT NOT NULL UNIQUE,
    type  TEXT NOT NULL -- e.g., "PERSON", "PLACE", "EVENT"
);
-- Start personal IDs at 1,000,000 to avoid collision with global L0 IDs
INSERT INTO sqlite_sequence (name, seq) VALUES ('personal_tokens', 999999);

CREATE TABLE IF NOT EXISTS personal_ignore (
    word  TEXT PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS personal_semkeys (
    personal_id INTEGER NOT NULL REFERENCES personal_tokens(id) ON DELETE CASCADE,
    memory_id   INTEGER NOT NULL,
    PRIMARY KEY (personal_id, memory_id)
);
CREATE INDEX IF NOT EXISTS idx_personal_semkeys_val ON personal_semkeys(personal_id);
