import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("test-go-module.sh")
CATALOG = "portico.local/server/internal/catalog"
PACKAGES = [CATALOG, CATALOG + "/scalebench", "portico.local/server/internal/httpapi"]


class ModuleTestPhases(unittest.TestCase):
    def run_phases(self, module, catalog_exit=0, remainder_exit=0, packages=PACKAGES):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            fake = root / "go"
            fake.write_text("""#!/usr/bin/env python3
import json, os, sys
with open(os.environ['CALLS'], 'a') as output:
    output.write(json.dumps(sys.argv[1:]) + '\\n')
if sys.argv[1] == 'list':
    print(os.environ['PACKAGES'])
    sys.exit(0)
is_catalog = sys.argv[2:] == ['portico.local/server/internal/catalog']
sys.exit(int(os.environ['CATALOG_EXIT' if is_catalog else 'REMAINDER_EXIT']))
""")
            fake.chmod(0o755)
            calls = root / "calls"
            env = {**os.environ, "PATH": str(root) + os.pathsep + os.environ["PATH"],
                   "CALLS": str(calls), "PACKAGES": "\n".join(packages),
                   "CATALOG_EXIT": str(catalog_exit), "REMAINDER_EXIT": str(remainder_exit)}
            result = subprocess.run(["sh", str(SCRIPT), module], env=env, capture_output=True, text=True)
            return result.returncode, [json.loads(line) for line in calls.read_text().splitlines()]

    def test_every_package_once_and_catalogue_alone_first(self):
        status, calls = self.run_phases("server")
        self.assertEqual(status, 0)
        self.assertEqual(calls, [["list", "./..."], ["test", CATALOG], ["test", *PACKAGES[1:]]])
        self.assertEqual([package for call in calls[1:] for package in call[1:]], PACKAGES)

    def test_catalogue_failure_still_runs_remainder_and_fails(self):
        status, calls = self.run_phases("server", catalog_exit=3)
        self.assertEqual(status, 3)
        self.assertEqual(calls[-1], ["test", *PACKAGES[1:]])

    def test_remainder_failure_fails_whole_gate(self):
        status, _ = self.run_phases("server", remainder_exit=4)
        self.assertEqual(status, 4)

    def test_apikit_keeps_existing_command(self):
        self.assertEqual(self.run_phases("apikit"), (0, [["test", "./..."]]))

    def test_missing_catalogue_is_a_gate_failure(self):
        status, calls = self.run_phases("server", packages=PACKAGES[1:])
        self.assertEqual(status, 2)
        self.assertEqual(calls, [["list", "./..."]])


if __name__ == "__main__":
    unittest.main()
