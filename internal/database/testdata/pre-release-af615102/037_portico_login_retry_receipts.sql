ALTER TABLE portico_login_requests ADD COLUMN exchange_result_json TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_portico_login_retryable_state
    ON portico_login_requests(state_hash, status, expires_at);
