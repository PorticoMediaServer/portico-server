#!/usr/bin/env python3
"""Discover, shard and authenticate the complete source-bound Go race suite."""
import argparse
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import subprocess
import tempfile
import zipfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
STAGE = ROOT / ".scratch/race-input"
OUT = ROOT / ".scratch/race-artifacts"
REPOSITORY = "PorticoMediaServer/portico-server"
GO_VERSION = "go1.26.9"
SHARDS = 16
FLAGS = ["-json", "-race", "-count=1", "-timeout=30m"]
CODE_FILES = (
    "scripts/build-server-release.py", "scripts/qualify-server-race.py",
    ".github/workflows/server-race-qualification.yml",
    ".github/workflows/release.yml", ".github/workflows/build-validation.yml",
)
NAME = re.compile(r"(?:Test|Example|Fuzz)\w*")
BASE_ENV = {"GOMAXPROCS": "2", "GOFLAGS": "-p=2", "PORTICO_TEST_TIER": "default"}


def encoded(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n").encode()


def sha(value):
    return hashlib.sha256(value).hexdigest()


def file_sha(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def write(path, document):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(encoded(document))


def builder():
    spec = importlib.util.spec_from_file_location("release_builder", ROOT / "scripts/build-server-release.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def origin():
    required = ("GITHUB_REPOSITORY", "GITHUB_SHA", "GITHUB_WORKFLOW_SHA", "GITHUB_WORKFLOW_REF",
                "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT", "GITHUB_EVENT_NAME")
    values = {name: os.environ[name] for name in required}
    if os.environ.get("GITHUB_ACTIONS") != "true":
        raise ValueError("Qualification custody requires the current GitHub Actions run")
    if values["GITHUB_REPOSITORY"].lower() != REPOSITORY.lower():
        raise ValueError("Foreign or fork repository is forbidden")
    if values["GITHUB_EVENT_NAME"] not in ("push", "workflow_dispatch"):
        raise ValueError("Only an owner-controlled push or manual run may qualify a release")
    commit = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    if subprocess.run(["git", "diff", "--quiet", "HEAD"], cwd=ROOT).returncode:
        raise ValueError("Qualification requires a clean reviewed checkout")
    untracked = subprocess.check_output(["git", "ls-files", "--others", "--exclude-standard"], cwd=ROOT, text=True).splitlines()
    if untracked:
        raise ValueError("Qualification refuses unreviewed source files: " + ", ".join(untracked[:10]))
    if commit != values["GITHUB_SHA"] or commit != values["GITHUB_WORKFLOW_SHA"]:
        raise ValueError("Source and executed workflow must name the same reviewed commit")
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise ValueError("Invalid source commit")
    if not values["GITHUB_RUN_ID"].isdigit() or not values["GITHUB_RUN_ATTEMPT"].isdigit():
        raise ValueError("Invalid Actions run identity")
    allowed = (REPOSITORY + "/.github/workflows/release.yml@",
               REPOSITORY + "/.github/workflows/build-validation.yml@",
               REPOSITORY + "/.github/workflows/server-race-qualification.yml@")
    if not values["GITHUB_WORKFLOW_REF"].lower().startswith(tuple(value.lower() for value in allowed)):
        raise ValueError("Unrecognized qualifying workflow")
    return values


def code_hashes():
    return {name: file_sha(ROOT / name) for name in CODE_FILES}


def test_env():
    environment = builder().controlled_env()
    environment.update(BASE_ENV)
    environment["PORTICO_PLAYBACK_TEST_HELPER"] = str(STAGE / "playback-test-helper")
    return environment


def go(*args, cwd, environment=None):
    return subprocess.run([str(ROOT / "scripts/env.sh"), "go", *args], cwd=cwd,
                          env=environment or test_env(), stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, text=True)


def json_stream(text):
    return json.JSONDecoder().raw_decode(text)


def objects(text):
    result = []
    while text.strip():
        text = text.lstrip()
        value, end = json_stream(text)
        result.append(value)
        text = text[end:]
    return result


def capture():
    module = builder()
    digest, _ = module.capture_source(ROOT / "server", STAGE, module.official_root()[1])
    return digest


def snapshot_after_tests(harnesses):
    for relative, content in harnesses.items():
        path = STAGE / relative
        if path.read_bytes() != content:
            raise ValueError("Qualification init harness was changed")
        path.unlink()
    return builder().snapshot_digest(STAGE / "source")[0]


def package_groups(plan, shard):
    groups = []
    for package in plan["packages"]:
        names = [name for name in package["tests"]
                 if int(sha((package["module"] + "\0" + package["package"] + "\0" + name).encode()), 16) % SHARDS == shard]
        init_only = not package["tests"] and int(sha(package["package"].encode()), 16) % SHARDS == shard
        if names or init_only:
            groups.append({"module": package["module"], "package": package["package"],
                           "directory": package["directory"], "names": names, "initOnly": init_only})
    return groups


def validate_plan(plan):
    if plan.get("schema") != 1 or plan.get("shards") != SHARDS or plan.get("goVersion") != GO_VERSION:
        raise ValueError("Unsupported qualification plan")
    if plan.get("flags") != FLAGS or plan.get("environment") != BASE_ENV:
        raise ValueError("Qualification changed race instrumentation or invocation flags")
    if plan["origin"] != origin() or plan["codeFiles"] != code_hashes():
        raise ValueError("Qualification source/workflow/run/attempt differs")
    if not re.fullmatch(r"[a-f0-9]{64}", plan["sourceDigest"]):
        raise ValueError("Invalid qualified source digest")
    identities = [(p["module"], p["package"]) for p in plan["packages"]]
    if not identities or len(identities) != len(set(identities)):
        raise ValueError("Missing or duplicate package coverage")
    for package in plan["packages"]:
        names = package["tests"]
        if names != sorted(set(names)) or any(not NAME.fullmatch(name) for name in names):
            raise ValueError("Invalid or duplicate discovered test/Example/Fuzz name")
        directory = pathlib.PurePosixPath(package["directory"])
        if directory.is_absolute() or ".." in directory.parts or directory.parts[0] not in ("source", "apikit"):
            raise ValueError("Package path escapes the captured modules")


def install_harnesses(plan):
    harnesses = {}
    for package in plan["packages"]:
        if package.get("initHarness"):
            relative = package["directory"] + "/zz_portico_qualification_init_test.go"
            content = ("package " + package["goPackage"] + "\n").encode()
            if sha(content) != package["initHarness"]:
                raise ValueError("Init harness input differs")
            path = STAGE / relative
            if path.exists():
                raise ValueError("Unexpected preexisting qualification harness")
            path.write_bytes(content)
            harnesses[relative] = content
    return harnesses


def prepare():
    current_origin = origin()
    digest = capture()
    version = go("env", "GOVERSION", cwd=STAGE / "source")
    if version.returncode or version.stdout.strip() != GO_VERSION:
        raise ValueError("Qualification requires the actual Go 1.26.9 compiler")
    helper = go("build", "-mod=readonly", "-o", str(STAGE / "playback-test-helper"),
                "./cmd/server", cwd=STAGE / "source")
    if helper.returncode:
        raise RuntimeError(helper.stderr)
    packages = []
    harnesses = {}
    for name, directory in (("server", STAGE / "source"), ("apikit", STAGE / "apikit")):
        verification = go("mod", "verify", cwd=directory)
        if verification.returncode:
            raise RuntimeError(verification.stderr)
        listing = go("list", "-race", "-json", "./...", cwd=directory)
        if listing.returncode:
            raise RuntimeError(listing.stderr)
        rows = objects(listing.stdout)
        for row in rows:
            relative = str(pathlib.Path(row["Dir"]).relative_to(STAGE))
            record = {"module": name, "package": row["ImportPath"], "directory": relative,
                      "goPackage": row["Name"], "tests": []}
            # A zero-function test harness makes no-test packages execute init,
            # including command packages, without invoking their application main.
            if not row.get("TestGoFiles") and not row.get("XTestGoFiles"):
                content = ("package " + row["Name"] + "\n").encode()
                relative_file = relative + "/zz_portico_qualification_init_test.go"
                path = STAGE / relative_file
                if path.exists():
                    raise ValueError("Reserved qualification harness already exists")
                path.write_bytes(content)
                harnesses[relative_file] = content
                record["initHarness"] = sha(content)
            packages.append(record)
        discovery = go("test", "-json", "-race", "-list", ".", "./...", cwd=directory)
        print(discovery.stderr, end="", flush=True)
        if discovery.returncode:
            raise RuntimeError("Race test discovery/initialization failed")
        by_name = {row["package"]: row for row in packages if row["module"] == name}
        terminal = set()
        for line in discovery.stdout.splitlines():
            event = json.loads(line)
            package = event["Package"]
            if package not in by_name:
                raise ValueError("Undiscovered package in test listing")
            if event["Action"] == "fail":
                raise ValueError("Package initialization failed")
            if event["Action"] in ("pass", "skip"):
                terminal.add(package)
            output = event.get("Output", "").strip()
            if NAME.fullmatch(output):
                by_name[package]["tests"].append(output)
        if terminal != set(by_name):
            raise ValueError("Package discovery did not initialize every package")
        for package in by_name.values():
            package["tests"] = sorted(package["tests"])
    if snapshot_after_tests(harnesses) != digest:
        raise ValueError("Source changed during race discovery")
    plan = {"schema": 1, "origin": current_origin, "sourceDigest": digest,
            "codeFiles": code_hashes(), "goVersion": GO_VERSION, "flags": FLAGS,
            "environment": BASE_ENV, "shards": SHARDS,
            "helperSha256": file_sha(STAGE / "playback-test-helper"),
            "packages": sorted(packages, key=lambda value: (value["module"], value["package"]))}
    validate_plan(plan)
    write(OUT / "plan.json", plan)
    print(f"Discovered {len(packages)} packages and {sum(len(p['tests']) for p in packages)} Test/Example/Fuzz functions.", flush=True)


def run_shard(shard, plan_path):
    plan = json.loads(plan_path.read_bytes())
    validate_plan(plan)
    if capture() != plan["sourceDigest"]:
        raise ValueError("Shard source snapshot differs from discovery")
    harnesses = install_harnesses(plan)
    environment = test_env()
    # The helper is built from the identical captured module, never downloaded
    # from another run. Tests must not fall back to their own test executable.
    helper = go("build", "-mod=readonly", "-o", str(STAGE / "playback-test-helper"),
                "./cmd/server", cwd=STAGE / "source", environment=environment)
    if helper.returncode or file_sha(STAGE / "playback-test-helper") != plan["helperSha256"]:
        raise ValueError("Shard playback helper differs from qualified source/compiler")
    invocations = []
    failed = False
    for group in package_groups(plan, shard):
        pattern = "^(" + "|".join(re.escape(name) for name in group["names"]) + ")$" if group["names"] else "^$"
        command = [*FLAGS, "-run", pattern, group["package"]]
        print(f"SHARD {shard}: {group['package']} ({len(group['names'])} functions, initOnly={group['initOnly']})", flush=True)
        result = go("test", *command, cwd=STAGE / group["directory"], environment=environment)
        print(result.stdout, end="", flush=True)
        print(result.stderr, end="", flush=True)
        events = [json.loads(line) for line in result.stdout.splitlines() if line.strip()]
        record = {**group, "flags": command, "exitCode": result.returncode,
                  "events": [{key: event[key] for key in ("Action", "Package", "Test", "Elapsed") if key in event}
                             for event in events if event["Action"] != "output"],
                  "skips": [{"test": event["Test"], "reason": "".join(
                      row.get("Output", "") for row in events
                      if row.get("Test") == event["Test"] and row["Action"] == "output").strip()}
                      for event in events if event["Action"] == "skip" and "Test" in event],
                  "logSha256": sha((result.stdout + result.stderr).encode())}
        invocations.append(record)
        failed |= result.returncode != 0
    if snapshot_after_tests(harnesses) != plan["sourceDigest"]:
        raise ValueError("Shard tests modified their captured source")
    document = {"schema": 1, "origin": origin(), "planSha256": sha(encoded(plan)),
                "sourceDigest": plan["sourceDigest"], "shard": shard, "invocations": invocations}
    write(OUT / f"shard-{shard}.json", document)
    # Write a complete failure report before failing the job; merger must never
    # turn a canceled, missing, failed or partially completed shard green.
    if failed:
        raise SystemExit(1)


def validate_results(plan, shards):
    validate_plan(plan)
    if len(shards) != SHARDS or {row.get("shard") for row in shards} != set(range(SHARDS)):
        raise ValueError("Missing or duplicate race shard")
    results = []
    for shard in sorted(shards, key=lambda value: value["shard"]):
        index = shard["shard"]
        if shard.get("origin") != plan["origin"] or shard.get("planSha256") != sha(encoded(plan)) or shard.get("sourceDigest") != plan["sourceDigest"]:
            raise ValueError("Foreign, stale or mismatched shard")
        expected = package_groups(plan, index)
        actual = shard["invocations"]
        if len(actual) != len(expected):
            raise ValueError("Missing or extra package invocation")
        for want, got in zip(expected, actual, strict=True):
            if any(got.get(key) != value for key, value in want.items()):
                raise ValueError("Package/function coverage differs from the complete plan")
            pattern = "^(" + "|".join(re.escape(name) for name in want["names"]) + ")$" if want["names"] else "^$"
            if got.get("flags") != [*FLAGS, "-run", pattern, want["package"]] or got.get("exitCode") != 0:
                raise ValueError("Race invocation failed or altered instrumentation")
            events = got["events"]
            if any(event.get("Package") != want["package"] or event["Action"] == "fail" for event in events):
                raise ValueError("Failed or foreign test event")
            package_terminal = [event["Action"] for event in events if "Test" not in event and event["Action"] in ("pass", "skip")]
            if package_terminal != ["pass"]:
                raise ValueError("Package was skipped or never completed")
            starts = [event["Test"] for event in events if event["Action"] == "run" and "/" not in event.get("Test", "")]
            finishes = [event["Test"] for event in events if event["Action"] in ("pass", "skip") and "/" not in event.get("Test", "") and "Test" in event]
            if sorted(starts) != want["names"] or sorted(finishes) != want["names"]:
                raise ValueError("A Test, Example or Fuzz function was missing, duplicated or not completed")
            skipped = [event["Test"] for event in events if event["Action"] == "skip" and "Test" in event]
            if ([row.get("test") for row in got.get("skips", [])] != skipped
                    or any(not row.get("reason", "").strip() for row in got.get("skips", []))):
                raise ValueError("Source-declared skips must retain their actual reasons")
            results.append({"shard": index, **want, "flags": got["flags"], "exitCode": 0,
                            "events": events, "skips": got.get("skips", []), "logSha256": got["logSha256"]})
    return {"schema": 1, "result": "passed", "planSha256": sha(encoded(plan)),
            "plan": plan, "results": results,
            "sourceDeclaredSkips": [{"package": row["package"], **skip}
                                    for row in results for skip in row["skips"]],
            "functions": {"discovered": sum(len(p["tests"]) for p in plan["packages"]),
                          "passed": sum(event["Action"] == "pass" and "Test" in event
                                        and "/" not in event["Test"] for row in results for event in row["events"]),
                          "skipped": sum(event["Action"] == "skip" and "Test" in event
                                         and "/" not in event["Test"] for row in results for event in row["events"])}}


def api(path):
    return json.loads(subprocess.check_output(["gh", "api", path], text=True))


def pages(path, field):
    result = []
    for page in range(1, 21):
        data = api(path + ("&" if "?" in path else "?") + f"per_page=100&page={page}")[field]
        result.extend(data)
        if len(data) < 100:
            return result
    raise ValueError("Unexpectedly large Actions evidence listing")


def fetch_artifact(name, inventory):
    matches = [row for row in inventory if row["name"] == name]
    if len(matches) != 1:
        raise ValueError("Missing or duplicate Actions artifact: " + name)
    artifact = matches[0]
    current = origin()
    run = artifact.get("workflow_run", {})
    if artifact.get("expired") or str(run.get("id")) != current["GITHUB_RUN_ID"] or run.get("head_sha") != current["GITHUB_SHA"]:
        raise ValueError("Artifact belongs to another/fork/expired run")
    expected_file = ("plan.json" if name.startswith("server-race-plan-") else
                     "qualification.json" if name.startswith("server-race-qualification-") else
                     f"shard-{name.split('-')[-1]}.json")
    with tempfile.TemporaryFile() as temporary:
        subprocess.run(["gh", "api", f"repos/{REPOSITORY}/actions/artifacts/{artifact['id']}/zip"], stdout=temporary, check=True)
        temporary.seek(0)
        digest = hashlib.file_digest(temporary, "sha256").hexdigest()
        if artifact.get("digest") != "sha256:" + digest:
            raise ValueError("Actions archive digest mismatch")
        temporary.seek(0)
        with zipfile.ZipFile(temporary) as archive:
            if archive.namelist() != [expected_file]:
                raise ValueError("Unexpected artifact payload or path")
            document = json.loads(archive.read(expected_file))
    return document, {"id": artifact["id"], "name": name, "digest": artifact["digest"]}


def fetched_report(require_merge=False):
    current = origin()
    run_id, attempt = current["GITHUB_RUN_ID"], current["GITHUB_RUN_ATTEMPT"]
    run = api(f"repos/{REPOSITORY}/actions/runs/{run_id}")
    if (run["head_sha"] != current["GITHUB_SHA"] or str(run["run_attempt"]) != attempt
            or run["head_repository"]["full_name"].lower() != REPOSITORY.lower()
            or run["repository"]["full_name"].lower() != REPOSITORY.lower()
            or run["event"] != current["GITHUB_EVENT_NAME"]):
        raise ValueError("Actions run origin changed or belongs to a fork")
    jobs = pages(f"repos/{REPOSITORY}/actions/runs/{run_id}/attempts/{attempt}/jobs", "jobs")
    suffixes = ["server-race-prepare", *(f"server-race-shard-{index}" for index in range(SHARDS))]
    if require_merge:
        suffixes.append("server-race-merge")
    evidence_jobs = []
    for suffix in suffixes:
        matches = [job for job in jobs if job["name"] == suffix or job["name"].endswith(" / " + suffix)]
        if len(matches) != 1 or matches[0]["status"] != "completed" or matches[0]["conclusion"] != "success":
            raise ValueError("Required race job failed, canceled, skipped or missing: " + suffix)
        evidence_jobs.append({"id": matches[0]["id"], "name": matches[0]["name"], "conclusion": "success"})
    inventory = pages(f"repos/{REPOSITORY}/actions/runs/{run_id}/artifacts", "artifacts")
    plan, plan_origin = fetch_artifact(f"server-race-plan-{attempt}", inventory)
    shards, artifact_origins = [], [plan_origin]
    for index in range(SHARDS):
        result, result_origin = fetch_artifact(f"server-race-shard-{attempt}-{index}", inventory)
        shards.append(result)
        artifact_origins.append(result_origin)
    report = validate_results(plan, shards)
    report["evidence"] = {"artifacts": artifact_origins, "jobs": evidence_jobs}
    return report, inventory


def merge():
    report, _ = fetched_report()
    write(OUT / "qualification.json", report)


def authenticate(output):
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    report, inventory = fetched_report(require_merge=True)
    attempt = origin()["GITHUB_RUN_ATTEMPT"]
    published, custody = fetch_artifact(f"server-race-qualification-{attempt}", inventory)
    # Merger could not claim its own terminal status before completing. All other
    # contents must match a fresh, authenticated download of every input artifact.
    expected = json.loads(encoded(report))
    expected["evidence"]["jobs"] = [row for row in expected["evidence"]["jobs"]
                                     if not row["name"].endswith("server-race-merge")]
    if published != expected:
        raise ValueError("Merged qualification differs from authenticated shard evidence")
    report["authentication"] = custody
    key_path = pathlib.Path(os.environ["PORTICO_RELEASE_SIGNING_KEY_FILE"])
    public_path = pathlib.Path(os.environ["PORTICO_RELEASE_PUBLIC_KEY_FILE"])
    if key_path.is_symlink() or key_path.stat().st_mode & 0o077:
        raise ValueError("Release signer is not protected")
    key = Ed25519PrivateKey.from_private_bytes(key_path.read_bytes())
    if key.public_key().public_bytes_raw() != public_path.read_bytes():
        raise ValueError("Approved signer and independent public pin differ")
    write(output, report)
    output.with_suffix(".sig").write_bytes(key.sign(output.read_bytes()))
    print(f"Authenticated all {len(report['plan']['packages'])} packages and {sum(len(p['tests']) for p in report['plan']['packages'])} Test/Example/Fuzz functions for this exact source/workflow/run/attempt.", flush=True)


def verify_report(path, public, expected_source, expected_builder):
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
    raw = path.read_bytes()
    Ed25519PublicKey.from_public_bytes(public).verify(path.with_suffix(".sig").read_bytes(), raw)
    document = json.loads(raw)
    plan = document["plan"]
    validate_plan(plan)
    if (document.get("result") != "passed" or document.get("planSha256") != sha(encoded(plan))
            or plan["sourceDigest"] != expected_source
            or plan["codeFiles"]["scripts/build-server-release.py"] != expected_builder
            or not document.get("authentication")):
        raise ValueError("Authenticated race qualification does not match this source/builder")
    # Reconstruct every shard from authenticated results and recheck exact
    # coverage rather than trusting a caller's passed/count boolean.
    shards = [{"schema": 1, "origin": plan["origin"], "sourceDigest": plan["sourceDigest"],
               "planSha256": sha(encoded(plan)), "shard": index,
               "invocations": [row for row in document["results"] if row["shard"] == index]}
              for index in range(SHARDS)]
    verified = validate_results(plan, shards)
    if any(verified[key] != document.get(key) for key in ("results", "sourceDeclaredSkips", "functions")):
        raise ValueError("Authenticated result inventory was reordered or replaced")
    return document


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("phase", choices=("prepare", "shard", "merge", "authenticate"))
    parser.add_argument("--shard", type=int)
    parser.add_argument("--plan", type=pathlib.Path, default=OUT / "plan.json")
    parser.add_argument("--output", type=pathlib.Path, default=OUT / "authenticated-qualification.json")
    args = parser.parse_args()
    if args.phase == "prepare":
        prepare()
    elif args.phase == "shard":
        if args.shard is None or not 0 <= args.shard < SHARDS:
            parser.error("shard must be in 0..15")
        run_shard(args.shard, args.plan)
    elif args.phase == "merge":
        merge()
    else:
        authenticate(args.output)


if __name__ == "__main__":
    main()
