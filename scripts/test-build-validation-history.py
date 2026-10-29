#!/usr/bin/env python3
"""Offline guard: the validation toolchain keeps the history its gate reads.

`scripts/build-ffmpeg.sh` opens with `scripts/check-ffmpeg-build-inputs.py`.
That gate is the proof a bundle was built from checked MAIN inputs, and it
refuses a Git checkout unless the pinned `checkedMain` commit is an ancestor of
both `HEAD` and `origin/main` and the checked inputs are unchanged. A shallow
`actions/checkout` (the action default, fetch-depth 1) supplies HEAD's tree
only: the pinned object and the main ref are absent, `git merge-base` exits 128,
and every toolchain leg stops before it builds anything.

The two halves have to stay true together, so this pins both: the workflow job
that runs the recipe fetches full history, and the gate still asks for both
ancestries. Everything here is text and mocked subprocess calls -- no Git,
network, container, compiler, Go or FFmpeg process runs. Run directly:

    python3 scripts/test-build-validation-history.py
"""
import hashlib
import importlib.util
import json
import pathlib
import re
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parent.parent
WORKFLOW = ROOT / ".github/workflows/build-validation.yml"
CHECK = ROOT / "scripts/check-ffmpeg-build-inputs.py"
RECIPE = ROOT / "scripts/build-ffmpeg.sh"
AUTHORITY = ROOT / "third_party/ffmpeg/source-authority.v1.json"
JOB = "toolchain"
CHECKOUT = "actions/checkout@"
HEAD_SHA = "0123456789abcdef0123456789abcdef01234567"


def job(text, name):
    """The YAML block for one workflow job, by indentation at the job level."""
    match = re.search(r"^  %s:\n(.*?)(?=^  [A-Za-z0-9_-]+:\s*$|\Z)" % re.escape(name), text, re.M | re.S)
    return match.group(1) if match else ""


def replace_job(text, name, body):
    pattern = re.compile(r"^  %s:\n(.*?)(?=^  [A-Za-z0-9_-]+:\s*$|\Z)" % re.escape(name), re.M | re.S)
    return pattern.sub(lambda _match: "  %s:\n%s" % (name, body), text, count=1)


def checkout_blocks(text):
    """Each `actions/checkout` step with the lines indented under it."""
    lines = text.splitlines()
    blocks = []
    for index, line in enumerate(lines):
        if CHECKOUT not in line:
            continue
        indent = len(line) - len(line.lstrip())
        block = [line]
        for following in lines[index + 1:]:
            if following.strip() and len(following) - len(following.lstrip()) <= indent:
                break
            block.append(following)
        blocks.append("\n".join(block))
    return blocks


def audit(text):
    """Every way the recipe job can lose the history the checked-MAIN gate reads."""
    problems = []
    block = job(text, JOB)
    if not block:
        return ["build-validation.yml: the %s job is gone" % JOB]
    if "scripts/build-ffmpeg.sh" not in block:
        problems.append("build-validation.yml %s: no longer builds the pinned FFmpeg recipe" % JOB)
    checkouts = checkout_blocks(block)
    if len(checkouts) != 1:
        problems.append("build-validation.yml %s: expected one checkout, found %d" % (JOB, len(checkouts)))
    for checkout in checkouts:
        depth = re.search(r"fetch-depth:\s*([0-9]+)", checkout)
        if not depth:
            problems.append("build-validation.yml %s: the checkout keeps the action default fetch-depth 1, "
                            "so the gate cannot see the pinned checkedMain commit or origin/main" % JOB)
        elif depth.group(1) != "0":
            problems.append("build-validation.yml %s: fetch-depth: %s does not carry the gate's history"
                            % (JOB, depth.group(1)))
    return problems


class Harness:
    """Records the Git commands the gate runs, without running Git."""

    def __init__(self, missing=None):
        self.calls = []
        self.missing = missing

    def run(self, argv, **_kwargs):
        self.calls.append(tuple(argv))
        if self.missing is not None and argv[-1] == self.missing:
            raise subprocess.CalledProcessError(128, argv)
        return subprocess.CompletedProcess(argv, 0)

    def check_output(self, argv, **_kwargs):
        self.calls.append(tuple(argv))
        return HEAD_SHA + "\n"

    def git(self):
        return [call for call in self.calls if call[0] == "git"]


def load_check():
    specification = importlib.util.spec_from_file_location("ffmpeg_build_input_check", CHECK)
    module = importlib.util.module_from_spec(specification)
    specification.loader.exec_module(module)
    return module


class ValidationHistoryTests(unittest.TestCase):
    def setUp(self):
        self.text = WORKFLOW.read_text()
        self.pinned = json.loads(AUTHORITY.read_text())["checkedMain"]
        self.temporary = tempfile.TemporaryDirectory(prefix="validation-history-")
        self.addCleanup(self.temporary.cleanup)

    def fixture_checkout(self):
        """A checkout that satisfies the file half of the gate, so only Git is mocked."""
        root = pathlib.Path(self.temporary.name) / "checkout"
        (root / "third_party/ffmpeg").mkdir(parents=True, exist_ok=True)
        (root / ".git").mkdir(exist_ok=True)
        authority = root / "third_party/ffmpeg/source-authority.v1.json"
        authority.write_text(json.dumps({"schema": 1, "checkedMain": self.pinned,
                                         "scope": "offline fixture", "files": {}}))
        (root / "third_party/ffmpeg/sources.lock.json").write_text(json.dumps({"sourceAuthority": {
            "path": "third_party/ffmpeg/source-authority.v1.json",
            "sha256": hashlib.sha256(authority.read_bytes()).hexdigest()}}))
        return root.resolve()

    def verify(self, harness):
        check = load_check()
        original = check.subprocess
        check.subprocess = harness
        try:
            return check.verify(self.fixture_checkout())
        finally:
            check.subprocess = original

    def test_the_recipe_job_fetches_the_history_its_gate_reads(self):
        self.assertEqual(audit(self.text), [])

    def test_the_recipe_still_opens_with_the_checked_main_gate(self):
        recipe = RECIPE.read_text()
        self.assertIn("scripts/check-ffmpeg-build-inputs.py", recipe)
        gate = recipe.index("scripts/check-ffmpeg-build-inputs.py")
        for name in ("jellyfin-ffmpeg-source.tar.gz", "./build.sh", "makeimage.sh"):
            if name in recipe:
                self.assertLess(gate, recipe.index(name), name)

    def test_a_shallow_or_missing_depth_is_reported(self):
        body = job(self.text, JOB)
        shallow = replace_job(self.text, JOB, body.replace("        with: {fetch-depth: 0}\n", ""))
        problems = audit(shallow)
        self.assertTrue(any("fetch-depth 1" in problem for problem in problems), problems)
        bounded = replace_job(self.text, JOB, body.replace("fetch-depth: 0", "fetch-depth: 1"))
        problems = audit(bounded)
        self.assertTrue(any("fetch-depth: 1 does not carry" in problem for problem in problems), problems)

    def test_a_recipe_job_that_stops_building_is_reported(self):
        body = job(self.text, JOB).replace("./scripts/build-ffmpeg.sh", "./scripts/other-build.sh")
        problems = audit(replace_job(self.text, JOB, body))
        self.assertTrue(any("no longer builds the pinned FFmpeg recipe" in problem for problem in problems), problems)

    def test_the_gate_reads_the_pinned_commit_from_both_head_and_main(self):
        harness = Harness()
        result = self.verify(harness)
        ancestry = [call for call in harness.git() if "--is-ancestor" in call]
        self.assertEqual([call[-1] for call in ancestry], ["HEAD", "origin/main"])
        for call in ancestry:
            self.assertEqual(call[-2], self.pinned)
        diffs = [call for call in harness.git() if "diff" in call]
        self.assertEqual(len(diffs), 1)
        self.assertIn("--quiet", diffs[0])
        self.assertIn("HEAD", diffs[0])
        self.assertEqual(result["checkedMain"], self.pinned)
        self.assertEqual(result["buildSourceCommit"], HEAD_SHA)
        self.assertTrue(result["kind"].startswith("Git checkout"))

    def test_a_checkout_without_the_pinned_main_history_fails_closed(self):
        for missing in ("HEAD", "origin/main"):
            with self.subTest(missing=missing):
                with self.assertRaises(subprocess.CalledProcessError) as raised:
                    self.verify(Harness(missing=missing))
                self.assertEqual(raised.exception.returncode, 128)


if __name__ == "__main__":
    unittest.main()
