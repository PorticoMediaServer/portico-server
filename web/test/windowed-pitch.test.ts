import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {settledPitch} from '../src/ui/settled-pitch.ts';

// A 1,000-entry playlist mixing films (poster rows, 76 px) and songs (square rows, 64 px) crashed
// with "Maximum update depth exceeded": the averaged pitch followed the window and the window the pitch.
test('the windowed row pitch settles: it never shrinks at one width', () => {
  assert.equal(settledPitch(undefined, 64), 64);
  let pitch: number | undefined;
  for (const measured of [76, 64, 70, 76, 64, 76]) pitch = settledPitch(pitch, measured);
  assert.equal(pitch, 76);
});

test('every windowed row is held to the settled pitch', () => {
  const grid = readFileSync(new URL('../src/ui/WindowedGrid.tsx', import.meta.url), 'utf8');
  const css = readFileSync(new URL('../src/ui/WindowedGrid.module.css', import.meta.url), 'utf8');
  const row = readFileSync(new URL('../src/ui/ListRow.module.css', import.meta.url), 'utf8');
  assert.match(grid, /settledPitch\(measured\.current\?\.pitch/);
  assert.match(grid, /setProperty\('--row-min'/);
  assert.match(grid, /removeProperty\('--row-min'\)/, 'a new width measures rows afresh');
  assert.match(css, /\.list > \* \{ min-height: var\(--row-min, auto\); \}/);
  assert.match(css, /grid-auto-rows: minmax\(var\(--row-min, auto\), auto\)/);
  assert.match(row, /min-height: max\(64px, var\(--row-min, 0px\)\)/);
});
