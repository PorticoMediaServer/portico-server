// FX-050 browser-evaluation collectors and the closed receipt builder.
//
// Authored source only: nothing in this file has been executed, and no function here clicks, opens,
// closes, focuses, changes props or retargets anything. The owner arms a recorder with a selector it
// already owns, then performs its own real open/close; the collector only reads the supplied node.
//
// Page functions (fx050*) are self-contained so they can be passed straight to `page.evaluate`.
// They must not close over module scope. The Node helpers (fx050Receipt, fx050Transform) run outside
// the page and only see the JSON the page returned plus owner-supplied receipt fields.
//
// Owner contract (the collector never acts):
//   1. Owner opens the surface with its own real action and owns the selector.
//   2. Entrance: `await page.evaluate(fx050MenuEntry, {selector, sampleMs})`.
//   3. Exit: `await page.evaluate(fx050ArmExit, {selector, key})`, then the owner performs its own
//      real close (and, for a reversal, its own real reopen on the same node), then
//      `await page.evaluate(fx050ReadExit, {key})`, then always `await page.evaluate(fx050Dispose, {key})`.
//   4. Declared keyframes for a CSS exit: `await page.evaluate(fx050DeclaredAnimation, {selector})`.
//   5. Receipt: `fx050Receipt({manifest, scope, viewport, source, screenshots, reduction, entry, exit, selectExit})`
//      with `reduction: 'none' | 'app' | 'system'`, the actual manifest commit, the containing source
//      path/commit and bare screenshot file names supplied by the owner.
//   A missing arm/event read is `unobserved`; it is never reported as a timing pass.

export const FX050_EXPECT = Object.freeze({
  enterMs: 180,
  exitMs: 120,
  reducedMs: 160,
  reducedEasing: 'linear',
  normalEnterEasing: 'cubic-bezier(0.2, 0.8, 0.2, 1)',
  normalExitEasing: 'cubic-bezier(0.4, 0, 1, 1)',
  offsetPx: 4,
  scale: 0.98,
  presenceMs: 160,
});

/** Arm a recorder for one already-rendered node. The caller owns the open/close action. */
export function fx050ArmExit(input) {
  const node = document.querySelector(input.selector);
  if (!node) return {armed: false, reason: 'node-missing'};
  const key = input.key ?? 'fx050';
  const read = () => {
    const style = getComputedStyle(node), rect = node.getBoundingClientRect();
    return {
      t: Math.round(performance.now() - started),
      opacity: style.opacity,
      transform: style.transform,
      visibility: style.visibility,
      state: node.getAttribute('data-state'),
      side: node.getAttribute('data-side'),
      inert: node.hasAttribute('inert'),
      ariaHidden: node.getAttribute('aria-hidden'),
      connected: node.isConnected,
      box: {x: Math.round(rect.x), y: Math.round(rect.y), w: Math.round(rect.width), h: Math.round(rect.height)},
    };
  };
  const started = performance.now(), state = {key, selector: input.selector, started, samples: [read()], events: [], animations: [], collectAnimations: undefined};
  state.frame = requestAnimationFrame(function loop() {
    state.samples.push(read());
    state.frame = requestAnimationFrame(loop);
  });
  state.observer = new MutationObserver(records => {
    for (const record of records) {
      state.events.push({t: Math.round(performance.now() - started), attribute: record.attributeName, value: node.getAttribute(record.attributeName)});
    }
    // Read the declared animation at the transition itself: a finished animation is gone later.
    state.animations = state.collectAnimations();
  });
  state.observer.observe(node, {attributes: true, attributeFilter: ['data-state', 'data-side', 'inert', 'aria-hidden']});
  // The node's own animations are listed at read time; their declared timing is evidence, not a copy.
  state.collectAnimations = () => node.getAnimations().map(animation => {
    const timing = animation.effect.getTiming();
    return {
      duration: timing.duration, easing: timing.easing, fill: timing.fill, currentTime: animation.currentTime,
      keyframes: animation.effect.getKeyframes().map(frame => ({offset: frame.offset, opacity: frame.opacity ?? null, transform: frame.transform ?? null})),
    };
  });
  window.__fx050Recorders = window.__fx050Recorders ?? new Map();
  window.__fx050Recorders.set(key, state);
  return {armed: true, key, first: state.samples[0]};
}

/** Read a recorder's samples. A recorder that was never armed is unobserved, not a pass. */
export function fx050ReadExit(input) {
  const state = window.__fx050Recorders?.get(input.key ?? 'fx050');
  if (!state) return {observed: false, reason: 'not-armed'};
  return {observed: true, key: state.key, selector: state.selector, events: state.events.slice(), animations: state.animations.length ? state.animations : state.collectAnimations(), samples: state.samples.slice()};
}

/** Stop one recorder and release its observer and frame loop. */
export function fx050Dispose(input) {
  const recorders = window.__fx050Recorders;
  const key = input?.key;
  if (!recorders?.size) return {disposed: 0};
  let disposed = 0;
  for (const [name, state] of [...recorders]) {
    if (key !== undefined && name !== key) continue;
    cancelAnimationFrame(state.frame);
    state.observer.disconnect();
    recorders.delete(name);
    disposed++;
  }
  if (!recorders.size) delete window.__fx050Recorders;
  return {disposed};
}

/** Menu entrance on the resolved side, including a late flip: keyframes, timing and frame samples. */
export function fx050MenuEntry(input) {
  const node = document.querySelector(input.selector);
  if (!node) return {observed: false, reason: 'node-missing'};
  const animation = node.getAnimations()[0] ?? null;
  const timing = animation ? animation.effect.getTiming() : null;
  const keyframes = animation ? animation.effect.getKeyframes().map(frame => ({offset: frame.offset, opacity: frame.opacity ?? null, transform: frame.transform ?? null})) : [];
  const started = performance.now(), duration = input.sampleMs ?? 260, samples = [];
  return new Promise(resolve => {
    const step = () => {
      const style = getComputedStyle(node), rect = node.getBoundingClientRect();
      samples.push({
        t: Math.round(performance.now() - started),
        opacity: style.opacity, transform: style.transform,
        side: node.getAttribute('data-side'), state: node.getAttribute('data-state'),
        box: {x: Math.round(rect.x), y: Math.round(rect.y), w: Math.round(rect.width), h: Math.round(rect.height)},
      });
      if (performance.now() - started < duration) requestAnimationFrame(step);
      else resolve({observed: true, timing: timing ? {duration: timing.duration, easing: timing.easing, fill: timing.fill} : null,
        animationAtRead: animation !== null, currentTime: animation ? animation.currentTime : null, keyframes, samples});
    };
    requestAnimationFrame(step);
  });
}

/** The node's own declared animation: timing, keyframes and currentTime, for WAAPI or CSS animations. */
export function fx050DeclaredAnimation(input) {
  const node = document.querySelector(input.selector);
  if (!node) return {observed: false, reason: 'node-missing'};
  const animations = node.getAnimations();
  if (!animations.length) return {observed: false, reason: 'no-animation'};
  return {observed: true, animations: animations.map(animation => {
    const timing = animation.effect.getTiming();
    return {
      duration: timing.duration, easing: timing.easing, fill: timing.fill, currentTime: animation.currentTime,
      keyframes: animation.effect.getKeyframes().map(frame => ({offset: frame.offset, opacity: frame.opacity ?? null, transform: frame.transform ?? null})),
    };
  })};
}

/** Parse a computed transform into the scale and vertical offset the entrance is authored with. */
export function fx050Transform(value) {
  if (typeof value !== 'string') return {scale: null, offsetY: null};
  if (value === 'none') return {scale: 1, offsetY: 0};
  const matrix = value.match(/^matrix\(([^)]+)\)$/);
  if (matrix) {
    const parts = matrix[1].split(',').map(Number);
    return {scale: parts[0], offsetY: parts[5]};
  }
  const scale = value.match(/scale\(([-\d.]+)\)/), translate = value.match(/translateY\(([-\d.]+)px\)/);
  return {scale: scale ? Number(scale[1]) : null, offsetY: translate ? Number(translate[1]) : null};
}

const FORBIDDEN = ['text', 'textContent', 'innerText', 'url', 'href', 'cookie', 'storage', 'localStorage', 'sessionStorage', 'body', 'protocol', 'headers'];

const scanForbidden = (value, path, found) => {
  if (!value || typeof value !== 'object') return;
  for (const [key, entry] of Object.entries(value)) {
    const next = `${path}.${key}`;
    if (FORBIDDEN.includes(key)) found.push(next);
    scanForbidden(entry, next, found);
  }
};

const number = value => typeof value === 'number' && Number.isFinite(value);
const close = (value, expected, tolerance) => number(value) && Math.abs(value - expected) <= tolerance;
// WAAPI and CSS report the same curve with different spacing/leading zeros.
const normEasing = value => typeof value === 'string' ? value.replace(/\s+/g, '').replace(/0\./g, '.') : value;
const sameEasing = (actual, expected) => normEasing(actual) === normEasing(expected);

/** Build the closed receipt from the owner's manifest/scope/screenshots plus the collected samples. */
export function fx050Receipt(input) {
  const required = ['manifest', 'scope', 'viewport', 'source', 'screenshots', 'reduction'];
  const missing = required.filter(field => input?.[field] === undefined);
  if (missing.length) throw new Error(`fx050 receipt missing: ${missing.join(', ')}`);
  const forbidden = [];
  scanForbidden(input, 'receipt', forbidden);
  if (forbidden.length) throw new Error(`fx050 receipt contains forbidden fields: ${forbidden.join(', ')}`);
  const {manifest, scope, viewport, source, screenshots, reduction} = input;
  if (!manifest?.commit || !source?.path || !source?.commit) throw new Error('fx050 receipt needs manifest.commit and source.path/source.commit');
  if (!Array.isArray(screenshots) || screenshots.some(name => typeof name !== 'string' || /[/\\]|:\/\//.test(name))) throw new Error('fx050 receipt screenshots must be bare file names');
  if (!number(viewport?.width) || !number(viewport?.height)) throw new Error('fx050 receipt needs numeric viewport width/height');
  const reduced = reduction === 'app' || reduction === 'system';
  const expectedMs = reduced ? FX050_EXPECT.reducedMs : null;
  const checks = [];

  const entry = input.entry;
  if (!entry?.observed) checks.push({check: 'menu-entry', evidence: 'unobserved', reason: entry?.reason ?? 'not collected'});
  else {
    const frames = entry.keyframes ?? [], first = frames.find(frame => frame.offset === 0), last = frames.find(frame => frame.offset === 1);
    const from = fx050Transform(first?.transform), to = fx050Transform(last?.transform);
    const sides = [...new Set((entry.samples ?? []).map(sample => sample.side).filter(Boolean))];
    checks.push({
      check: 'menu-entry', evidence: 'measured',
      duration: entry.timing?.duration ?? null, easing: entry.timing?.easing ?? null,
      fromScale: from.scale, fromOffsetY: from.offsetY, toTransform: last?.transform ?? null,
      sides, sideChanges: Math.max(0, sides.length - 1),
      durationMatch: reduced ? close(entry.timing?.duration, FX050_EXPECT.reducedMs, 1) : close(entry.timing?.duration, FX050_EXPECT.enterMs, 1),
      easingMatch: reduced ? sameEasing(entry.timing?.easing, FX050_EXPECT.reducedEasing) : sameEasing(entry.timing?.easing, FX050_EXPECT.normalEnterEasing),
      poseMatch: first && last ? close(Math.abs(from.offsetY ?? NaN), FX050_EXPECT.offsetPx, 0.5) && close(from.scale, FX050_EXPECT.scale, 0.005) && to.scale === 1 && to.offsetY === 0 : null,
      initialPose: entry.animationAtRead ? 'declared' : 'unobserved',
    });
  }

  const exit = input.exit;
  if (!exit?.observed) checks.push({check: 'stationary-exit', evidence: 'unobserved', reason: exit?.reason ?? 'not collected'});
  else {
    const samples = exit.samples ?? [];
    const boxes = samples.map(sample => sample.box).filter(Boolean);
    const first = boxes[0], moved = boxes.some(box => box && first && (box.x !== first.x || box.y !== first.y || box.w !== first.w || box.h !== first.h));
    const transforms = [...new Set(samples.map(sample => sample.transform))];
    const opacities = samples.map(sample => Number(sample.opacity));
    let reversal = null;
    for (let index = 1; index < opacities.length - 1; index++) {
      if (opacities[index] < opacities[index - 1] && opacities[index + 1] > opacities[index]) { reversal = {at: samples[index].t, opacity: opacities[index]}; break; }
    }
    const animation = (exit.animations ?? [])[0];
    checks.push({
      check: 'stationary-exit', evidence: 'measured',
      duration: animation?.duration ?? null, easing: animation?.easing ?? null,
      durationMatch: reduced ? close(animation?.duration, FX050_EXPECT.reducedMs, 1) : close(animation?.duration, FX050_EXPECT.exitMs, 1),
      transformStable: !moved && transforms.length <= 1,
      transforms, opacityStart: opacities[0] ?? null, opacityEnd: opacities[opacities.length - 1] ?? null,
      events: exit.events ?? [], reversal,
      disconnectedAt: samples.find(sample => sample.connected === false)?.t ?? null,
    });
  }

  const select = input.selectExit;
  if (select !== undefined) {
    if (!select?.observed) checks.push({check: 'select-exit', evidence: 'unobserved', reason: select?.reason ?? 'not collected'});
    else {
      const declared = select.animations?.[0] ?? select;
      const frames = declared.keyframes ?? [], first = frames.find(frame => frame.offset === 0), last = frames.find(frame => frame.offset === 1);
      const from = fx050Transform(first?.transform), to = fx050Transform(last?.transform);
      checks.push({
        check: 'select-exit', evidence: 'measured',
        duration: declared.timing?.duration ?? declared.duration ?? null,
        easing: declared.timing?.easing ?? declared.easing ?? null,
        durationMatch: reduced ? close(declared.timing?.duration ?? declared.duration, FX050_EXPECT.reducedMs, 1) : close(declared.timing?.duration ?? declared.duration, FX050_EXPECT.exitMs, 1),
        easingMatch: reduced ? sameEasing(declared.timing?.easing ?? declared.easing, FX050_EXPECT.reducedEasing) : sameEasing(declared.timing?.easing ?? declared.easing, FX050_EXPECT.normalExitEasing),
        stationary: first && last ? from.scale === 1 && from.offsetY === 0 && to.scale === 1 && to.offsetY === 0 : null,
      });
    }
  }

  const measured = checks.every(check => check.evidence === 'measured');
  // Every declared comparison must be a real `true`; a null/unobserved value never becomes a match.
  const matches = checks.every(check => check.evidence === 'measured'
    && Object.entries(check).every(([key, value]) => !key.endsWith('Match') || value === true)
    && check.transformStable !== false && check.stationary !== false);
  return {
    receiptVersion: 1,
    runtime: true,
    // This is measurement evidence only. It is deliberately not a WORKS/DONE/FIXES status.
    evidence: measured ? 'measured' : 'partly-unobserved',
    matches,
    manifest, scope, viewport, source, screenshots, reduction,
    expected: {enterMs: FX050_EXPECT.enterMs, exitMs: FX050_EXPECT.exitMs, reducedMs: FX050_EXPECT.reducedMs, reducedEasing: FX050_EXPECT.reducedEasing, offsetPx: FX050_EXPECT.offsetPx, scale: FX050_EXPECT.scale},
    checks,
    limits: [
      'Collectors never click, open, close, focus or change props; a missing start event is unobserved, never a timing pass.',
      'No DOM text, URL, cookie, storage, body or protocol value is accepted into the receipt.',
      'This receipt does not grant runtime, frame or row completion by itself; it records what the owner-supplied manifest measured.',
      'Every armed recorder must be released with fx050Dispose after its read; the recorder samples until then.',
    ],
  };
}
