"""Compare a drawn tree with the frame's measured truth, citing every difference.

The frame decides what is right; the snapshot only detects. Nothing here reads a
screenshot impression, judges type rasterisation, blur, motion or real data, or
claims a style value the snapshot does not carry.

Real data (`textMode: "data"`) pairs by role family and is compared for geometry
only: its text is never compared, and it is never reported as a missing or extra
element because the frame's sample content differs.
"""
from . import cite, colour as colour_module, geometry, match

ORDER = {'missing-control': 0, 'missing-text': 1, 'extra-element': 2, 'text': 3,
         'size': 4, 'position': 5, 'colour': 6}


def _label_state(element, node):
    if match.is_data(element):
        return 'data'
    return 'same' if element['label'] == node['label'] else 'differs'


def _declared_fill(element):
    """The frame's flat opaque fill, or (None, reason) when it is not one."""
    style = element['style']
    image = str(style.get('backgroundImage') or 'none').strip().lower()
    if image not in ('none', 'initial', 'unset'):
        # gradient / image / other: not a flat fill, so no colour judgement.
        return None, 'no-solid-fill'
    normalised = element.get('normalizedRGB')
    if normalised is None:
        style_normalised = style.get('normalizedRGB')
        if isinstance(style_normalised, dict):
            flat = style_normalised.get('backgroundColor')
            if isinstance(flat, (list, tuple)) and len(flat) == 4:
                if flat[3] < 1:
                    return None, 'translucent-fill'
                normalised = list(flat[:3])
    return colour_module.declared(style.get('backgroundColor') or style.get('background'), normalised)


def _fill(element, node, png, scale, tolerance, notes):
    """Sampled fill for a flat, non-text element, or None with a closed note."""
    if png is None or node['box'] is None:
        return None
    if match.family(element['role']) in ('text', 'field'):
        return None
    declared, reason = _declared_fill(element)
    if declared is None:
        if reason and reason not in notes:
            notes.append(reason)
        return None
    try:
        sampled = colour_module.sample(png, node['box'], scale, tolerance)
    except colour_module.ColourUnavailable as unavailable:
        if unavailable.code not in notes:
            notes.append(unavailable.code)
        return None
    state, change = colour_module.compare(declared, sampled, tolerance)
    return change if state == 'differs' else None


def compare(frame, snapshot, options, png=None, colour_blocked=None):
    """Return the receipt payload for one screen."""
    position = float(options.get('position_tolerance', 2.0))
    size = float(options.get('size_tolerance', 2.0))
    tolerance = int(options.get('colour_tolerance', 12))
    scale = None
    if png is not None:
        width, height = png[0], png[1]
        scale = width / frame['device']['width']
        expected_height = frame['device']['height'] * scale
        if scale <= 0 or abs(expected_height - height) > max(2, height * 0.02):
            png, scale = None, None
            colour_blocked = 'screenshot-size-mismatch'
    notes = [colour_blocked] if colour_blocked else []
    differences, elements = [], []
    counts = {'expected': len(frame['elements']), 'drawn': len(snapshot['elements']), 'matched': 0,
              'missing': 0, 'extra': 0, 'moved': 0, 'resized': 0, 'text': 0, 'colour': 0,
              'data': 0, 'dataUnpaired': 0}
    sampled = 0
    data_families = {match.family(element['role']) for element in frame['elements'] if match.is_data(element)}

    for row in match.align(frame['elements'], snapshot['elements']):
        if row[0] == 'missing':
            element = frame['elements'][row[1]]
            if match.is_data(element):
                counts['dataUnpaired'] += 1
                continue
            kind = 'missing-control' if match.family(element['role']) in ('button', 'toggle', 'tab', 'slider') else 'missing-text'
            counts['missing'] += 1
            elements.append({'order': row[1], 'selector': element['selector'], 'status': kind})
            differences.append(cite.build(frame, element, None, 'missing', None, kind))
            continue
        if row[0] == 'extra':
            node = snapshot['elements'][row[1]]
            if match.family(node['role']) in data_families:
                # More (or other) real items than the frame sampled: not a finding.
                counts['dataUnpaired'] += 1
                continue
            counts['extra'] += 1
            elements.append({'order': None, 'selector': None, 'status': 'extra'})
            differences.append(cite.extra(frame, node))
            continue
        element, node = frame['elements'][row[1]], snapshot['elements'][row[2]]
        counts['matched'] += 1
        if match.is_data(element):
            counts['data'] += 1
        label_state = _label_state(element, node)
        fill = _fill(element, node, png, scale, tolerance, notes)
        if fill is not None:
            sampled += 1
        change = geometry.deltas(element['box'], node['box']) if node['box'] is not None else None
        state = geometry.status(change, position, size) if change is not None else 'no-geometry'
        elements.append({'order': element['index'], 'selector': element['selector'], 'status': state})
        if node['box'] is None:
            continue
        if label_state == 'differs':
            counts['text'] += 1
            differences.append(cite.build(frame, element, node, label_state, None, 'text'))
        if state in ('moved', 'moved-resized'):
            counts['moved'] += 1
            differences.append(cite.build(frame, element, node, label_state, None, 'position'))
        if state in ('resized', 'moved-resized'):
            counts['resized'] += 1
            differences.append(cite.build(frame, element, node, label_state, None, 'size'))
        if fill is not None:
            counts['colour'] += 1
            differences.append(cite.build(frame, element, node, label_state, fill.get('actual'), 'colour',
                                          expected_fill=fill.get('expected')))

    differences.sort(key=lambda item: (ORDER.get(item['kind'], 9), item['selector'] or ''))
    return {
        'tool': 'native-layout',
        'version': 2,
        'surface': frame.get('surface') or options.get('surface') or '',
        'frame': {'id': frame['frame'], 'file': frame['source']['file'], 'label': frame['source']['label']},
        'device': frame['device'],
        'units': frame['units'],
        'outcome': 'MEASURED',
        'reason': None,
        'counts': counts,
        'colour': {'sampled': sampled, 'notes': notes},
        'differences': differences,
        'elements': elements,
    }
