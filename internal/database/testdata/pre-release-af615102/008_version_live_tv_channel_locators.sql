DROP INDEX IF EXISTS idx_live_tv_channel_locators_channel;

CREATE INDEX IF NOT EXISTS idx_live_tv_channel_locators_channel
ON live_tv_channel_locators(channel_id, active, last_seen_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_live_tv_channel_locators_evidence
ON live_tv_channel_locators(
    channel_id,
    provider_kind,
    provider_key,
    stream_url,
    tvg_id,
    channel_number,
    normalized_name
);
