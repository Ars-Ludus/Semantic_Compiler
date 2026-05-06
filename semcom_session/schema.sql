-- Create: semcom_session/schema.sql
CREATE TABLE IF NOT EXISTS session_retrievals (
    session_id TEXT NOT NULL,
    memory_id  INTEGER NOT NULL,
    PRIMARY KEY (session_id, memory_id)
);
