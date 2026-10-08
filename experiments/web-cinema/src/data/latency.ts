/**
 * Simulated network latency for the fixture backend, so skeletons and transitions are real.
 * `?latency=0` turns it off, `?latency=2` doubles it; the choice sticks for the browser session.
 */
let factor = 1;
try {
  const q = new URLSearchParams(location.search).get('latency');
  if (q !== null) sessionStorage.setItem('portico.latency', q);
  const v = Number(q ?? sessionStorage.getItem('portico.latency') ?? '1');
  factor = Number.isFinite(v) && v >= 0 ? v : 1;
} catch {
  /* storage unavailable: keep the default */
}

export const latencyFactor = () => factor;
export function setLatencyFactor(v: number) {
  factor = v;
  try { sessionStorage.setItem('portico.latency', String(v)); } catch { /* ignore */ }
}

export function latency(ms: number): Promise<void> {
  if (factor === 0) return Promise.resolve();
  return new Promise(r => setTimeout(r, ms * factor));
}
