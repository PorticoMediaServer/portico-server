import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {enUS} from '@i18n';
import {componentModule} from './helpers/component-harness.mjs';

const src = (path: string) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const pageTitle = () => componentModule(new URL('../src/app/page-title.tsx', import.meta.url), {react: {default: {}}}) as Promise<typeof import('../src/app/page-title.tsx')>;
/** A document whose main region holds this one element (or nothing). */
const page = (found?: {attribute?: string; text?: string; provisional?: boolean}) => ({
  querySelector: (selector: string) => {
    assert.equal(selector, 'main [data-page-title], main h1', 'only the main region names the page: never the player or a dialog');
    return found ? {getAttribute: (name: string) => (name === 'data-page-title' ? found.attribute ?? null : name === 'data-provisional' && found.provisional ? 'true' : null), textContent: found.text ?? ''} : null;
  },
});

test('a page is named by its heading, or by the name it gives itself', async () => {
  const {pageName} = await pageTitle();
  const name = (found?: Parameters<typeof page>[0]) => pageName(page(found))?.name ?? '';
  assert.equal(name({text: 'Settings'}), 'Settings');
  assert.equal(name({text: '  The Lord of the Rings:\n   The Two Towers '}), 'The Lord of the Rings: The Two Towers');
  assert.equal(name({attribute: 'Home', text: 'ignored'}), 'Home', 'Home’s heading is the featured title, so Home names itself');
  assert.equal(name({text: ' '}), '', 'a heading still loading names nothing yet');
  assert.equal(name(), '');
  assert.equal(name({text: 'x'.repeat(500)}).length, 200);
  assert.equal(name({text: 'Library', provisional: true}), '', 'a stand-in heading shown before the real name is known is not the page’s name');
  assert.match(src('screens/home/Home.tsx'), /data-page-title=\{currentI18n\(\)\.t\('web\.shell\.home'\)\}/);
  assert.match(src('screens/library/Library.tsx'), /provisional=\{!library\?\.name && !projection\?\.heading\.fallback\}/, 'the library page marks its stand-in title');
  assert.match(src('ui/Page.tsx'), /<h1 className=\{s\.title\} data-provisional=\{provisional \|\| undefined\}>/);
});

test('a destination is announced once, after it has a name of its own, and never on first load or for an unchanged address', async () => {
  const {pageTitleMoved, pageTitleRead} = await pageTitle();
  const home = {element: {}, name: 'Home'}, settings = {element: {}, name: 'Settings'};
  // First load: the browser and screen reader already read the document title.
  let step = pageTitleRead(pageTitleMoved(undefined, 'v', '/', undefined), home);
  assert.equal(step.announce, '');
  // A move: nothing while the old heading is still up or the new page is loading, then its name once.
  step = pageTitleRead(pageTitleMoved(step.state, 'v', '/settings', home), home);
  assert.equal(step.announce, '');
  step = pageTitleRead(step.state, undefined);
  assert.equal(step.announce, '');
  step = pageTitleRead(step.state, settings);
  assert.equal(step.announce, 'Settings');
  // Background refresh, a renamed item, a changed tab in the query: the same address is not repeated.
  step = pageTitleRead(step.state, settings);
  assert.equal(step.announce, '');
  step = pageTitleRead(step.state, {...settings, name: 'Preferences'});
  assert.equal(step.announce, '');
  // A page whose heading is already its own when the address changes is announced at the first look.
  step = pageTitleRead(pageTitleMoved(step.state, 'v', '/', home), home);
  assert.equal(step.announce, 'Home');
});

test('what the last destination showed is never announced as the new one', async () => {
  const {pageTitleMoved, pageTitleRead} = await pageTitle();
  const hero = {};
  const dune = {element: hero, name: 'Dune'}, arrival = {element: hero, name: 'Arrival'};
  let step = pageTitleRead(pageTitleMoved(undefined, 'v', '/media/a', dune), dune);
  // The address moves to another film; the page keeps the first film’s title up while it loads.
  step = pageTitleRead(pageTitleMoved(step.state, 'v', '/media/b', dune), dune);
  assert.equal(step.announce, '', 'still the previous film’s heading');
  step = pageTitleRead(step.state, dune);
  assert.equal(step.announce, '', 'for as long as it stays');
  step = pageTitleRead(step.state, arrival);
  assert.equal(step.announce, 'Arrival', 'the new film’s own name, once it is there');
});

test('a change of viewer announces nothing and never passes on what the previous viewer was shown', async () => {
  const {pageTitleMoved, pageTitleRead} = await pageTitle();
  const a = {element: {}, name: 'A film the first profile opened'};
  // The first profile has just opened a film (not announced yet) when the profile changes on the same address.
  let state = pageTitleMoved(pageTitleMoved(undefined, 'one', '/', undefined), 'one', '/media/a', a);
  assert.equal(state.pending, true);
  let step = pageTitleRead(pageTitleMoved(state, 'two', '/media/a', a), a);
  assert.equal(step.announce, '');
  assert.equal(pageTitleRead(step.state, {element: {}, name: 'Not available on this profile'}).announce, '', 'the same address is not a navigation');
  // A profile change that also lands somewhere else: what is on screen at that moment belonged to the first profile.
  state = pageTitleMoved(pageTitleMoved(undefined, 'one', '/media/a', a), 'two', '/', a);
  step = pageTitleRead(state, a);
  assert.equal(step.announce, '');
  step = pageTitleRead(step.state, {element: {}, name: 'Home'});
  assert.equal(step.announce, 'Home', 'the second profile’s own page is announced');
});

/** Mounts the component with just enough of React, the router, the document and the clock to run its effect. */
async function mounted(t: import('node:test').TestContext) {
  t.mock.timers.enable({apis: ['setTimeout']});
  const world = {pathname: '/', scope: {viewerId: 'viewer-1', serverId: 'srv'}, heading: undefined as {attribute?: string; text?: string; provisional?: boolean; element?: object} | undefined, title: 'Portico'};
  const observers: (() => void)[] = [];
  const fakeDocument = {
    body: {}, get title() { return world.title; }, set title(v: string) { world.title = v; },
    querySelector: () => (world.heading ? Object.assign(world.heading.element ??= {}, {getAttribute: (name: string) => (name === 'data-page-title' ? world.heading!.attribute ?? null : name === 'data-provisional' && world.heading!.provisional ? 'true' : null), textContent: world.heading.text ?? ''}) : null),
  };
  const had = ['document', 'MutationObserver'].map(name => [name, Object.getOwnPropertyDescriptor(globalThis, name)] as const);
  Object.defineProperty(globalThis, 'document', {configurable: true, value: fakeDocument});
  Object.defineProperty(globalThis, 'MutationObserver', {configurable: true, value: class { fn: () => void; constructor(fn: () => void) { this.fn = fn; } observe() { observers.push(this.fn); } disconnect() { const i = observers.indexOf(this.fn); if (i >= 0) observers.splice(i, 1); } }});
  t.after(() => { for (const [name, descriptor] of had) { if (descriptor) Object.defineProperty(globalThis, name, descriptor); else delete (globalThis as Record<string, unknown>)[name]; } });
  const slots: any[] = []; let cursor = 0, effect: {deps: unknown[]; cleanup?: () => void} | undefined;
  const react = {
    createElement: (type: unknown, props: object, ...children: unknown[]) => ({type, props: {...props, children}}),
    useRef: (v: unknown) => (slots[cursor++] ??= {current: v}),
    useState: (v: unknown) => { const n = cursor++; if (!(n in slots)) slots[n] = v; return [slots[n], (next: unknown) => { slots[n] = next; }]; },
    useEffect: (run: () => (() => void) | void, deps: unknown[]) => { if (effect && deps.every((d, i) => d === effect!.deps[i])) return; effect?.cleanup?.(); effect = {deps, cleanup: run() || undefined}; },
  };
  const module = await componentModule(new URL('../src/app/page-title.tsx', import.meta.url), {
    react: {default: react, ...react},
    '@tanstack/react-router': {useRouterState: ({select}: {select: (s: unknown) => unknown}) => select({location: {pathname: world.pathname}})},
    './i18n': {currentI18n: () => ({t: (_id: string, v: {page: string; product: string}) => v.page + ' · ' + v.product})},
    './viewer-scope': {useViewerScope: () => world.scope},
  }) as typeof import('../src/app/page-title.tsx');
  const render = () => { cursor = 0; return (module.PageTitle as unknown as () => {props: {children: unknown[]}})().props.children[0] as string; };
  /** Something on the page changed, and `ms` passed. Returns what the live region says now. */
  const after = (ms: number) => { render(); for (const notify of [...observers]) notify(); t.mock.timers.tick(ms); return render(); };
  return {world, render, after};
}

test('the announcement waits for the destination’s own name, however late it arrives', async t => {
  const {world, after} = await mounted(t);
  world.heading = {attribute: 'Home', text: 'Featured film'};
  assert.equal(after(300), '', 'the first page is not announced');
  assert.equal(world.title, 'Home · Portico');
  // To a library whose name is not known yet: the old page's heading is still up at first, then a stand-in.
  world.pathname = '/library/lib_1';
  assert.equal(after(300), '', 'Home’s leftover heading is not the library’s name');
  world.heading = {text: 'Library', provisional: true};
  assert.equal(after(300), '');
  assert.equal(world.title, 'Portico', 'nor is a stand-in the tab’s title');
  assert.equal(after(2000), '', 'and no amount of waiting makes it the name');
  world.heading = {text: 'Anime'};
  assert.equal(after(300), 'Anime');
  assert.equal(world.title, 'Anime · Portico');
  world.heading = {text: 'Anime (renamed)', element: world.heading.element};
  assert.equal(after(300), 'Anime', 'a later change on the same page is not announced again');
  assert.equal(world.title, 'Anime (renamed) · Portico');
});

test('a profile change cancels what was about to be announced and never speaks the previous profile’s page', async t => {
  const {world, render, after} = await mounted(t);
  world.heading = {text: 'Home'};
  after(300);
  world.pathname = '/media/a';
  world.heading = {text: 'A film only the first profile can see'};
  render();
  // The read is due in a moment, but the profile changes first, on the same address.
  world.scope = {viewerId: 'viewer-2', serverId: 'srv'};
  assert.equal(after(300), '', 'the first profile’s page is not announced to the second');
  assert.equal(after(2000), '');
  // The second profile then goes somewhere itself.
  world.pathname = '/';
  world.heading = {text: 'Home'};
  assert.equal(after(300), 'Home', 'its own navigation is');
});

test('the browser tab names the page and falls back to the product name', async () => {
  const {documentTitle} = await pageTitle();
  const t = (id: string, values: Record<string, string>) => (enUS as Record<string, string>)[id]!.replace('{page}', values.page!).replace('{product}', values.product!);
  assert.equal(documentTitle('Settings', 'Portico', t), 'Settings · Portico');
  assert.equal(documentTitle('', 'Portico', t), 'Portico');
  assert.match(src('app/router.tsx'), /<Outlet \/>\n\s*<PageTitle \/>/, 'mounted once for every route, signed in or not');
});
