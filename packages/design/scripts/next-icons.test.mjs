import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync, mkdtempSync, rmSync, writeFileSync} from 'node:fs';
import {execFileSync, spawnSync} from 'node:child_process';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {parseSprite, renderNative, renderShared} from './next-icons.mjs';
import {nextIcons, iconNames, nextIconSprite} from '../src/next-icons.ts';

const here = path.dirname(fileURLToPath(import.meta.url));
const svg = readFileSync(path.resolve(here, '../src/next-icons.svg'), 'utf8');
const icons = parseSprite(svg);
const document = body => `<svg><defs>${body}</defs></svg>`;
const symbol = '<symbol id="i-test" viewBox="0 0 24 24"><path fill="currentColor" d="M0 0h24v24H0z" /></symbol>';

test('every canonical symbol has the same faithful ID and geometry in both outputs', () => {
  const web = renderShared(icons), native = renderNative(icons);
  assert.equal(icons.length, 245);
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

test('selection queue choices preserve the exact next and later source geometry', () => {
  const next = icons.find(i => i.attrs.id === 'i-pnext');
  assert.equal(next.attrs.viewBox, '0 0 24 24');
  assert.equal(next.children[0].children[0].attrs.d, 'M4 6.5h8M4 12h5M4 17.5h8');
  assert.equal(next.children[1].attrs.d, 'M14.5 8.2v7.6c0 .5.5.8.9.5l5-3.8a.6.6 0 0 0 0-1l-5-3.8c-.4-.3-.9 0-.9.5z');
  const later = icons.find(i => i.attrs.id === 'i-plater');
  assert.equal(later.attrs.viewBox, '0 0 24 24');
  assert.equal(later.children[0].children[0].attrs.d, 'M4 6.5h16M4 11.5h16M4 16.5h8M17 14v6M14 17h6');
});

test('selection Keep only registers the exact drawn album in the shared and native outputs', () => {
  // select.html:1660 at design/ui-v1 8377791ae573e17b0fc5199ae87f7ad3feee0f16.
  const album = icons.find(i => i.attrs.id === 'i-album');
  assert.deepEqual(album, {
    tag: 'symbol', attrs: {id: 'i-album', viewBox: '0 0 24 24'}, children: [{
      tag: 'g', attrs: {fill: 'none', stroke: 'currentColor', 'stroke-width': '2'}, children: [
        {tag: 'rect', attrs: {x: '4', y: '4', width: '16', height: '16', rx: '3'}, children: []},
        {tag: 'circle', attrs: {cx: '12', cy: '12', r: '3.6'}, children: []},
      ],
    }],
  });
  const body = '<g fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="16" height="16" rx="3"></rect><circle cx="12" cy="12" r="3.6"></circle></g>';
  assert.deepEqual(nextIcons['i-album'], {viewBox: '0 0 24 24', body});
  assert.equal(iconNames.filter(name => name === 'i-album').length, 1);
  assert.ok(nextIconSprite.includes(`<symbol id="i-album" viewBox="0 0 24 24">${body}</symbol>`));
  assert.ok(renderNative(icons).includes('case "i-album": return <G><G fill="none" stroke="currentColor" strokeWidth="2"><Rect x="4" y="4" width="16" height="16" rx="3" /><Circle cx="12" cy="12" r="3.6" /></G></G>;'));
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
    assert.equal(execFileSync(process.execPath, [path.join(here, 'next-icons.mjs'), '--check'], {encoding: 'utf8'}).trim(), 'next-icons: 245 symbols; 1 output(s) verified');
  } finally { rmSync(temp, {recursive: true, force: true}); }
});
