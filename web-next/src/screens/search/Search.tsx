import type {SearchSelection} from './selection';
import {SelectionCircle} from '../../kit/selection';
import {t, locale} from '../detail/text';
import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {preferenceValue} from '@core/preferences.ts';
import {searchQueryProblem, type SearchEntry, type SearchGroup, type SearchGroupResult} from '@core/search.ts';
import {SEARCH_CHIPS, orderedSearchGroups, searchGroupLabel, topSearchResult} from '@core/presentation/search-model.ts';
import {Button, Chip, TextField} from '../../kit/controls';
import {PosterCard, SquareCard, CircleCard, EpisodeTile, GenreTile, LiveCard} from '../../kit/cards';
import {Artwork} from '../../kit/artwork';
import {WindowedGrid} from '../../kit/layout';
import {Skeleton} from '../../kit/states';
import {Banner} from '../../kit/overlays';
import {SearchEmpty} from '../../kit/states/SearchEmpty';
import {entryCard} from '../detail/model';
import {dependsOn, titlesWithPersonalState} from '@core/freshness-hub.ts';
import {useFreshDocument} from '../../platform/freshness';
import {Icon} from '../../kit/Icon';
import type {SearchBindings} from './bindings';
import type {SearchOnNowCard} from './on-now';
import {appendedWindow} from './window';
import styles from './search.module.css';
import {TopResult} from './TopResult';
import {runScreenCommand} from '../detail/context';
import {useLoadingClock} from '../detail/Loading';
import {ScreenProblem} from '../detail/ScreenProblem';
import {useShellWidth} from '../../shell/viewport';
import {RecentSearches} from './RecentSearches';
import {nowNext} from '@core/guide/index.ts';
import {channelTint, type FactPart} from '@portico/client-core/kit-model';
import {clockText, durationText} from '@core/presentation/kit-model/text.ts';
import {defaultI18n} from '@i18n';

type ChipId = SearchGroup | 'all';
const keyOf = (entry: SearchEntry) => JSON.stringify(['libraryId' in entry ? entry.libraryId : '', entry.kind, entry.id]);
const searchTimezone = () => defaultI18n.region.timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone;
const onNowTime = (at: number) => new Intl.DateTimeFormat(locale(), {hour: 'numeric', minute: '2-digit', timeZone: searchTimezone()}).format(new Date(at));
function onNowProgramme(card: SearchOnNowCard, now: number) {
  const current = nowNext(card.programmes, now), programme = current.now;
  if (!programme) return {title: t('guide.noInformation'), facts: [] as readonly FactPart[]};
  const facts: FactPart[] = [];
  if (programme.subtitle) facts.push({id: 'programme-subtitle', role: 'context', text: programme.subtitle});
  facts.push({id: 'ends', role: 'context', text: t('guide.ends', {time: onNowTime(programme.end)})});
  return {title: programme.title, facts, progress: current.progress, live: programme.flags?.live === true};
}
export type SearchScreenProps = {binding: SearchBindings; selection?: SearchSelection; linkedQuery?: string; onConsumeLinkedQuery?: () => void; onOpenLive?: (sourceId?: string) => void};

export function SearchScreen({binding, selection, linkedQuery, onConsumeLinkedQuery, onOpenLive}: SearchScreenProps) {
  useSyncExternalStore(selection?.subscribe ?? noSelectionSubscription, selection?.getSnapshot ?? noSelectionSnapshot, selection?.getSnapshot ?? noSelectionSnapshot);
  const selectionEpoch = selection?.epoch();
  const compact = useShellWidth() < 600;
  const {service, context} = binding, initial = binding.session();
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  useFreshDocument([dependsOn.libraries, ...titlesWithPersonalState(), dependsOn.settings('preferences'), dependsOn.event('stream.resync')], refresh => service.revalidate(refresh), () => service.getSnapshot().phase === 'ready');
  const preferences = useSyncExternalStore(context.preferences.subscribe, context.preferences.getSnapshot, context.preferences.getSnapshot);
  const [text, setText] = useState(linkedQuery ?? initial.text), [chip, setChip] = useState<ChipId>(initial.group ?? 'all');
  const [recent, setRecent] = useState<readonly string[]>([]), [clearFailed, setClearFailed] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const genres = useSyncExternalStore(binding.genres.subscribe, binding.genres.getSnapshot, binding.genres.getSnapshot);
  const onNow = useSyncExternalStore(binding.onNow.subscribe, binding.onNow.getSnapshot, binding.onNow.getSnapshot);
  const input = useRef<HTMLInputElement>(null), results = useRef<HTMLDivElement>(null), committed = useRef('');
  const group = chip === 'all' ? undefined : chip, problem = searchQueryProblem(text);
  const remember = preferences.effective['search.rememberHistory'] === true;
  const submit = (record = false) => {
    if (!context.isCurrent()) return;
    void binding.select(problem ? '' : text.trim(), {...(group ? {group} : {}), ...(record && remember ? {record: true} : {})}).catch(context.effects.reportError);
  };
  useEffect(() => { if (linkedQuery !== undefined) onConsumeLinkedQuery?.(); }, [linkedQuery, onConsumeLinkedQuery]);
  useEffect(() => {
    if (snapshot.query?.q === text.trim() && snapshot.query?.group === group) return;
    // Withdraw the old query immediately, including during the debounce.
    service.cancel();
    const timer = setTimeout(() => { void binding.select(problem ? '' : text.trim(), group ? {group} : {}).catch(context.effects.reportError); }, 260);
    return () => clearTimeout(timer);
  }, [binding, service, text, group, problem, context.effects.reportError, snapshot.query?.q, snapshot.query?.group]);
  useEffect(() => { input.current?.focus(); }, [binding]);
  useEffect(() => {
    let active = true;
    if (!remember) { setRecent([]); return; }
    void binding.recent().then(page => { if (active && context.isCurrent()) setRecent(page.entries.slice(0, 8).map(item => item.q)); }, () => {});
    return () => { active = false; };
  }, [binding, context, remember]);
  useEffect(() => {
    if (text.trim()) return;
    if (binding.genres.getSnapshot().phase === 'idle') void binding.genres.load().catch(() => {});
  }, [binding, text]);
  useEffect(() => {
    if (text.trim() || context.identity?.liveAvailable !== true) return;
    if (binding.onNow.getSnapshot().phase === 'idle') void binding.onNow.load().catch(() => {});
  }, [binding, context.identity?.liveAvailable, text]);
  useEffect(() => {
    if (text.trim()) return;
    const timer = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, [text]);
  const commit = () => { if (remember && text.trim() && !problem && committed.current !== text.trim()) { committed.current = text.trim(); submit(true); } };
  const showing = snapshot.data?.query.q === text.trim() && snapshot.data.query.group === (group ?? '') ? snapshot.data : null;
  const groups = showing ? snapshot.groups : [], top = !group && !selection?.isActive() ? topSearchResult(text, groups) : undefined;
  useEffect(() => {if (selection && top?.entry && 'libraryId' in top.entry) return selection.register(top.entry, -1);}, [selection, top?.entry, selectionEpoch]);
  const failed = groups.filter(value => value.status === 'error');
  const size = preferenceValue<number>(preferences, 'appearance.cardSizePercent') / 100;
  const empty = showing && groups.every(value => value.status === 'success' && !value.items.length);
  const recentBlock = <RecentSearches queries={recent} compact={compact}
    onSelect={query => {if (context.isCurrent()) {setText(query);setChip('all');committed.current = '';}}}
    onClear={() => {
      void binding.clearRecent().then(() => {if (context.isCurrent()) {setRecent([]);setClearFailed(false);}}, () => {if (context.isCurrent()) setClearFailed(true);});
    }}/>;
  const videoKinds = new Intl.ListFormat(locale(), {style: 'long', type: 'conjunction'}).format([
    t('web.libraryKind.plural', {kind: 'movie'}), t('web.libraryKind.plural', {kind: 'tv'}), t('web.libraryKind.plural', {kind: 'anime'})]);
  const liveSourceCount = new Set(onNow.cards.map(card => card.channel.sourceId)).size;
  const beforeTyping = <div className={styles.empty}>
    {recentBlock}
    {genres.phase === 'ready' && genres.genres.length > 0 && <section className={styles.emptySection} aria-labelledby="search-genres">
      <header><h2 id="search-genres">{t('next.search.browseByGenre')}</h2><span>{videoKinds}</span></header>
      <div className={styles.genres}>{genres.genres.flatMap((genre, index) => {
        if (!genre.backdrop) return [];
        const art = context.artwork.foreground(genre.backdrop.path, 632, 16 / 9, {group: 'search-genres', order: index});
        if (!art) return [];
        return [<GenreTile key={genre.value} variant="video" context="search" artwork={art} visible
          model={{title: genre.label, caption: [{id: 'count', role: 'supplement', text: t('next.selection.titles', {count: genre.count})}]}}
          onNavigate={() => runScreenCommand(context, () => {const browse = context.effects.openBrowse;if (browse) browse(binding.genres.browseTarget(genre));})} />];
      })}</div>
    </section>}
    {context.identity?.liveAvailable === true && onNow.phase === 'ready' && onNow.cards.length > 0 && <section className={styles.emptySection} aria-labelledby="search-on-now">
      <header><h2 id="search-on-now">{t('next.home.onNow')}</h2><span>{onNow.sourceNames.length ? onNow.sourceNames.join(', ') : t('next.live.sources', {count: liveSourceCount})}</span>
        <Button variant="text" size="small" className={styles.trailingLink} icon="i-right" disabled={!onOpenLive} onClick={() => onOpenLive?.()}>{t('next.controls.guide')}</Button></header>
      <div className={styles.lives}>{onNow.cards.map(card => <LiveCard key={card.channel.sourceId + ':' + card.channel.id}
        channel={{sourceId: card.channel.sourceId, id: card.channel.id, name: card.channel.name, ...(card.channel.number !== undefined ? {number: card.channel.number} : {}),
          ...(card.channel.mark ? {mark: {...card.channel.mark, sourceId: card.channel.sourceId, id: card.channel.id}} : {})}}
        tint={card.channel.mark ? channelTint(card.channel.mark.seed) : undefined}
        logo={card.channel.logoUrl ? context.artwork.foreground(card.channel.logoUrl, 68, 1) : undefined}
        programme={onNowProgramme(card, now)} onNavigate={() => onOpenLive?.(card.channel.sourceId)} />)}</div>
    </section>}
  </div>;
  return <div className={styles.search} style={{'--search-card-scale': size, '--search-columns': Math.max(1, Math.round(7 / size))} as React.CSSProperties}>
    {selection?.toolbar}
    <div className={styles.wash} aria-hidden="true" />
    <form className={styles.field} role="search" onSubmit={event => { event.preventDefault(); submit(true); }}>
      <TextField ref={input} variant="large-search" type="search" autoComplete="off" spellCheck={false} value={text}
        aria-label={t('web.search.title')} placeholder={t('next.controls.searchLibraries')}
        onChange={event => { setText(event.currentTarget.value); setChip('all'); committed.current = ''; }}
        onKeyDown={event => { if (event.key === 'ArrowDown') { event.preventDefault(); results.current?.querySelector<HTMLElement>('button:not(:disabled),a[href]')?.focus(); } }}
        trailing={text ? <Button variant="icon" icon="i-x" iconOnly aria-label={t('web.search.clear')} onClick={() => { setText(''); setChip('all'); input.current?.focus(); }} />
          : <span className={styles.hint}><kbd>{t('next.controls.searchShortcut')}</kbd>{t('next.controls.searchFromAnywhere')}</span>} />
    </form>
    {text.trim() && !(empty && !group) && <div className={styles.chips} role="group" aria-label={t('web.search.title')}>{SEARCH_CHIPS.map(value => {
      const capability = showing?.capabilities.groups.find(item => item.id === value.id), result = groups.find(item => item.id === value.id) ?? binding.summary().find(item => item.id === value.id);
      if (value.id !== 'all' && capability?.available === false) return null;
      const off = value.id !== 'all' && result?.status === 'success' && result.totalCountExact && result.totalCount === 0;
      return <Chip key={value.id} variant="search" selected={chip === value.id} dim={off} disabled={off} retry={result?.status === 'error'}
        count={!off && result?.totalCountExact ? result.totalCount : undefined} onClick={() => { setChip(value.id); }}>{t(value.label)}</Chip>;
    })}</div>}
    <div ref={results} className={styles.results}>
      {!text.trim() && beforeTyping}
      {clearFailed && <p role="alert">{t('web.search.clearFailed')}</p>}
      {problem && <p role="status">{t(problem === 'short' ? 'web.search.tooShort' : problem === 'words' ? 'web.search.tooManyWords' : 'web.search.tooLong')}</p>}
      {snapshot.phase === 'loading' && !showing && <SearchLoading key={snapshot.generation} />}
      {(snapshot.phase === 'error' || snapshot.phase === 'refresh-required') && <ScreenProblem context={context} error={snapshot.error} retry={() => { void service.retry(); }} />}
      {failed.length > 0 && <div role="status" className={styles.partial}><Icon name="i-clock" size={22} /><span>{t('web.search.partial', {groups: new Intl.ListFormat(locale(), {style: 'long', type: 'conjunction'}).format(failed.map(value => t(searchGroupLabel(value.id))))})}</span><Button icon="i-retry" size="small" onClick={() => { void service.refresh(); }}>{t('next.states.tryAgain')}</Button></div>}
      {empty && <SearchEmpty title={t(group ? 'web.search.nothingIn' : 'web.search.noMatches', group ? {group: t(searchGroupLabel(group))} : {query: text.trim()})} body={group ? undefined : t('web.search.noMatchesBody')}
        action={group ? <Button variant="primary" size="compact" onClick={() => setChip('all')}>{t('web.search.everything')}</Button> : undefined} />}
      {empty && !group && recentBlock}
      {top && <TopResult binding={binding} result={top} />}
      {group && !empty ? <SearchGroupWindow binding={binding} group={group} result={groups.find(value => value.id === group)} selection={selection} onCommit={commit} />
        : orderedSearchGroups(groups).map((result, groupIndex) => {
          const entries = result.items.filter(entry => entry !== top?.entry), cap = result.id === 'episodes' || result.id === 'songs' ? 4 : result.id === 'movies' || result.id === 'shows' ? 7 : 4;
          if (!entries.length) return null;
          return <section key={result.id} className={styles.section} data-group={result.id}>
            <header><h2>{t(searchGroupLabel(result.id))}</h2>{result.totalCountExact && <span>{result.totalCount > cap ? t('web.search.someOf', {shown: Math.min(cap, entries.length), total: result.totalCount}) : result.totalCount}</span>}
              {(result.hasMore || entries.length > cap) && <Button variant="text" size="small" onClick={() => setChip(result.id)}><span className={styles.seeAll}>{t('web.search.seeAll')}<Icon name="i-right" size={18} /></span></Button>}</header>
            <div className={styles.cards}>{entries.slice(0, cap).map((entry, position) => <SearchResult key={keyOf(entry)} entry={entry} binding={binding} selection={selection} position={groupIndex * 1000 + position} onCommit={commit} />)}</div>
          </section>;
        })}
    </div>
  </div>;
}

function SearchLoading() {
  const elapsed = useLoadingClock();
  return <div className={styles.skeleton} role="status" aria-label={t('next.states.loading')}>{elapsed >= 300 && Array.from({length: 7}, (_, index) => <Skeleton className={styles.skeletonBlock} sheen={index === 2} key={index} />)}{elapsed >= 8000 && <Banner variant="quiet">{t('next.states.stillLoading')}</Banner>}</div>;
}

const noSelectionSubscription = () => () => {};
const noSelectionSnapshot = () => 0;
export function SearchResult({entry, binding, selection, position = 0, onCommit}: {entry: SearchEntry; binding: SearchBindings; selection?: SearchSelection; position?: number; onCommit: () => void}) {
  useSyncExternalStore(selection?.subscribe ?? noSelectionSubscription, selection?.getSnapshot ?? noSelectionSnapshot, selection?.getSnapshot ?? noSelectionSnapshot);
  const epoch = selection?.epoch();
  useEffect(() => {if (selection && 'libraryId' in entry) return selection.register(entry, position);}, [selection, entry, position, epoch]);
  const picked = 'libraryId' in entry ? selection?.card(entry) : undefined;
  const {context} = binding, open = () => runScreenCommand(context, () => { binding.open(entry); onCommit(); });
  const liveCards = useSyncExternalStore(binding.live.subscribe, binding.live.getSnapshot, binding.live.getSnapshot);
  const art = context.artwork.foreground(entry.posterUrl, 380, 2 / 3);
  if (entry.kind === 'channel') {
    const live = liveCards.get(entry.id);
    if (live) return <LiveCard className={styles.liveCard} context="search" channel={live.channel}
      programme={live.programme} tint={live.tint} results onNavigate={open} logo={live.channel.logoUrl ? context.artwork.foreground(live.channel.logoUrl, 68, 1) : undefined} />;
    return <button className={styles.channel} type="button" onClick={open}><Icon name="i-tv" size={24} /><b>{entry.title}</b>{entry.subtitle && <small>{entry.subtitle}</small>}</button>;
  }
  if (!('libraryId' in entry)) return <CircleCard context="search" model={{title: entry.title, caption: entry.subtitle ? [{id: 'caption', text: entry.subtitle, role: 'context'}] : []}} covers={art ? [art] : []}
    initials={entry.title.split(/\s+/).filter(Boolean).slice(0, 2).map(word => Array.from(word)[0]).join('')} onNavigate={open} />;
  const model = entryCard(entry, {t, locale: locale()}, 'search'), playable = entry.available === true && !!entry.playback;
  const props = {model, selection: picked, context: 'search' as const, watched: entry.watched, isNew: entry.new, onNavigate: open,
    onPlay: playable ? () => { void binding.play(entry).then(onCommit).catch(context.effects.reportError); } : undefined};
  if (entry.kind === 'song') return <div className={styles.songSelection}>{picked && <SelectionCircle variant="row" selected={picked.checked} selecting={picked.active} accessibilityLabel={t('next.selection.select')} onClick={event => {event.preventDefault(); picked.onToggle({range: event.shiftKey, source: event.detail === 0 ? 'keyboard' : 'pointer'});}}/>}<button className={styles.song} type="button" onClick={event => {if (picked?.active) picked.onToggle({range: event.shiftKey, source: event.detail === 0 ? 'keyboard' : 'pointer'}); else if (playable) void binding.play(entry).then(onCommit).catch(context.effects.reportError); else open();}}><Artwork source={context.artwork.foreground(entry.posterUrl, 88, 1)} alt="" /><span><b>{entry.displayTitle ?? entry.title}</b><small>{entry.artist?.name}{entry.album?.name && <span>{entry.album.name}</span>}</small></span><span>{clockText(entry.duration)}</span></button></div>;
  if (entry.kind === 'episode') return <EpisodeTile {...props} length={durationText(entry.duration, t)} artwork={context.artwork.foreground(entry.stillUrl ?? entry.backdropUrl, 632, 16 / 9)} artworkKind={entry.stillUrl ? 'still' : 'backdrop'} description={entry.overview} />;
  if (entry.kind === 'artist') return <CircleCard {...props} covers={art ? [art] : []} variant="artist" />;
  if (['album', 'book', 'audiobook_file'].includes(entry.kind)) return <SquareCard {...props} artwork={context.artwork.foreground(entry.posterUrl, 300, 1)} />;
  return <PosterCard {...props} artwork={art} progressPlacement="inset" />;
}

function SearchGroupWindow({binding, group, result, selection, onCommit}: {binding: SearchBindings; selection?: SearchSelection; group: SearchGroup; result?: SearchGroupResult; onCommit: () => void}) {
  const window = useMemo(() => appendedWindow<SearchEntry>(keyOf), []);
  const entries = useRef<readonly SearchEntry[]>([]), key = useRef(''), [, redraw] = useState(0);
  const snapshot = binding.service.getSnapshot(), queryKey = JSON.stringify([snapshot.query?.q, group, snapshot.data?.scope.viewerFence, snapshot.data?.revision]);
  useEffect(() => {
    if (!result || snapshot.phase !== 'ready') return;
    if (key.current !== queryKey || !snapshot.pagination.cursor) entries.current = [];
    key.current = queryKey;
    const seen = new Set(entries.current.map(keyOf));
    entries.current = [...entries.current, ...result.items.filter(value => !seen.has(keyOf(value)))];
    window.set(entries.current); redraw(value => value + 1);
  }, [window, result, queryKey, snapshot.phase, snapshot.pagination.cursor]);
  useEffect(() => () => window.dispose(), [window]);
  return <section className={styles.single}><h2>{t(searchGroupLabel(group))}</h2><WindowedGrid collection={window.collection} label={t(searchGroupLabel(group))}
    cardWidth={group === 'songs' ? 10000 : group === 'episodes' ? 316 : 174} rowHeight={group === 'songs' ? 58 : group === 'episodes' ? 285 : 320} height="calc(100vh - 270px)" gap={16}
    renderItem={(entry, item) => <SearchResult entry={entry} binding={binding} selection={selection} position={item.index} onCommit={onCommit} />} renderPlaceholder={index => <span className={styles.placeholder} key={index} />}
    onVisibleRange={(_, last) => { if (last >= entries.current.length - 20 && result?.nextCursor && binding.service.getSnapshot().phase === 'ready') void binding.service.next(group); }} /></section>;
}
