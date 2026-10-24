import {defaultI18n} from '@i18n';
import {formatBehindSec} from '@core/presentation/index.ts';
import type {LiveChromePort, LiveChromeWindow} from './live-chrome-types';

const t = defaultI18n.t;
const clamp = (value: number) => Math.min(100, Math.max(0, value));

/** The rail is drawn across the programme's wall-clock span; the bright band is the actual
 * retained window from tuning in to now, the dashed remainder is still ahead of the source. */
export type LiveRail = Readonly<{gone: number; buffer: number; played: number; future: number; edge: number; knob: number}>;
export function liveRail(window: LiveChromeWindow): LiveRail {
  const from = window.programmeStartMs ?? window.seekStartMs, to = window.programmeEndMs ?? window.seekEndMs;
  const span = to - from;
  if (!(span > 0)) return {gone: 0, buffer: 0, played: 0, future: 100, edge: 100, knob: 100};
  const percent = (value: number) => clamp((value - from) / span * 100);
  const start = percent(window.seekStartMs), position = percent(window.positionMs), edge = percent(window.liveEdgeMs);
  return {gone: start, buffer: Math.max(0, edge - start), played: Math.max(0, position - start), future: Math.max(0, 100 - edge), edge, knob: position};
}

/** "8:00 to 8:15 PM" and "4:03 PM" from the same locale/time-zone as the player. */
export function liveTimeRange(startMs: number, endMs: number, format: (ms: number) => string): string {
  const from = format(startMs), to = format(endMs);
  const meridiem = from.match(/ (AM|PM)$/i)?.[1];
  const same = meridiem !== undefined && meridiem === to.match(/ (AM|PM)$/i)?.[1];
  return t('guide.cellTime', {start: same ? from.slice(0, -(meridiem.length + 1)) : from, end: to});
}

export function liveBehindLabel(seconds: number): string | undefined {
  if (!(seconds > 2)) return undefined;
  return t('player.live.behindLive', {duration: formatBehindSec(seconds)});
}

export function liveRecordLabel(programme: LiveChromePort['programme']): string {
  return programme?.recording ? t('next.kit.recording') : t('guide.record');
}

export function liveNeighbour(port: LiveChromePort, direction: -1 | 1) {
  return direction === -1 ? port.down : port.up;
}

/** The next programme line is the session's actual next airing, never a title guessed from the guide page. */
export function liveNextLine(port: LiveChromePort): string | undefined {
  return port.nextProgramme ? t('next.live.then', {title: port.nextProgramme.title}) : undefined;
}
