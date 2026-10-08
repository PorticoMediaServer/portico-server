/**
 * The client's quality request (spec §5.2). Only the client's request and admin/profile
 * ceilings decide quality: the server never lowers it on its own. "Automatic" is a client-side
 * choice made here from the viewer's `quality.<network>.*` preferences and the client's own
 * bandwidth estimate, and changed later with `PATCH` when the estimate moves decisively.
 */

export type NetworkClass = 'local' | 'remote' | 'cellular';
/** The preference registry's per-network lanes (`quality.<lane>.*`). */
export type QualityLane = 'local' | 'wifi' | 'cellular' | 'unknown';
export type QualityMode = 'off' | 'automatic' | 'original' | 'high' | 'standard' | 'data-saver';

export type QualityPreferences = Readonly<{
  mode: QualityMode;
  maxVideoHeight?: number;
  maxVideoBitrateMbps?: number;
  maxAudioBitrateKbps?: number;
  allowHDR?: boolean;
}>;

/** The `quality` field of `POST /v1/playback/sessions` (spec §5.1). */
export type QualityRequest =
  | Readonly<{mode: 'original'}>
  | Readonly<{mode: 'limit'; maxVideoBitrateKbps?: number; maxHeight?: number; maxAudioBitrateKbps?: number}>;

export type QualityDecision =
  | Readonly<{allowed: false; reason: 'network_off'}>
  | Readonly<{allowed: true; quality: QualityRequest; network: NetworkClass; rung?: number}>;

/** Automatic rungs, highest first: height and the video bit rate a stream at that height needs. */
export const qualityLadder: readonly Readonly<{height: number; kbps: number}>[] = Object.freeze([
  {height: 2160, kbps: 40_000}, {height: 1440, kbps: 20_000}, {height: 1080, kbps: 8000},
  {height: 720, kbps: 4000}, {height: 480, kbps: 1500}, {height: 360, kbps: 800},
]);

/** Which preference lane applies: the home network, Wi-Fi away from home, cellular, or unknown. */
export function qualityLane(network: NetworkClass, onWifi?: boolean): QualityLane {
  if (network === 'local') return 'local';
  if (network === 'cellular') return 'cellular';
  return onWifi === true ? 'wifi' : 'unknown';
}

/** Read one lane's preferences from the server's snapshot (`value(key, fallback)`). */
export function qualityPreferences(value: <T>(key: string, fallback: T) => T, lane: QualityLane): QualityPreferences {
  const p = `quality.${lane}.`;
  const mode = value<string>(p + 'mode', 'automatic');
  const known: readonly QualityMode[] = ['off', 'automatic', 'original', 'high', 'standard', 'data-saver'];
  return Object.freeze({
    mode: (known as readonly string[]).includes(mode) ? mode as QualityMode : 'automatic',
    maxVideoHeight: value<number>(p + 'maxVideoHeight', 0) || undefined,
    maxVideoBitrateMbps: value<number>(p + 'maxVideoBitrateMbps', 0) || undefined,
    maxAudioBitrateKbps: value<number>(p + 'maxAudioBitrateKbps', 0) || undefined,
    allowHDR: value<boolean>(p + 'allowHDR', true),
  });
}

const min = (a: number | undefined, b: number | undefined) => (a === undefined ? b : b === undefined ? a : Math.min(a, b));

function capped(prefs: QualityPreferences, height?: number, kbps?: number, audioKbps?: number): QualityRequest {
  const maxHeight = min(prefs.maxVideoHeight, height);
  const maxVideoBitrateKbps = min(prefs.maxVideoBitrateMbps !== undefined ? prefs.maxVideoBitrateMbps * 1000 : undefined, kbps);
  const maxAudioBitrateKbps = min(prefs.maxAudioBitrateKbps, audioKbps);
  return Object.freeze({mode: 'limit', ...(maxVideoBitrateKbps !== undefined ? {maxVideoBitrateKbps} : {}), ...(maxHeight !== undefined ? {maxHeight} : {}), ...(maxAudioBitrateKbps !== undefined ? {maxAudioBitrateKbps} : {})});
}

/**
 * The rung "Automatic" should use for a bandwidth estimate. Down-switches are immediate (use at
 * most 80 % of the estimate); an up-switch needs 25 % headroom over that, so a noisy estimate
 * doesn't flap between rungs. `previous` is the rung index currently in use.
 */
export function automaticRung(estimateKbps: number, previous?: number, capHeight?: number): number {
  const usable = estimateKbps * 0.8;
  let rung = qualityLadder.findIndex(r => r.kbps <= usable && (capHeight === undefined || r.height <= capHeight));
  if (rung < 0) rung = qualityLadder.length - 1;
  if (previous !== undefined && rung < previous) {
    // Going up: climb only while each higher rung has 25 % headroom.
    let up = previous;
    for (let r = previous - 1; r >= rung; r--) {
      if (usable >= qualityLadder[r]!.kbps * 1.25) up = r;
      else break;
    }
    return up;
  }
  return rung;
}

/** What to ask the server for, on this network, with these preferences. */
export function qualityDecision(prefs: QualityPreferences, network: NetworkClass, estimateKbps?: number, previousRung?: number): QualityDecision {
  switch (prefs.mode) {
    case 'off': return Object.freeze({allowed: false, reason: 'network_off'});
    case 'original': return Object.freeze({allowed: true, quality: Object.freeze({mode: 'original'}), network});
    case 'high': return Object.freeze({allowed: true, quality: capped(prefs), network});
    case 'standard': return Object.freeze({allowed: true, quality: capped(prefs, 1080, 8000), network});
    case 'data-saver': return Object.freeze({allowed: true, quality: capped(prefs, 480, 1500, 128), network});
    case 'automatic': {
      if (estimateKbps === undefined || !(estimateKbps > 0)) return Object.freeze({allowed: true, quality: capped(prefs), network});
      const rung = automaticRung(estimateKbps, previousRung, prefs.maxVideoHeight);
      const r = qualityLadder[rung]!;
      return Object.freeze({allowed: true, quality: capped(prefs, r.height, r.kbps), network, rung});
    }
  }
}

/** Whether two requests differ (so the client should `PATCH` the session). */
export function sameQuality(a: QualityRequest, b: QualityRequest): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}
