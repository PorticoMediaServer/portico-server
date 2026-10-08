package postgres

// 0019: a consumed refresh token stays while a recoverable receipt names it as
// its input. A client whose rotation response was lost retries with the old
// token days later; the lookup starts from refresh_tokens, so pruning that row
// after one day signed the client out although its receipt was still valid.
// Retention now skips those rows, and this index keeps the check a seek.
const migration0019 = `CREATE INDEX IF NOT EXISTS refresh_receipts_input ON refresh_receipts(input_hash) WHERE ciphertext IS NOT NULL;`
