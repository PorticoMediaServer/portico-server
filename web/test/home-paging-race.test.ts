import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule, hooks} from './helpers/component-harness.mjs';

function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; }); return {resolve, reject, promise}; }

async function homeHarness() {
  const h = hooks();
  const effects: (() => unknown)[] = [];
  h.react.useEffect = (effect: () => unknown) => { effects.push(effect); };
  const reads: ReturnType<typeof deferred<any>>[] = [];
  const pages: ReturnType<typeof deferred<any>>[] = [];
  const module = await componentModule(new URL('../src/app/home.ts', import.meta.url), {
    react: h.react,
    '@core/home.ts': {
      fetchHome: () => { const next = deferred<any>(); reads.push(next); return next.promise; },
      fetchHomeRow: () => { const next = deferred<any>(); pages.push(next); return next.promise; },
      appendHomeRowPage: (row: any, page: any) => ({...row, entries: [...row.entries, ...page.entries], nextCursor: page.nextCursor}),
    },
    './session': {sessionIdentity: () => 'viewer', useSession: () => ({api: {}, session: {}})},
    './viewer-scope': {useViewerScope: () => ({serverId: 'server', viewerId: 'viewer'})},
    './not-interested': {RECOMMENDATIONS_RESET: 'reset', recommendationViewer: () => 'viewer'},
    './i18n': {currentI18n: () => ({t: () => ''})},
    '@core/presentation/index.ts': {serverTextLabel: () => ''},
    '@i18n': {},
  });
  return {render: () => h.render(() => module.useHome()), effects, reads, pages};
}
const document = (title: string) => ({rows: [{id: 'recommended', nextCursor: 'next', entries: [{id: title}]}], layout: {revision: 1}});

test('Home discards an old row page when a newer document replaces it', async () => {
  const app = await homeHarness();
  app.render();
  app.effects[0]();
  app.reads[0].resolve(document('old'));
  await new Promise(setImmediate);
  const ready = app.render();
  const more = ready.more('recommended');
  assert.equal(app.pages.length, 1);
  const refresh = ready.refresh();
  app.reads[1].resolve(document('new'));
  await refresh;
  app.pages[0].resolve({entries: [{id: 'old-page'}], nextCursor: ''});
  await more;
  assert.deepEqual(app.render().state.document.rows[0].entries.map((entry: any) => entry.id), ['new']);
});

test('stale continuation visibly clears the old Home document while refreshing', async () => {
  const app = await homeHarness();
  app.render();
  app.effects[0]();
  app.reads[0].resolve(document('old'));
  await new Promise(setImmediate);
  const more = app.render().more('recommended');
  app.pages[0].reject(Object.assign(new Error('changed'), {code: 'stale_continuation', status: 409}));
  await more;
  assert.equal(app.render().state.phase, 'loading');
  assert.equal(app.render().state.document, undefined);
  app.reads[1].resolve(document('fresh'));
  await new Promise(setImmediate);
  assert.deepEqual(app.render().state.document.rows[0].entries.map((entry: any) => entry.id), ['fresh']);
});

test('two current row pages can both append to the same Home generation', async () => {
  const app = await homeHarness();
  app.render();
  app.effects[0]();
  app.reads[0].resolve({rows: [{id: 'first', nextCursor: 'one', entries: [{id: 'a'}]}, {id: 'second', nextCursor: 'two', entries: [{id: 'b'}]}], layout: {revision: 1}});
  await new Promise(setImmediate);
  const ready = app.render();
  const first = ready.more('first');
  const second = ready.more('second');
  app.pages[0].resolve({entries: [{id: 'a2'}], nextCursor: ''});
  await first;
  app.pages[1].resolve({entries: [{id: 'b2'}], nextCursor: ''});
  await second;
  assert.deepEqual(app.render().state.document.rows.map((row: any) => row.entries.map((entry: any) => entry.id)), [['a', 'a2'], ['b', 'b2']]);
});
