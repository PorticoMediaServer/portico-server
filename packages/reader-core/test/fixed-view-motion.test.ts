import test from 'node:test';
import assert from 'node:assert/strict';
import {mountFixed, type FixedPreferences, type PageSurface} from '../src/pdf/fixed-view.ts';
import type {ReaderLocation, ReaderSurfaceInput} from '../src/ports.ts';
const tick = async () => {for (let n = 0; n < 30; n++) await Promise.resolve();};
const preferences: FixedPreferences = {twoPages: false, firstPageAlone: true, comicFit: 'auto', comicDirection: 'file', pdfView: 'page'};
async function fixture({format = 'pdf' as 'pdf' | 'cbz', rtl = false, appReduce = false, systemReduce = false} = {}) {
  const frames = new Map<number, FrameRequestCallback>();let sequence = 0;
  class Media extends EventTarget {matches = systemReduce;}
  const media = new Media(), listeners = new Set<() => void>();
  let reduced = appReduce, loads = 0, released = 0, disposed = 0, deferPage: number | undefined, releaseMount: (() => void) | undefined;
  const oldHTMLElement = globalThis.HTMLElement;
  class Node extends EventTarget {
    ownerDocument: any;dataset: Record<string, string> = {};style: any = {transform: '', opacity: '', overflow: ''};children: Node[] = [];parentElement: Node | null = null;
    tabIndex = 0;scrollLeft = 0;scrollTop = 0;scrollWidth = 100;scrollHeight = 100;clientWidth = 100;clientHeight = 100;textContent = '';
    constructor() {super();this.ownerDocument = document;}
    get firstElementChild() {return this.children[0] ?? null;}
    get isConnected(): boolean {return this === container || this === head || this.parentElement?.isConnected === true;}
    setAttribute() {}setPointerCapture() {}
    append(...nodes: Node[]) {for (const node of nodes) {node.remove();this.children.push(node);node.parentElement = this;}}
    replaceChildren(...nodes: Node[]) {for (const child of [...this.children]) child.remove();this.append(...nodes);}
    remove() {if (this.parentElement) {const parent = this.parentElement;parent.children = parent.children.filter(node => node !== this);this.parentElement = null;}}
    closest(selector: string) {return selector === '[data-reader-page]' && this.dataset.readerPage !== undefined ? this : null;}
    getBoundingClientRect() {return {left: 0, top: 0, right: 100, bottom: 100, width: 100, height: 100};}
    querySelectorAll(selector: string): Node[] {
      return this.children.flatMap(child => [...(selector === '[data-reader-page]' && child.dataset.readerPage !== undefined ? [child] : []), ...child.querySelectorAll(selector)]);
    }
    querySelector(selector: string) {const index = /data-reader-page="(\d+)"/.exec(selector)?.[1];return this.querySelectorAll('[data-reader-page]').find(node => node.dataset.readerPage === index) ?? null;}
  }
  const document: any = {createElement: () => new Node(), getSelection: () => ({isCollapsed: true}), defaultView: {
    matchMedia: () => media, requestAnimationFrame(callback: FrameRequestCallback) {const id = ++sequence;frames.set(id, callback);return id;}, cancelAnimationFrame(id: number) {frames.delete(id);},
  }};
  const container = new Node(), head = new Node();document.head = head;
  globalThis.HTMLElement = Node as any;
  const locations: ReaderLocation[] = [], inputs: ReaderSurfaceInput[] = [], lifetime = new AbortController();
  const policy = {reduced: () => reduced, subscribe(listener: () => void) {listeners.add(listener);return () => {listeners.delete(listener);};}};
  const view = await mountFixed(format, 5, rtl ? 'rtl' : 'ltr', async (index, {signal}) => {
    signal.throwIfAborted();loads++;
    const surface: PageSurface = {width: 100, height: 100, async mount(target, _scale, signal) {
      if (deferPage === index) await new Promise<void>(resolve => {releaseMount = resolve;});
      signal.throwIfAborted();const canvas = new Node();canvas.dataset.actualPage = String(index);(target as any).append(canvas);
      return () => {released++;canvas.remove();};
    }, dispose() {disposed++;}};return surface;
  }, {container: container as any, preferences, viewport: {width: 100, height: 100}, signal: lifetime.signal, motion: policy,
    onLocation: value => locations.push(value), onSelection() {}, onSurfaceInput: event => inputs.push(event)});
  const host = container.children[0];
  const step = async (at: number) => {const callbacks = [...frames.values()];frames.clear();for (const callback of callbacks) callback(at);await tick();};
  const pointer = (type: string, x: number, at: number, patch: Record<string, unknown> = {}) => {
    const event = new Event(type, {cancelable: true});Object.defineProperty(event, 'timeStamp', {value: at});
    Object.assign(event, {pointerId: 1, pointerType: 'touch', clientX: x, clientY: 20, ...patch});host.dispatchEvent(event);
  };
  const actualPages = (node: Node = host): number[] => node.children.flatMap(child => [...(child.dataset.actualPage !== undefined ? [Number(child.dataset.actualPage)] : []), ...actualPages(child)]);
  return {view, container, host, frames, locations, inputs, policy, listeners, step, pointer, actualPages,
    get loads() {return loads;}, get released() {return released;}, get disposed() {return disposed;},
    defer(index: number) {deferPage = index;}, async releaseMount() {releaseMount?.();await tick();},
    reduce(app: boolean, system = false) {reduced = app;media.matches = system;for (const listener of [...listeners]) listener();media.dispatchEvent(new Event('change'));},
    async close() {await view.dispose();globalThis.HTMLElement = oldHTMLElement;},
  };
}

for (const format of ['pdf', 'cbz'] as const) test(`actual ${format} retains real outgoing+prepared incoming until one320ms turn settles`, async () => {
  const f = await fixture({format});try {
    const old = f.host.firstElementChild!, navigation = f.view.navigate({kind: 'next'}, new AbortController().signal);await tick();
    assert.deepEqual(f.actualPages(), [0, 1]);assert.equal(f.host.firstElementChild, old);assert.equal(f.view.getLocation()?.format, format);
    await f.step(0);await f.step(160);assert.match(old.style.transform, /translate3d\(-/);assert.equal((f.view.getLocation() as any).page, 0);
    await f.step(320);await navigation;assert.deepEqual(f.actualPages(), [1]);assert.equal((f.view.getLocation() as any).page, 1);
    assert.equal(f.locations.length, 2);assert.equal(f.inputs.filter(event => event.kind === 'turned').length, 0);
  } finally {await f.close();}
});

for (const policy of ['app', 'system'] as const) test(`actual initial ${policy} reduction has opacity-only160 and adopts after settlement`, async () => {
  const f = await fixture({appReduce: policy === 'app', systemReduce: policy === 'system'});try {
    const old = f.host.firstElementChild!, navigation = f.view.navigate({kind: 'next'}, new AbortController().signal);await tick();
    await f.step(0);await f.step(40);assert.equal(old.style.transform, 'translate3d(0px,0,0)');assert.equal(old.style.opacity, '0.5');
    await f.step(80);assert.equal((f.view.getLocation() as any).page, 0);await f.step(160);await navigation;
    assert.deepEqual(f.actualPages(), [1]);assert.equal((f.view.getLocation() as any).page, 1);
  } finally {await f.close();}
});

test('actual fixed caller abort withdraws original RAF/paint and late callback cannot displace a successor', async () => {
  const f = await fixture();try {
    const caller = new AbortController(), old = f.host.firstElementChild!, navigation = f.view.navigate({kind: 'next'}, caller.signal);await tick();
    await f.step(0);await f.step(100);const callbacks = [...f.frames.values()];caller.abort();
    assert.equal(f.frames.size, 0);assert.equal(old.style.transform, '');assert.deepEqual(f.actualPages(), [0]);
    const successor = f.view.navigate({kind: 'next'}, new AbortController().signal);await tick();
    for (const callback of callbacks) callback(1000);await assert.rejects(navigation, {name: 'AbortError'});
    assert.equal((f.view.getLocation() as any).page, 0);await f.step(1000);await f.step(1320);await successor;
    assert.deepEqual(f.actualPages(), [1]);assert.equal(f.locations.length, 2);
  } finally {await f.close();}
});

test('real surface mount completion is required; caller abort during rendering cannot adopt or publish', async () => {
  const f = await fixture();try {
    f.defer(1);const caller = new AbortController(), navigation = f.view.navigate({kind: 'next'}, caller.signal);await tick();
    assert.deepEqual(f.actualPages(), [0]);assert.equal(f.frames.size, 0);caller.abort();await f.releaseMount();
    await assert.rejects(navigation, {name: 'AbortError'});assert.deepEqual(f.actualPages(), [0]);assert.equal(f.locations.length, 1);
  } finally {await f.close();}
});

for (const rtl of [false, true]) test(`actual ${rtl ? 'RTL' : 'LTR'} fitted touch tracks both real pages and reports only one committed turn`, async () => {
  const f = await fixture({format: 'cbz', rtl});try {
    const old = f.host.firstElementChild!, sign = rtl ? 1 : -1;
    f.pointer('pointerdown', 50, 0);f.pointer('pointermove', 50 + sign * 30, 50);await tick();
    assert.deepEqual(f.actualPages(), [0, 1]);assert.equal(old.style.transform, `translate3d(${sign * 30}px,0,0)`);
    f.pointer('pointerup', 50 + sign * 30, 60);for (let at = 0; at <= 3200; at += 16) await f.step(at);
    assert.deepEqual(f.actualPages(), [1]);assert.equal(f.inputs.filter(event => event.kind === 'turned').length, 1);
  } finally {await f.close();}
});

test('mid-held reduction withdraws the actual current pose, does not follow another move, and resolves the retained release once', async () => {
  const f = await fixture();try {
    const old = f.host.firstElementChild!;f.pointer('pointerdown', 50, 0);f.pointer('pointermove', 20, 50);await tick();
    assert.equal(old.style.transform, 'translate3d(-30px,0,0)');f.reduce(true);f.pointer('pointermove', 0, 100);
    assert.equal(old.style.transform, 'translate3d(-30px,0,0)');f.pointer('pointerup', 0, 120);
    await f.step(0);await f.step(40);assert.equal(old.style.transform, 'translate3d(-30px,0,0)');assert.equal(old.style.opacity, '0.5');
    await f.step(80);assert.equal(old.style.transform, 'translate3d(0px,0,0)');assert.equal(old.style.opacity, '0');await f.step(160);
    await f.step(160);await f.step(240);await f.step(320);
    assert.deepEqual(f.actualPages(), [1]);assert.equal(f.inputs.filter(event => event.kind === 'turned').length, 1);
  } finally {await f.close();}
});

test('short held gesture returns to its original page; resize and disposal retire only original surfaces', async () => {
  const f = await fixture();try {
    f.pointer('pointerdown', 50, 0);f.pointer('pointermove', 40, 200);await tick();f.pointer('pointerup', 40, 400);
    for (let at = 0; at <= 3200; at += 16) await f.step(at);
    assert.deepEqual(f.actualPages(), [0]);assert.equal(f.locations.length, 1);assert.equal(f.inputs.filter(event => event.kind === 'turned').length, 0);
    const navigation = f.view.navigate({kind: 'next'}, new AbortController().signal);await tick();await f.step(4000);const saved = [...f.frames.values()];
    await f.view.resize({width: 120, height: 100}, new AbortController().signal);await assert.rejects(navigation, {name: 'AbortError'});
    for (const callback of saved) callback(5000);assert.deepEqual(f.actualPages(), [0]);assert.equal(f.frames.size, 0);
  } finally {await f.close();assert.equal(f.listeners.size, 0);}
});
