import type {Recording} from '@core/dvr.ts';

/**
 * FEAT-08: group Recorded by the recording's `seriesId` (else `programme.seriesId`) into show rows
 * titled by `seriesTitle` (else the first programme's title) that expand to episodes. Series without an id (movies, one-offs) stay as single rows keyed by recording id.
 * Order follows the guide (first appearance wins).
 */
export function groupRecordings(recordings: readonly Recording[]): {key: string; title: string; items: Recording[]}[] {
  const order: string[] = [];
  const byKey = new Map<string, Recording[]>();
  for (const r of recordings) {
    const key = seriesKey(r);
    if (!byKey.has(key)) { byKey.set(key, []); order.push(key); }
    byKey.get(key)!.push(r);
  }
  return order.map(key => {
    const items = byKey.get(key)!;
    return {key, title: items.find(r => r.seriesTitle)?.seriesTitle || items[0].programme.title, items};
  });
}

/** The recording's show: the server's `seriesId`, else the programme's; one-offs are their own row. */
export function seriesKey(r: Pick<Recording, 'id' | 'seriesId' | 'programme'>): string {
  const series = r.seriesId || r.programme.seriesId;
  return series || `one:${r.id}`;
}

/** FEAT-08: badges only for exceptions in Recorded (Failed, Incomplete; Kept is separate). */
export function isExceptionBadge(r: Pick<Recording, 'state'>): boolean {
  return r.state === 'failed' || r.state === 'incomplete-playable';
}

/** FEAT-02: padding choices (seconds) and keep-count choices for record/rule dialogs, bounded by
 * `validRecordingOptions` (before/after 0–21600 s, episodeLimit 0–100000). Shared by `DVR.tsx`
 * and the guide `ProgramSheet` so both offer the same bounded values. */
export const dvrPaddings = [0, 60, 120, 300, 600, 900];
export const dvrLimits = [0, 1, 3, 5, 10, 20];
