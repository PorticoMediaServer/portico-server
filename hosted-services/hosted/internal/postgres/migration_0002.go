package postgres

// 0002 contains every Hosted remediation schema change after 0001. The runner
// applies it once under Store.Migrate's advisory lock, including on databases
// created by older builds; fresh databases take the same path.
const migration0002 = `
ALTER TABLE certificate_namespaces ADD COLUMN server_identity text UNIQUE;
ALTER TABLE claim_operations ADD COLUMN replaces_server_id text, ADD COLUMN replaces_claim_generation bigint, ADD COLUMN replaces_credential_generation bigint;
ALTER TABLE claim_approvals DROP CONSTRAINT claim_approvals_family_id_account_id_fkey;
ALTER TABLE claim_approvals ADD CONSTRAINT claim_approvals_family_id_account_id_fkey FOREIGN KEY(family_id,account_id) REFERENCES session_families(id,account_id) ON DELETE CASCADE;
ALTER TABLE certificate_challenges DROP CONSTRAINT certificate_challenges_order_id_fkey;
ALTER TABLE certificate_challenges ADD CONSTRAINT certificate_challenges_order_id_fkey FOREIGN KEY(order_id) REFERENCES certificate_orders(id) ON DELETE CASCADE;
ALTER TABLE account_erasure_jobs DROP CONSTRAINT account_erasure_jobs_deletion_id_fkey;
ALTER TABLE account_erasure_jobs ADD CONSTRAINT account_erasure_jobs_deletion_id_fkey FOREIGN KEY(deletion_id) REFERENCES account_deletions(id);
ALTER TABLE account_sessions DROP CONSTRAINT account_sessions_family_id_account_id_fkey;
ALTER TABLE account_sessions ADD CONSTRAINT account_sessions_family_id_account_id_fkey FOREIGN KEY(family_id,account_id) REFERENCES session_families(id,account_id) ON DELETE CASCADE;
ALTER TABLE hosted_devices DROP CONSTRAINT hosted_devices_family_id_account_id_fkey;
ALTER TABLE hosted_devices ADD CONSTRAINT hosted_devices_family_id_account_id_fkey FOREIGN KEY(family_id,account_id) REFERENCES session_families(id,account_id) ON DELETE CASCADE;
ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_family_id_fkey;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_family_id_fkey FOREIGN KEY(family_id) REFERENCES session_families(id) ON DELETE CASCADE;
ALTER TABLE refresh_receipts DROP CONSTRAINT refresh_receipts_family_id_fkey;
ALTER TABLE refresh_receipts ADD CONSTRAINT refresh_receipts_family_id_fkey FOREIGN KEY(family_id) REFERENCES session_families(id) ON DELETE CASCADE;
ALTER TABLE identity_transactions DROP CONSTRAINT identity_security_purpose;
ALTER TABLE identity_transactions ADD CONSTRAINT identity_security_purpose CHECK(purpose IN('password_set_or_change','account_deletion_preview','email_change','provider_link','provider_unlink','mfa_enroll','mfa_disable','recovery_codes','server_transfer','server_remove','account_export'));
ALTER TABLE account_recent_proofs DROP CONSTRAINT account_security_proof_purpose;
ALTER TABLE account_recent_proofs ADD CONSTRAINT account_security_proof_purpose CHECK(purpose IN('password_set_or_change','account_deletion_preview','email_change','provider_link','provider_unlink','mfa_enroll','mfa_disable','recovery_codes','server_transfer','server_remove','account_export'));
ALTER TABLE account_mail_outbox DROP CONSTRAINT account_mail_outbox_kind_check;
ALTER TABLE account_mail_outbox ADD CONSTRAINT account_mail_outbox_kind_check CHECK(kind IN('registration','oidc_contact','password_reset','email_change','security_notice','invitation','server_offline','storage_nearly_full','invitation_accepted','new_device','account_export'));
CREATE TABLE account_password_backoff(account_id text PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,failures integer NOT NULL CHECK(failures>0),first_failure_at timestamptz NOT NULL,locked_until timestamptz NOT NULL);
ALTER TABLE networking_route_candidates DROP CONSTRAINT networking_route_candidates_class_check;
ALTER TABLE networking_route_candidates ADD CONSTRAINT networking_route_candidates_class_check CHECK(class IN('lan','public','manual','relay'));
CREATE TABLE cgnat_aggregate(singleton boolean PRIMARY KEY CHECK(singleton),reports bigint NOT NULL DEFAULT 0,cgnat_reports bigint NOT NULL DEFAULT 0);
INSERT INTO cgnat_aggregate(singleton) VALUES(true);
` + claimWebSchema + notificationSchema + retentionIndexes + exportSchema + observabilitySchema
