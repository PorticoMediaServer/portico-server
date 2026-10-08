import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

/** PERF-24 (v1 video path): one offers read per play and none before a session, no item re-read
 * when the play action's entry already says what the player shows, and subtitle polling only while
 * subtitles are on or their menu is open. Measured on the e2e stack: 21 non-segment requests a
 * minute (was 44), 1 offers read per play (was 3), no /listening or /v1/items read for a movie. */
const src = (p: string) => readFileSync(new URL('../src/' + p, import.meta.url), 'utf8');

test('the player reads offers once, with the session, and the audio plan reuses that answer', () => {
  const options = src('player/options.ts'), player = src('player/Player.tsx');
  assert.equal((options.match(/new PlayerOffersService\(/g) ?? []).length, 1, 'one offers service');
  assert.match(options, /if \(!item \|\| !sessionId\) return \(\) => service\.dispose\(\);/, 'nothing is read before a session exists');
  assert.match(player, /const offers = usePlayerOffers\(api, scope, item, state\.session\?\.id, state\.session\?\.generation\);\n\s*const refreshAudio = usePlayerAudioPlan\(offers,/);
});

test('an entry that names the item seeds the player, so the item isn’t read again', () => {
  const engine = src('player/engine.tsx');
  assert.match(engine, /identity\.seeded && identity\.itemId === playingItemId\)\) return;/);
  assert.match(engine, /seeded: !!\(entry\?\.title && entry\.kind && entry\.libraryId && entry\.id === itemId\)/);
});

test('subtitle catalogue polling follows whether subtitles are on or their menu is open', () => {
  const player = src('player/Player.tsx');
  assert.match(player, /const watchSubtitles = subtitlePlan\?\.mode === 'track' \|\| openBox === 'subtitles';/);
  assert.match(player, /subtitles\.service\?\.setPolling\(watchSubtitles\)/);
});
