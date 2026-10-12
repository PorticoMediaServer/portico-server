import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {enUS} from '@i18n';
import {componentModule} from './helpers/component-harness.mjs';

const src = (path: string) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const pageTitle = () => componentModule(new URL('../src/app/page-title.tsx', import.meta.url), {react: {default: {}}}) as Promise<typeof import('../src/app/page-title.tsx')>;
/** A document whose main region holds this one element (or nothing). */
const page = (found?: {attribute?: string; text?: string}) => ({
  querySelector: (selector: string) => {
    assert.equal(selector, 'main [data-page-title], main h1', 'only the main region names the page: never the player or a dialog');
    return found ? {getAttribute: () => found.attribute ?? null, textContent: found.text ?? ''} : null;
  },
});

test('a page is named by its heading, or by the name it gives itself', async () => {
  const {pageName} = await pageTitle();
  assert.equal(pageName(page({text: 'Settings'})), 'Settings');
  assert.equal(pageName(page({text: '  The Lord of the Rings:\n   The Two Towers '})), 'The Lord of the Rings: The Two Towers');
  assert.equal(pageName(page({attribute: 'Home', text: 'ignored'})), 'Home', 'Home’s heading is the featured title, so Home names itself');
  assert.equal(pageName(page({text: ' '})), '', 'a heading still loading names nothing yet');
  assert.equal(pageName(page()), '');
  assert.equal(pageName(page({text: 'x'.repeat(500)})).length, 200);
  assert.match(src('screens/home/Home.tsx'), /data-page-title=\{currentI18n\(\)\.t\('web\.shell\.home'\)\}/);
});

test('a destination is announced once, after it has a name, and never on first load or for an unchanged address', async () => {
  const {pageTitleStep} = await pageTitle();
  // First load: the browser and screen reader already read the document title.
  let step = pageTitleStep(undefined, '/', '');
  assert.equal(step.announce, false);
  step = pageTitleStep(step.state, '/', 'Home');
  assert.equal(step.announce, false);
  // A move: nothing while the new page is still loading, then its name once.
  step = pageTitleStep(step.state, '/settings', '');
  assert.equal(step.announce, false);
  step = pageTitleStep(step.state, '/settings', 'Settings');
  assert.equal(step.announce, true);
  // Background refresh, a renamed item, a changed tab in the query: the same address is not repeated.
  step = pageTitleStep(step.state, '/settings', 'Settings');
  assert.equal(step.announce, false);
  step = pageTitleStep(step.state, '/settings', 'Preferences');
  assert.equal(step.announce, false);
  step = pageTitleStep(step.state, '/media/abc', 'Dune');
  assert.equal(step.announce, true);
});

test('the browser tab names the page and falls back to the product name', async () => {
  const {documentTitle} = await pageTitle();
  const t = (id: string, values: Record<string, string>) => (enUS as Record<string, string>)[id]!.replace('{page}', values.page!).replace('{product}', values.product!);
  assert.equal(documentTitle('Settings', 'Portico', t), 'Settings · Portico');
  assert.equal(documentTitle('', 'Portico', t), 'Portico');
  assert.match(src('app/router.tsx'), /<Outlet \/>\n\s*<PageTitle \/>/, 'mounted once for every route, signed in or not');
});
