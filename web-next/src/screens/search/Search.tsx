import {t, locale} from '../detail/text';
import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {preferenceValue} from '@core/preferences.ts';
import {searchQueryProblem, type SearchEntry, type SearchGroup, type SearchGroupResult} from '@core/search.ts';
import {SEARCH_CHIPS, orderedSearchGroups, searchGroupLabel, topSearchResult} from '@core/presentation/search-model.ts';
import {Button, Chip, TextField} from '../../kit/controls';
import {PosterCard, SquareCard, CircleCard, EpisodeTile, LiveCard} from '../../kit/cards';
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
import {appendedWindow} from './window';
import styles from './search.module.css';
import {TopResult} from './TopResult';
import {runScreenCommand} from '../detail/context';
import {useLoadingClock} from '../detail/Loading';
import {ScreenProblem} from '../detail/ScreenProblem';
import {useShellWidth} from '../../shell/viewport';
import {RecentSearches} from './RecentSearches';

type ChipId = SearchGroup | 'all';
const keyOf = (entry: SearchEntry) => JSON.stringify(['libraryId' in entry ? entry.libraryId : '', entry.kind, entry.id]);
export type SearchScreenProps = {binding: SearchBindings; linkedQuery?: string; onConsumeLinkedQuery?: () => void};

export function SearchScreen({binding, linkedQuery, onConsumeLinkedQuery}: SearchScreenProps) {
  const compact = useShellWidth() < 600;
  const {service, context} = binding, initial = binding.session();
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  useFreshDocument([dependsOn.libraries, ...titlesWithPersonalState(), dependsOn.settings('preferences'), dependsOn.event('stream.resync')], refresh => service.revalidate(refresh), () => service.getSnapshot().phase === 'ready');
  const preferences = useSyncExternalStore(context.preferences.subscribe, context.preferences.getSnapshot, context.preferences.getSnapshot);
  const [text, setText] = useState(linkedQuery ?? initial.text), [chip, setChip] = useState<ChipId>(initial.group ?? 'all');
  const [recent, setRecent] = useState<readonly string[]>([]), [clearFailed, setClearFailed] = useState(false);
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
  const commit = () => { if (remember && text.trim() && !problem && committed.current !== text.trim()) { committed.current = text.trim(); submit(true); } };
  const showing = snapshot.data?.query.q === text.trim() && snapshot.data.query.group === (group ?? '') ? snapshot.data : null;
  const groups = showing ? snapshot.groups : [], top = !group ? topSearchResult(text, groups) : undefined;
  const failed = groups.filter(value => value.status === 'error');
  const size = preferenceValue<number>(preferences, 'appearance.cardSizePercent') / 100;
  const empty = showing && groups.every(value => value.status === 'success' && !value.items.length);
  const recentBlock = <RecentSearches queries={recent} compact={compact}
    onSelect={query => {if (context.isCurrent()) {setText(query);setChip('all');committed.current = '';}}}
    onClear={() => {
      void binding.clearRecent().then(() => {if (context.isCurrent()) {setRecent([]);setClearFailed(false);}}, () => {if (context.isCurrent()) setClearFailed(true);});
    }}/>;
  return <div className={styles.search} style={{'--search-card-scale': size, '--search-columns': Math.max(1, Math.round(7 / size))} as React.CSSProperties}>
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
      {!text.trim() && recentBlock}
      {clearFailed && <p role="alert">{t('web.search.clearFailed')}</p>}
      {problem && <p role="status">{t(problem === 'short' ? 'web.search.tooShort' : problem === 'words' ? 'web.search.tooManyWords' : 'web.search.tooLong')}</p>}
      {snapshot.phase === 'loading' && !showing && <SearchLoading key={snapshot.generation} />}
      {(snapshot.phase === 'error' || snapshot.phase === 'refresh-required') && <ScreenProblem context={context} error={snapshot.error} retry={() => { void service.retry(); }} />}
      {failed.length > 0 && <div role="status" className={styles.partial}><Icon name="i-clock" size={22} /><span>{t('web.search.partial', {groups: new Intl.ListFormat(locale(), {style: 'long', type: 'conjunction'}).format(failed.map(value => t(searchGroupLabel(value.id))))})}</span><Button icon="i-retry" size="small" onClick={() => { void service.refresh(); }}>{t('next.states.tryAgain')}</Button></div>}
      {empty && <SearchEmpty title={t(group ? 'web.search.nothingIn' : 'web.search.noMatches', group ? {group: t(searchGroupLabel(group))} : {query: text.trim()})} body={group ? undefined : t('web.search.noMatchesBody')}
        action={group ? <Button variant="primary" size="compact" onClick={() => setChip('all')}>{t('web.search.everything')}</Button> : undefined} />}
      {empty && !group && recentBlock}
      {top && <TopResult binding={binding} result={top} />}
      {group && !empty ? <SearchGroupWindow binding={binding} group={group} result={groups.find(value => value.id === group)} onCommit={commit} />
        : orderedSearchGroups(groups).map(result => {
          const entries = result.items.filter(entry => entry !== top?.entry), cap = result.id === 'episodes' || result.id === 'songs' ? 4 : result.id === 'movies' || result.id === 'shows' ? 7 : 4;
          if (!entries.length) return null;
          return <section key={result.id} className={styles.section} data-group={result.id}>
            <header><h2>{t(searchGroupLabel(result.id))}</h2>{result.totalCountExact && <span>{result.totalCount > cap ? t('web.search.someOf', {shown: Math.min(cap, entries.length), total: result.totalCount}) : result.totalCount}</span>}
              {(result.hasMore || entries.length > cap) && <Button variant="text" size="small" icon="i-right" onClick={() => setChip(result.id)}>{t('web.search.seeAll')}</Button>}</header>
            <div className={styles.cards}>{entries.slice(0, cap).map(entry => <SearchResult key={keyOf(entry)} entry={entry} binding={binding} onCommit={commit} />)}</div>
          </section>;
        })}
    </div>
  </div>;
}

function SearchLoading() {
  const elapsed = useLoadingClock();
  return <div className={styles.skeleton} role="status" aria-label={t('next.states.loading')}>{elapsed >= 300 && Array.from({length: 7}, (_, index) => <Skeleton className={styles.skeletonBlock} sheen={index === 2} key={index} />)}{elapsed >= 8000 && <Banner variant="quiet">{t('next.states.stillLoading')}</Banner>}</div>;
}

export function SearchResult({entry, binding, onCommit}: {entry: SearchEntry; binding: SearchBindings; onCommit: () => void}) {
  const {context} = binding, open = () => runScreenCommand(context, () => { binding.open(entry); onCommit(); });
  const liveCards = useSyncExternalStore(binding.live.subscribe, binding.live.getSnapshot, binding.live.getSnapshot);
  const art = context.artwork.foreground(entry.posterUrl, 380, 2 / 3);
  if (entry.kind === 'channel') {
    const live = liveCards.get(entry.id);
    if (live) return <LiveCard className={styles.liveCard} context="search" model={{title: live.programme?.title ?? entry.title, caption: []}} channel={live.channel}
      programme={live.programme} tint={live.tint} results onNavigate={open} logo={live.channel.logoUrl ? context.artwork.foreground(live.channel.logoUrl, 68, 1) : undefined} />;
    return <button className={styles.channel} type="button" onClick={open}><Icon name="i-tv" size={24} /><b>{entry.title}</b>{entry.subtitle && <small>{entry.subtitle}</small>}</button>;
  }
  if (!('libraryId' in entry)) return <CircleCard context="search" model={{title: entry.title, caption: entry.subtitle ? [{id: 'caption', text: entry.subtitle, role: 'context'}] : []}} covers={art ? [art] : []}
    initials={entry.title.split(/\s+/).filter(Boolean).slice(0, 2).map(word => Array.from(word)[0]).join('')} onNavigate={open} />;
  const model = entryCard(entry, {t, locale: locale()}, 'search'), playable = entry.available === true && !!entry.playback;
  const props = {model, context: 'search' as const, watched: entry.watched, isNew: entry.new, onNavigate: open,
    onPlay: playable ? () => { void binding.play(entry).then(onCommit).catch(context.effects.reportError); } : undefined};
  if (entry.kind === 'song') return <button className={styles.song} type="button" onClick={playable ? () => { void binding.play(entry).then(onCommit).catch(context.effects.reportError); } : open}><Artwork source={context.artwork.foreground(entry.posterUrl, 88, 1)} alt="" /><span><b>{entry.displayTitle ?? entry.title}</b><small>{entry.artist?.name}{entry.album?.name && <span>{entry.album.name}</span>}</small></span><span>{model.detail.find(part => part.id === 'duration')?.text}</span></button>;
  if (entry.kind === 'episode') return <EpisodeTile {...props} artwork={context.artwork.foreground(entry.stillUrl ?? entry.backdropUrl, 632, 16 / 9)} artworkKind={entry.stillUrl ? 'still' : 'backdrop'} description={entry.overview} />;
  if (entry.kind === 'artist') return <CircleCard {...props} covers={art ? [art] : []} variant="artist" />;
  if (['album', 'book', 'audiobook_file'].includes(entry.kind)) return <SquareCard {...props} artwork={context.artwork.foreground(entry.posterUrl, 300, 1)} />;
  return <PosterCard {...props} artwork={art} progressPlacement="inset" />;
}

function SearchGroupWindow({binding, group, result, onCommit}: {binding: SearchBindings; group: SearchGroup; result?: SearchGroupResult; onCommit: () => void}) {
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
    renderItem={entry => <SearchResult entry={entry} binding={binding} onCommit={onCommit} />} renderPlaceholder={index => <span className={styles.placeholder} key={index} />}
    onVisibleRange={(_, last) => { if (last >= entries.current.length - 20 && result?.nextCursor && binding.service.getSnapshot().phase === 'ready') void binding.service.next(group); }} /></section>;
}
