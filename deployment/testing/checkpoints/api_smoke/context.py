"""Admission for one private runtime context and its explicit loopback origin.

The context is produced by the runner after its real fixture/API setup and is
read here by reference only. It names the exact Job run and the live runtime
process that owns the loopback listener; custody is verified separately before
any request or mutation. It is never printed, copied or extended with runtime
state, and every rejection is a closed reason rather than an exception message.
"""
from __future__ import annotations

import json
import os
import re
import stat
from dataclasses import dataclass

class ContextError(Exception):
    """The private runtime context, origin or custody was not admitted."""

_KEYS = frozenset({'origin', 'accessToken', 'installationId', 'owner', 'scope', 'runId', 'runtime'})
_RUNTIME_KEYS = frozenset({'pid', 'startTimeTicks'})
_SCOPES = frozenset({'checkpoint-disposable'})
_LOOPBACK = re.compile(r'http://127\.0\.0\.1:([0-9]{4,5})\Z')
_PRINTABLE = re.compile(r'[!-~]{8,4096}\Z')
_NAME = re.compile(r'[A-Za-z0-9._~-]{1,256}\Z')
_RUN_ID = re.compile(r'[a-z0-9-]{1,96}\Z')

@dataclass(frozen=True)
class Context:
    origin: str
    host: str
    port: int
    access_token: str
    installation_id: str
    owner: bool
    scope: str
    run_id: str
    runtime_pid: int
    runtime_start: int

def admitted_origin(raw):
    """Accept exactly one explicit loopback HTTP origin and nothing else."""
    if not isinstance(raw, str) or len(raw) > 40 or raw != raw.strip():
        raise ContextError('origin-not-admitted')
    match = _LOOPBACK.fullmatch(raw)
    if not match:
        raise ContextError('origin-not-admitted')
    port = int(match.group(1))
    if not 1024 <= port <= 65535 or str(port) != match.group(1):
        raise ContextError('origin-not-admitted')
    return '127.0.0.1', port, raw

def _token(value):
    if not isinstance(value, str) or not _PRINTABLE.fullmatch(value):
        raise ContextError('private-context-shape')
    return value

def _name(value):
    if not isinstance(value, str) or not _NAME.fullmatch(value):
        raise ContextError('private-context-shape')
    return value

def _read(path):
    try:
        descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    except OSError:
        raise ContextError('private-context-unavailable') from None
    try:
        info = os.fstat(descriptor)
        if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600 or info.st_uid != os.getuid():
            raise ContextError('private-context-permissions')
        if info.st_size > 65536:
            raise ContextError('private-context-size')
        chunks, remaining = [], 65537
        while remaining > 0:
            chunk = os.read(descriptor, remaining)
            if not chunk:
                break
            chunks.append(chunk)
            remaining -= len(chunk)
        raw = b''.join(chunks)
    finally:
        os.close(descriptor)
    if len(raw) > 65536:
        raise ContextError('private-context-size')
    return raw

def _runtime(value):
    if not isinstance(value, dict) or set(value) != _RUNTIME_KEYS:
        raise ContextError('private-context-shape')
    pid = value['pid']
    start = value['startTimeTicks']
    if isinstance(pid, bool) or not isinstance(pid, int) or not 1 <= pid <= 4194304:
        raise ContextError('private-context-shape')
    if isinstance(start, bool) or not isinstance(start, int) or not 1 <= start <= 2 ** 63 - 1:
        raise ContextError('private-context-shape')
    return pid, start

def load(path):
    """Read the owner-only runtime context by reference and validate its shape."""
    raw = _read(path)
    try:
        document = json.loads(raw)
    except ValueError:
        raise ContextError('private-context-invalid') from None
    if not isinstance(document, dict) or set(document) != _KEYS:
        raise ContextError('private-context-shape')
    host, port, origin = admitted_origin(document['origin'])
    owner = document['owner']
    if not isinstance(owner, bool):
        raise ContextError('private-context-shape')
    scope = document['scope']
    if scope not in _SCOPES:
        raise ContextError('private-context-scope')
    run_id = document['runId']
    if not isinstance(run_id, str) or not _RUN_ID.fullmatch(run_id):
        raise ContextError('private-context-shape')
    pid, start = _runtime(document['runtime'])
    return Context(origin=origin, host=host, port=port, access_token=_token(document['accessToken']),
                   installation_id=_name(document['installationId']), owner=owner, scope=scope,
                   run_id=run_id, runtime_pid=pid, runtime_start=start)
