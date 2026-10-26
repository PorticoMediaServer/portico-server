"""A closed, loopback-only HTTP client for the checkpoint API smoke.

It talks to one numeric 127.0.0.1 origin with `http.client`, so environment
proxies, redirects and TLS egress cannot apply. Responses are read under a
fixed byte cap, and an error is reduced to a closed code before it leaves this
module. Raw bodies, headers, URLs and exception text are never returned.
"""
from __future__ import annotations

import http.client
import json
import re
import socket
import time
from dataclasses import dataclass
from urllib.parse import urlencode

# Endpoint cause codes this smoke can observe. Anything else is reduced to
# "unclassified" rather than published.
ERROR_CODES = frozenset({
    'analysis_unavailable', 'artwork_gone', 'artwork_pending', 'body_too_large', 'challenge_invalid',
    'channel_revision_conflict', 'connection_too_slow', 'conversion_capacity', 'feature_restricted',
    'forbidden', 'home_layout_conflict', 'idempotency_key_required', 'idempotency_key_reused',
    'internal', 'invalid_channel_input', 'invalid_credentials', 'invalid_cursor', 'invalid_query',
    'invalid_request', 'library_configuration_conflict', 'metadata_conflict', 'metadata_unavailable',
    'method_not_allowed', 'no_tuner_available', 'not_found', 'not_permitted', 'operation_expired',
    'origin_denied', 'personal_needs_resolution', 'personal_needs_review', 'personal_state_conflict',
    'queue_building', 'queue_too_large', 'rate_limited', 'revision_mismatch', 'revision_required',
    'request_in_progress', 'search_busy', 'session_ended', 'session_expired', 'session_mode_denied',
    'source_changed', 'source_unavailable', 'stale_continuation', 'stream_limit_reached',
    'transcode_not_allowed', 'unauthorized', 'unsupported_media', 'unsupported_selector',
    'web_unavailable',
})
CLOSED_CODES = ERROR_CODES | frozenset({'unclassified'})
TRANSPORT_FAILURES = frozenset({'timeout', 'transport', 'oversized', 'redirect', 'invalid-json'})
METHODS = frozenset({'GET', 'HEAD', 'POST', 'PUT', 'DELETE'})
HEADERS = frozenset({'Idempotency-Key', 'If-Match'})
MAX_BODY = 4 * 1024 * 1024
_PATH = re.compile(r'/[A-Za-z0-9._~/-]{0,2048}\Z')

@dataclass(frozen=True)
class Reply:
    operation: str
    method: str
    status: int | None = None
    code: str | None = None
    failure: str | None = None
    data: object | None = None
    content_length: int | None = None
    body_length: int = 0

class Deadline:
    def __init__(self, seconds):
        if not isinstance(seconds, (int, float)) or not 1 <= seconds <= 1800:
            raise ValueError('deadline out of range')
        self._end = time.monotonic() + float(seconds)

    def remaining(self):
        return max(0.0, self._end - time.monotonic())

    def expired(self):
        return self.remaining() <= 0

def sleep_within(deadline, seconds):
    if deadline.remaining() <= seconds + 0.5:
        return False
    time.sleep(seconds)
    return True

def _length(value):
    if isinstance(value, str) and value.isdigit():
        number = int(value)
        if 0 <= number <= 10 ** 12:
            return number
    return None

def _closed_code(data):
    value = None
    if isinstance(data, dict):
        error = data.get('error')
        if isinstance(error, dict):
            value = error.get('code')
        elif isinstance(error, str):
            value = error
        if not isinstance(value, str):
            value = data.get('code')
    return value if isinstance(value, str) and value in ERROR_CODES else 'unclassified'

class Client:
    def __init__(self, context, deadline):
        self._host = context.host
        self._port = context.port
        self._token = context.access_token
        self._installation = context.installation_id
        self._deadline = deadline

    def call(self, operation, method, path, *, query=None, body=None, headers=None, expect=(200,),
             auth=True, parse_json=True, limit=MAX_BODY, force=False):
        """Make one bounded request. Every failure is a closed transport class."""
        if method not in METHODS:
            raise ValueError('method not admitted')
        if not isinstance(path, str) or not _PATH.fullmatch(path) or '//' in path or '..' in path.split('/'):
            raise ValueError('path not admitted')
        remaining = self._deadline.remaining()
        if remaining <= 0 and not force:
            return Reply(operation, method, failure='timeout')
        target = path + ('?' + urlencode(query, doseq=True) if query else '')
        request_headers = {'Accept': 'application/json', 'Connection': 'close'}
        if auth:
            request_headers['Authorization'] = 'Bearer ' + self._token
        request_headers['X-Portico-Installation-Id'] = self._installation
        payload = None
        if body is not None:
            payload = json.dumps(body, separators=(',', ':')).encode()
            request_headers['Content-Type'] = 'application/json'
        for name, value in (headers or {}).items():
            if name not in HEADERS or not isinstance(value, str) or any(c in value for c in '\r\n'):
                raise ValueError('header not admitted')
            request_headers[name] = value
        timeout = 5.0 if force else max(1.0, min(20.0, remaining))
        connection = http.client.HTTPConnection(self._host, self._port, timeout=timeout)
        try:
            connection.request(method, target, body=payload, headers=request_headers)
            response = connection.getresponse()
            status = response.status
            content_length = _length(response.getheader('Content-Length'))
            raw = response.read(limit + 1)
        except (TimeoutError, socket.timeout):
            return Reply(operation, method, failure='timeout')
        except (OSError, http.client.HTTPException):
            return Reply(operation, method, failure='transport')
        finally:
            try:
                connection.close()
            except Exception:
                pass
        if 300 <= status < 400:
            return Reply(operation, method, status=status, failure='redirect', body_length=len(raw))
        if len(raw) > limit:
            return Reply(operation, method, status=status, failure='oversized', body_length=len(raw))
        data = None
        if parse_json and raw:
            try:
                data = json.loads(raw)
            except ValueError:
                if status in expect:
                    return Reply(operation, method, status=status, failure='invalid-json', body_length=len(raw))
        code = _closed_code(data) if status >= 400 or status not in expect else None
        return Reply(operation, method, status=status, code=code, data=data,
                     content_length=content_length, body_length=len(raw))
