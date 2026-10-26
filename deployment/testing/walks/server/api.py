"""A closed loopback JSON client for walk-server setup and verification.

Only 127.0.0.1 is contacted. Proxies and redirects cannot apply because the
client uses `http.client` directly and refuses 3xx. Bodies are bounded and an
error is reduced to a closed code or an HTTP status, never a raw body.
"""
from __future__ import annotations

import http.client
import json
import re
import socket
from urllib.parse import urlencode

_PATH = re.compile(r'/(?:[A-Za-z0-9._~/-]|%[0-9A-Fa-f]{2}){0,8192}\Z')
_MAX = 8 * 1024 * 1024

class ApiError(Exception):
    def __init__(self, code, status=None):
        self.code = code
        self.status = status

class Client:
    def __init__(self, port, timeout=45):
        self._port = port
        self._timeout = timeout

    def call(self, method, path, body=None, token=None, installation=None, origin=None,
             expect=(200,), limit=_MAX, query=None, timeout=None):
        if method not in ('GET', 'POST', 'PATCH', 'PUT', 'DELETE'):
            raise ApiError('api-method')
        if not isinstance(path, str) or not _PATH.fullmatch(path) or '//' in path or '..' in path.split('/'):
            raise ApiError('api-path')
        target = path + ('?' + urlencode(query, doseq=True) if query else '')
        headers = {'Accept': 'application/json', 'Connection': 'close'}
        if token:
            headers['Authorization'] = 'Bearer ' + token
        if installation:
            headers['X-Portico-Installation-Id'] = installation
        if origin:
            headers['Origin'] = origin
        payload = None
        if body is not None:
            payload = json.dumps(body, separators=(',', ':')).encode()
            headers['Content-Type'] = 'application/json'
        request_timeout = self._timeout if timeout is None else max(0.1, min(self._timeout, timeout))
        connection = http.client.HTTPConnection('127.0.0.1', self._port, timeout=request_timeout)
        try:
            connection.request(method, target, body=payload, headers=headers)
            response = connection.getresponse()
            status = response.status
            raw = response.read(limit + 1)
        except (TimeoutError, socket.timeout):
            raise ApiError('api-timeout') from None
        except (OSError, http.client.HTTPException):
            raise ApiError('api-transport') from None
        finally:
            try:
                connection.close()
            except Exception:
                pass
        if 300 <= status < 400:
            raise ApiError('api-redirect', status)
        if len(raw) > limit:
            raise ApiError('api-oversized', status)
        if status not in expect:
            raise ApiError('api-http', status)
        if not raw:
            return None
        try:
            return json.loads(raw)
        except ValueError:
            raise ApiError('api-shape', status) from None

    def get(self, path, **kwargs):
        return self.call('GET', path, **kwargs)

    def post(self, path, body=None, **kwargs):
        return self.call('POST', path, body=body, **kwargs)

    def patch(self, path, body=None, **kwargs):
        return self.call('PATCH', path, body=body, **kwargs)

    def delete(self, path, **kwargs):
        return self.call('DELETE', path, **kwargs)
