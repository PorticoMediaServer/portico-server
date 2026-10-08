# Cross-domain conformance scenarios

Revision: portico.reconciled.1. These are required implementation outcomes, not executed tests. Preserve and port the original domain outcome fixtures as well. Every scenario needs an exact schema/build identity, independent code review, a behavioral result and applicable client evidence before being marked qualified. This contract reconciliation executes documentation validation only.

| ID | Boundary / hostile scenario | Required outcome |
|---|---|---|
| CF01 | Hosted and Local accounts reuse identical account/profile strings | Private data, receipts, events and quotas remain isolated by stable authority-qualified owner |
| CF02 | Valid policy renews unchanged; later a relevant permission narrows | Renewal preserves durable owner/history; narrowing fences stale work and rechecks access without inventing a new profile |
| CF03 | Two concurrent creates when seven profiles exist | Exactly one eighth active profile; no ninth; clear account-level limit; primary counts |
| CF04 | Account-session revoke races refresh and delayed client logout | One consistent family outcome; revoked token cannot recover; a newly selected account remains signed in |
| CF05 | Claim/redeem commits but response or local credential write is lost | Same claim/family recovered or quarantined; no duplicate server/account; no false setup reset |
| CF06 | Code rotates while approval is in flight, or local/Hosted codes coincide | QR/code remain coherent; exact authority and approved session decide result; no cross-authority lookup |
| CF07 | TV has zero eligible servers or phone lacks LAN permission | Account activation/manual code approval still works; no fabricated server context |
| CF08 | An old signed-policy renewal arrives after local restriction | Restriction stays effective until matching authorized acknowledgment; timestamp does not restore broader access |
| CF09 | A candidate echoes the correct server ID/fingerprint without controlling its key | Candidate cannot obtain authenticated public-route trust from static strings alone |
| CF10 | Signed route expires while remembered address still works | Reprobe/authenticate using valid current authority; expired route statement is not renewed implicitly |
| CF11 | General internet flag is false but local server is reachable | Direct server work is attempted under valid local policy; Hosted unavailability does not clear credentials |
| CF12 | Hosted cold restart with overdue fleet heartbeats and revocations | Bounded indexed queues, jitter and retained desired acknowledgments; no full-fleet synchronous repair storm |
| CF13 | Same-revision signed policy has different content | Reject collision; retain current safe state; no last-write-wins |
| CF14 | File List Only includes media, NFO, artwork, subtitles and STRM | Inventory succeeds with zero object-content reads; deferred facts are honestly unavailable |
| CF15 | First file probe hangs/fails, later healthy files exist | Inventory becomes visible independently; per-source/analysis bounds prevent healthy work starvation |
| CF16 | Network mount disappears, exposing an accessible empty root | Root path accessibility cannot authorize absence/deletion; last catalog retained with source failure |
| CF17 | One subdirectory/page fails, then terminal loop sees no pending rows | Incomplete coverage cannot publish authoritative absence; failure/partial outcome retained |
| CF18 | File renamed, replaced at same path, or seen through two libraries | Strong continuity preserves identity; replacement invalidates bytes/facts; one library's scan cannot invalidate another's shared source globally |
| CF19 | Source config/binary changes during probe or mount launch | Old work cannot publish/claim new config; explicit executable revalidation and owned-process cleanup |
| CF20 | Worker loses lease, newer attempt starts, then old worker succeeds | Publication/release requires exact token/generation; old result discarded safely |
| CF21 | Cancel commits just before artifact rename/DB reference commit | No late ready/success; cleanup remains observable; terminal intent cannot be revived by automatic retry |
| CF22 | Crash between verified artifact installation and reference commit | Installed orphan quarantined/reconciled; no unverified reference or silent data loss |
| CF23 | Provider result returns after Identify/source change/manual edit | Stale result cannot publish; locks, accepted identity, provenance and current projection retained |
| CF24 | Edit display title then refresh provider metadata | Matching still uses source/accepted identity; user display override cannot cause wrong-title rematch |
| CF25 | Automatic result has duplicate weak signals or contradictory entity kind | No false high-confidence acceptance; candidate remains unresolved/reviewable |
| CF26 | Provider unavailable while detail/cards request uncached artwork | Image response uses local retained asset/fallback; no interactive provider fetch; bounded repair independent |
| CF27 | Selected-art cache pressure or failed replacement staging | Still-selected original retained; prior valid selection preserved; derivative pruning cannot delete it |
| CF28 | Hierarchy merge with >500 descendants and an unknown reference owner | No truncation/partial visible identity switch; unknown references block; restart resumes bounded staging |
| CF29 | Catalog/projection changes between pages or permissions narrow | Cursor rejects/restarts safely; no private leak, silent duplicate continuation or client whole-library sorting |
| CF30 | Search/detail return, refresh, large text or locale change | Stable shell/context and accessible focus; scoped in-memory query retained appropriately, no private URL/history serialization |
| CF31 | Same profile independently plays on two devices | Separate occurrences/queues/history; canonical resume writer follows document 32; no implicit stop/handoff |
| CF32 | Explicit watched/unwatched competes with old playback/offline event | Old evidence cannot undo newer explicit intent; conflicting explicit branches need resolution |
| CF33 | Offline backward seek followed by later upload | Event sequence preserves actual final intent; max-position merge cannot erase deliberate rewind |
| CF34 | Offline explicit mutation returns after base history/receipt expiry | Needs-review/expired outcome, not blind overwrite or a newly created duplicate operation |
| CF35 | Playlist contains repeated media and a shared inaccessible entry | Occurrences remain distinct; reorder uses exact revision/IDs; private metadata stays hidden |
| CF36 | Download transfer resumes after grant or artifact changes | Renew only exact artifact under current owner; incompatible partial bytes not spliced |
| CF37 | OS background task completes after account sign-out/profile switch | Signed-out work cannot publish; inactive-profile coherent bytes remain protected/invisible |
| CF38 | Receipt issued under old authorization revision is revalidated by same owner | Current rights evaluated; valid replacement allowed; revision difference alone not out-of-scope |
| CF39 | Receipt hits exact thirty-day boundary while server unavailable | Playback blocked; bytes preserved; no grace or indefinite extension |
| CF40 | Correct download belongs to inactive profile; wrong profile attempts renewal | No private receipt facts; preserve correct stored download; retry only under its actual owner |
| CF41 | Local time moves backward or file identity changes after verification | Do not extend authorization or trust modified bytes; bounded revalidation/integrity recovery |
| CF42 | Guide update moves programme start and duplicates provider rows | Stable occurrence gets revised, not duplicate recording; ambiguous mapping stays unresolved |
| CF43 | DVR reservations conflict across DST/overnight padding | UTC intervals and deterministic priorities agree after restart; visible countdown precedes contracted preemption |
| CF44 | Unknown tuner estimate versus actual adapter refusal | Report uncertainty; no estimate-based ordinary playback cap; actual exclusive allocation/refusal handled without killing unrelated stream |
| CF45 | Capture exits zero after missing most of intended programme | Valid partial is incomplete, not completed; expected/observed interval and gap evidence retained |
| CF46 | Crash/cancel/retention races recording publication or active playback | Exact lease/reference protection; kept/in-use/uncertain artifacts never silently deleted |
| CF47 | Channel rule has no candidates, bad durations or huge result set | Bounded deterministic generation; honest slate/gap; no infinite loop or silent first-N scope loss |
| CF48 | Channel regenerate/extend overlaps current programme and config edit | Active occurrence stable; old generation fenced; atomic guide publication and preserved tail evidence |
| CF49 | Channel next programme becomes unauthorized or media format changes | No title leak; server resolves safe unavailable interval/transition; document 32 delivery semantics |
| CF50 | Handoff commit response is lost and target later reports playing | Reconcile exact readiness-based commit; no second first-playing trigger or dual writer; source not guessed active |
| CF51 | Controller disconnects or browses another server | Receiver continues; retained controller has only contracted narrow rights; Stop/sign-out revocation remains explicit |
| CF52 | Host disconnects, server restarts, member buffers, another command arrives | Durable 2/10-minute host deadlines; no reset or unauthorized command replay; one member buffering does not pause everyone |
| CF53 | Participant cannot access a future queue item | Opaque placeholder only; host must resolve before load; no private group/media Hosted payload |
| CF54 | Settings save times out after commit, later settings edit occurs | Original receipt proves old commit, fresh snapshot proves current state; no replay overwrites newer settings |
| CF55 | Candidate secret/source config fails after test or old callback returns | Active generation remains; stale candidate cannot activate; write-only secret never echoed |
| CF56 | Support snapshot/export loses authority or redaction fails | No private export; bounded safe failure; no unredacted fallthrough or automatic upload |
| CF57 | Backup contains encrypted references but recovery key missing | restoreReady false with exact prerequisite; no false usable-backup claim |
| CF58 | Restore old DB, then rollback during health failure | Fresh external security incarnation survives both paths; restored tokens/receipts invalid; Hosted projections quarantined |
| CF59 | Restore status token is lost or inherited from old DB | No duplicate restore; external supervisor/Local recovery finds exact operation; old status token cannot authorize it |
| CF60 | Old offline client remains disconnected during restore | No claim of instantaneous remote wipe; learned revocation applies on reconnect under retained thirty-day model |
| CF61 | Install/update/uninstall encounters active media or incompatible DB | Honest deferred/manual/recovery state; no implicit DB downgrade or source-media deletion |
| CF62 | Third-party/DLNA client cannot supply needed auth or safe child grants | Capability limitation; no anonymous library or unscoped LAN media exception |
| CF63 | Revoked viewer replays previously successful mutation receipt | Reauthorize before receipt disclosure; historical success cannot recreate access or leak protected result |
| CF64 | A low-priority blocked I/O job competes with revocation and healthy source | Separate bounded control path remains available; resource prediction does not veto new authorized playback |

## Architectural walkthrough results

The reconciliation explicitly walked these combined journeys at the design level: Hosted outage plus profile switch plus offline receipt; interrupted scan plus mount replacement plus existing playback; manual edit plus delayed provider/asset publication; duplicate playlist entries plus permission-narrowed continuation; guide correction plus recording crash/retention; receiver handoff plus account/server switch; and restore rollback plus old credentials/offline state. Result: the new ownership/fencing/publication rules give a defined authoritative outcome at each boundary. These walkthroughs are architectural reasoning, not runtime passes.

The design pass corrected four draft ambiguities: local code uniqueness cannot span disconnected authorities; a policy revision is not a durable owner; offline monotonicity applies to event sequence rather than increasing position; and restoring a DB epoch cannot supply an anti-rollback security boundary. The current corpus also resolves the existing first-playing/readiness handoff conflict in favor of document 32.
