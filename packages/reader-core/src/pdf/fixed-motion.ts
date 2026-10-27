import {createPageTurnAnimation, pageGesture} from '../epub/page-motion.ts';

/** Internal original-gesture projection; it has no navigation or source power. */
export type FixedGesture = Readonly<{
  read(): Readonly<{displacement: number; velocity: number; released: boolean; canceled: boolean}>;
  subscribe(changed: () => void): () => void;
}>;
export type FixedTurnInput = Readonly<{
  host: HTMLElement; outgoing: HTMLElement; incoming: HTMLElement; width: number; height: number;
  direction: -1 | 1; rtl: boolean; signal: AbortSignal; gesture?: FixedGesture;
  current(): boolean; reduced(): boolean;
}>;

/** Two actual prepared surfaces share a viewport. No cloned image, guessed page
 * or decode flag is created. Construction is inert; the renderer publishes this
 * exact receipt before start can attach or paint. Adoption remains the renderer's
 * responsibility after finished=true, including the whole reduced fade. */
export function createFixedPageTurn(input: FixedTurnInput) {
  const {host, outgoing, incoming, width, height, direction, rtl, signal, gesture} = input;
  const document = host.ownerDocument, browser = document.defaultView;
  const frame = browser?.requestAnimationFrame.bind(browser), cancelFrame = browser?.cancelAnimationFrame.bind(browser);
  const read = gesture?.read.bind(gesture), subscribe = gesture?.subscribe.bind(gesture);
  const addAbort = signal.addEventListener.bind(signal), removeAbort = signal.removeEventListener.bind(signal);
  let live = true, started = false, stage: HTMLElement | undefined, stop: (() => void) | undefined;
  let motion: ReturnType<typeof createPageTurnAnimation> | undefined, phase: 'drag' | 'handoff' | 'settle' = gesture ? 'drag' : 'settle';
  let translation = 0, landed = false;
  const distance = direction * width * (rtl ? -1 : 1);
  const original = {transform: outgoing.style.transform, opacity: outgoing.style.opacity, overflow: host.style.overflow};
  let resolve!: (accepted: boolean) => void;
  const finished = new Promise<boolean>(done => {resolve = done;});
  const owns = () => {
    if (!live || signal.aborted) return false;
    let accepted = false;try {accepted = input.current();} catch { /* Original render refused. */ }
    return live && !signal.aborted && accepted;
  };
  const reduced = () => {try {return input.reduced() !== false;} catch {return true;}};
  const paint = (value: Readonly<{translation: number; opacity: number}>) => {
    if (!owns() || !stage) return;
    const originalPhase=phase,originalMotion=motion,reduce=phase==='handoff'||reduced();
    if(!owns()||phase!==originalPhase||motion!==originalMotion)return;
    translation = value.translation;
    if (landed) {
      outgoing.style.opacity = '0';stage.style.transform = 'translate3d(0,0,0)';stage.style.opacity = String(value.opacity);
    } else {
      outgoing.style.transform = `translate3d(${translation}px,0,0)`;outgoing.style.opacity = String(value.opacity);
      stage.style.transform = `translate3d(${translation + distance}px,0,0)`;stage.style.opacity = reduce ? '0' : '1';
    }
  };
  const finish = (accepted: boolean) => {
    if (!live) return;
    // Test ownership before withdrawal, then close before invoking cancel/off.
    let originalPaint = false;try {originalPaint = live && input.current() && live;} catch { /* Original paint retired. */ }
    live = false;
    const animation = motion;motion = undefined;animation?.cancel();removeAbort('abort', abort);
    const off = stop;stop = undefined;try {off?.();} catch { /* Continue releasing the original stage. */ }
    stage?.remove();
    let stillOriginal = false;try {stillOriginal = originalPaint && input.current();} catch { /* A successor owns paint. */ }
    if (stillOriginal) {outgoing.style.transform = original.transform;outgoing.style.opacity = original.opacity;host.style.overflow = original.overflow;}
    resolve(accepted);
  };
  const abort = () => finish(false);
  const animate = (to: number, velocity: number, spring: boolean, handoff = false, accepted = true) => {
    if (!owns() || !frame || !cancelFrame) {finish(false);return;}
    const turn: ReturnType<typeof createPageTurnAnimation> = createPageTurnAnimation({from: translation, to, offset: 0, velocity, spring,
      ports: {frame, cancelFrame, current: () => motion === turn && owns(), reduced: () => handoff || reduced(), paint,
        // This changes only which real staged surface paints. The renderer does
        // not adopt page/location or release outgoing resources until settlement.
        commit: () => {if (owns()) {if (handoff) translation = 0;else landed = accepted;}},
      }});
    motion = turn;turn.start();
    void turn.finished.then(done => {
      if (motion !== turn || !owns()) {finish(false);return;}
      motion = undefined;
      if (!done) {finish(false);return;}
      if (handoff) {phase = 'drag';translation = 0;paint({translation: 0, opacity: 1});changed();}
      else finish(accepted);
    });
  };
  function changed() {
    if (!started || !owns()) {if (started) finish(false);return;}
    if (!read || phase === 'settle') return;
    let state: ReturnType<NonNullable<typeof read>>;try {state=read();} catch {finish(false);return;}if (!owns()) {finish(false);return;}
    if (state.canceled) {finish(false);return;}
    if (phase === 'handoff') return;
    const reduce = reduced();if (!owns()) {finish(false);return;}
    if (reduce && translation !== 0) {phase = 'handoff';animate(0, 0, false, true);return;}
    if (!state.released) {if (!reduce) paint({translation: Math.max(-width, Math.min(width, state.displacement)), opacity: 1});return;}
    const accepted = pageGesture(width, state.displacement, state.velocity, rtl) === direction;
    phase = 'settle';animate(accepted ? -distance : 0, state.velocity, true, false, accepted);
  }
  return {finished, changed, cancel: abort,
    start() {
      if (started || !live) return;started = true;
      if (!owns() || !frame || !cancelFrame || !Number.isFinite(width) || width <= 0 || !Number.isFinite(height) || height <= 0) {finish(false);return;}
      try {
      stage = document.createElement('div');stage.dataset.readerTurn = '';
      Object.assign(stage.style, {position: 'absolute', left: `${host.scrollLeft}px`, top: `${host.scrollTop}px`, width: `${width}px`, height: `${height}px`, display: 'flex', overflow: 'hidden', pointerEvents: 'none', opacity: '0'});
      stage.append(incoming);host.append(stage);host.style.overflow = 'hidden';
      addAbort('abort', abort, {once: true});if (!owns()) {finish(false);return;}
      if (subscribe) {const off = subscribe(changed);if (live) stop = off;else off();}
      if (!owns()) {finish(false);return;}
      if (read) changed();else {phase = 'settle';animate(-distance, 0, false);}
      } catch {finish(false);}
    },
  };
}
