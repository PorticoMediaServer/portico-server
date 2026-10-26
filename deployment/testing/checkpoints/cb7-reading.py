#!/usr/bin/env python3
"""Bounded one-page CB7 Reading qualification against an admitted runtime.

Proves one narrow fact on a server built with `release,portico_cb7`: the ordinary
Reading path lists a real CB7 item, opens an owned Reading session for its exact
source, serves page 0 as real image bytes under that session's publication
revision, and closes that session.

Corrected boundaries:

- the page is a binary blob, and `api_smoke.Client` returns no data for a
  non-JSON reply, so this helper owns one bounded loopback byte fetch and the
  shared client stays unchanged;
- the real handler requires exactly one `If-Match: "<revision>"` (66 characters
  including quotes) on the page request, so the revision is validated as 64
  lowercase hex characters and quoted; a page is requested only at index 0;
- custody of the admitted runtime is rechecked before every read and mutation,
  and a session that the server acknowledged is never forgotten: it is closed
  from `finally` even when the check already refused;
- a failed or unconfirmed close is `failed` even when the check had refused, and
  the original closed failure is kept separately in `failure`.

Closed output only: item and session format classes, revision class, page byte
count, page magic class, close status and one reason. No token, URL, title, id,
revision value or body text is printed.
"""
from __future__ import annotations

import argparse
import http.client
import json
import os
import re
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from api_smoke import ContextError
from api_smoke.client import Client, Deadline
from api_smoke.context import admitted_origin, load
from api_smoke.custody import verify

MAX_PAGE = 4 * 1024 * 1024
MIN_PAGE = 512
REVISION = re.compile(r'[0-9a-f]{64}\Z')
MEDIA_TYPES = frozenset({'image/png', 'image/jpeg', 'image/gif', 'image/webp'})
PAGE_PATH = re.compile(r'/v1/reading/sessions/[A-Za-z0-9._~-]{1,256}/pages/0\Z')


class CheckError(Exception):
    def __init__(self, reason):
        super().__init__(reason)
        self.reason = reason


def magic_class(sample):
    """Strict image signature; a bare RIFF header is not WebP."""
    if not isinstance(sample, (bytes, bytearray)):
        return 'short'
    head = bytes(sample[:16])
    if head.startswith(b'\x89PNG\r\n\x1a\n'):
        return 'png'
    if head.startswith(b'\xff\xd8\xff'):
        return 'jpeg'
    if head.startswith(b'GIF87a') or head.startswith(b'GIF89a'):
        return 'gif'
    if len(head) >= 12 and head.startswith(b'RIFF') and head[8:12] == b'WEBP':
        return 'webp'
    return 'short' if len(head) < 8 else 'other'


class FencedClient:
    """Custody around every JSON call, without erasing an acknowledged mutation.

    The runtime is fenced before each call and again after a read; a mutation is
    never followed by a post-assert, because the server may already have
    acknowledged real state that still has to be closed from `finally`.
    """
    READS = frozenset({'GET', 'HEAD', 'OPTIONS'})

    def __init__(self, client, context, custody=verify):
        self.client = client
        self.context = context
        self.custody = custody

    def call(self, operation, method, path, **kwargs):
        self.custody(self.context)
        reply = self.client.call(operation, method, path, **kwargs)
        if method in self.READS:
            self.custody(self.context)
        return reply


class PageBytes:
    """One bounded same-origin blob read; never follows a redirect."""

    def __init__(self, context, deadline, custody=verify):
        self.context = context
        self.deadline = deadline
        self.custody = custody

    def get(self, path, revision, width=None, limit=MAX_PAGE):
        if not isinstance(path, str) or not PAGE_PATH.fullmatch(path):
            raise CheckError('page-path-not-admitted')
        if isinstance(limit, bool) or not isinstance(limit, int) or not 1 <= limit <= MAX_PAGE:
            raise CheckError('page-limit-not-admitted')
        if not isinstance(revision, str) or not REVISION.fullmatch(revision):
            raise CheckError('revision-not-admitted')
        if width is not None:
            if isinstance(width, bool) or not isinstance(width, int) or not 32 <= width <= 2048:
                raise CheckError('page-width-not-admitted')
            path = path + '?width=' + str(width)
        remaining = self.deadline.remaining()
        if remaining < 1:
            raise CheckError('page-budget')
        self.custody(self.context)
        connection = http.client.HTTPConnection(self.context.host, self.context.port,
                                                timeout=min(20.0, max(1.0, remaining)))
        try:
            connection.request('GET', path, headers={
                'Authorization': 'Bearer ' + self.context.access_token,
                'X-Portico-Installation-Id': self.context.installation_id,
                'If-Match': '"' + revision + '"',
                'Accept': 'image/png,image/jpeg,image/gif,image/webp',
                'Connection': 'close'})
            response = connection.getresponse()
            status = response.status
            content = (response.getheader('Content-Type') or '').split(';', 1)[0].strip().lower()
            chunks, size = [], 0
            while not response.isclosed():
                remaining = self.deadline.remaining()
                if remaining <= 0:
                    raise CheckError('page-budget')
                sock = connection.sock
                if sock is None and response.fp is not None:
                    sock = getattr(getattr(response.fp, 'raw', None), '_sock', None)
                if sock is not None:
                    sock.settimeout(min(20.0, remaining))
                chunk = response.read1(min(65536, limit - size + 1))
                if not chunk:
                    break
                chunks.append(chunk)
                size += len(chunk)
                if size > limit:
                    raise CheckError('page-oversized')
            body = b''.join(chunks)
        except (TimeoutError, OSError, http.client.HTTPException):
            raise CheckError('page-transport') from None
        finally:
            try:
                connection.close()
            except Exception:
                pass
        self.custody(self.context)
        if len(body) > limit:
            raise CheckError('page-oversized')
        return status, content, body


class Check:
    def __init__(self, client, library, item=None, page=0, width=800, bytes_reader=None):
        self.client = client
        self.library = library
        self.item = item
        self.page = page
        self.width = width
        self.reader = bytes_reader
        self.session = None
        self.created = False
        self.revision = None
        self.document = {'check': 'cb7-reading-one-page', 'item': None, 'session': None,
                         'page': None, 'failure': None, 'cleanup': None,
                         'result': 'refused', 'reason': None}

    def _call(self, operation, method, path, **kwargs):
        reply = self.client.call(operation, method, path, **kwargs)
        if reply.failure is not None:
            raise CheckError('transport-' + reply.failure)
        return reply

    def execute(self):
        try:
            if self.page != 0:
                raise CheckError('page-index-not-admitted')
            item = self._item()
            declared = item.get('format')
            self.document['item'] = {'format': declared if declared in ('cb7', 'cbz', 'pdf', 'epub', 'cbr') else 'other',
                                     'inventoryAvailable': item.get('inventoryAvailable') is True}
            if declared != 'cb7':
                raise CheckError('item-not-cb7')
            self._open(item)
            self._page()
            self.document['result'] = 'verified'
        except CheckError as error:
            self.document['failure'] = {'reason': error.reason}
            self.document['reason'] = error.reason
            self.document['result'] = 'refused'
        except Exception:
            self.document['failure'] = {'reason': 'unexpected-shape'}
            self.document['reason'] = 'unexpected-shape'
            self.document['result'] = 'refused'
        finally:
            self.document['cleanup'] = self._close()
            if self.created and self.document['cleanup'].get('confirmed') is not True:
                # Only a session the server actually acknowledged can make cleanup
                # a failure; a pure refusal that never created one keeps its own
                # refused result and reason.
                self.document['result'] = 'failed'
                self.document['reason'] = 'cleanup-' + str(self.document['cleanup'].get('reason') or 'unconfirmed')
        return self.document

    def _item(self):
        reply = self._call('reading-items', 'GET', '/v1/reading/items',
                           query={'libraryId': self.library, 'limit': 100})
        if reply.status != 200 or not isinstance(reply.data, dict):
            raise CheckError('items-' + (reply.code or 'shape'))
        items = reply.data.get('items')
        if not isinstance(items, list):
            raise CheckError('items-shape')
        if not isinstance(self.item, str) or not self.item:
            raise CheckError('item-required')
        found = [candidate for candidate in items[:100]
                 if isinstance(candidate, dict) and candidate.get('id') == self.item]
        if not found:
            raise CheckError('cb7-not-admitted')
        if len(found) != 1 or found[0].get('libraryId') != self.library:
            raise CheckError('item-scope-mismatch')
        return found[0]

    def _open(self, item):
        source = item.get('sourceId')
        if not isinstance(source, str) or not source:
            raise CheckError('source-absent')
        reply = self._call('reading-open', 'POST', '/v1/reading/sessions',
                           body={'itemId': self.item, 'sourceId': source})
        if reply.status == 404:
            raise CheckError('cb7-not-admitted')
        if reply.status != 200 or not isinstance(reply.data, dict):
            raise CheckError('session-' + (reply.code or 'shape'))
        session = reply.data.get('sessionId')
        revision = reply.data.get('revision')
        if not isinstance(session, str) or not session:
            raise CheckError('session-identity')
        # Own the session the moment the server acknowledged it, before any
        # later judgement, so a refusal still closes real state.
        self.session = session
        self.created = True
        if not isinstance(revision, str) or not REVISION.fullmatch(revision):
            raise CheckError('revision-invalid')
        self.revision = revision
        declared = reply.data.get('format')
        self.document['session'] = {'format': declared if declared in ('cb7', 'cbz', 'pdf', 'epub')
                                    else 'other', 'opened': True, 'revisionClass': 'hex64'}
        if declared != 'cb7':
            raise CheckError('session-format-not-cb7')

    def _page(self):
        if not self.session or not self.revision:
            raise CheckError('session-identity')
        if self.reader is None:
            raise CheckError('page-reader-absent')
        if self.page != 0:
            raise CheckError('page-index-not-admitted')
        path = '/v1/reading/sessions/%s/pages/0' % self.session
        try:
            status, content, body = self.reader.get(path, self.revision, self.width)
        except CheckError:
            raise
        if status != 200:
            raise CheckError('page-status')
        if content not in MEDIA_TYPES:
            raise CheckError('page-content-type')
        if len(body) < MIN_PAGE:
            raise CheckError('page-short')
        kind = magic_class(body)
        if kind not in ('png', 'jpeg', 'gif', 'webp'):
            raise CheckError('page-not-image')
        self.document['page'] = {'bytes': len(body), 'magic': kind, 'pageIndex': 0}

    def _close(self):
        if not self.session:
            return {'confirmed': False, 'reason': 'no-session'}
        session = self.session
        self.session = None
        try:
            reply = self.client.call('reading-close', 'DELETE',
                                     '/v1/reading/sessions/' + session, expect=(200, 204),
                                     force=True)
        except Exception:
            return {'confirmed': False, 'reason': 'close-transport'}
        if reply.failure is not None:
            return {'confirmed': False, 'reason': 'close-' + reply.failure}
        if reply.status in (200, 204):
            return {'confirmed': True, 'status': reply.status}
        return {'confirmed': False, 'reason': reply.code or 'close-failed'}


def run(context_path, origin, run_id, library, item=None, page=0, budget_seconds=60, width=800):
    if not 15 <= budget_seconds <= 120:
        raise ContextError('budget-not-admitted')
    if page != 0:
        raise ContextError('page-index-not-admitted')
    if isinstance(width, bool) or not isinstance(width, int) or not 32 <= width <= 2048:
        raise ContextError('page-width-not-admitted')
    context = load(context_path)
    if context.origin != origin:
        raise ContextError('origin-mismatch')
    if context.run_id != run_id:
        raise ContextError('run-id-mismatch')
    verify(context)
    deadline = Deadline(budget_seconds)
    document = Check(FencedClient(Client(context, deadline), context), library, item, page, width,
                     bytes_reader=PageBytes(context, deadline)).execute()
    document['runId'] = context.run_id
    return document


def write_receipt(path, document):
    """Same-directory atomic replace with 0600; no partial receipt is left."""
    directory = path.parent
    descriptor, temporary = tempfile.mkstemp(prefix='.cb7-receipt-', dir=str(directory))
    try:
        os.fchmod(descriptor, 0o600)
        with os.fdopen(descriptor, 'w') as stream:
            json.dump(document, stream, indent=2)
            stream.write('\n')
        os.replace(temporary, path)
    except Exception:
        try:
            os.unlink(temporary)
        except OSError:
            pass
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--context', required=True, type=Path)
    parser.add_argument('--origin', required=True)
    parser.add_argument('--run-id', required=True)
    parser.add_argument('--library', required=True)
    parser.add_argument('--item', required=True)
    parser.add_argument('--width', type=int, default=800)
    parser.add_argument('--output', default='-')
    args = parser.parse_args()
    os.umask(0o077)
    try:
        admitted_origin(args.origin)
        document = run(args.context, args.origin, args.run_id, args.library, args.item, width=args.width)
    except ContextError as error:
        print(json.dumps({'error': str(error)}))
        return 2
    blob = json.dumps(document, indent=2) + '\n'
    if args.output == '-':
        sys.stdout.write(blob)
    else:
        try:
            write_receipt(Path(args.output), document)
        except OSError:
            print(json.dumps({'error': 'output-unavailable'}))
            return 3
    return 0 if document['result'] == 'verified' else 1


if __name__ == '__main__':
    raise SystemExit(main())
