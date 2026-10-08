#!/usr/bin/env node
// Hard-coded user-facing strings in screens (A11Y-12, D-I18N). A ratchet:
// each file may keep the literals it has today (the baseline); a file that
// gains literals is flagged. Move copy into @portico/i18n (`i18n.t('id')`)
// instead of adding literals; mark a deliberate exception with
// `lint-strings-allow` on the line.
//
//   node packages/i18n/scripts/lint-strings.mjs <app-src-dir> <baseline.json> [--strict] [--update]
//
// --strict  exit 1 when any file exceeds its baseline (default: warn, exit 0)
// --update  lower the baseline to today's counts (never raises an existing entry)
// --all     scan every file under <app-src-dir>, not only screens/, ui/ and app/
import {readdirSync, readFileSync, statSync, writeFileSync, existsSync} from 'node:fs';
import {join, relative, resolve} from 'node:path';
import {pathToFileURL} from 'node:url';

const PROPS = 'label|title|subtitle|body|message|description|help|hint|placeholder|caption|eyebrow|meta|detail|action|confirmLabel|cancelLabel|backLabel|aria-label|accessibilityLabel|accessibilityHint|consequence|lede|error';
const propString = new RegExp(`\\b(?:${PROPS})\\s*=\\s*(?:"([^"]*)"|\\{\\s*'([^']*)'\\s*\\}|\\{\\s*\`([^\`]*)\`\\s*\\})`, 'g');
const objectString = new RegExp(`\\b(?:${PROPS})\\s*:\\s*(?:'([^']*)'|"([^"]*)"|\`([^\`]*)\`)`, 'g');
const jsxText = />\s*([^<>{}\n]*[A-Za-z][^<>{}\n]*?)\s*</g;
// MU7 (CON-18 addendum): copy hidden in a JSX expression still reaches the
// screen — `label={cond ? 'Copy' : 'Other'}` and `>{cond ? 'Copy' : 'Other'}<`
// slipped past propString/jsxText (e.g. ServerSetup's recovery strings and
// SignIn's lede). These match the copy positions above, so quoted literals
// inside the braces are checked with the same isCopy rule. Lines with nested
// braces (interpolations) are a known remaining gap, not a pass.
const propExprString = new RegExp(`\\b(?:${PROPS})\\s*=\\s*\\{([^{}]*)\\}`, 'g');
const jsxExprString = />\s*\{([^{}]*)\}\s*</g;
const quotedString = /"([^"`]{1,200}?)"|'([^'`]{1,200}?)'|`([^`]{1,200}?)`/g;

/** Human copy: a word of 2+ letters, and a space, a capital first letter or a typographic apostrophe. */
export function isCopy(text) {
  const t = (text ?? '').replace(/\$\{[^}]*\}/g, 'x').trim();
  if (!/[A-Za-z]{2,}/.test(t)) return false;
  if (/^[a-z][a-zA-Z0-9]*$/.test(t) || /^[a-z0-9_.:/-]+$/.test(t)) return false; // identifiers, ids, paths
  if (/^(?:https?:|\/)/.test(t)) return false;
  return /\s/.test(t) || /^[A-Z]/.test(t) || t.includes('’');
}

export function countLiterals(source, {jsx = true} = {}) {
  let count = 0;
  const hits = [];
  source.split('\n').forEach((line, i) => {
    if (line.includes('lint-strings-allow')) return;
    const trimmed = line.trim();
    if (trimmed.startsWith('//') || trimmed.startsWith('*') || trimmed.startsWith('/*') || trimmed.startsWith('import ')) return;
    for (const re of jsx ? [propString, objectString, jsxText, propExprString, jsxExprString] : [propString, objectString]) {
      re.lastIndex = 0;
      for (const m of line.matchAll(re)) {
        if (re === propExprString || re === jsxExprString) {
          // A lone `prop={'Copy'}` is already owned by propString above; only
          // count when the braces hold an expression (ternary, condition).
          if (re === propExprString && /^\s*(?:"[^"]*"|'[^']*'|`[^`]*`)\s*$/.test(m[1])) continue;
          quotedString.lastIndex = 0;
          for (const q of m[1].matchAll(quotedString)) {
            const text = q[1] ?? q[2] ?? q[3];
            // A match that spans code (`'' : tick.kind === 'day'`) is not copy.
            if (/===|!==|&&|\|\||=>/.test(text)) continue;
            if (isCopy(text)) { count++; hits.push({line: i + 1, text: text.trim().slice(0, 60)}); }
          }
          continue;
        }
        const text = m[1] ?? m[2] ?? m[3];
        if (re === jsxText && /[=;()]|=>|&&|\?\s|:\s/.test(text)) continue; // expressions, not JSX text
        if (re === jsxText && /(?:^|\s|\|)(?:new\s+)?(?:Promise|ReturnType|Record|Readonly|Partial|Array|Set|Map)$/.test(text.trim())) continue; // TypeScript generics
        if (isCopy(text)) { count++; hits.push({line: i + 1, text: text.trim().slice(0, 60)}); }
      }
    }
  });
  return {count, hits};
}

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (/\.tsx?$/.test(name) && !/\.(d|test)\.tsx?$/.test(name)) out.push(p);
  }
  return out;
}

function main() {
const args = process.argv.slice(2);
const flags = new Set(args.filter(a => a.startsWith('--')));
const [srcDir, baselinePath] = args.filter(a => !a.startsWith('--'));
if (!srcDir || !baselinePath) {
  console.error('usage: lint-strings.mjs <app-src-dir> <baseline.json> [--strict] [--update]');
  process.exit(2);
}

const root = resolve(srcDir);
// Screens, and the shared UI and app layers (their copy is shared by every screen).
const scope = flags.has('--all') ? 'src' : 'screens, ui and app';
const files = flags.has('--all') ? walk(root) : ['screens', 'ui', 'app'].map(d => join(root, d)).filter(existsSync).flatMap(d => walk(d));
const baseline = existsSync(baselinePath) ? JSON.parse(readFileSync(baselinePath, 'utf8')) : {};
const current = {};
const over = [];
for (const file of files) {
  const rel = relative(root, file);
  const {count, hits} = countLiterals(readFileSync(file, 'utf8'), {jsx: file.endsWith('.tsx')});
  if (count) current[rel] = count;
  const allowed = baseline[rel] ?? 0;
  if (count > allowed) over.push({rel, count, allowed, hits});
}
const total = Object.values(current).reduce((a, b) => a + b, 0);
if (flags.has('--update')) {
  // A ratchet: an entry only goes down. A file over its baseline keeps the old number (and keeps
  // failing); a file new to the scan starts at its count (review the diff).
  const next = {};
  for (const [rel, count] of Object.entries(current)) next[rel] = baseline[rel] === undefined ? count : Math.min(baseline[rel], count);
  writeFileSync(baselinePath, JSON.stringify(Object.fromEntries(Object.entries(next).sort()), null, 2) + '\n');
  console.log(`strings baseline updated: ${total} literals in ${Object.keys(current).length} files`);
  return 0;
}
for (const o of over) {
  console.warn(`${o.rel}: ${o.count} hard-coded strings (baseline ${o.allowed}). Move new copy into @portico/i18n. Newest-looking:`);
  for (const h of o.hits.slice(-Math.max(1, o.count - o.allowed))) console.warn(`  ${o.rel}:${h.line}: "${h.text}"`);
}
const baseTotal = Object.values(baseline).reduce((a, b) => a + b, 0);
console.log(`hard-coded strings in ${scope}: ${total} (baseline ${baseTotal})${over.length ? `, ${over.length} file(s) over baseline` : ''}`);
return over.length && flags.has('--strict') ? 1 : 0;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) process.exit(main());
