#!/usr/bin/env python3
"""Owner metadata plan/apply for the magazine PDF, on the real repair contract.

The magazine saved group needs one explicit owner `publicationType=magazine` and a
real series on the exact PDF item. The correct API is the generic metadata repair
surface: `GET /v1/metadata/item/{id}` returns a `RepairState` (revision plus the
field schema), and `POST /v1/metadata/item/{id}` takes a `RepairCommand`
(`action: "edit"`, `expectedRevision` and per-field edits). For ebook/comic items
the published schema in `metadata/repair_schema.go` already declares
`publicationType` (enum book/magazine/newspaper) plus `series`.

This corrects an earlier helper that read the legacy `/v1/items/{id}/metadata/manual`
title/description API and wrongly concluded the fields were missing.

`--item` is required and must resolve to exactly one item in the given library: the
caller names the intended original fixture, so the first PDF in a library is never
guessed. `--series` is the explicit owner fixture intent taken from the fixture's
private manifest or ordinary setup spec; a PDF's embedded metadata cannot guarantee
a series, so no title is ever parsed or invented. If the item already reports a
different series the plan refuses rather than silently overriding it.

`--apply` performs exactly one CAS edit with `locked: true` on the fields, using the
revision read a moment earlier, then reads the state once more to distinguish an
acknowledged edit from a confirmed one: confirmation requires that same returned
revision, original server/viewer/item/library and the exact locked owner fields.
A weak or failed readback leaves
`acknowledged: true` and `readback: unconfirmed`, and is never retried. Every call
is fenced against the original admitted runtime before it runs and after a read; a
mutation is not post-asserted. Closed output only: no token, URL, title, id,
revision value or author is printed.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from api_smoke import ContextError
from api_smoke.client import Client, Deadline
from api_smoke.context import admitted_origin, load
from api_smoke.custody import verify

PUBLICATION_TYPES = frozenset({'book', 'magazine', 'newspaper'})
SOURCES = frozenset({'owner', 'file-metadata'})
NEEDED = ('publicationType', 'series')
REVISION = re.compile(r'[0-9a-f]{64}\Z')


class Fenced:
    """Custody around every call, without post-asserting an acknowledged edit."""
    READS = frozenset({'GET', 'HEAD', 'OPTIONS'})

    def __init__(self, client, context, custody=verify):
        self.client = client
        self.context = context
        self.custody = custody

    def call(self, operation, method, path, **kwargs):
        self.custody(self.context)
        reply = self.client.call(operation, method, path, **kwargs)
        if method in self.READS:
            self.custody(self.context)
        return reply


class PlanError(Exception):
    def __init__(self, reason):
        super().__init__(reason)
        self.reason = reason


def _class(value, admitted):
    return value if isinstance(value, str) and value in admitted else 'unknown'


def _item_facts(item):
    if not isinstance(item, dict):
        raise PlanError('item-shape')
    publication = item.get('publicationType')
    publication = publication if isinstance(publication, dict) else {}
    series = item.get('series')
    series = series if isinstance(series, dict) else {}
    return {
        'format': item.get('format') if item.get('format') in ('pdf', 'epub', 'cbz', 'cbr', 'cb7') else 'other',
        'inventoryAvailable': item.get('inventoryAvailable') is True,
        'publicationType': _class(publication.get('value'), PUBLICATION_TYPES),
        'publicationSource': _class(publication.get('source'), SOURCES),
        'seriesPresent': isinstance(series.get('value'), str) and bool(series.get('value')),
        'seriesSource': _class(series.get('source'), SOURCES),
    }


class Plan:
    def __init__(self, client, library, item=None, series=None):
        self.client = client
        self.library = library
        self.item = item
        self.series = series
        self.series_fact = None
        self.scope = None
        self._revision = None
        self.document = {'helper': 'reading-metadata-plan', 'routes': [], 'item': None,
                         'repair': None, 'edit': None, 'result': 'refused',
                         'reason': None, 'apply': False}

    def _call_quiet(self, operation, method, path, **kwargs):
        try:
            return self.client.call(operation, method, path, **kwargs)
        except Exception:
            return None

    def _call(self, operation, method, path, **kwargs):
        reply = self.client.call(operation, method, path, **kwargs)
        if reply.failure is not None:
            raise PlanError('transport-' + reply.failure)
        return reply

    def execute(self, apply=False):
        self.document['apply'] = bool(apply)
        try:
            item = self._item()
            self.document['item'] = _item_facts(item)
            fact = item.get('series')
            if isinstance(fact, dict) and isinstance(fact.get('value'), str):
                self.series_fact = fact['value'] or None
            repair = self._repair_state()
            self.document['repair'] = repair
            reason = self._gate(repair)
            if reason:
                self.document['reason'] = reason
                return self.document
            if not apply:
                self.document['result'] = 'ready'
                return self.document
            self._apply(repair)
            self.document['result'] = 'confirmed' if self.document['edit']['readback'] == 'confirmed' else 'acknowledged'
        except PlanError as error:
            self.document['reason'] = error.reason
            self.document['result'] = 'refused'
        except Exception:
            self.document['reason'] = 'unexpected-shape'
            self.document['result'] = 'acknowledged' if self.document['edit'] else 'refused'
        return self.document

    def _item(self):
        reply = self._call('reading-items', 'GET', '/v1/reading/items',
                           query={'libraryId': self.library, 'limit': 100})
        if reply.status != 200 or not isinstance(reply.data, dict):
            raise PlanError('items-' + (reply.code or 'shape'))
        items = reply.data.get('items')
        if not isinstance(items, list):
            raise PlanError('items-shape')
        matches = [candidate for candidate in items[:100]
                   if isinstance(candidate, dict) and candidate.get('id') == self.item]
        if len(matches) != 1:
            raise PlanError('item-not-unique' if matches else 'item-absent')
        chosen = matches[0]
        if chosen.get('libraryId') != self.library:
            raise PlanError('item-library-mismatch')
        if chosen.get('inventoryAvailable') is not True or not isinstance(chosen.get('sourceId'), str) or not chosen['sourceId']:
            raise PlanError('source-unavailable')
        self.document['routes'].append({'operation': 'reading-items', 'status': reply.status})
        return chosen

    def _repair_state(self):
        reply = self._call('metadata-state', 'GET', '/v1/metadata/item/' + self.item)
        if reply.status == 404:
            raise PlanError('item-absent')
        if reply.status != 200 or not isinstance(reply.data, dict):
            raise PlanError('state-' + (reply.code or 'shape'))
        data = reply.data
        if not self._state_scope(data):
            raise PlanError('state-scope-mismatch')
        self.scope = (data['serverId'], data['viewerFence'])
        revision = data.get('revision')
        schema = data.get('schema')
        if not isinstance(revision, str) or not REVISION.fullmatch(revision):
            raise PlanError('revision-invalid')
        if not isinstance(schema, list) or len(schema) > 64:
            raise PlanError('schema-shape')
        specs = {}
        for spec in schema:
            if not isinstance(spec, dict) or not isinstance(spec.get('field'), str) or spec['field'] in specs:
                raise PlanError('schema-shape')
            specs[spec['field']] = spec
        if 'series' in specs and specs['series'].get('type') != 'text':
            raise PlanError('series-schema-invalid')
        if 'publicationType' in specs and specs['publicationType'].get('type') != 'enum':
            raise PlanError('publication-schema-invalid')
        fields = sorted(spec.get('field') for spec in schema[:64]
                        if isinstance(spec, dict) and isinstance(spec.get('field'), str))
        publication = next((spec for spec in schema[:64]
                            if isinstance(spec, dict) and spec.get('field') == 'publicationType'), None)
        allowed = publication.get('allowed') if isinstance(publication, dict) else None
        if publication is not None and (not isinstance(allowed, list) or not all(isinstance(v, str) for v in allowed)):
            raise PlanError('publication-schema-invalid')
        self._revision = revision
        self.document['routes'].append({'operation': 'metadata-state', 'status': reply.status})
        return {
            'revisionClass': 'present',
            'hasPublicationType': 'publicationType' in fields,
            'hasSeries': 'series' in fields,
            'publicationAllowed': sorted(value for value in (allowed or [])
                                         if isinstance(value, str) and value in PUBLICATION_TYPES),
        }

    def _gate(self, repair):
        facts = self.document['item']
        fact_series = self.series_fact
        if facts['format'] != 'pdf':
            return 'not-a-pdf-item'
        if (facts['publicationType'] == 'magazine' and facts['publicationSource'] == 'owner'
                and self.series and fact_series == self.series):
            return 'already-owner'
        if not repair['hasPublicationType'] or not repair['hasSeries']:
            return 'producer-unsupported'
        if 'magazine' not in repair['publicationAllowed']:
            return 'publication-type-not-allowed'
        if not isinstance(self.series, str) or not self.series.strip() or len(self.series)>4096 or any(ord(c)<32 for c in self.series):
            return 'series-intent-required'
        if fact_series and fact_series != self.series:
            return 'series-conflicts-with-item'
        return None

    def _apply(self, repair):
        command = {
            'action': 'edit',
            'expectedRevision': self._revision,
            'fields': {
                'publicationType': {'value': 'magazine', 'locked': True},
                'series': {'value': self.series, 'locked': True},
            },
        }
        reply = self.client.call('metadata-edit', 'POST', '/v1/metadata/item/' + self.item, body=command)
        if reply.status == 200:
            # Keep a known accepting reply even if its body/readback is unusable.
            self.document['edit'] = {'acknowledged': True, 'status': 200,
                                     'revisionChanged': False, 'readback': 'unconfirmed'}
        if reply.failure is not None:
            if self.document['edit']:
                return
            raise PlanError('transport-' + reply.failure)
        if reply.status == 404:
            raise PlanError('item-absent')
        if reply.status == 409:
            raise PlanError('revision-conflict')
        if reply.status != 200:
            raise PlanError('edit-' + (reply.code or 'shape'))
        before = self._revision
        next_revision = reply.data.get('revision') if isinstance(reply.data, dict) else None
        edit = self.document['edit']
        self.document['routes'].append({'operation': 'metadata-edit', 'status': reply.status})
        # A fresh read distinguishes an acknowledged edit from a confirmed one. A
        # failed readback never erases the acknowledgement and never retries.
        try:
            readback = self._call_quiet('metadata-state', 'GET', '/v1/metadata/item/' + self.item)
        except PlanError:
            readback = None
        if readback is not None and readback.failure is None and readback.status == 200 and isinstance(readback.data, dict):
            after = readback.data.get('revision')
            if (isinstance(after, str) and REVISION.fullmatch(after) and after != before
                    and after == next_revision and self._state_scope(readback.data)
                    and (readback.data['serverId'],readback.data['viewerFence']) == self.scope
                    and self._intended_fields(readback.data)):
                edit['readback'] = 'confirmed'
                edit['revisionChanged'] = True
        self.document['edit'] = edit

    def _state_scope(self, data):
        return (isinstance(data, dict) and data.get('target') == {'kind':'item','id':self.item}
                and data.get('libraryId') == self.library
                and isinstance(data.get('serverId'), str) and bool(data['serverId'])
                and isinstance(data.get('viewerFence'), str) and bool(data['viewerFence']))

    def _intended_fields(self, data):
        snapshot = data.get('snapshot')
        fields = snapshot.get('fields') if isinstance(snapshot, dict) else None
        if not isinstance(fields, dict):
            return False
        for key, value in (('publicationType','magazine'),('series',self.series)):
            fact = fields.get(key)
            if not isinstance(fact, dict) or fact.get('value') != value or fact.get('locked') is not True or fact.get('source') != 'owner':
                return False
        return True



def run(context_path, origin, run_id, library, item=None, series=None, apply=False, budget_seconds=60):
    if not 15 <= budget_seconds <= 120:
        raise ContextError('budget-not-admitted')
    context = load(context_path)
    if context.origin != origin:
        raise ContextError('origin-mismatch')
    if context.run_id != run_id:
        raise ContextError('run-id-mismatch')
    verify(context)
    deadline = Deadline(budget_seconds)
    plan = Plan(Fenced(Client(context, deadline), context), library, item, series)
    document = plan.execute(apply=apply)
    document['runId'] = context.run_id
    return document


def write_receipt(path, document):
    """Same-directory atomic replace with 0600; no partial receipt is left."""
    directory = path.parent
    descriptor, temporary = tempfile.mkstemp(prefix='.metadata-receipt-', dir=str(directory))
    try:
        os.fchmod(descriptor, 0o600)
        with os.fdopen(descriptor, 'w') as stream:
            json.dump(document, stream, indent=2)
            stream.write('\n')
        os.replace(temporary, path)
    except Exception:
        try:
            os.unlink(temporary)
        except OSError:
            pass
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--context', required=True, type=Path)
    parser.add_argument('--origin', required=True)
    parser.add_argument('--run-id', required=True)
    parser.add_argument('--library', required=True)
    parser.add_argument('--item', required=True)
    parser.add_argument('--series', required=True)
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--output', default='-')
    args = parser.parse_args()
    os.umask(0o077)
    try:
        admitted_origin(args.origin)
        document = run(args.context, args.origin, args.run_id, args.library, args.item,
                       args.series, args.apply)
    except ContextError as error:
        print(json.dumps({'error': str(error)}))
        return 2
    blob = json.dumps(document, indent=2) + '\n'
    if args.output == '-':
        sys.stdout.write(blob)
    else:
        try:
            write_receipt(Path(args.output), document)
        except OSError:
            print(json.dumps({'error': 'output-unavailable'}))
            return 3
    return 0 if document['result'] in ('ready', 'confirmed') else 1


if __name__ == '__main__':
    raise SystemExit(main())
