import assert from 'node:assert/strict';
import {test} from 'node:test';
import {authorPredicate, loadFacetPage} from '../../src/screens/read/facets.ts';

const page = (values: unknown[]) => ({field: 'author', total: values.length, totalIsLowerBound: false, values});

test('an author facet page keeps the server value, its long label and its count', async () => {
  let requested = '';
  const label = 'A'.repeat(4096);
  const values = await loadFacetPage(async path => {
    requested = path;
    return page([
      {value: 'Ursula K. Le Guin', label, count: 12, countIsLowerBound: false},
      {value: 'Frank Herbert', label: 'Frank Herbert', count: 3, countIsLowerBound: true},
    ]);
  }, {libraryId: 'lib-reading', field: 'author', pivot: 'books', limit: 50, offset: 100}, new AbortController().signal);
  assert.match(requested, /\/v1\/libraries\/lib-reading\/facets\?/);
  assert.match(requested, /field=author/);
  assert.match(requested, /limit=50/);
  assert.match(requested, /offset=100/);
  assert.equal(values.values[0]!.label, label);
  assert.equal(values.values[0]!.count, 12);
  assert.equal(values.values[1]!.countIsLowerBound, true);
});

test('a series facet page carries the server-issued opaque series id', async () => {
  const seriesId = 'a'.repeat(43);
  const values = await loadFacetPage(async () => ({field: 'series', total: 1, totalIsLowerBound: false, values: [{value: 'Dune', label: 'Dune', count: 6, seriesId}]}),
    {libraryId: 'lib-reading', field: 'series', pivot: 'comics'}, new AbortController().signal);
  assert.equal(values.values[0]!.seriesId, seriesId);
  assert.equal(values.values[0]!.value, 'Dune');
});

test('an author facet omits a series id the server did not send', async () => {
  const values = await loadFacetPage(async () => page([{value: 'Frank Herbert', label: 'Frank Herbert', count: 6}]), {libraryId: 'lib', field: 'author', pivot: 'books'}, new AbortController().signal);
  assert.equal('seriesId' in values.values[0]!, false);
});

test('the page refuses a bad request before it reaches the server', async () => {
  let called = false;
  const request = async () => { called = true; return page([]); };
  await assert.rejects(loadFacetPage(request, {libraryId: 'lib', field: 'author', pivot: 'books', limit: 0}, new AbortController().signal), /invalid_request/);
  await assert.rejects(loadFacetPage(request, {libraryId: 'lib', field: 'author', pivot: 'books', offset: -1}, new AbortController().signal), /invalid_request/);
  assert.equal(called, false);
});

test('an author value filters browse by the accepted attribution string', () => {
  assert.deepEqual(authorPredicate('Ursula K. Le Guin'), {field: 'author', operator: 'equals', value: 'Ursula K. Le Guin'});
  assert.throws(() => authorPredicate(''), /invalid_request/);
});
