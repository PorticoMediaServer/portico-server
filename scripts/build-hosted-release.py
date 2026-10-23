#!/usr/bin/env python3
"""Build publishable Hosted, DNS and Web artifacts from one captured source tree."""
import argparse
import datetime
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
# The web app Hosted serves (web.getportico.tv) is built from the server repository.
SERVER = Path(os.environ.get("PORTICO_SERVER_REPO", ROOT.parent / "apps/portico-server")).resolve()
EXCLUDED = {"node_modules", "dist", "build", ".git", ".cache", "__pycache__"}
# (repository, path): both land at the same relative path in the snapshot.
INPUTS = [(ROOT, "hosted-services/hosted"), (ROOT, "hosted-services/dns"), (ROOT, "hosted-services/apikit"),
          (ROOT, "scripts/build-hosted-release.py"), (SERVER, "web"), (SERVER, "packages/client-core"),
          (SERVER, "packages/contracts"),
          (SERVER, "packages/design"), (SERVER, "packages/i18n"), (SERVER, "third_party/hls.js"),
          (SERVER, "third_party/libass-wasm"), (SERVER, "third_party/subtitle-fonts"),
          (SERVER, "package.json"), (SERVER, "package-lock.json"), (SERVER, "scripts/build-server-release.py")]


def release_inputs(web_ui="legacy"):
    if web_ui == "legacy":
        return INPUTS
    if web_ui != "web-next":
        raise ValueError("Unknown Hosted web input")
    # npm's root workspace graph still needs the legacy package manifest, but no
    # legacy UI source is part of the independent Services entry build.
    return [(repo, name) for repo, name in INPUTS if (repo, name) != (SERVER, "web")] + [
        (SERVER, "web/package.json"), (SERVER, "web-next"),
        (SERVER, "packages/player-core"), (SERVER, "packages/reader-core"), (SERVER, ".node-version")]


def inventory(root=None, web_ui="legacy"):
    """Every input file with its digest; root=None reads the two repositories, else a snapshot."""
    entries = []
    for repo, name in release_inputs(web_ui):
        base = (root or repo) / name
        if not base.exists():
            raise ValueError("Missing release input: " + name)
        for path in sorted(base.rglob("*") if base.is_dir() else [base]):
            relative = path.relative_to(root or repo)
            if any(part in EXCLUDED for part in relative.parts):
                continue
            if path.suffix == ".tsbuildinfo" or path.name == ".DS_Store":
                continue
            if path.is_symlink():
                raise ValueError("Release input must not be a symlink: " + str(relative))
            if path.is_file():
                entries.append({"path": str(relative), "sha256": hashlib.sha256(path.read_bytes()).hexdigest()})
    return entries


def source_digest(entries):
    return hashlib.sha256(json.dumps(entries, separators=(",", ":"), sort_keys=True).encode()).hexdigest()


def source_file(relative, web_ui="legacy"):
    for repo, name in release_inputs(web_ui):
        if relative == name or relative.startswith(name + "/"):
            return repo / relative
    raise ValueError("Not a release input: " + relative)


def capture(destination, web_ui="legacy"):
    before = inventory(web_ui=web_ui)
    for entry in before:
        target = destination / entry["path"]
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source_file(entry["path"], web_ui), target)
    if inventory(destination, web_ui) != before or inventory(web_ui=web_ui) != before:
        raise ValueError("Source changed during capture; retry after edits settle")
    return before


def run(args, cwd, env=None):
    subprocess.run(args, cwd=cwd, env=env, check=True)


def build_web(snapshot, destination, web_ui="legacy", *, hosted_origin="https://web.getportico.tv"):
    release_inputs(web_ui)  # Refuse unknown input before installing or building.
    web_env = {k: v for k, v in os.environ.items() if not k.startswith("VITE_") and k != "NODE_OPTIONS"}
    if web_ui == "web-next":
        wanted = "v" + (snapshot / ".node-version").read_text().strip()
        actual = subprocess.check_output(["node", "--version"], cwd=snapshot, env=web_env, text=True).strip()
        if actual != wanted:
            raise ValueError("Hosted web-next requires the captured .node-version")
    run(["npm", "ci", "--ignore-scripts", "--no-audit", "--no-fund"], snapshot, web_env)
    # The release CLI keeps the production default. Isolated composition checks
    # can build against their real loopback API without rewriting built assets.
    web_env.update(VITE_HOSTED_WEB="true", VITE_ACCOUNT_WORKSPACE="true", VITE_HOSTED_URL=hosted_origin, PORTICO_RELEASE_WEB=str(destination))
    code = "import {build} from 'vite'; await build({build:{outDir:process.env.PORTICO_RELEASE_WEB,emptyOutDir:true,sourcemap:false}});"
    run(["node", "--input-type=module", "-e", code], snapshot / ("web-next" if web_ui == "web-next" else "web"), web_env)


def main():
    spec = importlib.util.spec_from_file_location("server_release", SERVER / "scripts/build-server-release.py")
    release = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(release)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True, type=release.release_version)
    parser.add_argument("--web-ui", choices=("legacy", "web-next"), default="legacy")
    args = parser.parse_args()
    root_key, root_pem = release.official_root()
    now = datetime.datetime.now(datetime.timezone.utc)
    commit = {"internal": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip(),
              "server": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=SERVER, text=True).strip()}
    # No build reads the mutable checkout after this capture. npm installs only
    # from the captured lockfile and tarball into the private build workspace.
    with tempfile.TemporaryDirectory(prefix="portico-hosted-build-") as temporary:
        snapshot = Path(temporary)
        inputs = capture(snapshot, args.web_ui)
        inputs.append({"path":"deployment/hosted/root-public-key.pem","sha256":hashlib.sha256(root_pem).hexdigest()})
        source = source_digest(inputs)
        build_id = "portico-" + now.strftime("%Y%m%dT%H%M%SZ") + "-" + source[:12]
        out = ROOT / "runtime/hosted-releases" / build_id
        out.mkdir(parents=True)
        env = release.controlled_env()
        env.update(GOOS="linux", GOARCH="amd64", CGO_ENABLED="0")
        fields = {"Version": args.version, "BuildID": build_id, "SourceDigest": source,
                  "BuiltAt": now.isoformat(), "Proof": "unsealed"}
        flags = "-s -w " + " ".join("-X portico.local/hosted/internal/buildinfo." + k + "=" + v for k, v in fields.items())
        flags += " -X portico.local/hosted/trust.OfficialRoot=" + root_key
        run(["go", "mod", "verify"], snapshot / "hosted-services/hosted", env)
        run(["go", "mod", "verify"], snapshot / "hosted-services/dns", env)
        run(["go", "build", "-tags", "release", "-buildvcs=false", "-mod=readonly", "-trimpath", "-ldflags", flags, "-o", str(out / "hosted"), "./cmd/hosted"], snapshot / "hosted-services/hosted", env)
        run(["go", "build", "-buildvcs=false", "-mod=readonly", "-trimpath", "-ldflags", "-s -w", "-o", str(out / "web-host"), "./cmd/web"], snapshot / "hosted-services/hosted", env)
        run(["go", "build", "-buildvcs=false", "-mod=readonly", "-trimpath", "-ldflags", "-s -w -X main.buildID=" + build_id,
             "-o", str(out / "portico-dns"), "./cmd/portico-dns"], snapshot / "hosted-services/dns", env)
        build_web(snapshot, out / "web", args.web_ui)
        if inventory(snapshot, args.web_ui) != inputs[:-1]:
            raise ValueError("Captured source changed during build; discard output")
        manifest = {"version": args.version, "buildId": build_id, "sourceDigest": source, "builtAt": now.isoformat(),
                    "gitCommit": commit, "sourceFiles": inputs, "buildConfiguration": {"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0", "hostedWeb": True, "hostedWebUI": args.web_ui},
                    "includesWorkingTree": True, "proof": "unsealed", "hostedOrigin": "https://web.getportico.tv"}
        (out / "web/portico-build.json").write_text(json.dumps(manifest, indent=2) + "\n")
        manifest["files"] = {str(p.relative_to(out)): hashlib.sha256(p.read_bytes()).hexdigest()
                             for p in sorted(out.rglob("*")) if p.is_file()}
        (out / "release.json").write_text(json.dumps(manifest, indent=2) + "\n")
        with tarfile.open(str(out) + ".tar.gz", "w:gz") as archive:
            for name in ["hosted", "web-host", "portico-dns", "web", "release.json"]:
                archive.add(out / name, arcname=name)
        print(json.dumps({"release": str(out), "archive": str(out) + ".tar.gz", "buildId": build_id}))


if __name__ == "__main__":
    main()
