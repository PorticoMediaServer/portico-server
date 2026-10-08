import React, {createContext, useCallback, useContext, useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {DownloadsService, downloadBytes, downloadReasonMessage, type DownloadOptionsView, type DownloadTarget, type DownloadsSnapshot, type DownloadRequestKind, type DownloadEpisodes} from '@core/index.ts';
import {sessionIdentity, useSession} from './session';
import {Button, Dialog, ListRow, Loading, Notice, Surface, Text} from '../ui';
import {errorText} from './errors';
import {useI18n} from './i18n';

type ContainerTarget = {containerId: string; containerKind: DownloadRequestKind};
type Request = {target: DownloadTarget & Partial<ContainerTarget>; title: string; optionsFor?: string};
type Value = {service: DownloadsService | null; ask: (request: Request) => void};
const Ctx = createContext<Value>({service: null, ask: () => {}});

/** One downloads service per signed-in session, and one "what quality?" dialog for the whole
 * app, living in the shell so it outlives the menu that opened it. */
export function DownloadsProvider({children}: {children: React.ReactNode}) {
  const {api, session} = useSession();
  const key = sessionIdentity(session);
  const service = useMemo(() => (key ? new DownloadsService(api) : null), [api, key]);
  // One early read tells the whole app whether this server offers downloads at all (rail, Settings, menus).
  useEffect(() => { void service?.refresh(); return () => service?.dispose(); }, [service]);
  const [request, setRequest] = useState<Request | null>(null);
  const ask = useCallback((r: Request) => setRequest(r), []);
  const value = useMemo(() => ({service, ask}), [service, ask]);
  return <Ctx.Provider value={value}>{children}{request && service ? <DownloadDialog request={request} service={service} onClose={() => setRequest(null)} /> : null}</Ctx.Provider>;
}

const idle: DownloadsSnapshot = Object.freeze<DownloadsSnapshot>({phase: 'idle', items: [], busy: [], nextCursor: '', loadingMore: false, requests: []});
const noSubscribe = () => () => {};
export function useDownloads() {
  const {service, ask} = useContext(Ctx);
  const state = useSyncExternalStore(service?.subscribe ?? noSubscribe, service?.getSnapshot ?? (() => idle));
  return {service, state, ask};
}
export const useDownloadRequest = () => useContext(Ctx).ask;

const policies: readonly DownloadEpisodes[] = ['all', 'unwatched', 'next'];

function DownloadDialog({request, service, onClose}: {request: Request; service: DownloadsService; onClose: () => void}) {
  const i18n = useI18n();
  const {session} = useSession();
  const [view, setView] = useState<DownloadOptionsView>();
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const [done, setDone] = useState('');
  // Whole-container downloads (show, season, album, book, playlist)
  // go through POST /v1/downloads/requests with a policy, not the
  // per-preparation endpoint (which now refuses containers over 100).
  const container = request.target.containerId && request.target.containerKind
    ? {kind: request.target.containerKind, id: request.target.containerId}
    : undefined;
  const [policy, setPolicy] = useState<DownloadEpisodes>('all');
  const [keepNext, setKeepNext] = useState(1);
  const itemId = request.optionsFor ?? request.target.mediaId;
  useEffect(() => {
    if (!itemId) return;
    const controller = new AbortController();
    service.options(itemId, controller.signal).then(setView, e => { if (!controller.signal.aborted) setError(errorText(e, 'downloads', 'load')); });
    return () => controller.abort();
  }, [service, itemId]);
  const choose = async (quality: string) => {
    setBusy(quality); setError('');
    try {
      if (container) {
        const deviceId = session?.deviceId;
        if (!deviceId) { setError(i18n.t('web.downloads.noDevice')); return; }
        const followed = await service.requestContainer({target: {...container, title: request.title}, deviceId, quality, policy: policy === 'next' ? {episodes: policy, keepNext} : {episodes: policy}});
        setDone(followed.totalKnown
          ? i18n.t('downloads.addedMany', {count: followed.total})
          : i18n.t('web.downloads.requestPreparing'));
        return;
      }
      const batch = await service.request(request.target, quality);
      setDone(batch.accepted ? (batch.rejected.length ? i18n.t('downloads.addedMixed', {accepted: batch.accepted, rejected: batch.rejected.length, reason: downloadReasonMessage(batch.rejected[0].code)}) : batch.duplicate ? i18n.t('downloads.already') : i18n.t('downloads.addedMany', {count: batch.accepted})) : downloadReasonMessage(batch.rejected[0]?.code ?? ''));
    } catch (e) { setError(errorText(e, 'downloads', 'save')); } finally { setBusy(''); }
  };
  if (done) return <Dialog open onOpenChange={o => !o && onClose()} title={i18n.t('feature.downloads')} width={420} actions={<><Button variant="ghost" label={i18n.t('action.close')} onClick={onClose} /><Button variant="primary" label={i18n.t('downloads.view')} to="/downloads" onClick={onClose} /></>}><Text as="p" variant="body" tone="secondary">{done}</Text></Dialog>;
  const blocked = view && !view.policy.allowDownloads;
  // Without a single title to ask about (a whole season), offer the ladder's common rungs.
  const fallback = [{quality: 'original', label: i18n.t('downloads.quality.original'), help: i18n.t('downloads.quality.originalHelp')}, {quality: '1080p', label: '1080p', help: i18n.t('downloads.quality.smaller')}, {quality: '720p', label: '720p', help: i18n.t('downloads.quality.smallest')}];
  return (
    <Dialog open onOpenChange={o => !o && onClose()} title={i18n.t('action.download')} description={request.title} width={460} actions={<Button variant="ghost" label={i18n.t('action.cancel')} onClick={onClose} />}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {error ? <Notice tone="error" compact>{error}</Notice> : null}
        {itemId && !view && !error ? <Loading label={i18n.t('downloads.checking')} /> : null}
        {blocked ? <Notice tone="info" compact>{downloadReasonMessage(view.policy.reason) || i18n.t('downloads.offForProfile')}</Notice> : null}
        {container && !session?.deviceId ? <Notice tone="info" compact>{i18n.t('web.downloads.noDevice')}</Notice> : null}
        {container ? (
          <Surface padless>
            {policies.map(p => <ListRow key={p} title={i18n.t(p === 'all' ? 'web.downloads.policyAll' : p === 'unwatched' ? 'web.downloads.policyUnwatched' : 'web.downloads.policyNext')} trailingIcon={policy === p ? 'check' : undefined} onClick={() => setPolicy(p)} />)}
          </Surface>
        ) : null}
        {container && policy === 'next' ? (
          <Surface padless>
            {[1, 3, 5].map(n => <ListRow key={n} title={i18n.t('web.downloads.keepNext', {count: n})} trailingIcon={keepNext === n ? 'check' : undefined} onClick={() => setKeepNext(n)} />)}
          </Surface>
        ) : null}
        {view && !blocked ? (
          <Surface padless>
            {view.options.map(o => <ListRow key={o.quality} trailingIcon={o.available && !busy ? 'download' : undefined} title={o.label} subtitle={o.available ? [o.estimated ? i18n.t('downloads.aboutSize', {size: downloadBytes(o.estimatedBytes)}) : downloadBytes(o.estimatedBytes), o.requiresPreparation ? i18n.t('downloads.serverPrepares') : ''].filter(Boolean).join(' · ') : downloadReasonMessage(o.reason)} meta={busy === o.quality ? i18n.t('downloads.adding') : undefined} onClick={o.available && !busy ? () => void choose(o.quality) : undefined} />)}
          </Surface>
        ) : null}
        {!itemId ? <Surface padless>{fallback.map(o => <ListRow key={o.quality} trailingIcon={busy ? undefined : 'download'} title={o.label} subtitle={o.help} meta={busy === o.quality ? i18n.t('downloads.adding') : undefined} onClick={!busy ? () => void choose(o.quality) : undefined} />)}</Surface> : null}
        {view?.storage.remainingBytes != null ? <Text variant="caption" tone="tertiary">{i18n.t('downloads.spaceLeft', {size: downloadBytes(view.storage.remainingBytes)})}</Text> : null}
      </div>
    </Dialog>
  );
}
