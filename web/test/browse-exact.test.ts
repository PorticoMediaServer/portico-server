import test from 'node:test';
import assert from 'node:assert/strict';
import {browsePageSource} from '../src/app/browse-page-source.ts';

const entry = (id: string, extra: Record<string, unknown> = {}) => ({id, kind: 'movie', title: `Title ${id}`, ...extra});
const signal = new AbortController().signal;

function browsePage(opts: {start: number; total: number; hasMore: boolean; nextCursor?: string; revision?: string; positionIndex?: Array<{key: string; index: number}>; entries?: Array<{id: string; [k: string]: unknown}>}): any {
  const count = opts.entries?.length ?? 60;
  return {
    pivot: 'movies',
    applied: {query: {field: 'genre', operator: 'contains', value: 'Action'}, sort: [{field: 'title', direction: 'asc'}], seek: null},
    entries: opts.entries ?? Array.from({length: count}, (_, i) => entry(`e${opts.start + i}`)),
    pageInfo: {
      start: opts.start, total: opts.total, revision: opts.revision ?? 'rev',
      hasMore: opts.hasMore, nextCursor: opts.nextCursor ?? (opts.hasMore ? 'cursor-next' : ''),
    },
    positionIndex: opts.positionIndex ?? [],
  };
}

test('an ad hoc query pages by range above 10,000 with an exact total', async () => {
  const seen: Array<{start?: number; cursor?: string}> = [];
  const api = {
    request: async (_path: string, _method?: string, body?: unknown) => {
      const b = body as {range?: {start?: number}; cursor?: string; limit?: number};
      seen.push({start: b.range?.start, cursor: b.cursor});
      const start = b.range?.start ?? 0;
      return browsePage({start, total: 10140, hasMore: start + 60 < 10140, nextCursor: start + 60 < 10140 ? `cursor-${start + 60}` : ''});
    },
  };
  const source = browsePageSource(api, {libraryId: 'lib', pivot: 'movies', predicates: [{field: 'genre', operator: 'contains', value: 'Action'}], sort: {field: 'title', direction: 'asc'}});
  const first = await source(0, 60, signal);
  assert.equal(first.total, 10140);
  assert.deepEqual(first.positionIndex, {});
  const nearCap = await source(9960, 60, signal);
  assert.equal(nearCap.total, 10140);
  const pastCap = await source(10020, 60, signal);
  assert.equal(pastCap.items[0]!.id, 'e10020');
  assert.equal(pastCap.total, 10140);
  assert.ok(seen.some(s => s.start === 10020));
  assert.ok(seen.every(s => s.cursor === undefined));
});

test('W1: a stale non-title sort re-navigates through the buckets after reload', async () => {
  const calls: Array<{start?: number; revision?: string}> = [];
  let revision = 'rev-one';
  const anchors = [{key: '1995', index: 480}, {key: '1988', index: 540}];
  const api = {
    request: async (_path: string, _method?: string, body?: unknown) => {
      const b = body as {range?: {start?: number; revision?: string}};
      const start = b.range?.start ?? 0;
      calls.push({start, revision: b.range?.revision});
      assert.ok(!('anchorId' in (b.range ?? {})), 'never sends anchorId');
      if (b.range?.revision === 'rev-one' && start !== 0) throw Object.assign(new Error('expired'), {code: 'stale_continuation', status: 409});
      if (start === 0 && b.range?.revision === undefined && calls.length > 2) {
        revision = 'rev-two';
        return browsePage({start: 0, total: 1000, hasMore: true, revision: 'rev-two', positionIndex: [{key: '1995', index: 500}, {key: '1988', index: 560}], entries: Array.from({length: 60}, (_, i) => entry(`n${i}`))});
      }
      if (start === 0) return browsePage({start: 0, total: 1000, hasMore: true, revision: 'rev-one', positionIndex: anchors});
      if (start === 480 && revision === 'rev-one') return browsePage({start: 480, total: 1000, hasMore: true, revision: 'rev-one', entries: Array.from({length: 60}, (_, i) => entry(`e${480 + i}`, {year: 1995}))});
      if (start === 500) return browsePage({start: 500, total: 1000, hasMore: true, revision: 'rev-two', entries: Array.from({length: 60}, (_, i) => entry(`n${500 + i}`, {year: 1995}))});
      return browsePage({start, total: 1000, hasMore: start + 60 < 1000, revision});
    },
  };
  const source = browsePageSource(api, {libraryId: 'lib', pivot: 'movies', predicates: [], sort: {field: 'year', direction: 'desc'}});
  await source(0, 60, signal);
  const before = await source(480, 60, signal);
  assert.equal((before.items[0] as {year?: number}).year, 1995);
  const after = await source(480, 60, signal);
  assert.equal(after.items[0]!.id, 'n500');
  const bucketCall = calls.find(c => c.start === 500);
  assert.ok(bucketCall, 'jumped to the 1995 bucket');
  assert.ok(!calls.some(c => (c as {anchorId?: string}).anchorId !== undefined));
});
