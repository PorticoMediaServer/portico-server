import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {defaultI18n} from '@i18n';
import {transformTsx} from './transform-tsx.mjs';

registerHooks({load(url, context, next) {
  if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
  if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
  return next(url, context);
}});

// Artwork pixel sizes are read from the browser global; the server render has no window.
(globalThis as {devicePixelRatio?: number}).devicePixelRatio = 1;

const {TitleActions, TitleDetails} = await import('../src/screens/detail/Detail.tsx');
const {createDetailSpecimen} = await import('../src/dev/entries/detail-specimen.tsx');
const {DetailService} = await import('@core/detail.ts');

const specimenContext = (specimen: ReturnType<typeof createDetailSpecimen>) => ({
  api: specimen.api, events: specimen.events, scope: {serverId: specimen.serverId, viewerId: specimen.personalFence},
  isCurrent: () => true, artwork: specimen.artwork,
  preferences: {getSnapshot: () => specimen.preferences, subscribe: specimen.subscribePreferences},
  effects: {openEntry() {}, openSettings() {}, async play() {}, reportError(error: unknown) { throw error; }},
});

async function detailData(frame: '2' | '3') {
  const specimen = createDetailSpecimen(frame);
  const service = new DetailService({api: specimen.api, scope: {serverId: specimen.serverId, viewerId: specimen.personalFence},
    requestId: async () => crypto.randomUUID()});
  await service.select({libraryId: 'movies', itemId: specimen.itemId});
  const data = service.getSnapshot().data;
  assert.ok(data);
  const binding = {context: specimenContext(specimen), service, readVersions: async () => { throw new Error('not asked'); },
    play: async () => {}, mutate: async () => {}, browseGenre: () => { throw new Error('not asked'); }} as never;
  return {specimen, service, data, binding};
}

const text = (html: string) => html.replace(/<[^>]+>/g, '');

test('frame 2 draws the white Resume button with the long time left and Start over beside it', async () => {
  const {data, binding, service} = await detailData('2');
  try {
    const html = renderToStaticMarkup(createElement(TitleActions, {binding, data}));
    const body = text(html);
    assert.match(body, new RegExp(defaultI18n.t('next.controls.resume')));
    // The drawn caption is the title's own long duration, not the compact "1h 26m left".
    assert.match(body, /1 h 26 min left/);
    assert.match(body, new RegExp(defaultI18n.t('next.controls.startOver')));
  } finally { service.dispose(); }
});

test('frame 3 draws About with Rated and the collection banner with honest member facts', async () => {
  const {data, binding, service} = await detailData('3');
  try {
    const body = text(renderToStaticMarkup(createElement(TitleDetails, {binding, data})));
    assert.match(body, /About/);
    assert.match(body, /Rated/);
    assert.match(body, /PG-13/);
    // The producer counts every visible member; it never claims a film-only total.
    assert.match(body, /Part of a collection/);
    assert.match(body, /Dune collection/);
    assert.match(body, /2 titles on your server, from 2021 to 2024\./);
    assert.doesNotMatch(body, /films/);
    assert.match(body, new RegExp(defaultI18n.t('next.home.openCollection')));
    assert.match(body, new RegExp(defaultI18n.t('shelf.seeAllCount', {count: 64}).replace(/[.*+?^${}()|[\]\\]/g, '\\$&')));
  } finally { service.dispose(); }
});
