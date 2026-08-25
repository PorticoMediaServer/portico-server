/**
 * Retry scheduling shared by Hosted control-plane clients.
 *
 * A persisted installation identifier is intentionally only a scheduling
 * cohort. It is never sent as authorization and never changes a retry's
 * eligibility. The delay remains positive so a retry cannot spin in a tight
 * loop, while Retry-After is supplied by the caller as the lower bound.
 */
export function positiveFullJitterDelay(
  capMilliseconds: number,
  cohort = "",
  attempt = 0,
  random: () => number = Math.random,
): number {
  const cap = Math.max(1, Math.floor(capMilliseconds));
  // A persisted cohort should produce the same schedule after a process or
  // fleet restart. Per-process entropy would re-cluster clients unpredictably
  // and makes an otherwise bounded interactive recovery time flaky.
  const sample = cohort.trim()
    ? stableCohortHash(`${cohort}:${attempt}`) / 0x100000000
    : Math.min(0.999999999, Math.max(0, random()));
  return 1 + Math.floor(sample * cap);
}

export function stableCohortHash(value: string): number {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}
