"""Bounded transcode-environment check for one admitted loopback runtime.

One ordinary authenticated start on a named fixture item with a low quality
ceiling, then a bounded read of the bytes the server actually delivered and a
probe of those bytes. A conversion is only reported `passed` when the session's
own delivery mode and route say converted, a real media sample was fetched from
the session's own delivery URL on the same admitted origin, the probe identified
a real video stream within the requested height ceiling, and the session this
check created was stopped and confirmed ended.

A 202 that already carries a real session view is owned immediately and cleaned
up in `finally`; the same body and idempotency key are replayed, and the id and
generation must stay stable from 202 to 201. Plan or process facts alone never
pass. Closed facts only: no URL, token, title, path, header, body text or
exception message leaves this package.
"""
from __future__ import annotations

import secrets

from api_smoke.client import sleep_within

from .media import MAX_SAMPLE, MediaError

MODES = frozenset({'direct', 'stream'})
ACTIONS = frozenset({'direct', 'copy', 'transcode', 'burn'})
CONTAINERS = frozenset({'mp4', 'mkv', 'fmp4_hls', 'mpegts_hls', 'flac'})
ROUTES = frozenset({'original', 'repackaged', 'converted'})
POLICY_MODES = frozenset({'prefer', 'allow', 'never'})
REASON_KINDS = frozenset({'compatible', 'container', 'format', 'quality', 'subtitles'})
REAL_VIDEO = frozenset({'h264', 'hevc', 'av1', 'vp9', 'mpeg2video'})
SUCCESS = frozenset({200, 201, 202, 204, 206, 404})
START_ATTEMPTS = 12
RETRY_MIN_MS = 1
RETRY_MAX_MS = 10000
HEIGHT_CEILING = 480


class CheckFailure(Exception):
    """A step failed with a closed reason; the receipt still gets written."""

    def __init__(self, reason):
        super().__init__(reason)
        self.reason = reason


def bucket(value, edges=(1, 2, 4, 8, 16, 32, 64, 128, 256)):
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        return 'unknown'
    for edge in edges:
        if value < edge:
            return 'under-%d' % edge
    return 'at-least-%d' % edges[-1]


def mode_class(value):
    return value if isinstance(value, str) and value in MODES else 'other'


def action_class(value):
    return value if isinstance(value, str) and value in ACTIONS else 'other'


def route_class(value):
    return value if isinstance(value, str) and value in ROUTES else 'unknown'


def policy_mode(value):
    return value if isinstance(value, str) and value in POLICY_MODES else 'unknown'


def text_class(value, admitted):
    return value if isinstance(value, str) and value in admitted else 'unknown'


def status_class(value):
    return value if value in SUCCESS else 'other'


def retry_hint(document):
    """The official 202 retry hint is one required strict integer."""
    if not isinstance(document, dict):
        raise CheckFailure('retry-hint-invalid')
    value = document.get('retryAfterMs')
    if isinstance(value, bool) or not isinstance(value, int):
        raise CheckFailure('retry-hint-invalid')
    if not RETRY_MIN_MS <= value <= RETRY_MAX_MS:
        raise CheckFailure('retry-hint-invalid')
    return value


def session_ref(document):
    """A real session view carries one id and one positive generation."""
    if not isinstance(document, dict):
        raise CheckFailure('session-view-invalid')
    session = document.get('id')
    presentation = document.get('presentation') if isinstance(document.get('presentation'), dict) else {}
    generation = presentation.get('generation')
    if isinstance(generation, bool) or not isinstance(generation, int) or generation < 1:
        raise CheckFailure('session-generation-invalid')
    if not isinstance(session, str) or not session or len(session) > 256:
        raise CheckFailure('session-identity')
    return session, generation


def _integer(value):
    return value if isinstance(value, int) and not isinstance(value, bool) and 0 <= value <= 65536 else None


class Check:
    def __init__(self, client, deadline, owner, item, media=None, max_sample=MAX_SAMPLE,
                 sleep=sleep_within):
        if not isinstance(item, str) or not item:
            raise CheckFailure('item-not-admitted')
        self.client = client
        self.deadline = deadline
        self.owner = owner
        self.item = item
        self.media = media
        self.max_sample = max_sample
        self.sleep = sleep
        self.session = ''
        self.owned = None

    # -- steps ------------------------------------------------------------
    def execute(self):
        document = {'check': 'transcode-environment', 'steps': [], 'capacity': None,
                    'policy': None, 'admission': None, 'session': None, 'delivered': None,
                    'cleanup': None, 'result': 'failed',
                    'runtimeProfile': 'ordinary authenticated playback against the existing walk server; '
                                      'one owned session and a bounded sample of its delivered bytes; '
                                      'no quality, fidelity or walk acceptance',
                    'limitations': [
                        'a direct-play session is reported unavailable, never passed',
                        'the sample proves delivered container and stream facts, not playback quality',
                        'no PlaybackInfo or probe fact is an app-render or playback-progress result',
                    ]}
        try:
            try:
                self._capacity(document)
                self._policy(document)
                if self._start(document):
                    self._delivered(document)
            except CheckFailure as failure:
                document['steps'].append({'step': 'check', 'status': 'fail', 'reason': failure.reason})
            except Exception:
                # An unexpected body or transport shape is a closed failure, never
                # an escape: the receipt is still written and cleanup still runs.
                document['steps'].append({'step': 'check', 'status': 'fail',
                                          'reason': 'unexpected-shape'})
        finally:
            document['cleanup'] = self._stop()
        document['result'] = self._verdict(document)
        return document

    def _step(self, document, name, status, reason=None):
        entry = {'step': name, 'status': status}
        if reason is not None:
            entry['reason'] = reason
        document['steps'].append(entry)

    def _call(self, operation, method, path, **kwargs):
        reply = self.client.call(operation, method, path, **kwargs)
        if reply.failure is not None:
            raise CheckFailure('transport-' + reply.failure)
        return reply

    def _capacity(self, document):
        if not self.owner:
            self._step(document, 'capacity', 'unavailable', 'owner-required')
            return
        reply = self._call('transcode-capacity', 'GET', '/v1/admin/transcode/capacity')
        if reply.status != 200 or not isinstance(reply.data, dict):
            raise CheckFailure('capacity-' + (reply.code or 'shape'))
        # Owner telemetry is a projected console {data, scope} envelope.
        data = reply.data.get('data')
        if not isinstance(data, dict) or not isinstance(data.get('enabled'), bool):
            raise CheckFailure('capacity-shape')
        toolchain = data.get('toolchain') if isinstance(data.get('toolchain'), dict) else {}
        encoders = toolchain.get('encoders') if isinstance(toolchain.get('encoders'), dict) else {}
        filters = toolchain.get('filters') if isinstance(toolchain.get('filters'), dict) else {}
        muxers = toolchain.get('muxers') if isinstance(toolchain.get('muxers'), dict) else {}
        dependencies = data.get('dependencies') if isinstance(data.get('dependencies'), dict) else {}
        temporary = data.get('temporaryDirectory') if isinstance(data.get('temporaryDirectory'), dict) else {}
        hardware = data.get('hardware') if isinstance(data.get('hardware'), dict) else {}
        document['capacity'] = {
            'read': True,
            'enabled': data.get('enabled') is True,
            'toolchainReported': bool(toolchain),
            'hasLibx264': encoders.get('libx264') is True,
            'hasLibx265': encoders.get('libx265') is True,
            'hasScaleFilter': filters.get('scale') is True,
            'hasHlsMuxer': muxers.get('hls') is True,
            'dependencies': {name: (dependencies.get(name) or {}).get('ok') is True
                             for name in ('ffmpeg', 'ffprobe')},
            'temporaryDirectoryReady': temporary.get('ready') is True,
            'temporaryDirectoryFallback': temporary.get('fallback') is True,
            'presets': bucket(len(data.get('presets') or [])),
            'warnings': bucket(len(data.get('warnings') or [])),
            'hardwareConfigured': hardware.get('configured') == 'hardware',
            'hardwareBackends': bucket(len(hardware.get('probes') or [])),
            'directStreamRemux': data.get('directStreamRemux') is True,
            'planningPolicy': text_class(data.get('planningPolicy'),
                                         {'auto', 'hardware', 'software', 'direct'}),
        }
        self._step(document, 'capacity', 'ok')

    def _policy(self, document):
        reply = self._call('playback-delivery-policy', 'GET', '/v1/playback/delivery-policy')
        if reply.status != 200 or not isinstance(reply.data, dict):
            raise CheckFailure('policy-' + (reply.code or 'shape'))
        decisions = reply.data.get('policy')
        decisions = decisions if isinstance(decisions, dict) else {}
        mode = policy_mode(decisions.get('transcode'))
        document['policy'] = {
            'read': True,
            'hasDirectPlay': 'directPlay' in decisions,
            'hasDirectStream': 'directStream' in decisions,
            'hasTranscode': 'transcode' in decisions,
            'hasClamps': 'clamps' in decisions,
            'transcodeMode': mode,
            'transcodeAllowed': mode in ('prefer', 'allow'),
        }
        self._step(document, 'policy', 'ok')

    def _own(self, document, ref):
        """Capture a real session view for cleanup as soon as it exists."""
        if self.owned is None:
            self.owned = ref
            self.session = ref[0]
            document['admission'] = {'status': 'preparing', 'reason': 'owned-202',
                                     'attempts': bucket(1)}
            return
        if ref != self.owned:
            raise CheckFailure('session-identity-changed')

    def _start(self, document):
        # The same idempotency key and body are replayed while the server reports
        # 202, within the whole remaining budget. A 202 that already carries a
        # real session view is owned immediately: dropping it would leak a real
        # resource that the server already created.
        key = secrets.token_urlsafe(24)
        body = {'itemId': self.item, 'state': 'playing',
                'quality': {'mode': 'limit', 'maxVideoBitrateKbps': 400, 'maxHeight': 480}}
        reply = self._call('playback-start', 'POST', '/v1/playback/sessions',
                           body=body, headers={'Idempotency-Key': key}, expect=(201, 202))
        attempts = 0
        while reply.status == 202:
            data = reply.data if isinstance(reply.data, dict) else {}
            if 'id' in data:
                self._own(document, session_ref(data))
            hint = retry_hint(data)
            if attempts >= START_ATTEMPTS:
                break
            if self.deadline is not None and self.deadline.remaining() <= hint / 1000.0:
                break
            if not self.sleep(self.deadline, hint / 1000.0):
                break
            attempts += 1
            reply = self._call('playback-start', 'POST', '/v1/playback/sessions',
                               body=body, headers={'Idempotency-Key': key}, expect=(201, 202))
        if reply.status == 202:
            document['admission'] = (document.get('admission')
                                     or {'status': 'preparing', 'attempts': bucket(attempts)})
            document['admission']['reason'] = 'retry-exhausted' if self.owned else 'retry-only'
            self._step(document, 'start', 'unavailable', document['admission']['reason'])
            return False
        if reply.status != 201:
            raise CheckFailure('start-' + (reply.code or 'refused'))
        ref = session_ref(reply.data if isinstance(reply.data, dict) else {})
        if self.owned is not None and ref != self.owned:
            raise CheckFailure('session-identity-changed')
        self.owned = ref
        self.session = ref[0]
        document['admission'] = {'status': status_class(reply.status), 'attempts': bucket(attempts)}
        # The final observed state is the one recorded, not the admission snapshot.
        view = self._view()
        state = view.get('state')
        if state not in ('preparing', 'playing', 'paused'):
            raise CheckFailure('session-state')
        presentation = view.get('presentation') if isinstance(view.get('presentation'), dict) else {}
        decision = presentation.get('decision') if isinstance(presentation.get('decision'), dict) else {}
        video = decision.get('video') if isinstance(decision.get('video'), dict) else {}
        audio = decision.get('audio') if isinstance(decision.get('audio'), dict) else {}
        document['session'] = {
            'started': True,
            'state': state,
            'mode': mode_class(presentation.get('mode')),
            'container': text_class(presentation.get('container'), CONTAINERS),
            'videoHeight': _integer(presentation.get('videoHeight')),
            'bitrateKbps': bucket(presentation.get('bitrateKbps') or 0),
            'decision': {
                'videoAction': action_class(video.get('action')),
                'audioAction': action_class(audio.get('action')),
                'reasons': bucket(len(video.get('reasons') or [])),
            },
            'mediaPath': 'published' if isinstance(presentation.get('url'), str)
                         and presentation.get('url') else 'absent',
        }
        self._step(document, 'start', 'ok')
        return True

    def _view(self):
        reply = self._call('playback-session', 'GET', '/v1/playback/sessions/' + self.session)
        if reply.status != 200 or not isinstance(reply.data, dict):
            raise CheckFailure('session-' + (reply.code or 'shape'))
        if session_ref(reply.data) != self.owned:
            raise CheckFailure('session-identity')
        return reply.data

    def _delivered(self, document):
        view = self._view()
        presentation = view.get('presentation') if isinstance(view.get('presentation'), dict) else {}
        url = presentation.get('url')
        if not isinstance(url, str) or not url:
            raise CheckFailure('media-url-absent')
        if self.media is None:
            raise CheckFailure('media-client-absent')
        try:
            sample = self.media.fetch(url, self.max_sample)
        except MediaError as error:
            raise CheckFailure(error.reason) from None
        try:
            probe = self.media.probe(sample['sample'])
        except MediaError as error:
            raise CheckFailure(error.reason) from None
        if not isinstance(probe, dict):
            raise CheckFailure('probe-shape')
        info = view.get('playbackInfo')
        info = info if isinstance(info, dict) else {}
        reason = info.get('reason') if isinstance(info.get('reason'), dict) else {}
        delivered = info.get('delivered') if isinstance(info.get('delivered'), dict) else {}
        video = delivered.get('video') if isinstance(delivered.get('video'), dict) else {}
        document['delivered'] = {
            'fetched': True,
            'bytes': bucket(sample['bytes']),
            'status': sample['status'],
            'content': sample['content'],
            'via': sample['via'],
            'playlistHops': sample.get('playlistHops', 0),
            'container': sample['container'],
            'format': probe.get('format'),
            'probeAvailable': probe.get('available') is True,
            'probeReason': probe.get('reason'),
            'probeVideo': probe.get('video'),
            'probeAudio': probe.get('audio'),
            'probeSubtitleTracks': probe.get('subtitleTracks'),
            'route': route_class(info.get('route')),
            'reasonKind': text_class(reason.get('kind'), REASON_KINDS),
            'pageVideoCodec': _codec_class(video.get('codec')),
            'pageHeight': _integer(video.get('height')),
        }
        self._step(document, 'delivered', 'ok')

    def _stop(self):
        if not self.owned:
            return {'stopped': False, 'reason': 'no-session'}
        session = self.owned[0]
        self.session = ''
        try:
            reply = self.client.call('playback-stop', 'DELETE', '/v1/playback/sessions/' + session,
                                     expect=(204,), force=True)
            if reply.failure is not None or reply.status != 204:
                return {'stopped': False, 'reason': reply.failure or reply.code or 'stop-failed'}
            readback = self.client.call('playback-stop-read', 'GET',
                                        '/v1/playback/sessions/' + session,
                                        expect=(200, 404), force=True)
        except Exception:
            return {'stopped': False, 'reason': 'stop-transport'}
        if readback.failure is not None:
            return {'stopped': False, 'reason': 'stop-readback-' + readback.failure}
        if readback.status == 404:
            return {'stopped': True, 'ended': True, 'inFinally': True}
        data = readback.data if isinstance(readback.data, dict) else {}
        ended = data.get('state') in ('stopped', 'ended') or isinstance(data.get('end'), dict)
        return {'stopped': True, 'ended': bool(ended), 'inFinally': True}

    def _verdict(self, document):
        cleanup = document.get('cleanup') or {}
        admission = document.get('admission') or {}
        if any(step['status'] == 'fail' for step in document['steps']):
            return 'failed'
        # A session the server already created is a real resource: losing it in
        # cleanup fails the check even when it was only ever a 202.
        if self.owned is not None:
            if cleanup.get('stopped') is not True or cleanup.get('ended') is not True:
                return 'failed'
        if admission.get('reason') in ('retry-only', 'retry-exhausted'):
            return 'unavailable'
        session = document.get('session') or {}
        delivered = document.get('delivered') or {}
        probe_video = delivered.get('probeVideo')
        if session.get('mode') == 'direct':
            return 'unavailable'
        if (session.get('mode') == 'stream'
                and delivered.get('route') == 'converted'
                and delivered.get('fetched') is True
                and delivered.get('probeAvailable') is True
                and isinstance(probe_video, dict)
                and probe_video.get('codec') in REAL_VIDEO
                and isinstance(probe_video.get('height'), int)
                and 1 <= probe_video['height'] <= HEIGHT_CEILING):
            return 'passed'
        return 'unavailable'


def _codec_class(value):
    return value if isinstance(value, str) and value in REAL_VIDEO else 'other'
