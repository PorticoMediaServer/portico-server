import { AlertTriangle, ListVideo, Radio, RefreshCw, Tv, TvMinimalPlay, Video } from '#portico-icons';
import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { SecondaryButton } from '../../components/controls/Buttons';
import { productText, reviewedProductErrorText } from '../../components/ProductLanguage';
import { useAuthSession, useLiveTVSources } from '../../data/DataProvider';
import { ChannelsWorkspace } from './LiveChannels';
import { GuideWorkspace } from './LiveGuide';
import { DVRWorkspace } from './DVRWorkspace';
import { LibraryChannelsWorkspace } from './LibraryChannelsWorkspace';
import { productState, timeLabel } from './liveFormat';
import './live-tv.css';

type LiveTab = 'guide' | 'channels' | 'dvr' | 'library-channels';

function errorKind(error: Error) {
  const status = (error as Error & { status?: number }).status;
  return status === 401 || status === 403 ? 'permission' as const : 'error' as const;
}

export function StateSurface({
  kind,
  title,
  message,
  onRetry,
}: {
  kind: 'loading' | 'empty' | 'error' | 'permission';
  title?: string;
  message: string;
  onRetry?: () => void;
}) {
  const Icon = kind === 'loading' ? RefreshCw : kind === 'error' || kind === 'permission' ? AlertTriangle : Radio;
  const heading = title ?? (kind === 'permission' ? 'Live TV access is restricted' : kind === 'error' ? 'Live TV is unavailable' : kind === 'empty' ? 'Nothing to show yet' : 'Loading Live TV');
  return <div className={`live-state ${kind}`} role={kind === 'error' || kind === 'permission' ? 'alert' : 'status'} aria-busy={kind === 'loading'}>
    <Icon className={kind === 'loading' ? 'state-spinner' : ''} />
    <strong>{heading}</strong>
    <p>{message}</p>
    {onRetry && <SecondaryButton onClick={onRetry}><RefreshCw /> Try again</SecondaryButton>}
  </div>;
}

function tabUrl(tab: LiveTab, sourceId: string) {
  const parameters = new URLSearchParams({ tab });
  if (sourceId) parameters.set('source', sourceId);
  return `?${parameters.toString()}`;
}

export function LiveTVPage() {
  const auth = useAuthSession();
  const permissions = auth.viewer?.user?.permissions ?? {};
  const canUseDVR = Boolean(permissions.viewDVR || permissions.scheduleDVR || permissions.deleteDVRRecordings || permissions.manageDVR || permissions.manageServer);
  const [parameters, setParameters] = useSearchParams();
  const requestedTab = parameters.get('tab');
  const tab: LiveTab = requestedTab === 'channels' || requestedTab === 'library-channels' || (requestedTab === 'dvr' && canUseDVR) ? requestedTab : 'guide';
  const [sourceRevision, setSourceRevision] = useState(0);
  const sources = useLiveTVSources(sourceRevision);
  const requestedSourceId = parameters.get('source') ?? '';
  const requestedChannelId = parameters.get('channel') ?? '';
  const requestedChannelQuery = parameters.get('q') ?? '';
  const sourceId = sources.status === 'success'
    ? sources.data.find((source) => source.id === requestedSourceId)?.id ?? sources.data[0]?.id ?? ''
    : requestedSourceId;
  const [clock, setClock] = useState(() => new Date());

  useEffect(() => {
    const timer = window.setInterval(() => setClock(new Date()), 30_000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    if (tab === 'library-channels' || sources.status !== 'success' || sources.data.length === 0) return;
    if (requestedSourceId !== sourceId || !requestedTab) {
      const next = new URLSearchParams(parameters);
      next.set('source', sourceId);
      next.set('tab', tab);
      setParameters(next, { replace: true });
    }
  }, [parameters, requestedSourceId, requestedTab, setParameters, sourceId, sources, tab]);

  const selectSource = (nextSourceId: string) => {
    const next = new URLSearchParams(parameters);
    next.set('source', nextSourceId);
    next.set('tab', tab);
    next.delete('section');
    setParameters(next);
  };

  return <div className="standard-page live-page-next">
    <header className="page-header live-title-row"><div><p className="route-context">Live television</p><h1>Live TV</h1><p>Guide, channels, recordings, and rules.</p></div><time>{timeLabel(clock)}</time></header>
    <nav className="page-tabs live-tabs" aria-label="Live TV views">
      <Link className={tab === 'guide' ? 'active' : ''} aria-current={tab === 'guide' ? 'page' : undefined} to={tabUrl('guide', sourceId)}><Tv /> {productText('live-tv.tab.guide')}</Link>
      <Link className={tab === 'channels' ? 'active' : ''} aria-current={tab === 'channels' ? 'page' : undefined} to={tabUrl('channels', sourceId)}><ListVideo /> {productText('live-tv.tab.channels')}</Link>
      {canUseDVR && <Link className={tab === 'dvr' ? 'active' : ''} aria-current={tab === 'dvr' ? 'page' : undefined} to={tabUrl('dvr', sourceId)}><Video /> {productText('live-tv.tab.dvr')}</Link>}
      <Link className={tab === 'library-channels' ? 'active' : ''} aria-current={tab === 'library-channels' ? 'page' : undefined} to={tabUrl('library-channels', '')}><TvMinimalPlay /> {productText('live-tv.tab.library-channels')}</Link>
    </nav>
    {tab === 'library-channels' && <LibraryChannelsWorkspace StateSurface={StateSurface} />}
    {tab !== 'library-channels' && sources.status === 'loading' && <StateSurface kind="loading" {...productState('live-tv.loading')} />}
    {tab !== 'library-channels' && sources.status === 'error' && (() => {
      const kind = errorKind(sources.error);
      const state = productState(kind === 'permission' ? 'live-tv.restricted' : 'live-tv.offline');
      return <StateSurface kind={kind} title={state.title} message={reviewedProductErrorText(sources.error, kind === 'permission' ? 'live-tv.restricted' : 'live-tv.offline')} onRetry={() => setSourceRevision((current) => current + 1)} />;
    })()}
    {tab !== 'library-channels' && sources.status === 'success' && sources.data.length === 0 && <StateSurface kind="empty" {...productState('live-tv.empty')} />}
    {tab !== 'library-channels' && sources.status === 'success' && sources.data.length > 0 && sourceId && <>
      {tab === 'guide' && <GuideWorkspace sources={sources.data} sourceId={sourceId} setSourceId={selectSource} StateSurface={StateSurface} />}
      {tab === 'channels' && <ChannelsWorkspace sources={sources.data} sourceId={sourceId} requestedChannelId={requestedChannelId} initialQuery={requestedChannelQuery} setSourceId={selectSource} StateSurface={StateSurface} />}
      {tab === 'dvr' && <DVRWorkspace sources={sources.data} sourceId={sourceId} StateSurface={StateSurface} />}
    </>}
  </div>;
}
