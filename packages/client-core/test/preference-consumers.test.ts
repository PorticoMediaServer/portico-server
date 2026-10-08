/**
 * X-01: the preference-consumer ratchet. The check script fails when a
 * registry key has no reader outside the registry, label files and tests.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import {mkdtempSync, writeFileSync} from 'node:fs';
import {join} from 'node:path';
import {tmpdir} from 'node:os';
import {extractRegistryKeys, findReaders, isExcludedFile, isExcludedLine, EXCEPTIONS} from '../scripts/check-preference-consumers.mjs';

const GO = `
var preferenceRegistry = func() []PreferenceField {
  out := []PreferenceField{
    prefBool("appearance.showBackdrops", true, ScopeDeviceClass),
    prefBool("notifications.badges", true, ScopeProfileServer),
    prefText("region.locale", "en-US", 35, ScopeProfileServer),
  }
  return out
}();
`;

test('X-01: registry keys are extracted, with the quality lanes expanded', () => {
  const keys = extractRegistryKeys(GO);
  assert.ok(keys.includes('appearance.showBackdrops'));
  assert.ok(keys.includes('notifications.badges'));
  assert.ok(keys.includes('region.locale'));
  assert.ok(keys.includes('quality.local.mode'));
  assert.ok(keys.includes('quality.cellular.maxVideoHeight'));
  assert.equal(keys.filter(k => k.startsWith('quality.')).length, 20);
});

test('X-01: the registry, label files, tests and exclusion lists are not readers', () => {
  assert.equal(isExcludedFile('server/internal/operations/preferences.go'), true);
  assert.equal(isExcludedFile('packages/client-core/src/presentation/preference-copy.ts'), true);
  assert.equal(isExcludedFile('packages/i18n/src/catalog/en-US/preferences.ts'), true);
  assert.equal(isExcludedFile('server/internal/operations/preferences_test.go'), true);
  assert.equal(isExcludedFile('server/internal/catalog/foo_test.go'), true);
  assert.equal(isExcludedFile('web/test/x.test.ts'), true);
  assert.equal(isExcludedFile('web/src/shell/Shell.tsx'), false);
  assert.equal(isExcludedLine("  'appearance.showBackdrops', 'appearance.cardSizePercent',"), true);
  assert.equal(isExcludedLine(' * backdrops (`appearance.showBackdrops`) are off'), true);
  assert.equal(isExcludedLine("  const backdrops = server.value('appearance.showBackdrops', true);"), false);
  assert.equal(isExcludedLine('  mode := values.Text("playback.subtitleMode")'), false);
});

test('X-01: a key read only in an exclusion list has no reader; a value() read counts', () => {
  const dir = mkdtempSync(join(tmpdir(), 'm9-pref-'));
  const listOnly = join(dir, 'SettingsPages.tsx');
  writeFileSync(listOnly, `const notOnApple = new Set([\n  'appearance.showBackdrops',\n]);\n`);
  const reader = join(dir, 'Shell.tsx');
  writeFileSync(reader, `const backdrops = server.value('appearance.showBackdrops', true);\n`);
  const keys = ['appearance.showBackdrops', 'notifications.badges'];
  const listHits = findReaders(keys, [listOnly]);
  assert.deepEqual(listHits.get('appearance.showBackdrops'), []);
  const both = findReaders(keys, [listOnly, reader]);
  assert.equal(both.get('appearance.showBackdrops')!.length, 1);
  assert.deepEqual(both.get('notifications.badges'), []);
});

test('X-01: only the documented blocked keys may lack readers', () => {
  assert.deepEqual(Object.keys(EXCEPTIONS).sort(), [
    'music.repeatDefault',
    'music.shuffleDefault',
    'navigation.pinnedLibraryIds',
    'navigation.sidebarCollapsed',
    'privacy.includeInWatchTogether',
  ]);
});
