import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {albumHeroFacts, artistHeroFacts, discNumberFromSubtitle, seeAllTarget, songArtistFromSubtitle, trackSubtitleForAlbum} from '../src/app/library-words.ts';

const t = defaultI18n.t;

test('WEB-DETAIL-10: song artist parsing (guest artists only)', () => {
  assert.equal(songArtistFromSubtitle('Disc 1 · Track 3 · Adele'), 'Adele');
  assert.equal(songArtistFromSubtitle('Track 3 · Adele'), 'Adele');
  assert.equal(songArtistFromSubtitle('Adele'), 'Adele');
  assert.equal(songArtistFromSubtitle('Disc 1 · Track 3'), undefined);
  assert.equal(songArtistFromSubtitle(undefined), undefined);
  assert.equal(trackSubtitleForAlbum('Disc 1 · Track 3 · Adele', 'Adele'), undefined);
  assert.equal(trackSubtitleForAlbum('Disc 1 · Track 3 · Beyoncé', 'Adele'), 'Beyoncé');
  assert.equal(trackSubtitleForAlbum('Disc 1 · Track 3', 'Adele'), undefined);
});

test('WEB-DETAIL-10: disc numbers for multi-disc subheadings', () => {
  assert.equal(discNumberFromSubtitle('Disc 2 · Track 1 · Adele'), 2);
  assert.equal(discNumberFromSubtitle('Track 1 · Adele'), undefined);
  assert.equal(discNumberFromSubtitle(undefined), undefined);
});

test('WEB-DETAIL-10: album and artist hero facts', () => {
  assert.deepEqual(albumHeroFacts(t, {year: '2011', songCount: 11, durationSeconds: 2880}), ['Album', '2011', '11 songs', '48m']);
  assert.deepEqual(albumHeroFacts(t, {songCount: 11}), ['Album', '11 songs']);
  assert.deepEqual(albumHeroFacts(t, {}), ['Album']);
  assert.deepEqual(artistHeroFacts(t, {albumCount: 4, songCount: 40}), ['4 albums', '40 songs']);
  assert.deepEqual(artistHeroFacts(t, {}), []);
});

test('WEB-LIB-05: See all keeps the row meaning (recent sorts by date added)', () => {
  assert.deepEqual(seeAllTarget({id: 'recently_added', headingKey: 'recent'}), {view: 'browse', sort: 'added', direction: 'desc'});
  assert.deepEqual(seeAllTarget({id: 'recent', headingKey: 'recent'}), {view: 'browse', sort: 'added', direction: 'desc'});
  // Rows with no browse equivalent open as a full-page row view that pages
  // the row's own cursor.
  assert.deepEqual(seeAllTarget({id: 'recommended', headingKey: 'recommended'}), {view: 'discover', row: 'recommended'});
  assert.deepEqual(seeAllTarget({id: 'trending_now', headingKey: 'trending'}), {view: 'discover', row: 'trending_now'});
});
