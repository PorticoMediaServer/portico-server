/** localStorage that never throws (private windows, blocked storage). */
export const store = {
  get<T>(key: string, fallback: T): T {
    try { const v = localStorage.getItem(`portico.${key}`); return v === null ? fallback : (JSON.parse(v) as T); } catch { return fallback; }
  },
  set(key: string, value: unknown) {
    try { localStorage.setItem(`portico.${key}`, JSON.stringify(value)); } catch { /* storage unavailable */ }
  },
  remove(key: string) {
    try { localStorage.removeItem(`portico.${key}`); } catch { /* storage unavailable */ }
  },
};
