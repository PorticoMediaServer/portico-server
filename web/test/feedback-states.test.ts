/** CON-18 (MU7): StateView and Notice keep one look on both platforms —
 * one primary action with an optional ghost secondary, and one glyph per
 * tone — and the loading/empty copy keys resolve to the §4.4 wording
 * ("Loading {thing}", no ellipsis, never a bare "Loading"). */
import test from 'node:test';
import assert from 'node:assert/strict';
import {enUS, formatMessage} from '../../packages/i18n/src/index.ts';
import {componentModule, hooks} from './helpers/component-harness.mjs';

const css = () => ({default: new Proxy({}, {get: (_t, k) => k})});
const uiStubs = {
  './cx': {cx: (...parts: unknown[]) => parts.filter(Boolean).join(' ')},
  './i18n': {uiI18n: () => ({t: (id: string) => id})},
  './Icon': {Icon: 'Icon'},
  './Button': {Button: 'Button'},
  './Feedback.module.css': css(),
};

async function load() {
  const h = hooks();
  return componentModule(new URL('../src/ui/Feedback.tsx', import.meta.url), {react: h.react, ...uiStubs});
}

/** Every element in the stub tree, depth first. */
function all(node: unknown, out: {type?: unknown; props?: any}[] = []): {type?: unknown; props?: any}[] {
  if (!node || typeof node !== 'object') return out;
  if (Array.isArray(node)) { node.forEach(n => all(n, out)); return out; }
  out.push(node as {type?: unknown; props?: any});
  all((node as {props?: any}).props?.children, out);
  return out;
}

const buttons = (root: unknown) => all(root).filter(n => n.type === 'Button').map(n => n.props);
const icons = (root: unknown) => all(root).filter(n => n.type === 'Icon').map(n => n.props);

test('CON-18: StateView renders one primary action and an optional ghost secondary', async () => {
  const m = await load();
  const view = m.StateView({title: 'This library couldn’t load', action: {label: 'Try again', onClick: () => {}}, secondaryAction: {label: 'Go back', onClick: () => {}}});
  const found = buttons(view);
  assert.equal(found.length, 2);
  assert.equal(found[0].variant, 'primary');
  assert.equal(found[1].variant, 'ghost');
});

test('CON-18: StateView with a single action renders only the primary', async () => {
  const m = await load();
  const view = m.StateView({title: 'Nothing here yet', action: {label: 'Try again', onClick: () => {}}});
  const found = buttons(view);
  assert.equal(found.length, 1);
  assert.equal(found[0].variant, 'primary');
});

test('CON-18: Notice actions are secondary with a ghost secondary; errors use role=alert', async () => {
  const m = await load();
  for (const tone of ['info', 'success', 'warning', 'error'] as const) {
    const notice = m.Notice({tone, action: {label: 'Try again', onClick: () => {}}, secondaryAction: {label: 'Dismiss', onClick: () => {}}});
    assert.equal(notice.props.role, tone === 'error' ? 'alert' : 'status');
    const found = buttons(notice);
    assert.equal(found[0].variant, 'secondary');
    assert.equal(found[0].size, 'sm');
    assert.equal(found[1].variant, 'ghost');
  }
});

test('CON-18: Notice draws one glyph per tone from the shared semantic set', async () => {
  const m = await load();
  const expected: Record<string, string> = {info: 'info', success: 'success', warning: 'warning', error: 'error'};
  for (const [tone, glyph] of Object.entries(expected)) {
    const notice = m.Notice({tone: tone as 'info', children: 'x'});
    assert.equal(icons(notice)[0].name, glyph);
  }
});

test('CON-18: loading and empty copy keys resolve to the §4.4 wording', () => {
  const table: [string, (v: string) => void][] = [
    ['status.loadingThing', v => assert.match(formatMessage(v, {thing: 'Saved items'}), /^Loading Saved items$/)],
    ['saved.loading', v => assert.match(v, /^Loading .+/)],
    ['library.emptyLibrary', v => assert.ok(v.length > 0)],
    ['library.emptyLibraryBody', v => assert.ok(v.length > 0)],
    ['downloads.emptyBody', v => assert.match(v, /Press and hold/)],
    ['title.playlistEmptyBody', v => assert.match(v, /Press and hold a title/)],
    ['title.playlistEmptyBodyTV', v => assert.match(v, /Press and hold Select/)],
  ];
  for (const [key, check] of table) {
    const value = (enUS as Record<string, string>)[key];
    assert.ok(value, `${key} resolves`);
    assert.doesNotMatch(value, /…|\.\.\./, `${key} has no ellipsis`);
    assert.notEqual(value, 'Loading', `${key} is never a bare "Loading"`);
    check(value);
  }
  // Web gesture variant names the menu; the TV variant names the Select button.
  assert.match((enUS as Record<string, string>)['web.downloads.emptyBody'], /menu/);
});

test('X-04 addendum: the ServerSetup strings live in web.setup.* with the same wording', () => {
  const c = enUS as Record<string, string>;
  assert.equal(c['web.setup.recoveryTitleHosted'], 'Create a recovery owner');
  assert.equal(c['web.setup.recoveryTitleLocal'], 'Create the owner account');
  assert.equal(c['web.setup.mismatch'], c['web.directJoin.mismatch']);
  assert.equal(c['web.setup.createRecoveryOwner'], 'Create recovery owner');
  assert.equal(c['web.setup.createOwner'], 'Create owner');
  for (const key of ['web.setup.recoveryTitleHosted', 'web.setup.recoveryTitleLocal', 'web.setup.recoveryBodyHosted', 'web.setup.recoveryBodyLocal', 'web.setup.mismatch', 'web.setup.createRecoveryOwner', 'web.setup.createOwner', 'web.signin.hostedLede', 'web.signin.accountLede']) {
    assert.doesNotMatch(c[key], /…|\.\.\./, `${key} has no ellipsis`);
  }
});
