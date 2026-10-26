"""Verify the walk fixture media and its qualified runtime bundle.

The media tree is hashed once by the owner with `fixtures manifest`; each cycle
rechecks the small runtime bundle, the pinned EPUB and the manifest itself. A
fixture without a manifest is refused rather than treated as verified.
"""
from __future__ import annotations

import hashlib
import json
import os
import tempfile
from dataclasses import dataclass
from pathlib import Path

from .config import now_iso

_KEYS = frozenset({'source', 'mediaSHA256', 'runtimeSHA256', 'epubSHA256', 'createdAt'})

class FixtureError(Exception):
    """A closed fixture refusal."""

@dataclass(frozen=True)
class Fixtures:
    root: Path
    media: Path
    runtime: Path
    manifest_sha256: str
    media_sha256: str
    runtime_sha256: str
    epub_sha256: str

def _file_hash(path):
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        while True:
            block = stream.read(1024 * 1024)
            if not block:
                break
            digest.update(block)
    return digest.hexdigest()

def _tree_hash(root):
    root = Path(root)
    digest = hashlib.sha256()
    for path in sorted(root.rglob('*')):
        if path.is_symlink():
            # Qualified shared-library bundles contain SONAME links. Bind both
            # the link spelling and actual target bytes, never an external path.
            target = path.resolve()
            if not target.is_relative_to(root.resolve()) or not target.is_file():
                raise FixtureError('fixture-symlink')
            digest.update(str(path.relative_to(root)).encode() + b'\0link\0')
            digest.update(os.readlink(path).encode() + b'\0')
            digest.update(_file_hash(target).encode() + b'\n')
            continue
        if not path.is_file():
            continue
        relative = str(path.relative_to(root))
        digest.update(relative.encode() + b'\0')
        digest.update(str(path.stat().st_size).encode() + b'\0')
        digest.update(_file_hash(path).encode() + b'\n')
    return digest.hexdigest()

def _epub_hash(media):
    books = Path(media) / 'Read' / 'Books'
    found = sorted(books.rglob('*.epub')) if books.is_dir() else []
    if len(found) > 1:
        raise FixtureError('fixture-multiple-epub')
    return _file_hash(found[0]) if found else ''

def manifest_document(root):
    root = Path(root)
    media = root / 'media'
    runtime = root / 'runtime' / 'bundle'
    if not media.is_dir() or not runtime.is_dir():
        raise FixtureError('fixture-missing')
    return {
        'source': 'Portico Demo Media / runnerfixture',
        'mediaSHA256': _tree_hash(media),
        'runtimeSHA256': _tree_hash(runtime),
        'epubSHA256': _epub_hash(media),
        'createdAt': now_iso(),
    }

def write_manifest(root):
    root = Path(root)
    document = manifest_document(root)
    target = root / 'manifest.json'
    descriptor, name = tempfile.mkstemp(prefix='.manifest-', dir=root)
    try:
        os.fchmod(descriptor, 0o644)
        with os.fdopen(descriptor, 'w') as stream:
            json.dump(document, stream, indent=2, sort_keys=True)
            stream.write('\n')
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(name, target)
    finally:
        if os.path.exists(name):
            os.unlink(name)
    return target

def verify(root):
    root = Path(root)
    media = root / 'media'
    runtime = root / 'runtime' / 'bundle'
    if not media.is_dir() or not runtime.is_dir():
        raise FixtureError('fixture-missing')
    for name in ('toolchain-manifest.json', 'bin/ffmpeg', 'bin/ffprobe'):
        if not (runtime / name).is_file():
            raise FixtureError('runtime-incomplete')
    manifest = root / 'manifest.json'
    try:
        raw = manifest.read_bytes()
    except OSError:
        raise FixtureError('fixtures-unverified') from None
    try:
        document = json.loads(raw)
    except ValueError:
        raise FixtureError('fixtures-unverified') from None
    if not isinstance(document, dict) or set(document) != _KEYS:
        raise FixtureError('fixtures-unverified')
    for key in ('mediaSHA256', 'runtimeSHA256'):
        if not isinstance(document[key], str) or len(document[key]) != 64:
            raise FixtureError('fixtures-unverified')
    if not isinstance(document['epubSHA256'], str) or len(document['epubSHA256']) not in (0, 64):
        raise FixtureError('fixtures-unverified')
    runtime_hash = _tree_hash(runtime)
    if runtime_hash != document['runtimeSHA256']:
        raise FixtureError('runtime-mismatch')
    epub_hash = _epub_hash(media)
    if epub_hash != document['epubSHA256']:
        raise FixtureError('epub-mismatch')
    return Fixtures(root=root, media=media, runtime=runtime,
                    manifest_sha256=hashlib.sha256(raw).hexdigest(),
                    media_sha256=document['mediaSHA256'], runtime_sha256=runtime_hash,
                    epub_sha256=epub_hash)
