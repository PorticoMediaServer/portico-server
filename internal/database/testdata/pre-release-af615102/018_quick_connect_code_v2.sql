UPDATE quick_connect_requests
SET status = 'expired', updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE status IN ('pending', 'approved')
  AND expires_at <= strftime('%Y-%m-%dT%H:%M:%SZ', 'now');

WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY code
            ORDER BY
                CASE WHEN status = 'approved' THEN 0 ELSE 1 END,
                created_at DESC,
                id DESC
        ) AS duplicate_rank
    FROM quick_connect_requests
    WHERE status IN ('pending', 'approved')
)
UPDATE quick_connect_requests
SET status = 'expired', updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE id IN (
    SELECT id
    FROM ranked
    WHERE duplicate_rank > 1
);

DROP INDEX IF EXISTS idx_quick_connect_pending_code;
DROP INDEX IF EXISTS idx_quick_connect_active_code;

CREATE UNIQUE INDEX idx_quick_connect_active_code
ON quick_connect_requests(code)
WHERE status IN ('pending', 'approved');
