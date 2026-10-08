package postgres

// 0009: password backoff is a cool-off only (Justin, 23 Sep); nothing is
// admitted by network trust any more.
const migration0009 = `
ALTER TABLE account_password_backoff DROP COLUMN trusted_until;
CREATE INDEX account_password_backoff_idle ON account_password_backoff(window_start);
`
