# Native provider completion

Native Google and web-based Apple OAuth use a callback-bound completion proof.
The initiating client keeps its existing `X-Portico-Continuation` secret. That
secret and the public transaction ID alone cannot complete a login.

After a verified provider callback, Hosted generates 32 random bytes, stores only
SHA-256 of their unpadded base64url encoding, and redirects the callback browser to:

`https://web.getportico.tv/app/account-return#tx=<transactionId>&code=<completionCode>`

The app must handle this claimed HTTPS link and send `completionCode` in the JSON
body of `POST /v1/identity/transactions/{id}/complete`, with its original continuation
header. Neither Start nor transaction polling returns the completion code. The
callback is not replayable. Hosted does not fall back to an interceptable custom
URL scheme.

The proof expires after two minutes (or the transaction expiry, if sooner) and
is consumed atomically with session issuance, a recent-auth result, or issuance of
an MFA challenge. A failed completion rolls back consumption; a username conflict
also preserves the proof while replacing the onboarding token. The MFA challenge
then uses its existing separate continuation. If delivery or completion is lost,
restart sign-in. This intentionally does not return a session receipt on replay
of a consumed native callback proof.

The direct native Apple assertion flow remains bound to its existing assertion
nonce and continuation and does not use this browser callback proof. Browser-mode
OAuth retains its HttpOnly cookie binding.

Client/release dependency: the lead must implement the `/app/account-return` route,
claimed app-link association and native completion-code forwarding. Do not release
the new backend with old native clients. The claimed link carries no access or
refresh token; its fragment must never be logged or sent to analytics.

## Database

A pre-release reset creates numbered migration `0001_initial`, including
`identity_native_completions`. The old unnumbered schema is rejected explicitly;
it is not silently adopted. A current-schema restart only reads the contract and
migration history, without re-running DDL. No development or production database
was reset by this change.

## Verification

- Unit tests cover missing, incorrect, cross-transaction, consumed and expired proofs.
- `TestNativeSSOInitiatorCannotTakeVictimSession` uses a disposable PostgreSQL
  schema and a fixture provider assertion; it never contacts Google or Apple.
- `TestCurrentHostedRestartNeedsNoDDL` runs the initializer in a read-only
  transaction to reject any attempt to write or execute DDL on restart.
- Run these integration tests with `PORTICO_TEST_DATABASE_URL_FILE` pointing to
  a disposable database. A skipped integration test is not acceptance evidence.
