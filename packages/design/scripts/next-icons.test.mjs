import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync, mkdtempSync, rmSync, writeFileSync} from 'node:fs';
import {execFileSync, spawnSync} from 'node:child_process';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {parseSprite, renderNative, renderShared} from './next-icons.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const svg = readFileSync(path.resolve(here, '../src/next-icons.svg'), 'utf8');
const icons = parseSprite(svg);
const document = body => `<svg><defs>${body}</defs></svg>`;
const symbol = '<symbol id="i-test" viewBox="0 0 24 24"><path fill="currentColor" d="M0 0h24v24H0z" /></symbol>';

test('every canonical symbol has the same faithful ID and geometry in both outputs', () => {
  const web = renderShared(icons), native = renderNative(icons);
  assert.equal(icons.length, 242);
  assert.equal(new Set(icons.map(i => i.attrs.id)).size, icons.length);
  assert.equal((native.match(/case "/g) || []).length, icons.length);
  for (const icon of icons) {
    assert.ok(web.includes(JSON.stringify(icon.attrs.id) + ': {viewBox: ' + JSON.stringify(icon.attrs.viewBox)));
    assert.ok(native.includes('case ' + JSON.stringify(icon.attrs.id) + ':'));
    const visit = node => {
      for (const [key, value] of Object.entries(node.attrs)) {
        if (key === 'id' || key === 'viewBox') continue;
        assert.ok(web.includes(value), `${icon.attrs.id}: shared ${key}`);
        assert.ok(native.includes(value), `${icon.attrs.id}: native ${key}`);
      }
      node.children.forEach(visit);
    };
    visit(icon);
  }
  assert.equal(icons.find(i => i.attrs.id === 'i-wifi').attrs.viewBox, '0 0 16 12');
  assert.equal(icons.find(i => i.attrs.id === 'i-bat').attrs.viewBox, '0 0 27 13');
  assert.equal(icons.find(i => i.attrs.id === 'i-sig').attrs.viewBox, '0 0 18 12');
});

test('activity Levels preserves the approved ring and three bars', () => {
  const activity = icons.find(i => i.attrs.id === 'i-act');
  assert.equal(activity.children[0].children[0].tag, 'circle');
  assert.equal(activity.children[0].children[0].attrs.r, '9');
  assert.equal(activity.children[0].children[1].attrs.d, 'M8.25 16v-3.5M12 16V8M15.75 16v-5.5');
});

test('unsupported SVG features, unsafe attributes and malformed geometry fail closed', () => {
  for (const invalid of [
    symbol + symbol,
    symbol.replace('<path ', '<script '),
    symbol.replace('fill="currentColor"', 'onclick="alert(1)"'),
    symbol.replace('fill="currentColor"', 'class="ic"'),
    symbol.replace('24 24', '0 24'),
    symbol.replace('24 24', 'NaN 24'),
    symbol.replace('fill="currentColor"', 'fill="currentColor" fill="none"'),
    symbol.replace('</symbol>', ''),
    symbol.replace('<path ', 'text<path '),
  ]) assert.throws(() => parseSprite(document(invalid)));
  assert.throws(() => parseSprite('<svg><defs /></svg><svg><defs /></svg>'));
  assert.throws(() => parseSprite(document('')));
});

test('generation is deterministic and check detects stale or missing native output', () => {
  assert.equal(renderShared(parseSprite(svg)), renderShared(parseSprite(svg)));
  assert.equal(renderNative(parseSprite(svg)), renderNative(parseSprite(svg)));
  const temp = mkdtempSync(path.join(tmpdir(), 'portico-icons-'));
  const target = path.join(temp, 'IconGlyphs.tsx');
  const run = extra => spawnSync(process.execPath, [path.join(here, 'next-icons.mjs'), '--check', '--native', target, ...extra], {encoding: 'utf8'});
  try {
    assert.notEqual(run([]).status, 0);
    assert.match(run([]).stderr, /Missing icon output/);
    writeFileSync(target, renderNative(icons));
    assert.equal(run([]).status, 0);
    writeFileSync(target, renderNative(icons) + '\n');
    assert.notEqual(run([]).status, 0);
    assert.match(run([]).stderr, /Stale icon output/);
    assert.equal(execFileSync(process.execPath, [path.join(here, 'next-icons.mjs'), '--check'], {encoding: 'utf8'}).trim(), 'next-icons: 242 symbols; 1 output(s) verified');
  } finally { rmSync(temp, {recursive: true, force: true}); }
});
