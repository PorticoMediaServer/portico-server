"""Start, verify, stop and recover the one loopback walk-server process.

The server uses the normal service profile (`PORTICO_SERVICE=1`) under bwrap,
with `/var/cache/portico` confined to the owned state transcode directory. The
launcher may be a bwrap parent, so the coordinator resolves the actual server
child by executable, start ticks and loopback listener ownership. Cleanup kills
the owned launcher and server process groups; logs stay private.
"""
from __future__ import annotations

import os
import shutil
import signal
import socket
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path

from .api import ApiError, Client
from .config import (GOMAXPROCS, GOMEMLIMIT, HEALTH_INTERVAL, START_TIMEOUT, STOP_GRACE,
                     now_iso)

class RuntimeFailure(Exception):
    """A closed runtime refusal."""

@dataclass(frozen=True)
class Runtime:
    pid: int
    start: int
    exe: str
    port: int
    pgid: int
    launcher_pid: int
    launcher_start: int
    launcher_pgid: int
    state: Path
    build: Path
    log: Path
    started_at: str
    process: subprocess.Popen | None = None

def port_free(port):
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            sock.bind(('127.0.0.1', port))
        except OSError:
            return False
    return True

def _stat_fields(pid):
    try:
        text = Path('/proc/%d/stat' % pid).read_text()
    except OSError:
        return None
    fields = text.rpartition(')')[2].split()
    return fields or None

def start_ticks(pid):
    fields = _stat_fields(pid)
    if fields is None:
        return None
    try:
        return int(fields[19])
    except (IndexError, ValueError):
        return None

def process_state(pid):
    fields = _stat_fields(pid)
    return fields[0] if fields else None

def process_ids(pid):
    fields = _stat_fields(pid)
    if fields is None:
        return None
    try:
        return int(fields[1]), int(fields[2]), int(fields[3])
    except (IndexError, ValueError):
        return None

def exe_path(pid):
    try:
        return os.readlink('/proc/%d/exe' % pid)
    except OSError:
        return None

def _children(pid):
    try:
        tasks = sorted(Path('/proc/%d/task' % pid).iterdir(), key=lambda p: int(p.name))[:256]
    except OSError:
        return []
    result = []
    # Go's accepting child may be spawned by a non-leader OS thread. Linux
    # exposes children per task, not the complete process, in this file.
    for task in tasks:
        try:
            values = (task / 'children').read_text().split()
        except OSError:
            continue
        for value in values:
            if value.isdigit() and int(value) not in result:
                result.append(int(value))
    return result

def descendants(pid, limit=64):
    result = [pid]
    pending = [pid]
    while pending and len(result) < limit:
        current = pending.pop(0)
        for child in _children(current):
            if child not in result:
                result.append(child)
                pending.append(child)
                if len(result) >= limit:
                    break
    return result

def listener_inodes(port):
    wanted = format(port, '04X')
    inodes = set()
    try:
        lines = Path('/proc/net/tcp').read_text().splitlines()[1:]
    except OSError:
        return inodes
    for line in lines:
        fields = line.split()
        if len(fields) < 10 or fields[3] != '0A':
            continue
        local = fields[1]
        if not local.startswith('0100007F:') or local.split(':', 1)[1].upper() != wanted:
            continue
        inode = fields[9]
        if inode.isdigit() and inode != '0':
            inodes.add(inode)
    return inodes

def _owns_listener(pid, inodes):
    if not inodes:
        return False
    try:
        with os.scandir('/proc/%d/fd' % pid) as entries:
            for index, entry in enumerate(entries):
                if index >= 4096:
                    break
                try:
                    target = os.readlink(entry.path)
                except OSError:
                    continue
                if target.startswith('socket:[') and target[8:-1] in inodes:
                    return True
    except OSError:
        return False
    return False

def _resolve_server(launcher_pid, port, expected_exe):
    expected = os.path.realpath(expected_exe)
    deadline = time.monotonic() + START_TIMEOUT
    while time.monotonic() < deadline:
        inodes = listener_inodes(port)
        if inodes:
            for candidate in descendants(launcher_pid):
                exe = exe_path(candidate)
                if exe is None or os.path.realpath(exe) != expected:
                    continue
                if start_ticks(candidate) is None:
                    continue
                if _owns_listener(candidate, inodes):
                    return candidate
        time.sleep(HEALTH_INTERVAL)
    raise RuntimeFailure('runtime-listener')

def health(port, timeout=2):
    try:
        Client(port, timeout=timeout).get('/health/ready', expect=(200,))
        return True
    except ApiError:
        return False

def system(port, timeout=5):
    try:
        return Client(port, timeout=timeout).get('/v1/system')
    except ApiError:
        raise RuntimeFailure('system-unavailable') from None

def _wait_ready(port, process=None):
    deadline = time.monotonic() + START_TIMEOUT
    while time.monotonic() < deadline:
        if health(port):
            return
        if process is not None and process.poll() is not None:
            raise RuntimeFailure('runtime-exited')
        time.sleep(HEALTH_INTERVAL)
    raise RuntimeFailure('runtime-readiness')

def start(build, state, port, paths, runtime_bundle):
    build = build.directory
    bwrap = shutil.which('bwrap')
    if not bwrap:
        raise RuntimeFailure('runtime-bwrap')
    if not port_free(port):
        raise RuntimeFailure('port-busy')
    for name in ('data', 'cache', 'transcode', 'tmp'):
        (Path(state) / name).mkdir(mode=0o700, parents=True, exist_ok=True)
    log = paths.logs / (Path(state).name + '-' + str(port) + '.log')
    descriptor = os.open(log, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    stream = os.fdopen(descriptor, 'ab')
    env = dict(os.environ,
               PORTICO_BIND='127.0.0.1:' + str(port),
               PORTICO_DATA_DIR=str(Path(state) / 'data'),
               PORTICO_WEB_DIR=str(Path(build) / 'web'),
               PORTICO_FFMPEG=str(Path(runtime_bundle) / 'bin/ffmpeg'),
               PORTICO_FFPROBE=str(Path(runtime_bundle) / 'bin/ffprobe'),
               PORTICO_SERVICE='1',
               PORTICO_DISCOVERY='0',
               XDG_CACHE_HOME=str(Path(state) / 'cache'),
               TMPDIR=str(Path(state) / 'tmp'),
               GOMAXPROCS=GOMAXPROCS,
               GOMEMLIMIT=GOMEMLIMIT)
    argv = [bwrap, '--unshare-user', '--ro-bind', '/', '/',
            '--bind', str(paths.work), str(paths.work),
            '--tmpfs', '/var/cache', '--dir', '/var/cache/portico',
            '--bind', str(Path(state) / 'transcode'), '/var/cache/portico',
            '--proc', '/proc', '--dev', '/dev', '--', str(Path(build) / 'portico-server')]
    try:
        process = subprocess.Popen(argv, cwd=str(build), env=env, stdin=subprocess.DEVNULL,
                                   stdout=stream, stderr=subprocess.STDOUT,
                                   start_new_session=True, close_fds=True)
    finally:
        stream.close()
    launcher_pid = process.pid
    launcher_start = start_ticks(launcher_pid)
    try:
        _wait_ready(port, process)
        actual = _resolve_server(launcher_pid, port, str(Path(build) / 'portico-server'))
        ticks = start_ticks(actual)
        exe = exe_path(actual)
        ids = process_ids(actual)
        if ticks is None or exe is None or ids is None:
            raise RuntimeFailure('runtime-identity')
        launcher_ids = process_ids(launcher_pid)
        launcher_pgid = launcher_ids[1] if launcher_ids else launcher_pid
        runtime_object = Runtime(pid=actual, start=ticks, exe=os.path.realpath(exe), port=port,
                                 pgid=ids[1], launcher_pid=launcher_pid,
                                 launcher_start=launcher_start or 0, launcher_pgid=launcher_pgid,
                                 state=Path(state), build=Path(build), log=log,
                                 started_at=now_iso(), process=process)
        return runtime_object
    except Exception:
        _kill_groups(launcher_pid, launcher_start, process_ids(launcher_pid), process)
        raise

def _alive(pid, start):
    if start is None:
        return False
    current = start_ticks(pid)
    return current is not None and current == start and process_state(pid) != 'Z'

def _kill_group(pgid, sig):
    if not pgid or pgid <= 1:
        return
    try:
        os.killpg(pgid, sig)
    except ProcessLookupError:
        pass

def _kill_groups(launcher_pid, launcher_start, launcher_ids, process):
    launcher_pgid = launcher_ids[1] if launcher_ids else launcher_pid
    actual_pids = []
    for candidate in descendants(launcher_pid):
        if _alive(candidate, start_ticks(candidate)):
            actual_pids.append(candidate)
    groups = {launcher_pgid}
    for pid in actual_pids:
        ids = process_ids(pid)
        if ids:
            groups.add(ids[1])
    for group in sorted(groups):
        _kill_group(group, signal.SIGTERM)
    deadline = time.monotonic() + STOP_GRACE
    while time.monotonic() < deadline:
        if process is not None:
            if process.poll() is not None:
                break
        elif not any(_alive(pid, start_ticks(pid)) for pid in actual_pids):
            break
        time.sleep(0.2)
    for group in sorted(groups):
        _kill_group(group, signal.SIGKILL)
    if process is not None:
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            pass

def stop_fields(pid, start, pgid, launcher_pid, launcher_start, launcher_pgid, process=None):
    actual_alive = _alive(pid, start)
    launcher_alive = _alive(launcher_pid, launcher_start) if launcher_start else False
    if not actual_alive and not launcher_alive:
        if process is not None:
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                pass
        return True
    if not actual_alive and launcher_alive:
        # The launcher is still ours; kill only its owned group.
        groups = {launcher_pgid}
    elif actual_alive and not launcher_alive:
        groups = {pgid}
    else:
        groups = {pgid, launcher_pgid}
    for group in sorted(groups):
        _kill_group(group, signal.SIGTERM)
    deadline = time.monotonic() + STOP_GRACE
    while time.monotonic() < deadline:
        if not _alive(pid, start) and (not launcher_start or not _alive(launcher_pid, launcher_start)):
            break
        time.sleep(0.2)
    for group in sorted(groups):
        _kill_group(group, signal.SIGKILL)
    if process is not None:
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            pass
    return not _alive(pid, start) and (not launcher_start or not _alive(launcher_pid, launcher_start))

def stop(runtime_object):
    return stop_fields(runtime_object.pid, runtime_object.start, runtime_object.pgid,
                       runtime_object.launcher_pid, runtime_object.launcher_start,
                       runtime_object.launcher_pgid, runtime_object.process)

def from_active(active, paths):
    if not isinstance(active, dict):
        return None
    try:
        pid = int(active.get('pid'))
        start = int(active.get('start'))
        port = int(active.get('port'))
        pgid = int(active.get('pgid'))
        launcher_pid = int(active.get('launcherPid') or 0)
        launcher_start = int(active.get('launcherStart') or 0)
        launcher_pgid = int(active.get('launcherPgid') or 0)
    except (TypeError, ValueError):
        return None
    exe = str(active.get('exe') or '')
    if not exe or not _alive(pid, start) or not health(port):
        return None
    expected = paths.builds / str(active.get('build')) / 'portico-server'
    if os.path.realpath(exe) != os.path.realpath(expected):
        return None
    if not _owns_listener(pid, listener_inodes(port)):
        return None
    return Runtime(pid=pid, start=start, exe=exe, port=port, pgid=pgid,
                   launcher_pid=launcher_pid, launcher_start=launcher_start,
                   launcher_pgid=launcher_pgid, state=paths.states / str(active.get('state')),
                   build=paths.builds / str(active.get('build')),
                   log=paths.logs / ('recovered-' + str(pid) + '.log'),
                   started_at=str(active.get('startedAt')), process=None)

def active_document(runtime_object, commit, state_name):
    return {
        'commit': commit,
        'build': runtime_object.build.name,
        'state': state_name,
        'port': runtime_object.port,
        'pid': runtime_object.pid,
        'start': runtime_object.start,
        'exe': runtime_object.exe,
        'pgid': runtime_object.pgid,
        'launcherPid': runtime_object.launcher_pid,
        'launcherStart': runtime_object.launcher_start,
        'launcherPgid': runtime_object.launcher_pgid,
        'startedAt': runtime_object.started_at,
    }
