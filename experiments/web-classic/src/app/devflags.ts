import {setLatencyFactor} from '@/data/fixtures/util';

/**
 * Review switches read once from the URL and kept for the tab:
 * `?latency=0` turns simulated network latency off, `?latency=3` slows it;
 * `?latency=1` restores the default.
 */
export function readDevFlags() {
  let factor = 1;
  try {
    const url = new URL(location.href);
    const raw = url.searchParams.get('latency');
    if (raw !== null) sessionStorage.setItem('portico.latency', raw);
    const kept = sessionStorage.getItem('portico.latency');
    if (kept !== null && !Number.isNaN(Number(kept))) factor = Number(kept);
  } catch { /* ignore */ }
  setLatencyFactor(factor);
}
