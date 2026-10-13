# Operations

Release targets are Windows, macOS, Linux and Docker. The implementation programme wrote packaging and runtime code without deploying or publishing it. [Operations packaging](../../_program/reports/ops-packaging.md) is the source for this document and its cold acceptance procedures. Live infrastructure is described in `deployment/hosted/TOPOLOGY.md`; historical host names are not evidence of current deployment.

## Process and packages

A small restart parent runs the server worker. Owner Restart and restore can restart the worker under portable launch or plain Docker. Shutdown joins the worker and helpers before cleanup. Windows packaging uses SCM, LocalService by default and a service SID; owner-configured accounts are required for authenticated UNC shares. macOS uses a system launchd daemon as `_portico`, with state at `/Library/Application Support/Portico Media Server`; the app opens the browser. Linux packaging preserves enabled/stopped state during upgrade. Uninstall preserves application state and original media.

These paths cross-compile and have packaging fixture checks. Actual SCM/launchd/Linux package lifecycle, Docker init/drain, GPU access and signed release workflow remain unverified. Docker bridge multicast is not LAN discovery; use suitable networking or explicit addresses. No current installed service is inferred from the package source.

## Storage, backup and restore

State remains plain SQLite/files. Optional cache and backup destinations are wired. Manual backups stay until deleted; scheduled retention uses the owner's keep count. Backup timing uses the configured local window, including DST handling. New databases reclaim pages incrementally. Pending migrations make retained online copies; three pre-migration copies are kept beside the database.

Restore validation/staging is detached and pollable, with phase/byte progress. Safety copies, staged files and result markers support install/rollback. Session scrubbing is replayable and backed by completion evidence; restore ends ordinary sign-ins. API credentials can reappear from the backup and need owner review. Current pre-release schema/baseline checks apply; no older-database upgrade path is promised. See [Backups](Specs/Spec%20%E2%80%94%20Backups%20%28Plex%20model%29.md) and [accounts review](../../_program/reports/review-accounts-security.md).

A minimal startup recovery page is reported by [limits/settings](../../_program/reports/limits-settings.md). The complete recovery console and broad failure instrumentation remain open in ops-packaging. Large online-backup WAL growth and restore behaviour on actual OS filesystems still need acceptance.

## Console and reports

Owner APIs expose resource usage, effective cache/backup paths, reclaimable database bytes, pre-upgrade copies and FFmpeg validation state. Signed-in clients can send bounded redacted reports; owners can list, read, download and delete them. Report limits are 100 total, five per device, 256 KiB per body and 1 MiB per daily account budget. Console follow-ups add corresponding screens; native report sending and catalogue integration still need device acceptance.

Background speed is Gentle/Normal/Fast. An explicit temporary pause lasts one hour; backup scheduling does not gate all background jobs. Measured usage must say unknown when not measured, rather than zero. [Limits/settings](../../_program/reports/limits-settings.md), [server follow-ups](../../_program/reports/server-followups.md) and [console](../../_program/reports/console.md) record remaining seams.

## Release and verification

Manual signed updates remain the policy. The default feed reports available releases with opt-out; no automatic install is inferred. Official artifacts need an independently pinned public signing key and authenticated final inventory, including web payloads. macOS installer signing needs its real installer identity. Native mpv/libass/FFmpeg notices and corresponding source distribution remain release tasks.

Normal development and test work runs on personal Ubuntu; Apple SDK/device work runs on the Mac. Use the reports' focused checks and cold procedures. Builds do not establish hardware codecs, tuners, proxy/IPv6 discovery, millions of titles or sustained client capacity. Private testing guidance remains in `deployment/testing/README.md`. No full builds or suites are needed for this documentation update.

## Current independent review

The independent [operations review](../../_program/reports/review-ops.md) is in progress, including filesystem-failure recovery checks. Existing successful fixtures do not certify every rollback boundary.
