import test from 'node:test';
import assert from 'node:assert/strict';
import {colors, componentSpec, contrastRatio, spacingGrid, targets, type ColorToken} from '../../design/src/index.ts';

const grid = new Set<number>(spacingGrid);
const hex = (token: ColorToken) => colors[token].slice(0, 7);
/** Composite a token at `alpha` over an opaque surface. */
function over(token: ColorToken, alpha: number, surface: ColorToken): string {
  const a = hex(token), b = hex(surface);
  const mix = [1, 3, 5].map(i => Math.round(parseInt(a.slice(i, i + 2), 16) * alpha + parseInt(b.slice(i, i + 2), 16) * (1 - alpha)));
  return '#' + mix.map(v => v.toString(16).padStart(2, '0')).join('');
}

test('every padding, gap, inset and margin in the component spec is on the spacing grid', () => {
  const offGrid: string[] = [];
  const walk = (value: unknown, path: string) => {
    if (value && typeof value === 'object') { for (const [k, v] of Object.entries(value)) walk(v, `${path}.${k}`); return; }
    if (typeof value !== 'number') return;
    if (!/(padding|gap|inset|margin)/i.test(path) || /Alpha|alpha/.test(path)) return;
    if (!grid.has(Math.abs(value))) offGrid.push(`${path} = ${value}`);
  };
  walk(componentSpec, 'spec');
  assert.deepEqual(offGrid, []);
});

test('interactive heights meet the platform minimum targets', () => {
  const {button, row, menu, field, chip} = componentSpec;
  for (const size of ['md', 'lg'] as const) {
    assert.ok(button.height[size].phone >= targets.touch, `button ${size} phone`);
    assert.ok(button.height[size].tv >= targets.tenFoot, `button ${size} tv`);
  }
  assert.ok(button.minHit.phone >= targets.touch && chip.minHit.phone >= targets.touch);
  assert.ok(row.minHeight.single.phone >= targets.touch && row.minHeight.single.tv >= targets.tenFoot);
  assert.ok(menu.row.minHeight.phone >= targets.touch && menu.row.minHeight.web >= targets.menuRowPointer && menu.row.minHeight.tv >= targets.tenFoot);
  assert.ok(field.height.phone >= targets.touch && field.height.tv >= targets.tenFoot);
});

test('nothing focusable on TV is below the ten-foot minimum (APL-SYS-01)', () => {
  const {button, chip, segmented, row, menu, field, toggle} = componentSpec;
  const heights: [string, number][] = [
    ...(['sm', 'md', 'lg'] as const).map(s => [`button.${s}`, button.height[s].tv] as [string, number]),
    ['chip', chip.height.tv], ['segmented', segmented.height.tv], ['row', row.minHeight.single.tv], ['menu row', menu.row.minHeight.tv], ['field', field.height.tv],
  ];
  assert.deepEqual(heights.filter(([, h]) => h < targets.tenFoot), []);
  assert.ok(segmented.minHit.phone >= targets.touch);
  assert.ok(toggle.track.height.tv < targets.tenFoot, 'a switch is never focused on its own on TV: its row is');
});

test('button and chip labels keep 4.5:1 in every interactive state, on page and overlay surfaces', () => {
  const failures: string[] = [];
  for (const surface of ['ink', 'raised', 'overlaySurface'] as const) {
    for (const [variant, states] of Object.entries(componentSpec.button.variants)) {
      for (const [state, fill] of Object.entries(states)) {
        if (state === 'disabled') continue;
        const bg = fill.background === 'transparent' ? hex(surface) : over(fill.background, fill.backgroundAlpha ?? 1, surface);
        const ratio = contrastRatio(hex(fill.foreground), bg);
        if (ratio < 4.5) failures.push(`${variant}/${state} on ${surface}: ${ratio.toFixed(2)}`);
      }
    }
    for (const [state, fill] of Object.entries({rest: componentSpec.chip.rest, hover: componentSpec.chip.hover, selected: componentSpec.chip.selected})) {
      const bg = fill.background === 'transparent' ? hex(surface) : over(fill.background, fill.backgroundAlpha ?? 1, surface);
      const ratio = contrastRatio(hex(fill.foreground), bg);
      if (ratio < 4.5) failures.push(`chip/${state} on ${surface}: ${ratio.toFixed(2)}`);
    }
  }
  assert.deepEqual(failures, []);
});

test('overlays are opaque everywhere except desktop web, and pickers alone carry a close button', () => {
  const {overlay} = componentSpec;
  assert.equal(overlay.surface, 'overlaySurface');
  assert.deepEqual(overlay.translucentAllowed, {web: true, phone: false, tablet: false, tv: false});
  assert.deepEqual(overlay.closeButton.kinds, ['picker']);
  assert.deepEqual(overlay.footer.kinds, ['form', 'acknowledge']);
  assert.equal(overlay.closeButton.size.tv, 0);
});

test('title hero: one height rule per device, well-formed scrims (Spec — Title Pages §1)', () => {
  const {hero} = componentSpec;
  assert.equal(hero.height.tv.viewportFraction, 0.6);
  assert.deepEqual(hero.height.web, {viewportFraction: 0.7, max: 720});
  assert.deepEqual(hero.height.tablet, hero.height.web);
  assert.equal(hero.height.phone.backdropAspect, 16 / 9);
  for (const stops of [hero.scrim.bottom, hero.scrim.left, hero.scrim.phoneBottom]) {
    assert.ok(stops.every((s, i) => s.alpha >= 0 && s.alpha <= 1 && s.at >= 0 && s.at <= 1 && (i === 0 || s.at > stops[i - 1]!.at)));
  }
  assert.equal(hero.scrim.bottom.at(-1)!.alpha, 1, 'the hero melts into the page');
  assert.deepEqual(hero.backdropWidth, {web: 1920, phone: 800, tablet: 1920, tv: 1920});
});
