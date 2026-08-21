-- Canonical V1 viewer preference documents. Values are requests rather than
-- authorization: every consumer must still apply current server, membership,
-- profile, media, and platform policy.
CREATE TABLE viewer_preference_documents (
    id TEXT PRIMARY KEY,
    scope_type TEXT NOT NULL CHECK (scope_type IN (
        'profile-server',
        'profile-device-class',
        'account-server-installation'
    )),
    authority TEXT NOT NULL CHECK (authority IN ('local', 'hosted')),
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL DEFAULT '',
    server_id TEXT NOT NULL CHECK (server_id <> ''),
    device_class TEXT NOT NULL DEFAULT '' CHECK (device_class IN ('', 'web', 'mobile', 'television')),
    installation_id TEXT NOT NULL DEFAULT '',
    version TEXT NOT NULL CHECK (version = 'v1'),
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    values_json TEXT NOT NULL CHECK (json_valid(values_json) AND json_type(values_json) = 'object'),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (
        (scope_type = 'profile-server' AND profile_id <> '' AND device_class = '' AND installation_id = '')
        OR (scope_type = 'profile-device-class' AND profile_id <> '' AND device_class <> '' AND installation_id = '')
        OR (scope_type = 'account-server-installation' AND profile_id = '' AND device_class = '' AND installation_id <> '')
    ),
    UNIQUE (scope_type, authority, account_id, profile_id, server_id, device_class, installation_id)
);

CREATE INDEX idx_viewer_preference_documents_profile
    ON viewer_preference_documents(account_id, profile_id, server_id, scope_type);
CREATE INDEX idx_viewer_preference_documents_installation
    ON viewer_preference_documents(account_id, server_id, installation_id, device_class);

CREATE TRIGGER trg_viewer_preference_profile_insert_guard
BEFORE INSERT ON viewer_preference_documents
WHEN NEW.profile_id <> '' AND NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.disabled_at = ''
)
BEGIN
    SELECT RAISE(ABORT, 'preference profile does not belong to account');
END;

CREATE TRIGGER trg_viewer_preference_profile_update_guard
BEFORE UPDATE OF account_id, profile_id ON viewer_preference_documents
WHEN NEW.profile_id <> '' AND NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.disabled_at = ''
)
BEGIN
    SELECT RAISE(ABORT, 'preference profile does not belong to account');
END;

CREATE TRIGGER trg_viewer_preference_profile_cleanup
AFTER DELETE ON profiles
BEGIN
    DELETE FROM viewer_preference_documents WHERE profile_id = OLD.id;
END;

CREATE TRIGGER trg_viewer_preference_profile_disable_cleanup
AFTER UPDATE OF disabled_at ON profiles
WHEN OLD.disabled_at = '' AND NEW.disabled_at <> ''
BEGIN
    DELETE FROM viewer_preference_documents WHERE profile_id = NEW.id;
END;

-- A five-minute, non-renewing step-up is required for local household profile
-- administration. Only a hash of the bearer proof is retained and the proof is
-- bound to the browser/native server session and current primary PIN revision.
CREATE TABLE local_profile_admin_proofs (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    primary_profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    pin_revision INTEGER NOT NULL CHECK (pin_revision >= 0),
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_local_profile_admin_proofs_session
    ON local_profile_admin_proofs(session_id, expires_at);
CREATE INDEX idx_local_profile_admin_proofs_account
    ON local_profile_admin_proofs(account_id, expires_at);

CREATE TRIGGER trg_local_profile_admin_proof_primary_guard
BEFORE INSERT ON local_profile_admin_proofs
WHEN NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.primary_profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.is_primary = 1
      AND profiles.origin = 'local'
      AND profiles.disabled_at = ''
      AND profiles.pin_revision = NEW.pin_revision
)
BEGIN
    SELECT RAISE(ABORT, 'profile administration proof requires current local primary profile');
END;

CREATE TRIGGER trg_local_profile_admin_proof_revoke_security_update
AFTER UPDATE OF pin_revision, disabled_at ON profiles
BEGIN
    DELETE FROM local_profile_admin_proofs WHERE account_id = NEW.account_id;
END;

-- Automatic profile selection is an authorization decision, not a preference.
-- This revocable bearer trust is issued only after an authenticated profile
-- selection and is bound to the exact server installation and PIN revision.
CREATE TABLE automatic_profile_selection_trusts (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    authority TEXT NOT NULL CHECK (authority IN ('local', 'hosted')),
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id TEXT NOT NULL CHECK (server_id <> ''),
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    installation_id TEXT NOT NULL CHECK (installation_id <> ''),
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    pin_revision INTEGER NOT NULL CHECK (pin_revision >= 0),
    expires_at TEXT NOT NULL,
    revoked_at TEXT NOT NULL DEFAULT '',
    last_used_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (authority, account_id, server_id, device_id, installation_id, profile_id)
);

CREATE INDEX idx_automatic_profile_trust_lookup
    ON automatic_profile_selection_trusts(account_id, server_id, device_id, installation_id, expires_at);
CREATE INDEX idx_automatic_profile_trust_profile
    ON automatic_profile_selection_trusts(profile_id, pin_revision, revoked_at);

CREATE TRIGGER trg_automatic_profile_trust_device_insert_guard
BEFORE INSERT ON automatic_profile_selection_trusts
WHEN NOT EXISTS (
    SELECT 1 FROM devices
    WHERE devices.id = NEW.device_id
      AND devices.user_id = NEW.account_id
      AND devices.installation_id = NEW.installation_id
      AND COALESCE(devices.revoked_at, '') = ''
)
BEGIN
    SELECT RAISE(ABORT, 'automatic profile trust device mismatch');
END;

CREATE TRIGGER trg_automatic_profile_trust_device_update_guard
BEFORE UPDATE OF account_id, device_id, installation_id ON automatic_profile_selection_trusts
WHEN NOT EXISTS (
    SELECT 1 FROM devices
    WHERE devices.id = NEW.device_id
      AND devices.user_id = NEW.account_id
      AND devices.installation_id = NEW.installation_id
      AND COALESCE(devices.revoked_at, '') = ''
)
BEGIN
    SELECT RAISE(ABORT, 'automatic profile trust device mismatch');
END;

CREATE TRIGGER trg_automatic_profile_trust_profile_guard
BEFORE INSERT ON automatic_profile_selection_trusts
WHEN NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.disabled_at = ''
      AND profiles.pin_revision = NEW.pin_revision
)
BEGIN
    SELECT RAISE(ABORT, 'automatic profile trust requires current profile security state');
END;

CREATE TRIGGER trg_automatic_profile_trust_profile_update_guard
BEFORE UPDATE OF account_id, profile_id, pin_revision ON automatic_profile_selection_trusts
WHEN NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.disabled_at = ''
      AND profiles.pin_revision = NEW.pin_revision
)
BEGIN
    SELECT RAISE(ABORT, 'automatic profile trust requires current profile security state');
END;

CREATE TRIGGER trg_automatic_profile_trust_revoke_security_update
AFTER UPDATE OF pin_revision, disabled_at ON profiles
BEGIN
    UPDATE automatic_profile_selection_trusts
       SET revoked_at = CURRENT_TIMESTAMP,
           updated_at = CURRENT_TIMESTAMP
     WHERE profile_id = NEW.id AND revoked_at = '';
END;

-- Legacy display/navigation/profile JSON remains readable during the rolling
-- migration. The application performs an idempotent, identity-aware backfill
-- the first time each V1 bundle is read because the stable server_id may not
-- exist until application startup. Legacy account authorization keys are never
-- removed from users.preferences_json by this migration.
