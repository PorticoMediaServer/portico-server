-- Separate the server membership/account boundary from the viewing-profile
-- boundary. Existing users retain an id-matched primary profile so upgrades do
-- not invalidate sessions or viewer data while new profiles may use distinct
-- identifiers.
ALTER TABLE users ADD COLUMN allow_account_profiles INTEGER NOT NULL DEFAULT 1
    CHECK (allow_account_profiles IN (0, 1));

-- Profiles historically came from the database compatibility pass after SQL
-- migrations. Create that legacy shape when upgrading a brand-new database so
-- this migration is equally valid for clean installs and existing servers.
CREATE TABLE IF NOT EXISTS profiles (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL,
    permissions_json TEXT NOT NULL,
    preferences_json TEXT NOT NULL DEFAULT '{}',
    max_content_rating TEXT NOT NULL DEFAULT '',
    max_active_sessions INTEGER NOT NULL DEFAULT 0,
    remote_bitrate_limit_mbps INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

ALTER TABLE profiles ADD COLUMN account_id TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN origin TEXT NOT NULL DEFAULT 'local'
    CHECK (origin IN ('local', 'hosted'));
ALTER TABLE profiles ADD COLUMN external_profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN is_primary INTEGER NOT NULL DEFAULT 0
    CHECK (is_primary IN (0, 1));
ALTER TABLE profiles ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;
ALTER TABLE profiles ADD COLUMN avatar_url TEXT NOT NULL DEFAULT '';
ALTER TABLE profiles ADD COLUMN restrictions_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE profiles ADD COLUMN pin_required INTEGER NOT NULL DEFAULT 0
    CHECK (pin_required IN (0, 1));
ALTER TABLE profiles ADD COLUMN disabled_at TEXT NOT NULL DEFAULT '';

UPDATE profiles
SET account_id = id,
    is_primary = 1,
    origin = CASE
        WHEN EXISTS (
            SELECT 1 FROM users
            WHERE users.id = profiles.id AND users.auth_origin = 'portico'
        ) THEN 'hosted'
        ELSE 'local'
    END
WHERE account_id = '';

CREATE INDEX IF NOT EXISTS idx_profiles_account_sort
    ON profiles(account_id, sort_order, created_at, id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_account_primary
    ON profiles(account_id) WHERE is_primary = 1;
CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_account_external
    ON profiles(account_id, external_profile_id) WHERE external_profile_id <> '';

CREATE TRIGGER IF NOT EXISTS trg_profiles_account_insert_guard
BEFORE INSERT ON profiles
WHEN NEW.account_id = '' OR NOT EXISTS (SELECT 1 FROM users WHERE id = NEW.account_id)
BEGIN
    SELECT RAISE(ABORT, 'profile account does not exist');
END;

CREATE TRIGGER IF NOT EXISTS trg_profiles_account_update_guard
BEFORE UPDATE OF account_id ON profiles
WHEN NEW.account_id = '' OR NOT EXISTS (SELECT 1 FROM users WHERE id = NEW.account_id)
BEGIN
    SELECT RAISE(ABORT, 'profile account does not exist');
END;

CREATE TRIGGER IF NOT EXISTS trg_users_profiles_delete
AFTER DELETE ON users
BEGIN
    DELETE FROM profiles WHERE account_id = OLD.id;
END;

CREATE TABLE IF NOT EXISTS local_profile_pin_credentials (
    profile_id TEXT PRIMARY KEY REFERENCES profiles(id) ON DELETE CASCADE,
    pin_hash TEXT NOT NULL,
    failed_attempts INTEGER NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    locked_until TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TRIGGER IF NOT EXISTS trg_local_profile_pin_origin_guard
BEFORE INSERT ON local_profile_pin_credentials
WHEN NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id AND profiles.origin = 'local'
)
BEGIN
    SELECT RAISE(ABORT, 'local profile PIN requires a local profile');
END;

ALTER TABLE native_refresh_tokens ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
UPDATE native_refresh_tokens SET profile_id = user_id WHERE profile_id = '';
CREATE INDEX IF NOT EXISTS idx_native_refresh_tokens_profile
    ON native_refresh_tokens(profile_id, revoked_at, expires_at);
