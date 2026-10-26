# Unattended campaign checkpoints

CI runs on the whole `main` tree hourly. The machine scheduler is configured for separate M0 checkpoints every
four hours without an agent driving the applications. They never gate a merge,
stop an owner's process, or change the public demo/Hosted deployment.

Every run freezes published `main` commits, builds on Netcup/Contabo, creates
fresh disposable state, and records each flow separately. Credentials, mail and
browser profiles exist only in private job scratch. Screenshots are withheld
while populated credential controls remain. Evidence contains source pins,
closed operation/error receipts and screenshots, never tokens or message bodies.

A second flaky result permanently drops that flow from automatic execution;
its receipt says that Justin's hands-on must cover it. Product failures do not
become passes, and one surface never holds another. A busy build/device lock
produces a resource-unavailable receipt and leaves the owner working.

The browser checkpoint is `browser.mjs` plus small modules under `browser/`:
`harness.mjs` owns the browser lifetime, per-flow records, redacted captures and
bounded receipts; `receipts.mjs` holds the closed operation/error/CSP allowlists;
`auth.mjs` the real signed-out and sign-in steps; `web.mjs` the web core flows;
`hosted.mjs` the ordinary Hosted account flows; `reading.mjs` publication
geometry. Reading passes only on the paginator's own section frame, rendered
through its whole ancestor chain, with visible text Range geometry on at least
two lines and at least 80 characters; images are recorded in the receipt but are
not a substitute for the unchanged fixture text. The Reading screen's shell
chrome is not a publication. The retired `web:search-real-title` flow is dropped,
not re-enabled. Receipts stay code-only: operation names (including the closed
staged Reading operations: create, progress, source, pages, annotations, search,
assets, file-save, close), status classes, CSP directive and blocked-class counts,
and geometry counts, never URLs, bodies, nonces or account identifiers.

Android automation is the next rollout stage, reusing the existing qualified
APK/Reader packaging and private native metadata checks. Apple runs only after the other automated flows are stable.
`Portico_CertTest_API36` is never a target. All child processes, emulators and
scratch started by a run are retired in `finally` paths.

The loopback Hosted fixture uses the application’s existing isolated development-origin profile with a fresh certified signing root, fresh password-protected PostgreSQL and a verified-TLS private mail gateway. It makes no production-origin/TLS/device-proof acceptance claim. Every message recipient is under `m0.getportico.invalid`; no real user is mailed. Official generators run against each captured tree before compilation; any derived changes are returned as a patch receipt for Release adoption, never silently credited as merged main.

The ledger and owner work lists refresh from all three published-main histories every two hours. The generator preserves Owner notes. A separate hourly machine audit writes CI failures to the Release inbox; app owners receive concrete findings without holding their merges.

## Device custody for one AVD

`avd_custody.py` is the shared helper for every emulator use, as a **two-phase lease
that takes the lock before anything boots or opens**.

    from avd_custody import lease, acquire_mapping, prepare_mapping, read_sessions

    try:
        with lease('PorticoTest_Phone_API36', 'emulator-5554',
                   prepare=lambda: prepare_mapping(device_open()),   # optional, under lock
                   acquire=lambda: acquire_mapping(open_session()),  # real open, under lock
                   handback=close_session,                           # returns a closed result
                   sessions=read_sessions(argv=[agent_device, 'session', 'list', ...]),
                   receipt=out / 'custody-phone.json') as held:
            install(exact_apk)                     # only after admission
            run_flows(held['sessionId'])
    except DeviceInUse:
        record('unavailable', 'DEVICE_IN_USE')     # release and skip, never force
    except CustodyRefused as refusal:
        record('unavailable', refusal.code)        # nothing installed, nothing adopted

A receipt is **mandatory**: the lease writes its held fact before `prepare` runs, and a
missing or unwritable receipt parent refuses `RECEIPT_UNAVAILABLE` (or `RECEIPT_REQUIRED`
when no path is given) before prepare, acquire or the body, so a device is never used
without a written record. `python3 avd_custody.py <avd>` stays the optional lock-only
probe and writes nothing.

`prepare` (optional) runs under the lock and may device_open or boot the target; it must
return exactly `{'serial','avd'}`, and a different device refuses `PREPARE_IDENTITY`. A
known target may already be booted — that is allowed and `prepare` is skipped, but it is
never treated as identity or admission.

**Conflicts are decided before anything opens — including before `prepare`.** Any other
Android session that reports this serial **or** this exact AVD on any serial is a
claimant. One that already exists when the lease starts makes the helper raise
`DeviceInUse` **before `prepare` runs**, so a `device_open` or boot is never attempted
against a live session; the list is checked again after prepare and identity, still
before `acquire`. A claimant that appears after the open refuses
`SESSION_NOT_EXCLUSIVE`. A foreign session that reports the same serial with a different
device name is still a conflict — the display name can never filter it away.

`acquire()` must return exactly `{'sessionId','serial','avd'}` from a real open
(`ACQUIRE_SHAPE`, `ACQUIRE_IDENTITY` otherwise, no coercion). The returned session id is
written to the receipt **immediately** after the open, so a later refusal still leaves
the handle on record, and there is no automatic cleanup when identity is unclear. The
session must not have been listed before (`SESSION_NOT_NEWLY_ADMITTED`), and afterwards
the list must hold **exactly one** row with the returned name, `platform: android`, this
exact serial and this display name: a duplicate name refuses `SESSION_NOT_EXCLUSIVE`, a
non-Android platform refuses `SESSION_WRONG_PLATFORM`, and another serial or display
refuses `SESSION_NOT_ADMITTED`, as does a missing row. Rows are type-checked: a non-string
`name`, `id`, `device` or `platform` makes the list unreadable rather than guessed at.

`handback` receives the exact admitted `{'sessionId','serial','avd'}` and must **return an
explicit structured acknowledgement**: `{'operation': 'app-close'|'panel-close',
'result': 'closed'|'already-absent'|'refused', 'code': …}`. Nothing else counts. A
subprocess exit status — even zero — is not an acknowledgement, and neither is `None`,
`{}`, a bare `{'result': 'closed'}` without an operation, or an unknown operation such as
`adb-kill`; all of those record `unspecified`, never success. Every callback-supplied
code is normalised to a bounded closed form (`[a-z][a-z0-9-]{1,48}`) or to `unspecified`,
so no raw exception text, path or message reaches the receipt. `agentSessionRetirement`
stays `unproven by this helper` because an app or panel close does not retire an agent
session. The helper performs no close of its own.

Integration seam, stated rather than claimed: `native-checkpoint.py` already holds the
AVD lock, but its target file carries a session opened earlier by the owner and the
checkpoint has no supported agent-device open argv, so wiring it would mean adopting that
pre-existing session — which this helper refuses — or adding a new session-open path.
The checkpoint, driver and wrapper are unchanged — checkpoint ordering is **not**
integrated — so this helper covers cooperative scripted walks and the owner of
`device_open` only, and acquire-before-install ordering stays a wrapper decision. `python3 avd_custody.py <avd>` prints `free`/`busy` without touching a device.

Synthetic regression tests, `test_avd_custody.py`, run with no adb, CLI, session or
device: 23 cases cover refused admission running no install/use/cleanup, serial mismatch
never acquiring, a stale claimant skipping before open, same-serial/wrong-name and
same-AVD/other-serial conflicts, conflicts appearing only after admission (handle
persisted, nothing admitted), strict acquire and prepare shapes, `prepare` seeing the lock
held, a missing or absent receipt parent refusing before anything runs, callback codes
normalised to a bounded form, and handback `None`/`{}`/command/operationless/unknown
results.

What is *not* enforced: a session opened outside the helper, an `adb` or launcher call made
around it, and any physical handback. Cooperative callers are covered; the claimant check
catches another lane's live session, not an arbitrary external tool.
