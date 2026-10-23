import test from 'node:test';
import assert from 'node:assert/strict';
import {watchViewerSelection} from '../src/platform/viewer-selection-events.ts';
import {sameShellAuthority} from '../src/shell/bootstrap.ts';
import type {LocalSession} from '@core/index.ts';
const session = (changes: Partial<LocalSession> = {}): LocalSession => ({accessToken: 'private-access', expiresAt: '2099-01-01T00:00:00Z', authorizationHorizon: '2099-02-01T00:00:00Z', tokenGeneration: '1', sessionFamilyId: 'family', viewer: {authority: 'local', serverId: 'server', accountId: 'account', profileId: 'profile', role: 'member'}, ...changes});
const tick = () => new Promise(resolve => setImmediate(resolve));
test('selection observer ignores rotations but retires captured viewer on actual family change or removal', async () => {
  for (const next of [null, session({sessionFamilyId: 'replacement'})]) {
    let current = session(), selected: {session: LocalSession} | null = {session: current}, listener = () => {}, changed = 0;
    const stop = watchViewerSelection({subscribe: value => {listener = value; return () => {listener = () => {};};}, read: async () => selected, current: () => current, same: sameShellAuthority, changed: () => {changed++;}});
    await tick(); current = session({accessToken: 'rotated', tokenGeneration: '2'}); selected = {session: current}; listener(); await tick(); assert.equal(changed, 0);
    selected = next && {session: next}; listener(); await tick(); assert.equal(changed, 1); stop(); listener(); await tick(); assert.equal(changed, 1);
  }
});
test('profile/server/account changes are observed independently of bearer text', async () => {
  for (const key of ['profileId', 'serverId', 'accountId'] as const) {
    const current = session(); let changed = 0;
    const stop = watchViewerSelection({subscribe: () => () => {}, read: async () => ({session: session({viewer: {...current.viewer, [key]: 'other'}})}), current: () => current, same: sameShellAuthority, changed: () => {changed++;}});
    await tick(); assert.equal(changed, 1); stop();
  }
});
test('an in-flight read cannot publish after its viewer owner is retired', async () => {
  let resolve!: (value: null) => void, changed = 0;
  const stop = watchViewerSelection({subscribe: () => () => {}, read: () => new Promise(done => {resolve = done;}), current: session, same: sameShellAuthority, changed: () => {changed++;}});
  stop(); resolve(null); await tick(); assert.equal(changed, 0);
});
test('a storage failure is reported without treating temporary unavailability as sign-out', async () => {
  let changed = 0, failed = 0;
  const stop = watchViewerSelection({subscribe: () => () => {}, read: async () => {throw new Error('locked');}, current: session, same: sameShellAuthority, changed: () => {changed++;}, failed: () => {failed++;}});
  await tick(); assert.equal(changed, 0); assert.equal(failed, 1); stop();
});
test('a second notification during a read is coalesced and the final durable state is re-read', async () => {
  const current = session(); let listener = () => {}, resolve!: (value: {session: LocalSession}) => void, reads = 0, changed = 0;
  const stop = watchViewerSelection({subscribe: value => {listener = value; return () => {};}, read: async () => ++reads === 1 ? new Promise(done => {resolve = done;}) : null,
    current: () => current, same: sameShellAuthority, changed: () => {changed++;}});
  listener(); listener(); resolve({session: current}); await tick(); assert.equal(reads, 2); assert.equal(changed, 1); stop();
});
