#!/usr/bin/env python3
"""Persistent Contabo walk-server deployment coordinator (source only).

The coordinator tracks published server main, builds it with the release guard,
prepares a fresh disposable state through the ordinary local setup APIs, starts
the one loopback walk listener, keeps the active/previous build and state for
rollback, and reports usable only after the owner's private-entry marker exists.

It does not install, deploy or start anything by itself. The owner installs the
systemd service and tunnel, supplies the fixture/credential inputs, and runs it
after the existing Contabo M0 runtime is cleaned up.
"""
from __future__ import annotations

import argparse
import fcntl
import importlib.util
import json
import os
import secrets
import sys
from pathlib import Path

def _load_helpers():
    root = Path(__file__).resolve().parent
    package = root / 'server'
    spec = importlib.util.spec_from_file_location('walks_server', package / '__init__.py',
                                                  submodule_search_locations=[str(package)])
    if spec is None or spec.loader is None:
        raise SystemExit('walk server helpers unavailable')
    module = importlib.util.module_from_spec(spec)
    sys.modules['walks_server'] = module
    spec.loader.exec_module(module)

_load_helpers()

from walks_server import (archive, build as buildmod, config, credentials, entry, fixtures,
                          receipt, runtime, source, state, tunnel)

class CommandError(Exception):
    def __init__(self, code):
        self.code = code

def _code(error):
    if isinstance(error, state.StateError):
        return error.code
    if hasattr(error, 'code') and isinstance(error.code, str):
        return error.code
    if error.args and isinstance(error.args[0], str):
        return error.args[0]
    return 'unclassified'

def _common(parser):
    parser.add_argument('--repo', type=Path, default=config.DEFAULT_REPO)
    parser.add_argument('--fixtures', type=Path, default=config.DEFAULT_FIXTURES)
    parser.add_argument('--work-root', type=Path, default=config.WORK_ROOT)
    parser.add_argument('--port', type=int, default=config.DEFAULT_PORT)
    parser.add_argument('--access-address', default=config.DEFAULT_ACCESS)
    parser.add_argument('--server-md', type=Path)
    parser.add_argument('--retain-builds', type=int, default=config.DEFAULT_RETAIN_BUILDS)
    parser.add_argument('--retain-states', type=int, default=config.DEFAULT_RETAIN_STATES)

def _paths(args):
    config.validate_port(args.port)
    access = tunnel.validate(args.access_address)
    paths = config.paths(args.work_root, args.server_md)
    return paths, access

def _lock(paths):
    descriptor = os.open(paths.lock, os.O_RDWR | os.O_CREAT, 0o600)
    try:
        fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        os.close(descriptor)
        return None
    return descriptor

def _unlock(descriptor):
    if descriptor is not None:
        try:
            fcntl.flock(descriptor, fcntl.LOCK_UN)
        finally:
            os.close(descriptor)

def _existing_build(paths, commit):
    for directory in sorted(paths.builds.iterdir(), key=lambda path: path.stat().st_mtime, reverse=True):
        if directory.is_symlink() or not directory.is_dir():
            continue
        document = receipt.read_json(directory / 'manifest.json')
        if not isinstance(document, dict) or document.get('commit') != commit:
            continue
        try:
            candidate = buildmod.load(directory)
            if candidate.build_tags == buildmod.BUILD_TAGS:
                return candidate
        except Exception:
            continue
    return None

def _build_new(paths, args, fx, commit):
    stamp = config.now_iso().replace(':', '').replace('-', '').replace('.', '') + '-' + secrets.token_hex(2)
    build_dir = paths.builds / config.safe_component(commit[:8] + '-' + stamp)
    build_dir.mkdir(mode=0o700, parents=True, exist_ok=False)
    source_tree = build_dir / 'source'
    log = build_dir / 'build.log'
    source_object = None
    try:
        source_object = source.resolve_at(args.repo, commit, source_tree, log)
        return buildmod.build(source_object, build_dir, fx, paths)
    finally:
        if source_object is not None:
            source.cleanup(source_object)

def _start(build, state_dir, paths, fx, port):
    runtime_object = runtime.start(build, state_dir, port, paths, fx.runtime)
    try:
        system = runtime.system(port)
    except runtime.RuntimeFailure:
        runtime.stop(runtime_object)
        raise
    if (not isinstance(system, dict) or system.get('setupRequired') is not False
            or system.get('signIn') != 'local' or build.commit[:8] not in str(system.get('version', ''))):
        runtime.stop(runtime_object)
        raise runtime.RuntimeFailure('system-shape')
    return runtime_object

def _active_runtime(active, paths):
    return runtime.from_active(active, paths)

def _mark_stopped(paths, name):
    if not isinstance(name, str):
        return
    try:
        archive.stopped(paths.builds / config.safe_component(name))
    except Exception:
        pass

def _mark_active(paths, build, usable, note, continuity=None, identity_retired=None):
    try:
        archive.active(build.directory, usable, note, continuity, identity_retired)
    except Exception:
        pass

def _continuity_notice(continuity, identity_retired, first_deployment):
    if first_deployment:
        return 'First deployment; new identity and sessions.'
    if continuity == 'reused':
        return 'State reused; schema digest unchanged; existing sessions remain.'
    if continuity == 'rollback':
        return 'Rolled back; sessions from the replaced state are retired; sign in again if needed.'
    if identity_retired:
        return 'Baseline or walk credentials changed; identity and sessions retired; sign in again with the walk credentials.'
    return 'Fresh state; identity and sessions replaced.'

def _publish(paths, active, commit, access, entry_ok, usable, note, notice=''):
    try:
        receipt.publish(paths, active, {'commit': commit}, {}, access, entry_ok, usable, note, notice)
    except Exception:
        pass

def _stop_document(active):
    if not isinstance(active, dict):
        return True
    try:
        pid = int(active.get('pid'))
        start = int(active.get('start'))
        pgid = int(active.get('pgid'))
        launcher_pid = int(active.get('launcherPid') or 0)
        launcher_start = int(active.get('launcherStart') or 0)
        launcher_pgid = int(active.get('launcherPgid') or 0)
    except (TypeError, ValueError):
        return True
    return runtime.stop_fields(pid, start, pgid, launcher_pid, launcher_start, launcher_pgid)

def _activate(paths, args, build, prepared, fx, access, continuity, identity_retired, first_deployment):
    old = receipt.read_json(paths.active, {})
    if isinstance(old, dict) and old.get('pid'):
        if not _stop_document(old):
            raise runtime.RuntimeFailure('runtime-not-ours')
        _mark_stopped(paths, old.get('build'))
    try:
        runtime_object = _start(build, prepared.directory, paths, fx, args.port)
    except Exception:
        _mark_stopped(paths, build.directory.name)
        raise
    entry_ok = entry.verified(paths, access)
    active = runtime.active_document(runtime_object, build.commit, prepared.name)
    active.update({'continuity': continuity, 'identityRetired': bool(identity_retired),
                   'firstDeployment': bool(first_deployment), 'schemaDigest': build.schema_digest})
    if isinstance(old, dict) and old:
        receipt.write_json(paths.previous, old)
    receipt.write_json(paths.active, active)
    _mark_active(paths, build, entry_ok,
                 'active walk server' if entry_ok else 'server ready; private entry not verified',
                 continuity, identity_retired)
    notice = _continuity_notice(continuity, identity_retired, first_deployment)
    _publish(paths, active, build.commit, access, entry_ok, entry_ok,
             'active' if entry_ok else 'private entry not verified', notice)
    return runtime_object, active, entry_ok

def _restart_active(paths, args, fx, access):
    previous = receipt.read_json(paths.active, {})
    if not isinstance(previous, dict) or not previous.get('build') or not previous.get('state'):
        raise runtime.RuntimeFailure('no-active')
    build = buildmod.load(paths.builds / config.safe_component(previous['build']))
    runtime_object = _start(build, paths.states / config.safe_component(previous['state']),
                            paths, fx, args.port)
    continuity = previous.get('continuity')
    identity_retired = bool(previous.get('identityRetired'))
    first_deployment = bool(previous.get('firstDeployment'))
    active = runtime.active_document(runtime_object, build.commit, str(previous['state']))
    active.update({'continuity': continuity, 'identityRetired': identity_retired,
                   'firstDeployment': first_deployment, 'schemaDigest': build.schema_digest})
    receipt.write_json(paths.active, active)
    entry_ok = entry.verified(paths, access)
    _mark_active(paths, build, entry_ok,
                 'active walk server' if entry_ok else 'server ready; private entry not verified',
                 continuity, identity_retired)
    notice = _continuity_notice(continuity, identity_retired, first_deployment)
    _publish(paths, active, build.commit, access, entry_ok, entry_ok,
             'active' if entry_ok else 'private entry not verified', notice)
    return runtime_object, active, entry_ok

def _deploy_once(paths, args, fx, creds, access, failure=None):
    old = receipt.read_json(paths.active, {})
    try:
        commit = source.fetch_main(args.repo)
        build = _existing_build(paths, commit) or _build_new(paths, args, fx, commit)
        active_build = None
        active_state = None
        if isinstance(old, dict) and old.get('build'):
            try:
                active_build = buildmod.load(paths.builds / config.safe_component(old['build']))
            except Exception:
                active_build = None
        if isinstance(old, dict) and old.get('state'):
            try:
                active_state = state.load(paths, old['state'])
            except Exception:
                active_state = None
        reuse = (active_build is not None and active_state is not None
                 and active_build.schema_digest == build.schema_digest
                 and active_build.build_tags == build.build_tags
                 and active_state.credentials_digest == credentials.digest(creds)
                 and active_state.fixtures_manifest_sha256 == fx.manifest_sha256
                 and active_state.runtime_sha256 == fx.runtime_sha256)
        had_active = bool(old)
        if reuse:
            prepared = active_state
            continuity = 'reused'
            identity_retired = False
            first_deployment = False
        else:
            prepared = state.prepare(build, fx, creds, paths, args.port)
            first_deployment = not had_active
            identity_retired = had_active
            continuity = 'first' if first_deployment else 'fresh-identity'
        runtime_object, active, entry_ok = _activate(
            paths, args, build, prepared, fx, access, continuity, identity_retired, first_deployment)
    except (source.SourceError, buildmod.BuildError, state.StateError, runtime.RuntimeFailure,
            fixtures.FixtureError, credentials.CredentialsError) as error:
        code = _code(error)
        operation = getattr(error, 'operation', None)
        receipt.receipt(paths, 'deploy-failed', code, status=getattr(error, 'status', None),
                        operation=operation)
        if failure is not None:
            failure.update({'code': code, 'status': getattr(error, 'status', None),
                            'operation': operation})
        if isinstance(old, dict) and old and not _active_runtime(old, paths):
            try:
                _restart_active(paths, args, fx, access)
            except Exception:
                pass
        return None
    except Exception:
        receipt.receipt(paths, 'deploy-failed', 'unclassified')
        if failure is not None:
            failure['code'] = 'unclassified'
        if isinstance(old, dict) and old and not _active_runtime(old, paths):
            try:
                _restart_active(paths, args, fx, access)
            except Exception:
                pass
        return None
    try:
        archive.retain(paths, active['build'], (old or {}).get('build'),
                       active['state'], (old or {}).get('state'),
                       args.retain_builds, args.retain_states)
    except Exception:
        receipt.receipt(paths, 'retention-failed', 'retention-failed')
    return {'runtime': runtime_object, 'build': build, 'state': prepared, 'entry': entry_ok,
            'commit': commit, 'active': active, 'continuity': continuity,
            'identityRetired': identity_retired, 'firstDeployment': first_deployment}

def _mark_active_name(paths, name, usable, note):
    if not isinstance(name, str):
        return
    try:
        archive.active(paths.builds / config.safe_component(name), usable, note)
    except Exception:
        pass

def _status(paths, args, access):
    active = receipt.read_json(paths.active, {})
    current = _active_runtime(active, paths) if isinstance(active, dict) else None
    entry_ok = entry.verified(paths, access)
    continuity = active.get('continuity') if isinstance(active, dict) else None
    identity_retired = bool(active.get('identityRetired')) if isinstance(active, dict) else False
    first_deployment = bool(active.get('firstDeployment')) if isinstance(active, dict) else False
    notice = 'Stopped; not usable.' if current is None else _continuity_notice(
        continuity, identity_retired, first_deployment)
    if isinstance(active, dict) and active.get('pid'):
        if current is None:
            _mark_stopped(paths, active.get('build'))
            _publish(paths, active, active.get('commit'), access, entry_ok, False, 'stopped', notice)
        else:
            _mark_active_name(paths, active.get('build'), entry_ok,
                              'active walk server' if entry_ok else 'server ready; private entry not verified')
            _publish(paths, active, active.get('commit'), access, entry_ok, entry_ok,
                     'active' if entry_ok else 'private entry not verified', notice)
    usable = bool(current and entry_ok)
    note = 'running' if current else 'stopped'
    if current and not entry_ok:
        note = 'running; private entry not verified'
    document = receipt.status_document(active if isinstance(active, dict) else {},
                                       {'commit': active.get('commit') if isinstance(active, dict) else None},
                                       {}, access, entry_ok, usable, note, notice)
    print(json.dumps(document, sort_keys=True))
    return 0

def _deploy(paths, args, fx, creds, access):
    lock = _lock(paths)
    if lock is None:
        print(json.dumps({'event': 'busy'}))
        return 0
    try:
        failure = {}
        result = _deploy_once(paths, args, fx, creds, access, failure)
        document = {'event': 'deploy', 'ok': bool(result),
                    'commit': result['commit'] if result else None,
                    'continuity': result['continuity'] if result else None,
                    'identityRetired': result['identityRetired'] if result else None}
        if not result and failure:
            document['code'] = failure.get('code')
            if isinstance(failure.get('status'), int):
                document['status'] = failure['status']
            if failure.get('operation') in receipt.OPERATIONS:
                document['operation'] = failure['operation']
        print(json.dumps(document))
        return 0 if result else 1
    finally:
        _unlock(lock)

def _rollback(paths, args, fx, access):
    lock = _lock(paths)
    if lock is None:
        print(json.dumps({'event': 'busy'}))
        return 0
    current = {}
    stopped_current = False
    try:
        current = receipt.read_json(paths.active, {})
        previous = receipt.read_json(paths.previous, {})
        if not isinstance(previous, dict) or not previous.get('build') or not previous.get('state'):
            raise CommandError('no-previous')
        if isinstance(current, dict) and current.get('pid'):
            if _stop_document(current):
                stopped_current = True
        build = buildmod.load(paths.builds / config.safe_component(previous['build']))
        runtime_object = _start(build, paths.states / config.safe_component(previous['state']),
                                paths, fx, args.port)
        active = runtime.active_document(runtime_object, build.commit, str(previous['state']))
        active.update({'continuity': 'rollback', 'identityRetired': True,
                       'firstDeployment': False, 'schemaDigest': build.schema_digest})
        receipt.write_json(paths.active, active)
        receipt.write_json(paths.previous, current)
        entry_ok = entry.verified(paths, access)
        _mark_active(paths, build, entry_ok, 'rolled back walk server', 'rollback', True)
        _publish(paths, active, build.commit, access, entry_ok, entry_ok, 'rolled back',
                 _continuity_notice('rollback', True, False))
        print(json.dumps({'event': 'rollback', 'ok': True, 'commit': build.commit}))
        return 0
    except Exception:
        if stopped_current and isinstance(current, dict) and current.get('build') and current.get('state'):
            try:
                current_build = buildmod.load(paths.builds / config.safe_component(current['build']))
                runtime_object = _start(current_build, paths.states / config.safe_component(current['state']),
                                        paths, fx, args.port)
                active = runtime.active_document(runtime_object, current_build.commit, str(current['state']))
                continuity = current.get('continuity')
                identity_retired = bool(current.get('identityRetired'))
                first_deployment = bool(current.get('firstDeployment'))
                active.update({'continuity': continuity, 'identityRetired': identity_retired,
                               'firstDeployment': first_deployment, 'schemaDigest': current_build.schema_digest})
                receipt.write_json(paths.active, active)
                entry_ok = entry.verified(paths, access)
                _mark_active(paths, current_build, entry_ok, 'rollback failed; previous runtime restored',
                             continuity, identity_retired)
                _publish(paths, active, current_build.commit, access, entry_ok, entry_ok,
                         'rollback failed; previous runtime restored',
                         _continuity_notice(continuity, identity_retired, first_deployment))
            except Exception:
                pass
        raise
    finally:
        _unlock(lock)

def _fixtures_command(args):
    if args.action == 'manifest':
        fixtures.write_manifest(args.fixtures)
        print(json.dumps({'fixtures': 'manifest', 'written': True}))
        return 0
    fixtures.verify(args.fixtures)
    print(json.dumps({'fixtures': 'verified', 'runtime': True}))
    return 0

def _entry_command(args):
    access = tunnel.validate(args.access_address)
    paths = config.paths(args.work_root)
    print(json.dumps({'entryVerified': entry.verified(paths, access), 'address': access}))
    return 0

def _archive_command(args):
    paths = config.paths(args.work_root)
    print(json.dumps({'archive': archive.list_archive(paths)}, sort_keys=True))
    return 0

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    deploy = sub.add_parser('deploy', help='one build/state rotation; owner runs this with credentials on stdin')
    _common(deploy)
    status = sub.add_parser('status', help='read the closed status without starting anything')
    _common(status)
    rollback = sub.add_parser('rollback', help='restore the previous build and state')
    _common(rollback)
    fixtures_parser = sub.add_parser('fixtures', help='create or verify the fixture manifest')
    fixtures_parser.add_argument('action', choices=('manifest', 'verify'))
    fixtures_parser.add_argument('--fixtures', type=Path, default=config.DEFAULT_FIXTURES)
    entry_parser = sub.add_parser('entry', help='check the owner private-entry marker')
    entry_parser.add_argument('action', choices=('check',))
    entry_parser.add_argument('--work-root', type=Path, default=config.WORK_ROOT)
    entry_parser.add_argument('--access-address', default=config.DEFAULT_ACCESS)
    archive_parser = sub.add_parser('archive', help='list archived handoff manifests')
    archive_parser.add_argument('action', choices=('list',))
    archive_parser.add_argument('--work-root', type=Path, default=config.WORK_ROOT)
    args = parser.parse_args()
    try:
        if args.command == 'fixtures':
            return _fixtures_command(args)
        if args.command == 'entry':
            return _entry_command(args)
        if args.command == 'archive':
            return _archive_command(args)
        paths, access = _paths(args)
        if args.command == 'status':
            return _status(paths, args, access)
        fx = fixtures.verify(args.fixtures)
        if args.command == 'deploy':
            creds = credentials.load_stdin()
            return _deploy(paths, args, fx, creds, access)
        if args.command == 'rollback':
            return _rollback(paths, args, fx, access)
        raise CommandError('command-not-admitted')
    except (config.ConfigError, tunnel.TunnelError, fixtures.FixtureError, credentials.CredentialsError,
            entry.EntryError, source.SourceError, buildmod.BuildError, runtime.RuntimeFailure,
            state.StateError, archive.ArchiveError, CommandError) as error:
        print(json.dumps({'error': _code(error)}))
        return 2
    except Exception:
        print(json.dumps({'error': 'unclassified'}))
        return 3

if __name__ == '__main__':
    raise SystemExit(main())
