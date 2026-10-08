package postgres

// Additive indexes only: session review reuses the existing family authority.
const sessionReviewSchema = `
CREATE INDEX IF NOT EXISTS session_families_review_page ON session_families(account_id,created_at DESC,id DESC);
`
