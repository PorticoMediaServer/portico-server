import React from 'react';
import {useNavigate} from '@tanstack/react-router';
import {addLanguage, availableLanguages, compositePatch, languageName, localeChoices, MAX_LANGUAGES, moveLanguage, preferenceValueLabel, readComposite, removeLanguage, type SettingsChoice, type SettingsRow as StructureRow} from '@core/presentation/index.ts';
import type {PreferencePatch, PreferenceValue} from '@core/preferences.ts';
import {useSession} from '../../app/session';
import {useI18n, type MessageId} from '../../app/i18n';
import {usePreferences} from '../../app/preferences';
import {useServerPreferences} from '../../app/server-preferences';
import {Badge, Button, IconButton, Select, SettingsRow, Switch} from '../../ui';
import {ResetRecommendationsRow} from './ResetRecommendations';
import s from './Settings.module.css';

/** The rows of the Account pages; server settings have their own renderer (`ServerForms.tsx`). */
type Row = Exclude<StructureRow, {kind: 'custom'} | {kind: 'server-setting'}>;

/**
 * One authored settings row with the web's controls. The structure decides what the row is and
 * says; nothing here reads the registry's order, labels or domains.
 */
export function RowView({row, anchor, save}: {row: Row; anchor: string; save: (patch: PreferencePatch) => void}) {
  const {t} = useI18n();
  const i18n = useI18n();
  const server = useServerPreferences();
  const device = usePreferences();
  const session = useSession();
  const navigate = useNavigate();
  const text = t(row.label);
  const label = row.scope === 'device' ? <>{text}<Badge>{t('settings.thisDevice')}</Badge></> : text;
  const help = row.help ? t(row.help) : undefined;
  if (row.kind === 'action') {
    if (row.id === 'resetRecommendations') return <ResetRecommendationsRow />;
    if (row.id === 'linkTV') return <SettingsRow id={anchor} label={label} help={help} onClick={() => void navigate({to: '/device'})} />;
    return <SettingsRow id={anchor} label={label} help={help} control={<Button size="sm" variant={row.destructive ? 'danger' : 'secondary'} icon={row.icon} label={text} onClick={session.signOut} />} />;
  }
  const field = row.kind === 'preference' ? server.snapshot?.registry.fields.find(f => f.key === row.key) : undefined;
  const read = (key: string) => server.snapshot?.registry.fields.some(f => f.key === key) ? server.value<PreferenceValue>(key, '') : undefined;
  let value: PreferenceValue | undefined;
  let write: (next: string | number | boolean | readonly string[]) => void;
  if (row.kind === 'preference') {
    value = field ? server.value(row.key, field.default) : undefined;
    write = next => save({[row.key]: next});
  } else if (row.storage === 'registry') {
    value = readComposite(row.id, read);
    write = next => save(compositePatch(row.id, next as string | boolean));
  } else {
    // The device's own values: this browser keeps them (`portico.preferences.v1`).
    value = row.id === 'libraryView' ? device.preferences.libraryView : undefined;
    write = next => { if (row.id === 'libraryView') device.update({libraryView: next === 'list' ? 'list' : 'grid'}); };
  }
  if (value === undefined) return null;
  const control = row.control;
  switch (control.type) {
    case 'switch': return <SettingsRow id={anchor} label={label} help={help} control={<Switch checked={value === true} onCheckedChange={write} label={text} />} />;
    case 'choice': {
      // A value only an API client could have stored is shown as what it is, never as the nearest choice.
      const known = control.choices.some(c => c.value === value);
      const options = [...(known || row.kind !== 'preference' ? [] : [{value: String(value), label: preferenceValueLabel(row.key, value as string | number)}]), ...control.choices.map(c => ({value: String(c.value), label: choiceLabel(c, t)}))];
      return <SettingsRow id={anchor} label={label} help={help} control={<Select hideLabel label={text} options={options} value={String(value)} onChange={e => { const picked = control.choices.find(c => String(c.value) === e.target.value); if (picked && picked.value !== value) write(picked.value); }} />} />;
    }
    case 'languages': return <SettingsRow id={anchor} stack label={label} help={help} full start control={<LanguageList label={text} value={value as readonly string[]} onChange={write} />} />;
    case 'locale': {
      // X-03: language is a choice, never free text.
      const current = typeof value === 'string' && value ? value : 'auto';
      const options = localeChoices(i18n, current);
      return <SettingsRow id={anchor} label={label} help={help} control={<Select hideLabel label={text} options={options.map(o => ({value: o.id, label: o.label}))} value={current} onChange={e => { if (e.target.value !== current) write(e.target.value); }} />} />;
    }
  }
}

const choiceLabel = (choice: SettingsChoice, t: (id: MessageId, values?: Record<string, string | number>) => string) => t(choice.label, choice.labelValues as Record<string, string | number> | undefined);

/** Preferred languages in order: move, remove, add by name. The first one a title has is used. */
function LanguageList({label, value, onChange}: {label: string; value: readonly string[]; onChange: (next: readonly string[]) => void}) {
  const {t} = useI18n();
  const i18n = useI18n();
  const name = (code: string) => languageName(code, key => (i18n.has(key) ? t(key as MessageId) : undefined), i18n.locale);
  const offered = availableLanguages(value).map(code => ({value: code, label: name(code)})).sort((a, b) => a.label.localeCompare(b.label));
  return (
    <div className={s.languages}>
      {value.length ? (
        <ol className={s.languageList} aria-label={label}>
          {value.map((code, index) => (
            <li key={code} className={s.language}>
              <span className={s.languageOrder}>{index + 1}</span>
              <span className={s.languageName}>{name(code)}</span>
              <IconButton name="chevronUp" size="sm" variant="ghost" label={t('settings.languages.moveUp', {language: name(code)})} disabled={index === 0} onClick={() => onChange(moveLanguage(value, index, 'up'))} />
              <IconButton name="chevronDown" size="sm" variant="ghost" label={t('settings.languages.moveDown', {language: name(code)})} disabled={index === value.length - 1} onClick={() => onChange(moveLanguage(value, index, 'down'))} />
              <IconButton name="close" size="sm" variant="ghost" label={t('settings.languages.remove', {language: name(code)})} onClick={() => onChange(removeLanguage(value, index))} />
            </li>
          ))}
        </ol>
      ) : <p className={s.languageNone}>{t('settings.languages.none')}</p>}
      {value.length < MAX_LANGUAGES && offered.length ? <div className={s.languageAdd}><Select hideLabel label={t('settings.languages.add')} placeholder={t('settings.languages.add')} options={offered} value="" onChange={e => { if (e.target.value) onChange(addLanguage(value, e.target.value)); }} /></div> : null}
    </div>
  );
}
