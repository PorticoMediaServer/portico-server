"""Bounded transcode-environment check for one admitted loopback runtime."""
from __future__ import annotations

from api_smoke.client import Client, Deadline
from api_smoke.context import ContextError, load
from api_smoke.custody import verify
from .check import Check, CheckFailure
from .media import LoopbackMedia

__all__ = ['Check', 'CheckFailure', 'ContextError', 'run']


class OriginalRuntimeClient:
    """Fence every request without erasing an acknowledged mutation's handle."""
    def __init__(self, context, deadline):
        self.context = context
        self.client = Client(context, deadline)

    def call(self, operation, method, path, **kwargs):
        verify(self.context)
        reply = self.client.call(operation, method, path, **kwargs)
        # The start caller captures a real 201/202 before its next read. A
        # postassert here must not discard that original cleanup obligation.
        if method in ('GET', 'HEAD'):
            verify(self.context)
        return reply


def run(context_path, origin, run_id, item, budget_seconds=90, ffprobe=None, scratch_root=None):
    """Admit the context, verify custody, then run the one bounded check."""
    if not 30 <= budget_seconds <= 300:
        raise ContextError('budget-not-admitted')
    context = load(context_path)
    if context.origin != origin:
        raise ContextError('origin-mismatch')
    if context.run_id != run_id:
        raise ContextError('run-id-mismatch')
    verify(context)
    deadline = Deadline(budget_seconds)
    media = LoopbackMedia(context, deadline, ffprobe=ffprobe, scratch_root=scratch_root, verify=verify)
    document = Check(OriginalRuntimeClient(context, deadline), deadline, context.owner, item, media=media).execute()
    document['runId'] = context.run_id
    document['budgetSeconds'] = budget_seconds
    return document
