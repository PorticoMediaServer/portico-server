-- Private viewer notification content is fetched by its exact recipient. The
-- event channel is a content-free wake-up; clients refetch all recipient state
-- through the authenticated notification endpoint.
CREATE TABLE viewer_notifications (
    id TEXT PRIMARY KEY,
    authority TEXT NOT NULL CHECK (authority IN ('local', 'hosted')),
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id TEXT NOT NULL CHECK (server_id <> ''),
    profile_id TEXT NOT NULL DEFAULT '',
    audience TEXT NOT NULL CHECK (audience IN ('profile', 'account-admin')),
    kind TEXT NOT NULL CHECK (kind IN (
        'account.security', 'download.ready', 'download.failed', 'dvr.conflict',
        'dvr.recording-failed', 'feedback.received', 'feedback.updated',
        'library-channel.degraded', 'membership.changed', 'server.message'
    )),
    severity TEXT NOT NULL CHECK (severity IN ('informational', 'success', 'warning', 'error')),
    message_id TEXT NOT NULL,
    icon_id TEXT NOT NULL,
    interpolation_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(interpolation_json) AND json_type(interpolation_json) = 'object'),
    content_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(content_json) AND json_type(content_json) = 'object'),
    actions_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(actions_json) AND json_type(actions_json) = 'array'),
    source_feedback_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    CHECK (
        (audience = 'profile' AND profile_id <> '')
        OR (audience = 'account-admin' AND profile_id = '')
    ),
    CHECK (
        (kind IN ('feedback.received', 'feedback.updated') AND source_feedback_id <> '')
        OR (kind NOT IN ('feedback.received', 'feedback.updated') AND source_feedback_id = '')
    )
);

CREATE INDEX idx_viewer_notifications_profile_page
    ON viewer_notifications(account_id, server_id, profile_id, audience, created_at DESC, id DESC);
CREATE INDEX idx_viewer_notifications_admin_page
    ON viewer_notifications(account_id, server_id, audience, created_at DESC, id DESC);
CREATE INDEX idx_viewer_notifications_expiry ON viewer_notifications(expires_at);

CREATE TRIGGER trg_viewer_notification_profile_insert_guard
BEFORE INSERT ON viewer_notifications
WHEN NEW.profile_id <> '' AND NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.disabled_at = ''
)
BEGIN
    SELECT RAISE(ABORT, 'notification profile does not belong to account');
END;

CREATE TRIGGER trg_viewer_notification_profile_update_guard
BEFORE UPDATE OF account_id, profile_id, audience ON viewer_notifications
WHEN (
    (NEW.audience = 'profile' AND NOT EXISTS (
        SELECT 1 FROM profiles
        WHERE profiles.id = NEW.profile_id
          AND profiles.account_id = NEW.account_id
          AND profiles.disabled_at = ''
    ))
    OR (NEW.audience = 'account-admin' AND NEW.profile_id <> '')
)
BEGIN
    SELECT RAISE(ABORT, 'notification recipient is invalid');
END;

CREATE TABLE viewer_notification_receipts (
    notification_id TEXT NOT NULL REFERENCES viewer_notifications(id) ON DELETE CASCADE,
    authority TEXT NOT NULL CHECK (authority IN ('local', 'hosted')),
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id TEXT NOT NULL CHECK (server_id <> ''),
    profile_id TEXT NOT NULL DEFAULT '',
    audience TEXT NOT NULL CHECK (audience IN ('profile', 'account-admin')),
    read_at TEXT NOT NULL DEFAULT '',
    archived_at TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    CHECK (
        (audience = 'profile' AND profile_id <> '')
        OR (audience = 'account-admin' AND profile_id = '')
    ),
    PRIMARY KEY (notification_id, authority, account_id, server_id, profile_id, audience)
);

CREATE INDEX idx_viewer_notification_receipts_recipient
    ON viewer_notification_receipts(authority, account_id, server_id, profile_id, audience, archived_at, read_at);

CREATE TRIGGER trg_viewer_notification_receipt_recipient_guard
BEFORE INSERT ON viewer_notification_receipts
WHEN NOT EXISTS (
    SELECT 1 FROM viewer_notifications n
    WHERE n.id = NEW.notification_id
      AND n.authority = NEW.authority
      AND n.account_id = NEW.account_id
      AND n.server_id = NEW.server_id
      AND n.profile_id = NEW.profile_id
      AND n.audience = NEW.audience
)
BEGIN
    SELECT RAISE(ABORT, 'notification receipt recipient mismatch');
END;

CREATE TRIGGER trg_viewer_notification_receipt_update_recipient_guard
BEFORE UPDATE OF notification_id, authority, account_id, server_id, profile_id, audience ON viewer_notification_receipts
WHEN NOT EXISTS (
    SELECT 1 FROM viewer_notifications n
    WHERE n.id = NEW.notification_id
      AND n.authority = NEW.authority
      AND n.account_id = NEW.account_id
      AND n.server_id = NEW.server_id
      AND n.profile_id = NEW.profile_id
      AND n.audience = NEW.audience
)
BEGIN
    SELECT RAISE(ABORT, 'notification receipt recipient mismatch');
END;

CREATE TABLE viewer_notification_revisions (
    authority TEXT NOT NULL CHECK (authority IN ('local', 'hosted')),
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id TEXT NOT NULL CHECK (server_id <> ''),
    profile_id TEXT NOT NULL DEFAULT '',
    audience TEXT NOT NULL CHECK (audience IN ('profile', 'account-admin')),
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    updated_at TEXT NOT NULL,
    CHECK (
        (audience = 'profile' AND profile_id <> '')
        OR (audience = 'account-admin' AND profile_id = '')
    ),
    PRIMARY KEY (authority, account_id, server_id, profile_id, audience)
);

-- Viewer feedback is intentionally bounded one-way communication, not chat.
-- Diagnostics are constructed by the server and must never contain paths,
-- credentials, provider URLs, IP addresses, raw logs, or arbitrary client JSON.
CREATE TABLE viewer_feedback (
    id TEXT PRIMARY KEY,
    authority TEXT NOT NULL CHECK (authority IN ('local', 'hosted')),
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('general', 'playback', 'media', 'quality')),
    category TEXT NOT NULL CHECK (category IN (
        'wont-play', 'buffering', 'playback-stopped', 'wrong-video', 'wrong-audio',
        'wrong-subtitles', 'incorrect-media-information', 'higher-quality-request', 'other'
    )),
    message TEXT NOT NULL,
    media_id TEXT NOT NULL DEFAULT '',
    playback_session_id TEXT NOT NULL DEFAULT '',
    device_class TEXT NOT NULL CHECK (device_class IN ('web', 'mobile', 'television')),
    platform TEXT NOT NULL,
    app_version TEXT NOT NULL,
    error_category TEXT NOT NULL DEFAULT '',
    diagnostics_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(diagnostics_json) AND json_type(diagnostics_json) = 'object'),
    duplicate_hash TEXT NOT NULL,
    duplicate_count INTEGER NOT NULL DEFAULT 0 CHECK (duplicate_count >= 0),
    status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'read', 'resolved', 'dismissed')),
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    owner_response TEXT NOT NULL DEFAULT '',
    responded_by_account_id TEXT NOT NULL DEFAULT '',
    responded_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

CREATE INDEX idx_viewer_feedback_owner_page
    ON viewer_feedback(status, created_at DESC, id DESC);
CREATE INDEX idx_viewer_feedback_recipient
    ON viewer_feedback(account_id, profile_id, created_at DESC, id DESC);
CREATE INDEX idx_viewer_feedback_duplicate
    ON viewer_feedback(account_id, profile_id, duplicate_hash, created_at DESC);
CREATE INDEX idx_viewer_feedback_expiry ON viewer_feedback(expires_at);

CREATE TRIGGER trg_viewer_feedback_profile_insert_guard
BEFORE INSERT ON viewer_feedback
WHEN NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.disabled_at = ''
)
BEGIN
    SELECT RAISE(ABORT, 'feedback profile does not belong to account');
END;

CREATE TRIGGER trg_viewer_notification_feedback_source_guard
BEFORE INSERT ON viewer_notifications
WHEN NEW.source_feedback_id <> '' AND NOT EXISTS (
    SELECT 1 FROM viewer_feedback f
    WHERE f.id = NEW.source_feedback_id
      AND (
          (
              NEW.kind = 'feedback.received'
              AND NEW.audience = 'account-admin'
              AND EXISTS (
                  SELECT 1 FROM users owner
                  WHERE owner.id = NEW.account_id
                    AND owner.role IN ('owner', 'admin')
                    AND COALESCE(owner.disabled_at, '') = ''
              )
          )
          OR (
              NEW.kind = 'feedback.updated'
              AND NEW.audience = 'profile'
              AND f.account_id = NEW.account_id
              AND f.profile_id = NEW.profile_id
          )
      )
)
BEGIN
    SELECT RAISE(ABORT, 'notification feedback source mismatch');
END;


CREATE TRIGGER trg_viewer_notification_feedback_source_update_guard
BEFORE UPDATE OF source_feedback_id, kind, authority, account_id, profile_id, audience ON viewer_notifications
WHEN NEW.source_feedback_id <> '' AND NOT EXISTS (
    SELECT 1 FROM viewer_feedback f
    WHERE f.id = NEW.source_feedback_id
      AND (
          (
              NEW.kind = 'feedback.received'
              AND NEW.audience = 'account-admin'
              AND EXISTS (
                  SELECT 1 FROM users owner
                  WHERE owner.id = NEW.account_id
                    AND owner.role IN ('owner', 'admin')
                    AND COALESCE(owner.disabled_at, '') = ''
              )
          )
          OR (
              NEW.kind = 'feedback.updated'
              AND NEW.audience = 'profile'
              AND f.account_id = NEW.account_id
              AND f.profile_id = NEW.profile_id
          )
      )
)
BEGIN
    SELECT RAISE(ABORT, 'notification feedback source mismatch');
END;
