/** Formatting helpers (US English). */
export function formatTime(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const r = s % 60;
  return h ? `${h}:${String(m).padStart(2, '0')}:${String(r).padStart(2, '0')}` : `${m}:${String(r).padStart(2, '0')}`;
}
export function formatDuration(sec?: number): string {
  if (!sec) return '';
  const m = Math.round(sec / 60);
  if (m < 60) return `${m} min`;
  return m % 60 ? `${Math.floor(m / 60)} hr ${m % 60} min` : `${Math.floor(m / 60)} hr`;
}
export function remaining(sec: number, progress: number): string {
  const left = Math.max(1, Math.round((sec * (1 - progress)) / 60));
  return left >= 60 ? (left % 60 ? `${Math.floor(left / 60)} hr ${left % 60} min left` : `${Math.floor(left / 60)} hr left`) : `${left} min left`;
}
export const clock = (t: number) => new Date(t).toLocaleTimeString('en-US', {hour: 'numeric', minute: '2-digit'});
export const day = (t: number) => new Date(t).toLocaleDateString('en-US', {weekday: 'short', month: 'short', day: 'numeric'});
export const date = (t: number) => new Date(t).toLocaleDateString('en-US', {month: 'short', day: 'numeric', year: 'numeric'});
export function ago(t: number): string {
  const m = Math.round((Date.now() - t) / 60000);
  if (m < 1) return 'Just now';
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h} hr ago`;
  const d = Math.round(h / 24);
  if (d === 1) return 'Yesterday';
  if (d < 7) return `${d} days ago`;
  return date(t);
}
export const plural = (n: number, one: string, many = `${one}s`) => `${n.toLocaleString('en-US')} ${n === 1 ? one : many}`;
export const count = (n: number) => n.toLocaleString('en-US');
