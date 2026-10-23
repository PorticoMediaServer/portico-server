import assert from 'node:assert/strict';
import {test} from 'node:test';
import {EntryMenuDetails, type EntryMenuDetailsTarget} from '../src/home-selection/entry-menu-details.ts';
import type {PlaybackOptions} from '../src/playback-v1/types.ts';

const target = {context: {scope: {serverId: 'server', authority: 'local', accountId: 'account', profileId: 'profile', viewId: 'home', privacyFence: 'fence'}, revision: {catalogRevision: '1', viewerRevision: '2'}}, scope: {serverId: 'server', viewerId: 'viewer'}, title: {libraryId: 'library', itemId: 'title'}, playbackItemId: 'playable'} as EntryMenuDetailsTarget;
const options = (ids: string[]): PlaybackOptions => ({itemId: 'playable', versions: ids.map(id => ({id, parts: [{id: 'part-a'}, {id: 'part-b'}]}))}) as PlaybackOptions;

test('Q6 counts available grouped versions, never multipart files', async () => {
  let ids = ['one'];
  const details = new EntryMenuDetails({api: {async request() {throw new Error('unexpected File info read');}}, options: {async options() {return options(ids);}}, isCurrent: () => true});
  details.select(target); await details.availableVersions(); assert.equal(details.hasDifferentVersion(), false);
  ids = ['one', 'two']; await details.availableVersions(); assert.equal(details.hasDifferentVersion(), true);
  await assert.rejects(details.playVersion('invented', async () => {throw new Error('must not start');}), /version-not-offered/);
  details.dispose();
});
test('closing or changing the current item rejects a late options result', async () => {
  let resolve!: (value: PlaybackOptions) => void;
  const details = new EntryMenuDetails({api: {async request() {throw new Error('unexpected File info read');}}, options: {options: () => new Promise<PlaybackOptions>(done => {resolve = done;})}, isCurrent: () => true});
  details.select(target); const pending = details.availableVersions();
  details.clear(); resolve(options(['one', 'two']));
  await assert.rejects(pending, /stale-home-context/); assert.equal(details.hasDifferentVersion(), false);
});
test('File info rejects a response from a different current viewer fence', async () => {
  const details = new EntryMenuDetails({api: {async request<T>() {return {scope: {serverId: 'server', libraryId: 'library', itemId: 'title', viewerFence: 'other'}, revision: {catalog: 1, viewer: 2}, kind: 'movie', files: []} as T;}}, options: {async options() {return options([]);}}, isCurrent: () => true});
  details.select(target); await assert.rejects(details.fileInfo(), /home-offers-changed/); details.dispose();
});
