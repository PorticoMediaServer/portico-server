// Simulated network conditions for the fixture backend.
//
//   ?latency=fast | normal | slow | hang   (default normal: 140–480 ms)
//   ?fail=all | <method name>              (reject calls, to review error states)
//
// Values are remembered for the tab in sessionStorage so navigation keeps them.

type Mode = 'fast' | 'normal' | 'slow' | 'hang';

function readParam(name: string): string | null {
  try {
    const v = new URLSearchParams(window.location.search).get(name);
    if (v !== null) {
      sessionStorage.setItem(`portico.${name}`, v);
      return v;
    }
    return sessionStorage.getItem(`portico.${name}`);
  } catch {
    return null;
  }
}

export function networkMode(): Mode {
  const v = readParam('latency');
  return v === 'fast' || v === 'slow' || v === 'hang' ? v : 'normal';
}

export function failing(method: string): boolean {
  const v = readParam('fail');
  if (!v || v === 'none') return false;
  return v === 'all' || v.split(',').includes(method);
}

export class ApiError extends Error {
  status: number;
  constructor(message: string, status = 503) {
    super(message);
    this.status = status;
  }
}

const range: Record<Mode, [number, number]> = { fast: [0, 20], normal: [140, 480], slow: [900, 2200], hang: [1e9, 1e9] };

export function simulate<T>(method: string, work: () => T, weight = 1): Promise<T> {
  const [lo, hi] = range[networkMode()];
  const ms = lo + Math.random() * (hi - lo) * weight;
  return new Promise((resolve, reject) => {
    setTimeout(() => {
      if (failing(method)) {
        reject(new ApiError('Harbor didn’t respond. It may be restarting or offline.'));
        return;
      }
      try {
        resolve(structuredClone(work()));
      } catch (e) {
        reject(e);
      }
    }, Math.min(ms, 2_000_000_000));
  });
}
