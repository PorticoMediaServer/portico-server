"""The rendered frame document: the mock-up's measured truth for one frame.

A finding has to cite the frame element it came from, so the document names its
mock-up file and frame label, every element carries a selector, and every element
declares whether its text is static copy or real data. A data element's text is
never compared, so a different real title can never become a finding.
"""
import json
from pathlib import Path

from .errors import BlockedEnv

UNITS = ('points', 'pixels')
TEXT_MODES = ('static', 'data')


def _box(value):
    if isinstance(value, dict):
        try:
            return [float(value['x']), float(value['y']), float(value['width']), float(value['height'])]
        except (KeyError, TypeError, ValueError):
            return None
    if isinstance(value, (list, tuple)) and len(value) == 4:
        try:
            return [float(item) for item in value]
        except (TypeError, ValueError):
            return None
    return None


def _rgb(value):
    """An explicit opaque normalisation: [r, g, b] or '#rrggbb'."""
    if isinstance(value, str):
        text = value.strip().lstrip('#')
        if len(text) != 6:
            return None
        try:
            return [int(text[index:index + 2], 16) for index in (0, 2, 4)]
        except ValueError:
            return None
    if isinstance(value, (list, tuple)) and len(value) == 3:
        if all(isinstance(item, int) and not isinstance(item, bool) and 0 <= item <= 255 for item in value):
            return list(value)
    return None


def load(path):
    """Load and validate a render-frame document, or raise BlockedEnv."""
    location = Path(path)
    if not location.is_file():
        raise BlockedEnv('frame-missing')
    try:
        doc = json.loads(location.read_text())
    except (OSError, ValueError):
        raise BlockedEnv('frame-unreadable')
    if not isinstance(doc, dict):
        raise BlockedEnv('frame-schema-unsupported')
    source = doc.get('source')
    if not isinstance(source, dict):
        raise BlockedEnv('frame-schema-unsupported')
    if not isinstance(source.get('file'), str) or not isinstance(source.get('label'), str):
        raise BlockedEnv('frame-schema-unsupported')
    units = doc.get('units')
    if units not in UNITS:
        raise BlockedEnv('units-unsupported')
    device = doc.get('device')
    if not isinstance(device, dict):
        raise BlockedEnv('frame-schema-unsupported')
    try:
        width, height = int(device['width']), int(device['height'])
    except (KeyError, TypeError, ValueError):
        raise BlockedEnv('frame-schema-unsupported')
    if width <= 0 or height <= 0:
        raise BlockedEnv('frame-schema-unsupported')
    raw = doc.get('elements')
    if not isinstance(raw, list) or not raw:
        raise BlockedEnv('frame-schema-unsupported')
    elements = []
    for index, item in enumerate(raw):
        if not isinstance(item, dict):
            raise BlockedEnv('frame-schema-unsupported')
        box = _box(item.get('box'))
        if box is None or box[2] <= 0 or box[3] <= 0:
            raise BlockedEnv('frame-geometry-missing')
        selector = item.get('selector')
        if not isinstance(selector, str) or not selector.strip():
            raise BlockedEnv('frame-schema-unsupported')
        text_mode = item.get('textMode', 'static')
        if text_mode not in TEXT_MODES:
            raise BlockedEnv('frame-schema-unsupported')
        label = item.get('label')
        style = item.get('style') if isinstance(item.get('style'), dict) else {}
        # Element-level normalizedRGB [r, g, b] is written only for a flat opaque
        # background and wins. style.normalizedRGB carries [r, g, b, a] per channel set.
        normalised = _rgb(item.get('normalizedRGB')) if 'normalizedRGB' in item else None
        style_normalised = style.get('normalizedRGB')
        if normalised is None and isinstance(style_normalised, dict):
            flat = style_normalised.get('backgroundColor')
            if isinstance(flat, (list, tuple)) and len(flat) == 4 and flat[3] == 1:
                if all(isinstance(value, int) and 0 <= value <= 255 for value in flat[:3]):
                    normalised = list(flat[:3])
        identifier = item.get('identifier')
        elements.append({
            'index': index,
            'selector': selector.strip(),
            'label': label if isinstance(label, str) else '',
            'role': str(item.get('role') or 'other'),
            'box': box,
            'style': style,
            'textMode': text_mode,
            'identifier': identifier.strip() if isinstance(identifier, str) and identifier.strip() else None,
            'normalizedRGB': normalised,
        })
    return {
        'frame': str(doc.get('frame') or source['label']),
        'surface': str(doc.get('surface') or ''),
        'source': {'file': source['file'], 'label': source['label']},
        'device': {'width': width, 'height': height},
        'units': units,
        'elements': elements,
    }
