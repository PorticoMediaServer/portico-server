# Contract reconciliation decisions

5 September 2026. Original product choices are presumed intentional, per Justin. Implementation details may be replaced while preserving them. References are frozen in the original baseline; current governing details are in the linked family contracts, Foundation and architecture-amendments.md.

## Preserved product decisions

| ID | Decision retained | Source / later clarification |
|---|---|---|
| P01 | Eight active profiles per account, including primary; chooser/PIN/primary-profile semantics retained | Original Identity §2; user instruction in this reconciliation |
| P02 | Portico Account versus Direct Sign-In; independent Local recovery owner; account sign-in succeeds with zero servers | Original Identity/Onboarding; journey 08 terminology |
| P03 | Original username, password, contact-email, Google/Apple, MFA/recovery and privacy choices remain required | Original Identity; incomplete password-only integration is not a product change |
| P04 | Ten-minute XXXX-XXXX activation, same visible QR/code, separate device proof, manual approval from any network | Original Onboarding; alphabet spelling and 31-symbol count preserved |
| P05 | Seven-day signed policy, unchanged daily renewal, thirty-minute heartbeat, forty-five-minute presence, 500ms discovery hedge | Original Networking and accepted journey 08 |
| P06 | Thirty-day offline receipt, scoped trusted profile proof, expiry blocks without deleting, learned revocation/sign-out deletes, inactive profile remains protected | Original Identity and Saved/Downloads; independent validity conditions |
| P07 | Unlimited simultaneous playback by default; optional owner per-account/total caps; independent devices; explicit handoff; no predictive hardware veto | Later user decisions, exclusively implemented through document 32 |
| P08 | Five media families including local audiobooks; remote audiobook enrichment deferred | User clarification; original local-evidence model retained |
| P09 | Server-side provider calls with distributed project keys, owner provider policy/disclosure and attribution; no Hosted broker | Original Metadata and accepted rewrite |
| P10 | TMDB primary/TVDB selectable-fallback, AniList anime default, MusicBrainz typed music identity, local evidence default prefer | Original Metadata; no unrequested provider removal |
| P11 | File List Only/Basic/Complete/Custom, exact I/O promises, optional owner-supplied rclone and read-only compatibility mount, native WebDAV, private STRM resolution | Original Scanning |
| P12 | Manual edits lock by default; explicit Identify; undo by new revision; matching threshold/margin retained | Original Metadata |
| P13 | Named libraries, server-defined rows/order/filter/sort, correct media hierarchies, stable shell and native focus semantics | Original Presentation/Navigation plus user screenshots and journey 08/10 |
| P14 | Current Manrope/blue atmosphere and canonical brand assets; top-right web account menu; accessible adaptive layout | Original visual intent plus reviewed new build direction; no unsolicited visual reset |
| P15 | Distinct personal fields, duplicate playlist entries, unordered collections, explicit offline conflicts, Source download remains lossless | Original Saved/Downloads |
| P16 | DVR priorities, countdown before reservation preemption, Keep and partial-recording semantics; Library Channels separate | Original Live/DVR and old channel behavior as reference |
| P17 | Watch Together explicit invitations, host control, Start anyway, no automatic pause for one buffering viewer, 0.75/3s correction, 2/10min host loss | Original Sharing/Social |
| P18 | Native personal settings and consumer actions; broad administration/secret/source/restore operations web-first | Original Settings §14; resolves generic native-management references |
| P19 | Settings versus Console; desired/effective state; explicit high-risk confirmation; manual updates by default; original retention defaults | Original Settings/Operations |
| P20 | Local server core authority, privacy-minimized self-managed Hosted/PostgreSQL, public DNS separate, Cloudflare in front of Hosted | User direction and accepted deployment model |
| P21 | Native players and React Native where viable; initial Web/iOS/tvOS; later device families retained; default server port 32500 and OS services | User decisions, journey 08, document 32 |
| P22 | Free product; future paid relay separate infrastructure; no current billing/relay transport | User scope, CAP-066 |

## Reconciled implementation choices

| ID | Previous conflict or missing boundary | Current resolution | Existing-code relevance |
|---|---|---|---|
| R01 | Profile implementation twenty versus product eight | Eight creation cap; no silent truncation/deletion of inconsistent data; defensive parsing distinct | Existing Hosted/profile clients/tests need alignment |
| R02 | Authorization revision embedded in durable viewer identity; some new tables use raw profile ID | Stable authority/account/profile/server owner, separate permission/version fences; immutable local surrogate allowed | Existing personal/progress ownership needs alignment; coordinate with current playback schema owner |
| R03 | Several incompatible resource priority lists | One Foundation order; execution scheduling separate from document 32 admission policy | Playback campaign already owns admission; no second scheduler prescription |
| R04 | Inventory/probe success conflated; root path treated as identity | Separate source/object/revision/logical link, early inventory publication, exact coverage generation, scoped absence | Existing scanner/catalog foundation needs evolution |
| R05 | Provider HTTP result can publish against changed item; scalar lock triggers mistaken for whole-domain protection | Provider/source/identity/policy/base revision fences, staged publication, durable evidence | Existing provider writer requires alignment; preserve current manual editor fixes |
| R06 | Interactive artwork fetch/cache versus retained local selection | Acquire/validate/select before publication; image reads use local assets and deduplicated background repair; selected originals not pruned | Existing TMDB image path requires alignment |
| R07 | Old first-playing handoff conflicts with reviewed playback | Document 32 readiness-based commit; paused/unpresented candidates; timeout is ambiguous | Already part of active playback campaign; no new work instruction needed |
| R08 | Receipt revision mismatch makes renewal out-of-scope | Authenticate stable owner, re-evaluate current authority, issue immutable replacement | Future offline implementation |
| R09 | Offline “monotonic progress” loses deliberate backward seeks | Monotonic event sequence, explicit mutation conflict/base evidence; document 32 online arbitration | Future offline integration; online work already governed by 32 |
| R10 | Guide occurrence identity changes every import/time correction | Stable occurrence lineage plus mutable guide revision/interval; uncertain mapping stays unresolved | Future Live/DVR |
| R11 | Restore increments an epoch recovered from old backup | Durable external supervisor journal and fresh security incarnation through success/rollback | Future operations; no runtime restore change made |
| R12 | “Atomic” DB/file/provider work with no failure protocol | Private staging, verified install, fenced DB reference, orphan/recovery journal | Cross-domain future architecture; existing artwork identified separately |
| R13 | Scope of JSON schemas/generators from old runtime ambiguous | Preserve references; new implementation owns one reviewed public schema; port outcome vectors | No legacy schema import or generator execution |
| R14 | Private search state allowed and prohibited in different sections | In-memory scoped search only; no ordinary route URL/history payload/persisted restoration | Existing shared search appears directionally compatible; no new code correction claimed |
| R15 | Generic Hosted push versus server-owned inbox | Server inbox; optional opaque account/invitation wake only; no room/media payload | Future notifications |
| R16 | Metadata terms claim interpreted as a fixed automatic purge timer | Current provider-policy record governs; historical legal assertion is not verified current law | No key policy reversal or new setup friction |
| R17 | Rating 0–10 versus five half-star UI | Lossless semantic mapping; preserve choices/unset | No UI rewrite required solely for representation |
| R18 | Retry must reuse job ID versus current linked-new scan retry | Automatic attempt stays under same intent; explicit terminal Retry may be linked new operation | Current scan-retry UX can remain |
| R19 | Library Channels and DLNA lacked full domain coverage | Dedicated channel contract; authenticated capability-limited DLNA boundary; no generic LAN-public media | Future capabilities; retained scope, no silent deferral |
| R20 | Expired dedup receipt may turn a replay into a new mutation | Published replay window plus compact durable retirement/epoch rule; no blind recreation | Cross-domain completion requirement; no exploit reproduced in this review |
| R21 | Whole-reference merge in one huge transaction | Bounded staged reference closure plus atomic canonical alias/projection publication | Future metadata merge |
| R22 | Generic `ViewerScope` demands profile for account settings | Account identity/family works without selected profile/server; profile added when the action requires it | Existing account-session design retained |

These are architectural decisions, not a declaration that every old contract statement is now implemented. The full scope ledger still applies. No additional product opinion from Justin is required for this reconciliation; genuinely new future product scope remains his decision.
