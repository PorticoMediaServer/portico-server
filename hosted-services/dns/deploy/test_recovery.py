"""No-network deployment/recovery control-flow regressions."""
import ast
import importlib.util
import json
from pathlib import Path
import subprocess
import types
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[3]


def load_helper():
    spec = importlib.util.spec_from_file_location("recovery", ROOT / "scripts/rebuild-dns-after-hosted-restore.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class RecoveryTests(unittest.TestCase):
    def test_both_stopped_before_build_and_matching_before_restart(self):
        module = load_helper()
        calls = []
        receipt = {"sequence": 5, "rows": 2, "digest": "a" * 64}

        def run(host, argv):
            calls.append((host, argv))
            return json.dumps(receipt) if argv[-1] == "dns-rebuild" else ""
        self.assertEqual(module.recover(["one", "two"], run), receipt)
        self.assertEqual([host for host, _ in calls[:2]], ["one", "two"])
        self.assertTrue(all(argv == module.STOP for _, argv in calls[:2]))
        self.assertEqual([argv[-1] for _, argv in calls[2:4]], ["dns-rebuild"] * 2)
        self.assertIn("User=portico-dns", calls[2][1])
        self.assertEqual([argv for _, argv in calls[4:6]], [module.START] * 2)

    def test_mismatch_and_failed_rebuild_leave_both_stopped(self):
        for failure in ("mismatch", "interrupted"):
            with self.subTest(failure=failure):
                module = load_helper()
                calls = []

                def run(host, argv):
                    calls.append((host, argv))
                    if argv[-1] == "dns-rebuild":
                        if failure == "interrupted" and host == "two":
                            raise subprocess.CalledProcessError(1, argv)
                        return json.dumps({"sequence": 0, "rows": 0, "digest": host[0] * 64})
                    return ""
                with self.assertRaises((RuntimeError, subprocess.CalledProcessError)):
                    module.recover(["one", "two"], run)
                self.assertFalse(any(argv == module.START for _, argv in calls))
                self.assertEqual([host for host, argv in calls[-2:]], ["one", "two"])
                self.assertTrue(all(argv == module.STOP for _, argv in calls[-2:]))

    def test_deployer_rolls_back_when_the_feed_does_not_sync(self):
        tree = ast.parse((ROOT / "scripts/deploy-dns-release.py").read_text())
        program = next(node.value.value for node in ast.walk(tree) if isinstance(node, ast.Assign)
                       and any(isinstance(target, ast.Name) and target.id == "PROGRAM" for target in node.targets))
        calls, links, printed = [], [], []

        class FakePath:
            def __init__(self, path): self.path = str(path)
            def __truediv__(self, path): return FakePath(self.path + "/" + str(path))
            def __str__(self): return self.path
            @property
            def name(self): return self.path.rsplit("/", 1)[-1]
            def with_name(self, name): return FakePath(self.path.rsplit("/", 1)[0] + "/" + name)
            def read_bytes(self): return b"fixture-binary"
            def write_bytes(self, value): pass
            def chmod(self, value): pass
            def unlink(self): pass
            def exists(self): return False
            def is_file(self): return True
            def is_symlink(self): return self.name == "current"
            def stat(self): return types.SimpleNamespace(st_size=45)
            def symlink_to(self, target): links.append(str(target))
            def __fspath__(self): return self.path
            def iterdir(self): return iter(())

        def run(argv, **kwargs):
            calls.append(argv)
            if argv[-1] == "version":
                return types.SimpleNamespace(returncode=0, stdout="portico-fixture\n")
            return types.SimpleNamespace(returncode=1 if argv[-1] == "dns-readiness" else 0, stdout="")
        digest = __import__("hashlib").sha256(b"fixture-binary").hexdigest()
        with patch("pathlib.Path", FakePath), patch("os.readlink", return_value="releases/portico-previous"), \
             patch("os.replace"), patch("subprocess.run", run), patch("time.sleep"), patch("builtins.print", printed.append):
            with self.assertRaisesRegex(RuntimeError, "authenticate and synchronize"):
                exec(program, {"BUILD": "portico-fixture", "DIGEST": digest})
        self.assertEqual(sum(argv[-1] == "dns-readiness" for argv in calls), 180)
        self.assertEqual(links, ["releases/portico-fixture", "releases/portico-previous"])
        self.assertEqual(sum(argv == ["systemctl", "restart", "portico-dns"] for argv in calls), 2)
        self.assertEqual(printed, [])


if __name__ == "__main__":
    unittest.main()
