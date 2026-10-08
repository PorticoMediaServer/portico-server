package postgres

// 0018 (ARCH-HOSTED-04 / BE-HOSTED-08): the 15-minute presence renewal is gone
// (Justin rejected periodic presence, 23 Sep), and with it the column that
// recorded it. Hosted learns about a server from its events and its
// weekly/daily check-in; whether it is up right now is the app's to find out
// by contacting it.
const migration0018 = `ALTER TABLE servers DROP COLUMN IF EXISTS presence_seen_at;`
