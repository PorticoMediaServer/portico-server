/**
 * Paint admission for already-decoded artwork.
 *
 * The scheduler, the request, the cache, retention and decode lifetime stay with
 * their owner. This module owns one narrow decision: in which order, and how far
 * apart, a row reveals the decoded paint of images that already finished decoding.
 *
 * The shape that makes reading order real:
 * - the owner calls `mount({group, order})` when a slot becomes **visible**, before
 *   its image decodes. That registration is the pending predecessor. A mounted but
 *   undecoded predecessor blocks its later siblings; that is reading order, not a
 *   guess.
 * - the owner calls `ready(paint)` only when that slot's own image has decoded, and
 *   `retire()` when it failed, went offscreen, was replaced or unmounted. A retired
 *   slot is skipped at once and can never strand the siblings behind it.
 * - admissions in one group are spaced by one bounded 40 ms step; the first paint
 *   in a group is immediate.
 * - a slot with no real `group`, or one mounted without the owning consumer's
 *   `scope`, never joins a queue: it paints on its own. There is no invented global
 *   group, and two schedulers with the same group name never share a queue.
 * - the helper creates no authority, timeout or fallback fact. The owner keeps the
 *   request/decode lifetime and is responsible for settling or retiring every
 *   mounted registration; an unready mounted head simply waits for its own owner.
 * - a retired or re-created slot cannot paint later: every slot carries its own
 *   generation, and a group's timer is bound to that group's generation.
 */
export type AdmissionMount = Readonly<{
  /**
   * The owning consumer's scheduler identity, supplied as the object it already
   * holds. Groups never cross this boundary: two schedulers with the same group
   * name keep separate queues. Absent means the slot paints on its own.
   */
  scope?: unknown;
  /** The owner's real request group. Absent means this slot paints on its own. */
  group?: string;
  /** The owner's real request order in that group; lower is earlier on screen. */
  order?: number;
}>;

export type AdmissionSlot = Readonly<{
  /** The slot's own image decoded. Paints at most once, and only while it is live. */
  ready: (paint: () => void) => void;
  /** The slot failed, left the viewport, was replaced or unmounted. */
  retire: () => void;
}>;

export type PaintAdmission = Readonly<{
  mount: (target: AdmissionMount) => AdmissionSlot;
  dispose: () => void;
  /** Test seam: live registrations, grouped or independent. */
  size: () => number;
}>;

export type AdmissionOptions = Readonly<{
  stepMs?: number;
  now?: () => number;
  setTimer?: (callback: () => void, waitMs: number) => unknown;
  clearTimer?: (timer: unknown) => void;
}>;

type Entry = {
  order: number;
  seq: number;
  live: boolean;
  ready: boolean;
  paint?: () => void;
};

type Group = {queue: Entry[]; timer?: unknown; admittedAt?: number; generation: number};

const DEFAULT_STEP_MS = 40;

export function createPaintAdmission(options: AdmissionOptions = {}): PaintAdmission {
  const stepMs = options.stepMs ?? DEFAULT_STEP_MS;
  const now = options.now ?? (() => Date.now());
  const setTimer = options.setTimer ?? ((callback, wait) => setTimeout(callback, wait));
  const clearTimer = options.clearTimer ?? (timer => clearTimeout(timer as ReturnType<typeof setTimeout>));
  // scope -> group name -> queue. The scope is the owning consumer's own identity;
  // nothing here invents one, and a mount without a scope never shares a queue.
  const scoped = new Map<unknown, Map<string, Group>>();
  let seq = 0, live = 0, disposed = false;

  const remove = (scope: unknown, name: string, entry: Entry) => {
    entry.live = false;
    live = Math.max(0, live - 1);
    const group = groupOf(scope, name);
    if (!group) return;
    group.queue = group.queue.filter(candidate => candidate.live);
    flush(scope, name);
  };

  const groupOf = (scope: unknown, name: string): Group | undefined => scoped.get(scope)?.get(name);

  const stop = (group: Group) => {
    if (group.timer !== undefined) {
      try {clearTimer(group.timer);} catch { /* the timer is gone either way */ }
      group.timer = undefined;
    }
  };

  const prune = (scope: unknown, name: string, group: Group) => {
    if (group.queue.length) return;
    stop(group);
    const names = scoped.get(scope);
    if (names?.get(name) === group) {
      names.delete(name);
      if (!names.size) scoped.delete(scope);
    }
  };

  const flush = (scope: unknown, name: string) => {
    if (disposed) return;
    const group = groupOf(scope, name);
    if (!group) return;
    group.queue = group.queue.filter(entry => entry.live);
    const head = group.queue[0];
    if (!head) {prune(scope, name, group); return;}
    // A mounted predecessor that has not decoded yet is the pending predecessor. Wait
    // for its own owner; a failed or offscreen one is retired, never inferred.
    if (!head.ready || !head.paint) {stop(group); return;}
    if (group.admittedAt !== undefined) {
      const remaining = stepMs - (now() - group.admittedAt);
      if (remaining > 0) {
        if (group.timer === undefined) {
          const generation = group.generation;
          group.timer = setTimer(() => {
            group.timer = undefined;
            if (disposed || groupOf(scope, name) !== group || group.generation !== generation) return;
            flush(scope, name);
          }, remaining);
        }
        return;
      }
    }
    stop(group);
    group.queue.shift();
    group.admittedAt = now();
    const paint = head.paint;
    head.live = false;
    live = Math.max(0, live - 1);
    try {paint();} catch { /* one paint callback never blocks its siblings */ }
    if (group.queue.length) flush(scope, name); else prune(scope, name, group);
  };

  return {
    mount(target) {
      if (disposed) return {ready() {}, retire() {}};
      const scope = target.scope;
      const name = typeof target.group === 'string' && target.group !== '' ? target.group : undefined;
      const grouped = name !== undefined && scope !== undefined && scope !== null;
      const entry: Entry = {
        order: typeof target.order === 'number' && Number.isFinite(target.order) ? target.order : Number.MAX_SAFE_INTEGER,
        seq: seq++, live: true, ready: false,
      };
      live++;
      if (!grouped) {
        // No real group from the owner: this slot paints independently, exactly once.
        return {
          ready(paint) {
            if (disposed || !entry.live) return;
            entry.live = false;
            live = Math.max(0, live - 1);
            try {paint();} catch { /* a paint callback never changes admission */ }
          },
          retire() {
            if (!entry.live) return;
            entry.live = false;
            live = Math.max(0, live - 1);
          },
        };
      }
      let names = scoped.get(scope);
      if (!names) {names = new Map(); scoped.set(scope, names);}
      let group = names.get(name!);
      if (!group) {group = {queue: [], generation: 0}; names.set(name!, group);}
      group.queue.push(entry);
      group.queue.sort((left, right) => left.order - right.order || left.seq - right.seq);
      flush(scope, name!);
      return {
        ready(paint) {
          if (disposed || !entry.live) return;
          entry.ready = true;
          entry.paint = paint;
          flush(scope, name!);
        },
        retire() {
          if (!entry.live) return;
          remove(scope, name!, entry);
        },
      };
    },
    dispose() {
      disposed = true;
      for (const names of scoped.values()) {
        for (const group of names.values()) {
          stop(group);
          group.generation++;
          for (const entry of group.queue) entry.live = false;
          group.queue = [];
        }
      }
      scoped.clear();
      live = 0;
    },
    size() {return live;},
  };
}
