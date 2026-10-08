/** Relative time in US English: "just now", "5 min ago", "1 day ago", "in 3 hr". */
export function ago(t: number, now = Date.now()) {
  const m = Math.round((now - t) / 60000);
  const abs = Math.abs(m);
  if (abs < 2) return 'just now';
  const days = Math.round(abs / 1440);
  const s = abs < 60 ? `${abs} min` : abs < 1440 ? `${Math.round(abs / 60)} hr` : `${days} ${days === 1 ? 'day' : 'days'}`;
  return m < 0 ? `in ${s}` : `${s} ago`;
}
