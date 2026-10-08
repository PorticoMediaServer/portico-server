import {cardCaptionOf} from '../../app/content';
import type {MenuAnchor} from '../../ui';
import {useCallback, useEffect, useMemo, useState, useSyncExternalStore} from 'react';
import {useParams} from '@tanstack/react-router';
import type {ContentEntry} from '@core/library-content.ts';
import {readPerson, type PersonCredit, type PersonPage, type PersonRole} from '@core/people.ts';
import type {CollectionStatus, WindowedCollection} from '@core/collections/index.ts';
import {createWindowedCollection} from '@core/collections/index.ts';
import {PERSON_CREDIT_PAGE, PERSON_CREDIT_RESIDENT_PAGES, personCreditsSource} from './person-credits';
import {captionFor, iconFor, progressFor, shapeFor} from '@core/presentation/index.ts';
import {useEntryHref, useOpenEntry} from '../../app/open';
import {useSession} from '../../app/session';
import {defaultI18n} from '@i18n';
import {Artwork, Button, Card, Chip, Chips, Grid, Inset, Page, PageHeader, Row, Shelf, Skeleton, StateView, Text, WindowedGrid, gridColumnMin} from '../../ui';

const t = defaultI18n.t;
/** Life dates in words ("March 4, 1956"), falling back to what the server sent. */
function lifeDate(value: string): string {
  const ms = Date.parse(value);
  return Number.isFinite(ms) && /^\d{4}-\d{2}-\d{2}/.test(value) ? new Date(ms).toLocaleDateString(undefined, {year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC'}) : value;
}
import {ErrorNotice, ErrorState} from '../../app/errors';
import {useViewerScope} from '../../app/viewer-scope';

type State = {phase: 'loading' | 'ready' | 'error'; page?: PersonPage; error?: unknown};

/**
 * A person: portrait, life dates, biography, "known for" and every credit on
 * this server. The server resolves the identity and pages the credits; the
 * role filter is a request parameter, never a client-side split. Credits show
 * through the windowed grid, so a prolific filmography never mounts more than
 * ~300 cards (PERF-S17).
 */
export function PersonScreen() {
  const {personId} = useParams({from: '/app/person/$personId'});
  const {api} = useSession();
  const scope = useViewerScope();
  const [role, setRole] = useState<PersonRole>('all');
  const [state, setState] = useState<State>({phase: 'loading'});
  const load = useCallback(async () => {
    setState(prev => ({phase: 'loading', page: prev.page}));
    try {
      // The header only: one credit keeps the request small; the grid pages the rest.
      const page = await readPerson(api, scope, personId, {role, limit: 1});
      setState({phase: 'ready', page});
    } catch (e) {
      setState(prev => ({phase: 'error', page: prev.page, error: e}));
    }
  }, [api, scope, personId, role]);
  useEffect(() => { void load(); }, [load]);
  const credits = useMemo<WindowedCollection<PersonCredit>>(
    () => createWindowedCollection<PersonCredit>({fetchPage: personCreditsSource(api, scope, personId, role), pageSize: PERSON_CREDIT_PAGE, maxResidentPages: PERSON_CREDIT_RESIDENT_PAGES, keyOf: c => `${c.media.id}:${c.role}:${c.character}`}),
    [api, scope, personId, role],
  );
  useEffect(() => () => credits.dispose(), [credits]);
  const creditsSnapshot = useSyncExternalStore(credits.subscribe, credits.getSnapshot);
  const open = useOpenEntry();
  const person = state.page?.person;
  const dates = person ? (person.deathDate ? t('web.person.lived', {born: lifeDate(person.birthDate) || '?', died: lifeDate(person.deathDate)}) : person.birthDate ? t('web.person.born', {date: lifeDate(person.birthDate)}) : '') : '';
  const total = state.page?.pageInfo.total ?? 0;
  // WEB-PERSON-01: "Known for" only when it adds something the credits below
  // don't already show. With a moving window the loaded set is unstable, so the
  // rule is static: the shelf shows whenever more credits exist than it holds.
  const knownFor = person && role === 'all' && total > person.knownFor.length ? person.knownFor : [];
  return (
    <Page>
      <PageHeader onBack={() => window.history.back()} title={person?.name ?? (state.phase === 'loading' ? <Skeleton style={{width: 220, height: 32}} /> : t('web.person.fallback'))} eyebrow={person?.roles.join(' · ')} subtitle={dates || undefined} />
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
      <Inset><Chips>{(['all', 'cast', 'crew'] as const).map(r => <Chip key={r} label={r === 'all' ? t('web.search.all') : r === 'cast' ? t('web.person.cast') : t('web.person.crew')} pressed={role === r} onClick={() => setRole(r)} />)}</Chips></Inset>
      {state.phase === 'error' && person ? <Inset><ErrorNotice error={state.error} context="person" retry={() => void load()} /></Inset> : null}
      {state.phase === 'loading' && !person ? <Grid density="poster">{Array.from({length: 12}).map((_, i) => <Skeleton key={i} style={{aspectRatio: '2 / 3', width: '100%'}} />)}</Grid>
      : state.phase === 'ready' && total === 0 ? <StateView inline icon="people" title={t('web.person.noCredits')} body={role === 'all' ? t('web.person.noCreditsBody') : role === 'cast' ? t('web.person.noCast') : t('web.person.noCrew')} />
      : <CreditsGrid collection={credits} status={creditsSnapshot.status} error={creditsSnapshot.error} retry={() => credits.retry()} />}
    </Page>
  );
}

function CreditsGrid({collection, status, error, retry}: {collection: WindowedCollection<PersonCredit>; status: CollectionStatus; error: unknown; retry: () => void}) {
  const open = useOpenEntry();
  return (
    <>
      {status === 'error' ? <Inset><ErrorNotice error={error} context="person" retry={retry} /></Inset> : null}
      <WindowedGrid
        collection={collection}
        columnMin={gridColumnMin('poster')}
        estimateRowHeight={300}
        label={t('web.person.credits')}
        renderItem={c => <EntryCard key={`${c.media.id}:${c.role}:${c.character}`} entry={c.media} caption={c.creditKind === 'cast' ? (c.character ? t('web.person.as', {character: c.character}) : c.role) : c.role || c.department} onOpen={() => open(c.media)} onPlay={c.media.playback ? () => open(c.media, 'play') : undefined} onMore={anchor => open(c.media, 'more', undefined, anchor)} />}
        renderPlaceholder={() => <div aria-hidden><Skeleton style={{aspectRatio: '2 / 3', width: '100%'}} /><Skeleton height={14} width="70%" style={{marginTop: 8}} /></div>}
      />
    </>
  );
}

function Biography({text}: {text: string}) {
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
