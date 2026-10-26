"""Bounded server API flows for one isolated checkpoint runtime.

Each flow publishes only a closed operation id, pass/fail/unavailable status,
an optional HTTP status, a closed cause code, a closed reason and bounded
boolean/count facts. Titles, ids, tokens, URLs, bodies and exception text never
leave this module.
"""
from __future__ import annotations

import re
import secrets

from .client import CLOSED_CODES, sleep_within

FORMATS = frozenset({'epub', 'pdf', 'cbz', 'cb7'})
STATES = frozenset({'playing', 'paused', 'preparing', 'stopped', 'ended'})
LABELS = frozenset({
    'epub', 'pdf', 'cbz', 'cb7', 'movie', 'show', 'episode', 'album', 'artist', 'song', 'book',
    'collection', 'playlist', 'direct', 'remux', 'transcode', 'playing', 'paused', 'preparing',
    'stopped', 'ended', 'admission', 'other',
})
REASONS = frozenset({
    'timeout', 'transport', 'oversized', 'redirect', 'invalid-json', 'internal', 'unexpected',
    'deadline', 'not-owner', 'shape', 'identifier-shape', 'no-movie', 'no-title',
    'no-book-library', 'no-reading-session', 'no-reading-progress', 'no-reading-progress-save',
    'no-existing-reading-location', 'reading-source-mismatch', 'reading-location-unusable',
    'no-playback-session', 'no-queue', 'no-home-rows',
    'no-home-entries',
    'no-libraries', 'no-movie-library', 'no-library-entries', 'no-search-results',
    'detail-identity', 'detail-title', 'no-reading-items', 'no-reading-source',
    'reading-session-shape', 'reading-format', 'reading-revision-shape', 'no-reading-resources',
    'empty-resource', 'unsupported-format', 'progress-not-saved', 'progress-revision',
    'progress-missing', 'progress-value', 'guard-open', 'guard-code', 'queue-shape',
    'queue-identity', 'no-playback-versions', 'no-preferred', 'no-playback-plan',
    'session-identity', 'session-state', 'controls-shape',
    'own-session-absent', 'still-playing', 'stop-readback', 'queue-still-visible', 'me-shape',
    'downloads-settings-shape', 'console-settings-revision', 'console-settings-values',
    'configuration-shape',
})
_IDENTIFIER = re.compile(r'[A-Za-z0-9._~-]{1,128}\Z')
_HOME_ROW = re.compile(r'/v1/home/rows/[A-Za-z0-9._~-]{1,128}\Z')
_HEX = frozenset('0123456789abcdefABCDEF')
_FACT_KEY = re.compile(r'[a-z][a-zA-Z0-9]{0,31}\Z')

class Missing(Exception):
    """A flow cannot run because its prerequisite did not pass."""

    def __init__(self, reason):
        self.reason = reason if reason in REASONS else 'unexpected'

class FlowFail(Exception):
    """A flow ran and did not meet its contract."""

    def __init__(self, reply=None, code=None, reason=None):
        self.reply = reply
        self.code = code if code in CLOSED_CODES else None
        self.reason = reason if reason in REASONS else None

def count(value):
    if isinstance(value, bool) or not isinstance(value, int):
        return 0
    return max(0, min(value, 999))

def segment(value):
    if not isinstance(value, str) or not _IDENTIFIER.fullmatch(value):
        raise FlowFail(reason='identifier-shape')
    return value

def quoted_revision(value):
    if not isinstance(value, str) or len(value) != 64 or any(c not in _HEX for c in value):
        raise FlowFail(reason='reading-revision-shape')
    return '"' + value + '"'

def require(reply, expect=(200,)):
    if reply.failure is not None:
        raise FlowFail(reply, reason=reply.failure)
    if reply.status not in expect:
        raise FlowFail(reply, code=reply.code or 'unclassified')
    return reply

def object_data(reply, reason='shape'):
    if not isinstance(reply.data, dict):
        raise FlowFail(reply, reason=reason)
    return reply.data

class Receipt:
    def __init__(self, deadline):
        self.deadline = deadline
        self.flows = []

    def add(self, operation, status, *, http=None, code=None, reason=None, facts=None, label=None):
        record = {'surface': 'server', 'id': operation, 'status': status, 'kind': 'api', 'operation': operation}
        if isinstance(http, int) and 100 <= http <= 599:
            record['http'] = http
        if code in CLOSED_CODES:
            record['code'] = code
        if reason in REASONS:
            record['reason'] = reason
        elif status == 'fail' and 'code' not in record:
            record['reason'] = 'unexpected'
        if label in LABELS:
            record['label'] = label
        published = {}
        for key, value in (facts.items() if isinstance(facts, dict) else ()):
            if not isinstance(key, str) or not _FACT_KEY.fullmatch(key):
                continue
            if isinstance(value, bool):
                published[key] = value
            elif isinstance(value, int) and not isinstance(value, bool):
                published[key] = count(value)
        if published:
            record['facts'] = published
        self.flows.append(record)

    def run(self, operation, action):
        if self.deadline.expired():
            self.add(operation, 'unavailable', reason='deadline')
            return False
        try:
            facts = action()
        except Missing as error:
            self.add(operation, 'unavailable', reason=error.reason)
            return False
        except FlowFail as error:
            reply = error.reply
            code = error.code or (reply.code if reply is not None else None)
            self.add(operation, 'fail', http=reply.status if reply is not None else None,
                     code=code, reason=error.reason)
            return False
        except Exception:
            self.add(operation, 'fail', reason='internal', facts={'internalError': True})
            return False
        label = None
        if isinstance(facts, dict):
            candidate = facts.pop('label', None)
            if candidate in LABELS:
                label = candidate
        self.add(operation, 'pass', facts=facts, label=label)
        return True

    def document(self):
        return {
            'surface': 'server',
            'id': 'api-smoke',
            'runtimeProfile': 'isolated loopback API smoke; admission and server session state only, no playback-progress or production acceptance',
            'cleanup': 'finally path closes only the owned reading session, playback session and queue; the fixture item progress mark stays in the disposable run scratch; no scans or broad deletes',
            'flows': self.flows,
        }

class Smoke:
    def __init__(self, client, deadline, owner):
        self.client = client
        self.deadline = deadline
        self.owner = owner
        self.receipt = Receipt(deadline)
        self.movie = ''
        self.movie_title = ''
        self.movie_library = ''
        self.book_library = ''
        self.queue = ''
        self.session = ''
        self.reading_item = ''
        self.reading_source = ''
        self.reading_session = ''
        self.reading_revision = ''
        self.reading_format = ''
        self.reading_resources = []
        self.reading_progress_before = 0
        self.reading_progress_revision = 0
        self.reading_progress_ready = False
        self.reading_had_progress = False
        self.reading_source_matched = False
        self.reading_location = None
        self.reading_progress_saved = False

    def execute(self):
        run = self.receipt.run
        try:
            run('health-live', self.health_live)
            run('health-ready', self.health_ready)
            run('home-real-rows', self.home)
            run('library-real-content', self.library)
            run('detail-real-item', self.detail)
            run('search-real-title', self.search)
            run('reading-list', self.reading_list)
            run('reading-session-open', self.reading_session_open)
            run('reading-resource', self.reading_resource)
            run('reading-progress-read', self.reading_progress_read)
            run('reading-progress-save', self.reading_progress_save)
            run('reading-progress-confirm', self.reading_progress_confirm)
            run('reading-progress-stale-guard', self.reading_progress_stale)
            run('reading-session-close', self.reading_session_close)
            run('playback-options', self.playback_options)
            run('playback-queue-create', self.playback_queue_create)
            run('playback-queue-read', self.playback_queue_read)
            run('playback-start-admission', self.playback_start_admission)
            run('playback-preparing', self.playback_preparing)
            run('playback-current', self.playback_current)
            run('playback-controls-read', self.playback_controls)
            run('playback-stop', self.playback_stop)
            run('playback-queue-cleanup', self.playback_queue_cleanup)
            run('settings-me', self.settings_me)
            run('settings-downloads', self.settings_downloads)
            run('settings-console', self.settings_console)
            run('settings-configuration', self.settings_configuration)
        finally:
            self.cleanup()
        return self.receipt.document()

    def health_live(self):
        require(self.client.call('health-live', 'GET', '/health/live', auth=False, parse_json=False), (200,))
        return {}

    def health_ready(self):
        require(self.client.call('health-ready', 'GET', '/health/ready', auth=False, parse_json=False), (200,))
        return {}

    def home(self):
        reply = require(self.client.call('home-real-rows', 'GET', '/v1/home', query={'limit': 20}), (200,))
        document = object_data(reply)
        rows = document.get('rows')
        if not isinstance(rows, list) or not rows:
            raise FlowFail(reply, reason='no-home-rows')
        # GET /v1/home publishes the row manifest: every visible row is deferred
        # and composes its own preview at endpoint. Follow bounded rows in the
        # server's order until one publishes real entries, as the web client does.
        with_entries, movie, title, followed = 0, '', '', 0
        for row in rows[:24]:
            if not isinstance(row, dict):
                continue
            entries = row.get('entries')
            if not isinstance(entries, list) or not entries:
                endpoint = row.get('endpoint') if row.get('deferred') else None
                if not isinstance(endpoint, str) or not _HOME_ROW.fullmatch(endpoint):
                    continue
                if followed == 8:
                    break
                followed += 1
                row_reply = self.client.call('home-real-rows', 'GET', endpoint, query={'limit': 20})
                if row_reply.failure is not None or row_reply.status != 200:
                    continue
                row_document = row_reply.data
                if not isinstance(row_document, dict):
                    continue
                entries = row_document.get('entries')
            if not isinstance(entries, list) or not entries:
                continue
            with_entries += 1
            for entry in entries[:200]:
                if not isinstance(entry, dict) or entry.get('kind') != 'movie':
                    continue
                if isinstance(entry.get('id'), str) and entry['id']:
                    movie = entry['id']
                    title = entry.get('title') if isinstance(entry.get('title'), str) else ''
                    break
            break
        if with_entries == 0:
            raise FlowFail(reply, reason='no-home-entries')
        if movie and not self.movie:
            self.movie, self.movie_title = movie, title
        return {'rows': count(len(rows)), 'rowsWithEntries': count(with_entries), 'hasMovie': bool(movie)}

    def library(self):
        reply = require(self.client.call('library-real-content', 'GET', '/v1/libraries'), (200,))
        document = object_data(reply)
        items = document.get('items')
        if not isinstance(items, list) or not items:
            raise FlowFail(reply, reason='no-libraries')
        movie_library, book_library, music_library = '', '', False
        for item in items[:100]:
            if not isinstance(item, dict) or not isinstance(item.get('id'), str):
                continue
            kind = item.get('kind')
            if kind == 'movie' and not movie_library:
                movie_library = item['id']
            elif kind == 'book' and not book_library:
                book_library = item['id']
            elif kind == 'music':
                music_library = True
        self.movie_library, self.book_library = movie_library, book_library
        if not movie_library:
            raise FlowFail(reply, reason='no-movie-library')
        content = require(self.client.call('library-real-content', 'GET',
                                           '/v1/libraries/' + segment(movie_library) + '/content',
                                           query={'limit': 24}), (200,))
        content_document = object_data(content)
        sections = content_document.get('sections')
        entries = 0
        if isinstance(sections, list):
            for section in sections[:32]:
                if not isinstance(section, dict):
                    continue
                values = section.get('entries')
                if not isinstance(values, list):
                    continue
                entries += len(values)
                if not self.movie:
                    for entry in values[:200]:
                        if isinstance(entry, dict) and entry.get('kind') == 'movie' and isinstance(entry.get('id'), str) and entry['id']:
                            self.movie = entry['id']
                            self.movie_title = entry.get('title') if isinstance(entry.get('title'), str) else ''
                            break
        if entries == 0:
            raise FlowFail(content, reason='no-library-entries')
        return {'libraries': count(len(items)), 'hasMovieLibrary': True, 'hasBookLibrary': bool(book_library),
                'hasMusicLibrary': music_library, 'sections': count(len(sections) if isinstance(sections, list) else 0),
                'entries': count(entries)}

    def detail(self):
        if not self.movie:
            raise Missing('no-movie')
        reply = require(self.client.call('detail-real-item', 'GET', '/v1/items/' + segment(self.movie)), (200,))
        document = object_data(reply)
        if document.get('id') != self.movie:
            raise FlowFail(reply, reason='detail-identity')
        title = document.get('title')
        if not isinstance(title, str) or not title.strip():
            raise FlowFail(reply, reason='detail-title')
        kind = document.get('kind')
        return {'titlePresent': True, 'overviewPresent': bool(document.get('overview')),
                'posterPresent': bool(document.get('posterUrl')),
                'label': kind if kind in LABELS else 'other'}

    def search(self):
        if not self.movie_title:
            raise Missing('no-title')
        query = self.movie_title.strip()[:64]
        reply = require(self.client.call('search-real-title', 'GET', '/v1/search',
                                         query={'q': query, 'limit': 5}), (200,))
        document = object_data(reply)
        groups = document.get('groups')
        if not isinstance(groups, list):
            raise FlowFail(reply, reason='shape')
        results = 0
        for group in groups[:32]:
            if isinstance(group, dict) and isinstance(group.get('items'), list):
                results += len(group['items'])
        if results == 0:
            raise FlowFail(reply, reason='no-search-results')
        return {'groups': count(len(groups)), 'results': count(results)}

    def reading_list(self):
        if not self.book_library:
            raise Missing('no-book-library')
        reply = require(self.client.call('reading-list', 'GET', '/v1/reading/items',
                                         query={'libraryId': self.book_library, 'limit': 20}), (200,))
        document = object_data(reply)
        items = document.get('items')
        if not isinstance(items, list) or not items:
            raise FlowFail(reply, reason='no-reading-items')
        item_id, source_id, item_format = '', '', ''
        for item in items[:100]:
            if not isinstance(item, dict):
                continue
            if isinstance(item.get('id'), str) and isinstance(item.get('sourceId'), str) and item.get('format') in FORMATS:
                item_id, source_id, item_format = item['id'], item['sourceId'], item['format']
                break
        if not item_id or not source_id:
            raise FlowFail(reply, reason='no-reading-source')
        self.reading_item, self.reading_source = item_id, source_id
        self.reading_format = item_format
        return {'items': count(len(items)), 'hasSource': True, 'label': item_format}

    def reading_session_open(self):
        if not self.reading_item:
            raise Missing('no-reading-source')
        reply = require(self.client.call('reading-session-open', 'POST', '/v1/reading/sessions',
                                         body={'itemId': self.reading_item, 'sourceId': self.reading_source}), (200,))
        document = object_data(reply)
        session = document.get('sessionId')
        revision = document.get('revision')
        item_format = document.get('format')
        if not isinstance(session, str) or not session or not isinstance(revision, str) or not revision:
            raise FlowFail(reply, reason='reading-session-shape')
        self.reading_session = session
        if item_format not in FORMATS:
            raise FlowFail(reply, reason='reading-format')
        self.reading_format = item_format
        quoted_revision(revision)
        self.reading_revision = revision
        resources, spine, contents = document.get('resources'), document.get('spine'), document.get('contents')
        paths = []
        if isinstance(resources, list):
            for resource in resources[:256]:
                if not isinstance(resource, dict) or not isinstance(resource.get('path'), str) or not resource['path']:
                    continue
                size = resource.get('size')
                paths.append((resource['path'], size if isinstance(size, int) and not isinstance(size, bool) and size > 0 else 0))
        self.reading_resources = paths
        return {'hasSpine': bool(isinstance(spine, list) and spine), 'hasContents': bool(isinstance(contents, list) and contents),
                'resources': count(len(paths)), 'label': item_format}

    def reading_resource(self):
        if not self.reading_session:
            raise Missing('no-reading-session')
        match = quoted_revision(self.reading_revision)
        base = '/v1/reading/sessions/' + segment(self.reading_session)
        if self.reading_format == 'epub':
            if not self.reading_resources:
                raise FlowFail(reason='no-reading-resources')
            chosen = None
            for path, size in self.reading_resources:
                if size and size <= 2 * 1024 * 1024 and (chosen is None or size < chosen[1]):
                    chosen = (path, size)
            if chosen is None:
                chosen = (self.reading_resources[0][0], 0)
            reply = self.client.call('reading-resource', 'GET', base + '/assets',
                                     query={'path': chosen[0]}, headers={'If-Match': match},
                                     parse_json=False, limit=2 * 1024 * 1024)
            require(reply, (200,))
            if reply.body_length <= 0:
                raise FlowFail(reply, reason='empty-resource')
            return {'resources': count(len(self.reading_resources)), 'served': True, 'bytesPresent': True}
        if self.reading_format == 'pdf':
            reply = self.client.call('reading-resource', 'HEAD', base + '/source', headers={'If-Match': match},
                                     parse_json=False)
            require(reply, (200,))
            if not reply.content_length or reply.content_length <= 0:
                raise FlowFail(reply, reason='empty-resource')
            return {'sourceHead': True, 'bytesPresent': True}
        if self.reading_format in ('cbz', 'cb7'):
            reply = self.client.call('reading-resource', 'GET', base + '/pages/0', query={'width': 256},
                                     headers={'If-Match': match}, parse_json=False, limit=2 * 1024 * 1024)
            require(reply, (200,))
            if reply.body_length <= 0:
                raise FlowFail(reply, reason='empty-resource')
            return {'pageServed': True, 'bytesPresent': True}
        raise FlowFail(reason='unsupported-format')

    def _location_usable(self, location):
        if not isinstance(location, dict) or location.get('format') != self.reading_format:
            return False
        progression = location.get('progression')
        if isinstance(progression, bool) or not isinstance(progression, (int, float)) or not 0 <= float(progression) <= 1:
            return False
        if self.reading_format == 'epub':
            cfi = location.get('cfi')
            if not isinstance(cfi, str) or not cfi.startswith('epubcfi(/') or not cfi.endswith(')') or len(cfi) > 4096:
                return False
            if any(ord(c) < 32 for c in cfi) or '<' in cfi or '>' in cfi:
                return False
            section = location.get('section')
            if section is not None:
                if not isinstance(section, str) or len(section) > 1024:
                    return False
                if section and (section.startswith('/') or '\\' in section or '..' in section.split('/')):
                    return False
            if location.get('pageIndex') is not None:
                return False
            return True
        if self.reading_format in ('pdf', 'cbz', 'cb7'):
            page = location.get('pageIndex')
            if isinstance(page, bool) or not isinstance(page, int) or not 0 <= page <= 1000000:
                return False
            if location.get('cfi') or location.get('section'):
                return False
            return True
        return False

    def _require_existing_location(self):
        if not self.reading_session:
            raise Missing('no-reading-session')
        if not self.reading_progress_ready:
            raise Missing('no-reading-progress')
        if not self.reading_had_progress:
            raise Missing('no-existing-reading-location')
        if not self.reading_source_matched:
            raise Missing('reading-source-mismatch')
        if not self.reading_location:
            raise Missing('reading-location-unusable')

    def reading_progress_read(self):
        if not self.reading_session:
            raise Missing('no-reading-session')
        reply = require(self.client.call('reading-progress-read', 'GET',
                                         '/v1/reading/sessions/' + segment(self.reading_session) + '/progress',
                                         headers={'If-Match': quoted_revision(self.reading_revision)}), (200,))
        document = object_data(reply)
        progress = document.get('progress')
        revision = progress.get('revision') if isinstance(progress, dict) else None
        revision = revision if isinstance(revision, int) and not isinstance(revision, bool) and revision >= 0 else 0
        self.reading_progress_before = revision
        self.reading_progress_revision = revision
        self.reading_progress_ready = True
        self.reading_progress_saved = False
        self.reading_had_progress = isinstance(progress, dict)
        self.reading_source_matched = False
        self.reading_location = None
        if self.reading_had_progress:
            source = progress.get('sourceRevision')
            self.reading_source_matched = isinstance(source, str) and source == self.reading_revision
            if self.reading_source_matched and self._location_usable(progress.get('location')):
                self.reading_location = progress.get('location')
        return {'hadProgress': self.reading_had_progress, 'sourceMatched': self.reading_source_matched,
                'locationUsable': self.reading_location is not None}

    def reading_progress_save(self):
        self._require_existing_location()
        reply = require(self.client.call('reading-progress-save', 'PUT',
                                         '/v1/reading/sessions/' + segment(self.reading_session) + '/progress',
                                         body={'expectedRevision': self.reading_progress_before,
                                               'location': self.reading_location},
                                         headers={'If-Match': quoted_revision(self.reading_revision)}), (200,))
        document = object_data(reply)
        progress = document.get('progress')
        if document.get('saved') is not True or not isinstance(progress, dict):
            raise FlowFail(reply, reason='progress-not-saved')
        revision = progress.get('revision')
        if not isinstance(revision, int) or isinstance(revision, bool) or revision <= self.reading_progress_before:
            raise FlowFail(reply, reason='progress-revision')
        self.reading_progress_revision = revision
        self.reading_progress_saved = True
        return {'saved': True, 'revisionAdvanced': True}

    def reading_progress_confirm(self):
        self._require_existing_location()
        if not self.reading_progress_saved:
            raise Missing('no-reading-progress-save')
        reply = require(self.client.call('reading-progress-confirm', 'GET',
                                         '/v1/reading/sessions/' + segment(self.reading_session) + '/progress',
                                         headers={'If-Match': quoted_revision(self.reading_revision)}), (200,))
        document = object_data(reply)
        progress = document.get('progress')
        if not isinstance(progress, dict):
            raise FlowFail(reply, reason='progress-missing')
        if progress.get('sourceRevision') != self.reading_revision:
            raise FlowFail(reply, reason='reading-source-mismatch')
        if progress.get('location') != self.reading_location:
            raise FlowFail(reply, reason='progress-value')
        if progress.get('revision') != self.reading_progress_revision:
            raise FlowFail(reply, reason='progress-revision')
        return {'matches': True, 'revisionAdvanced': True}

    def reading_progress_stale(self):
        self._require_existing_location()
        if not self.reading_progress_saved:
            raise Missing('no-reading-progress-save')
        reply = self.client.call('reading-progress-stale-guard', 'PUT',
                                 '/v1/reading/sessions/' + segment(self.reading_session) + '/progress',
                                 body={'expectedRevision': self.reading_progress_before,
                                       'location': self.reading_location},
                                 headers={'If-Match': quoted_revision(self.reading_revision)}, expect=(412,))
        if reply.failure is not None:
            raise FlowFail(reply, reason=reply.failure)
        if reply.status not in (412,):
            raise FlowFail(reply, reason='guard-open')
        if reply.code != 'revision_mismatch':
            raise FlowFail(reply, reason='guard-code')
        return {'rejected': True}

    def reading_session_close(self):
        if not self.reading_session:
            raise Missing('no-reading-session')
        reply = self.client.call('reading-session-close', 'DELETE',
                                 '/v1/reading/sessions/' + segment(self.reading_session), expect=(204,))
        if reply.failure is not None:
            raise FlowFail(reply, reason=reply.failure)
        if reply.status != 204:
            raise FlowFail(reply, code=reply.code or 'unclassified')
        self.reading_session = ''
        return {'closed': True}

    def playback_options(self):
        if not self.movie:
            raise Missing('no-movie')
        reply = require(self.client.call('playback-options', 'GET',
                                         '/v1/items/' + segment(self.movie) + '/playback-options'), (200,))
        document = object_data(reply)
        versions, preferred, plan = document.get('versions'), document.get('preferred'), document.get('plan')
        if not isinstance(versions, list) or not versions:
            raise FlowFail(reply, reason='no-playback-versions')
        if not isinstance(preferred, dict):
            raise FlowFail(reply, reason='no-preferred')
        if not isinstance(plan, dict):
            raise FlowFail(reply, reason='no-playback-plan')
        mode = plan.get('mode')
        return {'versions': count(len(versions)), 'hasPreferred': True, 'hasPlan': True,
                'label': mode if mode in ('direct', 'remux', 'transcode') else 'other'}

    def playback_queue_create(self):
        if not self.movie:
            raise Missing('no-movie')
        key = secrets.token_urlsafe(24)
        reply = self.client.call('playback-queue-create', 'POST', '/v1/queues',
                                 body={'selector': {'items': {'ids': [self.movie]}}, 'repeat': 'off'},
                                 headers={'Idempotency-Key': key}, expect=(201, 202))
        if reply.failure is not None:
            raise FlowFail(reply, reason=reply.failure)
        if reply.status not in (201, 202):
            raise FlowFail(reply, code=reply.code or 'unclassified')
        document = object_data(reply)
        queue = document.get('queue')
        if not isinstance(queue, dict) or not isinstance(queue.get('id'), str) or not queue['id']:
            raise FlowFail(reply, reason='queue-shape')
        self.queue = queue['id']
        total = queue.get('total')
        return {'created': True, 'entries': count(total if isinstance(total, int) else 0),
                'prepared': reply.status == 202, 'hasCurrent': isinstance(queue.get('current'), dict)}

    def playback_queue_read(self):
        if not self.queue:
            raise Missing('no-queue')
        reply = require(self.client.call('playback-queue-read', 'GET', '/v1/queues/' + segment(self.queue)), (200,))
        document = object_data(reply)
        if document.get('id') != self.queue:
            raise FlowFail(reply, reason='queue-identity')
        total = document.get('total')
        return {'entries': count(total if isinstance(total, int) else 0),
                'hasCurrent': isinstance(document.get('current'), dict)}

    def playback_start_admission(self):
        if not self.movie:
            raise Missing('no-movie')
        key = secrets.token_urlsafe(24)
        reply = self.client.call('playback-start', 'POST', '/v1/playback/sessions',
                                 body={'itemId': self.movie, 'state': 'playing'},
                                 headers={'Idempotency-Key': key}, expect=(201, 202))
        for attempt in range(3):
            if reply.failure is not None:
                raise FlowFail(reply, reason=reply.failure)
            document = reply.data if isinstance(reply.data, dict) else {}
            candidate = document.get('id')
            if isinstance(candidate, str) and candidate:
                self.session = candidate
            if reply.status == 201:
                break
            if reply.status != 202:
                raise FlowFail(reply, code=reply.code or 'unclassified')
            if attempt == 2:
                break
            if not sleep_within(self.deadline, 1.0):
                break
            reply = self.client.call('playback-start', 'POST', '/v1/playback/sessions',
                                     body={'itemId': self.movie, 'state': 'playing'},
                                     headers={'Idempotency-Key': key}, expect=(201, 202))
        document = object_data(reply)
        session = document.get('id')
        state = document.get('state')
        if not isinstance(session, str) or not session:
            raise FlowFail(reply, reason='session-identity')
        if state not in ('playing', 'paused', 'preparing'):
            raise FlowFail(reply, reason='session-state')
        self.session = session
        return {'admitted': True, 'sessionCreated': True,
                'preparing': state == 'preparing' or reply.status == 202, 'label': 'admission'}

    def playback_preparing(self):
        if not self.session:
            raise Missing('no-playback-session')
        reply = require(self.client.call('playback-preparing', 'GET',
                                         '/v1/playback/sessions/' + segment(self.session)), (200,))
        document = object_data(reply)
        if document.get('id') != self.session:
            raise FlowFail(reply, reason='session-identity')
        state = document.get('state')
        if state not in STATES:
            raise FlowFail(reply, reason='session-state')
        return {'preparing': state == 'preparing', 'serverStateKnown': True, 'label': state}

    def playback_current(self):
        if not self.session:
            raise Missing('no-playback-session')
        reply = require(self.client.call('playback-current', 'GET',
                                         '/v1/playback/sessions/' + segment(self.session)), (200,))
        document = object_data(reply)
        if document.get('id') != self.session:
            raise FlowFail(reply, reason='session-identity')
        state = document.get('state')
        if state not in STATES:
            raise FlowFail(reply, reason='session-state')
        return {'serverStatePlaying': state == 'playing', 'serverStatePaused': state == 'paused',
                'serverStatePreparing': state == 'preparing', 'serverStateKnown': True,
                'observedPlayback': isinstance(document.get('observedPlayback'), dict), 'label': state}

    def playback_controls(self):
        if not self.session:
            raise Missing('no-playback-session')
        reply = require(self.client.call('playback-controls-read', 'GET', '/v1/playback/controls',
                                         query={'limit': 20}), (200,))
        document = object_data(reply)
        items = document.get('items')
        if not isinstance(items, list):
            raise FlowFail(reply, reason='controls-shape')
        found = False
        for item in items[:100]:
            if isinstance(item, dict) and item.get('sessionId') == self.session:
                found = True
                break
        if not found:
            raise FlowFail(reply, reason='own-session-absent')
        return {'devices': count(len(items)), 'containsOwnSession': True}

    def playback_stop(self):
        if not self.session:
            raise Missing('no-playback-session')
        session = self.session
        reply = self.client.call('playback-stop', 'DELETE', '/v1/playback/sessions/' + segment(session),
                                 expect=(204,))
        if reply.failure is not None:
            raise FlowFail(reply, reason=reply.failure)
        if reply.status != 204:
            raise FlowFail(reply, code=reply.code or 'unclassified')
        self.session = ''
        readback = self.client.call('playback-stop-read', 'GET', '/v1/playback/sessions/' + segment(session),
                                    expect=(200, 404))
        if readback.failure is not None:
            raise FlowFail(readback, reason=readback.failure)
        if readback.status == 404:
            if readback.code != 'not_found':
                raise FlowFail(readback, reason='stop-readback')
            return {'stopped': True, 'ended': True}
        document = readback.data if isinstance(readback.data, dict) else {}
        state = document.get('state')
        if state not in ('stopped', 'ended') and not isinstance(document.get('end'), dict):
            raise FlowFail(readback, reason='still-playing')
        return {'stopped': True, 'ended': True}

    def playback_queue_cleanup(self):
        if not self.queue:
            raise Missing('no-queue')
        queue = self.queue
        reply = self.client.call('playback-queue-cleanup', 'DELETE', '/v1/queues/' + segment(queue), expect=(204,))
        if reply.failure is not None:
            raise FlowFail(reply, reason=reply.failure)
        if reply.status != 204:
            raise FlowFail(reply, code=reply.code or 'unclassified')
        self.queue = ''
        readback = self.client.call('playback-queue-cleanup', 'GET', '/v1/queues/' + segment(queue), expect=(404,))
        if readback.failure is not None:
            raise FlowFail(readback, reason=readback.failure)
        if readback.status != 404 or readback.code != 'not_found':
            raise FlowFail(readback, reason='queue-still-visible')
        return {'deleted': True}

    def settings_me(self):
        reply = require(self.client.call('settings-me', 'GET', '/v1/me'), (200,))
        document = object_data(reply, 'me-shape')
        viewer = document.get('viewer')
        if not isinstance(viewer, dict) or not viewer:
            raise FlowFail(reply, reason='me-shape')
        return {'hasViewer': True}

    def settings_downloads(self):
        reply = require(self.client.call('settings-downloads', 'GET', '/v1/downloads/settings'), (200,))
        document = object_data(reply, 'downloads-settings-shape')
        if not document:
            raise FlowFail(reply, reason='downloads-settings-shape')
        return {'hasRevision': isinstance(document.get('revision'), int),
                'hasQualities': isinstance(document.get('qualities'), list)}

    def settings_console(self):
        if not self.owner:
            raise Missing('not-owner')
        reply = require(self.client.call('settings-console', 'GET', '/v1/admin/console/settings'), (200,))
        document = object_data(reply, 'console-settings-values')
        data = document.get('data')
        if not isinstance(data, dict):
            raise FlowFail(reply, reason='console-settings-values')
        # The console envelope is {data, scope}; the settings revision and the
        # requested/effective values live inside data (data.revision, data.requested).
        if not isinstance(data.get('revision'), int):
            raise FlowFail(reply, reason='console-settings-revision')
        requested = data.get('requested')
        if not isinstance(requested, dict):
            raise FlowFail(reply, reason='console-settings-values')
        return {'hasRevision': True, 'hasRequested': True}

    def settings_configuration(self):
        if not self.owner:
            raise Missing('not-owner')
        reply = require(self.client.call('settings-configuration', 'GET', '/v1/admin/settings/configuration'), (200,))
        document = object_data(reply, 'configuration-shape')
        if not isinstance(document.get('data'), dict):
            raise FlowFail(reply, reason='configuration-shape')
        return {'hasData': True, 'hasScope': isinstance(document.get('scope'), dict)}

    def _close(self, operation, path, attribute):
        reply = self.client.call(operation, 'DELETE', path, expect=(204,), force=True)
        closed = reply.failure is None and reply.status == 204
        if closed:
            setattr(self, attribute, '')
        return closed, reply

    def cleanup(self):
        attempts = []
        if self.reading_session:
            try:
                attempts.append(self._close('cleanup-reading-session',
                                            '/v1/reading/sessions/' + segment(self.reading_session), 'reading_session'))
            except Exception:
                pass
        if self.session:
            try:
                attempts.append(self._close('cleanup-playback-session',
                                            '/v1/playback/sessions/' + segment(self.session), 'session'))
            except Exception:
                pass
        if self.queue:
            try:
                attempts.append(self._close('cleanup-queue', '/v1/queues/' + segment(self.queue), 'queue'))
            except Exception:
                pass
        all_closed = not (self.reading_session or self.session or self.queue)
        code, reason = None, None
        for closed, reply in attempts:
            if closed:
                continue
            all_closed = False
            if code is None and reply.code in CLOSED_CODES:
                code = reply.code
            if reason is None and reply.failure in REASONS:
                reason = reply.failure
        if not all_closed and code is None and reason is None:
            reason = 'unexpected'
        self.receipt.add('cleanup-owned-handles', 'pass' if all_closed else 'fail', code=code, reason=reason,
                         facts={'handles': count(len(attempts)), 'allClosed': all_closed})
