/** Root §23: the explicit Send app log carries a bounded, closed, code-only journal.
 * Codes are a reviewed static vocabulary mirrored on the server; no regex, dynamic
 * suffix, hashed identifier or free text is accepted. Entries are client-reported
 * timestamps only, kept for the current signed-in context. */
export const APP_LOG_CODES = Object.freeze([
  'audio_transition_unavailable',
  'audio_edge_late',
  'audio_processing_unavailable',
  'equalizer_filter_unavailable',
  'audio_autoplay_blocked',
] as const);

export type AppLogCode = (typeof APP_LOG_CODES)[number];
export type AppLogLevel = 'info' | 'warn' | 'error';
export type AppLogEntry = Readonly<{atMs: number; level: AppLogLevel; code: AppLogCode}>;

const MAX_ENTRIES = 200;
let context: string | undefined, entries: AppLogEntry[] = [];
const listeners = new Set<() => void>();

/** The journal belongs to one signed-in server/account/profile; a change clears it. */
export function setAppLogContext(next: string | undefined): void {
  if (context === next) return;
  context = next;
  entries = [];
  for (const listener of [...listeners]) listener();
}

export function noteAppLog(code: AppLogCode, level: AppLogLevel = 'warn'): void {
  if (!context) return;
  entries = [...entries.slice(-(MAX_ENTRIES - 1)), {atMs: Date.now(), level, code}];
  for (const listener of [...listeners]) listener();
}

export function appLogSnapshot(): readonly AppLogEntry[] { return entries; }
export function subscribeAppLog(listener: () => void): () => void { listeners.add(listener); return () => { listeners.delete(listener); }; }
export function clearAppLog(): void { entries = []; for (const listener of [...listeners]) listener(); }
