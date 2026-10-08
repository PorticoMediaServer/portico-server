package postgres

// Adopting a pre-migration ("contract version 2") Hosted database (A45).
//
// Production Hosted was provisioned before numbered migrations existed: its
// hosted_schema_contract says version 2 and it has no hosted_schema_migrations
// table. Its shape is the 22 Sep baseline, which differs from 0001 by the
// statements below (captured by diffing a baseline install against a 0001
// install; testdata/legacy_v2_schema.sql is that baseline). The bridge brings it
// to exactly the 0001 shape and records 0001; the ordinary migrations then run.
// Existing rows are kept; only the unused wake_outbox table is dropped.
const legacyV2Bridge = `
ALTER TABLE certificate_namespaces ADD COLUMN member_label text NOT NULL DEFAULT '' CONSTRAINT certificate_namespaces_member_label_check CHECK(member_label='' OR member_label ~ '^[0-9a-f]{32}$');
ALTER TABLE direct_addresses ADD COLUMN lan_ipv4 text NOT NULL DEFAULT '', ADD COLUMN lan_ipv6 text NOT NULL DEFAULT '';
ALTER TABLE servers ADD COLUMN last_seen_at timestamptz NOT NULL DEFAULT clock_timestamp(), ADD COLUMN retired_reason text NOT NULL DEFAULT '', ADD COLUMN replaced_by text;
ALTER TABLE wake_state ADD COLUMN first_attempt_at timestamptz, ADD COLUMN wake_unreachable boolean NOT NULL DEFAULT false;
DROP TABLE wake_outbox;
CREATE TABLE identity_native_completions(
 transaction_id text PRIMARY KEY REFERENCES identity_transactions(id) ON DELETE CASCADE,
 code_hash bytea NOT NULL CHECK(octet_length(code_hash)=32),
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz
);
ALTER TABLE account_sessions DROP CONSTRAINT account_sessions_family_id_account_id_fkey, ADD CONSTRAINT account_sessions_family_id_account_id_fkey FOREIGN KEY(family_id,account_id) REFERENCES session_families(id,account_id) ON DELETE CASCADE;
ALTER TABLE hosted_devices DROP CONSTRAINT hosted_devices_family_id_account_id_fkey, ADD CONSTRAINT hosted_devices_family_id_account_id_fkey FOREIGN KEY(family_id,account_id) REFERENCES session_families(id,account_id) ON DELETE CASCADE;
ALTER TABLE refresh_receipts DROP CONSTRAINT refresh_receipts_family_id_fkey, ADD CONSTRAINT refresh_receipts_family_id_fkey FOREIGN KEY(family_id) REFERENCES session_families(id) ON DELETE CASCADE;
ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_family_id_fkey, ADD CONSTRAINT refresh_tokens_family_id_fkey FOREIGN KEY(family_id) REFERENCES session_families(id) ON DELETE CASCADE;
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
ALTER TABLE hosted_schema_contract DROP CONSTRAINT hosted_schema_contract_version_check, ADD CONSTRAINT hosted_schema_contract_version_check CHECK(version>0);
CREATE TABLE hosted_schema_migrations(version integer PRIMARY KEY CHECK(version>0),name text NOT NULL,applied_at timestamptz NOT NULL DEFAULT clock_timestamp());
INSERT INTO hosted_schema_migrations(version,name) VALUES(1,'0001_initial');
UPDATE hosted_schema_contract SET version=1 WHERE singleton;
`
