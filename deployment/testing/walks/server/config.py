"""Host configuration for the persistent Contabo walk-server coordinator.

All paths and limits are fixed or bounded here. The coordinator never accepts a
credential value on the command line, never writes outside its owned work root
(except the private credentials file and an owner-supplied status path), and
never binds anything except loopback.
"""
from __future__ import annotations

import datetime
import os
import re
import stat
from dataclasses import dataclass
from pathlib import Path

WORK_ROOT = Path('/home/runner/work/portico-v3-walks')
CREDENTIALS = Path('/home/justin/projects/Portico/.private/walks/credentials.json')
DEFAULT_REPO = Path('/home/runner/work/portico-server')
DEFAULT_FIXTURES = Path('/home/runner/work/fixtures/portico-v3')
DEFAULT_PORT = 32510
DEFAULT_ACCESS = 'http://localhost:49300'
DEFAULT_RETAIN_BUILDS = 3
DEFAULT_RETAIN_STATES = 2

GOMAXPROCS = '2'
GOMEMLIMIT = '1400MiB'
NODE_OPTIONS = '--max-old-space-size=1536'
BUILD_TIMEOUT = 1800
GENERATION_TIMEOUT = 900
NPM_TIMEOUT = 1800
START_TIMEOUT = 240
SCAN_TIMEOUT = 420
STOP_GRACE = 15
HEALTH_INTERVAL = 0.25
LOCK_NAME = 'deploy.lock'

_COMPONENT = re.compile(r'[A-Za-z0-9][A-Za-z0-9._-]{0,95}\Z')
_COMMIT = re.compile(r'[0-9a-f]{40}\Z')

class ConfigError(Exception):
    """A closed configuration or path refusal."""

@dataclass(frozen=True)
class Paths:
    work: Path
    builds: Path
    states: Path
    archive: Path
    logs: Path
    receipts: Path
    active: Path
    previous: Path
    status: Path
    lock: Path
    entry: Path
    server_md: Path | None

def now_iso():
    return datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0).isoformat().replace('+00:00', 'Z')

def safe_component(value):
    if not isinstance(value, str) or not _COMPONENT.fullmatch(value):
        raise ConfigError('unsafe-name')
    return value

def safe_commit(value):
    if not isinstance(value, str) or not _COMMIT.fullmatch(value):
        raise ConfigError('commit-shape')
    return value

def validate_port(port):
    if isinstance(port, bool) or not isinstance(port, int) or not 1024 <= port <= 65535:
        raise ConfigError('port-not-admitted')
    return port

def owned_directory(path, mode=0o700, create=False):
    path = Path(path)
    if not path.is_absolute() or path == Path('/'):
        raise ConfigError('path-not-admitted')
    if create:
        path.mkdir(mode=mode, parents=True, exist_ok=True)
    try:
        info = path.stat()
    except OSError:
        raise ConfigError('path-unavailable') from None
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) & 0o077:
        raise ConfigError('path-permissions')
    return path

def paths(work_root=WORK_ROOT, server_md=None):
    work = owned_directory(work_root, create=True)
    builds = owned_directory(work / 'builds', create=True)
    states = owned_directory(work / 'states', create=True)
    archive = owned_directory(work / 'archive', create=True)
    logs = owned_directory(work / 'logs', create=True)
    receipts = owned_directory(work / 'receipts', create=True)
    status_md = None
    if server_md is not None:
        status_md = Path(server_md)
        if not status_md.is_absolute() or status_md.name != 'SERVER.md':
            raise ConfigError('server-md-not-admitted')
    return Paths(work=work, builds=builds, states=states, archive=archive, logs=logs,
                 receipts=receipts, active=work / 'active.json', previous=work / 'previous.json',
                 status=work / 'status.json', lock=work / LOCK_NAME, entry=work / 'entry.json',
                 server_md=status_md)
