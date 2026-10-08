package postgres

const migration0003 = `
ALTER TABLE account_exports DROP CONSTRAINT account_exports_state_check;
ALTER TABLE account_exports ADD CONSTRAINT account_exports_state_check CHECK(state IN('pending','processing','ready'));
ALTER TABLE account_exports ADD COLUMN lease_id text, ADD COLUMN lease_until timestamptz;
CREATE TABLE device_push_tokens(
 device_id text PRIMARY KEY REFERENCES hosted_devices(id) ON DELETE CASCADE,
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 token_hash bytea NOT NULL UNIQUE CHECK(octet_length(token_hash)=32),
 ciphertext bytea NOT NULL,
 registered_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE account_push_outbox(
 event_id text NOT NULL REFERENCES account_notification_events(id) ON DELETE CASCADE,
 device_id text NOT NULL REFERENCES hosted_devices(id) ON DELETE CASCADE,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN('pending','sending','sent','failed')),
 attempts integer NOT NULL DEFAULT 0,
 next_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 lease_until timestamptz,
 sent_at timestamptz,
 PRIMARY KEY(event_id,device_id)
);
CREATE INDEX account_push_due ON account_push_outbox(next_at) WHERE state IN('pending','sending');
`
