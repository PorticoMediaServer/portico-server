import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {enUS} from '@i18n';
import {componentModule} from './helpers/component-harness.mjs';

const src = (path: string) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const shellModule = () => componentModule(new URL('../src/shell/Shell.tsx', import.meta.url), {
  react: {default: {}, lazy: () => 'lazy', createContext: () => ({}), useEffect: () => {}, useMemo: () => undefined, useRef: () => ({current: null}), useState: (v: unknown) => [v, () => {}], Suspense: 'suspense'},
}) as Promise<typeof import('../src/shell/Shell.tsx')>;

test('the signed-in shell offers Skip to content first, and its target is the page’s main region', () => {
  const shell = src('shell/Shell.tsx');
  const frame = shell.slice(shell.indexOf('<div ref={frame}'));
  // The link lives in ShellAids, under the I18nProvider, so it speaks the viewer's language.
  const aids = shell.slice(shell.indexOf('function ShellAids()'));
  assert.ok(aids.indexOf("<a href={'#' + MAIN_ID} className={s.skipLink} onClick={skipToContent}>{t('web.shell.skipToContent')}</a>") > 0, 'ShellAids renders the skip link');
  const skip = frame.indexOf('<ShellAids />');
  assert.ok(skip > 0, 'the skip link is rendered in the frame');
  assert.ok(skip < frame.indexOf('<Rail ') && skip < frame.indexOf('<TopBar'), 'before the rail and the top bar, so it is the first stop');
  const main = frame.indexOf('<main id={MAIN_ID} tabIndex={-1} className={s.content}>');
  assert.ok(main > frame.indexOf('<TopBar'), 'the main region starts after the persistent chrome');
  assert.ok(frame.indexOf('<Outlet />') > main && frame.indexOf('</main>') > frame.indexOf('<Outlet />'), 'and holds the current destination');
  assert.equal(enUS['web.shell.skipToContent'], 'Skip to content');
});

test('Skip to content moves focus to the main region without changing the address', async t => {
  const {skipToContent} = await shellModule();
  const focused: string[] = [];
  let prevented = 0;
  const had = Object.getOwnPropertyDescriptor(globalThis, 'document');
  Object.defineProperty(globalThis, 'document', {configurable: true, value: {getElementById: (id: string) => ({focus: () => { focused.push(id); }})}});
  t.after(() => { if (had) Object.defineProperty(globalThis, 'document', had); else delete (globalThis as {document?: unknown}).document; });
  skipToContent({preventDefault: () => { prevented++; }});
  assert.deepEqual(focused, ['main-content']);
  assert.equal(prevented, 1, 'a fragment link would add a history entry');
});
