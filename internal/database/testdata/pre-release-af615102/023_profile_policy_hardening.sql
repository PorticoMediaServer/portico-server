-- Harden the account/profile boundary introduced in migration 020. Accounts
-- remain the membership and quota owner; profiles are the viewer-state and
-- policy identity.
ALTER TABLE users ADD COLUMN max_active_streams INTEGER NOT NULL DEFAULT 0
    CHECK (max_active_streams >= 0 AND max_active_streams <= 32);

ALTER TABLE profiles ADD COLUMN pin_revision INTEGER NOT NULL DEFAULT 0
    CHECK (pin_revision >= 0);
ALTER TABLE profiles ADD COLUMN policy_updated_at TEXT NOT NULL DEFAULT '';

UPDATE profiles
SET restrictions_json = '{"version":"v1","maximumAgeRating":null,"allowUnrated":true,"blockedLabels":[],"allowDownloads":true,"allowLiveTV":true,"allowDvr":true,"allowWatchWithFriends":true,"allowFeedback":true}'
WHERE restrictions_json = '' OR restrictions_json = '{}';

-- Viewer-owned records retain user_id as their account/authorization owner,
-- while profile_id becomes the explicit viewing identity. Compatibility code
-- rebuilds compound-key tables after migrations so child profiles can own
-- independent state for the same media/library/view.
ALTER TABLE user_media_state ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
UPDATE user_media_state SET profile_id = user_id WHERE profile_id = '';

ALTER TABLE user_display_preferences ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
UPDATE user_display_preferences SET profile_id = user_id WHERE profile_id = '';

ALTER TABLE user_library_navigation ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
UPDATE user_library_navigation SET profile_id = user_id WHERE profile_id = '';

ALTER TABLE playlists ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
UPDATE playlists SET profile_id = user_id WHERE profile_id = '';

ALTER TABLE saved_views ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
UPDATE saved_views SET profile_id = user_id WHERE profile_id = '';

ALTER TABLE playback_media_grants ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
UPDATE playback_media_grants SET profile_id = principal_user_id WHERE profile_id = '';

ALTER TABLE media_download_grants ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
UPDATE media_download_grants SET profile_id = principal_user_id WHERE profile_id = '';

CREATE INDEX IF NOT EXISTS idx_playlists_profile_kind
    ON playlists(profile_id, kind, updated_at);
CREATE INDEX IF NOT EXISTS idx_saved_views_profile_updated
    ON saved_views(profile_id, updated_at DESC, id DESC);

-- Short-lived grants deliberately separate account authentication from
-- selecting a viewing profile. Only a hash of the bearer grant is retained.
CREATE TABLE IF NOT EXISTS profile_selection_grants (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    auth_provider TEXT NOT NULL CHECK (auth_provider IN ('local', 'portico')),
    device_id TEXT NOT NULL DEFAULT '',
    installation_id TEXT NOT NULL DEFAULT '',
    pin_revision INTEGER NOT NULL DEFAULT 0 CHECK (pin_revision >= 0),
    expires_at TEXT NOT NULL,
    consumed_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_profile_selection_grants_lookup
    ON profile_selection_grants(token_hash, consumed_at, expires_at);
CREATE INDEX IF NOT EXISTS idx_profile_selection_grants_profile
    ON profile_selection_grants(profile_id, consumed_at, expires_at);

-- A complete hosted profile set is accepted only monotonically. snapshot_id
-- plus the canonical payload digest makes same-revision retry idempotence
-- explicit without accepting a different document at the same revision.
CREATE TABLE IF NOT EXISTS hosted_profile_snapshot_state (
    account_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    snapshot_id TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    payload_digest TEXT NOT NULL,
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    applied_at TEXT NOT NULL
);

-- Cloud profile-selection assertions are signed, but signatures alone do not
-- make a bearer proof single-use. Retaining the assertion identity and
-- canonical digest prevents replay and also rejects a different payload that
-- attempts to reuse an assertion ID.
CREATE TABLE IF NOT EXISTS hosted_profile_selection_assertion_receipts (
    assertion_id TEXT PRIMARY KEY,
    payload_digest TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    hosted_device_id TEXT NOT NULL,
    local_device_id TEXT NOT NULL,
    installation_id TEXT NOT NULL,
    pin_revision INTEGER NOT NULL CHECK (pin_revision >= 0),
    expires_at TEXT NOT NULL,
    accepted_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_hosted_profile_assertion_receipts_expiry
    ON hosted_profile_selection_assertion_receipts(expires_at);

CREATE TRIGGER IF NOT EXISTS trg_profiles_account_immutable
BEFORE UPDATE OF account_id ON profiles
WHEN NEW.account_id <> OLD.account_id
BEGIN
    SELECT RAISE(ABORT, 'profile account ownership is immutable');
END;

CREATE TRIGGER IF NOT EXISTS trg_profiles_primary_role_immutable
BEFORE UPDATE OF is_primary ON profiles
WHEN NEW.is_primary <> OLD.is_primary
BEGIN
    SELECT RAISE(ABORT, 'primary profile role is immutable');
END;

CREATE TRIGGER IF NOT EXISTS trg_profiles_primary_delete_guard
BEFORE DELETE ON profiles
WHEN OLD.is_primary = 1
    AND EXISTS (SELECT 1 FROM users WHERE users.id = OLD.account_id)
BEGIN
    SELECT RAISE(ABORT, 'primary profile cannot be deleted');
END;

CREATE TRIGGER IF NOT EXISTS trg_hosted_primary_external_id_immutable
BEFORE UPDATE OF external_profile_id ON profiles
WHEN OLD.origin = 'hosted' AND OLD.is_primary = 1
    AND OLD.external_profile_id <> ''
    AND NEW.external_profile_id <> OLD.external_profile_id
BEGIN
    SELECT RAISE(ABORT, 'hosted primary profile is immutable');
END;

CREATE TRIGGER IF NOT EXISTS trg_local_child_requires_primary_pin
BEFORE INSERT ON profiles
WHEN NEW.origin = 'local' AND NEW.is_primary = 0
    AND NOT EXISTS (
        SELECT 1
        FROM profiles primary_profile
        JOIN local_profile_pin_credentials credential
          ON credential.profile_id = primary_profile.id
        WHERE primary_profile.account_id = NEW.account_id
          AND primary_profile.is_primary = 1
          AND primary_profile.origin = 'local'
          AND primary_profile.disabled_at = ''
    )
BEGIN
    SELECT RAISE(ABORT, 'primary profile PIN is required before adding profiles');
END;

CREATE TRIGGER IF NOT EXISTS trg_primary_pin_delete_guard
BEFORE DELETE ON local_profile_pin_credentials
WHEN EXISTS (
        SELECT 1 FROM profiles primary_profile
        WHERE primary_profile.id = OLD.profile_id
          AND primary_profile.is_primary = 1
		  AND primary_profile.origin = 'local'
    )
    AND EXISTS (
        SELECT 1 FROM profiles child_profile
        WHERE child_profile.account_id = (
            SELECT account_id FROM profiles WHERE id = OLD.profile_id
        )
          AND child_profile.is_primary = 0
          AND child_profile.disabled_at = ''
    )
BEGIN
    SELECT RAISE(ABORT, 'primary profile PIN cannot be cleared while child profiles exist');
END;

CREATE TRIGGER IF NOT EXISTS trg_local_profile_pin_update_origin_guard
BEFORE UPDATE ON local_profile_pin_credentials
WHEN NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id AND profiles.origin = 'local'
)
BEGIN
    SELECT RAISE(ABORT, 'local profile PIN requires a local profile');
END;

CREATE TRIGGER IF NOT EXISTS trg_profile_hosted_pin_cleanup
AFTER UPDATE OF origin ON profiles
WHEN OLD.origin = 'local' AND NEW.origin = 'hosted'
BEGIN
    DELETE FROM local_profile_pin_credentials WHERE profile_id = NEW.id;
END;
