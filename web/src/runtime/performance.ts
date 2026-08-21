const prefix = 'portico:web:';

export function markWebTiming(name: string): void {
  try {
    if (typeof performance === 'undefined' || typeof performance.mark !== 'function') return;
    performance.mark(`${prefix}${name}`);
  } catch {
    // Diagnostics must never affect startup or recovery behavior.
  }
}

export function measureWebTiming(name: string, start: string, end: string): void {
  try {
    if (typeof performance === 'undefined' || typeof performance.measure !== 'function') return;
    performance.measure(`${prefix}${name}`, `${prefix}${start}`, `${prefix}${end}`);
  } catch {
    // A missing mark is expected in tests and restored-tab navigation.
  }
}
