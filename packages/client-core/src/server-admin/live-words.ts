/**
 * Live TV in words for the owner pages: what a guide or playlist import left out and why, and
 * what is using a tuner. The reasons are the server's catalogue keys; one this app has no words
 * for is left out, never shown as a key.
 */
import type {MessageId, MessageValues} from '../../../i18n/src/index.ts';

type Translate = (id: MessageId, values?: MessageValues) => string;
type Has = (id: string) => boolean;

export type ImportSkipped = Readonly<{/** "12 channels and 340 programs were left out. 5 program times were adjusted." */ summary: string; /** Why, in sentences, each once. */ reasons: readonly string[]}>;

const count = (v: unknown): number => (typeof v === 'number' && Number.isSafeInteger(v) && v > 0 ? v : 0);

/** What an import left out, or nothing when it left nothing out. The document is read part by part: an unreadable part counts as nothing. */
export function importSkipped(t: Translate, has: Has, diagnostics: unknown): ImportSkipped | undefined {
  const d = diagnostics && typeof diagnostics === 'object' ? (diagnostics as Record<string, unknown>) : {};
  const channels = count(d.skippedChannels), programmes = count(d.skippedProgrammes), adjusted = count(d.adjustedProgrammes);
  if (!channels && !programmes && !adjusted) return undefined;
  const parts = [
    channels && programmes ? t('server.live.skippedBoth', {channels, programs: programmes}) : channels ? t('server.live.skippedChannels', {count: channels}) : programmes ? t('server.live.skippedProgrammes', {count: programmes}) : '',
    adjusted ? t('server.live.adjusted', {count: adjusted}) : '',
  ].filter(Boolean);
  const reasons: string[] = [];
  for (const key of Array.isArray(d.examples) ? d.examples.slice(0, 10) : []) {
    if (typeof key !== 'string' || !key.startsWith('live.import.') || !has(key)) continue;
    const words = t(key as MessageId);
    if (!reasons.includes(words)) reasons.push(words);
  }
  return {summary: parts.join(' '), reasons};
}

/** One thing using a tuner: "Being watched · BBC One", "Recording · Match of the Day", or when a recording starts. */
export function tunerUseLine(t: Translate, use: Readonly<{kind: string; state: string; title: string; startsAt: string}>, when: (iso: string) => string): string {
  const what = use.kind === 'live' ? t('server.live.beingWatched') : use.state === 'recording' || use.kind === 'recording' && !use.startsAt ? t('server.live.recordingNow') : use.startsAt ? when(use.startsAt) : t('server.live.recordingNow');
  return [what, use.title, use.state === 'retiring' ? t('server.live.finishing') : ''].filter(Boolean).join(' · ');
}

/** How busy a source's tuners are, as its status says it. */
export function tunerStatus(t: Translate, source: Readonly<{conflicts: number; inUse: number; tunerCount: number}>): {tone: 'healthy' | 'warning' | 'danger'; text: string} {
  if (source.conflicts) return {tone: 'danger', text: t('server.live.clashes', {count: source.conflicts})};
  return {tone: source.tunerCount > 0 && source.inUse >= source.tunerCount ? 'warning' : 'healthy', text: t('server.live.tunersInUse', {used: source.inUse, total: source.tunerCount})};
}
