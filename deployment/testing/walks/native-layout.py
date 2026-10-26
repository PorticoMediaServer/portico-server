#!/usr/bin/env python3
"""Native layout comparer for feature walks: the frame's code against the drawn tree.

Inputs are supplied by reference and read from disk only: an agent-device
snapshot, its screenshot, and the render-frame document for the same frame at the
same device size. Nothing is installed, driven or captured here.

    native-layout.py --surface ios-phone --snapshot tree.json --frame frame.json \
        --device-size 393x852 --snapshot-units points \
        [--screenshot shot.png] [--out receipt.json] [--json]

The frame document is the mock-up's measured truth (render-frame output),
agreed with the frame renderer:

    {
      "tool": "render-frame",
      "frame": "auth#4",
      "surface": "ios-phone",
      "source": {"file": "auth.html", "label": "4"},
      "device": {"width": 393, "height": 852},
      "units": "points",                       // iOS/tvOS points, Android/web CSS pixels
      "target": {"width": 393, "height": 852}, // the caller's request; audit only
      "dataMapping": "explicit",               // audit only: the renderer always maps
      "elements": [
        {"selector": ".btn.play", "label": "Continue", "role": "button",
         "box": [24, 700, 345, 48],
         "style": {"backgroundColor": "rgb(31, 111, 235)", "color": "rgb(255, 255, 255)",
                   "fontSize": "17px", "fontWeight": "600", "borderRadius": "14px",
                   "backgroundImage": "none",
                   "normalizedRGB": {"backgroundColor": [31, 111, 235, 1],
                                     "color": [255, 255, 255, 1]}},
         "textMode": "static",                 // "data" for real titles, posters, counts, identities
         "identifier": "watch-title-play",     // optional: the app's own test identifier
         "normalizedRGB": [31, 111, 235]},     // optional: exact opaque background, wins over parsing
        {"selector": ".row.card .title", "label": "12 Angry Men", "role": "heading",
         "box": [16, 96, 160, 24], "style": {"color": "rgb(255, 255, 255)"}, "textMode": "data"}
      ]
    }

Roles are the renderer's closed set (button, link, img, heading, textbox, searchbox,
checkbox, radio, slider, combobox, navigation, main, list, listitem, table, row,
cell, text, other) and are folded to a family for pairing, next to the native type
names a snapshot reports. Style uses getComputedStyle names: backgroundColor and
color rather than background. A non-"none" backgroundImage, or an alpha below 1,
is skipped rather than judged.

Real data (`textMode: "data"`) is compared for geometry and role only: its text is
never compared, and another real title or a longer list is never a missing or extra
element. `identifier` maps a frame element to the app's own test identifier, so a
control can pair even when its drawn copy differs.

Every difference carries the citation WALKS.md §2b requires: the frame file and
label, the element's selector, and the frame's expected values next to what the
app drew. Values a snapshot cannot carry (radius, type, opacity, shadow, blur)
are listed as not measured, never implied to match.

Screens with an editable control (Settings, search, forms) are BLOCKED-ENV, not
compared: an editable field's own content is not read, and a screen measured
without it would be presented as if it had been compared. A password field is
refused outright.

Screenshots are bounded before allocation (32 MiB file, 8192 px a side, 20M
pixels); an oversized one keeps the geometry comparison and reports the closed
colour note instead.

Outcomes, with closed codes only:

    MEASURED            geometry, order, text state and flat fills compared
    BLOCKED-ENV         the walk setup is missing something; no screen verdict
    REFUSED-CREDENTIAL  credential-bearing snapshot; nothing in it is read/echoed

Exit status: 0 MEASURED, 2 BLOCKED-ENV, 3 REFUSED-CREDENTIAL, 64 usage.
"""
import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from native import colour as colour_module  # noqa: E402
from native import compare as compare_module  # noqa: E402
from native import frame as frame_module  # noqa: E402
from native import selfcheck as selfcheck_module  # noqa: E402
from native import snapshot as snapshot_module  # noqa: E402
from native.errors import BlockedEnv, ColourUnavailable, RefusedCredential  # noqa: E402

SURFACES = ('ios-phone', 'ipad', 'android-phone', 'tvos', 'android-tv', 'fire-os', 'vega',
            'web-desktop', 'web-phone')


def _size(text):
    try:
        width, height = (int(part) for part in text.lower().replace('×', 'x').split('x'))
    except (TypeError, ValueError):
        raise argparse.ArgumentTypeError('expected WIDTHxHEIGHT')
    if width <= 0 or height <= 0:
        raise argparse.ArgumentTypeError('expected positive WIDTHxHEIGHT')
    return width, height


def parse(argv):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('--surface', choices=SURFACES, help='walk surface this screen belongs to')
    parser.add_argument('--snapshot', type=Path, help='agent-device snapshot -i --json output')
    parser.add_argument('--frame', type=Path, dest='frame_path', help='render-frame document for this frame')
    parser.add_argument('--device-size', type=_size, help='device size the frame was rendered at, e.g. 393x852')
    parser.add_argument('--snapshot-units', choices=('points', 'pixels'),
                        help='units the snapshot reports; must match the frame document')
    parser.add_argument('--screenshot', type=Path, help='screenshot of the same screen, for flat fills')
    parser.add_argument('--position-tolerance', type=float, default=2.0, help='points (default 2)')
    parser.add_argument('--size-tolerance', type=float, default=2.0, help='points (default 2)')
    parser.add_argument('--colour-tolerance', type=int, default=12, help='per-channel 0-255 (default 12)')
    parser.add_argument('--out', type=Path, help='write the receipt JSON here instead of stdout only')
    parser.add_argument('--json', action='store_true', help='print the receipt JSON only')
    parser.add_argument('--self-check', action='store_true', help='synthetic unit validation of this tool')
    args = parser.parse_args(argv)
    if args.self_check:
        return args
    for name in ('surface', 'snapshot', 'frame_path', 'device_size', 'snapshot_units'):
        if getattr(args, name) in (None, ''):
            parser.error('--' + name.replace('_path', '').replace('_', '-') + ' is required')
    return args


def _png(args):
    """Decode the screenshot, or return the closed reason colours were not sampled."""
    if args.screenshot is None:
        return None, 'screenshot-missing'
    if not args.screenshot.is_file():
        return None, 'screenshot-missing'
    try:
        return colour_module.read_png(args.screenshot), None
    except ColourUnavailable as unavailable:
        return None, unavailable.code


def measure(args):
    # Vega snapshots are unsupported today (MANUAL §7), and web has its own
    # extractor; neither may fall through to a guessed comparison.
    if args.surface == 'vega':
        raise BlockedEnv('snapshot-vega-unsupported')
    if args.surface.startswith('web-'):
        raise BlockedEnv('surface-unsupported')
    frame = frame_module.load(args.frame_path)
    if frame['units'] != args.snapshot_units:
        raise BlockedEnv('units-mismatch')
    if frame['surface'] and frame['surface'] != args.surface:
        raise BlockedEnv('frame-surface-mismatch')
    if [frame['device']['width'], frame['device']['height']] != [args.device_size[0], args.device_size[1]]:
        raise BlockedEnv('frame-dimensions-mismatch')
    snapshot = snapshot_module.load(args.snapshot, args.surface)
    if snapshot['width'] is not None and int(snapshot['width']) != args.device_size[0]:
        raise BlockedEnv('snapshot-dimensions-mismatch')
    if snapshot['height'] is not None and int(snapshot['height']) != args.device_size[1]:
        raise BlockedEnv('snapshot-dimensions-mismatch')
    png, colour_blocked = _png(args)
    options = {'surface': args.surface,
               'position_tolerance': args.position_tolerance,
               'size_tolerance': args.size_tolerance,
               'colour_tolerance': args.colour_tolerance}
    return compare_module.compare(frame, snapshot, options, png, colour_blocked)


def summarise(receipt):
    counts = receipt['counts']
    lines = ['native-layout %s %s · %s · expected %d drawn %d matched %d · differences %d'
             % (receipt['surface'], receipt['frame']['id'], receipt['outcome'],
                counts['expected'], counts['drawn'], counts['matched'], len(receipt['differences']))]
    for item in receipt['differences']:
        lines.append('- %s %s' % (item['kind'], item['line']))
    if receipt['colour']['notes']:
        lines.append('colour not compared: ' + ', '.join(receipt['colour']['notes']))
    return '\n'.join(lines)


def emit(receipt, args):
    text = json.dumps(receipt, indent=2, sort_keys=True)
    if args.out:
        args.out.write_text(text + '\n')
    print(text if args.json else summarise(receipt))
    return 0


def main(argv):
    args = parse(argv)
    if args.self_check:
        result = selfcheck_module.run()
        print('PORTICO_NATIVE_LAYOUT_SELFCHECK ' + json.dumps(result, sort_keys=True))
        return 0 if result['failed'] == 0 else 1
    try:
        return emit(measure(args), args)
    except RefusedCredential as refused:
        payload = {'tool': 'native-layout', 'version': 1, 'surface': args.surface,
                   'outcome': 'REFUSED-CREDENTIAL', 'reason': refused.code}
        print(json.dumps(payload, sort_keys=True))
        print('native-layout: credential-bearing snapshot refused; nothing read or echoed', file=sys.stderr)
        return 3
    except BlockedEnv as blocked:
        payload = {'tool': 'native-layout', 'version': 1, 'surface': args.surface,
                   'outcome': 'BLOCKED-ENV', 'reason': blocked.code}
        print(json.dumps(payload, sort_keys=True))
        print('native-layout: BLOCKED-ENV ' + blocked.code, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
