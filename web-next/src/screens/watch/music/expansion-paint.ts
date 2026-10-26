import {easings, reducedMotion, springs, springTiming} from '@design';
import {PlayerMorph} from '../../../kit/dock/motion';
import {claimDockWords, dockWordsOwner, releaseDockWords} from '../../../kit/dock/dock-words';
import {contentStep, expansionFrame, type PaintRect} from './music-expansion';

export type MusicPaintNodes = Readonly<{
  surface: HTMLElement; dock: HTMLElement; frame?: HTMLElement | null; cover?: HTMLElement | null;
  copies: readonly HTMLElement[]; groups: readonly HTMLElement[];
}>;
const fields = ['left','top','width','height'] as const;
type Box = Readonly<{left: number; top: number; width: number; height: number}>;
const owners = new WeakMap<HTMLElement, symbol>();
const rect = (node: HTMLElement | null | undefined): Box | undefined => {
  if (!node?.isConnected) return;
  const value = node.getBoundingClientRect();
  return [value.left,value.top,value.width,value.height].every(Number.isFinite) && value.width > 0 && value.height > 0 ? value : undefined;
};
const paintRect = (value: Box): PaintRect => ({left: value.left, top: value.top, right: value.left + value.width, bottom: value.top + value.height});
const equal = (a: Box | undefined, b: Box) => !!a && fields.every(field => Math.abs(a[field] - b[field]) < .1);
const unit = (value: number) => Math.max(0, Math.min(1, value));
function translation(value: string): Readonly<{x: number; y: number}> | undefined {
  if (value === 'none') return {x: 0, y: 0};
  try {
    const matrix = new DOMMatrixReadOnly(value);
    if (matrix.is2D && matrix.a === 1 && matrix.d === 1 && matrix.b === 0 && matrix.c === 0 && Number.isFinite(matrix.e) && Number.isFinite(matrix.f)) return {x: matrix.e, y: matrix.f};
  } catch { /* Unknown transform is not an admitted target. */ }
}
/** Undo only this owner's actual screen translation when reading its real cover slot. */
function coverTarget(nodes: MusicPaintNodes): Box | undefined {
  const cover = rect(nodes.cover);
  if (!cover || !nodes.frame || !nodes.frame.contains(nodes.cover!)) return;
  const offset = translation(getComputedStyle(nodes.frame).transform);
  return offset ? {...cover, left: cover.left - offset.x, top: cover.top - offset.y} : undefined;
}

/** One original playback's visual leases. It never owns/dispatches media or commands. */
export class MusicExpansionPaint {
  private token = Symbol('music-expansion-paint');
  private live = true;
  private jobs = new Map<HTMLElement, Animation>();
  private styles = new Map<HTMLElement, {opacity: string; transform: string; width: string; height: string; visibility: string; borderRadius: string}>();
  private physics = new Map<string, PlayerMorph>();
  private progress = new PlayerMorph(0);
  private first = true;
  private previousTarget: Box | undefined;
  private previousOpen = false;
  private previousReduced = false;
  private previousFrame: HTMLElement | null | undefined;
  constructor(private readonly current: () => boolean, private readonly collapsed: () => void) {}
  private own(node: HTMLElement) {
    if (!this.styles.has(node)) this.styles.set(node, {opacity: node.style.opacity, transform: node.style.transform, width: node.style.width, height: node.style.height, visibility: node.style.visibility, borderRadius: node.style.borderRadius});
    owners.set(node, this.token);
  }
  private valid(node: HTMLElement) {return this.live && this.current() && node.isConnected && owners.get(node) === this.token;}
  private release(node: HTMLElement) {
    this.jobs.get(node)?.cancel();this.jobs.delete(node);
    const style = this.styles.get(node);this.styles.delete(node);
    if (!style || owners.get(node) !== this.token) return;
    owners.delete(node);
    if (!node.hasAttribute('data-dock-identity') || dockWordsOwner(node) === this.token) Object.assign(node.style, style);
    if (dockWordsOwner(node) === this.token) releaseDockWords(node, this.token);
  }
  private animate(node: HTMLElement, frames: Keyframe[], duration: number, easing: string, delay = 0, done?: () => void) {
    this.own(node);
    this.jobs.get(node)?.cancel();
    const job = node.animate(frames, {duration, easing, delay, fill: 'both'});this.jobs.set(node, job);
    job.onfinish = () => {if (this.valid(node) && this.jobs.get(node) === job) done?.();};
  }
  /** Freeze exact painted values before a re-entrant runtime publication/retarget. */
  freeze() {
    for (const [node, job] of this.jobs) {
      if (owners.get(node) !== this.token || !node.isConnected) {job.cancel();continue;}
      const style = getComputedStyle(node);node.style.opacity = style.opacity;node.style.transform = style.transform;job.cancel();
    }
    this.jobs.clear();
  }
  update(nodes: MusicPaintNodes, open: boolean, reduced: boolean, force = false) {
    if (!this.live || !this.current()) return;
    const surface = nodes.surface, dock = rect(nodes.dock), target = open ? coverTarget(nodes) : dock;
    if (!target || !dock || surface.ownerDocument !== nodes.dock.ownerDocument || nodes.cover && surface.ownerDocument !== nodes.cover.ownerDocument) return;
    // Only actual measured slots can define a path. This is the public pure helper's admission.
    if (!expansionFrame(paintRect(dock), paintRect(target), 1)) return;
    const retained = new Set([surface, ...nodes.copies, ...nodes.groups, ...(nodes.frame ? [nodes.frame] : [])]);
    for (const node of this.styles.keys()) if (!retained.has(node)) this.release(node);
    if (!force && !this.first && this.previousOpen === open && this.previousReduced === reduced && this.previousFrame === nodes.frame && equal(this.previousTarget, target)) return;
    const now = performance.now(), old = rect(surface), frameFresh = this.previousFrame !== nodes.frame;
    const from = old ?? target, painted = new Map([...nodes.copies, ...nodes.groups, ...(nodes.frame ? [nodes.frame] : [])].map(node => {
      const style = getComputedStyle(node);return [node, {opacity: style.opacity, transform: style.transform}] as const;
    }));
    const wasOpen = this.previousOpen, policyChange = !this.first && this.previousReduced !== reduced;
    const oldProgress = this.progress.sample(now);
    const velocity = new Map(fields.map(field => [field, this.physics.get(field)?.sample(now).velocity ?? 0]));
    this.freeze();
    this.own(surface);
    // Initial collapsed paint belongs to DockEntry/Handover. Do not steal its +300ms words stage.
    if (!open && !nodes.frame) {
      surface.style.visibility = 'visible';surface.style.width = `${target.width}px`;surface.style.height = `${target.height}px`;
      surface.style.transform = `translate(${target.left}px, ${target.top}px)`;
      surface.style.borderRadius = getComputedStyle(nodes.dock).borderRadius;
      this.first = false;this.previousTarget = target;this.previousOpen = false;this.previousReduced = reduced;this.previousFrame = nodes.frame;return;
    }
    const image = surface.querySelector<HTMLImageElement>('img[data-artwork-admitted]');
    const decoded = !!image && image.isConnected && image.complete && image.naturalWidth > 0;
    const frameKnown = !nodes.frame || translation(painted.get(nodes.frame)!.transform) !== undefined;
    const morph = !this.first && decoded && !reduced && !policyChange && frameKnown;
    const duration = reduced || policyChange ? reducedMotion.durationMs : morph ? springTiming(springs.player).durationMs : open ? 180 : 120;
    const radius = getComputedStyle(open ? nodes.cover! : nodes.dock).borderRadius;
    surface.style.visibility = 'visible';surface.style.width = `${morph ? from.width : target.width}px`;surface.style.height = `${morph ? from.height : target.height}px`;
    surface.style.transform = `translate(${morph ? from.left : target.left}px, ${morph ? from.top : target.top}px)`;
    for (const field of fields) {
      const spring = this.physics.get(field) ?? new PlayerMorph(from[field]);spring.track(from[field], velocity.get(field) ?? 0, now);spring.retarget(target[field], now);this.physics.set(field, spring);
    }
    this.progress.track(this.first ? open ? 1 : 0 : oldProgress.position, this.first ? 0 : oldProgress.velocity, now);this.progress.retarget(open ? 1 : 0, now);
    const samples = Array.from({length: 49}, (_, index) => ({time: now + duration * index / 48, offset: index / 48}));
    const done = () => {
      if (this.valid(surface)) surface.style.borderRadius = radius;
      if (!open) {for (const node of nodes.copies) this.release(node);this.collapsed();}
    };
    if (morph) this.animate(surface, samples.map(({time, offset}, index) => {
      const at = Object.fromEntries(fields.map(field => [field, index === 48 ? target[field] : this.physics.get(field)!.sample(time).position])) as unknown as Box;
      return {offset, transform: `translate(${at.left}px, ${at.top}px) scale(${at.width / from.width}, ${at.height / from.height})`, opacity: 1};
    }), duration, 'linear', 0, done);
    else {
      // Missing decoded source/target means no invented geometry animation. The real image keeps
      // its own decode/cache fade; only the current screen cross-fades.
      this.animate(surface, [{opacity: 1}, {opacity: 1}], duration, 'linear', 0, done);
    }
    if (nodes.frame) {
      const style = painted.get(nodes.frame)!, height = nodes.frame.getBoundingClientRect().height;
      const opacity = frameFresh && open ? 0 : Number(style.opacity);
      const transform = policyChange || reduced || !morph ? 'none' : frameFresh && open ? `translateY(${height}px)` : style.transform;
      const y = frameFresh && open ? height : translation(style.transform)?.y;
      const screen = this.physics.get('screen') ?? new PlayerMorph(y ?? 0);
      screen.track(y ?? 0, this.physics.get('screen')?.sample(now).velocity ?? 0, now);screen.retarget(open ? 0 : height, now);this.physics.set('screen', screen);
      const frames = morph ? samples.map(({time, offset}, index) => {
        const progress = unit(index === 48 ? open ? 1 : 0 : this.progress.sample(time).position);
        return {offset, transform: `translateY(${index === 48 ? open ? 0 : height : screen.sample(time).position}px)`, opacity: Math.min(1, progress * 2.5)};
      }) : [{opacity, transform: 'none'}, {opacity: open ? 1 : 0, transform: 'none'}];
      frames[0] = {...frames[0], opacity, transform};
      this.animate(nodes.frame, frames, duration, morph ? 'linear' : reduced || policyChange ? reducedMotion.easing : open ? easings.out : easings.in);
    }
    for (const node of nodes.copies) {
      if (node.hasAttribute('data-dock-identity')) claimDockWords(node, this.token);
      const opacity = Number(painted.get(node)!.opacity);
      const frames = morph ? samples.map(({time, offset}, index) => ({offset, opacity: 1 - unit((index === 48 ? open ? 1 : 0 : this.progress.sample(time).position) * 4)})) : [{opacity}, {opacity: open ? 0 : 1}];
      frames[0] = {...frames[0], opacity};this.animate(node, frames, duration, morph ? 'linear' : reduced || policyChange ? reducedMotion.easing : easings.out);
    }
    for (const node of nodes.groups) {
      const index = Number(node.dataset.musicContentGroup), step = contentStep(index);
      if (!step) continue;
      const opacity = frameFresh && open ? 0 : Number(painted.get(node)!.opacity);
      this.animate(node, [{opacity, transform: 'none'}, {opacity: open ? 1 : 0, transform: 'none'}], reduced || policyChange ? 160 : open ? step.durationMs : 120,
        reduced || policyChange ? 'linear' : open ? easings.out : easings.in, frameFresh && open && !reduced ? 260 + step.delayMs : 0);
    }
    this.first = false;this.previousTarget = target;this.previousOpen = open;this.previousReduced = reduced;this.previousFrame = nodes.frame;
    // First collapsed placement is immediate and never schedules a fake expansion.
    if (!wasOpen && !open && !nodes.frame) this.freeze();
  }
  dispose() {
    this.live = false;
    for (const node of [...this.styles.keys()]) this.release(node);
  }
}
