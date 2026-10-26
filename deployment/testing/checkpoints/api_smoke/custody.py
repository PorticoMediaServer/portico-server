"""Verify the context names the live, owned checkpoint runtime before use.

The marker is owner-issued under the Job: the runner owner writes the PID and
process start ticks of the exact spawned server process (or its exact original
descendant). The runner's `app.pid` may be the bwrap launcher and is not accepted
unless it owns the listener. This module checks that the same process is still
alive, is ours, and owns the listening socket on the admitted loopback port.
Nothing is printed.
"""
from __future__ import annotations

import os

from .context import ContextError

_LIMIT = 2 * 1024 * 1024

def _read(path, limit):
    try:
        descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    except OSError:
        raise ContextError('runtime-unavailable') from None
    try:
        chunks, remaining = [], limit + 1
        while remaining > 0:
            chunk = os.read(descriptor, remaining)
            if not chunk:
                break
            chunks.append(chunk)
            remaining -= len(chunk)
        raw = b''.join(chunks)
    finally:
        os.close(descriptor)
    if len(raw) > limit:
        raise ContextError('runtime-unavailable')
    return raw.decode('utf-8', 'replace')

def _process(context):
    root = '/proc/%d' % context.runtime_pid
    try:
        info = os.stat(root)
    except OSError:
        raise ContextError('runtime-not-running') from None
    if info.st_uid != os.getuid():
        raise ContextError('runtime-not-owned')
    fields = _read(root + '/stat', 8192).rpartition(')')[2].split()
    try:
        start = int(fields[19])
    except (IndexError, ValueError):
        raise ContextError('runtime-unavailable') from None
    if start != context.runtime_start:
        raise ContextError('runtime-start-mismatch')

def _listener(context):
    port = format(context.port, '04X')
    inodes = set()
    for line in _read('/proc/net/tcp', _LIMIT).splitlines()[1:]:
        fields = line.split()
        if len(fields) < 10 or fields[3] != '0A':
            continue
        local = fields[1]
        if not local.startswith('0100007F:') or local.split(':', 1)[1].upper() != port:
            continue
        inode = fields[9]
        if inode.isdigit() and inode != '0':
            inodes.add(inode)
    if not inodes:
        raise ContextError('runtime-listener-missing')
    try:
        with os.scandir('/proc/%d/fd' % context.runtime_pid) as entries:
            for index, entry in enumerate(entries):
                if index >= 4096:
                    break
                try:
                    target = os.readlink(entry.path)
                except OSError:
                    continue
                if target.startswith('socket:[') and target[8:-1] in inodes:
                    return
    except OSError:
        raise ContextError('runtime-listener-unreadable') from None
    raise ContextError('runtime-listener-owner-mismatch')

def verify(context):
    """Refuse anything that is not the exact live Job runtime."""
    _process(context)
    _listener(context)
