#!/usr/bin/env node
// The translated catalogues from the command line (see packages/i18n/TRANSLATING.md).
//
//   node packages/i18n/scripts/translations.mjs status [--json]           completeness of every locale
//   node packages/i18n/scripts/translations.mjs export <locale> [--all]   XLIFF 1.2 on stdout: what needs work
//                                                                          (missing, stale, invalid, machine), or --all
//   node packages/i18n/scripts/translations.mjs import <locale> <file.xlf> [--state machine|translated|reviewed]
//   node packages/i18n/scripts/translations.mjs build                     regenerate src/catalog/translated/*.ts
//   node packages/i18n/scripts/translations.mjs check                     fail when a generated module is out of date
//                                                                          or a translation is unusable
//
// Import takes every unit with a target, checks it (it must parse and take the English's
// arguments), and keeps the English the translator saw as the message's `source`. A unit whose
// English has since changed is kept but reported stale. XLIFF states map to ours: final and
// signed-off are `reviewed`; translated and needs-review-l10n are `translated`;
// needs-review-translation is `machine`; new and needs-translation units are skipped.
import {readFileSync, writeFileSync, existsSync} from 'node:fs';
import {pathToFileURL} from 'node:url';
import {resolve} from 'node:path';
import {enUS} from '../src/catalog/en-US.ts';
import {baseOf, checkTranslation, classify, generatedModule, generatedPath, messageNotes, readTranslations, states, translatableIds, translatedLocales, usableText, variantNote, writeTranslations} from './translations-lib.mjs';

const escape = text => text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
const unescape = text => text
  .replace(/<!\[CDATA\[([\s\S]*?)\]\]>/g, (_, raw) => raw.replace(/&/g, '&amp;').replace(/</g, '&lt;'))
  .replace(/&#x([0-9a-f]+);/gi, (_, hex) => String.fromCodePoint(parseInt(hex, 16)))
  .replace(/&#(\d+);/g, (_, dec) => String.fromCodePoint(Number(dec)))
  .replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&apos;/g, "'").replace(/&amp;/g, '&');

const xliffState = {machine: 'needs-review-translation', translated: 'translated', reviewed: 'final'};

export function exportXliff(locale, {all = false} = {}) {
  const record = readTranslations(locale);
  const base = baseOf(locale);
  const baseRecord = base ? readTranslations(base) : undefined;
  const status = classify(locale, record, baseRecord);
  // A variant's translator reviews everything: the base's words are its starting point.
  if (base) all = true;
  const invalid = new Map(status.invalid.map(i => [i.id, i.problems]));
  const stale = new Set(status.stale);
  const notes = messageNotes();
  const units = [];
  for (const id of translatableIds()) {
    const m = record.messages[id];
    const needsWork = !m || stale.has(id) || invalid.has(id) || m.state === 'machine';
    if (!all && !needsWork) continue;
    const parts = [`      <trans-unit id="${escape(id)}" resname="${escape(id)}">`, `        <source>${escape(enUS[id])}</source>`];
    const inherited = !m && baseRecord ? usableText(base, id, baseRecord) : undefined;
    if (m) parts.push(`        <target state="${stale.has(id) || invalid.has(id) ? 'needs-translation' : xliffState[m.state] ?? 'translated'}">${escape(m.text)}</target>`);
    else if (inherited !== undefined) parts.push(`        <target state="${xliffState[baseRecord.messages[id].state] ?? 'translated'}">${escape(inherited)}</target>`);
    if (notes[id]) parts.push(`        <note from="developer">${escape(notes[id])}</note>`);
    if (inherited !== undefined) parts.push(`        <note from="portico">Inherited from ${base}. ${escape(variantNote(locale))}</note>`);
    if (m && stale.has(id)) parts.push(`        <note from="portico">The English changed after this was translated. It was: ${escape(m.source)}</note>`);
    if (invalid.has(id)) parts.push(`        <note from="portico">Not usable: ${escape(invalid.get(id).join('; '))}</note>`);
    if (m?.state === 'machine' && !stale.has(id) && !invalid.has(id)) parts.push('        <note from="portico">Machine translation: review it, then mark it final.</note>');
    parts.push('      </trans-unit>');
    units.push(parts.join('\n'));
  }
  return `<?xml version="1.0" encoding="UTF-8"?>
<!-- Portico catalogue, ${translatedLocales[locale]}. ICU MessageFormat: keep {arguments}, plural/select keywords and keys,
     and #; use the typographic apostrophe (’), never ', which is ICU quoting. Mark reviewed units final. -->
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="portico" source-language="en-US" target-language="${locale}" datatype="plaintext">
    <body>
${units.join('\n')}
    </body>
  </file>
</xliff>
`;
}

const ourState = {final: 'reviewed', 'signed-off': 'reviewed', translated: 'translated', 'needs-review-l10n': 'translated', 'needs-review-adaptation': 'translated', 'needs-adaptation': 'translated', 'needs-l10n': 'translated', 'needs-review-translation': 'machine'};

/** Read XLIFF 1.2 trans-units (the subset translation tools write back: source, target, state). */
export function parseXliff(xml) {
  const units = [];
  for (const [, attributes, body] of xml.matchAll(/<trans-unit\b([^>]*)>([\s\S]*?)<\/trans-unit>/g)) {
    const id = unescape(/\bid="([^"]*)"/.exec(attributes)?.[1] ?? '');
    const source = /<source\b[^>]*>([\s\S]*?)<\/source>/.exec(body);
    const target = /<target\b([^>]*)>([\s\S]*?)<\/target>/.exec(body);
    if (!id || !source) continue;
    units.push({id, source: unescape(source[1]), target: target ? unescape(target[2]) : undefined, state: target ? /\bstate="([^"]*)"/.exec(target[1])?.[1] : undefined});
  }
  return units;
}

/** Merge an XLIFF file's translations into a locale's record (in place); the caller writes it. */
export function applyXliff(locale, xml, record, {state, baseRecord = baseOf(locale) ? readTranslations(baseOf(locale)) : undefined} = {}) {
  const report = {imported: 0, stale: 0, skipped: 0, inherited: 0, rejected: []};
  for (const unit of parseXliff(xml)) {
    if (!(unit.id in enUS) || unit.target === undefined || !unit.target.trim()) { report.skipped++; continue; }
    const kept = state ?? ourState[unit.state ?? 'translated'];
    if (!kept) { report.skipped++; continue; }
    const problems = checkTranslation(locale, unit.id, unit.target);
    if (problems.length) { report.rejected.push({id: unit.id, problems}); continue; }
    // A variant's target that is still its base's words is no override: it stays (or becomes) inherited.
    if (baseRecord && unit.target === usableText(baseOf(locale), unit.id, baseRecord)) { delete record.messages[unit.id]; report.inherited++; continue; }
    record.messages[unit.id] = {text: unit.target, source: unit.source, state: kept};
    report.imported++;
    if (unit.source !== enUS[unit.id]) report.stale++;
  }
  return report;
}

export function statusReport() {
  const total = translatableIds().length;
  return Object.keys(translatedLocales).map(locale => {
    const s = classify(locale);
    return {locale, name: translatedLocales[locale], base: baseOf(locale), total, current: s.current.length, inherited: s.inherited.length, stale: s.stale.length, invalid: s.invalid.length, missing: s.missing.length, orphaned: s.orphaned.length, ...Object.fromEntries(states.map(st => [st, s.byState[st] ?? 0])), complete: Math.floor(((s.current.length + s.inherited.length) / total) * 1000) / 10};
  });
}

function build({write}) {
  let drift = 0;
  for (const locale of Object.keys(translatedLocales)) {
    const want = generatedModule(locale);
    const path = generatedPath(locale);
    const have = existsSync(path) ? readFileSync(path, 'utf8') : '';
    if (have === want) continue;
    drift++;
    if (write) writeFileSync(path, want);
    else console.error(`translations: src/catalog/translated/${locale}.ts is out of date; run: node packages/i18n/scripts/translations.mjs build`);
  }
  return drift;
}

function main(argv) {
  const [command, ...rest] = argv;
  const locale = rest[0];
  const needLocale = () => { if (!(locale in translatedLocales)) { console.error(`translations: name a locale: ${Object.keys(translatedLocales).join(', ')}`); process.exit(2); } };
  switch (command) {
    case 'status': {
      const rows = statusReport();
      if (rest.includes('--json')) { console.log(JSON.stringify(rows, null, 2)); return 0; }
      console.log('| Locale | Complete | Current | Inherited | Stale | Invalid | Missing | Machine | Translated | Reviewed |');
      console.log('|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|');
      for (const r of rows) console.log(`| ${r.locale} ${r.name}${r.base ? ` (from ${r.base})` : ''} | ${r.complete}% | ${r.current} | ${r.inherited} | ${r.stale} | ${r.invalid} | ${r.missing} | ${r.machine} | ${r.translated} | ${r.reviewed} |`);
      console.log(`\n${rows[0]?.total ?? 0} translatable messages. Inherited = a regional variant reads its base language's translation. Stale = translated from English that has since changed (still used while its arguments match). Invalid and missing messages read en-US.`);
      return 0;
    }
    case 'export': needLocale(); process.stdout.write(exportXliff(locale, {all: rest.includes('--all')})); return 0;
    case 'import': {
      needLocale();
      const file = rest[1];
      if (!file || !existsSync(file)) { console.error('translations: import <locale> <file.xlf>'); return 2; }
      const flag = rest.indexOf('--state');
      const state = flag >= 0 ? rest[flag + 1] : undefined;
      if (state && !states.includes(state)) { console.error(`translations: --state is one of ${states.join(', ')}`); return 2; }
      const record = readTranslations(locale);
      const report = applyXliff(locale, readFileSync(file, 'utf8'), record, {state});
      writeTranslations(record);
      for (const r of report.rejected) console.error(`rejected ${r.id}: ${r.problems.join('; ')}`);
      console.log(`imported ${report.imported} (${report.stale} from older English), ${report.inherited ? `left ${report.inherited} inherited from ${baseOf(locale)}, ` : ''}skipped ${report.skipped}, rejected ${report.rejected.length}`);
      build({write: true});
      return report.rejected.length ? 1 : 0;
    }
    case 'build': build({write: true}); return 0;
    case 'check': {
      let failed = build({write: false});
      for (const locale of Object.keys(translatedLocales)) {
        const s = classify(locale);
        const record = readTranslations(locale);
        for (const i of s.invalid) {
          // A translation that no longer fits English changed since is a job for the next export, not a broken build.
          if (record.messages[i.id].source !== enUS[i.id]) { console.warn(`translations: ${locale} ${i.id} is stale and no longer fits the English; it reads en-US until retranslated`); continue; }
          console.error(`translations: ${locale} ${i.id}: ${i.problems.join('; ')}`);
          failed++;
        }
        for (const id of s.orphaned) { console.error(`translations: ${locale} ${id} is not a translatable en-US message; remove it`); failed++; }
      }
      if (failed) return 1;
      console.log('translations check passed: every translation is usable and the generated modules are current');
      return 0;
    }
    default:
      console.error('usage: translations.mjs status [--json] | export <locale> [--all] | import <locale> <file.xlf> [--state …] | build | check');
      return 2;
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) process.exit(main(process.argv.slice(2)));
