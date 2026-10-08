import { useEffect, useRef, useState, useSyncExternalStore } from 'react';

export function useMediaQuery(q: string): boolean {
  return useSyncExternalStore(
    (cb) => {
      const m = window.matchMedia(q);
      m.addEventListener('change', cb);
      return () => m.removeEventListener('change', cb);
    },
    () => window.matchMedia(q).matches,
    () => false,
  );
}

export const useIsMobile = () => useMediaQuery('(max-width: 767px)');
export const useIsDesktop = () => useMediaQuery('(min-width: 1100px)');
export const useReducedMotion = () => useMediaQuery('(prefers-reduced-motion: reduce)');

export function useInterval(fn: () => void, ms: number | null) {
  const saved = useRef(fn);
  saved.current = fn;
  useEffect(() => {
    if (ms === null) return;
    const id = setInterval(() => saved.current(), ms);
    return () => clearInterval(id);
  }, [ms]);
}

export function useNow(ms = 30_000) {
  const [now, setNow] = useState(() => Date.now());
  useInterval(() => setNow(Date.now()), ms);
  return now;
}

export function useScrolled(threshold = 8) {
  const [scrolled, setScrolled] = useState(false);
  useEffect(() => {
    let raf = 0;
    const on = () => {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => setScrolled(window.scrollY > threshold));
    };
    on();
    window.addEventListener('scroll', on, { passive: true });
    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener('scroll', on);
    };
  }, [threshold]);
  return scrolled;
}

export function useEvent(name: string, handler: (e: Event) => void, target: EventTarget | null = typeof window !== 'undefined' ? window : null) {
  const saved = useRef(handler);
  saved.current = handler;
  useEffect(() => {
    if (!target) return;
    const h = (e: Event) => saved.current(e);
    target.addEventListener(name, h);
    return () => target.removeEventListener(name, h);
  }, [name, target]);
}

export function useKey(key: string, handler: (e: KeyboardEvent) => void, opts: { meta?: boolean; enabled?: boolean; allowInInputs?: boolean } = {}) {
  const saved = useRef(handler);
  saved.current = handler;
  useEffect(() => {
    if (opts.enabled === false) return;
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      const typing = t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable);
      if (typing && !opts.allowInInputs && !opts.meta) return;
      if (opts.meta && !(e.metaKey || e.ctrlKey)) return;
      if (!opts.meta && (e.metaKey || e.ctrlKey || e.altKey)) return;
      if (e.key.toLowerCase() === key.toLowerCase() || e.key === key) saved.current(e);
    };
    window.addEventListener('keydown', h);
    return () => window.removeEventListener('keydown', h);
  }, [key, opts.meta, opts.enabled, opts.allowInInputs]);
}

export function useLockBody(locked: boolean) {
  useEffect(() => {
    if (!locked) return;
    const prev = document.documentElement.style.overflow;
    document.documentElement.style.overflow = 'hidden';
    return () => {
      document.documentElement.style.overflow = prev;
    };
  }, [locked]);
}

/** Relative time in the voice of the product: "Just now", "12 min ago", "Yesterday", "Mar 4". */
export function ago(iso: string | number, now = Date.now()): string {
  const t = typeof iso === 'number' ? iso : Date.parse(iso);
  const s = Math.round((now - t) / 1000);
  if (s < -60) return inFuture(-s);
  if (s < 45) return 'Just now';
  const m = Math.round(s / 60);
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h} hr ago`;
  const d = Math.round(h / 24);
  if (d === 1) return 'Yesterday';
  if (d < 7) return `${d} days ago`;
  const date = new Date(t);
  const sameYear = date.getFullYear() === new Date(now).getFullYear();
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', ...(sameYear ? {} : { year: 'numeric' }) });
}

function inFuture(s: number) {
  const m = Math.round(s / 60);
  if (m < 60) return `in ${m} min`;
  const h = Math.round(m / 60);
  if (h < 24) return `in ${h} hr`;
  const d = Math.round(h / 24);
  return d === 1 ? 'Tomorrow' : `in ${d} days`;
}

export const clockTime = (iso: string | number) =>
  new Date(iso).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });

export const fmtBytes = (bytes: number) => {
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  let i = 0;
  let v = bytes;
  while (v >= 1000 && i < u.length - 1) {
    v /= 1000;
    i++;
  }
  return `${v >= 100 || i === 0 ? v.toFixed(0) : v.toFixed(1)} ${u[i]}`;
};

export const fmtDuration = (secs: number) => {
  const h = Math.floor(secs / 3600);
  const m = Math.round((secs % 3600) / 60);
  if (h && m) return `${h} hr ${m} min`;
  if (h) return `${h} hr`;
  return `${m} min`;
};

export const fmtClock = (secs: number) => {
  const s = Math.max(0, Math.floor(secs));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const x = s % 60;
  return h ? `${h}:${String(m).padStart(2, '0')}:${String(x).padStart(2, '0')}` : `${m}:${String(x).padStart(2, '0')}`;
};

export const plural = (n: number, one: string, many = `${one}s`) => `${n.toLocaleString('en-US')} ${n === 1 ? one : many}`;
