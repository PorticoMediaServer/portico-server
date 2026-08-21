import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { LyricsEditor } from './LyricsEditor';

function setup() {
  const actions = {
    onUpload: vi.fn().mockResolvedValue(undefined),
    onFetch: vi.fn().mockResolvedValue(undefined),
    onSearch: vi.fn().mockResolvedValue([{ provider: 'lrclib', externalId: 'match-1', trackName: 'Kiara', artistName: 'Bonobo', albumName: 'Black Sands', durationSeconds: 230, format: 'lrc', synced: true }]),
    onApply: vi.fn().mockResolvedValue(undefined),
    onDelete: vi.fn().mockResolvedValue(undefined),
  };
  render(<LyricsEditor
    lyrics={[{ id: 'lyrics-1', source: 'provider', provider: 'lrclib', format: 'lrc', synced: true, language: 'en', createdAt: '2026-07-01T00:00:00Z' }]}
    defaultQuery="Kiara Bonobo"
    {...actions}
  />);
  return actions;
}

describe('LyricsEditor', () => {
  it('uploads, finds, searches, applies, and removes lyrics through production callbacks', async () => {
    const actions = setup();

    const file = new File(['[00:00.00] Kiara'], 'kiara.lrc', { type: 'text/plain' });
    fireEvent.change(document.querySelector('.technical-file-input')!, { target: { files: [file] } });
    await waitFor(() => expect(actions.onUpload).toHaveBeenCalledWith(file, 'en'));

    fireEvent.click(screen.getByRole('button', { name: 'Find automatically' }));
    await waitFor(() => expect(actions.onFetch).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByRole('button', { name: 'Search' }));
    expect(await screen.findByText('Bonobo · Black Sands · Synchronized · 3:50')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Use lyrics' }));
    await waitFor(() => expect(actions.onApply).toHaveBeenCalledWith(expect.objectContaining({ externalId: 'match-1' })));

    fireEvent.click(screen.getByRole('button', { name: 'Remove lyrics' }));
    await waitFor(() => expect(actions.onDelete).toHaveBeenCalledWith('lyrics-1'));
  });
});
