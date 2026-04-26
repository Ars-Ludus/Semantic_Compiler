CREATE TABLE IF NOT EXISTS memories (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    turn_id     INTEGER NOT NULL,
    summary_id  INTEGER,
    source      TEXT    NOT NULL CHECK(source IN ('user', 'model')),
    raw_message TEXT    NOT NULL,
    personal_tokens TEXT,
    discovered  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL
                DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS memory_semkeys (
    memory_id    INTEGER NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    semkey_value INTEGER NOT NULL,
    PRIMARY KEY (memory_id, semkey_value)
);

CREATE INDEX IF NOT EXISTS idx_semkeys_value ON memory_semkeys(semkey_value);
