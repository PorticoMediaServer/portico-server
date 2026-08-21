-- Completed recordings imported by older builds used the deterministic
-- dvr_<recording-id> media identity but had no ownership mapping. This
-- relation makes profile/channel authorization authoritative for playback.
CREATE TABLE dvr_recording_media (
    recording_id TEXT PRIMARY KEY REFERENCES live_tv_recordings(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL UNIQUE REFERENCES media_items(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL
);

INSERT OR IGNORE INTO dvr_recording_media (recording_id, media_id, created_at)
SELECT r.id, m.id, COALESCE(NULLIF(r.updated_at, ''), r.created_at)
FROM live_tv_recordings r
JOIN media_items m ON m.id = 'dvr_' || r.id AND m.type = 'recording';

-- Preserve completed artifacts while detaching historical duplicate rule
-- links. Reconciliation may materialize a guide program at most once per rule,
-- including when refresh workers race across server processes.
UPDATE live_tv_recordings
SET rule_id = NULL
WHERE rule_id IS NOT NULL AND program_id <> ''
  AND id NOT IN (
      SELECT MIN(id)
      FROM live_tv_recordings
      WHERE rule_id IS NOT NULL AND program_id <> ''
      GROUP BY rule_id, program_id
  );

CREATE UNIQUE INDEX idx_recordings_rule_program_unique
    ON live_tv_recordings(rule_id, program_id)
    WHERE rule_id IS NOT NULL AND program_id <> '';

CREATE INDEX idx_recordings_account_status
    ON live_tv_recordings(user_id, status, starts_at DESC, id);

CREATE INDEX idx_recording_rules_account
    ON live_tv_recording_rules(user_id, enabled, updated_at DESC, id);
