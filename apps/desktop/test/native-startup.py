"""Actual GTK/WK-page startup witness, not a screen or hardware qualification."""
import json
import os
from pathlib import Path

report = Path(os.environ["PORTICO_SPIKE_REPORT"])
observed = json.loads(report.read_text())
assert observed["platform"] == "linux"
assert observed["ui"]["rootMounted"] is True
assert observed["ui"]["desktopHost"] is True
assert observed["player"] is None and observed["audio"] is None, "missing native engine has no playback/output observations"
assert observed["native"]["renderCalls"] == 0
assert "close-finished 0" in report.with_suffix(".native.log").read_text()
print("actual Linux missing-engine native identity/app mount/clean close PASS; no media/HDR/screens")
