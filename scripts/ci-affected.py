#!/usr/bin/env python3
"""Full hourly checks, with bounded selection retained for explicit local use."""
import argparse
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
TEST = re.compile(r"^func (Test[A-Z]\w*)\(", re.M)


# Per-command bound. This is a hang guard for one suite invocation, not a scope
# filter or a test deadline: the full-tree and mandatory security commands stay
# exactly as selected, and each still gets the whole window.
COMMAND_TIMEOUT = 15 * 60
TERMINATE_GRACE = 5


class CommandTimeout(subprocess.CalledProcessError):
    """A command outlived COMMAND_TIMEOUT; its process group was terminated."""

    def __init__(self, argv, timeout):
        super().__init__(124, argv)  # 124 is the conventional timeout exit status
        self.timeout = timeout


def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT, text=True).splitlines()


def changed():
    if os.environ.get("CI_FULL_TREE") == "1":
        return set(git("ls-files"))
    base = os.environ.get("CI_BASE", "")
    if not base or set(base) == {"0"}:
        base = "HEAD^"
    # Missing history is an error, never permission to omit affected checks.
    return set(git("diff", "--name-only", "--diff-filter=ACMRTD", base, "HEAD", "--"))


def _signal_group(process, sig):
    try:
        os.killpg(process.pid, sig)
    except ProcessLookupError:
        pass


def _group_alive(process):
    try:
        os.killpg(process.pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def _await_group(process, seconds):
    """Wait up to `seconds` for the whole command group to empty, reaping its leader."""
    deadline = time.monotonic() + seconds
    while _group_alive(process):
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            return False
        process.poll()  # reap the leader as soon as it exits
        time.sleep(min(0.05, remaining))
    process.wait()
    return True


def _stop_group(process):
    """TERM the command's own group, give it a bounded grace, then KILL what remains."""
    _signal_group(process, signal.SIGTERM)
    _await_group(process, TERMINATE_GRACE)
    # The grace is for the group, not only its leader: a leader that exits on TERM
    # must not hide a descendant that ignores it, so the group is always confirmed
    # with KILL. Both waits stay bounded, and a command that cannot be killed is
    # reported by the failing timeout rather than hanging this runner again.
    _signal_group(process, signal.SIGKILL)
    _await_group(process, TERMINATE_GRACE)


def run(args, cwd=ROOT, timeout=None):
    argv = list(map(str, args))
    limit = COMMAND_TIMEOUT if timeout is None else timeout
    print("+", " ".join(argv), flush=True)
    process = subprocess.Popen(argv, cwd=cwd, start_new_session=True)
    try:
        code = process.wait(timeout=limit)
    except subprocess.TimeoutExpired:
        _stop_group(process)
        print(f"timed out after {limit:g}s, command terminated: {' '.join(argv)}",
              file=sys.stderr, flush=True)
        raise CommandTimeout(argv, limit) from None
    if code:
        raise subprocess.CalledProcessError(code, argv)
    return code


def run_all(commands):
    failed = False
    timed_out = False
    for args, cwd in commands:
        try:
            run(args, cwd)
        except subprocess.CalledProcessError as error:
            failed = True
            timed_out = timed_out or isinstance(error, CommandTimeout)
    if failed:
        raise SystemExit(124 if timed_out else 1)


def test_names(files):
    return {name for file in files if file.is_file()
            for name in TEST.findall(file.read_text()) if name != "TestMain"}


def go_plan(module, paths):
    prefix = module + "/"
    selected = {}
    for name in sorted(paths):
        if not name.startswith(prefix) or not name.endswith(".go"):
            continue
        file = ROOT / name
        directory = file.parent
        if not directory.is_dir():
            continue
        package = "./" + str(directory.relative_to(ROOT / module))
        tests = list(directory.glob("*_test.go"))
        if name.endswith("_test.go"):
            candidates = [file]
        else:
            candidates = list(directory.glob(file.stem + "*_test.go"))
            if not candidates:
                # Catalogue is deliberately never expanded to its full suite on a push.
                # Check compile/vet here; named counterparts and all package tests run nightly.
                candidates = [] if package == "./internal/catalog" else tests
        selected.setdefault(package, set()).update(test_names(candidates))
    if paths & {module + "/go.mod", module + "/go.sum"}:
        selected["./..."] = set()  # compile the changed module dependency graph
    return selected


def go_checks(module, group):
    if os.environ.get("CI_FULL_TREE") == "1" and group == "affected":
        vet = ["bash", str(ROOT / "scripts/vet-go.sh")] if module == "server" else ["go", "vet", "./..."]
        run_all([(["go", "test", "-count=1", "./..."], ROOT / module), (vet, ROOT / module)])
        return
    if group == "affected":
        selected = go_plan(module, changed())
    elif group == "auth":
        selected = {"./internal/httpapi": {
            "TestRefreshRoundTripOverPlainHTTP",
            "TestUnknownAndKnownNamesHaveTheSameSignInCooloff",
            "TestAccountRestoreUsesAccountScopeUntilExplicitProfileSelection",
            "TestSessionFamilyHTTPLogoutAuthenticGenerations",
            "TestASecondProfileOfTheOwnersAccountIsNotAnAdministrator",
            "TestRestrictedProfileRouteMatrix", "TestRestrictedProfileWriteRouteMatrix",
        }}
    elif group == "media":
        selected = {
            "./internal/mediaexec": test_names((ROOT / module / "internal/mediaexec").glob("*_test.go")),
            "./internal/remotemedia": {
                "TestPrivateRootsRedirectsAndMetadataAreExplicitlyFenced",
                "TestDNSChangesAreRevalidatedAgainstApprovedAddresses",
                "TestManualRedirectsReturnEachHopWithoutContactingDestination",
                "TestPolicyProxyRejectsRebindingAndUnauthenticatedRequests",
                "TestServeEndpointRejectsMembersBeforeDispatch",
                "TestServeDialChecksExactOriginAndReconnectBeforeNetwork",
            },
        }
    elif group == "hosted":
        selected = {
            "./internal/identity": test_names((ROOT / module / "internal/identity").glob("*_test.go")),
            "./internal/egress": test_names((ROOT / module / "internal/egress").glob("*_test.go")),
            "./internal/security": test_names((ROOT / module / "internal/security").glob("*_test.go")),
            "./internal/httpapi": {
                "TestBrowserCookieOriginsAndFamilyIntentAreIsolated",
                "TestIdentityContinuationReadIsNotMediaOriginAuthority",
                "TestSessionReviewQueryAllowlist",
            },
        }
    else:
        raise ValueError(group)
    commands = []
    for package, names in sorted(selected.items()):
        if group != "affected":
            actual = test_names((ROOT / module / package.removeprefix("./")).glob("*_test.go"))
            if not names or not names <= actual:
                raise RuntimeError(f"missing mandatory security test: {package}: {names - actual}")
        pattern = "^(" + "|".join(sorted(names)) + ")$" if names else "^$"
        commands.append((["go", "test", "-count=1", "-run", pattern, package], ROOT / module))
        vet = ["go", "vet"]
        if package == "./internal/thirdparty/sqlite":
            vet.append("-unsafeptr=false")  # same maintained-driver exception as vet-go.sh
        commands.append(([*vet, package], ROOT / module))
    run_all(commands)


def dependencies(file, known, workspaces):
    result = set()
    # Include imports, requires, new URL(..., import.meta.url), and bounded harness module maps.
    for spec in re.findall(r"['\"]([^'\"\n]+)['\"]", file.read_text(errors="replace")):
        if spec.startswith("."):
            base = file.parent / spec
        elif spec.startswith("@core/"):
            base = ROOT / "packages/client-core/src" / spec[6:]
        elif spec in {"@core", "@design", "@i18n"}:
            package = {"@core": "client-core", "@design": "design", "@i18n": "i18n"}[spec]
            base = ROOT / "packages" / package / "src/index.ts"
        elif spec.startswith("@/") and file.is_relative_to(ROOT / "web-next"):
            base = ROOT / "web-next/src" / spec[2:]
        else:
            workspace = next((name for name in workspaces if spec == name or spec.startswith(name + "/")), None)
            if not workspace:
                continue
            directory = workspaces[workspace]
            exports = json.loads((directory / "package.json").read_text()).get("exports", {})
            key = "." + spec[len(workspace):]
            target = exports if isinstance(exports, str) and key == "." else exports.get(key) if isinstance(exports, dict) else None
            if isinstance(target, dict):
                target = target.get("import") or target.get("default") or target.get("types")
            base = directory / (target if isinstance(target, str) else spec[len(workspace):].lstrip("/") or "src/index")
        base = base.resolve()
        candidates = [base, *(Path(str(base) + ext) for ext in (".ts", ".tsx", ".js", ".mjs", ".cjs")),
                      *(base / ("index" + ext) for ext in (".ts", ".tsx", ".js", ".mjs"))]
        if base.suffix in {".js", ".jsx"}:
            candidates.extend([base.with_suffix(".ts"), base.with_suffix(".tsx")])
        result.update(candidate for candidate in candidates if candidate in known)
    return result


def js_plan(paths, native=False):
    files = {ROOT / name for name in git("ls-files")
             if Path(name).suffix in {".ts", ".tsx", ".js", ".mjs", ".cjs", ".json"}
             and (ROOT / name).is_file()}
    workspaces = {}
    for file in files:
        if file.name == "package.json":
            name = json.loads(file.read_text()).get("name")
            if name:
                workspaces[name] = file.parent
    impacted = {ROOT / name for name in paths}
    for name in paths:
        file = ROOT / name
        if file.name in {"package.json", "package-lock.json", "register.mjs"}:
            directory = file.parent
            impacted.update(p for p in files if p.is_relative_to(directory))
    edges = {file: dependencies(file, files, workspaces) for file in files if file.suffix != ".json"}
    while True:
        more = {file for file, deps in edges.items() if deps & impacted} - impacted
        if not more:
            break
        impacted.update(more)
    tests = {p for p in impacted if re.search(r"\.test\.(?:[cm]?js|tsx?)$", str(p)) and p.is_file()}
    mandatory = (["apps/native/test/next-accounts-entry.test.ts", "apps/native/test/next-local-session-renewal.test.ts",
                  "apps/native/test/next-account-proof-entry.test.ts", "apps/native/test/next-shell-tv-profile-menu.test.ts"] if native else [
        "packages/client-core/test/account-sessions.test.ts", "packages/client-core/test/session-refresh.test.ts",
        "packages/client-core/test/identity-restrictions.test.ts", "packages/client-core/test/profiles-membership.test.ts",
        "packages/client-core/test/generated-routes.test.ts", "packages/client-core/test/credential-scoped-logout.test.ts",
        "web-next/test/account-entry-authority.test.ts", "web-next/test/viewer-storage.test.ts",
    ])
    for name in mandatory:
        if not (ROOT / name).is_file():
            raise RuntimeError("missing mandatory security test: " + name)
        tests.add(ROOT / name)
    return sorted(tests)


def js_checks(native):
    paths = changed()
    groups = {}
    for file in js_plan(paths, native):
        relative = file.relative_to(ROOT)
        if native and relative.parts[:3] == ("apps", "native", "test"):
            group = "apps/native"
        elif str(relative).startswith("web-next/test/"):
            group = "web-next"
        elif str(relative).startswith("web/test/"):
            group = "web"
        elif str(relative).startswith("packages/"):
            group = "/".join(relative.parts[:2])
        elif str(relative).startswith("server/receiver/cast/test/"):
            group = "."
        else:
            continue
        groups.setdefault(group, []).append(str(file.relative_to(ROOT / group)))
    commands = []
    if native:
        source = [str(Path(p).relative_to("apps/native")) for p in sorted(paths)
                  if p.startswith("apps/native/") and p.endswith((".ts", ".tsx"))
                  and (ROOT / p).is_file() and (p.startswith("apps/native/src-next/") or p == "apps/native/App.tsx")]
        if source:
            commands.append((["npx", "--no-install", "eslint", "--no-eslintrc", "-c", ".eslintrc.hooks.cjs", *source], ROOT / "apps/native"))
    for group, files in sorted(groups.items()):
        args = ["node", "--experimental-strip-types"]
        if group in ("apps/native", "web", "web-next"):
            args = ["node", "--experimental-transform-types", "--import", "./test/register.mjs"]
        commands.append(([*args, "--test-concurrency=1", "--test", *files], ROOT / group))
    run_all(commands)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("kind", choices=["go", "js", "native-js"])
    parser.add_argument("--module", default="server")
    parser.add_argument("--group", choices=["affected", "auth", "media", "hosted"], default="affected")
    options = parser.parse_args()
    if options.kind == "go":
        go_checks(options.module, options.group)
    else:
        js_checks(options.kind == "native-js")
