import type { AudioTrack, CastCredit, Chapter, CrewCredit, MediaVersion, Palette, Quality, SubtitleTrack } from '../types.ts';
import palettes from '../generated/palettes.json' with { type: 'json' };

const DAY = 86_400_000;

/** Fixture clock. Everything relative is anchored here so the demo always feels current. */
export const NOW = Date.now();

export const daysAgo = (d: number, hour = 20) => {
  const t = new Date(NOW - d * DAY);
  t.setHours(hour, (d * 37) % 60, 0, 0);
  return t.toISOString();
};
export const hoursAgo = (h: number) => new Date(NOW - h * 3_600_000).toISOString();
export const minutesAgo = (m: number) => new Date(NOW - m * 60_000).toISOString();

export const slug = (s: string) =>
  s
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/&/g, 'and')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '');

/** Small deterministic PRNG so fixture numbers are stable between reloads. */
export function rng(seed: string | number) {
  let h = typeof seed === 'number' ? seed : 0;
  if (typeof seed === 'string') for (let i = 0; i < seed.length; i++) h = (Math.imul(31, h) + seed.charCodeAt(i)) | 0;
  let a = h >>> 0 || 1;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const fallbackPalette: Palette = { base: '#12314a', accent: '#5cb3e2' };
const pal = palettes as Record<string, Palette>;

export const paletteFor = (artPath: string | undefined): Palette => (artPath && pal[artPath]) || fallbackPalette;

export const art = {
  poster: (id: string) => `/art/poster/${id}.webp`,
  backdrop: (id: string) => `/art/backdrop/${id}.webp`,
  logo: (id: string) => `/art/logo/${id}.webp`,
  still: (id: string) => `/art/still/${id}.webp`,
  cover: (id: string) => `/art/cover/${id}.webp`,
  artist: (id: string) => `/art/artist/${id}.webp`,
  photo: (id: string) => `/art/photo/${id}.webp`,
  photoThumb: (id: string) => `/art/photo-thumb/${id}.webp`,
  program: (id: string) => `/art/program/${id}.webp`,
};

// ---------- People ----------

export const personId = (name: string) => `p-${slug(name)}`;

export const castList = (pairs: [string, string][]): CastCredit[] =>
  pairs.map(([name, role]) => ({ personId: personId(name), name, role }));

export const crewList = (pairs: [string, string][]): CrewCredit[] =>
  pairs.map(([job, name]) => ({ personId: personId(name), name, job }));

// ---------- Media versions ----------

const subs = (langs: [string, string][], format: SubtitleTrack['format']): SubtitleTrack[] =>
  langs.map(([language, label], i) => ({ id: `s${i}`, language, label, format }));

const LANG_LABEL: Record<string, string> = {
  en: 'English',
  es: 'Spanish',
  fr: 'French',
  de: 'German',
  ja: 'Japanese',
  it: 'Italian',
  pt: 'Portuguese',
  sv: 'Swedish',
  no: 'Norwegian',
  da: 'Danish',
  nl: 'Dutch',
  ko: 'Korean',
};

export function versionsFor(id: string, quality: Quality, runtimeMin: number, lang = 'en'): MediaVersion[] {
  const r = rng(id);
  const secs = runtimeMin * 60;
  const orig = LANG_LABEL[lang] ?? 'English';
  const dub: AudioTrack[] =
    lang === 'en'
      ? []
      : [{ id: 'a9', language: 'en', label: 'English (Dub)', codec: 'E-AC3', channels: '5.1' }];

  const subsFull = subs(
    [
      ['en', 'English'],
      ['en', 'English (SDH)'],
      ['es', 'Spanish'],
      ['fr', 'French'],
      ['de', 'German'],
      ['ja', 'Japanese'],
      ['sv', 'Swedish'],
    ],
    'PGS',
  ).map((s, i) => (i === 1 ? { ...s, sdh: true } : s));
  if (lang !== 'en') {
    subsFull.unshift({ id: 'sf', language: 'en', label: 'English (Forced)', format: 'SRT', forced: true, isDefault: true });
  }

  const mk = (
    vid: string,
    label: string,
    resolution: MediaVersion['resolution'],
    codec: MediaVersion['videoCodec'],
    kbps: number,
    container: MediaVersion['container'],
    audio: AudioTrack[],
    subtitles: SubtitleTrack[],
    hdr?: MediaVersion['hdr'],
  ): MediaVersion => ({
    id: `${id}-${vid}`,
    label,
    resolution,
    videoCodec: codec,
    hdr,
    bitrateKbps: kbps,
    container,
    sizeBytes: Math.round((kbps * 1000 * secs) / 8),
    path: `/volume1/media/${resolution === '2160p' ? 'UHD' : 'HD'}/${id}/${id}.${container.toLowerCase()}`,
    audio,
    subtitles,
  });

  const atmos: AudioTrack[] = [
    { id: 'a0', language: lang, label: `${orig} · Dolby TrueHD Atmos`, codec: 'TrueHD', channels: '7.1', isDefault: true },
    { id: 'a1', language: lang, label: `${orig} · Dolby Digital`, codec: 'AC3', channels: '5.1' },
    ...dub,
    { id: 'a2', language: 'en', label: "Director's Commentary", codec: 'AAC', channels: '2.0', commentary: true },
  ];
  const dts: AudioTrack[] = [
    { id: 'a0', language: lang, label: `${orig} · DTS-HD Master Audio`, codec: 'DTS-HD MA', channels: '5.1', isDefault: true },
    ...dub,
    { id: 'a1', language: 'es', label: 'Spanish · Dolby Digital', codec: 'AC3', channels: '5.1' },
  ];
  const stereo: AudioTrack[] = [{ id: 'a0', language: lang, label: `${orig} · AAC`, codec: 'AAC', channels: '2.0', isDefault: true }];

  switch (quality) {
    case '4k-dv':
      return [
        mk('uhd', '4K · Dolby Vision', '2160p', 'HEVC', 58000 + Math.round(r() * 14000), 'MKV', atmos, subsFull, 'Dolby Vision'),
        mk('hd', '1080p', '1080p', 'H.264', 9800 + Math.round(r() * 3000), 'MP4', atmos.slice(1), subsFull.slice(0, 4).map((s) => ({ ...s, format: 'SRT' as const }))),
      ];
    case '4k-hdr':
      return [
        mk('uhd', '4K · HDR10', '2160p', 'HEVC', 44000 + Math.round(r() * 16000), 'MKV', atmos, subsFull, 'HDR10'),
        mk('hd', '1080p', '1080p', 'HEVC', 7200 + Math.round(r() * 2400), 'MKV', dts, subsFull.slice(0, 3)),
      ];
    case '1080p':
      return [mk('hd', '1080p', '1080p', r() > 0.5 ? 'HEVC' : 'H.264', 8000 + Math.round(r() * 7000), 'MKV', dts, subs([['en', 'English'], ['en', 'English (SDH)'], ['es', 'Spanish'], ['fr', 'French']], 'SRT'))];
    case '720p':
      return [mk('hd', '720p', '720p', 'H.264', 3200 + Math.round(r() * 1500), 'MP4', stereo, subs([['en', 'English'], ['es', 'Spanish']], 'SRT'))];
    case 'sd':
      return [
        mk('hd', '1080p Restoration', '1080p', 'H.264', 6200 + Math.round(r() * 2000), 'MKV', [{ id: 'a0', language: lang, label: `${orig} · Mono (Restored)`, codec: 'FLAC', channels: '1.0', isDefault: true }, { id: 'a1', language: 'en', label: 'Orchestral Score', codec: 'AC3', channels: '5.1' }], subs([['en', 'English Intertitles'], ['fr', 'French'], ['de', 'German']], 'SRT')),
      ];
  }
}

export function numberedChapters(runtimeMin: number, names?: string[]): Chapter[] {
  const secs = runtimeMin * 60;
  const n = names?.length ?? Math.max(6, Math.min(16, Math.round(runtimeMin / 9)));
  return Array.from({ length: n }, (_, i) => ({
    title: names?.[i] ?? `Chapter ${i + 1}`,
    start: i === 0 ? 0 : Math.round((secs * i) / n + ((i * 7919) % 97) - 48),
  }));
}

export const fmtSize = (bytes: number) => {
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  let i = 0;
  let v = bytes;
  while (v >= 1000 && i < u.length - 1) {
    v /= 1000;
    i++;
  }
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${u[i]}`;
};
