# Backups and restore: the Plex model (normative)


**Justin's philosophy:** Portico's data is open. An owner can open a backup in any SQLite viewer, copy the state folder to another machine and have it just work, and tinker freely. **No encryption of the user's own data.** Keep it simple, like Plex.

## 1. What a backup is
- A folder `<state>/backups/<UTC timestamp>/` (for example `2026-09-24T013000Z`) containing:
  - `portico.db`: a **consistent copy of the server database**, made with SQLite's online backup (or `VACUUM INTO`) while the server runs. It must never block writers for long (short steps, yielding between them).
  - The **few other state files** needed to bring the server back as the same server: its identity or key files, the configuration files. **Not** rebuildable data (transcodes, caches, downloaded artwork, logs, prepared media) and not `backups/` itself. The lane lists exactly which files, from the state-folder layout.
  - `manifest.json`: `{"format":"portico-backup","version":1,"serverId","serverVersion","schemaVersion","createdAt","kind":"scheduled|manual|pre-restore","files":[{"name","bytes","sha256"}]}`. The hashes are for integrity checks only, **not** security.
- **Plain files**, with the same ownership and permissions as the state folder. **No encryption, no keys, no archive format, no chunking.**
- **Schedule and retention:** the existing backup settings (when to run, how many to keep). "Back up now" on demand. Refuse with a clear code when free disk is below the estimated size.

## 2. Restore
- **Sources:** (a) one of the listed backups, or (b) **a path on the server** the owner points at: a backup folder (with a manifest) **or a bare `.db` file** that is a Portico database (it has the schema ledger). The console uses the existing folder picker.
- **Flow:**
  1. Validate the source: `PRAGMA integrity_check`; it is a Portico database; its schema version is **≤ the running server's**. An older version is fine, because migrations run on start.
  2. **Stage** it in `<state>/restore-staged/`.
  3. Answer `{staged: true, restartRequired: true}`, then **restart the server** (a clean exit for the service manager to restart, as the existing restart path does).
  4. At the next start, before opening the database:
     - move the current database and files into `<state>/backups/<ts>-pre-restore/` (`kind: pre-restore`);
     - move the staged copy into place;
     - start normally, **running the ordinary migrations** (an older backup is upgraded);
     - if integrity or migrations fail, **put the pre-restore copy back** automatically and start on it, recording the failure for the console.
  5. After a restore, existing sign-ins from before the backup may no longer be valid. That's expected; clients go to the sign-in screen as for any ended session.
- **No in-place restore engine, no owner proof, no journals beyond the one staged-restore marker.** The API is owner-only, like every admin route; the console asks for a plain confirmation.

## 3. Moving a server (documented, and tested)
**Stop Portico, copy the whole state folder to the new machine, point Portico at it, start.** It must just work. Portico's own files are referenced **relative to the state folder**, never by absolute path. Media library paths are absolute and may need changing in the console if the new machine mounts media elsewhere. Moving across operating systems (Windows ↔ Linux) is an aspiration, not a release requirement. Don't build it now, but don't design against it: no OS-specific paths or encodings in the database for Portico's own files.

## 4. API (owner-only; publish it in the OpenAPI with apigen)
- `GET /v1/admin/backups` → `{backups:[{id, createdAt, kind, schemaVersion, serverVersion, bytes, path}], running?: {jobId, phase, bytesDone, bytesTotal}, lastRestore?: {at, outcome: "restored"|"rolled_back", reason?}}`
- `POST /v1/admin/backups {operationId}` → starts a backup (idempotent); progress appears in `running`.
- `DELETE /v1/admin/backups/{id}`
- `POST /v1/admin/backups/restore {operationId, source: {backupId} | {path}}` → `{staged:true, restartRequired:true, validation:{schemaVersion, integrity:"ok"}}`, or 4xx with codes: `restore_source_invalid`, `restore_schema_newer`, `restore_integrity_failed`, `restore_not_portico`, `insufficient_disk`.
- The existing backup schedule and retention settings keep their current routes.

## 5. Tracker consequences
- **SEC-24** ("backup key beside backups"): **won't fix, by design.** No encryption, like Plex; the folder permissions protect it.

## 6. Plain state everywhere
The same openness rule applies to the whole state folder, not only backups.
1. **No sealed data with keys kept beside it.** Remove the encryption and the key files for:
   - `authorization-replay.key` (identity families, Quick Connect, onboarding, factors, refresh receipts);
   - `networking-tls-seal.key` (the TLS private key becomes a normal `.pem` file in the state folder, `0600`);
   - `networking-claim-credentials.key`;
   - the Live TV source store key (`livechannels/store.go`);
   - the rclone mount config (`mounts/config.go`: write rclone's plain config format).

   Store the values as plain database columns or plain files. Folder permissions are the protection, as in Plex. Remove the fail-closed "lost key" paths along with them. Hashing that is **not** encryption stays: password hashes, token hashes and the TOTP secret handling as designed.
2. **Permissions: warn, explain, offer; never force or refuse.** Remove the checks that refuse to run when the state folder isn't `0700` (e.g. `networking/claim_cipher.go`). Instead:
   - on start, check Portico's state folder and its files;
   - if they are readable by other users, show a console **warning** that explains why it matters ("other accounts on this computer can read your Portico data, including sign-in secrets") and offers a **"Fix permissions"** action. That action tightens only Portico's own files; the owner may dismiss the warning.
   - Never change permissions without the owner's action, and never refuse to start.
   - Publish it as a health item in the existing server health or alerts contract, with the fix as an owner-only operation (OpenAPI via apigen).
3. With the key files gone, §1's "identity or key files" means only what remains (the server identity and the TLS `.pem`). List them in the results.
