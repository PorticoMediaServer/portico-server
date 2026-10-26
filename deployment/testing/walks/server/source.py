"""Resolve the exact published server main and prepare an isolated build tree.

The coordinator fetches origin/main, records the full commit, extracts that
commit with `git archive`, and runs the official API/schema generators. A
non-empty generated overlay is allowed only when it is produced by those
official commands; the exact patch, its hash and the changed-file count are
retained in the immutable build receipt. Generator errors still refuse.
"""
from __future__ import annotations

import difflib
import hashlib
import os
import shutil
import subprocess
import tarfile
from dataclasses import dataclass
from pathlib import Path

from .config import GOMAXPROCS, GOMEMLIMIT, GENERATION_TIMEOUT, safe_commit
from .process import run as run_process

_GENERATED = (
    'packages/contracts/src',
    'server/api/portico.openapi.json',
    'server/internal/httpapi/route_lanes.go',
    'server/internal/persistence/known_columns.go',
    'server/internal/persistence/known_schema.go',
    'server/internal/persistence/schema_test.go',
)

class SourceError(Exception):
    """A closed source or generation refusal."""

@dataclass(frozen=True)
class Source:
    repo: Path
    commit: str
    tree: Path

@dataclass(frozen=True)
class Generation:
    clean: bool
    changed: int
    patch_sha256: str
    patch_path: Path | None

def fetch_main(repo):
    repo = Path(repo)
    env = dict(os.environ, GIT_TERMINAL_PROMPT='0')
    try:
        with open(os.devnull, 'wb') as sink:
            result = subprocess.run(['git', '-C', str(repo), 'fetch', 'origin', '--quiet'],
                                    stdout=sink, stderr=subprocess.STDOUT, timeout=180,
                                    env=env, check=False)
    except subprocess.TimeoutExpired:
        raise SourceError('source-fetch-failed') from None
    if result.returncode:
        raise SourceError('source-fetch-failed')
    try:
        result = subprocess.run(['git', '-C', str(repo), 'rev-parse', 'origin/main'],
                                stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=30,
                                env=env, check=False, text=True)
    except subprocess.TimeoutExpired:
        raise SourceError('source-unavailable') from None
    if result.returncode:
        raise SourceError('source-unavailable')
    return safe_commit(result.stdout.strip())

def _extract(tar_path, destination):
    destination = Path(destination)
    with tarfile.open(tar_path, 'r:') as archive:
        for member in archive.getmembers():
            name = member.name
            parts = Path(name).parts
            if not name or name.startswith('/') or '..' in parts:
                raise SourceError('source-archive-unsafe')
            if member.issym() or member.islnk() or member.ischr() or member.isblk() or member.isfifo():
                raise SourceError('source-archive-unsafe')
            target = destination / name
            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
                continue
            if not member.isfile():
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            source = archive.extractfile(member)
            if source is None:
                raise SourceError('source-archive-unsafe')
            with target.open('xb') as stream:
                shutil.copyfileobj(source, stream)
            os.chmod(target, 0o755 if member.mode & 0o111 else 0o644)

def _snapshot(tree, names):
    tree = Path(tree)
    found = {}
    for name in names:
        path = tree / name
        if path.is_dir():
            for file in sorted(path.rglob('*')):
                if file.is_file() and not file.is_symlink():
                    found[str(file.relative_to(tree))] = file.read_bytes()
        elif path.is_file():
            found[name] = path.read_bytes()
    return found

def _run(argv, cwd, log, timeout, env=None):
    return run_process(argv, cwd, log, timeout, env)

def generate(tree, log, patch_path):
    tree = Path(tree)
    before = _snapshot(tree, _GENERATED)
    env = dict(os.environ, GOMAXPROCS=GOMAXPROCS, GOMEMLIMIT=GOMEMLIMIT, GIT_TERMINAL_PROMPT='0')
    if _run(['go', 'run', './cmd/apigen', '-root', '..'], tree / 'server', log,
            GENERATION_TIMEOUT, env):
        raise SourceError('generation-failed')
    if _run(['python3', 'scripts/schema-sync.py'], tree, log, GENERATION_TIMEOUT, env):
        raise SourceError('generation-failed')
    after = _snapshot(tree, _GENERATED)
    changed = [name for name in sorted(set(before) | set(after)) if before.get(name) != after.get(name)]
    if not changed:
        return Generation(clean=True, changed=0, patch_sha256='', patch_path=None)
    parts = []
    for name in changed:
        old = before.get(name, b'').decode('utf-8', 'replace').splitlines(True)
        new = after.get(name, b'').decode('utf-8', 'replace').splitlines(True)
        parts.extend(difflib.unified_diff(old, new, fromfile='a/' + name, tofile='b/' + name))
    text = ''.join(parts)
    if len(text.encode()) > 4 * 1024 * 1024:
        raise SourceError('generation-patch-large')
    patch_path = Path(patch_path)
    descriptor = os.open(patch_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, 'w') as stream:
        stream.write(text)
        stream.flush()
        os.fsync(stream.fileno())
    digest = hashlib.sha256(text.encode()).hexdigest()
    return Generation(clean=False, changed=len(changed), patch_sha256=digest, patch_path=patch_path)

def resolve_at(repo, commit, destination, log):
    repo = Path(repo)
    commit = safe_commit(commit)
    destination = Path(destination)
    destination.mkdir(mode=0o700, parents=True, exist_ok=False)
    tar_path = destination.parent / ('.source-' + commit + '.tar')
    try:
        with tar_path.open('xb') as stream:
            try:
                result = subprocess.run(['git', '-C', str(repo), 'archive', '--format=tar', commit],
                                        stdout=stream, stderr=subprocess.PIPE, timeout=300, check=False)
            except subprocess.TimeoutExpired:
                raise SourceError('source-archive-failed') from None
        if result.returncode:
            raise SourceError('source-archive-failed')
        _extract(tar_path, destination)
    finally:
        if tar_path.exists():
            tar_path.unlink()
    return Source(repo=repo, commit=commit, tree=destination)

def resolve(repo, destination, log):
    return resolve_at(repo, fetch_main(repo), destination, log)

def cleanup(source):
    tree = Path(source.tree)
    if tree.exists():
        shutil.rmtree(tree)
