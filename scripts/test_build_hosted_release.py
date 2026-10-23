import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("hosted_release", Path(__file__).with_name("build-hosted-release.py"))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class HostedWebInputTests(unittest.TestCase):
    def test_legacy_default_and_services_workspace_closure(self):
        self.assertEqual(release.release_inputs(), release.INPUTS)
        inputs = release.release_inputs("web-next")
        for name in ("web-next", "web/package.json", "packages/player-core", "packages/reader-core", ".node-version"):
            self.assertIn((release.SERVER, name), inputs)
        self.assertNotIn((release.SERVER, "web"), inputs)
        with self.assertRaises(ValueError):
            release.release_inputs("other")

    def test_explicit_services_uses_actual_config_and_same_output_contract(self):
        with tempfile.TemporaryDirectory() as temporary:
            snapshot = Path(temporary)
            (snapshot / ".node-version").write_text("24.21.0\n")
            calls = []
            with patch.object(release.subprocess, "check_output", return_value="v24.21.0\n"), patch.object(release, "run", side_effect=lambda args, cwd, env: calls.append((args, cwd, dict(env)))), patch.dict(os.environ, {"VITE_HOSTED_WEB": "false", "VITE_OTHER": "untrusted", "NODE_OPTIONS": "untrusted"}, clear=True):
                release.build_web(snapshot, snapshot / "output/web", "web-next")
            self.assertEqual(calls[0][0], ["npm", "ci", "--ignore-scripts", "--no-audit", "--no-fund"])
            self.assertEqual(calls[0][1], snapshot)
            self.assertEqual(calls[1][1], snapshot / "web-next")
            env = calls[1][2]
            self.assertEqual((env["VITE_HOSTED_WEB"], env["VITE_ACCOUNT_WORKSPACE"]), ("true", "true"))
            self.assertEqual(env["VITE_HOSTED_URL"], "https://web.getportico.tv")
            self.assertEqual(env["PORTICO_RELEASE_WEB"], str(snapshot / "output/web"))
            self.assertNotIn("VITE_OTHER", env)
            self.assertNotIn("NODE_OPTIONS", env)

    def test_services_refuses_wrong_node_before_install(self):
        with tempfile.TemporaryDirectory() as temporary:
            snapshot = Path(temporary)
            (snapshot / ".node-version").write_text("24.21.0\n")
            with patch.object(release.subprocess, "check_output", return_value="v22.0.0\n"), patch.object(release, "run") as run:
                with self.assertRaises(ValueError):
                    release.build_web(snapshot, snapshot / "output/web", "web-next")
                run.assert_not_called()

    def test_legacy_still_builds_legacy_without_selecting_services(self):
        with patch.object(release, "run") as run, patch.object(release.subprocess, "check_output") as node:
            release.build_web(Path("/captured"), Path("/output/web"))
            self.assertEqual(run.call_args[0][1], Path("/captured/web"))
            node.assert_not_called()


if __name__ == "__main__":
    unittest.main()
