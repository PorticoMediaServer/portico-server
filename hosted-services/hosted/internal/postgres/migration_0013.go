package postgres

// 0013: presence for server switchers. A running server renews presence every
// 15 minutes (POST /v1/servers/{id}/presence); presence_seen_at records it, and
// a server is online while that is under 45 minutes old and its presence lease
// (presence_expires_at, which a clean shutdown ends) is current. The weekly
// check-in lease is unchanged. Renewal is one update by primary key.
const migration0013 = `ALTER TABLE servers ADD COLUMN presence_seen_at timestamptz;`
