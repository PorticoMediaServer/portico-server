/**
 * The v1 device capability profile (spec §3, `PUT /v1/me/devices/current/capabilities`) built from
 * what a client already knows for its client profile (`client-profile.ts`: the declared table for
 * its family, narrowed by probes), plus what only the v1 profile says: the form factor, the network
 * class and `audioDecode`, the pairs the client's own audio engine decodes (spec §18.1: the server
 * plays those `direct` and converts the rest to FLAC or Opus). Shared by every platform; each
 * passes its own `audioDecode` (only pairs that pass the §18.8 fixtures there).
 */
import type {ClientProfile, DynamicRange} from '../client-profile.ts';
import type {AudioDecodeCapability} from './audio-render.ts';
import type {CapabilityProfile} from './options.ts';

export type CapabilityOptions = Readonly<{
  form: CapabilityProfile['form'];
  network: CapabilityProfile['network'];
  audioDecode?: readonly AudioDecodeCapability[];
  features?: Readonly<Record<string, boolean>>;
}>;

/** The §3 names for a dynamic range (Dolby Vision by profile). */
function hdr(ranges: readonly DynamicRange[] | undefined, dolbyVision?: readonly number[]): string[] {
  const out: string[] = [];
  for (const r of ranges ?? []) {
    if (r === 'sdr') continue;
    if (r === 'dolby_vision') {
      for (const p of dolbyVision?.length ? dolbyVision : [5]) out.push(...(p === 8 ? ['dolbyvision:8.1', 'dolbyvision:8.4'] : [`dolbyvision:${p}`]));
    } else out.push(r);
  }
  return [...new Set(out)];
}

const defined = <T extends Record<string, unknown>>(o: T): T => Object.fromEntries(Object.entries(o).filter(([, v]) => v !== undefined)) as T;

export function capabilityProfileFromClient(client: ClientProfile, options: CapabilityOptions): CapabilityProfile {
  const transports = client.transports ?? [];
  const direct = transports.filter(t => t.transport === 'direct');
  const containers = [...new Set(direct.flatMap(t => t.containers ?? []))].map(container => Object.freeze({container, direct: true}));
  const video = (client.video ?? []).map(v => Object.freeze(defined({
    codec: v.codec,
    profiles: v.profiles?.length ? v.profiles : undefined,
    maxLevel: v.maxLevel || undefined,
    maxBitDepth: v.bitDepths?.length ? Math.max(...v.bitDepths) : undefined,
    maxWidth: v.maxWidth || undefined,
    maxHeight: v.maxHeight || undefined,
    maxFps: v.maxFrameRate || undefined,
    maxBitrateKbps: v.maxBitrateBps ? Math.round(v.maxBitrateBps / 1000) : undefined,
    hdr: hdr(v.dynamicRanges, v.dolbyVisionProfiles),
  })));
  const audio = (client.audio ?? []).map(a => Object.freeze(defined({
    codec: a.codec,
    maxChannels: a.maxChannels || undefined,
    maxSampleRate: a.maxSampleRate || undefined,
    passthrough: a.passthrough === true,
    atmos: !!a.objectAudio?.includes('atmos'),
  })));
  const subtitles = [
    ...(client.subtitles?.text ?? []).map(format => ({format, render: 'native' as const})),
    ...(client.subtitles?.styled ?? []).map(format => ({format, render: 'client' as const})),
    ...(client.subtitles?.bitmap ?? []).map(format => ({format, render: 'native' as const})),
  ].map(s => Object.freeze(s));
  const display = client.display ? Object.freeze(defined({hdr: hdr(client.display.dynamicRanges), maxWidth: client.display.width || undefined, maxHeight: client.display.height || undefined})) : undefined;
  return Object.freeze(defined({
    form: options.form,
    network: Object.freeze({...options.network}),
    containers: Object.freeze(containers),
    streaming: Object.freeze({hls: Object.freeze({fmp4: transports.some(t => t.transport === 'hls_fmp4'), ts: transports.some(t => t.transport === 'hls_ts')}), progressive: direct.length > 0}),
    video: Object.freeze(video),
    display,
    audio: Object.freeze(audio),
    audioDecode: options.audioDecode ? Object.freeze(options.audioDecode.map(d => Object.freeze({...d, containers: Object.freeze([...d.containers])}))) : undefined,
    subtitles: Object.freeze(subtitles),
    features: Object.freeze({...(options.features ?? {})}),
  })) as CapabilityProfile;
}

/** Local when the server answers on a private or link-local address or a `.local` name. */
export function networkClassOf(origin: string | null | undefined, cellular = false): CapabilityProfile['network'] {
  if (cellular) return Object.freeze({class: 'cellular'});
  let host = '';
  try { host = origin ? new URL(origin).hostname.replace(/^\[|\]$/g, '') : ''; } catch { host = ''; }
  const v4 = /^\d{1,3}(\.\d{1,3}){3}$/.test(host) ? host.split('.').map(Number) : undefined;
  // An IPv6 literal (it has a colon): loopback, unique-local fc00::/7 or link-local fe80::/10.
  // A hostname that merely starts with "fd" (fdroid.example.com) is not one.
  const v6 = host.includes(':') ? host.toLowerCase() : undefined;
  const local = host === 'localhost' || host.endsWith('.local')
    || (!!v4 && v4.every(n => n <= 255) && (v4[0] === 127 || v4[0] === 10 || (v4[0] === 192 && v4[1] === 168) || (v4[0] === 172 && v4[1]! >= 16 && v4[1]! <= 31) || (v4[0] === 169 && v4[1] === 254)))
    || (!!v6 && (v6 === '::1' || /^f[cd][0-9a-f]{2}:/.test(v6) || /^fe[89ab][0-9a-f]:/.test(v6)));
  return Object.freeze({class: local ? 'local' : 'remote'});
}
