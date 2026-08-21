-- Account authentication and profile selection are deliberately separate.
-- This short-lived, hashed proof is not a viewer session and grants access only
-- to the profile-selection endpoints for the bound account and installation.
CREATE TABLE IF NOT EXISTS profile_account_authentications (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    auth_provider TEXT NOT NULL CHECK (auth_provider IN ('local', 'portico')),
    purpose TEXT NOT NULL CHECK (purpose IN ('browser', 'native')),
    device_id TEXT NOT NULL CHECK (device_id <> ''),
    installation_id TEXT NOT NULL CHECK (installation_id <> ''),
    browser_binding_hash TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL,
    consumed_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_profile_account_authentications_lookup
    ON profile_account_authentications(token_hash, consumed_at, expires_at);
CREATE INDEX IF NOT EXISTS idx_profile_account_authentications_account
    ON profile_account_authentications(account_id, consumed_at, expires_at);

-- A profile grant carries its finalization surface and exact source proof. A
-- consumed native proof can therefore never satisfy browser finalization (or
-- vice versa), even when both belong to the same installation.
ALTER TABLE profile_selection_grants ADD COLUMN purpose TEXT NOT NULL DEFAULT 'native'
    CHECK (purpose IN ('browser', 'native'));
ALTER TABLE profile_selection_grants ADD COLUMN account_authentication_id TEXT NOT NULL DEFAULT '';
ALTER TABLE profile_selection_grants ADD COLUMN browser_binding_hash TEXT NOT NULL DEFAULT '';

-- Local grants minted from the public protocol must retain a relationally
-- valid source proof with identical account, purpose, device, and installation
-- bindings. Hosted assertion IDs remain governed by their signed receipt table.
CREATE TRIGGER IF NOT EXISTS trg_profile_selection_grant_local_source_proof
BEFORE INSERT ON profile_selection_grants
WHEN NEW.auth_provider = 'local' AND NEW.account_authentication_id LIKE 'pauth_%'
BEGIN
    SELECT CASE WHEN NOT EXISTS (
        SELECT 1 FROM profile_account_authentications source
        WHERE source.id = NEW.account_authentication_id
          AND source.account_id = NEW.account_id
          AND source.auth_provider = NEW.auth_provider
          AND source.purpose = NEW.purpose
          AND source.device_id = NEW.device_id
          AND source.installation_id = NEW.installation_id
    ) THEN RAISE(ABORT, 'invalid local profile selection source proof') END;
END;

-- Password, account-state, and profile-policy changes revoke any account proof
-- that has not yet been converted into a profile-bound one-time grant.
CREATE TRIGGER IF NOT EXISTS trg_profile_account_auth_revoke_user_security_update
AFTER UPDATE OF password_hash, disabled_at ON users
BEGIN
    DELETE FROM profile_account_authentications WHERE account_id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS trg_profile_account_auth_revoke_profile_security_update
AFTER UPDATE OF pin_revision, policy_updated_at, disabled_at ON profiles
BEGIN
    DELETE FROM profile_account_authentications WHERE account_id = NEW.account_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_profile_account_auth_revoke_profile_insert
AFTER INSERT ON profiles
BEGIN
    DELETE FROM profile_account_authentications WHERE account_id = NEW.account_id;
END;

CREATE TRIGGER IF NOT EXISTS trg_profile_account_auth_revoke_profile_delete
AFTER DELETE ON profiles
BEGIN
    DELETE FROM profile_account_authentications WHERE account_id = OLD.account_id;
END;

-- Hosted browser login retains optional installation metadata through the
-- Cloud redirect for client storage scoping. It is not authentication proof.
CREATE TABLE IF NOT EXISTS portico_login_requests (
    id TEXT PRIMARY KEY,
    state_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    return_url TEXT NOT NULL,
    server_id TEXT NOT NULL DEFAULT '',
    callback_url TEXT NOT NULL DEFAULT '',
    local_origin TEXT NOT NULL DEFAULT '',
    client_ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL,
    consumed_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    remember_on_browser INTEGER NOT NULL DEFAULT 1
);
ALTER TABLE portico_login_requests ADD COLUMN installation_id TEXT NOT NULL DEFAULT '';

-- Persisted inter-attempt delay prevents an offline/local attacker from
-- hammering profile PIN verification between terminal lockouts.
ALTER TABLE local_profile_pin_credentials ADD COLUMN next_attempt_at TEXT NOT NULL DEFAULT '';

-- Hosted profile policy has an explicit short freshness lease and a bounded
-- stale-if-error window. These values come only from a verified signed Cloud
-- directory snapshot and are clamped again by the server.
ALTER TABLE hosted_profile_snapshot_state ADD COLUMN checked_at TEXT NOT NULL DEFAULT '';
ALTER TABLE hosted_profile_snapshot_state ADD COLUMN max_age_seconds INTEGER NOT NULL DEFAULT 300;
ALTER TABLE hosted_profile_snapshot_state ADD COLUMN stale_if_error_seconds INTEGER NOT NULL DEFAULT 86400;
ALTER TABLE hosted_profile_snapshot_state ADD COLUMN refresh_retry_at TEXT NOT NULL DEFAULT '';
