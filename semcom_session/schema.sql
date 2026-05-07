CREATE TABLE IF NOT EXISTS session_retrievals (
    session_id TEXT NOT NULL,
    memory_id  INTEGER NOT NULL,
    PRIMARY KEY (session_id, memory_id)
);

CREATE TABLE IF NOT EXISTS session_distillation_retrievals (
    session_id      TEXT    NOT NULL,
    distillation_id INTEGER NOT NULL,
    PRIMARY KEY (session_id, distillation_id)
);
