# Backups and restore: the Plex model

Portico state is open SQLite and plain files. Nothing is encrypted at rest. Filesystem permissions protect secrets. Backups exclude original media and active sign-ins; hashes prove integrity, not authenticity against a host owner.

## Backup and retention

The server makes an online consistent database copy and manifest with required Portico-owned state. Cache, active media production and original media are not copied as the library. Regular backups may use `PORTICO_BACKUP_DIR`. Manual copies remain until deleted; scheduled retention uses the keep count. Three pre-migration copies and pre-restore safety copies stay beside the live database. Backups need space and bounded progress; large-database WAL growth still needs measurement.

## Restore

An owner chooses a listed backup or supported server-side source path and confirms. Validation/staging is detached and pollable with phase/byte progress. Integrity, source shape, current pre-release baseline/schema and available space are checked. Different historical baselines are not promised an upgrade path. Install happens under the restart parent with retained safety copies, durable staged/result evidence and verified rollback.

Ordinary session/refresh credentials are scrubbed and restore signs everyone out. Scrubbing is replayable after failure and must preserve rollback integrity. API credentials are the implemented exception: keys/tokens present in the backup return, including keys revoked after it. Review their lists after restore. Do not describe a universal new external security incarnation or credential invalidation as completed.

The typed current routes cover list/create/delete/stage/status. Read exact response shapes from the generated API and backup client service; the retired synchronous `{staged:true}` example is not a second contract. A minimal startup recovery page exists, but the full recovery console is unfinished.

## Moving and packaging

Stop before copying a complete state folder. Preserve required identity/configuration and update media paths where needed. A second simultaneously advertising clone is not a tested move. Cross-OS relocation, external-path filesystem semantics, SCM/launchd/Docker restore and restore under real load need the report's cold qualification. No deployment or destructive exercise is authorised by this document.

Sources: [operations packaging](../../../_program/reports/ops-packaging.md), [accounts/security](../../../_program/reports/accounts-security.md), [accounts review](../../../_program/reports/review-accounts-security.md), [API credential lifetime](../../../_program/reports/api-platform.md), [limits/settings](../../../_program/reports/limits-settings.md).
