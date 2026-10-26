#!/usr/bin/env python3
"""Synthetic tests for the one-page CB7 Reading check. No network or database.

Includes tests for the real bounded byte reader (`PageBytes`) against a fake
loopback context, so the If-Match quotation, the page-0 path and the strict
magic are exercised rather than only the Check wrapper.
"""
import importlib.util
import sys
import unittest
from pathlib import Path

HERE = Path(__file__).parent
sys.path.insert(0, str(HERE))
from api_smoke.client import Reply

spec = importlib.util.spec_from_file_location('cb7_reading', HERE / 'cb7-reading.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

REV = 'a' * 64
CB7 = {'id': 'item-1', 'libraryId': 'library-1', 'format': 'cb7', 'sourceId': 'source-1', 'inventoryAvailable': True}
PNG = b'\x89PNG\r\n\x1a\n' + b'0' * 900
WEBP = b'RIFF' + (900).to_bytes(4, 'little') + b'WEBP' + b'0' * 900


class FakeClient:
    def __init__(self, items, declared='cb7', open_status=200, close_status=204, revision=REV):
        self.items = items
        self.declared = declared
        self.open_status = open_status
        self.close_status = close_status
        self.revision = revision
        self.calls = []

    def call(self, operation, method, path, **kwargs):
        self.calls.append(operation)
        if operation == 'reading-items':
            return Reply(operation, method, status=200, data={'items': self.items})
        if operation == 'reading-open':
            if self.open_status != 200:
                return Reply(operation, method, status=self.open_status, code='not_found',
                             data={'error': {'code': 'not_found'}})
            return Reply(operation, method, status=200,
                         data={'sessionId': 'sess-1', 'revision': self.revision,
                               'format': self.declared})
        if operation == 'reading-close':
            return Reply(operation, method, status=self.close_status)
        raise AssertionError(operation)


class FakeReader:
    def __init__(self, status=200, content='image/png', body=PNG, error=None):
        self.status, self.content, self.body, self.error = status, content, body, error
        self.calls = []

    def get(self, path, revision, width=None, limit=None):
        self.calls.append((path, revision, width))
        if self.error:
            raise module.CheckError(self.error)
        return self.status, self.content, self.body


def run(items=CB7, reader=None, **kwargs):
    client = FakeClient(items if isinstance(items, list) else [items], **kwargs)
    check = module.Check(client, 'library-1', 'item-1', bytes_reader=reader or FakeReader())
    return check.execute(), client


class CheckTests(unittest.TestCase):
    def test_one_page_verified_with_revision_and_page_zero(self):
        reader = FakeReader()
        document, client = run(reader=reader)
        self.assertEqual(document['result'], 'verified')
        self.assertEqual(document['page']['magic'], 'png')
        self.assertEqual(reader.calls[0][0], '/v1/reading/sessions/sess-1/pages/0')
        self.assertEqual(reader.calls[0][1], REV)
        self.assertEqual(reader.calls[0][2], 800)
        self.assertTrue(document['cleanup']['confirmed'])

    def test_strict_magic_rejects_riff_only_and_wrong_content_type(self):
        document, _ = run(reader=FakeReader(body=b'RIFF' + b'0' * 900))
        self.assertEqual(document['failure']['reason'], 'page-not-image')
        document, _ = run(reader=FakeReader(content='text/html'))
        self.assertEqual(document['failure']['reason'], 'page-content-type')
        document, _ = run(reader=FakeReader(body=WEBP, content='image/webp'))
        self.assertEqual(document['result'], 'verified')

    def test_revision_must_be_hex64(self):
        document, client = run(revision='short')
        self.assertEqual(document['failure']['reason'], 'revision-invalid')
        self.assertIn('reading-close', client.calls)

    def test_ordinary_build_is_refused(self):
        document, _ = run(items=[])
        self.assertEqual(document['failure']['reason'], 'cb7-not-admitted')

    def test_wrong_session_format_closes_what_was_opened(self):
        document, client = run(declared='cbz')
        self.assertEqual(document['failure']['reason'], 'session-format-not-cb7')
        self.assertTrue(document['cleanup']['confirmed'])

    def test_a_pure_refusal_keeps_its_result_even_though_cleanup_has_no_session(self):
        document, _ = run(items=[])
        self.assertEqual(document['result'], 'refused')
        self.assertEqual(document['failure']['reason'], 'cb7-not-admitted')
        self.assertEqual(document['reason'], 'cb7-not-admitted')
        self.assertEqual(document['cleanup'], {'confirmed': False, 'reason': 'no-session'})

    def test_only_a_created_session_with_unconfirmed_close_fails(self):
        document, _ = run(close_status=500)
        self.assertEqual(document['result'], 'failed')
        self.assertEqual(document['reason'], 'cleanup-close-failed')
        self.assertIsNone(document['failure'])

    def test_unexpected_exception_is_closed_and_closes_session(self):
        document, client = run(reader=FakeReader(error='page-transport'))
        self.assertEqual(document['result'], 'verified' if False else 'refused')
        self.assertEqual(document['failure']['reason'], 'page-transport')
        self.assertIn('reading-close', client.calls)
        document, _ = run(reader=type('Boom', (), {'get': lambda *a, **k: (_ for _ in ()).throw(RuntimeError('x'))})())
        self.assertEqual(document['failure']['reason'], 'unexpected-shape')
        self.assertTrue(document['cleanup']['confirmed'])

    def test_receipt_has_no_identity_revision_or_url(self):
        document, _ = run()
        text = str(document)
        for forbidden in ('item-1', 'sess-1', 'library-1', '/v1/', REV, 'http://'):
            self.assertNotIn(forbidden, text)

    def test_page_bytes_reader_quotes_revision_and_fences_custody(self):
        seen = []

        class Context:
            host, port = '127.0.0.1', 65000
            access_token, installation_id = 'token-value', 'install'
        class Deadline:
            def remaining(self):
                return 10.0

        class Connection:
            sock = None
            def __init__(self, *a, **k): pass
            def request(self, method, path, headers=None):
                seen.append((method, path, headers))
            def getresponse(self):
                class R:
                    status = 200
                    fp = None
                    done = False
                    def getheader(self, name): return 'image/png'
                    def isclosed(self): return self.done
                    def read1(self, n): self.done = True; return PNG
                return R()
            def close(self): pass

        import http.client
        original = http.client.HTTPConnection
        http.client.HTTPConnection = Connection
        try:
            fences = []
            reader = module.PageBytes(Context(), Deadline(), custody=lambda c: fences.append(1))
            reader.get('/v1/reading/sessions/sess-1/pages/0', REV)
        finally:
            http.client.HTTPConnection = original
        self.assertEqual(fences, [1, 1])
        self.assertEqual(seen[0][2]['If-Match'], '"' + REV + '"')

    def test_slow_body_cannot_extend_the_original_deadline(self):
        from unittest.mock import patch
        times=iter([5.0,4.0,0.0])
        deadline=type('D',(),{'remaining':lambda _:next(times)})()
        class Response:
            status=200;fp=None
            def getheader(self,_):return 'image/png'
            def isclosed(self):return False
            def read1(self,_):return b'x'
        class Connection:
            sock=None
            def __init__(self,*a,**k):pass
            def request(self,*a,**k):pass
            def getresponse(self):return Response()
            def close(self):pass
        context=type('C',(),{'host':'127.0.0.1','port':1,'access_token':'fixture','installation_id':'fixture'})()
        with patch.object(module.http.client,'HTTPConnection',Connection):
            with self.assertRaisesRegex(module.CheckError,'page-budget'):
                module.PageBytes(context,deadline,custody=lambda _:None).get('/v1/reading/sessions/sess-1/pages/0',REV)

    def test_exact_item_and_library_are_required_before_open(self):
        for items in ([CB7,CB7],[dict(CB7,libraryId='other')]):
            doc,client=run(items=items)
            self.assertEqual(doc['reason'],'item-scope-mismatch')
            self.assertNotIn('reading-open',client.calls)


class FenceTests(unittest.TestCase):
    def test_reads_fence_before_and_after_and_mutations_only_before(self):
        calls = []

        class Inner:
            def call(self, operation, method, path, **kwargs):
                calls.append(('call', method))
                return Reply(operation, method, status=200, data={})

        class Context:
            pass

        def custody(_context):
            calls.append(('fence',))

        client = module.FencedClient(Inner(), Context(), custody=custody)
        client.call('list', 'GET', '/v1/reading/items')
        client.call('open', 'POST', '/v1/reading/sessions')
        client.call('close', 'DELETE', '/v1/reading/sessions/x')
        self.assertEqual(calls, [('fence',), ('call', 'GET'), ('fence',),
                                 ('fence',), ('call', 'POST'),
                                 ('fence',), ('call', 'DELETE')])

    def test_page_width_is_consumed_and_bounded(self):
        reader = module.PageBytes(type('C', (), {'host': '127.0.0.1', 'port': 1,
                                                 'access_token': 't', 'installation_id': 'i'})(),
                                  type('D', (), {'remaining': lambda self: 5.0})(),
                                  custody=lambda c: None)
        with self.assertRaises(module.CheckError):
            reader.get('/v1/reading/sessions/s/pages/0', REV, 10)
        with self.assertRaises(module.CheckError):
            reader.get('/v1/reading/sessions/s/pages/0', REV, 4000)

    def test_receipt_write_is_atomic_and_0600(self):
        import tempfile, os, json
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / 'receipt.json'
            module.write_receipt(target, {'a': 1})
            self.assertEqual(json.loads(target.read_text()), {'a': 1})
            self.assertEqual(target.stat().st_mode & 0o777, 0o600)
            self.assertEqual([p.name for p in Path(directory).iterdir()], ['receipt.json'])


if __name__ == '__main__':
    unittest.main(verbosity=1)
