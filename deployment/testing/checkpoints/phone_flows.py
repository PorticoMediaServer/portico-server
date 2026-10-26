#!/usr/bin/env python3
"""Scripted Android phone M0 flows, reusable from the unattended checkpoint.

Owner wiring is one call in ``deployment/testing/checkpoints/android.py``::

    if a.mode == 'phone':
        import phone_flows
        phone_flows.run_phone(d, flow, legacy, launched=True, refusal=Refusal,
                              unavailable=lambda name, code: flows.append(
                                  {'surface': surface, 'id': name, 'status': 'unavailable',
                                   'dependency': code}))
    else:
        ...existing TV sequence...

``launched=True`` keeps the restore flow android.py already records; drop it to
have this module run the restore itself. The machine installer (``install.py``)
copies the checkpoint modules it runs, so ``phone_flows.py`` belongs in that
list beside ``android.py``. The common driver (``flow``) is unchanged.

Two explicit exceptions, so a helper problem is never recorded as a product
failure:

* ``Refusal(code)`` - a drawn-flow expectation failed. ``code`` is a fixed,
  closed string from this file; no pattern, identifier, title, library name or
  other text is ever put in a receipt.
* ``Unavailable(code)`` - the device helper could not produce an authoritative
  tree, or a required playback observation was absent. ``run_phone`` reports
  those through the ``unavailable`` hook (or its returned ``skipped`` list) and
  never through the flow recorder.

Everything here drives the retained, already signed-in session through the
existing ``Device`` facade only: no ``pm clear``, no reset, no re-authentication
and no injected auth state. Every refusal and unavailable code is a fixed
string from this file, so a receipt never carries a pattern, identifier, title,
library name, URL, token or raw exception text.

A tree loss mid-flow reaches the shared driver as the explicit ``Unavailable``
class, which the driver records as an ``unavailable`` row rather than a failure;
the pre-flight and playback paths report through the caller's ``unavailable``
hook so they never reach the recorder at all.

Playback passes only on measured progress of this package's own session: the
same session identity and the same fixture title in two samples, a positive
playback speed, and a position that actually advanced. Otherwise the player's
own committed position in seconds must advance at least 1.5 s with the same
drawn duration and the fixture title visible in both samples. A tap, a 2xx, a
playing state alone or a drawn video image is never playback proof; when
neither observation is available the row is Unavailable, not a pass.
"""
import contextlib
import re
import subprocess
import time

PACKAGE = 'tv.getportico.portico'
HOME_ROWS = r'Continue Watching|New on|On now|Listen'
# The measured fixture this flow plays, compared inside our own session only.
EXPECTED_FIXTURE = '12 angry men'
# A real advance between two samples; smaller is clock or rounding noise.
MIN_ADVANCE_MS = 500
# The watch control reports seconds (not milliseconds), so this is a second-scale
# threshold: a quarter of the old value would have demanded minutes of playback.
MIN_POSITION_ADVANCE_SECONDS = 1.5

# Client settings page titles drawn on the phone sheet. Two of them is a real
# settings list; names shared with the rest of the app (a "Music" row) are left
# out so an ordinary Home screen cannot look like Settings.
SETTINGS_PAGES = ('Profile & account', 'Playback', 'Audio & subtitles', 'Quality & data',
                  'Appearance', 'Language & region', 'Privacy', 'This device')
# Device-facade refusals that may reach the pre-flight; anything else is reported
# as an unavailable tree rather than echoed.
DEVICE_CODES = ('credential-screen', 'authoritative-tree-unavailable', 'device-command-refused',
                'wrong-allocated-avd', 'non-emulator-target')
# A reading library's drawn name. Library ids are opaque, so the name is the only
# bounded handle in the tree; no match means this profile has none here.
READING_LIBRARY = re.compile(r'(?:books?|comics?|reading|manga)', re.I)
# The per-package block header MediaSessionService prints, e.g. "tv.getportico.portico (userId=0)".
SESSION_HEADER = re.compile(r'^\s{2,8}([A-Za-z][\w.]*\.[A-Za-z][\w.]*)\s+\(\s*userId=\d+[^)]*\)\s*$', re.M)
SESSION_ID = re.compile(r'MediaSessionRecord\{[^}]*?\bid=([^\s,}]+)', re.S)


class Refusal(Exception):
    """A drawn-flow expectation failed; the code is a fixed closed string."""

    def __init__(self, code):
        super().__init__(code)
        self.code = code


class Unavailable(Exception):
    """The device helper or a required observation was unavailable."""

    def __init__(self, code):
        super().__init__(code)
        self.code = code


_REFUSAL = Refusal


def _refuse(code):
    raise _REFUSAL(code)


def _unavailable(code):
    raise Unavailable(code)


def _nodes(device):
    try:
        return device.nodes()
    except Exception as error:
        if str(error) == 'authoritative-tree-unavailable':
            _unavailable('tree-unavailable')
        raise


def _text(device):
    try:
        return device.text()
    except Exception as error:
        if str(error) == 'authoritative-tree-unavailable':
            _unavailable('tree-unavailable')
        raise


def _refused_by_device(error):
    return str(error) == 'device-command-refused'


def _wait_text(device, pattern, timeout=20, code='expected-ui-timeout'):
    until = time.monotonic() + timeout
    while time.monotonic() < until:
        if re.search(pattern, _text(device), re.I):
            return True
        time.sleep(1)
    _refuse(code)


def _wait_identifier(device, prefix, timeout=25, code='expected-screen-timeout'):
    until = time.monotonic() + timeout
    while time.monotonic() < until:
        if any(str(node.get('identifier', '')).startswith(prefix) for node in _nodes(device)):
            return True
        time.sleep(1)
    _refuse(code)


def _pressable(node):
    if node.get('hittable') is not True or node.get('enabled') is False:
        return False
    identifier = str(node.get('identifier', ''))
    return str(node.get('type', '')).endswith('Button') or identifier.startswith('phone-tab-')


def _tap(device, pattern, identifier=None, exclude=None, timeout=20, code='control-not-drawn'):
    """Tap one drawn control by its exact visible label.

    A reference is only valid for the snapshot it came from, so a refused click
    re-reads and retries inside the same deadline. A tree the helper cannot
    produce is Unavailable, never a product failure. The identifier prefix picks
    one of several controls that share a label (the "Libraries" tab and a
    library header's own "Libraries" back control).
    """
    until = time.monotonic() + timeout
    ambiguous = 0
    while time.monotonic() < until:
        matches = [node for node in _nodes(device)
                   if _pressable(node) and re.fullmatch(pattern, str(node.get('label', '')))]
        if identifier is not None:
            matches = [node for node in matches if str(node.get('identifier', '')).startswith(identifier)]
        if exclude is not None:
            matches = [node for node in matches if not str(node.get('identifier', '')).startswith(exclude)]
        if len(matches) == 1:
            wanted = str(matches[0].get('ref', ''))
            if not re.fullmatch('e[0-9]+', wanted):
                _refuse('invalid-control-reference')
            try:
                device.command(['click', '@' + wanted])
                return True
            except Exception as error:
                if not _refused_by_device(error):
                    raise
                time.sleep(0.2)
                continue
        if matches:
            ambiguous += 1
            if ambiguous >= 3:
                _refuse('ambiguous-visible-control')
        time.sleep(0.5)
    _refuse(code)


def _click(device, node, code):
    wanted = str(node.get('ref', '')) if node else ''
    if not re.fullmatch('e[0-9]+', wanted):
        _refuse(code)
    device.command(['click', '@' + wanted])
    return True


def _first_control(device, prefix):
    for node in _nodes(device):
        if str(node.get('identifier', '')).startswith(prefix) and node.get('hittable') is True and node.get('enabled') is not False:
            return node
    return None


def _focus_field(device, pattern=None, identifier=None, timeout=15, code='editable-field-not-drawn'):
    until = time.monotonic() + timeout
    while time.monotonic() < until:
        fields = [node for node in _nodes(device) if node.get('editable') is True]
        if identifier is not None:
            fields = [node for node in fields if str(node.get('identifier', '')).startswith(identifier)]
        else:
            fields = [node for node in fields if re.fullmatch(pattern or '', str(node.get('label', '')))]
        if len(fields) == 1:
            if fields[0].get('focused') is True:
                return True
            try:
                _click(device, fields[0], 'invalid-field-reference')
                return True
            except Exception as error:
                if not _refused_by_device(error):
                    raise
        time.sleep(0.4)
    _refuse(code)


def _type(device, value):
    """Type a public search query; no credential or private value reaches this path."""
    escaped = value.replace(' ', '%s')
    subprocess.run(['adb', '-s', device.serial, 'shell', 'input', 'text', escaped],
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=20)


def _back(device, times=1):
    for _ in range(times):
        device.key('KEYCODE_BACK')
        time.sleep(0.8)


def _home_tab(device):
    return next((node for node in _nodes(device)
                 if str(node.get('identifier', '')) == 'phone-tab-home' and node.get('hittable') is True), None)


def _sheet_open(device):
    """True while the phone Settings sheet is up, by its own drawn Done control."""
    return any(str(node.get('identifier', '')) == 'settings-sheet-done' and node.get('hittable') is True
               for node in _nodes(device))


def _to_home(device, rows, timeout=20):
    """Reach Home from wherever the previous flow finished, without clearing anything.

    The Home tab is drawn on every phone route, so it is always the way back; a
    covered tab means the Settings sheet or a player is up, so the sheet leaves
    through its own Done control and anything else through one Back.
    """
    if _sheet_open(device):
        _settings_close(device)
    for _ in range(3):
        if _home_tab(device):
            _tap(device, r'Home', 'phone-tab-home', code='home-tab-unavailable')
            return _wait_text(device, rows, timeout, 'home-rows-timeout')
        _back(device)
    _refuse('home-tab-unavailable')


def _library_screen(device, timeout=25):
    """'overview' for the Libraries list, 'library' inside one library.

    Inactive routes stay mounted behind the active one, so the list is decided by
    cards that are actually hittable and only then by a drawn library frame.
    """
    until = time.monotonic() + timeout
    while time.monotonic() < until:
        nodes = _nodes(device)
        if any(str(node.get('identifier', '')).startswith('libraries-open-') and node.get('hittable') is True
               for node in nodes):
            return 'overview'
        if any(str(node.get('identifier', '')).startswith('native-library-') for node in nodes):
            return 'library'
        time.sleep(1)
    _refuse('library-screen-timeout')


def _library_cards(device):
    return [node for node in _nodes(device)
            if str(node.get('identifier', '')).startswith('libraries-open-') and node.get('hittable') is True]


def _libraries_overview(device, rows):
    """Open the Libraries list even when a remembered library opens straight in."""
    _to_home(device, rows)
    _tap(device, r'Libraries', 'phone-tab-libraries', code='libraries-tab-unavailable')
    if _library_screen(device) == 'overview':
        return _library_cards(device)
    # A remembered library is drawn instead; its own header returns to the list.
    _tap(device, r'Libraries', exclude='phone-tab-', code='libraries-back-unavailable')
    if _library_screen(device) != 'overview':
        _refuse('libraries-overview-unavailable')
    return _library_cards(device)


def _open_library(device, cards, name):
    choice = next((node for node in cards if str(node.get('label', '')) == name), None)
    if choice is None:
        _refuse('library-card-unavailable')
    _click(device, choice, 'library-card-unavailable')
    _wait_identifier(device, 'native-library-', 25, 'library-open-timeout')


def _settings_found(text):
    return {name for name in SETTINGS_PAGES if name in text}


def _settings_open(device, rows, timeout=20):
    """Open the phone Settings sheet from its Home avatar, or use it if it is already drawn."""
    found = _settings_found(_text(device))
    if len(found) >= 2:
        return found
    _to_home(device, rows)
    _tap(device, r'Settings', timeout=25, code='settings-entry-unavailable')
    until = time.monotonic() + timeout
    while time.monotonic() < until:
        found = _settings_found(_text(device))
        if len(found) >= 2:
            return found
        time.sleep(1)
    _refuse('settings-pages-timeout')


def _settings_close(device):
    """Leave the sheet through its own Done control, backing out of any pushed page first."""
    for _ in range(3):
        try:
            _tap(device, r'Done', timeout=3)
            return True
        except Exception:
            _back(device)
    return False


def _media_block(raw):
    """The media session block that belongs to this package, or None."""
    marks = list(SESSION_HEADER.finditer(raw))
    for index, mark in enumerate(marks):
        if mark.group(1) != PACKAGE:
            continue
        end = marks[index + 1].start() if index + 1 < len(marks) else len(raw)
        return raw[mark.end():end]
    return None


def _media_sample(probe):
    """One bounded observation of this package's own playing session, or None."""
    raw, code = probe.shell('dumpsys', 'media_session')
    if code:
        return None
    block = _media_block(raw)
    if block is None:
        return None
    state = (re.search(r'state=PlaybackState\s*\{[^}]*?state=(\d+)', block)
             or re.search(r'PlaybackState[^\n]*?state=(\d+)', block))
    if not state or int(state.group(1)) != 3:
        return None
    session = SESSION_ID.search(block)
    position = re.search(r'\bposition=(\d+)', block)
    speed = re.search(r'\bspeed=([0-9]+(?:\.[0-9]+)?)', block)
    return {
        'session': session.group(1) if session else None,
        'position': int(position.group(1)) if position else None,
        'speed': float(speed.group(1)) if speed else None,
        'fixture': EXPECTED_FIXTURE in block.lower(),
    }


def _media_progress(probe, window=30):
    """(detail, reason code) for a playing session that really advanced.

    Both samples must be the same session and the same fixture, must report a
    positive speed, and the second position must have advanced.
    """
    deadline = time.monotonic() + window
    first = None
    while time.monotonic() < deadline:
        sample = _media_sample(probe)
        if sample is not None:
            if sample['session'] is None:
                return None, 'playback-session-unavailable'
            if not sample['fixture']:
                return None, 'playback-target-unavailable'
            if sample['speed'] is None or sample['speed'] <= 0:
                return None, 'playback-speed-unavailable'
            if sample['position'] is None:
                return None, 'playback-position-unavailable'
            first = sample
            break
        time.sleep(2)
    if first is None:
        return None, 'playback-session-unavailable'
    time.sleep(3)
    second = _media_sample(probe)
    if second is None:
        return None, 'playback-session-ended'
    if second['session'] != first['session']:
        return None, 'playback-session-changed'
    if not second['fixture']:
        return None, 'playback-target-unavailable'
    if second['speed'] is None or second['speed'] <= 0:
        return None, 'playback-speed-unavailable'
    if second['position'] is None:
        return None, 'playback-position-unavailable'
    if second['position'] >= first['position'] + MIN_ADVANCE_MS:
        return 'media-session-position-advanced', None
    return None, 'playback-position-stalled'


def _seek_sample(device):
    """The player's own committed position and duration in seconds, with the drawn title.

    The watch control's accessibility value is seconds, not milliseconds, and it
    is a float, so both numbers stay as drawn. The fixture title must be visible
    in the same sample: position alone does not say which item is playing.
    """
    seek = None
    fixture = False
    for node in _nodes(device):
        if EXPECTED_FIXTURE in str(node.get('label', '')).lower():
            fixture = True
        if seek is None and str(node.get('identifier', '')) == 'watch-seek':
            seek = node
    if seek is None:
        return None
    value = seek.get('value', seek.get('accessibilityValue'))
    if isinstance(value, dict):
        now, end = value.get('now'), value.get('max')
    else:
        now, end = value, seek.get('max')
    if not isinstance(now, (int, float)) or isinstance(now, bool):
        return None
    return {'now': float(now),
            'end': float(end) if isinstance(end, (int, float)) and not isinstance(end, bool) else None,
            'fixture': fixture}


def _seek_progress(device, window=20):
    """(detail, reason code) for the player position advancing inside one drawn item."""
    first = _seek_sample(device)
    if first is None:
        return None, 'player-position-unavailable'
    if first['end'] is None or first['end'] <= 0:
        return None, 'player-target-unavailable'
    if not first['fixture']:
        return None, 'player-target-unavailable'
    compared, deadline = 0, time.monotonic() + window
    while time.monotonic() < deadline:
        time.sleep(2.5)
        again = _seek_sample(device)
        if again is None:
            continue
        if not again['fixture']:
            return None, 'player-target-unavailable'
        if again['end'] is None or abs(again['end'] - first['end']) > 0.001:
            return None, 'player-target-changed'
        compared += 1
        if again['now'] >= first['now'] + MIN_POSITION_ADVANCE_SECONDS:
            return 'player-position-advanced', None
    return None, ('player-position-stalled' if compared else 'player-position-unavailable')


def _playback_progress(device, legacy):
    """Measured progress only; anything unobservable is Unavailable, never a pass or a product fail."""
    probe = legacy.Device(device.serial, str(device.private))
    detail, reason = _media_progress(probe)
    if detail is not None:
        return detail
    detail, seek_reason = _seek_progress(device)
    if detail is not None:
        return detail
    _unavailable(reason or seek_reason or 'playback-progress-unavailable')


def _stop_playback(device):
    with contextlib.suppress(Exception):
        device.key('KEYCODE_MEDIA_STOP')
    time.sleep(0.5)
    with contextlib.suppress(Exception):
        _back(device)


def _launch(device):
    subprocess.run(['adb', '-s', device.serial, 'shell', 'am', 'start', '-W', '-n', PACKAGE + '/.MainActivity'],
                   check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=20)


def run_phone(device, flow, legacy, *, launched=False, unavailable=None, refusal=Refusal):
    """Run the bounded phone M0 flows through the checkpoint's own recorder.

    ``flow(name, fn)`` records one pass/fail row with its screenshot; it is used
    unchanged. ``legacy`` is the v2 smoke module, for its media-session probe and
    fixture text only. ``launched=True`` skips the restore, because the caller
    already ran it and the app is on Home. ``refusal`` lets the caller pass its
    own refusal class so a failed row carries the fixed code. ``unavailable(name,
    code)`` records an area this profile cannot offer, or an observation the
    device helper could not supply, instead of a false pass; without it those
    rows are returned in ``skipped``.
    """
    global _REFUSAL
    _REFUSAL = refusal
    if getattr(device, 'mode', None) != 'phone':
        _refuse('phone-allocation-required')
    if not re.fullmatch('emulator-[0-9]{4,5}', str(getattr(device, 'serial', ''))):
        _refuse('non-emulator-target')
    rows = getattr(getattr(legacy, 'HOME_ROWS', None), 'pattern', HOME_ROWS)
    names, skipped = [], []

    def not_reached(name, code):
        if unavailable is None:
            skipped.append({'id': name, 'reason': code})
        else:
            unavailable(name, code)
        return False

    def record(name, fn):
        try:
            _nodes(device)
        except Unavailable as error:
            return not_reached(name, error.code)
        except Exception as error:
            code = str(error)
            return not_reached(name, code if code in DEVICE_CODES else 'device-tree-unavailable')
        if flow(name, fn):
            names.append(name)
            return True
        return False

    def restored():
        _launch(device)
        _wait_text(device, rows, 30, 'home-rows-timeout')
        return 'restored retained family drew Home rows'

    if not launched and not record('launch-restored-family', restored):
        not_reached('remaining-phone-flows', 'restore-unavailable')
        return {'surface': 'android-phone', 'flows': names, 'skipped': skipped,
                'coverage': 'restore unavailable on the retained session; no reset or re-authentication fallback'}

    home = record('home-real-rows', lambda: _wait_text(device, rows, 20, 'home-rows-timeout'))
    if not home:
        not_reached('remaining-phone-flows', 'home-rows-timeout')

    def libraries():
        cards = _libraries_overview(device, rows)
        return 'libraries overview drew %d libraries' % len(cards)

    libraries_ok = home and record('libraries-open', libraries)

    def movies():
        if re.search(r'12 Angry Men', _text(device)):
            return 'movies grid already drawn'
        _open_library(device, _libraries_overview(device, rows), 'Movies')
        _wait_text(device, r'12 Angry Men', 25, 'movies-grid-timeout')
        return 'Movies opened with the fixture title'

    movies_ok = libraries_ok and record('movies-browse', movies)

    def detail():
        _tap(device, r'12 Angry Men(?:,.*)?', code='fixture-title-unavailable')
        _wait_text(device, r'Play|Resume', 20, 'detail-control-timeout')
        return 'detail drew its primary play control'

    detail_ok = movies_ok and record('movie-detail', detail)

    if detail_ok:
        def playback_open():
            _tap(device, r'(?:Play|Resume)(?:,.*)?', code='play-control-unavailable')
            return 'play control tapped on the fixture detail'

        if record('movie-playback-open', playback_open):
            evidence = None
            try:
                evidence = _playback_progress(device, legacy)
            except Unavailable as error:
                not_reached('movie-playback-progress', error.code)
            finally:
                _stop_playback(device)
            if evidence is not None:
                record('movie-playback-progress', lambda: evidence)

    def settings():
        found = _settings_open(device, rows)
        return '%d client settings pages drawn' % len(found)

    settings_ok = home and record('settings-open', settings)

    def settings_page():
        _settings_open(device, rows)
        _tap(device, r'Playback', code='settings-page-unavailable')
        _wait_text(device, r'Play the next episode automatically|Next episode', 20, 'settings-page-rows-timeout')
        _tap(device, r'Settings', code='settings-back-unavailable')
        return 'Playback page drew its own rows'

    if settings_ok:
        record('settings-client-page', settings_page)

    def profile():
        _settings_open(device, rows)
        _tap(device, r'Profile & account', code='profile-page-unavailable')
        _wait_text(device, r'Profiles|Password', 20, 'profile-page-timeout')
        _tap(device, r'Settings', code='settings-back-unavailable')
        _settings_close(device)
        return 'profile page reached from Settings'

    if settings_ok:
        record('profile-open', profile)

    def settings_search():
        _settings_open(device, rows)
        _focus_field(device, identifier='settings-search', code='settings-search-field-unavailable')
        _type(device, 'subtitles')
        _wait_text(device, r'Show subtitles', 20, 'settings-search-timeout')
        unfiltered = [name for name in SETTINGS_PAGES if name != 'Audio & subtitles' and name in _text(device)]
        if len(unfiltered) > 2:
            _refuse('settings-search-not-filtered')
        # Put the sheet back as it was drawn: the query is a checker input, not app state.
        device.key('KEYCODE_MOVE_END')
        for _ in range(len('subtitles')):
            device.key('KEYCODE_DEL')
        _wait_text(device, r'Profile & account', 20, 'settings-search-not-cleared')
        _settings_close(device)
        return 'settings search narrowed the list to matching rows'

    if settings_ok:
        record('settings-search', settings_search)

    def search():
        _to_home(device, rows)
        _tap(device, r'Search', 'phone-tab-search', code='search-tab-unavailable')
        _wait_identifier(device, 'native-search-screen', 20, 'search-screen-timeout')
        _focus_field(device, identifier='search-input', code='search-field-unavailable')
        _type(device, '12 Angry Men')
        try:
            _wait_text(device, r'12 Angry Men', 20, 'search-results-timeout')
        except _REFUSAL as error:
            # A drawn soft keyboard can cover the first rows; hide it once and look again.
            if str(error) != 'search-results-timeout':
                raise
            _back(device)
            _wait_text(device, r'12 Angry Men', 15, 'search-results-timeout')
        return 'query drew a matching result'

    search_ok = home and record('search-results', search)

    def search_open():
        _tap(device, r'12 Angry Men(?:,.*)?', code='search-result-unavailable')
        _wait_text(device, r'Play|Resume', 20, 'search-result-detail-timeout')
        _back(device)
        return 'result opened the title screen'

    if search_ok:
        record('search-open-result', search_open)

    if libraries_ok:
        # A profile with no reading library is a coverage gap, not a product failure, so the
        # probe settles that outside the recorder and only real trouble becomes a failed flow.
        failure, cards = None, None
        try:
            cards = _libraries_overview(device, rows)
        except Unavailable as error:
            not_reached('reading-reach', error.code)
        except Exception:
            failure = 'libraries-overview-unavailable'
        else:
            if not cards:
                failure = 'libraries-overview-empty'
        if failure is not None:
            def unreachable():
                _refuse(failure)
            record('reading-reach', unreachable)
        elif cards is not None:
            choice = next((node for node in cards
                           if READING_LIBRARY.fullmatch(str(node.get('label', '')))), None)
            if choice is None:
                not_reached('reading-reach', 'reading-library-unavailable')
            else:
                def reading():
                    _click(device, choice, 'reading-library-unavailable')
                    _wait_identifier(device, 'native-reading-library-', 25, 'reading-library-open-timeout')
                    book = _first_control(device, 'reading-item-')
                    if book is None:
                        _refuse('reading-book-unavailable')
                    _click(device, book, 'reading-book-unavailable')
                    _wait_identifier(device, 'native-read-book-', 25, 'reading-book-open-timeout')
                    return 'reading library and book screen reached; page content stays owner read'
                record('reading-reach', reading)

    return {
        'surface': 'android-phone',
        'flows': names,
        'skipped': skipped,
        'coverage': ('restore/Home/Libraries/Movies/detail, real playback progress, Settings pages and search, '
                     'in-app search and open, reading reach and profile settings; seek/pause/subtitles, profile '
                     'switching, sign-out and reader page content remain owner or hands-on'),
    }
