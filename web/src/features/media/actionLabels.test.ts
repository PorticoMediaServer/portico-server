import { describe, expect, it } from 'vitest';
import type { MediaItem } from '../../data/models';
import { compactSavedLabel, playActionLabel, savedAriaLabel, savedMenuLabel, selectedSavedCopy } from './actionLabels';

function item(entityKind: MediaItem['entityKind'], title = 'Example'): MediaItem {
  return { id: entityKind, title, subtitle: '', year: 0, entityKind, poster: '', backdrop: '', rating: '', length: '', genre: '' };
}

describe('context-aware media action labels', () => {
  it('uses listening language for artists, albums, and tracks', () => {
    const artist = item('artist', 'Bonobo');
    const album = item('album', 'Black Sands');
    const track = item('track', 'Kiara');
    expect(playActionLabel(artist)).toBe('Play mix');
    expect(playActionLabel(album)).toBe('Play album');
    expect(playActionLabel(track)).toBe('Play');
    expect(compactSavedLabel(album, false)).toBe('Save');
    expect(savedMenuLabel(track, true)).toBe('Remove from saved');
    expect(savedAriaLabel(artist, false)).toBe('Save Bonobo');
  });

  it('keeps established video watchlist language', () => {
    const movie = item('movie', 'Arrival');
    expect(playActionLabel(movie)).toBe('Play');
    expect(compactSavedLabel(movie, true)).toBe('In watchlist');
    expect(savedMenuLabel(movie, false)).toBe('Add to watchlist');
    expect(savedAriaLabel(movie, false)).toBe('Add Arrival to watchlist');
  });

  it('uses music-aware bulk copy only for an all-listening selection', () => {
    expect(selectedSavedCopy([item('album'), item('track')])).toEqual({ label: 'Save', notice: 'Saved.' });
    expect(selectedSavedCopy([item('album'), item('movie')])).toEqual({ label: 'Watchlist', notice: 'Added to watchlist.' });
  });
});
