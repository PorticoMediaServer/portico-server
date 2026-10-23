#!/usr/bin/env python3
"""Keep the account-controller false-positive exception narrowly scoped."""

import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import tempfile


def main():
    binary = Path(sys.argv[1]).resolve()
    config = Path(".gitleaks.toml").resolve()
    expression = "this.api,proof=this.moveProof;"
    account = "web-next/src/platform/account-entry.ts"
    with tempfile.TemporaryDirectory(dir=os.environ.get("TMPDIR")) as scratch:
        root = Path(scratch)
        (root / account).parent.mkdir(parents=True)
        # Runtime-generated specimen keys never leave this disposable fixture.
        for name in (account, "other.ts"):
            (root / name).write_text(
                expression + '\nconst apiKey="' + secrets.token_urlsafe(32) + '";\n'
            )
        report = root / "findings.json"
        result = subprocess.run(
            [str(binary), "dir", ".", "--config", str(config), "--redact",
             "--no-banner", "--exit-code=0", "--report-format=json",
             "--report-path=" + str(report)],
            cwd=root, capture_output=True, text=True,
        )
        if result.returncode:
            raise SystemExit("Gitleaks configuration fixture could not run; values withheld.")
        actual = {
            (finding["RuleID"], finding["File"], finding["StartLine"])
            for finding in json.loads(report.read_text())
        }
        expected = {
            ("generic-api-key", account, 2),
            ("generic-api-key", "other.ts", 1),
            ("generic-api-key", "other.ts", 2),
        }
        if actual != expected:
            raise SystemExit("Gitleaks exception exceeded its exact expression/path or lost detection.")
    print("Gitleaks configuration: exact account expression ignored; same-file keys and other paths detected.")


if __name__ == "__main__":
    main()
