import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {registerHooks} from 'node:module';

type Ref = {current: unknown};
type Effect = {deps: readonly unknown[]; cleanup?: () => void};
const bridgeKey = '__porticoDockEntryHooks';
const bridge = {current: undefined as HookFrame | undefined};
(globalThis as Record<string, unknown>)[bridgeKey] = bridge;
const hookModule = 'data:text/javascript,' + encodeURIComponent(`const bridge = globalThis.${bridgeKey}; ${['useRef', 'useState', 'useLayoutEffect'].map(name => `export const ${name} = (...args) => bridge.current.${name}(...args);`).join('\n')}`);
const loader = registerHooks({load(url, context, next) {
  if (url.endsWith('/kit/dock/dock-entry.ts') || url.endsWith('/kit/dock/use-motion.ts')) {
    return {format: 'module-typescript', shortCircuit: true, source: readFileSync(new URL(url), 'utf8').replace("from 'react'", `from ${JSON.stringify(hookModule)}`)};
  }
  return next(url, context);
}});

/** Minimal DOM + WAAPI, mirroring the existing layout-presence hook seam. */
class ControlledAnimation {
  playState: 'running' | 'finished' | 'idle' = 'running';
  cancelled = false;
  private resolveFinished!: () => void;
  readonly finished: Promise<void>;
  constructor(readonly keyframes: readonly Record<string, unknown>[], readonly options: Record<string, unknown>) {
    this.finished = new Promise<void>(resolve => {this.resolveFinished = resolve;});
  }
  cancel() {this.cancelled = true; this.playState = 'idle';}
  resolve() {if (this.playState !== 'running') return; this.playState = 'finished'; this.resolveFinished();}
}
class FakeElement {
  style: Record<string, string> = {};
  computed = {opacity: '1', transform: 'none'};
  readonly animations: ControlledAnimation[] = [];
  words?: FakeElement;
  animate(keyframes: readonly Record<string, unknown>[], options: Record<string, unknown>) {
    const animation = new ControlledAnimation(keyframes, options); this.animations.push(animation); return animation as unknown as Animation;
  }
  querySelector(selector: string) {return selector === '[data-dock-identity]' ? this.words ?? null : null;}
  dispatchEvent() {return true;}
  closest() {return null;}
}
class HookFrame {
  private slots: any[] = [];
  private cursor = 0;
  private dirty = false;
  private mounted = true;
  private queued: {effect: Effect; run: () => void | (() => void)}[] = [];
  useRef(value: unknown) {
    const i = this.cursor++;
    if (!this.slots[i]) this.slots[i] = {current: value};
    return this.slots[i];
  }
  useState(initial: unknown) {
    const i = this.cursor++;
    this.slots[i] ??= {value: initial};
    return [this.slots[i].value, (value: unknown) => {
      assert.ok(this.mounted, 'no state updates after unmount');
      if (!Object.is(value, this.slots[i].value)) {this.slots[i].value = value; this.dirty = true;}
    }];
  }
  useLayoutEffect(run: () => void | (() => void), deps: readonly unknown[]) {
    const i = this.cursor++, previous: Effect | undefined = this.slots[i];
    if (!previous || deps.length !== previous.deps.length || deps.some((value, index) => !Object.is(value, previous.deps[index]))) {
      const effect = {deps, cleanup: previous?.cleanup}; this.slots[i] = effect; this.queued.push({effect, run});
    }
  }
  render<T>(run: () => T, commit: (result: T) => void): T {
    for (let attempts = 0; attempts < 8; attempts++) {
      bridge.current = this; this.cursor = 0; this.dirty = false; this.queued = [];
      const result = run(); commit(result);
      this.queued.forEach(job => job.effect.cleanup?.());
      this.queued.forEach(job => {job.effect.cleanup = job.run() ?? undefined;});
      if (!this.dirty) return result;
    }
    throw new Error('Dock entry effects did not settle within eight renders');
  }
  unmount() {
    if (!this.mounted) return;
    this.mounted = false;
    for (const slot of this.slots) slot?.cleanup?.();
  }
}

// Environment the hooks read at call time.
const listeners = new Set<() => void>();
const media = {matches: false, addEventListener: (_: string, listener: () => void) => listeners.add(listener), removeEventListener: (_: string, listener: () => void) => listeners.delete(listener)};
(globalThis as Record<string, unknown>).window = {matchMedia: () => media};
(globalThis as Record<string, unknown>).document = {documentElement: {dataset: {}}, body: {dataset: {}}};
(globalThis as Record<string, unknown>).MutationObserver = class {observe() {} disconnect() {}};
const computed = new WeakMap<FakeElement, FakeElement['computed']>();
(globalThis as Record<string, unknown>).getComputedStyle = (element: FakeElement) => computed.get(element) ?? element.computed;
(globalThis as Record<string, unknown>).Animation = ControlledAnimation;

const screen = new FakeElement(), words = new FakeElement();
screen.words = words;
computed.set(screen, screen.computed); computed.set(words, words.computed);
const dossier = {current: screen as unknown as HTMLElement};
const setPreference = (reduced: boolean) => {media.matches = reduced; listeners.forEach(listener => listener());};

const {useDockEntry} = await import('../../src/kit/dock/dock-entry.ts');
const {useDockHandover} = await import('../../src/kit/dock/use-motion.ts');
test.after(() => {loader.deregister(); delete (globalThis as Record<string, unknown>)[bridgeKey];});

const frame = new HookFrame();
let identity = 'A';
const paint = () => frame.render(() => {useDockEntry(dossier, identity); useDockHandover(dossier, identity);}, () => {});

test('a mid-entry reduction retargets the words from their painted opacity, without a delay', () => {
  paint();
  const entryBar = screen.animations.at(-1)!;
  assert.equal(entryBar.options.duration, 320);
  const entryWords = words.animations.at(-1)!;
  assert.equal(entryWords.options.delay, 300);
  // Part-way through the words' entrance, before their delay elapses.
  words.computed.opacity = '0.35';
  setPreference(true);
  paint();
  const retarget = words.animations.at(-1)!;
  assert.notEqual(retarget, entryWords);
  assert.equal(retarget.options.duration, 160);
  assert.equal(retarget.options.easing, 'linear');
  assert.equal(retarget.options.delay, undefined);
  assert.deepEqual(retarget.keyframes[0], {opacity: '0.35'});
  // One owner: the handover did not start a competing words animation.
  assert.equal(words.animations.filter(animation => animation.playState === 'running').length, 1);
});

test('a rapid policy reversal retargets again from the newest painted opacity', () => {
  words.computed.opacity = '0.6';
  setPreference(false);
  paint();
  const retarget = words.animations.at(-1)!;
  assert.equal(retarget.options.duration, 160);
  assert.deepEqual(retarget.keyframes[0], {opacity: '0.6'});
});

test('an identity handover after the entry releases the claim and keeps its authored recipe', () => {
  identity = 'B';
  paint();
  const handover = words.animations.at(-1)!;
  assert.equal(handover.options.duration, 180);
  assert.equal(handover.options.easing, 'ease-out');
  assert.deepEqual(handover.keyframes[0], {opacity: '0', transform: 'translateY(6px)'});
});

test('a stale entry completion cannot cancel or reset the successor handover', () => {
  const successor = words.animations.at(-1)!;
  // Settle every older animation, then the entry's queued completion.
  for (const animation of words.animations.slice(0, -1)) animation.resolve();
  assert.equal(successor.playState, 'running');
  assert.equal(successor.cancelled, false);
});
