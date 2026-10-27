import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("affected", Path(__file__).with_name("ci-affected.py"))
ci = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ci)


@contextlib.contextmanager
def time_limit(seconds=30):
    """Fail a test instead of hanging it when a runner bound regresses."""
    def expired(signum, frame):
        raise AssertionError(f"the runner did not finish within {seconds}s")

    previous = signal.signal(signal.SIGALRM, expired)
    signal.setitimer(signal.ITIMER_REAL, seconds)
    try:
        yield
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        signal.signal(signal.SIGALRM, previous)


class SelectionTests(unittest.TestCase):
    def setUp(self):
        env = patch.dict(ci.os.environ, {"CI_FULL_TREE": "0"})
        env.start()
        self.addCleanup(env.stop)
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.root_patch = patch.object(ci, "ROOT", self.root)
        self.root_patch.start()
        self.addCleanup(self.root_patch.stop)

    def file(self, path, text):
        p = self.root / path
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(text)
        return p

    def test_full_tree_does_not_depend_on_cancelled_push_history(self):
        with patch.dict(ci.os.environ, {"CI_FULL_TREE": "1", "CI_BASE": "missing-commit"}), patch.object(ci, "git", return_value=["old/source.ts", "new/test.test.ts"]) as git:
            self.assertEqual(ci.changed(), {"old/source.ts", "new/test.test.ts"})
            git.assert_called_once_with("ls-files")

    def test_catalogue_uses_named_counterpart_never_full_suite(self):
        self.file("server/internal/catalog/movie.go", "package catalog")
        self.file("server/internal/catalog/movie_test.go", "func TestMovie(t *testing.T) {}")
        self.file("server/internal/catalog/slow_test.go", "func TestSlow(t *testing.T) {}")
        plan = ci.go_plan("server", {"server/internal/catalog/movie.go"})
        self.assertEqual(plan, {"./internal/catalog": {"TestMovie"}})

    def test_uncoupled_catalogue_source_is_compile_only_until_nightly(self):
        self.file("server/internal/catalog/other.go", "package catalog")
        self.file("server/internal/catalog/slow_test.go", "func TestSlow(t *testing.T) {}")
        self.assertEqual(ci.go_plan("server", {"server/internal/catalog/other.go"}), {"./internal/catalog": set()})

    def test_changed_test_keeps_all_named_assertions(self):
        self.file("server/internal/catalog/movie_test.go", "func TestFirst(t *testing.T) {}\nfunc TestSecond(t *testing.T) {}")
        self.assertEqual(ci.go_plan("server", {"server/internal/catalog/movie_test.go"}),
                         {"./internal/catalog": {"TestFirst", "TestSecond"}})

    def test_small_package_without_counterpart_runs_its_tests(self):
        self.file("server/internal/small/new.go", "package small")
        self.file("server/internal/small/old_test.go", "func TestExisting(t *testing.T) {}")
        self.assertEqual(ci.go_plan("server", {"server/internal/small/new.go"}),
                         {"./internal/small": {"TestExisting"}})

    def test_module_dependency_change_compiles_whole_module(self):
        self.assertEqual(ci.go_plan("server", {"server/go.sum"}), {"./...": set()})

    def test_import_dependency_resolves_ts_and_new_url(self):
        a = self.file("src/controller.ts", "export const value=1")
        b = self.file("test/helper.ts", "const file=new URL('../src/controller.ts', import.meta.url)")
        self.assertEqual(ci.dependencies(b, {a, b}, {}), {a})

    def test_workspace_subpath_uses_actual_declared_export(self):
        directory = self.root / 'packages/client-core'
        self.file('packages/client-core/package.json', '{"exports":{"./selection-model":"./src/selection-model/index.ts"}}')
        source = self.file('packages/client-core/src/selection-model/index.ts', 'export const value=1')
        caller = self.file('web-next/test/selection.test.ts', "import {value} from '@portico/client-core/selection-model'")
        self.assertEqual(ci.dependencies(caller, {source, caller}, {'@portico/client-core': directory}), {source})

    def test_declared_web_aliases_and_js_extension_reach_source(self):
        source = self.file('packages/client-core/src/index.ts', 'export const value=1')
        item = self.file('web-next/src/item.ts', 'export const item=1')
        caller = self.file('web-next/test/entry.test.ts', "import '@core'; import '@/item.js'")
        self.assertEqual(ci.dependencies(caller, {source, item, caller}, {}), {source, item})

    def test_transitive_imports_select_affected_test_not_unrelated(self):
        mandatory = ["next-accounts-entry", "next-local-session-renewal", "next-account-proof-entry", "next-shell-tv-profile-menu"]
        for name in mandatory:
            self.file("apps/native/test/" + name + ".test.ts", "// security fixture")
        self.file("apps/native/src-next/port.ts", "export const value=1")
        self.file("apps/native/test/helper.ts", "export {value} from '../src-next/port'")
        wanted = self.file("apps/native/test/changed.test.ts", "import './helper'")
        other = self.file("apps/native/test/unrelated.test.ts", "// no dependency")
        tracked = [str(p.relative_to(self.root)) for p in self.root.rglob('*') if p.is_file()]
        with patch.object(ci, "git", return_value=tracked):
            selected = ci.js_plan({"apps/native/src-next/port.ts"}, native=True)
        self.assertIn(wanted, selected)
        self.assertNotIn(other, selected)

    def test_failed_command_does_not_omit_later_security_commands(self):
        import subprocess
        with patch.object(ci, "run", side_effect=[subprocess.CalledProcessError(1, "first"), None]) as run:
            with self.assertRaises(SystemExit):
                ci.run_all([(["first"], self.root), (["security"], self.root)])
            self.assertEqual(run.call_count, 2)

    def test_deleted_test_still_compiles_surviving_package(self):
        self.file("server/internal/small/main.go", "package small")
        self.assertEqual(ci.go_plan("server", {"server/internal/small/deleted_test.go"}), {"./internal/small": set()})

    def test_missing_mandatory_security_file_fails_closed(self):
        with patch.object(ci, "git", return_value=[]):
            with self.assertRaisesRegex(RuntimeError, "missing mandatory security test"):
                ci.js_plan(set())

    def test_empty_push_base_uses_previous_commit_and_missing_history_is_not_ignored(self):
        with patch.dict(ci.os.environ, {"CI_BASE": "0" * 40}), patch.object(ci, "git", return_value=["src/a.ts"]) as git:
            self.assertEqual(ci.changed(), {"src/a.ts"})
            self.assertEqual(git.call_args.args[:4], ("diff", "--name-only", "--diff-filter=ACMRTD", "HEAD^"))


class ProcessChecks:
    """Assertions about processes left behind by a stopped command."""

    def assert_gone(self, pid):
        # A killed grandchild can stay a zombie until init reaps it; either state is gone.
        # /proc is the precise check on Linux; os.kill is the portable fallback.
        for _ in range(200):
            try:
                with open(f"/proc/{pid}/stat") as stat:
                    if stat.read().split()[2] == "Z":
                        return
            except OSError:
                pass
            try:
                os.kill(pid, 0)
            except ProcessLookupError:
                return
            time.sleep(0.05)
        self.fail(f"process {pid} outlived the command timeout")

    def assert_group_gone(self, pid):
        # A zombie left for init can keep the group listed for a moment after the KILL.
        for _ in range(100):
            try:
                os.killpg(pid, 0)
            except ProcessLookupError:
                return
            time.sleep(0.05)
        self.fail(f"process group {pid} outlived the command timeout")


class BoundedCommandTests(ProcessChecks, unittest.TestCase):
    """A hung suite fails with a named command instead of holding the whole job."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def test_timeout_terminates_the_whole_command_group(self):
        marker = self.root / "pids"
        script = (
            "import os, subprocess, sys, time\n"
            "child = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(120)'])\n"
            "with open(sys.argv[1], 'w') as pids:\n"
            "    pids.write(f'{os.getpid()} {child.pid}')\n"
            "time.sleep(120)\n"
        )
        started = time.monotonic()
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(ci.CommandTimeout) as raised:
                ci.run([sys.executable, "-c", script, str(marker)], cwd=self.root, timeout=0.5)
        elapsed = time.monotonic() - started
        self.assertEqual(raised.exception.returncode, 124)
        self.assertEqual(raised.exception.timeout, 0.5)
        self.assertLess(elapsed, ci.TERMINATE_GRACE + 10)
        leader, descendant = (int(pid) for pid in marker.read_text().split())
        for pid in (leader, descendant):
            self.assert_gone(pid)
        self.assert_group_gone(leader)

    def test_leader_exit_on_term_does_not_spare_a_terminating_descendant(self):
        marker = self.root / "pids"
        script = (
            "import os, subprocess, sys, time\n"
            "child = subprocess.Popen([sys.executable, '-c',\n"
            "    'import signal, time; signal.signal(signal.SIGTERM, signal.SIG_IGN); time.sleep(120)'])\n"
            "with open(sys.argv[1], 'w') as pids:\n"
            "    pids.write(f'{os.getpid()} {child.pid}')\n"
            "time.sleep(120)\n"
        )
        started = time.monotonic()
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(ci.CommandTimeout) as raised:
                ci.run([sys.executable, "-c", script, str(marker)], cwd=self.root, timeout=0.5)
        elapsed = time.monotonic() - started
        self.assertEqual(raised.exception.returncode, 124)
        leader, descendant = (int(pid) for pid in marker.read_text().split())
        # The leader takes TERM, so only it is reaped during the grace; the descendant
        # ignores TERM, so that grace must still run to its bound and end in a group KILL.
        self.assertGreaterEqual(elapsed, ci.TERMINATE_GRACE - 1)
        self.assertLess(elapsed, 2 * ci.TERMINATE_GRACE + 5)
        for pid in (leader, descendant):
            self.assert_gone(pid)
        self.assert_group_gone(leader)

    def test_successful_and_failing_commands_keep_their_semantics(self):
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(ci.run([sys.executable, "-c", "pass"], cwd=self.root), 0)
            with self.assertRaises(subprocess.CalledProcessError) as raised:
                ci.run([sys.executable, "-c", "raise SystemExit(3)"], cwd=self.root)
        self.assertEqual(raised.exception.returncode, 3)

    def test_timeout_fails_the_run_but_later_commands_still_run(self):
        commands = [(["slow"], self.root), (["failing"], self.root), (["security"], self.root)]
        failures = [ci.CommandTimeout(["slow"], 1), subprocess.CalledProcessError(1, "failing"), None]
        with patch.object(ci, "run", side_effect=failures) as run:
            with self.assertRaises(SystemExit) as raised:
                ci.run_all(commands)
        self.assertEqual(raised.exception.code, 124)
        self.assertEqual([call.args[0] for call in run.call_args_list], [args for args, _ in commands])

    def test_only_an_ordinary_failure_still_exits_one(self):
        with patch.object(ci, "run", side_effect=subprocess.CalledProcessError(2, "fail")):
            with self.assertRaises(SystemExit) as raised:
                ci.run_all([(["fail"], self.root)])
        self.assertEqual(raised.exception.code, 1)

    def test_the_bound_is_fifteen_minutes_with_a_five_second_grace(self):
        self.assertEqual((ci.COMMAND_TIMEOUT, ci.TERMINATE_GRACE), (15 * 60, 5))


FAKE_GO = '''#!/usr/bin/env python3
import json, os, subprocess, sys, time

PACKAGES = {packages!r}
MODES = {modes!r}
DECLARED = dict([
    ("pass", ["TestGood"]),
    ("fail", ["TestBad"]),
    ("deadline", ["TestStuck", "TestAfter"]),
    ("deadline-nolist", ["TestStuck", "TestAfter"]),
    ("hang", ["TestHangs", "TestNotRun"]),
    ("hang-child", ["TestHangs", "TestAfter"]),
    ("leak", ["TestLeak", "TestAfter"]),
])
args = sys.argv[1:]
with open(os.environ["FAKE_ARGV"], "a", encoding="utf-8") as log:
    log.write(" ".join(args) + "\\n")
if args[0] == "list":
    for name in PACKAGES:
        print("example.com/m/" + name + "\\t" + os.path.join(os.getcwd(), name))
    raise SystemExit(0)
assert args[0] == "test", args
package = args[-1].rsplit("/", 1)[-1]
mode = MODES[package]
if "-list" in args:
    if mode == "deadline-nolist":
        print("cannot list declared tests", file=sys.stderr)
        raise SystemExit(2)
    for name in DECLARED.get(mode, []):
        print(name)
    raise SystemExit(0)


def emit(**event):
    print(json.dumps(dict(Package="example.com/m/" + package, **event)), flush=True)


def leak_child():
    # Inherits this process's stdout, exactly like a helper a test forgot to reap.
    child = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(120)"])
    with open(os.path.join(os.environ["PID_DIR"], package), "w") as pids:
        pids.write(str(os.getpid()) + " " + str(child.pid))
    return child


emit(Action="start")
if mode == "pass":
    emit(Action="run", Test="TestGood")
    emit(Action="pass", Test="TestGood", Elapsed=0.01)
    emit(Action="pass", Elapsed=0.02)
elif mode == "fail":
    emit(Action="run", Test="TestBad")
    emit(Action="fail", Test="TestBad", Elapsed=0.01)
    emit(Action="fail", Elapsed=0.02)
    raise SystemExit(1)
elif mode in ("deadline", "deadline-nolist"):
    emit(Action="run", Test="TestStuck")
    emit(Action="output", Test="TestStuck", Output="panic: test timed out after 13m0s\\n")
    emit(Action="fail", Elapsed=780.0)
    raise SystemExit(1)
elif mode == "leak":
    emit(Action="run", Test="TestLeak")
    leak_child()
    # The leader is gone while its child still owns the inherited stdout.
    raise SystemExit(0)
elif mode == "hang-child":
    emit(Action="run", Test="TestHangs")
    leak_child()
    time.sleep(120)
elif mode == "hang":
    emit(Action="run", Test="TestHangs")
    time.sleep(120)
else:
    emit(Action="fail", Elapsed=0.0)
    raise SystemExit(1)
'''


class GoPackageRunnerTests(ProcessChecks, unittest.TestCase):
    """One bounded `go test -json` per package, so a hang names its package only."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "repo"
        (self.root / "server").mkdir(parents=True)
        (self.root / "pids").mkdir()
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.argv = self.root / "argv.log"
        self.argv.write_text("")
        environment = {"PATH": str(self.bin) + os.pathsep + os.environ.get("PATH", ""),
                       "FAKE_ARGV": str(self.argv), "PID_DIR": str(self.root / "pids")}
        environ = patch.dict(ci.os.environ, environment, clear=True)
        environ.start()
        self.addCleanup(environ.stop)
        root = patch.object(ci, "ROOT", self.root)
        root.start()
        self.addCleanup(root.stop)
        lanes = patch.object(ci, "GO_PACKAGE_JOBS", 1)
        lanes.start()
        self.addCleanup(lanes.stop)

    def install_go(self, packages, modes):
        script = self.bin / "go"
        script.write_text(FAKE_GO.format(packages=packages, modes=modes), encoding="utf-8")
        script.chmod(0o755)

    def test_each_package_runs_alone_with_json_and_its_own_timeout(self):
        self.install_go(["a", "b"], {"a": "fail", "b": "pass"})
        with contextlib.redirect_stdout(io.StringIO()) as shown:
            status = ci.go_test("server", [("./a", None), ("./b", None)])
        self.assertEqual(status, 1)
        calls = self.argv.read_text().splitlines()
        self.assertEqual(len(calls), 2)
        for call in calls:
            self.assertIn("test -json -count=1 -timeout=780s", call)
            self.assertNotIn(" -run ", call)
            self.assertNotIn("./...", call)
        text = shown.getvalue()
        self.assertIn("FAIL  ./a  tests=1", text)
        self.assertIn("failed TestBad", text)
        self.assertIn("PASS  ./b  tests=1", text)
        self.assertIn("exit: 1", text)

    def test_a_package_deadline_is_named_with_its_never_ran_tests(self):
        self.install_go(["a"], {"a": "deadline"})
        with contextlib.redirect_stdout(io.StringIO()) as shown:
            status = ci.go_test("server", [("./a", None)])
        self.assertEqual(status, 124)
        text = shown.getvalue()
        self.assertIn("TIMEOUT  ./a", text)
        self.assertIn("hung TestStuck", text)
        self.assertIn("no result for TestStuck, TestAfter (2 of 2 declared)", text)
        self.assertIn("exit: 124", text)

    def test_an_unavailable_test_inventory_is_reported_not_pretended(self):
        self.install_go(["a"], {"a": "deadline-nolist"})
        with contextlib.redirect_stdout(io.StringIO()) as shown:
            status = ci.go_test("server", [("./a", None)])
        self.assertEqual(status, 124)
        text = shown.getvalue()
        self.assertIn("test inventory unavailable", text)
        self.assertNotIn("declared)", text)

    def test_watchdog_names_the_hung_package_and_its_unfinished_tests(self):
        self.install_go(["a", "b"], {"a": "hang", "b": "pass"})
        started = time.monotonic()
        with contextlib.redirect_stdout(io.StringIO()) as shown, \
                patch.object(ci, "GO_PACKAGE_BACKSTOP", 2.0), patch.object(ci, "GO_PACKAGE_JOBS", 2):
            status = ci.go_test("server", [("./a", None), ("./b", None)])
        self.assertLess(time.monotonic() - started, 30)
        self.assertEqual(status, 124)
        text = shown.getvalue()
        self.assertIn("terminating that package only", text)
        self.assertIn("WATCHDOG  ./a", text)
        self.assertIn("hung TestHangs", text)
        self.assertIn("no result for TestHangs, TestNotRun (2 of 2 declared)", text)
        self.assertIn("PASS  ./b", text)

    def test_structured_records_keep_every_test_event_and_package_result(self):
        results = self.root / "results.jsonl"
        self.install_go(["a", "b"], {"a": "fail", "b": "pass"})
        with contextlib.redirect_stdout(io.StringIO()), \
                patch.dict(ci.os.environ, {"CI_GO_RESULTS": str(results)}):
            status = ci.go_test("server", [("./a", None), ("./b", None)])
        self.assertEqual(status, 1)
        records = [json.loads(line) for line in results.read_text().splitlines()]
        self.assertEqual(records[0]["record"], "plan")
        self.assertEqual(records[0]["packages"], ["./a", "./b"])
        self.assertEqual(records[-1]["record"], "summary")
        self.assertEqual(sorted(record["record"] for record in records[1:-1]),
                         ["package", "package", "package-start", "package-start", "test", "test", "test", "test"])
        starts = {record["package"] for record in records if record["record"] == "package-start"}
        self.assertEqual(starts, {"./a", "./b"})
        events = [(record["package"], record["test"], record["action"])
                  for record in records if record["record"] == "test"]
        self.assertIn(("./a", "TestBad", "run"), events)
        self.assertIn(("./a", "TestBad", "fail"), events)
        self.assertIn(("./b", "TestGood", "run"), events)
        self.assertIn(("./b", "TestGood", "pass"), events)
        packages = {record["package"]: record for record in records if record["record"] == "package"}
        self.assertEqual(packages["./a"]["status"], "fail")
        self.assertEqual(packages["./a"]["tests"], {"TestBad": "fail"})
        self.assertEqual(packages["./b"]["tests"], {"TestGood": "pass"})
        self.assertEqual(records[-1]["exit"], 1)

    def test_an_interrupted_package_is_proven_from_its_own_records(self):
        results = self.root / "results.jsonl"
        self.install_go(["a"], {"a": "hang"})
        with contextlib.redirect_stdout(io.StringIO()), \
                patch.dict(ci.os.environ, {"CI_GO_RESULTS": str(results)}), \
                patch.object(ci, "GO_PACKAGE_BACKSTOP", 2.0):
            status = ci.go_test("server", [("./a", None)])
        self.assertEqual(status, 124)
        records = [json.loads(line) for line in results.read_text().splitlines()]
        self.assertEqual(records[0]["record"], "plan")
        self.assertEqual(records[1]["record"], "package-start")
        self.assertEqual(records[1]["package"], "./a")
        events = [(record["test"], record["action"]) for record in records if record["record"] == "test"]
        self.assertEqual(events, [("TestHangs", "run")])
        package = next(record for record in records if record["record"] == "package")
        self.assertEqual(package["status"], "watchdog")
        self.assertEqual(package["unfinished"], ["TestHangs"])
        self.assertEqual(package["never_ran"], ["TestHangs", "TestNotRun"])
        self.assertEqual(records[-1]["hangs"][0]["package"], "./a")

    def test_report_directory_is_used_when_no_results_file_is_named(self):
        directory = self.root / "reports"
        self.install_go(["a"], {"a": "pass"})
        with contextlib.redirect_stdout(io.StringIO()), \
                patch.dict(ci.os.environ, {"CI_TEST_REPORT_DIR": str(directory)}):
            self.assertEqual(ci.go_test("server", [("./a", None)], 2, 4), 0)
        written = directory / "go-results-server-shard3-of-4.jsonl"
        self.assertTrue(written.is_file())
        records = [json.loads(line) for line in written.read_text().splitlines()]
        self.assertEqual(records[0]["record"], "plan")
        self.assertEqual(records[0]["shard"], {"index": 2, "total": 4})
        self.assertEqual(records[0]["packages"], ["./a"])

    def test_a_broken_report_location_keeps_the_console_result(self):
        blocked = self.root / "not-a-directory"
        blocked.write_text("")
        self.install_go(["a"], {"a": "pass"})
        with contextlib.redirect_stdout(io.StringIO()) as shown, \
                patch.dict(ci.os.environ, {"CI_TEST_REPORT_DIR": str(blocked)}):
            status = ci.go_test("server", [("./a", None)])
        self.assertEqual(status, 0)
        self.assertIn("structured record unavailable", shown.getvalue())
        self.assertIn("PASS  ./a", shown.getvalue())

    def test_a_hang_in_a_named_selection_reports_only_its_selected_tests(self):
        self.install_go(["a"], {"a": "hang"})
        with contextlib.redirect_stdout(io.StringIO()) as shown, \
                patch.object(ci, "GO_PACKAGE_BACKSTOP", 2.0):
            status = ci.go_test("server", [("./a", "^(TestHangs)$")])
        self.assertEqual(status, 124)
        text = shown.getvalue()
        self.assertIn("no result for TestHangs (1 of 1 selected)", text)
        self.assertNotIn("TestNotRun", text)

    def test_a_hang_names_the_package_and_every_result_gathered_before_it(self):
        self.install_go(["a", "b"], {"a": "fail", "b": "deadline"})
        with contextlib.redirect_stdout(io.StringIO()) as shown:
            status = ci.go_test("server", [("./a", None), ("./b", None)])
        self.assertEqual(status, 124)
        text = shown.getvalue()
        self.assertIn("FAIL  ./a", text)
        self.assertIn("TIMEOUT  ./b", text)

    def test_a_plan_record_and_summary_name_packages_that_never_started(self):
        results = self.root / "results.jsonl"
        passed = ci.PackageResult("./a", None)
        passed.action, passed.exit, passed.finished = "pass", 0, True
        with contextlib.redirect_stdout(io.StringIO()) as shown, \
                patch.dict(ci.os.environ, {"CI_GO_RESULTS": str(results)}):
            report = ci.GoReport("server", ["./a", "./b"], 0, 1)
            report.add(passed)
            code = report.summary(interrupted=True)
        self.assertEqual(code, 124)
        self.assertIn("NEVER RAN  ./b", shown.getvalue())
        records = [json.loads(line) for line in results.read_text().splitlines()]
        self.assertEqual(records[0]["record"], "plan")
        self.assertEqual(records[0]["packages"], ["./a", "./b"])
        self.assertEqual(records[-1]["never_started"], ["./b"])

    def test_an_interrupted_package_names_its_unfinished_tests_without_a_no_result_claim(self):
        results = self.root / "results.jsonl"
        running = ci.PackageResult("./a", None)
        running.tests["TestDone"] = "pass"
        running.tests["TestSlow"] = "run"
        running.interrupted = True
        running.exit = -signal.SIGTERM
        with contextlib.redirect_stdout(io.StringIO()) as shown, \
                patch.dict(ci.os.environ, {"CI_GO_RESULTS": str(results)}):
            report = ci.GoReport("server", ["./a", "./b"], 0, 1)
            report.add(running)
            code = report.summary(interrupted=True)
        text = shown.getvalue()
        self.assertEqual(code, 124)
        self.assertIn("INTERRUPTED  ./a", text)
        self.assertIn("unfinished TestSlow", text)
        self.assertIn("declared inventory not collected", text)
        # No inventory was collected, so no declared result may be claimed.
        self.assertNotIn("no result for", text)
        self.assertNotIn("declared)", text)
        self.assertIn("NEVER RAN  ./b", text)
        records = [json.loads(line) for line in results.read_text().splitlines()]
        package = next(record for record in records if record["record"] == "package")
        self.assertTrue(package["interrupted"])
        self.assertEqual(package["status"], "interrupted")
        self.assertEqual(package["unfinished"], ["TestSlow"])
        self.assertEqual(records[-1]["exit"], 124)
        self.assertTrue(records[-1]["interrupted"])

    def test_a_leader_that_exits_while_its_child_holds_stdout_is_named_and_stopped(self):
        self.install_go(["a", "b"], {"a": "leak", "b": "pass"})
        started = time.monotonic()
        with contextlib.redirect_stdout(io.StringIO()) as shown, \
                patch.object(ci, "PIPE_HOLD_GRACE", 1.0), \
                patch.object(ci, "GO_PACKAGE_BACKSTOP", 300), time_limit(30):
            status = ci.go_test("server", [("./a", None), ("./b", None)])
        elapsed = time.monotonic() - started
        text = shown.getvalue()
        self.assertEqual(status, 124)
        self.assertLess(elapsed, 30)
        self.assertIn("PIPE-HOLD  ./a", text)
        self.assertIn("still holds its output pipe", text)
        self.assertIn("hung TestLeak", text)
        self.assertIn("no result for TestLeak, TestAfter (2 of 2 declared)", text)
        # The lane was released, so the rest of the shard still ran.
        self.assertIn("PASS  ./b", text)
        self.assertIn("exit: 124", text)
        leader, child = (int(pid) for pid in (self.root / "pids" / "a").read_text().split())
        for pid in (leader, child):
            self.assert_gone(pid)
        self.assert_group_gone(leader)

    def test_the_watchdog_names_a_leader_that_exited_leaving_its_pipe_held(self):
        # The idle-pipe check is disabled here so the backstop itself is under test.
        self.install_go(["a"], {"a": "leak"})
        with contextlib.redirect_stdout(io.StringIO()) as shown, \
                patch.object(ci, "PIPE_HOLD_GRACE", 10_000), \
                patch.object(ci, "GO_PACKAGE_BACKSTOP", 1.0), time_limit(30):
            status = ci.go_test("server", [("./a", None)])
        text = shown.getvalue()
        self.assertEqual(status, 124)
        self.assertIn("descendant still holds", text)
        self.assertIn("PIPE-HOLD  ./a", text)
        self.assertIn("no result for TestLeak, TestAfter (2 of 2 declared)", text)
        leader, child = (int(pid) for pid in (self.root / "pids" / "a").read_text().split())
        for pid in (leader, child):
            self.assert_gone(pid)

    def test_inventory_lists_one_package_per_line(self):
        self.install_go(["a", "nested/b"], {})
        self.assertEqual(ci.module_packages("server"), ["./a", "./nested/b"])

    def test_empty_inventory_fails_closed(self):
        self.install_go([], {})
        with self.assertRaisesRegex(RuntimeError, "reported no packages"):
            ci.module_packages("server")

    def test_full_module_shard_runs_its_slice_and_still_vets_the_whole_module(self):
        with patch.dict(ci.os.environ, {"CI_FULL_TREE": "1", "CI_SHARD_INDEX": "1",
                                        "CI_SHARD_TOTAL": "2"}), \
                patch.object(ci, "module_packages", return_value=["./a", "./b", "./c"]) as inventory, \
                patch.object(ci, "go_test", return_value=0) as tests, \
                patch.object(ci, "run_all_status", return_value=0) as vet, \
                contextlib.redirect_stdout(io.StringIO()):
            ci.go_checks("server", "affected")
        inventory.assert_called_once_with("server")
        self.assertEqual(tests.call_args.args, ("server", [("./b", None)], 1, 2))
        self.assertEqual(vet.call_args.args[0],
                         [(["bash", str(self.root / "scripts/vet-go.sh")], self.root / "server")])

    def test_apikit_full_module_shard_still_vets_with_go_vet(self):
        with patch.dict(ci.os.environ, {"CI_FULL_TREE": "1", "CI_SHARD_INDEX": "0",
                                        "CI_SHARD_TOTAL": "1"}), \
                patch.object(ci, "module_packages", return_value=["./a"]), \
                patch.object(ci, "go_test", return_value=0) as tests, \
                patch.object(ci, "run_all_status", return_value=0) as vet, \
                contextlib.redirect_stdout(io.StringIO()):
            ci.go_checks("apikit", "affected")
        self.assertEqual(tests.call_args.args, ("apikit", [("./a", None)], 0, 1))
        self.assertEqual(vet.call_args.args[0], [(["go", "vet", "./..."], self.root / "apikit")])

    def test_dependency_change_compiles_the_module_one_package_per_run(self):
        with patch.dict(ci.os.environ, {"CI_FULL_TREE": "0"}), \
                patch.object(ci, "changed", return_value={"server/go.mod"}), \
                patch.object(ci, "module_packages", return_value=["./a", "./b"]), \
                patch.object(ci, "go_test", return_value=0) as tests, \
                patch.object(ci, "run_all_status", return_value=0) as vet:
            ci.go_checks("server", "affected")
        self.assertEqual(tests.call_args.args, ("server", [("./a", None), ("./b", None)]))
        self.assertEqual(vet.call_args.args[0], [(["go", "vet", "./..."], self.root / "server")])

    def test_mandatory_security_group_is_never_sharded_and_keeps_every_name(self):
        names = ["TestRefreshRoundTripOverPlainHTTP",
                 "TestUnknownAndKnownNamesHaveTheSameSignInCooloff",
                 "TestAccountRestoreUsesAccountScopeUntilExplicitProfileSelection",
                 "TestSessionFamilyHTTPLogoutAuthenticGenerations",
                 "TestASecondProfileOfTheOwnersAccountIsNotAnAdministrator",
                 "TestRestrictedProfileRouteMatrix", "TestRestrictedProfileWriteRouteMatrix"]
        package = self.root / "server" / "internal" / "httpapi"
        package.mkdir(parents=True)
        (package / "authority_test.go").write_text(
            "package httpapi\n\n" + "".join(f"func {name}(t *testing.T) {{}}\n" for name in names))
        with patch.dict(ci.os.environ, {"CI_FULL_TREE": "1", "CI_SHARD_INDEX": "0",
                                        "CI_SHARD_TOTAL": "4"}), \
                patch.object(ci, "go_test", return_value=0) as tests, \
                patch.object(ci, "run_all_status", return_value=0) as vet:
            ci.go_checks("server", "auth")
        selected = tests.call_args.args[1]
        self.assertEqual([entry[0] for entry in selected], ["./internal/httpapi"])
        pattern = selected[0][1]
        self.assertTrue(pattern.startswith("^(") and pattern.endswith(")$"))
        for name in names:
            self.assertIn(name, pattern)
        self.assertEqual(vet.call_args.args[0],
                         [(["go", "vet", "./internal/httpapi"], self.root / "server")])

    def test_missing_mandatory_go_security_test_fails_closed(self):
        with patch.object(ci, "go_test"), patch.object(ci, "run_all_status"):
            with self.assertRaisesRegex(RuntimeError, "missing mandatory security test"):
                ci.go_checks("server", "auth")


class GoSignalTests(ProcessChecks, unittest.TestCase):
    """SIGTERM stops every package the runner owns and keeps a usable record."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.pids = self.root / "pids"
        self.pids.mkdir()
        self.results = self.root / "results.jsonl"
        self.argv = self.root / "argv.log"
        self.argv.write_text("")

    def install_go(self, packages, modes):
        script = self.bin / "go"
        script.write_text(FAKE_GO.format(packages=packages, modes=modes), encoding="utf-8")
        script.chmod(0o755)

    def wait_for(self, predicate, seconds=30, message="condition"):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            if predicate():
                return
            time.sleep(0.05)
        self.fail(f"timed out waiting for {message}")

    def test_sigterm_stops_every_owned_package_child_and_keeps_the_records(self):
        self.install_go(["a", "b", "c", "d"],
                        {"a": "hang-child", "b": "hang-child",
                         "c": "hang-child", "d": "pass"})
        environment = {**os.environ,
                       "PATH": str(self.bin) + os.pathsep + os.environ.get("PATH", ""),
                       "PID_DIR": str(self.pids),
                       "FAKE_ARGV": str(self.argv),
                       "CI_GO_RESULTS": str(self.results)}
        runner = subprocess.Popen(
            [sys.executable, str(Path(ci.__file__)), "go-module", "--module", "apikit"],
            cwd=self.root, env=environment, stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT, text=True)
        self.addCleanup(lambda: runner.poll() is None and runner.kill())

        def recorded():
            if not self.results.is_file():
                return False
            text = self.results.read_text()
            return (text.count('"record": "package-start"') >= 3
                    and text.count('"record": "test"') >= 3)

        # Three lanes are running their packages and the fourth is still queued.
        self.wait_for(recorded, message="three started packages")
        started = {package: (self.pids / package).read_text().split()
                   for package in ("a", "b", "c")}
        runner.terminate()
        output, _ = runner.communicate(timeout=60)

        self.assertEqual(runner.returncode, 128 + signal.SIGTERM, output)
        self.assertIn("SIGTERM: interrupted", output)
        self.assertIn("INTERRUPTED  ./", output)
        self.assertIn("unfinished TestHangs", output)
        self.assertIn("declared inventory not collected", output)
        self.assertNotIn("no result for", output)
        self.assertIn("NEVER RAN  ./d", output)
        self.assertNotIn("PASS  ./a", output)

        for pids in started.values():
            for pid in (int(value) for value in pids):
                self.assert_gone(pid)
            self.assert_group_gone(int(pids[0]))

        records = [json.loads(line) for line in self.results.read_text().splitlines()]
        self.assertEqual(records[0]["record"], "plan")
        self.assertEqual(records[0]["packages"], ["./a", "./b", "./c", "./d"])
        packages = [record for record in records if record["record"] == "package"]
        self.assertEqual(sorted(record["package"] for record in packages), ["./a", "./b", "./c"])
        for record in packages:
            self.assertTrue(record["interrupted"], record)
            self.assertEqual(record["status"], "interrupted")
            self.assertEqual(record["never_ran"], [])
            self.assertIsNone(record["inventory"])
        events = [(record["package"], record["test"], record["action"])
                  for record in records if record["record"] == "test"]
        self.assertEqual(sorted(events), [("./a", "TestHangs", "run"),
                                          ("./b", "TestHangs", "run"),
                                          ("./c", "TestHangs", "run")])
        summary = records[-1]
        self.assertEqual(summary["record"], "summary")
        self.assertTrue(summary["interrupted"])
        self.assertEqual(summary["never_started"], ["./d"])
        self.assertEqual(summary["exit"], 124)


class ShardTests(unittest.TestCase):

    def test_shards_cover_every_package_exactly_once(self):
        packages = ["./e", "./c", "./a", "./d", "./b"]
        shards = [ci.shard(packages, index, 4) for index in range(4)]
        self.assertEqual(sorted(sum(shards, [])), sorted(packages))
        self.assertEqual(len(set(sum(shards, []))), len(packages))

    def test_shard_assignment_is_stable_across_input_order(self):
        self.assertEqual(ci.shard(["./b", "./a", "./c"], 1, 2), ["./b"])
        self.assertEqual(ci.shard(["./c", "./b", "./a"], 1, 2), ["./b"])
        self.assertEqual(ci.shard(packages=["./a", "./b", "./c", "./d", "./e"], index=0, total=2),
                         ["./a", "./c", "./e"])

    def test_malformed_shard_settings_fail_closed(self):
        for index, total in (("4", "4"), ("0", "0"), ("x", "2"), ("-1", "2"), ("0", "")):
            with patch.dict(ci.os.environ, {"CI_SHARD_INDEX": index, "CI_SHARD_TOTAL": total}):
                with self.assertRaisesRegex(RuntimeError, "invalid Go package shard"):
                    ci.shard_settings()

    def test_absent_shard_settings_mean_one_complete_shard(self):
        environment = os.environ.copy()
        environment.pop("CI_SHARD_INDEX", None)
        environment.pop("CI_SHARD_TOTAL", None)
        with patch.dict(ci.os.environ, environment, clear=True):
            self.assertEqual(ci.shard_settings(), (0, 1))
            self.assertEqual(ci.shard(["./a", "./b"]), ["./a", "./b"])


if __name__ == "__main__":
    unittest.main()
