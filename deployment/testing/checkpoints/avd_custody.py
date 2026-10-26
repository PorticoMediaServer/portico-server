#!/usr/bin/env python3
"""Nonblocking per-AVD custody: lock, prepare, identity, admission, handback.

One emulator, one owner. A helper-owned `flock /tmp/portico-avd-<exact-name>.lock`
is taken **before any boot or open** and held for the whole window. Under that lock
the caller may prepare the device (its `prepare` callback may device_open or boot it
and must return the exact serial and AVD), then the serial is proven to be that AVD,
then the caller's `acquire` callback opens the session for real. Any session already
claiming that serial, or that exact AVD on another serial, refuses the lease before
`acquire` is ever called.

    from avd_custody import lease, acquire_mapping, read_sessions
    try:
        with lease('PorticoTest_Phone_API36', 'emulator-5554',
                   prepare=lambda: prepare_mapping(device_open(...)),   # optional
                   acquire=lambda: acquire_mapping(open_session()),
                   handback=close_session,                              # returns a result
                   sessions=read_sessions(argv=[agent_device, 'session', 'list', ...]),
                   receipt=out / 'custody-phone.json') as held:
            install(exact_apk)                     # only after admission
            run_flows(held['sessionId'])
    except DeviceInUse:
        record('unavailable', 'DEVICE_IN_USE')     # release and skip, never force
    except CustodyRefused as refusal:
        record('unavailable', refusal.code)        # nothing installed, nothing adopted

A known target may already be booted when the lease starts; that is allowed, and
`prepare` is skipped. It is never treated as evidence of identity or admission. The
receipt states what the helper did: the lock was released, the returned session id
was persisted the moment it existed, and the handback result is the callback's own
closed result or explicitly unspecified. It never claims a supported session close,
a physical handback or a teardown.
"""
from __future__ import annotations

import contextlib
import datetime
import fcntl
import json
import os
import re
import subprocess
import time
from pathlib import Path

LOCK_TEMPLATE = '/tmp/portico-avd-{name}.lock'
NAME = re.compile(r'[A-Za-z0-9][A-Za-z0-9._-]{0,63}\Z')
SERIAL = re.compile(r'emulator-[0-9]{4,5}\Z')
SESSION_ID = re.compile(r'[A-Za-z0-9._-]{8,128}\Z')
ANDROID = 'android'
IDENTITY_KEYS = ('sessionId', 'serial', 'avd')
CLOSED_HANDBACK = ('closed', 'already-absent')
CODE_PATTERN = re.compile(r'[a-z][a-z0-9-]{1,48}\Z')
# Every code this helper raises itself. They are preserved verbatim in a receipt.
HELPER_CODES = frozenset({
    'BAD_AVD_NAME', 'BAD_SERIAL', 'SERIAL_UNAVAILABLE', 'SERIAL_AVD_MISMATCH',
    'PREPARE_SHAPE', 'PREPARE_IDENTITY', 'ACQUIRE_SHAPE', 'ACQUIRE_IDENTITY',
    'SESSION_LIST_UNREADABLE', 'SESSION_LIST_UNAVAILABLE', 'SESSION_NOT_NEWLY_ADMITTED',
    'SESSION_NOT_ADMITTED', 'SESSION_NOT_EXCLUSIVE', 'SESSION_WRONG_PLATFORM',
    'RECEIPT_REQUIRED', 'RECEIPT_UNAVAILABLE',
})
CLASS_PATTERN = re.compile(r'[A-Za-z][A-Za-z0-9_]{0,48}\Z')


def closed_code(value, fallback='unspecified'):
    """A code for the receipt: this helper's own codes verbatim, a bounded callback
    code as given, and anything else as the fallback. Raw text never passes."""
    if isinstance(value, str):
        if value in HELPER_CODES:
            return value
        if CODE_PATTERN.match(value):
            return value
    return fallback


def closed_class(value):
    if isinstance(value, str) and CLASS_PATTERN.match(value):
        return value
    return 'Exception'
CLOSED_OPERATIONS = ('app-close', 'panel-close')


class DeviceInUse(Exception):
    """A session already claims this device, or the lock is held: release and skip."""

    code = 'DEVICE_IN_USE'


class CustodyRefused(Exception):
    """Identity, acquisition or admission is not proven, so nothing may be used."""

    def __init__(self, code, detail=''):
        super().__init__(code)
        self.code = code
        self.detail = detail


def lock_path(name):
    if not NAME.fullmatch(str(name)):
        raise CustodyRefused('BAD_AVD_NAME')
    return Path(LOCK_TEMPLATE.format(name=name))


def same_device(display, avd):
    """The retained rows use a display name: `PorticoTest Phone API36` for
    `PorticoTest_Phone_API36`. Spaces and underscores are the only accepted
    difference; both sides must be non-empty strings."""
    return bool(display) and bool(avd) and str(display).replace(' ', '') == str(avd).replace('_', '')


def parse_avd_name(output):
    """The name from `adb -s SERIAL emu avd name`; its answer ends with `OK`."""
    lines = [line.strip() for line in str(output or '').splitlines() if line.strip()]
    if not lines:
        return ''
    return lines[0] if lines[0] != 'OK' else (lines[1] if len(lines) > 1 else '')


def session_rows(document):
    """The retained shape: {"success": true, "data": {"sessions": [ ... ]}}.

    Types are validated, never coerced: a row without a string platform, id and
    device is unreadable rather than guessed at.
    """
    if not isinstance(document, dict) or document.get('success') is not True:
        raise CustodyRefused('SESSION_LIST_UNREADABLE')
    data = document.get('data')
    rows = data.get('sessions') if isinstance(data, dict) else None
    if not isinstance(rows, list):
        raise CustodyRefused('SESSION_LIST_UNREADABLE')
    checked = []
    for row in rows:
        if not isinstance(row, dict):
            raise CustodyRefused('SESSION_LIST_UNREADABLE')
        for key in ('name', 'id', 'device', 'platform'):
            if not isinstance(row.get(key), str):
                raise CustodyRefused('SESSION_LIST_UNREADABLE')
        checked.append(row)
    return checked


def read_sessions(argv=None, runner=None, document=None):
    """A callable for `sessions=`, from an exact argv or a supplied document."""
    if document is not None:
        return lambda: document
    if not argv or not isinstance(argv, (list, tuple)) or not all(isinstance(part, str) for part in argv):
        raise CustodyRefused('SESSION_LIST_UNAVAILABLE')

    def read():
        run = runner or subprocess.run
        result = run(list(argv), capture_output=True, text=True, timeout=20)
        if result.returncode != 0:
            raise CustodyRefused('SESSION_LIST_UNAVAILABLE')
        return json.loads(result.stdout or '{}')

    return read


def claimants(rows, serial, avd, *, apart_from=None):
    """Android sessions claiming this serial **or** this exact AVD on any serial.

    A row is a claimant when its serial equals ours, or its device name is this AVD
    (spaces/underscores aside) whatever serial it reports. `apart_from` removes the
    one session we just admitted, so everything remaining is a conflict.
    """
    found = []
    for row in rows:
        if row.get('platform') != ANDROID:
            continue
        if apart_from is not None and row.get('name') == apart_from:
            continue
        if row.get('id') == serial or same_device(row.get('device'), avd):
            found.append(row)
    return found


def identity_mapping(opened, code):
    """Validate a prepare/acquire mapping: exact keys, string values, no coercion."""
    if not isinstance(opened, dict) or set(opened) != set(IDENTITY_KEYS):
        raise CustodyRefused(code)
    for key in IDENTITY_KEYS:
        if not isinstance(opened[key], str):
            raise CustodyRefused(code)
    if not SESSION_ID.fullmatch(opened['sessionId']):
        raise CustodyRefused(code)
    return dict(opened)


def acquire_mapping(opened):
    return identity_mapping(opened, 'ACQUIRE_SHAPE')


def prepare_mapping(opened):
    """`prepare` returns the exact serial and AVD it opened; no session is required."""
    if not isinstance(opened, dict) or set(opened) != {'serial', 'avd'}:
        raise CustodyRefused('PREPARE_SHAPE')
    if not isinstance(opened.get('serial'), str) or not isinstance(opened.get('avd'), str):
        raise CustodyRefused('PREPARE_SHAPE')
    return {'serial': opened['serial'], 'avd': opened['avd']}


def interpret_handback(returned):
    """Only an explicit structured acknowledgement counts, never a bare success.

    The adapter must name the operation it performed - app-close or panel-close -
    and its closed result. A subprocess exit status, None or an empty mapping is
    not a supported acknowledgement, because a zero exit from `adb` or a `{}` from
    device_close does not retire a session.
    """
    if not isinstance(returned, dict):
        return {'result': 'unspecified'}
    operation = returned.get('operation')
    if operation not in CLOSED_OPERATIONS:
        return {'result': 'unspecified'}
    result = returned.get('result')
    if result in CLOSED_HANDBACK:
        return {'operation': operation, 'result': result}
    if result == 'refused':
        return {'operation': operation, 'result': 'refused',
                'code': closed_code(returned.get('code'))}
    return {'operation': operation, 'result': 'unspecified'}


def _adb_name(serial):
    result = subprocess.run(['adb', '-s', serial, 'emu', 'avd', 'name'],
                            capture_output=True, text=True, timeout=15)
    if result.returncode != 0:
        raise CustodyRefused('SERIAL_UNAVAILABLE')
    return parse_avd_name(result.stdout)


def probe(name):
    """'free', 'busy' or 'bad-name' without touching any device."""
    try:
        path = lock_path(name)
    except CustodyRefused:
        return 'bad-name'
    with path.open('a') as held:
        try:
            fcntl.flock(held, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            return 'busy'
        fcntl.flock(held, fcntl.LOCK_UN)
    return 'free'


def _write_atomic(path, record):
    """Write the receipt or refuse. A lease never proceeds without a written fact."""
    if path is None:
        raise CustodyRefused('RECEIPT_REQUIRED')
    target = Path(path)
    try:
        if not target.parent.is_dir():
            raise OSError('receipt-parent-missing')
        temporary = target.with_name(target.name + '.new')
        descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(descriptor, 'w') as stream:
            json.dump(record, stream, indent=2)
            stream.write('\n')
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, target)
    except OSError:
        raise CustodyRefused('RECEIPT_UNAVAILABLE') from None


def _stamp():
    return datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds')


@contextlib.contextmanager
def lease(avd, serial, *, acquire, sessions, prepare=None, handback=None,
          receipt=None, adb_name=None, timeout=30.0):
    """Lock first, then prepare, identity, admission, use and handback.

    The lock is taken before anything can boot or open. `prepare` (optional) runs
    under the lock and may device_open or boot the target; it must return exactly
    `{'serial','avd'}`. `acquire` then opens the session for real and must return
    exactly `{'sessionId','serial','avd'}`. `handback` receives that admitted tuple
    and must return a closed result. Nothing is installed or used before the
    admitted receipt is written.
    """
    if not SERIAL.fullmatch(str(serial)):
        raise CustodyRefused('BAD_SERIAL')
    path = lock_path(avd)
    name_of = adb_name or _adb_name
    record = {'avd': avd, 'serial': serial, 'lockPath': str(path), 'startedAt': _stamp(),
              'lockHeld': False, 'admitted': False, 'sessionHandback': {'attempted': False},
              'agentSessionRetirement': 'unproven by this helper',
              'physicalState': 'retained/unresolved by this helper'}
    with path.open('a') as held:
        try:
            fcntl.flock(held, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            record.update(outcome='in-use', lockHeld=False)
            _write_atomic(receipt, record)
            raise DeviceInUse()
        try:
            record['lockHeld'] = True
            # The receipt is mandatory: a missing or unwritable parent refuses before
            # prepare, acquire or the body, so no use can happen without a written fact.
            _write_atomic(receipt, record)
            # A claimant means the device is already someone's: skip before anything
            # can boot or open, so prepare never runs against a live session.
            if claimants(session_rows(sessions()), serial, avd):
                record['outcome'] = 'in-use'
                _write_atomic(receipt, record)
                raise DeviceInUse()
            if prepare is not None:
                prepared = prepare_mapping(prepare())
                if prepared['avd'] != avd or prepared['serial'] != serial:
                    raise CustodyRefused('PREPARE_IDENTITY', 'prepare returned a different device')
                record['prepared'] = True
            deadline = time.monotonic() + timeout
            observed = ''
            while time.monotonic() < deadline:
                observed = name_of(serial)
                if observed == avd:
                    break
                if observed:
                    raise CustodyRefused('SERIAL_AVD_MISMATCH', 'serial=%s avd=%s' % (serial, observed))
                time.sleep(0.5)
            if observed != avd:
                raise CustodyRefused('SERIAL_AVD_MISMATCH', 'serial=%s avd=%s' % (serial, observed or 'unknown'))
            record['avdNameProven'] = observed
            # Re-check after prepare and identity, still before the open.
            before = session_rows(sessions())
            if claimants(before, serial, avd):
                record['outcome'] = 'in-use'
                _write_atomic(receipt, record)
                raise DeviceInUse()
            before_names = {row.get('name') for row in before}
            # Open now, and persist whatever came back the moment it exists.
            opened = acquire_mapping(acquire())
            record['openedSession'] = opened['sessionId']
            _write_atomic(receipt, record)
            if opened['avd'] != avd or opened['serial'] != serial:
                raise CustodyRefused('ACQUIRE_IDENTITY', 'acquire returned a different device')
            if opened['sessionId'] in before_names:
                raise CustodyRefused('SESSION_NOT_NEWLY_ADMITTED', 'the returned session was already listed')
            rows = session_rows(sessions())
            conflicts = claimants(rows, serial, avd, apart_from=opened['sessionId'])
            if conflicts:
                raise CustodyRefused('SESSION_NOT_EXCLUSIVE',
                                     '%d other sessions claim this device' % len(conflicts))
            admitted = [row for row in rows if row.get('name') == opened['sessionId']]
            if not admitted:
                raise CustodyRefused('SESSION_NOT_ADMITTED', 'the opened session is not in the supported list')
            if len(admitted) > 1:
                raise CustodyRefused('SESSION_NOT_EXCLUSIVE', 'the returned session is listed more than once')
            entry = admitted[0]
            if entry.get('platform') != ANDROID:
                raise CustodyRefused('SESSION_WRONG_PLATFORM', 'the admitted session is not android')
            if entry.get('id') != serial or not same_device(entry.get('device'), avd):
                raise CustodyRefused('SESSION_NOT_ADMITTED', 'the listed session names another device')
            record.update(admitted=True, sessionId=opened['sessionId'], outcome='held', admittedAt=_stamp())
            _write_atomic(receipt, record)      # held and admitted fact exists before use
            admitted_identity = {'sessionId': opened['sessionId'], 'serial': serial, 'avd': avd}
            try:
                yield {'avd': avd, 'serial': serial, 'sessionId': opened['sessionId'],
                       'lock': str(path), 'admittedAt': record['admittedAt'],
                       'identity': admitted_identity}
                record['outcome'] = 'used'
            finally:
                if handback is not None:
                    try:
                        record['sessionHandback'] = {'attempted': True,
                                                     **interpret_handback(handback(dict(admitted_identity)))}
                    except DeviceInUse:
                        record['sessionHandback'] = {'attempted': True, 'result': 'refused', 'code': 'DEVICE_IN_USE'}
                    except CustodyRefused as refusal:
                        record['sessionHandback'] = {'attempted': True, 'result': 'refused',
                                                     'code': closed_code(refusal.code)}
                    except BaseException as error:
                        record['sessionHandback'] = {'attempted': True, 'result': 'error',
                                                     'code': closed_class(type(error).__name__)}
        except BaseException as error:
            if record.get('outcome') not in ('held', 'used', 'in-use'):
                record['outcome'] = 'refused'
                record['code'] = closed_code(getattr(error, 'code', None), closed_class(type(error).__name__))
            raise
        finally:
            record['lockReleasedAt'] = _stamp()
            record['lockHeld'] = False
            record.setdefault('outcome', 'released')
            _write_atomic(receipt, record)
            fcntl.flock(held, fcntl.LOCK_UN)


if __name__ == '__main__':
    import argparse
    parser = argparse.ArgumentParser(description='Report the custody lock for one AVD, touching no device.')
    parser.add_argument('avd')
    print(probe(parser.parse_args().avd))
