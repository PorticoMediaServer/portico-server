"""Read the owner's private-entry verification marker.

The coordinator cannot test a tunnel that lives on mini-1, Android devices or
the Mac. The owner writes this 0600 marker only after the private entry actually
works; the coordinator then reports `usable: true`. Without a matching marker
the running server is still reported not usable for handoff.
"""
from __future__ import annotations

import json
import os
import re
import stat
from dataclasses import dataclass
from pathlib import Path

_KEYS = frozenset({'address', 'verifiedAt', 'verifiedBy'})
_BY = re.compile(r'[^\x00-\x1f\x7f]{1,64}\Z')

class EntryError(Exception):
    """A closed private-entry marker refusal."""

@dataclass(frozen=True)
class Entry:
    address: str
    verified_at: str
    verified_by: str

def read(path, address):
    path = Path(path)
    try:
        descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    except OSError:
        raise EntryError('entry-unverified') from None
    try:
        info = os.fstat(descriptor)
        if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600 or info.st_uid != os.getuid():
            raise EntryError('entry-permissions')
        if info.st_size > 4096:
            raise EntryError('entry-invalid')
        raw = os.read(descriptor, 4097)
    finally:
        os.close(descriptor)
    try:
        document = json.loads(raw)
    except ValueError:
        raise EntryError('entry-invalid') from None
    if not isinstance(document, dict) or set(document) != _KEYS:
        raise EntryError('entry-invalid')
    if document['address'] != address:
        raise EntryError('entry-address-mismatch')
    if not isinstance(document['verifiedAt'], str) or not document['verifiedAt']:
        raise EntryError('entry-invalid')
    if not isinstance(document['verifiedBy'], str) or not _BY.fullmatch(document['verifiedBy']):
        raise EntryError('entry-invalid')
    return Entry(document['address'], document['verifiedAt'], document['verifiedBy'])

def verified(paths, address):
    try:
        read(paths.entry, address)
        return True
    except EntryError:
        return False
