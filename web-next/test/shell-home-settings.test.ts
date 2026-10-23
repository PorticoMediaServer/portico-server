import test from 'node:test';
import assert from 'node:assert/strict';
import type {PreferenceSnapshot} from '@core/preferences.ts';
import {shellPlaybackSettings} from '../src/shell/settings.ts';

const preferences = {effective: {
  'playback.defaultSpeed': 1.25, 'audiobooks.defaultSpeed': 1.75,
  'playback.introSkip': 'auto', 'playback.creditsSkip': 'ask', 'playback.recapSkip': 'off',
  'music.gapless': true, 'music.crossfadeMs': 3000, 'music.audioNormalization': 'album',
  'music.shuffleDefault': true, 'music.repeatDefault': 'all',
  'quality.home': '2160p', 'quality.remote': '720p',
}} as unknown as PreferenceSnapshot;

test('the actual server resolves the network quality lane while video and audiobook rates remain distinct', () => {
  const settings = shellPlaybackSettings(preferences, {userAgent: 'Desktop', maxTouchPoints: 0});
  assert.equal(settings.defaultQuality, undefined);
  assert.equal(settings.videoSpeed(), 1.25); assert.equal(settings.audiobookSpeed(), 1.75);
  assert.deepEqual(settings.skipModes(), {intro: 'auto', credits: 'ask', recap: 'off'});
  assert.deepEqual(settings.audioEffects(), {gapless: true, crossfadeSeconds: 3, normalization: 'album'});
  assert.deepEqual(settings.musicDefaults?.(), {shuffle: true, repeat: 'all'});
  assert.equal(settings.webVolume(), true);
});
test('web volume is withheld on actual iPhone and desktop-mode iPad platforms', () => {
  assert.equal(shellPlaybackSettings(preferences, {userAgent: 'iPhone', maxTouchPoints: 5}).webVolume(), false);
  assert.equal(shellPlaybackSettings(preferences, {userAgent: 'Macintosh', maxTouchPoints: 5}).webVolume(), false);
  assert.equal(shellPlaybackSettings(preferences, {userAgent: 'Macintosh', maxTouchPoints: 0}).webVolume(), true);
});
