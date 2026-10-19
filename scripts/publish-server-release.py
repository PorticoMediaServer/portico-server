#!/usr/bin/env python3
"""Publish a complete version once, reconcile GitHub bytes, then move latest."""
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys
import tempfile

REPOSITORY = "PorticoMediaServer/portico-server"
NATIVE_ASSETS = {
    "Portico-Media-Server-Windows-x64-Setup.exe",
    "Portico-Media-Server-Windows-x64-Portable.zip",
    "Portico-Media-Server-Windows-arm64-Setup.exe",
    "Portico-Media-Server-Windows-arm64-Portable.zip",
    "Portico-Media-Server-macOS-arm64.dmg",
    "Portico-Media-Server-macOS-arm64.pkg",
    *{f"portico-media-server-linux-{arch}.{extension}"
      for arch in ("x64", "arm64") for extension in ("tar.gz", "deb", "rpm")},
}
METADATA_ASSETS = {
    "checksums.sha256", "portico-update-manifest.json", "portico-release.spdx.json",
    "portico-release-public-key.bin", "release-manifest.json", "release-manifest.sig",
}


def run(*args):
    return subprocess.check_output(args, text=True).strip()


def api(endpoint, *args):
    return json.loads(run("gh", "api", f"repos/{REPOSITORY}/{endpoint}", *args))


def version_tuple(tag):
    if not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag):
        raise ValueError("Only complete stable version tags may become latest")
    return tuple(map(int, tag[1:].split(".")))


def digest(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def remote_tag(tag):
    rows = run("git", "ls-remote", "origin", f"refs/tags/{tag}", f"refs/tags/{tag}^{{}}").splitlines()
    refs = dict(row.split()[::-1] for row in rows)
    return refs.get(f"refs/tags/{tag}^{{}}", refs.get(f"refs/tags/{tag}"))


def verify_assets(release, expected, download=False):
    assets = {entry["name"]: entry for entry in release["assets"]}
    if set(assets) != set(expected):
        raise ValueError("GitHub release asset inventory differs from sealed local inventory")
    for name, row in expected.items():
        asset = assets[name]
        if asset["state"] != "uploaded" or asset["size"] != row["bytes"]:
            raise ValueError(f"Incomplete uploaded asset: {name}")
        if asset.get("digest") != "sha256:" + row["sha256"]:
            raise ValueError(f"GitHub asset digest differs: {name}")
        if download:
            # Stream the actual GitHub download without duplicating multi-GB packages on disk.
            with subprocess.Popen(
                ["gh", "api", f"repos/{REPOSITORY}/releases/assets/{asset['id']}",
                 "-H", "Accept: application/octet-stream"], stdout=subprocess.PIPE
            ) as process:
                checksum = hashlib.file_digest(process.stdout, "sha256").hexdigest()
                if process.wait() != 0 or checksum != row["sha256"]:
                    raise ValueError(f"Downloaded asset checksum differs: {name}")
            print(f"Verified GitHub download: {name}", flush=True)


def main():
    directory = pathlib.Path(sys.argv[1]).resolve()
    tag, sha, version = (os.environ[name] for name in ("TAG", "SOURCE_SHA", "VERSION"))
    wanted = version_tuple(tag)
    if tag != "v" + version or not re.fullmatch(r"[0-9a-f]{40}", sha) or remote_tag(tag) != sha:
        raise ValueError("Version tag does not identify the tested source")
    files = {path.name: path for path in directory.iterdir()}
    if set(files) != NATIVE_ASSETS | METADATA_ASSETS:
        raise ValueError("Release requires every native package and authenticated metadata")
    if any(path.is_symlink() or not path.is_file() or not path.stat().st_size for path in files.values()):
        raise ValueError("Release assets must be nonempty regular files")
    subprocess.run(
        [sys.executable, str(pathlib.Path(__file__).with_name("seal-release-artifacts.py")), str(directory),
         "--public-key", str(directory / "portico-release-public-key.bin"), "--verify", "--source", sha],
        check=True,
    )
    expected = {name: {"bytes": path.stat().st_size, "sha256": digest(path)}
                for name, path in files.items()}
    # Never replace a numbered release or silently downgrade the moving aliases.
    releases = json.loads(run("gh", "api", f"repos/{REPOSITORY}/releases?per_page=100"))
    if any(release["tag_name"] == tag for release in releases):
        raise ValueError("Numbered release already exists; refusing any asset replacement")
    published = [release for release in releases if not release["draft"] and not release["prerelease"]
                 and re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", release["tag_name"])]
    if any(version_tuple(release["tag_name"]) >= wanted for release in published):
        raise ValueError("Refusing to downgrade latest or republish an existing version")
    image_digest = os.environ["IMAGE_DIGEST"]
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", image_digest):
        raise ValueError("A genuine published multiarchitecture container digest is required")
    notes = f"""Portico Media Server {version} — Development Preview

Source commit: `{sha}`; immutable source tag: `{tag}`.

Downloads: Windows x64 and ARM64 installers and portable ZIPs; macOS Apple Silicon ARM64 PKG and DMG; Linux x64 and ARM64 portable tarballs, DEB and RPM packages. Stable filenames support the website's permanent GitHub latest download links.

The operating-system installers are unsigned; the macOS application is not notarized. Install manually after verifying checksums. Automatic installation is disabled. Build, native media-tool and package-content checks do not establish physical-device playback or OS installation acceptance.

Build manifests and the final release inventory use the dedicated Ed25519 release signer. Verify `release-manifest.sig` against your independently retained public key, then check the inventory and `checksums.sha256`. The accompanying public key is for convenience; a key downloaded beside its signature does not independently establish trust.

Each package includes GPL-3.0-or-later application licensing and third-party notices. Its FFmpeg bundle includes the exact corresponding source archive, locked recipe, patches and dependency licence texts; FFmpeg is GPL version 3, with no nonfree components. An SPDX inventory is attached.

Container: `ghcr.io/porticomediaserver/portico-server:{version}` (Linux amd64 and arm64), digest `{image_digest}`. Build provenance and SBOM attestations accompany the container. The `latest` image tag is promoted to this tested digest after complete release reconciliation.
"""
    with tempfile.TemporaryDirectory() as temporary:
        note_file = pathlib.Path(temporary) / "release-notes.txt"
        note_file.write_text(notes)
        subprocess.run(
            ["gh", "release", "create", tag, *(str(path) for path in files.values()),
             "--repo", REPOSITORY, "--verify-tag", "--target", sha, "--draft",
             "--title", f"Portico Media Server {version} — Development Preview",
             "--notes-file", str(note_file)], check=True,
        )
    draft = api(f"releases/tags/{tag}")
    if not draft["draft"] or draft["tag_name"] != tag or remote_tag(tag) != sha:
        raise ValueError("Draft identity changed during upload")
    verify_assets(draft, expected)
    api(f"releases/{draft['id']}", "--method", "PATCH",
        "-F", "draft=false", "-F", "prerelease=false", "-f", "make_latest=true")
    release = api(f"releases/tags/{tag}")
    if release["draft"] or release["prerelease"] or release["tag_name"] != tag or remote_tag(tag) != sha:
        raise ValueError("Published release identity or tag target differs")
    verify_assets(release, expected, download=True)
    latest = api("releases/latest")
    if latest["id"] != release["id"]:
        raise ValueError("GitHub Latest does not select the published release")
    # Only now may the explicit Git latest alias advance. Numbered tags never move.
    reference = subprocess.run(
        ["gh", "api", f"repos/{REPOSITORY}/git/ref/tags/latest"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    if reference.returncode == 0:
        api("git/refs/tags/latest", "--method", "PATCH", "-f", f"sha={sha}", "-F", "force=true")
    else:
        api("git/refs", "--method", "POST", "-f", "ref=refs/tags/latest", "-f", f"sha={sha}")
    if remote_tag("latest") != sha:
        raise ValueError("Moving Git latest tag failed readback")
    print(json.dumps({"release": release["html_url"], "sourceCommit": sha,
                      "draft": release["draft"], "latestTag": remote_tag("latest"),
                      "assets": len(expected), "containerDigest": image_digest}))


if __name__ == "__main__":
    main()
