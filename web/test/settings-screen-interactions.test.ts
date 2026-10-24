/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the Settings screen's rail, search and pages behave as drawn.
 *
 * Settings is core flow 7. The rail lists the authored pages (Account, and Server for an owner),
 * marks the one on screen, the search field finds pages and rows and clears after a match, an
 * address selects its page (including addresses from before the one screen), and a narrow screen
 * shows the list or a page with a way back. The registry, session and downloads state are stubbed.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, textOf, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);

async function mount(options: {owner?: boolean; narrow?: boolean; address?: string} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const server = {snapshot: null, error: null, loading: false, value: (_key: string, fallback: any) => fallback, reload: () => {}, set: () => Promise.resolve()};
  const app = await componentModule(new URL('../src/screens/settings/Settings.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {
      Link: 'Link',
      useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); },
      useLocation: (options?: any) => (options?.select ? options.select({hash: ''}) : {hash: ''}),
      useParams: () => ({section: options.address ?? ''}),
      useSearch: () => ({}),
    },
    '../../app/session': {useSession: () => ({owner: !!options.owner, administrator: false, session: {viewer: {authority: 'hosted'}}, hosted: null, local: null})},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/server-preferences': {useServerPreferences: () => server},
    '../../app/downloads': {useDownloads: () => ({state: {unavailable: true}})},
    '../../app/errors': {errorText: () => 'failed'},
    '../../ui': {Icon: 'Icon', Input: 'Input', Loading: 'Loading', Notice: 'Notice', Page: 'Page', SettingsGroup: 'SettingsGroup', useNarrow: () => !!options.narrow},
    './SettingsRows': {RowView: 'RowView'},
    './AccountPanels': {AccountPanel: 'AccountPanel', isGroupPanel: () => false},
    './Settings.module.css': {default: {}},
  });
  const draw = () => (options.address === undefined ? app.SettingsScreen() : app.SettingsSectionScreen());
  h.render(draw);
  const flat = () => tree(h.render(draw));
  return {h, flat, navigated};
}

const nav = (rendered: any[]) => one(rendered, 'nav');
const links = (rendered: any[]) => all(rendered, 'Link');
const railLinks = (rendered: any[]) => links(rendered).filter(link => link.props.to === '/settings/$section' && !link.props.hash);

test('the rail lists the authored pages and marks the one on screen', async () => {
  const screen = await mount();
  const rendered = screen.flat();
  assert.equal(nav(rendered).props['aria-label'], t('settings.pages'));
  assert.ok(textOf(rendered).includes(t('settings.heading.account')));
  assert.ok(!textOf(rendered).includes(t('settings.heading.server')), 'no Server heading for a member');
  const pages = railLinks(rendered);
  assert.ok(pages.length >= 2);
  assert.equal(pages[0].props.params.section, 'profile', 'the first page is the fallback');
  assert.equal(pages.filter(link => link.props['aria-current'] === 'page').length, 1);
  assert.equal(pages[0].props['aria-current'], 'page');
  // The pane draws the page: its title is the one the rail marks.
  assert.ok(textOf(rendered).includes(t('settings.page.profile')));
});

test('an owner sees the Server pages in the rail beside Account', async () => {
  const screen = await mount({owner: true});
  const rendered = screen.flat();
  assert.ok(textOf(rendered).includes(t('settings.heading.server')));
  const serverPages = railLinks(rendered).filter(link => String(link.props.params.section).startsWith('server-'));
  assert.ok(serverPages.length > 0, 'server pages are addressed as server-<id>');
  assert.equal(railLinks(rendered).filter(link => link.props['aria-current'] === 'page').length, 1, 'still one page on screen');
});

test('the search field finds pages and rows, clears after a match, and says when nothing matches', async () => {
  const screen = await mount();
  const field = () => one(screen.flat(), 'Input');
  const insideNav = (rendered: any[]) => all(tree(nav(rendered)), 'Link');
  assert.equal(field().props.value, '');
  field().props.onChange({target: {value: 'play'}});
  let rendered = screen.flat();
  // The rail is replaced by the matches while a search is being typed.
  const matches = insideNav(rendered);
  assert.ok(matches.some(link => link.props.params.section === 'playback'), 'the Playback page is a match');
  // A match opens its page and puts the field back to empty, so the rail returns.
  matches.find(link => link.props.params.section === 'playback').props.onClick();
  rendered = screen.flat();
  assert.equal(one(rendered, 'Input').props.value, '');
  assert.ok(insideNav(rendered).length >= 2, 'the rail is back');
  // Nothing matches: the field says so with the words that were typed.
  field().props.onChange({target: {value: 'zzzz'}});
  rendered = screen.flat();
  assert.equal(insideNav(rendered).length, 0);
  assert.ok(textOf(rendered).includes(t('settings.search.none', {query: 'zzzz'})));
});

test('an address selects its page, a former address lands on the new one, and a narrow screen has a way back', async () => {
  // Wide: the rail marks the page the address names, and an old address lands on the new page.
  let screen = await mount({address: 'preferences'});
  let rendered = screen.flat();
  const current = railLinks(rendered).find(link => link.props['aria-current'] === 'page')!;
  assert.equal(current.props.params.section, 'playback', 'preferences is the old name for playback');
  assert.ok(textOf(rendered).includes(t('settings.page.playback')));
  // Narrow: the page pushes over the list and carries a Back to Settings.
  screen = await mount({address: 'preferences', narrow: true});
  rendered = screen.flat();
  assert.ok(textOf(rendered).includes(t('settings.page.playback')));
  assert.ok(links(rendered).some(link => link.props.to === '/settings'), 'a Back link on the pushed page');
  // Narrow with no page chosen: the list alone, with no Back to itself.
  screen = await mount({narrow: true});
  rendered = screen.flat();
  assert.equal(links(rendered).filter(link => link.props.to === '/settings').length, 0);
  assert.ok(textOf(rendered).includes(t('settings.title')));
});
