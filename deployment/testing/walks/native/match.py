"""Ordered alignment of the frame's elements against the drawn ones.

Pairing is explicit, in this order of precedence, and never guessed from content:

1. the frame element's `identifier`, when the walker mapped it to the app's own
   test identifier and the snapshot carries that identifier;
2. real data (`textMode: "data"`), which pairs by role family alone, in order, so
   a different title or poster is the same element and not a finding;
3. static copy, which pairs by role family and folded label.

A reordered control therefore shows as one missing and one extra element instead
of a false positional difference.
"""
import re

# The renderer's closed semantic set, plus the native type names a snapshot
# reports, folded into the families this comparer pairs on.
FAMILIES = {
    'button': 'button',
    'link': 'button',
    'tab': 'tab',
    'text': 'text',
    'heading': 'text',
    'label': 'text',
    'statictext': 'text',
    'header': 'text',
    'img': 'image',
    'image': 'image',
    'icon': 'image',
    'textbox': 'field',
    'searchbox': 'field',
    'combobox': 'field',
    'textfield': 'field',
    'edittext': 'field',
    'searchfield': 'field',
    'securetextfield': 'field',
    'checkbox': 'toggle',
    'radio': 'toggle',
    'switch': 'toggle',
    'toggle': 'toggle',
    'slider': 'slider',
    'adjustable': 'slider',
    'seekbar': 'slider',
    'navigation': 'group',
    'main': 'group',
    'list': 'group',
    'listitem': 'group',
    'table': 'group',
    'row': 'group',
    'cell': 'group',
    'webview': 'group',
    'other': 'other',
}


def family(role):
    key = re.sub(r'[^a-z]', '', str(role or '').lower())
    return FAMILIES.get(key, 'other')


def folded(label):
    return re.sub(r'\s+', ' ', str(label or '')).strip().casefold()


def is_data(element):
    return element.get('textMode') == 'data'


def _same(expected, drawn):
    if family(expected['role']) != family(drawn['role']):
        return False
    mapped = expected.get('identifier')
    if mapped is not None:
        return mapped == drawn.get('identifier')
    if is_data(expected):
        return True
    return folded(expected['label']) == folded(drawn['label'])


def align(expected, drawn):
    """Return ordered rows: ('pair', e, d), ('missing', e), ('extra', d)."""
    rows, columns = len(expected), len(drawn)
    table = [[0] * (columns + 1) for _ in range(rows + 1)]
    for i in range(rows - 1, -1, -1):
        for j in range(columns - 1, -1, -1):
            if _same(expected[i], drawn[j]):
                table[i][j] = table[i + 1][j + 1] + 1
            else:
                table[i][j] = max(table[i + 1][j], table[i][j + 1])
    result, i, j = [], 0, 0
    while i < rows and j < columns:
        if _same(expected[i], drawn[j]):
            result.append(('pair', i, j))
            i += 1
            j += 1
        elif table[i + 1][j] >= table[i][j + 1]:
            result.append(('missing', i))
            i += 1
        else:
            result.append(('extra', j))
            j += 1
    while i < rows:
        result.append(('missing', i))
        i += 1
    while j < columns:
        result.append(('extra', j))
        j += 1
    return result
