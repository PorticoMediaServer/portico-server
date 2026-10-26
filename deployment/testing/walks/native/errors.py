"""Closed failures for the native layout comparer.

Nothing here falls back to a guessed result: a missing tree, an unsupported
snapshot or a frame at another size stops the comparison with a fixed code.
"""

# The comparison itself cannot run.
BLOCKED_CODES = frozenset({
    'snapshot-missing',
    'snapshot-unreadable',
    'snapshot-unsupported-format',
    'snapshot-truncated',
    'snapshot-too-large',
    'snapshot-vega-unsupported',
    'snapshot-geometry-missing',
    'snapshot-dimensions-mismatch',
    'frame-missing',
    'frame-unreadable',
    'frame-schema-unsupported',
    'frame-geometry-missing',
    'frame-dimensions-mismatch',
    'frame-surface-mismatch',
    'surface-unsupported',
    'units-unsupported',
    'units-mismatch',
    # Settings, search and form frames: an editable control's own content is not read,
    # and a comparison without it would be a partial screen presented as a verdict.
    'editable-screen-unsupported',
})

# The snapshot is credential-bearing. Nothing in it is read or echoed.
REFUSED_CODES = frozenset({'credential-bearing-snapshot'})

# Geometry was measured; colours could not be compared for this reason.
COLOUR_CODES = frozenset({
    'screenshot-missing',
    'screenshot-unreadable',
    'screenshot-format-unsupported',
    'screenshot-size-mismatch',
    'screenshot-too-large',
    'screenshot-dimensions-unsupported',
    'no-solid-fill',
    'translucent-fill',
    'non-uniform-patch',
})


class BlockedEnv(Exception):
    """The walk setup is missing something, so no screen verdict is claimed."""

    def __init__(self, code):
        if code not in BLOCKED_CODES:
            raise AssertionError('unclosed blocked code: ' + str(code))
        super().__init__(code)
        self.code = code


class RefusedCredential(Exception):
    """A credential-bearing snapshot. It is refused whole, never summarised."""

    def __init__(self, code='credential-bearing-snapshot'):
        if code not in REFUSED_CODES:
            raise AssertionError('unclosed refused code: ' + str(code))
        super().__init__(code)
        self.code = code


class ColourUnavailable(Exception):
    """Colours were not compared; geometry stays valid."""

    def __init__(self, code):
        if code not in COLOUR_CODES:
            raise AssertionError('unclosed colour code: ' + str(code))
        super().__init__(code)
        self.code = code
