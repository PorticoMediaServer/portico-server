#!/usr/bin/env python3
"""Full hourly checks, with bounded selection retained for explicit local use."""
import argparse
import json
import os
from pathlib import Path
import queue
import re
import signal
import subprocess
import sys
import tempfile
import threading
import time

ROOT = Path(__file__).resolve().parents[1]
TEST = re.compile(r"^func (Test[A-Z]\w*)\(", re.M)

# Per-package deadline passed to `go test -timeout`. This is the hang guard for
# one package, not a scope filter: the slowest package measured on mini-1
# (cmd/server) took 248 s, so three times that, rounded up, still finishes every
# real package while a hang is named inside the package that hung.
GO_TEST_TIMEOUT = 13 * 60  # 780 s
# Outer watchdog for one package invocation. go test's own deadline fires first
# and prints every running test; only a go test that cannot exit reaches this,
# and that package is then terminated on its own.
GO_PACKAGE_BACKSTOP = GO_TEST_TIMEOUT + 2 * 60  # 900 s
# Packages run as separate processes, as `go test ./...` did with -p=3. A hung
# package holds its own lane only; other lanes and the rest of the inventory
# keep producing results in this run.
GO_PACKAGE_JOBS = 3
# Cores one run may use. mini-1 has 12; the campaign keeps 10 and leaves the
# system two, and any smaller machine keeps its own smaller count.
GO_RUN_CORES = 10
# Bound for the declared-test inventory (`go test -list`) collected after a hang.
GO_LIST_TIMEOUT = 5 * 60
# How long the output pipe may stay open after the go test leader has exited.
# `go test` closes its stdout when it exits, so a pipe that is still held belongs
# to a descendant that inherited it: that package is named and stopped instead of
# blocking its lane (and the whole run) forever.
PIPE_HOLD_GRACE = 30
# Idle heartbeat of one package's pipe loop; also the resolution of the pipe-hold
# check.
PIPE_POLL = 0.2
# How often a package's descendant groups are re-sampled. A sample walks /proc, so
# it is deliberately coarse: ancestry only has to be known before the leader exits.
DESCENDANT_SAMPLE = 2

# Per-command bound for everything else. This is a hang guard for one suite
# invocation, not a scope filter: the mandatory security commands stay exactly
# as selected, and each still gets the whole window.
COMMAND_TIMEOUT = 15 * 60
TERMINATE_GRACE = 5
TAIL = 6
# Bounded on-screen annotations: the first failures only. The complete list
# stays in the package records, the step summary and the raw event stream.
ANNOTATION_LIMIT = 10
# Bounds for the decoded failure detail kept in the package record and printed
# under the summary. The raw go test event file keeps every event unchanged.
OUTPUT_LIMIT = 400
MAX_OUTPUT_LINES = 8_000
PACKAGE_OUTPUT_LIMIT = 40
BLOCK_LINES = 20
STEP_SUMMARY_ROWS = 200
VERSION_TIMEOUT = 60


class CommandTimeout(subprocess.CalledProcessError):
    """A command outlived its bound; its process group was terminated."""

    def __init__(self, argv, timeout):
        super().__init__(124, argv)  # 124 is the conventional timeout exit status
        self.timeout = timeout


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT, text=True).splitlines()


def changed():
    if os.environ.get("CI_FULL_TREE") == "1":
        return set(git("ls-files"))
    base = os.environ.get("CI_BASE", "")
    if not base or set(base) == {"0"}:
        base = "HEAD^"
    # Missing history is an error, never permission to omit affected checks.
    return set(git("diff", "--name-only", "--diff-filter=ACMRTD", base, "HEAD", "--"))


def _process_rows():
    """pid -> (ppid, pgid) from /proc, or nothing when /proc is unavailable."""
    rows = {}
    proc = Path("/proc")
    if not proc.is_dir():
        return rows
    for entry in proc.iterdir():
        if not entry.name.isdigit():
            continue
        try:
            # The command name is parenthesised and may contain spaces, so the
            # interesting fields start after the last ')'.
            fields = (entry / "stat").read_text().rsplit(")", 1)[1].split()
            rows[int(entry.name)] = (int(fields[1]), int(fields[2]))
        except (OSError, ValueError, IndexError):
            continue
    return rows


def _descendants(roots, rows):
    """Every process that descends from `roots`, according to one /proc snapshot."""
    owned = set(roots)
    while True:
        found = {pid for pid, (parent, _) in rows.items()
                 if parent in owned and pid not in owned}
        if not found:
            return owned
        owned |= found


class ProcessTree:
    """Every process group owned by one child command.

    A command runs in its own session, so its tree normally shares one group. A
    helper may call setsid(), and ancestry cannot be recovered once the leader
    exits (its children are reparented), so the tree is sampled while the leader
    is alive and those groups are signalled afterwards. The runner never
    signals its own group.
    """

    def __init__(self, process):
        self.process = process
        self.pids = {process.pid}
        self.groups = {process.pid}
        self.lock = threading.Lock()

    def sample(self):
        rows = _process_rows()
        with self.lock:
            if rows:
                owned = _descendants(self.pids, rows)
                self.pids |= owned
                self.groups |= {rows[pid][1] for pid in owned if pid in rows}
            self.groups.discard(os.getpgrp())
            return set(self.groups)


def _signal_groups(groups, sig):
    for group in sorted(groups):
        try:
            os.killpg(group, sig)
        except (ProcessLookupError, PermissionError):
            pass


def _groups_alive(groups):
    for group in tuple(groups):
        try:
            os.killpg(group, 0)
        except ProcessLookupError:
            continue
        except PermissionError:
            return True
        return True
    return False


def _await_groups(process, tree, seconds):
    """Wait up to `seconds` for every owned group to empty, reaping the leader."""
    deadline = time.monotonic() + seconds
    while _groups_alive(tree.groups):
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            return False
        process.poll()  # reap the leader as soon as it exits
        time.sleep(min(0.05, remaining))
    process.poll()
    return True


def _stop_group(process, tree=None):
    """TERM the command's owned groups, give a bounded grace, then KILL what remains."""
    tree = tree if tree is not None else ProcessTree(process)
    tree.sample()
    _signal_groups(tree.groups, signal.SIGTERM)
    _await_groups(process, tree, TERMINATE_GRACE)
    # The grace is for the group, not only its leader: a leader that exits on TERM
    # must not hide a descendant that ignores it, so the group is always confirmed
    # with KILL. Both waits stay bounded, and a command that cannot be killed is
    # reported by the failing timeout rather than hanging this runner again.
    _signal_groups(tree.groups, signal.SIGKILL)
    _await_groups(process, tree, TERMINATE_GRACE)


def _stop_all(commands):
    """Stop several owned commands together: signal everything first, then reap.

    Stopping them one at a time would multiply the shutdown grace by the number
    of packages a shard has in flight; this keeps the same two bounded waits.
    """
    commands = list(commands)
    for command in commands:
        command.tree.sample()
    for command in commands:
        _signal_groups(command.tree.groups, signal.SIGTERM)
    for command in commands:
        _await_groups(command.process, command.tree, TERMINATE_GRACE)
    for command in commands:
        _signal_groups(command.tree.groups, signal.SIGKILL)
    for command in commands:
        _await_groups(command.process, command.tree, TERMINATE_GRACE)


class OwnedCommand:
    """One live child command that a signal must stop before this runner exits.

    go test packages each run in their own session, so the runner's own process
    group cannot reach them: the owned tree is what makes shutdown complete.
    """

    def __init__(self, process):
        self.process = process
        self.tree = ProcessTree(process)
        self.result = None
        self.stopped = False
        self.stop_lock = threading.Lock()

    def stop(self):
        with self.stop_lock:
            if self.stopped:
                return False
            self.stopped = True
        _stop_group(self.process, self.tree)
        return True


_ACTIVE = set()
_ACTIVE_LOCK = threading.Lock()


def _register_command(command):
    with _ACTIVE_LOCK:
        _ACTIVE.add(command)
    return command


def _unregister_command(command):
    with _ACTIVE_LOCK:
        _ACTIVE.discard(command)


def _active_commands():
    with _ACTIVE_LOCK:
        return list(_ACTIVE)


def run(args, cwd=ROOT, timeout=None, stdout=None):
    argv = list(map(str, args))
    limit = COMMAND_TIMEOUT if timeout is None else timeout
    print("+", " ".join(argv), flush=True)
    # A captured command keeps its output for parsing; the caller echoes what
    # matters, so a failing `go list` is never silently swallowed.
    process = subprocess.Popen(argv, cwd=cwd, start_new_session=True, stdout=stdout,
                               stderr=subprocess.STDOUT if stdout is not None else None)
    command = _register_command(OwnedCommand(process))
    try:
        code = process.wait(timeout=limit)
    except subprocess.TimeoutExpired:
        command.stop()
        print(f"timed out after {limit:g}s, command terminated: {' '.join(argv)}",
              file=sys.stderr, flush=True)
        raise CommandTimeout(argv, limit) from None
    finally:
        _unregister_command(command)
    if code:
        raise subprocess.CalledProcessError(code, argv)
    return code


def run_capture(args, cwd=ROOT, timeout=None):
    """Run one bounded command and return its combined output."""
    with tempfile.TemporaryFile(mode="w+", encoding="utf-8", errors="replace") as captured:
        error = None
        try:
            run(args, cwd, timeout, stdout=captured)
        except subprocess.CalledProcessError as failure:
            error = failure
        captured.seek(0)
        text = captured.read()
    if error is not None:
        if text:
            print(text, end="" if text.endswith("\n") else "\n", flush=True)
        raise error
    return text


def run_all_status(commands):
    """Run every command, keep going after failures, and return the exit status."""
    failed = False
    timed_out = False
    for args, cwd in commands:
        try:
            run(args, cwd)
        except subprocess.CalledProcessError as error:
            failed = True
            timed_out = timed_out or isinstance(error, CommandTimeout)
    return 124 if timed_out else 1 if failed else 0


def run_all(commands):
    status = run_all_status(commands)
    if status:
        raise SystemExit(status)


def test_names(files):
    return {name for file in files if file.is_file()
            for name in TEST.findall(file.read_text()) if name != "TestMain"}


def go_plan(module, paths):
    prefix = module + "/"
    selected = {}
    for name in sorted(paths):
        if not name.startswith(prefix) or not name.endswith(".go"):
            continue
        file = ROOT / name
        directory = file.parent
        if not directory.is_dir():
            continue
        package = "./" + str(directory.relative_to(ROOT / module))
        tests = list(directory.glob("*_test.go"))
        if name.endswith("_test.go"):
            candidates = [file]
        else:
            candidates = list(directory.glob(file.stem + "*_test.go"))
            if not candidates:
                # Catalogue is deliberately never expanded to its full suite on a push.
                # Check compile/vet here; named counterparts and all package tests run nightly.
                candidates = [] if package == "./internal/catalog" else tests
        selected.setdefault(package, set()).update(test_names(candidates))
    if paths & {module + "/go.mod", module + "/go.sum"}:
        selected["./..."] = set()  # compile the changed module dependency graph
    return selected


def module_packages(module):
    """List the module's packages one by one so each gets its own bounded run.

    `go list -e` still names packages with build errors, so a broken package is
    tested and reported on its own instead of cancelling the inventory. An empty
    or failed inventory is an error, never permission to run nothing.
    """
    root = ROOT / module
    try:
        text = run_capture(["go", "list", "-e", "-f", "{{.ImportPath}}\t{{.Dir}}", "./..."], root)
    except CommandTimeout as error:
        raise RuntimeError(f"go list in {module} did not finish within {error.timeout:g}s") from None
    except subprocess.CalledProcessError as error:
        raise RuntimeError(f"go list in {module} failed with exit {error.returncode}") from None
    packages = []
    for line in text.splitlines():
        _, _, directory = line.partition("\t")
        if not directory.strip():
            continue
        try:
            relative = Path(directory).resolve().relative_to(root.resolve())
        except (OSError, ValueError):
            continue
        packages.append("." if relative == Path(".") else "./" + relative.as_posix())
    if not packages:
        raise RuntimeError(f"go list in {module} reported no packages")
    return packages


def shard_settings():
    """Read the deterministic shard selection; a malformed shard fails closed."""
    index = os.environ.get("CI_SHARD_INDEX", "0")
    total = os.environ.get("CI_SHARD_TOTAL", "1")
    if not index.isdigit() or not total.isdigit() or int(total) < 1 or int(index) >= int(total):
        raise RuntimeError(f"invalid Go package shard: index={index!r} total={total!r}")
    return int(index), int(total)


def shard(packages, index=0, total=1):
    """Cover one sorted inventory disjointly: shard N holds positions N mod total."""
    return [package for position, package in enumerate(sorted(packages)) if position % total == index]


def report_path(module, shard_index, shard_total):
    """Where the structured per-run record goes, if anywhere.

    CI_GO_RESULTS names the file directly; otherwise the persistent
    CI_TEST_REPORT_DIR from the local job runner collects one file per shard.
    """
    explicit = os.environ.get("CI_GO_RESULTS")
    if explicit:
        return explicit
    directory = os.environ.get("CI_TEST_REPORT_DIR")
    if not directory:
        return None
    Path(directory).mkdir(parents=True, exist_ok=True)
    name = f"go-results-{module.replace('/', '-')}-shard{shard_index + 1}-of-{shard_total}.jsonl"
    return str(Path(directory) / name)


def events_path(module, shard_index, shard_total):
    """Where the unchanged go test -json event stream is retained, if anywhere.

    It stays separate from the compact package/test record so the record is
    readable and the raw stream is still complete. CI_GO_EVENTS names the file;
    otherwise it sits beside CI_GO_RESULTS (or in the runner's report directory)
    and still matches the artifact's `go-results-*.jsonl` upload pattern.
    """
    explicit = os.environ.get("CI_GO_EVENTS")
    if explicit:
        return explicit
    results = os.environ.get("CI_GO_RESULTS")
    if results:
        path = Path(results)
        return str(path.with_name(path.stem + "-events" + path.suffix))
    directory = os.environ.get("CI_TEST_REPORT_DIR")
    if not directory:
        return None
    Path(directory).mkdir(parents=True, exist_ok=True)
    name = (f"go-results-{module.replace('/', '-')}-shard{shard_index + 1}-of-"
            f"{shard_total}-events.jsonl")
    return str(Path(directory) / name)


def limit_cores(report):
    """Keep this run within GO_RUN_CORES and give every child an explicit GOMAXPROCS."""
    if not hasattr(os, "sched_setaffinity"):
        return
    allowed = sorted(os.sched_getaffinity(0))
    if len(allowed) > GO_RUN_CORES:
        allowed = allowed[:GO_RUN_CORES]
        os.sched_setaffinity(0, allowed)
        report.note(f"CPU affinity limited to {len(allowed)} cores: "
                    f"{', '.join(map(str, allowed))}")
    os.environ.setdefault("GOMAXPROCS", str(max(1, len(allowed))))


def selected_names(pattern):
    """The names our own anchored alternation selects, or None for the whole package."""
    match = re.fullmatch(r"\^\((.*)\)\$", pattern or "")
    return set(match.group(1).split("|")) if match else None


def declared_tests(module, package, report):
    """The package's declared tests, named after a hang; never guessed."""
    try:
        text = run_capture(["go", "test", "-list", "^Test", package], ROOT / module,
                           timeout=GO_LIST_TIMEOUT)
    except subprocess.CalledProcessError as error:
        report.note(f"{package}: test inventory unavailable "
                    f"(go test -list exit {error.returncode})")
        return None
    except CommandTimeout as error:
        report.note(f"{package}: test inventory unavailable "
                    f"(go test -list exceeded {error.timeout:g}s)")
        return None
    return [line.strip() for line in text.splitlines() if re.fullmatch(r"Test[A-Z]\w*", line.strip())]


_FAILURE_LINE = re.compile(
    r"^\s*(?P<file>[^\s:()]+\.go):(?P<line>\d+)(?::(?P<column>\d+))?:\s*(?P<text>.*)$")
_LOCATION = re.compile(r"(?P<file>[^\s:()]+\.go):(?P<line>\d+)(?::\d+)?(?::|\s|$)")
_QUIET_PREFIXES = ("=== RUN", "=== PAUSE", "=== CONT", "=== NAME",
                   "--- FAIL", "--- PASS", "--- SKIP")


def _quiet(line):
    """A go test framing line that carries no failure reason."""
    text = line.strip()
    return (not text or text in {"FAIL", "PASS"} or text.startswith("# ")
            or text.startswith(_QUIET_PREFIXES)
            or text.startswith(("ok  ", "ok\t", "FAIL\t", "FAIL ", "exit status ")))


def explain(lines):
    """The first real reason in one decoded test block, with its source line.

    go test writes `file_test.go:63: message` for a failed assertion and a stack
    frame as `\t/path/file_test.go:63 +0x…` for a panic. The assertion form wins
    as the message; the first frame of either form supplies the location.
    """
    message = ""
    file = ""
    line = 0
    for raw in lines:
        text = raw.strip()
        if not text:
            continue
        match = _FAILURE_LINE.match(text)
        if match:
            if not file:
                file, line = match.group("file"), int(match.group("line"))
            candidate = match.group("text").strip()
        else:
            located = _LOCATION.search(text)
            if located and not file:
                file, line = located.group("file"), int(located.group("line"))
            candidate = text
        if not message and candidate and not _quiet(candidate):
            message = candidate
    return message or "test failed", file, line


def bound_output(lines, limit=OUTPUT_LIMIT):
    """Keep a readable head and tail of one block; the raw stream keeps all."""
    if len(lines) <= limit:
        return list(lines)
    head = limit // 2
    tail = limit - head
    omitted = len(lines) - limit
    return [*lines[:head], f"... {omitted} output line(s) omitted; see raw go events ...",
            *lines[-tail:]]


def normal_message(message):
    """Collapse whitespace so identical failures group under one reason."""
    return " ".join((message or "test failed").split())


def escape_data(value):
    """Escape one GitHub workflow-command data value."""
    return str(value).replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")


def escape_property(value):
    """Escape one GitHub workflow-command property value."""
    return (escape_data(value).replace(":", "%3A").replace(",", "%2C"))


def markdown_cell(value):
    """One bounded, single-line Markdown table cell."""
    text = " ".join(str(value).split())
    if len(text) > 300:
        text = text[:297] + "..."
    return text.replace("\\", "\\\\").replace("|", "\\|")


def annotation(file, line, message):
    """One escaped ::error workflow command, with a location when it is known."""
    data = escape_data(message)
    if not file:
        return f"::error::{data}"
    properties = f"file={escape_property(file)}"
    if line:
        properties += f",line={int(line)}"
    return f"::error {properties}::{data}"


class PackageResult:
    """One package's own go test invocation, including what it never reported."""

    def __init__(self, package, pattern, module=None):
        self.package = package
        self.pattern = pattern
        self.module = module
        self.action = None       # go test's terminal action for the package
        self.elapsed = 0.0
        self.tests = {}          # test name -> last action (run/pass/fail/skip)
        self.outputs = {}        # test name -> decoded output lines (bounded)
        self.package_output = [] # decoded package-level output (last lines)
        self.timed_out = False   # go test's own -timeout fired inside the package
        self.watchdog = False    # the outer watchdog terminated this package
        self.pipe_hold = False   # the leader exited but a descendant kept its stdout
        self.interrupted = False  # a signal stopped this package before it finished
        self.exit = None
        self.finished = False
        self.tail = []
        self.declared = None     # declared test names, collected after a hang
        self.inventory = None    # "reported", "unavailable" or None
        self.expected = 0        # declared or selected tests the run owed a result
        self.selected = None     # names from the -run pattern, None for the whole package
        self.never_ran = []      # expected tests with no pass/fail/skip result
        self.lock = threading.Lock()  # an interrupt reads while the lane still writes

    def status(self):
        if self.interrupted:
            return "interrupted"
        if self.pipe_hold:
            return "pipe-hold"
        if self.watchdog:
            return "watchdog"
        if self.timed_out:
            return "timeout"
        if self.action in {"pass", "skip", "fail"}:
            return self.action
        if self.exit:
            return "fail"
        return "pass" if self.exit == 0 else "incomplete"

    def failed(self):
        return [name for name, action in self.tests.items() if action == "fail"]

    def unfinished(self):
        return [name for name, action in self.tests.items() if action == "run"]

    def location(self, path):
        """A repository-relative path for an annotation or a step summary.

        go test may print a bare test filename or an absolute path into this
        worktree. Both are normalised to `<module>/<package>/<file>` (relative to
        the repository) so an annotation opens the right source file.
        """
        if not path:
            return ""
        relative = Path(path)
        if relative.is_absolute():
            try:
                return relative.resolve().relative_to(ROOT.resolve()).as_posix()
            except (OSError, RuntimeError, ValueError):
                return relative.as_posix()
        text = str(path).replace("\\", "/")
        package = self.package.removeprefix("./")
        module = self.module or ""
        if not module:
            return f"{package}/{text}" if package not in {"", "."} and "/" not in text else text
        if text.startswith(module + "/"):
            return text
        if package and text.startswith(package + "/"):
            return f"{module}/{text}"
        if "/" not in text or text.startswith("../"):
            return f"{module}/{package}/{text}" if package not in {"", "."} else f"{module}/{text}"
        return f"{module}/{text}"

    def package_path(self):
        """The repository-relative directory of this package."""
        package = self.package.removeprefix("./")
        if self.module and package not in {"", "."}:
            return f"{self.module}/{package}"
        return self.module or package or self.package

    def failures(self):
        """Decoded failure blocks for every test this package reported as failed."""
        rows = []
        for name in sorted(self.tests):
            if self.tests[name] != "fail":
                continue
            lines = self.outputs.get(name, [])
            message, file, line = explain(lines)
            rows.append({"test": name, "message": message,
                         "file": self.location(file), "line": line,
                         "output": bound_output(lines)})
        return rows

    def package_reason(self):
        """The first real reason in the package's own, non-test output."""
        return explain(self.package_output)[0] if self.package_output else "test failed"

    def record(self):
        return {"record": "package", "module": self.module,
                "package": self.package, "status": self.status(),
                "exit": self.exit, "elapsed": round(self.elapsed, 3), "pattern": self.pattern,
                "timeout": self.timed_out, "watchdog": self.watchdog,
                "pipe_hold": self.pipe_hold, "interrupted": self.interrupted,
                "tests": self.tests, "failed": self.failed(), "unfinished": self.unfinished(),
                "failures": self.failures(),
                "package_output": self.package_output[-PACKAGE_OUTPUT_LIMIT:],
                "inventory": self.inventory, "declared": self.declared,
                "selected": sorted(self.selected) if self.selected is not None else None,
                "expected": self.expected, "never_ran": self.never_ran}


HALTED = {"timeout", "watchdog", "pipe-hold", "incomplete", "interrupted"}


def describe(result):
    status = result.status()
    parts = [status.upper(), result.package, f"tests={len(result.tests)}", f"{result.elapsed:.1f}s"]
    if result.failed():
        parts.append("failed " + ", ".join(result.failed()))
    if status in HALTED:
        running = result.unfinished()
        if status == "interrupted":
            parts.append("unfinished " + (", ".join(running) or "no running test reported"))
        else:
            parts.append("hung " + (", ".join(running) or "no running test reported"))
        if result.inventory == "reported":
            label = "declared" if result.selected is None else "selected"
            parts.append("no result for " + (", ".join(result.never_ran) or "no declared test")
                         + f" ({len(result.never_ran)} of {result.expected} {label})")
        elif result.inventory == "unavailable":
            parts.append("test inventory unavailable")
        else:
            # Interrupted before the inventory could be collected: name what was
            # still running, never claim a declared result the run did not gather.
            parts.append("declared inventory not collected")
    return "  ".join(parts)


def snapshot(result):
    """Describe and record one package consistently, even while a lane still runs."""
    with result.lock:
        return describe(result), result.record()


def decode(line):
    """One go test output line as (human text, parsed JSON event or None)."""
    text = line.rstrip("\n")
    if text.startswith("{"):
        try:
            return text, json.loads(text)
        except json.JSONDecodeError:
            return text, None
    return text, None


def feed(result, line, report, event=None):
    """Merge one go test -json event into the package result and the record."""
    text = line.rstrip("\n")
    if event is None:
        event = None
        if text.startswith("{"):
            try:
                event = json.loads(text)
            except json.JSONDecodeError:
                event = None
    if not isinstance(event, dict):
        with result.lock:
            if text:
                result.tail = (result.tail + [text])[-TAIL:]
            if "panic: test timed out" in text:
                result.timed_out = True
        return
    action = event.get("Action", "")
    if action == "output":
        output = event.get("Output") or ""
        if not isinstance(output, str):
            output = str(output)
        parts = [part for part in output.splitlines() if part]
        with result.lock:
            if "panic: test timed out" in output:
                result.timed_out = True
            result.tail = (result.tail + parts)[-TAIL:]
            name = event.get("Test")
            if name:
                buffer = result.outputs.setdefault(name, [])
                buffer.extend(parts)
                if len(buffer) > MAX_OUTPUT_LINES:
                    del buffer[:len(buffer) - MAX_OUTPUT_LINES]
            else:
                result.package_output.extend(parts)
                if len(result.package_output) > 2 * PACKAGE_OUTPUT_LIMIT:
                    del result.package_output[:-PACKAGE_OUTPUT_LIMIT]
        return
    if action == "build-fail":
        with result.lock:
            result.action = "fail"
        return
    name = event.get("Test")
    if name:
        with result.lock:
            if action in {"run", "pause", "cont"}:
                result.tests[name] = "run"
            elif action in {"pass", "fail", "skip"}:
                result.tests[name] = action
        if action in {"run", "pass", "fail", "skip"}:
            report.test_event(result, action, name, event.get("Elapsed"))
        return
    if action in {"pass", "fail", "skip"}:
        with result.lock:
            result.action = action
            result.elapsed = float(event.get("Elapsed") or 0.0)


class GoReport:
    """Live package results, a flushed JSONL record, and a closing summary."""

    def __init__(self, module, planned, shard_index, shard_total):
        self.module = module
        self.planned = list(planned)
        self.shard = (shard_index, shard_total)
        self.lock = threading.RLock()  # a signal may re-enter while a summary prints
        self.results = {}
        self.stream = None
        self.events = None
        path = None
        try:
            path = report_path(module, shard_index, shard_total)
            if path:
                self.stream = open(path, "w", encoding="utf-8")
        except OSError as error:
            # A broken report location must not cost the run its console record.
            self.write(f"!! {module}: structured record unavailable ({path}): {error}\n")
        events = None
        try:
            events = events_path(module, shard_index, shard_total)
            if events:
                self.events = open(events, "w", encoding="utf-8")
        except OSError as error:
            # The decoded failure record and console still stand on their own.
            self.write(f"!! {module}: raw go events unavailable ({events}): {error}\n")
        if self.stream:
            self.record({"record": "plan", "module": module,
                         "shard": {"index": shard_index, "total": shard_total},
                         "packages": self.planned})

    def write(self, text):
        with self.lock:
            sys.stdout.write(text)
            sys.stdout.flush()

    def note(self, message):
        self.write(f"!! {self.module}: {message}\n")

    def _append(self, entry):
        if self.stream:
            self.stream.write(json.dumps(entry, sort_keys=True, default=str) + "\n")
            self.stream.flush()

    def record(self, entry):
        with self.lock:
            self._append(entry)

    def raw_event(self, result, event=None, line=None):
        """Retain one complete go test event, flushed as the run goes."""
        with self.lock:
            if not self.events:
                return
            entry = {"record": "go-event", "module": self.module,
                     "package": result.package}
            if event is not None:
                entry["event"] = event
            else:
                entry["line"] = line
            self.events.write(json.dumps(entry, sort_keys=True, default=str) + "\n")
            self.events.flush()

    def package_start(self, result):
        with self.lock:
            self._append({"record": "package-start", "module": self.module,
                          "package": result.package, "pattern": result.pattern})

    def test_event(self, result, action, name, elapsed):
        with self.lock:
            entry = {"record": "test", "module": self.module, "package": result.package,
                     "test": name, "action": action}
            if elapsed is not None:
                entry["elapsed"] = elapsed
            self._append(entry)

    def add(self, result):
        line, record = snapshot(result)
        with self.lock:
            if result.package in self.results:
                return  # an interrupt already recorded this package
            self.results[result.package] = result
            sys.stdout.write(line + "\n")
            sys.stdout.flush()
            self._append(record)

    def failure_rows(self, results):
        """Every failing test (or a failing package with no test) in a stable order."""
        rows = []
        for result in sorted(results, key=lambda item: item.package):
            for failure in result.failures():
                rows.append({"package": result.package, "test": failure["test"],
                             "message": failure["message"], "file": failure["file"],
                             "line": failure["line"], "output": failure["output"],
                             "status": result.status()})
            if result.status() == "fail" and not result.failed():
                rows.append({"package": result.package, "test": "",
                             "message": result.package_reason(),
                             "file": result.package_path(), "line": 0,
                             "output": result.package_output[-BLOCK_LINES:],
                             "status": "fail"})
        return rows

    def write_step_summary(self, results, never, interrupted):
        """Append the package/test/message/location table to the GitHub job page."""
        path = os.environ.get("GITHUB_STEP_SUMMARY")
        if not path:
            return
        rows = self.failure_rows(results)
        halted = [result for result in results if result.status() in HALTED]
        lines = [f"### Go tests: `{self.module}` shard "
                 f"{self.shard[0] + 1}/{self.shard[1]}", ""]
        if rows:
            lines += ["| Package | Test | First message | Location |",
                      "| --- | --- | --- | --- |"]
            for row in rows[:STEP_SUMMARY_ROWS]:
                location = (f"{row['file']}:{row['line']}"
                            if row["file"] and row["line"] else row["file"])
                cells = (row["package"], row["test"] or "(package)",
                         row["message"], location or "\u2014")
                lines.append("| " + " | ".join(markdown_cell(cell) for cell in cells) + " |")
            if len(rows) > STEP_SUMMARY_ROWS:
                remaining = len(rows) - STEP_SUMMARY_ROWS
                lines.append(f"| \u2026 | {remaining} more failures | see go-results JSONL | \u2014 |")
        else:
            lines.append("No failing tests.")
        if halted:
            lines += ["", "Halted packages: " + ", ".join(
                f"{result.status().upper()} `{result.package}`" for result in halted)]
        if never:
            lines += ["", "Never started: " + ", ".join(f"`{package}`" for package in never)]
        if interrupted:
            lines += ["", "The run was interrupted; results gathered before it are above."]
        try:
            with open(path, "a", encoding="utf-8") as out:
                out.write("\n".join(lines) + "\n")
        except OSError as error:
            self.write(f"!! {self.module}: GitHub step summary unavailable ({path}): {error}\n")

    def summary(self, interrupted=False):
        with self.lock:
            results = list(self.results.values())
            counts = {}
            for result in results:
                counts[result.status()] = counts.get(result.status(), 0) + 1
            never = [package for package in self.planned if package not in self.results]
            rows = self.failure_rows(results)
            lines = [f"==== go test summary: {self.module} shard "
                     f"{self.shard[0] + 1}/{self.shard[1]} ====",
                     f"packages: {len(self.planned)} planned, {len(results)} started, "
                     f"{sum(1 for result in results if result.finished)} completed, "
                     f"{len(never)} never started",
                     "results: " + (", ".join(f"{name} {counts[name]}"
                                              for name in sorted(counts)) or "none")]
            groups = {}
            for row in rows:
                groups.setdefault(normal_message(row["message"]), []).append(row)
            repeated = [(message, members) for message, members in groups.items()
                        if len(members) > 1]
            if repeated:
                lines.append("identical failure messages:")
                for message, members in sorted(repeated,
                                               key=lambda pair: (-len(pair[1]), pair[0])):
                    first = members[0]
                    location = (f"{first['file']}:{first['line']}"
                                if first["file"] and first["line"] else first["file"])
                    suffix = f"  (first: {first['package']} {location})" if location else ""
                    lines.append(f"  x{len(members)}  {message}{suffix}")
            for result in results:
                if result.status() in {"pass", "skip"}:
                    continue
                line, _ = snapshot(result)
                lines.append(line)
                if result.status() in HALTED:
                    lines.extend("    " + tail for tail in result.tail[-TAIL:])
                failures = result.failures()
                for failure in failures:
                    location = (f"{failure['file']}:{failure['line']}"
                                if failure["file"] and failure["line"] else failure["file"])
                    heading = failure["test"] + (f"  {location}" if location else "")
                    lines.append(f"    FAIL  {heading}")
                    lines.append(f"      {failure['message']}")
                    block = failure["output"]
                    if block:
                        lines.append("      --- decoded output ---")
                        lines.extend("      " + item for item in block[:BLOCK_LINES])
                        if len(block) > BLOCK_LINES:
                            lines.append(f"      ... {len(block) - BLOCK_LINES} more "
                                         f"line(s); see raw go events ...")
                if result.status() == "fail" and not failures:
                    lines.append(f"    FAIL  (package)  {result.package}: "
                                 f"{result.package_reason()}")
                    lines.extend("      " + item
                                 for item in result.package_output[-BLOCK_LINES:])
            # A package with no record after the plan record was never started.
            for package in never:
                lines.append(f"NEVER RAN  {package}  no result  never started")
            # Test failures become annotations first; a package failure without a
            # test still gets one, all within the bounded count.
            for row in rows[:ANNOTATION_LIMIT]:
                what = (f"{row['test']}: {row['message']}" if row["test"]
                        else f"{row['package']}: {row['message']}")
                lines.append(annotation(row["file"], row["line"], what))
            halted = interrupted or any(result.status() in HALTED for result in results)
            failed = bool(never) or any(result.status() == "fail" for result in results)
            code = 124 if halted else 1 if failed else 0
            lines.append(f"exit: {code}")
            text = "\n".join(lines) + "\n"
            sys.stdout.write(text)
            sys.stdout.flush()
            self.write_step_summary(results, never, interrupted)
            if self.stream:
                self.stream.write(json.dumps(
                    {"record": "summary", "module": self.module,
                     "shard": {"index": self.shard[0], "total": self.shard[1]},
                     "packages": len(self.planned), "started": len(results),
                     "completed": sum(1 for result in results if result.finished),
                     "never_started": never, "results": counts, "exit": code,
                     "interrupted": interrupted, "failure_count": len(rows),
                     "hangs": [result.record() for result in results
                               if result.status() in HALTED]},
                    sort_keys=True, default=str) + "\n")
                self.stream.flush()
                self.stream.close()
                self.stream = None
            if self.events:
                self.events.flush()
                self.events.close()
                self.events = None
            return code


class PackageRun(OwnedCommand):
    """One live package invocation: its process tree, its pipe and its records."""

    def __init__(self, process, result, report):
        super().__init__(process)
        self.result = result
        self.report = report
        self.done = threading.Event()
        self.eof_seen = False
        self.exited_at = None
        self.abandon_at = None
        self.sampled_at = 0.0

    def sample_descendants(self):
        now = time.monotonic()
        if now - self.sampled_at < DESCENDANT_SAMPLE:
            return
        self.sampled_at = now
        self.tree.sample()

    def leader_exited_for(self):
        """Seconds since the leader was first seen gone, or None while it runs."""
        if self.process.poll() is None:
            self.exited_at = None
            return None
        now = time.monotonic()
        if self.exited_at is None:
            self.exited_at = now
        return now - self.exited_at


def _watchdog(run):
    """Name and stop one package whose go test cannot finish."""
    if run.done.is_set() or run.stopped:
        return
    result = run.result
    if run.process.poll() is None:
        result.watchdog = True
        run.report.note(f"{result.package}: still running after {GO_PACKAGE_BACKSTOP}s; "
                        f"terminating that package only")
    elif not run.eof_seen:
        # The leader exited but its stdout never reached EOF: a descendant it
        # started still owns the pipe, so this package cannot be called finished.
        result.pipe_hold = True
        run.report.note(f"{result.package}: go test exited but a descendant still holds "
                        f"its output pipe after {GO_PACKAGE_BACKSTOP}s; terminating that "
                        f"package only")
    else:
        result.watchdog = True
        run.report.note(f"{result.package}: still finishing after {GO_PACKAGE_BACKSTOP}s; "
                        f"terminating that package only")
    run.stop()


_EOF = object()


def _drain(stream, sink):
    """Read one package's output into the queue, and always publish EOF."""
    try:
        for line in stream:
            sink.put(line)
    except (OSError, ValueError):
        pass
    finally:
        sink.put(_EOF)


def _lines(sink, run, report):
    """Yield one package's output lines until its pipe closes.

    A leader that exits while a descendant keeps the inherited stdout open would
    otherwise block this loop forever. An idle pipe is therefore checked: once
    the leader is gone and no output arrives for PIPE_HOLD_GRACE, the remaining
    processes are stopped and the package is named. If the pipe survives even
    the KILL (a process outside the owned tree), it is abandoned after a bounded
    wait so the lane, and the run, still continues.
    """
    while True:
        try:
            line = sink.get(timeout=PIPE_POLL)
        except queue.Empty:
            # Keep the descendant snapshot current while the leader can be traced.
            run.sample_descendants()
            if run.stopped:
                if run.abandon_at is None:
                    run.abandon_at = time.monotonic() + 2 * TERMINATE_GRACE
                elif time.monotonic() >= run.abandon_at:
                    report.note(f"{run.result.package}: output pipe still open after the "
                                f"package's processes were stopped; abandoning it so the "
                                f"run can continue")
                    return
                continue
            gone_for = run.leader_exited_for()
            if gone_for is not None and gone_for >= PIPE_HOLD_GRACE:
                run.result.pipe_hold = True
                report.note(f"{run.result.package}: go test exited but a process it started "
                            f"still holds its output pipe after {PIPE_HOLD_GRACE}s; "
                            f"terminating that package only")
                run.stop()
                run.abandon_at = time.monotonic() + 2 * TERMINATE_GRACE
            continue
        if line is _EOF:
            run.eof_seen = True
            return
        yield line


def execute_package(module, result, report):
    argv = ["go", "test", "-json", "-count=1", f"-timeout={GO_TEST_TIMEOUT}s"]
    if result.pattern:
        argv += ["-run", result.pattern]
    argv.append(result.package)
    report.package_start(result)
    report.write("+ " + " ".join(argv) + "\n")
    process = subprocess.Popen(argv, cwd=ROOT / module, start_new_session=True,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                               text=True, errors="replace", bufsize=1)
    run = _register_command(PackageRun(process, result, report))
    watchdog = threading.Timer(GO_PACKAGE_BACKSTOP, _watchdog, (run,))
    watchdog.daemon = True
    watchdog.start()
    sink = queue.Queue()
    reader = threading.Thread(target=_drain, args=(process.stdout, sink),
                              name=f"go-pipe-{result.package}", daemon=True)
    reader.start()
    try:
        for line in _lines(sink, run, report):
            text, event = decode(line)
            if isinstance(event, dict):
                # The compact record and console get decoded text; the complete
                # event goes to its own JSONL so the console is plain go output.
                report.raw_event(result, event)
                if event.get("Action") == "output" and isinstance(event.get("Output"), str):
                    report.write(event["Output"])
            else:
                report.raw_event(result, line=text)
                if text:
                    report.write(text + "\n")
            feed(result, line, report, event)
        result.exit = process.wait()
    finally:
        watchdog.cancel()
        run.done.set()
        _unregister_command(run)
        if process.poll() is None:
            _stop_group(process, run.tree)
        # Only a reader that reached EOF may have its pipe closed; closing a pipe
        # another thread is blocked in could wait forever. An abandoned reader is
        # a daemon left with one descriptor until this process exits.
        reader.join(TERMINATE_GRACE)
        if not reader.is_alive():
            process.stdout.close()
        # The leader is gone; anything still in an owned group is a process the
        # package leaked. It cannot hold this lane any more, but it must not
        # outlive the package either.
        if _groups_alive(run.tree.groups):
            report.note(f"{result.package}: terminating processes left behind after "
                        f"go test exited")
            _stop_group(process, run.tree)
    if result.timed_out or result.watchdog or result.pipe_hold:
        # Name the tests that never got a result instead of calling the hang
        # "the package" and leaving its inventory unknown.
        result.declared = declared_tests(module, result.package, report)
        result.inventory = "unavailable" if result.declared is None else "reported"
        result.selected = selected_names(result.pattern)
        wanted = [name for name in result.declared or []
                  if result.selected is None or name in result.selected]
        result.expected = len(wanted)
        result.never_ran = [name for name in wanted
                            if result.tests.get(name) not in {"pass", "fail", "skip"}]
    result.finished = True


_ACTIVE_REPORT = None
_SHUTTING_DOWN = False


def _interrupt(signum, frame):
    """Stop every owned command before exiting, keeping every record gathered."""
    global _SHUTTING_DOWN
    if _SHUTTING_DOWN:
        # A repeated signal must not wait for the first shutdown's bounded grace.
        os._exit(128 + signum)
    _SHUTTING_DOWN = True
    report = _ACTIVE_REPORT
    commands = _active_commands()
    try:
        if report is not None:
            report.write(f"\n!! {signal.Signals(signum).name}: interrupted; stopping "
                         f"{len(commands)} active command(s)\n")
        # Name the lanes before stopping them: a lane that finishes while the
        # groups are being torn down must not be published as a real result.
        for command in commands:
            result = command.result
            if result is None or result.finished:
                continue
            with result.lock:
                result.interrupted = True
        _stop_all(commands)
        if report is not None:
            for command in commands:
                if command.result is not None:
                    report.add(command.result)
            report.summary(interrupted=True)
    finally:
        os._exit(128 + signum)


def print_tool_versions(report):
    """Record the actual go and node builds running this entry log."""
    for name, argv in (("node", ["node", "--version"]), ("go", ["go", "version"])):
        try:
            text = run_capture(argv, ROOT, VERSION_TIMEOUT).strip()
        except (OSError, subprocess.CalledProcessError, CommandTimeout) as error:
            report.note(f"{name} version unavailable: {error}")
            continue
        report.write(f"{name} version: {text.splitlines()[0] if text else '(no output)'}\n")


def go_test(module, selection, shard_index=0, shard_total=1):
    """Run every selected package alone, bounded, and keep the others going."""
    global _ACTIVE_REPORT, _SHUTTING_DOWN
    report = GoReport(module, [package for package, _ in selection], shard_index, shard_total)
    previous = {number: signal.signal(number, _interrupt)
                for number in (signal.SIGTERM, signal.SIGINT)}
    _ACTIVE_REPORT = report
    _SHUTTING_DOWN = False
    try:
        print_tool_versions(report)
        limit_cores(report)
        report.note(f"{len(selection)} package(s) in this shard; {GO_PACKAGE_JOBS} lane(s); "
                    f"package deadline {GO_TEST_TIMEOUT}s; watchdog {GO_PACKAGE_BACKSTOP}s")
        queue_ = list(selection)
        lock = threading.Lock()

        def worker():
            while True:
                # A signal stops the run from starting anything new; the handler
                # owns the shutdown from here.
                if _SHUTTING_DOWN:
                    return
                with lock:
                    if _SHUTTING_DOWN or not queue_:
                        return
                    package, pattern = queue_.pop(0)
                result = PackageResult(package, pattern, module)
                try:
                    execute_package(module, result, report)
                except Exception as error:  # one broken lane must not drop the others
                    report.note(f"{result.package}: could not be run: {error}")
                    if result.exit is None:
                        result.exit = 1
                report.add(result)

        threads = [threading.Thread(target=worker, name=f"go-package-{number}", daemon=True)
                   for number in range(max(1, min(GO_PACKAGE_JOBS, len(selection))))]
        for thread in threads:
            thread.start()
        for thread in threads:
            while thread.is_alive():
                thread.join(0.2)
        return report.summary()
    finally:
        _ACTIVE_REPORT = None
        for number, handler in previous.items():
            signal.signal(number, handler)


def finish(test_status, vet_status):
    status = 124 if 124 in (test_status, vet_status) else 1 if test_status or vet_status else 0
    if status:
        raise SystemExit(status)


def go_checks(module, group):
    if os.environ.get("CI_FULL_TREE") == "1" and group == "affected":
        shard_index, shard_total = shard_settings()
        planned = module_packages(module)
        selection = [(package, None) for package in shard(planned, shard_index, shard_total)]
        print(f"+ go test inventory: {len(planned)} packages, shard "
              f"{shard_index + 1}/{shard_total} runs {len(selection)}", flush=True)
        vet = (["bash", str(ROOT / "scripts/vet-go.sh")] if module == "server"
               else ["go", "vet", "./..."])
        finish(go_test(module, selection, shard_index, shard_total),
               run_all_status([(vet, ROOT / module)]))
        return
    if group == "affected":
        selected = go_plan(module, changed())
    elif group == "auth":
        selected = {"./internal/httpapi": {
            "TestRefreshRoundTripOverPlainHTTP",
            "TestUnknownAndKnownNamesHaveTheSameSignInCooloff",
            "TestAccountRestoreUsesAccountScopeUntilExplicitProfileSelection",
            "TestSessionFamilyHTTPLogoutAuthenticGenerations",
            "TestASecondProfileOfTheOwnersAccountIsNotAnAdministrator",
            "TestRestrictedProfileRouteMatrix", "TestRestrictedProfileWriteRouteMatrix",
        }}
    elif group == "media":
        selected = {
            "./internal/mediaexec": test_names((ROOT / module / "internal/mediaexec").glob("*_test.go")),
            "./internal/remotemedia": {
                "TestPrivateRootsRedirectsAndMetadataAreExplicitlyFenced",
                "TestDNSChangesAreRevalidatedAgainstApprovedAddresses",
                "TestManualRedirectsReturnEachHopWithoutContactingDestination",
                "TestPolicyProxyRejectsRebindingAndUnauthenticatedRequests",
                "TestServeEndpointRejectsMembersBeforeDispatch",
                "TestServeDialChecksExactOriginAndReconnectBeforeNetwork",
            },
        }
    elif group == "hosted":
        selected = {
            "./internal/identity": test_names((ROOT / module / "internal/identity").glob("*_test.go")),
            "./internal/egress": test_names((ROOT / module / "internal/egress").glob("*_test.go")),
            "./internal/security": test_names((ROOT / module / "internal/security").glob("*_test.go")),
            "./internal/httpapi": {
                "TestBrowserCookieOriginsAndFamilyIntentAreIsolated",
                "TestIdentityContinuationReadIsNotMediaOriginAuthority",
                "TestSessionReviewQueryAllowlist",
            },
        }
    else:
        raise ValueError(group)
    selection = []
    vet_commands = []
    for package, names in sorted(selected.items()):
        if group != "affected":
            actual = test_names((ROOT / module / package.removeprefix("./")).glob("*_test.go"))
            if not names or not names <= actual:
                raise RuntimeError(f"missing mandatory security test: {package}: {names - actual}")
        vet = ["go", "vet"]
        if package == "./internal/thirdparty/sqlite":
            vet.append("-unsafeptr=false")  # same maintained-driver exception as vet-go.sh
        vet_commands.append(([*vet, package], ROOT / module))
        if package == "./...":
            # A dependency change compiles the whole module graph, still one
            # package per bounded run so a hang cannot cancel the rest.
            selection.extend((nested, None) for nested in module_packages(module))
            continue
        pattern = "^(" + "|".join(sorted(names)) + ")$" if names else "^$"
        selection.append((package, pattern))
    finish(go_test(module, selection), run_all_status(vet_commands))


def module_tests(module):
    """The complete module inventory, one bounded package per invocation.

    This is the nightly counterpart of a full `go test ./...`: every package and
    every test still runs, exactly once, but a hung or broken package is named
    with the tests it never reported instead of cancelling the rest. Vet stays
    the nightly workflow's separate step, so this runs tests only.
    """
    selection = [(package, None) for package in module_packages(module)]
    status = go_test(module, selection)
    if status:
        raise SystemExit(status)


def dependencies(file, known, workspaces):
    result = set()
    # Include imports, requires, new URL(..., import.meta.url), and bounded harness module maps.
    for spec in re.findall(r"['\"]([^'\"\n]+)['\"]", file.read_text(errors="replace")):
        if spec.startswith("."):
            base = file.parent / spec
        elif spec.startswith("@core/"):
            base = ROOT / "packages/client-core/src" / spec[6:]
        elif spec in {"@core", "@design", "@i18n"}:
            package = {"@core": "client-core", "@design": "design", "@i18n": "i18n"}[spec]
            base = ROOT / "packages" / package / "src/index.ts"
        elif spec.startswith("@/") and file.is_relative_to(ROOT / "web-next"):
            base = ROOT / "web-next/src" / spec[2:]
        else:
            workspace = next((name for name in workspaces if spec == name or spec.startswith(name + "/")), None)
            if not workspace:
                continue
            directory = workspaces[workspace]
            exports = json.loads((directory / "package.json").read_text()).get("exports", {})
            key = "." + spec[len(workspace):]
            target = exports if isinstance(exports, str) and key == "." else exports.get(key) if isinstance(exports, dict) else None
            if isinstance(target, dict):
                target = target.get("import") or target.get("default") or target.get("types")
            base = directory / (target if isinstance(target, str) else spec[len(workspace):].lstrip("/") or "src/index")
        base = base.resolve()
        candidates = [base, *(Path(str(base) + ext) for ext in (".ts", ".tsx", ".js", ".mjs", ".cjs")),
                      *(base / ("index" + ext) for ext in (".ts", ".tsx", ".js", ".mjs"))]
        if base.suffix in {".js", ".jsx"}:
            candidates.extend([base.with_suffix(".ts"), base.with_suffix(".tsx")])
        result.update(candidate for candidate in candidates if candidate in known)
    return result


def js_plan(paths, native=False):
    files = {ROOT / name for name in git("ls-files")
             if Path(name).suffix in {".ts", ".tsx", ".js", ".mjs", ".cjs", ".json"}
             and (ROOT / name).is_file()}
    workspaces = {}
    for file in files:
        if file.name == "package.json":
            name = json.loads(file.read_text()).get("name")
            if name:
                workspaces[name] = file.parent
    impacted = {ROOT / name for name in paths}
    for name in paths:
        file = ROOT / name
        if file.name in {"package.json", "package-lock.json", "register.mjs"}:
            directory = file.parent
            impacted.update(p for p in files if p.is_relative_to(directory))
    edges = {file: dependencies(file, files, workspaces) for file in files if file.suffix != ".json"}
    while True:
        more = {file for file, deps in edges.items() if deps & impacted} - impacted
        if not more:
            break
        impacted.update(more)
    tests = {p for p in impacted if re.search(r"\.test\.(?:[cm]?js|tsx?)$", str(p)) and p.is_file()}
    mandatory = (["apps/native/test/next-accounts-entry.test.ts", "apps/native/test/next-local-session-renewal.test.ts",
                  "apps/native/test/next-account-proof-entry.test.ts", "apps/native/test/next-shell-tv-profile-menu.test.ts"] if native else [
        "packages/client-core/test/account-sessions.test.ts", "packages/client-core/test/session-refresh.test.ts",
        "packages/client-core/test/identity-restrictions.test.ts", "packages/client-core/test/profiles-membership.test.ts",
        "packages/client-core/test/generated-routes.test.ts", "packages/client-core/test/credential-scoped-logout.test.ts",
        "web-next/test/account-entry-authority.test.ts", "web-next/test/viewer-storage.test.ts",
    ])
    for name in mandatory:
        if not (ROOT / name).is_file():
            raise RuntimeError("missing mandatory security test: " + name)
        tests.add(ROOT / name)
    return sorted(tests)


def js_checks(native):
    paths = changed()
    groups = {}
    for file in js_plan(paths, native):
        relative = file.relative_to(ROOT)
        if native and relative.parts[:3] == ("apps", "native", "test"):
            group = "apps/native"
        elif str(relative).startswith("web-next/test/"):
            group = "web-next"
        elif str(relative).startswith("web/test/"):
            group = "web"
        elif str(relative).startswith("packages/"):
            group = "/".join(relative.parts[:2])
        elif str(relative).startswith("server/receiver/cast/test/"):
            group = "."
        else:
            continue
        groups.setdefault(group, []).append(str(file.relative_to(ROOT / group)))
    commands = []
    if native:
        source = [str(Path(p).relative_to("apps/native")) for p in sorted(paths)
                  if p.startswith("apps/native/") and p.endswith((".ts", ".tsx"))
                  and (ROOT / p).is_file() and (p.startswith("apps/native/src-next/") or p == "apps/native/App.tsx")]
        if source:
            commands.append((["npx", "--no-install", "eslint", "--no-eslintrc", "-c", ".eslintrc.hooks.cjs", *source], ROOT / "apps/native"))
    for group, files in sorted(groups.items()):
        args = ["node", "--experimental-strip-types"]
        if group in ("apps/native", "web", "web-next"):
            args = ["node", "--experimental-transform-types", "--import", "./test/register.mjs"]
        commands.append(([*args, "--test-concurrency=1", "--test", *files], ROOT / group))
    run_all(commands)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("kind", choices=["go", "go-module", "js", "native-js"])
    parser.add_argument("--module", default="server")
    parser.add_argument("--group", choices=["affected", "auth", "media", "hosted"], default="affected")
    options = parser.parse_args()
    if options.kind == "go":
        go_checks(options.module, options.group)
    elif options.kind == "go-module":
        module_tests(options.module)
    else:
        js_checks(options.kind == "native-js")
