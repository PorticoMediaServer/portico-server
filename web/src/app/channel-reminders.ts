/** In-app programme reminders for the signed-in browser. */
export type Reminder = {programId: string; channelId: string; title: string; channelName: string; start: number};
const REMINDER_KEY = 'portico.channels.reminders.v1';
const reminderListeners = new Set<() => void>();
export function readReminders(): Reminder[] {
  try {
    const list = JSON.parse(localStorage.getItem(REMINDER_KEY) ?? '[]') as Reminder[];
    // A reminder lapses five minutes after its programme starts; every other
    // reminder is kept, however many there are.
    return Array.isArray(list) ? list.filter(r => r && typeof r.programId === 'string' && typeof r.start === 'number' && r.start > Date.now() - 5 * 60_000) : [];
  } catch { return []; }
}
function saveReminders(list: Reminder[]) {
  try { localStorage.setItem(REMINDER_KEY, JSON.stringify(list)); } catch {}
  for (const l of reminderListeners) l();
}
export const hasReminder = (programId: string) => readReminders().some(r => r.programId === programId);
export function toggleReminder(r: Reminder): boolean {
  const list = readReminders();
  const on = list.some(x => x.programId === r.programId);
  saveReminders(on ? list.filter(x => x.programId !== r.programId) : [...list, r]);
  return !on;
}
export function removeReminder(programId: string) { saveReminders(readReminders().filter(r => r.programId !== programId)); }
export function subscribeReminders(fn: () => void) { reminderListeners.add(fn); return () => { reminderListeners.delete(fn); }; }
