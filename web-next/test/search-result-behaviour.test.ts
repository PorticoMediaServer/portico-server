/** Offline checkpoint preparation: real SearchResult handlers, no runtime acceptance claim. */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {runScreenCommand} from '../src/screens/detail/context.ts';
import {componentModule} from '../../web/test/helpers/component-harness.mjs';
import {all, createRenderer, one, tree} from '../../web/test/helpers/interaction-harness.mjs';
const t = (key: any, values?: any) => defaultI18n.t(key, values);
async function mount(entry: any, options: {play?: () => Promise<void>; live?: Map<string, any>; openFailure?: boolean} = {}) {
  const h = createRenderer(), calls: {name: string; value?: any}[] = [];
  const binding = {live: {subscribe: () => () => {}, getSnapshot: () => options.live ?? new Map()},
    open: (value: any) => { calls.push({name: 'open', value}); if (options.openFailure) throw new Error('failed'); },
    play: (value: any) => { calls.push({name: 'play', value}); return options.play?.() ?? Promise.resolve(); },
    context: {artwork: {foreground: (path: string) => path}, effects: {reportError: (value: any) => calls.push({name: 'error', value})}}};
  const app = await componentModule(new URL('../src/screens/search/Search.tsx', import.meta.url), {
    react: h.react, '../detail/text': {t, locale: () => 'en-US'}, '../detail/context': {runScreenCommand},
    '../detail/model': {entryCard: (value: any) => ({title: value.title})},
    '../../kit/cards': {PosterCard: 'PosterCard', SquareCard: 'SquareCard', CircleCard: 'CircleCard', EpisodeTile: 'EpisodeTile', GenreTile: 'GenreTile', LiveCard: 'LiveCard'},
    '../../kit/artwork': {Artwork: 'Artwork'}, '../../kit/Icon': {Icon: 'Icon'}, './search.module.css': {default: {}},
  });
  const flat = () => tree(h.render(() => app.SearchResult({entry, binding, onCommit: () => calls.push({name: 'commit'})})));
  return {flat, calls};
}
const movie = {id: 'item', libraryId: 'library', kind: 'movie', title: 'Stored title', available: true, playback: {itemId: 'item'}, duration: 60};
const settle = async () => { for (let i = 0; i < 6; i++) await Promise.resolve(); };

test('card navigation opens the exact entry and then commits history', async () => {
  const f = await mount(movie);
  one(f.flat(), 'PosterCard').props.onNavigate();
  assert.deepEqual(f.calls, [{name: 'open', value: movie}, {name: 'commit'}]);
});

test('Play commits only after successful completion; failure reports without committing', async () => {
  let resolve!: () => void;
  const held = new Promise<void>(done => { resolve = done; });
  const f = await mount(movie, {play: () => held});
  one(f.flat(), 'PosterCard').props.onPlay();
  assert.deepEqual(f.calls, [{name: 'play', value: movie}]);
  resolve(); await settle();
  assert.equal(f.calls.at(-1)?.name, 'commit');
  const error = new Error('refused');
  const failed = await mount(movie, {play: async () => { throw error; }});
  one(failed.flat(), 'PosterCard').props.onPlay(); await settle();
  assert.deepEqual(failed.calls, [{name: 'play', value: movie}, {name: 'error', value: error}]);
});

test('unavailable or missing playback entries retain navigation without a Play callback', async () => {
  for (const entry of [{...movie, available: false}, {...movie, playback: undefined}]) {
    const f = await mount(entry), card = one(f.flat(), 'PosterCard');
    assert.equal(card.props.onPlay, undefined);
    card.props.onNavigate(); assert.equal(f.calls[0].value, entry);
  }
});

test('episode and square reading entries keep their actual entry identity', async () => {
  for (const [kind, type] of [['episode', 'EpisodeTile'], ['book', 'SquareCard'], ['audiobook_file', 'SquareCard']]) {
    const entry = {...movie, kind}; const f = await mount(entry);
    one(f.flat(), type).props.onNavigate();
    assert.deepEqual(f.calls, [{name: 'open', value: entry}, {name: 'commit'}]);
  }
});

test('song row plays when available and opens details when unavailable', async () => {
  for (const available of [true, false]) {
    const entry = {...movie, kind: 'song', available}; const f = await mount(entry);
    one(f.flat(), 'button').props.onClick(); await settle();
    assert.deepEqual(f.calls, [{name: available ? 'play' : 'open', value: entry}, {name: 'commit'}]);
  }
});

test('channel uses live facts when present and retains fallback navigation otherwise', async () => {
  const entry = {id: 'channel', kind: 'channel', title: 'One'};
  const facts = {channel: {id: 'channel', name: 'One'}, programme: {title: 'Now'}, tint: 'blue'};
  const live = await mount(entry, {live: new Map([[entry.id, facts]])});
  const card = one(live.flat(), 'LiveCard');
  assert.equal(card.props.channel, facts.channel); assert.equal(card.props.programme, facts.programme);
  card.props.onNavigate(); assert.equal(live.calls[0].value, entry);
  const fallback = await mount(entry); one(fallback.flat(), 'button').props.onClick();
  assert.deepEqual(fallback.calls, [{name: 'open', value: entry}, {name: 'commit'}]);
});

test('failed navigation reports the error without committing history', async () => {
  const f = await mount(movie, {openFailure: true});
  one(f.flat(), 'PosterCard').props.onNavigate();
  assert.deepEqual(f.calls.map(call => call.name), ['open', 'error']);
  assert.equal(all(f.flat(), 'PosterCard').length, 1);
});
