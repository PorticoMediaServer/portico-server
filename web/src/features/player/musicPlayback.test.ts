import { describe, expect, it } from 'vitest';
import type { MediaItem } from '@porticomediaserver/client-core';
import { accountRepeatMode, musicAlbum, musicArtist, normalizeMusicPlaybackPreferences } from './musicPlayback';

describe('music playback preferences', () => {
  it('normalizes account defaults without creating browser-local overrides', () => {
    expect(normalizeMusicPlaybackPreferences({
      shuffleDefault: true,
      repeatDefault: 'all',
      autoplayDefault: false,
      normalizationMode: 'attenuate',
      crossfadeSeconds: 99,
      gapless: false,
    })).toEqual({
      shuffleDefault: true,
      repeatDefault: 'all',
      autoplayDefault: false,
      normalizationMode: 'attenuate',
      crossfadeSeconds: 12,
      gapless: true,
    });
  });

  it('maps the account off value onto the playback queue contract', () => {
    expect(accountRepeatMode('none')).toBe('off');
    expect(accountRepeatMode('one')).toBe('one');
  });

  it('derives Media Session artist and album labels from canonical music metadata', () => {
    const item = {
      typedMetadata: { artist: 'Big Girl', albumTitle: 'A New Set of Songs' },
      parentTitle: 'Fallback album',
    } as unknown as MediaItem;
    expect(musicArtist(item)).toBe('Big Girl');
    expect(musicAlbum(item)).toBe('A New Set of Songs');
  });
});
