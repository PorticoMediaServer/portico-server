"""Citations: every finding names the frame's code and both sides' values.

WALKS.md §2b: a finding cites the frame file, the frame label, the element's
selector, and the expected values taken from the mock-up's HTML and CSS, next to
what the app drew. Values the snapshot cannot measure are listed as such and are
never implied to match.
"""
from .geometry import rounded

# The renderer writes getComputedStyle names, so these are the names cited.
STYLE_KEYS = ('backgroundColor', 'background', 'backgroundImage', 'color', 'fontSize', 'fontWeight',
              'borderRadius', 'radius', 'lineHeight', 'letterSpacing', 'opacity', 'display')
UNMEASURABLE = ('backgroundColor', 'background', 'color', 'fontSize', 'fontWeight', 'borderRadius',
                'radius', 'lineHeight', 'letterSpacing', 'opacity', 'shadow', 'blur')


def expected(element):
    style = element['style']
    values = {
        'label': element['label'],
        'role': element['role'],
        'x': rounded(element['box'][0]),
        'y': rounded(element['box'][1]),
        'width': rounded(element['box'][2]),
        'height': rounded(element['box'][3]),
    }
    for key in STYLE_KEYS:
        if style.get(key) is not None:
            values[key] = style[key]
    return values


def actual(node, label_state, fill):
    """Only what the snapshot really carries: no invented style values."""
    values = {
        'x': rounded(node['box'][0]),
        'y': rounded(node['box'][1]),
        'width': rounded(node['box'][2]),
        'height': rounded(node['box'][3]),
        'role': node['role'],
        'labelState': label_state,
    }
    if fill is not None:
        values['backgroundColor'] = fill
    return values


def unmeasured(element):
    return [key for key in UNMEASURABLE if element['style'].get(key) is not None]


def _pair(label, values):
    return '%s %s' % (label, values)


def line(source, selector, expected_values, actual_values, unmeasured_keys, drawn=True):
    parts = []
    for key, label in (('label', 'label'), ('x', 'x'), ('y', 'y'), ('width', 'width'), ('height', 'height')):
        wanted, painted = expected_values.get(key), actual_values.get(key)
        if key in actual_values and wanted != painted:
            parts.append('%s expected %s actual %s' % (label, wanted, painted))
    if not drawn and 'label' in expected_values:
        parts.append('label expected %r; not drawn by the app' % expected_values['label'])
    elif actual_values.get('labelState') == 'data':
        parts.append('text not compared (frame marks it as real data)')
    elif actual_values.get('labelState') != 'same' and 'label' in expected_values:
        parts.append('label expected %r actual differs' % expected_values['label'])
    if 'backgroundColor' in actual_values:
        wanted = expected_values.get('backgroundColor', expected_values.get('background'))
        if wanted != actual_values['backgroundColor']:
            parts.append('backgroundColor expected %s actual %s'
                         % (wanted, actual_values['backgroundColor']))
    if unmeasured_keys:
        parts.append('not measured by this tool: ' + ', '.join(unmeasured_keys))
    head = '%s %s' % (source['file'], source['label'])
    if selector:
        head += ' · ' + selector
    return head + (' · ' + '; '.join(parts) if parts else '')


def build(frame, element, node, label_state, fill, kind, expected_fill=None):
    """The structured citation and its single-line form for a fix item.

    ``expected_fill`` is the value that was actually compared when the renderer's
    explicit normalization won over the parsed CSS string, so the citation shows
    the number the judgement rests on.
    """
    expected_values = expected(element)
    if expected_fill is not None:
        expected_values['backgroundColor'] = expected_fill
    actual_values = actual(node, label_state, fill) if node is not None else {}
    missing = unmeasured(element) if node is not None else UNMEASURABLE
    citation = {
        'kind': kind,
        'file': frame['source']['file'],
        'frame': frame['source']['label'],
        'selector': element['selector'] if element is not None else None,
        'identifier': element.get('identifier') if element is not None else None,
        'textMode': element.get('textMode') if element is not None else None,
        'expected': expected_values if element is not None else None,
        'actual': actual_values,
        'notMeasured': missing,
    }
    citation['line'] = line(frame['source'], citation['selector'], expected_values, actual_values, missing,
                             drawn=node is not None)
    return citation


def extra(frame, node):
    """A drawn element with no frame counterpart: cited to the frame, never to its text."""
    citation = {
        'kind': 'extra-element',
        'file': frame['source']['file'],
        'frame': frame['source']['label'],
        'selector': None,
        'expected': None,
        'actual': {'role': node['role'], 'index': node['index'], 'labelState': 'not-in-frame'},
        'notMeasured': [],
    }
    citation['line'] = '%s %s · extra drawn %s at index %d (no frame element)' % (
        frame['source']['file'], frame['source']['label'], node['role'], node['index'])
    return citation
