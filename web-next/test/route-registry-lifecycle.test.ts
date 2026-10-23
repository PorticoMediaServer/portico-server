import test from 'node:test';
import assert from 'node:assert/strict';
import {bindWebAccountEntry, isAccountEntryPath} from '../src/route-registry-lifecycle.ts';
import {ShellBootstrap, type ShellAssembler} from '../src/shell/bootstrap.ts';
import type {SelectedViewerRecord} from '@core/viewer-selection.ts';
function deferred<T>() {let resolve!: (value: T) => void;const promise = new Promise<T>(done => {resolve = done;});return {promise, resolve};}
function navigation(path: string) {
  let listener: ((event: {fromLocation: {pathname: string}; toLocation: {pathname: string}}) => void) | undefined;
  const router = {state: {location: {pathname: path}}, subscribe(_name: 'onBeforeNavigate', next: NonNullable<typeof listener>) {listener = next;return () => {listener = undefined;};}};
  return {router, go(to: string) {const fromLocation = router.state.location, toLocation = {pathname: to};listener?.({fromLocation, toLocation});router.state.location = toLocation;}};
}
const unavailable: ShellAssembler = async () => {throw new Error('Unexpected assembly');};
test('entry destinations require exact paths and do not restore private state on initial entry', () => {
  for (const path of ['/sign-in', '/profiles', '/servers']) {
    let reads = 0;const bootstrap = new ShellBootstrap(unavailable, {selected: {read: async () => {reads++;return null;}}, restore: async () => undefined});
    const nav = navigation(path), close = bindWebAccountEntry(nav.router, bootstrap);
    assert.equal(reads, 0);assert.deepEqual(bootstrap.getSnapshot(), {phase: 'signed-out', entry: true});close();
  }
  assert.equal(isAccountEntryPath('/servers/other'), false);assert.equal(isAccountEntryPath('/account'), false);
});
test('entering profiles fences an outstanding restore; leaving waits then reads the actual new selection', async () => {
  let reads = 0, restores = 0;const gate = deferred<SelectedViewerRecord | null>();
  const bootstrap = new ShellBootstrap(unavailable, {selected: {read: async () => ++reads === 1 ? gate.promise : null}, restore: async () => {restores++;return undefined;}});
  const nav = navigation('/'), close = bindWebAccountEntry(nav.router, bootstrap);
  nav.go('/profiles');assert.equal(bootstrap.getSnapshot().entry, true);
  nav.go('/');assert.equal(reads, 1);
  gate.resolve({session: {sessionFamilyId: 'retired'}} as SelectedViewerRecord);
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(reads, 2);assert.equal(restores, 0);assert.equal(bootstrap.getSnapshot().phase, 'signed-out');close();
});
test('disposing while account exit waits cannot resurrect a private restore', async () => {
  let reads = 0;const gate = deferred<SelectedViewerRecord | null>();
  const bootstrap = new ShellBootstrap(unavailable, {selected: {read: async () => {reads++;return gate.promise;}}, restore: async () => undefined});
  const nav = navigation('/'), close = bindWebAccountEntry(nav.router, bootstrap);
  nav.go('/servers');nav.go('/');close();gate.resolve(null);
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(reads, 1);assert.equal(bootstrap.getSnapshot().phase, 'signed-out');
  nav.go('/profiles');assert.equal(bootstrap.getSnapshot().entry, undefined);
});
