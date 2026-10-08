package postgres

// 0016: native provider sign-in has two return contracts while installed apps
// catch up. A transaction started with completion "code" (clients that saw
// nativeCompletion in GET /v1/identity/options) returns to
// /app/account-return#tx=&code= and must present that code; one started
// without it keeps the original portico://account-return?transactionId=
// return and view. native_completion records which.
const migration0016 = `ALTER TABLE identity_transactions ADD COLUMN native_completion boolean NOT NULL DEFAULT false;`
