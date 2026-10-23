#!/usr/bin/env python3
"""Stage an owned development libmpv closure. Never edits the input SDK or system libraries.

No Developer ID/notarization claim: local ad-hoc signatures only repair Mach-O integrity
after install-name relocation. The source, licence and physical-output gates remain separate.
"""
import argparse
import hashlib
import json
import pathlib
import re
import shutil
import subprocess
import sys

ABI = {"avcodec": 62, "avformat": 62, "avutil": 60, "avfilter": 11,
       "avdevice": 62, "swscale": 9, "swresample": 6}
SYSTEM = ("/System/Library/", "/usr/lib/")


def run(*argv):
    return subprocess.check_output(argv, text=True, stderr=subprocess.STDOUT).strip()


def sha(path):
    with path.open("rb") as source:
        digest = hashlib.sha256()
        while True:
            chunk = source.read(1024 * 1024)
            if not chunk:
                return digest.hexdigest()
            digest.update(chunk)


def dependencies(path):
    return [line.strip().split(" (compatibility version", 1)[0]
            for line in run("/usr/bin/otool", "-L", str(path)).splitlines()[1:]
            if line.strip()]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mpv", required=True, type=pathlib.Path)
    parser.add_argument("--ffmpeg-prefix", required=True, type=pathlib.Path)
    parser.add_argument("--dependency-directory", required=True, type=pathlib.Path)
    parser.add_argument("--ffmpeg-receipt", required=True, type=pathlib.Path)
    parser.add_argument("--lock", required=True, type=pathlib.Path)
    parser.add_argument("--output", required=True, type=pathlib.Path)
    parser.add_argument("--maximum-bytes", type=int, default=805306368)
    args = parser.parse_args()
    if sys.platform != "darwin":
        parser.error("requires the actual macOS SDK tools")
    if args.output.exists() or args.output.is_symlink():
        parser.error("output must be a new owned directory")
    receipt = json.loads(args.ffmpeg_receipt.read_text())
    lock = json.loads(args.lock.read_text())
    if (receipt.get("sourceCommit") != "253db2a7b0a8045c54ce68ce33d7f601229b1822"
            or receipt.get("sourceSha256") != "87bedcefc860cd234cdeddbebdcd3ce45aeedc81d0fbb258ed839ff778be3140"
            or len(receipt.get("patches", [])) != 100
            or any(not re.fullmatch(r"[a-f0-9]{64}", p.get("sha256", "")) for p in receipt["patches"])
            or sha(args.ffmpeg_receipt) != lock["ffmpeg"]["preparedReceiptSha256"]):
        parser.error("the prepared source receipt does not match the desktop source input")
    roots = [args.ffmpeg_prefix / "lib", args.dependency_directory]
    queue = [("libmpv.2.dylib", args.mpv),
             ("libMoltenVK.dylib", args.dependency_directory / "libMoltenVK.dylib")]
    files = {}
    byte_count = 0
    while queue:
        name, source = queue.pop(0)
        if name in files:
            continue
        if not source.is_file() or "arm64" not in run("/usr/bin/lipo", "-archs", str(source)).split():
            parser.error("missing arm64 native dependency: " + name)
        byte_count += source.stat().st_size
        if byte_count > args.maximum_bytes:
            parser.error("native closure exceeds the installed-size budget")
        install_id = run("/usr/bin/otool", "-D", str(source)).splitlines()[1]
        changes = []
        for dependency in dependencies(source):
            if dependency == install_id or dependency.startswith(SYSTEM):
                continue
            leaf = pathlib.PurePosixPath(dependency).name
            if not re.fullmatch(r"lib[A-Za-z0-9_.+-]+\.dylib", leaf):
                parser.error("unresolved non-system dependency: " + leaf)
            av = re.fullmatch(r"lib(avcodec|avformat|avutil|avfilter|avdevice|swscale|swresample)\.(\d+)(?:\.[\d.]+)?\.dylib", leaf)
            if av and (int(av[2]) != ABI[av[1]] or not (roots[0] / leaf).is_file()):
                parser.error("foreign FFmpeg ABI or missing canonical shared library: " + leaf)
            candidate = next((root / leaf for root in roots if (root / leaf).is_file()), None)
            if candidate is None:
                parser.error("dependency absent from the owned SDK: " + leaf)
            queue.append((leaf, candidate))
            changes.append((dependency, "@loader_path/" + leaf))
        files[name] = {"source": source, "inputSha256": sha(source), "changes": changes}
    args.output.mkdir(parents=True)
    manifest = {"status": "development-only; unqualified for release",
                "sourceReceiptSha256": sha(args.ffmpeg_receipt), "sourceReceipt": receipt,
                "ffmpegAbi": ABI, "localSignature": "ad-hoc; no Developer ID or notarization",
                "installedBytes": byte_count, "files": []}
    for name, item in sorted(files.items()):
        output = args.output / name
        shutil.copyfile(item["source"], output)
        output.chmod(0o755)
        run("/usr/bin/install_name_tool", "-id", "@rpath/" + name, str(output))
        for old, new in item["changes"]:
            run("/usr/bin/install_name_tool", "-change", old, new, str(output))
        run("/usr/bin/codesign", "--force", "--sign", "-", "--timestamp=none", str(output))
        run("/usr/bin/codesign", "--verify", "--strict", str(output))
        remaining = dependencies(output)
        if any(not value.startswith(SYSTEM + ("@loader_path/", "@rpath/" + name)) for value in remaining):
            raise RuntimeError("nonportable native dependency remains: " + name)
        manifest["files"].append({"name": name, "inputSha256": item["inputSha256"],
                                  "sha256": sha(output), "bytes": output.stat().st_size,
                                  "dependencies": remaining})
    (args.output / "native-manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(json.dumps({"files": len(files), "installedBytes": byte_count,
                      "manifestSha256": sha(args.output / "native-manifest.json")}))


if __name__ == "__main__":
    main()
