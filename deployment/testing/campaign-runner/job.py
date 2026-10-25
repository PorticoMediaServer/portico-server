#!/usr/bin/env python3
"""Remote endpoint; host limits are configured beside this file."""
import fcntl
import datetime
import json
import os
from pathlib import Path
import signal
import shutil
import subprocess
import sys
import tempfile
import time


FOOTPRINT_SCAN_ATTEMPTS = 3


def footprint_bytes(paths):
    """Total bytes of the existing roots, or None when no full scan succeeds.

    Concurrent scratch cleanup can remove entries while du walks a tree. du then
    exits non-zero after printing a partial total, so the attempt is discarded
    and the whole set is rescanned. After a bounded number of attempts admission
    fails closed instead of trusting an incomplete footprint.
    """
    for attempt in range(FOOTPRINT_SCAN_ATTEMPTS):
        total = 0
        complete = True
        for path in paths:
            try:
                if not Path(path).exists():
                    continue
                scan = subprocess.run(["du", "-sb", "--", str(path)],
                                      capture_output=True, text=True)
            except OSError:
                complete = False
                break
            if scan.returncode:
                complete = False
                break
            try:
                total += int(scan.stdout.split()[0])
            except (IndexError, ValueError):
                complete = False
                break
        if complete:
            return total
        if attempt + 1 < FOOTPRINT_SCAN_ATTEMPTS:
            time.sleep(0.2 * (attempt + 1))
    return None


def reserve(slots, numbers, prefix="slot"):
    for number in numbers:
        candidate = os.open(slots / ("%s-%d.lock" % (prefix, number)), os.O_RDWR | os.O_CREAT, 0o600)
        try:
            fcntl.flock(candidate, fcntl.LOCK_EX | fcntl.LOCK_NB)
            return candidate, number
        except BlockingIOError:
            os.close(candidate)
    return None, None


def wait_child(child, soft=None, hard=None):
    """Stop the owned foreground process group at soft/hard bounds."""
    def stop(sig):
        try:
            os.killpg(child.pid, sig)
        except ProcessLookupError:
            pass
    try:
        try:
            return child.wait(timeout=soft)
        except subprocess.TimeoutExpired:
            print("TIMEOUT: deadline reached; terminating owned job", flush=True)
            stop(signal.SIGTERM)
            try:
                child.wait(timeout=max(0, hard - soft))
            except subprocess.TimeoutExpired:
                stop(signal.SIGKILL)
                child.wait()
            return 124
    finally:
        # Quick jobs may not leave background work holding their reserved slot.
        if soft is not None:
            stop(signal.SIGKILL)


def run(spec, cfg):
    quick = bool(spec.get("quick"))
    emulator = bool(spec.get("emulator"))
    if quick and (spec.get("browser") or emulator):
        print("REFUSED: captures/emulators cannot use the quick slot", flush=True)
        return 75
    work = Path(cfg["workRoot"]).resolve()
    probe = bool(spec.get("probe"))
    root = work if probe else Path(spec["root"]).resolve()
    if not probe:
        root.relative_to(work / "jobs")
        cwd = (root / "source" / spec["cwd"]).resolve()
        cwd.relative_to(root / "source")
    soft, hard = (285, 300) if quick else (None, None)
    winddown = False
    if cfg.get("shutdownAt"):
        remaining = (datetime.datetime.fromisoformat(cfg["shutdownAt"].replace("Z", "+00:00"))
                     - datetime.datetime.now(datetime.timezone.utc)).total_seconds()
        if remaining <= 90:
            print("REFUSED: host weekend wind-down deadline", flush=True)
            return 75
        if soft is None or remaining - 90 < soft:
            soft, hard, winddown = remaining - 90, remaining - 30, True
    mem = {line.split(":")[0]: int(line.split()[1]) * 1024
           for line in Path("/proc/meminfo").read_text().splitlines()}
    if mem["MemAvailable"] < cfg["minMemoryBytes"]:
        print("REFUSED: insufficient MemAvailable; continue code-only", flush=True)
        return 75
    fs = os.statvfs(root)
    if fs.f_bavail * fs.f_frsize < cfg["minDiskFreeBytes"]:
        print("REFUSED: disk reserve", flush=True)
        return 75
    if cfg.get("maxLoad") and os.getloadavg()[0] > cfg["maxLoad"]:
        print("REFUSED: host load", flush=True)
        return 75
    if cfg.get("porticoCapBytes"):
        total = footprint_bytes(cfg["porticoRoots"])
        if total is None:
            print("REFUSED: Portico footprint unmeasurable", flush=True)
            return 75
        if total > cfg["porticoCapBytes"]:
            print("REFUSED: Portico footprint cap", flush=True)
            return 75
    slots = Path(cfg["slots"])
    slots.mkdir(parents=True, exist_ok=True)
    quick_slot = cfg.get("quickSlot", cfg["maxJobs"] - 1)
    general_slots = [n for n in range(cfg["maxJobs"]) if n != quick_slot]
    # The reserved slot protects short checks from long jobs. Short checks may
    # also use idle general capacity, within the same host-wide slot limit.
    numbers = [quick_slot, *general_slots] if quick else general_slots
    fd, number = reserve(slots, numbers)
    if fd is None:
        print("REFUSED: " + ("all quick/general slots occupied" if quick else "all general slots occupied") + "; continue code-only", flush=True)
        return 75
    emulator_fd = None
    browser_tmp = None
    try:
        if emulator:
            emulator_fd, emulator_number = reserve(slots, range(cfg.get("maxEmulators", 0)), "emulator")
            if emulator_fd is None:
                print("REFUSED: emulator slots occupied or unavailable", flush=True)
                return 75
        if probe:
            print(json.dumps({"available": True, "quick": quick, "slot": number}), flush=True)
            return 0
        env = os.environ.copy()
        env["PATH"] = cfg.get("extraPath", "") + ":" + env.get("PATH", "")
        env["TMPDIR"] = str(root / "tmp")
        Path(env["TMPDIR"]).mkdir(exist_ok=True)
        if spec.get("browser"):
            # Chromium appends its SingletonSocket path. The ordinary job path
            # can exceed Linux's 108-byte Unix-socket limit before launch.
            browser_parent = Path(cfg["workRoot"]) / "t"
            browser_parent.mkdir(mode=0o700, exist_ok=True)
            browser_tmp = Path(tempfile.mkdtemp(prefix="b-", dir=browser_parent))
            env["TMPDIR"] = str(browser_tmp)
        env.pop("PORTICO_RUNNER_SECONDARY_SOURCE", None)
        if spec.get("secondary"):
            secondary = root / "secondary-source"
            if secondary.is_symlink() or not secondary.is_dir():
                raise ValueError("secondary source archive missing")
            env["PORTICO_RUNNER_SECONDARY_SOURCE"] = str(secondary)
        env["PORTICO_RUNNER_TOOLS"] = str(Path(__file__).resolve().parent)
        env["PORTICO_RUNNER_OUTPUT"] = str(root / "outputs")
        Path(env["PORTICO_RUNNER_OUTPUT"]).mkdir(exist_ok=True)
        env["GOMAXPROCS"] = "2"
        env["GOFLAGS"] = "-p=1 -trimpath"
        cache = Path(cfg["workRoot"]) / "cache"
        cache.mkdir(exist_ok=True)
        env["GOCACHE"] = str(cache / "go-build")
        env["GOMODCACHE"] = str(cache / "go-mod")
        env["npm_config_cache"] = str(cache / "npm")
        os.ftruncate(fd, 0)
        os.write(fd, (json.dumps({"pid": os.getpid(), "commit": spec["commit"], "lane": spec["lane"],
                                 "quick": quick, "emulator": emulator}) + "\n").encode())
        print(json.dumps({"host": os.uname().nodename, "commit": spec["commit"], "slot": number,
                          "memAvailable": mem["MemAvailable"], "argv": spec["argv"],
                          "quick": quick, "emulator": emulator,
                          "softTimeoutSeconds": soft, "hardTimeoutSeconds": hard,
                          "browserTmp": str(browser_tmp) if browser_tmp else None}), flush=True)
        child = subprocess.Popen(spec["argv"], cwd=cwd, env=env, start_new_session=True,
                                 pass_fds=tuple(f for f in (fd, emulator_fd) if f is not None))
        def forward(sig, frame):
            try:
                os.killpg(child.pid, sig)
            except ProcessLookupError:
                pass
        signal.signal(signal.SIGTERM, forward)
        signal.signal(signal.SIGINT, forward)
        started = time.monotonic()
        code = wait_child(child, soft, hard)
        if winddown and time.monotonic() - started >= soft:
            # Deadline cancellation has no successful artifact to collect.
            shutil.rmtree(root)
        return code if code >= 0 else 128 - code
    finally:
        try:
            if browser_tmp is not None:
                shutil.rmtree(browser_tmp)
        finally:
            if emulator_fd is not None:
                os.close(emulator_fd)
            os.close(fd)


def main():
    cfg = json.loads(Path(__file__).with_name("limits.json").read_text())
    return run(json.loads(sys.argv[1]), cfg)


if __name__ == "__main__":
    sys.exit(main())
