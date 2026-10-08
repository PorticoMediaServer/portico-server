package postgres

// 0005: password backoff per (account, client network) plus one account-wide
// row, so an attacker can slow down only the networks it uses and never lock
// the real user out (A8). The previous per-account rows are transient failure
// counters; they are dropped, not carried over.
const migration0005 = `
DROP TABLE account_password_backoff;
CREATE TABLE account_password_backoff(
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 network bytea NOT NULL CHECK(octet_length(network)=32),
 failures integer NOT NULL DEFAULT 0 CHECK(failures>=0),
 window_start timestamptz NOT NULL DEFAULT clock_timestamp(),
 locked_until timestamptz NOT NULL DEFAULT clock_timestamp(),
 trusted_until timestamptz,
 PRIMARY KEY(account_id,network)
);
CREATE INDEX account_password_backoff_idle ON account_password_backoff(window_start) WHERE trusted_until IS NULL;
`
