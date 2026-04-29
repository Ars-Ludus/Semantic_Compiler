CREATE TABLE IF NOT EXISTS personal_tokens (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    word  TEXT NOT NULL UNIQUE,
    type  TEXT NOT NULL -- e.g., "PERSON", "PLACE", "PROJECT", "ORGANIZATION", "TOPIC"
);

CREATE TABLE IF NOT EXISTS memory_personal_tokens (
    memory_id   INTEGER NOT NULL,
    personal_id INTEGER NOT NULL,
    PRIMARY KEY (memory_id, personal_id)
);

CREATE INDEX IF NOT EXISTS idx_mpt_personal ON memory_personal_tokens (personal_id);
