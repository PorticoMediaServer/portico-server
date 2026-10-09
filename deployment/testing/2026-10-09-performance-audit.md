# Server performance and resilience audit — 9 October 2026

## Acceptance targets and current status

Portico must retain its existing features, permissions, recommendation ranking,
personalization, discovery and history semantics. Ordinary browsing and a few
direct-play viewers should work on the single-core, 1 GB demo VPS. The Raspberry Pi
example motivates eliminating unnecessary resource use; Justin later clarified
that literal Pi qualification is not an acceptance gate. A capable modern PC should
serve about 200 browsing users alongside at least 100 direct-play viewers. Normal
capacity should not depend on refusing a significant share of requests. Brief
bounded queues and client retries should absorb ordinary bursts, while overload
refusal remains a recoverable last resort.

Home and Discover have a soft server-processing target of 100–200 ms. Public HTTPS
latency includes authentication, JSON encoding, proxies and network transit; it
must be reported separately from catalogue-only microbenchmarks.

The initial Home timeout has been corrected and verified on the demo, as recorded
in [the Home investigation](2026-10-09-home-performance.md). The larger capacity
target **has not passed** in the recorded historical runs. The final corrections
below are committed in `82aaa54`, following `8f92fd3` and `b91366b`, with passing
normal and affected focused verification. An exact-commit canonical-fixture
capacity run is complete: 100 direct streams and the physical WAL gate passed,
while strict browsing latency/refusal gates still failed. The build is deployed;
final results are below and interactive checks are in the setup report. Major
resource-waste remediation is complete under the clarified scope. Isolated
improvements are not capacity acceptance evidence.

## Confirmed costs and corrections

| Area | Confirmed problem | Correction and preservation checks |
| --- | --- | --- |
| Home artwork | SQLite searched the ready-artwork cache for each requested title instead of seeking selected digests. | Pinned join order, deployed in `7b34e52d`; actual-driver planner regression and populated-cache tests preserve artwork selection and visibility. |
| Home decorations | Four metadata queries repeated for every finished rail; Recommended suppression and Trending fetched the same candidates independently. | Batch decorations across final previews and reuse immutable Trending candidates within one request. Field parity, ranking, reverse access order, hidden rows and fresh consent/personal-state tests pass. No authority cache or ranking cap introduced. |
| Recommendations | Every recommendation session hydrated all engaged/hidden title signals and all pending refresh jobs, even for a small candidate pool. | Candidate-key primary-key hydration at every engaged-state consumer; exact small job overlays retained, large job queues checked per candidate. Positive-seed partial index preserves old index. Eager-history oracle verifies ordering, fields, facets, dates and score equivalence. |
| Event feeds | Combined `MIN(seq),MAX(seq)` scanned the retained ring on each page/poll; visibility callbacks repeated for every event in the same library. | Independent indexed endpoint subqueries; per-page library visibility memo within the same read snapshot. Hidden/visible events, sparse/empty bounds, retention cursor and next-page permission refresh regressions pass. |
| Planner statistics | Combined rowid bounds scanned tables before bounded sampling. | Independent indexed endpoint probes, preserving empty/sparse tables, span calculation and sample behavior; actual-driver `EXPLAIN` asserts indexed searches. |
| Cancelled requests | Search-history/collection writes and playback grant/report checks detached from HTTP cancellation. | Propagate request/snapshot context through transactions, SQL and authority callbacks. Legacy non-HTTP wrappers remain. Contended-writer tests prove cancelled work does not later execute; cancellation never revokes a valid playback grant. |
| Image uploads | Avatar authorization occurred after full image decoding/rendering. Per-file compressed/pixel limits allowed many simultaneous large decoded buffers. | Authenticate management/profile access before HTTP body reads and before service decode; final publication still reauthorizes transactionally. Shared one-job decoder/render gate and four buffered-upload slots, bounded waiters and cancellation preserve formats/renditions/dimensions. |
| Slow upload bodies | A valid slow uploader could monopolize image capacity; clearing an expired read deadline let HTTP/1 response finalization drain a stalled body again. Gzip wrapper hid ResponseController capabilities. | Separate raw-body admission from decoder admission; attach body reads to existing request deadlines, close rejected/stalled HTTP/1 uploads, and unwrap compression for socket controls. Real-socket authenticated gzip regression times out promptly and subsequent legitimate upload succeeds. |
| Guide programme pages | Generation-only queries fetched every programme in a source, discarding unrequested channels afterward. | Match selected channel IDs as well as generation and time window. Separate channel-directory windows avoid fetching complete guide payloads simply to construct rails; capability-negotiated client fallback preserves older servers. |
| Admission/fairness | Unverified arbitrary credential strings could manufacture fairness identities; HTTP/2 multiplexing could create large waiting populations despite a socket cap. | Verified-secret accounting registry, peer fallback for unverified credentials, bounded global/per-client wait populations, memory-scaled transport limits and short refusal write budgets. Verified media grants inherit their issuer's accounting key rather than gaining one share per grant. These accounting entries never authorize a request. |
| Playback reporting | Repeated/stale timeline reports still renewed leases, and clients could send fresh sequence numbers continuously. | Existing stale/duplicate semantics retained while avoiding needless lease writes; verified-device token bucket permits normal reporting and seek/state bursts, with retryable 429 for sustained abusive cadence. Client report coalescing/retry work is tracked separately. |

## Isolated measurements

Measurements are diagnostic and have different scopes; they are not interchangeable.

| Experiment | Before | After | Scope |
| --- | --- | --- | --- |
| Home artwork correction | 1.60–1.66 s | 73.8–123.9 ms | Private demo database, actual Go SQLite driver, one Go CPU, catalogue work. |
| Home decoration batching and Trending reuse | 77.146 ms / 146 statements | 59.172 ms / 107 statements | Warm private demo database, one Go CPU, catalogue work; preserves returned cards/ranking. |
| Event-ring bounds | 2.129 ms | 25.23 µs | 20,000-event retained ring, actual modernc driver, 100 benchmark iterations on local i5; ~84× improvement for this statement. |
| Recommendation taste/job hydration | 297.17 ms / 17.57 MB | 0.53 ms / 25 KB | Isolated 100,000-signal plus 100,000-job fixture; candidate hydration only, not full Home or HTTP. |

Live `7b34e52d` VPS loopback HTTP Home samples were 273, 248, 248, 271 and
269 ms. Warm Discover libraries ranged approximately 17–177 ms; cold samples
included 206 and 315 ms. All 30 requests returned 200. These include auth, server
HTTP handling and JSON, exclude the Germany/public network path, and precede the
additional Home/recommendation patches. Evidence is in ignored local scratch
`apps/portico-server/.scratch/performance-audit/vps-loopback-7b34e52d.json`.

## Preservation and focused validation

- Recommendation regressions cover all media types, hidden and engaged history,
  franchise/book-series members, memo hits, pending jobs, stable paging, community
  contribution consent and current permissions. The eager oracle checks lazy
  hydration against the previous full-history representation. Schema migration
  tests cover fresh databases, upgrade from version 10, preserved history and
  immutable previous migrations.
- Event feed and planner tests use the actual modernc driver and production SQL;
  their query-plan regressions distinguish two indexed endpoint seeks from the
  old full scan.
- Cancelled collection/history writes are tested with the real writer gate held;
  releasing it later does not publish abandoned changes. Playback grant and
  progress cancellation tests preserve live sessions. Existing old-generation
  capability hiding remains HTTP 404; a cleanup-defer ordering regression was
  found and corrected before handoff.
- Avatar tests check 401/403 before any body read, all legitimate PNG renditions,
  cancelled work, real HTTP/1 slow-body deadline through gzip, and successful
  upload after cancellation. Shared-gate overflow, cancelled waiters, idempotent
  release and recovery pass. Artwork regression suite passed in 13.147 s;
  imagework plus focused HTTP avatar/session lifecycle race checks passed in
  41.221 s. Existing bcrypt admission/cool-off tests passed in 3.712 s.
- Linux effective-memory tests cover physical/cgroup minimum, 64 MB intermediate
  ancestor with an unlimited leaf, container namespace mappings, mount boundaries,
  malformed input and integer overflow.
- Admission/fairness/timeline focused HTTP race suite passed in 70.621 s;
  final avatar/unread-body refusal/gzip cleanup rerun passed in 34.2 s. Duplicate
  timeline lease races passed in 36.3 s. Actual HTTP/2 zero-window stall and
  recovery passed under race in 16.1 s. Ordinary API responses have a rolling
  15-second write budget, including compression completion; media/event streams
  retain their established progress budgets rather than a total playback timeout.

Fairness now uses a verified device binding rather than the rotating token hash.
The existing approved-device lookup returns its device ID as internal, JSON-hidden
accounting metadata; authentication and grant-family continuation inherit it with
no extra statements or schema changes. Token refresh, profile-family selection
and media-grant creation retain one share; distinct verified devices remain
independent. Live authority and revocation checks remain unchanged. Actual-driver
authentication still uses five statements including snapshot bookkeeping, and
integration tests cover refresh, a second profile family, grant continuation,
denied devices and unchanged reauthorization guards. Synthetic/legacy internal
principals without device metadata retain the token fallback. The bounded registry
is not proof against unrestricted legitimate creation of entirely new devices.
Typed timeline limits also use the verified device identity.

Complete combined server/client suites, current-binary capacity repetition and
post-deployment demo measurements remain separate release checks.

## Capacity test failure and active follow-up

The revised harness removes earlier hidden allowances for 45% refusals and 6–8 s
latency. A real-socket test on `ehler-personal` (i5-10500T, 6 cores/12 threads,
about 31 GB RAM, Windows VM unchanged) used 200 browsing viewers, 100 direct media
bodies, 20 event streams and a 4,000-item catalogue for a 180-second workload.
Source hash, host and parameters are recorded alongside the profile, so the
uncommitted source state is distinguishable from release `7b34e52d`.

The run sustained 100 server-side media bodies, each distinct reader consuming
more than 1 MiB, and recovered descriptors/goroutines/children without corruption.
However, **33,531 of 43,516 clean requests were refused (~77%)**; clean P95 was
1.50 s and P99 1.83 s. Recovery Home failed 38 of 43 attempts. This does not meet
the browsing or recovery acceptance target. WAL peaked at about 350 MiB, above
the harness's 256 MiB criterion, and the longest write-gate hold was 5.15 s.

That initial run used legacy playback controls and did not honor Retry-After
after normal-client 503 responses (only its ordinary 150–450 ms think time).
The harness now defaults to current typed v1 playback with device capabilities
and serialized timeline cadence. Ordinary requests retry once after Retry-After
plus jitter, while retaining raw refusal counts and logical end-to-end latency.
A small current-v1 socket smoke with eight viewers/four streams passed with zero
refusals, clean P95 171 ms and P99 246 ms. The corrected large workload was repeated below; the earlier failed run remains
useful bottleneck evidence rather than a final modern-client capacity acceptance
result.

CPU profiling attributes about 49.9% cumulative CPU to SQLite preparation/parsing
and 21.2% to execution. Home/detail visibility compositions are prominent. A
bounded per-connection prepared-statement reuse patch was initially measured with
64 retained handles per physical connection within a 512 KiB estimated bytecode
budget; final measured accounting and resource scaling are described below.
It leases a statement until its rows close, uses a transient handle for
overlapping identical queries, invalidates for configuration/schema commands and
retires failed/cancelled statements. It caches no rows or authorization decisions.
Tests cover changed bindings, early close, nested rows, constraint failures,
schema rollback and changes on another connection, cancellation/Next races,
snapshot write rejection, account changes, eviction and physical connection close.
Full dbwork and focused race checks pass; scope-CTE microbenchmark was approximately
39 µs uncached versus 5.3 µs cached. This microbenchmark is not a new capacity
result, and the bytecode budget is an estimate rather than a hard native-RSS limit.

The WAL baseline was only about 4 MiB, so the large peak arose during the measured
run. Two uncapped background catalogue writers continued ten-row transactions;
`dbwork.Yield` currently counts foreground pressure without delaying a batch.
Hard-pressure checkpointing performs TRUNCATE under the global writer gate while
old snapshots drain. These are plausible contributors to the long hold, but the
aggregate gate counter does not identify its holder. Per-class hold/checkpoint
timings are needed before attributing the 5.15 s hold to a specific operation.
The injected pool fault also acquires six handles sequentially before its 750 ms
hold begins; acquisition itself is not limited to that duration.

Evidence: ignored `apps/portico-server/.scratch/performance-audit-20261009/`
contains `soak-capacity-source.json`, `soak-capacity.log`,
`soak-capacity-cpu.pprof`, `soak-capacity-cpu-top.txt` and resource measurements.
The large chaos log includes deliberate contained panics; its presence is not a
crash or a successful capacity result.

## Second capacity failure and foreground/background scheduling

A fresh current-v1 run with the initial statement cache still failed the same
200-browse/100-body target. Of 24,385 raw clean requests, 13,110 were refused
(53.8%); clean P95 was 1.500 s and P99 1.816 s. Retry-aware logical clean calls
numbered 16,073, with 5,116 unresolved after retry and end-to-end P95 2.727 s.
Recovery Home refused 21 of 32 calls. All 100 actual bodies remained active and
all readers consumed more than 1 MiB. Integrity and eventual reclamation passed,
but WAL peaked at 547,034 KiB (about 534 MiB), with a 1.03-second maximum gate
hold. These remain failed acceptance results. A separate 1,122 playback-stop
400 responses came from a harness-only empty-body/JSON Content-Type mismatch;
the corrected harness must be used for the next run.

The first cache policy discarded every idle cached statement on ordinary SQL
that it chose not to retain. A real Home statement containing a semicolon inside
a string triggered this bypass, so repeated requests attained only about 18%
reuse. A lexical statement-boundary classifier now separates retention from
invalidation: ordinary oversized statements do not evict other VMs, quoted and
commented semicolons are recognized, and trailing comments/terminators can be
removed before preparing the single statement. Configuration/schema/temp/script
commands retain conservative invalidation. Subsequent combined failures are
recorded below; the final correction must still pass the acceptance workload.

No stream-lifetime snapshot leak was found in Home, direct media grants or SSE
page reads. Continuously overlapping short snapshots can also prevent a WAL
reset; the test does not identify its oldest reader. Physical file size and
uncheckpointed frame counts must be distinguished. Backup did hold its source
snapshot beyond the completed SQLite copy while syncing and checking the target;
that snapshot now ends immediately after `backup.Finish`. Backup was absent from
this workload, so this fix does not establish the cause of its WAL growth.

Checkpoint maintenance previously borrowed foreground pool capacity and inherited
the normal multi-second busy timeout while holding the writer gate for TRUNCATE.
It now borrows the background pool and temporarily uses a 50 ms SQLite busy
budget, restoring the connection's exact original timeout with an independent
cleanup context; failed restoration discards that connection. Actual modernc
pinned-reader tests show a blocked checkpoint returning within 500 ms, a gate
hold below 300 ms, a queued security writer progressing, and a later successful
TRUNCATE with all 9,001 rows and `quick_check=ok`. The SQLite busy wait ignores
context interruption; 50 ms limits that wait, rather than guaranteeing every
checkpoint copy finishes within a hard total wall-clock budget. Per-class gate
hold/wait counters now distinguish maintenance from playback and background work.

Continuous derived-data, lyrics-bulk and personal-state batch loops now call
shared `dbwork.PaceBackground` after the batch releases its database leases.
Without foreground pressure it returns immediately. With pressure it takes a
cancelable pause of three times the completed batch's elapsed time, bounded to
2–50 ms; each batch still advances, and `Yield` remains nonblocking. Existing
worker retry intervals did not guarantee a post-commit gap: elapsed batch time
and immediately pending commit notifications could consume those intervals.
The revised soak uses the same production pacing mechanism, rather than a test-
only sleep. An isolated actual-driver test completed all 24 background and 24
foreground transactions under continuous pressure in about 64 ms; this verifies
progress and released leases, not the full capacity bar. Production derived-data,
lyrics and durable personal-state scheduler completion tests pass with continuous
foreground pressure.

An independent client-path issue affected guide requests: a nested two-slot
mutation gate immediately rejected ordinary GET reads with 429. Guide and library
channel GETs now use the shared bounded HTTP/read admission, while mutations keep
two active slots and wait cancelably within the request deadline. Four actual
simultaneous live and library HTTP responses all complete with 200. A cancelled
third mutation never reads its body, unauthorized accounts are rejected before
waiting, and reads proceed while both mutation slots are held. Existing live
revocation and guide privacy tests still pass. Guide-only members and owners now
register their fairness identity only after the existing live checks succeed;
two independently verified devices behind one NAT learn separate accounting
shares, and a remembered revoked credential still fails live authentication.
The registry remains accounting-only.

A combined race run initially hit the personal-state completion test's existing
20-second budget. An isolated baseline without foreground pressure hit the same
180-of-205 progress boundary, while fixture construction alone took about 47 s
under the detector. The ordinary completion budget remains 20 s; detector builds
use a longer behavior-check budget and retain the same 205-item completion and
revision assertions. Both isolated checks then completed all 205 items: baseline 67.1 s and paced
41.7 s including fixture construction. Those timings reflect detector work,
not ordinary server throughput.

Evidence: `soak-v1-cache-capacity.log`, its source manifest and CPU profile in the
same ignored scratch directory. The next frozen binary must combine the cache
classification correction, checkpoint changes, production pacing and corrected
playback-stop harness before repeating the capacity acceptance run.

## Verification environment and subtitle rendering

The first full-suite run used an absolute TMPDIR containing a literal `/../`.
This triggered intended clean-path validation in memory hierarchy and media job
boundaries. The same directory normalized with `realpath` passes hostlimits;
production containment and ancestor protection were not relaxed.

Canonical-path testing then exposed a real sandbox subtitle issue on Ubuntu's
system FFmpeg/libass: `/etc/fonts/conf.d` rules point through symlinks into
`/usr/share/fontconfig/conf.avail`, while only `/etc/fonts` and the font faces
were mounted. Without those alias/scaling rules, Arial Bold selected a Droid
fallback lacking Latin glyphs and then a math face, drawing only 29 bright pixels
in the existing test frame. The font pack now makes the existing distribution
rule directory available read-only, without treating it as a font-face directory.
Sandbox and baseline both select LiberationSans-Bold and draw 507 bright pixels;
the original 100-pixel threshold remains unchanged. The full mediaexec package
passed in 3.95 s, including read/write/network confinement and process recovery.
Evidence logs are `mediaexec-subtitle-debug.log` and
`mediaexec-fontconfig-fixed-suite.log` in the ignored audit scratch directory.
This is a system-toolchain verification result; qualified production bundles
still need their own release checks.

## Resource envelope and remaining gaps

SQLite limits are six foreground connections, two background connections and one
writer, each with a 4 MiB page-cache ceiling: up to 36 MiB in aggregate. The older
policy comment's 32 MiB excludes the writer. Mmap is disabled. SQLite temporary
workspaces, Go heap, image buffers, sockets, TLS and helper processes are additional
memory, so this is not an RSS ceiling.

Password verification uses bcrypt, with two shared hashing slots and existing
source/account cool-offs. No current Argon/scrypt allocation multiplier was found.
Image work now bounds simultaneous decoded images, but one accepted 24-million-
pixel RGBA image alone can require about 96 MB before resize/encoding buffers.
Four compressed originals may coexist; preserving accepted image dimensions does
not imply all maximum-size uploads are cheap on a 256 MB machine.

ARMv6 release cross-compilation initially failed because JavaScript-safe integer
schema bounds defaulted to 32-bit `int`; explicit `int64` constants now compile.
This demonstrates build compatibility, not speed or memory acceptance on a
physical first-generation Pi. No physical Pi browsing/direct-play run has been
completed. Demo runtime memory was observed around 209 MB before the audit
release; no minimum-RAM acceptance claim follows from that single sample.

Recommendation candidate hydration is bounded, but exact facet/history work still
scales with relevant positive history. Isolated 100,000-signal probes measured
roughly 12.5 ms for the pending-job revision sum, 23 ms for franchise aggregation,
and 21 ms for book-series aggregation. Any further optimization must maintain the
exact semantic inputs; replacing history with a cap or approximate aggregate is
not authorized. Larger realistic catalogue/history fixtures and third-party
request-abuse/recovery tests remain necessary after the current bottlenecks are
corrected.

## Final local Home comparison and setup corrections

A quiet paired run used real authenticated HTTP Home requests against a private
copy of the demo database, with one Go execution thread on the Ubuntu i5-10500T.
Both immutable binaries included the same current cancellation patch and server
changes; only execution-statement reuse differed. Twenty warm requests improved
from median 58.726 ms / P95 62.716 ms to 48.884 ms / 50.724 ms. Twelve requests
with three concurrent clients improved from median 152.670 ms / P95 179.956 ms
to 110.417 ms / 139.758 ms. This is a single descriptive local pair, not VPS or
capacity acceptance. Recommendation and permission semantics were unchanged.

Actual Home statement reuse reached 70–71%, with zero bypass flushes. Whole
process user CPU fell from 2.25 s to 1.89 s, with maximum RSS 90,720 versus
93,048 KiB. That comparison used the original 64-entry / estimated 512 KiB
per-connection budget; the committed measured-accounting policy is documented
below. The private copy and
its sidecars were removed after measurement; the original catalogue and active
credentials were untouched. Immutable source and binary identities, request logs
and resource receipts are in `demo-home-final-source.json` and the matching
`demo-home-final-{cached,uncached}` logs under ignored server scratch.

The combined server suite used an unnormalized temporary path containing `..`.
Existing filesystem containment rejected those test paths as designed. The
runner was corrected; hostlimits, operations and persistence pass with normalized
temporary paths and no weaker production path validation. Networking's bare
fixture repeatedly installs the full schema without a transaction, generating
many durable DDL commits on SSD. A diagnostic interruption showed `fsync`, not a
database deadlock; its complete rerun on normalized `/tmp` passed in 51.128 s.
These are verification-environment observations, not production benchmark gains.

The full suite also exposed two actual Ubuntu toolchain portability issues:
sandboxed Fontconfig aliases pointed to an unmounted distribution rule directory,
and system FFprobe's exact dependency mounts exceeded the storage helper's
argument-count guard. The font rules are now available read-only; sandbox and
baseline render the same test subtitle using LiberationSans-Bold, with 507 bright
pixels each versus 29 before the fix. The 100-pixel threshold and sandbox network
and filesystem confinement checks remain unchanged. The generated-command limit
fix and its final scan checks are recorded below when complete.

The generated sandbox argv limit is now shared and bounded at 5,760 entries:
4,096 tool arguments, 512 exact dependency mounts at three entries each, and
128 entries of sandbox/shim overhead. The storage caller's 128 tool-argument
limit, 16 KiB per-argument guard and 64 KiB serialized helper request guard
remain enforced. Oversized generated invocations are rejected before execution.
No broad dependency-directory mounts or sandbox disabling were introduced.
Full mediaexec (3.827 s), storage (7.286 s) and ingestion (18.299 s) suites passed,
including both previously failing real scans and generated-argument boundaries.
One-off diagnostic helpers were moved to ignored scratch.

## Latest strict capacity result and bounded admission follow-up

The final immutable 200-browser / 100-current-v1-stream run still **failed**.
Across the quiet and recovery phases, 12,121 of 26,736 raw requests were refused
(45.3%); raw P95 was 1.501 s, P99 1.665 s, and logical end-to-end P95 with the
normal bounded retry was 2.718 s. There were 4,323 unresolved clean logical
requests despite retries. Four Go execution threads were approximately fully
occupied. The 100 actual media bodies remained active and all distinct readers
consumed at least 1 MiB; playback/media lanes had no refusals, and there were no
hard failures or integrity violations. These facts establish sustained direct
streaming in this fixture, not acceptable combined browsing capacity.

The browsing lane reached its 24 active / 48 waiting limits. It recorded 17,329
refusals across all phases: 14,482 queue-full, 2,798 queue-timeout and 49 caller
cancellations. The maximum write-gate hold was 248 ms, but observed WAL physical
allocation reached 806,703 KiB (about 788 MiB). The source/binary identities,
phase diagnostics, CPU profile and resource samples are retained under ignored
`apps/portico-server/.scratch/performance-audit-20261009/soak-final-capacity.*`.

The subsequent browsing queue correction leaves active work at 24 and each
verified device's waiting allowance at 24. Its global waiting limit uses effective
physical/cgroup RAM: a planning allowance of 256 KiB per waiter within
`min(RAM/16, 128 MiB)`, with the existing 48-place floor and a 512-place ceiling.
This gives 64 waiting places on 256 MiB, 256 on 1 GiB and 512 on larger hosts;
unknown memory retains 48. Only browsing's waiting deadline changes from 1.5 s
to 3 s; its 5 s handler budget and caller cancellation remain. The allowance is
an estimate, not a shared hard RSS reservation across transport, SQLite and
other work. No active execution limit was increased.

Admission-only regression tests hold 24 requests active and 176 queued from 200
previously verified devices behind one NAT, then drain every request successfully.
Additional tests cover memory ceilings/unknown hosts, full bounded queues,
invented credentials and rotated verified credentials sharing one client bound,
prompt cancellation and recovery without retained accounting. Scoped race checks
pass in 1.147 s. This removes a demonstrated burst refusal without proving that
CPU can serve sustained target traffic or that the latency target has passed.

WAL follow-up adds central, bounded reader-owner lifetime diagnostics and
checkpoint outcomes. Transaction/row duration is a conservative proxy, not a
measurement of SQLite's private read marks. Logical frame backlog must be
distinguished from an old physical WAL allocation that SQLite can reuse after
copying frames. At hard logical-frame pressure, new reader admission is paused
briefly so existing readers can drain; cancellation and all exits reopen it.
Drain/gate waiting is limited to 250 ms, followed by the existing 50 ms SQLite
busy budget. Copying and filesystem synchronization can take longer, so these
are not a guaranteed total wall-clock bound. Existing transaction continuations,
gated writers and fresh security-fence reads bypass the pause. A fresh security
read can still wait for an already-saturated connection pool before checkout;
the private gated writer pool is unaffected. Security or external reader pins
can defer a reset and must produce honest backoff rather than forced eviction.

Actual-driver tests preserve static pinned readers, cancellation during drain,
immediate retry backoff and eventual recovery. In the same real 208 MiB fixture,
four continually reopening ordinary snapshot readers completed 24 snapshots;
53,126 frames reset after 13 ms drain / 32 ms total. Ordinary and security
writers progressed before and after, new readers resumed, the reset reported
zero log/checkpoint frames and physical WAL allocation was below 1 MiB. A later
successful writer can legitimately leave a small fresh frame tail; requiring
that subsequent observation to remain exactly zero was an overstrict test
assertion, identified by the combined suite and corrected. The complete dbwork
rerun passed in 40.895 s, with the unchanged physical ceiling and explicit
zero-frame reset assertion; scoped race passed in 1.144 s. The rerun's four
readers completed 25 snapshots, resetting 53,126 frames after 28 ms drain /
47 ms total. No sleep or manual
reader termination manufactured that gap. The combined pressure/churn test
passed in 23.173 s; scoped lifetime/pressure race tests passed in 1.146 s.
Current identity and actual HTTP revocation/grant-fairness checks also passed.
These are focused preservation/convergence checks; a combined immutable capacity
comparison remains acceptance work. Periodic `Yield` is already nonblocking,
so it does not postpone checkpoints until foreground work stops.

Housekeeping now treats only completed `copied` or `truncated` outcomes as
settled. A drain timeout/backoff can retain SQLite's truthful Busy=0 and fully
copied PASSIVE frame counts, yet still need a hard reset. It remains retryable
even when no more commits arrive. Future deferred outcomes also cannot silently
settle. Focused housekeeping regressions passed in 0.009 s, and successful
truncate logging now uses the pre-reset frame count instead of zero.

## Shared pressure scheduling and subsequent combined load results

After the larger browsing queue, request-local hydration reuse and trusted
integer specialization, the unchanged strict four-thread run still failed:
10.5% clean raw refusals, no browsing queue-full refusals but 3,041 waiting
timeouts, and a 551.8 MiB physical WAL peak. All 100 actual media bodies remained
active without hard failures. An identical-code twelve-thread comparison also
failed: 6.93% clean raw refusals, 3.000 s P95, and a 625.8 MiB WAL peak.
Successful clean logical throughput changed only from 138.1 to 140.6 per second.
More execution threads alone did not establish the capacity target; request and
database contention still require measured investigation.

Reader lifetimes and successful resetting checkpoints showed that the remaining
WAL problem was not only pinned snapshots. The old 15-second harness / 30-second
production cadence let normal batches grow past the 192 MiB pressure threshold
before the next check. A reused physical allocation could also remain large
after the logical log/backlog had fallen to zero. The physical 256 MiB peak gate
remains unchanged.

Production and the harness now use the same `dbwork.CheckpointSchedule`: one-second
filesystem pressure probes, with regular SQL checkpoints every 30 seconds only
when unsettled or commits changed. Native filename resolution is bounded to five
seconds on the maintenance pool and cached privately. Ordinary probes issue no
SQL or connection acquisitions. Logical hard frames or physical allocation of
at least 192 MiB trigger the existing drain/TRUNCATE protections. Failed attempts
honor reader backoff plus a one-to-eight-second controller cooldown; unchanged
failed allocations cannot produce an unrestricted SQL retry loop. This is a
scheduling correction, not an absolute disk-growth bound against arbitrarily
large individual transactions or unbounded storage delays.

The faster outer ticker keeps ancillary receipt/backlog pruning and structural
checks on their original 30-second cadence. A real 1,050-expired-receipt test
advances 60 one-second ticks: all rows remain before tick 30, 50 remain until
tick 60, and the second pass finishes, using exactly 11 short writer transactions.
Affected housekeeping checks passed in 0.918 s. Faster pressure sampling does
not silently increase unrelated maintenance throughput.

The actual shared controller resets a fully copied/reused 208 MiB WAL with one
logical frame to zero bytes in 15 ms, even when marked settled, no new commit has
arrived, the normal interval is not due and foreground work is active. Tests also
cover native filenames with spaces, 100 pressure polls with zero SQL, idle
completion/new commits, cancelled or saturated-pool path lookup, physical
pressure overriding settled state, and retained large allocations remaining
unsettled after misleading successful copy counts. Full dbwork passed in 49.300 s;
scoped scheduling/read/security/cancellation race checks passed in 1.262 s.
Harness targeted checks passed in 2.246 s. Source is frozen for the next immutable
combined comparison; the preceding failed results remain acceptance evidence.

## Small-host memory policy and remaining image working set

The startup audit found that `hostlimits.Apply` derived its Go soft memory limit
only from a cgroup ceiling and unconditionally replaced the runtime setting.
Bare-metal physical RAM therefore supplied no default limit, while an owner's
explicit `GOMEMLIMIT` could be raised or overwritten. Startup now preserves any
nonempty `GOMEMLIMIT`, including `off`. Without that override, it uses the smaller
physical/cgroup budget, reserving the greater of 25% or 128 MiB outside the Go
runtime allowance. The resulting defaults are 128 MiB at 256 MiB effective RAM,
384 MiB at 512 MiB, and 768 MiB at 1 GiB. Unknown memory or a derived allowance
below 64 MiB remains unchanged; an already smaller programmatic limit is never
raised. This is a conservative soft policy, not an RSS ceiling or proof of Pi
acceptance. Go may exceed its soft limit when live work cannot be collected.

Actual subprocess startup checks cover an unset override, numeric 96 MiB,
explicit 8 TiB, `off`, a smaller programmatic 32 MiB setting and repeated Apply.
The hostlimits suite passed in 0.010 s and race in 6.062 s; affected cmd/server
housekeeping checks passed in 0.009 s. Integer policy checks include 256/512 MiB,
1 GiB, unknown/tiny memory and maximum integer arithmetic. Existing physical
and ancestor-cgroup tests continue to pass.

SQLite's pool comprises six foreground, two background and one writer
connection, giving a configured page-cache allowance of 36 MiB, rather than
the old comment's 32 MiB. The memory-scaled retained statement allowances total
4.5 MiB at 256 MiB RAM, 18 MiB at 1 GiB and 36 MiB at 2 GiB or above across nine
connections. These describe retained accounting, not total SQL working storage;
active/transient statements and `temp_store=MEMORY` sorts add work. The Linux
modernc libc allocator uses anonymous OS mappings, outside Go's runtime limit.
Neither configured page-cache sizes nor statement accounting establish a hard
RSS bound. Pool and cache policies were not altered by this startup correction.

Full image decoding remains a material unqualified small-host working set. Both
avatars and artwork accept 24 million pixels and 16-bit PNG; Go's PNG decoder
can allocate an eight-byte-per-pixel RGBA64/NRGBA64 backing store, approximately
183.1 MiB for that single buffer. Artwork's destination can add a 1920-square RGBA
buffer, approximately 14.1 MiB, plus encoding storage. The bounded scaler
correction below removes a separately measured large resampling allocation;
it does not eliminate the source decoder's working set. The existing
single shared decoder prevents simultaneous full-size decodes, but cannot make
one such allocation small. Four buffered-original upload slots can retain up
to 40 MiB of accepted compressed artwork bytes, with reading/multipart overhead
additional. Authentication remains before body buffering and permissions are
revalidated at publication. Formats, dimensions and accepted image features
remain unchanged. PNG16/Adam7 and progressive-JPEG decoder working sets remain
unresolved; the scaler improvement is not full-image memory acceptance.
Observed demo idle memory near 173 MiB and a large-host test peak near 310 MiB
do not constitute acceptance on a physical 256/512 MiB Pi.

## Committed final corrections and verification

Commit `82aaa54` combines the shared one-second checkpoint pressure controller,
preserved 30-second ancillary maintenance, owner-respecting startup memory
policy, request-local catalogue composition reuse, measured statement-cache
accounting and bounded artwork scaling. Historical failures remain preserved;
the exact committed representative capacity result appears below.

Recommendation composition now shares immutable candidate metadata and posting
heads only within the same read snapshot and exact profile/viewer/library/
restriction fences. Taste, scoring, eligibility, memo keys and ranking sessions
remain independent; there is no process-wide row or authorization cache.
Unknown facet keys are deduplicated without changing the rarity calculation.
The expanded catalogue regression suite passed in 39.727 s, with related-rail
freshness checks passing in 2.177 s. Paired canonical-credit Detail requests
returned identical full JSON digests in all 24 cases. They saved three to four
statements, with cold timing 27.46 to 26.70 ms and active timing 26.81 to
26.37 ms. This is a small catalogue-only effect, not a large HTTP capacity gain.

The final statement cache retains at most 128 handles per physical connection
within an effective-RAM-scaled 512 KiB to 4 MiB accounting budget. It measures
native SQLite statement storage after clearing bindings, includes retained SQL
copies and Go bookkeeping, and remeasures after automatic reprepare. An expanded
statement over budget retires when its rows close rather than remaining forever
under its original estimate. The maintained driver exposes only aggregate
statement-memory accounting; it does not expose query values. Per-connection
leases, transient overlapping statements, cancellation retirement, conservative
schema/configuration invalidation and current authority reads remain in place.
Budgets bound retained cache storage rather than all SQL/native RSS.

Image scaling previously retained an entire destination-width by source-height
array of four float64 values per pixel. The new separable Catmull-Rom scaler
retains bounded horizontal rows and coefficient work, while preserving the
upstream arithmetic and output. The actual 24MP aspect-preserving 1920-edge
scaler benchmark reduced allocated work from 295,682,256 to 2,589,632 B/op.
Source pixels are procedural and the destination is allocated outside timing,
so this is a scaler-only allocation measurement. It excludes image decoding,
destination storage and encoding, and is not peak RSS. The final single-sample
timings were approximately 1.106 versus 1.112 seconds; no CPU speedup is claimed.
Exact pixel and PNG/JPEG encoded parity cover multiple color representations,
YCbCr ratios, alpha, non-tight strides, up/down scaling and nonzero bounds.
Cancellation and extreme-axis allocation checks pass. Image parity/race checks
passed in 2.994 s and affected artwork integration checks in 9.454 s.

Final dbwork normal verification passed in 38.984 s after fixture generation was
moved outside the unchanged ten-second churn exercise budget. Deterministic
fully changed page payloads preserve the real WAL pressure and reset assertions;
no production timeout or physical-WAL ceiling was relaxed. Scoped pressure/
controller/reader-lifecycle race passed in 1.269 s. Older failed race/test logs
are retained as investigation history and are not represented as passing.

The final combined normal server run passed all packages except the HTTP golden
equivalence test after the load fixture's canonical IDs were made global. The
fixture change was corrected to an explicit opt-in for the representative load
workload, preserving legacy fixtures. The complete affected HTTP package then
passed in 97.651 s using the correct helper environment, closing the golden
regression. Application-package vet passed; the maintained native SQLite bridge
passed vet with its known unsafe-pointer check excluded. No blanket full-server
race or unqualified unsafe-pointer vet claim is made. Receipts are in ignored
`apps/portico-server/.scratch/performance-audit-20261009/`, including
`server-combined-final-test.log`, `httpapi-canonical-optin-full-helper.log`,
`image-scale-final.log`, `image-artwork-final.log` and the final dbwork receipts.

## Clarified performance scope

Justin clarified later on 9 October that the Raspberry Pi example calls for aggressive removal of unnecessary resource use, rather than acceptance of every extreme workload on 256 MB hardware. Keep broadly useful fixes, including the bounded artwork scaler, and stop specialized decoder qualification once major concerns are addressed. Full-frame image decoding remains a documented edge-case limit; no new codec dependency or feature restriction is introduced. The modern-hardware browsing/playback goals remain unproven until the strict capacity evidence supports them.

## Final canonical capacity receipt — 82aaa54

The exact clean committed archive `82aaa54f89efdc2e029b635746627cd8c8b71960`
ran for 180 seconds on all twelve i5-10500T host threads: 200 scripted browsers,
100 direct readers at 1MiB/s, 20 event holds and 4,000 catalogue items. Canonical
Acting/Directing/Writing credits and a real starring-recommendation assertion
exercise provider-style metadata; this is a new representative fixture, not a
same-input percentage comparison against the earlier lowercase-credit results.
Emulators and local builds were stopped; the Windows VM remained running.

**The strict browsing capacity target still failed.** Quiet/recovery had
639/22,681 raw refusals (2.82%), with p95 2.625s/p99 3.000s against unchanged
zero-refusal/750ms/1.5s gates. One normal Retry-After retry left five quiet and
twelve recovery logical calls unresolved, 17/21,654 (0.079%). Successful 2xx
Home p95 was 2.428s, Detail 2.361s and browse 2.350s. The workload achieved
212.7 attempts/s and 180.3 non-failed clean logical calls/s; the latter includes
expected revision-movement HTTP 409 protocol outcomes and does **not** mean
180.3 delivered 2xx content responses/s. The scripts repeatedly fetch full
Home/browse/search/detail/personal-state rounds with 150–450ms think time, so
these figures are not a direct count of occasionally browsing humans.

Real server media-body occupancy reached 100, with 169 consecutive clean and
108 recovery samples at 250ms intervals. All 100 distinct readers consumed at
least 1MiB; total delivery was 15,579MiB. Media response start p95 was 30ms,
playback creation 150ms and timeline updates 129ms. Twenty-six recovery Home
calls succeeded. There were no hard request failures, client timeouts, escaped
SQLite locks or integrity failures; `quick_check` was `ok`, with zero orphaned
rows. Faults were contained and existing direct media remained responsive.
This is loopback delivery of a shared cached file, not physical player,
internet, varied-disk, listener/proxy or transcoding qualification.

The unchanged physical-WAL gate passed: actual observed peak 250,895,672 bytes
(239.27MiB), compared with the diagnostic remembered 235.52MiB, below 256MiB.
The shared production controller completed eleven truncates and four bounded
reader timeouts, zero errors, maximum 253ms per attempt; later retries truncated
successfully. Quiet/fault/recovery truncates were 3/4/4 and timeouts 2/0/2.
Largest valid checkpoint-before logical backlog was 542 frames; final backlog
and active readers were zero. Final physical allocation was 85.27MiB.

Peak RSS was 301.4MiB with no swaps. Request/cleanup profile CPU was 865.24s
over 190.95s (453%); quiet/recovery interiors were approximately 489%/482%.
Cumulative CPU remained concentrated in SQLite execution (46.22%), Home
(37.33%) and Detail (24.32%); preparation was 7.79%, including 2.64% reprepare,
and background writers 5.77%. These categories overlap. Eligible statement
reuse was 96.8%; aggregate SQL pool waits were 352.9s. Browsing had no queue-full
refusals, but 1,130 queue timeouts and 78 cancellations overall. The remaining
miss is expensive browsing and queue delay under sustained synthetic load;
no new major software defect justified extending the audit loop.

Ignored evidence prefix: `soak-canonical-host12-capacity` (log, CPU profile,
resource/monitor logs, source and result JSON). Exact archive/source/binary
hashes and artifact hashes are recorded there; immutable source is
`capacity-commit-20261009T191045Z`. Test and monitor finished at
2026-10-09T19:15:35Z and the heavy window was released. No further capacity run
is planned under the clarified scope; responsive 200-browser/100-consumer
acceptance remains unestablished.

## Final demo activation

The tested `82aaa54` server/web build is active on the single-core/1GB demo. Full state/configuration backup `demo-20261009T191717Z`, SQLite verification, 356 retained entities, 256 retained playable files, disabled transcoding, public readiness and exact build identity all passed. Light loopback checks returned 30/30 HTTP 200: Home mean 105.84 ms (five samples, first 115.29 ms), Discover mean 35.90 ms (25 samples). These exclude WAN delay and are separate from the local 200/100 workload. At a later interactive check the service's systemd memory measurement was 167,006,208 bytes (~159.3 MiB), with nine tasks and no failure exit.
