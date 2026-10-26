"""Synthetic, unit-level validation of the comparer itself.

No app, device, snapshot or walk fixture takes part: the documents here are built
in code and the screenshots are generated. This proves the tool's arithmetic and
its closed refusals, never that any screen matches its frame.
"""
import json
import struct
import tempfile
import zlib
from pathlib import Path

from . import colour as colour_module, compare, frame as frame_module, snapshot as snapshot_module
from .errors import BlockedEnv, RefusedCredential

DEVICE = {'width': 100, 'height': 200}


def _chunk(kind, body):
    return struct.pack('>I', len(body)) + kind + body + struct.pack('>I', zlib.crc32(kind + body) & 0xFFFFFFFF)


def _png(width, height, rgb, filter_type=0):
    rows = b''.join(bytes([filter_type]) + bytes(rgb) * width for _ in range(height))
    header = struct.pack('>IIBBBBB', width, height, 8, 2, 0, 0, 0)
    return b'\x89PNG\r\n\x1a\n' + _chunk(b'IHDR', header) + _chunk(b'IDAT', zlib.compress(rows)) + _chunk(b'IEND', b'')


def _paeth(a, b, c):
    """The PNG specification's Paeth predictor, ties to a, then b, then c."""
    estimate = a + b - c
    distance_a, distance_b, distance_c = abs(estimate - a), abs(estimate - b), abs(estimate - c)
    if distance_a <= distance_b and distance_a <= distance_c:
        return a
    if distance_b <= distance_c:
        return b
    return c


def _paeth_png():
    """Two rows where the target pixel's tie is between above and upper-left.

    Row 0 is the previous row: pixel 0 is upper-left, pixel 1 is above. Row 1
    pixel 0 is left (10), pixel 1 is the target (100). For the target channels
    the three values are left 10, above 40, upper-left 20, so the estimates tie
    between above and upper-left and the specification chooses above (40). A
    decoder that took the numerically smaller value would answer 80.
    """
    channels = 3
    row0 = bytes([20, 20, 20, 40, 40, 40])
    row1 = bytes([10, 10, 10, 100, 100, 100])
    filtered = bytearray(b'\x00' + row0)
    line = bytearray()
    for index, value in enumerate(row1):
        left = row1[index - channels] if index >= channels else 0
        above = row0[index]
        corner = row0[index - channels] if index >= channels else 0
        line.append((value - _paeth(left, above, corner)) & 0xFF)
    filtered += b'\x04' + bytes(line)
    header = struct.pack('>IIBBBBB', 2, 2, 8, 2, 0, 0, 0)
    png = (b'\x89PNG\r\n\x1a\n' + _chunk(b'IHDR', header)
           + _chunk(b'IDAT', zlib.compress(bytes(filtered))) + _chunk(b'IEND', b''))
    return png, row1


def _frame(label='Home', text_mode='static', identifier=None, background='rgb(16, 32, 48)',
           extra_style=None):
    style = {'backgroundColor': background, 'backgroundImage': 'none',
             'color': 'rgb(237, 245, 250)', 'fontSize': '17px', 'fontWeight': '600',
             'borderRadius': '12px'}
    style.update(extra_style or {})
    element = {'selector': '.tab.home', 'label': label, 'role': 'button', 'box': [0, 0, 100, 40],
               'style': style, 'textMode': text_mode}
    if identifier:
        element['identifier'] = identifier
    return {
        'tool': 'render-frame',
        'frame': 'home#1',
        'surface': 'ios-phone',
        'source': {'file': 'home.html', 'label': '1'},
        'device': dict(DEVICE),
        'units': 'points',
        'elements': [
            element,
            {'selector': '.row.title', 'label': 'Continue Watching', 'role': 'heading',
             'box': [10, 50, 80, 20],
             'style': {'color': 'rgb(255, 255, 255)', 'backgroundColor': 'rgb(16, 32, 48)',
                       'backgroundImage': 'none'},
             'textMode': 'static'},
        ],
    }


def _snapshot(moved=0, resized=0, extra=None, label='Home', nodes=None):
    if nodes is None:
        nodes = [
            {'label': label, 'type': 'Button', 'frame': [0, 0 + moved, 100, 40 + resized]},
            {'label': 'Continue Watching', 'type': 'StaticText', 'frame': [10, 50, 80, 20]},
        ]
        if extra:
            nodes.append({'label': extra, 'type': 'Image', 'frame': [0, 180, 20, 20]})
    return {'success': True, 'data': {'nodes': nodes, 'truncated': False, 'device': dict(DEVICE)}}


def run():
    """Run every synthetic case; return the closed receipt payload."""
    cases, failures = 0, 0
    with tempfile.TemporaryDirectory(prefix='portico-native-layout-') as scratch:
        root = Path(scratch)

        def write(name, payload, binary=False):
            path = root / name
            path.write_bytes(payload if binary else json.dumps(payload).encode())
            return path

        def load(payload):
            return frame_module.load(write('frame.json', payload))

        def drawn(payload):
            return snapshot_module.load(write('snapshot.json', payload), 'ios-phone')

        def expect_code(action, wanted):
            nonlocal cases, failures
            cases += 1
            try:
                action()
            except (BlockedEnv, RefusedCredential) as blocked:
                if blocked.code != wanted:
                    failures += 1
            else:
                failures += 1

        # 1. An identical drawn tree measures as no difference at all.
        cases += 1
        receipt = compare.compare(load(_frame()), drawn(_snapshot()), {})
        if receipt['outcome'] != 'MEASURED' or any(receipt['counts'][key] for key in
                                                   ('missing', 'extra', 'moved', 'resized', 'text', 'colour')):
            failures += 1

        # 2. A moved control cites the frame element and both y values.
        cases += 1
        receipt = compare.compare(load(_frame()), drawn(_snapshot(moved=12)), {})
        moved = [item for item in receipt['differences'] if item['kind'] == 'position']
        if not moved or moved[0]['selector'] != '.tab.home' or moved[0]['file'] != 'home.html' \
                or 'y expected 0.0 actual 12.0' not in moved[0]['line']:
            failures += 1

        # 3. A resized control cites both heights.
        cases += 1
        receipt = compare.compare(load(_frame()), drawn(_snapshot(resized=-4)), {})
        sized = [item for item in receipt['differences'] if item['kind'] == 'size']
        if not sized or 'height expected 40.0 actual 36.0' not in sized[0]['line']:
            failures += 1

        # 4. A control the frame draws and the app does not is a missing control.
        only_text = {'success': True, 'data': {'nodes': [
            {'label': 'Continue Watching', 'type': 'StaticText', 'frame': [10, 50, 80, 20]}],
            'truncated': False, 'device': dict(DEVICE)}}
        cases += 1
        receipt = compare.compare(load(_frame()), drawn(only_text), {})
        missing = [item for item in receipt['differences'] if item['kind'] == 'missing-control']
        if not missing or missing[0]['selector'] != '.tab.home' or missing[0]['expected']['height'] != 40.0:
            failures += 1

        # 5. An extra drawn element is cited to the frame and never echoes its text.
        cases += 1
        receipt = compare.compare(load(_frame()), drawn(_snapshot(extra='Private Name')), {})
        extra = [item for item in receipt['differences'] if item['kind'] == 'extra-element']
        if not extra or 'Private Name' in json.dumps(receipt):
            failures += 1

        # 6. Real data with a different title is the same element: geometry only.
        cases += 1
        data_receipt = compare.compare(load(_frame(text_mode='data')),
                                       drawn(_snapshot(label='A Different Real Title')), {})
        if (data_receipt['differences'] or data_receipt['counts']['text']
                or data_receipt['counts']['missing'] or data_receipt['counts']['data'] != 1):
            failures += 1

        # 7. An explicit identifier maps a control whose text also differs.
        cases += 1
        mapped = _frame(identifier='phone-tab-home')
        node = {'label': 'Home tab', 'type': 'Button', 'identifier': 'phone-tab-home', 'frame': [0, 0, 100, 40]}
        other = {'label': 'Continue Watching', 'type': 'StaticText', 'frame': [10, 50, 80, 20]}
        receipt = compare.compare(load(mapped), drawn({'success': True, 'data': {
            'nodes': [node, other], 'truncated': False, 'device': dict(DEVICE)}}), {})
        if receipt['counts']['matched'] != 2 or not any(item['kind'] == 'text' for item in receipt['differences']):
            failures += 1

        # 8. A credential-bearing snapshot is refused whole; a plain editable one blocks.
        credential = {'success': True, 'data': {'nodes': [
            {'label': 'Password', 'type': 'SecureTextField', 'frame': [0, 0, 100, 40], 'password': True}],
            'truncated': False}}
        expect_code(lambda: drawn(credential), 'credential-bearing-snapshot')
        form = {'success': True, 'data': {'nodes': [
            {'label': 'Search', 'type': 'SearchField', 'frame': [0, 0, 100, 40], 'editable': True}],
            'truncated': False}}
        expect_code(lambda: drawn(form), 'editable-screen-unsupported')

        # 9. Vega snapshots, a missing snapshot and a truncated tree never compare.
        expect_code(lambda: snapshot_module.load(write('vega.json', _snapshot()), 'vega'),
                    'snapshot-vega-unsupported')
        expect_code(lambda: snapshot_module.load(root / 'absent.json', 'ios-phone'), 'snapshot-missing')
        truncated = {'success': True, 'data': {'nodes': _snapshot()['data']['nodes'], 'truncated': True}}
        expect_code(lambda: drawn(truncated), 'snapshot-truncated')

        # 10. A frame without a selector cannot cite a finding.
        broken = _frame()
        del broken['elements'][0]['selector']
        expect_code(lambda: load(broken), 'frame-schema-unsupported')

        # 11. A flat fill that differs is reported against the frame's computed value.
        shot = write('shot.png', _png(100, 200, (0x40, 0x50, 0x60)), binary=True)
        png = colour_module.read_png(shot)
        cases += 1
        receipt = compare.compare(load(_frame()), drawn(_snapshot()), {}, png=png)
        colour_rows = [item for item in receipt['differences'] if item['kind'] == 'colour']
        if not colour_rows or colour_rows[0]['selector'] != '.tab.home' or receipt['colour']['sampled'] != 1:
            failures += 1

        # 12. Text fill is never sampled, so rasterisation cannot become a finding.
        cases += 1
        if [item for item in receipt['differences']
                if item['kind'] == 'colour' and item['selector'] == '.row.title']:
            failures += 1

        # 13. Computed rgb()/rgba() fills: opaque compares, translucent is skipped.
        cases += 1
        if colour_module.declared('rgb(16, 32, 48)')[0] != [16, 32, 48] \
                or colour_module.declared('rgb(16 32 48)')[0] != [16, 32, 48] \
                or colour_module.declared('rgba(16, 32, 48, 1)')[0] != [16, 32, 48] \
                or colour_module.declared('rgba(16, 32, 48, 0.4)')[1] != 'translucent-fill' \
                or colour_module.declared('linear-gradient(red, blue)')[1] != 'no-solid-fill' \
                or colour_module.declared(None, [1, 2, 3])[0] != [1, 2, 3]:
            failures += 1

        # 14. Paeth decoding follows the specification's tie order.
        cases += 1
        encoded, raw_second_row = _paeth_png()
        paeth = write('paeth.png', encoded, binary=True)
        try:
            decoded = colour_module.read_png(paeth)
            if bytes(decoded[3][1]) != raw_second_row:
                failures += 1
        except colour_module.ColourUnavailable:
            failures += 1

        # 15. Oversized screenshots are refused before any allocation.
        header = struct.pack('>IIBBBBB', 20000, 20000, 8, 2, 0, 0, 0)
        huge = write('huge.png', b'\x89PNG\r\n\x1a\n' + _chunk(b'IHDR', header) + _chunk(b'IEND', b''), binary=True)
        cases += 1
        try:
            colour_module.read_png(huge)
            failures += 1
        except colour_module.ColourUnavailable as unavailable:
            if unavailable.code != 'screenshot-dimensions-unsupported':
                failures += 1

        # 16. A gradient or image background is skipped, never judged.
        cases += 1
        gradient = compare.compare(load(_frame(extra_style={'backgroundImage': 'gradient'})),
                                   drawn(_snapshot()), {}, png=png)
        if 'no-solid-fill' not in gradient['colour']['notes'] or gradient['counts']['colour']:
            failures += 1

        # 17. The explicit normalisation wins, both element-level and in style.
        cases += 1
        opaque = _frame(background='rgb(200, 200, 200)')
        opaque['elements'][0]['style']['normalizedRGB'] = {'backgroundColor': [90, 90, 90, 1],
                                                           'color': [237, 245, 250, 1]}
        opaque['elements'][0]['normalizedRGB'] = [16, 32, 48]
        style_only = _frame(background='rgb(200, 200, 200)')
        style_only['elements'][0]['style']['normalizedRGB'] = {'backgroundColor': [90, 90, 90, 1],
                                                               'color': [237, 245, 250, 1]}
        translucent = _frame()
        translucent['elements'][0]['style']['normalizedRGB'] = {'backgroundColor': [16, 32, 48, 0.5],
                                                                'color': [237, 245, 250, 1]}
        opaque_rows = [item for item in compare.compare(load(opaque), drawn(_snapshot()), {}, png=png)['differences']
                       if item['kind'] == 'colour']
        style_rows = [item for item in compare.compare(load(style_only), drawn(_snapshot()), {}, png=png)['differences']
                      if item['kind'] == 'colour']
        translucent_receipt = compare.compare(load(translucent), drawn(_snapshot()), {}, png=png)
        if not opaque_rows or 'backgroundColor expected #102030' not in opaque_rows[0]['line'] \
                or not style_rows or 'backgroundColor expected #5a5a5a' not in style_rows[0]['line'] \
                or 'translucent-fill' not in translucent_receipt['colour']['notes']:
            failures += 1

        # 18. The renderer's semantic roles pair with the native type names.
        cases += 1
        heading = {'label': 'Continue Watching', 'type': 'StaticText', 'frame': [10, 50, 80, 20]}
        heading_snapshot = {'success': True, 'data': {
            'nodes': [{'label': 'Home', 'type': 'Button', 'frame': [0, 0, 100, 40]}, heading],
            'truncated': False, 'device': dict(DEVICE)}}
        paired = compare.compare(load(_frame()), drawn(heading_snapshot), {})
        if paired['counts']['matched'] != 2 or paired['differences']:
            failures += 1

        # 19. The documented Android schema: text only in `value`, `rect` objects,
        # `kind` roles and a `viewport`. Fields as the device helper writes them; the
        # copy here is synthetic so no device content is retained.
        cases += 1
        documented = {'success': True, 'data': {
            'viewport': {'width': 100, 'height': 200}, 'truncated': False,
            'nodes': [
                {'type': 'android.widget.Button', 'kind': 'button', 'value': 'Home',
                 'rect': {'x': 0, 'y': 0, 'width': 100, 'height': 40}, 'hittable': True},
                {'type': 'android.widget.TextView', 'kind': 'text', 'value': 'Continue Watching',
                 'rect': {'x': 10, 'y': 50, 'width': 80, 'height': 20}},
            ]}}
        loaded = drawn(documented)
        receipt = compare.compare(load(_frame()), loaded, {})
        if loaded['width'] != 100 or loaded['elements'][0]['label'] != 'Home' \
                or receipt['counts']['matched'] != 2 or receipt['differences']:
            failures += 1

        # 20. A viewport narrower than the frame's device size is a setup mismatch,
        # reported by the caller's dimension check rather than scaled.
        cases += 1
        if drawn({'success': True, 'data': {
                'viewport': {'width': 90, 'height': 200}, 'truncated': False,
                'nodes': documented['data']['nodes']}})['width'] != 90:
            failures += 1

    return {'cases': cases, 'passed': cases - failures, 'failed': failures}
