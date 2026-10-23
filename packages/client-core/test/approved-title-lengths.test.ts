import test from 'node:test';
import assert from 'node:assert/strict';
import {formatMessage} from '../../i18n/src/messageformat.ts';
import {shared} from '../../i18n/src/catalog/en-US/next/shared.ts';
import {wireDisplayDurations} from '../src/wire-units.ts';
import {elapsedTime} from '../src/media-format.ts';
import {formatClock} from '../src/presentation/content.ts';
import {cardText, timelineFacts, titleFactParts, type KitTranslate} from '../src/presentation/kit-model/index.ts';
import {clockText, durationText} from '../src/presentation/kit-model/text.ts';

const t: KitTranslate = (id, ...args) => formatMessage(shared[id], args[0]);
const env = {t, locale: 'en-US'};

// These are approved fixture examples, not provider metadata.
const lengths = [
  {title: 'Infected', seconds: 3186, clock: '53:06', whole: '53 min'},
  {title: 'Dreams', seconds: 254, clock: '4:14', whole: '4 min'},
  {title: 'Lawnmower Dog', seconds: 1260, clock: '21:00', whole: '21 min'},
  {title: "I'm Used to It", seconds: 1440, clock: '24:00', whole: '24 min'},
] as const;

for (const value of lengths) test(`${value.title} uses one exact length for clocks and whole-minute facts`, () => {
  const skipped: string[] = [];
  const parsed = wireDisplayDurations({durationMs: value.seconds * 1000}, {duration: 'durationMs'}, part => skipped.push(part)) as {duration: number};
  assert.deepEqual(parsed, {duration: value.seconds});
  assert.deepEqual(skipped, []);
  for (const clock of [clockText, elapsedTime, formatClock]) {
    assert.equal(clock(parsed.duration), value.clock);
    assert.equal(clock(parsed.duration + .999), value.clock);
  }
  assert.equal(durationText(parsed.duration, t), value.whole);
  assert.equal(titleFactParts({durationSeconds: parsed.duration}, env).find(part => part.id === 'duration')?.text, value.whole);
});

test('approved continuation positions retain exact progress and rounded remaining labels', () => {
  for (const value of [
    {show: 'Rick and Morty', title: 'Lawnmower Dog', episode: 2, seconds: 1260, position: 769, remaining: 491, label: '8 min left'},
    {show: 'Solo Leveling', title: "I'm Used to It", episode: 1, seconds: 1440, position: 634, remaining: 806, label: '13 min left'},
  ]) {
    const model = cardText({id: `episode-${value.episode}`, kind: 'episode', title: value.title, show: {title: value.show},
      episode: {mode: 'seasonal', season: 1, episode: value.episode}, durationSeconds: value.seconds, positionSeconds: value.position}, 'continuation', env);
    assert.equal(model.title, value.show);
    assert.equal(model.detail.find(part => part.id === 'remaining')?.text, value.label);
    assert.equal(model.progress, value.position / value.seconds);
    assert.deepEqual(timelineFacts(value.seconds, value.position), {progress: value.position / value.seconds, remainingSeconds: value.remaining});
  }
});

test('song rows keep exact seconds while whole-minute facts round across hour boundaries', () => {
  const song = cardText({id: 'song', kind: 'song', title: 'Dreams', artist: 'Fleetwood Mac', album: 'Rumours', durationSeconds: 254}, 'song-row', env);
  assert.equal(song.detail.find(part => part.id === 'duration')?.text, '4:14');
  assert.equal(clockText(3599.999), '59:59');
  assert.equal(clockText(3600), '1:00:00');
  assert.equal(clockText(5160.999), '1:26:00');
  assert.equal(durationText(3599.999, t), '1 h');
  assert.equal(durationText(5129.999, t), '1 h 25 min');
  assert.equal(durationText(5130, t), '1 h 26 min');
});
