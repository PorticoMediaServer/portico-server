package postgres

// 0010: no push notifications (Justin, 23 Sep). The device push tables and
// their queue go; account_notification_events becomes the in-app feed (read
// state, a detail document, security events, export completion and the
// operator key-expiry warning). Alert email kinds leave the mail outbox:
// email is only for account setup, invitations and password reset.
const migration0010 = `
DROP TRIGGER portico_push_notify ON account_push_outbox;
DROP TABLE account_push_outbox;
DROP TABLE device_push_tokens;
DROP TRIGGER portico_notification_notify ON account_notification_events;
DROP INDEX notification_pending;
DROP INDEX notification_processed;
ALTER TABLE account_notification_events DROP COLUMN processed_at;
ALTER TABLE account_notification_events ADD COLUMN read_at timestamptz, ADD COLUMN detail jsonb NOT NULL DEFAULT '{}';
ALTER TABLE account_notification_events DROP CONSTRAINT account_notification_events_kind_check;
ALTER TABLE account_notification_events ADD CONSTRAINT account_notification_events_kind_check CHECK(kind IN('server_offline','storage_nearly_full','invitation_accepted','new_device','security_event','account_export_ready','signing_key_expiring'));
CREATE INDEX account_notification_feed ON account_notification_events(account_id,created_at DESC,id DESC);
CREATE INDEX account_notification_unread ON account_notification_events(account_id) WHERE read_at IS NULL;
CREATE INDEX account_notification_age ON account_notification_events(created_at);
DELETE FROM account_mail_outbox WHERE kind NOT IN('registration','oidc_contact','password_reset','email_change','invitation');
ALTER TABLE account_mail_outbox DROP CONSTRAINT account_mail_outbox_kind_check;
ALTER TABLE account_mail_outbox ADD CONSTRAINT account_mail_outbox_kind_check CHECK(kind IN('registration','oidc_contact','password_reset','email_change','invitation'));
`
