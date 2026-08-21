-- Preserve an encrypted, authenticated copy of the most recently issued
-- continuation credential so a client retry after a lost rotation response
-- receives the exact same replacement credential.
ALTER TABLE playback_session_continuation_credentials
    ADD COLUMN last_rotation_receipt TEXT NOT NULL DEFAULT '';
