"""Atomic private status and receipt files for the walk coordinator.

Receipts contain only closed codes and bounded counts; they never contain a
credential, token, raw URL, request body or exception message. Server logs stay
in the private work root and are never copied here.
"""
from __future__ import annotations

import json
import os
import tempfile
from pathlib import Path

from .config import CREDENTIALS, now_iso

# The coordinator's closed operation vocabulary. A receipt may name only these
# values, so a failed ordinary setup/read call is identifiable without copying
# a raw path, identifier, query or message into the private output.
OPERATIONS = frozenset({
    'setup-browser', 'setup-owner', 'setup-state', 'setup-finish',
    'device-retire', 'system-read', 'console-settings-read', 'console-settings-write',
    'library-create', 'library-scan', 'scan-job-read', 'libraries-read',
    'reading-items-read', 'reading-session-open', 'periodicals-read',
    'reading-session-close', 'home-read', 'library-content-read',
    'member-create', 'member-sign-in', 'unclassified',
})

def write_json(path, document, mode=0o600):
    path = Path(path)
    descriptor, name = tempfile.mkstemp(prefix='.tmp-', dir=path.parent)
    try:
        os.fchmod(descriptor, mode)
        with os.fdopen(descriptor, 'w') as stream:
            json.dump(document, stream, indent=2, sort_keys=True)
            stream.write('\n')
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(name, path)
    finally:
        if os.path.exists(name):
            os.unlink(name)

def read_json(path, default=None):
    try:
        return json.loads(Path(path).read_text())
    except (OSError, ValueError):
        return default

def write_text(path, text, mode=0o644):
    path = Path(path)
    descriptor, name = tempfile.mkstemp(prefix='.tmp-', dir=path.parent)
    try:
        os.fchmod(descriptor, mode)
        with os.fdopen(descriptor, 'w') as stream:
            stream.write(text)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(name, path)
    finally:
        if os.path.exists(name):
            os.unlink(name)

def receipt(paths, name, code, status=None, facts=None, operation=None):
    document = {'name': name, 'code': code, 'at': now_iso()}
    if isinstance(status, int):
        document['status'] = status
    if operation in OPERATIONS:
        document['operation'] = operation
    if facts:
        document['facts'] = {key: value for key, value in facts.items()
                             if isinstance(value, (bool, int))}
    path = paths.receipts / (now_iso().replace(':', '').replace('-', '') + '-' + name + '.json')
    write_json(path, document)
    return path

def status_document(active, build, state, access_address, entry_verified, usable, note='', notice=''):
    port = active.get('port') if isinstance(active, dict) else None
    identity_retired = bool(active.get('identityRetired')) if isinstance(active, dict) else False
    return {
        'commit': build.get('commit') if isinstance(build, dict) else None,
        'startedAt': active.get('startedAt') if isinstance(active, dict) else None,
        'port': port,
        'listener': ('127.0.0.1:' + str(port)) if isinstance(port, int) else None,
        'address': access_address or 'loopback only; owner-approved tunnel required',
        'state': active.get('state') if isinstance(active, dict) else None,
        'schemaDigest': active.get('schemaDigest') if isinstance(active, dict) else None,
        'continuity': active.get('continuity') if isinstance(active, dict) else None,
        'identityRetired': identity_retired,
        'sessionsRetired': identity_retired,
        'firstDeployment': bool(active.get('firstDeployment')) if isinstance(active, dict) else False,
        'entryVerified': bool(entry_verified),
        'credentialsPath': str(CREDENTIALS),
        'usable': bool(usable),
        'note': note,
        'notice': notice,
        'updatedAt': now_iso(),
    }

def server_markdown(document):
    return (
        '# Walk server\n\n'
        '- Listener: ' + str(document.get('listener')) + '\n'
        '- Address: ' + str(document.get('address')) + '\n'
        '- Commit: ' + str(document.get('commit')) + '\n'
        '- Started: ' + str(document.get('startedAt')) + '\n'
        '- State: ' + str(document.get('state')) + '\n'
        '- Continuity: ' + str(document.get('continuity')) + '\n'
        '- Identity/sessions retired: ' + ('yes' if document.get('identityRetired') else 'no') + '\n'
        '- First deployment: ' + ('yes' if document.get('firstDeployment') else 'no') + '\n'
        '- Entry verified: ' + ('yes' if document.get('entryVerified') else 'no') + '\n'
        '- Usable: ' + ('yes' if document.get('usable') else 'no') + '\n'
        '- Credentials: ' + str(document.get('credentialsPath')) + ' (owner local 0600; never in Git, logs or receipts).\n'
        '- Note: ' + str(document.get('note') or 'none') + '\n'
        '- Notice: ' + str(document.get('notice') or 'none') + '\n'
    )

def publish(paths, active, build, state, access_address, entry_verified, usable, note='', notice=''):
    document = status_document(active, build, state, access_address, entry_verified, usable, note, notice)
    write_json(paths.status, document)
    if paths.server_md is not None:
        write_text(paths.server_md, server_markdown(document))
    return document
