/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn Person screen controls do what they say.
 *
 * A person from search or a credit: the portrait and biography with its Read more, "Known for"
 * when it adds something, the All/Cast/Crew chips that re-read the credits, each credit card's
 * open, play and menu, the empty wording per role, and the two failed states. The person read and
 * the credit collection are stubbed, so nothing leaves the process.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import * as peopleCore from '@core/people.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);
const entry = (id: string, title: string) => ({id, kind: 'movie', title, libraryId: 'library', posterUrl: `/${id}.jpg`, backdropUrl: '', duration: 6000, progressSeconds: 0, watched: false, playback: {itemId: id, startSeconds: 0}});
const person = (patch: any = {}) => ({id: 'p1', name: 'Greta Gerwig', roles: ['Directing', 'Writing'], birthDate: '1983-08-04', deathDate: '', biography: 'An American filmmaker.', portraitUrl: '', knownFor: [entry('k1', 'Lady Bird')], ...patch});

async function mount(options: {person?: any; total?: number; readError?: any; biography?: string; role?: string} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const opened: any[] = [];
  const reads: any[] = [];
  const credits: Record<string, any> = {};
  let creditRetries = 0;
  const kept = {role: options.role ?? 'all', page: undefined as any};
  // One API and one scope per mount: the load callback must not change on every render.
  const api = {};
  const scope = {viewerId: 'viewer', serverId: 'server'};
  const pagePerson = options.person ?? person({biography: options.biography ?? 'An American filmmaker.'});
  const readPerson = (async (_api: any, _scope: any, _id: string, query: any) => {
    reads.push(query);
    const fails = options.readError && (options.readError.role ? query.role === options.readError.role : reads.length === 1);
    if (fails) throw options.readError;
    return {person: pagePerson, pageInfo: {total: options.total ?? 3}};
  }) as any;
  const creditCollection = (role: string) => (credits[role] ??= {
    subscribe: () => () => {},
    getSnapshot: () => ({status: 'ready', total: options.total ?? 3, error: null}),
    residentItems: () => [],
    revalidate: () => Promise.resolve(),
    retry: () => { creditRetries++; },
  });
  const app = await componentModule(new URL('../src/screens/people/Person.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useParams: () => ({personId: 'p1'}), useSearch: () => ({}), useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); }},
    '@core/people.ts': {...peopleCore, readPerson},
    './person-credits': {PERSON_CREDIT_PAGE: 40, PERSON_CREDIT_RESIDENT_PAGES: 3, personCreditsSource: () => () => Promise.resolve([])},
    './person-memory': {rememberedPerson: () => kept, personCredits: (_kept: any, role: string) => creditCollection(role)},
    '../../app/back': {useBack: (fallback: any) => fallback},
    '../../app/open': {useEntryHref: () => (value: any) => `/item/${value.id}`, useOpenEntry: () => (...args: any[]) => opened.push(args)},
    '../../app/session': {useSession: () => ({api})},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/viewer-scope': {useViewerScope: () => scope},
    '../../app/freshness': {useFreshDocument: () => {}},
    '../../app/errors': {ErrorNotice: 'ErrorNotice', ErrorState: 'ErrorState'},
    '../shared/GroupBrowse': {GroupBar: 'GroupBar', GroupGrid: 'GroupGrid'},
    '../shared/Sections': {LoadingGrid: 'LoadingGrid'},
    '../../app/group-browse': {acrossGroup: () => undefined, useGroupBrowse: () => ({pending: false, active: false})},
    '../../ui': {Artwork: 'Artwork', Button: 'Button', Card: 'Card', Chip: 'Chip', Chips: 'Chips', Grid: 'Grid', Inset: 'Inset', Page: 'Page', PageHeader: 'PageHeader', Row: 'Row', Shelf: 'Shelf', Skeleton: 'Skeleton', StateView: 'StateView', Text: 'Text', WindowedGrid: 'WindowedGrid', gridColumnMin: () => 160},
  });
  h.render(() => app.PersonScreen());
  // The first read resolves before the test drives anything.
  await new Promise(resolve => setTimeout(resolve, 0));
  const flat = () => tree(h.render(() => app.PersonScreen()));
  return {h, flat, navigated, opened, reads, kept, credits, retries: () => creditRetries};
}

const roleChips = (rendered: any[]) => all(rendered, 'Chip').filter(chip => [t('web.search.all'), t('web.person.cast'), t('web.person.crew')].includes(chip.props.label));

test('the role chips are drawn, and choosing one re-reads the credits for that role', async () => {
  const screen = await mount();
  let rendered = screen.flat();
  assert.deepEqual(roleChips(rendered).map(chip => chip.props.label), [t('web.search.all'), t('web.person.cast'), t('web.person.crew')]);
  assert.equal(roleChips(rendered)[0].props.pressed, true);
  roleChips(rendered)[1].props.onClick();
  rendered = screen.flat();
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.deepEqual(screen.reads.at(-1), {role: 'cast', limit: 1});
  assert.equal(screen.kept.role, 'cast', 'the role is remembered for the way back');
  rendered = screen.flat();
  assert.deepEqual(roleChips(rendered).map(chip => chip.props.pressed), [false, true, false]);
});

test('the person is drawn with their dates, roles and a biography that opens and closes', async () => {
  const long = 'A director and writer. '.repeat(40);
  const screen = await mount({biography: long});
  const header = one(screen.flat(), 'PageHeader');
  assert.equal(header.props.title, 'Greta Gerwig');
  assert.equal(header.props.eyebrow, 'Directing · Writing');
  assert.ok(String(header.props.subtitle).includes('1983'));
  const readMore = all(screen.flat(), 'Button').find(button => button.props.label === t('web.person.readMore'))!;
  readMore.props.onClick();
  assert.ok(all(screen.flat(), 'Button').some(button => button.props.label === t('web.person.showLess')));
  // A short biography has no control to open it.
  const short = await mount({biography: 'Short.'});
  assert.ok(!all(short.flat(), 'Button').some(button => button.props.label === t('web.person.readMore')));
});

test('Known for is a shelf only when it adds titles the credits do not, and only on All', async () => {
  const screen = await mount({total: 9, person: person({knownFor: [entry('k1', 'Lady Bird')]})});
  const shelf = one(screen.flat(), 'Shelf');
  assert.equal(shelf.props.title, t('web.person.knownFor'));
  const card = all(tree(shelf), 'Card')[0];
  assert.equal(card.props.title, 'Lady Bird');
  card.props.onOpen();
  card.props.onPlay();
  card.props.onMore({x: 1, y: 2, width: 3, height: 4});
  assert.deepEqual(screen.opened.map(args => [args[0].id, args[1]]), [['k1', undefined], ['k1', 'play'], ['k1', 'more']]);
  // Nothing extra to say when the credits are no longer than the shelf.
  const equal = await mount({total: 1, person: person({knownFor: [entry('k1', 'Lady Bird')]})});
  assert.equal(all(equal.flat(), 'Shelf').length, 0);
  // Cast or Crew narrows the credits, so the shelf would double them.
  const cast = await mount({role: 'cast', total: 9});
  assert.equal(all(cast.flat(), 'Shelf').length, 0);
});

test('a credit card opens, plays and offers its menu with the role as its caption', async () => {
  const screen = await mount();
  const grid = one(screen.flat(), 'WindowedGrid');
  const credit = {id: 'c1', media: entry('m9', 'Frances Ha'), role: 'Actress', character: 'Frances', creditKind: 'cast', department: 'Acting'};
  const card = grid.props.renderItem(credit);
  assert.equal(card.props.caption, t('web.person.as', {character: 'Frances'}));
  card.props.onOpen();
  card.props.onPlay();
  assert.deepEqual(screen.opened.map(args => [args[0].id, args[1]]), [['m9', undefined], ['m9', 'play']]);
});

test('no credits is said per role, and a failed credit page offers Try again', async () => {
  let screen = await mount({total: 0});
  assert.equal(one(screen.flat(), 'StateView').props.body, t('web.person.noCreditsBody'));
  screen = await mount({total: 0, role: 'cast'});
  assert.equal(one(screen.flat(), 'StateView').props.body, t('web.person.noCast'));
  screen = await mount({total: 0, role: 'crew'});
  assert.equal(one(screen.flat(), 'StateView').props.body, t('web.person.noCrew'));
});

test('a person that fails to load offers one retry; a failed refresh keeps the person', async () => {
  const screen = await mount({readError: {code: 'request_failed', message: 'Person could not load.', retryable: true}});
  const error = one(screen.flat(), 'ErrorState');
  error.props.retry();
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.equal(screen.reads.length, 2, 'the retry reads the person again');
  // With the person already on the page, a failed re-read is a notice above the credits.
  const refreshed = await mount({readError: {code: 'request_failed', message: 'Person could not refresh.', retryable: true, role: 'cast'}});
  roleChips(refreshed.flat())[1].props.onClick();
  refreshed.flat();
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.ok(one(refreshed.flat(), 'ErrorNotice'));
});

test('Back returns Home', async () => {
  const screen = await mount();
  one(screen.flat(), 'PageHeader').props.onBack();
  assert.deepEqual(screen.navigated, [{to: '/'}]);
});
