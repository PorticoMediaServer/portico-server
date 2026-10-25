# SECURITY — PAUSED handoff

As of **2026-10-25 14:28 UTC**, mini-1. Justin paused the campaign at about14:20. **Start no work, merge nothing and run no tests/builds/generation/devices until Justin resumes.** This file, `SECURITY-PENDING.md` and the complete `SECURITY-REVIEW-LEDGER.md` are the replacement for this thread. All decisions below are source-only unless the dated historical ledger explicitly identifies older evidence.

## Owned work and preservation

Security owns identity/Hosted/security source review and the sole restriction-matrix writer. No implementation, runner, emulator, server, browser or scratch process is running. Last headroom51GiB free (89% used),25GiB memory available; no cleanup of histories/databases. Main source fixes already merged: matrix **c85d1ec90b1d92e2e6fdf4472bc446dbd26d8c8b**, malformed receipt mapping **7e448f5e6face3a18e5e77b72e1e284b496c9431**, strict item-ID mark fix **c74f0d71f**. Frozen M0-005 fc93/40188273221 remains serverFAIL/apikitPASS; none of these source reviews establishes a rerun/green.

Review documentation was outside any Git repository at `_program/v2`. A local preservation commit is being made in **`.worktrees/cert-binding-server`**, branch **`checkpoint/security-m005-reading-matrix-20261025`**, starting at2cd99605ca8144f2254bca16ab1f5c53fa3ed42c. It stores this handoff, pending index, full ledger and an exact patch of the unrelated already-staged seven-line reverse change in `reading_mark.go`. **DO NOT MERGE THAT REVERSE CHANGE**: it removes the approved hostile-ID fix. It was excluded from matrix publication and is preserved as evidence, not authorized work. Original index/worktree left untouched. No main/root checkout edits or force/reset/stash.

## Reviews owed at pause

Full pins, branches, concerns and older unresolved candidates are in **[SECURITY-PENDING.md](SECURITY-PENDING.md)**. Age is measured at pause; where an original UTC was not retained, it is explicitly unknown rather than inferred from a Git author date or reset by a repin.

| Author | Current review | Status / waiting |
| --- | --- | --- |
| dsgo-sonic | **9c4b8437739505782bc61e57fe869ac2083a30ab**, prior03e97606 | **Functional HOLD**: exact rendered/committed target+context/activity+signal through actual sync/storage IO still owed. Contracts model9c approved independently. Original10:54:57Z: about3h33. False containment0e99234c remains approved. Historical97aed request is superseded. |
| dsgo-remote-b | **57b28dbc5e03** (supersedes b531/f3/647) | Required local/context-aware/Recheck contract and whole-page set fixes read. Remaining final Recheck uses parentctx instead of90s child; actual early remote-opener refusal/format integration/current-main composition not yet authored/reviewed. Latest amendments minutes old; original request UTC unknown. |
| ds-native-live-r2 | **21c57f6a3546090139daae15c0322edd136bf23b** OfflineCore | Full native handle/Reader/inventory/bridge/lifetime review pending. Nonce377 closure narrow only; no downloaded annotation composition. Original request25Oct, UTC unknown. |
| ds-native-live-r2 | **ed9e3170a2a6c2ff911f553ad36149a8b8c24387** LIVE01 | New iOS synchronized finish correction unread at pause; last73e hold exact detach/terminal/identity synchronization. Original request25Oct, latest just before pause. |
| dsgo-remote-a | **a490b5824 + row1927fa5e828536a5d1dab8c37994995353190e9e** | Move-progress/codes host cumulative current source reread owed; earlier Strict replay/exact-source/exact-receipt opening holds not waived. Root32 server emissionca8a6b716 + eventual official decoded lookup wiring also pending. Original UTC unknown. |
| dsgo-cast-a | **b3163487**, latestcf6994016dccc8761ed5c5e2bc8fe63d842b2dd9 | Base cumulative authority/readiness review pending. Focus96/0d no authority objection only; last one-line countcf not reviewed. Original UTC unknown. |
| dsgo-video-web | **1d552849e181a4fd88c51660299bdb52107c2a33** | Media source approval does not close Security authority/privacy/grant-reader/cancellation review. Original UTC unknown. |
| dsgo-video-tv | **c21f4b00 / cbd7696c / fccdc702**, latest fixes not pinned here | Phone Reader committed route/current rendered manifest/version/cursor/flight holds; no destination blanket approval. Original UTC unknown. |
| dsgo-hardware-a | **ea565845a**, **4c7023fb3** | Devices/revoke ticket lifetime correction and typed AwayQuality wire/storage reread pending separately. No runtime/enforcement approval. Original UTC unknown. |
| native04 / accounts02 | ff371 Devices,8d8f Notifications,f39 Downloads,Phone34 and setup-loading leaf | Postmerge reviews/precise scope in pending index. Devices original09:12:31Z (~5h15), Notifications09:15:05Z (~5h13), f39 original10:53:31Z (~3h35), Phone34 original08:50:38Z (~5h37); silence merges were **not** Security approval. |

## Step finished while pausing

**LocalImageFolders cumulative120406b2d7ad8c172dc83455cbb0ce62821f2214 is Security SOURCE APPROVED** after final two-line correction overda909/91/c50. Saved primary/shared captured comic formats now include image-folder before selection; Mobi configuration/fences unchanged. Hostile-ID and closed cover query fixes closed. Full physical/root/member/generation/privacy/Close custody reviewed. No remaining finding in that exact source slice; official Release16 schema/enum/route outputs and runtime/feature acceptance separate. **No merge during pause.** Verdict preserved in ledger; not sent as an instruction to wake a paused lane.

Other completed recent scopes: OS invitation7b approved, reported main**d332eac889cce0fef6c15383ab8b93319c8677b7**; secure-field30cf privacy-only approved (no credential/run permission); Root32 prepared displaycd4 approved/reported615a4e91; Native04d361 presentation/fixture approved/reported8e9981fa. EffectiveCredits293af cumulative approved/reported88fd5ec6; old5445 request is historical, not reopened.

## Hosted / identity / private state

- No Hosted/deployment/identity mutation by this lane in these source-review turns. Root31/32 rulings must be reread before resuming; decisions summarized in pending index. c942 server verifier +c65 Hosted signing +67be lookup schema are scoped-approved, **not full Start Over emission/client approval**. Official generation is Release-only.
- R2/R3 setup continuations remain expired/terminal/preserved; no owner, retry, renewal, DB repair or new setup target is authorized. Last retained live diagnostics and ordinary Hosted acceptance are dated evidence, not a current live pass.
- Release reported normal user-authorized DEMO password rotation only after all collectors closed: new sign-in/owner profile verified, old password and calling bearer401, own new device forgotten. Private atomic0600 active-credentials record updated; **never read/copy secrets**. APIkeys/PAT revocation not claimed. TV auth remains INCONCLUSIVE from stale-ref operator error; history preserved.
- Older ACME recovery99263c43 was applied once by Release23:07:04Z24Oct with protected displaced backup. No new ACME/provider operation authorized; see ledger/runbook. No active collectors/locks owned here.

## Successor first

Wait for Justin's resume. Read this index and ledger, then prioritize Sonic actual lifetime containment/functional hold, OfflineCore native and LIVE01 exact latest corrections; finish CB7 child-deadline/opener boundary before any exposure. Consult each author's preserved branch, not a stale root/main or historical request. Preserve queued annotation records; **never enable false-gated authoring, silently rebase old intent, delete originals, or infer approvals from Media/Contracts/silence**. No duplicate authors, per-item jobs, hand mirrors or generated artifacts.
