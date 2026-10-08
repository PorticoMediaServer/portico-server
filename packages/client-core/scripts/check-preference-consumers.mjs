#!/usr/bin/env node
/**
 * X-01: every server preference key must have a runtime consumer.
 *
 * Fails (exit 1) when a registry key has no reader outside the registry
 * itself, the label/copy files and tests. Keys come from the checked-in
 * registry source (`server/internal/operations/preferences.go`):
 * there is no OpenAPI enum of preference keys (verified: `server/api`
 * mentions preferences only in prose), so the registry source is the list.
 *
 * Readers count in client code (`web/src`,
 * the Apple app's `src` (portico-react-native), `packages/client-core/src`, `packages/i18n/src`)
 * and in server code (`server/internal`): a key enforced
 * server-side (delivery policy, personal activity, search history, home
 * layout) has a consumer even when no client reads it.
 *
 * Excluded (never readers):
 * - the registry file itself,
 * - label/copy files (`preference-copy.ts`, `preference-labels.ts`,
 *   `catalog/en-US/preferences.ts`),
 * - tests (`*.test.*`, `test/` directories),
 * - exclusion-list literals (`notOnApple`, `managedElsewhere` sets name keys
 *   precisely because nothing reads them),
 * - this script.
 *
 * `EXCEPTIONS` are keys with no reader yet. They warn but do not fail, so the ratchet
 * still catches every new unread key.
 */
import {existsSync, readFileSync, readdirSync, statSync} from 'node:fs';
import {join, relative, resolve, dirname} from 'node:path';
import {fileURLToPath} from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, '..', '..', '..');

const REGISTRY = join(root, 'server/internal/operations/preferences.go');
// The Apple app reads preferences too; it lives in the portico-react-native checkout
// beside this one. Without it a key only the Apple app reads would look unread.
const appleSource = resolve(process.env.PORTICO_REACT_NATIVE_REPO ?? join(root, '..', 'portico-react-native'), 'apps/apple/src');
if (!existsSync(appleSource)) {
  console.log(`preference consumers: skipped (no portico-react-native checkout at ${dirname(dirname(appleSource))}; set PORTICO_REACT_NATIVE_REPO)`);
  process.exit(0);
}
const SEARCH_ROOTS = [
  'web/src',
  relative(root, appleSource),
  'packages/client-core/src',
  'packages/i18n/src',
  'server/internal',
];

const QUALITY_LANES = ['local', 'wifi', 'cellular', 'unknown'];
const QUALITY_FIELDS = ['mode', 'maxVideoBitrateMbps', 'maxAudioBitrateKbps', 'maxVideoHeight', 'allowHDR'];

/** Keys with no reader that another lane must resolve (key: reason). */
export const EXCEPTIONS = {
  'music.shuffleDefault': 'player queue-start work (audit X-02 step 3), not M9; see Questions',
  'music.repeatDefault': 'player queue-start work (audit X-02 step 3), not M9; see Questions',
  'privacy.includeInWatchTogether': 'no enforcement in base; audit says enforce-or-remove (server lane); see Questions',
  'navigation.sidebarCollapsed': 'hidden via managedElsewhere, not user-facing (audit X-01); see Questions',
  'navigation.pinnedLibraryIds': 'pins use the library-pins endpoint; registry key unread (server lane); see Questions',
};

/** `quality.<lane>.*` keys are read through a dynamic lane prefix, never as
 * literals. These files contain the prefix read and cover the whole group. */
const PREFIX_READERS = [
  {prefix: 'quality.', files: ['packages/client-core/src/playback-v1/quality.ts', 'server/internal/playback/delivery_policy.go']},
];

export function extractRegistryKeys(goSource) {
  const keys = new Set();
  for (const m of goSource.matchAll(/pref\w+\("([^"]+)"/g)) keys.add(m[1]);
  for (const lane of QUALITY_LANES) for (const f of QUALITY_FIELDS) keys.add(`quality.${lane}.${f}`);
  return [...keys].sort();
}

export function isExcludedFile(rel) {
  return (
    rel === 'server/internal/operations/preferences.go' ||
    rel.endsWith('preference-copy.ts') ||
    rel.endsWith('preference-labels.ts') ||
    rel.endsWith('catalog/en-US/preferences.ts') ||
    rel.endsWith('check-preference-consumers.mjs') ||
    rel.includes('/test/') ||
    /(^|\/)(test|tests)(\/|$)/.test(rel) ||
    /\.test\.[mc]?[tj]sx?$/.test(rel) ||
    /_test\.go$/.test(rel)
  );
}

export function isExcludedLine(line) {
  if (line.includes('notOnApple') || line.includes('managedElsewhere')) return true;
  const trimmed = line.trim();
  // Comments never read a preference.
  if (trimmed.startsWith('//') || trimmed.startsWith('*') || trimmed.startsWith('/*') || trimmed.startsWith('#')) return true;
  // Bare list literals (`'a.b', 'c.d',` in an exclusion set) name keys
  // precisely because nothing reads them.
  const stripped = line.replace(/'[^']*'|"[^"]*"|`[^`]*`/g, '');
  return /^[\s,\[\]();{}]*$/.test(stripped);
}

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (/\.(go|[mc]?[tj]sx?)$/.test(name)) out.push(p);
  }
  return out;
}

export function findReaders(keys, files) {
  const hits = new Map(keys.map(k => [k, []]));
  const contents = new Map();
  for (const file of files) {
    let text;
    try { text = readFileSync(file, 'utf8'); } catch { continue; }
    contents.set(file, text);
  }
  for (const key of keys) {
    for (const [file, text] of contents) {
      if (!text.includes(key)) continue;
      const rel = relative(root, file);
      if (isExcludedFile(rel)) continue;
      const lines = text.split('\n');
      const at = [];
      lines.forEach((line, i) => {
        if (line.includes(key) && !isExcludedLine(line)) at.push(i + 1);
      });
      if (at.length) hits.get(key).push(`${rel}:${at[0]}`);
    }
  }
  // Dynamic lane-prefix reads cover their whole group.
  for (const {prefix, files: readers} of PREFIX_READERS) {
    const present = readers.filter(f => {
      const text = contents.get(join(root, f));
      return text && text.includes(prefix);
    });
    if (!present.length) continue;
    for (const key of keys) {
      if (key.startsWith(prefix) && !hits.get(key).length) hits.set(key, present.map(f => `${f} (lane prefix)`));
    }
  }
  return hits;
}

function main() {
  const go = readFileSync(REGISTRY, 'utf8');
  const keys = extractRegistryKeys(go);
  const files = SEARCH_ROOTS.flatMap(r => walk(join(root, r)));
  const hits = findReaders(keys, files);
  const missing = [];
  const excused = [];
  for (const key of keys) {
    if (hits.get(key).length) continue;
    if (key in EXCEPTIONS) excused.push(key);
    else missing.push(key);
  }
  for (const [file, lines] of hits) {
    if (lines.length) console.log(`ok   ${file} <- ${lines.slice(0, 3).join(', ')}${lines.length > 3 ? ` (+${lines.length - 3})` : ''}`);
  }
  for (const key of excused) console.log(`warn ${key} (exception: ${EXCEPTIONS[key]})`);
  if (missing.length) {
    console.error(`\nFAIL: ${missing.length} preference key(s) with no reader outside the registry, label files and tests:`);
    for (const key of missing) console.error(`  - ${key}`);
    process.exit(1);
  }
  console.log(`\nPASS: ${keys.length} keys, ${excused.length} documented exception(s).`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
