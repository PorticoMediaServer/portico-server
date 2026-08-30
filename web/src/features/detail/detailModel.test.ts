import { describe, expect, it } from 'vitest';
import type { MediaItem } from '../../data/models';
import {
  detailKindLabel,
  detailLibraryDestination,
  detailMetaParts,
  formatDetailBytes,
} from './detailModel';

function media(overrides: Partial<MediaItem> = {}): MediaItem {
  return {
    id: 'media-1',
    title: 'Media title',
    subtitle: '',
    year: 0,
    entityKind: 'movie',
    poster: '',
    backdrop: '',
    rating: '',
    length: '',
    genre: '',
    ...overrides,
  };
}

describe('detail Product Language presentation', () => {
  it('resolves canonical kind and breadcrumb labels, including safe future-kind fallback', () => {
    expect(detailKindLabel(media({ entityKind: 'collection' }))).toBe('Collection');
    expect(detailLibraryDestination(media({ entityKind: 'playlist' }))).toEqual({ path: '/saved?tab=playlists', label: 'Playlists' });
    expect(detailKindLabel(media({ entityKind: 'future-kind' as MediaItem['entityKind'] }))).toBe('Media');
  });

  it('uses canonical metadata messages for episodic, music, audiobook, and technical copy', () => {
    expect(detailMetaParts(media({ entityKind: 'episode', seasonNumber: 2, episodeNumber: 10 }))).toContain('S2 E10');
    expect(detailMetaParts(media({ entityKind: 'track', typedMetadata: { discNumber: '2', trackNumber: '4' } }))).toContain('Disc 2 · Track 4');
    expect(detailMetaParts(media({ entityKind: 'audiobook', typedMetadata: { narrator: 'LeVar Burton', series: 'Earthsea', seriesPosition: '3' } }))).toEqual(expect.arrayContaining(['Narrated by LeVar Burton', 'Earthsea · Book 3']));
    expect(detailMetaParts(media({ criticRating: 91 }))).toContain('91% critics');
    expect(formatDetailBytes(0)).toBe('Size unavailable');
  });
});
