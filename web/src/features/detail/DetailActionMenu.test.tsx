import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { DataProvider } from '../../data/DataProvider';
import { FixturePorticoDataSource } from '../../data/fixtureSource';
import { MediaVersionsDialog } from './DetailActionMenu';

describe('detail browser downloads', () => {
  it('offers only available versions and hands a direct grant to the browser', async () => {
    const source = new FixturePorticoDataSource();
    vi.spyOn(source, 'mediaDownloadOptions').mockResolvedValue({
      canDownload: true,
      defaultProfile: 'source',
      optimizedVersions: [],
      profiles: [],
      options: [
        { id: 'source', kind: 'source', profile: 'source', label: 'Original source', available: true },
        { id: 'pending', kind: 'optimized', profile: '1080p', label: '1080p optimized', available: false },
      ],
    });
    const grant = vi.spyOn(source, 'createMediaDownloadURL').mockResolvedValue('/api/media/movie/download/file?grant=browser');
    let handedAnchor: HTMLAnchorElement | undefined;
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) { handedAnchor = this; });
    const notice = vi.fn();

    render(<DataProvider source={source}>
      <MediaVersionsDialog item={{ id: 'movie', title: 'Movie', entityKind: 'movie', actions: ['download'] }} mode="download" onDismiss={vi.fn()} onNotice={notice} onChanged={vi.fn()} />
    </DataProvider>);

    expect(await screen.findByText('Original source')).toBeInTheDocument();
    expect(screen.queryByText('1080p optimized')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Download' }));

    await waitFor(() => expect(grant).toHaveBeenCalledWith('movie', 'source', expect.any(AbortSignal)));
    expect(handedAnchor).toMatchObject({
      download: '',
      rel: 'noreferrer',
      referrerPolicy: 'no-referrer',
    });
    expect(handedAnchor?.getAttribute('href')).toBe('/api/media/movie/download/file?grant=browser');
    expect(notice).toHaveBeenCalledWith(expect.objectContaining({ tone: 'success', title: 'Sent to browser' }));
    expect(handedAnchor && document.body.contains(handedAnchor)).toBe(false);
  });
});
