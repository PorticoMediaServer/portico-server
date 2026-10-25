import {useEffect, useRef, useState, type ReactNode} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import {artworkLimitBytes, artworkLimitText, metadataSettingsService, openSubtitlesSummary, type ArtworkMaxEdge, type MetadataSettings, type MetadataSettingsChange} from '@core/metadata-settings.ts';
import type {LibraryContentApi} from '@core/library-content.ts';
import {presentError} from '@core/presentation/errors.ts';
import {Button, Select, Switch, TextField} from '../../../kit/controls';
import {SettingsGroup} from '../../../kit/settings';
import {Skeleton} from '../../../kit/states';
import {SettingsRow} from '../../../kit/shell/SettingsSurfaces';

const t = defaultI18n.t;
type SecretDraft = Readonly<{value: string; remove: boolean}>;
const emptySecret = (): SecretDraft => ({value: '', remove: false});
type Draft = {limit: string; adult: boolean; edge: ArtworkMaxEdge; tmdb: SecretDraft; tvdb: SecretDraft; subtitleKey: SecretDraft; subtitleUser: string; subtitlePassword: string};
const draftFor = (document: MetadataSettings): Draft => ({limit: artworkLimitText(document.localArtworkLimitBytes), adult: document.includeAdult, edge: document.artworkMaxEdge,
  tmdb: emptySecret(), tvdb: emptySecret(), subtitleKey: emptySecret(), subtitleUser: document.openSubtitlesUsername, subtitlePassword: ''});
const secretValue = (draft: SecretDraft): string | undefined => draft.remove ? '' : draft.value.trim() || undefined;
const terminal = (error: unknown): boolean => {
  if (!error || typeof error !== 'object') return false;
  const value = error as {status?: number; statusCode?: number; code?: string};
  return value.status === 401 || value.status === 403 || value.statusCode === 401 || value.statusCode === 403 || ['unauthorized', 'forbidden', 'not_permitted'].includes(value.code ?? '');
};
const retired = () => Object.assign(new Error('metadata_scope_ended'), {name: 'AbortError'});
type Owned = {service: ReturnType<typeof metadataSettingsService>; current: () => boolean};

/** Only fields with real metadata-settings producers are offered. Credentials are local,
 * write-only drafts; neither a failed read nor a revision conflict silently resends them. */
export function MetadataSettingsPage({api, isCurrent, onLibraries}: {api: LibraryContentApi; isCurrent: () => boolean; onLibraries: () => void}) {
  const active = useRef<{api: LibraryContentApi; isCurrent: () => boolean}>({api, isCurrent}); active.current = {api, isCurrent};
  const [owned, setOwned] = useState<Owned>(), [document, setDocument] = useState<MetadataSettings>(), [draft, setDraft] = useState<Draft>();
  const [busy, setBusy] = useState(false), [error, setError] = useState<string>(), [denied, setDenied] = useState(false), [reload, setReload] = useState(0);
  const flight = useRef(false);
  useEffect(() => {
    const lifetime = new AbortController(); let live = true;
    const current = () => live && !lifetime.signal.aborted && active.current.api === api && active.current.isCurrent === isCurrent && isCurrent();
    const clear = () => {live = false; lifetime.abort(); setDenied(true); setDocument(undefined); setDraft(undefined);};
    const service = metadataSettingsService(async <T,>(path: string, method?: string, body?: unknown): Promise<T> => {
      if (!current()) throw retired();
      try {
        const result = await api.request<T>(path, method, body, lifetime.signal);
        if (!current()) throw retired();
        return result;
      } catch (problem) {if (terminal(problem) && current()) clear(); throw problem;}
    });
    const scope = {service, current}; setOwned(scope); setDenied(false); setDocument(undefined); setDraft(undefined); setError(undefined); setBusy(true); flight.current = true;
    void service.read().then(next => {if (current()) {setDocument(next); setDraft(draftFor(next));}}, problem => {
      if (current()) setError(presentError(problem, 'server-console').body);
    }).finally(() => {if (current()) {flight.current = false; setBusy(false);}});
    return () => {live = false; lifetime.abort(); flight.current = false;};
  }, [api, isCurrent, reload]);
  const current = () => !denied && !!owned?.current();
  if (!isCurrent() || denied) return <p role="alert">{t('next.remoteSources.error.forbidden')}</p>;
  const problem = error && <p role="alert">{error}</p>;
  if (!document || !draft || !owned || !owned.current()) return busy ? <div aria-busy="true"><Skeleton height={72}/><Skeleton height={96}/><Skeleton height={96}/></div>
    : <>{problem}<Button size="small" onClick={() => setReload(value => value + 1)}>{t('action.tryAgain')}</Button></>;
  const set = <K extends keyof Draft>(key: K, value: Draft[K]) => {if (!flight.current && current()) {setDraft({...draft, [key]: value}); setError(undefined);}};
  const bytes = artworkLimitBytes(draft.limit);
  const subtitleChanges = {
    ...(secretValue(draft.subtitleKey) !== undefined ? {openSubtitlesApiKey: secretValue(draft.subtitleKey)} : {}),
    ...(draft.subtitleUser !== document.openSubtitlesUsername ? {openSubtitlesUsername: draft.subtitleUser.trim()} : {}),
    ...(draft.subtitlePassword ? {openSubtitlesPassword: draft.subtitlePassword} : {}),
  };
  const dirty = bytes !== document.localArtworkLimitBytes || draft.adult !== document.includeAdult || draft.edge !== document.artworkMaxEdge || secretValue(draft.tmdb) !== undefined || secretValue(draft.tvdb) !== undefined || Object.keys(subtitleChanges).length > 0;
  const save = async () => {
    if (flight.current || !current() || bytes === undefined || !dirty) return;
    flight.current = true; setBusy(true); setError(undefined);
    const change: MetadataSettingsChange = {expectedRevision: document.revision, localArtworkLimitBytes: bytes, includeAdult: draft.adult, artworkMaxEdge: draft.edge,
      ...(secretValue(draft.tmdb) !== undefined ? {tmdbReadToken: secretValue(draft.tmdb)} : {}), ...(secretValue(draft.tvdb) !== undefined ? {tvdbProjectKey: secretValue(draft.tvdb)} : {}), ...subtitleChanges};
    // The explicit request owns these values from here. No retry draft retains a secret.
    setDraft({...draft, tmdb: emptySecret(), tvdb: emptySecret(), subtitleKey: emptySecret(), subtitlePassword: ''});
    try {const next = await owned.service.save(change); if (current()) {setDocument(next); setDraft(draftFor(next));}}
    catch (reason) {if (current()) setError(presentError(reason, 'server-console').body);}
    finally {if (current()) {flight.current = false; setBusy(false);}}
  };
  const check = async () => {
    if (flight.current || !current() || Object.keys(subtitleChanges).length) return;
    flight.current = true; setBusy(true); setError(undefined);
    try {const next = await owned.service.checkOpenSubtitles(); if (current()) setDocument({...document, openSubtitles: next});}
    catch (reason) {if (current()) setError(presentError(reason, 'server-console').body);}
    finally {if (current()) {flight.current = false; setBusy(false);}}
  };
  const secret = (key: 'tmdb' | 'tvdb' | 'subtitleKey', label: MessageId, configured: boolean): ReactNode => {
    const value = draft[key];
    return <SettingsRow label={t(label)} help={key === 'subtitleKey' ? t('next.settingsSpec.server.metadata.groups.4.rows.1.help') : t('server.metadata.keyHelp')} stacked>
      <div><p>{t(value.remove ? 'server.metadata.keyRemoving' : key === 'subtitleKey' ? document.openSubtitles.keySource === 'owner' ? 'server.metadata.openSubtitlesKeyOwn' : document.openSubtitles.keySource === 'environment' ? 'server.metadata.openSubtitlesKeyEnvironment' : 'server.metadata.openSubtitlesKeyNone' : configured ? 'server.metadata.keySet' : 'server.metadata.keyOurs')}</p>
        <TextField type="password" aria-label={t(label)} value={value.value} autoComplete="off" spellCheck={false} maxLength={2048} disabled={busy || value.remove}
          placeholder={t(configured ? 'server.metadata.keyReplace' : 'server.metadata.keyPaste')} onChange={event => set(key, {value: event.currentTarget.value, remove: false})}/>
        {configured && <Button size="small" variant="text" disabled={busy} onClick={() => set(key, {value: '', remove: !value.remove})}>{t(value.remove ? 'server.metadata.keyKeep' : key === 'subtitleKey' ? 'server.metadata.openSubtitlesRemoveKey' : 'server.metadata.keyRemove')}</Button>}
      </div>
    </SettingsRow>;
  };
  const summary = openSubtitlesSummary(document.openSubtitles, defaultI18n);
  return <div data-testid="server-metadata-settings">
    {problem}
    <SettingsGroup title={t('server.metadata.lookups')}>
      <SettingsRow label={t('server.metadata.perLibrary')} help={t('server.metadata.perLibraryHelp')}><Button size="small" onClick={() => {if (current()) onLibraries();}}>{t('settings.server.libraries')}</Button></SettingsRow>
      <SettingsRow label={t('metadata.settings.includeAdult')} help={t('server.metadata.adultHelp')}><Switch aria-label={t('metadata.settings.includeAdult')} checked={draft.adult} disabled={busy} onCheckedChange={value => set('adult', value)}/></SettingsRow>
    </SettingsGroup>
    <SettingsGroup title={t('next.settingsSpec.server.metadata.groups.2.title')}>
      {secret('tmdb', 'metadata.settings.tmdbToken', document.tmdbCredentialConfigured)}
      {secret('tvdb', 'metadata.settings.tvdbKey', document.tvdbCredentialConfigured)}
    </SettingsGroup>
    <SettingsGroup title={t('server.metadata.artwork')}>
      <SettingsRow label={t('metadata.settings.artworkLimit')} help={t('server.metadata.artworkUsed', {used: defaultI18n.bytes(document.localArtworkUsedBytes), limit: defaultI18n.bytes(document.localArtworkLimitBytes)})}>
        <TextField aria-label={t('metadata.settings.artworkLimit')} inputMode="decimal" value={draft.limit} disabled={busy} onChange={event => set('limit', event.currentTarget.value)} trailing={t('settings.unit.gb')} error={bytes === undefined ? t('server.metadata.limitProblem') : undefined}/>
      </SettingsRow>
      <SettingsRow label={t('next.settingsSpec.server.metadata.advanced.groups.2.rows.1.label')}><Select aria-label={t('next.settingsSpec.server.metadata.advanced.groups.2.rows.1.label')} value={String(draft.edge)} disabled={busy}
        options={[{value: '1280', label: t('next.settingsSpec.server.metadata.advanced.groups.2.rows.1.control.options.0.label')}, {value: '1920', label: t('next.settingsSpec.server.metadata.advanced.groups.2.rows.1.control.options.1.label')}, {value: '2560', label: t('next.settingsSpec.server.metadata.advanced.groups.2.rows.1.control.options.2.label')}]} onValueChange={value => {if (value === '1280' || value === '1920' || value === '2560') set('edge', Number(value) as ArtworkMaxEdge);}}/></SettingsRow>
    </SettingsGroup>
    <SettingsGroup title={t('next.settingsSpec.server.metadata.groups.4.title')}>
      <SettingsRow label={t('next.settingsSpec.server.metadata.groups.4.rows.0.label')}><a href="https://www.opensubtitles.com/en/consumers" target="_blank" rel="noopener noreferrer">{t('server.metadata.openSubtitlesGetKey')}</a></SettingsRow>
      {secret('subtitleKey', 'server.metadata.openSubtitlesKey', document.openSubtitlesKeyConfigured)}
      <SettingsRow label={t('server.metadata.openSubtitlesUser')} help={t('server.metadata.openSubtitlesAccountHelp')}><TextField aria-label={t('server.metadata.openSubtitlesUser')} autoComplete="off" spellCheck={false} maxLength={512} value={draft.subtitleUser} disabled={busy} onChange={event => {if (!flight.current && current()) {const subtitleUser = event.currentTarget.value; setDraft({...draft, subtitleUser, subtitlePassword: ''}); setError(undefined);}}}/></SettingsRow>
      {!!draft.subtitleUser.trim() && <SettingsRow label={t('server.metadata.openSubtitlesPassword')} help={document.openSubtitlesPasswordConfigured ? t('server.metadata.openSubtitlesPasswordSaved') : undefined}>
        <TextField aria-label={t('server.metadata.openSubtitlesPassword')} type="password" autoComplete="new-password" maxLength={512} value={draft.subtitlePassword} disabled={busy} onChange={event => set('subtitlePassword', event.currentTarget.value)}/>
      </SettingsRow>}
      <SettingsRow label={t('server.metadata.openSubtitlesConnection')} help={Object.keys(subtitleChanges).length ? t('server.metadata.openSubtitlesTestUnsaved') : summary.downloads}>
        <div><p>{summary.text}</p><Button size="small" disabled={busy || !!Object.keys(subtitleChanges).length || document.openSubtitles.keySource === 'none'} onClick={() => {void check();}}>{t(busy ? 'server.metadata.openSubtitlesTesting' : 'server.metadata.openSubtitlesTest')}</Button></div>
      </SettingsRow>
    </SettingsGroup>
    <div><Button size="small" variant="primary" disabled={busy || !dirty || bytes === undefined} onClick={() => {void save();}}>{t('action.save')}</Button>
      <Button size="small" variant="text" disabled={busy} onClick={() => {if (current()) {setDraft(draftFor(document)); setError(undefined);}}}>{t('action.cancel')}</Button>
      <Button size="small" variant="text" disabled={busy} onClick={() => {if (current()) setReload(value => value + 1);}}>{t('action.refresh')}</Button></div>
  </div>;
}
