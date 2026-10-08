import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from './helpers/component-harness.mjs';

const load = () => componentModule(new URL('../src/screens/server/LibrarySettings.tsx', import.meta.url), {
  react: {default: {}, useEffect: () => {}, useMemo: (f: () => unknown) => f(), useState: (v: unknown) => [v, () => {}]},
  '@core/administration.ts': {}, '../../admin/library-policy': {}, '../../admin/console': {}, '../../app/session': {}, '../../app/i18n': {}, '../../ui': {},
  '../../admin/metadata-screen': {}, './MetadataLookup': {}, './MetadataSource': {}, './operation-ids': {createOperationIds: () => ({forPayload: (k: string) => k, release: () => {}})},
}) as Promise<any>;

test('CD-05: the folder picker asks for at most 200 entries and follows the cursor', async () => {
  const {filesystemPath} = await load();
  assert.equal(filesystemPath(''), '/v1/admin/filesystem?limit=200');
  assert.equal(filesystemPath('/Volumes/Media'), '/v1/admin/filesystem?limit=200&path=%2FVolumes%2FMedia');
  assert.equal(filesystemPath('/m', 'c:abc'), '/v1/admin/filesystem?limit=200&path=%2Fm&cursor=c%3Aabc');
});

test('the next page is appended in the server’s order without duplicates', async () => {
  const {mergeFilesystemPages} = await load();
  const e = (name: string) => ({name, path: '/m/' + name, kind: 'directory', readable: true, symlink: false, bytes: 0});
  const merged = mergeFilesystemPages({path: '/m', entries: [e('A'), e('a')], nextCursor: 'x'}, {path: '/m', entries: [e('a'), e('b')], nextCursor: ''});
  assert.deepEqual(merged.entries.map((x: any) => x.name), ['A', 'a', 'b']);
  assert.equal(merged.nextCursor, '');
});

test('CD-43: a truncated page presents its roots as partial, never exhaustive', async () => {
  const {areFilesystemRootsPartial} = await load();
  assert.equal(areFilesystemRootsPartial(undefined), false);
  assert.equal(areFilesystemRootsPartial({truncated: false, roots: []}), false);
  assert.equal(areFilesystemRootsPartial({truncated: true, roots: Array.from({length: 64})}), true);
});
