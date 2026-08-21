ALTER TABLE identity_reconciliation_reviews ADD COLUMN subject_id TEXT NOT NULL DEFAULT '';
ALTER TABLE identity_reconciliation_reviews ADD COLUMN resolution TEXT NOT NULL DEFAULT '';
ALTER TABLE identity_reconciliation_reviews ADD COLUMN selected_candidate_id TEXT NOT NULL DEFAULT '';
ALTER TABLE identity_reconciliation_reviews ADD COLUMN resolved_by_user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE identity_reconciliation_reviews ADD COLUMN resolution_note TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_identity_reconciliation_reviews_subject
ON identity_reconciliation_reviews(domain, subject_id, status);
