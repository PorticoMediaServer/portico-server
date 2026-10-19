#!/usr/bin/env python3
"""Inspect actual installers/archives against their independently pinned build seal."""
import hashlib
import importlib.util
import json
import os
import pathlib
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location("release_builder", ROOT / "scripts/build-server-release.py")
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


def verify_payload(directory, version, target, public):
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
    raw = (directory / "manifest.json").read_bytes()
    Ed25519PublicKey.from_public_bytes(public).verify((directory / "manifest.sig").read_bytes(), raw)
    manifest = json.loads(raw)
    if manifest["version"] != version or manifest["target"] != target:
        raise ValueError("Packaged version or architecture differs")
    binary = directory / manifest["artifact"]["path"]
    if hashlib.file_digest(binary.open("rb"), "sha256").hexdigest() != manifest["artifact"]["sha256"]:
        raise ValueError("Packaged executable differs from sealed artifact")
    embedded, settings = builder.artifact_identity(binary)
    prefix = "portico.local/server/internal/buildinfo."
    if embedded[prefix + "Version"] != version or embedded[prefix + "SourceDigest"] != manifest["sourceDigest"]:
        raise ValueError("Embedded executable identity differs from manifest")
    if settings.get("GOOS") + "-" + settings.get("GOARCH") != target or settings.get("-tags") != "release":
        raise ValueError("Packaged executable has unexpected compiler target")
    if embedded.get("portico.local/server/internal/hostedtrust.OfficialRoot") != builder.official_root()[0]:
        raise ValueError("Packaged executable changed official Hosted trust")
    if json.loads((directory / "web/portico-build.json").read_text())["version"] != version:
        raise ValueError("Packaged web version differs")
    if not (directory / "web/index.html").is_file():
        raise ValueError("Web application missing")
    bundle = directory / "third_party/ffmpeg"
    subprocess.run(["python3", str(ROOT / "scripts/verify-ffmpeg-manifest.py"), str(bundle)], check=True)
    if hashlib.sha256((bundle / "toolchain-manifest.json").read_bytes()).hexdigest() != manifest["toolchainManifestSha256"]:
        raise ValueError("Packaged media-tool manifest differs from source seal")
    print(f"Verified packaged {target} {version}: {binary}")


def main():
    os_name, arch, version, output = sys.argv[1:]
    output = pathlib.Path(output).resolve()
    target = os_name + "-" + arch
    public = pathlib.Path(os.environ["PORTICO_RELEASE_PUBLIC_KEY_FILE"]).read_bytes()
    file_arch = "x64" if arch == "amd64" else arch
    if os_name == "darwin":
        binaries = list(output.rglob("portico-server"))
        if len(binaries) != 1:
            raise ValueError("Expected exactly one expanded macOS server")
        verify_payload(binaries[0].parent, version, target, public)
        return
    assets = ([output / f"portico-media-server-linux-{file_arch}.{ext}" for ext in ("tar.gz", "deb", "rpm")]
              if os_name == "linux" else
              [output / f"Portico-Media-Server-Windows-{file_arch}-{suffix}" for suffix in ("Portable.zip", "Setup.exe")])
    executable = "portico-server.exe" if os_name == "windows" else "portico-server"
    for asset in assets:
        with tempfile.TemporaryDirectory() as tmp:
            if asset.suffix == ".deb":
                assert subprocess.check_output(["dpkg-deb", "-f", str(asset), "Version"], text=True).strip() == version
                subprocess.run(["dpkg-deb", "-x", str(asset), tmp], check=True)
            elif asset.suffix == ".rpm":
                assert subprocess.check_output(["rpm", "-qp", "--qf", "%{VERSION}", str(asset)], text=True).strip() == version
                subprocess.run(["bsdtar", "-xf", str(asset), "-C", tmp], check=True)
            elif asset.suffix == ".exe":
                subprocess.run(["7z", "x", "-y", f"-o{tmp}", str(asset)], stdout=subprocess.DEVNULL, check=True)
            else:
                subprocess.run(["bsdtar", "-xf", str(asset), "-C", tmp], check=True)
            binaries = list(pathlib.Path(tmp).rglob(executable))
            if len(binaries) != 1:
                raise ValueError(f"Expected exactly one packaged executable in {asset}: {binaries}")
            verify_payload(binaries[0].parent, version, target, public)


if __name__ == "__main__":
    main()
