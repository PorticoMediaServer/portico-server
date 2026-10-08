import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const noop = () => ({});
async function load() {
  return componentModule(new URL('../src/screens/server/ChannelBuilder.tsx', import.meta.url), new Proxy({react: {default: {}, useEffect: noop, useMemo: noop, useRef: noop, useState: (v: unknown) => [v, noop], useCallback: (f: unknown) => f}}, {get: (t: any, k: string) => t[k] ?? new Proxy({}, {get: () => noop})})) as Promise<typeof import('../src/screens/server/ChannelBuilder.tsx')>;
}

test('template genre and year criteria become builder conditions (Family, Eighties)', async () => {
  const {withLegacyCriteria} = await load();
  const family = withLegacyCriteria({mode: 'all', items: []}, {genres: ['Family']});
  assert.equal(family.items.length, 1);
  assert.deepEqual((family.items[0] as any).predicates, [{field: 'genre', operator: 'contains-any', value: ['Family']}]);
  const eighties = withLegacyCriteria({mode: 'all', items: []}, {yearFrom: 1980, yearThrough: 1989});
  assert.deepEqual((eighties.items[0] as any).predicates, [{field: 'year', operator: 'between', value: [1980, 1989]}]);
  const none = {mode: 'all' as const, items: []};
  assert.equal(withLegacyCriteria(none, {genres: [], yearFrom: 0, yearThrough: 0}), none);
});
