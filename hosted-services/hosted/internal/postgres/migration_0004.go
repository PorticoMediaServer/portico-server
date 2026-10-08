package postgres

// 0004: server retirement and succession queue member-data cleanup jobs that
// belong to no account deletion, so their deletion_id is NULL (A43). The
// cleanup channel and the server acknowledgement already read it as nullable.
const migration0004 = `
ALTER TABLE account_erasure_jobs ALTER COLUMN deletion_id DROP NOT NULL;
`
