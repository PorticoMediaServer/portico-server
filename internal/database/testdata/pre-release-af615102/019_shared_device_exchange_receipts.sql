ALTER TABLE quick_connect_requests ADD COLUMN native_refresh_token_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tv_setup_sessions ADD COLUMN native_refresh_token_id TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_quick_connect_native_refresh_receipt
    ON quick_connect_requests(native_refresh_token_id)
    WHERE native_refresh_token_id <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_tv_setup_native_refresh_receipt
    ON tv_setup_sessions(native_refresh_token_id)
    WHERE native_refresh_token_id <> '';
