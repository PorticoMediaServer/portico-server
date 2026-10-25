"""PROPOSED, NOT RUN: owned driver output custody. Release approval required.

No device commands or credential entry occur on import. The caller supplies an
exact authorized command and private destination. Credential-phase output is
never returned for display; structured verification must consume it privately.
"""
from pathlib import Path
import json
import os
import stat
import subprocess


def reload_credentials(path: Path) -> dict:
    """Reload after rotation; reject symlinks, non-private mode and wrong owner."""
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600 or info.st_uid != os.getuid():
            raise RuntimeError('protected credential file ownership/mode invalid')
        with os.fdopen(fd, 'r') as stream:
            fd = -1
            return json.load(stream)
    finally:
        if fd >= 0:
            os.close(fd)


def private_capture(argv: list[str], directory: Path, name: str, *, timeout: int = 30, input_bytes: bytes | None = None) -> tuple[int, Path, Path]:
    """Capture BOTH streams before any display; don't raise argv-bearing errors."""
    if Path(name).name != name or not name:
        raise RuntimeError('invalid private output name')
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    info = directory.lstat()
    if not stat.S_ISDIR(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o700 or info.st_uid != os.getuid():
        raise RuntimeError('private output directory invalid')
    paths = [directory / (name + suffix) for suffix in ('.stdout', '.stderr')]
    handles = []
    try:
        for path in paths:
            fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
            handles.append(os.fdopen(fd, 'wb'))
        try:
            # Secret fill uses the CLI's --text-stdin support. Bytes never enter
            # argv, shell source, a recording, or a returned display summary.
            if input_bytes is not None and not isinstance(input_bytes, bytes):
                raise TypeError('private input must be bytes')
            input_options = {'input': input_bytes} if input_bytes is not None else {'stdin': subprocess.DEVNULL}
            result = subprocess.run(argv, **input_options, stdout=handles[0], stderr=handles[1], timeout=timeout, check=False)
            code = result.returncode
        except subprocess.TimeoutExpired:
            code = 124
        return code, paths[0], paths[1]
    finally:
        for handle in handles:
            handle.close()


def display_summary(text: str, known_literals: list[str], *, credential_phase: bool) -> str:
    """Never display credential-phase snapshots, including unexpected field text.

    Literal redaction is defense in depth for preselected safe summaries only;
    it cannot make arbitrary raw output safe (partial/concatenated secrets).
    """
    if credential_phase:
        return 'Credential-phase output withheld; private verification required.'
    for value in sorted((v for v in known_literals if v), key=len, reverse=True):
        text = text.replace(value, '[REDACTED]')
    return text
