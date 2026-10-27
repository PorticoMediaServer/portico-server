#!/usr/bin/env python3
"""Per-file test watchdog: one process per test file, named hangs, kept results.

A whole-suite invocation hides every result behind whichever test file leaks a
worker or blocks: the suite's own timeout fires, the log shows nothing useful and
the job reports only "timed out". This module runs each selected file in its own
process group instead, with:

* the Node test runner's structured reporter appending one JSON line per test
  start/pass/fail/cancel, so a stopped run still shows every result gathered;
* a per-test limit (`--test-timeout`, passed by the caller) that turns a slow test
  into a named failure for that file only;
* a per-file watchdog that stops only the file that hangs, names the file and the
  test that was still running, and carries on with the remaining files;
* a short exit grace after the file's tests finish, so a leaked handle becomes a
  named failure instead of a stall;
* an explicit never-ran inventory when the run is interrupted.

It is deliberately independent of the platform: a per-file log plus the structured
events are enough to report, and the caller supplies the exact argv for a file.
"""
import json
import hashlib
import os
import signal
import subprocess
import threading
import time
from pathlib import Path

PASS = "pass"
FAIL = "fail"
HANG = "hang"
INTERRUPTED = "interrupted"

DEFAULT_TEST_TIMEOUT_MS = 60_000
DEFAULT_FILE_SECONDS = 120.0
DEFAULT_EXIT_GRACE = 5.0
DEFAULT_TERMINATE_GRACE = 5.0
POLL_SECONDS = 0.05

_TERMINAL_EVENTS = {"test:pass", "test:fail", "test:cancel"}
_FAILURE_LIMIT = 10
_MESSAGE_LIMIT = 500


class FileResult:
    """One test file's outcome, including the events that justify it."""

    def __init__(self, group, file, status, *, tests=0, passed=0, failed=0,
                 cancelled=0, skipped=0, duration=0.0, active_test=None,
                 notes=None, failures=None, log=None, events=None):
        self.group = group
        self.file = file
        self.status = status
        self.tests = tests
        self.passed = passed
        self.failed = failed
        self.cancelled = cancelled
        self.skipped = skipped
        self.duration = duration
        self.active_test = active_test
        self.notes = list(notes or [])
        self.failures = list(failures or [])
        self.log = log
        self.events = events

    @property
    def ok(self):
        return self.status == PASS

    def as_dict(self):
        return {
            "group": self.group,
            "file": self.file,
            "status": self.status,
            "tests": self.tests,
            "passed": self.passed,
            "failed": self.failed,
            "cancelled": self.cancelled,
            "skipped": self.skipped,
            "duration_seconds": round(self.duration, 3),
            "active_test": self.active_test,
            "failures": self.failures,
            "notes": self.notes,
            "log": str(self.log) if self.log else None,
            "events": str(self.events) if self.events else None,
        }


class _EventReader:
    """Incremental JSON-lines reader: complete lines only, partial writes kept."""

    def __init__(self, path):
        self.path = Path(path)
        self.offset = 0
        self.buffer = b""

    def pump(self):
        if not self.path.exists():
            return []
        with self.path.open("rb") as stream:
            stream.seek(self.offset)
            chunk = stream.read()
        self.offset += len(chunk)
        if not chunk:
            return []
        self.buffer += chunk
        *lines, self.buffer = self.buffer.split(b"\n")
        events = []
        for line in lines:
            if not line.strip():
                continue
            try:
                events.append(json.loads(line))
            except (json.JSONDecodeError, UnicodeDecodeError):
                # A line we cannot parse is not a result; leave the rest intact.
                continue
        return events


def _owned_groups(process):
    """Snapshot the command's process groups before signalling it.

    Node's runner and any helper a test starts normally share this command's new
    session, but a nested helper may call setsid(). Descendants are therefore
    reached through PID ancestry from this snapshot, never with a global kill.
    """
    rows = {}
    proc = Path("/proc")
    if proc.is_dir():
        for entry in proc.iterdir():
            if not entry.name.isdigit():
                continue
            try:
                fields = (entry / "stat").read_text().rsplit(")", 1)[1].split()
                rows[int(entry.name)] = (int(fields[1]), int(fields[2]))
            except (OSError, ValueError, IndexError):
                continue
    owned = {process.pid}
    while True:
        found = {pid for pid, (parent, _) in rows.items() if parent in owned}
        if found <= owned:
            break
        owned.update(found)
    groups = {process.pid} | {group for pid, (_, group) in rows.items() if pid in owned}
    groups.discard(os.getpgrp())  # never signal the runner's own group
    return groups


class Runner:
    """Run a list of files sequentially, each in its own supervised process."""

    def __init__(self, events_dir, *, file_seconds=DEFAULT_FILE_SECONDS,
                 test_timeout_ms=DEFAULT_TEST_TIMEOUT_MS, exit_grace=DEFAULT_EXIT_GRACE,
                 terminate_grace=DEFAULT_TERMINATE_GRACE, context=None):
        self.events_dir = Path(events_dir)
        self.events_dir.mkdir(parents=True, exist_ok=True)
        self.file_seconds = float(file_seconds)
        self.test_timeout_ms = int(test_timeout_ms)
        self.exit_grace = float(exit_grace)
        self.terminate_grace = float(terminate_grace)
        self.context = dict(context or {})
        self.results = []
        self.not_run = []
        self.interrupted = None
        self.requested_stop = False
        self.elapsed = 0.0
        self.summary_path = None
        self.selection_path = None
        self._entries = []
        self.live_path = None
        self._started = 0.0
        self._previous_handlers = {}

    # -- lifecycle ---------------------------------------------------------

    def run(self, entries, argv_for, stop=None):
        """Run `(group, cwd, file)` entries in order and return this runner."""
        entries = [(group, Path(cwd), str(file)) for group, cwd, file in entries]
        self._entries = list(entries)
        self._started = time.monotonic()
        self._write_selection(entries)
        self._install_handlers()
        self._print_header(entries)
        try:
            for index, entry in enumerate(entries):
                if self._stop_requested(stop):
                    self.not_run = list(entries[index:])
                    break
                try:
                    result = self._run_entry(entry, argv_for, stop)
                except Exception as error:  # a runner fault must not hide later files
                    result = FileResult(entry[0], entry[2], FAIL,
                                        notes=[f"runner error: {type(error).__name__}: {error}"])
                self.results.append(result)
                self._print_result(result)
                if result.status == INTERRUPTED:
                    self.not_run = list(entries[index + 1:])
                    break
        finally:
            self._restore_handlers()
            self.elapsed = time.monotonic() - self._started
            self._finish()
        return self

    def _write_selection(self, entries):
        """Publish the selected set first, so never-ran files stay discoverable."""
        self.live_path = self.events_dir / "live.jsonl"
        self.live_path.write_text("")
        self.selection_path = self.events_dir / "selection.json"
        self.selection_path.write_text(json.dumps({
            "context": self.context,
            "settings": {
                "file_seconds": self.file_seconds,
                "test_timeout_ms": self.test_timeout_ms,
                "exit_grace": self.exit_grace,
                "terminate_grace": self.terminate_grace,
            },
            "files": [{"group": group, "cwd": str(cwd), "file": file}
                      for group, cwd, file in entries],
        }, indent=2) + "\n")

    @property
    def exit_code(self):
        if self.interrupted is not None:
            return 128 + int(self.interrupted)
        if self.requested_stop:
            return 1
        return 0 if all(result.ok for result in self.results) and not self.not_run else 1

    # -- one file ----------------------------------------------------------

    def _run_entry(self, entry, argv_for, stop):
        group, cwd, file = entry
        stem = "%s__%s" % (group.replace("/", "_") or "root", Path(file).name)
        digest = hashlib.sha256(("%s\0%s" % (group, file)).encode("utf-8")).hexdigest()[:8]
        base = "%s-%s" % (Path(stem).stem[:80], digest)
        events_path = self.events_dir / (base + ".jsonl")
        log_path = self.events_dir / (base + ".log")
        try:
            events_path.unlink()
        except FileNotFoundError:
            pass
        argv = [str(value) for value in argv_for(group, cwd, file)]
        environment = os.environ.copy()
        environment["PORTICO_TEST_EVENTS"] = str(events_path)
        if self.live_path is not None:
            environment["PORTICO_TEST_LIVE_EVENTS"] = str(self.live_path)
        started = time.monotonic()
        reader = _EventReader(events_path)
        with log_path.open("wb") as log:
            try:
                process = subprocess.Popen(
                    argv, cwd=str(cwd), stdout=log, stderr=subprocess.STDOUT,
                    start_new_session=True, env=environment)
            except OSError as error:
                return FileResult(group, file, FAIL, duration=time.monotonic() - started,
                                  notes=[f"could not start: {error}"],
                                  log=log_path, events=events_path)
            outcome = self._supervise(process, reader, started, stop)
            if process.poll() is None:
                self._stop(process)
            try:
                code = process.wait(timeout=self.terminate_grace)
            except subprocess.TimeoutExpired:
                self._stop(process)
                code = None
        duration = time.monotonic() - started
        return self._classify(group, file, outcome, code, duration, log_path, events_path)

    def _supervise(self, process, reader, started, stop):
        running = {}
        counts = {"tests": 0, "passed": 0, "failed": 0, "cancelled": 0, "skipped": 0}
        failures = []
        summary = None
        summary_at = None
        last_event = started
        terminal_tests = 0
        verdict = "exit"

        def consume():
            nonlocal summary, summary_at, last_event, terminal_tests
            for event in reader.pump():
                data = event.get("data") or {}
                kind = event.get("type")
                name = data.get("name")
                nesting = data.get("nesting", 0)
                key = (data.get("file"), name, nesting)
                last_event = time.monotonic()
                if self._is_file_event(data, name, nesting):
                    if kind == "test:fail":
                        failures.append(self._text(self._error_of(data)))
                    continue
                if kind in ("test:dequeue", "test:start"):
                    # Node reports a test as started only as its result is sent;
                    # the dequeue is the live "this test is the one running" signal.
                    running[key] = (name, time.monotonic())
                elif kind in _TERMINAL_EVENTS:
                    running.pop(key, None)
                    terminal_tests += 1
                    if kind == "test:pass":
                        counts["passed"] += 1
                    elif kind == "test:fail":
                        counts["failed"] += 1
                        failures.append(self._failure_text(data))
                    else:
                        counts["cancelled"] += 1
                elif kind == "test:summary" and data.get("file") and data.get("counts"):
                    summary = data
                    summary_at = time.monotonic()

        while True:
            consume()
            code = process.poll()
            now = time.monotonic()
            if self._stop_requested(stop):
                self._stop(process)
                verdict = INTERRUPTED
                break
            if code is not None:
                break
            if summary_at is not None and now - summary_at >= self.exit_grace:
                self._stop(process)
                verdict = "leak"
                break
            if now - started >= self.file_seconds:
                self._stop(process)
                verdict = HANG
                break
            if summary is None and terminal_tests and not running and now - last_event >= self.exit_grace:
                self._stop(process)
                verdict = "leak"
                break
            time.sleep(min(POLL_SECONDS, max(0.0, self.file_seconds - (now - started))))
        consume()  # keep the last events written just before the group was stopped

        if summary and isinstance(summary.get("counts"), dict):
            # The summary is authoritative for the number of tests and skips; the
            # per-event stream is authoritative for pass/fail/cancel, because Node
            # counts a timed-out test as "cancelled" there while its own test:fail
            # event (and the runner's exit status) say it failed.
            if summary["counts"].get("tests") is not None:
                counts["tests"] = summary["counts"]["tests"]
            if summary["counts"].get("skipped") is not None:
                counts["skipped"] = summary["counts"]["skipped"]
        active = None
        active_age = 0.0
        if running:
            active, active_since = list(running.values())[-1]
            active_age = max(0.0, time.monotonic() - active_since)
        return {
            "verdict": verdict,
            "counts": counts,
            "failures": failures[:_FAILURE_LIMIT],
            "active_test": active,
            "active_seconds": active_age,
            "summary": summary,
    }

    @staticmethod
    def _is_file_event(data, name, nesting):
        file = data.get("file")
        if not file or nesting:
            return False
        return name == Path(file).name or name == file

    @staticmethod
    def _error_of(data):
        error = (data.get("details") or {}).get("error")
        return error if isinstance(error, dict) else (data.get("error") if isinstance(data.get("error"), dict) else {})

    @staticmethod
    def _text(error):
        if not isinstance(error, dict):
            return ""
        message = error.get("message") or ""
        return str(message)[:_MESSAGE_LIMIT]

    def _failure_text(self, data):
        message = self._text(self._error_of(data))
        return f"{data.get('name')}: {message}" if message else str(data.get("name"))

    def _classify(self, group, file, outcome, code, duration, log, events):
        counts = outcome["counts"]
        verdict = outcome["verdict"]
        failures = list(outcome["failures"])
        notes = []
        if verdict == INTERRUPTED:
            status = INTERRUPTED
            notes.append("interrupted while this file was running")
        elif verdict == HANG:
            status = HANG
            notes.append(f"file did not finish within {self.file_seconds:g}s")
        elif counts["failed"] or (outcome["summary"] or {}).get("success") is False:
            status = FAIL
        elif verdict == "leak":
            # Tests may all have passed, but the file never exited: that is a real
            # test bug and must never be reported as a passing file.
            status = FAIL
            notes.append(
                f"all {counts['passed']} test(s) finished but the process did not exit "
                f"within {self.exit_grace:g}s (leaked handle or timer)")
        elif code:
            status = FAIL
            notes.append(f"exited with status {code}")
        else:
            status = PASS
        if verdict == "leak" and status == FAIL and not any("leaked handle" in note for note in notes):
            notes.append("the process did not exit after its tests finished")
        if status == HANG:
            active = outcome["active_test"]
            if active:
                notes.append(f"active test: {active} (running {outcome['active_seconds']:.1f}s)")
            else:
                notes.append("active test was not reported: the file blocked before Node "
                             "flushed its events (module load, or a synchronous block)")
        if counts["cancelled"]:
            notes.append(f"{counts['cancelled']} cancelled")
        return FileResult(
            group, file, status, tests=counts["tests"], passed=counts["passed"],
            failed=counts["failed"], cancelled=counts["cancelled"],
            skipped=counts["skipped"], duration=duration,
            active_test=outcome["active_test"], notes=notes, failures=failures,
            log=log, events=events)

    # -- stop --------------------------------------------------------------

    def _stop_requested(self, stop):
        if self.interrupted is not None:
            return True
        if stop is not None and stop():
            self.requested_stop = True
            return True
        return False

    def _alive(self, process, groups):
        process.poll()
        remaining = set()
        for group in groups:
            try:
                os.killpg(group, 0)
            except ProcessLookupError:
                continue
            except PermissionError:
                pass
            remaining.add(group)
        return remaining

    def _wait(self, process, groups, seconds):
        deadline = time.monotonic() + seconds
        while True:
            remaining = self._alive(process, groups)
            if not remaining:
                try:
                    process.wait(timeout=max(0.1, seconds))
                except subprocess.TimeoutExpired:
                    return False
                return True
            if time.monotonic() >= deadline:
                return False
            time.sleep(min(POLL_SECONDS, max(0.0, deadline - time.monotonic())))

    def _stop(self, process):
        groups = _owned_groups(process)
        for sig in (signal.SIGTERM, signal.SIGKILL):
            for group in self._alive(process, groups):
                try:
                    os.killpg(group, sig)
                except ProcessLookupError:
                    pass
            if self._wait(process, groups, self.terminate_grace):
                return True
        return False

    def _install_handlers(self):
        if threading.current_thread() is not threading.main_thread():
            return
        for sig in (signal.SIGTERM, signal.SIGINT):
            self._previous_handlers[sig] = signal.getsignal(sig)
            signal.signal(sig, self._handle_signal)

    def _restore_handlers(self):
        for sig, handler in self._previous_handlers.items():
            try:
                signal.signal(sig, handler)
            except (OSError, ValueError):
                pass
        self._previous_handlers = {}

    def _handle_signal(self, signum, frame):
        self.interrupted = signum

    # -- output ------------------------------------------------------------

    def _print_header(self, entries):
        print(f"WATCHDOG {len(entries)} file(s) | per-file {self.file_seconds:g}s | "
              f"per-test {self.test_timeout_ms}ms | exit grace {self.exit_grace:g}s", flush=True)
        for key, value in self.context.items():
            print(f"  {key}: {value}", flush=True)

    def _print_result(self, result):
        if result.status == PASS:
            print(f"PASS {result.file} ({result.tests} tests, {result.duration:.1f}s)", flush=True)
            return
        label = result.status.upper()
        detail = f"{result.tests} tests, {result.failed} failed, {result.duration:.1f}s"
        print(f"{label} {result.file} ({detail})", flush=True)
        for note in result.notes:
            print(f"  {note}", flush=True)
        for failure in result.failures:
            print(f"  x {failure}", flush=True)
        if result.log:
            print(f"  log: {result.log}", flush=True)

    def _finish(self):
        # Whatever was not reached stays explicit, including after a runner fault.
        if self._entries and len(self.results) + len(self.not_run) < len(self._entries):
            self.not_run = list(self.not_run) + self._entries[len(self.results) + len(self.not_run):]
        totals = {"selected": len(self.results) + len(self.not_run)}
        for status in (PASS, FAIL, HANG, INTERRUPTED):
            totals[status] = sum(1 for result in self.results if result.status == status)
        totals["not_run"] = len(self.not_run)
        if self.interrupted is not None:
            outcome = "interrupted"
        elif totals[PASS] == totals["selected"] and not self.requested_stop:
            outcome = "pass"
        else:
            outcome = "fail"
        summary = {
            "outcome": outcome,
            "elapsed_seconds": round(self.elapsed, 3),
            "settings": {
                "file_seconds": self.file_seconds,
                "test_timeout_ms": self.test_timeout_ms,
                "exit_grace": self.exit_grace,
                "terminate_grace": self.terminate_grace,
            },
            "context": self.context,
            "totals": totals,
            "live_events": str(self.live_path) if self.live_path else None,
            "selection": str(self.selection_path) if self.selection_path else None,
            "results": [result.as_dict() for result in self.results],
            "not_run": [{"group": group, "file": file} for group, _, file in self.not_run],
        }
        self.summary_path = self.events_dir / "summary.json"
        self.summary_path.write_text(json.dumps(summary, indent=2) + "\n")
        print(f"RESULTS {totals[PASS]} passed, {totals[FAIL]} failed, {totals[HANG]} hung, "
              f"{totals[INTERRUPTED]} interrupted, "
              f"{totals['not_run']} not run ({totals['selected']} files, {self.elapsed:.1f}s)",
              flush=True)
        for result in self.results:
            if result.status == PASS:
                continue
            print(f"NOT PASSED {result.status.upper()} {result.file}", flush=True)
        for group, _, file in self.not_run:
            print(f"NOT RUN {file}", flush=True)
        print(f"EVENTS {self.events_dir}", flush=True)
