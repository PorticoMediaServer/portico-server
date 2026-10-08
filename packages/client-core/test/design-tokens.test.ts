import test from 'node:test';
import assert from 'node:assert/strict';
import {colors, contrastRatio, icons, iconIds, isIconId, radii, tvRadiusMultiplier, typeScale} from '../../design/src/index.ts';

// Surfaces text can sit on. "raised" is the hover/selected surface; overlay is every sheet, menu and dialog.
const surfaces = {canvas: colors.ink, slate: colors.raised, raised: colors.raisedHigh, overlay: colors.overlaySurface} as const;
const textTones = {text: colors.text, textSecondary: colors.textSecondary, textTertiary: colors.textTertiary, muted: colors.muted, placeholder: colors.placeholder, accent: colors.accent, accentStrong: colors.accentStrong, danger: colors.danger, amber: colors.amber, healthy: colors.healthy, record: colors.record} as const;

test('every text tone is at least 4.5:1 on canvas, slate, raised and overlay surfaces (X-06, X-07)', () => {
  for (const [tone, fg] of Object.entries(textTones)) {
    for (const [surface, bg] of Object.entries(surfaces)) {
      const ratio = contrastRatio(fg, bg);
      assert.ok(ratio >= 4.5, `${tone} on ${surface} is ${ratio.toFixed(2)}:1`);
    }
  }
});

test('primary, secondary and tertiary text stay readable on the brightest ordinary surface', () => {
  for (const tone of ['text', 'textSecondary', 'textTertiary'] as const) {
    const ratio = contrastRatio(colors[tone], colors.raisedBright);
    assert.ok(ratio >= 4.5, `${tone} on raisedBright is ${ratio.toFixed(2)}:1`);
  }
});

test('action ink reads on every accent state, including pressed (X-11)', () => {
  for (const state of ['accent', 'accentStrong', 'accentDeep'] as const) {
    const ratio = contrastRatio(colors.actionInk, colors[state]);
    assert.ok(ratio >= 4.5, `actionInk on ${state} is ${ratio.toFixed(2)}:1`);
  }
});

test('control boundaries and focus meet the 3:1 non-text minimum (X-11, WCAG 1.4.11)', () => {
  for (const [surface, bg] of Object.entries({canvas: colors.ink, slate: colors.raised, raised: colors.raisedHigh})) {
    assert.ok(contrastRatio(colors.lineControl, bg) >= 3, `lineControl on ${surface}`);
    assert.ok(contrastRatio(colors.focus, bg) >= 3, `focus on ${surface}`);
  }
});

test('overlay surface is opaque; only the web translucent variant carries alpha', () => {
  assert.match(colors.overlaySurface, /^#[0-9a-f]{6}$/i);
  assert.match(colors.overlaySurfaceTranslucent, /^#[0-9a-f]{8}$/i);
  assert.ok(parseInt(colors.overlaySurfaceTranslucent.slice(7), 16) >= 0xf0, 'translucency stays very small');
  assert.equal(colors.overlaySurfaceTranslucent.slice(0, 7).toLowerCase(), colors.overlaySurface.toLowerCase());
});

test('muted never lowers contrast below tertiary text (X-06)', () => {
  assert.equal(colors.muted, colors.textTertiary);
});

test('radii follow the surface classes and the TV multiplier is documented', () => {
  assert.deepEqual({control: radii.control, surface: radii.surface, overlay: radii.overlay, artwork: radii.artwork}, {control: 8, surface: 10, overlay: 14, artwork: 8});
  assert.equal(Math.round(radii.control * tvRadiusMultiplier), 14);
});

test('type scale defines every role in both columns and ten-foot is never smaller', () => {
  const roles = Object.keys(typeScale.handheld);
  assert.deepEqual(Object.keys(typeScale.tenFoot).sort(), [...roles].sort());
  for (const role of roles as (keyof typeof typeScale.handheld)[]) {
    assert.ok(typeScale.tenFoot[role].size > typeScale.handheld[role].size, role);
    assert.ok(typeScale.handheld[role].lineHeight >= typeScale.handheld[role].size, role);
  }
});

test('icon registry: every glyph has a meaning and drawable elements', () => {
  assert.ok(iconIds.length >= 110);
  for (const id of iconIds) {
    const icon = icons[id];
    assert.equal(icon.id, id);
    assert.ok(icon.meaning.length > 3, id);
    assert.ok(icon.elements.length > 0, id);
    assert.ok(icon.strokeWidth > 0, id);
  }
});

test('icon registry carries the glyphs the audit found missing (X-17, CON-15/16)', () => {
  for (const id of ['airplay', 'googleCast', 'tv', 'tvDevice', 'keypad', 'flag', 'listPlus', 'sliders', 'watched', 'restart', 'archive', 'markUnread', 'bell', 'mail', 'phone', 'tablet', 'browser', 'wrench', 'logs', 'chevronUpDown', 'maximize', 'camera', 'image', 'error', 'success']) {
    assert.ok(isIconId(id), id);
  }
  assert.equal(isIconId('expand'), false);
  assert.equal(isIconId('constructor'), false);
});
