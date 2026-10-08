-- Cast reconnect recovery (SV-009). A reconnect rotates the device token; the
-- token it replaced stays usable for a short window, and only until the new one
-- is used, so a receiver whose answer was lost can reconnect again instead of
-- pairing again. Each device keeps one live viewer session family: the one it
-- was last issued, retired when the next is.
ALTER TABLE social_cast_devices ADD COLUMN previous_token_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE social_cast_devices ADD COLUMN previous_valid_until_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE social_cast_devices ADD COLUMN session_family_id TEXT NOT NULL DEFAULT '';
CREATE INDEX social_cast_devices_previous ON social_cast_devices(previous_token_digest) WHERE previous_token_digest<>'';
