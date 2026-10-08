
CREATE FUNCTION identity_link_revision_guard() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN
 IF NEW.id<>OLD.id OR NEW.account_id<>OLD.account_id OR NEW.provider<>OLD.provider OR NEW.issuer<>OLD.issuer OR NEW.subject<>OLD.subject OR NEW.created_at<>OLD.created_at OR NEW.revision<>OLD.revision+1 THEN
  RAISE EXCEPTION 'identity link authority is immutable and revision must advance';
 END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION identity_policy_revision_guard() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'identity policy cannot be deleted'; END IF;
 IF NEW.revision<>OLD.revision+1 OR NEW.singleton<>OLD.singleton THEN RAISE EXCEPTION 'identity policy revision must advance'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION p10_account_epoch_policy() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE rec record; next_revision bigint;
BEGIN
 IF NEW.auth_epoch=OLD.auth_epoch THEN RETURN NEW; END IF;
 FOR rec IN SELECT s.id FROM servers s WHERE EXISTS(SELECT 1 FROM memberships m WHERE m.server_id=s.id AND m.account_id=NEW.id) ORDER BY s.id FOR UPDATE LOOP
  UPDATE servers SET policy_revision=policy_revision+1,policy_issued_at=clock_timestamp() WHERE id=rec.id RETURNING policy_revision INTO next_revision;
  INSERT INTO wake_outbox(server_id,kind,target_revision) VALUES(rec.id,'policy',next_revision) ON CONFLICT DO NOTHING;
  INSERT INTO wake_state(server_id,kind,desired_revision) VALUES(rec.id,'policy',next_revision) ON CONFLICT(server_id,kind) DO UPDATE SET desired_revision=GREATEST(wake_state.desired_revision,EXCLUDED.desired_revision),next_attempt_at=LEAST(wake_state.next_attempt_at,clock_timestamp());
 END LOOP;
 DELETE FROM profile_trusted_selections WHERE account_id=NEW.id;
 DELETE FROM attachment_tickets WHERE account_id=NEW.id AND consumed_at IS NULL;
 RETURN NEW;
END $$;

CREATE FUNCTION p10_claim_origin() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF TG_OP='INSERT' THEN NEW.origin_account_id:=COALESCE(NEW.account_id,'');
 ELSIF NEW.origin_account_id<>OLD.origin_account_id THEN RAISE EXCEPTION 'immutable claim origin'; END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION p10_profile_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE n integer;
BEGIN
 PERFORM id FROM accounts WHERE id=NEW.account_id FOR UPDATE;
 SELECT count(*) INTO n FROM profiles WHERE account_id=NEW.account_id AND deleted_at IS NULL;
 IF n>=8 THEN RAISE EXCEPTION 'profile capacity reached' USING ERRCODE='23514'; END IF;
 NEW.is_primary := NOT EXISTS(SELECT 1 FROM profiles WHERE account_id=NEW.account_id);
 NEW.position := COALESCE((SELECT max(position)+1 FROM profiles WHERE account_id=NEW.account_id AND deleted_at IS NULL),0);
 RETURN NEW;
END $$;

CREATE FUNCTION portico_notify_wake() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
 BEGIN PERFORM pg_notify('portico_wake_' || current_schema(), ''); RETURN NULL; END $$;

CREATE TABLE account_contact_emails (
    id text NOT NULL,
    account_id text NOT NULL,
    address text NOT NULL,
    verified_at timestamp with time zone NOT NULL,
    source text NOT NULL,
    source_identity_id text,
    private_relay boolean DEFAULT false NOT NULL,
    is_primary boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT account_contact_emails_address_check CHECK (((octet_length(address) >= 3) AND (octet_length(address) <= 320))),
    CONSTRAINT account_contact_emails_check CHECK ((((source = 'provider'::text) AND (source_identity_id IS NOT NULL)) OR ((source = 'challenge'::text) AND (source_identity_id IS NULL)))),
    CONSTRAINT account_contact_emails_id_check CHECK (((octet_length(id) >= 1) AND (octet_length(id) <= 128))),
    CONSTRAINT account_contact_emails_source_check CHECK ((source = ANY (ARRAY['provider'::text, 'challenge'::text])))
);

CREATE TABLE account_deletions (
    id text NOT NULL,
    token_hash bytea NOT NULL,
    account_id text NOT NULL,
    password_hash bytea,
    snapshot_hash bytea,
    expires_at timestamp with time zone NOT NULL,
    accepted_at timestamp with time zone,
    completed_at timestamp with time zone,
    disposition text,
    receipt_expires_at timestamp with time zone,
    expected_family_id text,
    expected_account_epoch bigint,
    expected_family_generation bigint,
    expected_access_hash bytea,
    expected_session_mode text
);

CREATE TABLE account_email_challenges (
    id text NOT NULL,
    account_id text NOT NULL,
    purpose text NOT NULL,
    address text NOT NULL,
    contact_id text,
    account_epoch bigint NOT NULL,
    family_id text,
    family_generation bigint,
    access_hash bytea,
    password_fingerprint bytea,
    code_hash bytea NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    superseded_at timestamp with time zone,
    operation_id text NOT NULL,
    request_hash bytea NOT NULL,
    completion_hash bytea,
    CONSTRAINT account_email_challenges_attempts_check CHECK (((attempts >= 0) AND (attempts <= 5))),
    CONSTRAINT account_email_challenges_purpose_check CHECK ((purpose = ANY (ARRAY['password_reset'::text, 'email_change'::text])))
);

CREATE TABLE account_erasure_jobs (
    id text NOT NULL,
    deletion_id text NOT NULL,
    server_id text NOT NULL,
    account_id text NOT NULL,
    profile_ids text[] NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    disposition text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    acknowledged_at timestamp with time zone,
    claim_operation_id text,
    claim_generation bigint,
    credential_generation bigint
);

CREATE TABLE account_external_identities (
    id text NOT NULL,
    account_id text NOT NULL,
    provider text NOT NULL,
    issuer text NOT NULL,
    subject text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    verified_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    apple_revocation_at timestamp with time zone,
    CONSTRAINT account_external_identities_check CHECK ((((provider = 'google'::text) AND (issuer = 'https://accounts.google.com'::text)) OR ((provider = 'apple'::text) AND (issuer = 'https://appleid.apple.com'::text)))),
    CONSTRAINT account_external_identities_id_check CHECK (((octet_length(id) >= 1) AND (octet_length(id) <= 128))),
    CONSTRAINT account_external_identities_issuer_check CHECK (((octet_length(issuer) >= 1) AND (octet_length(issuer) <= 256))),
    CONSTRAINT account_external_identities_provider_check CHECK ((provider = ANY (ARRAY['google'::text, 'apple'::text]))),
    CONSTRAINT account_external_identities_revision_check CHECK ((revision > 0)),
    CONSTRAINT account_external_identities_subject_check CHECK (((octet_length(subject) >= 1) AND (octet_length(subject) <= 255)))
);

CREATE TABLE account_identity_schema (
    singleton boolean DEFAULT true NOT NULL,
    version integer NOT NULL,
    CONSTRAINT account_identity_schema_singleton_check CHECK (singleton)
);

CREATE TABLE account_mail_outbox (
    id text NOT NULL,
    kind text NOT NULL,
    resource_id text NOT NULL,
    revision bigint NOT NULL,
    account_id text,
    account_epoch bigint DEFAULT 0 NOT NULL,
    ciphertext bytea,
    payload_hash bytea NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    next_attempt_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    lease_id text,
    lease_until timestamp with time zone,
    last_failure text,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    sent_at timestamp with time zone,
    CONSTRAINT account_mail_outbox_attempts_check CHECK (((attempts >= 0) AND (attempts <= 8))),
    CONSTRAINT account_mail_outbox_check CHECK (((status = 'sending'::text) = ((lease_id IS NOT NULL) AND (lease_until IS NOT NULL)))),
    CONSTRAINT account_mail_outbox_check1 CHECK (((status = ANY (ARRAY['pending'::text, 'sending'::text])) = (ciphertext IS NOT NULL))),
    CONSTRAINT account_mail_outbox_id_check CHECK ((id ~ '^[A-Za-z0-9_-]{16,128}$'::text)),
    CONSTRAINT account_mail_outbox_kind_check CHECK ((kind = ANY (ARRAY['registration'::text, 'oidc_contact'::text, 'password_reset'::text, 'email_change'::text, 'security_notice'::text, 'invitation'::text]))),
    CONSTRAINT account_mail_outbox_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'sending'::text, 'sent'::text, 'failed'::text, 'cancelled'::text])))
);

CREATE TABLE account_mfa (
    account_id text NOT NULL,
    secret bytea,
    last_step bigint DEFAULT '-1'::integer NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    pending_secret bytea,
    pending_id text,
    pending_expires_at timestamp with time zone,
    pending_family_id text,
    pending_family_generation bigint,
    pending_epoch bigint,
    pending_access_hash bytea,
    pending_attempts integer DEFAULT 0 NOT NULL,
    CONSTRAINT account_mfa_check CHECK (((pending_secret IS NULL) = (pending_id IS NULL))),
    CONSTRAINT account_mfa_pending_attempts_check CHECK (((pending_attempts >= 0) AND (pending_attempts <= 5)))
);

CREATE TABLE account_mfa_challenges (
    id text NOT NULL,
    proof_hash bytea NOT NULL,
    account_id text NOT NULL,
    account_epoch bigint NOT NULL,
    purpose text NOT NULL,
    session_mode text NOT NULL,
    identity_transaction_id text,
    credential_kind text NOT NULL,
    password_hash bytea,
    identity_id text,
    identity_revision bigint,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    consumed_at timestamp with time zone,
    family_id text,
    family_generation bigint,
    receipt bytea,
    receipt_expires_at timestamp with time zone,
    completion_hash bytea,
    CONSTRAINT account_mfa_challenges_attempts_check CHECK (((attempts >= 0) AND (attempts <= 5))),
    CONSTRAINT account_mfa_challenges_credential_kind_check CHECK ((credential_kind = ANY (ARRAY['password'::text, 'provider'::text]))),
    CONSTRAINT account_mfa_challenges_proof_hash_check CHECK ((octet_length(proof_hash) = 32)),
    CONSTRAINT account_mfa_challenges_purpose_check CHECK ((purpose = 'login'::text)),
    CONSTRAINT account_mfa_challenges_session_mode_check CHECK ((session_mode = ANY (ARRAY['native'::text, 'browser'::text])))
);

CREATE TABLE account_password_logins (
    binding_hash bytea NOT NULL,
    request_id text NOT NULL,
    request_hash bytea NOT NULL,
    account_id text NOT NULL,
    account_epoch bigint NOT NULL,
    session_mode text NOT NULL,
    ciphertext bytea NOT NULL,
    expires_at timestamp with time zone NOT NULL
);

CREATE TABLE account_preferences (
    account_id text NOT NULL,
    locale text DEFAULT ''::text NOT NULL,
    time_zone text DEFAULT ''::text NOT NULL,
    hour_cycle text DEFAULT 'auto'::text NOT NULL,
    pause_watch_history boolean DEFAULT false NOT NULL,
    show_activity_to_members boolean DEFAULT true NOT NULL,
    include_in_watch_together boolean DEFAULT true NOT NULL,
    marketing_email boolean DEFAULT false NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT account_preferences_hour_cycle_check CHECK ((hour_cycle = ANY (ARRAY['auto'::text, 'h11'::text, 'h12'::text, 'h23'::text, 'h24'::text]))),
    CONSTRAINT account_preferences_locale_check CHECK ((length(locale) <= 35)),
    CONSTRAINT account_preferences_revision_check CHECK ((revision > 0)),
    CONSTRAINT account_preferences_time_zone_check CHECK ((length(time_zone) <= 64))
);

CREATE TABLE account_recent_proofs (
    proof_hash bytea NOT NULL,
    account_id text NOT NULL,
    family_id text NOT NULL,
    account_epoch bigint NOT NULL,
    family_generation bigint NOT NULL,
    session_mode text NOT NULL,
    captured_access_hash bytea NOT NULL,
    credential_kind text NOT NULL,
    identity_id text,
    identity_revision bigint,
    purpose text NOT NULL,
    intent_digest bytea NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    CONSTRAINT account_recent_proofs_account_epoch_check CHECK ((account_epoch >= 0)),
    CONSTRAINT account_recent_proofs_captured_access_hash_check CHECK ((octet_length(captured_access_hash) = 32)),
    CONSTRAINT account_recent_proofs_check CHECK ((((credential_kind = 'password'::text) AND (identity_id IS NULL) AND (identity_revision IS NULL)) OR ((credential_kind = 'provider'::text) AND (identity_id IS NOT NULL) AND (identity_revision IS NOT NULL)))),
    CONSTRAINT account_recent_proofs_check1 CHECK (((expires_at > created_at) AND (expires_at <= (created_at + '00:05:00'::interval)))),
    CONSTRAINT account_recent_proofs_check2 CHECK (((consumed_at IS NULL) OR ((consumed_at >= created_at) AND (consumed_at < expires_at)))),
    CONSTRAINT account_recent_proofs_credential_kind_check CHECK ((credential_kind = ANY (ARRAY['password'::text, 'provider'::text]))),
    CONSTRAINT account_recent_proofs_family_generation_check CHECK ((family_generation >= 0)),
    CONSTRAINT account_recent_proofs_identity_revision_check CHECK ((identity_revision > 0)),
    CONSTRAINT account_recent_proofs_intent_digest_check CHECK ((octet_length(intent_digest) = 32)),
    CONSTRAINT account_recent_proofs_proof_hash_check CHECK ((octet_length(proof_hash) = 32)),
    CONSTRAINT account_recent_proofs_session_mode_check CHECK ((session_mode = ANY (ARRAY['native'::text, 'browser'::text]))),
    CONSTRAINT account_security_proof_purpose CHECK ((purpose = ANY (ARRAY['password_set_or_change'::text, 'account_deletion_preview'::text, 'email_change'::text, 'provider_link'::text, 'provider_unlink'::text, 'mfa_enroll'::text, 'mfa_disable'::text, 'recovery_codes'::text, 'server_transfer'::text])))
);

CREATE TABLE account_recovery_codes (
    account_id text NOT NULL,
    code_hash bytea NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    consumed_at timestamp with time zone,
    CONSTRAINT account_recovery_codes_code_hash_check CHECK ((octet_length(code_hash) = 32))
);

CREATE TABLE account_registrations (
    id text NOT NULL,
    binding_hash bytea NOT NULL,
    request_id text NOT NULL,
    request_hash bytea NOT NULL,
    session_mode text NOT NULL,
    username text NOT NULL,
    display_name text NOT NULL,
    address text NOT NULL,
    password_hash bytea,
    code_hash bytea,
    revision bigint DEFAULT 1 NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    resend_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    code_expires_at timestamp with time zone NOT NULL,
    last_resend_request text,
    verified_at timestamp with time zone,
    completed_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    account_id text,
    family_id text,
    account_epoch bigint,
    family_generation bigint,
    receipt bytea,
    receipt_expires_at timestamp with time zone,
    CONSTRAINT account_registrations_attempts_check CHECK (((attempts >= 0) AND (attempts <= 5))),
    CONSTRAINT account_registrations_binding_hash_check CHECK ((octet_length(binding_hash) = 32)),
    CONSTRAINT account_registrations_check CHECK ((expires_at > created_at)),
    CONSTRAINT account_registrations_check1 CHECK ((code_expires_at <= expires_at)),
    CONSTRAINT account_registrations_request_hash_check CHECK ((octet_length(request_hash) = 32)),
    CONSTRAINT account_registrations_resend_count_check CHECK (((resend_count >= 0) AND (resend_count <= 5))),
    CONSTRAINT account_registrations_session_mode_check CHECK ((session_mode = ANY (ARRAY['native'::text, 'browser'::text])))
);

CREATE TABLE account_security_limits (
    subject_hash bytea NOT NULL,
    kind text NOT NULL,
    window_start timestamp with time zone NOT NULL,
    attempts integer NOT NULL
);

CREATE TABLE account_security_receipts (
    account_id text NOT NULL,
    operation_id text NOT NULL,
    purpose text NOT NULL,
    family_id text NOT NULL,
    access_hash bytea NOT NULL,
    request_hash bytea NOT NULL,
    ciphertext bytea NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    account_epoch bigint,
    family_generation bigint,
    result_epoch bigint,
    terminal_reason text DEFAULT ''::text NOT NULL,
    terminal_revoked_at timestamp with time zone,
    mfa_revision bigint
);

CREATE TABLE account_security_schema (
    singleton boolean NOT NULL,
    version integer NOT NULL,
    CONSTRAINT account_security_schema_singleton_check CHECK (singleton),
    CONSTRAINT account_security_schema_version_check CHECK ((version = 1))
);

CREATE TABLE account_sessions (
    token_hash bytea NOT NULL,
    account_id text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    family_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE accounts (
    id text NOT NULL,
    username text NOT NULL,
    display_name text NOT NULL,
    password_hash bytea,
    auth_epoch bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT accounts_password_hash_check CHECK (((password_hash IS NULL) OR (octet_length(password_hash) > 0)))
);

CREATE TABLE apple_account_notifications (
    id_hash bytea NOT NULL,
    payload_hash bytea NOT NULL,
    event_type text NOT NULL,
    received_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT apple_account_notifications_id_hash_check CHECK ((octet_length(id_hash) = 32)),
    CONSTRAINT apple_account_notifications_payload_hash_check CHECK ((octet_length(payload_hash) = 32))
);

CREATE TABLE apple_identity_events (
    subject_hash bytea NOT NULL,
    revoked_at timestamp with time zone,
    deleted_at timestamp with time zone,
    authorized_at timestamp with time zone,
    CONSTRAINT apple_identity_events_subject_hash_check CHECK ((octet_length(subject_hash) = 32))
);

CREATE TABLE apple_notification_schema (
    singleton boolean DEFAULT true NOT NULL,
    version integer NOT NULL,
    CONSTRAINT apple_notification_schema_singleton_check CHECK (singleton),
    CONSTRAINT apple_notification_schema_version_check CHECK ((version = 1))
);

CREATE TABLE apple_relay_events (
    subject_hash bytea NOT NULL,
    email_hash bytea NOT NULL,
    event_at timestamp with time zone NOT NULL,
    disabled boolean NOT NULL,
    CONSTRAINT apple_relay_events_email_hash_check CHECK ((octet_length(email_hash) = 32)),
    CONSTRAINT apple_relay_events_subject_hash_check CHECK ((octet_length(subject_hash) = 32))
);

CREATE TABLE appstore_notification_failures (
    id bigint NOT NULL,
    reason text NOT NULL,
    observed_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

CREATE SEQUENCE appstore_notification_failures_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE appstore_notification_failures_id_seq OWNED BY appstore_notification_failures.id;

CREATE TABLE appstore_notifications (
    uuid text NOT NULL,
    notification_type text NOT NULL,
    subtype text DEFAULT ''::text NOT NULL,
    bundle_id text DEFAULT ''::text NOT NULL,
    environment text DEFAULT ''::text NOT NULL,
    original_transaction_id text DEFAULT ''::text NOT NULL,
    app_account_token text DEFAULT ''::text NOT NULL,
    account_id text,
    signed_at timestamp with time zone,
    received_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT appstore_notifications_environment_check CHECK ((environment = ANY (ARRAY[''::text, 'Sandbox'::text, 'Production'::text])))
);

CREATE TABLE attachment_tickets (
    token_hash bytea NOT NULL,
    server_id text NOT NULL,
    account_id text NOT NULL,
    profile_id text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone
);

CREATE TABLE certificate_acme_accounts (
    issuer text NOT NULL,
    directory_url text NOT NULL,
    key_cipher bytea NOT NULL,
    account_url text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE certificate_challenges (
    order_id text NOT NULL,
    authorization_url text NOT NULL,
    challenge_url text NOT NULL,
    namespace text NOT NULL,
    txt_value text NOT NULL,
    cleanup_done boolean DEFAULT false NOT NULL,
    cleanup_after timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT certificate_challenges_txt_value_check CHECK ((length(txt_value) = 43))
);

CREATE TABLE certificate_namespaces (
    operation_id text NOT NULL,
    namespace text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT certificate_namespaces_namespace_check CHECK ((namespace ~ '^ptc-[a-z2-7]{20}$'::text))
);

CREATE TABLE certificate_order_admissions (
    id bigint NOT NULL,
    issuer text NOT NULL,
    order_id text NOT NULL,
    admitted_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

CREATE SEQUENCE certificate_order_admissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE certificate_order_admissions_id_seq OWNED BY certificate_order_admissions.id;

CREATE TABLE certificate_orders (
    id text NOT NULL,
    request_id text NOT NULL,
    operation_id text NOT NULL,
    server_id text NOT NULL,
    account_id text NOT NULL,
    claim_generation bigint NOT NULL,
    credential_generation bigint NOT NULL,
    token_hash bytea NOT NULL,
    public_key bytea NOT NULL,
    namespace text NOT NULL,
    issuer text NOT NULL,
    csr bytea NOT NULL,
    csr_hash bytea NOT NULL,
    state text NOT NULL,
    provider_url text DEFAULT ''::text NOT NULL,
    chain bytea,
    not_before timestamp with time zone,
    not_after timestamp with time zone,
    creation_attempted boolean DEFAULT false NOT NULL,
    cancel_requested boolean DEFAULT false NOT NULL,
    revocation_done boolean DEFAULT false NOT NULL,
    next_attempt_at timestamp with time zone DEFAULT now() NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    lease_token text DEFAULT ''::text NOT NULL,
    lease_until timestamp with time zone,
    error_code text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    cleanup_due_at timestamp with time zone,
    authority_check_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT certificate_orders_csr_check CHECK (((octet_length(csr) >= 100) AND (octet_length(csr) <= 8192))),
    CONSTRAINT certificate_orders_state_check CHECK ((state = ANY (ARRAY['queued'::text, 'creating'::text, 'ordering'::text, 'finalizing'::text, 'issued'::text, 'failed'::text, 'reconciliation_required'::text, 'cancelled'::text, 'revoked'::text])))
);

CREATE TABLE claim_approvals (
    operation_id text NOT NULL,
    account_id text NOT NULL,
    family_id text NOT NULL,
    access_hash bytea NOT NULL,
    account_epoch bigint NOT NULL,
    family_generation bigint NOT NULL,
    session_mode text NOT NULL,
    revision bigint NOT NULL,
    issued_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT claim_approvals_access_hash_check CHECK ((octet_length(access_hash) = 32)),
    CONSTRAINT claim_approvals_account_epoch_check CHECK ((account_epoch >= 0)),
    CONSTRAINT claim_approvals_check CHECK (((expires_at > issued_at) AND (expires_at <= (issued_at + '00:05:00'::interval)))),
    CONSTRAINT claim_approvals_family_generation_check CHECK ((family_generation >= 0)),
    CONSTRAINT claim_approvals_revision_check CHECK ((revision > 0)),
    CONSTRAINT claim_approvals_session_mode_check CHECK ((session_mode = ANY (ARRAY['native'::text, 'browser'::text])))
);

CREATE TABLE claim_cancellations (
    operation_id text NOT NULL,
    request_id text NOT NULL,
    request_digest bytea NOT NULL,
    cancelled_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT claim_cancellations_request_digest_check CHECK ((octet_length(request_digest) = 32)),
    CONSTRAINT claim_cancellations_request_id_check CHECK ((request_id ~ '^[A-Za-z0-9_-]{1,128}$'::text))
);

CREATE TABLE claim_nonces (
    id text NOT NULL,
    operation_id text NOT NULL,
    approval_revision bigint NOT NULL,
    purpose text NOT NULL,
    request_id text NOT NULL,
    request_digest bytea NOT NULL,
    nonce bytea NOT NULL,
    issued_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    CONSTRAINT claim_nonces_approval_revision_check CHECK ((approval_revision > 0)),
    CONSTRAINT claim_nonces_check CHECK (((expires_at > issued_at) AND (expires_at <= (issued_at + '00:00:30'::interval)))),
    CONSTRAINT claim_nonces_check1 CHECK (((consumed_at IS NULL) OR ((consumed_at >= issued_at) AND (consumed_at < expires_at)))),
    CONSTRAINT claim_nonces_id_check CHECK ((id ~ '^[A-Za-z0-9_-]{1,128}$'::text)),
    CONSTRAINT claim_nonces_nonce_check CHECK ((octet_length(nonce) = 32)),
    CONSTRAINT claim_nonces_purpose_check CHECK ((purpose = ANY (ARRAY['claim-finalize'::text, 'claim-result'::text]))),
    CONSTRAINT claim_nonces_request_digest_check CHECK ((octet_length(request_digest) = 32)),
    CONSTRAINT claim_nonces_request_id_check CHECK ((request_id ~ '^[A-Za-z0-9_-]{1,128}$'::text))
);

CREATE TABLE claim_operations (
    id text NOT NULL,
    server_id text NOT NULL,
    account_id text,
    public_key bytea NOT NULL,
    local_generation bigint NOT NULL,
    server_name text NOT NULL,
    owner_profile_id text,
    intent_digest bytea NOT NULL,
    state text NOT NULL,
    claim_generation bigint,
    credential_generation bigint,
    committed_token_hash bytea,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    installed_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    origin_account_id text DEFAULT ''::text NOT NULL,
    CONSTRAINT claim_operations_check CHECK (((state = 'cancelled'::text) OR ((account_id IS NOT NULL) AND (owner_profile_id IS NOT NULL)))),
    CONSTRAINT claim_operations_check1 CHECK (((state <> 'approved'::text) OR (claim_generation IS NULL))),
    CONSTRAINT claim_operations_check2 CHECK (((claim_generation IS NULL) = (credential_generation IS NULL))),
    CONSTRAINT claim_operations_check3 CHECK (((claim_generation IS NULL) = (committed_token_hash IS NULL))),
    CONSTRAINT claim_operations_check4 CHECK (((state <> ALL (ARRAY['committed_pending_install'::text, 'installed'::text])) OR (claim_generation IS NOT NULL))),
    CONSTRAINT claim_operations_check5 CHECK ((((state = 'installed'::text) = ((installed_at IS NOT NULL) AND (cancelled_at IS NULL))) OR (state = 'cancelled'::text))),
    CONSTRAINT claim_operations_check6 CHECK (((state = 'cancelled'::text) = (cancelled_at IS NOT NULL))),
    CONSTRAINT claim_operations_claim_generation_check CHECK ((claim_generation > 0)),
    CONSTRAINT claim_operations_committed_token_hash_check CHECK ((octet_length(committed_token_hash) = 32)),
    CONSTRAINT claim_operations_credential_generation_check CHECK ((credential_generation > 0)),
    CONSTRAINT claim_operations_id_check CHECK ((id ~ '^[A-Za-z0-9_-]{1,128}$'::text)),
    CONSTRAINT claim_operations_intent_digest_check CHECK ((octet_length(intent_digest) = 32)),
    CONSTRAINT claim_operations_local_generation_check CHECK ((local_generation >= 0)),
    CONSTRAINT claim_operations_public_key_check CHECK ((octet_length(public_key) = 32)),
    CONSTRAINT claim_operations_server_id_check CHECK ((server_id ~ '^[A-Za-z0-9_-]{1,128}$'::text)),
    CONSTRAINT claim_operations_server_name_check CHECK (((octet_length(server_name) >= 1) AND (octet_length(server_name) <= 120))),
    CONSTRAINT claim_operations_state_check CHECK ((state = ANY (ARRAY['approved'::text, 'committed_pending_install'::text, 'installed'::text, 'cancelled'::text])))
);

CREATE TABLE current_server_endpoints (
    server_id text NOT NULL,
    operation_id text NOT NULL,
    account_id text NOT NULL,
    claim_generation bigint NOT NULL,
    credential_generation bigint NOT NULL,
    origin text NOT NULL,
    proof_digest bytea NOT NULL,
    verified_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT current_server_endpoints_claim_generation_check CHECK ((claim_generation > 0)),
    CONSTRAINT current_server_endpoints_credential_generation_check CHECK ((credential_generation > 0)),
    CONSTRAINT current_server_endpoints_origin_check CHECK ((origin ~~ 'https://%'::text)),
    CONSTRAINT current_server_endpoints_proof_digest_check CHECK ((octet_length(proof_digest) = 32))
);

CREATE TABLE deleted_identity_markers (
    marker_key_id text NOT NULL,
    subject_digest bytea NOT NULL,
    deleted_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT deleted_identity_markers_check CHECK (((expires_at > deleted_at) AND (expires_at <= (deleted_at + '00:15:00'::interval)))),
    CONSTRAINT deleted_identity_markers_marker_key_id_check CHECK ((marker_key_id ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT deleted_identity_markers_subject_digest_check CHECK ((octet_length(subject_digest) = 32))
);

CREATE TABLE device_authorizations (
    id text NOT NULL,
    creation_hash bytea NOT NULL,
    request_hash bytea NOT NULL,
    creation_cipher bytea NOT NULL,
    device_hash bytea NOT NULL,
    user_hash bytea,
    device_name text NOT NULL,
    platform text NOT NULL,
    app_version text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    next_poll_at timestamp with time zone NOT NULL,
    interval_seconds integer DEFAULT 5 NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    approved_account_id text,
    approved_epoch bigint,
    decided_at timestamp with time zone,
    session_hash bytea,
    session_cipher bytea,
    session_expires_at timestamp with time zone,
    recovery_until timestamp with time zone,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    protocol text DEFAULT ''::text NOT NULL,
    tv_public_key text DEFAULT ''::text NOT NULL,
    client_request_id text DEFAULT ''::text NOT NULL,
    tv_grant bytea,
    session_mode text DEFAULT 'native'::text NOT NULL,
    CONSTRAINT device_authorizations_session_mode_check CHECK ((session_mode = ANY (ARRAY['native'::text, 'browser'::text]))),
    CONSTRAINT device_authorizations_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'denied'::text, 'cancelled'::text, 'consumed'::text])))
);

CREATE TABLE device_rate_limits (
    subject_hash bytea NOT NULL,
    kind text NOT NULL,
    window_start timestamp with time zone NOT NULL,
    attempts integer NOT NULL
);

CREATE TABLE direct_address_events (
    id bigint NOT NULL,
    server_id text NOT NULL,
    namespace text NOT NULL,
    address text NOT NULL,
    previous text DEFAULT ''::text NOT NULL,
    observed_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

CREATE SEQUENCE direct_address_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE direct_address_events_id_seq OWNED BY direct_address_events.id;

CREATE SEQUENCE direct_address_sequence
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

CREATE TABLE direct_addresses (
    namespace text NOT NULL,
    server_id text NOT NULL,
    ipv4 text DEFAULT ''::text NOT NULL,
    ipv6 text DEFAULT ''::text NOT NULL,
    sequence bigint NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

CREATE TABLE family_cookie_names (
    suffix text NOT NULL,
    family_id text NOT NULL,
    CONSTRAINT family_cookie_names_suffix_check CHECK ((suffix ~ '^[a-f0-9]{64}$'::text))
);

CREATE TABLE hosted_devices (
    id text NOT NULL,
    account_id text NOT NULL,
    name text NOT NULL,
    platform text NOT NULL,
    app_version text NOT NULL,
    session_hash bytea NOT NULL,
    family_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    revoked_at timestamp with time zone,
    app text DEFAULT ''::text NOT NULL,
    installation_id text DEFAULT ''::text NOT NULL,
    ip text DEFAULT ''::text NOT NULL,
    last_seen_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    trusted boolean DEFAULT false NOT NULL
);

CREATE TABLE hosted_schema_contract (
    singleton boolean NOT NULL,
    kind text NOT NULL,
    version integer NOT NULL,
    CONSTRAINT hosted_schema_contract_kind_check CHECK ((kind = 'portico-hosted'::text)),
    CONSTRAINT hosted_schema_contract_singleton_check CHECK (singleton),
    CONSTRAINT hosted_schema_contract_version_check CHECK ((version = 2))
);

CREATE TABLE identity_browser_bindings (
    binding_hash bytea NOT NULL,
    generation bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT identity_browser_bindings_binding_hash_check CHECK ((octet_length(binding_hash) = 32)),
    CONSTRAINT identity_browser_bindings_check CHECK (((expires_at > created_at) AND (expires_at <= (created_at + '24:00:00'::interval)))),
    CONSTRAINT identity_browser_bindings_generation_check CHECK ((generation >= 0))
);

CREATE TABLE identity_completion_receipts (
    transaction_id text NOT NULL,
    transaction_expires_at timestamp with time zone NOT NULL,
    binding_generation bigint NOT NULL,
    account_id text NOT NULL,
    family_id text NOT NULL,
    account_epoch bigint NOT NULL,
    family_generation bigint NOT NULL,
    ciphertext bytea NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT identity_completion_receipts_account_epoch_check CHECK ((account_epoch >= 0)),
    CONSTRAINT identity_completion_receipts_binding_generation_check CHECK ((binding_generation >= 0)),
    CONSTRAINT identity_completion_receipts_check CHECK (((expires_at > created_at) AND (expires_at <= (created_at + '00:05:00'::interval)) AND (expires_at <= transaction_expires_at))),
    CONSTRAINT identity_completion_receipts_ciphertext_check CHECK (((octet_length(ciphertext) >= 28) AND (octet_length(ciphertext) <= 32768))),
    CONSTRAINT identity_completion_receipts_family_generation_check CHECK ((family_generation >= 0))
);

CREATE TABLE identity_contact_challenges (
    id text NOT NULL,
    transaction_id text NOT NULL,
    transaction_expires_at timestamp with time zone NOT NULL,
    address text NOT NULL,
    challenge_hash bytea NOT NULL,
    revision bigint NOT NULL,
    is_current boolean DEFAULT true NOT NULL,
    issued_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    verified_at timestamp with time zone,
    attempt_count integer DEFAULT 0 NOT NULL,
    verified_source text,
    request_id text,
    CONSTRAINT identity_contact_challenges_address_check CHECK (((octet_length(address) >= 3) AND (octet_length(address) <= 320))),
    CONSTRAINT identity_contact_challenges_attempt_count_check CHECK (((attempt_count >= 0) AND (attempt_count <= 5))),
    CONSTRAINT identity_contact_challenges_challenge_hash_check CHECK ((octet_length(challenge_hash) = 32)),
    CONSTRAINT identity_contact_challenges_check CHECK (((expires_at > issued_at) AND (expires_at <= (issued_at + '00:10:00'::interval)) AND (expires_at <= transaction_expires_at))),
    CONSTRAINT identity_contact_challenges_check1 CHECK (((verified_at IS NULL) OR ((verified_at >= issued_at) AND (verified_at < expires_at)))),
    CONSTRAINT identity_contact_challenges_id_check CHECK (((octet_length(id) >= 1) AND (octet_length(id) <= 128))),
    CONSTRAINT identity_contact_challenges_revision_check CHECK ((revision > 0))
);

CREATE TABLE identity_contact_outbox (
    id text NOT NULL,
    challenge_id text NOT NULL,
    challenge_revision bigint NOT NULL,
    challenge_expires_at timestamp with time zone NOT NULL,
    ciphertext bytea,
    status text DEFAULT 'pending'::text NOT NULL,
    lease_id text,
    lease_until timestamp with time zone,
    acquired_at timestamp with time zone,
    attempts integer DEFAULT 0 NOT NULL,
    next_attempt_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    last_failure text,
    CONSTRAINT identity_contact_outbox_attempts_check CHECK (((attempts >= 0) AND (attempts <= 5))),
    CONSTRAINT identity_contact_outbox_check CHECK ((expires_at <= challenge_expires_at)),
    CONSTRAINT identity_contact_outbox_check1 CHECK ((((status = 'sending'::text) AND (lease_id IS NOT NULL) AND (lease_until IS NOT NULL) AND (acquired_at IS NOT NULL) AND (lease_until > acquired_at) AND (lease_until <= (acquired_at + '00:00:30'::interval)) AND (lease_until <= expires_at)) OR ((status <> 'sending'::text) AND (lease_id IS NULL) AND (lease_until IS NULL) AND (acquired_at IS NULL)))),
    CONSTRAINT identity_contact_outbox_check2 CHECK ((((status = ANY (ARRAY['pending'::text, 'sending'::text])) AND (ciphertext IS NOT NULL)) OR ((status = ANY (ARRAY['sent'::text, 'failed'::text, 'cancelled'::text])) AND (ciphertext IS NULL)))),
    CONSTRAINT identity_contact_outbox_ciphertext_check CHECK (((octet_length(ciphertext) >= 28) AND (octet_length(ciphertext) <= 4096))),
    CONSTRAINT identity_contact_outbox_id_check CHECK (((octet_length(id) >= 1) AND (octet_length(id) <= 128))),
    CONSTRAINT identity_contact_outbox_last_failure_check CHECK ((last_failure = ANY (ARRAY['unavailable'::text, 'rejected'::text, 'expired'::text, 'superseded'::text]))),
    CONSTRAINT identity_contact_outbox_lease_id_check CHECK (((octet_length(lease_id) >= 1) AND (octet_length(lease_id) <= 128))),
    CONSTRAINT identity_contact_outbox_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'sending'::text, 'sent'::text, 'failed'::text, 'cancelled'::text])))
);

CREATE TABLE identity_exchange_claims (
    transaction_id text NOT NULL,
    transaction_expires_at timestamp with time zone NOT NULL,
    lease_id text NOT NULL,
    transaction_revision bigint NOT NULL,
    binding_generation bigint NOT NULL,
    acquired_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    lease_until timestamp with time zone NOT NULL,
    CONSTRAINT identity_exchange_claims_binding_generation_check CHECK ((binding_generation >= 0)),
    CONSTRAINT identity_exchange_claims_check CHECK (((lease_until > acquired_at) AND (lease_until <= (acquired_at + '00:00:30'::interval)) AND (lease_until <= transaction_expires_at))),
    CONSTRAINT identity_exchange_claims_lease_id_check CHECK (((octet_length(lease_id) >= 1) AND (octet_length(lease_id) <= 128))),
    CONSTRAINT identity_exchange_claims_transaction_revision_check CHECK ((transaction_revision > 0))
);

CREATE TABLE identity_exchange_secrets (
    transaction_id text NOT NULL,
    transaction_expires_at timestamp with time zone NOT NULL,
    sealed_verifier bytea,
    sealed_code bytea,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT identity_exchange_secrets_check CHECK ((expires_at <= transaction_expires_at)),
    CONSTRAINT identity_exchange_secrets_check1 CHECK (((sealed_verifier IS NOT NULL) OR (sealed_code IS NOT NULL))),
    CONSTRAINT identity_exchange_secrets_sealed_code_check CHECK (((octet_length(sealed_code) >= 28) AND (octet_length(sealed_code) <= 16384))),
    CONSTRAINT identity_exchange_secrets_sealed_verifier_check CHECK (((octet_length(sealed_verifier) >= 28) AND (octet_length(sealed_verifier) <= 1024)))
);

CREATE TABLE identity_onboarding (
    transaction_id text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    username text,
    display_name text,
    CONSTRAINT identity_onboarding_check CHECK (((username IS NULL) = (display_name IS NULL))),
    CONSTRAINT identity_onboarding_display_name_check CHECK (((octet_length(display_name) >= 1) AND (octet_length(display_name) <= 120))),
    CONSTRAINT identity_onboarding_revision_check CHECK ((revision > 0)),
    CONSTRAINT identity_onboarding_username_check CHECK ((username ~ '^[a-z0-9][a-z0-9._-]{1,30}[a-z0-9]$'::text))
);

CREATE TABLE identity_protocol_policy (
    singleton boolean DEFAULT true NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    marker_key_id text,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT identity_protocol_policy_check CHECK (((NOT enabled) OR (marker_key_id IS NOT NULL))),
    CONSTRAINT identity_protocol_policy_marker_key_id_check CHECK ((marker_key_id ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT identity_protocol_policy_revision_check CHECK ((revision > 0)),
    CONSTRAINT identity_protocol_policy_singleton_check CHECK (singleton)
);

CREATE TABLE identity_provider_descriptors (
    id text NOT NULL,
    provider text NOT NULL,
    revision bigint NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    issuer text NOT NULL,
    audience text NOT NULL,
    callback_uri text NOT NULL,
    CONSTRAINT identity_provider_descriptors_audience_check CHECK (((octet_length(audience) >= 1) AND (octet_length(audience) <= 256))),
    CONSTRAINT identity_provider_descriptors_callback_uri_check CHECK ((((octet_length(callback_uri) >= 9) AND (octet_length(callback_uri) <= 2048)) AND (callback_uri ~~ 'https://%'::text))),
    CONSTRAINT identity_provider_descriptors_check CHECK ((((provider = 'google'::text) AND (issuer = 'https://accounts.google.com'::text)) OR ((provider = 'apple'::text) AND (issuer = 'https://appleid.apple.com'::text)))),
    CONSTRAINT identity_provider_descriptors_id_check CHECK (((octet_length(id) >= 1) AND (octet_length(id) <= 128))),
    CONSTRAINT identity_provider_descriptors_issuer_check CHECK (((octet_length(issuer) >= 1) AND (octet_length(issuer) <= 256))),
    CONSTRAINT identity_provider_descriptors_provider_check CHECK ((provider = ANY (ARRAY['google'::text, 'apple'::text]))),
    CONSTRAINT identity_provider_descriptors_revision_check CHECK ((revision > 0))
);

CREATE TABLE identity_transactions (
    id text NOT NULL,
    binding_hash bytea NOT NULL,
    binding_generation bigint NOT NULL,
    state_hash bytea NOT NULL,
    nonce_hash bytea NOT NULL,
    descriptor_id text NOT NULL,
    provider text NOT NULL,
    descriptor_revision bigint NOT NULL,
    policy_revision bigint NOT NULL,
    marker_key_id text NOT NULL,
    mode text NOT NULL,
    session_mode text NOT NULL,
    purpose text,
    intent_digest bytea,
    expected_account_id text,
    expected_family_id text,
    expected_account_epoch bigint,
    expected_family_generation bigint,
    expected_access_hash bytea,
    expected_identity_id text,
    expected_identity_revision bigint,
    revision bigint DEFAULT 1 NOT NULL,
    status text DEFAULT 'started'::text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    terminal_at timestamp with time zone,
    onboarding_token_hash bytea,
    onboarding_token_cipher bytea,
    operation_id text,
    start_request_hash bytea,
    authorization_cipher bytea,
    native_nonce_cipher bytea,
    intent_target text,
    security_intent_cipher bytea,
    mfa_challenge_id text,
    mfa_challenge_cipher bytea,
    native_apple boolean DEFAULT false NOT NULL,
    CONSTRAINT identity_security_authority CHECK ((((mode = 'login'::text) AND (purpose IS NULL) AND (intent_digest IS NULL) AND (expected_account_id IS NULL) AND (expected_family_id IS NULL) AND (expected_account_epoch IS NULL) AND (expected_family_generation IS NULL) AND (expected_access_hash IS NULL) AND (expected_identity_id IS NULL) AND (expected_identity_revision IS NULL)) OR ((mode = ANY (ARRAY['reauthenticate'::text, 'link'::text])) AND (purpose IS NOT NULL) AND (intent_digest IS NOT NULL) AND (expected_account_id IS NOT NULL) AND (expected_family_id IS NOT NULL) AND (expected_account_epoch IS NOT NULL) AND (expected_family_generation IS NOT NULL) AND (expected_access_hash IS NOT NULL) AND (((mode = 'reauthenticate'::text) AND (expected_identity_id IS NOT NULL) AND (expected_identity_revision IS NOT NULL)) OR ((mode = 'link'::text) AND (purpose = 'provider_link'::text) AND (expected_identity_id IS NULL) AND (expected_identity_revision IS NULL)))))),
    CONSTRAINT identity_security_mode CHECK ((mode = ANY (ARRAY['login'::text, 'reauthenticate'::text, 'link'::text]))),
    CONSTRAINT identity_security_onboarding CHECK (((status <> ALL (ARRAY['onboarding'::text, 'contact_pending'::text, 'completed'::text])) OR (mode = 'login'::text))),
    CONSTRAINT identity_security_proof CHECK (((status <> 'proof_issued'::text) OR (mode = ANY (ARRAY['reauthenticate'::text, 'link'::text])))),
    CONSTRAINT identity_security_purpose CHECK ((purpose = ANY (ARRAY['password_set_or_change'::text, 'account_deletion_preview'::text, 'email_change'::text, 'provider_link'::text, 'provider_unlink'::text, 'mfa_enroll'::text, 'mfa_disable'::text, 'recovery_codes'::text, 'server_transfer'::text]))),
    CONSTRAINT identity_security_session_mode CHECK ((session_mode = ANY (ARRAY['native'::text, 'browser'::text]))),
    CONSTRAINT identity_security_status CHECK ((status = ANY (ARRAY['started'::text, 'exchanging'::text, 'verified'::text, 'onboarding'::text, 'contact_pending'::text, 'completed'::text, 'proof_issued'::text, 'cancelled'::text, 'failed'::text]))),
    CONSTRAINT identity_security_terminal CHECK (((status = ANY (ARRAY['completed'::text, 'proof_issued'::text, 'cancelled'::text, 'failed'::text])) = (terminal_at IS NOT NULL))),
    CONSTRAINT identity_transactions_binding_generation_check CHECK ((binding_generation >= 0)),
    CONSTRAINT identity_transactions_check4 CHECK (((terminal_at IS NULL) OR ((terminal_at >= created_at) AND (terminal_at <= expires_at)))),
    CONSTRAINT identity_transactions_check5 CHECK (((expires_at > created_at) AND (expires_at <= (created_at + '00:10:00'::interval)))),
    CONSTRAINT identity_transactions_descriptor_revision_check CHECK ((descriptor_revision > 0)),
    CONSTRAINT identity_transactions_expected_access_hash_check CHECK ((octet_length(expected_access_hash) = 32)),
    CONSTRAINT identity_transactions_expected_account_epoch_check CHECK ((expected_account_epoch >= 0)),
    CONSTRAINT identity_transactions_expected_family_generation_check CHECK ((expected_family_generation >= 0)),
    CONSTRAINT identity_transactions_expected_identity_revision_check CHECK ((expected_identity_revision > 0)),
    CONSTRAINT identity_transactions_id_check CHECK (((octet_length(id) >= 1) AND (octet_length(id) <= 128))),
    CONSTRAINT identity_transactions_intent_digest_check CHECK ((octet_length(intent_digest) = 32)),
    CONSTRAINT identity_transactions_marker_key_id_check CHECK ((marker_key_id ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT identity_transactions_nonce_hash_check CHECK ((octet_length(nonce_hash) = 32)),
    CONSTRAINT identity_transactions_policy_revision_check CHECK ((policy_revision > 0)),
    CONSTRAINT identity_transactions_revision_check CHECK ((revision > 0)),
    CONSTRAINT identity_transactions_state_hash_check CHECK ((octet_length(state_hash) = 32))
);

CREATE TABLE identity_verified_assertions (
    transaction_id text NOT NULL,
    issuer text NOT NULL,
    subject text NOT NULL,
    audience text NOT NULL,
    provider_issued_at timestamp with time zone NOT NULL,
    verified_at timestamp with time zone NOT NULL,
    nonce_hash bytea NOT NULL,
    provider_email text,
    email_verified boolean DEFAULT false NOT NULL,
    private_relay boolean DEFAULT false NOT NULL,
    transaction_revision bigint NOT NULL,
    lease_id text NOT NULL,
    suggested_name text,
    CONSTRAINT identity_verified_assertions_audience_check CHECK (((octet_length(audience) >= 1) AND (octet_length(audience) <= 256))),
    CONSTRAINT identity_verified_assertions_check CHECK (((NOT email_verified) OR (provider_email IS NOT NULL))),
    CONSTRAINT identity_verified_assertions_check1 CHECK (((NOT private_relay) OR (provider_email IS NOT NULL))),
    CONSTRAINT identity_verified_assertions_issuer_check CHECK (((octet_length(issuer) >= 1) AND (octet_length(issuer) <= 256))),
    CONSTRAINT identity_verified_assertions_lease_id_check CHECK (((octet_length(lease_id) >= 1) AND (octet_length(lease_id) <= 128))),
    CONSTRAINT identity_verified_assertions_nonce_hash_check CHECK ((octet_length(nonce_hash) = 32)),
    CONSTRAINT identity_verified_assertions_provider_email_check CHECK (((octet_length(provider_email) >= 3) AND (octet_length(provider_email) <= 320))),
    CONSTRAINT identity_verified_assertions_subject_check CHECK (((octet_length(subject) >= 1) AND (octet_length(subject) <= 255))),
    CONSTRAINT identity_verified_assertions_transaction_revision_check CHECK ((transaction_revision > 0))
);

CREATE TABLE invitations (
    id text NOT NULL,
    server_id text NOT NULL,
    owner_id text NOT NULL,
    recipient_id text,
    operation_id text NOT NULL,
    request_hash bytea NOT NULL,
    allowed_libraries text[] NOT NULL,
    expected_policy_revision bigint NOT NULL,
    expected_local_revision bigint NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    accepted_profile_id text,
    accepted_policy_revision bigint,
    accepted_local_revision bigint,
    recipient_kind text DEFAULT 'username'::text NOT NULL,
    recipient_email text,
    delivery_hash bytea,
    delivery_revision bigint DEFAULT 1 NOT NULL,
    allow_subprofiles boolean DEFAULT false NOT NULL,
    CONSTRAINT invitations_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'accepted'::text, 'cancelled'::text, 'revoked'::text, 'declined'::text])))
);

CREATE TABLE membership_operation_receipts (
    account_id text NOT NULL,
    family_id text NOT NULL,
    operation_id text NOT NULL,
    kind text NOT NULL,
    request_hash bytea NOT NULL,
    result_ciphertext bytea NOT NULL,
    expires_at timestamp with time zone NOT NULL
);

CREATE TABLE memberships (
    server_id text NOT NULL,
    account_id text NOT NULL,
    profile_id text NOT NULL,
    role text NOT NULL,
    allowed_libraries text[] DEFAULT '{}'::text[] NOT NULL,
    revoked boolean DEFAULT false NOT NULL,
    allow_subprofiles boolean DEFAULT false NOT NULL,
    all_libraries boolean DEFAULT false NOT NULL,
    CONSTRAINT memberships_role_check CHECK ((role = ANY (ARRAY['owner'::text, 'member'::text])))
);

CREATE TABLE mutation_receipts (
    server_id text NOT NULL,
    operation_id text NOT NULL,
    kind text NOT NULL,
    request_hash bytea NOT NULL,
    local_revision bigint NOT NULL,
    policy_revision bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE networking_route_candidates (
    server_id text NOT NULL,
    operation_id text NOT NULL,
    generation bigint NOT NULL,
    origin text NOT NULL,
    class text NOT NULL,
    state text NOT NULL,
    verified_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT networking_route_candidates_class_check CHECK ((class = ANY (ARRAY['lan'::text, 'public'::text, 'manual'::text]))),
    CONSTRAINT networking_route_candidates_generation_check CHECK ((generation > 0)),
    CONSTRAINT networking_route_candidates_state_check CHECK ((state = ANY (ARRAY['reported'::text, 'verified'::text, 'failed'::text, 'identity_mismatch'::text])))
);

CREATE TABLE networking_route_state (
    server_id text NOT NULL,
    operation_id text NOT NULL,
    claim_generation bigint NOT NULL,
    credential_generation bigint NOT NULL,
    generation bigint NOT NULL,
    observation_digest bytea NOT NULL,
    remote_enabled boolean NOT NULL,
    observed_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT networking_route_state_generation_check CHECK ((generation > 0)),
    CONSTRAINT networking_route_state_observation_digest_check CHECK ((octet_length(observation_digest) = 32))
);

CREATE TABLE profile_deletions (
    id text NOT NULL,
    account_id text NOT NULL,
    profile_id text NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

CREATE TABLE profile_erasure_jobs (
    id text NOT NULL,
    deletion_id text NOT NULL,
    server_id text NOT NULL,
    account_id text NOT NULL,
    profile_id text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    policy_revision bigint NOT NULL,
    acknowledged_at timestamp with time zone
);

CREATE TABLE profile_server_policies (
    account_id text NOT NULL,
    profile_id text NOT NULL,
    server_id text NOT NULL,
    allowed_libraries text[] NOT NULL
);

CREATE TABLE profile_trusted_selections (
    token_hash bytea NOT NULL,
    account_id text NOT NULL,
    profile_id text NOT NULL,
    server_id text NOT NULL,
    installation_id text NOT NULL,
    pin_revision bigint NOT NULL,
    profile_revision bigint NOT NULL,
    trust_revision bigint NOT NULL,
    policy_revision bigint NOT NULL,
    auth_epoch bigint NOT NULL,
    operation_id text NOT NULL,
    expires_at timestamp with time zone NOT NULL
);

CREATE TABLE profiles (
    id text NOT NULL,
    account_id text NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    is_primary boolean DEFAULT false NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    art text DEFAULT 'blue'::text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    pin_revision bigint DEFAULT 1 NOT NULL,
    authorization_revision bigint DEFAULT 1 NOT NULL,
    trust_revision bigint DEFAULT 1 NOT NULL,
    pin_hash bytea,
    pin_attempts integer DEFAULT 0 NOT NULL,
    pin_locked_until timestamp with time zone,
    deleted_at timestamp with time zone
);

CREATE TABLE refresh_receipts (
    family_id text NOT NULL,
    request_id text NOT NULL,
    input_hash bytea NOT NULL,
    output_hash bytea NOT NULL,
    access_hash bytea NOT NULL,
    ciphertext bytea,
    expires_at timestamp with time zone NOT NULL
);

CREATE TABLE refresh_tokens (
    token_hash bytea NOT NULL,
    family_id text NOT NULL,
    generation bigint NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE restrictions (
    server_id text NOT NULL,
    account_id text NOT NULL,
    profile_id text NOT NULL,
    local_revision bigint NOT NULL,
    allowed_libraries text[] NOT NULL,
    revoked boolean NOT NULL
);

CREATE TABLE security_audit (
    id bigint NOT NULL,
    actor_id text NOT NULL,
    action text NOT NULL,
    target_id text NOT NULL,
    revision bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE SEQUENCE security_audit_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE security_audit_id_seq OWNED BY security_audit.id;

CREATE TABLE server_ownership_transfers (
    server_id text NOT NULL,
    operation_id text NOT NULL,
    claim_operation_id text NOT NULL,
    previous_account_id text NOT NULL,
    new_account_id text NOT NULL,
    policy_revision bigint NOT NULL,
    transferred_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

CREATE TABLE servers (
    id text NOT NULL,
    owner_account_id text,
    name text NOT NULL,
    base_url text,
    public_key text NOT NULL,
    token_hash bytea,
    token_ciphertext bytea,
    claim_generation bigint NOT NULL,
    credential_generation bigint NOT NULL,
    revoked_at timestamp with time zone,
    policy_revision bigint DEFAULT 1 NOT NULL,
    local_revision bigint DEFAULT 0 NOT NULL,
    policy_issued_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    last_heartbeat_at timestamp with time zone,
    presence_expires_at timestamp with time zone,
    endpoint_verified boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT servers_check CHECK ((((revoked_at IS NULL) AND (owner_account_id IS NOT NULL) AND (token_hash IS NOT NULL) AND (token_ciphertext IS NOT NULL)) OR ((revoked_at IS NOT NULL) AND (owner_account_id IS NULL) AND (token_hash IS NULL) AND (token_ciphertext IS NULL) AND (base_url IS NULL) AND (NOT endpoint_verified)))),
    CONSTRAINT servers_check1 CHECK (((NOT endpoint_verified) OR ((base_url IS NOT NULL) AND (base_url ~~ 'https://%'::text)))),
    CONSTRAINT servers_claim_generation_check CHECK ((claim_generation > 0)),
    CONSTRAINT servers_credential_generation_check CHECK ((credential_generation > 0)),
    CONSTRAINT servers_id_check CHECK ((id ~ '^srv_[A-Za-z0-9_-]{43}$'::text)),
    CONSTRAINT servers_local_revision_check CHECK ((local_revision >= 0)),
    CONSTRAINT servers_policy_revision_check CHECK ((policy_revision > 0)),
    CONSTRAINT servers_public_key_check CHECK ((public_key ~ '^[A-Za-z0-9_-]{43}$'::text)),
    CONSTRAINT servers_token_ciphertext_check CHECK (((token_ciphertext IS NULL) OR ((octet_length(token_ciphertext) >= 28) AND (octet_length(token_ciphertext) <= 4096)))),
    CONSTRAINT servers_token_hash_check CHECK (((token_hash IS NULL) OR (octet_length(token_hash) = 32)))
);

CREATE TABLE session_families (
    id text NOT NULL,
    account_id text NOT NULL,
    mode text NOT NULL,
    current_refresh_hash bytea NOT NULL,
    generation bigint DEFAULT 0 NOT NULL,
    account_epoch bigint NOT NULL,
    absolute_expires_at timestamp with time zone NOT NULL,
    idle_expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    revoke_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT session_families_mode_check CHECK ((mode = ANY (ARRAY['native'::text, 'browser'::text])))
);

CREATE TABLE wake_outbox (
    server_id text NOT NULL,
    kind text NOT NULL,
    target_revision bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE wake_state (
    server_id text NOT NULL,
    kind text NOT NULL,
    desired_revision bigint DEFAULT 0 NOT NULL,
    acked_revision bigint DEFAULT 0 NOT NULL,
    lease_token text,
    lease_until timestamp with time zone,
    next_attempt_at timestamp with time zone DEFAULT now() NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    last_status integer
);

ALTER TABLE ONLY appstore_notification_failures ALTER COLUMN id SET DEFAULT nextval('appstore_notification_failures_id_seq'::regclass);

ALTER TABLE ONLY certificate_order_admissions ALTER COLUMN id SET DEFAULT nextval('certificate_order_admissions_id_seq'::regclass);

ALTER TABLE ONLY direct_address_events ALTER COLUMN id SET DEFAULT nextval('direct_address_events_id_seq'::regclass);

ALTER TABLE ONLY security_audit ALTER COLUMN id SET DEFAULT nextval('security_audit_id_seq'::regclass);

INSERT INTO account_identity_schema VALUES (true, 1);

INSERT INTO account_security_schema VALUES (true, 1);

INSERT INTO apple_notification_schema VALUES (true, 1);

INSERT INTO hosted_schema_contract VALUES (true, 'portico-hosted', 2);

INSERT INTO identity_protocol_policy VALUES (true, 1, false, NULL, '2026-09-22 23:43:20.052088-03');

SELECT pg_catalog.setval('appstore_notification_failures_id_seq', 1, false);

SELECT pg_catalog.setval('certificate_order_admissions_id_seq', 1, false);

SELECT pg_catalog.setval('direct_address_events_id_seq', 1, false);

SELECT pg_catalog.setval('direct_address_sequence', 1, false);

SELECT pg_catalog.setval('security_audit_id_seq', 1, false);

ALTER TABLE ONLY account_contact_emails
    ADD CONSTRAINT account_contact_emails_account_id_address_key UNIQUE (account_id, address);

ALTER TABLE ONLY account_contact_emails
    ADD CONSTRAINT account_contact_emails_pkey PRIMARY KEY (id);

ALTER TABLE ONLY account_deletions
    ADD CONSTRAINT account_deletions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY account_deletions
    ADD CONSTRAINT account_deletions_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY account_email_challenges
    ADD CONSTRAINT account_email_challenges_account_id_purpose_operation_id_key UNIQUE (account_id, purpose, operation_id);

ALTER TABLE ONLY account_email_challenges
    ADD CONSTRAINT account_email_challenges_code_hash_key UNIQUE (code_hash);

ALTER TABLE ONLY account_email_challenges
    ADD CONSTRAINT account_email_challenges_pkey PRIMARY KEY (id);

ALTER TABLE ONLY account_erasure_jobs
    ADD CONSTRAINT account_erasure_jobs_deletion_id_server_id_key UNIQUE (deletion_id, server_id);

ALTER TABLE ONLY account_erasure_jobs
    ADD CONSTRAINT account_erasure_jobs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY account_external_identities
    ADD CONSTRAINT account_external_identities_id_account_id_key UNIQUE (id, account_id);

ALTER TABLE ONLY account_external_identities
    ADD CONSTRAINT account_external_identities_issuer_subject_key UNIQUE (issuer, subject);

ALTER TABLE ONLY account_external_identities
    ADD CONSTRAINT account_external_identities_pkey PRIMARY KEY (id);

ALTER TABLE ONLY account_identity_schema
    ADD CONSTRAINT account_identity_schema_pkey PRIMARY KEY (singleton);

ALTER TABLE ONLY account_mail_outbox
    ADD CONSTRAINT account_mail_outbox_pkey PRIMARY KEY (id);

ALTER TABLE ONLY account_mfa_challenges
    ADD CONSTRAINT account_mfa_challenges_pkey PRIMARY KEY (id);

ALTER TABLE ONLY account_mfa_challenges
    ADD CONSTRAINT account_mfa_challenges_proof_hash_key UNIQUE (proof_hash);

ALTER TABLE ONLY account_mfa
    ADD CONSTRAINT account_mfa_pkey PRIMARY KEY (account_id);

ALTER TABLE ONLY account_password_logins
    ADD CONSTRAINT account_password_logins_pkey PRIMARY KEY (binding_hash, request_id);

ALTER TABLE ONLY account_preferences
    ADD CONSTRAINT account_preferences_pkey PRIMARY KEY (account_id);

ALTER TABLE ONLY account_recent_proofs
    ADD CONSTRAINT account_recent_proofs_pkey PRIMARY KEY (proof_hash);

ALTER TABLE ONLY account_recovery_codes
    ADD CONSTRAINT account_recovery_codes_pkey PRIMARY KEY (account_id, code_hash);

ALTER TABLE ONLY account_registrations
    ADD CONSTRAINT account_registrations_binding_hash_request_id_key UNIQUE (binding_hash, request_id);

ALTER TABLE ONLY account_registrations
    ADD CONSTRAINT account_registrations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY account_security_limits
    ADD CONSTRAINT account_security_limits_pkey PRIMARY KEY (subject_hash, kind);

ALTER TABLE ONLY account_security_receipts
    ADD CONSTRAINT account_security_receipts_pkey PRIMARY KEY (account_id, operation_id, purpose);

ALTER TABLE ONLY account_security_schema
    ADD CONSTRAINT account_security_schema_pkey PRIMARY KEY (singleton);

ALTER TABLE ONLY account_sessions
    ADD CONSTRAINT account_sessions_pkey PRIMARY KEY (token_hash);

ALTER TABLE ONLY accounts
    ADD CONSTRAINT accounts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY accounts
    ADD CONSTRAINT accounts_username_key UNIQUE (username);

ALTER TABLE ONLY apple_account_notifications
    ADD CONSTRAINT apple_account_notifications_pkey PRIMARY KEY (id_hash);

ALTER TABLE ONLY apple_identity_events
    ADD CONSTRAINT apple_identity_events_pkey PRIMARY KEY (subject_hash);

ALTER TABLE ONLY apple_notification_schema
    ADD CONSTRAINT apple_notification_schema_pkey PRIMARY KEY (singleton);

ALTER TABLE ONLY apple_relay_events
    ADD CONSTRAINT apple_relay_events_pkey PRIMARY KEY (subject_hash, email_hash);

ALTER TABLE ONLY appstore_notification_failures
    ADD CONSTRAINT appstore_notification_failures_pkey PRIMARY KEY (id);

ALTER TABLE ONLY appstore_notifications
    ADD CONSTRAINT appstore_notifications_pkey PRIMARY KEY (uuid);

ALTER TABLE ONLY attachment_tickets
    ADD CONSTRAINT attachment_tickets_pkey PRIMARY KEY (token_hash);

ALTER TABLE ONLY certificate_acme_accounts
    ADD CONSTRAINT certificate_acme_accounts_pkey PRIMARY KEY (issuer);

ALTER TABLE ONLY certificate_challenges
    ADD CONSTRAINT certificate_challenges_namespace_txt_value_key UNIQUE (namespace, txt_value);

ALTER TABLE ONLY certificate_challenges
    ADD CONSTRAINT certificate_challenges_pkey PRIMARY KEY (order_id, authorization_url);

ALTER TABLE ONLY certificate_namespaces
    ADD CONSTRAINT certificate_namespaces_namespace_key UNIQUE (namespace);

ALTER TABLE ONLY certificate_namespaces
    ADD CONSTRAINT certificate_namespaces_pkey PRIMARY KEY (operation_id);

ALTER TABLE ONLY certificate_order_admissions
    ADD CONSTRAINT certificate_order_admissions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY certificate_orders
    ADD CONSTRAINT certificate_orders_operation_id_request_id_key UNIQUE (operation_id, request_id);

ALTER TABLE ONLY certificate_orders
    ADD CONSTRAINT certificate_orders_pkey PRIMARY KEY (id);

ALTER TABLE ONLY claim_approvals
    ADD CONSTRAINT claim_approvals_pkey PRIMARY KEY (operation_id);

ALTER TABLE ONLY claim_cancellations
    ADD CONSTRAINT claim_cancellations_pkey PRIMARY KEY (operation_id);

ALTER TABLE ONLY claim_nonces
    ADD CONSTRAINT claim_nonces_operation_id_purpose_request_id_key UNIQUE (operation_id, purpose, request_id);

ALTER TABLE ONLY claim_nonces
    ADD CONSTRAINT claim_nonces_pkey PRIMARY KEY (id);

ALTER TABLE ONLY claim_operations
    ADD CONSTRAINT claim_operations_id_account_id_key UNIQUE (id, account_id);

ALTER TABLE ONLY claim_operations
    ADD CONSTRAINT claim_operations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY current_server_endpoints
    ADD CONSTRAINT current_server_endpoints_pkey PRIMARY KEY (server_id);

ALTER TABLE ONLY deleted_identity_markers
    ADD CONSTRAINT deleted_identity_markers_pkey PRIMARY KEY (marker_key_id, subject_digest);

ALTER TABLE ONLY device_authorizations
    ADD CONSTRAINT device_authorizations_creation_hash_key UNIQUE (creation_hash);

ALTER TABLE ONLY device_authorizations
    ADD CONSTRAINT device_authorizations_device_hash_key UNIQUE (device_hash);

ALTER TABLE ONLY device_authorizations
    ADD CONSTRAINT device_authorizations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY device_authorizations
    ADD CONSTRAINT device_authorizations_user_hash_key UNIQUE (user_hash);

ALTER TABLE ONLY device_rate_limits
    ADD CONSTRAINT device_rate_limits_pkey PRIMARY KEY (subject_hash, kind);

ALTER TABLE ONLY direct_address_events
    ADD CONSTRAINT direct_address_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY direct_addresses
    ADD CONSTRAINT direct_addresses_pkey PRIMARY KEY (namespace);

ALTER TABLE ONLY direct_addresses
    ADD CONSTRAINT direct_addresses_sequence_key UNIQUE (sequence);

ALTER TABLE ONLY family_cookie_names
    ADD CONSTRAINT family_cookie_names_family_id_key UNIQUE (family_id);

ALTER TABLE ONLY family_cookie_names
    ADD CONSTRAINT family_cookie_names_pkey PRIMARY KEY (suffix);

ALTER TABLE ONLY hosted_devices
    ADD CONSTRAINT hosted_devices_pkey PRIMARY KEY (id);

ALTER TABLE ONLY hosted_schema_contract
    ADD CONSTRAINT hosted_schema_contract_pkey PRIMARY KEY (singleton);

ALTER TABLE ONLY identity_browser_bindings
    ADD CONSTRAINT identity_browser_bindings_pkey PRIMARY KEY (binding_hash);

ALTER TABLE ONLY identity_completion_receipts
    ADD CONSTRAINT identity_completion_receipts_pkey PRIMARY KEY (transaction_id);

ALTER TABLE ONLY identity_contact_challenges
    ADD CONSTRAINT identity_contact_challenges_challenge_hash_key UNIQUE (challenge_hash);

ALTER TABLE ONLY identity_contact_challenges
    ADD CONSTRAINT identity_contact_challenges_id_revision_expires_at_key UNIQUE (id, revision, expires_at);

ALTER TABLE ONLY identity_contact_challenges
    ADD CONSTRAINT identity_contact_challenges_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity_contact_outbox
    ADD CONSTRAINT identity_contact_outbox_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity_exchange_claims
    ADD CONSTRAINT identity_exchange_claims_lease_id_key UNIQUE (lease_id);

ALTER TABLE ONLY identity_exchange_claims
    ADD CONSTRAINT identity_exchange_claims_pkey PRIMARY KEY (transaction_id);

ALTER TABLE ONLY identity_exchange_secrets
    ADD CONSTRAINT identity_exchange_secrets_pkey PRIMARY KEY (transaction_id);

ALTER TABLE ONLY identity_onboarding
    ADD CONSTRAINT identity_onboarding_pkey PRIMARY KEY (transaction_id);

ALTER TABLE ONLY identity_protocol_policy
    ADD CONSTRAINT identity_protocol_policy_pkey PRIMARY KEY (singleton);

ALTER TABLE ONLY identity_provider_descriptors
    ADD CONSTRAINT identity_provider_descriptors_id_provider_key UNIQUE (id, provider);

ALTER TABLE ONLY identity_provider_descriptors
    ADD CONSTRAINT identity_provider_descriptors_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity_transactions
    ADD CONSTRAINT identity_transactions_id_expires_at_key UNIQUE (id, expires_at);

ALTER TABLE ONLY identity_transactions
    ADD CONSTRAINT identity_transactions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY identity_transactions
    ADD CONSTRAINT identity_transactions_state_hash_key UNIQUE (state_hash);

ALTER TABLE ONLY identity_verified_assertions
    ADD CONSTRAINT identity_verified_assertions_pkey PRIMARY KEY (transaction_id);

ALTER TABLE ONLY invitations
    ADD CONSTRAINT invitations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY invitations
    ADD CONSTRAINT invitations_server_id_operation_id_key UNIQUE (server_id, operation_id);

ALTER TABLE ONLY membership_operation_receipts
    ADD CONSTRAINT membership_operation_receipts_pkey PRIMARY KEY (account_id, family_id, operation_id);

ALTER TABLE ONLY memberships
    ADD CONSTRAINT memberships_pkey PRIMARY KEY (server_id, account_id, profile_id);

ALTER TABLE ONLY mutation_receipts
    ADD CONSTRAINT mutation_receipts_pkey PRIMARY KEY (server_id, operation_id);

ALTER TABLE ONLY networking_route_candidates
    ADD CONSTRAINT networking_route_candidates_pkey PRIMARY KEY (server_id, generation, origin);

ALTER TABLE ONLY networking_route_state
    ADD CONSTRAINT networking_route_state_pkey PRIMARY KEY (server_id);

ALTER TABLE ONLY profile_deletions
    ADD CONSTRAINT profile_deletions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY profile_deletions
    ADD CONSTRAINT profile_deletions_profile_id_key UNIQUE (profile_id);

ALTER TABLE ONLY profile_erasure_jobs
    ADD CONSTRAINT profile_erasure_jobs_deletion_id_server_id_key UNIQUE (deletion_id, server_id);

ALTER TABLE ONLY profile_erasure_jobs
    ADD CONSTRAINT profile_erasure_jobs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY profile_server_policies
    ADD CONSTRAINT profile_server_policies_pkey PRIMARY KEY (account_id, profile_id, server_id);

ALTER TABLE ONLY profile_trusted_selections
    ADD CONSTRAINT profile_trusted_selections_account_id_profile_id_server_id__key UNIQUE (account_id, profile_id, server_id, installation_id);

ALTER TABLE ONLY profile_trusted_selections
    ADD CONSTRAINT profile_trusted_selections_pkey PRIMARY KEY (token_hash);

ALTER TABLE ONLY profiles
    ADD CONSTRAINT profiles_pkey PRIMARY KEY (id);

ALTER TABLE ONLY refresh_receipts
    ADD CONSTRAINT refresh_receipts_pkey PRIMARY KEY (family_id, request_id);

ALTER TABLE ONLY refresh_tokens
    ADD CONSTRAINT refresh_tokens_family_id_generation_key UNIQUE (family_id, generation);

ALTER TABLE ONLY refresh_tokens
    ADD CONSTRAINT refresh_tokens_pkey PRIMARY KEY (token_hash);

ALTER TABLE ONLY restrictions
    ADD CONSTRAINT restrictions_pkey PRIMARY KEY (server_id, account_id, profile_id);

ALTER TABLE ONLY security_audit
    ADD CONSTRAINT security_audit_pkey PRIMARY KEY (id);

ALTER TABLE ONLY server_ownership_transfers
    ADD CONSTRAINT server_ownership_transfers_pkey PRIMARY KEY (server_id, operation_id);

ALTER TABLE ONLY servers
    ADD CONSTRAINT servers_id_claim_generation_credential_generation_key UNIQUE (id, claim_generation, credential_generation);

ALTER TABLE ONLY servers
    ADD CONSTRAINT servers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY servers
    ADD CONSTRAINT servers_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY session_families
    ADD CONSTRAINT session_families_id_account_id_key UNIQUE (id, account_id);

ALTER TABLE ONLY session_families
    ADD CONSTRAINT session_families_pkey PRIMARY KEY (id);

ALTER TABLE ONLY wake_outbox
    ADD CONSTRAINT wake_outbox_pkey PRIMARY KEY (server_id, kind, target_revision);

ALTER TABLE ONLY wake_state
    ADD CONSTRAINT wake_state_pkey PRIMARY KEY (server_id, kind);

CREATE UNIQUE INDEX account_contact_one_primary ON account_contact_emails USING btree (account_id) WHERE is_primary;

CREATE INDEX account_deletions_account ON account_deletions USING btree (account_id);

CREATE INDEX account_deletions_expiry ON account_deletions USING btree (expires_at) WHERE (accepted_at IS NULL);

CREATE INDEX account_deletions_receipt_expiry ON account_deletions USING btree (receipt_expires_at) WHERE (accepted_at IS NOT NULL);

CREATE INDEX account_email_challenge_expiry ON account_email_challenges USING btree (expires_at, id);

CREATE INDEX account_email_challenge_live ON account_email_challenges USING btree (account_id, purpose) WHERE ((consumed_at IS NULL) AND (superseded_at IS NULL));

CREATE INDEX account_erasure_authority ON account_erasure_jobs USING btree (server_id, claim_operation_id, claim_generation, credential_generation) WHERE (acknowledged_at IS NULL);

CREATE INDEX account_erasure_deletion ON account_erasure_jobs USING btree (deletion_id);

CREATE INDEX account_erasure_pending ON account_erasure_jobs USING btree (server_id, id) WHERE (acknowledged_at IS NULL);

CREATE INDEX account_external_identity_account ON account_external_identities USING btree (account_id, id);

CREATE INDEX account_mail_due ON account_mail_outbox USING btree (next_attempt_at, id) WHERE (status = 'pending'::text);

CREATE INDEX account_mail_expiry ON account_mail_outbox USING btree (expires_at, id);

CREATE INDEX account_mail_lease ON account_mail_outbox USING btree (lease_until, id) WHERE (status = 'sending'::text);

CREATE INDEX account_mfa_challenge_expiry ON account_mfa_challenges USING btree (expires_at, id);

CREATE INDEX account_password_login_expiry ON account_password_logins USING btree (expires_at);

CREATE INDEX account_recent_proof_expiry ON account_recent_proofs USING btree (expires_at, proof_hash);

CREATE INDEX account_recent_proof_live ON account_recent_proofs USING btree (account_id, created_at, proof_hash) WHERE (consumed_at IS NULL);

CREATE INDEX account_registration_binding ON account_registrations USING btree (binding_hash, created_at DESC);

CREATE INDEX account_registration_expiry ON account_registrations USING btree (expires_at, id);

CREATE INDEX account_security_limit_expiry ON account_security_limits USING btree (window_start);

CREATE INDEX account_security_receipt_expiry ON account_security_receipts USING btree (expires_at, account_id, operation_id);

CREATE INDEX account_security_receipt_lookup ON account_security_receipts USING btree (access_hash, operation_id, purpose);

CREATE INDEX account_sessions_expiry ON account_sessions USING btree (expires_at);

CREATE INDEX account_sessions_family ON account_sessions USING btree (family_id);

CREATE INDEX apple_relay_disabled ON apple_relay_events USING btree (email_hash) WHERE disabled;

CREATE INDEX appstore_notification_failure_recent ON appstore_notification_failures USING btree (observed_at DESC);

CREATE INDEX appstore_notifications_account ON appstore_notifications USING btree (account_id, received_at DESC) WHERE (account_id IS NOT NULL);

CREATE INDEX appstore_notifications_transaction ON appstore_notifications USING btree (original_transaction_id, received_at DESC);

CREATE INDEX attachment_expiry ON attachment_tickets USING btree (expires_at);

CREATE INDEX certificate_account_orders ON certificate_orders USING btree (account_id, created_at DESC);

CREATE INDEX certificate_active_work ON certificate_orders USING btree (issuer, next_attempt_at, created_at, id) WHERE ((state = ANY (ARRAY['queued'::text, 'creating'::text, 'ordering'::text, 'finalizing'::text])) OR ((state = 'failed'::text) AND (NOT revocation_done)) OR (cancel_requested AND (NOT revocation_done) AND (state <> 'reconciliation_required'::text)));

CREATE INDEX certificate_authority_work ON certificate_orders USING btree (issuer, authority_check_at, id) WHERE ((state = ANY (ARRAY['issued'::text, 'reconciliation_required'::text])) AND (NOT cancel_requested));

CREATE INDEX certificate_claim_orders ON certificate_orders USING btree (operation_id, created_at DESC);

CREATE INDEX certificate_cleanup_work ON certificate_orders USING btree (issuer, GREATEST(next_attempt_at, cleanup_due_at), created_at, id) WHERE ((cleanup_due_at IS NOT NULL) AND (state = ANY (ARRAY['issued'::text, 'failed'::text, 'cancelled'::text, 'revoked'::text, 'reconciliation_required'::text])));

CREATE INDEX certificate_due ON certificate_orders USING btree (issuer, next_attempt_at, created_at);

CREATE UNIQUE INDEX certificate_one_active ON certificate_orders USING btree (operation_id) WHERE (state = ANY (ARRAY['queued'::text, 'creating'::text, 'ordering'::text, 'finalizing'::text, 'reconciliation_required'::text]));

CREATE INDEX certificate_order_admissions_window ON certificate_order_admissions USING btree (issuer, admitted_at DESC);

CREATE INDEX certificate_pending_challenges ON certificate_challenges USING btree (order_id, cleanup_after) WHERE (NOT cleanup_done);

CREATE UNIQUE INDEX certificate_provider_order ON certificate_orders USING btree (issuer, provider_url) WHERE (provider_url <> ''::text);

CREATE INDEX certificate_quarantine_work ON certificate_orders USING btree (issuer, next_attempt_at, created_at, id) WHERE ((state = 'reconciliation_required'::text) AND (NOT cancel_requested));

CREATE INDEX certificate_server_orders ON certificate_orders USING btree (server_id, created_at DESC);

CREATE INDEX claim_nonce_operation ON claim_nonces USING btree (operation_id, issued_at, id);

CREATE INDEX claim_operation_account ON claim_operations USING btree (account_id, created_at, id);

CREATE UNIQUE INDEX claim_operation_committed_server ON claim_operations USING btree (server_id) WHERE (state = ANY (ARRAY['committed_pending_install'::text, 'installed'::text]));

CREATE INDEX deleted_identity_marker_expiry ON deleted_identity_markers USING btree (expires_at, marker_key_id, subject_digest);

CREATE INDEX device_authorization_expiry ON device_authorizations USING btree (expires_at);

CREATE INDEX device_rate_expiry ON device_rate_limits USING btree (window_start);

CREATE INDEX device_setup_recovery ON device_authorizations USING btree (recovery_until) WHERE (tv_grant IS NOT NULL);

CREATE INDEX direct_address_events_server ON direct_address_events USING btree (server_id, observed_at DESC);

CREATE INDEX hosted_devices_account ON hosted_devices USING btree (account_id);

CREATE INDEX hosted_devices_recent ON hosted_devices USING btree (account_id, last_seen_at DESC) WHERE (revoked_at IS NULL);

CREATE INDEX identity_assertion_subject ON identity_verified_assertions USING btree (issuer, subject, transaction_id);

CREATE INDEX identity_binding_expiry ON identity_browser_bindings USING btree (expires_at, binding_hash);

CREATE INDEX identity_contact_challenge_expiry ON identity_contact_challenges USING btree (expires_at, id);

CREATE UNIQUE INDEX identity_contact_current_challenge ON identity_contact_challenges USING btree (transaction_id) WHERE is_current;

CREATE UNIQUE INDEX identity_contact_request ON identity_contact_challenges USING btree (transaction_id, request_id);

CREATE INDEX identity_exchange_lease_expiry ON identity_exchange_claims USING btree (lease_until, transaction_id);

CREATE INDEX identity_outbox_due ON identity_contact_outbox USING btree (next_attempt_at, id) WHERE (status = 'pending'::text);

CREATE INDEX identity_outbox_expiry ON identity_contact_outbox USING btree (expires_at, id);

CREATE INDEX identity_outbox_lease ON identity_contact_outbox USING btree (lease_until, id) WHERE (status = 'sending'::text);

CREATE UNIQUE INDEX identity_provider_one_enabled ON identity_provider_descriptors USING btree (provider) WHERE enabled;

CREATE INDEX identity_receipt_account ON identity_completion_receipts USING btree (account_id, transaction_id);

CREATE INDEX identity_receipt_expiry ON identity_completion_receipts USING btree (expires_at, transaction_id);

CREATE UNIQUE INDEX identity_start_request ON identity_transactions USING btree (binding_hash, operation_id) WHERE (operation_id IS NOT NULL);

CREATE INDEX identity_transaction_account ON identity_transactions USING btree (expected_account_id, id) WHERE (expected_account_id IS NOT NULL);

CREATE INDEX identity_transaction_binding_live ON identity_transactions USING btree (binding_hash, created_at, id) WHERE (terminal_at IS NULL);

CREATE INDEX identity_transaction_expiry ON identity_transactions USING btree (expires_at, id);

CREATE UNIQUE INDEX invitations_delivery ON invitations USING btree (delivery_hash) WHERE (delivery_hash IS NOT NULL);

CREATE INDEX invitations_recipient ON invitations USING btree (recipient_id, expires_at) WHERE (status = 'pending'::text);

CREATE UNIQUE INDEX membership_one_owner ON memberships USING btree (server_id) WHERE (role = 'owner'::text);

CREATE INDEX memberships_account ON memberships USING btree (account_id, server_id) WHERE (NOT revoked);

CREATE INDEX profiles_account ON profiles USING btree (account_id, id);

CREATE INDEX profiles_active_order ON profiles USING btree (account_id, "position", id) WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX profiles_primary ON profiles USING btree (account_id) WHERE is_primary;

CREATE INDEX refresh_receipts_expiry ON refresh_receipts USING btree (expires_at) WHERE (ciphertext IS NOT NULL);

CREATE INDEX session_families_account ON session_families USING btree (account_id);

CREATE INDEX session_families_expiry ON session_families USING btree (absolute_expires_at);

CREATE INDEX session_families_review_page ON session_families USING btree (account_id, created_at DESC, id DESC);

CREATE UNIQUE INDEX verified_server_endpoint ON servers USING btree (base_url) WHERE endpoint_verified;

CREATE INDEX wake_due ON wake_state USING btree (next_attempt_at) WHERE (desired_revision > acked_revision);

CREATE TRIGGER identity_link_revision BEFORE UPDATE ON account_external_identities FOR EACH ROW EXECUTE FUNCTION identity_link_revision_guard();

CREATE TRIGGER identity_policy_revision BEFORE DELETE OR UPDATE ON identity_protocol_policy FOR EACH ROW EXECUTE FUNCTION identity_policy_revision_guard();

CREATE TRIGGER p10_account_epoch_policy AFTER UPDATE OF auth_epoch ON accounts FOR EACH ROW EXECUTE FUNCTION p10_account_epoch_policy();

CREATE TRIGGER p10_claim_origin BEFORE INSERT OR UPDATE OF origin_account_id ON claim_operations FOR EACH ROW EXECUTE FUNCTION p10_claim_origin();

CREATE TRIGGER p10_profile_insert BEFORE INSERT ON profiles FOR EACH ROW EXECUTE FUNCTION p10_profile_insert();

CREATE TRIGGER portico_wake_notify_insert AFTER INSERT ON wake_state FOR EACH ROW WHEN ((new.desired_revision > new.acked_revision)) EXECUTE FUNCTION portico_notify_wake();

CREATE TRIGGER portico_wake_notify_update AFTER UPDATE ON wake_state FOR EACH ROW WHEN (((new.desired_revision > new.acked_revision) AND (new.next_attempt_at <= now()) AND ((new.desired_revision IS DISTINCT FROM old.desired_revision) OR (new.next_attempt_at IS DISTINCT FROM old.next_attempt_at)))) EXECUTE FUNCTION portico_notify_wake();

ALTER TABLE ONLY account_contact_emails
    ADD CONSTRAINT account_contact_emails_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY account_contact_emails
    ADD CONSTRAINT account_contact_emails_source_identity_id_account_id_fkey FOREIGN KEY (source_identity_id, account_id) REFERENCES account_external_identities(id, account_id) ON DELETE CASCADE;

ALTER TABLE ONLY account_email_challenges
    ADD CONSTRAINT account_email_challenges_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY account_erasure_jobs
    ADD CONSTRAINT account_erasure_jobs_deletion_id_fkey FOREIGN KEY (deletion_id) REFERENCES account_deletions(id);

ALTER TABLE ONLY account_erasure_jobs
    ADD CONSTRAINT account_erasure_jobs_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id);

ALTER TABLE ONLY account_external_identities
    ADD CONSTRAINT account_external_identities_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE RESTRICT;

ALTER TABLE ONLY account_mfa
    ADD CONSTRAINT account_mfa_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY account_mfa_challenges
    ADD CONSTRAINT account_mfa_challenges_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY account_mfa_challenges
    ADD CONSTRAINT account_mfa_challenges_identity_transaction_id_fkey FOREIGN KEY (identity_transaction_id) REFERENCES identity_transactions(id) ON DELETE CASCADE;

ALTER TABLE ONLY account_password_logins
    ADD CONSTRAINT account_password_logins_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY account_preferences
    ADD CONSTRAINT account_preferences_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY account_recent_proofs
    ADD CONSTRAINT account_recent_proofs_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY account_recent_proofs
    ADD CONSTRAINT account_recent_proofs_family_id_account_id_fkey FOREIGN KEY (family_id, account_id) REFERENCES session_families(id, account_id) ON DELETE CASCADE;

ALTER TABLE ONLY account_recent_proofs
    ADD CONSTRAINT account_recent_proofs_identity_id_account_id_fkey FOREIGN KEY (identity_id, account_id) REFERENCES account_external_identities(id, account_id) ON DELETE CASCADE;

ALTER TABLE ONLY account_recovery_codes
    ADD CONSTRAINT account_recovery_codes_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY account_sessions
    ADD CONSTRAINT account_sessions_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id);

ALTER TABLE ONLY account_sessions
    ADD CONSTRAINT account_sessions_family_id_account_id_fkey FOREIGN KEY (family_id, account_id) REFERENCES session_families(id, account_id);

ALTER TABLE ONLY appstore_notifications
    ADD CONSTRAINT appstore_notifications_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE SET NULL;

ALTER TABLE ONLY attachment_tickets
    ADD CONSTRAINT attachment_tickets_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id);

ALTER TABLE ONLY attachment_tickets
    ADD CONSTRAINT attachment_tickets_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES profiles(id);

ALTER TABLE ONLY attachment_tickets
    ADD CONSTRAINT attachment_tickets_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id);

ALTER TABLE ONLY certificate_challenges
    ADD CONSTRAINT certificate_challenges_namespace_fkey FOREIGN KEY (namespace) REFERENCES certificate_namespaces(namespace);

ALTER TABLE ONLY certificate_challenges
    ADD CONSTRAINT certificate_challenges_order_id_fkey FOREIGN KEY (order_id) REFERENCES certificate_orders(id);

ALTER TABLE ONLY certificate_orders
    ADD CONSTRAINT certificate_orders_namespace_fkey FOREIGN KEY (namespace) REFERENCES certificate_namespaces(namespace);

ALTER TABLE ONLY claim_approvals
    ADD CONSTRAINT claim_approvals_family_id_account_id_fkey FOREIGN KEY (family_id, account_id) REFERENCES session_families(id, account_id) ON DELETE RESTRICT;

ALTER TABLE ONLY claim_approvals
    ADD CONSTRAINT claim_approvals_operation_id_account_id_fkey FOREIGN KEY (operation_id, account_id) REFERENCES claim_operations(id, account_id) ON DELETE RESTRICT;

ALTER TABLE ONLY claim_cancellations
    ADD CONSTRAINT claim_cancellations_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES claim_operations(id) ON DELETE RESTRICT;

ALTER TABLE ONLY claim_nonces
    ADD CONSTRAINT claim_nonces_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES claim_operations(id) ON DELETE RESTRICT;

ALTER TABLE ONLY claim_operations
    ADD CONSTRAINT claim_operations_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE SET NULL;

ALTER TABLE ONLY claim_operations
    ADD CONSTRAINT claim_operations_owner_profile_id_fkey FOREIGN KEY (owner_profile_id) REFERENCES profiles(id) ON DELETE SET NULL;

ALTER TABLE ONLY current_server_endpoints
    ADD CONSTRAINT current_server_endpoints_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY current_server_endpoints
    ADD CONSTRAINT current_server_endpoints_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES claim_operations(id);

ALTER TABLE ONLY current_server_endpoints
    ADD CONSTRAINT current_server_endpoints_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id);

ALTER TABLE ONLY device_authorizations
    ADD CONSTRAINT device_authorizations_approved_account_id_fkey FOREIGN KEY (approved_account_id) REFERENCES accounts(id);

ALTER TABLE ONLY direct_addresses
    ADD CONSTRAINT direct_addresses_namespace_fkey FOREIGN KEY (namespace) REFERENCES certificate_namespaces(namespace) ON DELETE CASCADE;

ALTER TABLE ONLY family_cookie_names
    ADD CONSTRAINT family_cookie_names_family_id_fkey FOREIGN KEY (family_id) REFERENCES session_families(id) ON DELETE CASCADE;

ALTER TABLE ONLY hosted_devices
    ADD CONSTRAINT hosted_devices_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id);

ALTER TABLE ONLY hosted_devices
    ADD CONSTRAINT hosted_devices_family_id_account_id_fkey FOREIGN KEY (family_id, account_id) REFERENCES session_families(id, account_id);

ALTER TABLE ONLY identity_completion_receipts
    ADD CONSTRAINT identity_completion_receipts_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY identity_completion_receipts
    ADD CONSTRAINT identity_completion_receipts_family_id_account_id_fkey FOREIGN KEY (family_id, account_id) REFERENCES session_families(id, account_id) ON DELETE CASCADE;

ALTER TABLE ONLY identity_completion_receipts
    ADD CONSTRAINT identity_completion_receipts_transaction_id_transaction_ex_fkey FOREIGN KEY (transaction_id, transaction_expires_at) REFERENCES identity_transactions(id, expires_at) ON DELETE CASCADE;

ALTER TABLE ONLY identity_contact_challenges
    ADD CONSTRAINT identity_contact_challenges_transaction_id_transaction_exp_fkey FOREIGN KEY (transaction_id, transaction_expires_at) REFERENCES identity_transactions(id, expires_at) ON DELETE CASCADE;

ALTER TABLE ONLY identity_contact_outbox
    ADD CONSTRAINT identity_contact_outbox_challenge_id_challenge_revision_ch_fkey FOREIGN KEY (challenge_id, challenge_revision, challenge_expires_at) REFERENCES identity_contact_challenges(id, revision, expires_at) ON DELETE CASCADE;

ALTER TABLE ONLY identity_exchange_claims
    ADD CONSTRAINT identity_exchange_claims_transaction_id_transaction_expire_fkey FOREIGN KEY (transaction_id, transaction_expires_at) REFERENCES identity_transactions(id, expires_at) ON DELETE CASCADE;

ALTER TABLE ONLY identity_exchange_secrets
    ADD CONSTRAINT identity_exchange_secrets_transaction_id_transaction_expir_fkey FOREIGN KEY (transaction_id, transaction_expires_at) REFERENCES identity_transactions(id, expires_at) ON DELETE CASCADE;

ALTER TABLE ONLY identity_onboarding
    ADD CONSTRAINT identity_onboarding_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES identity_transactions(id) ON DELETE CASCADE;

ALTER TABLE ONLY identity_transactions
    ADD CONSTRAINT identity_transactions_binding_hash_fkey FOREIGN KEY (binding_hash) REFERENCES identity_browser_bindings(binding_hash) ON DELETE CASCADE;

ALTER TABLE ONLY identity_transactions
    ADD CONSTRAINT identity_transactions_descriptor_id_provider_fkey FOREIGN KEY (descriptor_id, provider) REFERENCES identity_provider_descriptors(id, provider) ON DELETE RESTRICT;

ALTER TABLE ONLY identity_transactions
    ADD CONSTRAINT identity_transactions_expected_account_id_fkey FOREIGN KEY (expected_account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY identity_transactions
    ADD CONSTRAINT identity_transactions_expected_family_id_expected_account__fkey FOREIGN KEY (expected_family_id, expected_account_id) REFERENCES session_families(id, account_id) ON DELETE CASCADE;

ALTER TABLE ONLY identity_transactions
    ADD CONSTRAINT identity_transactions_expected_identity_id_expected_accoun_fkey FOREIGN KEY (expected_identity_id, expected_account_id) REFERENCES account_external_identities(id, account_id) ON DELETE CASCADE;

ALTER TABLE ONLY identity_verified_assertions
    ADD CONSTRAINT identity_verified_assertions_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES identity_transactions(id) ON DELETE CASCADE;

ALTER TABLE ONLY invitations
    ADD CONSTRAINT invitations_accepted_profile_id_fkey FOREIGN KEY (accepted_profile_id) REFERENCES profiles(id);

ALTER TABLE ONLY invitations
    ADD CONSTRAINT invitations_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES accounts(id);

ALTER TABLE ONLY invitations
    ADD CONSTRAINT invitations_recipient_id_fkey FOREIGN KEY (recipient_id) REFERENCES accounts(id);

ALTER TABLE ONLY invitations
    ADD CONSTRAINT invitations_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id);

ALTER TABLE ONLY membership_operation_receipts
    ADD CONSTRAINT membership_operation_receipts_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY membership_operation_receipts
    ADD CONSTRAINT membership_operation_receipts_family_id_fkey FOREIGN KEY (family_id) REFERENCES session_families(id) ON DELETE CASCADE;

ALTER TABLE ONLY memberships
    ADD CONSTRAINT memberships_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id);

ALTER TABLE ONLY memberships
    ADD CONSTRAINT memberships_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES profiles(id);

ALTER TABLE ONLY memberships
    ADD CONSTRAINT memberships_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id);

ALTER TABLE ONLY mutation_receipts
    ADD CONSTRAINT mutation_receipts_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id);

ALTER TABLE ONLY networking_route_candidates
    ADD CONSTRAINT networking_route_candidates_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES claim_operations(id);

ALTER TABLE ONLY networking_route_candidates
    ADD CONSTRAINT networking_route_candidates_server_id_fkey FOREIGN KEY (server_id) REFERENCES networking_route_state(server_id) ON DELETE CASCADE;

ALTER TABLE ONLY networking_route_state
    ADD CONSTRAINT networking_route_state_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES claim_operations(id);

ALTER TABLE ONLY networking_route_state
    ADD CONSTRAINT networking_route_state_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id);

ALTER TABLE ONLY profile_deletions
    ADD CONSTRAINT profile_deletions_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY profile_deletions
    ADD CONSTRAINT profile_deletions_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE;

ALTER TABLE ONLY profile_erasure_jobs
    ADD CONSTRAINT profile_erasure_jobs_deletion_id_fkey FOREIGN KEY (deletion_id) REFERENCES profile_deletions(id) ON DELETE CASCADE;

ALTER TABLE ONLY profile_erasure_jobs
    ADD CONSTRAINT profile_erasure_jobs_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE;

ALTER TABLE ONLY profile_server_policies
    ADD CONSTRAINT profile_server_policies_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY profile_server_policies
    ADD CONSTRAINT profile_server_policies_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE;

ALTER TABLE ONLY profile_server_policies
    ADD CONSTRAINT profile_server_policies_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE;

ALTER TABLE ONLY profile_trusted_selections
    ADD CONSTRAINT profile_trusted_selections_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY profile_trusted_selections
    ADD CONSTRAINT profile_trusted_selections_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES profiles(id) ON DELETE CASCADE;

ALTER TABLE ONLY profile_trusted_selections
    ADD CONSTRAINT profile_trusted_selections_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE;

ALTER TABLE ONLY profiles
    ADD CONSTRAINT profiles_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id);

ALTER TABLE ONLY refresh_receipts
    ADD CONSTRAINT refresh_receipts_family_id_fkey FOREIGN KEY (family_id) REFERENCES session_families(id);

ALTER TABLE ONLY refresh_tokens
    ADD CONSTRAINT refresh_tokens_family_id_fkey FOREIGN KEY (family_id) REFERENCES session_families(id);

ALTER TABLE ONLY restrictions
    ADD CONSTRAINT restrictions_server_id_account_id_profile_id_fkey FOREIGN KEY (server_id, account_id, profile_id) REFERENCES memberships(server_id, account_id, profile_id);

ALTER TABLE ONLY restrictions
    ADD CONSTRAINT restrictions_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id);

ALTER TABLE ONLY server_ownership_transfers
    ADD CONSTRAINT server_ownership_transfers_claim_operation_id_fkey FOREIGN KEY (claim_operation_id) REFERENCES claim_operations(id);

ALTER TABLE ONLY server_ownership_transfers
    ADD CONSTRAINT server_ownership_transfers_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id);

ALTER TABLE ONLY servers
    ADD CONSTRAINT servers_owner_account_id_fkey FOREIGN KEY (owner_account_id) REFERENCES accounts(id);

ALTER TABLE ONLY session_families
    ADD CONSTRAINT session_families_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id);

ALTER TABLE ONLY wake_outbox
    ADD CONSTRAINT wake_outbox_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id);

ALTER TABLE ONLY wake_state
    ADD CONSTRAINT wake_state_server_id_fkey FOREIGN KEY (server_id) REFERENCES servers(id);

