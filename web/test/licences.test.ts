import test from 'node:test';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
import {existsSync, readFileSync} from 'node:fs';
import {defaultI18n} from '@i18n';
import {lockPackages, productionPackages, serialiseLicenceData} from '../../packages/client-core/src/licences/inventory.ts';
import {licenceSections, parseLicenceData, type LicenceData} from '../../packages/client-core/src/presentation/licences.ts';
import {componentModule, hooks} from './helpers/component-harness.mjs';

const root = new URL('../../', import.meta.url);
const file = new URL('../src/generated/licences.json', import.meta.url);
const data = JSON.parse(readFileSync(file, 'utf8')) as LicenceData;
const REGENERATE = 'run: node --experimental-strip-types web/scripts/licences.ts';

test('the licence list is current: every inventory it was generated from is unchanged', () => {
  assert.ok(Object.keys(data.inputs).includes('package-lock.json'));
  assert.ok(Object.keys(data.inputs).includes('server/go.sum'));
  for (const [rel, digest] of Object.entries(data.inputs)) {
    const now = createHash('sha256').update(readFileSync(new URL(rel, root))).digest('hex');
    assert.equal(now, digest, `${rel} changed since web/src/generated/licences.json was generated; ${REGENERATE}`);
  }
});

test('every production package of the lockfile is listed, and nothing else from npm', () => {
  const lock = JSON.parse(readFileSync(new URL('package-lock.json', root), 'utf8'));
  const expected = productionPackages(lockPackages(lock)).filter(p => !p.name.startsWith('@portico/') && p.name !== 'portico-web').map(p => `${p.name}@${p.version}`).sort();
  const listed = data.components.filter(c => c.id.startsWith('npm:')).map(c => `${c.name}@${c.version}`).sort();
  assert.deepEqual(listed, expected, REGENERATE);
  // Development tooling never appears.
  for (const tool of ['vite', 'typescript', '@vitejs/plugin-react', 'rolldown']) assert.ok(!listed.some(p => p.startsWith(tool + '@')), tool);
});

test('the file is well formed: Portico first, every text present, sources stated', () => {
  assert.ok(parseLicenceData(data));
  assert.equal(data.components[0]!.id, 'portico');
  assert.equal(data.components[0]!.licence, 'GPL-3.0-or-later');
  assert.match(data.texts[data.components[0]!.text!]!, /GNU GENERAL PUBLIC LICENSE\s+Version 3/);
  for (const c of data.components) {
    assert.ok(c.text === undefined || typeof data.texts[c.text] === 'string', c.id);
    assert.ok(c.text !== undefined || c.textPending || (c.textAtUrl && c.url), `${c.id} has no licence text`);
  }
  assert.equal(new Set(data.texts).size, data.texts.length, 'identical texts are stored once');
  assert.deepEqual(data.sources.map(s => s.id), ['portico', 'ffmpeg']);
  assert.deepEqual(licenceSections(data).map(s => s.group), ['portico', 'packages', 'subtitles', 'fonts', 'server']);
  for (const id of ['font:manrope', 'icons:lucide', 'vendor:libass-wasm@4.2.4', 'server:ffmpeg', 'go:std']) assert.ok(data.components.some(c => c.id === id), id);
});

// Where node_modules and Go's module cache are present (a developer's checkout, the release
// build), a fresh run must give exactly the committed file: a dependency change that leaves the
// lockfiles alone (a new Go import of an existing module) is caught too.
const canRegenerate = existsSync(new URL('node_modules/react/package.json', root)) && (() => { try { execFileSync('go', ['version'], {stdio: 'ignore'}); return true; } catch { return false; } })();
test('a fresh generation gives exactly the committed file', {skip: !canRegenerate && 'needs node_modules and Go'}, async () => {
  const {generate} = await import('../scripts/licences.ts');
  assert.equal(serialiseLicenceData(generate()), readFileSync(file, 'utf8'), REGENERATE);
});

type Node = {type: unknown; props: Record<string, any> & {children?: unknown}};
const all = (node: unknown, match: (n: Node) => boolean, out: Node[] = []): Node[] => {
  if (Array.isArray(node)) { for (const child of node) all(child, match, out); return out; }
  if (!node || typeof node !== 'object') return out;
  const n = node as Node;
  if (match(n)) out.push(n);
  all(n.props?.children, match, out);
  return out;
};

async function screen() {
  const h = hooks();
  const navigated: unknown[] = [];
  const opened: unknown[] = [];
  (globalThis as {window?: unknown}).window = {open: (...a: unknown[]) => opened.push(a)};
  const mod = await componentModule(new URL('../src/screens/settings/Licences.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {Link: 'Link', useNavigate: () => (to: unknown) => { navigated.push(to); }},
    '../../app/i18n': {useI18n: () => ({t: defaultI18n.t})},
    '../../ui': {Icon: 'Icon', KeyValue: 'KeyValue', Loading: 'Loading', Notice: 'Notice', SettingsGroup: 'SettingsGroup', SettingsRow: 'SettingsRow', Text: 'Text'},
    './Settings.module.css': {default: new Proxy({}, {get: (_, key) => String(key)})},
  }) as typeof import('../src/screens/settings/Licences.tsx');
  return {mod, h, navigated, opened};
}

test('the list: Portico’s licence and the source first, then a row per component that opens it', async () => {
  const {mod, h, navigated, opened} = await screen();
  const tree = h.render(() => mod.LicenceList({data}));
  const groups = all(tree, n => n.type === 'SettingsGroup').map(g => g.props.title);
  assert.deepEqual(groups.slice(0, 2), [defaultI18n.t('licenses.section.portico'), defaultI18n.t('licenses.source.title')]);
  assert.ok(groups.includes(defaultI18n.t('licenses.section.server')));
  const rows = all(tree, n => n.type === 'SettingsRow');
  // Portico, two source rows, and every component.
  assert.equal(rows.length, 1 + 2 + data.components.length - 1);
  const react = rows.find(r => r.props.label === 'react')!;
  assert.match(react.props.help, /^19\.\d+\.\d+ · MIT$/);
  react.props.onClick();
  assert.deepEqual(navigated.at(-1), {to: '/settings/$section', params: {section: 'about'}, search: {tab: 'licenses', id: data.components.find(c => c.name === 'react')!.id}});
  rows.find(r => r.props.icon === 'external')!.props.onClick();
  assert.equal((opened.at(-1) as unknown[])[0], data.sources[0]!.url);
});

test('a component: its version, licence, copyright lines and full text', async () => {
  const {mod, h} = await screen();
  const id = data.components.find(c => c.name === 'react')!.id;
  const tree = h.render(() => mod.LicenceView({data, id}));
  const kv = all(tree, n => n.type === 'KeyValue')[0]!;
  assert.equal(kv.props.rows[1][1], 'MIT');
  const text = all(tree, n => n.type === 'pre')[0]!;
  assert.match(String(text.props.children), /Permission is hereby granted/);
  assert.ok(all(tree, n => n.type === 'p').some(p => /Copyright/.test(String(p.props.children))));
  const missing = h.render(() => mod.LicenceView({data, id: 'npm:nothing@0'}));
  assert.deepEqual(all(missing, n => n.type === 'Notice')[0]!.props.children, [defaultI18n.t('licenses.notFound')]);
});
