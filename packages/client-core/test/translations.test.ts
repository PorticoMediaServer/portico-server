import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {catalogue, catalogueReady, createI18n, enUS, formatMessage, loadCatalogue, provideCatalogue, messageArguments, parseMessage, resolveRegion, translatedLocales, type MessageId} from '../../i18n/src/index.ts';
import {pseudoMessage} from '../../i18n/src/catalog/pseudo.ts';
import {checkTranslation, classify, generatedModule, generatedPath, readTranslations, requiredCategories, translatableIds} from '../../i18n/scripts/translations-lib.mjs';
import {applyXliff, exportXliff, parseXliff} from '../../i18n/scripts/translations.mjs';
import {localeChoices} from '../src/presentation/region-choices.ts';

const region = (locale: string) => resolveRegion({locale}, {locales: ['en-US'], timeZone: 'UTC'});

test('translations: every translated message parses and takes the English arguments, plurals and select keys', async () => {
  for (const locale of translatedLocales) {
    await loadCatalogue(locale);
    assert.ok(catalogueReady(locale), locale);
    for (const [id, text] of Object.entries(catalogue(locale))) {
      assert.ok(id in enUS, `${locale} ${id} is not an en-US message`);
      assert.deepEqual(checkTranslation(locale, id, text!), [], `${locale} ${id}`);
    }
  }
});

test('translations: the generated modules are today’s translations/<locale>.json, and no record holds an unusable translation', () => {
  for (const locale of translatedLocales) {
    assert.equal(readFileSync(generatedPath(locale), 'utf8'), generatedModule(locale), `${locale}: run node packages/i18n/scripts/translations.mjs build`);
    const record = readTranslations(locale);
    const broken = classify(locale, record).invalid.filter(i => record.messages[i.id]!.source === enUS[i.id as MessageId]);
    assert.deepEqual(broken, [], locale);
  }
});

test('translations: a message a locale lacks reads en-US, before and after its catalogue loads', async () => {
  const fr = createI18n(region('fr-FR'));
  assert.equal(fr.locale, 'fr');
  await loadCatalogue('fr');
  const words = catalogue('fr');
  const untranslated = (Object.keys(enUS) as MessageId[]).find(id => words[id] === undefined)!;
  assert.equal(createI18n(region('fr')).t(untranslated), enUS[untranslated] && formatMessage(enUS[untranslated], {}, {locale: 'fr'}));
  // A language's own name is never translated, so it is always the en-US text.
  assert.equal(createI18n(region('de')).t('locale.fr'), 'Français (France)');
});

test('translations: plural forms follow the catalogue’s language, numbers the format locale', async () => {
  await loadCatalogue('ru');
  const ru = createI18n(region('ru'));
  const forms = [1, 2, 5, 21, 112].map(n => ru.t('count.titles', {count: n}));
  // one, few, many, one, many: the noun agrees with the number.
  assert.equal(new Set(forms.map(f => f.replace(/[\d\s]/g, ''))).size, 3, forms.join(' | '));
  assert.equal(forms[0]!.replace(/[\d\s]/g, ''), forms[3]!.replace(/[\d\s]/g, ''));
  assert.equal(forms[2]!.replace(/[\d\s]/g, ''), forms[4]!.replace(/[\d\s]/g, ''));
  // A message Japanese lacks reads English and keeps English plurals: Japanese has no "one" form.
  await loadCatalogue('ja');
  const japanese = createI18n(resolveRegion({locale: 'ja-JP'}, {locales: ['en-US'], timeZone: 'UTC'}));
  assert.equal(japanese.locale, 'ja');
  assert.equal(japanese.t('count.titles', {count: 1}), '1 title');
  const english = createI18n(resolveRegion({locale: 'sw-KE'}, {locales: ['en-US'], timeZone: 'UTC'}));
  assert.equal(english.locale, 'en-US');
  assert.equal(english.t('count.titles', {count: 1}), '1 title');
  assert.equal(formatMessage('{n, plural, one {# título} other {# títulos}}', {n: 1000}, {locale: 'de-DE', pluralLocale: 'es'}), '1.000 títulos');
});

test('translation checks: arguments, apostrophes, plural categories, select keys, product names', () => {
  assert.deepEqual(requiredCategories('ru'), ['few', 'many', 'one', 'other']);
  assert.deepEqual(requiredCategories('fr'), ['one', 'other']);
  const id = 'error.rateLimitedSeconds' as const;
  assert.deepEqual(checkTranslation('fr', id, 'Trop de tentatives. Réessayez dans {seconds, plural, one {# seconde} other {# secondes}}.'), []);
  assert.match(checkTranslation('ru', id, 'Повторите через {seconds, plural, one {# секунду} other {# секунд}}.').join(), /missing few, many/);
  assert.match(checkTranslation('fr', id, 'Réessayez plus tard.').join(), /missing the argument \{seconds\}/);
  assert.match(checkTranslation('fr', 'pin.progress', "{count} sur {length} chiffres saisis, c'est tout").join(), /U\+2019/);
  assert.match(checkTranslation('fr', 'pin.progress', '{count} sur {lenght} chiffres').join(), /\{lenght\} is not an argument/);
  // A bare number in English may become a plural where the language needs the noun to agree.
  assert.deepEqual(checkTranslation('ru', 'pin.progress', 'Введено {count} из {length, plural, one {# цифры} few {# цифр} many {# цифр} other {# цифры}}'), []);
  const accountId = (Object.keys(enUS) as MessageId[]).find(k => enUS[k] === 'Portico can’t reach your Portico Account right now. It will keep trying.')!;
  assert.match(checkTranslation('de', accountId, 'Portico erreicht dein Konto gerade nicht. Es wird weiter versucht.').join(), /Portico Account/);
  const selectId = (Object.keys(enUS) as MessageId[]).find(k => /\{\w+, select, /.test(enUS[k]))!;
  const renamed = enUS[selectId].replace(/(\{\w+, select, )(\w+)/, '$1renamed');
  assert.match(checkTranslation('de', selectId, renamed).join(), /must keep exactly the keys/);
});

test('XLIFF: export carries source, target, state and context; import takes it back and keeps the English it translated', () => {
  const locale = 'de';
  const record = readTranslations(locale);
  const xml = exportXliff(locale, {all: true});
  const units = parseXliff(xml);
  assert.equal(units.length, translatableIds().length);
  const translated = units.find(u => u.target !== undefined);
  if (translated) {
    const copy = {...record, messages: {} as typeof record.messages};
    const report = applyXliff(locale, xml, copy);
    assert.equal(report.rejected.length, 0);
    assert.equal(copy.messages[translated.id]!.text, record.messages[translated.id]!.text);
    assert.equal(copy.messages[translated.id]!.state, record.messages[translated.id]!.state);
  }
  // A reviewer's file: entities and CDATA decode, final means reviewed, a broken target is refused.
  const id = 'error.rateLimitedSeconds';
  const file = `<xliff version="1.2"><file source-language="en-US" target-language="de"><body>
    <trans-unit id="${id}"><source>${enUS[id].replace(/</g, '&lt;')}</source><target state="final">Zu viele Versuche &amp; so. Versuche es in {seconds, plural, one {# Sekunde} other {# Sekunden}} erneut.</target></trans-unit>
    <trans-unit id="error.offline"><source>${enUS['error.offline']}</source><target state="needs-review-translation"><![CDATA[Dieses Gerät ist offline.]]></target></trans-unit>
    <trans-unit id="error.busy"><source>${enUS['error.busy']}</source><target state="translated">Kaputt {oops}</target></trans-unit>
    <trans-unit id="error.timeout"><source>An older English text.</source><target state="translated">Dein Server hat zu lange gebraucht.</target></trans-unit>
  </body></file></xliff>`;
  const into = {locale, name: 'German', messages: {} as Record<string, {text: string; source: string; state: string}>};
  const report = applyXliff(locale, file, into);
  assert.equal(into.messages[id]!.state, 'reviewed');
  assert.match(into.messages[id]!.text, /Versuche &amp;|Versuche & so/);
  assert.equal(into.messages['error.offline']!.state, 'machine');
  assert.equal(into.messages['error.offline']!.text, 'Dieses Gerät ist offline.');
  assert.deepEqual(report.rejected.map(r => r.id), ['error.busy']);
  assert.equal(report.stale, 1);
  assert.equal(into.messages['error.timeout']!.source, 'An older English text.');
});

test('a registered language with nothing translated exports every message, and a partial import is usable', () => {
  for (const locale of ['pt-BR', 'pl', 'uk', 'zh-Hant', 'ja'] as const) {
    const units = parseXliff(exportXliff(locale));
    assert.equal(units.length, translatableIds().length, locale);
    assert.ok(units.every(u => u.target === undefined), locale);
  }
  // Polish needs one, few and many; Chinese needs only other.
  assert.deepEqual(requiredCategories('pl'), ['few', 'many', 'one', 'other']);
  assert.deepEqual(requiredCategories('cs'), ['few', 'one', 'other']);
  assert.deepEqual(requiredCategories('zh-Hans'), ['other']);
  const file = `<xliff version="1.2"><file source-language="en-US" target-language="zh-Hans"><body>
    <trans-unit id="count.titles"><source>${enUS['count.titles']}</source><target state="translated">{count, plural, other {# 个标题}}</target></trans-unit>
  </body></file></xliff>`;
  const record = {locale: 'zh-Hans', name: 'Chinese (Simplified)', messages: {} as Record<string, {text: string; source: string; state: string}>};
  assert.equal(applyXliff('zh-Hans', file, record).imported, 1);
  const status = classify('zh-Hans', record);
  assert.deepEqual([status.current.length, status.invalid.length, status.missing.length], [1, 0, translatableIds().length - 1]);
  assert.match(generatedModule('zh-Hans', record), /'count\.titles': '\{count, plural, other \{# 个标题\}\}'/);
});

test('regional variants read their own words, then their base language, then en-US, and export and import only what they change', async () => {
  await loadCatalogue('fr-CA');
  assert.ok(catalogueReady('fr-CA') && catalogueReady('fr'));
  const fr = catalogue('fr');
  const translated = (Object.keys(fr) as MessageId[]).filter(id => !/\{/.test(fr[id]!));
  const [overridden, inheritedId] = [translated[0]!, translated[1]!];
  provideCatalogue('fr-CA', {[overridden]: 'Mot du Québec'});
  try {
    const quebec = createI18n(region('fr-CA'));
    assert.equal(quebec.t(overridden), 'Mot du Québec');
    assert.equal(quebec.t(inheritedId), fr[inheritedId]);
    const missing = (Object.keys(enUS) as MessageId[]).find(id => fr[id] === undefined)!;
    assert.equal(quebec.t(missing), createI18n().t(missing));
  } finally {
    provideCatalogue('fr-CA', {});
  }
  // A full export, prefilled with the base's words; returning them unchanged stores nothing.
  const xml = exportXliff('fr-CA');
  const units = parseXliff(xml);
  assert.equal(units.length, translatableIds().length);
  assert.equal(units.find(u => u.id === inheritedId)!.target, fr[inheritedId]);
  assert.match(xml, /Inherited from fr\./);
  const record = {locale: 'fr-CA', name: 'French (Canada)', messages: {} as Record<string, {text: string; source: string; state: string}>};
  const changed = xml.replace(/(<trans-unit id="error\.offline"[^]*?<target[^>]*>)[^<]*/, '$1Cet appareil n’est pas branché sur Internet. Vérifiez votre connexion, puis réessayez.');
  const report = applyXliff('fr-CA', changed, record);
  assert.equal(report.rejected.length, 0);
  assert.deepEqual(Object.keys(record.messages), ['error.offline']);
  const status = classify('fr-CA', record);
  assert.deepEqual([status.current.length, status.missing.length], [1, 0]);
  assert.equal(status.inherited.length, translatableIds().length - 1);
});

test('pseudo-locale: every message stays valid ICU with its arguments, accented, bracketed and about a third longer', () => {
  for (const [id, source] of Object.entries(enUS)) {
    const pseudo = pseudoMessage(source);
    assert.doesNotThrow(() => parseMessage(pseudo), id);
    assert.deepEqual(messageArguments(pseudo), messageArguments(source), id);
  }
  const i18n = createI18n(region('en-XA'));
  const text = i18n.t('home.row.recentlyAddedIn', {library: 'Movies'});
  assert.match(text, /^\[Ŕéçéñţļý áððéð îñ Movies .+\]$/);
  assert.ok(text.length >= 'Recently added in Movies'.length * 1.3, text);
  assert.equal(i18n.t('count.titles', {count: 1}).startsWith('[1 '), true);
  assert.equal(formatMessage(pseudoMessage("It''s '{literal}'"), {}, {locale: 'en-US'}).startsWith('[Îţ’š {ļîţéŕáļ}') || formatMessage(pseudoMessage("It''s '{literal}'"), {}, {locale: 'en-US'}).startsWith("[Îţ'š {ļîţéŕáļ}"), true);
});

test('language choices: follow the device, then each language named in itself; never the pseudo-locale', () => {
  const choices = localeChoices(createI18n(region('de')));
  assert.equal(choices[0]!.id, 'auto');
  const named = Object.fromEntries(choices.map(c => [c.id, c.label]));
  assert.equal(named.fr, 'Français (France)');
  assert.equal(named['fr-CA'], 'Français (Canada)');
  assert.equal(named.hi, 'हिन्दी');
  assert.equal(named.ru, 'Русский');
  assert.equal(named['en-XA'], undefined);
  // Someone who set it by hand still sees what they chose.
  assert.ok(localeChoices(createI18n(), 'en-XA').some(c => c.id === 'en-XA'));
});

test('metadata sources read in the viewer’s language by id and kind; an unknown source keeps the server’s words', async () => {
  const {metadataAgentWords} = await import('../src/presentation/metadata-agent.ts');
  await loadCatalogue('de');
  const de = createI18n(region('de'));
  const online = {id: 'online', name: 'Portico online metadata', description: 'Matches films with TMDB…'};
  assert.equal(metadataAgentWords(de, online, 'movie').name, catalogue('de')['web.metadataSource.onlineName'] ?? enUS['web.metadataSource.onlineName']);
  assert.equal(metadataAgentWords(de, online, 'music').description, catalogue('de')['web.metadataSource.onlineDescription.music']);
  assert.equal(metadataAgentWords(de, online, 'podcast').description, 'Matches films with TMDB…');
  assert.deepEqual(metadataAgentWords(de, {id: 'future', name: 'Future source', description: 'New.'}, 'movie'), {name: 'Future source', description: 'New.'});
});

test('the shared duration, size and bit-rate helpers follow the published catalogue; English keeps its forms', async () => {
  const {formatBytes, formatDuration, formatBitRate, setServiceI18n} = await import('../src/presentation/index.ts');
  await loadCatalogue('ru');
  try {
    assert.equal(formatDuration(6120), '1h 42m');
    assert.equal(formatBytes(3_145_728), '3.0 MB');
    assert.equal(formatBitRate(283_000), '283 kbps');
    setServiceI18n(createI18n(region('ru')));
    assert.equal(formatDuration(120), createI18n(region('ru')).t('duration.minutes', {minutes: 2}));
    assert.notEqual(formatDuration(120), '2m');
    assert.match(formatBytes(3_670_016).replace(/\s/g, ' '), /^3,5 МБ$/);
    assert.match(formatBitRate(283_000).replace(/\s/g, ' '), /^283 кбит\/[сc]$/);
  } finally {
    setServiceI18n(createI18n());
  }
});

test('storage categories and day presets read in the viewer’s language by id; an unknown id keeps the server’s words', async () => {
  const {storageCategoryWords, maintenanceCadenceName} = await import('../src/presentation/maintenance-words.ts');
  await loadCatalogue('fr');
  const fr = createI18n(region('fr'));
  assert.deepEqual(storageCategoryWords(fr, {id: 'logs', name: 'Logs', description: 'Diagnostic records written to disk.'}), {name: catalogue('fr')['server.storage.category.logs'], description: catalogue('fr')['server.storage.category.logs.description']});
  assert.deepEqual(storageCategoryWords(fr, {id: 'future', name: 'Future files', description: 'New.'}), {name: 'Future files', description: 'New.'});
  assert.equal(maintenanceCadenceName(fr, {id: 'weekly', name: 'Once a week'}), catalogue('fr')['server.maintenance.cadence.weekly']);
  assert.equal(maintenanceCadenceName(fr, {id: 'fortnightly', name: 'Every other week'}), 'Every other week');
});
