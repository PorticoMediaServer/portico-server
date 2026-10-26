"""Bounded server API smoke for the isolated checkpoint runtime."""
from __future__ import annotations

from .client import Client, Deadline
from .context import ContextError, load
from .custody import verify
from .flows import Smoke

def run(context_path, origin, run_id, deadline_seconds=300):
    context = load(context_path)
    if context.origin != origin:
        raise ContextError('origin-mismatch')
    if context.run_id != run_id:
        raise ContextError('run-id-mismatch')
    verify(context)
    deadline = Deadline(deadline_seconds)
    return Smoke(Client(context, deadline), deadline, context.owner).execute()

__all__ = ['ContextError', 'run']
