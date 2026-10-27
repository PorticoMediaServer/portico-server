import test from 'node:test';
import assert from 'node:assert/strict';
import {castProofKey} from '@core/cast/index.ts';
import {componentModule} from './helpers/component-harness.mjs';

// Root50: a sender belongs to one exact captured context — the same facts the pairing proof is
// keyed by. These tests read the component's own hook state, so the lifetime rule is proven from
// the source instead of by handing a sender a context by hand.

type Viewer = {serverId: string; authority: string; accountId: string; profileId: string};
type Session = {viewer: Viewer; deviceId: string; sessionFamilyId: string; accessToken: string; refreshToken: string};
type Pin = {serverId: string; publicKey: string};
type Api = {baseUrl: string; getRouteConnection: () => {pin: Pin} | undefined};

const context = {origin: 'https://server.example', publicKey: 'A'.repeat(43), serverId: 'server', authority: 'local', accountId: 'account', profileId: 'profile', senderDeviceId: 'device', senderFamilyId: 'family'};
const session = (change: Partial<Omit<Session, 'viewer'>> & {viewer?: Partial<Viewer>} = {}): Session => ({
  viewer: {serverId: 'server', authority: 'local', accountId: 'account', profileId: 'profile', ...change.viewer},
  deviceId: change.deviceId ?? 'device', sessionFamilyId: change.sessionFamilyId ?? 'family',
  accessToken: change.accessToken ?? 'access-one', refreshToken: change.refreshToken ?? 'refresh-one',
});

class RecordedSender {
  disposed = false;
  constructor(readonly api: unknown, readonly displayName?: string, readonly controllerContext?: Record<string, string>) {}
  getSnapshot() { return {phase: 'off'}; }
  subscribe() { return () => {}; }
  prepare() { return Promise.resolve(); }
  stop() {}
  dispose() { this.disposed = true; }
}

// Hook storage with effect cleanup between renders: the provider's own dispose effect is what
// retires a sender, so the test has to run it the way React does.
function harness() {
  const slots: {deps?: unknown[]; value?: unknown; cleanup?: () => void; current?: unknown}[] = [];
  let cursor = 0; const pending: (() => void)[] = [];
  const react = {
    createContext: (value: unknown) => ({value, Provider: 'provider'}),
    createElement: (type: unknown, props: Record<string, unknown>, ...children: unknown[]) => ({type, props: {...props, children}}),
    useRef: (value: unknown) => { const n = cursor++; return slots[n] ??= {current: value}; },
    useState: (initial: unknown) => {
      const n = cursor++; if (!(n in slots)) slots[n] = {value: typeof initial === 'function' ? (initial as () => unknown)() : initial};
      return [slots[n].value, (value: unknown) => { slots[n].value = typeof value === 'function' ? (value as (v: unknown) => unknown)(slots[n].value) : value; }];
    },
    useMemo: (fn: () => unknown, deps: unknown[]) => {
      const n = cursor++;
      const slot = slots[n];
      if (!slot || !slot.deps || deps.length !== slot.deps.length || deps.some((v, i) => v !== slot.deps![i])) slots[n] = {deps, value: fn()};
      return slots[n].value;
    },
    useCallback: (fn: unknown) => fn,
    useEffect: (fn: () => void | (() => void), deps: unknown[]) => {
      const n = cursor++;
      const slot = slots[n] ?? (slots[n] = {});
      if (slot.deps && deps.length === slot.deps.length && deps.every((v, i) => v === slot.deps![i])) return;
      slot.deps = deps;
      pending.push(() => { slot.cleanup?.(); slot.cleanup = fn(); });
    },
    useSyncExternalStore: (_subscribe: unknown, get: () => unknown) => get(),
    useContext: (value: {value: unknown}) => value.value,
  };
  return {
    react: {...react, default: react},
    render: (fn: () => unknown) => { cursor = 0; const out = fn(); for (const run of pending.splice(0)) run(); return out; },
  };
}

async function provider(state: {api: Api; session?: Session}) {
  // The provider's idle effect reads `window`; an idle API keeps it from arming a timer.
  (globalThis as unknown as {window: unknown}).window = {requestIdleCallback: () => 1, cancelIdleCallback: () => {}};
  const h = harness();
  const module = await componentModule(new URL('../src/app/cast.tsx', import.meta.url), {
    react: h.react,
    './session': {useSession: () => state},
    '../player/engine': {usePlayer: () => ({active: false})},
    './server-preferences': {useServerPreferences: () => ({value: () => 0})},
    './i18n': {useI18n: () => ({t: (key: string) => key})},
    './cast.module.css': {default: {}},
    '../bridge/cast': {CastSender: RecordedSender},
  }) as Promise<{CastProvider: (props: {children: unknown}) => {props: {value: RecordedSender | null}}}>;
  const mount = () => h.render(() => module.CastProvider({children: null})).props.value;
  return {mount};
}

test('the Cast sender survives an access-token rotation and is retired by every proof fact', async () => {
  const api: Api = {baseUrl: 'https://server.example/', getRouteConnection: () => ({pin: {serverId: 'server', publicKey: 'A'.repeat(43)}})};
  const state = {api, session: session()};
  const {mount} = await provider(state);
  const first = mount();
  assert.ok(first, 'a signed-in viewer gets a sender');
  assert.deepEqual(first.controllerContext, context);
  assert.ok(Object.isFrozen(first.controllerContext), 'the captured context is immutable');
  assert.doesNotMatch(JSON.stringify(first.controllerContext), /access-one|refresh-one/, 'no token is part of the identity');
  const proofKey = castProofKey(first.controllerContext!, 'receiver-1');

  // A rotated access token is the same sign-in, the same proof key and the same sender.
  state.session = session({accessToken: 'access-two', refreshToken: 'refresh-two'});
  assert.equal(mount(), first);
  assert.equal(first.disposed, false, 'a token rotation does not retire the sender');
  assert.equal(castProofKey(mount().controllerContext!, 'receiver-1'), proofKey);

  // Every fact in the proof's scope is a different sender, and the old one is disposed.
  const retire = (change: () => void, facts: Record<string, string> = {}) => {
    const before = mount();
    change();
    const after = mount();
    assert.notEqual(after, before);
    assert.equal(before!.disposed, true);
    for (const [name, value] of Object.entries(facts)) assert.equal((after!.controllerContext as Record<string, string>)[name], value);
    return after!;
  };
  let current = retire(() => { state.session = session({viewer: {authority: 'hosted'}})}, {authority: 'hosted'});
  current = retire(() => { state.session = session({viewer: {authority: 'hosted', accountId: 'account-2'}})}, {accountId: 'account-2'});
  current = retire(() => { state.session = session({viewer: {authority: 'hosted', accountId: 'account-2', profileId: 'profile-2'}})}, {profileId: 'profile-2'});
  current = retire(() => { state.session = session({viewer: {authority: 'hosted', accountId: 'account-2', profileId: 'profile-2'}, deviceId: 'device-2'})}, {senderDeviceId: 'device-2'});
  current = retire(() => { state.session = session({viewer: {authority: 'hosted', accountId: 'account-2', profileId: 'profile-2'}, deviceId: 'device-2', sessionFamilyId: 'family-2'})}, {senderFamilyId: 'family-2'});
  current = retire(() => { api.getRouteConnection = () => ({pin: {serverId: 'server', publicKey: 'B'.repeat(43)}})}, {publicKey: 'B'.repeat(43)});
  // A pin for another server is not this sender's identity: it can pair, but claims no controller.
  current = retire(() => { api.getRouteConnection = () => ({pin: {serverId: 'other', publicKey: 'B'.repeat(43)}})});
  assert.equal(current.controllerContext, undefined, 'a pin for another server is never this sender’s proof');
  // The receiver address is part of the identity too, even when the api object is unchanged.
  current = retire(() => { api.baseUrl = 'https://other.example/'; api.getRouteConnection = () => ({pin: {serverId: 'server', publicKey: 'B'.repeat(43)}})}, {origin: 'https://other.example'});
  current = retire(() => { state.session = session({viewer: {serverId: 'server-2', authority: 'hosted', accountId: 'account-2', profileId: 'profile-2'}, deviceId: 'device-2', sessionFamilyId: 'family-2'}); api.getRouteConnection = () => ({pin: {serverId: 'server-2', publicKey: 'B'.repeat(43)}})}, {serverId: 'server-2'});

  // Signing out removes the sender; the cast bar and its proof go with the session.
  state.session = undefined;
  assert.equal(mount(), null);
  assert.equal(current.disposed, true);
});

test('a plain-HTTP server keeps a sender that never claims a controller', async () => {
  const api: Api = {baseUrl: 'http://server.example/', getRouteConnection: () => ({pin: {serverId: 'server', publicKey: 'A'.repeat(43)}})};
  const {mount} = await provider({api, session: session()});
  const sender = mount();
  assert.ok(sender, 'Cast is still offered so prepare() can report why it cannot run');
  assert.equal(sender!.controllerContext, undefined);
});
