import test from 'node:test';
import assert from 'node:assert/strict';
import type {LibraryAdminDocument, LibrarySettings} from '@core/administration.ts';
import type {LibraryContentApi} from '@core/library-content.ts';
import {SettingsLibrarySource} from '../src/screens/settings/library-source.ts';

const settings: LibrarySettings = {scanDepth: 'basic', groupCollections: true, localTags: 'prefer', lyricsSource: 'files', titleSource: 'tags', episodeNumbering: 'absolute', findExtras: true, sortIgnoreArticles: true, refreshMode: 'replace_unlocked', acoustidEnabled: false,
  continueWatching: {weeks: 16, maximumItems: 40, includeSeasonPremieres: true, videoPlayedThreshold: 95, videoCompletion: 'earliest'}, allowMediaDeletion: false, monitorChanges: true, providers: [], analysis: ['probe', 'local_metadata', 'subtitle_discovery'],
  navigation: {trickplayIntervalMs: 10000, trickplayTileWidth: 320, trickplayMaxTiles: 1080, chapterThumbnailMode: 'embedded', videoPreviewEnabled: false, videoPreviewMs: 30000}, segmentDetection: 'server'};
function document(libraryId: string, revision = 7): LibraryAdminDocument {
  return {libraryId, libraryName: libraryId, libraryKind: 'anime', revision, digest: 'actual-document', eligibleControls: ['libraries.cwWeeks', 'libraries.episodeNumbering'], audience: {everyone: false, accounts: 3}, actions: ['scan', 'rename'],
    settings, defaults: {...settings, continueWatching: {...settings.continueWatching, weeks: 8}}, analysisMatrix: {costClasses: [], operations: []}, availableProviders: [], enumerations: {}, sources: []};
}
function fixture(request: LibraryContentApi['request']) {
  let current = true;
  const errors: unknown[] = [];
  const source = new SettingsLibrarySource({request}, () => current, () => {}, error => {errors.push(error);});
  return {source, errors, retire: () => {current = false;}};
}

test('Library state requires explicit selection and uses only producer defaults and eligible controls', async () => {
  const calls: string[] = [];
  const f = fixture(async <T>(path: string) => {calls.push(path);return document('selected') as T;});
  try {
    assert.deepEqual(calls, []);assert.equal(f.source.document(), undefined);
    await f.source.select('selected');
    assert.deepEqual(calls, ['/v1/admin/libraries/selected/settings']);
    assert.equal(f.source.read('selected', 'libraries.cwWeeks')?.current, 16);
    assert.equal(f.source.read('selected', 'libraries.cwWeeks')?.default, 8);
    assert.equal(f.source.document('selected')?.audience.accounts, 3);
    assert.equal(f.source.read('selected', 'libraries.allowDelete'), undefined);
    assert.equal(f.source.read('unselected', 'libraries.cwWeeks'), undefined);
    await f.source.select('selected');assert.equal(calls.length, 1);
    await f.source.select();assert.equal(f.source.document('selected'), undefined);
  } finally {f.source.dispose();}
});

test('selection replacement aborts the old read and ignores a transport that returns it late', async () => {
  let release!: (value: LibraryAdminDocument) => void, oldSignal: AbortSignal | undefined;
  const f = fixture(async <T>(path: string, _method?: string, _body?: unknown, signal?: AbortSignal) => {
    if (path.includes('/first/')) {oldSignal = signal;return new Promise(resolve => {release = resolve;}) as Promise<T>;}
    return document('second') as T;
  });
  try {
    const first = f.source.select('first');await f.source.select('second');
    assert.equal(oldSignal?.aborted, true);release(document('first'));await first;
    assert.equal(f.source.document('second')?.libraryId, 'second');assert.equal(f.source.document('first'), undefined);
  } finally {f.source.dispose();}
});

test('Library save uses original CAS and complete canonical settings, adopts only confirmed results and resets to actual defaults', async () => {
  const writes: {expectedRevision: number; operationId: string; settings: LibrarySettings}[] = [];
  let release!: () => void, hold: Promise<void> | undefined;
  let server = document('selected');
  const f = fixture(async <T>(_path: string, method?: string, body?: unknown) => {
    if (method === 'PUT') {
      const write = body as typeof writes[number];writes.push(write);await hold;
      server = {...server, revision: server.revision + 1, settings: write.settings};
    }
    return server as T;
  });
  try {
    await f.source.select('selected');
    hold = new Promise(resolve => {release = resolve;});
    const saving = f.source.change('selected', 'libraries.cwWeeks', 20);
    assert.equal(f.source.getSnapshot().saving, true);
    assert.equal(f.source.read('selected', 'libraries.cwWeeks')?.current, 16);
    await f.source.change('selected', 'libraries.cwWeeks', 30);assert.equal(writes.length, 1);
    assert.equal(writes[0].expectedRevision, 7);assert.ok(writes[0].operationId);
    assert.equal(writes[0].settings.episodeNumbering, 'absolute');
    assert.deepEqual(writes[0].settings.navigation, settings.navigation);
    release();await saving;hold = undefined;
    assert.equal(f.source.read('selected', 'libraries.cwWeeks')?.current, 20);
    await f.source.reset('selected', 'libraries.cwWeeks');
    assert.equal(writes[1].expectedRevision, 8);assert.equal(writes[1].settings.continueWatching.weeks, 8);
    assert.equal(f.source.read('selected', 'libraries.cwWeeks')?.current, 8);
    await assert.rejects(f.source.change('selected', 'libraries.allowDelete', true));assert.equal(writes.length, 2);
  } finally {f.source.dispose();}
});

test('an ambiguous or conflicting save removes its old revision until a genuine reread; there is no automatic replay', async () => {
  let calls = 0, writes = 0;
  const failure = new Error('response lost');
  const f = fixture(async <T>(_path: string, method?: string) => {
    calls++;if (method === 'PUT') {writes++;throw failure;}return document('selected', calls > 2 ? 8 : 7) as T;
  });
  try {
    await f.source.select('selected');
    await assert.rejects(f.source.change('selected', 'libraries.cwWeeks', 20), failure);
    assert.equal(f.source.document('selected'), undefined);assert.equal(f.source.getSnapshot().error, failure);
    await f.source.change('selected', 'libraries.cwWeeks', 24);assert.equal(writes, 1);
    await f.source.select('selected');assert.equal(f.source.document('selected')?.revision, 8);assert.equal(writes, 1);
  } finally {f.source.dispose();}
});

test('foreign library answers and retired authority cannot publish facts or start writes', async () => {
  let calls = 0;
  const f = fixture(async <T>() => {calls++;return document('foreign') as T;});
  await f.source.select('selected');assert.equal(f.source.document('selected'), undefined);assert.equal(f.errors.length, 1);
  f.retire();await f.source.select('foreign');await f.source.change('selected', 'libraries.cwWeeks', 20);
  assert.equal(calls, 1);assert.equal(f.source.getSnapshot().document, undefined);f.source.dispose();
});

test('closing or disposing during a save aborts it without exposing a late new revision', async () => {
  for (const close of ['select', 'dispose']) {
    let release!: () => void, signal: AbortSignal | undefined;
    const f = fixture(async <T>(_path: string, method?: string, _body?: unknown, captured?: AbortSignal) => {
      if (method === 'PUT') {signal = captured;await new Promise<void>(resolve => {release = resolve;});}
      return document('selected', method === 'PUT' ? 8 : 7) as T;
    });
    try {
      await f.source.select('selected');const pending = f.source.change('selected', 'libraries.cwWeeks', 20);
      if (close === 'select') await f.source.select();else f.source.dispose();
      assert.equal(signal?.aborted, true);release();await pending;
      assert.equal(f.source.document('selected'), undefined);assert.deepEqual(f.errors, []);
    } finally {f.source.dispose();}
  }
});
