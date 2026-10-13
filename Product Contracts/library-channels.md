# Library Channels

Contract ID: PC-LIBRARY-CHANNELS. Revision: portico.october.1. Covers CAP-047, with presentation/playback/metadata/operations imports. This fills the original corpus's missing domain contract using existing product behavior as reference; no original code is imported into the build.

## Product and ownership

Library Channels are server-authored linear schedules built from existing catalog media. They remain a separate tab/source type from tuner-backed Live TV. They use the same authorized player/delivery pipeline, but consume no tuner and create no DVR capture merely because something is “on air.” The server defines channel order, rules, guide entries, current programme, next programme and tune offset. Clients render/tune; they do not shuffle or calculate schedules independently.

Server owner administration supports create/edit/enable/disable/delete/reorder, defaults, rule selection, schedule blocks, preview/regenerate, built-in templates, artwork/logo and supported on-screen logo policy. Viewer controls support directory/guide, current/next programme, tune and appropriate player operations. Private media never appears to an unauthorized viewer through a channel name, guide title or next programme.

## Stable configuration and selection

Channel ID is stable. Configuration revision owns name/description, enabled/order, timezone, default rule, requested quality, seed, template provenance, logo and rule/block definitions. Rules have stable IDs, explicit server query, order, selection/episode/exhaustion mode, deduplication window and maximum consecutive limits. Query validation uses current catalog fields and permissions; invalid saved predicates require review instead of silently broadening membership.

Retain sequential, shuffle-bag and weighted-random selection; episode modes none, in-order, marathon and randomized; exhaustion modes loop and slate. Publish supported values and their meaning through one server semantic model. Weights/limits must be positive/finite where required; empty or impossible rule sets yield truthful schedule gaps/slate, not an infinite generator loop. Duplicate source rows cannot skew selection weights accidentally.

Blocks retain weekday mask, local start/end, priority, fallback rule, anchoring and overrun choice. Interpret wall time in the channel's IANA timezone, convert concrete occurrences to UTC and keep that result with the schedule. Test DST skipped/repeated times and overnight blocks; define one deterministic policy in the schedule response rather than letting each client choose. Use earliest valid instant for a skipped start and the first occurrence of a repeated boundary, preserving nonnegative intervals; the preview displays the resulting actual interval. An owner can change configuration before publication if that result is unwanted.

## Schedule generations

Generation is a durable operation, not a synchronous UI request. Pin config revision, catalog candidate generation, stable selected versions/durations, deterministic seed and current rule cursors. Candidate queries page through all relevant rows or an explicitly configured, displayed semantic selection limit; no hidden first-N truncation. Persist enough evidence to resume deterministic selection after crash without loading all candidates into RAM.

Generate a rolling seven-day guide by default, with bounded per-batch work and explicit generated-through time. Seven calendar days in the chosen timezone need not equal 168 hours across DST. Each entry has stable occurrence identity, UTC interval, media/version or slate identity, source rule/block and generation. The same inputs produce the same output order and timing. Unknown/unreliable duration excludes an item from a fixed timeline until analysis or an explicit supported fallback resolves it.

Stage a new generation, validate non-overlap/coverage/durations and atomically publish its active pointer. A failed/cancelled generation keeps the last valid guide. Tail extension preserves previously committed/current entries and rule cursors. Regenerate explicitly previews the replacement boundary; it cannot silently rewrite a currently playing occurrence or its accepted timeline. Configuration change fences an older unfinished generation.

## Tune and continuity

The server resolves a tune to exact schedule generation/entry and current programme offset. It authorizes the requesting viewer against the current channel and underlying library/item, then opens an ordinary document 32 playback occurrence. Source/quality/track policy and owner caps are enforced through that pipeline. The client never receives an unprotected file URL or a schedule-wide media credential.

A schedule transition or source revision change uses the same planned delivery/discontinuity mechanisms as playback; no raw concatenation of incompatible codecs/timestamps. Current programme completion advances to the server-resolved next authorized entry. Missing/revoked media becomes a safe unavailable/slate interval and triggers bounded repair; it does not reveal the hidden title or silently choose unrelated content. Live channel position follows the published linear timeline; pause/seek/restart-from-beginning appear only where the server publishes those capabilities. Buffered old entries are fenced normally on revocation.

Disable prevents new tunes and future generation while preserving current accepted playback unless the owner explicitly stops it. Delete previews active use and retained references, disables admission, fences schedule jobs and removes configuration only after safe reference cleanup; it never deletes the source media.

## Templates, logos and observability

Preserve existing template intent: broad movie/television, decade/genre/family selections, anime, recent additions and marathon modes. Template installation is explicit and idempotent; inapplicable/empty templates are omitted or explained, not installed as enabled dead channels. Updates do not overwrite owner-customized rules. Template candidate limits and minimums are visible semantic choices, distinct from pagination safety limits.

Channel logos use the metadata artwork storage/security path. An on-screen channel logo is a distinct optional effect, with owner choice, corner/size/inset/treatment and disclosed conversion overhead where server rendering is needed. It cannot silently enable transcoding against owner policy or leak across unrelated playback. Do not claim a logo overlay capability from a static guide logo alone.

Owner state distinguishes disabled, generating, healthy, stale guide, no eligible media, unresolved duration, invalid configuration and source failure. No fabricated progress or success from a generated-through string alone. Guide/tune is consumer-safe; query internals, source locations and diagnostics stay owner-only.

## Required evidence

Deterministic repeat generation; paged large candidate set; interrupted tail extension; concurrent config edit; empty/zero-duration rule; DST/overnight/overrun; template idempotency and customization preservation; tune exactly at an entry boundary; permission change on next programme; source replacement mid-programme; incompatible media transitions; regeneration during playback; disable/delete with active use; logo/transcoding policy; server-restart continuity; web/iOS/tvOS guide/tune and honest capability states.

## October scope and implementation evidence

Library Channels remain server-composed schedules distinct from shared tuner inputs and profile-private DVR. Original quality is the default; any viewer Automatic choice follows the current playback contract. One device capability document and ordinary session authority apply; historic document 32 references do not define a second API.

The configuration/schedule/outcome descriptions above are requirements, not a claim that every editor, logo policy, seven-day generation or lifecycle fixture has been qualified. Sources: [Live TV](../../_program/reports/livetv.md), [planner wiring](../../_program/reports/planner-wiring.md), [native sync](../../_program/reports/native-sync.md). Real channel transition, codec, input/focus and all-client acceptance remain unverified. No mail/push, plugin/theme or Roku release work is introduced.
