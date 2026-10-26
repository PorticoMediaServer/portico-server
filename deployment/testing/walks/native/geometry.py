"""Numeric geometry: the frame's box against the drawn box, in the same units."""


def rounded(value):
    return round(float(value), 1)


def deltas(expected, actual):
    return {
        'dx': rounded(actual[0] - expected[0]),
        'dy': rounded(actual[1] - expected[1]),
        'dw': rounded(actual[2] - expected[2]),
        'dh': rounded(actual[3] - expected[3]),
    }


def status(change, position_tolerance, size_tolerance):
    moved = abs(change['dx']) > position_tolerance or abs(change['dy']) > position_tolerance
    resized = abs(change['dw']) > size_tolerance or abs(change['dh']) > size_tolerance
    if moved and resized:
        return 'moved-resized'
    if moved:
        return 'moved'
    if resized:
        return 'resized'
    return 'same'
