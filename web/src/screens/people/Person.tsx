import {cardCaptionOf} from '../../app/content';
import {useBack} from '../../app/back';
import type {MenuAnchor} from '../../ui';
import {useCallback, useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {useNavigate, useParams, useSearch} from '@tanstack/react-router';
import type {ContentEntry} from '@core/library-content.ts';
import {readPerson, type PersonCredit, type PersonPage, type PersonRole} from '@core/people.ts';
import type {CollectionStatus, WindowedCollection} from '@core/collections/index.ts';
import {createWindowedCollection} from '@core/collections/index.ts';
import {PERSON_CREDIT_PAGE, PERSON_CREDIT_RESIDENT_PAGES, personCreditsSource} from './person-credits';
import {personCredits, rememberedPerson} from './person-memory';
import {captionFor, iconFor, progressFor, shapeFor} from '@core/presentation/index.ts';
import {useEntryHref, useOpenEntry} from '../../app/open';
import {useSession} from '../../app/session';
import {Artwork, Button, Card, Chip, Chips, Grid, Inset, Page, PageHeader, Row, Shelf, Skeleton, StateView, Text, WindowedGrid, gridColumnMin} from '../../ui';

/** Life dates in words ("March 4, 1956"), falling back to what the server sent. */
function lifeDate(i18n: I18n, value: string): string {
  const ms = Date.parse(value);
  // A calendar date, not a moment: read in UTC so it is the same day in every zone.
  return Number.isFinite(ms) && /^\d{4}-\d{2}-\d{2}/.test(value) ? i18n.formatDate(ms, {year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC'}) : value;
}
import {ErrorNotice, ErrorState} from '../../app/errors';
import {useViewerScope} from '../../app/viewer-scope';
import {useFreshDocument} from '../../app/freshness';
import {titlesWithPersonalState} from '@core/freshness-hub.ts';
import {useI18n, type I18n} from '../../app/i18n';
import type {BrowseNode} from '@core/browse.ts';
import {spanningGroup, titlePivot, useGroupBrowse, useGroupLibraries, type GroupChoice} from '../../app/group-browse';
import {GroupBar, GroupGrid, GroupLibraryChips} from '../shared/GroupBrowse';
import {LoadingGrid} from '../shared/Sections';

type State = {phase: 'loading' | 'ready' | 'error'; page?: PersonPage; error?: unknown};
/** People are credited on films and shows: the libraries a person's titles can be sorted and filtered in. */
const creditPivot = (libraryKind: string) => (libraryKind === 'movie' || libraryKind === 'tv' || libraryKind === 'anime' ? titlePivot(libraryKind) : undefined);

/**
 * A person: portrait, life dates, biography, "known for" and every credit on
 * this server. The server resolves the identity and pages the credits; the
 * role filter is a request parameter, never a client-side split. Credits show
 * through the windowed grid, so a prolific filmography never mounts more than
 * ~300 cards (PERF-S17).
 */
const personDependencies = Object.freeze([...titlesWithPersonalState()]);

export function PersonScreen() {
  const i18n = useI18n();
  const {t} = i18n;
  const {personId} = useParams({from: '/app/person/$personId'});
  const {api} = useSession();
  const scope = useViewerScope();
  const address = useSearch({from: '/app/person/$personId'});
  const navigate = useNavigate();
  // Back to a person shows what was there: kept for the last few people visited (person-memory).
  const kept = useMemo(() => rememberedPerson(api, scope, personId), [api, scope, personId]);
  const [role, setRoleState] = useState<PersonRole>(kept.role);
  const setRole = (next: PersonRole) => { kept.role = next; setRoleState(next); };
  const [state, setState] = useState<State>(() => (kept.page ? {phase: 'ready', page: kept.page} : {phase: 'loading'}));
  const load = useCallback(async () => {
    setState(prev => ({phase: 'loading', page: prev.page}));
    try {
      // The header only: one credit keeps the request small; the grid pages the rest.
      const page = await readPerson(api, scope, personId, {role, limit: 1});
      if (role === kept.role) kept.page = page;
      setState({phase: 'ready', page});
    } catch (e) {
      setState(prev => ({phase: 'error', page: prev.page, error: e}));
    }
  }, [api, scope, personId, role, kept]);
  useEffect(() => { void load(); }, [load]);
  // The grid is the kept one (the memory releases it when the person is pushed out).
  const credits = useMemo<WindowedCollection<PersonCredit>>(
    () => personCredits(kept, role, () => createWindowedCollection<PersonCredit>({fetchPage: personCreditsSource(api, scope, personId, role), pageSize: PERSON_CREDIT_PAGE, maxResidentPages: PERSON_CREDIT_RESIDENT_PAGES, keyOf: c => `${c.media.id}:${c.role}:${c.character}`})),
    [api, scope, personId, role, kept],
  );
  // A person's titles follow the libraries while the page is open: a title that arrives, goes or is watched updates in place.
  useFreshDocument(personDependencies, context => credits.revalidate({fetchPage: personCreditsSource(api, scope, personId, role), hold: context.settle}), () => credits.getSnapshot().total !== undefined);
  const creditsSnapshot = useSyncExternalStore(credits.subscribe, credits.getSnapshot);
  // The library's sort and filters on this person's titles. A person's credits span libraries and a
  // library's vocabulary is its own, so with titles in several the viewer picks one. The server's
  // person condition covers every credit, so the toolbar is offered under All, not under Cast or Crew.
  const base = useMemo<readonly BrowseNode[]>(() => [{field: 'person', operator: 'equals', value: personId}], [personId]);
  const held = useGroupLibraries(role === 'all' ? base : undefined, creditPivot);
  const scoped = useMemo(() => spanningGroup(held.libraries, address.library, base, 'default'), [held.libraries, address.library, base]);
  const choose = useCallback((next: {library?: string} & GroupChoice) => void navigate({to: '/person/$personId', params: {personId}, search: next, replace: true, resetScroll: false}), [navigate, personId]);
  const group = useGroupBrowse(scoped.spec, {sort: address.sort, direction: address.direction, filters: address.filters}, useCallback((next: GroupChoice) => choose({library: scoped.chosen, ...next}), [choose, scoped.chosen]));
  const chooseRole = (next: PersonRole) => { setRole(next); if (next !== 'all' && (address.library || address.sort || address.filters)) choose({}); };
  const open = useOpenEntry();
  const homeward = useNavigate();
  const back = useBack(() => void homeward({to: '/'}));
  const person = state.page?.person;
  const dates = person ? (person.deathDate ? t('web.person.lived', {born: lifeDate(i18n, person.birthDate) || '?', died: lifeDate(i18n, person.deathDate)}) : person.birthDate ? t('web.person.born', {date: lifeDate(i18n, person.birthDate)}) : '') : '';
  const total = state.page?.pageInfo.total ?? 0;
  // WEB-PERSON-01: "Known for" only when it adds something the credits below
  // don't already show. With a moving window the loaded set is unstable, so the
  // rule is static: the shelf shows whenever more credits exist than it holds.
  const knownFor = person && role === 'all' && total > person.knownFor.length ? person.knownFor : [];
  return (
    <Page>
      <PageHeader onBack={back} pageKey={person?.id} title={person?.name ?? (state.phase === 'loading' ? <Skeleton style={{width: 220, height: 32}} /> : t('web.person.fallback'))} eyebrow={person?.roles.join(' · ')} subtitle={dates || undefined} />
      {state.phase === 'error' && !person ? <ErrorState error={state.error} context="person" retry={() => void load()} /> : null}
      {person ? (
        <Inset>
          <Row style={{alignItems: 'flex-start', gap: 24}}>
            <div style={{width: 140, flexShrink: 0}}><Artwork path={person.portraitUrl || undefined} shape="circle" alt={person.name} initial={person.name[0]} icon="person" /></div>
            <div style={{flex: 1, minWidth: 0}}>
              {person.biography ? <Biography text={person.biography} /> : <Text variant="body" tone="secondary">{t('web.person.noBio')}</Text>}
            </div>
          </Row>
        </Inset>
      ) : null}
      {knownFor.length ? <Shelf title={t('web.person.knownFor')} density="poster">{knownFor.map(e => <EntryCard key={e.id} entry={e} onOpen={() => open(e)} onPlay={e.playback ? () => open(e, 'play') : undefined} onMore={anchor => open(e, 'more', undefined, anchor)} />)}</Shelf> : null}
      <Inset>
        <Row style={{justifyContent: 'space-between', alignItems: 'baseline'}}>
          <Text variant="heading">{t('web.person.credits')}</Text>
          {state.page && total > 0 ? <Text variant="caption" tone="tertiary">{t('web.person.titles', {count: state.page.pageInfo.total})}</Text> : null}
        </Row>
      </Inset>
      <Inset><Chips>{(['all', 'cast', 'crew'] as const).map(r => <Chip key={r} label={r === 'all' ? t('web.search.all') : r === 'cast' ? t('web.person.cast') : t('web.person.crew')} pressed={role === r} onClick={() => chooseRole(r)} />)}</Chips></Inset>
      {role === 'all' && scoped.choose ? <Inset><GroupLibraryChips libraries={held.libraries} value={scoped.chosen} onChange={library => choose(library ? {library} : {})} /></Inset> : null}
      {role === 'all' && scoped.spec ? <Inset><GroupBar group={group} /></Inset> : null}
      {state.phase === 'error' && person ? <Inset><ErrorNotice error={state.error} context="person" retry={() => void load()} /></Inset> : null}
      {state.phase === 'loading' && !person ? <Grid density="poster">{Array.from({length: 12}).map((_, i) => <Skeleton key={i} style={{aspectRatio: '2 / 3', width: '100%'}} />)}</Grid>
      : state.phase === 'ready' && total === 0 ? <StateView inline icon="people" title={t('web.person.noCredits')} body={role === 'all' ? t('web.person.noCreditsBody') : role === 'cast' ? t('web.person.noCast') : t('web.person.noCrew')} />
      : role === 'all' && group.pending ? <LoadingGrid />
      : role === 'all' && group.active ? <GroupGrid group={group} square={false} />
      : <CreditsGrid collection={credits} status={creditsSnapshot.status} error={creditsSnapshot.error} retry={() => credits.retry()} />}
    </Page>
  );
}

const creditPlaceholder = () => <div aria-hidden><Skeleton style={{aspectRatio: '2 / 3', width: '100%'}} /><Skeleton height={14} width="70%" style={{marginTop: 8}} /></div>;

function CreditsGrid({collection, status, error, retry}: {collection: WindowedCollection<PersonCredit>; status: CollectionStatus; error: unknown; retry: () => void}) {
  const {t} = useI18n();
  const open = useOpenEntry();
  // Stable renderers: each page of credits is memoised and redraws only when its own page changes.
  const renderCredit = useCallback((c: PersonCredit) => <EntryCard key={`${c.media.id}:${c.role}:${c.character}`} entry={c.media} caption={c.creditKind === 'cast' ? (c.character ? t('web.person.as', {character: c.character}) : c.role) : c.role || c.department} onOpen={() => open(c.media)} onPlay={c.media.playback ? () => open(c.media, 'play') : undefined} onMore={anchor => open(c.media, 'more', undefined, anchor)} />, [t, open]);
  return (
    <>
      {status === 'error' ? <Inset><ErrorNotice error={error} context="person" retry={retry} /></Inset> : null}
      <WindowedGrid
        collection={collection}
        columnMin={gridColumnMin('poster')}
        estimateRowHeight={300}
        label={t('web.person.credits')}
        renderItem={renderCredit}
        renderPlaceholder={creditPlaceholder}
      />
    </>
  );
}

function Biography({text}: {text: string}) {
  const {t} = useI18n();
  const [expanded, setExpanded] = useState(false);
  const long = text.length > 480;
  return (
    <div>
      <Text variant="body" tone="secondary" style={long && !expanded ? {display: '-webkit-box', WebkitLineClamp: 6, WebkitBoxOrient: 'vertical', overflow: 'hidden'} : undefined}>{text}</Text>
      {long ? <Button variant="ghost" size="sm" label={expanded ? t('web.person.showLess') : t('web.person.readMore')} onClick={() => setExpanded(v => !v)} /> : null}
    </div>
  );
}

function EntryCard({entry, caption, onOpen, onPlay, onMore}: {entry: ContentEntry; caption?: string; onOpen: () => void; onPlay?: () => void; onMore: (anchor: MenuAnchor) => void}) {
  const hrefOf = useEntryHref();
  return <Card href={hrefOf(entry)} title={entry.title} caption={caption ?? cardCaptionOf(entry)} path={entry.posterUrl ?? entry.backdropUrl} shape={shapeFor(entry.kind)} icon={iconFor(entry.kind)} progress={progressFor(entry)} watched={entry.watched} onOpen={onOpen} onPlay={onPlay} onMore={onMore} />;
}
