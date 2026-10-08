import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

const src = (p: string) => readFileSync(new URL('../src/' + p, import.meta.url), 'utf8');

test('an album, artist or collection page has its own More menu: the card’s actions, without Details for the page already open', () => {
  const entity = src('screens/library/Entity.tsx');
  assert.match(entity, /icon="more" aria-label=\{currentI18n\(\)\.t\('title\.moreActions'\)\}[^\n]*player\.more\(entityMore, anchorOf\(e\.c/);
  assert.match(entity, /libraryId: entity\.libraryId \|\| libraryId, navigation: entity\.navigation \?\? \{view:/);
  const actions = src('screens/shared/EntryActions.tsx');
  assert.match(actions, /details: !!entry\.navigation && !\(entry\.navigation\.entityId && location\.pathname\.endsWith\(`\/\$\{entry\.navigation\.view\}\/\$\{entry\.navigation\.entityId\}`\)\)/);
});

test('M25-3: album/book pages mark watched through useContainerWatched with hero counts, and download through the container dialog', () => {
  const entity = src('screens/library/Entity.tsx');
  // Watched through the shared hook where the kind supports container state.
  assert.match(entity, /containerKindFor\(entity\.kind\)/);
  assert.match(entity, /useContainerWatched\(api, containerKind/);
  assert.match(entity, /containerWatched\.setWatched\(!\(containerWatched\.state\?\.watched \?\? false\)\)/);
  // Hero shows the visible watched/unwatched counts when present.
  assert.match(entity, /containerWatched\.state\?\.watchedCount !== undefined && containerWatched\.state\?\.unwatchedCount !== undefined/);
  // Download album/book through DownloadsService.requestContainer policy dialog.
  assert.match(entity, /downloads\.ask\(\{target: \{containerId: entity\.id, containerKind: downloadKind\}/);
  assert.match(entity, /!downloads\.state\.unavailable/);
  // Catalogue copy only: no hard-coded watched/download strings.
  assert.doesNotMatch(entity, /Mark as watched/);
  assert.doesNotMatch(entity, /Download album/);
});
