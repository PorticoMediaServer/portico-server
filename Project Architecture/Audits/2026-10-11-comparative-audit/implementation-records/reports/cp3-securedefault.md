# cp3-securedefault — secure connection by default

## 1 Outcome

- **First run, web.** "Use a Portico Account" is still the recommended choice. It now carries straight on from creating the server into approving it in that account. The Portico Account window opens during the Create click, so pop-up blockers allow it. The person signs in there if they need to and approves. The server then installs the claim and wakes the certificate manager at once. The certificate setting was already on by default, so the certificate is requested straight away: a secure connection is on from the start. The setup page then signs the owner in. There is no trip to Settings, no separate Connect step and no external help page. While it waits, the page offers "Open Portico Account" (in case the window was blocked or closed) and "Do this later" (signs in; Settings then says how to connect). On a failure it shows the catalogue message and "Start again".
- **The server finishes the claim itself.** Once the web approval is registered, the server holds Hosted's decision stream (`/v1/server-claims/events`). Before this change, the browser's open `await-approval` request held it. A closed setup tab, an approval made on a phone, or two windows now all end in the same installed claim and certificate request. `await-approval` now just waits on the server's watch: several pages share one Hosted stream, and a late page reads the decision the watch left. A stream that breaks before any decision ends the watch, and the next wait asks Hosted again.
- **Server settings › Remote access › Secure connection** (web and phone/tablet, from the shared settings structure) is now the first section after the reachability line. It has:
  - **Status:** one plain sentence with a badge (On / Starting / Needs attention / Off / Waiting). When the server knows its secure address, the sentence names it.
  - **What it does:** the section description.
  - **Switch:** "Use a secure connection". It is the certificate setting, shown only once the server has a Portico Account connection to switch.
  - **Certificate details:** name, issuer, expiry, renewal and secure address, plus Try again on a problem.
  - **Use your own certificate:** the existing own-certificate section, moved directly under it.
  - **Port:** the certificate's public port moved to Advanced as "Secure connection port". It is shown only once connected.
- **Without a Portico Account.** Plain HTTP keeps working. The status sentence says: connect this server to your Portico Account (Portico then gets and renews a certificate by itself), or use your own certificate below. On a build without the certificate service, it points only to your own certificate.
  - **Web:** "Turn on secure connection" runs the existing approval flow.
  - **Phone/tablet signed in to a Portico Account:** "Turn on with <account>" does it in the app. It prepares the claim for that account, gets the approval envelope from Hosted's `/v1/server-claims/approve` with the app's own account sign-in, then installs; the certificate follows.
  - **Phone/tablet without an account in the app:** "Turn on in a browser" opens the server's Remote access page.
- **`SECURE_CONNECTION_HELP_URL` is gone** (shared package, web and native). The signed-out plain-HTTP note now reads: "This connection uses HTTP outside your home networks. The server's owner can turn on a secure connection in Settings › Remote access." There is no link.
- **Hosted:** no code change was needed. The web-claim registration, decision stream, code approval, account approval and certificate services are reused as they are. `Project Architecture/07 Hosted Services.md` now describes the secure default.

## 2 Decisions I made

- **Approval stays explicit.** The local web UI cannot hold a Portico Account session (Hosted only accepts its own origins), so "signed in" during first-run setup means the person signing in and approving in the account window. That approval is the act that creates the server in the account. I did not add auto-approval, because it would let anyone tie a server to an account without that account's say-so.
- **The phone/tablet one-tap path uses the account's first profile as owner profile.** That is the same default the account approval page proposes.
- **The server-side watch reuses the owner's request guard.** It is captured from the page that started the approval. If that owner sign-in ends before the decision, the approval is not applied ("approval_changed"), as before.
- **The watch is not restarted after a server restart.** There is no owner guard at boot. A restart while an approval is pending means opening Settings and pressing Turn on again; the same code is reused while it is unexpired.
- **Removed copy.** I removed the certificate status sentences that were hard-coded in English (`certificateSummary`, `certificateBadge`). The new status line is catalogue copy. I also removed the strings that are no longer used: the certificate section title, lede and switch text, the three certificate badge labels, `signIn.plainHttp.link`, and the old setup copy. The copy that changed is under new IDs.
- I edited three existing strings in place because the meaning was unchanged and the wording was stale or now incomplete: `web.claim.lede`, `web.claim.connected` and `web.claim.openFromServer`. The last one used to point at a setup checklist that no longer exists. `web.general.accountConnectionHelp` now mentions the secure connection.
- **The Portico Account connection panel stays below the Secure connection section.** It still has its own Connect, Check status and Cancel actions. Two Connect buttons on one page do the same thing; the secure section's button is the primary one.

## 3 Audit bugs closed and decision gaps closed

- Remaining-sweep **C16** (a website page behind `SECURE_CONNECTION_HELP_URL`) no longer applies: the link is removed and the explanation lives in Settings.
- Justin's 15 Oct answer ("Secure connection: explained simply in Server Settings, and the default for most users…") is delivered for web first run, web settings and phone/tablet settings.
- Web-claim reliability: a closed tab no longer drops an approval. The test is `TestWebClaimWatchBelongsToTheServer`.

## 4 API changes

- Routes and shapes are unchanged. `apigen` produced no diff.
- Behaviour change: `POST /v1/networking/claim/web-approval` now also starts the server's own wait for the decision. `POST /v1/networking/claim/await-approval` joins that wait and keeps the same answers (final claim status, or `{state: approval_closed | approval_changed}`).
  - When Hosted cannot be reached, or the stream breaks, the waiting page now gets `approval_closed` after its 200 header. Before, it got an HTTP error. The web shows "The connection wasn't approved. You can try again." and Try again works at once.
- Client-core:
  - New `server-admin/secure-connection.ts`: `secureConnectionView`, `certificateClaimed`, `claimConnected`, `claimWaiting`.
  - The certificate form's draft is now `CertificateDraft` (config plus a read-only `claimed`).
  - New settings custom panel id `server.secureConnection`.
  - Removed: `certificateSummary`, `certificateBadge` and `SECURE_CONNECTION_HELP_URL`.

## 5 Schema changes

None.

## 6 Settings added

- None new. The existing certificate `enabled` (default on) is now presented as "Use a secure connection", with its own help, in the Secure connection section. It is shown once the server is connected; the screen exists on web and phone/tablet.
- The certificate `publicPort` is now under Remote access › Advanced as "Secure connection port".

## 7 Tests run

- `go test ./internal/networking/ -count=1` → ok (whole package, 411 s), including the new `claim_web_watch_test.go`.
- `go build ./...` → ok. `go vet ./internal/networking/` → ok. `go run ./cmd/apigen` → no change.
- `node --test packages/client-core/test/*.test.ts` → 1992 pass, including the new `secure-connection.test.ts` and the new settings-structure test in `settings-console-pages.test.ts`.
- Web:
  - `tsc --noEmit` → ok. oxlint hooks → ok. `lint:strings` → 0 (baseline 0); raw-error and glossary lints clean.
  - Catalogue and sign-in related tests (20 files plus `claim-connect` and `sign-in-gates`) → 100 pass.
- Native:
  - `tsc --noEmit` → ok. `lint:hooks` → ok. `lint:strings` → 0 (baseline 5).
  - `test/owner-structure.test.ts` → 11 pass.
- Vega: `tsc --noEmit` → ok.
- Shared packages mirrored with `--write`; `--check` reports them identical at `bf46e785`.
- Not run: the e2e Hosted driver against the local Hosted fixtures, and any browser or device run (tonight's limited-testing rule).

## 8 Device test notes

- **Web first run** (local Hosted fixture, new server, Chrome/Safari/Firefox):
  1. Choose "Use a Portico Account" and create the recovery owner.
  2. Check that a Portico Account window opens at once (not blocked).
  3. Sign in there and approve. The setup page should sign you in by itself.
  4. Settings › Remote access › Secure connection shows "Starting", then "On" with the secure address. The certificate details fill in.
  5. Repeat, closing the setup tab before approving. After approving, sign in with the recovery owner: the server should already be connected and getting its certificate.
  6. Repeat with pop-ups blocked: "Open Portico Account" must work.
  7. "Do this later" signs you in, and the section says how to turn it on.
- **Phone/tablet** (signed in to a Portico Account in the app, owner of an unconnected server): Settings › Remote access › "Turn on with <account>". The server connects without any browser, and the status moves to Starting and then On. Signed in to the server only (no account in the app): "Turn on in a browser" opens the server's Remote access page.
- **Plain HTTP from outside the home networks:** the sign-in note shows the one sentence with no link (web, phone, TV).
- **Switch off "Use a secure connection":** the status says Off and remote TLS stops. Switching it on again requests a certificate.

## 9 Not finished

- **No first-run setup on phones, tablets or TVs.** Native still refuses a server that needs setup. First run happens in the server's web UI, which is where servers are created today.
- **The pending-approval watch is not resumed after a server restart** (see Decisions).
- **No end-to-end run against the Hosted fixtures yet**, and no browser or device run. The phone one-tap path relies on Hosted's existing `/v1/server-claims/approve` account route; nothing in this lane exercised it.
- **Apple and Android builds** were type-checked only.

## Commits

- portico-server (`audit-implementation/cp3-securedefault`):
  - `679a1403` The server waits for the Portico Account approval itself
  - `d60a4655` Secure connection by default: setup and Settings
  - `10979c9b` Approval page copy
  - `9a381c32` Setup skips the account window on a build without the service
  - `bf46e785` merge of main
- portico-react-native (`audit-implementation/cp3-securedefault`):
  - `419d436` mirror
  - `5d9c312` phone/tablet secure connection
  - `154884b` merge of main and re-mirror
- portico-internal (`audit-implementation/cp3-securedefault`):
  - `108bc3c` Hosted Services doc
  - `b5847dc` merge of main
- Nothing pushed or deployed. The server and native worktrees have `node_modules` hard-linked from `main-server` and `main-native` for type-checks; they are git-ignored.
