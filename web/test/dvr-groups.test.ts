import test from 'node:test';
import assert from 'node:assert/strict';
import {groupRecordings, isExceptionBadge} from '../src/screens/live/dvr-groups.ts';

const rec = (id: string, seriesId: string, state = 'completed'): any => ({id, programme: {seriesId, title: `Show ${seriesId || id}`}, state});

test('recorded groups by seriesId; shows without one stay single', () => {
  const groups = groupRecordings([rec('a', 's1'), rec('b', 's1'), rec('c', 's1'), rec('d', ''), rec('e', '')]);
  assert.deepEqual(groups.map(g => g.items.map(r => r.id)), [['a', 'b', 'c'], ['d'], ['e']]);
  assert.equal(groups[0]!.key, 's1');
});

test('recorded groups preserve guide order', () => {
  const groups = groupRecordings([rec('b', 's2'), rec('a', 's1'), rec('c', 's2')]);
  assert.deepEqual(groups.map(g => g.key), ['s2', 's1']);
  assert.deepEqual(groups[0]!.items.map(r => r.id), ['b', 'c']);
});

test('the server\'s seriesId groups recordings whose programmes differ, and seriesTitle names the row', () => {
  const a = {...rec('a', 'p1'), seriesId: 'show', seriesTitle: 'The Show'}, b = {...rec('b', 'p2'), seriesId: 'show'}, c = rec('c', '');
  const groups = groupRecordings([a, b, c]);
  assert.deepEqual(groups.map(g => g.items.map(r => r.id)), [['a', 'b'], ['c']]);
  assert.equal(groups[0]!.title, 'The Show');
  assert.equal(groups[1]!.title, 'Show c');
});

test('badges only for exceptions in Recorded', () => {
  assert.equal(isExceptionBadge({state: 'failed'} as any), true);
  assert.equal(isExceptionBadge({state: 'incomplete-playable'} as any), true);
  assert.equal(isExceptionBadge({state: 'completed'} as any), false);
  assert.equal(isExceptionBadge({state: 'scheduled'} as any), false);
  assert.equal(isExceptionBadge({state: 'recording'} as any), false);
});

test('padding and keep choices stay within what saveRule/schedule accept', async () => {
  const {dvrPaddings, dvrLimits} = await import('../src/screens/live/dvr-groups.ts');
  const {validRecordingOptions, defaultRecordingOptions} = await import('@core/dvr.ts');
  for (const n of dvrPaddings) {
    assert.ok(validRecordingOptions({...defaultRecordingOptions, beforeSeconds: n}), `before ${n}`);
    assert.ok(validRecordingOptions({...defaultRecordingOptions, afterSeconds: n}), `after ${n}`);
  }
  for (const n of dvrLimits) assert.ok(validRecordingOptions({...defaultRecordingOptions, episodeLimit: n}), `keep ${n}`);
});
