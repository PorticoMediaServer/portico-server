import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {runInNewContext} from 'node:vm';
import {createPageTurnAnimation, pageGesture} from '../src/epub/page-motion.ts';

/** Execute the production paginator's real private turn/touch/scroll/cleanup
 * methods. Fixture-only seeding supplies measured DOM geometry and a visible
 * text range, without implementing a second paginator or motion algorithm. */
function fixture({rtl = false, reduced = false, offset = 100, sections = 1} = {}) {
  let next = 0, now = 0, Constructor: any;
  const frames = new Map<number, (now: number) => void>();
  class Node extends EventTarget {
    style: any = {setProperty() {}};attributes = new Map<string, string>();ownerDocument: any;
    scrollLeft = 100;scrollTop = 0;width = 100;height = 100;children: any[] = [];
    constructor() {super();this.ownerDocument = document;}
    getBoundingClientRect() {return {width: this.width, height: this.height};}
    attachShadow() {const nodes = new Map<string, Node>();return {innerHTML: '', prepend() {}, getElementById(id: string) {let node = nodes.get(id);if (!node) {node = new Node();nodes.set(id, node);}return node;}};}
    hasAttribute(name: string) {return this.attributes.has(name);}
    getAttribute(name: string) {return this.attributes.get(name);}
    setAttribute(name: string, value: string) {this.attributes.set(name, value);}
    removeAttribute(name: string) {this.attributes.delete(name);}
    append(child: any) {this.children.push(child);}prepend() {}remove() {}
  }
  const document: any = {hidden: false, querySelector: () => null, createElement: () => new Node(), defaultView: {getSelection: () => null}};
  const visibleRange: any = {startContainer: {ownerDocument: document}};visibleRange.cloneRange = () => visibleRange;
  const source = readFileSync(new URL('../src/epub/vendor/paginator.js', import.meta.url), 'utf8')
    .replace(/^import .*\n/gm, '').replace(/^export /gm, '')
    .replace('    #getVisibleRange() {', '    #getVisibleRange() { if (globalThis.__fixtureRange) return globalThis.__fixtureRange;')
    .replace('    prev(distance) {', `    __fixtureSeed(view, rtl) {
      this.#view = {element:view, document:globalThis.__fixtureDocument, destroy(){}};
      this.#index=0;this.#rtl=rtl;this.#container.scrollLeft=rtl?-globalThis.__fixtureOffset:globalThis.__fixtureOffset;
      this.#scrollBounds=[this.#container.scrollLeft,100,100];this.sections=Array.from({length:globalThis.__fixtureSections},()=>({linear:'yes',unload(){},load:()=>{this.#cancelPageMotion();return null}}));
    }
    __fixtureReflow(){this.#cancelPageMotion();}
    __fixtureRead(){return {offset:this.#container[this.scrollProp],locked:this.#locked,view:this.#view?.element};}
    prev(distance) {`);
  const context: any = {Event, EventTarget, CustomEvent, HTMLElement: Node, document, console,
    NodeFilter: {SHOW_ELEMENT: 1, SHOW_TEXT: 4, SHOW_CDATA_SECTION: 8, FILTER_ACCEPT: 1, FILTER_REJECT: 2, FILTER_SKIP: 3},
    ResizeObserver: class {observe() {}disconnect() {}}, matchMedia: () => ({addEventListener() {}, removeEventListener() {}}),
    customElements: {define(_name: string, value: any) {Constructor = value;}},
    requestAnimationFrame(callback: (now: number) => void) {const id = ++next;frames.set(id, callback);return id;}, cancelAnimationFrame(id: number) {frames.delete(id);},
    setTimeout, clearTimeout, createPageTurnAnimation, pageGesture,
    unprefixEpubCss: (s: string) => s, lastLinearIndex: () => 0, __fixtureRange: visibleRange, __fixtureOffset: offset, __fixtureSections: sections, __fixtureDocument: {...document, body: {}, hasFocus: () => false},
    visualViewport: {scale: 1}};
  runInNewContext(source, context);
  const view = new Node();view.width = 600;
  const paginator = new Constructor();paginator.setAttribute('portico-motion', '');if (reduced) paginator.setAttribute('reduced-motion', '');paginator.__fixtureSeed(view, rtl);
  let turns = 0;paginator.addEventListener('portico-turn', () => {turns++;});
  const step = async (at: number) => {now = at;const callbacks = [...frames.values()];frames.clear();for (const callback of callbacks) callback(now);for (let pending = 0; pending < 8; pending++) await Promise.resolve();};
  const touch = (kind: string, x: number, at: number) => {
    const event = new Event(kind, {cancelable: true});Object.defineProperty(event, 'timeStamp', {value: at});
    Object.assign(event, {changedTouches: [{identifier: 1, screenX: x, screenY: 20}], touches: kind === 'touchend' ? [] : [{identifier: 1, screenX: x, screenY: 20}]});paginator.dispatchEvent(event);
  };
  return {paginator, view, frames, step, touch, get turns() {return turns;}};
}

test('real paginator next is320ms transform with one offset commit and no queued held turn', async () => {
  const f = fixture();const first = f.paginator.next(), duplicate = f.paginator.next();await duplicate;
  await f.step(0);await f.step(160);assert.equal(f.paginator.__fixtureRead().offset, 100);assert.match(f.view.style.transform, /translate3d\(-/);
  await f.step(320);await first;assert.equal(f.paginator.__fixtureRead().offset, 200);assert.equal(f.paginator.__fixtureRead().locked, false);
  assert.equal(f.view.style.transform, '');assert.equal(f.turns, 0, 'programmatic turn is not another gesture intent');f.paginator.destroy();
});

for (const rtl of [false, true]) test(`real touch ${rtl ? 'RTL' : 'LTR'} drags both columns and commits one gesture notification`, async () => {
  const f = fixture({rtl});const direction = rtl ? 1 : -1;
  f.touch('touchstart', 50, 0);f.touch('touchmove', 50 + direction * 30, 50);
  assert.match(f.view.style.transform, new RegExp(`translate3d\\(${direction * 30}px`));assert.equal(f.paginator.__fixtureRead().offset, rtl ? -100 : 100);
  f.touch('touchend', 50 + direction * 30, 60);
  for (let at = 0; at <= 3200; at += 16) await f.step(at);
  assert.equal(f.paginator.__fixtureRead().offset, rtl ? -200 : 200);assert.equal(f.turns, 1);assert.equal(f.paginator.__fixtureRead().locked, false);f.paginator.destroy();
});

test('a short paused touch returns to its original page; canceled and destroyed turns never land', async () => {
  const f = fixture();f.touch('touchstart', 50, 0);f.touch('touchmove', 30, 200);f.touch('touchend', 30, 400);
  for (let at = 0; at <= 3200; at += 16) await f.step(at);
  assert.equal(f.paginator.__fixtureRead().offset, 100);assert.equal(f.turns, 0);
  const turning = f.paginator.next();await f.step(4000);const saved = [...f.frames.values()];f.paginator.destroy();
  for (const callback of saved) callback(5000);await turning;assert.equal(f.frames.size, 0);assert.equal(f.turns, 0);
});

test('real reduced paginator release preserves the finger then snaps at zero opacity in160', async () => {
  const f = fixture({reduced: true});f.touch('touchstart', 50, 0);f.touch('touchmove', 20, 50);f.touch('touchend', 20, 60);
  await f.step(0);await f.step(40);assert.equal(f.paginator.__fixtureRead().offset, 100);assert.equal(f.view.style.opacity, '0.5');
  await f.step(80);assert.equal(f.paginator.__fixtureRead().offset, 200);assert.equal(f.view.style.opacity, '0');await f.step(160);
  assert.equal(f.turns, 1);f.paginator.destroy();
});


test('real chapter crossing keeps original navigation and releases the lock after its own layout', async () => {
  const f = fixture({offset: 400, sections: 2});f.touch('touchstart', 50, 0);f.touch('touchmove', 20, 50);f.touch('touchend', 20, 60);
  for (let at = 0; at <= 3200; at += 16) await f.step(at);
  assert.equal(f.paginator.__fixtureRead().offset, 100);assert.equal(f.paginator.__fixtureRead().locked, false);assert.equal(f.turns, 1);f.paginator.destroy();
});

test('ignored external goTo during a locked turn does not strand its lock; reflow cancels original paint', async () => {
  const f = fixture();const turning = f.paginator.next();await f.step(0);await f.paginator.goTo({index: 0, anchor: 0});await f.step(320);await turning;
  assert.equal(f.paginator.__fixtureRead().locked, false);
  const next = f.paginator.next();await f.step(400);await f.step(500);f.paginator.__fixtureReflow();await next;
  assert.equal(f.paginator.__fixtureRead().offset, 200);assert.equal(f.paginator.__fixtureRead().locked, false);assert.equal(f.view.style.transform, '');
  assert.equal(f.frames.size, 0);f.paginator.destroy();
});

test('a flick past the first real page never lands on its empty padding', async () => {
  const f = fixture();f.touch('touchstart', 50, 0);f.touch('touchmove', 90, 10);f.touch('touchend', 90, 20);
  for (let at = 0; at <= 3200; at += 16) await f.step(at);
  assert.equal(f.paginator.__fixtureRead().offset, 100);assert.equal(f.turns, 0);f.paginator.destroy();
});
