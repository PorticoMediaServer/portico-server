import {useEffect, useMemo, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {artworkLimitBytes as limitBytes, artworkLimitText as limitText, metadataSettingsService, type MetadataSettings} from '@core/metadata-settings.ts';
import {parseEnvelope, parseStorageReport} from '@core/administration.ts';
import {problem, useRead} from '../../admin/console';
import {useInlineForm} from '../settings/ServerForms';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, Input, Loading, Notice, PasswordInput, SettingsGroup, SettingsRow, Status, Switch} from '../../ui';
import page from '../settings/Settings.module.css';

/** A provider key as the owner edits it: left alone, replaced by what was typed, or removed so Portico's own is used again. */
type KeyEdit = Readonly<{typed: string; remove: boolean}>;
const untouched: KeyEdit = {typed: '', remove: false};
const keyChange = (edit: KeyEdit): string | undefined => (edit.remove ? '' : edit.typed.trim() ? edit.typed.trim() : undefined);

/**
 * Metadata: what every library shares when it looks titles up. The owner's own provider keys
 * (never shown back; the page says only whether one is set), whether adult titles are searched,
 * and how much local artwork is kept. Which provider a library uses is that library's choice.
 * One document, saved with the page's Save.
 */
export function MetadataSettingsPanel() {
  const i18n = useI18n();
  const {t} = i18n;
  const {api, system, session} = useSession();
  const navigate = useNavigate();
  const serverId = system?.id ?? session?.viewer.serverId ?? '';
  const service = useMemo(() => metadataSettingsService(<T,>(path: string, method?: string, body?: unknown) => api.request<T>(path, method, body)), [api]);
  const read = useRead(() => service.read(), [service]);
  // What artwork takes on disk now, from the same measurement Storage & backups shows. It counts
  // fetched artwork too, which the limit does not, and the row says so.
  const usage = useRead(async () => parseEnvelope(await api.request<unknown>('/v1/admin/storage-usage'), serverId, parseStorageReport).result.categories.find(c => c.id === 'artwork')?.bytes, [api, serverId]);
  const doc = read.data;
  const [limit, setLimit] = useState('');
  const [adult, setAdult] = useState(false);
  const [tmdb, setTmdb] = useState<KeyEdit>(untouched);
  const [tvdb, setTvdb] = useState<KeyEdit>(untouched);
  const reset = (d: MetadataSettings) => { setLimit(limitText(d.localArtworkLimitBytes)); setAdult(d.includeAdult); setTmdb(untouched); setTvdb(untouched); };
  useEffect(() => { if (doc) reset(doc); }, [doc]);
  const bytes = limitBytes(limit);
  const dirty = !!doc && (bytes !== doc.localArtworkLimitBytes || adult !== doc.includeAdult || keyChange(tmdb) !== undefined || keyChange(tvdb) !== undefined);
  const inline = useInlineForm('metadata-settings', {
    dirty,
    problem: bytes === undefined ? 'server.metadata.limitProblem' : undefined,
    discard: () => { if (doc) reset(doc); },
    save: async () => {
      if (!doc || bytes === undefined) return;
      const tmdbKey = keyChange(tmdb), tvdbKey = keyChange(tvdb);
      await service.save({expectedRevision: doc.revision, localArtworkLimitBytes: bytes, includeAdult: adult, ...(tmdbKey !== undefined ? {tmdbReadToken: tmdbKey} : {}), ...(tvdbKey !== undefined ? {tvdbProjectKey: tvdbKey} : {})});
      read.reload();
    },
  });
  if (!doc) return read.error ? <Notice tone="error" action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice> : <Loading label={t('settings.server.metadata')} />;
  const key = (label: string, set: boolean, edit: KeyEdit, change: (next: KeyEdit) => void) => (
    <SettingsRow label={label} help={t('server.metadata.keyHelp')} stack
      state={<Status tone={edit.remove ? 'warning' : set ? 'healthy' : 'neutral'}>{edit.remove ? t('server.metadata.keyRemoving') : set ? t('server.metadata.keySet') : t('server.metadata.keyOurs')}</Status>}
      control={<div style={{display: 'flex', gap: 8, alignItems: 'flex-start', flexWrap: 'wrap', width: '100%'}}>
        <div style={{flex: 1, minWidth: 220, maxWidth: 520}}><PasswordInput hideLabel label={label} autoComplete="off" spellCheck={false} maxLength={2048} disabled={edit.remove} placeholder={set ? t('server.metadata.keyReplace') : t('server.metadata.keyPaste')} value={edit.typed} onChange={e => change({typed: e.target.value, remove: false})} /></div>
        {set ? <Button variant="ghost" label={edit.remove ? t('server.metadata.keyKeep') : t('server.metadata.keyRemove')} onClick={() => change({typed: '', remove: !edit.remove})} /> : null}
      </div>} />
  );
  return (
    <>
      {read.error ? <Notice tone="warning" compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice> : null}
      {inline.error ? <Notice tone="error" compact>{problem(inline.error, 'action')}</Notice> : null}
      <SettingsGroup title={t('server.metadata.lookups')} description={t('server.metadata.lookupsLede')}>
        <SettingsRow label={t('server.metadata.perLibrary')} help={t('server.metadata.perLibraryHelp')} control={<Button size="sm" variant="secondary" label={t('settings.server.libraries')} onClick={() => void navigate({to: '/settings/$section', params: {section: 'server-libraries'}, search: {}})} />} />
        {key(t('metadata.settings.tmdbToken'), doc.tmdbCredentialConfigured, tmdb, setTmdb)}
        {key(t('metadata.settings.tvdbKey'), doc.tvdbCredentialConfigured, tvdb, setTvdb)}
        <SettingsRow label={t('metadata.settings.includeAdult')} help={t('server.metadata.adultHelp')} control={<Switch checked={adult} onCheckedChange={setAdult} label={t('metadata.settings.includeAdult')} />} />
      </SettingsGroup>
      <SettingsGroup title={t('server.metadata.artwork')}>
        <SettingsRow label={t('metadata.settings.artworkLimit')} help={t('server.metadata.artworkHelp')} state={usage.data !== undefined ? t('server.metadata.artworkUsage', {size: i18n.bytes(usage.data)}) : undefined}
          control={<span className={page.numberField}><Input hideLabel label={t('metadata.settings.artworkLimit')} inputMode="decimal" value={limit} onChange={e => setLimit(e.target.value)} error={bytes === undefined ? t('server.metadata.limitProblem') : undefined} /><span className={page.unit}>{t('settings.unit.gb')}</span></span>} />
      </SettingsGroup>
    </>
  );
}
