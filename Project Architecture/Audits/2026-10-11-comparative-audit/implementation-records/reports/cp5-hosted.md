# cp5-hosted — Hosted to milliseconds (server + portico-internal Hosted)

## 1 Outcome

The two seconds-based values left between the server and Hosted after cp4-ms are now milliseconds, changed on both sides together:

- **Check-in interval:** Hosted's check-in answer sends `heartbeatAfterMs` instead of `heartbeatAfterSeconds`; the server reads `heartbeatAfterMs` with the same bounds (1 minute to 8 days) and the same ±10% jitter.
- **Hosted's toolkit copy** (`hosted-services/apikit`) has been re-synced byte for byte from the server's `apikit` with `scripts/shared-apikit.sh --write`. Hosted error bodies now carry `retryAfterMs`, and the `Retry-After` header stays in whole seconds. The copy also picks up the server-side toolkit changes made since the last sync: `Conditional` early 304s, validator comparison before marshalling, and path/query/body id rewrites. Hosted doesn't install a rewrite, so the rewrites do nothing there.
- Hosted's generated contracts (`api/hosted.openapi.json`, `contracts/src/hosted/schemas.ts`, `types.ts`) were regenerated. The only change in them is the renamed field. The error envelope isn't part of Hosted's generated schemas, so it needed no regeneration.
- Quick Connect / the device flow (`expiresIn`, `interval`) is unchanged, as the brief asked.

No other duration fields exist on Hosted's wire. I swept all JSON tags and query parameters: `after`, `through`, sequences and revisions are counters, `expires` in the cursor is an opaque token, and `etaSeconds` appears only in a toolkit test type. Nothing on web, native or Vega read either field: clients take Hosted's retry wait from the header. So the native worktree is unchanged.

## 2 Decisions I made

- The server's ceiling is a duration (`maxCheckIn = 8 * 24 * time.Hour`) compared in milliseconds, replacing the old seconds constant.
- No compatibility path: Hosted will be redeployed on an empty database (Justin), and the server and Hosted ship together. A server with this change can't check in against the Hosted that is currently deployed: it would refuse the answer as invalid and retry with backoff. **Deploy both together.**
- I didn't re-mirror the shared packages into native. No shared package changed: `shared-packages.mjs --check` reports identical to 3d1f2b66. A `--write` would only have moved the lock to a commit that exists only on this branch. The orchestrator's re-mirror after merging covers it.

## 3 Audit bugs / gaps closed

- cp4-ms "Not finished" Hosted follow-up (A4 tail): `retryAfterSeconds` → `retryAfterMs` in Hosted error bodies (toolkit re-sync) and `heartbeatAfterSeconds` → `heartbeatAfterMs` in the check-in (server + Hosted). Tests: the toolkit's `TestMessageLintAndSafeWriter` asserts `"retryAfterMs":2000` and no `retryAfterSeconds`; Hosted's `check_in_signer_test` uses `HeartbeatAfterMs: 60000`; the server's `portico_membership_test` fake Hosted answers `heartbeatAfterMs`, and the check-in path passes with it. The PostgreSQL `integration_test` assertion is updated, but I couldn't run it (see 7).

## 4 API changes

| Service / route | Old | New |
|---|---|---|
| Hosted `POST /v1/servers/{id}/heartbeat` (check-in) answer | `heartbeatAfterSeconds` (int) | `heartbeatAfterMs` (int64, 86400000 or 604800000 today) |
| Every Hosted apikit error body | `retryAfterSeconds` | `retryAfterMs` (header `Retry-After` unchanged, whole seconds) |

## 5 Schema changes

None. Neither the Hosted PostgreSQL baseline nor the server SQLite baseline changed.

## 6 Settings added

None.

## 7 Tests run

- `scripts/shared-apikit.sh` (PORTICO_SERVER_REPO=server-cp5-hosted) → identical to portico-server.
- Hosted toolkit: `go vet ./...` and `go test ./... -count=1` → pass.
- Hosted: `go build ./...` and `go vet ./...` → clean. `go test ./... -count=1` → all pass. The PostgreSQL-backed tests **skipped**: there's no `PORTICO_TEST_DATABASE_URL_FILE`, and the PostgreSQL server binaries that earlier lanes extracted into `/tmp/portico-hosted-postgres` are gone. The system has only client tools (`/usr/lib/postgresql/18/bin` has no `initdb`/`postgres`), and getting the server binaries would mean a package download, which I didn't do. `go run ./cmd/apigen -root ../..` regenerated the contracts; the contract `Generated`/golden tests pass.
- DNS module: `go build ./...` → clean (unchanged; it builds against the re-synced toolkit).
- Server: `go build ./...`, `go vet ./internal/hosted` → clean. `go test ./internal/hosted -count=1` → pass. `go run ./cmd/apigen` → no diff.
- Native: `shared-packages.mjs --check` → identical to 3d1f2b66 (no native change).
- GOCACHE `_program/scratch/gocache-cp5-hosted` deleted at the end.

## 8 Device test notes

Nothing on devices. For the Hosted redeploy: after a server claims with a Portico Account, its first check-in succeeds. The server log shows no `ErrInvalid` on the heartbeat, and the next check-in is scheduled about 1 day out (about 7 days when Hosted can wake it). To check a Hosted error body (e.g. hit a rate limit), confirm it shows `retryAfterMs` alongside a `Retry-After` header in seconds.

Run the PostgreSQL selection from `reports/hosted-followups.md` once a cluster is available. Its integration test checks the check-in answer:
`go test ./internal/postgres ./internal/contract -run 'Test(CheckInDelivers|OlderReplica|SigningRollout|CurrentHostedRestart|Generated|Golden)' -count=1` with `PORTICO_TEST_DATABASE_URL_FILE` set.

## 9 Not finished

- The PostgreSQL-backed Hosted tests haven't been run (no server binaries on this host; see 7).
- Two pieces deliberately stay as they are: Quick Connect's device-flow fields, and client-side `ApiError.retryAfterSeconds`, which is parsed from the `Retry-After` header and isn't a wire field.
- Some files in Hosted were already not gofmt-clean before this lane (`directroute/service.go`, `mailoutbox/*`, `httpapi/claims_envelope_test.go`). I left them alone because they're outside this lane.

## Commits

- portico-server (`audit-implementation/cp5-hosted`): `7b0b6594` Read the Hosted check-in interval in milliseconds.
- portico-internal (`audit-implementation/cp5-hosted`): `975f172` Re-sync the API toolkit copy from portico-server; `5b6a0ab` Send the check-in interval in milliseconds.
- portico-react-native: none.

`audit-implementation/main` was already contained in all three branches, so no merge was needed.
