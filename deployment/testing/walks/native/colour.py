"""Colour sampling from the screenshot, for flat opaque fills only.

Text colour is never compared: anti-aliasing and glyph coverage make a sampled
value meaningless, and WALKS.md forbids treating rasterisation as a defect. A
translucent fill, a gradient, or a patch that is not uniform is skipped with a
closed note rather than judged.

The declared side accepts what a browser's getComputedStyle returns (`rgb(...)`,
`rgba(...)`), a hex colour, or an explicit `normalizedRGB` from the renderer.
"""
import re
import struct
import zlib

from .errors import ColourUnavailable

MAX_FILE_BYTES = 32 * 1024 * 1024
MAX_SIDE = 8192
MAX_PIXELS = 20_000_000
_COLOUR = re.compile(r'^rgba?\(([^)]*)\)$')


def read_png(path):
    """Decode an 8-bit RGB/RGBA non-interlaced PNG into rows of RGB bytes, bounded first."""
    try:
        size = path.stat().st_size
        data = path.read_bytes()
    except OSError:
        raise ColourUnavailable('screenshot-unreadable')
    if size > MAX_FILE_BYTES:
        raise ColourUnavailable('screenshot-too-large')
    if not data.startswith(b'\x89PNG\r\n\x1a\n'):
        raise ColourUnavailable('screenshot-format-unsupported')
    offset, width, height, depth, colour, interlace = 8, None, None, None, None, None
    payload = bytearray()
    while offset + 8 <= len(data):
        length, kind = struct.unpack('>I4s', data[offset:offset + 8])
        offset += 8
        body = data[offset:offset + length]
        offset += length + 4
        if kind == b'IHDR':
            width, height, depth, colour, _, _, interlace = struct.unpack('>IIBBBBB', body)
        elif kind == b'IDAT':
            payload += body
        elif kind == b'IEND':
            break
    if depth != 8 or colour not in (2, 6) or interlace not in (0, None):
        raise ColourUnavailable('screenshot-format-unsupported')
    if width is None or height is None or width <= 0 or height <= 0:
        raise ColourUnavailable('screenshot-format-unsupported')
    if width > MAX_SIDE or height > MAX_SIDE or width * height > MAX_PIXELS:
        raise ColourUnavailable('screenshot-dimensions-unsupported')
    channels = 3 if colour == 2 else 4
    stride = width * channels
    expected = height * (stride + 1)
    inflater = zlib.decompressobj()
    try:
        raw = inflater.decompress(bytes(payload), expected)
        if len(raw) < expected:
            raw += inflater.flush()
    except zlib.error:
        raise ColourUnavailable('screenshot-unreadable')
    if len(raw) < expected:
        raise ColourUnavailable('screenshot-unreadable')
    rows, previous, cursor = [], bytearray(stride), 0
    for _ in range(height):
        filter_type = raw[cursor]
        line = bytearray(raw[cursor + 1:cursor + 1 + stride])
        cursor += stride + 1
        _unfilter(filter_type, line, previous, channels)
        rows.append(bytes(line))
        previous = line
    return width, height, channels, rows


def _unfilter(filter_type, line, previous, channels):
    if filter_type == 0:
        return
    if filter_type == 1:
        for index in range(channels, len(line)):
            line[index] = (line[index] + line[index - channels]) & 0xFF
        return
    if filter_type == 2:
        for index in range(len(line)):
            line[index] = (line[index] + previous[index]) & 0xFF
        return
    if filter_type == 3:
        for index in range(len(line)):
            left = line[index - channels] if index >= channels else 0
            line[index] = (line[index] + ((left + previous[index]) >> 1)) & 0xFF
        return
    if filter_type == 4:
        for index in range(len(line)):
            left = line[index - channels] if index >= channels else 0
            above = previous[index]
            corner = previous[index - channels] if index >= channels else 0
            estimate = left + above - corner
            # PNG Paeth: the predictor closest to the estimate, ties going to
            # left, then above, then upper-left — in that order, by specification.
            if abs(estimate - left) <= abs(estimate - above) and abs(estimate - left) <= abs(estimate - corner):
                line[index] = (line[index] + left) & 0xFF
            elif abs(estimate - above) <= abs(estimate - corner):
                line[index] = (line[index] + above) & 0xFF
            else:
                line[index] = (line[index] + corner) & 0xFF
        return
    raise ColourUnavailable('screenshot-format-unsupported')


def _points(box, scale, inset=0.3):
    x = (box[0] + box[2] * inset) * scale
    y = (box[1] + box[3] * inset) * scale
    right = (box[0] + box[2] * (1 - inset)) * scale
    bottom = (box[1] + box[3] * (1 - inset)) * scale
    for step_y in range(3):
        for step_x in range(3):
            yield int(x + (right - x) * step_x / 2), int(y + (bottom - y) * step_y / 2)


def _hex(rgb):
    return '#%02x%02x%02x' % tuple(rgb)


def _channel(text):
    text = text.strip()
    if text.endswith('%'):
        try:
            return round(max(0.0, min(100.0, float(text[:-1]))) * 255 / 100)
        except ValueError:
            return None
    try:
        return max(0, min(255, round(float(text))))
    except ValueError:
        return None


def _alpha(text):
    text = text.strip()
    if not text:
        return 1.0
    try:
        return max(0.0, min(1.0, float(text[:-1]) / 100 if text.endswith('%') else float(text)))
    except ValueError:
        return None


def parse_colour(value):
    """(rgb, reason) from a computed CSS colour; reason explains why there is none."""
    text = str(value or '').strip()
    if not text:
        return None, 'no-solid-fill'
    lowered = text.lower()
    if lowered in ('transparent', 'none', 'inherit', 'initial', 'unset', 'currentcolor'):
        return None, 'no-solid-fill'
    if lowered.startswith('#'):
        digits = lowered[1:]
        if len(digits) in (3, 4):
            digits = ''.join(character * 2 for character in digits)
        if len(digits) not in (6, 8):
            return None, 'no-solid-fill'
        try:
            channels = [int(digits[index:index + 2], 16) for index in range(0, 6, 2)]
            alpha = int(digits[6:8], 16) / 255 if len(digits) == 8 else 1.0
        except ValueError:
            return None, 'no-solid-fill'
        if alpha < 1.0:
            return None, 'translucent-fill'
        return channels, None
    match = _COLOUR.match(lowered)
    if not match:
        # Gradients, url(), color-mix(), var() and anything else are not a flat fill.
        return None, 'no-solid-fill'
    parts = [part for part in re.split(r'[\s,/]+', match.group(1).strip()) if part]
    if len(parts) < 3:
        return None, 'no-solid-fill'
    channels = [_channel(part) for part in parts[:3]]
    if any(channel is None for channel in channels):
        return None, 'no-solid-fill'
    alpha = _alpha(parts[3]) if len(parts) > 3 else 1.0
    if alpha is None:
        return None, 'no-solid-fill'
    if alpha < 1.0:
        return None, 'translucent-fill'
    return channels, None


def declared(value, normalised=None):
    """(rgb, reason) for the frame's declared fill, preferring an explicit normalisation."""
    if isinstance(normalised, (list, tuple)) and len(normalised) == 3:
        return [int(channel) for channel in normalised], None
    return parse_colour(value)


def sample(png, box, scale, tolerance):
    """Median RGB of a small inset grid, or ColourUnavailable for a non-flat patch."""
    width, height, channels, rows = png
    samples = []
    for x, y in _points(box, scale):
        if 0 <= x < width and 0 <= y < height:
            row = rows[y]
            samples.append((row[x * channels], row[x * channels + 1], row[x * channels + 2]))
    if len(samples) < 5:
        raise ColourUnavailable('screenshot-size-mismatch')
    for channel in range(3):
        spread = max(item[channel] for item in samples) - min(item[channel] for item in samples)
        if spread > max(8, tolerance):
            raise ColourUnavailable('non-uniform-patch')
    ordered = sorted(samples, key=lambda item: item[0] + item[1] + item[2])
    return ordered[len(ordered) // 2]


def compare(expected_rgb, actual_rgb, tolerance):
    """(state, change) for a declared opaque fill against the sampled one."""
    if expected_rgb is None or actual_rgb is None:
        return None, None
    change = [actual_rgb[index] - expected_rgb[index] for index in range(3)]
    state = 'differs' if max(abs(item) for item in change) > tolerance else 'same'
    return state, {'dr': change[0], 'dg': change[1], 'db': change[2],
                   'expected': _hex(expected_rgb), 'actual': _hex(actual_rgb)}
