/**
 * Paint admission for already-decoded artwork.
 *
 * The scheduler and the image source stay exactly as they are: this module owns
 * no request, no cache, no concurrency, no retention and no ordering fact. It only
 * decides *when* a decoded sibling may show its decoded paint, so individual
 * `img.decode()` completions stop racing each other into the DOM.
 *
 * The bounded shape:
 * - siblings sharing one `group` (a row or other run) are admitted in the real
 *   `request.order` the caller already passes to the scheduler, lower first;
 * - consecutive admissions in a group are spaced by `stepMs` (40 ms), so a row
 *   reveals in reading order rather than decode order;
 * - the first admission in a group is immediate, and a cached image never waits
 *   for this module at all (the caller paints it directly);
 * - a refusal, unmount or replaced image retires its token: a retired token never
 *   paints, and it never strands the siblings behind it;
 * - nothing waits for a predecessor that is undecoded, failed, unmounted or
 *   offscreen: only entries that actually became ready are ever queued.
 */
export type AdmissionRequest = Readonly<{
  /** A run to pace together (the caller's existing request group). */
  group?: string;
  /** Screen position from the caller's existing request order; lower first. */
  order?: number;
  /** Called at most once, and only while the token is still live. */
  paint: () => void;
}>;

export type PaintAdmission = Readonly<{
  request: (request: AdmissionRequest) => number;
  retire: (token: number) => void;
  dispose: () => void;
  /** Test seam: the number of live tokens, queued or admitted. */
  size: () => number;
}>;

export type AdmissionOptions = Readonly<{
  stepMs?: number;
  now?: () => number;
  setTimer?: (callback: () => void, waitMs: number) => unknown;
  clearTimer?: (timer: unknown) => void;
}>;

type Entry = {order: number; seq: number; paint: () => void; live: boolean};
type Group = {queue: Entry[]; timer?: unknown; admittedAt?: number};

const DEFAULT_STEP_MS = 40;

export function createPaintAdmission(options: AdmissionOptions = {}): PaintAdmission {
  const stepMs = options.stepMs ?? DEFAULT_STEP_MS;
  const now = options.now ?? (() => Date.now());
  const setTimer = options.setTimer ?? ((callback, wait) => setTimeout(callback, wait));
  const clearTimer = options.clearTimer ?? (timer => clearTimeout(timer as ReturnType<typeof setTimeout>));
  const groups = new Map<string, Group>();
  const tokens = new Map<number, {group: string; entry: Entry}>();
  let seq = 0, nextToken = 1, disposed = false;

  const groupOf = (name: string): Group => {
    let group = groups.get(name);
    if (!group) {group = {queue: []}; groups.set(name, group);}
    return group;
  };

  const stop = (group: Group) => {
    if (group.timer !== undefined) {try {clearTimer(group.timer);} catch { /* the timer is gone either way */ } group.timer = undefined;}
  };

  const flush = (name: string) => {
    if (disposed) return;
    const group = groups.get(name);
    if (!group) return;
    group.queue = group.queue.filter(entry => entry.live);
    const head = group.queue[0];
    if (!head) {stop(group); return;}
    if (group.admittedAt !== undefined) {
      const remaining = stepMs - (now() - group.admittedAt);
      if (remaining > 0) {
        if (group.timer === undefined) {
          group.timer = setTimer(() => {group.timer = undefined; flush(name);}, remaining);
        }
        return;
      }
    }
    stop(group);
    group.queue.shift();
    group.admittedAt = now();
    head.live = false;
    for (const [token, record] of tokens) if (record.entry === head) tokens.delete(token);
    try {head.paint();} catch { /* a paint callback never blocks its siblings */ }
    if (group.queue.length) flush(name);
  };

  return {
    request(request) {
      if (disposed) return 0;
      const entry: Entry = {order: typeof request.order === 'number' && Number.isFinite(request.order) ? request.order : Number.MAX_SAFE_INTEGER,
        seq: seq++, paint: request.paint, live: true};
      const name = request.group ?? '';
      const token = nextToken++;
      tokens.set(token, {group: name, entry});
      const group = groupOf(name);
      group.queue.push(entry);
      group.queue.sort((left, right) => left.order - right.order || left.seq - right.seq);
      flush(name);
      return token;
    },
    retire(token) {
      const record = tokens.get(token);
      if (!record) return;
      tokens.delete(token);
      record.entry.live = false;
      const group = groups.get(record.group);
      if (group) {
        group.queue = group.queue.filter(entry => entry.live);
        flush(record.group);
      }
    },
    dispose() {
      disposed = true;
      for (const group of groups.values()) stop(group);
      groups.clear();
      tokens.clear();
    },
    size() {return tokens.size;},
  };
}
