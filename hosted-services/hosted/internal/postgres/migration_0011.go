package postgres

// 0011: web claims are prepared without an account; the approver's account is
// bound at approval (Plex-style). Pending claims are capped per server key
// instead of per account (A48).
const migration0011 = `
ALTER TABLE claim_web_pending ALTER COLUMN account_id DROP NOT NULL;
ALTER TABLE claim_web_pending ADD COLUMN server_id text;
UPDATE claim_web_pending SET server_id=request->>'serverId';
ALTER TABLE claim_web_pending ALTER COLUMN server_id SET NOT NULL;
CREATE INDEX claim_web_pending_server ON claim_web_pending(server_id,expires_at);
`
