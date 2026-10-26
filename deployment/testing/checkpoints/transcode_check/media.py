"""Bounded read of the delivered media for one admitted playback session.

The session view publishes `presentation.url` as a same-origin capability path
(`/v1/media/<grant>`). This module fetches a small bounded sample of that URL
through the admitted loopback origin only, follows at most three playlist hops to a
real segment, and probes the sample bytes with an explicitly named `ffprobe`
(never a PATH lookup).

Custody is rechecked against the admitted runtime before **and** after every
fetch, and every timeout is capped by the same remaining budget the check uses.
The probe runs in private scratch with TMPDIR under the caller's scratch root and
that scratch is removed in a finally path; there is no `/tmp` profile, cache or
sample of our own. Nothing here decides acceptance, and it never returns a URL,
header, body text or probe stderr.
"""
from __future__ import annotations

import http.client
import json
import os
import re
import selectors
import shutil
import signal
import stat
import subprocess
import tempfile
import time
from pathlib import Path

MAX_SAMPLE = 4 * 1024 * 1024
MIN_SAMPLE = 1024
_REQUEST_TIMEOUT = 20
_PROBE_TIMEOUT = 20
MAX_PROBE_OUTPUT = 65536
_PATH = re.compile(r'/v1/media/[A-Za-z0-9._~/?=&%+-]{1,2048}\Z')
_CONTENT = frozenset({'video/mp4', 'video/mp2t', 'video/x-matroska', 'application/octet-stream'})
_PLAYLIST = frozenset({'application/vnd.apple.mpegurl', 'application/x-mpegurl', 'audio/mpegurl'})


class MediaError(Exception):
    def __init__(self, reason):
        super().__init__(reason)
        self.reason = reason


def admit_media_path(value):
    """Accept only one absolute same-origin media capability path."""
    if not isinstance(value, str) or value != value.strip() or '..' in value:
        raise MediaError('media-path-not-admitted')
    if not _PATH.fullmatch(value):
        raise MediaError('media-path-not-admitted')
    return value


def admit_ffprobe(value):
    """Accept one explicit absolute executable path; never search PATH."""
    if not isinstance(value, str) or not value.startswith('/') or len(value) > 4096:
        raise MediaError('probe-path-not-admitted')
    if not os.path.isfile(value) or not os.access(value, os.X_OK):
        raise MediaError('probe-path-not-admitted')
    return value


def content_class(value):
    if not isinstance(value, str):
        return 'unknown'
    base = value.split(';', 1)[0].strip().lower()
    if base in _CONTENT:
        return base
    return 'playlist' if base in _PLAYLIST else 'other'


def classify(sample):
    """Container class from the first bytes actually delivered."""
    if not isinstance(sample, (bytes, bytearray)) or len(sample) < 8:
        return 'short'
    head = bytes(sample[:16])
    if head.startswith(b'#EXTM3U'):
        return 'playlist'
    if head.startswith(b'\x1a\x45\xdf\xa3'):
        return 'matroska'
    if head[4:8] == b'ftyp':
        return 'fmp4'
    if head[0] == 0x47 and (len(sample) < 189 or sample[188] == 0x47):
        return 'mpegts'
    return 'unknown'


def first_segment(playlist, base_path):
    """Resolve the first media segment in one playlist, staying in /v1/media/."""
    if not isinstance(playlist, str):
        return None
    directory = base_path.rsplit('/', 1)[0] + '/'
    for line in playlist.splitlines()[:4000]:
        text = line.strip()
        if not text or text.startswith('#'):
            continue
        if text.startswith('http://') or text.startswith('https://') or text.startswith('//'):
            return None
        candidate = text if text.startswith('/') else directory + text
        return candidate if _PATH.fullmatch(candidate) and '..' not in candidate else None
    return None


class LoopbackMedia:
    """Same-origin bounded fetch, custody-rechecked, plus one qualified probe."""

    def __init__(self, context, deadline, ffprobe=None, scratch_root=None, verify=None):
        self.context = context
        self.deadline = deadline
        self.ffprobe = ffprobe
        self.scratch_root = scratch_root
        self.verify = verify
        self.last_used = 0

    def _timeout(self):
        remaining = self.deadline.remaining() if self.deadline is not None else 0
        return remaining

    def _custody(self):
        if self.verify is None:
            raise MediaError('custody-unverified')
        try:
            self.verify(self.context)
        except Exception:
            raise MediaError('custody-unverified') from None

    def fetch(self, path, limit=MAX_SAMPLE):
        admitted = admit_media_path(path)
        self._custody()
        status, sample, content = self._get(admitted, limit)
        hops = 0
        while classify(sample) == 'playlist':
            if hops >= 3 or len(sample) > 65536:
                raise MediaError('playlist-limit')
            segment = first_segment(sample.decode('utf-8', 'replace'), admitted)
            if segment is None:
                raise MediaError('playlist-segment-absent')
            status, sample, content = self._get(segment, limit)
            admitted = segment
            hops += 1
        via = 'segment' if hops else 'direct'
        self._custody()
        if status not in (200, 206):
            raise MediaError('media-status')
        if len(sample) < MIN_SAMPLE:
            raise MediaError('media-short')
        return {'sample': sample, 'bytes': len(sample), 'status': status,
                'content': content_class(content), 'via': via,
                'container': classify(sample), 'playlistHops': hops}

    def _get(self, path, limit):
        self._custody()
        remaining = self._timeout()
        if remaining < 1:
            raise MediaError('media-budget')
        timeout = min(_REQUEST_TIMEOUT, max(1.0, remaining))
        connection = http.client.HTTPConnection(self.context.host, self.context.port, timeout=timeout)
        try:
            connection.request('GET', path, headers={
                'Authorization': 'Bearer ' + self.context.access_token,
                'X-Portico-Installation-Id': self.context.installation_id,
                'Range': 'bytes=0-%d' % (limit - 1),
                'Accept': '*/*',
                'Connection': 'close'})
            response = connection.getresponse()
            status = response.status
            content = response.getheader('Content-Type') or ''
            chunks = []
            size = 0
            while size <= limit:
                if response.isclosed():
                    break
                remaining = self._timeout()
                if remaining <= 0:
                    raise MediaError('media-budget')
                sock = connection.sock or getattr(getattr(response.fp, 'raw', None), '_sock', None)
                if sock is None:
                    raise MediaError('media-transport')
                sock.settimeout(min(_REQUEST_TIMEOUT, remaining))
                chunk = response.read1(min(65536, limit + 1 - size))
                if not chunk:
                    break
                chunks.append(chunk)
                size += len(chunk)
            body = b''.join(chunks)
        except (TimeoutError, OSError, http.client.HTTPException):
            raise MediaError('media-transport') from None
        finally:
            try:
                connection.close()
            except Exception:
                pass
        if len(body) > limit:
            raise MediaError('media-oversized')
        self._custody()
        return status, body, content

    def probe(self, sample):
        """Probe the delivered sample with the admitted ffprobe path only."""
        remaining = self._timeout()
        if remaining < 1:
            return {'available': False, 'reason': 'probe-timeout'}
        try:
            executable = admit_ffprobe(self.ffprobe)
        except MediaError:
            return {'available': False, 'reason': 'probe-unavailable'}
        if not isinstance(self.scratch_root, str) or not self.scratch_root.startswith('/'):
            return {'available': False, 'reason': 'probe-unavailable'}
        try:
            root = Path(self.scratch_root)
            info = root.lstat()
            if (not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid()
                    or info.st_mode & 0o077 or root.resolve() != root
                    or root == Path('/tmp') or Path('/tmp') in root.parents):
                raise OSError('scratch-not-admitted')
            directory = tempfile.mkdtemp(prefix='probe-', dir=root)
        except OSError:
            return {'available': False, 'reason': 'probe-unavailable'}
        process = None
        try:
            sample_path = Path(directory) / 'sample.bin'
            descriptor = os.open(sample_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, 0o600)
            with os.fdopen(descriptor, 'wb') as stream:
                stream.write(sample)
            environment = dict(os.environ, TMPDIR=directory, LC_ALL='C')
            process = subprocess.Popen(
                [executable, '-v', 'error', '-protocol_whitelist', 'file,pipe', '-show_entries',
                 'format=format_name:stream=codec_type,codec_name,width,height,channels,sample_rate',
                 '-of', 'json', str(sample_path)],
                stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, stdin=subprocess.DEVNULL,
                env=environment, start_new_session=True, cwd=directory)
            end = time.monotonic() + min(_PROBE_TIMEOUT, self._timeout())
            raw = bytearray()
            with selectors.DefaultSelector() as selector:
                selector.register(process.stdout, selectors.EVENT_READ)
                while True:
                    wait = min(end - time.monotonic(), self._timeout())
                    if wait <= 0:
                        return {'available': False, 'reason': 'probe-timeout'}
                    if not selector.select(wait):
                        return {'available': False, 'reason': 'probe-timeout'}
                    chunk = os.read(process.stdout.fileno(), min(8192, MAX_PROBE_OUTPUT + 1 - len(raw)))
                    if not chunk:
                        break
                    raw.extend(chunk)
                    if len(raw) > MAX_PROBE_OUTPUT:
                        return {'available': False, 'reason': 'probe-failed'}
            process.wait(timeout=max(.001, min(end - time.monotonic(), self._timeout())))
            if process.returncode != 0:
                return {'available': False, 'reason': 'probe-failed'}
            return _probe_facts(bytes(raw))
        except subprocess.TimeoutExpired:
            return {'available': False, 'reason': 'probe-timeout'}
        except OSError:
            return {'available': False, 'reason': 'probe-unavailable'}
        finally:
            if process is not None:
                if process.poll() is None:
                    try:
                        os.killpg(process.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                process.wait()
                process.stdout.close()
            shutil.rmtree(directory, ignore_errors=True)


def _probe_facts(raw):
    try:
        document = json.loads(raw)
    except ValueError:
        return {'available': False, 'reason': 'probe-failed'}
    streams = document.get('streams') if isinstance(document.get('streams'), list) else []
    facts = {'available': False, 'format': 'unknown', 'video': None, 'audio': None,
             'subtitleTracks': 0, 'reason': 'probe-empty'}
    form = document.get('format') if isinstance(document.get('format'), dict) else {}
    name = form.get('format_name')
    if isinstance(name, str):
        parts = set(name.split(','))
        facts['format'] = ('mp4' if parts & {'mov', 'mp4', 'm4a'} else
                           'mpegts' if 'mpegts' in parts else
                           'matroska' if 'matroska' in parts or 'webm' in parts else 'other')
    for stream in streams[:32]:
        if not isinstance(stream, dict):
            continue
        kind = stream.get('codec_type')
        if kind == 'video' and facts['video'] is None:
            facts['video'] = {'codec': _codec(stream.get('codec_name')),
                              'width': _integer(stream.get('width')),
                              'height': _integer(stream.get('height'))}
            facts['available'] = True
        elif kind == 'audio' and facts['audio'] is None:
            facts['audio'] = {'codec': _codec(stream.get('codec_name')),
                              'channels': _integer(stream.get('channels')),
                              'sampleRate': _integer(stream.get('sample_rate'))}
        elif kind == 'subtitle':
            facts['subtitleTracks'] += 1
    if facts['available']:
        facts['reason'] = 'ok'
    return facts


_CODECS = frozenset({'h264', 'hevc', 'av1', 'vp9', 'mpeg2video', 'aac', 'ac3', 'eac3', 'mp3',
                     'opus', 'flac', 'subrip', 'ass', 'webvtt', 'mov_text'})


def _codec(value):
    return value if isinstance(value, str) and value in _CODECS else 'other'


def _integer(value):
    if isinstance(value, str) and value.isdigit():
        value = int(value)
    return value if isinstance(value, int) and not isinstance(value, bool) and 0 <= value <= 65536 else None
