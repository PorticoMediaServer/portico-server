import {cardCaptionOf} from '../../app/content';
import React, {useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {ContentEntry} from '@core/library-content.ts';
import {SearchService, searchQueryProblem, type SearchEntry, type SearchGroup, type SearchGroupResult, type SearchSnapshot} from '@core/search.ts';
import {useNavigate, useRouter, useSearch} from '@tanstack/react-router';
import {NoLibraries} from '../shared/NoLibraries';
import {useLibrariesContext} from '../../app/libraries';
import {SEARCH_CHIPS, iconFor, orderedSearchGroups, progressFor, searchGroupLabel, shapeFor, topResultKindLabel, topSearchResult, type TopSearchResult} from '@core/presentation/index.ts';
import {useEntryHref, useOpenEntry} from '../../app/open';
import {useSession} from '../../app/session';
import {Artwork, Button, Card, Chip, Chips, Grid, Icon, Inset, Notice, Page, PageHeader, Row, Shelf, StateView, Text} from '../../ui';
import {LoadingShelves} from '../shared/Sections';
import s from './Search.module.css';
import {ErrorNotice, ErrorState, changedError} from '../../app/errors';
import {useViewerScope} from '../../app/viewer-scope';

const t = defaultI18n.t;
type ChipId = SearchGroup | 'all';
/** The type chips, in the one authored order (client-core `search-model.ts`); result sections follow it. */
const chips = (): readonly {id: ChipId; label: string}[] => SEARCH_CHIPS.map(chip => ({id: chip.id, label: t(chip.label)}));
const DEBOUNCE_MS = 260;
const RECENT_LIMIT = 8;

/**
 * WEB-SEARCH-01: the search session outlives the screen, so opening a result and coming back
 * shows the same query, chip and results (the router restores the scroll offset). The query and
 * the chip are also in the address (`/search?q=…&type=…`), so a search can be linked and Back
 * returns to it. The session belongs to one viewer on one server: a profile switch or sign-out
 * starts a fresh one. Recent queries (WEB-SEARCH-02) are private to the same viewer and
 * clearable.
 */
type Kept = {key: string; api: unknown; service: SearchService; text: string; chip: ChipId; recent: string[]};
let kept: Kept | null = null;
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
  const {api} = useSession();
  const libraries = useLibrariesContext();
  const scope = useViewerScope();
  const mem = useSearchSession(api, scope);
  const service = mem.service;
  const snapshot: SearchSnapshot = useSyncExternalStore(service.subscribe, service.getSnapshot);
  const address = useSearch({from: '/app/search'});
  const navigate = useNavigate();
  // A linked search (`?q=`) wins over the remembered one.
  const linked = useRef(address.q !== undefined && address.q !== mem.text);
  if (linked.current) { linked.current = false; mem.text = address.q ?? ''; mem.chip = (address.type as ChipId | undefined) ?? 'all'; }
  const [text, setTextState] = useState(mem.text);
  const [chip, setChipState] = useState<ChipId>(mem.chip);
  const [recent, setRecent] = useState<string[]>(mem.recent);
  const setText = (value: string) => { mem.text = value; setTextState(value); };
  const setChip = (value: ChipId) => { mem.chip = value; setChipState(value); };
  // The address follows what was searched (replaced, so typing does not fill history).
  useEffect(() => {
    const q = text.trim();
    const timer = setTimeout(() => void navigate({to: '/search', search: {...(q ? {q} : {}), ...(q && chip !== 'all' ? {type: chip} : {})}, replace: true}), 400);
    return () => clearTimeout(timer);
  }, [text, chip, navigate]);
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
    if (restored.current) { restored.current = false; return; }
    timer.current = setTimeout(() => submit(text, group), DEBOUNCE_MS);
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [text]);
  const firstGroup = useRef(true);
  useEffect(() => {
    if (firstGroup.current) { firstGroup.current = false; return; }
    if (text.trim()) submit(text, group);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [group]);
  useEffect(() => { if (!mem.text) input.current?.focus(); }, [mem]);
  /* A query that found something becomes a recent search. */
  const settled = snapshot.phase === 'ready' ? snapshot.query?.q ?? '' : '';
  useEffect(() => {
    const q = settled.trim();
    if (!q || !snapshot.groups.some(g => g.items.length)) return;
    const next = [q, ...mem.recent.filter(r => r.toLowerCase() !== q.toLowerCase())].slice(0, RECENT_LIMIT);
    mem.recent = next;
    setRecent(next);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [settled]);
  const clearRecent = () => { mem.recent = []; setRecent([]); };
  /* ArrowDown from the field moves into the first result. */
  const intoResults = () => (results.current?.querySelector<HTMLElement>('button, a[href]'))?.focus();
  /* Per-group totals come with every response; a chip shows its count so the viewer knows where the hits are. */
  const counts = useMemo(() => new Map(snapshot.groups.map(g => [g.id, g.totalCount] as const)), [snapshot.groups]);
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
            <input ref={input} className={s.input} type="search" value={text} onChange={e => setText(e.target.value)} placeholder={t('web.search.placeholder')} aria-label={t('web.search.title')} autoCapitalize="off" autoCorrect="off" spellCheck={false} enterKeyHint="search" onKeyDown={e => { if (e.key === 'Enter') submit(text, group); else if (e.key === 'ArrowDown') { e.preventDefault(); intoResults(); } }} />
            {text ? <Button variant="ghost" icon="close" aria-label={t('web.search.clear')} size="sm" className={s.clear} onClick={() => { setText(''); input.current?.focus(); }} /> : null}
          </div>
          {problem && hasQuery ? <Text variant="caption" tone="secondary" role="status">{t(problem === 'short' ? 'web.search.tooShort' : problem === 'words' ? 'web.search.tooManyWords' : 'web.search.tooLong')}</Text> : null}
        </Inset>
        <Inset>
          <Chips>
            {chips().map(item => {
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
          </div>
        </Inset>
      ) : null}
      {!hasQuery && !recent.length ? (!libraries.items.length && !libraries.loading ? <NoLibraries where="search" /> : <StateView icon="search" title={t('web.search.emptyTitle')} body={t('web.search.emptyBody')} />) : null}
      {snapshot.phase === 'error' ? <Inset><ErrorNotice error={snapshot.error} context="search" retry={() => void service.retry().catch(() => {})} refresh={() => void service.refresh().catch(() => {})} /></Inset> : null}
      {snapshot.phase === 'refresh-required' ? <Inset><ErrorNotice error={snapshot.error ?? changedError} context="search" refresh={() => void service.refresh().catch(() => {})} /></Inset> : null}
      {snapshot.phase === 'loading' && !snapshot.groups.length ? <LoadingShelves /> : null}
      {failed.length && snapshot.phase === 'ready' ? <Inset><Notice tone="warning" action={{label: t('action.tryAgain'), onClick: () => void service.refresh().catch(() => {})}}>{t('web.search.partial', {groups: new Intl.ListFormat(undefined, {type: 'conjunction'}).format(failed.map(g => t(searchGroupLabel(g.id))))})}</Notice></Inset> : null}
      {snapshot.phase === 'ready' && !sections.length && hasQuery ? <StateView icon="search" title={t('web.search.noMatches', {query})} body={group ? t('web.search.nothingIn', {group: chips().find(c => c.id === group)?.label ?? group}) : t('web.search.noMatchesBody')} action={group ? {label: t('web.search.everything'), onClick: () => setChip('all')} : undefined} /> : null}
      <div ref={results} style={{display: 'contents'}}>
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
  return (
    <Shelf title={t(searchGroupLabel(section.id))} count={section.totalCount} density={densityFor(section)} action={onSeeAll ? {label: t('web.search.seeAll'), onClick: onSeeAll} : undefined}>
      {section.items.map(e => <ResultCard key={e.id} entry={e} />)}
    </Shelf>
  );
}
function GroupResults({section, canPrevious, onNext, onPrevious, busy}: {section: SearchGroupResult; canPrevious: boolean; onNext?: () => void; onPrevious?: () => void; busy: boolean}) {
  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
      <Inset><Row style={{justifyContent: 'space-between'}}><Text variant="heading">{t(searchGroupLabel(section.id))}</Text><Text variant="caption" tone="tertiary">{section.totalCount > section.items.length ? t('web.search.someOf', {shown: section.items.length, total: section.totalCount}) : t('web.search.results', {count: section.totalCount})}</Text></Row></Inset>
      <Grid density={densityFor(section)}>{section.items.map(e => <ResultCard key={e.id} entry={e} />)}</Grid>
      {onNext || canPrevious ? <Inset><Row style={{justifyContent: 'center'}}><Button variant="secondary" icon="back" label={t('web.live.previous')} disabled={!onPrevious || busy} onClick={onPrevious} /><Button variant="secondary" iconAfter="forward" label={t('web.search.next')} disabled={!onNext || busy} loading={busy && !!onNext} onClick={onNext} /></Row></Inset> : null}
    </div>
  );
}

/** One title that matches the words strongly, above the sections: its artwork, what it is, and Play or Open. */
function TopResult({top}: {top: TopSearchResult}) {
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
