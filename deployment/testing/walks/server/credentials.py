"""Credentials are delivered only as a private stdin/FD stream.

The sole local 0600 file is the owner's
`/home/justin/projects/Portico/.private/walks/credentials.json`. The owner pipes
it over encrypted SSH to a one-shot `deploy`; the coordinator reads it from the
inherited FD, keeps it in memory only for that setup, and never creates, writes
or retains a credential file on the Contabo host.
"""
from __future__ import annotations

import json
import os
import re
import hashlib
from dataclasses import dataclass

_KEYS = frozenset({'owner', 'members'})
_OWNER_KEYS = frozenset({'username', 'password'})
_MEMBER_KEYS = frozenset({'username', 'password', 'name'})
_USERNAME = re.compile(r'[a-z0-9][a-z0-9_-]{2,63}\Z')
_PASSWORD = re.compile(r'[!-~]{12,256}\Z')
_NAME = re.compile(r'[^\x00-\x1f\x7f]{1,64}\Z')

class CredentialsError(Exception):
    """A closed credential-stream refusal; it never carries a value."""

@dataclass(frozen=True)
class Member:
    username: str
    password: str
    name: str

@dataclass(frozen=True)
class Credentials:
    owner_username: str
    owner_password: str
    members: tuple[Member, ...]

def _validate(document):
    if not isinstance(document, dict) or set(document) != _KEYS:
        raise CredentialsError('credentials-shape')
    owner = document['owner']
    if not isinstance(owner, dict) or set(owner) != _OWNER_KEYS:
        raise CredentialsError('credentials-shape')
    if not isinstance(owner['username'], str) or not _USERNAME.fullmatch(owner['username']):
        raise CredentialsError('credentials-shape')
    if not isinstance(owner['password'], str) or not _PASSWORD.fullmatch(owner['password']):
        raise CredentialsError('credentials-shape')
    members = document['members']
    if not isinstance(members, list) or not 1 <= len(members) <= 8:
        raise CredentialsError('credentials-shape')
    parsed = []
    usernames = {owner['username']}
    for member in members:
        if not isinstance(member, dict) or set(member) != _MEMBER_KEYS:
            raise CredentialsError('credentials-shape')
        if not isinstance(member['username'], str) or not _USERNAME.fullmatch(member['username']):
            raise CredentialsError('credentials-shape')
        if not isinstance(member['password'], str) or not _PASSWORD.fullmatch(member['password']):
            raise CredentialsError('credentials-shape')
        if not isinstance(member['name'], str) or not _NAME.fullmatch(member['name']):
            raise CredentialsError('credentials-shape')
        if member['username'] in usernames:
            raise CredentialsError('credentials-shape')
        usernames.add(member['username'])
        parsed.append(Member(member['username'], member['password'], member['name']))
    return Credentials(owner['username'], owner['password'], tuple(parsed))

def load_stdin(fd=0, limit=65536):
    """Read one bounded credential document from the inherited FD."""
    try:
        duplicate = os.dup(fd)
    except OSError:
        raise CredentialsError('credentials-stdin') from None
    try:
        with os.fdopen(duplicate, 'rb') as stream:
            raw = stream.read(limit + 1)
    except OSError:
        raise CredentialsError('credentials-stdin') from None
    if not raw:
        raise CredentialsError('credentials-stdin-required')
    if len(raw) > limit:
        raise CredentialsError('credentials-size')
    try:
        document = json.loads(raw)
    except ValueError:
        raise CredentialsError('credentials-invalid') from None
    return _validate(document)

def public_counts(credentials):
    return 1, len(credentials.members)

def digest(credentials):
    document = {
        'owner': {'username': credentials.owner_username, 'password': credentials.owner_password},
        'members': [{'username': member.username, 'password': member.password, 'name': member.name}
                    for member in credentials.members],
    }
    return hashlib.sha256(json.dumps(document, sort_keys=True, separators=(',', ':')).encode()).hexdigest()
