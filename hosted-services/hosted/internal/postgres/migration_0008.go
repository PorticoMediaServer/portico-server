package postgres

// 0008: the Hosted operator is alerted 30 days before the signing certificate
// expires, through the ordinary account notification path (mail and push).
const migration0008 = `
ALTER TABLE account_notification_events DROP CONSTRAINT account_notification_events_kind_check;
ALTER TABLE account_notification_events ADD CONSTRAINT account_notification_events_kind_check CHECK(kind IN('server_offline','storage_nearly_full','invitation_accepted','new_device','signing_key_expiring'));
ALTER TABLE account_mail_outbox DROP CONSTRAINT account_mail_outbox_kind_check;
ALTER TABLE account_mail_outbox ADD CONSTRAINT account_mail_outbox_kind_check CHECK(kind IN('registration','oidc_contact','password_reset','email_change','security_notice','invitation','server_offline','storage_nearly_full','invitation_accepted','new_device','account_export','signing_key_expiring'));
`
