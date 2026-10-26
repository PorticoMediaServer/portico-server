"""Prepare a fresh disposable state with ordinary local setup APIs.

Every account, profile and library is created through the published HTTP API.
There is no SQL seeding, auth-state injection or device-approval/pending-grant
path. The setup runs on a staging loopback port, is verified, then the server is
stopped; the portable state is activated later on the owned walk port.
"""
from __future__ import annotations

import secrets
import socket
import time
import uuid
from urllib.parse import quote
from dataclasses import dataclass
from pathlib import Path

from .api import ApiError, Client
from .config import SCAN_TIMEOUT, now_iso, safe_component
from .credentials import digest as credential_digest
from .receipt import read_json, write_json
from .runtime import RuntimeFailure, start as start_runtime, stop as stop_runtime

class StateError(Exception):
    def __init__(self, code, status=None):
        self.code = code
        self.status = status

@dataclass(frozen=True)
class PreparedState:
    commit: str
    name: str
    directory: Path
    data: Path
    cache: Path
    transcode: Path
    manifest: Path
    server_id: str
    home_rows: int
    libraries: int
    members: int
    created_at: str
    schema_digest: str
    credentials_digest: str
    fixtures_manifest_sha256: str
    runtime_sha256: str

def segment(value):
    if not isinstance(value, str) or not 1 <= len(value) <= 4096 or any(ord(c) < 32 or ord(c) == 127 for c in value):
        raise StateError('api-reference')
    return quote(value, safe='')

def _call(client, method, path, body=None, **kwargs):
    try:
        return client.call(method, path, body=body, **kwargs)
    except ApiError as error:
        raise StateError(error.code, error.status) from None

def _free_port(exclude):
    for _ in range(20):
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        if port != exclude:
            return port
    raise StateError('staging-port')

def _setup(client, port, credentials, fixtures, commit):
    installation = secrets.token_urlsafe(32)
    origin = 'http://127.0.0.1:' + str(port)
    setup = _call(client, 'POST', '/v1/setup/browser', {}, installation=installation, origin=origin)
    token = setup.get('setupToken') if isinstance(setup, dict) else None
    if not isinstance(token, str) or not token:
        raise StateError('setup-shape')
    owner = _call(client, 'POST', '/v1/setup', {
        'setupToken': token,
        'requestId': str(uuid.uuid4()),
        'name': 'Portico walks',
        'authMode': 'local',
        'interactive': True,
        'username': credentials.owner_username,
        'password': credentials.owner_password,
    }, installation=installation, origin=origin, expect=(201,))
    session = owner.get('session', owner) if isinstance(owner, dict) else None
    if not isinstance(session, dict):
        raise StateError('setup-shape')
    owner_token = session.get('accessToken')
    owner_device = session.get('deviceId')
    if not isinstance(owner_token, str) or not isinstance(owner_device, str):
        raise StateError('setup-shape')
    onboarding = _call(client, 'GET', '/v1/setup/state', token=owner_token, installation=installation)
    if not isinstance(onboarding, dict) or 'revision' not in onboarding:
        raise StateError('setup-shape')
    _call(client, 'POST', '/v1/setup/finish', {'revision': onboarding['revision']},
          token=owner_token, installation=installation)
    _conversion(client, owner_token, installation)
    libraries = _libraries(client, owner_token, installation, fixtures)
    home_rows = _verify_home(client, owner_token, installation)
    reading_items = _verify_reading(client, owner_token, installation, libraries['Books'])
    member_count = _members(client, owner_token, installation, credentials, libraries)
    _call(client, 'DELETE', '/v1/devices/' + segment(owner_device),
          token=owner_token, installation=installation, expect=(204,))
    system = _call(client, 'GET', '/v1/system')
    if not isinstance(system, dict) or system.get('setupRequired') is not False or system.get('signIn') != 'local':
        raise StateError('system-shape')
    if commit[:8] not in str(system.get('version', '')):
        raise StateError('version-mismatch')
    return {
        'serverId': system.get('id'),
        'homeRows': home_rows,
        'libraries': len(libraries),
        'members': member_count,
        'readingItems': reading_items,
        'conversionMode': 'all',
    }

def _conversion(client, token, installation):
    document = _call(client, 'GET', '/v1/admin/console/settings', token=token, installation=installation)
    if not isinstance(document, dict):
        raise StateError('settings-shape')
    data = document.get('data')
    if not isinstance(data, dict) or not isinstance(data.get('revision'), int):
        raise StateError('settings-shape')
    requested = data.get('requested') if isinstance(data, dict) else None
    effective = data.get('effective') if isinstance(data, dict) else None
    if not isinstance(requested, dict) or not isinstance(effective, dict):
        raise StateError('settings-shape')
    values = dict(requested)
    values['conversionMode'] = 'all'
    if requested.get('conversionMode') != 'all' or effective.get('conversionMode') != 'all':
        changed = _call(client, 'PATCH', '/v1/admin/console/settings', {
            'expectedRevision': data['revision'],
            'idempotencyKey': str(uuid.uuid4()),
            'values': values,
        }, token=token, installation=installation)
        data = changed.get('data') if isinstance(changed, dict) else None
        effective = data.get('effective') if isinstance(data, dict) else None
        if not isinstance(effective, dict) or effective.get('conversionMode') != 'all':
            raise StateError('conversion-mode')

def _library(client, token, installation, name, kind, path):
    if not Path(path).is_dir():
        raise StateError('fixture-library-missing')
    created = _call(client, 'POST', '/v1/libraries', {
        'name': name,
        'kind': kind,
        'path': str(path),
        'metadataAgent': 'local',
    }, token=token, installation=installation, expect=(200, 201))
    library = created.get('library', created) if isinstance(created, dict) else None
    if not isinstance(library, dict) or not isinstance(library.get('id'), str):
        raise StateError('library-shape')
    library_id = library['id']
    segment(library_id)
    job = _call(client, 'POST', '/v1/libraries/' + segment(library_id) + '/scans', {},
                token=token, installation=installation, expect=(200, 201, 202))
    job_doc = job.get('job', job) if isinstance(job, dict) else None
    job_id = job_doc.get('jobId', job_doc.get('id')) if isinstance(job_doc, dict) else None
    if not isinstance(job_id, str):
        raise StateError('scan-shape')
    segment(job_id)
    return library_id, job_id

def _libraries(client, token, installation, fixtures):
    media = fixtures.media
    created = {}
    jobs = []
    for name, kind, relative in (
        ('Movies', 'movie', 'Movies'),
        ('Music', 'music', 'Music'),
        ('Books', 'book', 'Read/Books'),
    ):
        library_id, job_id = _library(client, token, installation, name, kind, media / relative)
        created[name] = library_id
        jobs.append(job_id)
    # Optional walk publications, added only when the owner has staged the
    # directory. Required Movies/Music/Books above never depend on these.
    for name, kind, relative in (
        ('Comics', 'comic', 'Walks/Comics'),
        ('PDF', 'book', 'Walks/PDF'),
        ('Magazines', 'book', 'Walks/Magazines'),
        ('Mixed', 'book', 'Walks/Mixed'),
    ):
        if not (media / relative).is_dir():
            continue
        library_id, job_id = _library(client, token, installation, name, kind, media / relative)
        created[name] = library_id
        jobs.append(job_id)
    deadline = time.monotonic() + SCAN_TIMEOUT
    while time.monotonic() < deadline:
        states = []
        for job in jobs:
            document = _call(client, 'GET', '/v1/admin/ingestion/jobs/' + segment(job),
                             token=token, installation=installation)
            item = document.get('job', document) if isinstance(document, dict) else None
            states.append(item.get('status') if isinstance(item, dict) else None)
        if all(state in ('complete', 'complete_with_warnings', 'completed', 'failed', 'cancelled') for state in states):
            if any(state in ('failed', 'cancelled') for state in states):
                raise StateError('library-scan-failed')
            break
        time.sleep(2)
    else:
        raise StateError('library-scan-timeout')
    listed = _call(client, 'GET', '/v1/libraries', token=token, installation=installation)
    items = listed.get('items') if isinstance(listed, dict) else None
    if not isinstance(items, list) or len(items) < len(created):
        raise StateError('libraries-missing')
    return created

def _verify_home(client, token, installation):
    home = _call(client, 'GET', '/v1/home', token=token, installation=installation)
    rows = home.get('rows') if isinstance(home, dict) else None
    if not isinstance(rows, list) or not rows:
        raise StateError('home-empty')
    return len(rows)

def _verify_reading(client, token, installation, book_library):
    page = _call(client, 'GET', '/v1/reading/items', query={'libraryId': book_library, 'limit': 5},
                 token=token, installation=installation)
    items = page.get('items') if isinstance(page, dict) else None
    if not isinstance(items, list) or not items:
        raise StateError('reading-empty')
    return len(items)

def _verify_member_browse(client, token, installation, libraries):
    listed = _call(client, 'GET', '/v1/libraries', token=token, installation=installation)
    items = listed.get('items') if isinstance(listed, dict) else None
    if not isinstance(items, list):
        raise StateError('member-library-access')
    visible = {item.get('id') for item in items if isinstance(item, dict)}
    if not set(libraries.values()) <= visible:
        raise StateError('member-library-access')
    content = _call(client, 'GET', '/v1/libraries/' + segment(libraries['Movies']) + '/content',
                    query={'limit': 5}, token=token, installation=installation)
    sections = content.get('sections') if isinstance(content, dict) else None
    if not isinstance(sections, list) or not any(isinstance(section, dict) and section.get('entries')
                                                 for section in sections):
        raise StateError('member-library-access')
    reading = _call(client, 'GET', '/v1/reading/items',
                    query={'libraryId': libraries['Books'], 'limit': 5},
                    token=token, installation=installation)
    reading_items = reading.get('items') if isinstance(reading, dict) else None
    if not isinstance(reading_items, list) or not reading_items:
        raise StateError('member-library-access')
    if 'Comics' in libraries:
        comics = _call(client, 'GET', '/v1/libraries/' + segment(libraries['Comics']) + '/content',
                       query={'limit': 5}, token=token, installation=installation)
        comic_sections = comics.get('sections') if isinstance(comics, dict) else None
        if not isinstance(comic_sections, list) or not any(isinstance(section, dict) and section.get('entries')
                                                           for section in comic_sections):
            raise StateError('member-library-access')

def _members(client, token, installation, credentials, libraries):
    allowed = list(libraries.values())
    count = 0
    for member in credentials.members:
        _call(client, 'POST', '/v1/direct/members', {
            'username': member.username,
            'password': member.password,
            'name': member.name,
            'allowedLibraries': allowed,
        }, token=token, installation=installation, expect=(201,))
        member_installation = secrets.token_urlsafe(32)
        signed = _call(client, 'POST', '/v1/direct/sign-in', {
            'username': member.username,
            'password': member.password,
        }, installation=member_installation)
        member_session = signed.get('session', signed) if isinstance(signed, dict) else None
        if not isinstance(member_session, dict):
            raise StateError('member-sign-in')
        member_token = member_session.get('accessToken')
        member_device = member_session.get('deviceId')
        if not isinstance(member_token, str) or not isinstance(member_device, str):
            raise StateError('member-sign-in')
        _verify_member_browse(client, member_token, member_installation, libraries)
        _call(client, 'DELETE', '/v1/devices/' + segment(member_device),
              token=member_token, installation=member_installation, expect=(204,))
        count += 1
    return count

def prepare(build, fixtures, credentials, paths, active_port):
    name = safe_component(build.commit[:8] + '-' + now_iso().replace(':', '').replace('-', '')
                          + '-' + secrets.token_hex(2))
    directory = paths.states / name
    directory.mkdir(mode=0o700, parents=True, exist_ok=False)
    staging = _free_port(active_port)
    runtime = None
    try:
        runtime = start_runtime(build, directory, staging, paths, fixtures.runtime)
        client = Client(staging)
        facts = _setup(client, staging, credentials, fixtures, build.commit)
    except RuntimeFailure as error:
        raise StateError(error.args[0] if error.args else 'runtime') from None
    finally:
        if runtime is not None:
            stop_runtime(runtime)
    manifest = {
        'commit': build.commit,
        'name': name,
        'createdAt': now_iso(),
        'fixturesManifestSHA256': fixtures.manifest_sha256,
        'runtimeSHA256': fixtures.runtime_sha256,
        'schemaDigest': build.schema_digest,
        'credentialsDigest': credential_digest(credentials),
        'serverId': facts.get('serverId'),
        'homeRows': facts.get('homeRows'),
        'libraries': facts.get('libraries'),
        'members': facts.get('members'),
        'readingItems': facts.get('readingItems'),
        'conversionMode': facts.get('conversionMode'),
        'state': 'prepared',
        'runtimeVerified': False,
        'usable': False,
    }
    manifest_path = directory / 'state.json'
    write_json(manifest_path, manifest)
    return PreparedState(commit=build.commit, name=name, directory=directory,
                         data=directory / 'data', cache=directory / 'cache',
                         transcode=directory / 'transcode', manifest=manifest_path,
                         server_id=str(facts.get('serverId') or ''),
                         home_rows=int(facts.get('homeRows') or 0),
                         libraries=int(facts.get('libraries') or 0),
                         members=int(facts.get('members') or 0),
                         created_at=manifest['createdAt'],
                         schema_digest=build.schema_digest,
                         credentials_digest=credential_digest(credentials),
                         fixtures_manifest_sha256=fixtures.manifest_sha256,
                         runtime_sha256=fixtures.runtime_sha256)

def load(paths, name):
    directory = paths.states / safe_component(name)
    manifest_path = directory / 'state.json'
    manifest = read_json(manifest_path)
    if not isinstance(manifest, dict) or not (directory / 'data').is_dir():
        return None
    schema = manifest.get('schemaDigest')
    if not isinstance(schema, str) or len(schema) != 64:
        return None
    return PreparedState(commit=str(manifest.get('commit') or ''), name=directory.name,
                         directory=directory, data=directory / 'data', cache=directory / 'cache',
                         transcode=directory / 'transcode', manifest=manifest_path,
                         server_id=str(manifest.get('serverId') or ''),
                         home_rows=int(manifest.get('homeRows') or 0),
                         libraries=int(manifest.get('libraries') or 0),
                         members=int(manifest.get('members') or 0),
                         created_at=str(manifest.get('createdAt') or ''),
                         schema_digest=schema,
                         credentials_digest=str(manifest.get('credentialsDigest') or ''),
                         fixtures_manifest_sha256=str(manifest.get('fixturesManifestSHA256') or ''),
                         runtime_sha256=str(manifest.get('runtimeSHA256') or ''))
