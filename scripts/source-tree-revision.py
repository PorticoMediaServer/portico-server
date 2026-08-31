#!/usr/bin/env python3
"""Return a deterministic SHA-256 revision for a Portico Server source tree."""

from __future__ import annotations

import hashlib
import os
import stat
import struct
import subprocess
import sys
from pathlib import Path


def frame(digest, value: bytes) -> None:
    digest.update(struct.pack(">Q", len(value)))
    digest.update(value)


def listed_source_paths(root: Path) -> list[bytes]:
    result = subprocess.run(
        [
            "git",
            "-C",
            os.fspath(root),
            "ls-files",
            "-z",
            "--cached",
            "--others",
            "--exclude-standard",
        ],
        check=True,
        stdout=subprocess.PIPE,
    )
    paths = result.stdout.split(b"\0")
    if paths and paths[-1] == b"":
        paths.pop()
    return paths


def source_revision(root: Path) -> str:
    root = root.resolve(strict=True)
    if not root.is_dir():
        raise ValueError("Server source root is not a directory")

    top_level = subprocess.run(
        ["git", "-C", os.fspath(root), "rev-parse", "--show-toplevel"],
        check=True,
        stdout=subprocess.PIPE,
        text=True,
    ).stdout.strip()
    if Path(top_level).resolve(strict=True) != root:
        raise ValueError("Server source root is not the Git worktree root")

    files: list[tuple[bytes, Path, int]] = []
    for encoded_relative in listed_source_paths(root):
        relative = Path(os.fsdecode(encoded_relative))
        if relative.is_absolute() or ".." in relative.parts:
            raise ValueError(f"Git listed an unsafe source path: {relative}")

        path = root / relative
        try:
            metadata = path.lstat()
        except FileNotFoundError:
            # A deleted tracked file is absent from the current source tree.
            continue

        ancestor = path.parent
        while ancestor != root:
            ancestor_metadata = ancestor.lstat()
            if not stat.S_ISDIR(ancestor_metadata.st_mode):
                raise ValueError(f"Server source contains a link or special directory: {relative}")
            ancestor = ancestor.parent
        if not stat.S_ISREG(metadata.st_mode):
            raise ValueError(f"Server source contains a link or special file: {relative}")
        files.append((encoded_relative, path, stat.S_IMODE(metadata.st_mode)))

    digest = hashlib.sha256(b"portico-server-source-tree-v1\0")
    for relative, path, mode in sorted(files):
        frame(digest, relative)
        digest.update(struct.pack(">I", mode))
        digest.update(struct.pack(">Q", path.stat().st_size))
        with path.open("rb") as source:
            while chunk := source.read(1024 * 1024):
                digest.update(chunk)
    return digest.hexdigest()


def main() -> int:
    if len(sys.argv) != 2:
        print(f"usage: {Path(sys.argv[0]).name} <server-source-root>", file=sys.stderr)
        return 2
    try:
        print(source_revision(Path(sys.argv[1])))
    except (OSError, ValueError, UnicodeError, subprocess.CalledProcessError) as error:
        print(f"cannot calculate Server source revision: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
