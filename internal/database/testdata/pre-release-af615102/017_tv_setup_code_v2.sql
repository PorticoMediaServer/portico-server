UPDATE tv_setup_sessions
SET status = 'expired', updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE status IN ('pending', 'grant_ready')
  AND expires_at <= strftime('%Y-%m-%dT%H:%M:%SZ', 'now');

WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY code
            ORDER BY created_at DESC, id DESC
        ) AS duplicate_rank
    FROM tv_setup_sessions
    WHERE status IN ('pending', 'grant_ready')
)
UPDATE tv_setup_sessions
SET status = 'expired', updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE id IN (
    SELECT id
    FROM ranked
    WHERE duplicate_rank > 1
);

DROP INDEX IF EXISTS idx_tv_setup_active_code;

CREATE UNIQUE INDEX idx_tv_setup_active_code
ON tv_setup_sessions(code)
WHERE status IN ('pending', 'grant_ready');
