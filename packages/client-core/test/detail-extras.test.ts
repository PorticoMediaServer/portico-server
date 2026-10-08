import test from 'node:test';
import assert from 'node:assert/strict';
import {DetailService, detailExtraTypes, type DetailApi} from '../src/detail.ts';

const scope = {serverId: 'server', viewerId: JSON.stringify(['local', 'account', 'profile'])};
const target = {libraryId: 'library', itemId: 'item'};

function extra(id: string, title: string) {
  return {id, kind: 'extra', libraryId: 'library', title, available: true, navigation: {view: 'item', entityId: id}, playback: {itemId: id}};
}
function detail(extras: unknown): any {
  return {
    scope: {serverId: 'server', libraryId: 'library', itemId: 'item', viewerFence: 'fence'},
    revision: {catalog: 1, viewer: 0},
    item: {id: 'item', libraryId: 'library', title: 'Server title', kind: 'movie', duration: 100, progressSeconds: 0, available: true},
    personal: {watchlisted: false, favorite: false, rating: null, revision: 0, watched: false, reaction: 'none', progressSeconds: 0, lastPlayedAt: '', status: 'current', conflicts: []},
    actions: [{id: 'watchlist', labelKey: 'action.watchlist', enabled: true}],
    metadata: {status: 'available', ratings: [], genres: [], credits: []},
    ...(extras === undefined ? {} : {extras}),
  };
}
function service(payload: any) {
  const api: DetailApi = {request: async <T>() => payload as T};
  return new DetailService({api, scope, timeoutMs: 1000, requestId: async () => '12345678-1234-4123-8123-000000000001'});
}

test('detail extras arrive grouped by a published type with playable items', async () => {
  const client = service(detail([
    {type: 'trailer', label: 'Trailers', items: [extra('x1', 'Teaser'), extra('x3', 'Full Trailer')]},
    {type: 'featurette', label: 'Featurettes', items: [extra('x2', 'Design')]},
  ]));
  await client.select(target);
  const data = client.getSnapshot().data!;
  assert.equal(data.extras!.length, 2);
  assert.equal(data.extras![0].type, 'trailer');
  assert.equal(data.extras![0].items[1].id, 'x3');
  assert.equal(data.extras![0].items[0].playback!.itemId, 'x1');
  assert.throws(() => (data.extras as unknown as unknown[]).push({}), TypeError);
  assert.equal(detailExtraTypes.length, 8);
});

test('detail without extras stays valid and malformed extras are refused', async () => {
  const plain = service(detail(undefined));
  await plain.select(target);
  assert.equal(plain.getSnapshot().phase, 'ready');
  assert.equal(plain.getSnapshot().data!.extras, undefined);
  for (const broken of [
    [{type: 'blooper', label: 'Bloopers', items: [extra('x1', 'One')]}],
    [{type: 'trailer', label: '', items: [extra('x1', 'One')]}],
    [{type: 'trailer', label: 'Trailers', items: []}],
    [{type: 'trailer', label: 'Trailers', items: [extra('x1', 'One'), extra('x1', 'One')]}],
    [{type: 'trailer', label: 'Trailers', items: [{...extra('x1', 'One'), kind: 'movie'}]}],
    [{type: 'trailer', label: 'Trailers', items: [extra('item', 'Self')]}],
    [{type: 'trailer', label: 'Trailers', items: [{...extra('x1', 'One'), playback: {itemId: 'other'}}]}],
    [{type: 'trailer', label: 'A', items: [extra('x1', 'One')]}, {type: 'trailer', label: 'B', items: [extra('x2', 'Two')]}],
  ]) {
    const client = service(detail(broken));
    await client.select(target);
    const snapshot = client.getSnapshot();
    assert.equal(snapshot.phase, 'error', JSON.stringify(broken));
    assert.equal(snapshot.error!.code, 'invalid_detail');
  }
});
