import { describe, expect, it } from 'vitest';
import { libraryChannelQualityOptions } from './LibraryChannelOperations';
import { playbackQualityPreferenceOptions } from './PersonalSettings';

describe('Web-owned quality labels', () => {
  it('uses friendly library-channel labels instead of contract ids', () => {
    expect(libraryChannelQualityOptions).toEqual([
      { value: 'auto', label: 'Automatic' },
      { value: 'original', label: 'Original Quality' },
      { value: '1080p-medium', label: '1080p · 5 Mbps' },
      { value: '720p-medium', label: '720p · 2.5 Mbps' },
      { value: '480p', label: '480p · 1.5 Mbps' },
    ]);
  });

  it('uses canonical names for locally owned playback preferences', () => {
    expect(playbackQualityPreferenceOptions).toContainEqual({
      value: 'automatic',
      label: 'Automatic',
    });
    expect(playbackQualityPreferenceOptions).toContainEqual({
      value: 'original',
      label: 'Original Quality',
    });
  });
});
