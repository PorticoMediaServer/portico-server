import {useEffect, useMemo, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {artworkLimitBytes as limitBytes, artworkLimitText as limitText, metadataSettingsService, type MetadataSettings, type MetadataSettingsChange} from '@core/metadata-settings.ts';
import {problem, useRead} from '../../admin/console';
import {isStaleAnswer} from '@core/freshness-actions.ts';
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
  const {api} = useSession();
  const navigate = useNavigate();
  const service = useMemo(() => metadataSettingsService(<T,>(path: string, method?: string, body?: unknown) => api.request<T>(path, method, body)), [api]);
  const read = useRead(() => service.read(), [service]);
  const doc = read.data;
  const [limit, setLimit] = useState('');
  const [adult, setAdult] = useState(false);
  const [tmdb, setTmdb] = useState<KeyEdit>(untouched);
  const [tvdb, setTvdb] = useState<KeyEdit>(untouched);
  // OpenSubtitles: a key like the others, and an account (a name shown back, a password never).
  const [subtitleKey, setSubtitleKey] = useState<KeyEdit>(untouched);
  const [subtitleUser, setSubtitleUser] = useState('');
  const [subtitlePassword, setSubtitlePassword] = useState('');
  const reset = (d: MetadataSettings) => { setLimit(limitText(d.localArtworkLimitBytes)); setAdult(d.includeAdult); setTmdb(untouched); setTvdb(untouched); setSubtitleKey(untouched); setSubtitleUser(d.openSubtitlesUsername); setSubtitlePassword(''); };
  useEffect(() => { if (doc) reset(doc); }, [doc]);
  const bytes = limitBytes(limit);
  const userChange = doc && subtitleUser.trim() !== doc.openSubtitlesUsername ? subtitleUser.trim() : undefined;
  const subtitleChanges = (): Partial<Pick<MetadataSettingsChange, 'openSubtitlesApiKey' | 'openSubtitlesUsername' | 'openSubtitlesPassword'>> => {
    const key = keyChange(subtitleKey);
    return {...(key !== undefined ? {openSubtitlesApiKey: key} : {}), ...(userChange !== undefined ? {openSubtitlesUsername: userChange} : {}), ...(subtitlePassword ? {openSubtitlesPassword: subtitlePassword} : {})};
  };
  const dirty = !!doc && (bytes !== doc.localArtworkLimitBytes || adult !== doc.includeAdult || keyChange(tmdb) !== undefined || keyChange(tvdb) !== undefined || Object.keys(subtitleChanges()).length > 0);
  const inline = useInlineForm('metadata-settings', {
    dirty,
    problem: bytes === undefined ? 'server.metadata.limitProblem' : undefined,
    discard: () => { if (doc) reset(doc); },
    save: async () => {
      if (!doc || bytes === undefined) return;
      const tmdbKey = keyChange(tmdb), tvdbKey = keyChange(tvdb);
      const keys = {...(tmdbKey !== undefined ? {tmdbReadToken: tmdbKey} : {}), ...(tvdbKey !== undefined ? {tvdbProjectKey: tvdbKey} : {}), ...subtitleChanges()};
      try { await service.save({expectedRevision: doc.revision, localArtworkLimitBytes: bytes, includeAdult: adult, ...keys}); }
      catch (e) {
        if (!isStaleAnswer(e)) throw e;
        // These settings moved on the server: what was not changed here takes the server's value, and it is saved again, unless the same setting was changed there too.
        const fresh = await service.read().catch(() => { throw e; });
        const limitMine = bytes !== doc.localArtworkLimitBytes, adultMine = adult !== doc.includeAdult;
        if ((limitMine && fresh.localArtworkLimitBytes !== doc.localArtworkLimitBytes && fresh.localArtworkLimitBytes !== bytes) || (adultMine && fresh.includeAdult !== doc.includeAdult && fresh.includeAdult !== adult)) { read.reload(); throw e; }
        await service.save({expectedRevision: fresh.revision, localArtworkLimitBytes: limitMine ? bytes : fresh.localArtworkLimitBytes, includeAdult: adultMine ? adult : fresh.includeAdult, ...keys});
      }
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
      <SettingsGroup title={t('server.metadata.subtitles')} description={t('server.metadata.subtitlesLede')}>
        <SettingsRow label={t('server.metadata.openSubtitlesKey')} stack
          state={<Status tone={subtitleKey.remove ? 'warning' : doc.openSubtitlesKeyConfigured ? 'healthy' : 'neutral'}>{subtitleKey.remove ? t('server.metadata.keyRemoving') : doc.openSubtitlesKeyConfigured ? t('server.metadata.openSubtitlesKeyOwn') : t('server.metadata.openSubtitlesKeyServer')}</Status>}
          control={<div style={{display: 'flex', gap: 8, alignItems: 'flex-start', flexWrap: 'wrap', width: '100%'}}>
            <div style={{flex: 1, minWidth: 220, maxWidth: 520}}><PasswordInput hideLabel label={t('server.metadata.openSubtitlesKey')} autoComplete="off" spellCheck={false} maxLength={2048} disabled={subtitleKey.remove} placeholder={doc.openSubtitlesKeyConfigured ? t('server.metadata.keyReplace') : t('server.metadata.keyPaste')} value={subtitleKey.typed} onChange={e => setSubtitleKey({typed: e.target.value, remove: false})} /></div>
            {doc.openSubtitlesKeyConfigured ? <Button variant="ghost" label={subtitleKey.remove ? t('server.metadata.keyKeep') : t('server.metadata.openSubtitlesUseServer')} onClick={() => setSubtitleKey({typed: '', remove: !subtitleKey.remove})} /> : null}
          </div>} />
        <SettingsRow label={t('server.metadata.openSubtitlesUser')}
          control={<div style={{display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap'}}>
            <Input hideLabel label={t('server.metadata.openSubtitlesUser')} autoComplete="off" spellCheck={false} maxLength={512} value={subtitleUser} onChange={e => setSubtitleUser(e.target.value)} />
            {doc.openSubtitlesUsername ? <Button variant="ghost" label={t('server.metadata.openSubtitlesSignOut')} onClick={() => { setSubtitleUser(''); setSubtitlePassword(''); }} /> : null}
          </div>} />
        {subtitleUser.trim() ? (
          <SettingsRow label={t('server.metadata.openSubtitlesPassword')} state={doc.openSubtitlesPasswordConfigured && !subtitlePassword && userChange === undefined ? t('server.metadata.openSubtitlesPasswordSaved') : undefined}
            control={<PasswordInput hideLabel label={t('server.metadata.openSubtitlesPassword')} autoComplete="new-password" maxLength={512} value={subtitlePassword} onChange={e => setSubtitlePassword(e.target.value)} />} />
        ) : null}
      </SettingsGroup>
      <SettingsGroup title={t('server.metadata.artwork')}>
        <SettingsRow label={t('metadata.settings.artworkLimit')} help={t('server.metadata.artworkHelp')} state={t('server.metadata.artworkUsed', {used: i18n.bytes(doc.localArtworkUsedBytes), limit: i18n.bytes(doc.localArtworkLimitBytes)})}
          control={<span className={page.numberField}><Input hideLabel label={t('metadata.settings.artworkLimit')} inputMode="decimal" value={limit} onChange={e => setLimit(e.target.value)} error={bytes === undefined ? t('server.metadata.limitProblem') : undefined} /><span className={page.unit}>{t('settings.unit.gb')}</span></span>} />
      </SettingsGroup>
    </>
  );
}
