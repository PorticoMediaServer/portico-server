# Hosted services and signed routes fix report

Source/tests only on ehler-homelab-mini-1. No live endpoints, existing databases, secrets, deployment or Git mutations used. Context sync could not write its read-only lock; supplied revision and readable canonical notes used. Restricted-sandbox snapshots replace commits.

### B-22.1 — Accept the dedicated secure LAN alias
- Status: FIXED
- What I found: Server networking/remote_manager.go:411–418 publishes HTTPS private literals when its trusted TLS listener is ready. Hosted postgres/routes.go:164–175 maps them to directdns/table.go:35's dedicated LAN hostname; DNS server.go:441–445 selects its private LAN address set. Client route-identity.ts:82 previously rejected that legitimate signed form before RouteConnection could probe any candidate.
- Change: Accept only HTTPS l-<32 lowercase hex>.<canonical 96-bit ptc namespace>.direct.getportico.tv as an additional signed LAN candidate form. HTTP eligibility stays restricted to private/local hosts. Signature, audience, canonical origin, expiry, duplicate rejection and server pin/proof checks stay intact. This fixes the consumer rather than replacing publicly trusted TLS names with certificate-incompatible IP literals.
- Commit: snapshot hosted-routes/01-B-22.1 "Accept signed secure LAN route aliases"
- Tests: Added packages/client-core/test/route-identity.test.ts: "signed secure LAN aliases preserve the LAN and public candidates" failed before the change with invalid_routes, then passed. "signed LAN aliases require the exact dedicated HTTPS name" rejects plaintext aliases, public aliases, malformed/noncanonical namespaces, foreign suffixes, extra labels, credentials and paths, plus wrong audience and signature. Targeted Node suite passed 2/2; npm run test:core passed 1456/1456; npm run check --workspace portico-web passed including 400 web tests and its lints/typecheck.
- Not verified: No device, browser, real DNS, TLS listener, or live Hosted connection tested.
- Docs to update: Clarify the exact secure LAN alias in Networking Contract §5/§13 when maintaining its implementation details; no architecture/spec edits made.
- Later deploy: Rebuild server-bundled web clients and mirror the two packages/client-core files into native before client builds. No Hosted/DNS/server Go binary change, schema migration, data rewrite, certificate issuance or key rotation required.

### B-22.2 — De-duplicate final signed origins
- Status: FIXED — UNTESTED ON full Hosted module / PostgreSQL
- What I found: Hosted internal/postgres/routes.go:164–175 rewrites multiple HTTPS private IPv4/IPv6 literals on one port to exactly the same LAN alias. directory/routes.go:118 only de-duplicates the incoming literal observations, while signRoutes at :146 signs the projected array unchanged. Client route-identity.ts:90 still correctly refuses duplicate signed origins.
- Change: De-duplicate by final origin within each server's projected RouteSet, after all LAN/public name normalization. Preserve distinct ports and plaintext/private origins. For a collision keep the newest generation and, within that generation, current reachable evidence. This also covers current/previous public names that normalize to one current alias. No query, DNS-address mapping, signature check, schema or egress restriction changed.
- Commit: snapshot hosted/01-B-22.2 "Deduplicate signed route origins after name normalization"
- Tests: Added internal/postgres/routes_candidates_test.go / TestRouteSetsDeduplicateNormalizedOrigins (same-port IPv4/IPv6 collapse, distinct ports, plaintext, rotated public labels, generation/quality preference, and per-server isolation). The test failed for duplicated production RouteSets output before the fix, then passed in a dependency-free isolated harness under /tmp/hosted-route-projection-test; that harness copies the unchanged production method, route structs and DNS name helpers directly from source. Harness go test and go vet passed. Added internal/postgres/routes_secure_lan_test.go / TestSecureLANRouteDocument to exercise observation, PostgreSQL projection, signature, LAN/public candidates and selected dual-family DNS addresses together; not executed here.
- Not verified: Owning module go test ./internal/postgres ./internal/directory -count=1 and go vet ./internal/postgres ./internal/directory were attempted with GOCACHE=/tmp/gocache-codex GOPROXY=off GOTOOLCHAIN=local, but internal/postgres could not compile: pgx/v5, go-oidc/v3, miekg/dns and oauth2 are absent from the local module cache. The adjacent internal/directory unit tests passed in that same command, and its separately run go vet passed. No dependency downloads performed. PostgreSQL initdb/postgres/pg_ctl are absent (only client tools installed); no database connection succeeded or was attempted against an existing instance. Isolated logic execution is not a full-module compile or database verification.
- Test note: On an isolated test host with this module's declared dependencies already cached and PostgreSQL 18 server tools installed, create a fresh cluster beneath /tmp with a unique Unix socket and empty listen_addresses; create a database containing "test" in its name and write its URL to a mode-0600 file. From hosted-services/hosted run GOCACHE=/tmp/gocache-codex PORTICO_TEST_DATABASE_URL_FILE=<that file> go test ./internal/postgres ./internal/directory -count=1, then go vet ./internal/postgres ./internal/directory. TestSecureLANRouteDocument must run (not SKIP), verify the signature and yield four unique candidates, retaining a remote candidate and both private DNS families; all tests must pass. Stop and delete only the owned cluster. The source-only harness can be regenerated here with python3 /tmp/hosted-route-projection-test.py then GOCACHE=/tmp/gocache-codex GOPROXY=off GOTOOLCHAIN=local go -C /tmp/hosted-route-projection-test test ./internal/postgres -count=1.
- Docs to update: None made stale. The exact LAN alias and its bounded one-address-per-family DNS set can be clarified alongside B-22.1; interface-specific aliases remain outside this fix.
- Later deploy: Rebuild Hosted binary only. Existing reports are normalized at read/sign time; no data migration, key rotation, certificate reissue, DNS-node binary or server Go binary change required. Deploy with the client B-22.1 correction for usable secure LAN documents.

### N-V01.1 — Learned signer revocation and existing server sessions
- Status: SKIPPED
- What I found: server/internal/hosted/trust.go:82–95 persists root revocations and revokes only retired Hosted-policy authority families selected by hosted_policy_signers. The current hosted/identity_assertion.go:175 passes account identity/consent, identity/portico_accounts.go:101–115 issues a local family and stores only a Portico-admission marker; the baseline schema at persistence/migrations/0001_baseline.sql:3228–3231 records no signer for families or factor challenges. Profile/TV descendants inherit that boolean, so the server cannot target families admitted by a particular compromised signer.
- Reason: Identity Contract §5 (:165–177) establishes separate Hosted/server-native revocation boundaries and treats verified revocation as terminal but does not say whether documents-signer revocation withdraws previously admitted server authority. ACCOUNT_SECURITY.md:125–144 describes Hosted account changes, not emergency documents-signer compromise; Hosted at Scale §9 (:207–209) explicitly keeps server sessions independent of ordinary Hosted sign-out/password changes. An emergency exception is reasonable but not explicit enough to decide here. The second review itself asks Justin to confirm this policy. Furthermore, targeted retirement requires durable per-family signer provenance, including MFA and derived profile/TV families, hence a schema migration that the shared brief expressly prohibits. Revoking every linked/direct family or reconstructing provenance from latest mutable account consent would affect unrelated authority and is not a correct substitute. No existing D-number specifically resolves this question.
- Commit: None (no source changes or snapshot).
- Tests: Existing GOCACHE=/tmp/gocache-codex GOPROXY=off GOTOOLCHAIN=local go -C server test ./internal/hosted -run '^TestRootRevocation' -count=1 passed. Those tests establish permanent old-key refusal and legacy Hosted-policy family withdrawal while preserving local families; they do not cover retirement of current Portico-admitted families.
- Not verified: The current-model persistence gap is source-confirmed, not reproduced in a new session compromise experiment. No runtime deployment or live signer queried.
- Docs to update: Identity Contract §5 and Hosted trust incident guidance must specify whether learning a signer revocation retires existing admitted families, which descendants inherit provenance, and whether unrelated direct/recovery sessions remain active.
- Later deploy: No deploy change in this lane. A future approved targeted fix would need a server binary plus a provenance schema migration; Hosted would deliver its existing root-signed revocation. Existing families' missing provenance would need an explicit retirement/backfill policy. No key operation performed.

### B-25.5 — Synchronize and enforce the shared toolkit copy
- Status: FIXED — UNTESTED ON GitHub Actions
- What I found: hosted-services/apikit/generate.go:65 omits canonical apikit's explicit int64 bounds, so its 32-bit build overflows. scripts/shared-apikit.sh:16 already correctly exits 1 on that difference; the enforcement defect is that .github/workflows/ci.yml never invoked it. First-pass and second-review material were checked (V06 carries this Low finding without rechecking).
- Change: Copy the canonical generate.go verbatim. CI's repository job now checks out only canonical apikit at server source revision 82aaa54f89efdc2e029b635746627cd8c8b71960 and runs the existing drift check before the existing repository guard. The pinned source is this lane's server base and is contained by the locally fetched origin/codex/performance-resilience ref; no network query was used. Checked copies remain the existing ownership model; this does not settle D-49's alternative publishing/repository arrangements.
- Commit: snapshot hosted/02-B-25.5 "Keep the shared API toolkit synchronized and check drift in CI"
- Tests: Before the fix, the existing shared-apikit comparison exited 1 and GOARCH=386 compilation reported overflow at generate.go:65. After copying, the full recursive comparison passes, and go test ./... plus go vet ./... pass in both hosted-services/apikit and the unchanged canonical server-worktree apikit, with GOCACHE=/tmp/gocache-codex GOPROXY=off GOTOOLCHAIN=local. GOARCH=386 go test -exec /bin/true ./... passes compilation for every copied apikit package (test binaries intentionally not executed). PyYAML parsing and a disposable local copy executed the CI comparison command: identical sources exit 0; changed-file and extra-file drift each exit 1. sh -n scripts/shared-apikit.sh and git diff --check pass.
- Not verified: Actual GitHub Actions checkout and workflow were not run, and no remote operation was performed. The first 386 go test attempt could not execute other package binaries under this sandbox's seccomp restrictions (bad system call), so 386 runtime tests are not claimed. The later compilation-only check passed.
- Test note: After the coordinator commits this snapshot, run the normal repository CI job on GitHub Actions. Its canonical-toolkit checkout must fetch revision 82aaa54f89efdc2e029b635746627cd8c8b71960, and "Check shared API toolkit" must print "shared-apikit: identical to portico-server" and succeed. A missing/inaccessible canonical revision or repository fails this gate and needs review; it must not be bypassed. Local fixture checks already prove changed/extra source files fail with exit 1. No live Hosted infrastructure is involved.
- Docs to update: None made stale. For future canonical apikit updates, update the copied source and CI's immutable source revision together.
- Later deploy: CI/configuration enforcement requires no deployed binary. Rebuilding Hosted incorporates the copied generator fix; generated schema integer bounds/JSON output are unchanged on amd64. No database data/schema, certificates or key changes.



PostgreSQL test follow-up commands, from the internal worktree root on the stated isolated test host (PostgreSQL 18 binaries and Go dependencies already present):
```sh
set -eu
cluster_dir="$(mktemp -d /tmp/portico-hosted-test.XXXXXX)"
umask 077
mkdir "$cluster_dir/socket"
/usr/lib/postgresql/18/bin/initdb -D "$cluster_dir/data" -A trust > "$cluster_dir/init.log"
trap '/usr/lib/postgresql/18/bin/pg_ctl -D "$cluster_dir/data" -m fast -w stop; rm -rf "$cluster_dir"' EXIT
/usr/lib/postgresql/18/bin/pg_ctl -D "$cluster_dir/data" -l "$cluster_dir/postgres.log" -o "-k $cluster_dir/socket -p 55438 -c listen_addresses=''" -w start
/usr/lib/postgresql/18/bin/createdb -h "$cluster_dir/socket" -p 55438 portico_hosted_test
printf '%s\n' "postgres:///portico_hosted_test?host=$cluster_dir/socket&port=55438&sslmode=disable" > "$cluster_dir/url"
GOCACHE=/tmp/gocache-codex GOPROXY=off GOTOOLCHAIN=local PORTICO_TEST_DATABASE_URL_FILE="$cluster_dir/url" go -C hosted-services/hosted test ./internal/postgres ./internal/directory -count=1 -v
GOCACHE=/tmp/gocache-codex GOPROXY=off GOTOOLCHAIN=local go -C hosted-services/hosted vet ./internal/postgres ./internal/directory
```
Pass requires TestSecureLANRouteDocument to run and PASS, plus the owning suite/vet to pass. Any SKIP for that test, build failure, repeated signed origin, absent remote route or wrong selected IPv4/IPv6 is a failed verification. The unique private socket points only to the cluster these commands create; there is no TCP listener.

## Snapshots and final verification

Counts: 3 fixed (B-22.2 carries the full-module/PostgreSQL limitation; B-25.5 carries the GitHub Actions limitation), 1 skipped, 0 not-a-bug, 0 already-fixed. No commits were created because the lane's Git metadata is read-only; git log --oneline <base>..HEAD is empty in both worktrees. Bases remain internal 19cc2a7eb6fb40ce675decd65036769c95b42c5d and server 82aaa54f89efdc2e029b635746627cd8c8b71960.

Snapshots in implementation order:
1. hosted-routes/01-B-22.1 — "Accept signed secure LAN route aliases"
2. hosted/01-B-22.2 — "Deduplicate signed route origins after name normalization"
3. hosted/02-B-25.5 — "Keep the shared API toolkit synchronized and check drift in CI"

The latest cumulative snapshot in each worktree was compared byte-for-byte with its current tracked diff, untracked list and archived untracked file contents; both exactly match. No missing snapshot or remaining source edits beyond those snapshots.

Every changed file under packages/ (canonical server worktree; mirror later):
- packages/client-core/src/route-identity.ts
- packages/client-core/test/route-identity.test.ts

Final commands/results:
- Server-worktree root: npm run test:core — PASS, 1456/1456 (final rerun after the exact canonical namespace guard); npm run check --workspace portico-web — PASS, lints/typecheck and 400/400 web tests. Logs: /tmp/hosted-routes-core-final.log and /tmp/hosted-routes-web.log.
- Server-worktree root: node --experimental-strip-types --test packages/client-core/test/route-identity.test.ts — PASS, 2/2. The acceptance regression failed before its implementation.
- Both roots' apikit modules: GOCACHE=/tmp/gocache-codex GOPROXY=off GOTOOLCHAIN=local go -C <apikit directory> test ./... and vet ./... — PASS. Logs: /tmp/hosted-apikit-tests.log and /tmp/hosted-routes-apikit-tests.log.
- Internal-worktree root: GOCACHE=/tmp/gocache-codex GOPROXY=off GOTOOLCHAIN=local GOARCH=386 go -C hosted-services/apikit test -exec /bin/true ./... — compilation PASS; runtime tests not executed. Before synchronization the 386 compiler overflowed at generate.go:65. Log: /tmp/hosted-apikit-386-compile.log.
- Internal-worktree root: PORTICO_SERVER_REPO=/home/justin/projects/Portico/.worktrees/audit-fixes/server-hosted-routes scripts/shared-apikit.sh — PASS, trees identical. Exact CI comparison command tested with a disposable fixture: equality PASS, changed-file/extra-file drift each exit 1. Workflow parsed as YAML and pinned source verified from local Git refs; no remote checkout. sh -n scripts/shared-apikit.sh — PASS.
- Internal-worktree root: GOCACHE=/tmp/gocache-codex GOPROXY=off GOTOOLCHAIN=local go -C hosted-services/hosted test ./internal/postgres ./internal/directory -count=1 — internal/directory PASS, internal/postgres BLOCKED before compilation by missing cached dependencies. Corresponding combined go vet BLOCKED; separate go vet ./internal/directory PASS. Logs: /tmp/hosted-lane-go-tests.log and /tmp/hosted-lane-go-vet.log.
- Source-copied isolated projection: python3 /tmp/hosted-route-projection-test.py; GOCACHE=/tmp/gocache-codex GOPROXY=off GOTOOLCHAIN=local go -C /tmp/hosted-route-projection-test test ./internal/postgres -count=1 and vet ./internal/postgres — PASS. Before the fix the same projection test failed for duplicate origins. This is deliberately limited to actual projection logic and does not establish a full Hosted package compile, PostgreSQL query behavior or signature integration.
- Server-worktree root: GOCACHE=/tmp/gocache-codex GOPROXY=off GOTOOLCHAIN=local go -C server test ./internal/hosted -run '^TestRootRevocation' -count=1 and go -C server vet ./internal/hosted (same environment) — PASS. Log: /tmp/hosted-routes-revocation.log.
- Changed Go files: gofmt -l with GOCACHE=/tmp/gocache-codex — no output. git diff --check — PASS in both worktrees.

Outside assigned scope: No additional bugs investigated or changed. Existing LAN DNS publication chooses one private address per family; this lane preserves that mapping and makes no interface-selection decision. No live service/DNS/server calls, dependency installs/downloads, secret reads, existing-database access, deployment, remote Git operations or Git metadata writes were used.

Review first: full Hosted/PostgreSQL integration remains unverified; run TestSecureLANRouteDocument and the owning suite before relying on B-22.2. N-V01.1 remains an open security gap pending explicit compromise-retirement policy and authorization for a provenance migration.

## After review

Read the Hosted review sections for 6777e0b, 7e4ffcb and 05d7708. Both worktrees began this pass clean at their committed tips (internal 05d7708; server 6777e0b). This section supersedes the earlier B-25.5 pinned-source description and GitHub Actions test note; earlier sections retain the first-pass history. Context refresh again could not write its sandbox-protected lock; relevant readable guidance was used.

### B-25.5 — Off-main CI revision (Should-fix)

- Status: FIXED — UNTESTED ON GitHub Actions.
- Verified: .github/workflows/ci.yml:16 pinned 82aaa54f89efdc2e029b635746627cd8c8b71960. Local git branch -r --contains shows only origin/codex/performance-resilience, not main; git show origin/main:apikit/generate.go confirms untyped bounds remain in the tracked main revision 2eb0e7557dd1dc7b1da66256d9b70178f724af40. No local main/tag carrying the corrected toolkit was found. The reviewer is correct: a fetched feature-branch commit is not a stable published-main/tag pin.
- Change: Remove the source SHA and comment that the canonical checkout compares the published default branch and requires canonical fixes to be published there before this gate is expected to pass. This follows the reviewer's drop-ref alternative and the existing native repository check. Only .github/workflows/ci.yml changed; sparse checkout, read-only credentials and strict failure on drift remain intact.
- Assumption / D-49: Use the existing native default-branch comparison as the narrow correction for now. The canonical int64 fix must reach the default branch before Hosted CI is expected to be green. The locally recorded main still differs, and the gate correctly fails that genuine drift. A permanent cross-repository source-pinning policy remains Justin's open decision; no replacement off-main pin or new publishing model was introduced. Actual remote state was not queried or modified.
- Snapshot: hosted-r2/01-B-25.5-ci — "Compare the shared API toolkit against the published default branch".
- Checks: python3 /tmp/hosted-r2-check-ci.py failed before the change on the feature-branch dependency assertion, then passed after it. The test parses actual YAML, checks default-branch checkout settings, executes the exact comparison command against a disposable local fixture, verifies equality exits 0 and changed/extra files each exit 1, then uses git archive of locally fetched origin/main to verify actual main drift still exits 1. PORTICO_SERVER_REPO=/home/justin/projects/Portico/.worktrees/audit-fixes/server-hosted-routes scripts/shared-apikit.sh passes; sh -n scripts/shared-apikit.sh and git diff --check pass. Snapshot patch and untracked list match the new work byte-for-byte. No Go/TS source changed, so previously completed suites were not repeated.
- Revised test note: After the canonical integer-bounds fix reaches portico-server's published default branch and this snapshot is committed, run the normal internal repository CI job on GitHub Actions. The canonical checkout must resolve the default branch and create .scratch/portico-server/apikit; "Check shared API toolkit" must print "shared-apikit: identical to portico-server" and pass. Before canonical publication, generate.go:65 drift is an expected failure, not grounds to bypass or relax the gate. Future canonical changes require synchronizing the copy. Real GitHub checkout/workflow execution remains unverified.
- Later deploy / docs: CI-only correction; no deployed binary, database, key or certificate changes. No product document made stale. General policy D-49 stays open.

### B-22.1 — Accepted secure LAN rule and optional nits

- Accepted fix unchanged (6777e0b); no new snapshot.
- Hard-coded-zone nit: Deferred. Hosted and clients currently use the same production zone. Staging/custom-zone support or a new shared configuration model is beyond this narrow correction.
- Native-mirror nit: Remains a coordinator task as originally instructed. Mirror packages/client-core/src/route-identity.ts and packages/client-core/test/route-identity.test.ts from the canonical server repository before native builds. This lane did not edit the native copy.
- Checks: Re-read the accepted trust-boundary review; no new route edits or repeated client suites. First-pass client-core/web results remain the evidence.

### B-22.2 — Accepted de-duplication and PostgreSQL verification

- Accepted fix unchanged (7e4ffcb); no new snapshot.
- Equal-generation/already-reachable replacement nit: Deferred; reviewer confirms it is harmless and does not require a defect correction.
- One-address-per-family/link-local nit: Pre-existing producer mapping remains out of scope.
- Status remains FIXED — UNTESTED ON full Hosted module / PostgreSQL. The real postgres package has not compiled or run in either the lane or independent review because declared modules are uncached; current PostgreSQL server tools are also absent. Source-copied harness execution is limited logic evidence.
- Exact environment: Disposable Ubuntu test host, regular non-root user, locally installed Go 1.26.2, PostgreSQL 18 server/client binaries under /usr/lib/postgresql/18/bin (17 also acceptable if substituted), and all direct/transitive versions in hosted-services/hosted/go.mod/go.sum and hosted-services/dns/go.mod/go.sum already available in the Go module cache. In particular provision pgx/v5, go-oidc/v3, miekg/dns and oauth2 and their declared transitive dependencies before assigning the test run. GOPROXY=off forbids dependency downloads. No Hosted daemon, existing database or live endpoint is needed.
- Exact commands: From /home/justin/projects/Portico/.worktrees/audit-fixes/internal-hosted, or an isolated checkout containing the accepted fixes, run the following full-suite procedure.

```sh
set -eu
export GOCACHE=/tmp/gocache-codex GOPROXY=off GOTOOLCHAIN=local
hosted_pg_bin=/usr/lib/postgresql/18/bin
for executable in initdb pg_ctl createdb; do
  test -x "$hosted_pg_bin/$executable"
done
go version
cluster_dir="$(mktemp -d /tmp/portico-hosted-review-test.XXXXXX)"
umask 077
mkdir "$cluster_dir/socket"
cleanup() {
  "$hosted_pg_bin/pg_ctl" -D "$cluster_dir/data" -m fast -w stop >/dev/null 2>&1 || true
  rm -rf -- "$cluster_dir"
}
trap cleanup EXIT
"$hosted_pg_bin/initdb" -D "$cluster_dir/data" -A trust > "$cluster_dir/init.log"
"$hosted_pg_bin/pg_ctl" -D "$cluster_dir/data" -l "$cluster_dir/postgres.log" -o "-k $cluster_dir/socket -p 55438 -c listen_addresses=''" -w start
"$hosted_pg_bin/createdb" -h "$cluster_dir/socket" -p 55438 portico_hosted_test
printf '%s\n' "postgres:///portico_hosted_test?host=$cluster_dir/socket&port=55438&sslmode=disable" > "$cluster_dir/url"
export PORTICO_TEST_DATABASE_URL_FILE="$cluster_dir/url"
go -C hosted-services/hosted test ./... -count=1 -v
go -C hosted-services/hosted vet ./...
```

- Procedure check: bash -n /tmp/hosted-pg-review-followup.sh passes (syntax only; not executed).
- Pass/fail: Go must report the declared local toolchain. The full Hosted suite and vet must succeed; TestRouteSetsDeduplicateNormalizedOrigins and TestSecureLANRouteDocument must explicitly RUN and PASS, not SKIP. The latter checks signature, four unique candidates including the remote origin, and selected IPv4/IPv6 values 192.168.1.8 / fd00::8. Any dependency/build error, skipped PostgreSQL test, duplicate origin, missing remote candidate, invalid signature, wrong private address or owning-suite failure means verification has not passed. The unique mode-0700 parent holds a private Unix socket; listen_addresses is empty, so no TCP listener exists. Cleanup stops/removes only this owned cluster.
- Deployment order remains client-first or simultaneous for B-22.1, then Hosted for B-22.2. No schema/key/certificate changes. B-22.2 must not be called verified until the real full suite passes.

Review-pass snapshot order: hosted-r2/01-B-25.5-ci only. No hosted-routes-r2 snapshot is due: that worktree remains clean and unchanged. No additional findings changed. No remote Git operations, dependency downloads, live endpoints, secrets, existing databases or Git metadata writes used.
