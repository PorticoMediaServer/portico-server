# Round-2 review — platform-packaging lane

Reviewer: independent second-round read (Claude). Lane author: GPT 6.1 Sol via Codex CLI.
Worktree: `/home/justin/projects/Portico/.worktrees/audit-fixes/server-platform-packaging`, branch `audit-fixes/platform-packaging`, base 82aaa54. Nine follow-up commits `ff0ffad..0b7901f` reviewed against the first review (`review-platform-packaging.md`). Nothing in the worktree was modified (`git status --short` empty before and after every step).

## Headline

- **B-05.28 is no longer blocked in the code.** By reading, a Windows owner can now add a local folder, get titles from a first scan, direct-play an original, convert, and extract embedded subtitles. **None of it has ever run on Windows.** The honest status is "implemented, compiles, unverified", not "partial because of the version contract".
- **The "no-in-place-mutation / strong version evidence" piece the lane calls the remaining blocker is not on any production path, on any platform.** `storage.Client.VersionPolicy` is never set outside tests, so the strong local-version path refuses on Linux and macOS exactly as it does on Windows (`storage/versioned_playback.go:35-36`), and the two preparers that would use it are constructed only in tests. It is not what stands between Windows and local libraries. Details and options in "Question 1".
- No Blocker in the nine commits. Three Should-fix items in the new Windows code: a 30-second stall on every failed root registration, a Windows-only test that cannot pass as written, and ancestor-directory share locks that reach the drive root. All are small.

## How this was checked

Evidence is reading, cross-compiling, the lane's tests on Linux, a small experiment, and Go's own Windows runtime source (`$GOROOT/src/os`, `src/syscall`, `src/internal/syscall/windows`, go1.27.1). No Windows or macOS host, NSIS, `schtasks`, `launchd` or `codesign` was available.

**Cross-compile.** Same method as the first review, in my own scratch directory (`scratchpad/r2-pp/`): an alternate module file that replaces only `github.com/mattn/go-isatty` and `github.com/ncruces/go-strftime` with two-line stubs. Nothing was downloaded (`GOPROXY=off`, `GOTOOLCHAIN=local`).

| Command (with the two stubbed leaf modules) | Result |
| --- | --- |
| `GOOS=windows GOARCH=amd64 go build -modfile=… ./...` | PASS |
| `GOOS=windows GOARCH=arm64 go build -modfile=… ./...` | PASS |
| `GOOS=darwin GOARCH=arm64 go build -modfile=… ./...` | PASS |
| `GOOS=windows go vet` (amd64 and arm64) on `storage`, `sourceaccess`, `mediaexec`, `subtitlevideo`, `subtitles`, `backup`, `httpapi`, `cmd/server`, `childprocess`, `assets`, `localmetadata`, `playback` (compiles the Windows-only tests too) | PASS |
| `GOOS=darwin go vet ./internal/storage/... ./internal/backup/...` | PASS |

`os.Process.WithHandle` (used by the Windows root registration) is in `api/go1.26.txt`; `server/go.mod` says `go 1.26.2`, so it is available.

**Native (Linux) runs:** `go build -o <scratch>/portico-test-helper ./cmd/server`, then with `PORTICO_PLAYBACK_TEST_HELPER` set: `go test ./internal/storage ./internal/sourceaccess ./internal/assets ./internal/localmetadata ./internal/subtitlevideo ./internal/mediaexec ./internal/subtitles ./internal/decoder ./internal/backup ./internal/servicelog ./cmd/server` — all PASS. `go vet` on `storage`, `backup`, `mediaexec`, `sourceaccess` clean. `python3 scripts/test-portable-packaging.py` 2 PASS, `scripts/test-windows-task.py` 1 PASS, `scripts/test-macos-seal.py` 8 PASS. `bash -n` / `sh -n` on the packaging scripts clean; the launchd plist parses.

**Experiment** (for finding 8b55a9f-1): a 30-line Go program in the scratch directory confirmed that `exec.Cmd.Wait` does not return after the child has exited while `cmd.Stdin` is an `io.MultiReader` ending in an unclosed `io.Pipe` — it returned only when the pipe writer was closed.

---

## Per-commit verdicts

### ff0ffad — Preserve complete Windows file identities and reject unsupported roots  (bug B-05.28; first review 2d0a27d findings 2–5)

- **Verdict: RESOLVED WITH NITS** (ACCEPT WITH CHANGES).
- **What it does:** The attribute-only handle now shares read/write/delete and the comment no longer claims it locks anything. Identity is `volume:<64-bit serial>:file:<128-bit id hex>` from `GetFileInformationByHandleEx(FileIdInfo)`, with a fallback to the 32-bit serial and 64-bit index, and an all-zero id is refused. A followed symlink is bound through its resolved target. A library root whose last component is any reparse point is refused before resolution (`checkLocalRoot`).
- **Against the first review:** finding 2 (share mode, comment, last-write time read from the same handle) fixed; finding 4 (ReFS 128-bit id, zero id) fixed; finding 5 (followed symlink) fixed; residual risk (i) (directory size compared between a path stat and a handle stat) fixed for the binding — directories no longer compare size (`local_observation_windows.go:33`). Finding 3 (junction and mounted-folder roots) is still a refusal, now deliberate, held for Justin. Findings 6 and 7 (silent errors; duplicate helper in `administration/`) untouched.
- **Win32 use, checked:** `CreateFile(FILE_READ_ATTRIBUTES, share all, OPEN_EXISTING, BACKUP_SEMANTICS|OPEN_REPARSE_POINT)` is right for files and directories. The `FILE_ID_INFO` struct (`uint64` + `[16]byte`, 24 bytes), `FILE_ATTRIBUTE_TAG_INFO` (8 bytes) and `FILE_BASIC_INFO` (40 bytes) layouts are correct. The fallback writes the 64-bit index little-endian into the low eight bytes, which is where NTFS puts it in the 128-bit id, so an NTFS file has the same id by either route (the volume serial differs between the two routes, 64-bit against 32-bit — only matters if one volume flips between them). Handles are closed through `os.NewFile` + `Close`. Long paths work in the helper because Go's runtime marks the process long-path aware (`runtime/os_windows.go:446`). Unix files gain only a no-op `checkLocalRoot`; `inventory_page.go:198-199` sets `v.Path` to the same value `snapshot` used to be given, so Unix output is unchanged.
- **Findings:**
  1. **Nit** — `local_observation_windows.go:56`: the fallback to the 64-bit index is taken only for `ERROR_INVALID_PARAMETER`, `ERROR_NOT_SUPPORTED` and `ERROR_INVALID_FUNCTION`. Some SMB and third-party filesystems answer `FileIdInfo` with other codes (`ERROR_INVALID_LEVEL`, for one). Any other error leaves the entry with no identity. Fall back on every error except access-denied and invalid-handle.
  2. **Nit / decision** — `client.go:201` + `local_observation_windows.go:80-93`: the root refusal is policy, not a boundary (it is a path check followed by `EvalSymlinks` and `os.Stat`, and `os.OpenRoot` later follows links in the root name anyway). It refuses *any* reparse attribute on the leaf — directory symlink, junction, mounted volume, and also cloud-placeholder folders — while a junction one level up (`C:\Media` → `D:\Stuff`, root `C:\Media\Movies`) is admitted. And it still surfaces to the owner as the generic unavailable error, because the helper exits 1 without saying why (`cmd/server/main.go:112-116`). If the refusal stays, give it its own owner-facing reason.
  3. **Nit** — the helper's silent `os.Exit(1)` is now the single biggest obstacle to native acceptance: every new fail-closed Windows path (job creation, sharing violation, identity mismatch, unsupported reparse tag) looks identical. Print the error class to stderr in the helper; the supervisor already captures and scrubs the tail (`supervisor.go:479`).
- **Tests run:** `go test ./internal/storage` PASS on Linux, including the two new platform-neutral id tests (they fail without the fix by construction: high-bit collision, zero id). Windows vet compiles the Windows tests.
- **Residual risk (needs Windows):** NTFS, ReFS, exFAT and one SMB share: add each as a library root; confirm identity is non-empty, survives rename, and changes on replacement. Scan a folder with ~1,000 files (the page revision at `inventory_page.go:204-214` still compares a directory's handle-stat size with its path-stat size). Confirm a probe does not itself change NTFS ChangeTime — the analysis compares the object revision before and after (`ingestion/analysis.go:288-296`, `:433-435`, outcome `source_revision_changed`), and ChangeTime is in that revision; if reading bumped it, every file would fail. A junction *inside* a library still fails its whole directory (first review residual iv).

### 4979a41 — Run Windows scan probes against a locked admitted file  (bug B-05.28; first review rows 1–5, the blocker)

- **Verdict: RESOLVED WITH NITS, unverified on Windows** (ACCEPT WITH CHANGES). The first review's blocker — no Windows way to run a probe — is addressed on the production path.
- **What it does:** The Windows helper opens the admitted file through `os.Root` as before, asks Windows for that handle's real path (`GetFinalPathNameByHandle`), opens that path again with read access and `FILE_SHARE_READ` only (so nothing can open it for write or delete), opens every ancestor directory up to the drive root the same way, re-checks that the path still names the admitted object (128-bit id and ChangeTime), then runs ffprobe/ffmpeg **as a child** with the real path in place of a marker argument, inside a job object it owns (kill on close, 300 CPU-seconds, below-normal priority, no breakaway). The parent no longer sets `ExtraFiles` for this helper on Windows.
- **Production path, traced:** `ingestion/analysis.go:340` → `assets.Probe.inspect` (`assets/assets.go:114`) → `storage.RunScanCommand` (`scan_content.go:46-106`) → `mediaexec.Argv` (on Windows: no shim, no sandbox; `shim.go:115`, `sandbox_other.go`) → helper op `inventory-command` → `scan_exec_windows.go:145-172`. Same route for playback analysis (`assets/playback_analysis.go:36`) and embedded cover art (`localmetadata/audio.go:81`).
- **Win32 use, checked:**
  - *File lock* (`scan_exec_windows.go:46`): `GENERIC_READ` + `FILE_SHARE_READ` coexists with every other reader Portico has (the `os.Root` source handle shares everything; `os.Open` shares read+write; ffmpeg's own open is `SH_DENYNO`; `OpenPlaybackSource` is the same mode), and refuses new writers, renames and deletes for the life of the probe. If something already holds the file for write, the lock open fails with a sharing violation and the probe fails closed — a reasonable answer for a file still being written.
  - *Path handed to the tool:* `\\?\C:\…` or `\\?\UNC\server\share\…`, buffer 32,768 UTF-16 units, so long paths and UNC are covered; FFmpeg treats a name that does not start with a scheme as a file. `filepath.Dir` on these prefixes stops at `\\?\C:\` and `\\?\UNC\server\share\` (checked against `internal/filepathlite/path_windows.go:206-251`), so the ancestor loop terminates.
  - *Handle inheritance:* none of the lock handles is inheritable (`CreateFile` with nil security attributes), and Go starts children with an explicit handle list, so ffprobe receives only stdin (NUL), stdout and stderr. The admitted file reaches the tool by name, not by handle.
  - *Job object* (`scan_exec_windows.go:120-143`): flags, the 100 ns unit of `PerJobUserTimeLimit`, the non-inheritable job handle, and self-assignment with the pseudo-handle are right; nested jobs need Windows 8+, and Go requires Windows 10. If the job cannot be set up, every probe is refused.
  - *Cleanup:* a parent cancel kills the helper (`childprocess.Kill`), the job handle closes with it, and the tool dies with the job.
  - *Unix unchanged:* `scan_exec_unix.go` is untouched; `scan_content.go` gains only a `runtime.GOOS == "windows"` branch; `scan_exec_other.go` only narrowed its build tag.
- **Findings:**
  1. **Should fix (test cannot pass on Windows)** — `scan_exec_windows_test.go:61-68`, `TestLockedScanInputRejectsPathSubstitution`: the fixture does `os.Open(path)` and then `os.Rename(path, path+".old")`. Go opens files on Windows without `FILE_SHARE_DELETE` (`syscall/syscall_windows.go:395`), so the rename fails with a sharing violation and the test stops at `t.Fatal` on line 67, before it reaches the code under test. Open the fixture handle through `os.OpenRoot(dir).Open(name)` (which shares delete, as production does) or with `windows.CreateFile` and all three share flags.
  2. **Should fix (verify on Windows, then narrow)** — `scan_exec_windows.go:66-80`: every ancestor directory, **including the volume root**, is opened with `GENERIC_READ` and deny-write/deny-delete sharing for the length of each probe. Three consequences. (a) For the duration of a first scan, the library folder, its parents and `C:\` are almost continuously held this way; any other program that opens one of those directories with write-data or delete access (archive tools and sync clients that open a folder with `GENERIC_WRITE` to set its timestamp, a rename in Explorer) gets a sharing violation. (b) If another process *already* holds any ancestor with such access, or with a share mode that omits read, every probe under it fails. (c) `GENERIC_READ` needs list permission on every ancestor; an account that can traverse but not list a parent (ordinary on shared drives; not SYSTEM) cannot probe at all, although inventory through `os.Root` works. Meanwhile the protection is mostly redundant: NTFS and FAT already refuse to rename a directory while a file beneath it is open, which the locked file handle guarantees. Recommended: keep the file lock and the identity re-check, drop the ancestor locks (or stop at the library root and never take the drive root), and re-check the path's identity once more after the tool exits, discarding the output if it no longer names the admitted object.
  3. **Should fix** — `scan_exec_windows.go:166`: the tool runs under `context.Background()`. Lifetime depends on the parent killing the helper (20 s for a probe, `assets/assets.go:97`). If the *server* dies first, nothing ends the helper: it is blocked in `cmd.Run`, the job only bounds CPU time, and a tool stuck on a hung network read uses none. On Unix the ledger and parent-death signal cover this. Give the Windows helper a wall-clock deadline, or have it exit when its stdin closes. (See also cross-commit note on `childprocess_windows.go`.)
  4. **Nit** — the re-check at `scan_exec_windows.go:83-98` proves the path named the admitted object when checked; the tool opens the path itself afterwards. With the file handle locked the only way to redirect the tool is to swap an ancestor in that gap, which needs write access above the library and a filesystem that permits renaming a directory with open descendants. Narrow, and closed by the post-run re-check suggested in finding 2. A path-free hand-off (the loopback bridge already used for subtitle extraction) would remove the question entirely; that is a design choice, not a defect in this commit.
  5. **Nit** — `scan_exec_windows.go:26-29`: a library on a volume mounted only at a folder, with no drive letter, resolves to a path through the mount point; the ancestor loop then meets a name-surrogate reparse point and refuses every probe. Falls away if finding 2 is taken.
  6. **Nit** — the tool now sees the real file name where Unix gives it `/dev/fd/3`. The scan probe carries `-protocol_whitelist file,pipe` and a format whitelist with no playlist, concat, image-sequence or paired-file demuxer (`assets/assets.go:89`), so I found no extra read surface; noted because it is a difference.
- **Tests run:** `go test ./internal/storage ./internal/assets ./internal/localmetadata` PASS on Linux (`TestScanInputReplacementKeepsProtocolAndFormatBounds` exercises the argument replacement). The four Windows tests compile under vet and have never run.
- **Residual risk (needs Windows):** everything. First: run `go test ./internal/storage` on NTFS; scan a folder of movie, TV and music files and confirm titles appear with no `probe_or_sidecar_failed`; repeat as a standard user, not only SYSTEM (setting a job priority class is documented as needing a privilege in some configurations — if `SetInformationJobObject` fails, all probes are refused); repeat with the library on a UNC share and with a path over 260 characters; open a parent folder in a program that holds it for write and scan.

### 844edb1 — Read embedded Windows lyrics from the admitted media handle  (bug B-05.28; first review row 6)

- **Verdict: RESOLVED** (ACCEPT), subject to the same findings as 4979a41, whose lock and job it reuses.
- **What it does:** Adds a Windows `probeLyricTags` that locks the source the same way, runs the server-built ffprobe argv with the path substituted, under the same job, with the existing 8-second timeout, 1 MiB output cap and 128-stream bound. The JSON parsing and the capped buffer moved to a shared file.
- **Unix unchanged:** the diff to `lyrics_probe_unix.go` removes exactly the code that reappears verbatim in `lyrics_probe.go`; the stub's build tag is narrowed.
- **Findings:** none of its own. Findings 2 and 3 under 4979a41 apply (ancestor locks; here the 8-second context does bound the tool).
- **Tests run:** `go test ./internal/storage` PASS on Linux. `TestWindowsEmbeddedLyricProbeUsesAdmittedMedia` compiles; it is sound by reading (it opens with `os.Open` but does not rename).
- **Residual risk (needs Windows):** FLAC and MP3 with embedded synced and unsynced lyrics.

### 8b55a9f — Retain registered Windows roots through supervised observed reads  (bugs B-05.28 and B-04.31; first review rows 7–8 and 9a68c81 finding 1)

- **Verdict: RESOLVED WITH NITS, plus one NEW PROBLEM (Should fix, not a blocker)** (ACCEPT WITH CHANGES).
- **What it does:** Windows root registration: a helper opens the root with `os.OpenRoot`, reports its directory handle number, and waits; the server duplicates that handle out of the helper process (`Process.WithHandle` + `DuplicateHandle`) and acknowledges. Borrowing duplicates the handle in-process. An observed-read helper receives an inheritable duplicate through `SysProcAttr.AdditionalInheritedHandles`, with the handle number in the request, compares it by 128-bit id with the root it opens by path, and reads the file through `os.Root`. Name-surrogate reparse points are refused; other reparse tags (Data Deduplication) are admitted. Unix gets small wrappers so the shared file has no `runtime.GOOS` test.
- **Against the first review:** rows 7 and 8 implemented; B-04.31's dependency (embedded subtitle extraction from local files claimed available with nothing behind it) is now backed by an implementation on the production path (`subtitlevideo/input.go:146-151` → `sourceaccess.Registry.Borrow` → `RegisterRoot` → `OpenObservedPlayback`). The first review's dedup point is addressed in the lane's own checks; see residual risk for `os.Root`.
- **Win32 use, checked:** `DuplicateHandle` arguments are right in all three places (same access, non-inheritable for the server's copies, inheritable only for the one copy listed for the child). The inheritable copy cannot leak into other children because Go always passes an explicit handle list. It is closed after the helper's physical exit and on every early-return path (`observed_playback.go:110-128`, `:174`). Supervisor's `configureProcess` on Windows only clears `ExtraFiles`, so the `SysProcAttr` survives. A wrong or hostile handle number yields nothing useful: the helper stats it, requires a directory, and requires the same object as the root it opens itself; the server never does I/O on the handle. `Process.WithHandle` pins the process object, so there is no PID reuse.
- **Unix behaviour:** equivalent. `sameObservedRoot` is the old `os.SameFile` call. `observedFileBinding` does one extra `fstat` per observation, so size/mtime and inode/ctime now come from two calls instead of one; any change still shows in a later observation, so detection is not weakened. `registerRootHelper` gains an ignored reader parameter.
- **Findings:**
  1. **Should fix (new defect)** — `root_registration_windows.go:53-55` and `:73-103`: `cmd.Stdin` is `io.MultiReader(request, input)` where `input` is an `io.Pipe`, and the pipe writer is closed only on the success path (`:95`) or by the deferred closes that run *after* `RunOwnedCompletion` returns (`:71-72`). When the helper exits without answering — root missing, access denied, reparse root, any error — the reader callback returns `ErrRootLease`, the supervisor calls `cmd.Wait()`, and `Wait` blocks on the stdin copy goroutine, which is blocked reading the pipe. Nothing moves until the context deadline (`c.Timeout`, 30 s in production, `storage.New`). The caller then gets `context deadline exceeded` instead of the real error, and a playback supervisor slot and a `SourceOperations` scope are held for those 30 seconds. Confirmed by experiment (see "How this was checked"). The existing code knows this trap: `playback_stream.go:87` and `observed_playback.go:140` both `defer writer.Close()` at the top of the callback with a comment saying why. Fix: the same one line at the top of the callback at `:73`. `TestRootRegistrationRejectsInvalidAndCanceled` should catch this on Windows (it uses a 1 s timeout and will see the wrong error).
  2. **Nit** — `observed_playback_test.go:21` and `root_registration_test.go`: the shared Unix-written tests now run on Windows (`observedPlaybackSupported` is true). Good coverage, but several rename a root or a file while handles are open, which behaves differently on Windows. Expect fixture work on the first native run; do not read early failures as product failures without looking.
  3. **Nit** — `observed_root_windows.go:15-23`: `cmd.SysProcAttr` is replaced rather than amended. Correct today (nothing sets it earlier for this command); fragile if a caller ever does.
- **Tests run:** `go test ./internal/storage ./internal/sourceaccess ./internal/subtitlevideo ./internal/subtitles ./internal/decoder ./internal/mediaexec` PASS on Linux (the spawn-site inventory test accepts the new Windows entry). Windows build and vet PASS.
- **Residual risk (needs Windows):** `go test ./internal/storage -run 'TestRootRegistration|TestObservedStorage'`; select an embedded SRT/ASS and a PGS track on a local MKV; a file on a Data Deduplication volume — the lane's own code admits it, but `os.Root` opens with `OBJ_DONT_REPARSE` (`internal/syscall/windows/at_windows.go:109-111`) and I could not establish by reading whether that refuses a dedup stub before the lane's check is reached.

### 5fb0e05 — Register Windows startup tasks without relying on PowerShell policy  (bug B-09.4; first review be23914 findings 1–3)

- **Verdict: RESOLVED WITH NITS** (ACCEPT WITH CHANGES — the change is "compile it with real makensis").
- **What it does:** The installer ships `startup-task.xml`, rewrites it at install time into UTF-16 with the launcher path substituted (`&` escaped), and registers it with `schtasks /Create /XML … /F`. The earlier `schtasks /Delete` is gone, so a failed registration leaves the previous task in place; on failure the installer restarts the previous task, shows a message that does not block a silent install, and aborts. The PowerShell script is deleted.
- **Against the first review:** finding 1 (policy can block PowerShell, and the old task was deleted first) fixed; finding 2 (`-InputFormat None`) moot; finding 3 (`/SD IDOK`, duplicated task name) fixed.
- **Task XML against the Task Scheduler schema (by reading):**
  - Root `Task version="1.2"` with the `…/2004/02/mit/task` namespace: correct for Windows Vista and later. The children of `Task`, `Settings` and `Principal` are unordered in the schema, so the order used is valid.
  - *Principal:* `<UserId>S-1-5-18</UserId>` + `<RunLevel>HighestAvailable</RunLevel>`, no `LogonType` — this is the form Windows itself exports for a SYSTEM task. `Actions Context="Portico"` matches `Principal id="Portico"`. No password is needed for SYSTEM; `schtasks` must run elevated, which the installer is.
  - *Trigger:* `BootTrigger` enabled — equivalent to `/SC ONSTART`.
  - *Settings:* `ExecutionTimeLimit` `PT0S` (no limit), `DisallowStartIfOnBatteries` false, `StopIfGoingOnBatteries` false, `MultipleInstancesPolicy` `IgnoreNew`, `AllowStartOnDemand` true, `Enabled` true. These are the three settings the bug is about, set explicitly.
  - *Action:* `%SystemRoot%\System32\cmd.exe` (Task Scheduler expands environment variables in the command; Microsoft's own tasks use the same form) with `/d /s /c ""<launcher>""`. With `/s`, cmd strips the outer quote pair and runs the quoted path, so spaces and `&` in the install path are safe. `&` is the only XML metacharacter a Windows path can contain, and it is escaped (`installer.nsi:51`). No working directory is set, as before; the launcher uses absolute paths.
  - *Encoding:* the output starts with a UTF-16 LE byte-order mark and declares `UTF-16`, which is what `schtasks /Query /XML` produces and what `/Create /XML` expects.
- **Findings:**
  1. **Should fix (unverified build)** — `installer.nsi:2-3`, `:51-66`: this is new NSIS logic (`StrFunc.nsh`, `${StrRep}`, `FileRead`, `FileWriteUTF16LE /BOM`, labels) that has never been through makensis; the packaging test uses a fake. By reading the syntax is right, each template line is far under NSIS's 1,024-character string limit, and the read loop ends on the error flag. Compile it before relying on it.
  2. **Nit (worth doing while the XML is open)** — no `<Priority>`. The schema default is 7, which runs the server at below-normal CPU priority and, on current Windows, reduced I/O and memory priority. That is the same default the old `schtasks /SC ONSTART` line had, so not a regression, but it is a poor default for a media server; `<Priority>4</Priority>` gives normal priority.
  3. **Nit (pre-existing)** — `installer.nsi:40-46`: the batch launcher is still written with `FileWrite`, which converts to the ANSI code page, and cmd reads batch files in the OEM code page. The task XML now carries an accented install path correctly; the launcher it points at does not. The default install path is ASCII. The lane's own test note (install under a path with accented letters) will show this.
  4. **Nit** — on a failed registration during a fresh install the files, uninstaller and registry entries are already in place and there is no task; the owner gets the message and an aborted installer. Acceptable; say so in the installer text if it is ever reworded.
- **Tests run:** `python3 scripts/test-windows-task.py` PASS. It parses the template after a Python imitation of the substitution; it does not exercise the NSIS code.
- **Residual risk (needs Windows):** the lane's note is specific and sufficient: compile with NSIS 3; install and upgrade with a path containing a space and `&`; `schtasks /Query /TN "Portico Media Server" /XML` must show `PT0S`, both battery settings false, `S-1-5-18`, `HighestAvailable`; `/S` install; force a registration failure on an upgrade and confirm the old task remains and is running; x64 and ARM64.

### 553058a — Discard launchd wrapper failures outside the managed startup logs  (bug B-09.6; first review f878c6b finding 1)

- **Verdict: RESOLVED** (ACCEPT).
- **What it does:** Both launchd output paths in the plist are now `/dev/null`, and the two `PlistBuddy` lines that pointed them at append-mode files are removed. The wrapper still writes and truncates the two startup files itself.
- **Checked:** an agent installed from the earlier plist differs from the new one, so `launcher.sh:22-25` boots it out and reinstalls. `launcher.sh:14` still creates the `logs` folder. Nothing can grow without bound: launchd's own sinks discard, and the wrapper's files are truncated on each start.
- **Findings:**
  1. **Nit** — a wrapper that cannot start the server (app moved, `logs` folder deleted) now leaves no evidence at all, while `KeepAlive` retries every five seconds. That is the trade the first review asked for. `mkdir -p "$PORTICO_STATE_DIR/logs"` at the top of `server-launcher.sh` would remove the one self-inflicted case.
- **Tests run:** `sh -n` on both scripts clean; the plist parses and both paths read `/dev/null`.
- **Residual risk (needs macOS):** unchanged from the first review — attribution of the background item and privacy prompts to Portico rather than `sh`; media on external and network volumes still readable.

### 4652e28 — Assess resolved state directories without claiming unchecked Windows access  (bug B-09.15; first review 751ca7e findings 1–2)

- **Verdict: RESOLVED WITH NITS** (ACCEPT).
- **What it does:** The state path is resolved once with `filepath.EvalSymlinks`; the existing lstat / open / same-file check and handle-only `chmod` then apply to the resolved directory. On Windows `CheckPermissions` returns an explicit "not checked" error instead of "not exposed".
- **Answers to question 6:**
  - *Can it change permissions outside the state folder?* No. The only write is still `file.Chmod` on one open directory handle (`stateperms.go:50`), which has been verified with `os.SameFile` against an `Lstat` of the resolved path that was a directory. No descendant is opened, listed or changed. The directory changed is the one the state path resolves to — which is the state folder the server itself uses. The change only removes group and other bits, and only on a directory the server account owns. A swap of the resolved path between the `Lstat` and the `Open` fails the same-file check. Someone who can replace a *parent* of the state folder before the call decides what "the state folder" is; that was already true and such a person controls the server's state anyway.
  - *Symlinked state path?* Handled: `TestPermissionRepairFollowsTheSelectedStateRootOnly` loosens the target, checks through the link, fixes, and asserts the target is `0700` and a `0644` child is untouched. It passes, and the lane reports it failed before the change (the old code rejected the link).
  - *Truthful on Windows?* Yes, as far as the code can say it. At start-up the check logs "State permissions check: state-folder access was not checked on this platform" once and returns before `console.Alert` (`cmd/server/main.go:830-834`), so no alert is set or cleared. The fix route returns `administration_unavailable` (`httpapi/backup_routes.go:167-170`) and never reaches the check.
- **Findings:**
  1. **Nit / decision** — the Windows console still shows nothing, which reads the same as a clean result; "not checked" is only in the log. There is no "not checked" state in `StatePermissionsDocument` to show. This is the open UX question the lane lists for Justin; the code no longer claims a result.
  2. **Nit** — Backups spec §6 wording still to be updated (first review, unchanged).
- **Tests run:** `go test ./internal/backup` PASS on Linux; Windows vet compiles the amended `stateperms_windows_test.go`.
- **Residual risk:** a state folder on a filesystem that ignores `chmod` keeps warning after the fix, as before.

### 7ec0ee4 — Keep portable Windows launchers out of the installer payload  (bug B-09.27; first review 1bb58bc finding 1)

- **Verdict: RESOLVED** (ACCEPT).
- **What it does:** For Windows the script copies the clean payload to `$OUT/portable-windows-<arch>`, adds the portable README and launcher there, and zips that copy. `makensis` is given the untouched payload.
- **Checked:** the release workflow's asset pattern (`dist/*Windows*x64*.*`, `release.yml:138`) is case-sensitive and needs a dot, so the new lowercase directory is not uploaded. The Linux branch is unchanged. The fake `makensis` in the test now fails if either portable file is in the directory it is handed; the test passes, and would fail against the previous script.
- **Findings:**
  1. **Nit** — the first review's CRLF point for `start-portico.cmd` and `README.txt` (finding 3 there) is not addressed.
  2. See Cross-commit issues for the textual conflict with `audit-fixes/identity-ops-live`, which is still present.
- **Tests run:** `python3 scripts/test-portable-packaging.py` 2 PASS; `bash -n scripts/package-release.sh` clean.
- **Residual risk (needs Windows):** unpack the ZIP and run the launcher; inspect the Setup payload.

### 0b7901f — Verify macOS release seals with public material and matching provenance  (bug B-25.8; first review b4202a3 findings 2–5)

- **Verdict: RESOLVED WITH NITS** (ACCEPT). Finding 1 of the first review — the release private key on the hosted macOS runner — is unchanged and remains Justin's decision.
- **What it does:** `--verify` now takes `--public-key` (raw Ed25519, 32 bytes — the same format the builder writes, `build-server-release.py:43`) and refuses `--key`. Sealing can write the public key to a file, and `package-macos.sh` uses that file for the check after app signing. Verification compares the preserved original manifest with the final one field by field, allowing only the two digest fields and the added `sourceSeal` to differ, and checks the preserved toolchain manifest against the digest in the original signed manifest. The workflow installs the signing tools into a virtual environment. The three sandbox-specific environment values are gone from the test.
- **Answers to question 5:**
  - *Public material only?* Yes. I ran verification with the private key deleted (the new test does this) and separately confirmed `--verify --key …` exits 2 with "does not accept a private key".
  - *Can the provenance checks be bypassed?* Not by anyone without the release private key. The final manifest's signature is verified over the raw bytes before anything is parsed (`seal-macos-payload.py:22-23`); `sourceSeal` and its three digests are inside that signed manifest; the original signature is verified with the same key; malformed input raises and exits non-zero. I reproduced the lane's fixture and confirmed the rejection is for the intended reason ("Original source seal describes a different build"), which the committed test does not assert. One thing the holder of the key can do: a manifest re-signed *without* `sourceSeal` verifies (I tried it) — but a key holder can sign anything, so this is a completeness point, not a hole.
  - *Is the signing chain weakened?* No. Nothing was removed from sealing: the private-key file checks, the pre-codesign verification against the builder's signature (which is what ties the key on the macOS runner to the key that signed the build), the post-codesign identity check and the final re-verification are all still there. The public key used after app signing is derived from that same private key, so that step checks that app signing did not alter sealed bytes; it is not, and does not claim to be, a check against an independently published key.
  - *Machine-specific strings?* None left in `scripts/test-macos-seal.py`, the other two script tests, the packaging files or the new Go files (searched for the sandbox cache path, tool and model names, home and temporary paths).
- **Findings:**
  1. **Nit** — `test-macos-seal.py` `test_authenticated_provenance_must_describe_the_same_build`: asserts only a non-zero exit. Add `assertIn("different build", result.stderr)` so it cannot pass for another reason (a missing key file would also exit non-zero).
  2. **Nit** — add `--require-source-seal` and pass it in `package-macos.sh`'s post-app check and in the documented independent check, so a codesigned package without provenance is rejected rather than merely impossible in the normal flow.
  3. **Nit** — the rewritten toolchain manifest's `sourceManifestSha256` is not compared with `sourceSeal["toolchain-manifest.json"]`, and the final toolchain manifest is not compared with the preserved one outside the re-signed `bin/` entries. Both are covered by the final signature; checking them would make the provenance statement complete.
  4. **Nit (unchanged)** — nothing imports a signing certificate into a keychain, so only an ad-hoc identity could work on a hosted runner today.
- **Tests run:** `python3 scripts/test-macos-seal.py` 8 PASS; my own reproduction of the provenance, stripped-seal and private-key-refusal cases (scratch script, temporary directories only).
- **Residual risk (needs macOS and a real identity):** as in the first review — that `go version -m` still reads the build identity from a codesigned Mach-O, and that app signing leaves nested binaries byte-identical (both are checked and fail closed). The virtual-environment step is unrun; by reading, later steps find `python3` and `cryptography` through `GITHUB_PATH`, and the seal script's own `python3` subprocess resolves the same way.

---

## Question 1 — where B-05.28 stands

### End-to-end trace on this branch (by reading; nothing has run on Windows)

| Step | Path | Result on Windows now |
| --- | --- | --- |
| Add a local folder library | `catalog` → `storage.Client.InspectRoot` → helper `root` (`client.go:200-219`) → `checkLocalRoot` → `snapshot` → `observedObjectBinding` | Succeeds for an ordinary folder. Refused if the folder itself is a symlink, junction, mounted volume or cloud placeholder (`local_observation_windows.go:89-91`); the owner sees the generic unavailable error. |
| Scan: inventory | `InventoryPage` → retained stream (`inventory_page.go:92-94`) → `LocalInventoryPage` on `os.Root` | Files and folders listed with identities. Unchanged since the first review, except followed symlinks now bind correctly. |
| Scan: probe | `ingestion/analysis.go:340` → `RunScanCommand` → `scan_exec_windows.go:145` | **Now implemented.** ffprobe runs on the locked file; facts are returned. |
| Scan: playback analysis, cover art | `assets/playback_analysis.go:36`, `localmetadata/audio.go:81` → same route | Implemented. |
| Scan: lyrics | `storage/lyrics.go:33` → `lyrics_probe_windows.go` | Implemented. |
| Scan: publish | `catalog.CommitInventoryAnalysis` (`ingestion/analysis.go:440`) | Reached. **The library gets titles.** |
| Scan: embedded subtitle text | `subtitles.ImportScanText` → `subtitlevideo.Runtime.local` → `RegisterRoot` + `OpenObservedPlayback` + loopback bridge + ffmpeg | Implemented. Failure paths stall 30 s (8b55a9f finding 1). |
| Play an original file | `httpapi/media_original.go:38` → `assets.OpenIsolatedPlayback` → `storage.Client.OpenPlayback` (`playback_stream.go:56`) | **Works by reading, and did before this lane** — a helper per stream reads the file and checks size, modification time and same-file on every block. Nothing in it is Unix-specific and nothing refuses. It has never run on Windows. |
| Play with conversion (HLS) | `playback/hls.go:313` → `assets.OpenPlaybackInput` → `OpenPlaybackSource` (`playback_source_windows.go:43`) | Pre-existing Windows implementation (share-mode lock, path to ffmpeg), marked unverified in its own comment. Not refused. |
| Select an embedded subtitle | `subtitlevideo/input.go:146-151` | Implemented in this lane. |

### What is still refused or stubbed on the library → scan → analyse → play → subtitle path

| # | Where | What | Effect for the owner |
| --- | --- | --- | --- |
| 1 | `storage/local_version_other.go:11-13` (`localVersionEvidence` → `ErrIdentityRequired`) | Strong local version evidence | **None.** Unreachable in production on every platform — see below. |
| 2 | `storage/playback_descriptor_windows.go:14` (`DescriptorPassing = false`) | No handle hand-off to the server | Direct play uses one helper process per stream (cap 128, `supervisor.go:54`). A performance difference, not a refusal. |
| 3 | `storage/playback_descriptor_windows.go:21-24` (`ValidatePlaybackDescriptor` → `ErrPlaybackSource`) | Used only by finite VOD (`playback/finite_hls.go:68`, `playback/vod/encoder.go:132`) | None unless `PORTICO_FINITE_VOD=1` (`cmd/server/main.go:670`, default off). With it on, converted playback fails on Windows. |
| 4 | `local_observation_windows.go:80-93` | Reparse-point library roots | "Add library" fails for a junction, symlink or mounted-folder root. Decision for Justin. |
| 5 | `local_observation_windows.go:97-109`; `os.Root` | Junctions and directory symlinks *inside* a library | The directory containing one fails to inventory (first review residual iv). File symlinks work with "follow symlinks" on. |
| 6 | `windows_object_id.go:6-8` | All-zero file id | A share whose server reports no file ids cannot be a library. |
| 7 | `storage/mount_windows.go`, `root_registration_windows.go:37-39`, `:174-175` | Managed (rclone) mounts | Unsupported; plain local and UNC folders are unaffected. |
| 8 | `storage/inventory_changes_other.go` | No directory change watcher | Same as macOS; changes are found by revision checks. |
| 9 | `mediaexec/sandbox_other.go` | No decoder sandbox | Tools run with the baseline only. `PORTICO_DECODER_SANDBOX=required` refuses all scanning and conversion, by design. |
| 10 | `backup/stateperms.go:19`, `:38` | State-folder access not assessed | Log line only. |

### Plain answer

With this branch, by reading, **a Windows owner can use local libraries**: add a folder, scan it into titles, play originals, convert, and use embedded subtitles. Nobody has seen it do so. The correct label is *implemented, not qualified*.

**The smallest remaining piece of work is one session on a real Windows machine**, not more design:

1. Fix the three Should-fix items (one line in `root_registration_windows.go`; the test fixture; decide on the ancestor locks) and make the helper say why it failed.
2. On Windows: `go mod download` once, `go test ./internal/storage ./internal/sourceaccess`, then the manual pass in the lane's notes (scan, play, convert, subtitle; NTFS first, then a UNC share; SYSTEM and a standard user).

### The "no-in-place-mutation contract" — for Justin

**What it is.** `mediasource.Evidence` of kind `LocalRevision` must carry `NoInPlaceMutation: true` (`mediasource/identity.go:31-36`, enforced at `:60-63`): a promise from the storage adapter that the bytes of this file will not be changed in place while a version of it is in use. The comment is explicit that inode, size and modification time cannot establish that.

**What Unix does to meet it: nothing in the operating system.** `local_version_linux.go:17` and its macOS twin (`local_version_darwin.go:17`) take ordinary `stat` facts (device and inode; mtime and ctime; size) and set `NoInPlaceMutation: true` themselves. The guarantee is supplied by configuration instead: `storage.Client.VersionPolicy` must return, per root, a policy with `StableObjectIDs`, `RevisionTracksWrites` and `NoInPlaceMutation` all true — "a trusted configured root contract … established by the source owner" (`versioned_playback.go:17-27`, checked at `:29-43`).

**And nothing in production configures it.** `VersionPolicy` is assigned only in test files. With it nil, `localPolicy` returns `ErrIdentityRequired` at `versioned_playback.go:35-36` on every platform, before any helper starts. Its two consumers, `playback.NewStrongInputPreparer` and `playback.NewObservedLocalInputPreparer`, are constructed only in tests. So strong local versions are dormant foundation code on Linux and macOS too; the Windows stub differs from Linux only in a place production never reaches.

So the lane is right that Windows "cannot honestly assert" the guarantee from file facts — but neither can Linux, and Linux does not try; it asserts it and relies on a policy gate that is closed. This is not a Windows limitation and not a B-05.28 item.

**What Windows offers, if strong local versions are ever switched on:**

| Mechanism | What it gives | Cost and limits |
| --- | --- | --- |
| Share modes (open without `FILE_SHARE_WRITE` / `FILE_SHARE_DELETE`) | Real enforcement: nobody can write, rename, replace or delete the file while the handle is open. Already used by `OpenPlaybackSource` and the new scan lock. Stronger than anything Unix has. | Other software gets "file in use" for as long as a version is leased (a whole playback). Fails if a writer already has the file. Only for the handle's lifetime; honoured by NTFS, ReFS and Windows SMB servers, not by every NAS. |
| Opportunistic locks (`FSCTL_REQUEST_OPLOCK`) | Notification *before* another opener gets conflicting access; the holder can end its lease instead of blocking them. The closest match to "no mutation while leased, otherwise the lease is broken". | Overlapped I/O and break handling to write and test; not available on every filesystem or share. |
| USN change journal (`FSCTL_READ_FILE_USN_DATA`) | A per-file revision number that advances on every data or metadata change and cannot be set by an ordinary program. A better revision token than timestamps. | NTFS and ReFS only, journal must be active; detects after the fact, does not prevent. |
| 128-bit file id | Stable object identity across renames; not reused while the file exists. Now implemented. | Identity only. On exFAT/FAT it is positional. |
| Last-write time + size + file id + ChangeTime re-check | Detection before and after each read. This is what the observed path already does. | Detection only — and on Windows ChangeTime *can* be set by a program with write-attributes access (`FILE_BASIC_INFO`), unlike Unix ctime, so it is weaker evidence against deliberate tampering. |

**Realistic options:**

- **A. Do nothing now (recommended).** Record that strong local versions are unconfigured on all platforms and that the Windows stub is a placeholder with no owner-visible effect. Re-label B-05.28's remainder as "native Windows acceptance".
- **B. When the strong path is wired in, enforce on Windows with a share-mode lease.** Smallest code (the pattern exists twice), a real guarantee, and the only platform where the contract would be enforced rather than assumed. The price is locking the owner's files against their other tools during playback.
- **C. Same, without blocking others: file id + USN as the revision, with an oplock to learn of a conflicting open.** Best behaviour, most work, NTFS/ReFS only.
- **D. Treat Windows like Unix: assert the flag and gate it on owner-configured policy.** Consistent, cheap, and exactly as honest as Linux is today.

The decision is only needed when someone decides to turn strong local versions on. It should not hold up Windows.

## Question 2 — Windows code correctness (summary)

Covered per commit above. In one place:

- **`CreateFile` flags and sharing:** attribute-only identity open shares everything (correct, locks nothing, says so). Scan lock: `GENERIC_READ`, `FILE_SHARE_READ`, `OPEN_EXISTING`, `FILE_FLAG_BACKUP_SEMANTICS | FILE_FLAG_OPEN_REPARSE_POINT` — right for the file; for ancestor directories it works but is broader than needed (4979a41 finding 2). `BACKUP_SEMANTICS` is present everywhere a directory may be opened.
- **File identity:** `FILE_ID_INFO` layout correct; 64-bit volume serial with 128-bit id; checked fallback; zero id refused.
- **Handing the file to the tool:** by locked, verified path — not by handle. No lock handle is inheritable; the tool inherits only its three standard handles.
- **Handle inheritance server → helper:** only the one explicitly listed root handle for observed reads; closed after the helper's exit and on all early returns.
- **Process cleanup:** helper-owned job with kill-on-close and no breakaway. Gap: no wall-clock bound in the scan helper if the server itself dies (4979a41 finding 3).
- **Path length and UNC:** covered in the helper (Go marks the process long-path aware) and for the tool (`\\?\` and `\\?\UNC\` paths from a 32,768-unit buffer).
- **Build tags:** every stub was narrowed with `&& !windows`; all three targets build; no duplicate or missing symbol.
- **Unix behaviour:** unchanged apart from one extra `fstat` per observed read and an ignored parameter.

## Question 3 — decoder confinement

- **Linux: nothing weakened.** `server/internal/decoder`, `mediaexec` (other than one line in a test's allow-list), `childprocess`, `subtitles`, `subtitlevideo`, `sourceaccess`, `playback` and `assets` have no changes in these nine commits. The Linux scan helper still dups the admitted file to descriptor 3 and execs the sandboxed argv; the Linux branch of `scanHelperCommand` is the old code.
- **What the Windows probe path provides:** no sandbox. ffprobe and ffmpeg run as the server's account — **SYSTEM** under the installer's task — with that account's full file and network access. What bounds them: the protocol whitelist (`file,pipe`) and, for the scan probe, a format whitelist; a minimal environment (`mediaexec.go:290-304`); stdin from NUL; output capped by the parent (4 MiB scan, 1 MiB lyrics); the parent's wall-clock timeout; the helper's job (killed with the helper, 300 CPU-seconds, below-normal priority). The input file cannot be changed or replaced during the probe. `PORTICO_DECODER_SANDBOX=required` still refuses to run anything (`mediaexec.go:196-199`, in the server, before a helper starts). This is the baseline the package documentation already describes for Windows; it is no weaker than what Windows converted playback already had, and it is materially weaker than Linux. Running a media parser as SYSTEM is a pre-existing product posture worth a line in the Windows notes.

## Question 4 — startup task

See 5fb0e05. The XML is valid against the schema as far as reading can tell and sets the three settings explicitly; quoting handles spaces and `&`. Unverified: the NSIS code that renders it.

## Question 5 — macOS seal

See 0b7901f. Public-only verification confirmed by running it; provenance comparison confirmed to reject for the intended reason; signing chain intact; no machine-specific strings.

## Question 6 — state directory

See 4652e28. Cannot change permissions outside the resolved state folder; symlinked path handled and tested; Windows reports "not checked" in the log and makes no claim in the console.

---

## Skipped items

The lane skipped nothing in this round. Two things it declined, and my view:

- **Strong local version evidence on Windows** — declined as too large and safety-sensitive. Declining was right; the reason given overstates its importance (see Question 1). It is not a remaining B-05.28 item.
- **Junction and mounted-folder roots** — left refused for Justin. Reasonable; the owner-facing message should say so.

First-review nits left unaddressed (none blocking): helper errors are silent; duplicate Windows identity helper in `administration/`; CRLF for the portable `.cmd` and README; Backups spec §6 wording; B-09.14's console-side diagnostics; Docker state-directory mode.

## Cross-commit issues

**Inside this lane**

- **4979a41 × 844edb1 × 8b55a9f** share `lockWindowsMediaInput`, `ensureWindowsProbeJob` and `windowsFileBinding`; a change to the ancestor locking fixes all three callers at once.
- **8b55a9f completes 9a68c81 (first round):** the embedded-subtitle gate removed in round one is now backed for local files. Until it has run on Windows, a Windows scan with subtitles enabled will attempt extraction for every embedded text track, and any registration failure costs 30 s per file (8b55a9f finding 1). Fix that line before the first native scan.
- **5fb0e05 × 7ec0ee4:** consistent — `startup-task.xml` is added by the installer script itself, not through the payload, so it does not appear in the portable ZIP.

**Outside this lane's commits, but it changes how the new code behaves**

- `childprocess/childprocess_windows.go:37-43` (base commit, marked unverified in its own comment): the server's job object is created with `JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK`. With that flag, child processes are **not** placed in the job. The comment at `:18-21` and the log text at `cmd/server/main.go:208-213` say the opposite. If that reading is right, a crashed Windows server does not take its helpers or tools with it. The lane sidestepped this correctly by giving the probe helper its own job; the long-lived read helpers exit when their stdin closes; the scan helper is the one left exposed (4979a41 finding 3). Worth its own bug: remove the flag (nested jobs make it unnecessary on Windows 10) and test.

**Against `audit-fixes/identity-ops-live`** (checked with `git merge-file` on scratch copies; nothing written to either repository)

| File | Result | Resolution |
| --- | --- | --- |
| `scripts/package-release.sh` | **Textual conflict, still present**, in the `windows)` case. | As the lane says: the other lane's two lines that make `OUT` and `PAYLOAD` absolute go first, then this lane's portable-copy block, then `zip` and `makensis`. The order matters: `PORTABLE_PAYLOAD` is derived from `OUT`, and the `zip` runs from inside it. Re-run `scripts/test-portable-packaging.py` after merging. |
| `.github/workflows/release.yml` | Merges cleanly. | None. |
| `server/internal/backup/backup_test.go` | Merges cleanly. | Run `go vet ./internal/backup` after the merge. |

## Summary

| Commit | Bug | Against first review | Brief verdict | Blockers |
| --- | --- | --- | --- | --- |
| ff0ffad | B-05.28 | RESOLVED WITH NITS | ACCEPT WITH CHANGES | none |
| 4979a41 | B-05.28 | RESOLVED WITH NITS (unverified on Windows) | ACCEPT WITH CHANGES | none; 3 Should fix |
| 844edb1 | B-05.28 | RESOLVED | ACCEPT | none |
| 8b55a9f | B-05.28, B-04.31 | RESOLVED WITH NITS + NEW PROBLEM (30 s stall on failed registration) | ACCEPT WITH CHANGES | none; 1 Should fix |
| 5fb0e05 | B-09.4 | RESOLVED WITH NITS | ACCEPT WITH CHANGES | none (installer never compiled) |
| 553058a | B-09.6 | RESOLVED | ACCEPT | none |
| 4652e28 | B-09.15 | RESOLVED WITH NITS | ACCEPT | none |
| 7ec0ee4 | B-09.27 | RESOLVED | ACCEPT | none (merge conflict with the other lane) |
| 0b7901f | B-25.8 | RESOLVED WITH NITS | ACCEPT | none (one decision carried over) |

## For Justin

1. **B-05.28 status.** Call it "implemented, awaiting Windows acceptance". The version-contract item the lane lists as the remainder is dormant on every platform and should come off the Windows list. Option A above.
2. **One Windows session is the gate.** Nothing on Windows has run. Fix the three small items first.
3. **Ancestor locking during scans.** Keep the lane's lock-everything-to-the-drive-root approach, or drop it for a file lock plus re-check (my recommendation). Best decided after seeing it on a real machine.
4. **Junction, symlink and mounted-folder library roots on Windows** — still refused. Linux and macOS admit the equivalents.
5. **Windows console wording for state-folder access** — now says nothing rather than "not exposed".
6. **Release private key on the hosted macOS runner** — unchanged from the first review; still needed before signed macOS builds are switched on.
7. **Server job object on Windows** (`childprocess_windows.go`) — outside this lane; likely does not bind children to the server at all. Worth a bug of its own.
