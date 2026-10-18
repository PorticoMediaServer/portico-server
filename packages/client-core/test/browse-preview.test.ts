import test from 'node:test';
import assert from 'node:assert/strict';
import {browsePreview, browseStart, homeHero, previewOverview, PREVIEW_OVERVIEW_LIMIT} from '../src/presentation/index.ts';
import {createI18n} from '../../i18n/src/index.ts';
import type {ContentEntry} from '../src/library-content.ts';

// The television's passive browsing preview (Justin, 18 Oct 2026): the focused card described
// from the row's own entry, with no request per card, worded as Home's hero is.
const {t} = createI18n();
const duration = (seconds: number) => `${Math.round(seconds / 60)}m`;
const episode: ContentEntry = {id: 'e1', kind: 'episode', title: 'Ozymandias', subtitle: 'Breaking Bad · Season 5', seasonNumber: 5, episodeNumber: 14, year: 2013, contentRating: 'TV-MA', duration: 2820, progressSeconds: 1410, genres: ['Crime', 'Drama', 'Thriller'], backdropUrl: '/b', posterUrl: '/p', overview: 'Walt goes on the run.', playback: {itemId: 'e1'}};

test('browsePreview: words the focused card as the hero would, from the entry alone', () => {
  const preview = browsePreview(episode, t, {duration});
  assert.equal(preview.title, 'Breaking Bad');
  assert.equal(preview.line, 'S5 E14 · Ozymandias');
  assert.equal(preview.meta, '2013 · TV-MA · 47m · Crime · Drama');
  assert.equal(preview.overview, 'Walt goes on the run.');
  assert.equal(preview.progress, 0.5);
  assert.deepEqual([preview.art, preview.artUrl], ['backdrop', '/b']);
  // An ordinary card opens its page when pressed: the preview promises nothing else.
  assert.equal(preview.resume, undefined);
  // Its words are the hero's words for the same entry.
  const hero = homeHero({rows: [{id: 'continue', title: 'Continue', kind: 'rail', entries: [episode]}], hero: {rowId: 'continue', entryId: 'e1'}} as never, t, {duration})!;
  assert.deepEqual([hero.title, hero.line, hero.meta, hero.playLabel], [preview.title, preview.line, preview.meta, t('entry.resumeRemaining', {remaining: '24m'})]);
});

test('browsePreview: a Continue card says it resumes; a card with nothing playable never does', () => {
  assert.equal(browsePreview(episode, t, {duration, resume: true}).resume, t('entry.resumeRemaining', {remaining: '24m'}));
  const next = {...episode, progressSeconds: undefined};
  assert.equal(browsePreview(next, t, {duration, resume: true}).resume, t('action.play'), 'a next episode not yet started plays');
  assert.equal(browsePreview({...episode, playback: undefined}, t, {duration, resume: true}).resume, undefined);
});

test('browsePreview: art falls back from backdrop to cover to none; a collection says its count', () => {
  const album: ContentEntry = {id: 'a', kind: 'album', title: 'Kind of Blue', artist: {id: 'm', name: 'Miles Davis'}, year: 1959, posterUrl: '/c'};
  const preview = browsePreview(album, t, {duration});
  assert.deepEqual([preview.art, preview.artUrl, preview.coverShape, preview.line], ['cover', '/c', 'square', 'Miles Davis']);
  const collection = browsePreview({id: 'c', kind: 'collection', title: 'Pixar', count: 12}, t, {duration});
  assert.deepEqual([collection.art, collection.artUrl], ['none', undefined]);
  assert.equal(collection.meta, t('title.itemCount', {count: 12}), 'no year or genres: the card\'s own caption');
});

test('previewOverview: one line, bounded by code points, never splitting a surrogate pair', () => {
  assert.equal(previewOverview(undefined), undefined);
  assert.equal(previewOverview('  \n\t '), undefined);
  assert.equal(previewOverview('First line.\n\nSecond   line.'), 'First line. Second line.');
  const emoji = '🎬'.repeat(PREVIEW_OVERVIEW_LIMIT + 5);
  const bounded = previewOverview(emoji)!;
  assert.equal([...bounded].length, PREVIEW_OVERVIEW_LIMIT + 1, 'the limit, then an ellipsis');
  assert.ok(bounded.endsWith('🎬…'), 'cut between code points');
  assert.equal(previewOverview('a'.repeat(PREVIEW_OVERVIEW_LIMIT)), 'a'.repeat(PREVIEW_OVERVIEW_LIMIT), 'exactly the limit is kept whole');
  // A huge synopsis is cut before any work over the whole of it.
  const long = previewOverview('word '.repeat(200_000))!;
  assert.ok([...long].length <= PREVIEW_OVERVIEW_LIMIT + 1 && long.endsWith('word…'), 'the cut drops the trailing space before the ellipsis');
});

test('browseStart: the server\'s Continue card, else the first that resumes, else the first usable card', () => {
  const card = (id: string, extra: Partial<ContentEntry> = {}): ContentEntry => ({id, kind: 'movie', title: id, playback: {itemId: id}, ...extra});
  const added = {id: 'recently_added', entries: [card('r1', {available: false}), card('r2')]};
  const watching = {id: 'continue', entries: [card('c1', {available: false}), card('c2')]};
  const listening = {id: 'continue_listening', entries: [card('l1')]};
  // The hero the server named, when it will resume.
  assert.deepEqual(browseStart([added, watching, listening], {rowId: 'continue_listening', entryId: 'l1'}), {rowIndex: 2, entryIndex: 0});
  // A named card whose files are gone is not where browsing starts: the first Continue card that resumes is.
  assert.deepEqual(browseStart([added, watching, listening], {rowId: 'continue', entryId: 'c1'}), {rowIndex: 1, entryIndex: 1});
  // A hero that is not in the rows shown (removed a moment ago) is passed over.
  assert.deepEqual(browseStart([added, listening], {rowId: 'continue', entryId: 'c2'}), {rowIndex: 1, entryIndex: 0});
  // No Continue card: the first usable card of the first row that has one.
  assert.deepEqual(browseStart([{id: 'empty', entries: []}, added]), {rowIndex: 1, entryIndex: 1});
  // Nothing usable anywhere: still somewhere to start.
  assert.deepEqual(browseStart([{id: 'gone', entries: [card('g', {available: false})]}]), {rowIndex: 0, entryIndex: 0});
  assert.equal(browseStart([]), undefined);
  assert.equal(browseStart([{id: 'empty', entries: []}]), undefined);
});
