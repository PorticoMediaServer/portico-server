// Resolves what the player is showing: titles, tracks, chapters, next episode.
import type { AudioTrack, Chapter, SubtitleTrack } from '../../data/types';
import { episodes, homeVideos, movies, shows } from '../../data/fixtures/index';
import { channels, programs, recordings } from '../../data/fixtures/live';

export interface PlayerMeta {
  id: string;
  kind: 'movie' | 'episode' | 'homevideo' | 'channel' | 'recording' | 'extra';
  title: string;
  subtitle?: string;
  backHref: string;
  image?: string;
  logo?: string;
  audio: AudioTrack[];
  subtitles: SubtitleTrack[];
  chapters: Chapter[];
  introEnd?: number;
  creditsStart?: number;
  next?: { id: string; title: string; subtitle: string; image: string };
  season?: { showId: string; number: number; episodes: { id: string; number: number; title: string; image: string; runtime: number }[] };
  version: { label: string; video: string; audio: string; container: string; bitrate: number };
  live?: { channelId: string; number: string; name: string; programTitle: string; start: number; end: number; color: string; logoText: string };
  stream: string;
}

const STEREO: AudioTrack[] = [{ id: 'a0', language: 'en', label: 'English · AAC Stereo', codec: 'AAC', channels: '2.0', isDefault: true }];

export function resolveMeta(id: string, extra?: string | null): PlayerMeta | null {
  const m = movies.find((x) => x.id === id);
  if (m) {
    const v = m.versions[0];
    if (extra) {
      const x = m.extras.find((e) => e.id === extra);
      return {
        id: m.id,
        kind: 'extra',
        title: x?.title ?? 'Trailer',
        subtitle: m.title,
        backHref: `/movie/${m.id}`,
        image: m.art.backdrop,
        audio: STEREO,
        subtitles: v.subtitles.slice(0, 3),
        chapters: [],
        version: { label: '1080p', video: 'H.264 1080p', audio: 'AAC 2.0', container: 'MP4', bitrate: 8000 },
        stream: 'sintel-trailer',
      };
    }
    return {
      id: m.id,
      kind: 'movie',
      title: m.title,
      subtitle: `${m.year} · ${m.contentRating}`,
      backHref: `/movie/${m.id}`,
      image: m.art.backdrop,
      logo: m.art.logo,
      audio: v.audio,
      subtitles: v.subtitles,
      chapters: m.chapters,
      creditsStart: m.runtime > 20 ? m.runtime * 60 - 300 : undefined,
      version: { label: v.label, video: `${v.videoCodec} ${v.resolution}${v.hdr ? ` ${v.hdr}` : ''}`, audio: v.audio[0].label.split(' · ')[1] ?? v.audio[0].codec, container: v.container, bitrate: v.bitrateKbps },
      stream: m.streamId,
    };
  }
  const e = episodes.find((x) => x.id === id);
  if (e) {
    const show = shows.find((s) => s.id === e.showId)!;
    const all = show.seasons.flatMap((s) => s.episodes);
    const i = all.findIndex((x) => x.id === id);
    const nx = all[i + 1];
    const season = show.seasons.find((s) => s.number === e.season)!;
    const v = e.versions[0];
    return {
      id: e.id,
      kind: 'episode',
      title: e.title,
      subtitle: `${show.title} · S${e.season} E${e.number}`,
      backHref: `/show/${show.id}?season=${e.season}`,
      image: e.art.thumb,
      logo: show.art.logo,
      audio: v.audio,
      subtitles: v.subtitles,
      chapters: e.chapters,
      introEnd: e.introEnd,
      creditsStart: e.creditsStart,
      next: nx ? { id: nx.id, title: nx.title, subtitle: `S${nx.season} · E${nx.number}`, image: nx.art.thumb! } : undefined,
      season: { showId: show.id, number: season.number, episodes: season.episodes.map((x) => ({ id: x.id, number: x.number, title: x.title, image: x.art.thumb!, runtime: x.runtime })) },
      version: { label: v.label, video: `${v.videoCodec} ${v.resolution}${v.hdr ? ` ${v.hdr}` : ''}`, audio: v.audio[0].label.split(' · ')[1] ?? v.audio[0].codec, container: v.container, bitrate: v.bitrateKbps },
      stream: 'coffee-run',
    };
  }
  const hv = homeVideos.find((x) => x.id === id);
  if (hv)
    return {
      id: hv.id,
      kind: 'homevideo',
      title: hv.title,
      subtitle: new Date(hv.recordedAt).toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' }),
      backHref: `/home-video/${hv.id}`,
      image: hv.art.thumb,
      audio: [{ id: 'a0', language: 'en', label: 'Original · AAC Stereo', codec: 'AAC', channels: '2.0', isDefault: true }],
      subtitles: [],
      chapters: [],
      version: { label: '4K HLG', video: 'HEVC 2160p HLG', audio: 'AAC 2.0', container: 'MOV', bitrate: 48000 },
      stream: 'coffee-run',
    };
  const ch = channels.find((c) => c.id === id);
  if (ch) {
    const now = Date.now();
    const p = programs.find((x) => x.channelId === ch.id && Date.parse(x.start) <= now && Date.parse(x.end) > now);
    return {
      id: ch.id,
      kind: 'channel',
      title: p?.title ?? ch.name,
      subtitle: `${ch.number} ${ch.name}${p?.episodeTitle ? ` · ${p.episodeTitle}` : ''}`,
      backHref: '/live',
      image: p?.image,
      audio: [{ id: 'a0', language: 'en', label: 'English · AC3 5.1', codec: 'AC3', channels: '5.1', isDefault: true }, { id: 'a1', language: 'es', label: 'Spanish (SAP) · AC3 Stereo', codec: 'AC3', channels: '2.0' }],
      subtitles: [{ id: 'cc1', language: 'en', label: 'English (CC1)', format: 'CC' }],
      chapters: [],
      version: { label: ch.hd ? '1080i' : '480i', video: ch.source === 'IPTV' ? 'H.264 1080p (HLS)' : 'MPEG-2 1080i → H.264', audio: 'AC3 5.1', container: 'MPEG-TS', bitrate: ch.hd ? 14000 : 4000 },
      live: { channelId: ch.id, number: ch.number, name: ch.name, programTitle: p?.title ?? ch.name, start: p ? Date.parse(p.start) : now, end: p ? Date.parse(p.end) : now + 3600_000, color: ch.logoColor, logoText: ch.logoText },
      stream: ch.streamId,
    };
  }
  const r = recordings.find((x) => x.id === id);
  if (r)
    return {
      id: r.id,
      kind: 'recording',
      title: r.title,
      subtitle: `${r.episodeTitle ? `${r.episodeTitle} · ` : ''}Recorded from ${r.channelName}`,
      backHref: '/live/dvr',
      image: r.image,
      audio: STEREO,
      subtitles: [{ id: 'cc1', language: 'en', label: 'English (CC1)', format: 'CC' }],
      chapters: [{ title: 'Start', start: 0 }, { title: 'Part 2', start: r.duration * 0.32 }, { title: 'Part 3', start: r.duration * 0.66 }],
      version: { label: 'DVR 1080i', video: 'H.264 1080p (remuxed)', audio: 'AC3 5.1', container: 'MKV', bitrate: 12000 },
      stream: 'coffee-run',
    };
  return null;
}

// Descriptive (SDH-style) captions: fit any footage, so they read naturally over the fixture streams.
const CUES: Record<string, string[]> = {
  en: ['[soft piano music]', '[footsteps on wet pavement]', '[distant traffic]', '[door creaks open]', '♪ gentle strings swell ♪', '[phone buzzes]', '[wind gusting]', '[laughter in the distance]', '[coffee machine hisses]', '[music intensifies]'],
  es: ['[música suave de piano]', '[pasos sobre el pavimento mojado]', '[tráfico a lo lejos]', '[la puerta cruje al abrirse]', '♪ cuerdas suaves ♪', '[vibra un teléfono]', '[ráfagas de viento]', '[risas a lo lejos]', '[sisea la cafetera]', '[la música se intensifica]'],
  fr: ['[musique douce au piano]', '[pas sur le trottoir mouillé]', '[circulation au loin]', '[une porte grince]', '♪ les cordes s’élèvent ♪', '[un téléphone vibre]', '[rafales de vent]', '[rires au loin]', '[la machine à café siffle]', '[la musique s’intensifie]'],
  de: ['[sanfte Klaviermusik]', '[Schritte auf nassem Pflaster]', '[Verkehr in der Ferne]', '[eine Tür knarrt]', '♪ sanfte Streicher ♪', '[ein Handy vibriert]', '[Windböen]', '[Lachen in der Ferne]', '[die Kaffeemaschine zischt]', '[die Musik schwillt an]'],
  ja: ['［穏やかなピアノの音楽］', '［濡れた歩道を歩く足音］', '［遠くの車の音］', '［ドアがきしむ］', '♪ 弦楽器が高まる ♪', '［電話が振動する］', '［風が吹きつける］', '［遠くで笑い声］', '［コーヒーマシンの音］', '［音楽が高まる］'],
  sv: ['[mjuk pianomusik]', '[steg på våt asfalt]', '[trafik på avstånd]', '[en dörr knarrar]', '♪ stråkar stiger ♪', '[en telefon vibrerar]', '[vindbyar]', '[skratt på avstånd]', '[kaffemaskinen väser]', '[musiken stegras]'],
  it: ['[musica di pianoforte]', '[passi sul selciato bagnato]', '[traffico in lontananza]', '[una porta cigola]', '♪ archi delicati ♪', '[un telefono vibra]', '[raffiche di vento]', '[risate in lontananza]', '[la macchina del caffè sibila]', '[la musica cresce]'],
};

export function cueAt(lang: string, t: number): string | null {
  const list = CUES[lang] ?? CUES.en;
  const slot = Math.floor(t / 9);
  const within = t - slot * 9;
  if (within > 4.2) return null;
  return list[slot % list.length];
}
