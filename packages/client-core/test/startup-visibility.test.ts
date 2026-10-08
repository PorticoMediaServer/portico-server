import test from 'node:test';
import assert from 'node:assert/strict';
import {PlaybackService, type PageVisibility, type PlaybackSnapshot} from '../src/index.ts';
import {toPlaybackSession} from '../src/playback-v1/legacy-bridge.ts';
import {parseSession} from '../src/playback-v1/types.ts';

const wait = (ms: number) => new Promise(r => setTimeout(r, ms));
const api = {createPlayback: async () => ({id: 's1', generation: 1, streamUrl: '/v1/media/g', mode: 'direct' as const, duration: 120, resumeSeconds: 0}), stopPlayback: async () => {}, progressPlayback: async () => {}};
function page(hidden: boolean) {
  const listeners = new Set<() => void>();
  const v: PageVisibility & {hiddenNow: boolean; show(): void} = {
    hiddenNow: hidden,
    hidden: () => v.hiddenNow,
    onVisible(l) { listeners.add(l); return () => { listeners.delete(l); }; },
    show() { v.hiddenNow = false; for (const l of [...listeners]) l(); },
  };
  return v;
}

// O4, 24 Sep: browsers defer loading media in a hidden page, so a start there can't become ready.
test('a start in a hidden page waits for the page instead of failing, then gets a full window', async () => {
  const visibility = page(true);
  const s = new PlaybackService(api, () => 'k', {startupTimeoutMs: 15, pageVisibility: visibility});
  await s.play('movie', 0);
  await wait(40);
  assert.notEqual(s.getSnapshot().phase, 'error', 'no failure while hidden');
  visibility.show();
  await wait(5);
  assert.notEqual(s.getSnapshot().phase, 'error', 'shown: a fresh window starts, not an immediate failure');
  await wait(30);
  assert.equal(s.getSnapshot().phase, 'error', 'still not ready a full window after being shown');
  s.leave();
});

test('a visible page keeps the startup window as before; becoming ready clears a pending wait', async () => {
  const visible = new PlaybackService(api, () => 'k', {startupTimeoutMs: 15, pageVisibility: page(false)});
  await visible.play('movie', 0);
  await wait(30);
  assert.equal(visible.getSnapshot().phase, 'error');
  visible.leave();
  const visibility = page(true);
  const s = new PlaybackService(api, () => 'k', {startupTimeoutMs: 15, pageVisibility: visibility});
  await s.play('movie', 0);
  await wait(25);
  s.ready(s.getSnapshot().intentId);
  visibility.show();
  await wait(30);
  assert.notEqual(s.getSnapshot().phase, 'error');
  s.leave();
});

// The lead's session from the demo (Dune: Part Two, 24 Sep 02:52 UTC): a v1 direct presentation
// resuming at 33 s reaches the adapter as the direct media URL with a pending seek to 33 s.
test('a v1 direct presentation reaches the adapter with its URL and the resume seek', async () => {
  const session = parseSession({id: 'ps_1', revision: '1', kind: 'vod', role: 'local', state: 'playing', itemId: 'item1', queue: {queueId: 'q1', entryId: 'e1'},
    lease: {reportEveryMs: 10000}, presentation: {generation: 1, mode: 'direct', url: '/v1/media/grantABC_-1', startPositionMs: 33000, subtitles: [], decision: {video: {action: 'direct'}, audio: {action: 'direct'}}}});
  assert.ok(session, 'the demo response parses');
  const s = new PlaybackService(api, () => 'k', {queueRequired: true, startupTimeoutMs: 1000});
  const applied: PlaybackSnapshot[] = [];
  s.attachAdapter({apply: next => { applied.push(next); }});
  const intent = s.beginQueueIntent('item1');
  assert.equal(await s.adoptQueueSession('item1', toPlaybackSession(session!, 9960), intent), true);
  const last = applied.at(-1)!;
  assert.equal(last.session?.streamUrl, '/v1/media/grantABC_-1');
  assert.equal(last.session?.mode, 'direct');
  assert.equal(last.pendingSeek?.positionSeconds, 33);
  s.ready(last.intentId);
  assert.ok(s.seekApplied(last.intentId, last.pendingSeek!.revision, 33, 'playing'));
  assert.equal(s.getSnapshot().positionSeconds, 33);
  s.leave();
});
