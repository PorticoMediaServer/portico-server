import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { MediaItem } from '../../data/models';
import { TechnicalMediaEditor } from './TechnicalMediaEditor';

const item: MediaItem = {
  id: 'fargo',
  title: 'Fargo',
  subtitle: 'The Castle',
  year: 2015,
  entityKind: 'episode',
  poster: '/poster.jpg',
  backdrop: '/backdrop.jpg',
  rating: 'TV-MA',
  length: '48m',
  genre: 'Drama',
  actions: ['media.optimize'],
  availability: 'available',
  fileCount: 1,
  streams: [
    { id: 'video-1', kind: 'video', codec: 'hevc', displayTitle: '4K HEVC', width: 3840, height: 2160, bitDepth: 10 },
    { id: 'subtitle-1', kind: 'subtitle', codec: 'webvtt', displayTitle: 'English SDH', language: 'en', sourceUrl: '/subtitle.vtt', subtitleOffsetMs: 0 },
  ],
};

function setup(loadOptions = vi.fn().mockResolvedValue({
  canDownload: true,
  defaultProfile: '1080p-high',
  optimizedVersions: [{ id: 'version-1', profile: '720p-medium', profileName: '720p balanced', sizeBytes: 1000, available: true, createdAt: '2026-07-01T00:00:00Z', updatedAt: '2026-07-01T00:00:00Z' }],
  profiles: [
    { id: '720p-medium', label: '720p balanced', height: 720, videoKbps: 3200, audioKbps: 192 },
    { id: '1080p-high', label: '1080p high quality', height: 1080, videoKbps: 8000, audioKbps: 320 },
  ],
  options: [{ id: 'source', kind: 'source', label: 'Original source', available: true, sizeBytes: 1000, container: 'mkv', videoCodec: 'hevc', audioCodec: 'eac3', sourceKind: 'local' }],
}), mediaItem: MediaItem = item) {
  const actions = {
    loadOptions,
    onUploadSubtitle: vi.fn().mockResolvedValue(undefined),
    onUpdateSubtitle: vi.fn().mockResolvedValue(undefined),
    onDeleteSubtitle: vi.fn().mockResolvedValue(undefined),
    onCreateVersion: vi.fn().mockResolvedValue(undefined),
    onDeleteVersion: vi.fn().mockResolvedValue(undefined),
  };
  render(<TechnicalMediaEditor item={mediaItem} {...actions} />);
  return actions;
}

describe('TechnicalMediaEditor', () => {
  it('uses real subtitle and optimized-version callbacks', async () => {
    const actions = setup();
    await screen.findByText('MKV · HEVC · EAC3');

    fireEvent.click(screen.getByRole('button', { name: 'Timing' }));
    fireEvent.change(screen.getByLabelText('Offset (ms)'), { target: { value: '1250' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save timing' }));
    await waitFor(() => expect(actions.onUpdateSubtitle).toHaveBeenCalledWith('subtitle-1', 1250));

    fireEvent.click(screen.getByRole('button', { name: 'Remove English SDH' }));
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
    await waitFor(() => expect(actions.onDeleteSubtitle).toHaveBeenCalledWith('subtitle-1'));

    const subtitle = new File(['WEBVTT'], 'english.vtt', { type: 'text/vtt' });
    fireEvent.change(document.querySelector('.technical-file-input')!, { target: { files: [subtitle] } });
    await waitFor(() => expect(actions.onUploadSubtitle).toHaveBeenCalledWith(subtitle, 'en', ''));

    fireEvent.change(screen.getByRole('combobox', { name: 'New version' }), { target: { value: '1080p-high' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create version' }));
    await waitFor(() => expect(actions.onCreateVersion).toHaveBeenCalledWith('1080p-high'));

    fireEvent.click(screen.getByRole('button', { name: 'Remove 720p balanced' }));
    await waitFor(() => expect(actions.onDeleteVersion).toHaveBeenCalledWith('720p-medium'));
  });

  it('shows a retryable source-options failure without hiding stream management', async () => {
    const loadOptions = vi.fn().mockRejectedValueOnce(new Error('Source service unavailable')).mockResolvedValueOnce({
      canDownload: false,
      defaultProfile: '',
      optimizedVersions: [],
      profiles: [],
      options: [],
    });
    const actions = setup(loadOptions);

    expect(await screen.findByText('Source details unavailable')).toBeInTheDocument();
    expect(screen.getByText('4K HEVC')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(actions.loadOptions).toHaveBeenCalledTimes(2));
  });

  it('groups exact file and stream facts by source version', async () => {
    setup(undefined, {
      ...item,
      fileCount: 2,
      streams: [],
      mediaFiles: [
        {
          id: 'file-primary',
          path: '/media/Fargo/Season 2/The Castle.mkv',
          originalFilename: 'The Castle.mkv',
          versionLabel: 'Original 4K',
          container: 'mkv',
          sizeBytes: 18_000_000_000,
          durationSeconds: 2880,
          bitrate: 50_000_000,
          width: 3840,
          height: 2160,
          aspectRatio: '16:9',
          frameRate: 23.976,
          videoCodec: 'hevc',
          videoProfile: 'Main 10',
          bitDepth: 10,
          audioCodec: 'eac3',
          audioChannels: 6,
          audioChannelLayout: '5.1',
          audioSampleRate: 48000,
          available: true,
          selected: true,
          streams: [
            { id: 'file-primary-video', kind: 'video', codec: 'hevc', displayTitle: '4K HEVC', profile: 'Main 10', width: 3840, height: 2160, bitDepth: 10, index: 0 },
            { id: 'file-primary-audio', kind: 'audio', codec: 'eac3', displayTitle: 'English 5.1', channels: 6, channelLayout: '5.1', sampleRate: 48000, index: 1 },
          ],
        },
        {
          id: 'file-mobile',
          originalFilename: 'The Castle - 1080p.mp4',
          versionLabel: '1080p alternate',
          container: 'mp4',
          available: false,
          streams: [],
        },
      ],
    });

    expect(await screen.findByText('/media/Fargo/Season 2/The Castle.mkv')).toBeInTheDocument();
    expect(screen.getByText('The Castle.mkv')).toBeInTheDocument();
    expect(screen.getByText(/HEVC · 3840 × 2160 · 16:9 aspect · 23.976 fps · Profile Main 10 · 10-bit/)).toBeInTheDocument();
    expect(screen.getAllByText(/EAC3 · 6 channels · 5.1 · 48 kHz/)).toHaveLength(3);
    expect(screen.getByText('1080p alternate')).toBeInTheDocument();
    expect(screen.getByText('Unavailable')).toBeInTheDocument();
    expect(screen.getByText('2 streams')).toBeInTheDocument();
    expect(screen.queryByText('No analyzed streams')).not.toBeInTheDocument();
  });
});
