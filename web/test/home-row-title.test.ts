import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import * as presentation from '@core/presentation/index.ts';
import {componentModule, hooks} from './helpers/component-harness.mjs';

const app = await componentModule(new URL('../src/app/home.ts', import.meta.url), {
  react: hooks().react, '@core/home.ts': {}, './session': {}, './viewer-scope': {}, './not-interested': {}, './i18n': {currentI18n: () => defaultI18n}, '@core/presentation/index.ts': presentation, '@i18n': {},
});
const homeRowTitle = app.homeRowTitle;

// P7: titleText first, then the catalogue by row id, then the server's title.
const none = () => undefined;
test('Home rows are named by titleText, with the id table and server title as fallbacks', () => {
  assert.equal(homeRowTitle('for_you:genre:drama', 'Drama for you', none, {code: 'home.row.genreForYou', params: {genre: 'Drama'}, fallback: 'Drama for you'}), 'Drama for you');
  assert.equal(homeRowTitle('for_you:person:tmdb:1', 'More with Ada', none, {code: 'home.row.moreWith', params: {name: 'Ada Lovelace'}, fallback: 'More with Ada'}), 'More with Ada Lovelace');
  assert.equal(homeRowTitle('for_you', 'Picks for you', none, {code: 'home.row.picksForYou', fallback: 'Picks for you'}), 'Picks for you');
  assert.equal(homeRowTitle('for_you:future', 'A future row', none, {code: 'home.row.notShippedYet', fallback: 'A future row'}), 'A future row');
  assert.equal(homeRowTitle('recent_lib', 'Recently added in Films', () => 'Films', {code: 'home.row.recentlyAddedIn', params: {library: 'Films'}, fallback: 'Recently added in Films'}), 'Recently added in Films');
  // Fixed rows without titleText keep their catalogue names.
  assert.equal(homeRowTitle('continue', 'Continue watching', none), 'Continue Watching');
  assert.equal(homeRowTitle('trending_now', undefined, none), 'Trending now');
});
