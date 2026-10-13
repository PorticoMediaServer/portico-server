import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {holdPaused} from '../src/app/hold-paused.ts';

const source = readFileSync(new URL('../src/app/together.tsx', import.meta.url), 'utf8');


function player(initial: {itemId?: string; phase: string; intent: string}) {
  let state = initial, pauses = 0;
  const listeners = new Set<() => void>();
  return {
    getSnapshot: () => state,
    subscribe: (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; },
    pause: () => { pauses++; state = {...state, intent: 'paused'}; for (const l of [...listeners]) l(); },
    set: (next: Partial<typeof state>) => { state = {...state, ...next}; for (const l of [...listeners]) l(); },
    get pauses() { return pauses; },
    get listening() { return listeners.size; },
  };
}

test('a paused group loads the title paused, before it is ready', () => {
  const p = player({itemId: 'film', phase: 'idle', intent: 'paused'});
  holdPaused(p, 'film');
  // The start begins: the player sets it to playing, and the hold answers at once.
  p.set({phase: 'starting', intent: 'playing'});
  assert.equal(p.getSnapshot().intent, 'paused');
  assert.equal(p.pauses, 1);
  // Something else sets it playing again while it is still starting: held again.
  p.set({intent: 'playing'});
  assert.equal(p.getSnapshot().intent, 'paused');
  // Ready: the hold ends, and the group's own play is not fought.
  p.set({phase: 'ready'});
  assert.equal(p.listening, 0);
  p.set({intent: 'playing'});
  assert.equal(p.getSnapshot().intent, 'playing');
});

test('the hold lets go when another title takes over or the start fails', () => {
  const other = player({itemId: 'film', phase: 'starting', intent: 'paused'});
  holdPaused(other, 'film');
  other.set({itemId: 'another', phase: 'starting', intent: 'playing'});
  assert.equal(other.getSnapshot().intent, 'playing', 'another title is not paused by a stale hold');
  assert.equal(other.listening, 0);
  const failed = player({itemId: 'film', phase: 'starting', intent: 'playing'});
  holdPaused(failed, 'film');
  failed.set({phase: 'error'});
  assert.equal(failed.listening, 0);
});

test('the web follower passes the group\'s paused state into the load', () => {
  assert.match(source, /load: \(itemId, positionSeconds, playing\) => \{/);
  assert.match(source, /if \(!playing\) holdPaused\(live\.current\.service, itemId\);/);
});
