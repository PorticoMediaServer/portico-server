import contextlib
import importlib.util
import io
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("affected", Path(__file__).with_name("ci-affected.py"))
ci = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ci)


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

    def test_full_module_runs_tests_without_a_name_or_diff_filter(self):
        with patch.dict(ci.os.environ, {"CI_FULL_TREE": "1"}), patch.object(ci, "changed", side_effect=AssertionError("diff must not be read")), patch.object(ci, "run_all") as run:
            ci.go_checks("apikit", "affected")
            self.assertEqual(run.call_args.args[0], [(["go", "test", "-count=1", "./..."], self.root / "apikit"), (["go", "vet", "./..."], self.root / "apikit")])

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


class BoundedCommandTests(unittest.TestCase):
    """A hung suite fails with a named command instead of holding the whole job."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

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


if __name__ == "__main__":
    unittest.main()
