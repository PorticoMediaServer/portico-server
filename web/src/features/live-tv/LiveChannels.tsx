import type { ActionableLiveTVChannel, ActionableLiveTVSource } from '../../data/models';
import { type ComponentType, useDeferredValue, useEffect, useState } from 'react';
import { NavigationPreviousIcon, NavigationDisclosureIcon, PlaybackPlayIcon, NavigationSearchIcon, ActionRateIcon } from '#portico-icons';
import { useNavigate } from 'react-router-dom';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { productText, reviewedProductErrorText } from '../../components/ProductLanguage';
import { useLiveTVMutations, usePorticoDataSource } from '../../data/DataProvider';
import { usePlaybackSession } from '../player/PlayerSurface';
import { LiveChoiceMenu } from './LiveControls';
import { hasAction, liveActions } from './liveActions';
import { productState } from './liveFormat';
import { useChannelPage } from './liveQueries';

const channelPageSize = 100;

function ChannelLogo({ channel, source, featured = false }: { channel: ActionableLiveTVChannel; source: ReturnType<typeof usePorticoDataSource>; featured?: boolean }) {
  const url = channel.logoUrl ? source.playbackResourceUrl(channel.logoUrl) : '';
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [url]);
  return <span className={`channel-directory-logo ${featured ? 'featured' : ''}`}>{url && !failed ? <img src={url} alt="" onError={() => setFailed(true)} /> : <span>{channel.number || channel.name.slice(0, 2)}</span>}</span>;
}

function ChannelTitle({ name }: { name: string }) {
  const match = name.match(/https:\/\/[^\s)]+/);
  if (!match || !match[0]) return <>{name}</>;
  const [before, after] = name.split(match[0]);
  return <>{before}<a className="live-attribution-link" href={match[0]} target="_blank" rel="noopener noreferrer">{match[0]}</a>{after}</>;
}

export function ChannelsWorkspace({
  sources,
  sourceId,
  requestedChannelId,
  initialQuery = '',
  setSourceId,
  StateSurface,
}: {
  sources: ActionableLiveTVSource[];
  sourceId: string;
  requestedChannelId?: string;
  initialQuery?: string;
  setSourceId: (id: string) => void;
  StateSurface: ComponentType<{ kind: 'loading' | 'empty' | 'error' | 'permission'; title?: string; message: string; onRetry?: () => void }>;
}) {
  const source = usePorticoDataSource();
  const navigate = useNavigate();
  const player = usePlaybackSession();
  const mutations = useLiveTVMutations();
  const [revision, setRevision] = useState(0);
  const [query, setQuery] = useState(initialQuery);
  const deferredQuery = useDeferredValue(query.trim());
  const [favoritesOnly, setFavoritesOnly] = useState(false);
  const [group, setGroup] = useState('all');
  const [cursors, setCursors] = useState<string[]>([]);
  const [selectedId, setSelectedId] = useState('');
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const { query: channels, supportsPaging } = useChannelPage(sourceId, {
    limit: channelPageSize,
    cursor: cursors.at(-1),
    query: deferredQuery || undefined,
    favoritesOnly,
    group: group === 'all' ? undefined : group,
  }, revision);

  useEffect(() => setCursors([]), [deferredQuery, favoritesOnly, group, sourceId]);
  useEffect(() => {
    setGroup('all');
    setSelectedId('');
  }, [sourceId]);
  useEffect(() => {
    if (channels.status !== 'success') return;
    if (channels.data.items.some((channel) => channel.id === selectedId)) return;
    const requested = channels.data.items.find((channel) => channel.id === requestedChannelId)?.id;
    setSelectedId(requested ?? channels.data.items[0]?.id ?? '');
  }, [channels, requestedChannelId, selectedId]);

  const selected = channels.status === 'success' ? channels.data.items.find((channel) => channel.id === selectedId) : undefined;
  const groups = channels.status === 'success' ? channels.data.groups ?? [] : [];
  const watch = async (channel: ActionableLiveTVChannel) => {
    setBusy(`watch:${channel.id}`);
    setError('');
    try {
      const playback = await player.startLive(channel.id);
      if (playback) navigate(`/watch/${playback.media.id}`);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: 'open this channel' }));
    } finally {
      setBusy('');
    }
  };
  const favorite = async (channel: ActionableLiveTVChannel) => {
    setBusy(`favorite:${channel.id}`);
    setError('');
    try {
      await mutations.favoriteChannel(channel.id, !channel.favorite);
      setRevision((current) => current + 1);
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'live-tv.action-failed', { actionName: 'change this favorite' }));
    } finally {
      setBusy('');
    }
  };

  return <div className="live-workspace channels-workspace">
    {selected && <section className="channel-focus-strip" aria-label="Selected channel">
      <ChannelLogo channel={selected} source={source} featured />
      <div><p>{selected.number ? `Channel ${selected.number}` : selected.groupTitle || 'Live channel'}</p><h2><ChannelTitle name={selected.name} /></h2><span>{[selected.groupTitle, selected.country, selected.programCount ? `${selected.programCount} guide entries` : 'No guide entries'].filter(Boolean).join(' · ')}</span></div>
      <div>
        {hasAction(selected, liveActions.play) && <PrimaryButton disabled={busy === `watch:${selected.id}`} onClick={() => void watch(selected)}><PlaybackPlayIcon fill="currentColor" /> {busy === `watch:${selected.id}` ? productText('state.opening', { destination: productText('destination.live-tv') }) : productText('action.watch-live')}</PrimaryButton>}
        {(hasAction(selected, liveActions.favoriteAdd) || hasAction(selected, liveActions.favoriteRemove)) && <SecondaryButton disabled={busy === `favorite:${selected.id}`} selected={selected.favorite} onClick={() => void favorite(selected)}><ActionRateIcon fill={selected.favorite ? 'currentColor' : 'none'} /> {productText(selected.favorite ? 'action.remove-favorite' : 'action.add-favorite')}</SecondaryButton>}
      </div>
    </section>}
    <div className="live-toolbar channels-toolbar">
      <LiveChoiceMenu className="source-choice" label="Source" value={sourceId} choices={sources.map((candidate) => ({ id: candidate.id, label: candidate.name, detail: `${candidate.channelCount} channels` }))} onChange={setSourceId} />
      <label className="guide-search"><NavigationSearchIcon /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Filter channels" aria-label="Filter channels" /></label>
      {groups.length > 1 && <LiveChoiceMenu className="group-choice" label="Group" value={group} choices={[{ id: 'all', label: 'All channels' }, ...groups.map((name) => ({ id: name, label: name }))]} onChange={setGroup} />}
      <SecondaryButton selected={favoritesOnly} onClick={() => setFavoritesOnly((current) => !current)}><ActionRateIcon fill={favoritesOnly ? 'currentColor' : 'none'} /> Favorites</SecondaryButton>
    </div>
    {error && <p className="live-action-message error" role="alert">{error}</p>}
    {channels.status === 'loading' && <StateSurface kind="loading" {...productState('live-tv.loading')} />}
    {channels.status === 'error' && (() => {
      const restricted = (channels.error as Error & { status?: number }).status === 401 || (channels.error as Error & { status?: number }).status === 403;
      const state = productState(restricted ? 'live-tv.restricted' : 'live-tv.offline');
      return <StateSurface kind={restricted ? 'permission' : 'error'} title={state.title} message={reviewedProductErrorText(channels.error, restricted ? 'live-tv.restricted' : 'live-tv.offline')} onRetry={() => setRevision((current) => current + 1)} />;
    })()}
    {channels.status === 'success' && channels.data.items.length === 0 && <StateSurface kind="empty" {...productState(query || favoritesOnly || group !== 'all' ? 'live-tv.channels-filter-empty' : 'live-tv.empty')} />}
    {channels.status === 'success' && channels.data.items.length > 0 && <div className="channel-directory" aria-label="Channels">{channels.data.items.map((channel) => <article className={selectedId === channel.id ? 'selected' : ''} key={channel.id}>
      <button type="button" className="channel-directory-select" onClick={() => setSelectedId(channel.id)} aria-label={`Select ${channel.name}`}>
        <ChannelLogo channel={channel} source={source} />
        <span><strong>{channel.name}</strong><span>{[channel.number && `Channel ${channel.number}`, channel.groupTitle, channel.country].filter(Boolean).join(' · ')}</span></span>
      </button>
      <div>{hasAction(channel, liveActions.play) && <SecondaryButton disabled={busy === `watch:${channel.id}`} onClick={() => void watch(channel)}><PlaybackPlayIcon /> {productText('action.watch-live')}</SecondaryButton>}{(hasAction(channel, liveActions.favoriteAdd) || hasAction(channel, liveActions.favoriteRemove)) && <IconButton disabled={busy === `favorite:${channel.id}`} label={`${channel.favorite ? 'Remove' : 'Add'} ${channel.name} ${channel.favorite ? 'from' : 'to'} favorites`} className={channel.favorite ? 'selected' : ''} onClick={() => void favorite(channel)}><ActionRateIcon fill={channel.favorite ? 'currentColor' : 'none'} /></IconButton>}</div>
    </article>)}</div>}
    {channels.status === 'success' && supportsPaging && (cursors.length > 0 || channels.data.pageInfo.hasMore) && <div className="guide-page-controls">
      <SecondaryButton disabled={cursors.length === 0} onClick={() => setCursors((current) => current.slice(0, -1))}><NavigationPreviousIcon /> Previous</SecondaryButton>
      <span>Page {cursors.length + 1}{channels.data.pageInfo.total != null ? ` · ${channels.data.pageInfo.total} channels` : ''}</span>
      <SecondaryButton disabled={!channels.data.pageInfo.hasMore || !channels.data.pageInfo.nextCursor} onClick={() => channels.data.pageInfo.nextCursor && setCursors((current) => [...current, channels.data.pageInfo.nextCursor!])}>Next <NavigationDisclosureIcon /></SecondaryButton>
    </div>}
  </div>;
}
