// The translated catalogues: where they live, what a valid translation is, and how they turn into
// the runtime modules. Shared by scripts/translations.mjs (the command line) and the tests.
//
// translations/<locale>.json is the record a translator's work is kept in. Each message keeps the
// English it was translated from (`source`), so a change to the en-US catalogue shows up as a
// stale translation instead of passing silently, and how far it can be trusted (`state`):
//
//   machine     machine translation; needs a native speaker's review
//   translated  a person translated it; not yet reviewed
//   reviewed    a native speaker reviewed it
//
// src/catalog/translated/<locale>.ts is generated from it: only the text, and only translations
// that still parse and still take the same arguments as today's English. Anything else falls back
// to en-US at runtime, which is always safe.
import {readFileSync, readdirSync, writeFileSync, existsSync, mkdirSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {enUS} from '../src/catalog/en-US.ts';
import {messageStructure} from '../src/messageformat.ts';
import {variantBase} from '../src/catalog/index.ts';

/** A regional variant's base language (fr-CA → fr); its record holds only the messages it words differently. */
export const baseOf = locale => variantBase[locale];

const here = dirname(fileURLToPath(import.meta.url));
export const root = resolve(here, '..');
export const translationsDir = join(root, 'translations');
export const generatedDir = join(root, 'src/catalog/translated');

/** The translated locales: catalogue id → English name, for reports and translator files. */
export const translatedLocales = {
  fr: 'French (France)', es: 'Spanish (Spain)', de: 'German', ru: 'Russian',
  'pt-BR': 'Portuguese (Brazil)', it: 'Italian', nl: 'Dutch', pl: 'Polish', sv: 'Swedish', da: 'Danish', nb: 'Norwegian Bokmål', cs: 'Czech', tr: 'Turkish', uk: 'Ukrainian', 'zh-Hans': 'Chinese (Simplified)', 'zh-Hant': 'Chinese (Traditional)', ja: 'Japanese', ko: 'Korean',
  'fr-CA': 'French (Canada)', 'es-419': 'Spanish (Latin America)', 'pt-PT': 'Portuguese (Portugal)', el: 'Greek', hu: 'Hungarian', ro: 'Romanian', fi: 'Finnish', id: 'Indonesian', vi: 'Vietnamese', th: 'Thai', hi: 'Hindi',
};
export const states = ['machine', 'translated', 'reviewed'];

/** Messages that read the same in every language: a language's own name is written in that language. */
export const untranslatable = id => id.startsWith('locale.');
export const translatableIds = () => Object.keys(enUS).filter(id => !untranslatable(id));

/** Names that are never translated; a translation keeps every one its English has. */
export const productNames = ['Portico Services', 'Portico Account', 'Portico'];

/** Every integer 0…1000 lands in one of these: a plural must have a branch for each, plus `other`. */
const categories = new Map();
export function requiredCategories(locale) {
  if (!categories.has(locale)) {
    const rules = new Intl.PluralRules(locale);
    const found = new Set(['other']);
    for (let n = 0; n <= 1000; n++) found.add(rules.select(n));
    categories.set(locale, [...found].sort());
  }
  return categories.get(locale);
}

const count = (text, word) => text.split(word).length - 1;
/** Product names without the longer names that contain them: "Portico" in "Portico Account" is not a second "Portico". */
function names(text) {
  let rest = text;
  const out = {};
  for (const name of productNames) { out[name] = count(rest, name); rest = rest.split(name).join(' '); }
  return out;
}

/** An argument may be used more specifically in a translation: a bare `{n}` English counts with can become a plural. */
const upgrades = {text: ['number', 'plural'], number: ['plural']};

/**
 * The problems with one translation, as sentences; empty when it can be used. Checks that it parses,
 * takes exactly the source's arguments, keeps every select key and exact plural key, gives each
 * plural the locale's own categories, keeps product names and surrounding spaces, and uses the
 * typographic apostrophe (an ASCII one is ICU quoting and can swallow an argument).
 */
export function checkTranslation(locale, id, text, source = enUS[id]) {
  const problems = [];
  if (source === undefined) return [`${id} is not an en-US message`];
  if (typeof text !== 'string' || (!text.trim() && source.trim())) return ['empty translation'];
  let target, english;
  try { english = messageStructure(source); } catch (e) { return [`the English does not parse: ${e.message}`]; }
  try { target = messageStructure(text); } catch (e) { return [`does not parse: ${e.message.replace(/ in message:.*$/s, '')}`]; }
  for (const [name, kind] of Object.entries(english.args)) {
    const used = target.args[name];
    if (!used) problems.push(`missing the argument {${name}}`);
    else if (used !== kind && !(upgrades[kind] ?? []).includes(used)) problems.push(`{${name}} is a ${kind} in English but a ${used} here`);
  }
  for (const name of Object.keys(target.args)) if (!(name in english.args)) problems.push(`{${name}} is not an argument of the English`);
  for (const [name, keys] of Object.entries(english.selects)) {
    const have = target.selects[name];
    if (have && have.join() !== keys.join()) problems.push(`select {${name}} must keep exactly the keys ${keys.join(', ')}`);
  }
  const needed = requiredCategories(locale);
  for (const plural of target.plurals) {
    if (plural.ordinal) continue;
    const missing = needed.filter(c => !plural.categories.includes(c));
    if (missing.length) problems.push(`plural {${plural.name}} needs ${locale}’s categories: missing ${missing.join(', ')}`);
  }
  for (const plural of english.plurals) {
    const mine = target.plurals.filter(p => p.name === plural.name && p.ordinal === plural.ordinal);
    if (!mine.length) continue;
    for (const exact of plural.exact) if (!mine.some(p => p.exact.includes(exact))) problems.push(`plural {${plural.name}} must keep its ${exact} branch`);
    if (mine.some(p => p.offset !== plural.offset)) problems.push(`plural {${plural.name}} must keep offset:${plural.offset}`);
  }
  const wanted = names(source), got = names(text);
  for (const name of productNames) if (wanted[name] && got[name] < 1) problems.push(`keep the product name “${name}” untranslated`);
  if (text.includes("'") && !source.includes("'")) problems.push('use ’ (U+2019), not the ASCII apostrophe, which is ICU quoting');
  const lead = s => /^\s*/.exec(s)[0], trail = s => /\s*$/.exec(s)[0];
  if (lead(text) !== lead(source) || trail(text) !== trail(source)) problems.push('keep the leading and trailing spaces of the English');
  return problems;
}

/** A record's translation of a message when it can be used (it fits today's English), else undefined. */
export function usableText(locale, id, record) {
  const m = record.messages[id];
  return m && !checkTranslation(locale, id, m.text).length ? m.text : undefined;
}

/** translations/<locale>.json, or an empty record. */
export function readTranslations(locale) {
  const file = join(translationsDir, `${locale}.json`);
  if (!existsSync(file)) return {locale, name: translatedLocales[locale], messages: {}};
  return JSON.parse(readFileSync(file, 'utf8'));
}

export function writeTranslations(record) {
  mkdirSync(translationsDir, {recursive: true});
  const messages = Object.fromEntries(Object.keys(record.messages).sort().map(id => {
    const m = record.messages[id];
    return [id, {text: m.text, source: m.source, state: m.state}];
  }));
  writeFileSync(join(translationsDir, `${record.locale}.json`), JSON.stringify({locale: record.locale, name: translatedLocales[record.locale] ?? record.name, messages}, null, 1) + '\n');
}

/**
 * Where each translatable message stands for a locale: `current` (translated from today's English),
 * `stale` (translated from older English; still used while its arguments match), `invalid` (fails
 * the checks; not used), `missing`.
 */
export function classify(locale, record = readTranslations(locale), baseRecord = baseOf(locale) ? readTranslations(baseOf(locale)) : undefined) {
  const out = {current: [], stale: [], invalid: [], missing: [], inherited: [], orphaned: [], byState: Object.fromEntries(states.map(s => [s, 0]))};
  for (const id of translatableIds()) {
    const m = record.messages[id];
    if (!m) {
      // A variant reads its base's translation for anything it does not override.
      if (baseRecord && usableText(baseOf(locale), id, baseRecord) !== undefined) out.inherited.push(id);
      else out.missing.push(id);
      continue;
    }
    const problems = checkTranslation(locale, id, m.text);
    if (problems.length) { out.invalid.push({id, problems}); continue; }
    (m.source === enUS[id] ? out.current : out.stale).push(id);
    out.byState[m.state] = (out.byState[m.state] ?? 0) + 1;
  }
  for (const id of Object.keys(record.messages)) if (!(id in enUS) || untranslatable(id)) out.orphaned.push(id);
  return out;
}

/** Context for a variant's translator. */
export function variantNote(locale) {
  return `${translatedLocales[locale]} reads ${translatedLocales[baseOf(locale)]} (${baseOf(locale)}) for every message it does not override. Change the text only where ${locale} words it differently; a target left as the ${baseOf(locale)} text stays inherited.`;
}

const quote = text => `'${text.replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/\n/g, '\\n')}'`;

/** The runtime module for a locale: the usable translations, in catalogue order. */
export function generatedModule(locale, record = readTranslations(locale)) {
  const status = classify(locale, record);
  const usable = new Set([...status.current, ...status.stale]);
  const lines = Object.keys(enUS).filter(id => usable.has(id)).map(id => `  ${quote(id)}: ${quote(record.messages[id].text)},`);
  return `// Generated by packages/i18n/scripts/translations.mjs from translations/${locale}.json. Do not edit:
// export the locale, translate, and import it (see packages/i18n/TRANSLATING.md).
// ${translatedLocales[locale]}. Machine-translated messages are marked in translations/${locale}.json and need a native speaker's review.
import type {MessageId} from '../en-US.ts';

export const messages: Partial<Record<MessageId, string>> = {
${lines.join('\n')}
};
`;
}

export function generatedPath(locale) { return join(generatedDir, `${locale}.ts`); }

/**
 * Context for a translator: the comments above a message in the en-US sources. A comment with a
 * "──" rule heads a section until the next one; any other comment covers the messages under it
 * until a blank line.
 */
export function messageNotes() {
  const dir = join(root, 'src/catalog');
  const files = [join(dir, 'en-US.ts'), ...readdirSync(join(dir, 'en-US')).filter(f => f.endsWith('.ts')).map(f => join(dir, 'en-US', f))];
  const notes = {};
  for (const file of files) {
    let section = '', local = [], pendingLocal = false;
    for (const line of readFileSync(file, 'utf8').split('\n')) {
      const trimmed = line.trim();
      if (trimmed.startsWith('//')) {
        const text = trimmed.replace(/^\/\/\s?/, '');
        if (text.includes('──')) { section = text.replace(/[─]+/g, '').replace(/\s+/g, ' ').trim(); local = []; pendingLocal = false; continue; }
        if (!pendingLocal) local = [];
        local.push(text);
        pendingLocal = true;
        continue;
      }
      if (!trimmed) { local = []; pendingLocal = false; continue; }
      pendingLocal = false;
      const m = /^'([^']+)'\s*:/.exec(trimmed);
      if (!m) continue;
      const parts = [section, local.join(' ')].filter(Boolean);
      if (parts.length) notes[m[1]] = parts.join(' — ');
    }
  }
  return notes;
}
