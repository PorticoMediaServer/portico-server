import importlib.util
from pathlib import Path
import tempfile
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


if __name__ == "__main__":
    unittest.main()
