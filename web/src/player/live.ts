import type {GuideChannel, GuideProgramme} from '@core/channel-guide.ts';
import {miniGuideNeighbors, nowNextProgrammes, sortChannelsByNumber} from '@core/presentation/index.ts';

/**
 * MU4 FEAT-07: the channels this player tuned, in guide order. The guide's
 * loaded page lives outside the player (MU3), so the mini guide works on the
 * tune history it saw itself: bounded, number-ordered, never a whole lineup.
 */
export type LiveProgramme = Readonly<{id: string; title: string; startMs: number; endMs: number}>;

const MAX_TUNED = 50;
let tuned: GuideChannel[] = [];

/** Remember a tuned channel (every `engine.tune` calls this). */
export function noteTunedChannel(channel: GuideChannel): void {
  tuned = [channel, ...tuned.filter(c => !(c.id === channel.id && c.sourceId === channel.sourceId))].slice(0, MAX_TUNED);
}

/** Tuned channels, ordered by channel number. */
export function tunedChannels(): GuideChannel[] {
  return sortChannelsByNumber(tuned);
}

/** The tuned channel matching a playback channel reference, if this player tuned it. */
export function tunedChannelFor(ref: Readonly<{channelId: string; sourceId: string}> | undefined): GuideChannel | undefined {
  if (!ref) return undefined;
  return tuned.find(c => c.id === ref.channelId && c.sourceId === ref.sourceId);
}

/** A channel's programmes as millis (malformed entries dropped). */
export function liveProgrammes(channel: GuideChannel): LiveProgramme[] {
  const out: LiveProgramme[] = [];
  for (const p of channel.programmes) {
    const startMs = Date.parse(p.start), endMs = Date.parse(p.end);
    if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= startMs) continue;
    out.push({id: p.id, title: p.title, startMs, endMs});
  }
  return out;
}

/** The programme on now on a tuned channel (null outside programmes or guide gaps). */
export function currentProgramme(channel: GuideChannel, nowMs = Date.now()): GuideProgramme | null {
  for (const p of channel.programmes) {
    const startMs = Date.parse(p.start), endMs = Date.parse(p.end);
    if (Number.isFinite(startMs) && Number.isFinite(endMs) && startMs <= nowMs && nowMs < endMs) return p;
  }
  return null;
}

/** Mini-guide rows over the tune history: previous/current/next with now/next programmes. */
export function miniGuideRows(nowMs = Date.now()): {previous: GuideChannel | null; current: GuideChannel | null; next: GuideChannel | null; programmes: (channel: GuideChannel) => {now: LiveProgramme | null; next: LiveProgramme | null}} {
  // Channel ids repeat across sources, so neighbours are picked by source/channel key.
  const entries = tunedChannels().map(channel => ({channel, id: `${channel.sourceId}/${channel.id}`}));
  const last = tuned[0];
  const found = miniGuideNeighbors(entries, last ? `${last.sourceId}/${last.id}` : '');
  return {previous: found.previous?.channel ?? null, current: found.current?.channel ?? null, next: found.next?.channel ?? null, programmes: channel => nowNextProgrammes(liveProgrammes(channel), nowMs)};
}
