import { describe, expect, it } from 'vitest';
import type { MediaItem } from '../../data/models';
import { compactSavedLabel, playActionLabel, savedAriaLabel, savedMenuLabel, selectedSavedCopy } from './actionLabels';

function item(kind: MediaItem['kind'], type: MediaItem['type'], title = 'Example'): MediaItem {
  return { id: kind, title, subtitle: '', year: 0, type, kind, poster: '', backdrop: '', rating: '', length: '', genre: '' };
}

describe('context-aware media action labels', () => {
  it('uses listening language for artists, albums, and tracks', () => {
    const artist = item('artist', 'music', 'Bonobo');
    const album = item('album', 'music', 'Black Sands');
    const track = item('track', 'music', 'Kiara');
    expect(playActionLabel(artist)).toBe('Play mix');
    expect(playActionLabel(album)).toBe('Play album');
    expect(playActionLabel(track)).toBe('Play');
    expect(compactSavedLabel(album, false)).toBe('Save');
    expect(savedMenuLabel(track, true)).toBe('Remove from saved');
    expect(savedAriaLabel(artist, false)).toBe('Save Bonobo');
  });

  it('keeps established video watchlist language', () => {
    const movie = item('movie', 'movie', 'Arrival');
    expect(playActionLabel(movie)).toBe('Play');
    expect(compactSavedLabel(movie, true)).toBe('In watchlist');
    expect(savedMenuLabel(movie, false)).toBe('Add to watchlist');
    expect(savedAriaLabel(movie, false)).toBe('Add Arrival to watchlist');
  });

  it('uses music-aware bulk copy only for an all-listening selection', () => {
    expect(selectedSavedCopy([item('album', 'music'), item('track', 'music')])).toEqual({ label: 'Save', notice: 'Saved.' });
    expect(selectedSavedCopy([item('album', 'music'), item('movie', 'movie')])).toEqual({ label: 'Watchlist', notice: 'Added to watchlist.' });
  });
});
