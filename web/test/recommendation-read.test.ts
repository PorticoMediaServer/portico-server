import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule, hooks} from './helpers/component-harness.mjs';

async function reads() {
  const h = hooks();
  let effects: (() => unknown)[] = [];
  h.react.useEffect = (effect: () => unknown) => { effects.push(effect); };
  const requests: {path: string; resolve: (value: unknown) => void; reject: (error: unknown) => void}[] = [];
  const api = {request: <T,>(path: string) => new Promise<T>((resolve, reject) => { requests.push({path, resolve, reject}); })};
  const app = await componentModule(new URL('../src/app/detail.ts', import.meta.url), {
    react: h.react,
    '@core/detail.ts': {}, '@core/recommendations.ts': {
      fetchItemRecommendations: (api: typeof api, id: string, _limit: number, signal: AbortSignal) => api.request(`/items/${id}`, 'GET', undefined, signal),
      fetchShowRecommendations: (api: typeof api, id: string, _limit: number, signal: AbortSignal) => api.request(`/shows/${id}`, 'GET', undefined, signal),
    },
    '@core/show-workspace.ts': {}, './content': {}, '@core/presentation/index.ts': {},
    './session': {useSession: () => ({api})}, './viewer-scope': {useViewerScope: () => ({serverId: 'server', viewerId: 'viewer'})},
    './not-interested': {RECOMMENDATIONS_RESET: 'reset', recommendationViewer: () => 'viewer'},
  });
  const render = (kind: 'item' | 'show') => { effects = []; const state = h.render(() => kind === 'item' ? app.useItemRecommendations({libraryId: 'library', itemId: 'movie', kind: 'movie'}) : app.useShowRecommendations('show')); return {state, effect: effects[0]}; };
  return {render, requests};
}

test('movie recommendation rejection remains an error with retry, then a successful empty result', async () => {
  const app = await reads();
  assert.equal(app.render('item').state.phase, 'loading');
  app.render('item').effect();
  const failure = {code: 'invalid_recommendations'};
  app.requests[0].reject(failure);
  await new Promise(setImmediate);
  const failed = app.render('item').state;
  assert.equal(failed.phase, 'error');
  assert.equal(failed.error, failure);
  failed.retry();
  app.render('item').effect();
  app.requests[1].resolve({rows: []});
  await new Promise(setImmediate);
  assert.equal(app.render('item').state.phase, 'ready');
  assert.deepEqual(app.render('item').state.data, {rows: []});
});

test('show recommendation rejection remains distinct from a successful empty result', async () => {
  const app = await reads();
  app.render('show').effect();
  app.requests[0].reject({code: 'server_unavailable'});
  await new Promise(setImmediate);
  assert.equal(app.render('show').state.phase, 'error');
  app.render('show').state.retry();
  app.render('show').effect();
  app.requests[1].resolve({rows: []});
  await new Promise(setImmediate);
  assert.equal(app.render('show').state.phase, 'ready');
});
