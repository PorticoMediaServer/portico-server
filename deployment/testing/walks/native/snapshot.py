"""The agent-device snapshot: what the app actually drew, by bounded fields only.

The helper's field names vary by path and version, so the documented facts are
read explicitly rather than assumed: drawn text is `label` or `value` (today's T3
agent-device writes text nodes with `value` only), geometry is `rect` as an object
in device pixels, the semantic role is `kind` next to the Android class in `type`,
and the screen size is `viewport`. A snapshot with no recognisable geometry, or
none of those shapes, is blocked rather than converted.

Two guards, in this order:

* a password field means a credential screen; the snapshot is refused whole and
  nothing in it is read or echoed;
* any other editable control means a Settings, search or form frame. That is not
  a refusal and not a comparison either: it blocks with a closed code, because an
  editable field's own content is not read and a screen measured without it would
  be reported as if it had been compared.
"""
import json
from pathlib import Path

from .errors import BlockedEnv, RefusedCredential

GEOMETRY_KEYS = ('frame', 'rect', 'bounds', 'geometry')
LIMIT = 600


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


def _credential(node):
    return (node.get('password') is True
            or node.get('kind') == 'secure-text-field'
            or node.get('type') in ('SecureTextField', 'XCUIElementTypeSecureTextField'))


def _editable(node):
    return (node.get('editable') is True
            or node.get('kind') in ('text-field', 'search-field', 'secure-text-field')
            or node.get('type') in ('android.widget.EditText', 'TextField', 'SecureTextField', 'SearchField',
                                    'XCUIElementTypeTextField', 'XCUIElementTypeSecureTextField', 'XCUIElementTypeSearchField'))


def load(path, surface):
    """Load and validate an agent-device snapshot, or raise BlockedEnv/RefusedCredential."""
    if surface == 'vega':
        # MANUAL §7: Vega snapshots are not supported yet.
        raise BlockedEnv('snapshot-vega-unsupported')
    location = Path(path)
    if not location.is_file():
        raise BlockedEnv('snapshot-missing')
    try:
        doc = json.loads(location.read_text())
    except (OSError, ValueError):
        raise BlockedEnv('snapshot-unreadable')
    data = doc.get('data') if isinstance(doc, dict) else None
    if not isinstance(data, dict):
        raise BlockedEnv('snapshot-unsupported-format')
    nodes = data.get('nodes')
    if not isinstance(nodes, list) or not nodes:
        raise BlockedEnv('snapshot-unsupported-format')
    if data.get('truncated') is not False:
        raise BlockedEnv('snapshot-truncated')
    if len(nodes) > LIMIT:
        raise BlockedEnv('snapshot-too-large')
    if any(not isinstance(node, dict) for node in nodes):
        raise BlockedEnv('snapshot-unsupported-format')
    if any(_credential(node) for node in nodes):
        raise RefusedCredential()
    if any(_editable(node) for node in nodes):
        raise BlockedEnv('editable-screen-unsupported')
    elements = []
    for index, node in enumerate(nodes):
        box = None
        for key in GEOMETRY_KEYS:
            if key in node:
                box = _box(node[key])
                if box is not None:
                    break
        text = node.get('label')
        if not isinstance(text, str) or not text.strip():
            text = node.get('value')
        role = node.get('kind') or node.get('type') or 'other'
        elements.append({
            'index': index,
            'label': text if isinstance(text, str) else '',
            'role': str(role),
            'identifier': str(node.get('identifier') or ''),
            'box': box,
            'hittable': node.get('hittable') is True,
        })
    if not any(element['box'] is not None for element in elements):
        raise BlockedEnv('snapshot-geometry-missing')
    size = data.get('device') if isinstance(data.get('device'), dict) else {}
    viewport = data.get('viewport') if isinstance(data.get('viewport'), dict) else {}
    return {
        'elements': elements,
        'width': viewport.get('width', size.get('width')),
        'height': viewport.get('height', size.get('height')),
    }
