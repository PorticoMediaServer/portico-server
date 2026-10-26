"""Build/state retention and handoff manifests.

A stopped ephemeral walk runtime is never labeled usable. The active and
previous build/state are retained for rollback; older build directories are
removed only after their manifest is archived as a non-usable handoff record.
"""
from __future__ import annotations

import shutil
from pathlib import Path

from .config import DEFAULT_RETAIN_BUILDS, DEFAULT_RETAIN_STATES, now_iso, safe_component
from .receipt import read_json, write_json

class ArchiveError(Exception):
    """A closed archive refusal."""

def manifest_path(directory):
    return Path(directory) / 'manifest.json'

def read_manifest(directory):
    document = read_json(manifest_path(directory))
    if not isinstance(document, dict):
        raise ArchiveError('manifest-missing')
    return document

def mark(directory, state, runtime_verified, usable, note, continuity=None, identity_retired=None):
    path = manifest_path(directory)
    document = read_manifest(directory)
    document['state'] = state
    document['runtimeVerified'] = bool(runtime_verified)
    document['usable'] = bool(usable)
    document['handoff'] = note
    if continuity:
        document['continuity'] = continuity
    if identity_retired is not None:
        document['identityRetired'] = bool(identity_retired)
    document['updatedAt'] = now_iso()
    write_json(path, document)
    return document

def stopped(directory):
    return mark(directory, 'stopped', False, False,
                'stopped ephemeral walk checkpoint; not a usable server; start a new build or roll back',
                continuity='stopped', identity_retired=None)

def active(directory, usable, note, continuity=None, identity_retired=None):
    return mark(directory, 'active', True, usable, note, continuity=continuity,
                identity_retired=identity_retired)

def _archive_manifest(paths, directory):
    document = read_manifest(directory)
    document['state'] = 'archived'
    document['runtimeVerified'] = False
    document['usable'] = False
    document['handoff'] = 'build directory removed by retention; rebuild from the recorded commit; not a usable server'
    document['archivedAt'] = now_iso()
    write_json(paths.archive / (Path(directory).name + '.json'), document)

def _children(root):
    root = Path(root)
    result = []
    for path in root.iterdir():
        if path.is_symlink() or not path.is_dir():
            continue
        try:
            safe_component(path.name)
        except Exception:
            continue
        result.append(path)
    return sorted(result, key=lambda path: path.stat().st_mtime, reverse=True)

def retain(paths, active_build, previous_build, active_state, previous_state,
           retain_builds=DEFAULT_RETAIN_BUILDS, retain_states=DEFAULT_RETAIN_STATES):
    keep_builds = {Path(active_build).name, Path(previous_build).name} if previous_build else {Path(active_build).name}
    for index, directory in enumerate(_children(paths.builds)):
        if directory.name in keep_builds or index < retain_builds:
            continue
        try:
            _archive_manifest(paths, directory)
        except ArchiveError:
            pass
        if directory.parent == paths.builds:
            shutil.rmtree(directory, ignore_errors=True)
    keep_states = {Path(active_state).name, Path(previous_state).name} if previous_state else {Path(active_state).name}
    for index, directory in enumerate(_children(paths.states)):
        if directory.name in keep_states or index < retain_states:
            continue
        if directory.parent == paths.states:
            shutil.rmtree(directory, ignore_errors=True)
    for log in sorted(paths.logs.glob('*.log'), key=lambda path: path.stat().st_mtime, reverse=True)[20:]:
        log.unlink(missing_ok=True)

def list_archive(paths):
    rows = []
    for path in sorted(paths.archive.glob('*.json')):
        document = read_json(path)
        if isinstance(document, dict):
            rows.append({'name': path.stem, 'commit': document.get('commit'),
                         'state': document.get('state'), 'usable': bool(document.get('usable'))})
    return rows
