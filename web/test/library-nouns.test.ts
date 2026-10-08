import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {captionFor} from '@core/presentation/index.ts';
import {browseCountLabel, isQuickFilterVisible, kindCaptionFor, kindOwnsCaption, libraryKindSingular, quickFilterLabelForKind} from '../src/app/library-words.ts';

const t = defaultI18n.t;
const quick = (id: string, labelKey: string) => ({id, labelKey});

test('WEB-LIB-03: quick filter labels speak the library kind (US English)', () => {
  const rows: Array<[string, string | undefined, string]> = [
    ['filter.unwatched', 'music', 'Unplayed'],
    ['filter.watched', 'music', 'Played'],
    ['filter.favorites', 'music', 'Favorites'],
    ['filter.unwatched', 'audiobook', 'Not started'],
    ['filter.inProgress', 'audiobook', 'In progress'],
    ['filter.watched', 'audiobook', 'Finished'],
    ['filter.favorites', 'audiobook', 'Favorites'],
    ['filter.unwatched', 'movie', 'Unwatched'],
    ['filter.watched', 'movie', 'Watched'],
    ['filter.favorites', 'movie', 'Favorites'],
    ['filter.watchlist', 'movie', 'My List'],
  ];
  for (const [labelKey, kind, expected] of rows) {
    assert.equal(quickFilterLabelForKind(t, labelKey, kind), expected, `${labelKey} in ${kind}`);
  }
});

test('WEB-LIB-03: quick filter visibility (no My List for music, no Missing/Available in the bar)', () => {
  const cases: Array<[{id: string; labelKey: string}, string | undefined, string | undefined, boolean]> = [
    [quick('unwatched', 'filter.unwatched'), 'music', 'artists', true],
    [quick('watched', 'filter.watched'), 'music', 'artists', true],
    [quick('favorites', 'filter.favorites'), 'music', 'artists', true],
    [quick('watchlist', 'filter.watchlist'), 'music', 'artists', false],
    [quick('in-progress', 'filter.inProgress'), 'music', 'artists', false],
    [quick('missing', 'filter.missing'), 'music', 'artists', false],
    [quick('available', 'filter.available'), 'music', 'artists', false],
    [quick('unwatched', 'filter.unwatched'), 'audiobook', 'books', true],
    [quick('in-progress', 'filter.inProgress'), 'audiobook', 'books', true],
    [quick('watched', 'filter.watched'), 'audiobook', 'books', true],
    [quick('missing', 'filter.missing'), 'audiobook', 'books', false],
    [quick('unwatched', 'filter.unwatched'), 'movie', 'collections', false],
    [quick('favorites', 'filter.favorites'), 'movie', 'collections', false],
  ];
  for (const [filter, kind, pivot, expected] of cases) {
    assert.equal(isQuickFilterVisible(filter, kind, pivot), expected, `${filter.id} in ${kind}/${pivot}`);
  }
});

test('WEB-LIB-03: summary noun follows the pivot', () => {
  assert.equal(browseCountLabel(t, 10, 'artists', 'music'), '10 artists');
  assert.equal(browseCountLabel(t, 1, 'artists', 'music'), '1 artist');
  assert.equal(browseCountLabel(t, 80, 'movies', 'movie'), '80 movies');
  assert.equal(browseCountLabel(t, 12, 'books', 'audiobook'), '12 books');
  assert.equal(browseCountLabel(t, 11, 'songs', 'music'), '11 songs');
  assert.equal(browseCountLabel(t, 4, 'albums', 'music'), '4 albums');
});

test('WEB-LIB-03: library kind singular for eyebrows', () => {
  assert.equal(libraryKindSingular('movie'), 'Movie');
  assert.equal(libraryKindSingular('music'), 'Music');
  assert.equal(libraryKindSingular('audiobook'), 'Audiobook');
});

test('WEB-LIB-03: kind-aware captions, never 0 items', () => {
  assert.equal(kindCaptionFor(t, {kind: 'artist', count: 4}), '4 songs', 'the server counts an artist’s member items: songs');
  assert.equal(kindCaptionFor(t, {kind: 'artist', count: 1}), '1 song');
  assert.equal(kindCaptionFor(t, {kind: 'artist', count: 0}), undefined);
  assert.equal(kindCaptionFor(t, {kind: 'artist'}), undefined);
  assert.equal(kindCaptionFor(t, {kind: 'album', subtitle: 'Adele · 2011', count: 11}), '2011 · 11 songs');
  assert.equal(kindCaptionFor(t, {kind: 'album', subtitle: 'Adele', count: 11}), '11 songs');
  assert.equal(kindCaptionFor(t, {kind: 'album', subtitle: 'Adele', count: 0}), undefined);
  const book = kindCaptionFor(t, {kind: 'book', subtitle: 'Frank Herbert', duration: 33120});
  assert.ok(book?.startsWith('Frank Herbert · '), `book caption: ${book}`);
  assert.equal(kindCaptionFor(t, {kind: 'book', subtitle: 'Frank Herbert', duration: 0}), 'Frank Herbert');
  assert.equal(kindCaptionFor(t, {kind: 'book', count: 0}), undefined);
});

test('Follow-up Q1: shared card/list call sites (kind-aware, never 0 items)', () => {
  // The exact expression EntryCard and EntryListRow now render, exercised with
  // an album, an artist and a book entry (plus a movie that falls through).
  // A zero count never renders "0 items": the fallback keeps the subtitle.
  const caption = (entry: Parameters<typeof kindCaptionFor>[1] & {count?: number; subtitle?: string}) =>
    kindCaptionFor(t, entry) ?? (kindOwnsCaption(entry.kind) || entry.count === 0 ? entry.subtitle : captionFor(entry as Parameters<typeof captionFor>[0]));
  assert.equal(caption({kind: 'album', subtitle: 'Adele · 2011', count: 11}), '2011 · 11 songs');
  assert.equal(caption({kind: 'artist', count: 4}), '4 songs');
  const book = caption({kind: 'book', subtitle: 'Frank Herbert', duration: 33120});
  assert.ok(book?.startsWith('Frank Herbert · '), `book caption: ${book}`);
  assert.equal(caption({kind: 'artist', count: 0}), undefined);
  assert.equal(caption({kind: 'book', count: 0}), undefined);
  assert.equal(caption({kind: 'movie', subtitle: '1998', count: 0}), '1998');
  assert.equal(caption({kind: 'book', count: 1}), undefined, 'a book without author or length says nothing, not "1 item"');
});
