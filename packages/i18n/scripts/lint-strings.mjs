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
import {createRequire} from 'node:module';

const PROPS = 'label|title|subtitle|body|message|description|help|hint|placeholder|caption|eyebrow|meta|detail|action|confirmLabel|cancelLabel|backLabel|aria-label|accessibilityLabel|accessibilityHint|consequence|lede|error';
const copyNames = new Set(PROPS.split('|'));
// Reuse the installed web/native developer tool, not a new parser dependency.
// Native packages live beside apps/native's install; Vega shares that tooling.
const parserLocations = [import.meta.url, pathToFileURL(resolve('package.json')), new URL('../../../apps/native/package.json', import.meta.url)];
let parserPath;
for (const location of parserLocations) {
  try { parserPath = createRequire(location).resolve('rolldown/utils'); break; }
  catch (error) { if (error.code !== 'MODULE_NOT_FOUND') throw error; }
}
if (!parserPath) throw new Error('String lint requires the existing web or native Rolldown developer tools.');
const {parseSync} = await import(pathToFileURL(parserPath).href);

/** Human copy: a word of 2+ letters, and a space, a capital first letter or a typographic apostrophe. */
export function isCopy(text) {
  const t = (text ?? '').replace(/\$\{[^}]*\}/g, 'x').trim();
  if (!/[A-Za-z]{2,}/.test(t)) return false;
  if (/^[a-z][a-zA-Z0-9]*$/.test(t) || /^[a-z0-9_.:/-]+$/.test(t)) return false; // identifiers, ids, paths
  if (/^(?:https?:|\/)/.test(t)) return false;
  return /\s/.test(t) || /^[A-Z]/.test(t) || t.includes('’');
}

export function countLiterals(source, {jsx = true} = {}) {
  const lines = source.split('\n'), hits = [], seen = new Set();
  const lineStarts = [0];
  for (let i = 0; i < source.length; i++) if (source[i] === '\n') lineStarts.push(i + 1);
  function add(text, offset) {
    let lo = 0, hi = lineStarts.length;
    while (lo + 1 < hi) { const mid = (lo + hi) >>> 1; if (lineStarts[mid] <= offset) lo = mid; else hi = mid; }
    if (lines[lo].includes('lint-strings-allow') || !isCopy(text) || seen.has(offset)) return;
    seen.add(offset); hits.push({line: lo + 1, text: text.trim().slice(0, 60), offset});
  }
  const parse = text => parseSync(jsx ? 'strings.tsx' : 'strings.ts', text, {sourceType: 'unambiguous'});
  function scan(program, shift = 0) {
    function visit(node, copy = false, parent) {
      if (!node || typeof node !== 'object' || !node.type) return;
      // Type names, literal types and import specifiers never render. Assertions
      // still contain a runtime expression, which keeps its surrounding context.
      if (node.type.startsWith('TS')) {
        if (node.expression) visit(node.expression, copy, node);
        else if (node.type === 'TSModuleDeclaration') visit(node.body, false, node);
        else if (node.type === 'TSModuleBlock') node.body.forEach(child => visit(child, false, node));
        else if (node.type === 'TSParameterProperty') visit(node.parameter, false, node);
        return;
      }
      if (node.type === 'ImportDeclaration' || node.type === 'ExportAllDeclaration') return;
      if (node.type === 'JSXText') {
        if (jsx) {
          let offset = node.start + shift;
          const rawLines = (node.raw ?? node.value).split('\n');
          node.value.split('\n').forEach((line, index) => { add(line, offset); offset += (rawLines[index] ?? line).length + 1; });
        }
        return;
      }
      if (node.type === 'Literal') { if (copy && typeof node.value === 'string') add(node.value, node.start + shift); return; }
      if (node.type === 'TemplateLiteral') {
        if (copy) add(node.quasis.map(part => part.value.cooked ?? part.value.raw).join('${x}'), node.start + shift);
        node.expressions.forEach(expression => visit(expression, copy, node)); return;
      }
      if (node.type === 'JSXAttribute') {
        visit(node.value, jsx && copyNames.has(node.name.name), node); return;
      }
      if (node.type === 'JSXExpressionContainer') {
        visit(node.expression, copy || jsx && (parent?.type === 'JSXElement' || parent?.type === 'JSXFragment'), node); return;
      }
      if (node.type === 'Property' || node.type === 'PropertyDefinition') {
        const name = node.computed ? undefined : node.key.name ?? node.key.value;
        visit(node.value, copy || copyNames.has(name), node); return;
      }
      if (node.type === 'VariableDeclarator' || node.type === 'AssignmentExpression' || node.type === 'AssignmentPattern') {
        const target = node.id ?? node.left;
        const name = target?.type === 'Identifier' ? target.name : target?.type === 'MemberExpression' && !target.computed ? target.property.name : undefined;
        visit(node.init ?? node.right, copy || copyNames.has(name), node);
        visit(target, false, node); return;
      }
      for (const [key, value] of Object.entries(node)) if (key !== 'parent') {
        if (Array.isArray(value)) value.forEach(child => visit(child, copy, node));
        else if (value && typeof value === 'object') visit(value, copy, node);
      }
    }
    visit(program);
  }
  const parsed = parse(source);
  if (!parsed.errors.length) scan(parsed.program);
  else {
    // countLiterals also accepts the existing independent one-line snippets
    // used by the core lint contract. Each must parse in full as an expression;
    // malformed real source fails closed rather than disappearing from counts.
    for (let i = 0; i < lines.length; i++) {
      if (!lines[i].trim() || lines[i].trim().startsWith('//')) continue;
      const fragment = parse(`(${lines[i]}\n)`);
      if (fragment.errors.length) throw new SyntaxError(`String lint cannot parse source at line ${i + 1}: ${parsed.errors.map(error => error.message).join('; ')}`);
      scan(fragment.program, lineStarts[i] - 1);
    }
  }
  hits.sort((a, b) => a.offset - b.offset);
  return {count: hits.length, hits: hits.map(({line, text}) => ({line, text}))};
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
