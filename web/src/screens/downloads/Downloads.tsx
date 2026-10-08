import {useEffect, useState} from 'react';
import {useNavigate} from '@tanstack/react-router';
import {defaultI18n} from '@i18n';
import {downloadBytes, type DownloadAction, type DownloadPreparation} from '@core/index.ts';
import {useDownloads} from '../../app/downloads';
import {useSession} from '../../app/session';
import {ErrorNotice, ErrorState, errorText} from '../../app/errors';
import {Badge, Button, ConfirmDialog, Inset, Loading, Notice, Page, PageHeader, ProgressBar, StateView, Surface, Text} from '../../ui';

const t = defaultI18n.t;
function stateLabel(state: DownloadPreparation['state']): string {
  switch (state) {
    case 'queued': return t('web.downloads.state.waiting');
    case 'running': return t('web.downloads.state.preparing');
    case 'ready': return t('web.downloads.state.ready');
    case 'paused': return t('web.downloads.state.paused');
    case 'failed': return t('web.dvr.state.failed');
    case 'unavailable': return t('web.live.unavailable');
    case 'cancelled': return t('download.state.canceled');
    case 'expired': return t('web.downloads.state.expired');
  }
}
function actionLabel(action: DownloadAction): string {
  switch (action) {
    case 'pause': return t('action.pause');
    case 'resume': return t('web.downloads.resume');
    case 'cancel': return t('web.downloads.stop');
    case 'retry': return t('action.tryAgain');
    case 'remove': return t('action.removeShort');
  }
}
/** WEB-DL-01: the preparation carries no title yet (deferred-to-contract), so the file's name
 * without its extension stands in, and the row links to the title itself. */
const displayName = (p: DownloadPreparation) => (p.artifact.fileName ? p.artifact.fileName.replace(/\.[a-z0-9]{2,5}$/i, '').replace(/[._]+/g, ' ').trim() : '') || t('web.downloads.untitled');

/** Titles the server has prepared for this profile. In a browser a download is a file you
 * save; the apps keep downloads inside Portico and play them without a connection. */
export function DownloadsScreen() {
  const {service, state} = useDownloads();
  const {api, owner} = useSession();
  const navigate = useNavigate();
  const [confirm, setConfirm] = useState<{p: DownloadPreparation; action: 'cancel' | 'remove'} | null>(null);
  const [problem, setProblem] = useState('');
  useEffect(() => { service?.start(); return () => service?.stop(); }, [service]);
  const save = async (p: DownloadPreparation) => {
    if (!service) return;
    setProblem('');
    try {
      const grant = await service.grant(p);
      // The grant in the address is the permission, and it is short-lived: navigate to it now.
      window.location.assign(api.mediaUrl(grant.url));
    } catch (e) { setProblem(errorText(e, 'downloads', 'action')); }
  };
  const act = (p: DownloadPreparation, a: DownloadAction) => {
    // Stopping or removing a prepared copy is confirmed (tier 1); the rest act at once.
    if (a === 'cancel' || a === 'remove') setConfirm({p, action: a});
    else void service?.act(p, a);
  };
  return (
    <Page>
      <PageHeader title={t('web.settings.downloads')} eyebrow={state.usage && state.usage.profileBytes > 0 ? t('web.downloads.usage', {size: downloadBytes(state.usage.profileBytes)}) : undefined} />
      <Inset>
        <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
          {state.error && state.items.length ? <ErrorNotice error={state.error} context="downloads" compact retry={() => void service?.refresh()} /> : null}
          {problem ? <Notice tone="error" compact action={{label: t('action.dismiss'), onClick: () => setProblem('')}}>{problem}</Notice> : null}
          {state.phase === 'loading' ? <Loading label={t('status.loadingThing', {thing: t('web.downloads.thing')})} /> : null}
          {state.phase === 'error' ? <ErrorState error={state.error} context="downloads" retry={() => void service?.refresh()} /> : null}
          {state.unavailable ? <StateView icon="download" title={t('web.downloads.offTitle')} body={owner ? t('web.downloads.offOwner') : t('web.downloads.offMember')} /> : null}
          {state.phase === 'ready' && !state.unavailable && !state.items.length && !state.requests.length ? <StateView icon="download" title={t('web.downloads.emptyTitle')} body={t('web.downloads.emptyBody')} /> : null}
          {state.requests.length ? (
            <Surface padless>
              {state.requests.map((tracked, i) => {
                const r = tracked.request;
                const label = r.totalKnown ? t('web.downloads.requestProgress', {ready: r.ready, total: r.total}) : t('web.downloads.requestPreparing');
                return (
                  <div key={r.requestId} style={{display: 'flex', gap: 16, alignItems: 'center', padding: '16px 16px', borderTop: i ? '1px solid var(--line-soft)' : undefined, flexWrap: 'wrap'}}>
                    <div style={{flex: '1 1 260px', minWidth: 0, display: 'flex', flexDirection: 'column', gap: 4}}>
                      <div style={{display: 'flex', gap: 8, alignItems: 'baseline', flexWrap: 'wrap'}}>
                        <Text variant="bodyStrong" clamp={1}>{tracked.title || t('web.downloads.requestsTitle')}</Text>
                        <Badge tone={r.state === 'complete' ? 'healthy' : r.state === 'failed' ? 'warning' : 'neutral'}>{label}</Badge>
                      </div>
                      {r.failed ? <Text variant="caption" tone="warning">{t('web.downloads.requestFailed', {failed: r.failed})}</Text> : null}
                      {r.state === 'failed' && r.errorCode === 'selection_changed' ? <Text variant="caption" tone="warning">{t('web.downloads.requestChanged')}</Text> : null}
                      {r.state === 'capturing' || r.state === 'admitting' || r.state === 'preparing' ? <ProgressBar value={r.totalKnown && r.total ? r.ready / r.total : undefined} indeterminate={!r.totalKnown} /> : null}
                    </div>
                  </div>
                );
              })}
            </Surface>
          ) : null}
          {state.items.length ? (
            <Surface padless>
              {state.items.map((p, i) => {
                const busy = state.busy.includes(p.id), reason = service?.reason(p);
                return (
                  <div key={p.id} style={{display: 'flex', gap: 16, alignItems: 'center', padding: '16px 16px', borderTop: i ? '1px solid var(--line-soft)' : undefined, flexWrap: 'wrap'}}>
                    <div style={{flex: '1 1 260px', minWidth: 0, display: 'flex', flexDirection: 'column', gap: 4}}>
                      <div style={{display: 'flex', gap: 8, alignItems: 'baseline', flexWrap: 'wrap'}}>
                        <Text variant="bodyStrong" clamp={1}>{displayName(p)}</Text>
                        <Badge tone={p.state === 'ready' ? 'healthy' : p.state === 'failed' || p.state === 'unavailable' || p.state === 'expired' ? 'warning' : 'neutral'}>{stateLabel(p.state)}</Badge>
                      </div>
                      <Text variant="caption" tone="tertiary">{[p.quality === 'original' ? t('web.downloads.original') : p.quality, p.artifact.bytes ? (p.artifact.estimated ? t('web.downloads.about', {size: downloadBytes(p.artifact.bytes)}) : downloadBytes(p.artifact.bytes)) : '', p.state === 'ready' && p.expiresAt ? t('web.downloads.keptUntil', {date: new Date(p.expiresAt).toLocaleDateString(undefined, {month: 'short', day: 'numeric'})}) : '', p.state === 'running' && p.progress.etaSeconds ? t('web.downloads.minutesLeft', {count: Math.max(1, Math.round(p.progress.etaSeconds / 60))}) : ''].filter(Boolean).join(' · ')}</Text>
                      {reason ? <Text variant="caption" tone="warning">{reason}</Text> : null}
                      {p.state === 'running' || p.state === 'paused' ? <ProgressBar value={p.progress.percent / 100} /> : null}
                    </div>
                    <div style={{display: 'flex', gap: 8, flexWrap: 'wrap'}}>
                      {p.state === 'ready' ? <Button size="sm" variant="secondary" icon="download" label={t('web.downloads.save')} onClick={() => void save(p)} /> : null}
                      <Button size="sm" variant="ghost" label={t('web.downloads.goToTitle')} onClick={() => void navigate({to: '/media/$itemId', params: {itemId: p.itemId}, search: {}})} />
                      {p.actions.map(a => <Button key={a} size="sm" variant="ghost" label={actionLabel(a)} disabled={busy} onClick={() => act(p, a)} />)}
                    </div>
                  </div>
                );
              })}
            </Surface>
          ) : null}
          {state.nextCursor ? <div><Button size="sm" variant="secondary" label={t('action.showMore')} loading={state.loadingMore} onClick={() => void service?.loadMore()} /></div> : null}
        </div>
      </Inset>
      <ConfirmDialog open={!!confirm} onOpenChange={o => !o && setConfirm(null)} title={confirm?.action === 'cancel' ? t('confirm.stopPreparing.title') : t('confirm.removePrepared.title')} body={confirm?.action === 'cancel' ? t('confirm.stopPreparing.body') : t('confirm.removePrepared.body')} confirmLabel={confirm?.action === 'cancel' ? t('web.downloads.stop') : t('action.removeShort')} cancelLabel={t('web.dvr.keepIt')} destructive onConfirm={() => { const c = confirm!; setConfirm(null); void service?.act(c.p, c.action); }} />
    </Page>
  );
}
