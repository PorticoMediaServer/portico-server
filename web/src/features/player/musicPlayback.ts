import type { MediaItem, PlaybackRepeatMode } from '@porticomediaserver/client-core';
import type { MusicPlaybackPreferences } from '../../data/models';

export const defaultMusicPlaybackPreferences: MusicPlaybackPreferences = {
  shuffleDefault: false,
  repeatDefault: 'none',
  autoplayDefault: true,
  normalizationMode: 'off',
  crossfadeSeconds: 0,
  gapless: true,
};

export function normalizeMusicPlaybackPreferences(value: MusicPlaybackPreferences | undefined): MusicPlaybackPreferences {
  if (!value) return defaultMusicPlaybackPreferences;
  const crossfadeSeconds = Number.isFinite(value.crossfadeSeconds)
    ? Math.max(0, Math.min(12, Math.floor(value.crossfadeSeconds)))
    : 0;
  return {
    shuffleDefault: value.shuffleDefault === true,
    repeatDefault: value.repeatDefault === 'one' || value.repeatDefault === 'all' ? value.repeatDefault : 'none',
    autoplayDefault: value.autoplayDefault !== false,
    normalizationMode: value.normalizationMode === 'attenuate' ? 'attenuate' : 'off',
    crossfadeSeconds,
    gapless: crossfadeSeconds > 0 || value.gapless !== false,
  };
}

export function accountRepeatMode(value: MusicPlaybackPreferences['repeatDefault']): PlaybackRepeatMode {
  return value === 'one' || value === 'all' ? value : 'off';
}

export function musicArtist(item: MediaItem): string {
  return firstText(
    item.typedMetadata?.albumArtist,
    item.typedMetadata?.artist,
    item.typedMetadata?.artistName,
    item.grandparentTitle,
    item.parentTitle,
  );
}

export function musicAlbum(item: MediaItem): string {
  return firstText(
    item.typedMetadata?.albumTitle,
    item.typedMetadata?.albumName,
    item.parentTitle,
  );
}

function firstText(...values: Array<string | undefined>): string {
  return values.find((value) => value?.trim())?.trim() ?? '';
}
