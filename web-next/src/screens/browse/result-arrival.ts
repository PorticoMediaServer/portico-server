/** The accepted-result witness the browse collection already publishes: a new
 * generation only after its first page swapped in, the pending marker while a
 * replacement is loading, and the settled state. Presentation reads these; it
 * never invents a query generation, count or clock. */
export type ResultWitness = Readonly<{
  generation: number;
  total: number | undefined;
  refreshing: boolean;
  settled: boolean;
}>;

export type ResultArrival = Readonly<{durationMs: number; easing: 'ease' | 'linear'}>;

export function resultWitness(snapshot: Readonly<{generation: number; total: number | undefined; refreshing: boolean; status: string}>): ResultWitness {
  return Object.freeze({generation: snapshot.generation, total: snapshot.total, refreshing: snapshot.refreshing, settled: snapshot.status === 'ready'});
}

/** The authored result crossfade (motion §3.8): 180 standard when a genuinely
 * accepted answer lands, 160 linear opacity-only when the app or system asks
 * for reduction. The first render is immediate; a pending replacement starts
 * nothing, and a quiet update that keeps the same generation and total does not
 * restart the paint. */
export function resultArrival(previous: ResultWitness | undefined, current: ResultWitness, reducedMotion: boolean): ResultArrival | undefined {
  if (!previous || !current.settled || current.refreshing) return undefined;
  if (previous.generation === current.generation && previous.total === current.total) return undefined;
  return Object.freeze(reducedMotion ? {durationMs: 160, easing: 'linear' as const} : {durationMs: 180, easing: 'ease' as const});
}
