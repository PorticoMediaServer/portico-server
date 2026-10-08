package postgres

import (
	"context"
	"database/sql"
)

// P10 owns these additive, restart-safe changes. The current Hosted format and
// identity/claim/security migrations retain their independent authority.
func InstallProfilesMembershipSchemaTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
ALTER TABLE claim_operations ADD COLUMN IF NOT EXISTS origin_account_id text NOT NULL DEFAULT '';
UPDATE claim_operations SET origin_account_id=COALESCE(account_id,'') WHERE origin_account_id='';
CREATE OR REPLACE FUNCTION p10_claim_origin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' THEN NEW.origin_account_id:=COALESCE(NEW.account_id,'');
 ELSIF NEW.origin_account_id<>OLD.origin_account_id THEN RAISE EXCEPTION 'immutable claim origin'; END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS p10_claim_origin ON claim_operations;
CREATE TRIGGER p10_claim_origin BEFORE INSERT OR UPDATE OF origin_account_id ON claim_operations FOR EACH ROW EXECUTE FUNCTION p10_claim_origin();
CREATE TABLE IF NOT EXISTS server_ownership_transfers(
 server_id text NOT NULL REFERENCES servers(id),operation_id text NOT NULL,claim_operation_id text NOT NULL REFERENCES claim_operations(id),
 previous_account_id text NOT NULL,new_account_id text NOT NULL,policy_revision bigint NOT NULL,
 transferred_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(server_id,operation_id));
ALTER TABLE identity_transactions DROP CONSTRAINT IF EXISTS identity_security_purpose;
ALTER TABLE identity_transactions ADD CONSTRAINT identity_security_purpose CHECK(purpose IN('password_set_or_change','account_deletion_preview','email_change','provider_link','provider_unlink','mfa_enroll','mfa_disable','recovery_codes','server_transfer'));
ALTER TABLE account_recent_proofs DROP CONSTRAINT IF EXISTS account_security_proof_purpose;
ALTER TABLE account_recent_proofs ADD CONSTRAINT account_security_proof_purpose CHECK(purpose IN('password_set_or_change','account_deletion_preview','email_change','provider_link','provider_unlink','mfa_enroll','mfa_disable','recovery_codes','server_transfer'));
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS is_primary boolean NOT NULL DEFAULT false;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS position integer NOT NULL DEFAULT 0;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS art text NOT NULL DEFAULT 'blue';
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS revision bigint NOT NULL DEFAULT 1;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS pin_revision bigint NOT NULL DEFAULT 1;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS authorization_revision bigint NOT NULL DEFAULT 1;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS trust_revision bigint NOT NULL DEFAULT 1;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS pin_hash bytea;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS pin_attempts integer NOT NULL DEFAULT 0;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS pin_locked_until timestamptz;
ALTER TABLE profiles ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
UPDATE profiles p SET is_primary=true WHERE p.id=(SELECT q.id FROM profiles q WHERE q.account_id=p.account_id ORDER BY q.created_at,q.id LIMIT 1) AND NOT EXISTS(SELECT 1 FROM profiles q WHERE q.account_id=p.account_id AND q.is_primary);
CREATE UNIQUE INDEX IF NOT EXISTS profiles_primary ON profiles(account_id) WHERE is_primary;
CREATE INDEX IF NOT EXISTS profiles_active_order ON profiles(account_id,position,id) WHERE deleted_at IS NULL;
ALTER TABLE memberships ADD COLUMN IF NOT EXISTS allow_subprofiles boolean NOT NULL DEFAULT false;
ALTER TABLE memberships ADD COLUMN IF NOT EXISTS all_libraries boolean NOT NULL DEFAULT false;
CREATE TABLE IF NOT EXISTS profile_server_policies(
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 profile_id text NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
 server_id text NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 allowed_libraries text[] NOT NULL,PRIMARY KEY(account_id,profile_id,server_id));
CREATE TABLE IF NOT EXISTS profile_trusted_selections(
 token_hash bytea PRIMARY KEY,account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 profile_id text NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
 server_id text NOT NULL REFERENCES servers(id) ON DELETE CASCADE,installation_id text NOT NULL,
 pin_revision bigint NOT NULL,profile_revision bigint NOT NULL,trust_revision bigint NOT NULL,
 policy_revision bigint NOT NULL,auth_epoch bigint NOT NULL,operation_id text NOT NULL,expires_at timestamptz NOT NULL,
 UNIQUE(account_id,profile_id,server_id,installation_id));
CREATE TABLE IF NOT EXISTS profile_deletions(
 id text PRIMARY KEY,account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 profile_id text NOT NULL UNIQUE REFERENCES profiles(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp());
CREATE TABLE IF NOT EXISTS profile_erasure_jobs(
 id text PRIMARY KEY,deletion_id text NOT NULL REFERENCES profile_deletions(id) ON DELETE CASCADE,
 server_id text NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 account_id text NOT NULL,profile_id text NOT NULL,revision bigint NOT NULL DEFAULT 1,
 policy_revision bigint NOT NULL,acknowledged_at timestamptz,
 UNIQUE(deletion_id,server_id));
ALTER TABLE invitations ALTER COLUMN recipient_id DROP NOT NULL;
ALTER TABLE invitations DROP CONSTRAINT IF EXISTS invitations_status_check;
ALTER TABLE invitations ADD CONSTRAINT invitations_status_check CHECK(status IN ('pending','accepted','cancelled','revoked','declined'));
ALTER TABLE invitations ADD COLUMN IF NOT EXISTS recipient_kind text NOT NULL DEFAULT 'username';
ALTER TABLE invitations ADD COLUMN IF NOT EXISTS recipient_email text;
ALTER TABLE invitations ADD COLUMN IF NOT EXISTS delivery_hash bytea;
ALTER TABLE invitations ADD COLUMN IF NOT EXISTS delivery_revision bigint NOT NULL DEFAULT 1;
ALTER TABLE invitations ADD COLUMN IF NOT EXISTS allow_subprofiles boolean NOT NULL DEFAULT false;
CREATE UNIQUE INDEX IF NOT EXISTS invitations_delivery ON invitations(delivery_hash) WHERE delivery_hash IS NOT NULL;
CREATE TABLE IF NOT EXISTS membership_operation_receipts(
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 family_id text NOT NULL REFERENCES session_families(id) ON DELETE CASCADE,
 operation_id text NOT NULL,kind text NOT NULL,request_hash bytea NOT NULL,
 result_ciphertext bytea NOT NULL,expires_at timestamptz NOT NULL,
 PRIMARY KEY(account_id,family_id,operation_id));
CREATE OR REPLACE FUNCTION p10_account_epoch_policy() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE rec record; next_revision bigint;
BEGIN
 IF NEW.auth_epoch=OLD.auth_epoch THEN RETURN NEW; END IF;
 FOR rec IN SELECT s.id FROM servers s WHERE EXISTS(SELECT 1 FROM memberships m WHERE m.server_id=s.id AND m.account_id=NEW.id) ORDER BY s.id FOR UPDATE LOOP
  UPDATE servers SET policy_revision=policy_revision+1,policy_issued_at=clock_timestamp() WHERE id=rec.id RETURNING policy_revision INTO next_revision;
  INSERT INTO wake_state(server_id,kind,desired_revision) VALUES(rec.id,'policy',next_revision) ON CONFLICT(server_id,kind) DO UPDATE SET first_attempt_at=CASE WHEN EXCLUDED.desired_revision>wake_state.desired_revision THEN NULL ELSE wake_state.first_attempt_at END,wake_unreachable=CASE WHEN EXCLUDED.desired_revision>wake_state.desired_revision THEN false ELSE wake_state.wake_unreachable END,attempts=CASE WHEN EXCLUDED.desired_revision>wake_state.desired_revision THEN 0 ELSE wake_state.attempts END,desired_revision=GREATEST(wake_state.desired_revision,EXCLUDED.desired_revision),next_attempt_at=LEAST(wake_state.next_attempt_at,clock_timestamp());
 END LOOP;
 DELETE FROM profile_trusted_selections WHERE account_id=NEW.id;
 DELETE FROM attachment_tickets WHERE account_id=NEW.id AND consumed_at IS NULL;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS p10_account_epoch_policy ON accounts;
CREATE TRIGGER p10_account_epoch_policy AFTER UPDATE OF auth_epoch ON accounts FOR EACH ROW EXECUTE FUNCTION p10_account_epoch_policy();
CREATE OR REPLACE FUNCTION p10_profile_insert() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE n integer;
BEGIN
 PERFORM id FROM accounts WHERE id=NEW.account_id FOR UPDATE;
 SELECT count(*) INTO n FROM profiles WHERE account_id=NEW.account_id AND deleted_at IS NULL;
 IF n>=8 THEN RAISE EXCEPTION 'profile capacity reached' USING ERRCODE='23514'; END IF;
 NEW.is_primary := NOT EXISTS(SELECT 1 FROM profiles WHERE account_id=NEW.account_id);
 NEW.position := COALESCE((SELECT max(position)+1 FROM profiles WHERE account_id=NEW.account_id AND deleted_at IS NULL),0);
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS p10_profile_insert ON profiles;
CREATE TRIGGER p10_profile_insert BEFORE INSERT ON profiles FOR EACH ROW EXECUTE FUNCTION p10_profile_insert();
`)
	return err
}
