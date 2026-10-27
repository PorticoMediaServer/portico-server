#!/usr/bin/env python3
"""Full hourly checks, with bounded selection retained for explicit local use."""
import argparse
import json
import os
from pathlib import Path
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

# Per-command bound for everything else. This is a hang guard for one suite
# invocation, not a scope filter: the mandatory security commands stay exactly
# as selected, and each still gets the whole window.
COMMAND_TIMEOUT = 15 * 60
TERMINATE_GRACE = 5
TAIL = 6


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


def _signal_group(process, sig):
    try:
        os.killpg(process.pid, sig)
    except ProcessLookupError:
        pass


def _group_alive(process):
    try:
        os.killpg(process.pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def _await_group(process, seconds):
    """Wait up to `seconds` for the whole command group to empty, reaping its leader."""
    deadline = time.monotonic() + seconds
    while _group_alive(process):
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            return False
        process.poll()  # reap the leader as soon as it exits
        time.sleep(min(0.05, remaining))
    process.wait()
    return True


def _stop_group(process):
    """TERM the command's own group, give it a bounded grace, then KILL what remains."""
    _signal_group(process, signal.SIGTERM)
    _await_group(process, TERMINATE_GRACE)
    # The grace is for the group, not only its leader: a leader that exits on TERM
    # must not hide a descendant that ignores it, so the group is always confirmed
    # with KILL. Both waits stay bounded, and a command that cannot be killed is
    # reported by the failing timeout rather than hanging this runner again.
    _signal_group(process, signal.SIGKILL)
    _await_group(process, TERMINATE_GRACE)


def run(args, cwd=ROOT, timeout=None, stdout=None):
    argv = list(map(str, args))
    limit = COMMAND_TIMEOUT if timeout is None else timeout
    print("+", " ".join(argv), flush=True)
    # A captured command keeps its output for parsing; the caller echoes what
    # matters, so a failing `go list` is never silently swallowed.
    process = subprocess.Popen(argv, cwd=cwd, start_new_session=True, stdout=stdout,
                               stderr=subprocess.STDOUT if stdout is not None else None)
    try:
        code = process.wait(timeout=limit)
    except subprocess.TimeoutExpired:
        _stop_group(process)
        print(f"timed out after {limit:g}s, command terminated: {' '.join(argv)}",
              file=sys.stderr, flush=True)
        raise CommandTimeout(argv, limit) from None
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


class PackageResult:
    """One package's own go test invocation, including what it never reported."""

    def __init__(self, package, pattern):
        self.package = package
        self.pattern = pattern
        self.action = None       # go test's terminal action for the package
        self.elapsed = 0.0
        self.tests = {}          # test name -> last action (run/pass/fail/skip)
        self.timed_out = False   # go test's own -timeout fired inside the package
        self.watchdog = False    # the outer watchdog terminated this package
        self.exit = None
        self.finished = False
        self.tail = []
        self.declared = None     # declared test names, collected after a hang
        self.inventory = None    # "reported", "unavailable" or None
        self.expected = 0        # declared or selected tests the run owed a result
        self.selected = None     # names from the -run pattern, None for the whole package
        self.never_ran = []      # expected tests with no pass/fail/skip result

    def status(self):
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

    def record(self):
        return {"record": "package", "package": self.package, "status": self.status(),
                "exit": self.exit, "elapsed": round(self.elapsed, 3), "pattern": self.pattern,
                "timeout": self.timed_out, "watchdog": self.watchdog,
                "tests": self.tests, "failed": self.failed(), "unfinished": self.unfinished(),
                "inventory": self.inventory, "declared": self.declared,
                "selected": sorted(self.selected) if self.selected is not None else None,
                "expected": self.expected, "never_ran": self.never_ran}


def describe(result):
    status = result.status()
    parts = [status.upper(), result.package, f"tests={len(result.tests)}", f"{result.elapsed:.1f}s"]
    if result.failed():
        parts.append("failed " + ", ".join(result.failed()))
    if status in {"timeout", "watchdog", "incomplete"}:
        hung = result.unfinished()
        parts.append("hung " + (", ".join(hung) if hung else "no running test reported"))
        if result.inventory == "reported":
            label = "declared" if result.selected is None else "selected"
            parts.append("no result for " + (", ".join(result.never_ran) or "no declared test")
                         + f" ({len(result.never_ran)} of {result.expected} {label})")
        elif result.inventory == "unavailable":
            parts.append("test inventory unavailable")
    return "  ".join(parts)


def feed(result, line, report):
    """Merge one go test -json event into the package result and the record."""
    text = line.rstrip("\n")
    event = None
    if text.startswith("{"):
        try:
            event = json.loads(text)
        except json.JSONDecodeError:
            event = None
    if not isinstance(event, dict):
        if text:
            result.tail = (result.tail + [text])[-TAIL:]
        if "panic: test timed out" in text:
            result.timed_out = True
        return
    action = event.get("Action", "")
    if action == "output":
        output = event.get("Output", "")
        if "panic: test timed out" in output:
            result.timed_out = True
        result.tail = (result.tail + [part for part in output.splitlines() if part])[-TAIL:]
        return
    if action == "build-fail":
        result.action = "fail"
        return
    name = event.get("Test")
    if name:
        if action in {"run", "pause", "cont"}:
            result.tests[name] = "run"
        elif action in {"pass", "fail", "skip"}:
            result.tests[name] = action
        if action in {"run", "pass", "fail", "skip"}:
            report.test_event(result, action, name, event.get("Elapsed"))
        return
    if action in {"pass", "fail", "skip"}:
        result.action = action
        result.elapsed = float(event.get("Elapsed") or 0.0)


class GoReport:
    """Live package results, a flushed JSONL record, and a closing summary."""

    def __init__(self, module, planned, shard_index, shard_total):
        self.module = module
        self.planned = list(planned)
        self.shard = (shard_index, shard_total)
        self.lock = threading.Lock()
        self.results = {}
        self.stream = None
        path = None
        try:
            path = report_path(module, shard_index, shard_total)
            if path:
                self.stream = open(path, "w", encoding="utf-8")
        except OSError as error:
            # A broken report location must not cost the run its console record.
            self.write(f"!! {module}: structured record unavailable ({path}): {error}\n")
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
        with self.lock:
            self.results[result.package] = result
            sys.stdout.write(describe(result) + "\n")
            sys.stdout.flush()
            self._append(result.record())

    def summary(self, interrupted=False):
        with self.lock:
            results = list(self.results.values())
            counts = {}
            for result in results:
                counts[result.status()] = counts.get(result.status(), 0) + 1
            never = [package for package in self.planned if package not in self.results]
            lines = [f"==== go test summary: {self.module} shard "
                     f"{self.shard[0] + 1}/{self.shard[1]} ====",
                     f"packages: {len(self.planned)} planned, {len(results)} started, "
                     f"{sum(1 for result in results if result.finished)} completed, "
                     f"{len(never)} never started",
                     "results: " + (", ".join(f"{name} {counts[name]}"
                                              for name in sorted(counts)) or "none")]
            for result in results:
                if result.status() in {"pass", "skip"}:
                    continue
                lines.append(describe(result))
                if result.status() in {"timeout", "watchdog", "incomplete"}:
                    lines.extend("    " + line for line in result.tail[-TAIL:])
            # A package with no record after the plan record was never started.
            for package in never:
                lines.append(f"NEVER RAN  {package}  no result  never started")
            halted = interrupted or any(result.timed_out or result.watchdog
                                        or result.status() == "incomplete" for result in results)
            failed = bool(never) or any(result.status() == "fail" for result in results)
            code = 124 if halted else 1 if failed else 0
            lines.append(f"exit: {code}")
            text = "\n".join(lines) + "\n"
            sys.stdout.write(text)
            sys.stdout.flush()
            if self.stream:
                self.stream.write(json.dumps(
                    {"record": "summary", "module": self.module,
                     "shard": {"index": self.shard[0], "total": self.shard[1]},
                     "packages": len(self.planned), "started": len(results),
                     "completed": sum(1 for result in results if result.finished),
                     "never_started": never, "results": counts, "exit": code,
                     "hangs": [result.record() for result in results
                               if result.status() in {"timeout", "watchdog", "incomplete"}]},
                    sort_keys=True, default=str) + "\n")
                self.stream.flush()
                self.stream.close()
                self.stream = None
            return code


def _watchdog(process, result, report):
    if process.poll() is not None:
        return
    result.watchdog = True
    report.note(f"{result.package}: still running after {GO_PACKAGE_BACKSTOP}s; "
                f"terminating that package only")
    _stop_group(process)


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
    watchdog = threading.Timer(GO_PACKAGE_BACKSTOP, _watchdog, (process, result, report))
    watchdog.daemon = True
    watchdog.start()
    try:
        for line in process.stdout:
            report.write(line)
            feed(result, line, report)
        result.exit = process.wait()
    finally:
        watchdog.cancel()
        if process.poll() is None:
            _stop_group(process)
        process.stdout.close()
    if result.watchdog:
        _stop_group(process)  # never leave a witness process behind
    if result.timed_out or result.watchdog:
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


def _interrupt(signum, frame):
    report = _ACTIVE_REPORT
    if report is not None:
        report.write(f"\n!! {signal.Signals(signum).name}: interrupted; "
                     f"every result gathered so far follows\n")
        report.summary(interrupted=True)
    os._exit(128 + signum)


def go_test(module, selection, shard_index=0, shard_total=1):
    """Run every selected package alone, bounded, and keep the others going."""
    global _ACTIVE_REPORT
    report = GoReport(module, [package for package, _ in selection], shard_index, shard_total)
    previous = {number: signal.signal(number, _interrupt)
                for number in (signal.SIGTERM, signal.SIGINT)}
    _ACTIVE_REPORT = report
    try:
        limit_cores(report)
        report.note(f"{len(selection)} package(s) in this shard; {GO_PACKAGE_JOBS} lane(s); "
                    f"package deadline {GO_TEST_TIMEOUT}s; watchdog {GO_PACKAGE_BACKSTOP}s")
        queue = list(selection)
        lock = threading.Lock()

        def worker():
            while True:
                with lock:
                    if not queue:
                        return
                    package, pattern = queue.pop(0)
                result = PackageResult(package, pattern)
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
    parser.add_argument("kind", choices=["go", "js", "native-js"])
    parser.add_argument("--module", default="server")
    parser.add_argument("--group", choices=["affected", "auth", "media", "hosted"], default="affected")
    options = parser.parse_args()
    if options.kind == "go":
        go_checks(options.module, options.group)
    else:
        js_checks(options.kind == "native-js")
