-- Persist the normalized playback request used to resolve an active session so
-- partial renegotiations cannot silently discard route, decoder, language, or
-- quality policy. Prepared handoffs are durable, profile-scoped, and use an
-- explicit state machine so retries cannot start the same handoff twice.
ALTER TABLE playback_sessions ADD COLUMN client_profile_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE playback_sessions ADD COLUMN playback_intent_json TEXT NOT NULL DEFAULT '{}';

CREATE TABLE IF NOT EXISTS playback_prepared_handoffs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    source_session_id TEXT NOT NULL REFERENCES playback_sessions(id) ON DELETE CASCADE,
    client_instance_id TEXT NOT NULL DEFAULT '',
    media_id TEXT NOT NULL,
    queue_media_ids_json TEXT NOT NULL DEFAULT '[]',
    source_context_json TEXT NOT NULL DEFAULT '{}',
    queue_revision INTEGER NOT NULL DEFAULT 0,
    playback_revision INTEGER NOT NULL DEFAULT 0,
    state TEXT NOT NULL DEFAULT 'prepared' CHECK (state IN ('prepared', 'committing', 'committed')),
    request_id TEXT NOT NULL DEFAULT '',
    request_fingerprint TEXT NOT NULL DEFAULT '',
    committed_response TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_playback_prepared_handoff_scope
    ON playback_prepared_handoffs(user_id, profile_id, source_session_id, expires_at);
