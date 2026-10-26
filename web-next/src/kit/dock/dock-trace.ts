/**
 * Read-only observation for the desktop dock bar. It samples the browser's running
 * animations, the bar's geometry and its connection; it never starts, pauses, cancels or
 * replays a paint, and product code does not import it. Run it around an ordinary owner
 * action (minimise, Reduce, identity change, expand) and keep the receipt as evidence.
 * Nothing here infers media, session or engine survival.
 */
export type DockTraceSample = Readonly<{
  at: number;
  target: 'bar' | 'identity' | string;
  connected: boolean;
  parent: string | null;
  opacity: string;
  transform: string;
  top: number;
  duration: number;
  delay: number;
  easing: string;
  playState: AnimationPlayState;
  currentTime: number | null;
}>;
export type DockTraceReceipt = Readonly<{
  startedAt: number;
  endedAt: number;
  visibleAtStart: boolean;
  samples: readonly DockTraceSample[];
}>;

function label(element: Element): string {
  if (element.hasAttribute('data-dock-identity')) return 'identity';
  if (element.matches('section')) return 'bar';
  return element.tagName.toLowerCase();
}

/** Samples until `stop()`; no paint is created, changed or cancelled by the recorder. */
export function startDockTrace(root: HTMLElement, options: Readonly<{maxSamples?: number; frameMs?: number}> = {}): Readonly<{stop: () => DockTraceReceipt}> {
  const startedAt = performance.now(), maxSamples = options.maxSamples ?? 900, frameMs = options.frameMs ?? 16;
  const samples: DockTraceSample[] = [], rect = root.getBoundingClientRect();
  const visibleAtStart = root.isConnected && rect.bottom > 0 && rect.top < (root.ownerDocument.defaultView?.innerHeight ?? 0);
  let stopped = false, last = 0, frame = 0;
  const sample = (at: number) => {
    if (stopped || at - last < frameMs || samples.length >= maxSamples) return;
    last = at;
    for (const animation of root.ownerDocument.getAnimations()) {
      const target = (animation.effect as KeyframeEffect | null)?.target;
      if (!(target instanceof Element) || !(target === root || root.contains(target))) continue;
      const timing = animation.effect?.getTiming(), style = getComputedStyle(target), box = target.getBoundingClientRect();
      samples.push(Object.freeze({at: at - startedAt, target: label(target), connected: target.isConnected, parent: target.parentElement?.tagName.toLowerCase() ?? null,
        opacity: style.opacity, transform: style.transform, top: box.top,
        duration: typeof timing?.duration === 'number' ? timing.duration : 0, delay: typeof timing?.delay === 'number' ? timing.delay : 0,
        easing: String(timing?.easing ?? ''), playState: animation.playState, currentTime: typeof animation.currentTime === 'number' ? animation.currentTime : null}));
    }
  };
  const tick = (at: number) => {sample(at); if (!stopped) frame = requestAnimationFrame(tick);};
  frame = requestAnimationFrame(tick);
  return Object.freeze({stop: () => {
    stopped = true; cancelAnimationFrame(frame);
    return Object.freeze({startedAt, endedAt: performance.now(), visibleAtStart, samples: [...samples]});
  }});
}

/** Names what the observed samples do and do not show. Pure; no expectation is assumed. */
export function dockTraceFindings(receipt: DockTraceReceipt): readonly string[] {
  const findings: string[] = [];
  const bar = receipt.samples.filter(sample => sample.target === 'bar'), words = receipt.samples.filter(sample => sample.target === 'identity');
  if (!receipt.visibleAtStart) findings.push('bar was not visibly connected at the start; entrance timing is unobserved');
  const entry = bar.find(sample => sample.duration === 320 && sample.easing.includes('0.2, 0.8, 0.2, 1'));
  findings.push(entry ? `entry: bar sampled at ${entry.at.toFixed(0)}ms with 320ms out` : 'entry: no bar sample with the authored 320ms out paint');
  const staged = words.find(sample => sample.delay === 300);
  findings.push(staged ? `words: sampled with the authored 300ms delay at ${staged.at.toFixed(0)}ms` : 'words: no sample with the authored 300ms delay');
  const handoff = words.find(sample => sample.duration === 160 && sample.easing === 'linear' && Number(sample.opacity) > 0 && Number(sample.opacity) < 1);
  findings.push(handoff ? `handover: partial word opacity ${handoff.opacity} retargeted in 160ms linear` : 'handover: no partial-opacity 160ms linear sample');
  const gone = bar.find(sample => !sample.connected);
  findings.push(gone ? `host: bar sampled disconnected at ${gone.at.toFixed(0)}ms` : 'host: no disconnection sampled; the bar survived every observed action');
  return Object.freeze(findings);
}
