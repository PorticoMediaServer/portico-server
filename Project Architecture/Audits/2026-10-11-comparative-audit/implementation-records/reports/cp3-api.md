# cp3-api — API platform and freshness (15 Oct 2026)

Branch `audit-implementation/cp3-api` in `server-cp3-api` and `native-cp3-api`, merged with `audit-implementation/main` (13c406d9 / main-native at merge time) in both. No subagents or helpers. Nothing pushed.

## 1 Outcome

- **Webhook edits are revision-fenced (A1).** A webhook carries `revision`; `PATCH /v1/admin/webhooks/{id}` needs `expectedRevision` (428 without, 412 `revision_mismatch` with the current webhook when stale). One edit is one revision. New `GET /v1/admin/webhooks/{id}`. Web and phone/tablet send only the fields the owner changed. When someone else changed something else, the client reads the webhook again and re-sends quietly. Only a change to the same field to a different value is shown. The MCP `update_webhook` tool reads the revision first.
- **Saved-resource conflicts (A2)** on playlists, saved lists, pins and collection order now answer the registered `412 revision_mismatch`, and every contract lists it. `playlist_conflict` is gone from the server and from client-core's server-code lists.
- **Conditional GET (A3)** now runs on `GET /v1/me/devices/current/capabilities` (the revision is read first; the document is not parsed or encoded on a match) and on `GET /v1/rating-systems` (a build-constant digest). Both answer 304 before composing.
- **Freshness hints (A13):**
  - New `list.changed` hints: `guide` (everyone; a source added, removed, re-imported or switched on or off), `myRecordings` (the profile's own recordings and rules), `myDownloads` (the profile's own preparations and receipt revocations) and `recommendations` (everyone; the provider trending list, or someone starting a play, for Trending and Popular).
  - `personal.changed`/`progress.changed` now carry `data.titles`: the title, then its parent and grandparent. They carry it only while every change the hint stands for was about that title or one of its ancestors. Two unrelated titles leave it out, which means "any title".
  - client-core gains `titleWithPersonalState` and `dependsOn.personalTitle/progressTitle`.
  - On web and on native/TV/Vega (shared screens), the Guide, DVR/Recordings and Downloads read again on these hints instead of the 30-second clock. Web Downloads keeps a 30 s clock only while something is being prepared, because progress has no hint. Native Recordings and Downloads previously had no live refresh at all.
  - Home and Home row pages depend on `recommendations`. Title pages (detail, show workspace) refresh only for their own title.
- **Read-one-by-id (A14):** `GET /v1/admin/access/members/{id}`, `/invitations/{id}`, `/devices/{id}` (admin scope; a device the caller may not decide about is 404), and the webhook route above. client-core `readAccessRecord`. Web People/Access and native PeoplePanels use them instead of walking up to 32 list pages.
- **A15:** maintenance prunes `identity_credential_attempts` rows a day past their last failure and past any delay, in batches of 100, on the existing 15-minute sweep. Index added.
- **A25:** a sign-in that names no installation gets an `unnamed-` installation id. Before a new one is made, the account's waiting unnamed records beyond the newest 4 are removed, and only those with no session family or refresh credential.
- **API keys usable for almost everything (Justin's addition):**
  - Moved to **admin** scope: profiles (create, edit, order, picture, PIN reset, deletion preview, delete), members (`/v1/direct/members*` including deletion and its preview), `/v1/devices*` (list, rename, approve, sign out, forget), `/v1/direct/sessions` (list, sign out one), owner device trust, and forgetting a Cast device.
  - `GET /v1/direct` is **read**. Client-log upload is **playback**.
  - `directCallerTx` accepts an edge-admitted key or token, so these act as the member and profile that made it.
  - Still session-only, with reasons in the `apikit.Scope` comment: sign-in, sign-out (including sign-out-everywhere), refresh, register, password, two-factor, PIN recovery, keys and tokens, ownership, Cast pairing and TV codes, invitation accept, remembered accounts, profile select/switch, device registration and session binding, networking claim and identity proofs, setup.
  - Keys may travel as `?apiKey=` on GET/HEAD. The router lifts the key into `Authorization` and strips it from the address before anything else reads the request: logs, handlers and errors never see it. It is dropped on other methods. A session token in the address is ignored. Header plus query is refused as ambiguous.
  - The cross-origin check (cp3-network's `browser_origin.go`) already sees the lifted header; a comment notes this and a test covers it.
  - `/v1/me/api-access` publishes `queryParameter`. The guide (07, 06) and the scope strings are updated.

## 2 Decisions I made

- Saved-resource conflicts use the existing `revision_mismatch` (412) rather than a new code.
- Webhook delete stays unfenced (removing is the same intent whatever changed). Deliveries and failure counters do not move the revision.
- The title naming in personal hints uses "same title or one of its ancestors, keep the longer chain", otherwise unnamed. This is correct by construction (unnamed = old behaviour) and costs at most three primary-key reads per change. Container undo always sends unnamed.
- The Guide has no clock at all: "now" is drawn on the device, and programme data only changes when the guide is brought in again (hinted).
- Device and session listing and revocation are **admin** scope, not read (they show addresses and end sign-ins). A key can still never end another key: the schema triggers that protect credential families already refuse it.
- Query parameter name `apiKey`. Signed addresses keep their other parameters verbatim.
- Reworded `api.scopes.note` and `api.scope.admin.help` rather than adding new strings.

## 3 Audit items closed

| ID | Change | Test |
|---|---|---|
| A1 | webhook revision fence, get-one, client silent retry, MCP | `webhook_routes_test.go`, `webhooks_test.go`, client-core `api-access.test.ts` (new retry test), native `owner-rules.test.ts` |
| A2 | `ErrPlaylistConflict` → 412 `revision_mismatch`; contracts list it | client-core `saved`/`personal-saved` tests updated; httpapi playlist/saved tests pass |
| A3 | `Conditional` on capabilities and rating systems | `playback_v1_options_test.go`, `foundation_test.go` (304 cases) |
| A13 | new list hints, title-named personal hints, clients on hints | catalog `TestPersonalHintNamesTheTitle`, apievents `TestPublicTitles…`, client-core hub tests (2 new) |
| A14 | three admin read-one routes and clients | covered by build/type-check; route matrix entries added |
| A15 | attempt-ledger prune | not unit-tested (one bounded DELETE) |
| A25 | bounded unnamed waiting devices | identity package tests pass; no new test |
| Keys | scope moves, credential direct caller, `?apiKey=` | `TestAdminKeyManagesTheAccountButNeverCredentials`, `TestQueryCredentialLeavesTheAddress`, updated `TestAPIKeyIsShown…`, `TestOperationsCredentialScopesAndRestart` |

## 4 API changes

- New: `GET /v1/admin/webhooks/{id}`, `GET /v1/admin/access/members/{id}`, `GET /v1/admin/access/invitations/{id}`, `GET /v1/admin/access/devices/{id}`.
- `PATCH /v1/admin/webhooks/{id}` requires `expectedRevision`; `Webhook` gains `revision`.
- Playlist, saved-resource, pin and collection-order writes: 409 `playlist_conflict` → 412 `revision_mismatch`.
- `CredentialOptions` gains `queryParameter`.
- Scope moves are listed in §1.
- Events: new `list.changed` ids `guide`, `myRecordings`, `myDownloads`, `recommendations`; `data.titles` on personal and progress hints.

## 5 Schema changes (baseline)

- `viewer_revisions.title_path`; every writer of `viewer_revisions` now sets it.
- Personal hint triggers coalesce titles.
- New list-hint triggers on `live_sources`, `dvr_recordings`, `dvr_rules`, `download_preparations`, `download_receipts`, `metadata_discovery_items` and `personal_history`.
- Index `identity_credential_attempts_age`.
- `schema-sync` run.

## 6 Settings added

None.

## 7 Tests run

- Go:
  - `go build ./...`; `go vet` on access, identity and webhooks.
  - `go test` on webhooks, apievents, identity and access (all ok).
  - Targeted httpapi runs (Webhook, scope, credential, matrix, MCP, device, direct, profile, member, session, contract, registry, cast; after fixes, all ok).
  - catalog personal-freshness tests.
  - `go run ./cmd/apigen`.
- Server repo: `node --test` over all of client-core (2010 pass); web `tsc --noEmit`, `lint:hooks`, `lint:strings` clean.
- Native: `tsc` (app and tests), `lint:hooks`, `lint:strings`, `owner-rules` and `fresh-send` tests (41 pass); Vega `tsc` clean.
- No full suites, drivers or emulators.

## 8 Device test notes

- **Two phones on one profile:** start a download on one; within seconds the other's Downloads shows it with no pull. Set a recording from the web guide; the phone's Recordings and guide badge update. On TV, the guide updates after the owner refreshes a source.
- **Owner, two devices:** both open the same webhook. One turns it off; the other renames it; both saves succeed silently. Both change the address: the second sees the conflict.
- **Script:** with an admin key, `curl 'http://server:port/v1/items/{id}/artwork…?apiKey=pk.…'` works. The server log must not contain `pk.`.

## 9 Not finished

- A3: only two typed routes adopt `Conditional`. Others either have no cheap complete validator (queues and windows depend on title metadata; maintenance settings include pause state) or already skip encoding through their revision ETag.
- Title-scoped refresh is wired for the detail page and the show workspace only. Entity/content lists and Person pages still use library-wide personal dependencies.
- No dedicated tests for A14 routes, A15 pruning or A25 pruning.
- Native has no web-style "keep a clock while preparing" for download progress. Native Downloads refreshes on hints and pull only (the device's own transfers report themselves).

## Commits

- server: `b4a237c9`, `5103947f`, `2417687f`, `35e17ffc`, `08448448`, `22f2603e`, `00362e35` (merge), `fe93de0b`
- native: `6458e7b` (mirror), `ef49cf3`, `1c2ef66` (merge with re-mirror from `fe93de0b`)
