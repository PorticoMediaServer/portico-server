import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {shortcutKey} from '../src/player/shortcut-key.ts';

const press = (key: string, held: Partial<{ctrlKey: boolean; metaKey: boolean; altKey: boolean; isComposing: boolean; altGraph: boolean}> = {}) =>
  shortcutKey({key, ctrlKey: !!held.ctrlKey, metaKey: !!held.metaKey, altKey: !!held.altKey, isComposing: !!held.isComposing, getModifierState: (name: string) => name === 'AltGraph' && !!held.altGraph});

test('a keystroke with Ctrl, Cmd or Alt held belongs to the browser, not the player', () => {
  // Address bar, find, history, mute tab: none of these may seek, go fullscreen or be swallowed.
  for (const key of ['l', 'f', 'k', 'j', 'm', ' ', 'ArrowLeft', 'ArrowRight']) {
    assert.equal(press(key, {ctrlKey: true}), undefined, 'Ctrl+' + key);
    assert.equal(press(key, {metaKey: true}), undefined, 'Cmd+' + key);
    assert.equal(press(key, {altKey: true}), undefined, 'Alt+' + key);
  }
  assert.equal(press('k', {isComposing: true}), undefined, 'text being composed is not a shortcut');
});

test('plain keys reach the player, with letters matched whatever Caps Lock or Shift says', () => {
  assert.equal(press('k'), 'k');
  assert.equal(press('K'), 'k');
  assert.equal(press('F'), 'f');
  assert.equal(press(' '), ' ');
  assert.equal(press('?'), '?');
  assert.equal(press('ArrowLeft'), 'ArrowLeft');
  assert.equal(press('Escape'), 'Escape');
  // AltGr types characters on some layouts and reports Ctrl+Alt on Windows: still a plain character.
  assert.equal(press('?', {ctrlKey: true, altKey: true, altGraph: true}), '?');
});

test('the player’s key handler decides through it before acting or revealing the controls', () => {
  const player = readFileSync(new URL('../src/player/Player.tsx', import.meta.url), 'utf8');
  assert.match(player, /const pressed = shortcutKey\(e\);\n\s*if \(pressed === undefined\) return;\n\s*reveal\(\);\n\s*switch \(pressed\) \{/);
});
