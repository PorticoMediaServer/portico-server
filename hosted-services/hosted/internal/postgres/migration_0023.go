package postgres

// Fresh current-format composition only. Existing formats are refused before
// this initializer; no production queue mutation or in-place upgrade is run.
const migration0023 = `
ALTER TABLE account_mail_outbox DROP CONSTRAINT account_mail_outbox_kind_check;
ALTER TABLE account_mail_outbox ADD CONSTRAINT account_mail_outbox_kind_check CHECK(kind IN('registration','oidc_contact','password_reset','email_change'));
`
