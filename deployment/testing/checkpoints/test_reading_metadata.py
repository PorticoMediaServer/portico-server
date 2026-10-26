#!/usr/bin/env python3
"""Pure tests for the magazine metadata plan on the repair contract."""
import importlib.util
import sys
import unittest
from pathlib import Path

HERE = Path(__file__).parent
sys.path.insert(0, str(HERE))
from api_smoke.client import Reply

spec = importlib.util.spec_from_file_location('reading_metadata', HERE / 'reading-metadata.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

REV = 'b' * 64
PDF = {'id': 'item-1', 'libraryId': 'library-1', 'format': 'pdf', 'inventoryAvailable': True,
       'sourceId': 'source-1',
       'publicationType': {'value': 'book', 'source': 'file-metadata'},
       'series': {'value': 'Portico Walk Magazine', 'source': 'file-metadata'}}
STATE = {'target': {'kind':'item','id':'item-1'}, 'libraryId':'library-1',
         'serverId':'original-server','viewerFence':'original-viewer', 'revision': REV, 'schema': [
    {'field': 'series', 'type': 'text'},
    {'field': 'publicationType', 'type': 'enum', 'allowed': ['', 'book', 'magazine', 'newspaper']}]}
AFTER = dict(STATE, revision='c'*64, snapshot={'fields': {
    'publicationType': {'value':'magazine','locked':True,'source':'owner'},
    'series': {'value':'Portico Walk Magazine','locked':True,'source':'owner'}}})


class FakeClient:
    def __init__(self, items, state=STATE, edit=200, readback_state=None, fail=None):
        self.items = items
        self.state = state
        self.state_after = readback_state
        self.edit = edit
        self.reads = 0
        self.calls = []
        self.bodies = []
        self.fail = fail or set()

    def call(self, operation, method, path, **kwargs):
        self.calls.append((operation, method))
        if operation in self.fail:
            raise RuntimeError('boom')
        if operation == 'reading-items':
            return Reply(operation, method, status=200, data={'items': self.items})
        if operation == 'metadata-state':
            self.reads += 1
            if self.reads == 1:
                return Reply(operation, method, status=200, data=self.state)
            if self.state_after is None:
                return Reply(operation, method, status=503, code='internal',
                             data={'error': {'code': 'internal'}})
            return Reply(operation, method, status=200, data=self.state_after)
        if operation == 'metadata-edit':
            self.bodies.append(kwargs.get('body'))
            if self.edit == 409:
                return Reply(operation, method, status=409, code='revision_mismatch',
                             data={'error': {'code': 'revision_mismatch'}})
            return Reply(operation, method, status=200, data={'revision': 'c' * 64})
        raise AssertionError(operation)


def plan(items=PDF, series='Portico Walk Magazine', apply=False, item='item-1', **kwargs):
    client = FakeClient(items if isinstance(items, list) else [items], **kwargs)
    document = module.Plan(client, 'library-1', item, series).execute(apply=apply)
    return document, client


class PlanTests(unittest.TestCase):
    def test_ready_requires_explicit_intent_and_reads_the_schema(self):
        document, client = plan()
        self.assertEqual(document['result'], 'ready')
        self.assertIn('metadata-state', [c[0] for c in client.calls])
        self.assertEqual(document['repair']['publicationAllowed'], ['book', 'magazine', 'newspaper'])

    def test_apply_sends_one_cas_and_confirms_the_readback(self):
        document, client = plan(apply=True, readback_state=AFTER)
        self.assertEqual(document['result'], 'confirmed')
        body = client.bodies[0]
        self.assertEqual(body['action'], 'edit')
        self.assertEqual(body['expectedRevision'], REV)
        self.assertEqual(body['fields']['series'], {'value': 'Portico Walk Magazine', 'locked': True})
        self.assertEqual(document['edit']['readback'], 'confirmed')

    def test_unconfirmed_readback_is_acknowledged_not_retried(self):
        document, client = plan(apply=True, readback_state=None)
        self.assertEqual(document['result'], 'acknowledged')
        self.assertEqual(document['edit']['acknowledged'], True)
        self.assertEqual(document['edit']['readback'], 'unconfirmed')
        self.assertEqual(len(client.bodies), 1)

    def test_missing_intent_series_conflict_and_revision_shape(self):
        self.assertEqual(plan(series='')[0]['reason'], 'series-intent-required')
        self.assertEqual(plan(series='Different Series')[0]['reason'], 'series-conflicts-with-item')
        bad = dict(STATE, revision='short')
        self.assertEqual(plan(state=bad)[0]['reason'], 'revision-invalid')

    def test_exact_item_required_and_library_must_match(self):
        other = dict(PDF, id='item-2')
        self.assertEqual(plan(items=[PDF, other], item='item-9')[0]['reason'], 'item-absent')
        wrong_library = dict(PDF, libraryId='library-2')
        self.assertEqual(plan(items=wrong_library)[0]['reason'], 'item-library-mismatch')

    def test_schema_gate_and_conflict_are_closed(self):
        state = dict(STATE, schema=[{'field': 'series','type':'text'}])
        self.assertEqual(plan(state=state)[0]['reason'], 'producer-unsupported')
        state = dict(STATE, schema=[
            {'field': 'series','type':'text'}, {'field': 'publicationType','type':'enum', 'allowed': ['book']}])
        self.assertEqual(plan(state=state)[0]['reason'], 'publication-type-not-allowed')
        self.assertEqual(plan(apply=True, edit=409)[0]['reason'], 'revision-conflict')

    def test_unrelated_revision_or_wrong_field_scope_cannot_confirm(self):
        import copy
        for change in ('no-fields','wrong-series','unlocked','other-viewer','other-target'):
            state=copy.deepcopy(AFTER)
            if change=='no-fields':state.pop('snapshot')
            if change=='wrong-series':state['snapshot']['fields']['series']['value']='another series'
            if change=='unlocked':state['snapshot']['fields']['series']['locked']=False
            if change=='other-viewer':state['viewerFence']='successor'
            if change=='other-target':state['target']['id']='successor'
            doc,client=plan(apply=True,readback_state=state)
            self.assertEqual(doc['result'],'acknowledged',change)
            self.assertEqual(doc['edit']['readback'],'unconfirmed',change)
            self.assertEqual(len(client.bodies),1)

    def test_series_and_identity_never_enter_receipt(self):
        doc,_=plan(apply=True,readback_state=AFTER)
        for forbidden in ('Portico Walk Magazine','item-1','library-1','original-server','original-viewer',REV):
            self.assertNotIn(forbidden,str(doc))

    def test_malformed_schema_and_initial_foreign_scope_send_no_edit(self):
        cases=[dict(STATE,viewerFence=''),dict(STATE,target={'kind':'item','id':'foreign'}),
               dict(STATE,schema=[{'field':'series','type':'enum'}]),
               dict(STATE,schema=[{'field':'publicationType','type':'enum','allowed':'magazine'}])]
        for state in cases:
            doc,client=plan(apply=True,state=state)
            self.assertEqual(doc['result'],'refused')
            self.assertEqual(client.bodies,[])

    def test_unexpected_shape_writes_closed_fact_without_edit(self):
        doc,client=plan(apply=True,fail={'metadata-state'})
        self.assertEqual(doc['reason'],'unexpected-shape')
        self.assertEqual(client.bodies,[])

    def test_fence_wraps_reads_and_mutations(self):
        order = []

        class Inner:
            def call(self, operation, method, path, **kwargs):
                order.append(('call', method))
                return Reply(operation, method, status=200, data={})

        class Context:
            pass

        client = module.Fenced(Inner(), Context(), custody=lambda c: order.append(('fence',)))
        client.call('s', 'GET', '/x')
        client.call('e', 'POST', '/x')
        self.assertEqual(order, [('fence',), ('call', 'GET'), ('fence',), ('fence',), ('call', 'POST')])

    def test_receipt_is_atomic_and_closed(self):
        import tempfile, json
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'receipt.json'
            module.write_receipt(target, {'result': 'ready'})
            self.assertEqual(json.loads(target.read_text()), {'result': 'ready'})
            self.assertEqual(target.stat().st_mode & 0o777, 0o600)
            self.assertEqual([p.name for p in Path(directory).iterdir()], ['receipt.json'])


if __name__ == '__main__':
    unittest.main(verbosity=1)
