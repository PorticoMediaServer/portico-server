#!/usr/bin/env python3
"""The machine device gate: no lease producer, so device paths refuse.

A checkpoint or standalone driver may touch an emulator only inside a live lease
produced by a real admission: the supported session open performed under the AVD
lock, with the exact returned session admitted. No such producer is wired yet, so
`require_lease()` always refuses and the callers stop before any device I/O - no
probe, boot, boot check, clock read, reverse, install, launch, log capture or
cleanup.

Nothing can enable this. A target is a description, and neither a plan, a field, a
target name, an AVD nor an object with an `open` attribute is admission. There is no
registration API on purpose: a real producer must be a separate, reviewed
implementation, and until one exists this gate always refuses.
"""
from __future__ import annotations

CODE = 'runtime-admission-unavailable'
REASON = 'no admitted device lease producer; machine device paths refuse before any device I/O'

class RuntimeUnavailable(Exception):
    """No admitted lease exists, so no device work may start."""

    def __init__(self, code=CODE, reason=REASON):
        super().__init__(code)
        self.code = code
        self.reason = reason


def require_lease():
    """Always refuse: no admitted lease producer is wired in this tree."""
    raise RuntimeUnavailable()


def refused():
    """Always true: machine device work stays unavailable until a reviewed producer."""
    return True


def unavailable_rows(surfaces):
    """Closed unavailable rows for every surface this gate blocks."""
    return [{'surface': surface, 'id': 'core-flows', 'status': 'unavailable',
             'dependency': 'admission-unavailable: ' + REASON} for surface in surfaces]
