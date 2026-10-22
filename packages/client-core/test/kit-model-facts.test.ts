import test from 'node:test';
import assert from 'node:assert/strict';
import {formatMessage} from '../../i18n/src/messageformat.ts';
import {shared} from '../../i18n/src/catalog/en-US/next/shared.ts';
import {cardText, titleFactParts, episodeLabel, playbackIdentity, timelineFacts, type CardContext, type EpisodeNumbering, type FactPart, type KitTranslate} from '../src/presentation/kit-model/index.ts';

const t: KitTranslate = (id, ...args) => formatMessage(shared[id], args[0]);
const env = {t, locale: 'en-US'};
const parts = (p: readonly FactPart[]) => p.map(x => [x.id, x.text, x.role]);

test('title facts keep the exact year, runtime, genre, rating and season order without joining', () => {
  assert.deepEqual(parts(titleFactParts({year: 2024, durationSeconds: 9960, genres: ['Science Fiction', 'Adventure'], contentRating: 'PG-13'}, env)), [
    ['year', '2024', 'value'], ['duration', '2 h 46 min', 'value'], ['genre-0', 'Science Fiction', 'supplement'], ['genre-1', 'Adventure', 'supplement'], ['content-rating', 'PG-13', 'supplement'],
  ]);
  assert.deepEqual(parts(titleFactParts({year: 2022, genres: ['Crime', 'Drama', 'Mystery'], seasonCount: 1}, env)).at(-1), ['season-count', '1 season', 'supplement']);
  assert.deepEqual(titleFactParts({durationSeconds: NaN, year: 0, contentRating: '', genres: [''], seasonCount: 0}, env), []);
});

const modes: readonly [EpisodeNumbering, string, string][] = [
  [{mode: 'seasonal', season: 1, episode: 2}, 'Episode 2', 'S1 E2'],
  [{mode: 'absolute', number: 37}, 'Episode 37', 'Episode 37'],
  [{mode: 'date', day: '2026-10-21'}, 'Oct 21, 2026', 'Oct 21, 2026'],
  [{mode: 'special'}, 'Special', 'Special'],
];
const contexts: readonly CardContext[] = ['ordinary', 'continuation', 'search', 'in-show', 'song-row', 'dock'];
for (const [numbering, local, mixed] of modes) for (const context of [...contexts, 'player', 'up-next'] as const) test(`${numbering.mode} episode label in ${context}`, () => {
  assert.deepEqual(episodeLabel(numbering, context, env), {id: 'episode-label', text: context === 'in-show' ? local : mixed, role: 'context'});
  if (context === 'player' || context === 'up-next') return;
  const model = cardText({id: 'episode-real', kind: 'episode', title: 'Infected', show: {id: 'show-real', title: 'The Last of Us'}, episode: numbering, durationSeconds: 53 * 60, positionSeconds: 29 * 60}, context, env);
  if (context === 'continuation' || context === 'dock') {
    assert.equal(model.title, 'The Last of Us');
    assert.deepEqual(parts(model.caption), [['episode-label', mixed, 'context'], ['remaining', '24 min left', 'supplement']]);
    assert.deepEqual(model.detail, []);
  } else {
    assert.equal(model.title, 'Infected');
    assert.deepEqual(parts(model.caption), [...(context === 'in-show' ? [] : [['show', 'The Last of Us', 'identity']]), ['episode-label', context === 'in-show' ? local : mixed, 'context']]);
    assert.deepEqual(parts(model.detail), [['duration', '53 min', 'supplement']]);
  }
  assert.equal(model.id, 'episode-real'); assert.equal(model.progress, 29 / 53);
});

test('incomplete episode numbering is omitted; calendar days validate and retain the full year', () => {
  for (const n of [undefined, {mode: 'seasonal'}, {mode: 'seasonal', season: 1, episode: -1}, {mode: 'absolute', number: Infinity}, {mode: 'date', day: '2026-02-29'}, {mode: 'date', day: '2026-02-30'}, {mode: 'date', day: '2026-13-01'}, {mode: 'date', day: 'Oct 21'}] as const) assert.equal(episodeLabel(n, 'in-show', env), undefined);
  assert.equal(episodeLabel({mode: 'date', day: '2024-02-29'}, 'in-show', env)?.text, 'Feb 29, 2024');
  assert.equal(episodeLabel({mode: 'seasonal', episode: 8}, 'search', env)?.text, 'Episode 8');
  assert.equal(episodeLabel({mode: 'seasonal', season: 0, episode: 1}, 'search', env)?.text, 'S0 E1');
  assert.equal(episodeLabel({mode: 'absolute', number: 1043}, 'in-show', env)?.text, 'Episode 1043');
});

test('ordinary movie/show/season and special-season captions retain identity', () => {
  assert.deepEqual(cardText({id: 'm', kind: 'movie', title: 'Dune: Part Two', year: 2024}, 'ordinary', env), {id: 'm', title: 'Dune: Part Two', caption: [{id: 'year', text: '2024', role: 'supplement'}], detail: []});
  assert.equal(cardText({id: 's', kind: 'show', title: 'Show', seasonCount: 6}, 'ordinary', env).caption[0]?.text, '6 seasons');
  assert.deepEqual(parts(cardText({id: 's1', kind: 'season', title: 'Season one', seasonNumber: 1, show: {title: 'Show'}}, 'ordinary', env).caption), [['season', 'Season 1', 'context']]);
  assert.equal(cardText({id: 's0', kind: 'season', title: 'Extras', seasonNumber: 0}, 'ordinary', env).caption[0]?.text, 'Specials');
});

test('song rows separate artist, album, year, counts and clocks; dock stays quiet', () => {
  const song = {id: 'song', kind: 'song', title: 'DNA.', artist: 'Kendrick Lamar', album: 'DAMN.', year: 2017, durationSeconds: 185} as const;
  assert.deepEqual(parts(cardText(song, 'song-row', env).caption), [['artist', 'Kendrick Lamar', 'identity'], ['album', 'DAMN.', 'context'], ['year', '2017', 'supplement']]);
  assert.deepEqual(parts(cardText(song, 'song-row', env).detail), [['duration', '3:05', 'supplement']]);
  assert.deepEqual(parts(cardText(song, 'dock', env).caption), [['artist', 'Kendrick Lamar', 'identity']]);
  assert.deepEqual(cardText(song, 'dock', env).detail, []);
  assert.deepEqual(parts(cardText({...song, kind: 'album', songCount: 4, durationSeconds: 900}, 'ordinary', env).detail), [['song-count', '4 songs', 'supplement'], ['duration', '15 min', 'supplement']]);
});

test('music and audiobook title facts follow their own metadata order with real counts only', () => {
  assert.deepEqual(parts(titleFactParts({artist: 'The Weeknd', year: 2020, genres: ['R&B/Soul'], songCount: 4, durationSeconds: 900}, env, 'music')), [['artist', 'The Weeknd', 'identity'], ['year', '2020', 'value'], ['genre-0', 'R&B/Soul', 'supplement'], ['song-count', '4 songs', 'supplement'], ['duration', '15 min', 'value']]);
  assert.deepEqual(parts(titleFactParts({author: 'J. K. Rowling', narrator: 'Stephen Fry', durationSeconds: 30000, chapterCount: 17}, env, 'audiobook')), [['author', 'J. K. Rowling', 'identity'], ['duration', '8 h 20 min', 'value'], ['narrator', 'Read by Stephen Fry', 'context'], ['chapter-count', '17 chapters', 'supplement']]);
});

test('song counts never become album counts; unknown counts and metadata disappear', () => {
  assert.deepEqual(parts(cardText({id: 'a', kind: 'artist', title: 'The Weeknd', songCount: 8}, 'ordinary', env).caption), [['song-count', '8 songs', 'supplement']]);
  assert.deepEqual(parts(cardText({id: 'a', kind: 'artist', title: 'The Weeknd', albumCount: 1, songCount: 4}, 'ordinary', env).caption), [['album-count', '1 album', 'supplement'], ['song-count', '4 songs', 'supplement']]);
  assert.deepEqual(cardText({id: 'a', kind: 'artist', title: 'Artist', albumCount: 0, songCount: NaN}, 'ordinary', env).caption, []);
  assert.equal(cardText({id: 'c', kind: 'collection', title: 'Collection', itemCount: 10000, countIsLowerBound: true}, 'ordinary', env).caption[0]?.text, '10,000+ items');
  assert.deepEqual(cardText({id: 'c', kind: 'category', title: 'Category'}, 'ordinary', env).caption, []);
});

test('audiobook book keeps author, narrator, chapter and whole-book remaining separate', () => {
  const model = cardText({id: 'book', kind: 'book', title: 'The Martian', author: 'Andy Weir', narrator: 'R.C. Bray', chapterNumber: 6, chapterCount: 19, durationSeconds: 36000, positionSeconds: 19080}, 'continuation', env);
  assert.deepEqual(parts(model.caption), [['author', 'Andy Weir', 'identity']]);
  assert.deepEqual(parts(model.detail), [['narrator', 'Read by R.C. Bray', 'context'], ['chapter', 'Chapter 6 of 19', 'context'], ['remaining', '4 h 42 min left', 'supplement']]);
  assert.deepEqual(cardText({id: 'b', kind: 'book', title: 'No metadata'}, 'ordinary', env).caption, []);
});

for (const bad of [undefined, NaN, Infinity, -Infinity, -1, 0]) test(`unknown or invalid duration ${String(bad)} invents no runtime/progress`, () => {
  assert.deepEqual(timelineFacts(bad, 60), {});
  const m = cardText({id: 'm', kind: 'movie', title: 'Film', durationSeconds: bad, positionSeconds: 60}, 'continuation', env);
  assert.deepEqual(m.caption, []); assert.equal('progress' in m, false);
});
for (const bad of [undefined, NaN, Infinity, -Infinity, -1]) test(`unknown or invalid position ${String(bad)} invents no remaining/progress`, () => {
  assert.deepEqual(timelineFacts(3600, bad), {});
});
test('real zero, overshoot and sub-minute timelines are bounded truthfully', () => {
  assert.deepEqual(timelineFacts(3600, 0), {progress: 0, remainingSeconds: 3600});
  assert.deepEqual(timelineFacts(3600, 4000), {progress: 1, remainingSeconds: 0});
  assert.deepEqual(cardText({id: 'm', kind: 'movie', title: 'Film', durationSeconds: 3600, positionSeconds: 0}, 'continuation', env).caption, []);
  assert.equal(cardText({id: 'm', kind: 'movie', title: 'Film', durationSeconds: 60, positionSeconds: 40}, 'continuation', env).caption[0]?.text, '20 s left');
});

test('full and dock identities preserve long show/episode names without renderer truncation', () => {
  const show = 'The Inheritance: A Very Long Show Title That Should Truncate Gracefully In Every Player';
  const title = 'An episode with a long provider title · punctuation intact';
  for (const context of ['full', 'dock'] as const) assert.deepEqual(playbackIdentity({kind: 'video', item: {id: 'e', kind: 'episode', title, show: {id: 's', title: show}, episode: {mode: 'seasonal', season: 1, episode: 2}}}, context, env), {id: 'e', kind: 'video', title: show, subtitle: [{id: 'episode-label', text: 'S1 E2', role: 'context'}, {id: 'episode-title', text: title, role: 'identity'}]});
  assert.equal(cardText({id: 'a', kind: 'album', title: 'Long title', artist: title}, 'ordinary', env).caption[0]?.text, title);
});

test('music, audiobook, VOD and true channel identities have distinct supplied facts', () => {
  const item = {id: 's', kind: 'song', title: 'Dreams', artist: 'Fleetwood Mac', album: 'Rumours'} as const;
  assert.deepEqual(parts(playbackIdentity({kind: 'music', item}, 'full', env).subtitle), [['artist', 'Fleetwood Mac', 'identity'], ['album', 'Rumours', 'context']]);
  assert.deepEqual(parts(playbackIdentity({kind: 'music', item}, 'dock', env).subtitle), [['artist', 'Fleetwood Mac', 'identity']]);
  assert.deepEqual(playbackIdentity({kind: 'video', item: {id: 'f', kind: 'movie', title: 'Dune'}}, 'dock', env).subtitle, []);
  const channel = {kind: 'channel', id: 'c1', sourceId: 's1', channelName: 'Summit Sports', programmeTitle: 'Cycling: Mountain Stage'} as const;
  assert.deepEqual(playbackIdentity(channel, 'dock', env), {id: 'c1', sourceId: 's1', kind: 'channel', title: 'Cycling: Mountain Stage', subtitle: [{id: 'channel', text: 'Summit Sports', role: 'identity'}]});
  assert.equal(playbackIdentity({...channel, live: true}, 'full', env).subtitle.at(-1)?.text, 'Live');
  assert.equal(playbackIdentity({...channel, programmeTitle: undefined}, 'full', env).title, 'Summit Sports');
  assert.deepEqual(parts(playbackIdentity({kind: 'audiobook', item: {id: 'b', kind: 'book', title: 'The Hobbit', author: 'J. R. R. Tolkien', narrator: 'Andy Serkis'}}, 'full', env).subtitle), [['author', 'J. R. R. Tolkien', 'identity'], ['narrator', 'Read by Andy Serkis', 'context']]);
});
