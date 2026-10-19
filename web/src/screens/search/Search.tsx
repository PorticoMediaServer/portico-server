import {cardCaptionOf} from '../../app/content';
import React, {useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import type {ContentEntry} from '@core/library-content.ts';
import {SearchService, clearSearchHistory, readSearchHistory, searchQueryProblem, type SearchEntry, type SearchGroup, type SearchGroupResult, type SearchSnapshot} from '@core/search.ts';
import {useNavigate, useRouter, useSearch} from '@tanstack/react-router';
import {NoLibraries} from '../shared/NoLibraries';
import {useLibrariesContext} from '../../app/libraries';
import {SEARCH_CHIPS, iconFor, orderedSearchGroups, progressFor, searchGroupLabel, shapeFor, topResultKindLabel, topSearchResult, type TopSearchResult} from '@core/presentation/index.ts';
import {useEntryHref, useOpenEntry} from '../../app/open';
import {useSession} from '../../app/session';
import {Artwork, Button, Card, Chip, Chips, Grid, Icon, Inset, Notice, Page, PageHeader, Row, Shelf, StateView, Text} from '../../ui';
import {LoadingShelves} from '../shared/Sections';
import s from './Search.module.css';
import {useServerPreferences} from '../../app/server-preferences';
import {ErrorNotice, ErrorState, changedError} from '../../app/errors';
import {useViewerScope} from '../../app/viewer-scope';
import {useFreshDocument} from '../../app/freshness';
import {dependsOn, titlesWithPersonalState} from '@core/freshness-hub.ts';

import {useI18n, type I18n} from '../../app/i18n';
import {useRefreshRequiredRetry} from '../../app/quiet-refresh';

type ChipId = SearchGroup | 'all';
/** The type chips, in the one authored order (client-core `search-model.ts`); result sections follow it. */
const chips = (t: I18n['t']): readonly {id: ChipId; label: string}[] => SEARCH_CHIPS.map(chip => ({id: chip.id, label: t(chip.label)}));
const DEBOUNCE_MS = 260;
const RECENT_LIMIT = 8;

/**
 * WEB-SEARCH-01: the search session outlives the screen, so opening a result and coming back
 * shows the same query, chip and results (the router restores the scroll offset). What was typed
 * is never written into the address, so it does not enter the browser's history or anything that
 * syncs it; a link that carries a query (`/search?q=…`) is searched once and the query is then
 * taken out of the address. The session belongs to one viewer on one server: a profile switch or
 * sign-out starts a fresh one.
 *
 * Recent searches (WEB-SEARCH-02) are the profile's own, kept by the server while "Remember
 * search history" is on: a search is remembered when it is submitted or one of its results is
 * opened, never as it is typed. With the setting off nothing is remembered or shown, and Clear
 * removes what the server kept.
 */
type Kept = {key: string; api: unknown; service: SearchService; text: string; chip: ChipId; recent: string[]};
let kept: Kept | null = null;
/** Search reaches every library this viewer may see, with this profile's marks on the results. */
const searchDependencies = Object.freeze([dependsOn.libraries, ...titlesWithPersonalState()]);
function useSearchSession(api: ConstructorParameters<typeof SearchService>[0]['api'], scope: {viewerId: string; serverId: string}): Kept {
  const key = `${scope.serverId}\n${scope.viewerId}`;
  const session = useMemo(() => {
    if (kept && kept.key === key && kept.api === api) return kept;
    kept?.service.dispose();
    kept = {key, api, service: new SearchService({api, scope}), text: '', chip: 'all', recent: []};
    return kept;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, key]);
  return session;
}

/**
 * Search: one field and the type chips (both stay in view while results scroll), a Top result
 * when one title matches strongly, then server-grouped results in the chips' order. The server
 * owns matching and ranking.
 */
export function SearchScreen() {
  const i18n = useI18n();
  const {t} = i18n;
  const {api} = useSession();
  const libraries = useLibrariesContext();
  const scope = useViewerScope();
  const mem = useSearchSession(api, scope);
  // Results on screen follow the libraries: a title that arrives, changes or goes updates its card in place.
  useFreshDocument(searchDependencies, context => mem.service.revalidate(context), () => mem.service.getSnapshot().phase === 'ready');
  const service = mem.service;
  const snapshot: SearchSnapshot = useSyncExternalStore(service.subscribe, service.getSnapshot);
  const changedStuck = useRefreshRequiredRetry(snapshot.phase === 'refresh-required', () => service.refresh());
  const address = useSearch({from: '/app/search'});
  const navigate = useNavigate();
  // A linked search (`?q=`) wins over the remembered one.
  const linked = useRef(address.q !== undefined && address.q !== mem.text);
  if (linked.current) { linked.current = false; mem.text = address.q ?? ''; mem.chip = (address.type as ChipId | undefined) ?? 'all'; }
  const [text, setTextState] = useState(mem.text);
  // An IME composition is not a query yet: the search waits for the finished text, and Enter or
  // an arrow key that picks a candidate is not a submit or a jump into the results.
  const [composing, setComposing] = useState(false);
  const [chip, setChipState] = useState<ChipId>(mem.chip);
  const [recent, setRecent] = useState<string[]>(mem.recent);
  const setText = (value: string) => { mem.text = value; setTextState(value); };
  const setChip = (value: ChipId) => { mem.chip = value; setChipState(value); };
  // Search words stay out of the address: one that arrived with a link is searched, then removed from it.
  useEffect(() => {
    if (address.q !== undefined || address.type !== undefined) void navigate({to: '/search', search: {}, replace: true});
  }, [address.q, address.type, navigate]);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const input = useRef<HTMLInputElement>(null);
  const results = useRef<HTMLDivElement>(null);
  // A restored session already holds the results for its query; don't ask again on return.
  const restored = useRef(!!mem.text && snapshot.phase !== 'idle');
  const group: SearchGroup | undefined = chip === 'all' ? undefined : chip;
  // M15 CD-30: a query the server wouldn't search ("a", "!!!", nine words) is said here and sent
  // nowhere, and the previous query's results don't stay on screen as if they were its answer.
  const [problem, setProblem] = useState<'short' | 'words' | 'long'>();
  const submit = useCallback((value: string, selected: SearchGroup | undefined) => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
    const invalid = searchQueryProblem(value.trim());
    setProblem(invalid);
    try {
      if (invalid) { void service.select('').catch(() => {}); return; }
      void service.select(value.trim(), selected).catch(() => {});
    } catch {}
  }, [service]);
  useEffect(() => {
    if (timer.current) clearTimeout(timer.current);
    if (composing) return;
    if (restored.current) { restored.current = false; return; }
    timer.current = setTimeout(() => submit(text, group), DEBOUNCE_MS);
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [text, composing]);
  const firstGroup = useRef(true);
  useEffect(() => {
    if (firstGroup.current) { firstGroup.current = false; return; }
    if (text.trim()) submit(text, group);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [group]);
  useEffect(() => { if (!mem.text) input.current?.focus(); }, [mem]);
  /* Recent searches follow the profile's "Remember search history": the server keeps them, and
     with the setting off none are kept or shown. */
  const remember = useServerPreferences().value('search.rememberHistory', true);
  useEffect(() => {
    if (!remember) { mem.recent = []; setRecent([]); return; }
    const controller = new AbortController();
    void readSearchHistory(api, scope, controller.signal).then(page => {
      mem.recent = page.entries.slice(0, RECENT_LIMIT).map(entry => entry.q);
      setRecent(mem.recent);
    }, () => { /* recents are a convenience: without them the field still works */ });
    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [remember, api, scope]);
  /* A search is remembered when the person commits to it (Enter, or opening one of its results),
     never while it is being typed. The server records it; the chips follow at once. */
  const committed = useRef('');
  const commit = useCallback((typed?: string) => {
    const q = (typed ?? service.getSnapshot().query?.q ?? '').trim();
    if (!remember || !q || committed.current === q.toLowerCase() || searchQueryProblem(q)) return;
    committed.current = q.toLowerCase();
    const next = [q, ...mem.recent.filter(r => r.toLowerCase() !== q.toLowerCase())].slice(0, RECENT_LIMIT);
    mem.recent = next;
    setRecent(next);
    void service.select(q, {...(mem.chip === 'all' ? {} : {group: mem.chip}), record: true}).catch(() => {});
  }, [remember, service, mem]);
  const [clearFailed, setClearFailed] = useState(false);
  const clearRecent = () => {
    const before = mem.recent;
    mem.recent = []; setRecent([]); committed.current = ''; setClearFailed(false);
    // What the server kept is removed too; if that fails the chips come back and say so.
    void clearSearchHistory(api).catch(() => { mem.recent = before; setRecent(before); setClearFailed(true); });
  };
  /* ArrowDown from the field moves into the first result. */
  const intoResults = () => (results.current?.querySelector<HTMLElement>('button, a[href]'))?.focus();
  /* Show a count when the server has an exact total for this group. */
  const counts = useMemo(() => new Map(snapshot.groups.map(g => [g.id, g.totalCountExact ? g.totalCount : undefined] as const)), [snapshot.groups]);
  const query = snapshot.query?.q ?? '';
  const hasQuery = text.trim().length > 0;
  const sections = orderedSearchGroups(snapshot.groups);
  const top = !group && snapshot.phase === 'ready' ? topSearchResult(query, snapshot.groups) : undefined;
  const focused = group ? snapshot.groups.find(x => x.id === group) : undefined;
  const failed = snapshot.groups.filter(x => x.status === 'error');
  return (
    <Page>
      <PageHeader title={t('web.search.title')} />
      {/* The field and the type chips stay in view while results scroll. */}
      <div className={s.sticky}>
        <Inset>
          <div className={s.field}>
            <Icon name="search" size={20} className={s.fieldIcon} />
            <input ref={input} className={s.input} type="search" value={text} onChange={e => setText(e.target.value)} onCompositionStart={() => setComposing(true)} onCompositionEnd={e => { setComposing(false); setText(e.currentTarget.value); }} placeholder={t('web.search.placeholder')} aria-label={t('web.search.title')} autoCapitalize="off" autoCorrect="off" spellCheck={false} enterKeyHint="search" onKeyDown={e => { if (e.nativeEvent.isComposing) return; if (e.key === 'Enter') { submit(text, group); commit(text); } else if (e.key === 'ArrowDown') { e.preventDefault(); intoResults(); } }} />
            {text ? <Button variant="ghost" icon="close" aria-label={t('web.search.clear')} size="sm" className={s.clear} onClick={() => { setText(''); input.current?.focus(); }} /> : null}
          </div>
          {problem && hasQuery ? <Text variant="caption" tone="secondary" role="status">{t(problem === 'short' ? 'web.search.tooShort' : problem === 'words' ? 'web.search.tooManyWords' : 'web.search.tooLong')}</Text> : null}
        </Inset>
        <Inset>
          <Chips>
            {chips(t).map(item => {
              const count = item.id === 'all' ? undefined : counts.get(item.id);
              return <Chip key={item.id} label={item.label} count={count && count > 0 ? count : undefined} pressed={chip === item.id} onClick={() => setChip(item.id)} />;
            })}
          </Chips>
        </Inset>
      </div>
      {!hasQuery && recent.length ? (
        <Inset>
          <div className={s.recent}>
            <Row style={{justifyContent: 'space-between'}}><Text variant="label" tone="secondary">{t('web.search.recent')}</Text><Button variant="ghost" size="sm" label={t('web.search.clearRecent')} onClick={clearRecent} /></Row>
            <Chips>{recent.map(q => <Chip key={q} label={q} icon="clock" onClick={() => { setText(q); input.current?.focus(); }} />)}</Chips>
            {clearFailed ? <Notice tone="error" compact>{t('web.search.clearFailed')}</Notice> : null}
          </div>
        </Inset>
      ) : null}
      {!hasQuery && !recent.length ? (!libraries.items.length && !libraries.loading ? <NoLibraries where="search" /> : <StateView icon="search" title={t('web.search.emptyTitle')} body={t('web.search.emptyBody')} />) : null}
      {snapshot.phase === 'error' ? <Inset><ErrorNotice error={snapshot.error} context="search" retry={() => void service.retry().catch(() => {})} refresh={() => void service.refresh().catch(() => {})} /></Inset> : null}
      {changedStuck ? <Inset><ErrorNotice error={snapshot.error ?? changedError} context="search" refresh={() => void service.refresh().catch(() => {})} /></Inset> : null}
      {snapshot.phase === 'loading' && !snapshot.groups.length ? <LoadingShelves /> : null}
      {failed.length && snapshot.phase === 'ready' ? <Inset><Notice tone="warning" action={{label: t('action.tryAgain'), onClick: () => void service.refresh().catch(() => {})}}>{t('web.search.partial', {groups: i18n.list(failed.map(g => t(searchGroupLabel(g.id))))})}</Notice></Inset> : null}
      {snapshot.phase === 'ready' && !sections.length && hasQuery ? <StateView icon="search" title={t('web.search.noMatches', {query})} body={group ? t('web.search.nothingIn', {group: chips(t).find(c => c.id === group)?.label ?? group}) : t('web.search.noMatchesBody')} action={group ? {label: t('web.search.everything'), onClick: () => setChip('all')} : undefined} /> : null}
      <div ref={results} style={{display: 'contents'}} onClickCapture={e => { if (e.target instanceof Element && e.target.closest('a[href]')) commit(); }}>
      {group ? (
        focused && focused.items.length ? <GroupResults section={focused} canPrevious={snapshot.pagination.canPrevious} busy={snapshot.phase === 'loading'} onNext={focused.nextCursor ? () => void service.next(focused.id).catch(() => {}) : undefined} onPrevious={snapshot.pagination.canPrevious ? () => void service.previous().catch(() => {}) : undefined} /> : null
      ) : (
        <>
          {top ? <TopResult top={top} /> : null}
          {sections.map(section => <GroupShelf key={section.id} section={section} onSeeAll={section.totalCount > section.items.length || section.nextCursor ? () => setChip(section.id) : undefined} />)}
        </>
      )}
      </div>
    </Page>
  );
}

/** People and channels are search-only entries: they open their own pages rather than the item detail. */
function isContent(entry: SearchEntry): entry is ContentEntry {
  return entry.kind !== 'person' && entry.kind !== 'channel';
}
/** PERF-28: memoised on the entry, so typing (which re-renders the page) doesn't re-render every card. */
const ResultCard = React.memo(function ResultCard({entry}: {entry: SearchEntry}) {
  // Captions come from catalogue helpers: reading the catalogue here redraws the card in a new language.
  useI18n();
  const open = useOpenEntry();
  const hrefOf = useEntryHref();
  const router = useRouter();
  const navigate = useNavigate();
  if (!isContent(entry)) {
    const person = entry.kind === 'person';
    return <Card title={entry.title} caption={entry.subtitle} path={entry.posterUrl} shape={person ? 'circle' : 'landscape'} icon={person ? 'person' : 'live'} href={router.buildLocation(person ? {to: '/person/$personId', params: {personId: entry.navigation.entityId}} : {to: '/channels'}).href} onOpen={() => (person ? navigate({to: '/person/$personId', params: {personId: entry.navigation.entityId}}) : navigate({to: '/channels'}))} />;
  }
  // Episodes show their own still in search (the one place outside a show's page).
  if (entry.kind === 'episode') return <Card title={entry.title} caption={cardCaptionOf(entry)} path={entry.stillUrl ?? entry.backdropUrl ?? entry.posterUrl} shape="landscape" icon="tv" progress={progressFor(entry)} watched={entry.watched} href={hrefOf(entry)} onOpen={() => open(entry)} onPlay={entry.playback ? () => open(entry, 'play') : undefined} onMore={anchor => open(entry, 'more', undefined, anchor)} />;
  return <Card title={entry.title} caption={cardCaptionOf(entry)} path={entry.posterUrl ?? entry.backdropUrl} shape={shapeFor(entry.kind)} icon={iconFor(entry.kind)} progress={progressFor(entry)} watched={entry.watched} href={hrefOf(entry)} onOpen={() => open(entry)} onPlay={entry.playback ? () => open(entry, 'play') : undefined} onMore={anchor => open(entry, 'more', undefined, anchor)} />;
});
const densityFor = (section: SearchGroupResult) => {
  const first = section.items[0];
  if (!first) return 'poster';
  if (first.kind === 'person') return 'person';
  if (first.kind === 'channel' || first.kind === 'episode') return 'landscape';
  return shapeFor(first.kind) === 'square' ? 'square' : 'poster';
};

function GroupShelf({section, onSeeAll}: {section: SearchGroupResult; onSeeAll?: () => void}) {
  const {t} = useI18n();
  return (
    <Shelf title={t(searchGroupLabel(section.id))} count={section.totalCountExact ? section.totalCount : undefined} density={densityFor(section)} action={onSeeAll ? {label: t('web.search.seeAll'), onClick: onSeeAll} : undefined}>
      {section.items.map(e => <ResultCard key={e.id} entry={e} />)}
    </Shelf>
  );
}
function GroupResults({section, canPrevious, onNext, onPrevious, busy}: {section: SearchGroupResult; canPrevious: boolean; onNext?: () => void; onPrevious?: () => void; busy: boolean}) {
  const {t} = useI18n();
  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
      <Inset><Row style={{justifyContent: 'space-between'}}><Text variant="heading">{t(searchGroupLabel(section.id))}</Text>{section.totalCountExact ? <Text variant="caption" tone="tertiary">{section.totalCount > section.items.length ? t('web.search.someOf', {shown: section.items.length, total: section.totalCount}) : t('web.search.results', {count: section.totalCount})}</Text> : null}</Row></Inset>
      <Grid density={densityFor(section)}>{section.items.map(e => <ResultCard key={e.id} entry={e} />)}</Grid>
      {onNext || canPrevious ? <Inset><Row style={{justifyContent: 'center'}}><Button variant="secondary" icon="back" label={t('web.live.previous')} disabled={!onPrevious || busy} onClick={onPrevious} /><Button variant="secondary" iconAfter="forward" label={t('web.search.next')} disabled={!onNext || busy} loading={busy && !!onNext} onClick={onNext} /></Row></Inset> : null}
    </div>
  );
}

/** One title that matches the words strongly, above the sections: its artwork, what it is, and Play or Open. */
function TopResult({top}: {top: TopSearchResult}) {
  const {t} = useI18n();
  const open = useOpenEntry();
  const navigate = useNavigate();
  const entry = top.entry;
  const content = isContent(entry);
  const person = entry.kind === 'person';
  const go = () => (content ? open(entry) : person ? void navigate({to: '/person/$personId', params: {personId: entry.navigation.entityId}}) : void navigate({to: '/channels'}));
  const shape = person ? 'circle' : content ? shapeFor(entry.kind) : 'landscape';
  return (
    <Inset>
      <section className={s.top} aria-label={t('search.topResult')}>
        <Text variant="label" tone="tertiary">{t('search.topResult')}</Text>
        <div className={s.topBody}>
          <button type="button" className={s.topArt} data-shape={shape} onClick={go} aria-label={entry.title} tabIndex={-1}><Artwork path={entry.posterUrl ?? (content ? entry.backdropUrl : undefined)} shape={shape} icon={content ? iconFor(entry.kind) : person ? 'person' : 'live'} initial={person ? entry.title.slice(0, 1) : undefined} alt="" /></button>
          <div className={s.topCopy}>
            <Text as="h2" variant="heading">{entry.title}</Text>
            <Text variant="body" tone="secondary">{[t(topResultKindLabel(top.group) ?? searchGroupLabel(top.group)), content ? cardCaptionOf(entry) : entry.subtitle].filter(Boolean).join(' · ')}</Text>
            <Row>
              {content && entry.playback ? <Button variant="primary" icon="play" label={t('action.play')} onClick={() => open(entry, 'play')} /> : null}
              <Button variant={content && entry.playback ? 'secondary' : 'primary'} label={t('action.details')} onClick={go} />
            </Row>
          </div>
        </div>
      </section>
    </Inset>
  );
}
